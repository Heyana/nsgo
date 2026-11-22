//go:build js && wasm && nsgotest

package web

import (
	"sync"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func TestWebTestHostPressSettlesNativeFocus(t *testing.T) {
	if js.Global().Get("nsgoTest").Type() != js.TypeFunction {
		t.Skip("requires the browser test host: run with nsgo test -target web")
	}

	var (
		mu      sync.Mutex
		focused = map[driver.NodeID]bool{}
		ids     = map[string]driver.NodeID{}
		frames  = map[driver.NodeID]driver.Bounds{}
		parents = map[driver.NodeID]driver.NodeID{}
	)
	observe := func(_ uint32, b *driver.MountBatch) {
		mu.Lock()
		defer mu.Unlock()
		for _, it := range b.Items {
			switch it.Kind {
			case driver.ItemInsert:
				parents[it.ID] = it.Parent
			case driver.ItemViewProps:
				if it.ViewProps != nil && it.ViewProps.TestID != "" {
					ids[it.ViewProps.TestID] = it.ID
				}
			case driver.ItemFrame:
				frames[it.ID] = it.Frame
			}
		}
	}
	onEvent := func(_ uint32, ev driver.Event) {
		mu.Lock()
		defer mu.Unlock()
		switch ev.Kind {
		case driver.EventFocus:
			focused[ev.ID] = true
		case driver.EventBlur:
			focused[ev.ID] = false
		}
	}

	inner := new(core.Ref)
	disabledRef := new(core.Ref)
	var scope *core.Scope
	surf, err := (host{}).Open(func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{Style: core.Style{Padding: core.All(20), Gap: 8}, Children: []core.Node{
			core.Input{TestID: "direct", Style: core.Style{Height: core.Pt(32)}},
			core.Input{TestID: "disabled", Ref: disabledRef, Disabled: func() bool { return true }, Style: core.Style{Height: core.Pt(32)}},
			core.View{TestID: "search", OnPress: func(core.PressEvent) { inner.Focus() }, Style: core.Style{Padding: core.All(20)}, Children: []core.Node{
				core.Input{TestID: "inner", Ref: inner, Style: core.Style{Height: core.Pt(32)}},
			}},
		}}
	}, testhost.Options{
		Size:    driver.Size{Width: 320, Height: 260},
		Observe: observe,
		Event:   onEvent,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := surf.(*testSurface)
	defer ts.s.engine.Close()

	settle := func() {
		t.Helper()
		surf.Sync()
		deadline := time.Now().Add(2 * time.Second)
		for surf.Animating() {
			if time.Now().After(deadline) {
				t.Fatal("timed out waiting for web test surface to settle")
			}
			time.Sleep(10 * time.Millisecond)
		}
		surf.Sync()
	}
	settle()

	originOf := func(testID string) driver.Bounds {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		id := ids[testID]
		if id == 0 {
			t.Fatalf("view %q was not mounted", testID)
		}
		b := frames[id]
		for p := parents[id]; p != 0; p = parents[p] {
			pb := frames[p]
			b.Origin.X += pb.Origin.X
			b.Origin.Y += pb.Origin.Y
		}
		return b
	}
	centerOf := func(testID string) driver.Point {
		b := originOf(testID)
		return driver.Point{X: b.Origin.X + b.Size.Width/2, Y: b.Origin.Y + b.Size.Height/2}
	}
	pointIn := func(testID string, dx, dy driver.Points) driver.Point {
		b := originOf(testID)
		return driver.Point{X: b.Origin.X + dx, Y: b.Origin.Y + dy}
	}
	isFocused := func(testID string) bool {
		mu.Lock()
		defer mu.Unlock()
		return focused[ids[testID]]
	}
	check := func(want, blurred string) {
		t.Helper()
		if !isFocused(want) || isFocused(blurred) {
			t.Fatalf("reported native focus: %s=%v, %s=%v", want, isFocused(want), blurred, isFocused(blurred))
		}
	}

	// Press in search padding triggers OnPress -> inner.Focus().
	if err := surf.Press(pointIn("search", 10, 10), 0, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	settle()
	check("inner", "direct")

	// Direct press on an input moves native focus and blurs the previous field.
	if err := surf.Press(centerOf("direct"), 0, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	settle()
	check("direct", "inner")

	// Modified press in search padding moves focus back via OnPress.
	if err := surf.Press(pointIn("search", 10, 10), 0, driver.Modifiers{Shift: true}); err != nil {
		t.Fatal(err)
	}
	settle()
	check("inner", "direct")

	// Modified direct press focuses direct again.
	if err := surf.Press(centerOf("direct"), 0, driver.Modifiers{Shift: true}); err != nil {
		t.Fatal(err)
	}
	settle()
	check("direct", "inner")

	// Pressing or programmatically focusing a disabled input must not report EventFocus.
	if err := surf.Press(centerOf("disabled"), 0, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	settle()
	if isFocused("disabled") {
		t.Fatal("disabled input reported focus after press")
	}
	scope.Post(disabledRef.Focus)
	settle()
	if isFocused("disabled") {
		t.Fatal("disabled input reported focus after Ref.Focus()")
	}
}
