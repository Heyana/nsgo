//go:build android

package android

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestCanvasAtlasWire(t *testing.T) {
	src := driver.ImageSource{URI: "asset:atlas.png"}
	tile := driver.Bounds{Origin: driver.Point{X: 12, Y: 24}, Size: driver.Size{Width: 8, Height: 4}}
	d := driver.NewDrawing(nil).SetImageSmoothing(false).
		DrawAtlas(src, []driver.Bounds{tile, tile},
			[]driver.RSXform{{SCos: 2, TX: 16, TY: 32}, {SSin: -2, TX: 6, TY: 4}}, []float32{0.25}).
		SetAlpha(0.5)
	var w writer
	writeDrawing(&w, d)
	r := reader{buf: w.buf}
	length := r.u32()
	before := len(r.buf)
	if kind := r.byte(); kind != 41 {
		t.Fatalf("atlas kind = %d, want 41", kind)
	}
	if key, nearest, count := r.str(), r.byte(), r.u32(); key != src.Key() || nearest != 1 || count != 2 {
		t.Fatalf("encoded atlas is %q with sampling %d and %d sprites", key, nearest, count)
	}
	want := []float32{
		12, 24, 8, 4, 2, 0, 16, 32, 0.25,
		12, 24, 8, 4, 0, -2, 6, 4, 1,
	}
	got := make([]float32, len(want))
	for i := range got {
		got[i] = r.float()
	}
	if !slices.Equal(got, want) {
		t.Errorf("atlas data = %v, want %v", got, want)
	}
	if kind, alpha := r.byte(), r.float(); kind != byte(driver.DrawAlpha) || alpha != 0.5 {
		t.Errorf("op following the atlas is %d with alpha %v", kind, alpha)
	}
	if got := uint32(before - len(r.buf)); got != length {
		t.Errorf("read %d bytes of operations, want %d", got, length)
	}
}
