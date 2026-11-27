//go:build windows

package windows

import (
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestTransparentFittedPanelKeepsItsAnchor opens a transparent fitted panel
// beside an anchor with ShowAt, verifies DirectComposition attachment and
// rounded-card alpha pixels, and checks that growing and shrinking its content
// resizes the native HWND and swap chain while keeping it beside the anchor.
func TestTransparentFittedPanelKeepsItsAnchor(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a native panel")
	}
	type control struct {
		panel  *core.Window
		height *core.Signal[driver.Points]
		scope  *core.Scope
	}
	ready := make(chan control, 1)
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		h := core.NewSignal(cx, driver.Points(80))
		p := cx.OpenPanel(core.PanelOptions{
			Width:       280,
			Height:      240,
			FitContent:  true,
			Transparent: true,
			NoActivate:  true,
		}, func(*core.Scope) core.Node {
			return core.View{StyleFunc: func() core.Style {
				return core.Style{
					Width:        core.Pt(180),
					Height:       core.Pt(h.Get()),
					Background:   core.RGB(0x2255aa),
					CornerRadius: 12,
				}
			}}
		})
		ready <- control{p, h, cx}
	})
	root := startAppWith(t, app, Options{Title: "nsgo-fitted-panel-host", Width: 100, Height: 80, Hidden: true})
	var ctl control
	select {
	case ctl = <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("the app did not open the fitted panel")
	}
	defer ctl.scope.Post(ctl.panel.Close)

	var panel *window
	var primary displayPlacement
	var haveMonitor bool
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		onMain(t, root, func(own *window) {
			if !haveMonitor {
				if all := monitorLayout(); len(all) > 0 {
					primary = all[0]
					for _, m := range all {
						if m.display.Main {
							primary = m
							break
						}
					}
					haveMonitor = true
				}
			}
			for _, w := range windows {
				if w.hostWindow == own && w.kind == driver.SurfacePanel && w.root != 0 && !w.destroying {
					panel = w
				}
			}
		})
		if panel != nil && haveMonitor {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the native transparent fitted panel did not mount")
		}
	}

	// Place the anchor low enough in the primary work area that PlaceTop has
	// room for both the initial 80 pt and grown 180 pt panel heights.
	vis := primary.display.VisibleFrame
	anchor := driver.Bounds{
		Origin: driver.Point{
			X: vis.Origin.X + vis.Size.Width/2 - 20,
			Y: vis.Origin.Y + max(vis.Size.Height*2/3, 220),
		},
		Size: driver.Size{Width: 40, Height: 24},
	}
	shownErr := make(chan error, 1)
	ctl.scope.Post(func() { shownErr <- ctl.panel.ShowAt(anchor, core.PlaceTop) })
	if err := <-shownErr; err != nil {
		t.Fatal(err)
	}

	var shown bool
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		onMain(t, root, func(*window) {
			v, _, _ := isWindowVisible.Call(panel.hwnd)
			shown = v != 0
		})
		if shown {
			break
		}
		if time.Now().After(deadline) {
			if !sessionActive() {
				t.Skip("native panel visibility needs an interactive Windows desktop")
			}
			t.Fatal("the transparent fitted panel did not show")
		}
	}

	var composited bool
	onMain(t, root, func(*window) {
		composited = compositionAvailable()
		if got := panel.environment().WindowTransparent; got != composited {
			t.Errorf("WindowTransparent = %v, want %v", got, composited)
		}
		if composited {
			ex, _, _ := getWindowLongPtrW.Call(panel.hwnd, gwlExStyle)
			if ex&wsExNoRedirect == 0 {
				t.Error("transparent panel lacks WS_EX_NOREDIRECTIONBITMAP")
			}
		}
	})

	checkStep := func(width, height driver.Points, label string) {
		t.Helper()
		wantW, wantH := primary.px(width), primary.px(height)
		placedSize := driver.Size{Width: driver.Points(float64(wantW) / primary.scale), Height: driver.Points(float64(wantH) / primary.scale)}
		wantOrigin, _ := driver.PlaceWindow(anchor, placedSize, vis, driver.WindowPlaceTop)
		wx, wy := primary.pixel(wantOrigin.X, wantOrigin.Y)
		want := rect{wx, wy, wx + wantW, wy + wantH}

		var got rect
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			onMain(t, root, func(*window) {
				getWindowRect.Call(panel.hwnd, uintptr(unsafe.Pointer(&got)))
			})
			if got == want {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s panel rect = %+v, want %+v", label, got, want)
			}
		}

		// Render through DirectComposition and verify both the visual tree
		// attachment and the swap chain's alpha pixels at the new size.
		var pix []byte
		var swapW, swapH uint32
		var visualAttached, swapComposed bool
		onMain(t, root, func(*window) {
			// Two frames populate both buffers of the flip-sequential swap chain.
			panel.redrawAll()
			panel.frame()
			panel.redrawAll()
			panel.frame()
			if panel.swap != nil {
				swapW, swapH, swapComposed = panel.swap.width, panel.swap.height, panel.swap.composed
			}
			if panel.composition != nil && len(panel.composition.order) > 0 && panel.composition.order[0] == panel.composition.base {
				visualAttached = true
			}
			if composited && panel.swap != nil && panel.swap.target != nil {
				if g, err := devices(); err == nil {
					pix = bitmapPixels(g.dc, panel.swap.target, panel.swap.width, panel.swap.height, float32(panel.scale))
				}
			}
		})
		if swapW != uint32(wantW) || swapH != uint32(wantH) {
			t.Fatalf("%s swap chain size = %dx%d, want %dx%d", label, swapW, swapH, wantW, wantH)
		}
		if !composited {
			return
		}
		if !swapComposed {
			t.Errorf("%s transparent panel did not use a composition swap chain", label)
		}
		if !visualAttached {
			t.Errorf("%s DirectComposition root did not attach the panel's base visual", label)
		}
		if len(pix) != int(swapW)*int(swapH)*4 {
			t.Fatalf("%s could not read %dx%d swap chain pixels (got %d bytes)", label, swapW, swapH, len(pix))
		}
		center := (int(swapH)/2*int(swapW) + int(swapW)/2) * 4
		if pix[center+3] != 255 || pix[center+0] <= pix[center+2] {
			t.Errorf("%s card center BGRA = %v, want opaque blue", label, pix[center:center+4])
		}
		clear, antialias := 0, 0
		for i := 0; i < len(pix); i += 4 {
			switch a := pix[i+3]; {
			case a == 0:
				clear++
			case a < 255:
				antialias++
			}
		}
		if clear == 0 || antialias == 0 {
			t.Errorf("%s transparent rounded panel has no clear or antialiased pixels: clear=%d antialias=%d", label, clear, antialias)
		}
	}

	checkStep(180, 80, "initial")
	ctl.scope.Post(func() { ctl.height.Set(180) })
	checkStep(180, 180, "grown")
	ctl.scope.Post(func() { ctl.height.Set(40) })
	checkStep(180, 40, "shrunk")
}
