package nsgotest

import (
	"image/color"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestSecondaryWindowGPUReadPixelsUsesItsDevice(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless software GPU")
	}
	app := StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(func(*core.Scope) core.Node { return core.Text{Content: "Main"} })
		desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Inspector", Width: 64, Height: 64}, func(*core.Scope) core.Node {
			return core.GPUView{TestID: "viewport", Style: core.Style{Width: core.Pt(64), Height: core.Pt(64)}}
		})
	}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	window := app.Surface("title:Inspector")
	img, err := window.GPUReadPixels("id:viewport")
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 64 || img.Bounds().Dy() != 64 {
		t.Fatalf("secondary window GPU size = %v, want 64x64", img.Bounds())
	}
}

func TestClosedWindowGPUFramesDoNotReachReusedViewIDs(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless software GPU")
	}
	view := func(*core.Scope) core.Node {
		return core.GPUView{TestID: "viewport", Style: core.Style{Width: core.Pt(32), Height: core.Pt(32)}}
	}
	app := StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(view)
		desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Inspector", Width: 32, Height: 32}, view)
	}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	main, inspector := app.Surface("index:0"), app.Surface("title:Inspector")
	id := main.Find("id:viewport").ID
	if inspector.Find("id:viewport").ID != id {
		t.Fatal("fixture must exercise equal view IDs in different trees")
	}
	frame := func(surface uint32, fill core.Color) driver.GPUFrame {
		return driver.GPUFrame{SurfaceID: surface, ViewID: id, Generation: 1, Passes: []driver.GPUPass{{
			ColorAttachments: []driver.GPUColorAttachment{{LoadOp: driver.GPULoadClear, StoreOp: driver.GPUStoreStore, ClearColor: fill}},
		}}}
	}
	app.submitGPU(frame(main.Config().ID, core.RGB(0xff0000)))
	if !inspector.Close() {
		t.Fatal("inspector did not close")
	}
	// A frame delayed past surface removal must not reach the main window's live device with the same node ID.
	app.submitGPU(frame(inspector.Config().ID, core.RGB(0x0000ff)))
	img, err := app.GPUReadPixels("id:viewport")
	if err != nil {
		t.Fatal(err)
	}
	if got := img.RGBAAt(0, 0); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("closed surface overwrote the main GPU: %v", got)
	}
}
