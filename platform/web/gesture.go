//go:build js && wasm

package web

import (
	"math"
	"syscall/js"

	"github.com/oneo-me/nsgo/core"
)

// Gestures follow pointers on the host. A drag pans the nearest view
// that follows pans once its PanActivation activates it; the pan then
// captures the pointer and takes it from presses. Two touches pinch and
// rotate the nearest view that follows either, and desktop trackpads
// pinch it through Ctrl+wheel (Chrome, Firefox) or gesture events
// (Safari). Touch scrolling stays the browser's: views with gestures
// leave it only the axis their pan never takes (see touchAction), and the
// browser cancelling a pointer to scroll fails an undecided pan.

// velocityWindow is how far back a pointer's velocity is measured, in
// milliseconds; a pause longer than it leaves no velocity.
const velocityWindow = 100

// wheelPinchEnd is how long a trackpad pinch's wheel events pause before
// the pinch ends, in milliseconds.
const wheelPinchEnd = 150

// pointer is a pointer down on the host.
type pointer struct {
	id           int
	x, y         float64 // where it is, in the page's points
	startX       float64
	startY       float64
	samples      []sample
	pan          *view // the view it may pan
	panState     int   // as PanActivation.Decide: 1 active, -1 failed
	pinch        *view // the view two touches with it would pinch
	started, now float64
}

type sample struct{ t, x, y float64 }

// twoFinger is a pinch or rotation of two touches.
type twoFinger struct {
	view                *view
	a, b                *pointer
	dist, angle         float64 // at the start
	lastAngle, rotation float64 // the rotation unwrapped
	scale               float64
	gestureRate
}

// gestureRate follows how fast a pinch's scale and a rotation change.
type gestureRate struct {
	t                       float64
	scale, rotation         float64
	scaleSpeed, rotateSpeed float64
}

func (r *gestureRate) update(t, scale, rotation float64) {
	if dt := t - r.t; dt > 0 && r.t > 0 {
		r.scaleSpeed = (scale - r.scale) / dt * 1000
		r.rotateSpeed = (rotation - r.rotation) / dt * 1000
	}
	r.t, r.scale, r.rotation = t, scale, rotation
}

// wheelPinch is a trackpad pinch from Ctrl+wheel or Safari's gesture
// events.
type wheelPinch struct {
	view     *view
	scale    float64
	rotation float64
	timer    js.Value
	rate     gestureRate
}

type gestures struct {
	pointers []*pointer
	two      *twoFinger
	wheel    *wheelPinch
	wheelEnd js.Func
}

// listenGestures follows the host's pointers for gestures. Wheel and
// touchmove listeners are not passive, so they can keep the page from
// zooming or scrolling under a gesture.
func (s *surface) listenGestures(ls *listeners) {
	ls.on(s.host, "pointerdown", s.gestureDown)
	ls.on(s.host, "pointermove", s.gestureMove)
	ls.on(s.host, "pointerup", func(e js.Value) { s.gestureUp(e, false) })
	ls.on(s.host, "pointercancel", func(e js.Value) { s.gestureUp(e, true) })
	active := map[string]any{"passive": false}
	listen := func(target js.Value, name string, fn func(js.Value)) {
		target.Call("addEventListener", name, js.FuncOf(func(_ js.Value, args []js.Value) any {
			fn(args[0])
			return nil
		}), active)
	}
	listen(s.host, "wheel", s.wheelZoom)
	listen(s.host, "touchmove", func(e js.Value) {
		if s.gestures.two != nil || s.panning() {
			e.Call("preventDefault")
		}
	})
	// Safari's trackpad pinches; its touch pinches come as pointers.
	listen(s.host, "gesturestart", func(e js.Value) { s.safariGesture(e, core.GestureBegan) })
	listen(s.host, "gesturechange", func(e js.Value) { s.safariGesture(e, core.GestureChanged) })
	listen(s.host, "gestureend", func(e js.Value) { s.safariGesture(e, core.GestureEnded) })
	s.gestures.wheelEnd = js.FuncOf(func(js.Value, []js.Value) any {
		if w := s.gestures.wheel; w != nil {
			s.gestures.wheel = nil
			s.endWheelPinch(w, core.GestureEnded)
		}
		return nil
	})
}

// gestureView returns the nearest view holding el that follows kind.
func (s *surface) gestureView(el js.Value, kinds ...core.EventKind) *view {
	for ; el.Truthy() && !el.Equal(s.host); el = el.Get("parentElement") {
		if v := s.viewOf(el); v != nil {
			for _, k := range kinds {
				if v.events.Has(k) {
					return v
				}
			}
		}
	}
	return nil
}

func (s *surface) pointerOf(id int) *pointer {
	for _, p := range s.gestures.pointers {
		if p.id == id {
			return p
		}
	}
	return nil
}

// panning reports whether a pointer pans a view.
func (s *surface) panning() bool {
	for _, p := range s.gestures.pointers {
		if p.pan != nil && p.panState == 1 {
			return true
		}
	}
	return false
}

func (s *surface) gestureDown(e js.Value) {
	if e.Get("button").Int() != 0 {
		return
	}
	g := &s.gestures
	id := e.Get("pointerId").Int()
	if s.pointerOf(id) != nil {
		return
	}
	target := e.Get("target")
	x, y := e.Get("clientX").Float(), e.Get("clientY").Float()
	now := e.Get("timeStamp").Float()
	p := &pointer{id: id, x: x, y: y, startX: x, startY: y, started: now, now: now,
		samples: []sample{{now, x, y}}}
	// A drag in a text field selects; the first pointer alone pans.
	if !editing(target) && len(g.pointers) == 0 {
		p.pan = s.gestureView(target, core.EventPan)
	}
	p.pinch = s.gestureView(target, core.EventPinch, core.EventRotation)
	g.pointers = append(g.pointers, p)
	if len(g.pointers) == 2 && g.two == nil {
		s.startTwoFinger()
	}
}

func (s *surface) gestureMove(e js.Value) {
	p := s.pointerOf(e.Get("pointerId").Int())
	if p == nil {
		return
	}
	p.x, p.y, p.now = e.Get("clientX").Float(), e.Get("clientY").Float(), e.Get("timeStamp").Float()
	p.samples = append(p.samples, sample{p.now, p.x, p.y})
	if len(p.samples) > 16 {
		p.samples = p.samples[1:]
	}
	if t := s.gestures.two; t != nil && (t.a == p || t.b == p) {
		s.moveTwoFinger(t)
	}
	v := p.pan
	if v == nil || p.panState < 0 || s.view(v.id) != v {
		return
	}
	d := core.Point{X: core.Points(p.x - p.startX), Y: core.Points(p.y - p.startY)}
	if p.panState == 0 {
		p.panState = v.panActivation.Decide(d)
		if p.panState <= 0 {
			return
		}
		// The pan takes the pointer from presses and the page.
		v.el.Call("setPointerCapture", p.id)
		s.takePress(p.id)
		s.send(v, s.panEvent(p, core.GestureBegan))
		return
	}
	e.Call("preventDefault")
	s.send(v, s.panEvent(p, core.GestureChanged))
}

func (s *surface) gestureUp(e js.Value, cancelled bool) {
	g := &s.gestures
	p := s.pointerOf(e.Get("pointerId").Int())
	if p == nil {
		return
	}
	g.pointers = removePointer(g.pointers, p)
	if t := g.two; t != nil && (t.a == p || t.b == p) {
		g.two = nil
		phase := core.GestureEnded
		if cancelled {
			phase = core.GestureCancelled
		}
		s.sendTwoFinger(t, phase)
	}
	if p.pan == nil || p.panState != 1 {
		return
	}
	now := e.Get("timeStamp").Float()
	p.samples = append(p.samples, sample{now, p.x, p.y})
	phase := core.GestureEnded
	if cancelled {
		phase = core.GestureCancelled
	}
	s.send(p.pan, s.panEvent(p, phase))
}

func removePointer(ps []*pointer, p *pointer) []*pointer {
	out := ps[:0]
	for _, q := range ps {
		if q != p {
			out = append(out, q)
		}
	}
	return out
}

// takePress cancels the press of pointer id: its view is told the
// pointer went up, and no press follows.
func (s *surface) takePress(id int) {
	pr := &s.press
	if !pr.active || pr.pointer != id {
		return
	}
	s.stopLongPress()
	pr.cancel = true
	if pr.view != nil && !pr.done {
		pr.done = true
		s.send(pr.view, core.Event{Kind: core.EventPressUp})
	}
}

func (s *surface) panEvent(p *pointer, phase core.GesturePhase) core.Event {
	vx, vy := velocity(p.samples)
	return core.Event{Kind: core.EventPan, Phase: phase,
		Point:    core.Point{X: core.Points(p.x - p.startX), Y: core.Points(p.y - p.startY)},
		Velocity: core.Point{X: core.Points(vx), Y: core.Points(vy)}}
}

// velocity is the movement over the newest samples' last velocityWindow,
// in points per second: events come at uneven intervals, so a pair of
// them is noise.
func velocity(samples []sample) (float64, float64) {
	if len(samples) == 0 {
		return 0, 0
	}
	newest := samples[len(samples)-1]
	oldest := newest
	for i := len(samples) - 2; i >= 0 && newest.t-samples[i].t <= velocityWindow; i-- {
		oldest = samples[i]
	}
	dt := newest.t - oldest.t
	if dt < 1 {
		return 0, 0
	}
	return (newest.x - oldest.x) / dt * 1000, (newest.y - oldest.y) / dt * 1000
}

// startTwoFinger begins a pinch and a rotation of the two pointers down,
// on the view the first would pinch, else the second's.
func (s *surface) startTwoFinger() {
	a, b := s.gestures.pointers[0], s.gestures.pointers[1]
	v := a.pinch
	if v == nil {
		v = b.pinch
	}
	if v == nil {
		return
	}
	s.takePress(a.id)
	s.takePress(b.id)
	t := &twoFinger{view: v, a: a, b: b, scale: 1,
		dist:  math.Max(1, math.Hypot(b.x-a.x, b.y-a.y)),
		angle: math.Atan2(b.y-a.y, b.x-a.x)}
	t.lastAngle = t.angle
	t.gestureRate.update(math.Max(a.now, b.now), 1, 0)
	s.gestures.two = t
	s.sendTwoFinger(t, core.GestureBegan)
}

func (s *surface) moveTwoFinger(t *twoFinger) {
	a, b := t.a, t.b
	t.scale = math.Hypot(b.x-a.x, b.y-a.y) / t.dist
	// Screen angles grow clockwise; a turn past half a circle goes on.
	angle := math.Atan2(b.y-a.y, b.x-a.x)
	d := math.Remainder(angle-t.lastAngle, 2*math.Pi)
	t.lastAngle = angle
	t.rotation += d
	t.gestureRate.update(math.Max(a.now, b.now), t.scale, t.rotation)
	s.sendTwoFinger(t, core.GestureChanged)
}

func (s *surface) sendTwoFinger(t *twoFinger, phase core.GesturePhase) {
	s.sendPinch(t.view, phase, t.scale, t.rotation, &t.gestureRate)
}

// sendPinch reports a pinch's scale and a rotation's angle to v, each if
// v follows it.
func (s *surface) sendPinch(v *view, phase core.GesturePhase, scale, rotation float64, r *gestureRate) {
	s.send(v, core.Event{Kind: core.EventPinch, Phase: phase, Value: float32(scale),
		Velocity: core.Point{X: core.Points(r.scaleSpeed)}})
	s.send(v, core.Event{Kind: core.EventRotation, Phase: phase, Value: float32(rotation),
		Velocity: core.Point{X: core.Points(r.rotateSpeed)}})
}

// wheelZoom pinches the view under the pointer with Ctrl+wheel, which is
// how Chrome and Firefox report trackpad pinches, instead of zooming the
// page. The pinch ends when the wheel pauses.
func (s *surface) wheelZoom(e js.Value) {
	if !e.Get("ctrlKey").Bool() {
		return
	}
	g := &s.gestures
	w := g.wheel
	if w == nil || s.view(w.view.id) != w.view {
		v := s.gestureView(e.Get("target"), core.EventPinch)
		if v == nil {
			return
		}
		w = &wheelPinch{view: v, scale: 1}
		g.wheel = w
		w.rate.update(e.Get("timeStamp").Float(), 1, 0)
		s.send(v, core.Event{Kind: core.EventPinch, Phase: core.GestureBegan, Value: 1})
	}
	e.Call("preventDefault")
	dy := e.Get("deltaY").Float()
	switch e.Get("deltaMode").Int() {
	case 1: // lines
		dy *= 16
	case 2: // pages
		dy *= 400
	}
	w.scale *= math.Exp(-dy / 100)
	w.rate.update(e.Get("timeStamp").Float(), w.scale, 0)
	s.send(w.view, core.Event{Kind: core.EventPinch, Phase: core.GestureChanged, Value: float32(w.scale),
		Velocity: core.Point{X: core.Points(w.rate.scaleSpeed)}})
	if w.timer.Truthy() {
		js.Global().Call("clearTimeout", w.timer)
	}
	w.timer = js.Global().Call("setTimeout", g.wheelEnd, wheelPinchEnd)
}

func (s *surface) endWheelPinch(w *wheelPinch, phase core.GesturePhase) {
	s.send(w.view, core.Event{Kind: core.EventPinch, Phase: phase, Value: float32(w.scale),
		Velocity: core.Point{X: core.Points(w.rate.scaleSpeed)}})
}

// safariGesture follows Safari's gesture events, its trackpad's pinches
// and rotations (its rotation in degrees, clockwise). Its touch gestures
// are left to the pointers.
func (s *surface) safariGesture(e js.Value, phase core.GesturePhase) {
	g := &s.gestures
	if len(g.pointers) > 0 {
		if g.two != nil {
			e.Call("preventDefault")
		}
		return
	}
	w := g.wheel
	if phase == core.GestureBegan {
		if w != nil && w.timer.Truthy() {
			// A Ctrl+wheel pinch is still open.
			js.Global().Call("clearTimeout", w.timer)
			g.wheel = nil
			s.endWheelPinch(w, core.GestureEnded)
		}
		v := s.gestureView(e.Get("target"), core.EventPinch, core.EventRotation)
		if v == nil {
			return
		}
		w = &wheelPinch{view: v}
		g.wheel = w
	}
	if w == nil || s.view(w.view.id) != w.view {
		return
	}
	e.Call("preventDefault")
	w.scale = e.Get("scale").Float()
	w.rotation = e.Get("rotation").Float() * math.Pi / 180
	if phase == core.GestureBegan {
		w.scale, w.rotation = 1, 0
	}
	w.rate.update(e.Get("timeStamp").Float(), w.scale, w.rotation)
	if phase == core.GestureEnded {
		g.wheel = nil
	}
	s.sendPinch(w.view, phase, w.scale, w.rotation, &w.rate)
}

// touchAction is what touches on a view with gestures leave to the
// browser: nothing on views that pinch, rotate or pan freely; a pan
// activated along one axis only leaves scrolling along the other, so a
// scroll view can still take the drag while the pan is undecided.
func (v *view) touchAction() string {
	m := v.events
	if m.Has(core.EventPinch) || m.Has(core.EventRotation) {
		return "none"
	}
	if !m.Has(core.EventPan) {
		return ""
	}
	a := v.panActivation
	switch {
	case a.ActiveOffsetX != 0 && a.ActiveOffsetY == 0:
		return "pan-y"
	case a.ActiveOffsetY != 0 && a.ActiveOffsetX == 0:
		return "pan-x"
	}
	return "none"
}

// hasGestures reports whether v follows a pan, pinch or rotation.
func (v *view) hasGestures() bool {
	m := v.events
	return m.Has(core.EventPan) || m.Has(core.EventPinch) || m.Has(core.EventRotation)
}
