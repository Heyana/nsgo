package nsgotest

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestScopeRenderDrawingStubAndUnavailable(t *testing.T) {
	// Unavailable environment returns CodeUnavailable without calling the hook.
	var unavailableErr error
	var canRender bool
	app := Start(t, func(cx *core.Scope) core.Node {
		canRender = cx.Env().CanRenderDrawings()
		d := &core.Drawing{}
		d.SetFillColor(core.RGB(0x112233)).FillRect(0, 0, 8, 8)
		cx.RenderDrawing(d, core.Size{Width: 8, Height: 8}, 1, func(_ image.Image, err error) {
			unavailableErr = err
		})
		return core.View{}
	}, Environment(core.Environment{OS: core.OSAndroid}))
	app.Advance(0)
	if canRender {
		t.Fatal("CanRenderDrawings = true for Android environment")
	}
	if core.ErrorCode(unavailableErr) != core.CodeUnavailable {
		t.Fatalf("error = %v, want CodeUnavailable", unavailableErr)
	}

	// Custom RenderDrawing hook answers Scope.RenderDrawing in headless tests.
	var got image.Image
	var gotErr error
	want := color.RGBA{R: 12, G: 34, B: 56, A: 255}
	var scope *core.Scope
	app2 := Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{}
	}, Environment(core.Environment{CanRenderDrawings: true}))
	app2.RenderDrawing = func(d *core.Drawing, size core.Size, scale float64) (*image.RGBA, error) {
		img := image.NewRGBA(image.Rect(0, 0, int(float64(size.Width)*scale), int(float64(size.Height)*scale)))
		img.SetRGBA(0, 0, want)
		return img, nil
	}
	app2.Post(func() {
		d := &core.Drawing{}
		d.SetFillColor(core.RGB(0x0c2238)).FillRect(0, 0, 4, 4)
		scope.RenderDrawing(d, core.Size{Width: 4, Height: 4}, 2, func(img image.Image, err error) {
			got, gotErr = img, err
		})
	})
	app2.Advance(0)
	if gotErr != nil || got == nil {
		t.Fatalf("got = %v, err = %v", got, gotErr)
	}
	if got.Bounds() != image.Rect(0, 0, 8, 8) {
		t.Fatalf("bounds = %v, want 8x8", got.Bounds())
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, got); err != nil {
		t.Fatalf("png.Encode failed: %v", err)
	}
	decoded, err := png.Decode(&encoded)
	if err != nil {
		t.Fatalf("png.Decode failed: %v", err)
	}
	if c := color.RGBAModel.Convert(decoded.At(0, 0)).(color.RGBA); c != want {
		t.Fatalf("decoded pixel = %v, want %v", c, want)
	}
}
