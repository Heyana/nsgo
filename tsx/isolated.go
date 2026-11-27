package tsx

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sourcemap/sourcemap"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx/internal/js"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
	"github.com/nsgo-dev/nsgo/tsx/websrc"
)

var guestFnKey = js.NewSymbol("nsgo.isolatedFn")

func webIsolationAvailable() bool { return js.IsolatedFrameAvailable() }

// isolatedBridge hosts an untrusted sandboxed TSX package inside an
// opaque-origin <iframe sandbox="allow-scripts"> with a strict CSP and
// bridges Go-backed APIs, reactive DOM ops and plugin calls over a private
// MessagePort (via tsx/internal/js.IsolatedChannel). No untrusted package code
// ever executes in the host origin.
type isolatedBridge struct {
	r        *runtime
	invokeFn func(int, []js.Value) js.Value
	ch       *js.IsolatedChannel

	mu      sync.Mutex
	inbox   []map[string]any
	closed  bool
	waiting int
	wake    chan struct{}
	stopCh  chan struct{}
	stopOn  sync.Once

	nextReq   int
	replies   map[int]map[string]any
	exports   []string
	nodes     map[int]*node
	scopes    map[int]*core.Scope
	faceSigs  map[int]map[string]*core.Signal[js.Value]
	faceObjs  map[int]*js.Object
	handles   map[int]js.Value
	handleSig map[int]map[string]*core.Signal[js.Value]
	handleObj map[int]*js.Object

	// timers, frames, fetches and calls owned by this bridge.
	guestTimers  map[int]int64
	guestFrames  map[int]func()
	fetchCancels map[int]context.CancelFunc
	pendingCalls map[int]*deadline
	nextCallID   int

	// deadlines maps integer owner IDs passed across the bridge to *deadline.
	nextOwnerID int
	deadlines   map[int]*deadline
	ownerIDs    map[*deadline]int

	maps map[string]*sourcemap.Consumer
}

func (b *isolatedBridge) guestFnID(v js.Value) (int, bool) {
	o, ok := v.(*js.Object)
	if !ok {
		return 0, false
	}
	sym := o.GetSymbol(guestFnKey)
	if sym == nil || js.IsUndefined(sym) {
		return 0, false
	}
	return int(sym.ToInteger()), true
}

func (b *isolatedBridge) callGuest(fnID int, args []js.Value) js.Value {
	return b.invokeFn(fnID, args)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func buildIsolatedSrcdoc(cspNonce, initToken string) string {
	csp := fmt.Sprintf(
		"default-src 'none'; script-src 'nonce-%s' 'unsafe-eval'; connect-src 'none'; img-src 'none'; media-src 'none'; font-src 'none'; style-src 'none'; frame-src 'none'; worker-src 'none'; child-src 'none'; object-src 'none'; form-action 'none'; base-uri 'none';",
		cspNonce,
	)
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\">")
	b.WriteString("<meta http-equiv=\"Content-Security-Policy\" content=\"")
	b.WriteString(csp)
	b.WriteString("\"></head><body><script nonce=\"")
	b.WriteString(cspNonce)
	b.WriteString("\">\n(")
	b.WriteString(websrc.Isolated.Source)
	b.WriteString(")(")
	b.WriteString(strconv.Quote(initToken))
	b.WriteString(");\n</script></body></html>")
	return b.String()
}

// startIsolated starts a sandboxed bundle inside an opaque-origin sandboxed
// iframe on the web, returning the host runtime that owns its core tree and
// capability-checked services.
func (s Sandbox) startIsolated(cx *core.Scope, name, src string, view ViewOptions) *runtime {
	if !webIsolationAvailable() {
		panic(ErrUnisolated)
	}
	limits := DefaultLimits
	if s.Limits != nil {
		limits = *s.Limits
	}
	if limits.Stack <= 0 || limits.Stack > maxCallStack {
		limits.Stack = maxCallStack
	}
	for _, c := range []*pkg.Capabilities{s.Capabilities, s.Grant} {
		if c != nil {
			cx.ProvideAccess(access{c})
		}
	}
	cx.ProvideHost(s.Host)
	cx.Confine()
	if s.Storage != "" {
		cx.ProvideStorage(s.Storage, limits.Storage)
		cx.ProvideSecrets(s.Storage)
	}

	r := newRuntime(cx)
	r.limits = limits
	r.sandbox = &sandboxed{
		caps:           s.Capabilities != nil || s.Grant != nil,
		modules:        s.Modules,
		disableStorage: s.DisableStorage,
	}
	r.exportView = s.exportView || view.Export != ""
	if s.plugin {
		r.onError = s.OnError
	}

	b := &isolatedBridge{
		r:            r,
		wake:         make(chan struct{}, 1),
		stopCh:       make(chan struct{}),
		replies:      map[int]map[string]any{},
		nodes:        map[int]*node{0: r.root},
		scopes:       map[int]*core.Scope{0: cx},
		faceSigs:     map[int]map[string]*core.Signal[js.Value]{},
		faceObjs:     map[int]*js.Object{},
		handles:      map[int]js.Value{},
		handleSig:    map[int]map[string]*core.Signal[js.Value]{},
		handleObj:    map[int]*js.Object{},
		guestTimers:  map[int]int64{},
		guestFrames:  map[int]func(){},
		fetchCancels: map[int]context.CancelFunc{},
		pendingCalls: map[int]*deadline{},
		deadlines:    map[int]*deadline{},
		ownerIDs:     map[*deadline]int{},
		maps:         map[string]*sourcemap.Consumer{},
	}
	b.invokeFn = b.invokeGuest
	r.isolated = b
	b.parseSourceMap(name, src)

	trackView := !s.plugin && (s.OnViewRuntime != nil || s.OnStop != nil)
	if trackView {
		r.onStop = s.OnStop
		cx.Cleanup(func() {
			b.dispose()
			r.markStopped(ErrViewClosed)
			r.stopTimers()
			b.close()
			r.finishStop(r.stoppedErr())
		})
		if s.OnViewRuntime != nil {
			s.OnViewRuntime(&ViewRuntime{r: r})
		}
	} else if s.plugin {
		// Registered after newRuntime's r.vm.Close cleanup so LIFO disposal runs
		// guest cleanups while r.vm is still alive, before plugin.dispose fails calls.
		cx.Cleanup(func() {
			if r.stoppedErr() == nil {
				b.dispose()
			} else {
				b.close()
			}
		})
	} else {
		cx.Cleanup(b.dispose)
	}

	if limits.CallTotal > 0 && r.stoppedErr() == nil {
		cx.Cleanup(r.stopWatches)
		r.deadlines = true
	}

	if err := b.boot(name, src, view, limits); err != nil && !r.interrupted.Load() {
		b.close()
		err = r.failure(err)
		if trackView {
			r.markStopped(err)
			r.stopTimers()
			cx.Post(func() { r.finishStop(r.stoppedErr()) })
		}
		panic(err)
	}

	b.watchEnv()
	return r
}

func (b *isolatedBridge) boot(name, src string, view ViewOptions, limits Limits) error {
	if b.r.stoppedErr() != nil {
		return b.r.stoppedErr()
	}

	var exportProps map[string]any
	if view.Export != "" && view.Props != nil {
		raw, err := json.Marshal(view.Props)
		if err != nil {
			return fmt.Errorf("tsx: a view's props: %w", err)
		}
		if err := json.Unmarshal(raw, &exportProps); err != nil {
			return fmt.Errorf("tsx: a view's props: %w", err)
		}
	}

	cspNonce := randomHex(16)
	initToken := randomHex(16)
	srcdoc := buildIsolatedSrcdoc(cspNonce, initToken)

	b.ch = js.OpenIsolatedFrame(srcdoc, initToken, func(msg map[string]any) {
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return
		}
		b.inbox = append(b.inbox, msg)
		waiting := b.waiting > 0
		b.mu.Unlock()
		select {
		case b.wake <- struct{}{}:
		default:
		}
		if !waiting {
			b.r.cx.Post(b.drain)
		}
	})

	// Wait for the opaque-origin iframe bootstrap to connect the private MessagePort.
	bootTimeout := max(limits.Load, 5*time.Second)
	timer := time.NewTimer(bootTimeout)
	defer timer.Stop()
	select {
	case <-b.ch.Ready():
	case <-b.stopCh:
		return cmp.Or(b.r.stoppedErr(), ErrInterrupted)
	case <-timer.C:
		return ErrBudget
	}

	// Start the entry load deadline only once the iframe is ready to run the bundle,
	// matching goja where VM setup completes before the load budget starts.
	if b.r.deadlines && b.r.stoppedErr() == nil {
		b.r.current = newDeadline(cmp.Or(limits.Load, limits.CallTotal))
		b.r.watch(b.r.current)
		defer func() { b.r.current = nil }()
	}

	manifest, knownModules := b.buildManifest()
	envSnap := b.envSnapshot()
	cfg := map[string]any{
		"name":           name,
		"source":         src,
		"exportView":     b.r.exportView,
		"exportName":     view.Export,
		"exportProps":    exportProps,
		"harden":         limits.Harden,
		"hardenSource":   websrc.Harden.Source,
		"webSource":      websrc.Web.Source,
		"timeSource":     websrc.Time.Source,
		"splitSource":    websrc.Split.Source,
		"mergeSource":    websrc.Merge.Source,
		"limits":         map[string]any{"timers": limits.Timers, "nodes": limits.Nodes},
		"nowMS":          b.r.millis(b.r.cx.Now()),
		"wallMS":         float64(b.r.cx.WallTime().UnixNano()) / 1e6,
		"manifest":       manifest,
		"knownModules":   knownModules,
		"allowedNatives": b.allowedNatives(),
		"env":            envSnap,
	}

	reqID := b.nextReqID()
	ownerID := b.ownerIDFor(b.r.current)
	b.postToGuest(map[string]any{
		"type":    "init",
		"reqId":   reqID,
		"config":  cfg,
		"ownerId": ownerID,
	})

	reply, err := b.waitReply(reqID, limits.Load)
	if err != nil {
		return err
	}
	if mapStr(reply, "type") == "loadError" {
		switch mapStr(reply, "kind") {
		case "noViewExport":
			return fmt.Errorf("%w: %q", ErrNoViewExport, mapStr(reply, "name"))
		case "notComponent":
			return fmt.Errorf("%w: %q", ErrViewExportNotComponent, mapStr(reply, "name"))
		}
		return b.decodeError(mapObj(reply, "error"))
	}

	if ex := mapSlice(reply, "exports"); len(ex) > 0 {
		b.exports = make([]string, 0, len(ex))
		for _, item := range ex {
			if s, ok := item.(string); ok {
				b.exports = append(b.exports, s)
			}
		}
	}
	if ops := mapSlice(reply, "ops"); len(ops) > 0 {
		b.applyOps(ops)
	}
	return nil
}

func (b *isolatedBridge) close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.inbox = nil
	ch := b.ch
	b.ch = nil
	b.mu.Unlock()

	b.stopOn.Do(func() { close(b.stopCh) })

	for id, cancel := range b.fetchCancels {
		cancel()
		delete(b.fetchCancels, id)
	}
	for id, stop := range b.guestFrames {
		stop()
		delete(b.guestFrames, id)
	}
	if ch != nil {
		ch.Close()
	}
}

func (b *isolatedBridge) dispose() {
	b.mu.Lock()
	closed := b.closed
	ch := b.ch
	b.mu.Unlock()
	if closed || ch == nil {
		b.close()
		return
	}
	reqID := b.nextReqID()
	b.postToGuest(map[string]any{
		"type":   "dispose",
		"reqId":  reqID,
		"nowMS":  b.r.millis(b.r.cx.Now()),
		"wallMS": float64(b.r.cx.WallTime().UnixNano()) / 1e6,
	})
	if reply, err := b.waitReply(reqID, time.Second); err == nil {
		if ops := mapSlice(reply, "ops"); len(ops) > 0 {
			b.applyOps(ops)
		}
	}
	b.close()
}

func (b *isolatedBridge) exportNames() []string {
	return slices.Clone(b.exports)
}

func (b *isolatedBridge) nextReqID() int {
	b.nextReq++
	return b.nextReq
}

func (b *isolatedBridge) ownerIDFor(dl *deadline) int {
	if dl == nil {
		return 0
	}
	if id, ok := b.ownerIDs[dl]; ok {
		return id
	}
	b.nextOwnerID++
	id := b.nextOwnerID
	b.ownerIDs[dl] = id
	b.deadlines[id] = dl
	return id
}

func (b *isolatedBridge) deadlineOf(ownerID int) *deadline {
	if ownerID <= 0 {
		return nil
	}
	return b.deadlines[ownerID]
}

func (b *isolatedBridge) postToGuest(msg map[string]any) {
	b.mu.Lock()
	closed := b.closed
	ch := b.ch
	b.mu.Unlock()
	if closed || ch == nil {
		return
	}
	ch.Post(msg)
}

// drain processes all queued guest messages in FIFO order on the engine goroutine.
func (b *isolatedBridge) drain() {
	if b.r.stoppedErr() != nil || b.r.cx.Disposed() {
		return
	}
	for {
		b.mu.Lock()
		if b.closed || len(b.inbox) == 0 {
			b.mu.Unlock()
			return
		}
		msgs := b.inbox
		b.inbox = nil
		b.mu.Unlock()
		for _, msg := range msgs {
			if b.r.stoppedErr() != nil || b.r.cx.Disposed() {
				return
			}
			b.dispatchIncoming(msg)
		}
	}
}

// waitReply pumps incoming FIFO messages on the engine goroutine until the
// reply matching reqID arrives, the bridge stops, or budget expires.
func (b *isolatedBridge) waitReply(reqID int, budget time.Duration) (map[string]any, error) {
	b.mu.Lock()
	b.waiting++
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.waiting--
		hasMore := len(b.inbox) > 0 && !b.closed
		b.mu.Unlock()
		if hasMore {
			b.r.cx.Post(b.drain)
		}
	}()

	var timer *time.Timer
	var timeoutCh <-chan time.Time
	if budget > 0 {
		timer = time.NewTimer(budget)
		defer timer.Stop()
		timeoutCh = timer.C
	}

	for {
		if reply, ok := b.replies[reqID]; ok {
			delete(b.replies, reqID)
			return reply, nil
		}
		for {
			b.mu.Lock()
			if b.closed {
				b.mu.Unlock()
				return nil, cmp.Or(b.r.stoppedErr(), ErrInterrupted)
			}
			if len(b.inbox) == 0 {
				b.mu.Unlock()
				break
			}
			msg := b.inbox[0]
			b.inbox = b.inbox[1:]
			b.mu.Unlock()

			if typ := mapStr(msg, "type"); typ == "reply" || typ == "loaded" || typ == "loadError" {
				id := mapInt(msg, "reqId")
				if id == reqID {
					return msg, nil
				}
				b.replies[id] = msg
				continue
			}
			b.dispatchIncoming(msg)
			if err := b.r.stoppedErr(); err != nil {
				return nil, err
			}
		}

		select {
		case <-b.wake:
		case <-b.stopCh:
			return nil, cmp.Or(b.r.stoppedErr(), ErrInterrupted)
		case <-timeoutCh:
			return nil, ErrBudget
		}
	}
}

func (b *isolatedBridge) dispatchIncoming(msg map[string]any) {
	if msg == nil {
		return
	}
	typ := mapStr(msg, "type")
	ownerID := mapInt(msg, "ownerId")
	dl := b.deadlineOf(ownerID)

	switch typ {
	case "ops":
		b.r.cx.Run(func() {
			b.r.under(dl, func() {
				b.applyOps(mapSlice(msg, "ops"))
			})
		})
	case "setTimer":
		b.handleSetTimer(msg, dl, ownerID)
	case "clearTimer":
		b.handleClearTimer(mapInt(msg, "id"))
	case "raf":
		b.handleRAF(msg, dl, ownerID)
	case "cancelRaf":
		id := mapInt(msg, "id")
		if stop := b.guestFrames[id]; stop != nil {
			stop()
			delete(b.guestFrames, id)
		}
	case "fetch":
		b.handleFetch(msg, dl, ownerID)
	case "fetchCancel":
		id := mapInt(msg, "id")
		if cancel := b.fetchCancels[id]; cancel != nil {
			cancel()
			delete(b.fetchCancels, id)
		}
	case "rpc":
		b.handleRPC(msg, dl, ownerID)
	case "authStart":
		b.handleAuthStart(msg, dl, ownerID)
	case "callResult":
		b.handleCallResult(msg)
	case "reportError":
		err := b.decodeError(mapObj(msg, "err"))
		b.r.cx.Post(func() { b.r.callbackError(err) })
	case "console":
		log.Printf("js %s: %s", mapStr(msg, "level"), mapStr(msg, "text"))
	}
}

// ---- Plugin.Call over the isolated bridge ----

func (b *isolatedBridge) startCall(p *Plugin, name string, args []any, done func(any, error)) *deadline {
	r := b.r
	if err := r.stoppedErr(); err != nil || p.cx.Disposed() {
		done(nil, cmp.Or(err, ErrPluginClosed))
		return nil
	}
	if !slices.Contains(b.exports, name) {
		done(nil, fmt.Errorf("%w: %q", ErrNoExport, name))
		return nil
	}
	if r.calls == nil {
		r.calls = map[*deadline]struct{}{}
	}
	dl := newDeadline(r.limits.CallTotal)
	dl.done = done
	r.calls[dl] = struct{}{}
	r.watch(dl)

	b.nextCallID++
	callID := b.nextCallID
	b.pendingCalls[callID] = dl
	ownerID := b.ownerIDFor(dl)
	reqID := b.nextReqID()

	b.postToGuest(map[string]any{
		"type":    "call",
		"reqId":   reqID,
		"callId":  callID,
		"name":    name,
		"args":    args,
		"nowMS":   r.millis(r.cx.Now()),
		"wallMS":  float64(r.cx.WallTime().UnixNano()) / 1e6,
		"ownerId": ownerID,
	})

	p.cx.Run(func() {
		r.under(dl, func() {
			reply, err := b.waitReply(reqID, r.limits.Call)
			if err != nil {
				delete(b.pendingCalls, callID)
				if errors.Is(err, ErrBudget) && r.stoppedErr() == nil {
					r.expire(dl)
				} else {
					r.finish(dl, nil, err)
				}
				return
			}
			if ops := mapSlice(reply, "ops"); len(ops) > 0 {
				b.applyOps(ops)
			}
			if !mapBool(reply, "settled") {
				return
			}
			delete(b.pendingCalls, callID)
			if mapBool(reply, "noExport") {
				r.finish(dl, nil, fmt.Errorf("%w: %q", ErrNoExport, name))
				return
			}
			if errObj := mapObj(reply, "error"); errObj != nil {
				r.finish(dl, nil, b.decodeRejection(errObj))
				return
			}
			r.finish(dl, reply["value"], nil)
		})
	})
	return dl
}

func (b *isolatedBridge) handleCallResult(msg map[string]any) {
	callID := mapInt(msg, "callId")
	dl, ok := b.pendingCalls[callID]
	delete(b.pendingCalls, callID)
	if ops := mapSlice(msg, "ops"); len(ops) > 0 {
		b.r.cx.Run(func() { b.applyOps(ops) })
	}
	if !ok || dl == nil {
		return
	}
	if errObj := mapObj(msg, "error"); errObj != nil {
		b.r.finish(dl, nil, b.decodeRejection(errObj))
		return
	}
	b.r.finish(dl, msg["value"], nil)
}

// ---- Guest callback invocation (events, render props, BackHandler, keymap) ----

func (b *isolatedBridge) invokeGuest(fnID int, args []js.Value) js.Value {
	r := b.r
	if r.stoppedErr() != nil || r.cx.Disposed() {
		return js.Undefined()
	}
	dl := r.current
	if dl == nil && r.deadlines {
		dl = newDeadline(r.limits.CallTotal)
		r.watch(dl)
	}
	if r.passed(dl) {
		return js.Undefined()
	}
	reqID := b.nextReqID()
	encArgs := make([]any, len(args))
	for i, a := range args {
		encArgs[i] = b.toGuestValue(a)
	}
	b.postToGuest(map[string]any{
		"type":    "invoke",
		"reqId":   reqID,
		"fnId":    fnID,
		"args":    encArgs,
		"nowMS":   r.millis(r.cx.Now()),
		"wallMS":  float64(r.cx.WallTime().UnixNano()) / 1e6,
		"ownerId": b.ownerIDFor(dl),
	})
	reply, err := b.waitReply(reqID, r.limits.Call)
	if err != nil {
		if r.interrupted.Load() {
			return js.Undefined()
		}
		if errors.Is(err, ErrBudget) && dl != nil {
			r.expire(dl)
			return js.Undefined()
		}
		r.callbackError(err)
		return js.Undefined()
	}
	if ops := mapSlice(reply, "ops"); len(ops) > 0 {
		r.under(dl, func() { b.applyOps(ops) })
	}
	if errObj := mapObj(reply, "error"); errObj != nil {
		r.callbackError(b.decodeError(errObj))
		return js.Undefined()
	}
	return b.toHostValue(reply["value"])
}

// ---- Timers & RAF on the app's clock ----

func (b *isolatedBridge) handleSetTimer(msg map[string]any, dl *deadline, ownerID int) {
	r := b.r
	guestID := mapInt(msg, "id")
	delayMS := mapFloat(msg, "delay")
	repeat := mapBool(msg, "repeat")
	if r.sandbox != nil && r.limits.Timers > 0 && len(r.timers.stop) >= r.limits.Timers {
		return
	}
	r.timers.next++
	goID := r.timers.next
	b.guestTimers[guestID] = goID
	if dl != nil && (dl.done != nil || !dl.at.IsZero()) {
		if dl.timers == nil {
			dl.timers = map[int64]struct{}{}
		}
		dl.timers[goID] = struct{}{}
		r.watch(dl)
	}
	d := time.Duration(max(delayMS, 0) * float64(time.Millisecond))
	due := r.cx.Now()
	var schedule func()
	schedule = func() {
		due = due.Add(d)
		r.cx.Run(func() {
			stop := r.cx.AfterFunc(max(due.Sub(r.cx.Now()), 0), func() {
				if r.stoppedErr() != nil || r.passed(dl) {
					r.removeTimer(goID)
					delete(b.guestTimers, guestID)
					return
				}
				if repeat {
					schedule()
				} else {
					r.removeTimer(goID)
					delete(b.guestTimers, guestID)
				}
				r.under(dl, func() {
					reqID := b.nextReqID()
					b.postToGuest(map[string]any{
						"type":    "timer",
						"reqId":   reqID,
						"id":      guestID,
						"nowMS":   r.millis(r.cx.Now()),
						"wallMS":  float64(r.cx.WallTime().UnixNano()) / 1e6,
						"ownerId": ownerID,
					})
					reply, err := b.waitReply(reqID, r.limits.Call)
					if err != nil {
						return
					}
					if ops := mapSlice(reply, "ops"); len(ops) > 0 {
						b.applyOps(ops)
					}
					if errObj := mapObj(reply, "error"); errObj != nil {
						r.callbackError(b.decodeError(errObj))
					}
				})
			})
			r.timers.stop[goID] = pendingTimer{stop: stop, owner: dl}
		})
	}
	schedule()
}

func (b *isolatedBridge) handleClearTimer(guestID int) {
	if goID, ok := b.guestTimers[guestID]; ok {
		delete(b.guestTimers, guestID)
		b.r.stopTimer(goID)
	}
}

func (b *isolatedBridge) handleRAF(msg map[string]any, dl *deadline, ownerID int) {
	r := b.r
	guestID := mapInt(msg, "id")
	if r.sandbox != nil && r.limits.Timers > 0 && len(b.guestFrames) >= r.limits.Timers {
		return
	}
	if dl != nil {
		r.watch(dl)
	}
	r.cx.Run(func() {
		b.guestFrames[guestID] = r.cx.RequestAnimationFrame(func(at time.Time) {
			delete(b.guestFrames, guestID)
			if r.stoppedErr() != nil || r.passed(dl) {
				return
			}
			now := r.cx.Now()
			if at.After(now) {
				at = now
			}
			r.under(dl, func() {
				reqID := b.nextReqID()
				b.postToGuest(map[string]any{
					"type":    "raf",
					"reqId":   reqID,
					"id":      guestID,
					"ts":      r.millis(at),
					"nowMS":   r.millis(now),
					"wallMS":  float64(r.cx.WallTime().UnixNano()) / 1e6,
					"ownerId": ownerID,
				})
				reply, err := b.waitReply(reqID, r.limits.Call)
				if err != nil {
					return
				}
				if ops := mapSlice(reply, "ops"); len(ops) > 0 {
					b.applyOps(ops)
				}
				if errObj := mapObj(reply, "error"); errObj != nil {
					r.callbackError(b.decodeError(errObj))
				}
			})
		})
	})
}

// ---- Capability-gated Fetch over Go ----

func (b *isolatedBridge) handleFetch(msg map[string]any, dl *deadline, ownerID int) {
	r := b.r
	fetchID := mapInt(msg, "id")
	target := mapStr(msg, "url")
	method := mapStr(msg, "method")
	redirect := mapStr(msg, "redirect")

	var headers [][2]string
	for _, raw := range mapSlice(msg, "headers") {
		if pair, ok := raw.([]any); ok && len(pair) == 2 {
			k, _ := pair[0].(string)
			v, _ := pair[1].(string)
			headers = append(headers, [2]string{k, v})
		}
	}
	body, _ := msg["body"].([]byte)

	opts := fetchOptions{redirect: redirect, fetcher: fetcherKey.Use(r.cx), body: r.limits.Body}
	if err := r.fetchAllowed(target, method, len(body)); err != nil {
		b.postToGuest(map[string]any{
			"type":    "fetchReject",
			"id":      fetchID,
			"error":   encodeGoError(err),
			"ownerId": ownerID,
		})
		return
	}
	r.sandbox.requests++

	var ctx context.Context
	var cancel context.CancelFunc
	r.cx.Run(func() { ctx, cancel = context.WithCancel(r.cx.Context()) })
	b.fetchCancels[fetchID] = cancel

	if r.sandbox.caps {
		opts.check = func(u, m string) error {
			checked := make(chan error, 1)
			r.cx.Post(func() {
				if err := ctx.Err(); err != nil {
					checked <- err
					return
				}
				checked <- r.checkAccess(core.AccessRequest{Permission: core.AccessNetwork, Resource: u, Method: m})
			})
			select {
			case err := <-checked:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	finished := false
	asset, shipped := r.assets[target]
	go func() {
		var res *fetched
		var err error
		if strings.HasPrefix(target, "asset:") {
			res, err = fetchAsset(target, asset, shipped)
		} else {
			res, err = doFetch(ctx, target, method, headers, body, opts)
		}
		r.cx.Post(func() {
			delete(b.fetchCancels, fetchID)
			r.fetchDone(&finished)
			if r.stoppedErr() != nil || r.passed(dl) {
				return
			}
			if err != nil {
				if ctx.Err() == nil {
					b.postToGuest(map[string]any{
						"type":    "fetchReject",
						"id":      fetchID,
						"error":   encodeGoError(err),
						"nowMS":   r.millis(r.cx.Now()),
						"wallMS":  float64(r.cx.WallTime().UnixNano()) / 1e6,
						"ownerId": ownerID,
					})
				}
				return
			}
			hs := make([]any, len(res.headers))
			for i, h := range res.headers {
				hs[i] = []any{h[0], h[1]}
			}
			b.postToGuest(map[string]any{
				"type":       "fetchResolve",
				"id":         fetchID,
				"status":     res.status,
				"statusText": res.statusText,
				"url":        res.url,
				"redirected": res.redirected,
				"headers":    hs,
				"body":       res.body,
				"nowMS":      r.millis(r.cx.Now()),
				"wallMS":     float64(r.cx.WallTime().UnixNano()) / 1e6,
				"ownerId":    ownerID,
			})
		})
	}()
}

// ---- Capability-gated RPC & AuthSession ----

func (b *isolatedBridge) resolveTarget(mod, path string) (thisVal js.Value, fnVal js.Value, ok bool) {
	if mod == "$node" {
		id, err := strconv.Atoi(path)
		if err != nil {
			return nil, nil, false
		}
		n := b.nodes[id]
		if n == nil {
			return nil, nil, false
		}
		o := n.value()
		return o, o.Get("measure"), true
	}
	if mod == "$handle" {
		id, err := strconv.Atoi(path)
		if err != nil {
			return nil, nil, false
		}
		h := b.handles[id]
		if h == nil {
			return nil, nil, false
		}
		return h, h, true
	}
	m := b.r.require(mod)
	parts := strings.Split(path, ".")
	cur := m
	var parent js.Value = js.Undefined()
	for _, p := range parts {
		o, isObj := cur.(*js.Object)
		if !isObj {
			return nil, nil, false
		}
		parent = o
		cur = o.Get(p)
		if cur == nil || js.IsUndefined(cur) {
			return nil, nil, false
		}
	}
	return parent, cur, true
}

func (b *isolatedBridge) handleRPC(msg map[string]any, dl *deadline, ownerID int) {
	r := b.r
	rpcID := mapInt(msg, "id")
	mod := mapStr(msg, "mod")
	path := mapStr(msg, "path")
	rawArgs := mapSlice(msg, "args")

	sendReject := func(errVal map[string]any) {
		b.postToGuest(map[string]any{
			"type":    "rpcReject",
			"id":      rpcID,
			"error":   errVal,
			"nowMS":   r.millis(r.cx.Now()),
			"wallMS":  float64(r.cx.WallTime().UnixNano()) / 1e6,
			"ownerId": ownerID,
		})
	}
	sendResolve := func(v js.Value) {
		b.postToGuest(map[string]any{
			"type":    "rpcResolve",
			"id":      rpcID,
			"value":   b.toGuestValue(v),
			"nowMS":   r.millis(r.cx.Now()),
			"wallMS":  float64(r.cx.WallTime().UnixNano()) / 1e6,
			"ownerId": ownerID,
		})
	}

	r.cx.Run(func() {
		r.under(dl, func() {
			defer func() {
				if x := recover(); x != nil {
					sendReject(encodeRecoveredError(x))
				}
			}()
			var thisVal, fnVal js.Value
			var args []js.Value
			if mod == "$handle" {
				h, _, ok := b.resolveTarget(mod, path)
				if !ok {
					sendReject(map[string]any{"name": "TypeError", "message": "nsgo: handle is disposed"})
					return
				}
				o, ok := h.(*js.Object)
				if !ok || len(rawArgs) == 0 {
					sendReject(map[string]any{"name": "TypeError", "message": "nsgo: invalid handle call"})
					return
				}
				method, _ := rawArgs[0].(string)
				thisVal = o
				fnVal = o.Get(method)
				for _, a := range rawArgs[1:] {
					args = append(args, b.toHostValue(a))
				}
			} else if mod == "$node" {
				id, _ := strconv.Atoi(path)
				n := b.nodes[id]
				if n == nil {
					sendReject(map[string]any{"name": "TypeError", "message": "nsgo: node not found"})
					return
				}
				o := n.value()
				method := "measure"
				if len(rawArgs) > 0 {
					if m, ok := rawArgs[0].(string); ok {
						method = m
					}
				}
				thisVal = o
				fnVal = o.Get(method)
			} else {
				var ok bool
				thisVal, fnVal, ok = b.resolveTarget(mod, path)
				if !ok {
					sendReject(map[string]any{"name": "TypeError", "message": fmt.Sprintf("nsgo: %s.%s is not available", mod, path)})
					return
				}
				for _, a := range rawArgs {
					args = append(args, b.toHostValue(a))
				}
			}

			fn, ok := js.AssertFunction(fnVal)
			if !ok {
				sendReject(map[string]any{"name": "TypeError", "message": fmt.Sprintf("nsgo: %s.%s is not a function", mod, path)})
				return
			}
			res, err := fn(thisVal, args...)
			if err != nil {
				sendReject(encodeRecoveredError(err))
				return
			}
			if _, isPromise := res.Export().(*js.Promise); isPromise {
				b.bridgeHostPromise(res.(*js.Object), rpcID, dl, ownerID)
				return
			}
			sendResolve(res)
		})
	})
}

func (b *isolatedBridge) handleAuthStart(msg map[string]any, dl *deadline, ownerID int) {
	r := b.r
	handleID := mapInt(msg, "handleId")
	rpcID := mapInt(msg, "rpcId")
	rawArgs := mapSlice(msg, "args")
	r.cx.Run(func() {
		r.under(dl, func() {
			defer func() {
				if x := recover(); x != nil {
					b.postToGuest(map[string]any{
						"type":    "rpcReject",
						"id":      rpcID,
						"error":   encodeRecoveredError(x),
						"ownerId": ownerID,
					})
				}
			}()
			thisVal, fnVal, ok := b.resolveTarget("nsgo", "AuthSession.start")
			if !ok {
				b.postToGuest(map[string]any{"type": "rpcReject", "id": rpcID, "error": map[string]any{"name": "TypeError", "message": "AuthSession unavailable"}, "ownerId": ownerID})
				return
			}
			fn, _ := js.AssertFunction(fnVal)
			args := make([]js.Value, len(rawArgs))
			for i, a := range rawArgs {
				args[i] = b.toHostValue(a)
			}
			session, err := fn(thisVal, args...)
			if err != nil {
				b.postToGuest(map[string]any{"type": "rpcReject", "id": rpcID, "error": encodeRecoveredError(err), "ownerId": ownerID})
				return
			}
			b.handles[handleID] = session
			if so, ok := session.(*js.Object); ok {
				if p, ok := so.Get("result").(*js.Object); ok {
					b.bridgeHostPromise(p, rpcID, dl, ownerID)
				}
			}
		})
	})
}

func (b *isolatedBridge) bridgeHostPromise(p *js.Object, rpcID int, dl *deadline, ownerID int) {
	r := b.r
	then, ok := js.AssertFunction(p.Get("then"))
	if !ok {
		return
	}
	onResolve := r.function(func(call js.FunctionCall) js.Value {
		if r.stoppedErr() != nil || r.passed(dl) {
			return js.Undefined()
		}
		b.postToGuest(map[string]any{
			"type":    "rpcResolve",
			"id":      rpcID,
			"value":   b.toGuestValue(arg(call, 0)),
			"nowMS":   r.millis(r.cx.Now()),
			"wallMS":  float64(r.cx.WallTime().UnixNano()) / 1e6,
			"ownerId": ownerID,
		})
		return js.Undefined()
	})
	onReject := r.function(func(call js.FunctionCall) js.Value {
		if r.stoppedErr() != nil || r.passed(dl) {
			return js.Undefined()
		}
		b.postToGuest(map[string]any{
			"type":    "rpcReject",
			"id":      rpcID,
			"error":   encodeHostJSValueError(arg(call, 0)),
			"nowMS":   r.millis(r.cx.Now()),
			"wallMS":  float64(r.cx.WallTime().UnixNano()) / 1e6,
			"ownerId": ownerID,
		})
		return js.Undefined()
	})
	_, _ = then(p, onResolve, onReject)
}

// ---- Op Batch Applier ----

func (b *isolatedBridge) scopeOf(id int) *core.Scope {
	if s := b.scopes[id]; s != nil && !s.Disposed() {
		return s
	}
	return b.r.cx
}

func (b *isolatedBridge) applyOps(list []any) {
	r := b.r
	for _, raw := range list {
		if r.stoppedErr() != nil {
			return
		}
		op, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch mapStr(op, "op") {
		case "childScope":
			id := mapInt(op, "id")
			parent := mapInt(op, "parent")
			b.scopes[id] = b.scopeOf(parent).Child()

		case "disposeScope":
			id := mapInt(op, "id")
			if sc := b.scopes[id]; sc != nil && id != 0 {
				delete(b.scopes, id)
				sc.Dispose()
			}

		case "el":
			id := mapInt(op, "id")
			tag := mapStr(op, "tag")
			sc := b.scopeOf(mapInt(op, "scopeId"))
			var nd *node
			sc.Run(func() {
				prev := r.cx
				r.cx = sc
				defer func() { r.cx = prev }()
				nd = r.newElement(tag)
			})
			b.nodes[id] = nd

		case "raw":
			id := mapInt(op, "id")
			b.nodes[id] = r.newRaw(mapStr(op, "text"))

		case "setRaw":
			id := mapInt(op, "id")
			if nd := b.nodes[id]; nd != nil && nd.kind == kindRaw {
				text := mapStr(op, "text")
				if nd.raw != text {
					nd.raw = text
					if nd.parent != nil {
						r.textChanged(nd.parent)
					}
				}
			}

		case "region":
			id := mapInt(op, "id")
			sc := b.scopeOf(mapInt(op, "scopeId"))
			var nd *node
			sc.Run(func() {
				nd = &node{r: r, kind: kindRegion, elem: r.newCoreElement(sc, nil)}
			})
			b.nodes[id] = nd

		case "prop":
			id := mapInt(op, "id")
			if nd := b.nodes[id]; nd != nil {
				r.setProp(nd, mapStr(op, "key"), b.toHostValue(op["val"]))
			}

		case "ins":
			p := b.nodes[mapInt(op, "p")]
			c := b.nodes[mapInt(op, "c")]
			var before *node
			if bid := mapInt(op, "before"); bid != 0 {
				before = b.nodes[bid]
			}
			if p != nil && c != nil {
				r.insertBefore(p, c, before)
			}

		case "rem":
			p := b.nodes[mapInt(op, "p")]
			c := b.nodes[mapInt(op, "c")]
			if p != nil && c != nil {
				r.removeChild(p, c)
			}

		case "face":
			b.applyCreateFace(op)

		case "faceProp":
			id := mapInt(op, "id")
			key := mapStr(op, "key")
			val := b.toHostValue(op["val"])
			if sigs := b.faceSigs[id]; sigs != nil {
				if sig := sigs[key]; sig != nil {
					sig.Set(val)
					continue
				}
			}
			if obj := b.faceObjs[id]; obj != nil {
				_ = obj.Set(key, val)
			}

		case "navPage":
			b.applyCreateNavPage(op)

		case "handleProp":
			id := mapInt(op, "id")
			key := mapStr(op, "key")
			val := b.toHostValue(op["val"])
			if sigs := b.handleSig[id]; sigs != nil {
				if sig := sigs[key]; sig != nil {
					sig.Set(val)
					continue
				}
			}
			if obj := b.handleObj[id]; obj != nil {
				_ = obj.Set(key, val)
			}

		case "nodeCall":
			id := mapInt(op, "id")
			if nd := b.nodes[id]; nd != nil {
				o := nd.value()
				method := mapStr(op, "method")
				if fn, ok := js.AssertFunction(o.Get(method)); ok {
					var args []js.Value
					for _, a := range mapSlice(op, "args") {
						args = append(args, b.toHostValue(a))
					}
					_, _ = fn(o, args...)
				}
			}

		case "subscribe":
			id := mapInt(op, "id")
			mod := mapStr(op, "mod")
			path := mapStr(op, "path")
			sc := b.scopeOf(mapInt(op, "scopeId"))
			if thisVal, fnVal, ok := b.resolveTarget(mod, path); ok {
				if fn, ok := js.AssertFunction(fnVal); ok {
					var args []js.Value
					for _, a := range mapSlice(op, "args") {
						args = append(args, b.toHostValue(a))
					}
					sc.Run(func() {
						prev := r.cx
						r.cx = sc
						defer func() { r.cx = prev }()
						if res, err := fn(thisVal, args...); err == nil {
							b.handles[id] = res
						}
					})
				}
			}

		case "createHandle":
			id := mapInt(op, "id")
			mod := mapStr(op, "mod")
			name := mapStr(op, "name")
			sc := b.scopeOf(mapInt(op, "scopeId"))
			if thisVal, fnVal, ok := b.resolveTarget(mod, name); ok {
				if fn, ok := js.AssertFunction(fnVal); ok {
					var args []js.Value
					for _, a := range mapSlice(op, "args") {
						args = append(args, b.toHostValue(a))
					}
					sc.Run(func() {
						prev := r.cx
						r.cx = sc
						defer func() { r.cx = prev }()
						if res, err := fn(thisVal, args...); err == nil {
							b.handles[id] = res
						}
					})
				}
			}

		case "construct":
			id := mapInt(op, "id")
			mod := mapStr(op, "mod")
			name := mapStr(op, "name")
			sc := b.scopeOf(mapInt(op, "scopeId"))
			if _, ctorVal, ok := b.resolveTarget(mod, name); ok {
				var args []js.Value
				for _, a := range mapSlice(op, "args") {
					args = append(args, b.toHostValue(a))
				}
				sc.Run(func() {
					prev := r.cx
					r.cx = sc
					defer func() { r.cx = prev }()
					if res, err := r.vm.New(ctorVal, args...); err == nil {
						b.handles[id] = res
					}
				})
			}

		case "callVoid":
			mod := mapStr(op, "mod")
			path := mapStr(op, "path")
			sc := b.scopeOf(mapInt(op, "scopeId"))
			if thisVal, fnVal, ok := b.resolveTarget(mod, path); ok {
				if fn, ok := js.AssertFunction(fnVal); ok {
					var args []js.Value
					for _, a := range mapSlice(op, "args") {
						args = append(args, b.toHostValue(a))
					}
					sc.Run(func() {
						prev := r.cx
						r.cx = sc
						defer func() { r.cx = prev }()
						_, _ = fn(thisVal, args...)
					})
				}
			}

		case "callHandle":
			id := mapInt(op, "id")
			method := mapStr(op, "method")
			if o, ok := b.handles[id].(*js.Object); ok {
				if fn, ok := js.AssertFunction(o.Get(method)); ok {
					var args []js.Value
					for _, a := range mapSlice(op, "args") {
						args = append(args, b.toHostValue(a))
					}
					_, _ = fn(o, args...)
				}
			}

		case "disposeHandle":
			id := mapInt(op, "id")
			h := b.handles[id]
			delete(b.handles, id)
			delete(b.handleSig, id)
			delete(b.handleObj, id)
			if o, ok := h.(*js.Object); ok {
				if fn, ok := js.AssertFunction(o); ok {
					_, _ = fn(js.Undefined())
				} else if fn, ok := js.AssertFunction(o.Get("dispose")); ok {
					_, _ = fn(o)
				} else if fn, ok := js.AssertFunction(o.Get("remove")); ok {
					_, _ = fn(o)
				}
			}

		case "rt":
			method := mapStr(op, "method")
			if thisVal, fnVal, ok := b.resolveTarget("nsgo:rt", method); ok {
				if fn, ok := js.AssertFunction(fnVal); ok {
					var args []js.Value
					for _, a := range mapSlice(op, "args") {
						args = append(args, b.toHostValue(a))
					}
					_, _ = fn(thisVal, args...)
				}
			}
		}
	}
}

func (b *isolatedBridge) makePropsObject(sc *core.Scope, op map[string]any) (*js.Object, map[string]*core.Signal[js.Value]) {
	r := b.r
	obj := r.vm.NewObject()
	sigs := map[string]*core.Signal[js.Value]{}
	getterSet := map[string]bool{}
	for _, item := range mapSlice(op, "getterKeys") {
		if k, ok := item.(string); ok {
			getterSet[k] = true
		}
	}
	for _, item := range mapSlice(op, "keys") {
		k, ok := item.(string)
		if !ok {
			continue
		}
		if getterSet[k] {
			sig := core.NewSignalFunc[js.Value](sc, js.Undefined(), strictEquals)
			sigs[k] = sig
			_ = obj.DefineAccessorProperty(k, r.accessor(sig.Get), nil, js.FLAG_TRUE, js.FLAG_TRUE)
		} else {
			_ = obj.Set(k, js.Undefined())
		}
	}
	return obj, sigs
}

func (b *isolatedBridge) applyCreateFace(op map[string]any) {
	r := b.r
	id := mapInt(op, "id")
	mod := mapStr(op, "mod")
	name := mapStr(op, "name")
	sc := b.scopeOf(mapInt(op, "scopeId"))
	obj, sigs := b.makePropsObject(sc, op)
	b.faceSigs[id] = sigs
	b.faceObjs[id] = obj

	var compFn js.Value
	if strings.HasPrefix(mod, "native:") {
		if nc, ok := js.AssertFunction(r.require("nsgo").(*js.Object).Get("nativeComponent")); ok {
			compFn, _ = nc(js.Undefined(), r.vm.ToValue(name))
		}
	} else {
		_, compFn, _ = b.resolveTarget(mod, name)
	}
	fn, ok := js.AssertFunction(compFn)
	if !ok {
		return
	}
	sc.Run(func() {
		prev := r.cx
		r.cx = sc
		defer func() { r.cx = prev }()
		if res, err := fn(js.Undefined(), obj); err == nil {
			if nd := r.nodeOf(res); nd != nil {
				b.nodes[id] = nd
			}
		}
	})
}

func (b *isolatedBridge) applyCreateNavPage(op map[string]any) {
	r := b.r
	id := mapInt(op, "id")
	sc := b.scopeOf(mapInt(op, "scopeId"))
	obj, sigs := b.makePropsObject(sc, op)
	b.handleSig[id] = sigs
	b.handleObj[id] = obj

	_, fnVal, ok := b.resolveTarget("nsgo/base", "NavPage")
	if !ok {
		return
	}
	fn, ok := js.AssertFunction(fnVal)
	if !ok {
		return
	}
	sc.Run(func() {
		prev := r.cx
		r.cx = sc
		defer func() { r.cx = prev }()
		if res, err := fn(js.Undefined(), obj); err == nil {
			b.handles[id] = res
		}
	})
}

// ---- Manifest & Environment Snapshots ----

func (b *isolatedBridge) buildManifest() (map[string]any, []any) {
	r := b.r
	knownSet := map[string]bool{}
	for name := range moduleTable {
		knownSet[name] = true
	}
	for _, name := range registeredModuleNames() {
		knownSet[name] = true
	}
	knownList := make([]string, 0, len(knownSet))
	for name := range knownSet {
		knownList = append(knownList, name)
	}
	slices.Sort(knownList)
	knownOut := make([]any, len(knownList))
	for i, name := range knownList {
		knownOut[i] = name
	}

	manifest := map[string]any{}
	for _, modName := range knownList {
		if !r.sandbox.importsModule(modName) {
			continue
		}
		if modName == "nsgo:rt" {
			manifest[modName] = map[string]any{}
			continue
		}
		mObj, ok := r.require(modName).(*js.Object)
		if !ok {
			continue
		}
		modSpec := map[string]any{}
		for _, k := range mObj.Keys() {
			v := mObj.Get(k)
			modSpec[k] = classifyExport(modName, k, v)
		}
		manifest[modName] = modSpec
	}
	return manifest, knownOut
}

func classifyExport(mod, name string, v js.Value) map[string]any {
	if v == nil || js.IsUndefined(v) || js.IsNull(v) {
		return map[string]any{"kind": "val", "value": nil}
	}
	o, isObj := v.(*js.Object)
	if !isObj {
		return map[string]any{"kind": "val", "value": v.Export()}
	}
	if isRegisteredClass(mod, name) || (mod == "nsgo" && (name == "AnimatedValue" || name == "AnimatedNode" || name == "CanvasGradient" || name == "CanvasRenderingContext2D" || name == "Path2D")) {
		var statics []any
		for _, sk := range o.Keys() {
			statics = append(statics, sk)
		}
		return map[string]any{"kind": "class", "statics": statics}
	}
	if isFunction(o) {
		var statics []any
		for _, sk := range o.Keys() {
			statics = append(statics, sk)
		}
		if isComponentExport(mod, name) {
			return map[string]any{"kind": "component", "statics": statics}
		}
		return map[string]any{"kind": "fn", "statics": statics}
	}
	members := map[string]any{}
	for _, sk := range o.Keys() {
		sv := o.Get(sk)
		if isFunction(sv) {
			members[sk] = map[string]any{"kind": "fn"}
		} else if mod == "nsgo" && name == "host" && sk == "params" {
			members[sk] = map[string]any{"kind": "accessor"}
		} else if so, ok := sv.(*js.Object); ok {
			members[sk] = map[string]any{"kind": "obj", "value": exportPlainObject(so)}
		} else if sv != nil && !js.IsUndefined(sv) {
			members[sk] = map[string]any{"kind": "val", "value": sv.Export()}
		} else {
			members[sk] = map[string]any{"kind": "val", "value": nil}
		}
	}
	return map[string]any{"kind": "service", "members": members}
}

func isComponentExport(mod, name string) bool {
	if isRegisteredComponent(mod, name) {
		return true
	}
	switch mod {
	case "nsgo":
		switch name {
		case "View", "Text", "Scroll", "Image", "Overlay", "WebView", "Video", "List", "Canvas",
			"Shader", "ShaderEffect", "ShaderTransition", "ScreenEffect", "ScreenTransition", "ErrorBoundary":
			return true
		}
	case "nsgo/base":
		switch name {
		case "Input", "Dialog", "NavStack", "NavPage", "List", "AlertDialog", "ToastManager", "ToastStack", "ResizablePanel":
			return true
		}
	case "nsgo/mobile":
		switch name {
		case "InputAccessoryView", "TabBar":
			return true
		}
	}
	return false
}

func (b *isolatedBridge) allowedNatives() []any {
	var out []any
	for _, m := range b.r.sandbox.modules {
		if m == "*" {
			out = append(out, "*")
		} else if name, ok := strings.CutPrefix(m, "native:"); ok {
			out = append(out, name)
		}
	}
	return out
}

func (b *isolatedBridge) envSnapshot() map[string]any {
	r := b.r
	var snap map[string]any
	core.Untracked(r.cx, func() bool {
		e := r.cx.Env()
		scheme := "light"
		if e.Dark() {
			scheme = "dark"
		}
		idiom := map[core.Idiom]string{
			core.IdiomPhone:   "phone",
			core.IdiomTablet:  "pad",
			core.IdiomDesktop: "desktop",
			core.IdiomMac:     "mac",
			core.IdiomTV:      "tv",
			core.IdiomWatch:   "watch",
		}[e.Idiom()]
		a := core.App()
		appInfo := map[string]any{
			"id":          nilIfEmpty(a.ID),
			"name":        nilIfEmpty(a.Name),
			"version":     nilIfEmpty(a.Version),
			"build":       nilIfEmpty(a.Build),
			"development": a.Development,
		}
		win := e.Window()
		sa := e.SafeArea()
		snap = map[string]any{
			"windowSize":          map[string]any{"width": float64(win.Width), "height": float64(win.Height)},
			"colorScheme":         scheme,
			"fontScale":           float64(e.FontScale()),
			"pixelRatio":          float64(e.PixelRatio()),
			"locale":              e.Locale(),
			"reduceMotion":        e.ReduceMotion(),
			"increaseContrast":    e.IncreaseContrast(),
			"lifecycle":           e.Lifecycle().String(),
			"screenReaderEnabled": e.ScreenReader(),
			"safeArea":            map[string]any{"top": float64(sa.Top), "right": float64(sa.Right), "bottom": float64(sa.Bottom), "left": float64(sa.Left)},
			"keyboardFrame":       map[string]any{"height": float64(e.KeyboardHeight()), "duration": float64(e.KeyboardDuration().Milliseconds())},
			"platform":            map[string]any{"os": string(e.OS()), "version": e.OSVersion().String(), "idiom": idiom, "catalyst": false},
			"appInfo":             appInfo,
			"hostAvailable":       r.cx.HasHost(),
			"hostParams":          r.cx.HostParams(),
			"tokens":              b.tokenSnapshot(),
		}
		return true
	})
	return snap
}

func (b *isolatedBridge) tokenSnapshot() map[string]any {
	out := map[string]any{}
	theme := b.r.cx.Theme()
	p := b.r.cx.Palette()
	for k, v := range theme.Tokens {
		if l, ok := cssLength(v); ok {
			out[k] = float64(l)
		} else {
			out[k] = v
		}
	}
	for _, role := range []string{
		"background", "surface", "foreground", "muted", "border", "accent", "danger", "success", "warning",
	} {
		if c, ok := p.Color(role); ok {
			out[role] = hexColor(c)
		}
	}
	return out
}

func (b *isolatedBridge) watchEnv() {
	r := b.r
	first := true
	r.cx.Effect(func() {
		e := r.cx.Env()
		win := e.Window()
		sa := e.SafeArea()
		scheme := "light"
		if e.Dark() {
			scheme = "dark"
		}
		patch := map[string]any{
			"windowSize":          map[string]any{"width": float64(win.Width), "height": float64(win.Height)},
			"colorScheme":         scheme,
			"fontScale":           float64(e.FontScale()),
			"pixelRatio":          float64(e.PixelRatio()),
			"locale":              e.Locale(),
			"reduceMotion":        e.ReduceMotion(),
			"increaseContrast":    e.IncreaseContrast(),
			"lifecycle":           e.Lifecycle().String(),
			"screenReaderEnabled": e.ScreenReader(),
			"safeArea":            map[string]any{"top": float64(sa.Top), "right": float64(sa.Right), "bottom": float64(sa.Bottom), "left": float64(sa.Left)},
			"keyboardFrame":       map[string]any{"height": float64(e.KeyboardHeight()), "duration": float64(e.KeyboardDuration().Milliseconds())},
			"tokens":              b.tokenSnapshot(),
		}
		if first {
			first = false
			return
		}
		b.postToGuest(map[string]any{"type": "env", "patch": patch})
	})
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ---- Value Marshaling (Go port values <-> Host js.Value) ----

func (b *isolatedBridge) toHostValue(v any) js.Value {
	r := b.r
	switch x := v.(type) {
	case nil:
		return js.Null()
	case bool, int, int64, float64, string:
		return r.vm.ToValue(x)
	case []byte:
		return r.vm.ToValue(r.vm.NewArrayBuffer(x))
	case []any:
		items := make([]any, len(x))
		for i, item := range x {
			items[i] = b.toHostValue(item)
		}
		return r.vm.NewArray(items...)
	case map[string]any:
		if mapInt(x, "__nsgo_u") == 1 {
			return js.Undefined()
		}
		if _, ok := x["__nsgo_fn"]; ok {
			fnID := mapInt(x, "__nsgo_fn")
			o := r.function(func(call js.FunctionCall) js.Value {
				return b.invokeGuest(fnID, call.Arguments)
			})
			_ = o.SetSymbol(guestFnKey, fnID)
			return o
		}
		if _, ok := x["__nsgo_node"]; ok {
			if nd := b.nodes[mapInt(x, "__nsgo_node")]; nd != nil {
				return nd.value()
			}
			return js.Undefined()
		}
		if _, ok := x["__nsgo_obj"]; ok {
			if h := b.handles[mapInt(x, "__nsgo_obj")]; h != nil {
				return h
			}
			return js.Undefined()
		}
		if raw, ok := x["__nsgo_bytes"].([]byte); ok {
			return r.vm.ToValue(r.vm.NewArrayBuffer(raw))
		}
		if _, ok := x["__nsgo_date"]; ok {
			return r.vm.ToValue(time.UnixMilli(int64(mapFloat(x, "__nsgo_date"))))
		}
		o := r.vm.NewObject()
		for k, val := range x {
			if k == "__proto__" || k == "constructor" || k == "prototype" {
				continue
			}
			_ = o.Set(k, b.toHostValue(val))
		}
		return o
	default:
		return js.Undefined()
	}
}

func (b *isolatedBridge) toGuestValue(v js.Value) any {
	return b.toGuestValueDepth(v, 0)
}

func (b *isolatedBridge) toGuestValueDepth(v js.Value, depth int) any {
	if depth > 32 || v == nil || js.IsUndefined(v) {
		return map[string]any{"__nsgo_u": 1}
	}
	if js.IsNull(v) {
		return nil
	}
	o, ok := v.(*js.Object)
	if !ok {
		return v.Export()
	}
	if buf, ok := o.Export().(js.ArrayBuffer); ok {
		return map[string]any{"__nsgo_bytes": buf.Bytes()}
	}
	if o.ClassName() == "Error" {
		return map[string]any{"__nsgo_err": encodeHostJSValueError(o)}
	}
	if b.r.isArray(o) {
		items := b.r.arrayItems(o)
		out := make([]any, len(items))
		for i, it := range items {
			out[i] = b.toGuestValueDepth(it, depth+1)
		}
		return out
	}
	keys := o.Keys()
	var methods []any
	hasMethod := false
	for _, k := range keys {
		if isFunction(o.Get(k)) {
			hasMethod = true
			methods = append(methods, k)
		}
	}
	if hasMethod {
		hid := len(b.handles) + 100000
		for b.handles[hid] != nil {
			hid++
		}
		b.handles[hid] = o
		return map[string]any{"__nsgo_obj": hid, "methods": methods}
	}
	out := map[string]any{}
	for _, k := range keys {
		out[k] = b.toGuestValueDepth(o.Get(k), depth+1)
	}
	return out
}

func exportPlainObject(o *js.Object) map[string]any {
	out := map[string]any{}
	for _, k := range o.Keys() {
		v := o.Get(k)
		if v == nil || js.IsUndefined(v) || isFunction(v) {
			continue
		}
		if sub, ok := v.(*js.Object); ok {
			out[k] = exportPlainObject(sub)
		} else {
			out[k] = v.Export()
		}
	}
	return out
}

// ---- Map Accessor Helpers ----

func mapStr(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func mapBool(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}

func mapInt(m map[string]any, key string) int {
	switch x := m[key].(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

func mapFloat(m map[string]any, key string) float64 {
	switch x := m[key].(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case int:
		return float64(x)
	}
	return 0
}

func mapObj(m map[string]any, key string) map[string]any {
	o, _ := m[key].(map[string]any)
	return o
}

func mapSlice(m map[string]any, key string) []any {
	s, _ := m[key].([]any)
	return s
}

// ---- Error & Source Map Helpers ----

const isolatedInlineMap = "//# sourceMappingURL=data:application/json;base64,"

var isolatedPosRE = regexp.MustCompile(`([^\s()]+):(\d+):(\d+)`)

func (b *isolatedBridge) parseSourceMap(name, src string) {
	b.maps[name] = nil
	if i := strings.LastIndex(src, isolatedInlineMap); i >= 0 {
		data, _, _ := strings.Cut(src[i+len(isolatedInlineMap):], "\n")
		if raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(data)); err == nil {
			if sm, err := sourcemap.Parse(name, raw); err == nil {
				b.maps[name] = sm
			}
		}
	}
}

func (b *isolatedBridge) mapStack(stack string) string {
	if len(b.maps) == 0 || stack == "" {
		return stack
	}
	return isolatedPosRE.ReplaceAllStringFunc(stack, func(at string) string {
		m := isolatedPosRE.FindStringSubmatch(at)
		if len(m) != 4 {
			return at
		}
		sm := b.maps[m[1]]
		if sm == nil {
			return at
		}
		line, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		if src, _, l, c, ok := sm.Source(line, col-1); ok {
			return fmt.Sprintf("%s:%d:%d", src, l, c+1)
		}
		return at
	})
}

// decodeRejection matches runtime.rejection for Plugin.Call errors: message
// plus optional code, without appending a JS stack trace.
func (b *isolatedBridge) decodeRejection(v map[string]any) error {
	if v == nil {
		return errors.New("tsx: script error")
	}
	msg := mapStr(v, "message")
	if code := mapStr(v, "code"); code != "" {
		return &core.Error{
			Code:       code,
			Permission: mapStr(v, "permission"),
			Resource:   mapStr(v, "resource"),
			Message:    msg,
		}
	}
	return errors.New(msg)
}

func (b *isolatedBridge) decodeError(v map[string]any) error {
	if v == nil {
		return errors.New("tsx: script error")
	}
	msg := mapStr(v, "message")
	if code := mapStr(v, "code"); code != "" {
		return &core.Error{
			Code:       code,
			Permission: mapStr(v, "permission"),
			Resource:   mapStr(v, "resource"),
			Message:    msg,
		}
	}
	if mapBool(v, "raw") {
		return errors.New(msg)
	}
	if stack := mapStr(v, "stack"); stack != "" {
		mapped := b.mapStack(stack)
		if strings.Contains(mapped, msg) {
			return errors.New(mapped)
		}
		return errors.New(msg + "\n" + mapped)
	}
	return errors.New(msg)
}

func encodeGoError(err error) map[string]any {
	out := map[string]any{"name": "Error", "message": err.Error()}
	if code := core.ErrorCode(err); code != "" {
		out["code"] = code
	}
	var coded *core.Error
	if errors.As(err, &coded) {
		if coded.Permission != "" {
			out["permission"] = coded.Permission
		}
		if coded.Resource != "" {
			out["resource"] = coded.Resource
		}
	}
	return out
}

func encodeHostJSValueError(v js.Value) map[string]any {
	o, ok := v.(*js.Object)
	if !ok {
		return map[string]any{"name": "Error", "message": fmt.Sprint(v)}
	}
	out := map[string]any{"name": "Error", "message": o.String()}
	if n := o.Get("name"); defined(n) {
		out["name"] = n.String()
	}
	if m := o.Get("message"); defined(m) {
		out["message"] = m.String()
	}
	if c := o.Get("code"); defined(c) {
		out["code"] = c.String()
	}
	if p := o.Get("permission"); defined(p) {
		out["permission"] = p.String()
	}
	if res := o.Get("resource"); defined(res) {
		out["resource"] = res.String()
	}
	return out
}

func encodeRecoveredError(x any) map[string]any {
	switch e := x.(type) {
	case *js.Exception:
		return encodeHostJSValueError(e.Value())
	case js.Value:
		return encodeHostJSValueError(e)
	case error:
		return encodeGoError(e)
	default:
		return map[string]any{"name": "Error", "message": fmt.Sprint(x)}
	}
}
