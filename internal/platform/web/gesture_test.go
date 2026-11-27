//go:build js && wasm

package web

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestRemovePointerLetsGoOfTheRemoved(t *testing.T) {
	a, b, c := &pointer{id: 1}, &pointer{id: 2}, &pointer{id: 3}
	ps := []*pointer{a, b, c}
	got := removePointer(ps, b)
	if len(got) != 2 || got[0] != a || got[1] != c {
		t.Fatalf("removePointer left %v", got)
	}
	if ps[2] != nil {
		t.Error("the backing array keeps the removed pointer's place")
	}
}

func TestTouchAction(t *testing.T) {
	touch := driver.EventMask(1 << driver.EventTouch)
	pan := driver.EventMask(1 << driver.EventPan)
	pinch := driver.EventMask(1 << driver.EventPinch)
	rotate := driver.EventMask(1 << driver.EventRotation)

	for _, tc := range []struct {
		name string
		v    view
		want string
	}{
		{"plain", view{}, ""},
		{"touch only", view{events: touch}, "none"},
		{"pinch", view{events: pinch}, "none"},
		{"rotation", view{events: rotate}, "none"},
		{"pan both axes", view{events: pan}, "none"},
		{"pan horizontal", view{events: pan, panActivation: driver.PanActivation{ActiveOffsetX: 10}}, "pan-y"},
		{"pan vertical", view{events: pan, panActivation: driver.PanActivation{ActiveOffsetY: 10}}, "pan-x"},
		{"pan both explicit", view{events: pan, panActivation: driver.PanActivation{ActiveOffsetX: 10, ActiveOffsetY: 10}}, "none"},
		{"horizontal pan with touch", view{events: pan | touch, panActivation: driver.PanActivation{ActiveOffsetX: 10}}, "pan-y"},
		{"vertical pan with touch", view{events: pan | touch, panActivation: driver.PanActivation{ActiveOffsetY: 10}}, "pan-x"},
		{"scroll view ignores touchAction", view{kind: driver.KindScroll, events: touch}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v.touchAction(); got != tc.want {
				t.Fatalf("touchAction() = %q, want %q", got, tc.want)
			}
			css := string(tc.v.feelCSS(nil))
			gotCSS := ""
			for _, d := range strings.Split(css, ";") {
				if name, value, ok := strings.Cut(d, ":"); ok && name == "touch-action" {
					gotCSS = value
				}
			}
			if gotCSS != tc.want {
				t.Fatalf("feelCSS touch-action = %q, want %q (%s)", gotCSS, tc.want, css)
			}
		})
	}
}
