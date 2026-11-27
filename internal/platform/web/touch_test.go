//go:build js && wasm

package web

import (
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestTouchMovePreventDefaultSync(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	host := doc.Call("createElement", "div")
	doc.Get("body").Call("appendChild", host)
	defer host.Call("remove")

	s := &surface{doc: doc, host: host, views: make([]*view, 10)}
	var ls listeners
	s.listenTouches(&ls)
	defer ls.release()

	addView := func(id driver.NodeID, parent js.Value, kind driver.ViewKind, events driver.EventMask, pan driver.PanActivation, noScroll bool) *view {
		el := doc.Call("createElement", "div")
		el.Set("nsgoID", int(id))
		parent.Call("appendChild", el)
		v := &view{id: id, el: el, kind: kind, events: events, panActivation: pan}
		if kind == driver.KindScroll {
			v.sc = &scrollState{config: driver.ScrollConfig{Behavior: driver.ScrollBehavior{NoScroll: noScroll}}}
		}
		s.views[id] = v
		return v
	}

	touchMask := driver.EventMask(1 << driver.EventTouch)
	panMask := driver.EventMask(1 << driver.EventPan)

	// 1. Direct OnTouch view and its plain child prevent touchmove synchronously.
	pad := addView(1, host, driver.KindView, touchMask, driver.PanActivation{}, false)
	padChild := doc.Call("createElement", "span")
	pad.el.Call("appendChild", padChild)

	// 2. Scrollable KindScroll inside an OnTouch ancestor still allows native scroll.
	scroll := addView(2, pad.el, driver.KindScroll, 0, driver.PanActivation{}, false)
	scrollChild := addView(3, scroll.el, driver.KindView, 0, driver.PanActivation{}, false)

	// 3. OnTouch view inside a KindScroll prevents touchmove.
	innerPad := addView(4, scroll.el, driver.KindView, touchMask, driver.PanActivation{}, false)

	// 4. KindScroll with NoScroll inside an OnTouch ancestor falls through to the ancestor.
	lockedScroll := addView(5, pad.el, driver.KindScroll, 0, driver.PanActivation{}, true)
	lockedChild := addView(6, lockedScroll.el, driver.KindView, 0, driver.PanActivation{}, false)

	// 5. Editable input inside an OnTouch view keeps native touch behavior.
	inputView := addView(7, pad.el, driver.KindInput, 0, driver.PanActivation{}, false)
	inputEl := doc.Call("createElement", "input")
	inputView.el.Call("appendChild", inputEl)

	// 6. Single-axis pan + touch leaves perpendicular scrolling to the browser until ItemBlockNative.
	axisPan := addView(8, host, driver.KindView, panMask|touchMask, driver.PanActivation{ActiveOffsetX: 10}, false)

	// 7. Plain view without OnTouch does not prevent touchmove.
	plain := addView(9, host, driver.KindView, 0, driver.PanActivation{}, false)

	dispatchTouchMove := func(target js.Value) bool {
		ev := js.Global().Get("Event").New("touchmove", map[string]any{"bubbles": true, "cancelable": true})
		target.Call("dispatchEvent", ev)
		return ev.Get("defaultPrevented").Bool()
	}

	for _, tc := range []struct {
		name   string
		target js.Value
		want   bool
	}{
		{"OnTouch view", pad.el, true},
		{"child of OnTouch view", padChild, true},
		{"scroll view inside OnTouch ancestor", scroll.el, false},
		{"plain child inside scroll view", scrollChild.el, false},
		{"OnTouch view inside scroll view", innerPad.el, true},
		{"NoScroll child inside OnTouch ancestor", lockedChild.el, true},
		{"input view inside OnTouch", inputView.el, false},
		{"native input inside OnTouch", inputEl, false},
		{"axis-locked pan with touch", axisPan.el, false},
		{"plain view", plain.el, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.touchBlocksScroll(tc.target); got != tc.want {
				t.Errorf("touchBlocksScroll = %v, want %v", got, tc.want)
			}
			if got := dispatchTouchMove(tc.target); got != tc.want {
				t.Errorf("touchmove defaultPrevented = %v, want %v", got, tc.want)
			}
		})
	}

	// ItemBlockNative still blocks touchmove even on a plain view or axis-locked pan.
	s.touch.blocked = true
	if !dispatchTouchMove(plain.el) {
		t.Error("touchmove was not prevented while s.touch.blocked is true")
	}
	if !dispatchTouchMove(axisPan.el) {
		t.Error("touchmove on axis-locked pan was not prevented while s.touch.blocked is true")
	}
	s.touch.blocked = false
}
