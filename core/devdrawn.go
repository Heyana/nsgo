package core

import (
	"math"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// transformProps is the AnimatedValues.Set mask for properties that move, scale,
// or rotate a view's drawn bounds.
const transformProps = uint16(1<<driver.PropTranslateX | 1<<driver.PropTranslateY |
	1<<driver.PropScale | 1<<driver.PropScaleX | 1<<driver.PropScaleY |
	1<<driver.PropRotate | 1<<driver.PropRotateX | 1<<driver.PropRotateY)

// allStyleTransitionProps matches the default set of properties animated when
// Transition.Properties is 0 (everything except TransitionLayout).
const allStyleTransitionProps = driver.TransitionOpacity | driver.TransitionBackground |
	driver.TransitionTransform | driver.TransitionBorder | driver.TransitionShadow

// devDrawn tracks where views actually draw on screen while DevTools or their
// overlay read bounds: the native animation graph, style transform transitions,
// layout animations (frame moves, matched geometry, scale-ins), keyframes, and
// sticky offsets. It is pure Go driven by commits, events, and the tree's clock,
// so DevTools' bounds, Pick, highlights, and the layout audit match what every
// platform draws and work identically in headless tests.
type devDrawn struct {
	anim  *driver.Animator
	epoch time.Time
	now   time.Time
	views map[NodeID]*drawnView

	// Per-commit sync state so multi-round commits (e.g. audit redraws) apply
	// each emitted MountItem once.
	synced         int
	insertedSynced int
	batchCreated   map[NodeID]bool
	batchInserted  []NodeID
	batchMatched   map[NodeID]bool
}

type drawnView struct {
	kind   driver.ViewKind
	parent NodeID
	laid   Bounds
	style  driver.ViewStyle
	styled bool

	moving        *drawnMove
	fading        *drawnFade
	transition    *drawnTransition
	keyframeStart time.Time
}

type drawnMove struct {
	from, to Bounds
	start    time.Time
	tr       driver.LayoutTransition
	shown    Bounds
}

type drawnFade struct {
	start time.Time
	tr    driver.LayoutTransition
	shown float64
}

type drawnTransition struct {
	from, to driver.ViewStyle
	start    time.Time // after delay
	duration time.Duration
	curve    driver.Curve
	props    driver.TransitionProperty
	shown    driver.ViewStyle
}

func (v *drawnView) shownLaid() Bounds {
	if v.moving != nil {
		return v.moving.shown
	}
	return v.laid
}

func (v *drawnView) baseStyle() driver.ViewStyle {
	if v.transition != nil {
		return v.transition.shown
	}
	return v.style
}

// ensureDevDrawn lazily creates t.devDrawn and seeds any views already committed.
func (t *Tree) ensureDevDrawn() *devDrawn {
	if t.devSource != nil {
		return nil
	}
	if t.devDrawn != nil {
		return t.devDrawn
	}
	now := t.rt.clock.Now()
	d := &devDrawn{
		anim:         driver.NewAnimator(func(driver.Event) {}),
		epoch:        now,
		now:          now,
		views:        map[NodeID]*drawnView{},
		batchCreated: map[NodeID]bool{},
		batchMatched: map[NodeID]bool{},
	}
	for _, n := range t.nodes {
		if n == nil || n.deleted || !n.created {
			continue
		}
		var parentID NodeID
		if p := n.nativeParent(); p != nil {
			parentID = p.id
		}
		v := &drawnView{
			kind:   n.kind,
			parent: parentID,
			laid:   n.sent.frame,
			style:  n.sent.viewStyle,
			styled: true,
		}
		if n.sent.viewStyle.Animation.Keyframes != 0 {
			v.keyframeStart = now
		}
		d.views[n.id] = v
	}
	t.devDrawn = d
	return d
}

func (d *devDrawn) beginCommit() {
	d.synced = 0
	d.insertedSynced = 0
	clear(d.batchCreated)
	d.batchInserted = d.batchInserted[:0]
	clear(d.batchMatched)
}

func (d *devDrawn) elapsed(now time.Time) time.Duration {
	if now.Before(d.epoch) {
		return 0
	}
	return now.Sub(d.epoch)
}

func (d *devDrawn) view(id NodeID) *drawnView {
	v := d.views[id]
	if v == nil {
		v = &drawnView{style: driver.DefaultViewStyle}
		d.views[id] = v
	}
	return v
}

func (t *Tree) isDevNode(id NodeID) bool {
	if t.highlights == nil {
		return false
	}
	n := t.node(id)
	return n != nil && t.highlights.owns(n)
}

func keyframeMovesBounds(k driver.Keyframes) bool {
	return k == driver.KeyframesSpin || k == driver.KeyframesPing || k == driver.KeyframesBounce
}

func geomStyleChanged(a, b driver.ViewStyle) bool {
	return a.Transform != b.Transform || a.Origin != b.Origin || a.OriginZ != b.OriginZ ||
		a.ChildPerspective != b.ChildPerspective || a.Hidden != b.Hidden
}

func hasGeomAnimOp(ops []driver.AnimationOp) bool {
	for i := range ops {
		switch ops[i].Kind {
		case driver.AnimBind:
			if (1<<ops[i].Prop)&transformProps != 0 {
				return true
			}
		case driver.AnimBindPan, driver.AnimBindScroll, driver.AnimBindPinch,
			driver.AnimBindRotation, driver.AnimBindPull,
			driver.AnimStart, driver.AnimStop, driver.AnimReset:
			return true
		}
	}
	return false
}

// syncDevDrawn applies newly emitted commit items to t.devDrawn at now,
// updating t.framesMoved when drawn geometry changed and scheduling audit ticks
// when animations or transitions become active.
func (t *Tree) syncDevDrawn(now time.Time) {
	d := t.devDrawn
	if d == nil {
		return
	}
	if d.synced >= len(t.items) {
		return
	}
	// Advance running transitions/moves to now so interrupted animations start
	// smoothly from where views currently draw.
	d.step(t, now)

	reduce := t.root.Env().reduce.GetUntracked()
	la := t.layoutAnim
	if reduce {
		la = nil
	}
	moved := false

	for i := d.synced; i < len(t.items); i++ {
		it := &t.items[i]
		switch it.Kind {
		case driver.ItemCreate:
			d.views[it.ID] = &drawnView{kind: it.View, style: driver.DefaultViewStyle}
			d.batchCreated[it.ID] = true

		case driver.ItemInsert:
			if v := d.views[it.ID]; v != nil {
				v.parent = it.Parent
			}
			if d.batchCreated[it.ID] {
				d.batchInserted = append(d.batchInserted, it.ID)
			}

		case driver.ItemRemove:
			if v := d.views[it.ID]; v != nil {
				v.parent = 0
			}

		case driver.ItemDelete:
			delete(d.views, it.ID)

		case driver.ItemFrame:
			v := d.view(it.ID)
			old := v.shownLaid()
			v.laid = it.Frame
			tr := it.Transition
			if la != nil && la.Update != nil && !d.batchCreated[it.ID] && v.kind != driver.KindModal {
				tr = la.Update
			}
			if reduce || tr == nil || old == (Bounds{}) || old == it.Frame {
				v.moving = nil
			} else {
				v.moving = &drawnMove{from: old, to: it.Frame, start: now, tr: *tr, shown: old}
				if !t.isDevNode(it.ID) {
					moved = true
				}
			}

		case driver.ItemMatch:
			if la != nil && la.Update != nil {
				v, old := d.views[it.ID], d.views[it.Before]
				if v != nil && old != nil && v.parent != 0 {
					was := t.drawnViewSurfaceBounds(old)
					in := t.drawnContentSurfaceOrigin(v.parent)
					from := Bounds{
						Origin: Point{X: was.Origin.X - in.X, Y: was.Origin.Y - in.Y},
						Size:   was.Size,
					}
					d.batchMatched[it.ID] = true
					v.fading = nil
					v.moving = &drawnMove{from: from, to: v.laid, start: now, tr: *la.Update, shown: from}
					if !t.isDevNode(it.ID) {
						moved = true
					}
				}
			}

		case driver.ItemViewStyle:
			if it.ViewStyle == nil {
				continue
			}
			v := d.view(it.ID)
			s := *it.ViewStyle
			from := v.baseStyle()
			styled := v.styled || !d.batchCreated[it.ID]
			if s.Animation != v.style.Animation || !styled {
				v.keyframeStart = now
				if keyframeMovesBounds(s.Animation.Keyframes) && !reduce && !t.isDevNode(it.ID) {
					moved = true
				}
			}
			if geomStyleChanged(v.style, s) && !t.isDevNode(it.ID) {
				moved = true
			}
			v.style, v.styled, v.transition = s, true, nil
			tr := s.Transition
			if styled && tr.Duration > 0 && !reduce {
				props := tr.Properties
				if props == 0 {
					props = allStyleTransitionProps
				}
				if props&driver.TransitionTransform != 0 {
					first := driver.MixStyles(from, s, 0, driver.TransitionTransform)
					last := driver.MixStyles(from, s, 1, driver.TransitionTransform)
					if first.Transform != last.Transform || first.Origin != last.Origin {
						v.transition = &drawnTransition{
							from:     from,
							to:       s,
							start:    now.Add(tr.Delay),
							duration: tr.Duration,
							curve:    tr.Easing,
							props:    driver.TransitionTransform,
							shown:    first,
						}
						if !t.isDevNode(it.ID) {
							moved = true
						}
					}
				}
			}

		case driver.ItemAnimation:
			beforeGen := d.anim.Generation()
			d.anim.Apply(it.Animation)
			if d.anim.Running() {
				// Anchor newly started runs at now so later steps measure exact elapsed time.
				d.anim.Step(d.elapsed(now))
			}
			if hasGeomAnimOp(it.Animation) || (d.anim.Generation() != beforeGen && d.animAffectsBounds(t)) {
				moved = true
			}

		case driver.ItemStickyCells:
			moved = true
		}
	}

	if la != nil && la.Create != nil && la.Create.Property == driver.LayoutScale {
		for i := d.insertedSynced; i < len(d.batchInserted); i++ {
			id := d.batchInserted[i]
			v := d.views[id]
			if v == nil || v.parent == 0 || d.batchCreated[v.parent] || d.batchMatched[id] || v.kind == driver.KindModal {
				continue
			}
			v.fading = &drawnFade{start: now, tr: *la.Create, shown: 0}
			if !t.isDevNode(id) {
				moved = true
			}
		}
	}
	d.synced = len(t.items)
	d.insertedSynced = len(d.batchInserted)

	if moved {
		t.framesMoved = true
	}
	if t.tickAudit != nil && d.active(t) {
		t.tickAudit()
	}
}

// drawnViewSurfaceBounds returns v's current laid/moving bounds in surface
// coordinates, for ItemMatch transitions.
func (t *Tree) drawnViewSurfaceBounds(v *drawnView) Bounds {
	b := v.shownLaid()
	d := t.devDrawn
	for pid := v.parent; pid != 0; {
		pv := d.views[pid]
		if pv == nil {
			break
		}
		o := pv.shownLaid().Origin
		b.Origin.X += o.X
		b.Origin.Y += o.Y
		if pn := t.node(pid); pn != nil && pn.scroll != nil {
			b.Origin.X -= pn.scroll.offset.X
			b.Origin.Y -= pn.scroll.offset.Y
		}
		pid = pv.parent
	}
	return b
}

// drawnContentSurfaceOrigin returns parentID's content origin in surface
// coordinates, for ItemMatch transitions.
func (t *Tree) drawnContentSurfaceOrigin(parentID NodeID) Point {
	var at Point
	d := t.devDrawn
	for pid := parentID; pid != 0; {
		pv := d.views[pid]
		if pv == nil {
			break
		}
		o := pv.shownLaid().Origin
		at.X += o.X
		at.Y += o.Y
		if pn := t.node(pid); pn != nil && pn.scroll != nil {
			at.X -= pn.scroll.offset.X
			at.Y -= pn.scroll.offset.Y
		}
		pid = pv.parent
	}
	return at
}

// step advances all active animations, transitions, layout animations, and
// keyframes to now, returning whether any drawn geometry changed.
func (d *devDrawn) step(t *Tree, now time.Time) bool {
	if now.Before(d.epoch) {
		now = d.epoch
	}
	changed := false
	if d.anim.Running() {
		before := d.anim.Generation()
		d.anim.Step(d.elapsed(now))
		if d.anim.Generation() != before && d.animAffectsBounds(t) {
			changed = true
		}
	}
	reduce := t.root.Env().reduce.GetUntracked()
	for _, v := range d.views {
		if m := v.moving; m != nil {
			p, done := m.tr.At(now.Sub(m.start))
			next := lerpBounds(m.from, m.to, p)
			if next != m.shown {
				m.shown = next
				changed = true
			}
			if done {
				v.moving = nil
				if next != m.to {
					changed = true
				}
			}
		}
		if f := v.fading; f != nil {
			p, done := f.tr.At(now.Sub(f.start))
			if p != f.shown {
				f.shown = p
				changed = true
			}
			if done {
				v.fading = nil
				if f.shown != 1 {
					changed = true
				}
			}
		}
		if tr := v.transition; tr != nil {
			if !now.Before(tr.start) {
				x := 1.0
				if tr.duration > 0 {
					x = float64(now.Sub(tr.start)) / float64(tr.duration)
				}
				if x >= 1 {
					v.transition = nil
					if tr.shown.Transform != tr.to.Transform || tr.shown.Origin != tr.to.Origin {
						changed = true
					}
				} else {
					p := tr.curve.At(x)
					next := driver.MixStyles(tr.from, tr.to, p, tr.props)
					if next.Transform != tr.shown.Transform || next.Origin != tr.shown.Origin {
						tr.shown = next
						changed = true
					}
				}
			}
		}
		if !reduce && keyframeMovesBounds(v.style.Animation.Keyframes) && now != d.now {
			if _, running := v.style.Animation.At(now.Sub(v.keyframeStart)); running {
				changed = true
			}
		}
	}
	d.now = now
	return changed
}

// active reports whether any animation, transition, layout animation, or
// keyframe is currently moving drawn bounds.
func (d *devDrawn) active(t *Tree) bool {
	if d.anim.Running() && d.animAffectsBounds(t) {
		return true
	}
	reduce := t.root.Env().reduce.GetUntracked()
	for _, v := range d.views {
		if v.moving != nil || v.fading != nil || v.transition != nil {
			return true
		}
		if !reduce && keyframeMovesBounds(v.style.Animation.Keyframes) {
			if _, running := v.style.Animation.At(d.now.Sub(v.keyframeStart)); running {
				return true
			}
		}
	}
	return false
}

// animAffectsBounds reports whether any live view binds a transform property
// or sticky inset to the native animation graph.
func (d *devDrawn) animAffectsBounds(t *Tree) bool {
	for id := range d.views {
		if vals, ok := d.anim.Values(id); ok && vals.Set&transformProps != 0 {
			return true
		}
	}
	for _, n := range t.nodes {
		if n == nil || n.deleted || n.scroll == nil {
			continue
		}
		for _, c := range n.scroll.sticky {
			if c.Node != 0 {
				return true
			}
		}
	}
	return false
}

// stepDevDrawn advances t.devDrawn to the tree clock's current time.
func (t *Tree) stepDevDrawn() {
	if t.devDrawn != nil {
		t.devDrawn.step(t, t.rt.clock.Now())
	}
}

// devDrawnGesture forwards a pan, pinch, or rotation event to t.devDrawn's
// shadow Animator without mutating ev.
func (t *Tree) devDrawnGesture(ev driver.Event) {
	if t.devDrawn == nil {
		return
	}
	copyEv := ev
	if t.devDrawn.anim.Gesture(&copyEv) && t.auditRead != nil && t.auditRead() {
		t.framesMoved, t.changed = true, true
	}
}

// devDrawnScroll forwards a scroll offset (and pull distance if bound) to
// t.devDrawn's shadow Animator.
func (t *Tree) devDrawnScroll(n *node, offset Point) {
	d := t.devDrawn
	if d == nil {
		return
	}
	d.anim.Scroll(n.id, offset)
	if n.scroll != nil && d.anim.Pulls(n.id) {
		var pull Points
		if !n.scroll.config.Horizontal {
			pull = max(-n.scroll.config.Behavior.ContentInset.Top-offset.Y, 0)
		}
		d.anim.Pull(n.id, pull)
	}
}

// devDrawnScrollEnd starts settling snapping diff clamps bound to scroll view n.
func (t *Tree) devDrawnScrollEnd(n *node) {
	d := t.devDrawn
	if d == nil {
		return
	}
	d.anim.Settle(n.id)
	if d.anim.Running() {
		d.anim.Step(d.elapsed(t.rt.clock.Now()))
		if t.tickAudit != nil && d.active(t) {
			t.tickAudit()
		}
	}
}

func lerpBounds(a, b Bounds, p float64) Bounds {
	f := Points(p)
	return Bounds{
		Origin: Point{
			X: a.Origin.X + (b.Origin.X-a.Origin.X)*f,
			Y: a.Origin.Y + (b.Origin.Y-a.Origin.Y)*f,
		},
		Size: Size{
			Width:  max(a.Size.Width+(b.Size.Width-a.Size.Width)*f, 0),
			Height: max(a.Size.Height+(b.Size.Height-a.Size.Height)*f, 0),
		},
	}
}

// stickyShift returns the main-axis sticky translation applied to n by its
// enclosing scroll view, or zero when n is not sticky.
func (t *Tree) stickyShift(n *node) Point {
	if n == nil || n.hoist != nil {
		return Point{}
	}
	for p := n.nativeParent(); p != nil; p = p.nativeParent() {
		if p.scroll != nil {
			if len(p.scroll.sticky) == 0 {
				return Point{}
			}
			var anim *driver.Animator
			if t.devDrawn != nil {
				anim = t.devDrawn.anim
			}
			for _, c := range p.scroll.sticky {
				if c.Cell == n.id {
					if p.scroll.config.Horizontal {
						return Point{X: anim.StickyShift(c, p.scroll.offset.X)}
					}
					return Point{Y: anim.StickyShift(c, p.scroll.offset.Y)}
				}
			}
			return Point{}
		}
		if p.hoist != nil {
			return Point{}
		}
	}
	return Point{}
}

// drawnFrame returns n's border-box frame in its native parent's content
// coordinates, following active layout animations and sticky offsets.
func (t *Tree) drawnFrame(n *node) Bounds {
	f := n.frame
	if d := t.devDrawn; d != nil {
		if v := d.views[n.id]; v != nil && v.moving != nil {
			f = v.moving.shown
		}
	}
	if shift := t.stickyShift(n); shift != (Point{}) {
		f.Origin.X += shift.X
		f.Origin.Y += shift.Y
	}
	return f
}

// drawnStyle returns n's ViewStyle as currently drawn, incorporating active
// style transitions and bound animated values.
func (t *Tree) drawnStyle(n *node) driver.ViewStyle {
	vs := n.sent.viewStyle
	d := t.devDrawn
	if d == nil {
		return vs
	}
	if v := d.views[n.id]; v != nil && v.transition != nil {
		vs = v.transition.shown
	}
	if vals, ok := d.anim.Values(n.id); ok && vals.Set != 0 {
		vs = vals.Apply(vs)
	}
	return vs
}

// drawnSelfMatrix returns n's own transform around its top-left corner,
// combining static/transitioned/animated transforms, keyframes, and layout
// scale animations.
func (t *Tree) drawnSelfMatrix(n *node, size Size) driver.Matrix3D {
	vs := t.drawnStyle(n)
	m := vs.Matrix(size)
	if d := t.devDrawn; d != nil {
		if v := d.views[n.id]; v != nil {
			if !t.root.Env().reduce.GetUntracked() && v.style.Animation.Keyframes != 0 {
				if k, running := v.style.Animation.At(d.now.Sub(v.keyframeStart)); running {
					m = k.Matrix(size).Mul(m)
				}
			}
			if f := v.fading; f != nil && f.tr.Property == driver.LayoutScale {
				s := max(f.shown, 0)
				cx, cy := float64(size.Width)/2, float64(size.Height)/2
				m = m.Mul(driver.Translate3D(cx, cy, 0)).
					Mul(driver.Matrix3D{s, 0, 0, 0, 0, s, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}).
					Mul(driver.Translate3D(-cx, -cy, 0))
			}
		}
	}
	return m
}

// drawnPointerMatrix returns n's transform around its top-left corner in its
// parent's content coordinates, including its parent's ChildPerspective.
func (t *Tree) drawnPointerMatrix(n *node, frame Bounds) driver.Matrix3D {
	m := t.drawnSelfMatrix(n, frame.Size)
	if parent := n.nativeParent(); parent != nil {
		if depth := t.drawnStyle(parent).ChildPerspective; depth > 0 {
			at := frame.Origin
			if parent.scroll != nil {
				at.X -= parent.scroll.offset.X
				at.Y -= parent.scroll.offset.Y
			}
			m = driver.ChildPerspective(depth, t.drawnFrame(parent).Size, at).Mul(m)
		}
	}
	return m
}

func scrollContentMatrix(p *node) driver.Matrix3D {
	if p == nil || p.scroll == nil {
		return driver.Identity3D
	}
	m := driver.Translate3D(-float64(p.scroll.offset.X), -float64(p.scroll.offset.Y), 0)
	if z := float64(p.scroll.zoom); z > 0 && z != 1 {
		m = m.Mul(driver.Matrix3D{z, 0, 0, 0, 0, z, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	}
	return m
}

// drawnMatrixWith computes n's local-to-surface matrix and drawn size given its
// native parent's content-to-surface matrix.
func (t *Tree) drawnMatrixWith(n, parent *node, parentContentM driver.Matrix3D) (driver.Matrix3D, Size) {
	f := t.drawnFrame(n)
	m := t.drawnPointerMatrix(n, f)
	local := driver.Translate3D(float64(f.Origin.X), float64(f.Origin.Y), 0).Mul(m)
	if parent == nil {
		return local, f.Size
	}
	return parentContentM.Mul(local), f.Size
}

// drawnMatrix returns the 4x4 matrix mapping n's local border-box coordinates
// to surface points, and n's drawn border-box size.
func (t *Tree) drawnMatrix(n *node) (driver.Matrix3D, Size) {
	parent := n.nativeParent()
	if parent == nil {
		return t.drawnMatrixWith(n, nil, driver.Identity3D)
	}
	pm, _ := t.drawnMatrix(parent)
	return t.drawnMatrixWith(n, parent, pm.Mul(scrollContentMatrix(parent)))
}

// drawnBounds returns n's axis-aligned bounding box in surface points, where n
// currently draws.
func (t *Tree) drawnBounds(n *node) Bounds {
	m, size := t.drawnMatrix(n)
	return transformBounds(m, Bounds{Size: size})
}

// transformBounds returns the axis-aligned bounding box of r after m maps its
// four corners. Pure translations preserve r.Size without floating-point drift.
func transformBounds(m driver.Matrix3D, r Bounds) Bounds {
	if m == driver.Identity3D {
		return r
	}
	if a, b, c, d, e, f, ok := m.Affine(); ok && a == 1 && b == 0 && c == 0 && d == 1 {
		return Bounds{
			Origin: Point{X: r.Origin.X + Points(e), Y: r.Origin.Y + Points(f)},
			Size:   r.Size,
		}
	}
	x0, y0 := float64(r.Origin.X), float64(r.Origin.Y)
	x1, y1 := x0+float64(r.Size.Width), y0+float64(r.Size.Height)
	corners := [4][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
	var minX, minY, maxX, maxY float64
	for i, pt := range corners {
		px, py := m.Apply(pt[0], pt[1])
		if math.IsNaN(px) || math.IsNaN(py) || math.IsInf(px, 0) || math.IsInf(py, 0) {
			return Bounds{}
		}
		if i == 0 {
			minX, maxX, minY, maxY = px, px, py, py
		} else {
			minX, maxX = min(minX, px), max(maxX, px)
			minY, maxY = min(minY, py), max(maxY, py)
		}
	}
	return Bounds{
		Origin: Point{X: Points(minX), Y: Points(minY)},
		Size:   Size{Width: Points(maxX - minX), Height: Points(maxY - minY)},
	}
}

// hitDrawnWith returns the innermost view at p (in n's parent's content
// coordinates) that ok accepts, using each view's current drawn frame, sticky
// offset, and transforms.
func (t *Tree) hitDrawnWith(n *node, p Point, ok func(*node) bool) (NodeID, Point) {
	frame := t.drawnFrame(n)
	local, mapped := driver.IntoView(frame, t.drawnPointerMatrix(n, frame), p)
	if !mapped {
		return 0, Point{}
	}
	clipX, clipY := n.clips()
	self, inside := driver.ReachView(frame.Size, &n.props, n.sent.overflow, true, local)
	if inside {
		if clipX && (local.X < 0 || local.X >= frame.Size.Width) {
			inside = false
		}
		if clipY && (local.Y < 0 || local.Y >= frame.Size.Height) {
			inside = false
		}
	}
	if n.style.Display == DisplayNone || n.style.Visibility == VisibilityHidden || n.hidden || !self && !inside ||
		(n.attach != nil && n.attach.cut) {
		return 0, Point{}
	}
	at := local
	if n.scroll != nil {
		local.X += n.scroll.offset.X
		local.Y += n.scroll.offset.Y
		if z := n.scroll.zoom; z > 0 && z != 1 {
			local.X /= Points(z)
			local.Y /= Points(z)
		}
	}
	if inside {
		kids := n.nativeChildren()
		for i := len(kids) - 1; i >= 0; i-- {
			if id, in := t.hitDrawnWith(kids[i], local, ok); id != 0 {
				return id, in
			}
		}
	}
	if self && ok(n) {
		return n.id, at
	}
	return 0, Point{}
}
