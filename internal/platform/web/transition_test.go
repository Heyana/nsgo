//go:build js && wasm

package web

import (
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Run in a browser: nsgo test -target web.
func TestTransitionsRunBesideAnimatedValues(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	host := doc.Call("createElement", "div")
	doc.Get("body").Call("appendChild", host)
	defer host.Call("remove")
	s := &surface{doc: doc, host: host, animator: driver.NewAnimator(nil)}

	tr := driver.Transition{Duration: 200 * time.Millisecond, Easing: driver.EaseOut}
	style := driver.ViewStyle{
		Opacity:    1,
		Background: driver.RGB(0x112233),
		Transform:  driver.Transform{Scale: 1},
		Transition: tr,
	}
	s.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemViewStyle, ID: 1, ViewStyle: &style},
		{Kind: driver.ItemAnimation, Animation: []driver.AnimationOp{
			{Kind: driver.AnimCreateValue, Node: 10, Value: 12},
			{Kind: driver.AnimBind, View: 1, Prop: driver.PropTranslateX, Node: 10},
		}},
	}})
	v := s.view(1)
	inline := func(prop string) string {
		return v.el.Get("style").Call("getPropertyValue", prop).String()
	}
	if inline("opacity") != "" || inline("background-color") != "" {
		t.Errorf("unbound properties set inline: opacity=%q background=%q", inline("opacity"), inline("background-color"))
	}
	if !strings.Contains(inline("transform"), "12px") {
		t.Errorf("bound transform inline = %q, want 12px", inline("transform"))
	}
	if trCSS := inline("transition"); strings.Contains(trCSS, "transform") || !strings.Contains(trCSS, "opacity") || !strings.Contains(trCSS, "background-color") {
		t.Errorf("inline transition = %q, want opacity and background-color without transform", trCSS)
	}

	// Changing Opacity and Scale keeps CSS opacity transition and starts a frame-stepped transform transition for unbound Scale beside TranslateX.
	style.Opacity = 0.2
	style.Transform.Scale = 0.5
	s.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 1, ViewStyle: &style},
	}})
	if v.styleTr == nil || !s.transitions[v] {
		t.Fatal("unbound Scale change beside bound TranslateX did not start a frame-stepped transition")
	}
	if inline("opacity") != "" {
		t.Errorf("unbound opacity written inline = %q", inline("opacity"))
	}
	s.stepTransitions(s.clock() + 100*time.Millisecond)
	if tf := inline("transform"); !strings.Contains(tf, "12px") || strings.Contains(tf, "scale(0.5, 0.5)") || strings.Contains(tf, "scale(1, 1)") {
		t.Errorf("mid-transition transform = %q, want translate 12px and intermediate scale", tf)
	}
}
