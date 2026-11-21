package driver

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
)

func testPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCustomCursorSelectSourceHiDPI(t *testing.T) {
	png1x := testPNG(t, 16, 16, color.RGBA{R: 255, A: 255})
	png2x := testPNG(t, 32, 32, color.RGBA{G: 255, A: 255})

	both := &CustomCursor{
		Image:   ImageSource{Data: png1x},
		Image2x: ImageSource{Data: png2x},
	}
	for _, tc := range []struct {
		scale    float32
		want2x   bool
		wantData []byte
	}{
		{0, false, png1x},
		{1.0, false, png1x},
		{1.25, false, png1x},
		{1.49, false, png1x},
		{1.5, true, png2x},
		{2.0, true, png2x},
		{3.0, true, png2x},
	} {
		src, is2x := both.SelectSource(tc.scale)
		if is2x != tc.want2x || !bytes.Equal(src.Data, tc.wantData) {
			t.Errorf("scale=%v: got is2x=%v len=%d, want is2x=%v len=%d",
				tc.scale, is2x, len(src.Data), tc.want2x, len(tc.wantData))
		}
	}

	only1x := &CustomCursor{Image: ImageSource{Data: png1x}}
	if src, is2x := only1x.SelectSource(2.0); is2x || !bytes.Equal(src.Data, png1x) {
		t.Errorf("1x-only on 2x display: got is2x=%v len=%d", is2x, len(src.Data))
	}

	only2x := &CustomCursor{Image2x: ImageSource{Data: png2x}}
	if src, is2x := only2x.SelectSource(1.0); !is2x || !bytes.Equal(src.Data, png2x) {
		t.Errorf("2x-only on 1x display: got is2x=%v len=%d", is2x, len(src.Data))
	}

	var nilCursor *CustomCursor
	if !nilCursor.Empty() {
		t.Error("nil CustomCursor should be Empty")
	}
	if src, is2x := nilCursor.SelectSource(2.0); !src.Empty() || is2x {
		t.Errorf("nil CustomCursor.SelectSource = (%v, %v), want empty", src, is2x)
	}
}

func TestClampHotspot(t *testing.T) {
	nan := Points(math.NaN())
	inf := Points(math.Inf(1))
	ninf := Points(math.Inf(-1))

	for _, tc := range []struct {
		name string
		hot  Point
		size Size
		want Point
	}{
		{"inside", Point{X: 8, Y: 12}, Size{Width: 32, Height: 32}, Point{X: 8, Y: 12}},
		{"negative clamped to zero", Point{X: -5, Y: -1}, Size{Width: 32, Height: 32}, Point{X: 0, Y: 0}},
		{"exceeding bounds clamped to size-1", Point{X: 32, Y: 99}, Size{Width: 32, Height: 24}, Point{X: 31, Y: 23}},
		{"1x1 clamps to 0,0", Point{X: 5, Y: 5}, Size{Width: 1, Height: 1}, Point{X: 0, Y: 0}},
		{"unknown size keeps positive and clamps negative", Point{X: -3, Y: 10}, Size{}, Point{X: 0, Y: 10}},
		{"nan and inf safe", Point{X: nan, Y: inf}, Size{Width: 16, Height: 16}, Point{X: 0, Y: 15}},
		{"neg inf safe", Point{X: ninf, Y: inf}, Size{}, Point{X: 0, Y: 0}},
	} {
		if got := ClampHotspot(tc.hot, tc.size); got != tc.want {
			t.Errorf("%s: ClampHotspot(%v, %v) = %v, want %v", tc.name, tc.hot, tc.size, got, tc.want)
		}
	}
}

func TestClampHotspotPixels(t *testing.T) {
	for _, tc := range []struct {
		hot     Point
		w, h    int
		density float32
		wantX   int
		wantY   int
	}{
		{Point{X: 8, Y: 4}, 32, 32, 1, 8, 4},
		{Point{X: 8, Y: 4}, 64, 64, 2, 16, 8},
		{Point{X: -4, Y: -2}, 32, 32, 1, 0, 0},
		{Point{X: 50, Y: 50}, 32, 24, 1, 31, 23},
		{Point{X: 32, Y: 32}, 64, 64, 2, 63, 63},
		{Point{X: 5, Y: 5}, 0, 0, 1, 0, 0},
	} {
		x, y := ClampHotspotPixels(tc.hot, tc.w, tc.h, tc.density)
		if x != tc.wantX || y != tc.wantY {
			t.Errorf("ClampHotspotPixels(%v, %d, %d, %g) = (%d, %d), want (%d, %d)",
				tc.hot, tc.w, tc.h, tc.density, x, y, tc.wantX, tc.wantY)
		}
	}
}

func TestCustomCursorEqualAndLogicalSize(t *testing.T) {
	png1x := testPNG(t, 24, 18, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	png1xCopy := append([]byte(nil), png1x...)
	png2x := testPNG(t, 48, 36, color.RGBA{R: 10, G: 20, B: 30, A: 255})

	a := &CustomCursor{Image: ImageSource{Data: png1x}, Hotspot: Point{X: 4, Y: 5}, Fallback: CursorCrosshair}
	b := &CustomCursor{Image: ImageSource{Data: png1xCopy}, Hotspot: Point{X: 4, Y: 5}, Fallback: CursorCrosshair}
	if !a.Equal(b) {
		t.Error("CustomCursors with equal byte slices should be Equal")
	}
	if got := a.LogicalSize(); got != (Size{Width: 24, Height: 18}) {
		t.Errorf("LogicalSize(1x PNG) = %v, want 24x18", got)
	}

	only2x := &CustomCursor{Image2x: ImageSource{Data: png2x}}
	if got := only2x.LogicalSize(); got != (Size{Width: 24, Height: 18}) {
		t.Errorf("LogicalSize(2x-only PNG) = %v, want 24x18", got)
	}

	px := image.NewRGBA(image.Rect(0, 0, 20, 12))
	fromPx := &CustomCursor{Image: ImageSource{Pixels: px, Version: 1}}
	if got := fromPx.LogicalSize(); got != (Size{Width: 20, Height: 12}) {
		t.Errorf("LogicalSize(Pixels) = %v, want 20x12", got)
	}
	if fromPx.Equal(&CustomCursor{Image: ImageSource{Pixels: px, Version: 2}}) {
		t.Error("CustomCursors with different pixel Versions must not be Equal")
	}
}
