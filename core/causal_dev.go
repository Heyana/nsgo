//go:build nsgodev

package core

import (
	goruntime "runtime"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// causalRingCap bounds each causal-chain ring buffer per graph.
const causalRingCap = 256

// SignalCause identifies one signal write that scheduled downstream computations.
type SignalCause struct {
	Seq        uint64
	SignalID   uint64
	Generation uint64
	Site       string
}

// TriggerRun links one signal write to a computation it scheduled and executed.
type TriggerRun struct {
	Cause       SignalCause
	Kind        string
	Owner       uint64
	Site        string
	Duration    time.Duration
	Retriggered int
}

// DisposalReason explains why a scope was disposed.
type DisposalReason string

const (
	// DisposalExplicit records a direct Scope.Dispose (or resource Close/Holder.Dispose).
	DisposalExplicit DisposalReason = "dispose"
	// DisposalRerun records disposal of a computation's owned children and cleanups before re-running.
	DisposalRerun DisposalReason = "rerun"
	// DisposalCascade records a child scope disposed because an ancestor scope was disposed.
	DisposalCascade DisposalReason = "cascade"
)

// CleanupRecord describes one cleanup executed during scope disposal.
type CleanupRecord struct {
	Label string
	Site  string
}

// DisposalRecord describes one scope disposal and the cleanups it ran.
type DisposalRecord struct {
	Seq        uint64
	RootOwner  uint64
	Owner      uint64
	OwnerName  string
	Reason     DisposalReason
	Caller     string
	TriggerSeq uint64
	Cleanups   []CleanupRecord
}

type ringBuf[T any] struct {
	items []T
	head  int
	count int
}

func (b *ringBuf[T]) push(v T, limit int) {
	if len(b.items) < limit {
		b.items = append(b.items, v)
		b.count = len(b.items)
		return
	}
	b.items[b.head] = v
	b.head = (b.head + 1) % limit
	b.count = limit
}

func (b *ringBuf[T]) recent(n int) []T {
	if b.count == 0 {
		return nil
	}
	if n <= 0 || n > b.count {
		n = b.count
	}
	skip := b.count - n
	out := make([]T, n)
	size := len(b.items)
	for i := range n {
		out[i] = b.items[(b.head+skip+i)%size]
	}
	return out
}

type disposalFrame struct {
	rootOwner  uint64
	owner      uint64
	ownerName  string
	reason     DisposalReason
	caller     string
	triggerSeq uint64
	skip       bool
	childRan   bool
	cleanups   []CleanupRecord
}

// causalState holds per-graph causal-chain diagnostics in nsgodev builds.
type causalState struct {
	causal causalRecorder
}

type causalRecorder struct {
	nextOwnerID    uint64
	nextSignalID   uint64
	nextCauseSeq   uint64
	nextDisposal   uint64
	activeCause    SignalCause
	causes         ringBuf[SignalCause]
	triggers       ringBuf[TriggerRun]
	disposals      ringBuf[DisposalRecord]
	commitTriggers []driver.Trigger
	disposalStack  []disposalFrame
}

type scopeCausal struct {
	ownerID uint64
}

type signalCausal struct {
	devID uint64
	gen   uint64
}

type compCausal struct {
	causalSite      string
	pendingCause    SignalCause
	pendingTriggers int
	lastCause       SignalCause
	lastTriggerSeq  uint64
}

type cleanupCausal struct {
	label string
	site  string
}

type soundWaitCausal struct {
	site string
}

type signalWriteToken struct {
	active bool
	prev   SignalCause
}

type compRunToken struct {
	active      bool
	cause       SignalCause
	retriggered int
	began       time.Time
}

type disposalToken struct {
	active bool
}

func (sc *signalCausal) setDevID(id uint64) {
	sc.devID = id
}

func (rt *runtime) causalActive() bool {
	return devtools.Enabled || driver.TraceCauses
}

func (rt *runtime) scopeID(s *Scope) uint64 {
	if s == nil {
		return 0
	}
	if s.ownerID == 0 {
		rt.causal.nextOwnerID++
		s.ownerID = rt.causal.nextOwnerID
	}
	return s.ownerID
}

func (rt *runtime) nextCausalSignalID() uint64 {
	if rt.dev != nil {
		rt.dev.next++
		return rt.dev.next
	}
	rt.causal.nextSignalID++
	return rt.causal.nextSignalID
}

func (rt *runtime) isDevToolsScope(o *Scope) bool {
	if o == nil {
		return false
	}
	if t := o.tree(); t != nil && t.highlights != nil && t.highlights.ownsScope(o) {
		return true
	}
	return false
}

func (rt *runtime) scopeName(s *Scope) string {
	if s == nil {
		return ""
	}
	if s == rt.root {
		return "root"
	}
	if rt.dev != nil {
		if comp, ok := rt.dev.components[s]; ok && comp.name != "" {
			return comp.name
		}
	}
	if s.comp != nil {
		return compKindName(s.comp.kind)
	}
	return "scope"
}

func (rt *runtime) compSite(c *computation) string {
	if rt.dev != nil {
		if dc := rt.dev.comps[c]; dc != nil {
			if site := rt.dev.siteOf(c, dc); site != "" {
				return site
			}
		}
	}
	return c.causalSite
}

func (rt *runtime) noteNewComputation(c *computation) {
	if rt.dev != nil || !rt.causalActive() {
		return
	}
	site := causalCallerSite(false)
	if c.kind == renderEffect && !isPath(site) {
		return
	}
	c.causalSite = site
}

// causalCallerSite resolves the first caller outside NSGo's engine/core packages (and thin wrapper packages when skipWrappers is set).
func causalCallerSite(skipWrappers bool) string {
	var pcs [16]uintptr
	n := goruntime.Callers(2, pcs[:])
	frames := goruntime.CallersFrames(pcs[:n])
	last := ""
	for {
		f, more := frames.Next()
		if f.Function == "" {
			break
		}
		if !runsApps(f.Function) && !strings.HasPrefix(f.Function, "runtime.") && (!skipWrappers || !isResourceWrapper(f.Function)) {
			if strings.HasPrefix(f.Function, nsgoPackage+"tsx") && devtools.ScriptSite != nil {
				if file, line, ok := devtools.ScriptSite(); ok {
					return pathSite(file, line)
				}
			}
			return pathSite(f.File, f.Line)
		}
		if strings.HasPrefix(f.Function, "github.com/nsgo-dev/nsgo") {
			last = strings.TrimPrefix(strings.TrimPrefix(f.Function, nsgoPackage), "github.com/nsgo-dev/")
		}
		if !more {
			break
		}
	}
	if last == "" {
		return "unknown"
	}
	return last
}

func isResourceWrapper(fn string) bool {
	return strings.HasPrefix(fn, nsgoPackage+"desktop.") ||
		strings.HasPrefix(fn, nsgoPackage+"mobile.") ||
		strings.HasPrefix(fn, nsgoPackage+"audio.")
}

func (rt *runtime) beginSignalWrite(sc *signalCausal) signalWriteToken {
	if !rt.causalActive() || rt.isDevToolsScope(rt.owner) {
		return signalWriteToken{}
	}
	if sc.devID == 0 {
		sc.devID = rt.nextCausalSignalID()
	}
	sc.gen++
	rt.causal.nextCauseSeq++
	cause := SignalCause{
		Seq:        rt.causal.nextCauseSeq,
		SignalID:   sc.devID,
		Generation: sc.gen,
		Site:       causalCallerSite(false),
	}
	rt.causal.causes.push(cause, causalRingCap)
	prev := rt.causal.activeCause
	rt.causal.activeCause = cause
	return signalWriteToken{active: true, prev: prev}
}

func (rt *runtime) endSignalWrite(tok signalWriteToken) {
	if tok.active {
		rt.causal.activeCause = tok.prev
	}
}

func (rt *runtime) noteMarked(c *computation) {
	cause := rt.causal.activeCause
	if cause.Seq == 0 {
		return
	}
	if c.pendingCause.Seq == cause.Seq {
		return
	}
	c.pendingCause = cause
	c.pendingTriggers++
}

func (c *computation) clearPendingCause() {
	c.pendingCause = SignalCause{}
	c.pendingTriggers = 0
}

func (rt *runtime) beginCompRun(c *computation) compRunToken {
	if c.pendingCause.Seq == 0 {
		return compRunToken{}
	}
	cause := c.pendingCause
	retriggered := max(c.pendingTriggers-1, 0)
	c.pendingCause = SignalCause{}
	c.pendingTriggers = 0
	c.lastCause = cause
	c.lastTriggerSeq = cause.Seq
	return compRunToken{active: true, cause: cause, retriggered: retriggered, began: time.Now()}
}

func (rt *runtime) endCompRun(c *computation, tok compRunToken) {
	if !tok.active || rt.isDevToolsScope(&c.Scope) {
		return
	}
	took := time.Since(tok.began)
	ownerID := rt.scopeID(&c.Scope)
	site := rt.compSite(c)
	kind := compKindName(c.kind)
	run := TriggerRun{
		Cause:       tok.cause,
		Kind:        kind,
		Owner:       ownerID,
		Site:        site,
		Duration:    took,
		Retriggered: tok.retriggered,
	}
	rt.causal.triggers.push(run, causalRingCap)
	if driver.TraceCauses && len(rt.causal.commitTriggers) < causalRingCap {
		rt.causal.commitTriggers = append(rt.causal.commitTriggers, driver.Trigger{
			Seq:         tok.cause.Seq,
			SignalID:    tok.cause.SignalID,
			Generation:  tok.cause.Generation,
			WriteSite:   tok.cause.Site,
			Kind:        kind,
			Owner:       ownerID,
			Site:        site,
			Took:        took,
			Retriggered: tok.retriggered,
		})
	}
}

func (rt *runtime) beginMemoNotify(tok compRunToken) signalWriteToken {
	if !tok.active {
		return signalWriteToken{}
	}
	prev := rt.causal.activeCause
	rt.causal.activeCause = tok.cause
	return signalWriteToken{active: true, prev: prev}
}

func (rt *runtime) endMemoNotify(tok signalWriteToken) {
	if tok.active {
		rt.causal.activeCause = tok.prev
	}
}

func (rt *runtime) takeCommitTriggers(n int) []driver.Trigger {
	if len(rt.causal.commitTriggers) == 0 {
		return nil
	}
	src := rt.causal.commitTriggers
	if n > 0 && len(src) > n {
		src = src[len(src)-n:]
	}
	out := make([]driver.Trigger, len(src))
	copy(out, src)
	rt.causal.commitTriggers = rt.causal.commitTriggers[:0]
	return out
}

func (rt *runtime) beginExplicitDisposal(s *Scope) disposalToken {
	if !rt.causalActive() {
		return disposalToken{}
	}
	skip := rt.isDevToolsScope(s)
	id := rt.scopeID(s)
	rt.causal.disposalStack = append(rt.causal.disposalStack, disposalFrame{
		rootOwner: id,
		owner:     id,
		ownerName: rt.scopeName(s),
		reason:    DisposalExplicit,
		caller:    causalCallerSite(false),
		skip:      skip,
	})
	return disposalToken{active: true}
}

func (rt *runtime) beginRerunDisposal(c *computation) disposalToken {
	if !rt.causalActive() || (c.first == nil && len(c.cleanups) == 0) {
		return disposalToken{}
	}
	skip := rt.isDevToolsScope(&c.Scope)
	id := rt.scopeID(&c.Scope)
	caller := rt.compSite(c)
	if caller == "" {
		caller = c.lastCause.Site
	}
	rt.causal.disposalStack = append(rt.causal.disposalStack, disposalFrame{
		rootOwner:  id,
		owner:      id,
		ownerName:  rt.scopeName(&c.Scope),
		reason:     DisposalRerun,
		caller:     caller,
		triggerSeq: c.lastTriggerSeq,
		skip:       skip,
	})
	return disposalToken{active: true}
}

func (rt *runtime) beginCascadeDisposal(s *Scope) disposalToken {
	if len(rt.causal.disposalStack) == 0 {
		return disposalToken{}
	}
	parent := &rt.causal.disposalStack[len(rt.causal.disposalStack)-1]
	skip := parent.skip || rt.isDevToolsScope(s)
	id := rt.scopeID(s)
	rt.causal.disposalStack = append(rt.causal.disposalStack, disposalFrame{
		rootOwner:  parent.rootOwner,
		owner:      id,
		ownerName:  rt.scopeName(s),
		reason:     DisposalCascade,
		caller:     parent.caller,
		triggerSeq: parent.triggerSeq,
		skip:       skip,
	})
	return disposalToken{active: true}
}

func (rt *runtime) noteCleanupRan(c *cleanup) {
	if len(rt.causal.disposalStack) == 0 || c == nil {
		return
	}
	top := &rt.causal.disposalStack[len(rt.causal.disposalStack)-1]
	if top.skip || (c.label == "" && c.site == "") {
		return
	}
	top.cleanups = append(top.cleanups, CleanupRecord{Label: c.label, Site: c.site})
}

func (rt *runtime) endDisposal(tok disposalToken) {
	if !tok.active || len(rt.causal.disposalStack) == 0 {
		return
	}
	last := len(rt.causal.disposalStack) - 1
	f := rt.causal.disposalStack[last]
	rt.causal.disposalStack = rt.causal.disposalStack[:last]
	if f.skip {
		return
	}
	hasActivity := len(f.cleanups) > 0 || f.childRan
	if hasActivity && len(rt.causal.disposalStack) > 0 {
		rt.causal.disposalStack[len(rt.causal.disposalStack)-1].childRan = true
	}
	record := f.reason == DisposalExplicit ||
		(f.reason == DisposalRerun && hasActivity) ||
		(f.reason == DisposalCascade && len(f.cleanups) > 0)
	if !record {
		return
	}
	rt.causal.nextDisposal++
	rt.causal.disposals.push(DisposalRecord{
		Seq:        rt.causal.nextDisposal,
		RootOwner:  f.rootOwner,
		Owner:      f.owner,
		OwnerName:  f.ownerName,
		Reason:     f.reason,
		Caller:     f.caller,
		TriggerSeq: f.triggerSeq,
		Cleanups:   f.cleanups,
	}, causalRingCap)
}

func (o *Scope) annotateCleanup(c *cleanup, label string) {
	if c == nil || o == nil || o.rt == nil || !o.rt.causalActive() {
		return
	}
	c.label = label
	c.site = causalCallerSite(label != "cleanup")
}

func (o *Scope) annotateCleanupSite(c *cleanup, label, site string) {
	if c == nil || o == nil || o.rt == nil || !o.rt.causalActive() {
		return
	}
	c.label = label
	if site != "" {
		c.site = site
	} else {
		c.site = causalCallerSite(label != "cleanup")
	}
}

func (o *Scope) annotateSurfaceCleanup(c *cleanup, kind driver.SurfaceKind) {
	if c == nil || o == nil || o.rt == nil || !o.rt.causalActive() {
		return
	}
	c.label = surfaceCleanupLabel(kind)
	c.site = causalCallerSite(true)
}

func surfaceCleanupLabel(kind driver.SurfaceKind) string {
	switch kind {
	case driver.SurfaceWindow:
		return "window"
	case driver.SurfacePanel:
		return "panel"
	case driver.SurfaceOverlay:
		return "overlay"
	case driver.SurfaceTrayIcon:
		return "tray"
	case driver.SurfaceScene:
		return "scene"
	default:
		return "surface"
	}
}

func (w *soundWaitCausal) captureSite(rt *runtime) {
	if rt != nil && rt.causalActive() {
		w.site = causalCallerSite(true)
	}
}

func (w *soundWaitCausal) loadSite() string {
	return w.site
}

// Read-only accessors for nsgodev causal-chain diagnostics.

// DevOwnerID returns cx's monotonic diagnostic owner ID (nsgodev only).
func DevOwnerID(cx *Scope) uint64 {
	if cx == nil || cx.rt == nil {
		return 0
	}
	return cx.rt.scopeID(cx)
}

// DevLastCause returns the last signal write cause and sequence number that triggered cx when cx is a computation scope (nsgodev only).
func DevLastCause(cx *Scope) (SignalCause, uint64, bool) {
	if cx == nil || cx.comp == nil || cx.comp.lastTriggerSeq == 0 {
		return SignalCause{}, 0, false
	}
	return cx.comp.lastCause, cx.comp.lastTriggerSeq, true
}

// DevCauses returns up to limit of cx's graph's most recent signal write causes, oldest first (nsgodev only).
func DevCauses(cx *Scope, limit int) []SignalCause {
	if cx == nil || cx.rt == nil {
		return nil
	}
	return cx.rt.causal.causes.recent(limit)
}

// DevTriggers returns up to limit of cx's graph's most recent signal-triggered computation runs, oldest first (nsgodev only).
func DevTriggers(cx *Scope, limit int) []TriggerRun {
	if cx == nil || cx.rt == nil {
		return nil
	}
	return cx.rt.causal.triggers.recent(limit)
}

// DevDisposals returns up to limit of cx's graph's most recent scope disposals, oldest first (nsgodev only).
func DevDisposals(cx *Scope, limit int) []DisposalRecord {
	if cx == nil || cx.rt == nil {
		return nil
	}
	records := cx.rt.causal.disposals.recent(limit)
	for i := range records {
		if len(records[i].Cleanups) > 0 {
			records[i].Cleanups = append([]CleanupRecord(nil), records[i].Cleanups...)
		}
	}
	return records
}

// devtools.CausalSurface implementation on *devSurface (nsgodev only).

func toDevtoolsCause(c SignalCause) devtools.SignalCause {
	return devtools.SignalCause{
		Seq:        c.Seq,
		SignalID:   c.SignalID,
		Generation: c.Generation,
		Site:       c.Site,
	}
}

func toDevtoolsTrigger(r TriggerRun) devtools.TriggerRun {
	return devtools.TriggerRun{
		Cause:       toDevtoolsCause(r.Cause),
		Kind:        r.Kind,
		Owner:       r.Owner,
		Site:        r.Site,
		Duration:    int64(r.Duration),
		Retriggered: r.Retriggered,
	}
}

func (s *devSurface) fillOwnerCausal(o *Scope, out *devtools.Owner) {
	if s.t.rt.causalActive() {
		out.ID = s.t.rt.scopeID(o)
	}
}

func (s *devSurface) fillCompCausal(c *computation, out *devtools.Computation) {
	if c == nil || c.lastTriggerSeq == 0 {
		return
	}
	cause := toDevtoolsCause(c.lastCause)
	out.LastTriggerSeq = c.lastTriggerSeq
	out.LastCause = &cause
}

// Causes implements devtools.CausalSurface.
func (s *devSurface) Causes(n int) []devtools.CauseSummary {
	rt := s.t.rt
	causes := rt.causal.causes.recent(n)
	if len(causes) == 0 {
		return nil
	}
	bySeq := make(map[uint64][]devtools.TriggerRun, len(causes))
	for _, r := range rt.causal.triggers.recent(0) {
		bySeq[r.Cause.Seq] = append(bySeq[r.Cause.Seq], toDevtoolsTrigger(r))
	}
	out := make([]devtools.CauseSummary, len(causes))
	for i, c := range causes {
		out[i] = devtools.CauseSummary{
			Cause:    toDevtoolsCause(c),
			Triggers: bySeq[c.Seq],
		}
	}
	return out
}

// Triggers implements devtools.CausalSurface.
func (s *devSurface) Triggers(n int) []devtools.TriggerRun {
	runs := s.t.rt.causal.triggers.recent(n)
	if len(runs) == 0 {
		return nil
	}
	out := make([]devtools.TriggerRun, len(runs))
	for i, r := range runs {
		out[i] = toDevtoolsTrigger(r)
	}
	return out
}

// Disposals implements devtools.CausalSurface.
func (s *devSurface) Disposals(n int) []devtools.DisposalRecord {
	records := s.t.rt.causal.disposals.recent(n)
	if len(records) == 0 {
		return nil
	}
	out := make([]devtools.DisposalRecord, len(records))
	for i, r := range records {
		var cleanups []devtools.CleanupRecord
		if len(r.Cleanups) > 0 {
			cleanups = make([]devtools.CleanupRecord, len(r.Cleanups))
			for j, cl := range r.Cleanups {
				cleanups[j] = devtools.CleanupRecord{Label: cl.Label, Site: cl.Site}
			}
		}
		out[i] = devtools.DisposalRecord{
			Seq:        r.Seq,
			RootOwner:  r.RootOwner,
			Owner:      r.Owner,
			OwnerName:  r.OwnerName,
			Reason:     string(r.Reason),
			Caller:     r.Caller,
			TriggerSeq: r.TriggerSeq,
			Cleanups:   cleanups,
		}
	}
	return out
}
