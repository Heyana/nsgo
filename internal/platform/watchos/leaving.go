package watchos

import (
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// allProperties are what a style transition with no properties named moves.
const allProperties = driver.TransitionOpacity | driver.TransitionBackground | driver.TransitionTransform |
	driver.TransitionBorder | driver.TransitionShadow

// look is what a node's SwiftUI modifiers show (Host.swift's look op).
type look struct {
	opacity float64
	hidden  bool
	clips   bool
	// inset is how far the clip stands inside the box on each side, the border's widths (driver.ViewStyle.ClipBox), and radii are its corners there.
	inset     driver.Edges
	radii     driver.Corners
	transform [6]float64
}

// boxKey is the style and size boxOps last drew.
type boxKey struct {
	style driver.ViewStyle
	w, h  float32
}

// transition is a view's style moving from one ViewStyle to the next.
type transition struct {
	from, to driver.ViewStyle
	start    time.Time // after the delay
	duration time.Duration
	curve    driver.Curve
	props    driver.TransitionProperty
	shown    driver.ViewStyle
}

// layoutFade tracks a view appearing (Create) or leaving (Delete) during a
// commit's LayoutAnimation.
type layoutFade struct {
	tr    driver.LayoutTransition
	start time.Time
	out   bool
	from  float64 // visibility when leaving began (1, or mid-entrance part)
	part  float64 // current visibility, 0 (gone) to 1 (full)
}

func newAppearFade(tr driver.LayoutTransition, now time.Time) *layoutFade {
	return &layoutFade{tr: tr, start: now, from: 1, part: 0}
}

func newLeaveFade(tr driver.LayoutTransition, now time.Time, from float64) *layoutFade {
	return &layoutFade{tr: tr, start: now, out: true, from: from, part: from}
}

// step advances the fade to now and reports whether it has finished.
func (f *layoutFade) step(now time.Time) bool {
	p, done := f.tr.At(now.Sub(f.start))
	if f.out {
		f.part = f.from * (1 - p)
	} else {
		f.part = p
	}
	return done
}

// values returns the opacity multiplier and center scale for f's current progress.
func (f *layoutFade) values() (opacity, scale float64) {
	if f == nil {
		return 1, 1
	}
	if f.tr.Property == driver.LayoutScale {
		return 1, max(f.part, 1e-3)
	}
	return min(max(f.part, 0), 1), 1
}

// motion is a view's part of a layout animation: its frame moving, and it
// appearing or leaving.
type motion struct {
	move *driver.LayoutMotion
	fade *layoutFade
}

// layoutBatch is what applying a commit with a LayoutAnimation keeps.
type layoutBatch struct {
	anim       *driver.LayoutAnimation
	now        time.Time
	created    map[*watchNode]bool
	matchedOld map[*watchNode]bool
}

// hostIDs allocates unique host node IDs for SwiftUI's Store.nodes so a
// leaving ghost never shares an ID or SwiftUI ForEach identity with a new view
// that reuses its logical driver.NodeID.
type hostIDs struct {
	next uint32
	used map[driver.NodeID]struct{}
}

func (h *hostIDs) alloc() driver.NodeID {
	if h.used == nil {
		h.used = map[driver.NodeID]struct{}{}
	}
	for {
		h.next++
		if h.next == 0 {
			h.next = 1
		}
		id := driver.NodeID(h.next)
		if _, taken := h.used[id]; !taken {
			h.used[id] = struct{}{}
			return id
		}
	}
}

func (h *hostIDs) free(id driver.NodeID) {
	delete(h.used, id)
}

func (h *hostIDs) live() int {
	return len(h.used)
}

// watchNode is the host-independent state of one watchOS view node, including
// its unique SwiftUI hostID, tree links, animations, leaving-ghost state and
// last-sent look.
type watchNode struct {
	id        driver.NodeID
	hostID    driver.NodeID
	kind      driver.ViewKind
	parent    *watchNode
	children  []*watchNode
	leaving   []*watchNode // deleted children animating out under this parent
	leftFrom  *watchNode   // parent a leaving ghost root stays under on the host
	deleted   bool
	retired   bool // kept alive on the host while a leaving animation runs
	presented bool

	frame           driver.Bounds
	style           driver.ViewStyle
	styled          bool
	born            uint64
	props           driver.ViewProps
	events          driver.EventMask
	overflow        driver.Bounds
	transformedKids bool

	animated *driver.AnimatedValues
	moving   *motion
	trans    *transition
	kfStart  time.Time
	hasKF    bool

	// A scroll view's config, content size, offset, the app's scrolls applied
	// and its phase (Host.swift's codes).
	scroll  driver.ScrollConfig
	content driver.Size
	offset  driver.Point
	applied uint32
	phase   uint8

	// What the host was last sent.
	sentFrame  driver.Bounds
	hasFrame   bool
	sentLook   look
	sentInk    driver.Bounds
	sentAccess watchAccess
	draws      bool
	culled     bool
	back       []driver.DrawOp
	front      []driver.DrawOp
	boxFor     boxKey

	// owner links back to the ios-side *view when mounted in a watchOS surface.
	owner any
}

// shownFrame is where n shows now: mid layout animation or its own.
func (n *watchNode) shownFrame() driver.Bounds {
	if n.moving != nil && n.moving.move != nil {
		return n.moving.move.Shown()
	}
	return n.frame
}

// shownStyle is the style n shows now: mid transition or its own, with the
// values the animator binds over it.
func (n *watchNode) shownStyle() driver.ViewStyle {
	st := n.style
	if n.trans != nil {
		st = n.trans.shown
	}
	if n.animated != nil {
		st = n.animated.Apply(st)
	}
	return st
}

// keyframeAt is n's keyframe at now, and whether it has keyframes.
func (n *watchNode) keyframeAt(now time.Time) (driver.Keyframe, bool) {
	if !n.hasKF || n.style.Animation.Keyframes == 0 {
		return driver.Keyframe{Opacity: 1, Scale: 1}, false
	}
	k, _ := n.style.Animation.At(now.Sub(n.kfStart))
	return k, true
}

// layoutPart is what an appearing or leaving view multiplies its opacity by,
// and scales it by around its center.
func (n *watchNode) layoutPart() (opacity, scale float64) {
	if n.moving == nil || n.moving.fade == nil {
		return 1, 1
	}
	return n.moving.fade.values()
}

// pointerMatrixAt uses exactly the affine transform sent to SwiftUI at now.
func (n *watchNode) pointerMatrixAt(now time.Time) driver.Matrix3D {
	k, _ := n.keyframeAt(now)
	_, scale := n.layoutPart()
	return affinePointerMatrix(n.shownStyle(), n.shownFrame(), k, scale)
}

// within reports whether p, in n's coordinates, is on n or in its hit slop.
func (n *watchNode) within(p driver.Point) bool {
	sl, f := n.props.HitSlop, n.shownFrame()
	return p.X >= -sl.Left && p.Y >= -sl.Top && p.X < f.Size.Width+sl.Right && p.Y < f.Size.Height+sl.Bottom
}

// watchTree owns the watchOS view tree, host ID allocation, leaving ghosts,
// layout animations and host op writer.
type watchTree struct {
	batch    uint64
	views    []*watchNode
	byHost   map[driver.NodeID]*watchNode
	ids      hostIDs
	root     driver.NodeID
	rootHost driver.NodeID
	epoch    time.Time

	w       writer
	dirty   map[*watchNode]bool
	redraw  map[*watchNode]bool
	recull  bool
	motions map[*watchNode]bool
	leaving []*watchNode // active top-level leaving ghost roots
	sticky  stickyTracker
}

func (t *watchTree) initTree(epoch time.Time) {
	t.epoch = epoch
	t.byHost = map[driver.NodeID]*watchNode{}
	t.dirty = map[*watchNode]bool{}
	t.redraw = map[*watchNode]bool{}
	t.motions = map[*watchNode]bool{}
	t.sticky.hostID = t.hostIDOf
}

func (t *watchTree) hostIDOf(id driver.NodeID) driver.NodeID {
	if n := t.node(id); n != nil && !n.deleted {
		return n.hostID
	}
	return 0
}

// placedFrame is n.shownFrame shifted by any sticky offset its scroll view
// applies, where n shows and receives touches on the surface.
func (t *watchTree) placedFrame(n *watchNode) driver.Bounds {
	if n.retired {
		return n.shownFrame()
	}
	return placedBounds(n.shownFrame(), t.sticky.shift(n.id))
}

func (t *watchTree) ensureMaps() {
	if t.byHost == nil {
		t.byHost = map[driver.NodeID]*watchNode{}
	}
	if t.dirty == nil {
		t.dirty = map[*watchNode]bool{}
	}
	if t.redraw == nil {
		t.redraw = map[*watchNode]bool{}
	}
	if t.motions == nil {
		t.motions = map[*watchNode]bool{}
	}
}

func (t *watchTree) node(id driver.NodeID) *watchNode {
	if int(id) < len(t.views) {
		return t.views[id]
	}
	return nil
}

func (t *watchTree) hostNode(hostID driver.NodeID) *watchNode {
	return t.byHost[hostID]
}

// touchedNode marks n's look and ink to be sent again.
func (t *watchTree) touchedNode(n *watchNode) {
	t.ensureMaps()
	t.dirty[n] = true
	if !n.retired {
		t.recull = true
	}
}

func (t *watchTree) createNode(kind driver.ViewKind, id driver.NodeID) *watchNode {
	t.ensureMaps()
	for len(t.views) <= int(id) {
		t.views = append(t.views, nil)
	}
	hostID := t.ids.alloc()
	n := &watchNode{
		id:     id,
		hostID: hostID,
		kind:   kind,
		style:  driver.DefaultViewStyle,
		born:   t.batch,
	}
	t.views[id] = n
	t.byHost[hostID] = n
	// The host's kinds are driver.ViewKind's up to canvases; the rest are
	// plain nodes there.
	k := kind
	if k > driver.KindCanvas {
		k = driver.KindView
	}
	t.w.op(opCreate, hostID)
	t.w.byte(byte(k))
	t.touchedNode(n)
	return n
}

func (t *watchTree) detachNode(n *watchNode) {
	if p := n.parent; p != nil {
		if i := slices.Index(p.children, n); i >= 0 {
			p.children = slices.Delete(p.children, i, i+1)
		}
		n.parent = nil
	}
}

func (t *watchTree) insertNode(n, p, before *watchNode, lb *layoutBatch) {
	t.detachNode(n)
	at := len(p.children)
	var beforeHost driver.NodeID
	if before != nil {
		if i := slices.Index(p.children, before); i >= 0 {
			at = i
			beforeHost = before.hostID
		}
	}
	p.children = append(p.children, nil)
	copy(p.children[at+1:], p.children[at:])
	p.children[at] = n
	n.parent = p
	t.recull = true
	t.w.op(opInsert, n.hostID, p.hostID, beforeHost)
	if lb != nil && lb.created[n] {
		t.appearNode(n, lb)
	}
}

func (t *watchTree) removeNode(n *watchNode) {
	t.detachNode(n)
	t.w.op(opRemove, n.hostID)
}

func (t *watchTree) setRootNode(id driver.NodeID) {
	var rootHost driver.NodeID
	if r := t.node(id); r != nil {
		rootHost = r.hostID
	}
	if id != t.root || rootHost != t.rootHost {
		t.root = id
		t.rootHost = rootHost
		t.w.op(opRoot, rootHost)
	}
}

func (t *watchTree) newLayoutBatch(anim *driver.LayoutAnimation, now time.Time, reduced bool) *layoutBatch {
	if anim == nil || reduced {
		return nil
	}
	return &layoutBatch{
		anim:       anim,
		now:        now,
		created:    map[*watchNode]bool{},
		matchedOld: map[*watchNode]bool{},
	}
}

func (t *watchTree) motionOfNode(n *watchNode) *motion {
	t.ensureMaps()
	if n.moving == nil {
		n.moving = &motion{}
	}
	t.motions[n] = true
	return n.moving
}

func (t *watchTree) dropMotionNode(n *watchNode) {
	if m := n.moving; m != nil && m.move == nil && m.fade == nil {
		n.moving = nil
	}
}

// setNodeStyle applies st to n, animating to it when it has a transition.
func (t *watchTree) setNodeStyle(n *watchNode, st *driver.ViewStyle, now time.Time, reduced bool) {
	from := n.shownStyle()
	if n.animated != nil {
		from = n.style
		if n.trans != nil {
			from = n.trans.shown
		}
	}
	styled := n.styled || n.born != t.batch
	if st.Animation != n.style.Animation || !styled {
		if st.Animation.Keyframes != 0 {
			n.kfStart = now
			n.hasKF = true
			t.ensureMaps()
			t.motions[n] = true
		} else {
			n.hasKF = false
		}
	}
	n.style, n.styled, n.trans = *st, true, nil
	t.touchedNode(n)
	tr := st.Transition
	if !styled || tr.Duration <= 0 || reduced {
		return
	}
	props := tr.Properties
	if props == 0 {
		props = allProperties
	}
	first, last := driver.MixStyles(from, *st, 0, props), driver.MixStyles(from, *st, 1, props)
	if first == last {
		return
	}
	n.trans = &transition{
		from: from, to: *st, start: now.Add(tr.Delay), duration: tr.Duration,
		curve: tr.Easing, props: props, shown: first,
	}
	t.ensureMaps()
	t.motions[n] = true
}

// setNodeFrame applies frame to n, moving to it with the batch's Update when n
// existed before the batch.
func (t *watchTree) setNodeFrame(n *watchNode, frame driver.Bounds, lb *layoutBatch, now time.Time) {
	old := n.shownFrame()
	target := n.frame
	if n.moving != nil && n.moving.move != nil {
		target = n.moving.move.Target()
	}
	n.frame = frame
	t.touchedNode(n)
	if lb == nil || lb.anim.Update == nil || lb.created[n] || n.kind == driver.KindModal {
		if m := n.moving; m != nil {
			m.move = nil
			t.dropMotionNode(n)
		}
		return
	}
	stamp := now.Sub(t.epoch)
	if n.moving != nil && n.moving.move != nil {
		if frame == target {
			return
		}
		n.moving.move.Retarget(frame, *lb.anim.Update, stamp)
		if n.moving.move.Done() {
			n.moving.move = nil
			t.dropMotionNode(n)
		}
		return
	}
	if old == frame {
		return
	}
	m := t.motionOfNode(n)
	lm := driver.StartLayoutMotion(old, frame, *lb.anim.Update, stamp)
	m.move = &lm
}

// appearNode brings n in with the batch's Create when the batch made it and
// inserted it into a view that existed before.
func (t *watchTree) appearNode(n *watchNode, lb *layoutBatch) {
	if lb.anim.Create == nil || n.parent == nil || lb.created[n.parent] || n.kind == driver.KindModal {
		return
	}
	m := t.motionOfNode(n)
	m.fade = newAppearFade(*lb.anim.Create, lb.now)
	t.touchedNode(n)
}

// contentAtNode returns where n's content shows on the screen: its placed
// frame mid animation (including sticky shift), carried through its parents',
// less scroll offsets.
func (t *watchTree) contentAtNode(n *watchNode) driver.Point {
	var at driver.Point
	for p := n; p != nil; p = p.parent {
		o := t.placedFrame(p).Origin
		at.X += o.X - p.offset.X
		at.Y += o.Y - p.offset.Y
	}
	return at
}

// matchNode moves n, which the batch made, from where old, which it deletes,
// shows, with the batch's Update rather than appearing, and hides old at once
// so old does not linger as a leaving ghost.
func (t *watchTree) matchNode(n, old *watchNode, lb *layoutBatch) {
	if lb == nil || lb.anim.Update == nil || old == nil || n.parent == nil || old.parent == nil {
		return
	}
	was, in := t.contentAtNode(old.parent), t.contentAtNode(n.parent)
	o := t.placedFrame(old)
	from := driver.Bounds{
		Origin: driver.Point{X: was.X + o.Origin.X - in.X, Y: was.Y + o.Origin.Y - in.Y},
		Size:   o.Size,
	}
	old.style.Hidden = true
	lb.matchedOld[old] = true
	t.touchedNode(old)
	m := t.motionOfNode(n)
	m.fade = nil
	lm := driver.StartLayoutMotion(from, n.frame, *lb.anim.Update, lb.now.Sub(t.epoch))
	m.move = &lm
	t.touchedNode(n)
}

// visibleInTree reports whether n is mounted in the root tree with no hidden
// or culled ancestor.
func (t *watchTree) visibleInTree(n *watchNode) bool {
	if n == nil || n.deleted || n.parent == nil || n.parent.deleted {
		return false
	}
	var top *watchNode
	for p := n; p != nil; p = p.parent {
		if p.style.Hidden || p.culled {
			return false
		}
		top = p
	}
	return top != nil && top == t.node(t.root)
}

// canLeave reports whether n should stay on the host as a leaving ghost while
// lb.anim.Delete runs.
func (t *watchTree) canLeave(n *watchNode, lb *layoutBatch) bool {
	if lb == nil || lb.anim.Delete == nil || lb.anim.Delete.End() <= 0 {
		return false
	}
	if lb.created[n] || lb.matchedOld[n] || n.kind == driver.KindModal {
		return false
	}
	if !t.visibleInTree(n) {
		return false
	}
	sz := n.shownFrame().Size
	return sz.Width > 0 && sz.Height > 0
}

// retireNode detaches n's logical identity so its driver.NodeID is immediately
// free for reuse, freezes its drawn state, marks it non-interactive on the
// host, and keeps its host node alive until its leaving animation ends.
func (t *watchTree) retireNode(n *watchNode, onRetire func(*watchNode)) {
	n.frame = n.shownFrame()
	n.style = n.shownStyle()
	n.trans = nil
	n.animated = nil
	n.hasKF = false
	if onRetire != nil {
		onRetire(n)
	}
	n.retired = true
	n.deleted = true
	if int(n.id) < len(t.views) && t.views[n.id] == n {
		t.views[n.id] = nil
	}
	t.w.op(opRetire, n.hostID)
}

// deleteNode deletes n: when lb carries an active Delete transition for a
// mounted visible subtree, it keeps the subtree alive on the host as a
// non-interactive leaving ghost while freeing n's driver.NodeID immediately.
// It reports whether n was retained as a leaving ghost.
func (t *watchTree) deleteNode(n *watchNode, lb *layoutBatch, onRetire, onDestroy func(*watchNode)) bool {
	if n.parent != nil && n.parent.retired {
		// Goes with the leaving subtree it is inside.
		n.moving = nil
		delete(t.motions, n)
		t.retireNode(n, onRetire)
		t.touchedNode(n)
		return true
	}
	if !t.canLeave(n, lb) {
		t.destroyNode(n, onRetire, onDestroy)
		return false
	}
	from := 1.0
	if n.moving != nil && n.moving.fade != nil && n.moving.fade.tr.Property == lb.anim.Delete.Property {
		from = min(max(n.moving.fade.part, 0), 1)
	}
	p := n.parent
	t.detachNode(n)
	p.leaving = append(p.leaving, n)
	n.leftFrom = p
	t.leaving = append(t.leaving, n)
	t.retireNode(n, onRetire)
	n.moving = &motion{fade: newLeaveFade(*lb.anim.Delete, lb.now, from)}
	t.ensureMaps()
	t.motions[n] = true
	t.touchedNode(n)
	return true
}

// destroyNode removes n, any leaving ghosts attached under n, and any retired
// children inside n, freeing their host IDs and emitting opDelete for each.
func (t *watchTree) destroyNode(n *watchNode, onRetire, onDestroy func(*watchNode)) {
	for _, g := range append([]*watchNode(nil), n.leaving...) {
		g.leftFrom = nil
		t.destroyNode(g, onRetire, onDestroy)
	}
	n.leaving = nil

	for _, c := range append([]*watchNode(nil), n.children...) {
		c.parent = nil
		if c.retired {
			t.destroyNode(c, onRetire, onDestroy)
		}
	}
	n.children = nil

	if p := n.leftFrom; p != nil {
		if i := slices.Index(p.leaving, n); i >= 0 {
			p.leaving = slices.Delete(p.leaving, i, i+1)
		}
		n.leftFrom = nil
	}
	if i := slices.Index(t.leaving, n); i >= 0 {
		t.leaving = slices.Delete(t.leaving, i, i+1)
	}
	t.detachNode(n)

	wasRetired := n.retired
	if !wasRetired && onRetire != nil {
		onRetire(n)
	}
	if onDestroy != nil {
		onDestroy(n)
	}
	n.retired = false
	n.deleted = true
	n.moving = nil
	n.trans = nil
	n.animated = nil
	n.hasKF = false

	delete(t.dirty, n)
	delete(t.redraw, n)
	delete(t.motions, n)
	if int(n.id) < len(t.views) && t.views[n.id] == n {
		t.views[n.id] = nil
	}
	if t.byHost[n.hostID] == n {
		delete(t.byHost, n.hostID)
	}
	t.ids.free(n.hostID)
	t.w.op(opDelete, n.hostID)
}

// closeLeaving reaps every active leaving ghost and its retired descendants,
// emitting opDelete for each host node on surface teardown.
func (t *watchTree) closeLeaving(onDestroy func(*watchNode)) {
	for _, g := range append([]*watchNode(nil), t.leaving...) {
		t.destroyNode(g, nil, onDestroy)
	}
	t.leaving = nil
}

// stepNodes advances all running style transitions, keyframes, frame motions,
// entrances and leaving-ghost exits to now.
func (t *watchTree) stepNodes(now time.Time, onDestroy func(*watchNode)) {
	for n := range t.motions {
		if n.deleted && !n.retired {
			delete(t.motions, n)
			continue
		}
		keep := false
		if tr := n.trans; tr != nil {
			if now.Before(tr.start) {
				keep = true
			} else if e := now.Sub(tr.start); e >= tr.duration {
				n.trans = nil
			} else {
				tr.shown = driver.MixStyles(tr.from, tr.to, tr.curve.At(float64(e)/float64(tr.duration)), tr.props)
				keep = true
			}
			t.touchedNode(n)
		}
		if n.hasKF {
			if _, running := n.style.Animation.At(now.Sub(n.kfStart)); running {
				keep = true
			}
			t.touchedNode(n)
		}
		if m := n.moving; m != nil {
			if m.move != nil {
				if _, done := m.move.Step(now.Sub(t.epoch)); done {
					m.move = nil
				}
			}
			if m.fade != nil {
				done := m.fade.step(now)
				if done {
					if m.fade.out {
						t.destroyNode(n, nil, onDestroy)
						continue
					}
					m.fade = nil
				}
			}
			t.dropMotionNode(n)
			keep = keep || n.moving != nil
			t.touchedNode(n)
		}
		if !keep {
			delete(t.motions, n)
		}
	}
}

// computeLook builds the SwiftUI modifier state for a node's frame, style,
// keyframe and layout fade multipliers.
func computeLook(kind driver.ViewKind, f driver.Bounds, st driver.ViewStyle, k driver.Keyframe, opacityPart, scalePart float64) look {
	l := look{
		opacity:   float64(st.Opacity) * float64(k.Opacity) * opacityPart,
		hidden:    st.Hidden,
		transform: [6]float64{1, 0, 0, 1, 0, 0},
	}
	l.clips = st.Clip || kind == driver.KindImage || kind == driver.KindInput || kind == driver.KindScroll
	if l.clips {
		l.radii = st.Radii(f.Size)
		if st.Clip || kind == driver.KindScroll {
			// Inside the border, as CSS clips overflow to the padding box; SwiftUI's corners are circular, so each takes the smaller of its elliptical radii.
			_, _, _, _, inner := st.ClipBox(f.Size)
			l.inset = st.BorderWidths()
			r := func(c [2]float64) driver.Points { return driver.Points(min(c[0], c[1])) }
			l.radii = driver.Corners{TopLeft: r(inner[0]), TopRight: r(inner[1]), BottomRight: r(inner[2]), BottomLeft: r(inner[3])}
		}
	}
	m := k.Matrix(f.Size).Mul(st.Matrix(f.Size))
	if scalePart != 1 {
		center := driver.Point{X: f.Size.Width / 2, Y: f.Size.Height / 2}
		m = driver.Transform{Scale: float32(scalePart)}.Matrix(center).Mul(m)
	}
	if a, b, c, d, e, ff, ok := m.Affine(); ok {
		l.transform = [6]float64{a, b, c, d, e, ff}
	}
	return l
}

// writeNodeLook writes opFrame and opLook for n when they changed since the
// host last heard.
func (t *watchTree) writeNodeLook(n *watchNode, now time.Time) (driver.Bounds, driver.ViewStyle) {
	f := n.shownFrame()
	st := n.shownStyle()
	if !n.hasFrame || f != n.sentFrame {
		n.sentFrame = f
		n.hasFrame = true
		t.w.op(opFrame, n.hostID)
		t.w.f64(float64(f.Origin.X), float64(f.Origin.Y), float64(f.Size.Width), float64(f.Size.Height))
	}
	k, _ := n.keyframeAt(now)
	part, scale := n.layoutPart()
	l := computeLook(n.kind, f, st, k, part, scale)
	if l != n.sentLook {
		n.sentLook = l
		t.w.op(opLook, n.hostID)
		t.w.f64(l.opacity)
		t.w.flag(l.hidden)
		t.w.flag(l.clips)
		r := l.radii
		// SwiftUI's order: top leading, bottom leading, bottom trailing,
		// top trailing.
		t.w.f64(float64(r.TopLeft), float64(r.BottomLeft), float64(r.BottomRight), float64(r.TopRight))
		in := l.inset
		t.w.f64(float64(in.Top), float64(in.Left), float64(in.Bottom), float64(in.Right))
		t.w.f64(l.transform[:]...)
	}
	return f, st
}

// flushNodeLooks writes frame and look ops for all dirty live or retired nodes.
func (t *watchTree) flushNodeLooks(now time.Time) {
	for n := range t.dirty {
		if !n.deleted || n.retired {
			t.writeNodeLook(n, now)
		}
		delete(t.dirty, n)
	}
}

// hitNode returns the innermost live view under p that match takes, p in the
// coordinates of n's parent's content, sticky children before non-sticky
// siblings and front to back. Leaving ghosts are detached from children and
// marked deleted, so hitNode ignores them.
func (t *watchTree) hitNode(n *watchNode, p driver.Point, now time.Time, match func(*watchNode) bool) *watchNode {
	if n == nil || n.style.Hidden || n.deleted {
		return nil
	}
	f := t.placedFrame(n)
	local, mapped := driver.IntoView(f, n.pointerMatrixAt(now), p)
	if !mapped {
		return nil
	}
	area, transformed := n.overflow, n.transformedKids
	if n.shownStyle().Clip || n.kind == driver.KindScroll {
		area, transformed = driver.Bounds{}, false
	}
	self, children := driver.ReachView(f.Size, &n.props, area, transformed, local)
	if children {
		local.X += n.offset.X
		local.Y += n.offset.Y
		var found *watchNode
		hitChildOrder(n.children, func(c *watchNode) bool { return t.sticky.isSticky(c.id) }, func(c *watchNode) bool {
			if h := t.hitNode(c, local, now, match); h != nil {
				found = h
				return true
			}
			return false
		})
		if found != nil {
			return found
		}
	}
	if self && match(n) {
		return n
	}
	return nil
}
