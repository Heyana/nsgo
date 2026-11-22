//go:build ios && nsgotest

package ios_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/ios"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestResizeRelayoutsTheNativeWindow exercises a native UIWindow viewport. It does not request a Split View, Slide Over or Stage Manager arrangement.
func TestResizeRelayoutsTheNativeWindow(t *testing.T) {
	var mu sync.Mutex
	var size core.Size
	var safe core.Edges
	var presses atomic.Int32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() {
			mu.Lock()
			defer mu.Unlock()
			size, safe = cx.Env().Window(), cx.Env().SafeArea()
		})
		return core.View{TestID: "root", Style: core.Style{FlexGrow: 1, Background: core.RGB(0x225588)}, Children: []core.Node{
			core.View{TestID: "corner", Style: core.Style{Position: core.Absolute, Right: core.Pt(16), Top: core.Pt(80), Width: core.Pt(40), Height: core.Pt(40), Background: core.RGB(0xee3311)},
				OnPress: func(core.PressEvent) { presses.Add(1) }},
		}}
	})
	app.WaitFor("id:corner", 5*time.Second)
	original, _ := ios.WindowGeometryForTest()
	defer app.Resize(original.Width, original.Height)
	for i, want := range []core.Size{{Width: 320, Height: 600}, {Width: 600, Height: 500}, {Width: 375, Height: 640}} {
		app.Resize(want.Width, want.Height)
		if got := app.Find("id:root").Frame.Size; got != want {
			t.Fatalf("resize to %+v laid out root at %+v", want, got)
		}
		nativeSize, nativeSafe := ios.WindowGeometryForTest()
		mu.Lock()
		gotSize, gotSafe := size, safe
		mu.Unlock()
		if nativeSize != want || gotSize != want || gotSafe != nativeSafe {
			t.Fatalf("resize to %+v: native=%+v safe=%+v, environment=%+v safe=%+v", want, nativeSize, nativeSafe, gotSize, gotSafe)
		}
		img := app.Capture()
		scale := float64(img.Bounds().Dx()) / float64(want.Width)
		if scale <= 0 || float64(img.Bounds().Dy()) != float64(want.Height)*scale {
			t.Fatalf("resize to %+v captured %v", want, img.Bounds().Size())
		}
		// The corner must move in the native hierarchy, rather than a stale frame stretching to fill the resized window.
		r, g, b, _ := img.At(img.Bounds().Min.X+int(float64(want.Width-36)*scale), img.Bounds().Min.Y+int(100*scale)).RGBA()
		if r>>8 < 200 || g>>8 > 90 || b>>8 > 70 {
			t.Fatalf("resize to %+v: corner pixel = %d,%d,%d", want, r>>8, g>>8, b>>8)
		}
		app.Press("id:corner")
		app.WaitUntil(5*time.Second, func() bool { return presses.Load() == int32(i+1) })
	}
}

func TestKeyboardInsetFollowsViewportWithoutAnotherNotification(t *testing.T) {
	var keyboard atomic.Int32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() { keyboard.Store(int32(cx.Env().KeyboardHeight())) })
		return core.View{TestID: "root", Style: core.Style{FlexGrow: 1}}
	})
	app.WaitFor("id:root", 5*time.Second)
	original, _ := ios.WindowGeometryForTest()
	defer app.Resize(original.Width, original.Height)
	defer ios.NotifyKeyboardFrameForTest(core.Bounds{})
	app.Resize(600, 700)
	ios.NotifyKeyboardFrameForTest(core.Bounds{Origin: core.Point{Y: 500}, Size: core.Size{Width: 600, Height: 200}})
	check := func(want int32) {
		t.Helper()
		app.Settle()
		if got := keyboard.Load(); got != want {
			t.Fatalf("keyboard inset = %d, want %d", got, want)
		}
	}
	check(200)
	// Keep the notified screen rectangle still as the window changes.
	for _, step := range []struct{ height, inset int32 }{{550, 50}, {450, 0}, {700, 200}} {
		app.Resize(600, core.Points(step.height))
		check(step.inset)
	}
	ios.NotifyKeyboardFrameForTest(core.Bounds{Origin: core.Point{X: 650, Y: 500}, Size: core.Size{Width: 200, Height: 200}})
	check(0)
	ios.NotifyKeyboardFrameForTest(core.Bounds{Origin: core.Point{Y: -100}, Size: core.Size{Width: 600, Height: 900}})
	check(700)
	ios.NotifyKeyboardFrameForTest(core.Bounds{})
	check(0)
}
