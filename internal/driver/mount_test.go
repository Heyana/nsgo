package driver

import (
	"math"
	"testing"
)

// TestImageSourceKeyNamesDataByItsBytes keys inline data by its bytes, not its address: a collected buffer's address can come back in a new buffer of the same length, whose other bytes must not name the cached image of the old ones (#738).
func TestImageSourceKeyNamesDataByItsBytes(t *testing.T) {
	data := []byte{1, 2, 3}
	before := ImageSource{Data: data}.Key()
	if again := (ImageSource{Data: append([]byte(nil), data...)}).Key(); again != before {
		t.Fatalf("the same bytes in another buffer have another key: %q, not %q", again, before)
	}
	data[0]++
	if after := (ImageSource{Data: data}).Key(); after == before {
		t.Fatalf("other bytes at the same address keep the key %q", after)
	}
}

// TestClipBoxIsThePaddingBox checks that a view clips its children inside its border: the box less each side's width, and each corner less the widths of the sides it joins, elliptical where they differ, fitted to the box first, as CSS clips overflow.
func TestClipBoxIsThePaddingBox(t *testing.T) {
	type radii = [4][2]float64
	for _, c := range []struct {
		name       string
		vs         ViewStyle
		x, y, w, h float64
		radii      radii
	}{
		{"no border", ViewStyle{CornerRadius: 16}, 0, 0, 100, 60, radii{{16, 16}, {16, 16}, {16, 16}, {16, 16}}},
		{"one width", ViewStyle{CornerRadius: 20, BorderWidth: 3}, 3, 3, 94, 54, radii{{17, 17}, {17, 17}, {17, 17}, {17, 17}}},
		{"sides", ViewStyle{CornerRadius: 12, Borders: Edges{Top: 4, Right: 1, Bottom: 1, Left: 2}}, 2, 4, 97, 55,
			radii{{10, 8}, {11, 8}, {11, 11}, {10, 11}}},
		{"wider than the corner", ViewStyle{CornerRadius: 2, BorderWidth: 5}, 5, 5, 90, 50, radii{}},
		{"fitted", ViewStyle{CornerRadius: 100, BorderWidth: 10}, 10, 10, 80, 40, radii{{20, 20}, {20, 20}, {20, 20}, {20, 20}}},
	} {
		x, y, w, h, r := c.vs.ClipBox(Size{Width: 100, Height: 60})
		near := x == c.x && y == c.y && w == c.w && h == c.h
		for i := range r {
			for j := range r[i] {
				near = near && math.Abs(r[i][j]-c.radii[i][j]) < 1e-4
			}
		}
		if !near {
			t.Errorf("%s: ClipBox = (%v, %v, %v, %v) %v, want (%v, %v, %v, %v) %v", c.name, x, y, w, h, r, c.x, c.y, c.w, c.h, c.radii)
		}
	}
}
