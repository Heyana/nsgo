package miniprogram

import (
	"math"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The render layer reports every touch on a view that follows them (op
// e's touchFollow): touchstart, touchmove, touchend and touchcancel, with
// all the fingers down and the ones that changed, in page points. The
// innermost such view reports first; a finger belongs to the view it
// went down on. A drag pans the nearest view that pans once its
// PanActivation activates it, two fingers pinch and rotate it, and views
// that ask for EventTouch get the fingers as they are.

// finger is a touch the render layer reports.
type finger struct {
	id   int
	page driver.Point
}

// velocityWindow is how far back a finger's velocity is measured.
const velocityWindow = 100 * time.Millisecond

type sample struct {
	t    time.Duration
	x, y float64
}

// pointer is a finger down, and what it pans.
type pointer struct {
	id               int
	start, at        driver.Point
	samples          []sample
	view             *view // the view it went down on
	pan              *view // the view it may pan
	panState         int   // as PanActivation.Decide: 1 active, -1 failed
	pinch            *view // what two fingers with it would pinch
	touch            *view // the view that follows its EventTouch path
	reported, ending bool
}

type twoFinger struct {
	view        *view
	a, b        *pointer
	dist, angle float64
	lastAngle   float64
	rotation    float64
	scale       float64
}

type touches struct {
	geometry driver.TouchTracker
	touching bool
	pointers []*pointer
	two      *twoFinger
	// event is the touch event being handled: the views it bubbles
	// through report it again.
	event int
}

// forget drops what follows a deleted view.
func (t *touches) forget(v *view) {
	for _, p := range t.pointers {
		for _, w := range []**view{&p.view, &p.pan, &p.pinch, &p.touch} {
			if *w == v {
				*w = nil
			}
		}
	}
	if t.two != nil && t.two.view == v {
		t.two = nil
	}
}

func (t *touches) pointer(id int) *pointer {
	for _, p := range t.pointers {
		if p.id == id {
			return p
		}
	}
	return nil
}

// nearest returns the nearest view holding v, v included, that reports
// one of kinds.
func (m *mounter) nearest(v *view, kinds ...driver.EventKind) *view {
	for ; v != nil; v = v.parent {
		if v.props.PointerEvents == driver.PointerEventsNone {
			continue
		}
		for _, k := range kinds {
			if v.events.Has(k) || k == driver.EventPan && m.animator.PanBound(v.id) {
				return v
			}
		}
		if v.kind == driver.KindModal {
			return nil
		}
	}
	return nil
}

// touch reports a touch event on view id: phase, the changed fingers,
// every finger still down (none as the last lifts) and the event's time.
// seq numbers the render layer's events, so the views it bubbles through
// add nothing.
func (m *mounter) touch(id driver.NodeID, seq int, phase driver.GesturePhase, changed, all []finger, now time.Duration) {
	t := &m.touches
	if seq == t.event {
		return
	}
	t.event = seq
	if phase == driver.GestureBegan && len(changed) > 0 && m.selecting != nil && !m.selHandleActive && !m.selDragging {
		if !m.selectionContains(changed[0].page) {
			m.clearSelection()
			m.flush()
		}
	}
	if m.selDragging {
		switch phase {
		case driver.GestureChanged:
			if len(changed) > 0 {
				m.selectDrag(changed[0].page)
			}
			return
		case driver.GestureEnded, driver.GestureCancelled:
			m.selDragging = false
		}
	}
	v := m.view(id)
	for _, f := range changed {
		switch phase {
		case driver.GestureBegan:
			if t.pointer(f.id) != nil {
				continue
			}
			target := m.hitTarget(v, f.page, len(changed) > 1)
			if target == nil {
				continue
			}
			p := &pointer{id: f.id, start: f.page, at: f.page, view: target,
				samples: []sample{{now, float64(f.page.X), float64(f.page.Y)}}}
			if len(t.pointers) == 0 {
				p.pan = m.nearest(target, driver.EventPan)
			}
			p.pinch = m.nearest(target, driver.EventPinch, driver.EventRotation)
			p.touch = m.nearest(target, driver.EventTouch)
			if p.touch != nil {
				t.touching = true
			}
			t.pointers = append(t.pointers, p)
			if len(t.pointers) == 2 && t.two == nil {
				m.startTwoFinger()
			}
		case driver.GestureChanged:
			if p := t.pointer(f.id); p != nil {
				p.at = f.page
				p.samples = append(p.samples, sample{now, float64(f.page.X), float64(f.page.Y)})
				if len(p.samples) > 16 {
					p.samples = p.samples[1:]
				}
			}
		default:
			if p := t.pointer(f.id); p != nil {
				p.at, p.ending = f.page, true
				p.samples = append(p.samples, sample{now, float64(f.page.X), float64(f.page.Y)})
			}
		}
	}
	m.reportTouches(phase, changed, now)
	switch phase {
	case driver.GestureChanged:
		if tw := t.two; tw != nil {
			m.moveTwoFinger(tw)
		}
		for _, p := range t.pointers {
			m.movePan(p)
		}
	case driver.GestureEnded, driver.GestureCancelled:
		var kept []*pointer
		for _, p := range t.pointers {
			if !p.ending {
				kept = append(kept, p)
				continue
			}
			if tw := t.two; tw != nil && (tw.a == p || tw.b == p) {
				t.two = nil
				m.sendTwoFinger(tw, phase)
			}
			if p.pan != nil && p.panState == 1 {
				m.send(p.pan, m.panEvent(p, phase))
			}
		}
		t.pointers = kept
		if len(t.pointers) == 0 {
			t.touching = false
		}
	}
}

// reportTouches sends EventTouch with a presentation geometry snapshot for all fingers currently down while a view listens to touches.
func (m *mounter) reportTouches(phase driver.GesturePhase, changed []finger, now time.Duration) {
	t := &m.touches
	if !t.touching || len(t.pointers) == 0 {
		return
	}
	changedID := map[int]bool{}
	for _, f := range changed {
		changedID[f.id] = true
	}
	touches := make([]driver.Touch, 0, len(t.pointers))
	anyChanged := false
	for _, q := range t.pointers {
		c := changedID[q.id]
		anyChanged = anyChanged || c
		touches = append(touches, driver.Touch{ID: q.id, Position: q.at, PagePosition: q.at, Changed: c})
	}
	if !anyChanged {
		return
	}
	ev := driver.Event{Kind: driver.EventTouch, Phase: phase, Time: now, Touches: touches}
	m.captureTouches(&ev, changedID)
	m.h.dispatch(ev)
}

func (m *mounter) captureTouches(e *driver.Event, changedID map[int]bool) {
	m.touches.geometry.Capture(e, func(p driver.Point) driver.NodeID {
		for _, q := range m.touches.pointers {
			if changedID[q.id] && q.at == p && q.view != nil {
				return q.view.id
			}
		}
		if h := m.hitAt(p); h != nil {
			return h.id
		}
		return 0
	}, func(id driver.NodeID) (driver.NodeID, driver.EventMask) {
		v := m.view(id)
		if v == nil {
			return 0, 0
		}
		var up driver.NodeID
		if v.parent != nil {
			up = v.parent.id
		}
		return up, v.events
	}, func(id driver.NodeID, p driver.Point) (driver.Point, bool) {
		v := m.view(id)
		if v == nil {
			return driver.Point{}, false
		}
		return m.surfacePoint(v, p)
	})
}

func (m *mounter) shownFrame(v *view) driver.Bounds {
	f := v.frame
	f.Origin.X += v.shift.X
	f.Origin.Y += v.shift.Y
	return f
}

func (m *mounter) shownStyle(v *view) driver.ViewStyle {
	if v.animated.Set != 0 {
		return v.animated.Apply(v.style)
	}
	return v.style
}

func (m *mounter) pointerMatrix(v *view) driver.Matrix3D {
	st := m.shownStyle(v)
	f := m.shownFrame(v)
	mat := st.Matrix(f.Size)
	if p := v.parent; p != nil && v.kind != driver.KindModal {
		at := f.Origin
		if p.kind == driver.KindScroll {
			at.X -= p.offset.X
			at.Y -= p.offset.Y
		}
		mat = driver.ChildPerspective(m.shownStyle(p).ChildPerspective, p.frame.Size, at).Mul(mat)
	}
	return mat
}

func (m *mounter) surfacePoint(v *view, p driver.Point) (driver.Point, bool) {
	if v.parent != nil && v.kind != driver.KindModal {
		var ok bool
		p, ok = m.surfacePoint(v.parent, p)
		if !ok {
			return driver.Point{}, false
		}
		if v.parent.kind == driver.KindScroll {
			p.X += v.parent.offset.X
			p.Y += v.parent.offset.Y
		}
	}
	return driver.IntoView(m.shownFrame(v), m.pointerMatrix(v), p)
}

func (m *mounter) hasTransformedDescendant(v *view) bool {
	for _, c := range v.children {
		if c == nil || c.kind == driver.KindModal {
			continue
		}
		st := m.shownStyle(c)
		if st.Hidden {
			continue
		}
		if st.Transform != (driver.Transform{}) || c.shift != (driver.Point{}) {
			return true
		}
		if !st.Clip && c.kind != driver.KindScroll && m.hasTransformedDescendant(c) {
			return true
		}
	}
	return false
}

func (m *mounter) hitView(v *view, p driver.Point) *view {
	if v == nil {
		return nil
	}
	st := m.shownStyle(v)
	if st.Hidden {
		return nil
	}
	f := m.shownFrame(v)
	local, ok := driver.IntoView(f, m.pointerMatrix(v), p)
	if !ok {
		return nil
	}
	area, transformed := v.overflow, v.transformedKids
	if st.Clip || v.kind == driver.KindScroll {
		area, transformed = driver.Bounds{}, false
	} else if !transformed {
		transformed = m.hasTransformedDescendant(v)
	}
	self, children := driver.ReachView(f.Size, &v.props, area, transformed, local)
	if children {
		childPoint := local
		if v.kind == driver.KindScroll {
			childPoint.X += v.offset.X
			childPoint.Y += v.offset.Y
		}
		for i := len(v.children) - 1; i >= 0; i-- {
			c := v.children[i]
			if c.kind == driver.KindModal {
				continue
			}
			if h := m.hitView(c, childPoint); h != nil {
				return h
			}
		}
	}
	if self {
		return v
	}
	return nil
}

func (m *mounter) hitAt(p driver.Point) *view {
	for i := len(m.modals) - 1; i >= 0; i-- {
		if h := m.hitView(m.modals[i], p); h != nil {
			return h
		}
	}
	return m.hitView(m.view(m.root), p)
}

func (m *mounter) hitIn(v *view, p driver.Point) *view {
	if v == nil {
		return nil
	}
	inParent := p
	if v.parent != nil && v.kind != driver.KindModal {
		var ok bool
		inParent, ok = m.surfacePoint(v.parent, p)
		if !ok {
			return nil
		}
		if v.parent.kind == driver.KindScroll {
			inParent.X += v.parent.offset.X
			inParent.Y += v.parent.offset.Y
		}
	}
	return m.hitView(v, inParent)
}

func (m *mounter) hitTarget(v *view, p driver.Point, multi bool) *view {
	if v != nil {
		if h := m.hitIn(v, p); h != nil {
			return h
		}
	}
	if v == nil || multi {
		if h := m.hitAt(p); h != nil {
			return h
		}
	}
	return v
}

// movePan decides a finger's pan and moves it.
func (m *mounter) movePan(p *pointer) {
	v := p.pan
	if v == nil || p.panState < 0 || m.view(v.id) != v || m.touches.two != nil {
		return
	}
	d := driver.Point{X: p.at.X - p.start.X, Y: p.at.Y - p.start.Y}
	if p.panState == 0 {
		p.panState = v.props.PanActivation.Decide(d)
		if p.panState <= 0 {
			return
		}
		m.send(v, m.panEvent(p, driver.GestureBegan))
		return
	}
	m.send(v, m.panEvent(p, driver.GestureChanged))
}

func (m *mounter) panEvent(p *pointer, phase driver.GesturePhase) driver.Event {
	vx, vy := velocity(p.samples)
	ev := m.pointer(p.pan, driver.EventPan, p.at)
	ev.Phase = phase
	ev.Point = driver.Point{X: p.at.X - p.start.X, Y: p.at.Y - p.start.Y}
	ev.Position = m.pointer(p.pan, driver.EventPan, p.at).Point
	ev.PagePosition = p.at
	ev.Velocity = driver.Point{X: driver.Points(vx), Y: driver.Points(vy)}
	return ev
}

// velocity is the movement over the newest samples' last velocityWindow,
// in points per second.
func velocity(samples []sample) (float64, float64) {
	if len(samples) == 0 {
		return 0, 0
	}
	newest := samples[len(samples)-1]
	oldest := newest
	for i := len(samples) - 2; i >= 0 && newest.t-samples[i].t <= velocityWindow; i-- {
		oldest = samples[i]
	}
	dt := (newest.t - oldest.t).Seconds()
	if dt <= 0.001 {
		return 0, 0
	}
	return (newest.x - oldest.x) / dt, (newest.y - oldest.y) / dt
}

// startTwoFinger begins a pinch and a rotation of the two fingers down,
// on the view the first would pinch, else the second's.
func (m *mounter) startTwoFinger() {
	a, b := m.touches.pointers[0], m.touches.pointers[1]
	v := a.pinch
	if v == nil {
		v = b.pinch
	}
	if v == nil {
		return
	}
	for _, p := range []*pointer{a, b} {
		if p.pan != nil && p.panState == 1 {
			m.send(p.pan, m.panEvent(p, driver.GestureCancelled))
		}
		p.panState = -1
	}
	dx, dy := float64(b.at.X-a.at.X), float64(b.at.Y-a.at.Y)
	tw := &twoFinger{view: v, a: a, b: b, scale: 1, dist: math.Max(1, math.Hypot(dx, dy)), angle: math.Atan2(dy, dx)}
	tw.lastAngle = tw.angle
	m.touches.two = tw
	m.sendTwoFinger(tw, driver.GestureBegan)
}

func (m *mounter) moveTwoFinger(tw *twoFinger) {
	dx, dy := float64(tw.b.at.X-tw.a.at.X), float64(tw.b.at.Y-tw.a.at.Y)
	tw.scale = math.Hypot(dx, dy) / tw.dist
	angle := math.Atan2(dy, dx)
	tw.rotation += math.Remainder(angle-tw.lastAngle, 2*math.Pi)
	tw.lastAngle = angle
	m.sendTwoFinger(tw, driver.GestureChanged)
}

func (m *mounter) sendTwoFinger(tw *twoFinger, phase driver.GesturePhase) {
	center := driver.Point{X: (tw.a.at.X + tw.b.at.X) / 2, Y: (tw.a.at.Y + tw.b.at.Y) / 2}
	pinch := m.pointer(tw.view, driver.EventPinch, center)
	pinch.Phase, pinch.Value = phase, float32(tw.scale)
	m.send(tw.view, pinch)
	rotation := m.pointer(tw.view, driver.EventRotation, center)
	rotation.Phase, rotation.Value = phase, float32(tw.rotation)
	m.send(tw.view, rotation)
}
