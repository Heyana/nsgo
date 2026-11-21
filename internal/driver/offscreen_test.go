package driver

import (
	"math"
	"testing"
)

func TestOffscreenCanvasHelpers(t *testing.T) {
	var nilOff *OffscreenCanvas
	if got := nilOff.EffectiveScale(); got != 1 {
		t.Fatalf("nil EffectiveScale = %v, want 1", got)
	}
	if w, h := nilOff.PixelSize(); w != 0 || h != 0 {
		t.Fatalf("nil PixelSize = %dx%d, want 0x0", w, h)
	}

	off := &OffscreenCanvas{
		ID:      7,
		Version: 2,
		Size:    Size{Width: 64.2, Height: 32},
		Scale:   2,
		Drawing: NewDrawing(nil).FillRect(0, 0, 10, 10),
	}
	if w, h := off.PixelSize(); w != 129 || h != 64 {
		t.Fatalf("PixelSize = %dx%d, want 129x64", w, h)
	}

	badScale := &OffscreenCanvas{ID: 1, Version: 1, Size: Size{Width: 10, Height: 10}, Scale: float32(math.NaN())}
	if got := badScale.EffectiveScale(); got != 1 {
		t.Fatalf("NaN scale EffectiveScale = %v, want 1", got)
	}

	src := ImageSource{Offscreen: off, Size: off.Size, Version: off.Version}
	if src.Empty() {
		t.Fatal("offscreen ImageSource reported Empty")
	}
	if got := src.Key(); got != "offscreen:7:2" {
		t.Fatalf("Key = %q, want offscreen:7:2", got)
	}
	if got := src.Density(); got != 2 {
		t.Fatalf("Density = %v, want 2", got)
	}

	same := ImageSource{
		Offscreen: &OffscreenCanvas{ID: 7, Version: 2, Size: off.Size, Scale: 2},
		Size:      off.Size,
		Version:   2,
	}
	if !src.Same(same) {
		t.Fatal("Same returned false for equal offscreen ID and Version")
	}
	next := ImageSource{
		Offscreen: &OffscreenCanvas{ID: 7, Version: 3, Size: off.Size, Scale: 2},
		Size:      off.Size,
		Version:   3,
	}
	if src.Same(next) {
		t.Fatal("Same returned true for different Version")
	}
	if got := ItemReleaseOffscreen.String(); got != "ItemReleaseOffscreen" {
		t.Fatalf("ItemReleaseOffscreen.String() = %q", got)
	}
}
