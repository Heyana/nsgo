//go:build linux && !android

package linux

import (
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestBackdropFiltersBlurAndColorWhatIsBehind floats a blurred view over
// a red half and a blue half, which mix under it, and a grayscale one
// over the red half, which turns gray; when the halves change color, the
// blurred view follows, and once nothing changes, nothing draws.
func TestBackdropFiltersBlurAndColorWhatIsBehind(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-backdrop"
	var paint func(driver.Color)
	app := func(cx *core.Scope) core.Node {
		left := core.NewSignal(cx, driver.RGB(0xff0000))
		paint = func(c driver.Color) { cx.Post(func() { left.Set(c) }) }
		cx.SetTitle(title + "|ready")
		half := func(c func() driver.Color) core.Node {
			return core.View{StyleFunc: func() core.Style {
				return core.Style{Width: driver.Pt(100), Height: driver.Pt(100), Background: c()}
			}}
		}
		return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100), Direction: core.Row}, Children: []core.Node{
			half(left.Get),
			half(func() driver.Color { return driver.RGB(0x0000ff) }),
			core.View{Style: core.Style{Position: core.Absolute, Left: driver.Pt(60), Top: driver.Pt(10), Width: driver.Pt(80), Height: driver.Pt(40),
				BackdropFilters: []core.Filter{core.Blur(10)}}},
			core.View{Style: core.Style{Position: core.Absolute, Left: driver.Pt(10), Top: driver.Pt(60), Width: driver.Pt(40), Height: driver.Pt(30),
				BackdropFilters: []core.Filter{core.Grayscale(1)}}},
		}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 200, Height: 100})
	waitForTitle(t, w, title+"|ready")
	waitPixels(t, w, "red and blue mixed under the blur, gray under the grayscale view", func(p pixels) bool {
		mid, red, gray := p.at(100, 30), p.at(5, 30), p.at(30, 75) // BGRA
		return mid[0] > 60 && mid[2] > 60 && red[2] > 225 && red[0] < 30 &&
			gray[2] > 30 && gray[2] < 225 && absDiff(gray[0], gray[2]) < 8 && absDiff(gray[1], gray[2]) < 8
	})
	paint(driver.RGB(0x00ff00))
	waitPixels(t, w, "green and blue mixed under the blur", func(p pixels) bool {
		mid := p.at(100, 30)
		return mid[0] > 60 && mid[1] > 60 && mid[2] < 30
	})
	var frameCounter func(uintptr) int64
	purego.RegisterLibFunc(&frameCounter, gtkLib, "gdk_frame_clock_get_frame_counter")
	frames := func() (n int64) {
		w.onLoop(t, func() { n = frameCounter(gtkWidgetGetFrameClock(w.surface)) })
		return n
	}
	time.Sleep(200 * time.Millisecond)
	before := frames()
	time.Sleep(500 * time.Millisecond)
	if n := frames() - before; n > 2 {
		t.Errorf("%d frames drew in half a second with nothing changing", n)
	}
}

func absDiff(a, b byte) byte {
	if a > b {
		return a - b
	}
	return b - a
}
