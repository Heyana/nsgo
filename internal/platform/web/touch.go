//go:build js && wasm

package web

import (
	"slices"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Touches for the views that listen to them: each pointer down on the
// page is a finger of core.EventTouch, seen before the views below take
// it. The browser cancels a pointer it scrolls the page with.

// fingers are the pointers down, by pointerId, in order.
type fingers struct {
	geometry driver.TouchTracker
	down     []driver.Touch
	blocked  bool  // the responder stops the browser's scrolling
	handled  []int // touch identifiers started in a touch-handling view
}

func (s *surface) listenTouches(ls *listeners) {
	for _, name := range []string{"pointerdown", "pointermove", "pointerup", "pointercancel"} {
		fn := js.FuncOf(func(_ js.Value, args []js.Value) any {
			s.finger(args[0])
			return nil
		})
		*ls = append(*ls, listener{s.host, name, fn})
		s.host.Call("addEventListener", name, fn, true)
	}
	passive := map[string]any{"passive": true}
	start := js.FuncOf(func(_ js.Value, args []js.Value) any {
		s.touchStart(args[0])
		return nil
	})
	*ls = append(*ls, listener{s.host, "touchstart", start})
	s.host.Call("addEventListener", "touchstart", start, passive)
	for _, name := range []string{"touchend", "touchcancel"} {
		end := js.FuncOf(func(_ js.Value, args []js.Value) any {
			s.touchEnd(args[0])
			return nil
		})
		*ls = append(*ls, listener{s.host, name, end})
		s.host.Call("addEventListener", name, end, passive)
	}
	// A touchmove the page ignores scrolls it, so one is cancelled synchronously
	// when a touch started in a touch-handling view or while the touch responder
	// blocks native scrolling.
	block := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if args[0].Get("cancelable").Bool() && s.shouldPreventTouchMove(args[0]) {
			args[0].Call("preventDefault")
		}
		return nil
	})
	*ls = append(*ls, listener{s.host, "touchmove", block})
	s.host.Call("addEventListener", "touchmove", block, map[string]any{"passive": false})
}

func (s *surface) touchStart(e js.Value) {
	changed := e.Get("changedTouches")
	if !changed.Truthy() {
		return
	}
	for i := range changed.Length() {
		t := changed.Index(i)
		if s.touchBlocksScroll(t.Get("target")) {
			id := t.Get("identifier").Int()
			if !slices.Contains(s.touch.handled, id) {
				s.touch.handled = append(s.touch.handled, id)
			}
		}
	}
}

func (s *surface) touchEnd(e js.Value) {
	if touches := e.Get("touches"); touches.Truthy() && touches.Length() == 0 {
		s.touch.handled = s.touch.handled[:0]
		return
	}
	changed := e.Get("changedTouches")
	if !changed.Truthy() {
		return
	}
	for i := range changed.Length() {
		id := changed.Index(i).Get("identifier").Int()
		if idx := slices.Index(s.touch.handled, id); idx >= 0 {
			s.touch.handled = slices.Delete(s.touch.handled, idx, idx+1)
		}
	}
}

func (s *surface) shouldPreventTouchMove(e js.Value) bool {
	if s.touch.blocked || len(s.touch.handled) > 0 {
		return true
	}
	if touches := e.Get("touches"); touches.Truthy() && touches.Length() > 0 {
		for i := range touches.Length() {
			if s.touchBlocksScroll(touches.Index(i).Get("target")) {
				return true
			}
		}
		return false
	}
	return s.touchBlocksScroll(e.Get("target"))
}

// touchBlocksScroll reports whether a touch starting at target lands in a
// touch-handling view that takes drags directly, before reaching a scrollable
// view or an editable input.
func (s *surface) touchBlocksScroll(target js.Value) bool {
	if !target.Truthy() || !s.host.Call("contains", target).Bool() {
		return false
	}
	for el := target; el.Truthy() && !el.Equal(s.host); el = el.Get("parentElement") {
		if editing(el) {
			return false
		}
		v := s.viewOf(el)
		if v == nil {
			continue
		}
		if v.kind == driver.KindInput {
			return false
		}
		if v.kind == driver.KindScroll {
			if v.sc == nil || !v.sc.config.Behavior.NoScroll {
				return false
			}
			continue
		}
		if v.events.Has(driver.EventTouch) {
			return v.touchAction() == "none"
		}
	}
	return false
}

// finger reports pointer event e as a finger, while a view listens.
func (s *surface) finger(e js.Value) {
	id := e.Get("pointerId").Int()
	f := &s.touch
	i := slices.IndexFunc(f.down, func(t driver.Touch) bool { return t.ID == id })
	var phase driver.GesturePhase
	switch e.Get("type").String() {
	case "pointerdown":
		if i >= 0 || e.Get("button").Int() != 0 || !s.listensTouches() {
			return
		}
		phase = driver.GestureBegan
		f.down = append(f.down, driver.Touch{ID: id})
		i = len(f.down) - 1
	case "pointermove":
		if i < 0 {
			return
		}
		phase = driver.GestureChanged
	case "pointerup":
		phase = driver.GestureEnded
	default:
		phase = driver.GestureCancelled
	}
	if i < 0 {
		return
	}
	r := s.host.Call("getBoundingClientRect")
	f.down[i].PagePosition = driver.Point{
		X: driver.Points(e.Get("clientX").Float() - r.Get("left").Float() - s.host.Get("clientLeft").Float()),
		Y: driver.Points(e.Get("clientY").Float() - r.Get("top").Float() - s.host.Get("clientTop").Float()),
	}
	touches := slices.Clone(f.down)
	touches[i].Changed = true
	if phase == driver.GestureEnded || phase == driver.GestureCancelled {
		f.down = slices.Delete(f.down, i, i+1)
	}
	ev := driver.Event{Kind: driver.EventTouch, Phase: phase, Touches: touches,
		Time: time.Duration(e.Get("timeStamp").Float() * float64(time.Millisecond))}
	s.captureTouches(&ev)
	s.engine.Dispatch(ev)
}

func (s *surface) captureTouches(e *driver.Event) {
	r := s.host.Call("getBoundingClientRect")
	x := r.Get("left").Float() + s.host.Get("clientLeft").Float()
	y := r.Get("top").Float() + s.host.Get("clientTop").Float()
	// Native wrappers, text spans and scroll contents have no view ID.
	viewOf := func(el js.Value) *view {
		for ; el.Truthy() && !el.Equal(s.host); el = el.Get("parentElement") {
			if v := s.viewOf(el); v != nil {
				return v
			}
		}
		return nil
	}
	s.touch.geometry.Capture(e, func(p driver.Point) driver.NodeID {
		el := s.doc.Call("elementFromPoint", x+float64(p.X), y+float64(p.Y))
		if el.Truthy() && s.host.Call("contains", el).Bool() {
			if v := viewOf(el); v != nil {
				return v.id
			}
		}
		return 0
	}, func(id driver.NodeID) (driver.NodeID, driver.EventMask) {
		v := s.view(id)
		if v == nil {
			return 0, 0
		}
		var up driver.NodeID
		if p := viewOf(v.el.Get("parentElement")); p != nil {
			up = p.id
		}
		return up, v.events
	}, func(id driver.NodeID, p driver.Point) (driver.Point, bool) {
		v := s.view(id)
		if v == nil {
			return driver.Point{}, false
		}
		lx, ly := v.localPoint(x+float64(p.X), y+float64(p.Y))
		return driver.Point{X: driver.Points(lx), Y: driver.Points(ly)}, true
	})
}

// listensTouches reports whether a view listens to touches.
func (s *surface) listensTouches() bool {
	for _, v := range s.views {
		if v != nil && v.events.Has(driver.EventTouch) {
			return true
		}
	}
	return false
}
