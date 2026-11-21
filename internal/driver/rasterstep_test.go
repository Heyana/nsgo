package driver

import (
	"math"
	"testing"
)

func TestRasterStep(t *testing.T) {
	cases := []struct {
		scale float32
		want  float64
	}{
		{float32(math.NaN()), 1},
		{-2, 1},
		{0, 1},
		{0.5, 1},
		{1, 1},
		{1.01, 1.5},
		{1.5, 1.5},
		{1.51, 2},
		{2, 2},
		{2.25, 2.5},
		{2.5, 2.5},
		{3, 3},
		{3.4, 3.5},
		{3.6, 4},
		{4, 4},
		{10, 4},
	}
	for _, tc := range cases {
		if got := RasterStep(tc.scale); got != tc.want {
			t.Errorf("RasterStep(%v) = %v, want %v", tc.scale, got, tc.want)
		}
	}
}

func TestTransformRasterScaleAndAxisAligned(t *testing.T) {
	if got := (Transform{}).RasterScale(); got != 1 {
		t.Errorf("identity RasterScale = %v, want 1", got)
	}
	if got := (Transform{Scale: 3}).RasterScale(); got != 3 {
		t.Errorf("Scale 3 RasterScale = %v, want 3", got)
	}
	if got := (Transform{ScaleX: -2.5, ScaleY: 1.5}).RasterScale(); got != 2.5 {
		t.Errorf("ScaleX -2.5 RasterScale = %v, want 2.5", got)
	}
	chained := Transform{Scale: 2, Then: Matrix3D{1.5, 0, 0, 0, 0, 1.5, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
	if got := chained.RasterScale(); math.Abs(float64(got-3)) > 1e-5 {
		t.Errorf("chained RasterScale = %v, want 3", got)
	}
	if !chained.AxisAligned() {
		t.Error("chained scale should be axis aligned")
	}
	if (Transform{Rotate: 45}).AxisAligned() {
		t.Error("rotated transform should not be axis aligned")
	}
	if (Transform{SkewX: 10}).AxisAligned() {
		t.Error("skewed transform should not be axis aligned")
	}
	if (Transform{Perspective: 500}).AxisAligned() {
		t.Error("perspective transform should not be axis aligned")
	}
}
