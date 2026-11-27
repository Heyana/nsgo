package core

import (
	"image"
	"image/color"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestRenderDrawingRequestAndUnavailable(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node {
		cx = c
		return View{}
	})
	h.commit()

	// Unavailable when Env.CanRenderDrawings is false.
	if cx.Env().CanRenderDrawings() {
		t.Fatal("CanRenderDrawings = true before environment enables it")
	}
	d := &Drawing{}
	d.SetFillColor(RGB(0xff0000)).FillRect(0, 0, 10, 10)
	var unavailErr error
	cx.RenderDrawing(d, Size{Width: 10, Height: 10}, 1, func(img image.Image, err error) {
		if img != nil {
			t.Errorf("unexpected image when unavailable: %v", img)
		}
		unavailErr = err
	})
	if item := command(h.commit(), driver.ItemRenderDrawing); item != nil {
		t.Fatalf("emitted ItemRenderDrawing when unavailable: %+v", item)
	}
	h.drain()
	if ErrorCode(unavailErr) != CodeUnavailable {
		t.Fatalf("unavailable error = %v, want CodeUnavailable", unavailErr)
	}

	// Enable CanRenderDrawings on the environment.
	h.tree.SetEnvironment(Environment{CanRenderDrawings: true})
	if !cx.Env().CanRenderDrawings() {
		t.Fatal("CanRenderDrawings = false after environment enabled it")
	}

	// Invalid dimensions fail without sending a mount item.
	var invalidErr error
	cx.RenderDrawing(d, Size{Width: 10, Height: 10}, 0, func(_ image.Image, err error) {
		invalidErr = err
	})
	if item := command(h.commit(), driver.ItemRenderDrawing); item != nil {
		t.Fatalf("emitted ItemRenderDrawing for invalid scale: %+v", item)
	}
	h.drain()
	if invalidErr == nil {
		t.Fatal("zero scale did not fail")
	}

	// Valid request snapshots pixel buffers and delivers the returned image once.
	pix := image.NewRGBA(image.Rect(0, 0, 2, 2))
	pix.SetRGBA(0, 0, color.RGBA{0, 200, 0, 255})
	withPix := &Drawing{}
	withPix.DrawImageIn(ImageSource{Pixels: pix}, 0, 0, 10, 10)
	var got image.Image
	var calls int
	cx.RenderDrawing(withPix, Size{Width: 10, Height: 10}, 2, func(img image.Image, err error) {
		if err != nil || cx.current() != cx {
			t.Errorf("render callback failed or wrong owner: %v", err)
		}
		got, calls = img, calls+1
	})
	// Mutating the caller's pixel buffer after RenderDrawing must not affect the request snapshot.
	pix.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255})

	item := command(h.commit(), driver.ItemRenderDrawing)
	if item == nil || item.RenderDrawing == nil || calls != 0 {
		t.Fatalf("render item = %+v, calls = %d", item, calls)
	}
	if item.RenderDrawing.Size != (Size{Width: 10, Height: 10}) || item.RenderDrawing.Scale != 2 {
		t.Fatalf("render params = %+v", item.RenderDrawing)
	}
	if snapped := item.RenderDrawing.Drawing.Image(&item.RenderDrawing.Drawing.Ops[0]).Pixels.RGBAAt(0, 0); snapped != (color.RGBA{0, 200, 0, 255}) {
		t.Fatalf("pixel snapshot mutated: %v", snapped)
	}
	out := image.NewRGBA(image.Rect(0, 0, 20, 20))
	answer := driver.RenderDrawingResult(item.Count, out, nil)
	h.tree.Dispatch(answer)
	h.tree.Dispatch(answer)
	if got != out || calls != 1 {
		t.Fatalf("got = %v after %d calls", got, calls)
	}
}

func TestRenderDrawingEnablesCanvasAndFiltersAccess(t *testing.T) {
	spy := &spyFeatures{}
	var cx *Scope
	tree := NewTree(NewRoot(RootOptions{Post: func(fn func()) { fn() }}), spy)
	tree.SetEnvironment(Environment{CanRenderDrawings: true})
	tree.Mount(func(c *Scope) Node {
		c.ProvideAccess(hostsAccess{hosts: []string{"allowed.example"}})
		cx = c
		return View{}
	})
	tree.Commit()
	if spy.canvas != 0 {
		t.Fatalf("canvas enabled before RenderDrawing: %d", spy.canvas)
	}
	d := &Drawing{}
	d.DrawImage(ImageSource{URI: "https://blocked.example/no.png"}, 0, 0)
	d.DrawImage(ImageSource{URI: "https://allowed.example/ok.png"}, 0, 0)
	cx.RenderDrawing(d, Size{Width: 10, Height: 10}, 1, func(image.Image, error) {})
	if spy.canvas != 1 {
		t.Fatalf("canvas enabler calls = %d, want 1", spy.canvas)
	}
	item := command(tree.Commit(), driver.ItemRenderDrawing)
	if item == nil || item.RenderDrawing == nil {
		t.Fatal("missing ItemRenderDrawing")
	}
	dr := item.RenderDrawing.Drawing
	if img := dr.Image(&dr.Ops[0]); img != nil && img.URI != "" {
		t.Errorf("blocked URI was not refused: %q", img.URI)
	}
	if img := dr.Image(&dr.Ops[1]); img == nil || img.URI != "https://allowed.example/ok.png" {
		t.Errorf("allowed URI was dropped: %v", img)
	}
}
