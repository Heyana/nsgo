package driver

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestRenderPixelSize(t *testing.T) {
	d := NewDrawing(nil)
	w, h, err := RenderPixelSize(d, Size{Width: 10.5, Height: 20}, 2)
	if err != nil || w != 21 || h != 40 {
		t.Fatalf("RenderPixelSize = (%d, %d, %v), want (21, 40, nil)", w, h, err)
	}
	if _, _, err := RenderPixelSize(nil, Size{Width: 10, Height: 10}, 1); err == nil {
		t.Error("nil drawing did not fail")
	}
	withSlot := NewDrawing(nil)
	withSlot.Slots = []DrawSlot{{Op: 0}}
	if _, _, err := RenderPixelSize(withSlot, Size{Width: 10, Height: 10}, 1); err == nil {
		t.Error("unresolved animation slot did not fail")
	}
	withPaint := NewDrawing(nil)
	withPaint.ViewPaints = []ViewPaint{{Op: 0}}
	if _, _, err := RenderPixelSize(withPaint, Size{Width: 10, Height: 10}, 1); err == nil {
		t.Error("unresolved view paint did not fail")
	}
	for _, scale := range []float64{0, -1, math.NaN(), math.Inf(1), 1e10} {
		if _, _, err := RenderPixelSize(d, Size{Width: 10, Height: 10}, scale); err == nil {
			t.Errorf("invalid scale %v did not fail", scale)
		}
	}
	if _, _, err := RenderPixelSize(d, Size{Width: 0, Height: 10}, 1); err == nil {
		t.Error("zero width did not fail")
	}
}

func TestBGRAtoRGBA(t *testing.T) {
	// 2x2 premultiplied BGRA with 12-byte stride (4 bytes of row padding).
	rows := []byte{
		10, 20, 30, 255, 40, 50, 60, 128, 0, 0, 0, 0,
		70, 80, 90, 200, 0, 0, 0, 0, 0, 0, 0, 0,
	}
	got := BGRAtoRGBA(rows, 2, 2, 12)
	if got.Bounds() != image.Rect(0, 0, 2, 2) {
		t.Fatalf("bounds = %v", got.Bounds())
	}
	if c := got.RGBAAt(0, 0); c != (color.RGBA{30, 20, 10, 255}) {
		t.Errorf("(0,0) = %v", c)
	}
	if c := got.RGBAAt(1, 0); c != (color.RGBA{60, 50, 40, 128}) {
		t.Errorf("(1,0) = %v", c)
	}
	if c := got.RGBAAt(0, 1); c != (color.RGBA{90, 80, 70, 200}) {
		t.Errorf("(0,1) = %v", c)
	}
	if c := got.RGBAAt(1, 1); c != (color.RGBA{}) {
		t.Errorf("(1,1) = %v", c)
	}
}

func TestRenderDrawingResult(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	ev := RenderDrawingResult(7, img, nil)
	if ev.Kind != EventFiles || ev.Count != 7 || !ev.Finished || ev.Data != img || ev.Text != "" {
		t.Fatalf("success event = %+v", ev)
	}
	fail := RenderDrawingResult(8, nil, errSentinel{})
	if fail.Kind != EventFiles || fail.Count != 8 || fail.Finished || fail.Text != "boom" {
		t.Fatalf("error event = %+v", fail)
	}
}

type errSentinel struct{}

func (errSentinel) Error() string { return "boom" }
