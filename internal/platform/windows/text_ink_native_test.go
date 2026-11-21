//go:build windows

package windows

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestTextInkDrawsPastItsBox verifies that a text's decoration lines and
// shadow draw outside its layout box unless the text or an ancestor clips
// with OverflowHidden, and that partial frames redraw the overflowing ink
// cleanly.
func TestTextInkDrawsPastItsBox(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-native-" + t.Name()
	steps := make(chan func(bool), 1)
	app := func(cx *core.Scope) core.Node {
		shown := core.NewSignal(cx, true)
		steps <- func(on bool) { cx.Post(func() { shown.Set(on) }) }
		return core.View{
			Style: core.Style{Width: driver.Pt(320), Height: driver.Pt(240), Background: driver.RGB(0xffffff)},
			Children: []core.Node{
				// Unclipped text with an underline offset below its 20pt box.
				core.Text{
					Content: "MMMM",
					StyleFunc: func() core.Style {
						s := core.Style{
							Position: core.Absolute, Left: driver.Pt(20), Top: driver.Pt(20),
							Width: driver.Pt(100), Height: driver.Pt(20),
							FontSize: 16, Color: driver.RGB(0x000000),
						}
						if shown.Get() {
							s.TextDecoration = core.TextDecorationUnderline
							s.TextDecorationColor = driver.RGB(0xff0000)
							s.TextDecorationThickness = driver.Pt(4)
							s.TextUnderlineOffset = driver.Pt(16)
						}
						return s
					},
				},
				// Unclipped text with a shadow offset past its 20pt box.
				core.Text{
					Content: "MMMM",
					StyleFunc: func() core.Style {
						s := core.Style{
							Position: core.Absolute, Left: driver.Pt(20), Top: driver.Pt(80),
							Width: driver.Pt(100), Height: driver.Pt(20),
							FontSize: 16, Color: driver.RGB(0x000000),
						}
						if shown.Get() {
							s.TextShadowColor = driver.RGB(0x00aa00)
							s.TextShadowOffset = driver.Point{X: 0, Y: 18}
							s.TextShadowRadius = 2
						}
						return s
					},
				},
				// Same overflowing underline and shadow clipped by an OverflowHidden ancestor.
				core.View{
					Style: core.Style{
						Position: core.Absolute, Left: driver.Pt(160), Top: driver.Pt(20),
						Width: driver.Pt(100), Height: driver.Pt(20),
						Overflow: core.Hidden,
					},
					Children: []core.Node{
						core.Text{
							Content: "MMMM",
							Style: core.Style{
								Width: driver.Pt(100), Height: driver.Pt(20),
								FontSize: 16, Color: driver.RGB(0x000000),
								TextDecoration:          core.TextDecorationUnderline,
								TextDecorationColor:     driver.RGB(0xff0000),
								TextDecorationThickness: driver.Pt(4),
								TextUnderlineOffset:     driver.Pt(16),
								TextShadowColor:         driver.RGB(0x00aa00),
								TextShadowOffset:        driver.Point{X: 0, Y: 18},
								TextShadowRadius:        2,
							},
						},
					},
				},
			},
		}
	}
	hwnd := startApp(t, title, app, 320, 240)
	var setShown func(bool)
	select {
	case setShown = <-steps:
	case <-time.After(5 * time.Second):
		t.Fatal("the app did not start")
	}
	var scale float64
	onMain(t, hwnd, func(w *window) { scale = w.scale })
	px := func(p float64) int { return int(p * scale) }
	hasColor := func(pix []byte, width, height int, x0, y0, x1, y1 int, match func(r, g, b byte) bool) bool {
		for y := max(y0, 0); y < min(y1, height); y++ {
			for x := max(x0, 0); x < min(x1, width); x++ {
				at := (y*width + x) * 4
				if at+3 >= len(pix) {
					continue
				}
				// BGRA from clientPixels.
				if match(pix[at+2], pix[at+1], pix[at]) {
					return true
				}
			}
		}
		return false
	}
	isRed := func(r, g, b byte) bool { return r > 0xb0 && g < 0x60 && b < 0x60 }
	isGreen := func(r, g, b byte) bool { return g > 0x70 && r < 0x70 && b < 0x70 }

	time.Sleep(400 * time.Millisecond)
	pix, w, h, ok := clientPixels(hwnd)
	if !ok {
		t.Skip("cannot read the window's pixels")
	}
	// Below the unclipped underline text's box (y = 40..65).
	if !hasColor(pix, int(w), int(h), px(20), px(42), px(120), px(65), isRed) {
		t.Error("underline offset past the text's box drew no red pixels below the box")
	}
	// Below the unclipped shadow text's box (y = 100..135).
	if !hasColor(pix, int(w), int(h), px(20), px(102), px(120), px(135), isGreen) {
		t.Error("text shadow past the text's box drew no green pixels below the box")
	}
	// Below the OverflowHidden ancestor (x = 160..260, y = 42..65): must be clipped.
	if hasColor(pix, int(w), int(h), px(160), px(42), px(260), px(65), func(r, g, b byte) bool { return isRed(r, g, b) || isGreen(r, g, b) }) {
		t.Error("text ink escaped an OverflowHidden ancestor")
	}

	// Clearing the overflowing underline and shadow on a partial frame leaves no stale ink.
	setShown(false)
	time.Sleep(400 * time.Millisecond)
	pix, w, h, ok = clientPixels(hwnd)
	if !ok {
		t.Skip("cannot read the window's pixels")
	}
	if hasColor(pix, int(w), int(h), px(20), px(42), px(120), px(65), isRed) {
		t.Error("partial frame left stale underline pixels past the text's box")
	}
	if hasColor(pix, int(w), int(h), px(20), px(102), px(120), px(135), isGreen) {
		t.Error("partial frame left stale shadow pixels past the text's box")
	}
}
