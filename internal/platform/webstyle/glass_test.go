package webstyle

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestComputeGlassUniforms(t *testing.T) {
	none := driver.DefaultViewStyle
	if _, ok := ComputeGlassUniforms(&none, 100, 60, 2, false); ok {
		t.Fatal("GlassNone returned ok")
	}
	reg := driver.DefaultViewStyle
	reg.Glass = driver.GlassRegular
	reg.CornerRadius = 16
	reg.Background = driver.RGBA(0x2080ff, 0.4)
	if _, ok := ComputeGlassUniforms(&reg, 0, 60, 2, false); ok {
		t.Fatal("zero width returned ok")
	}

	u, ok := ComputeGlassUniforms(&reg, 100, 60, 2, false)
	if !ok {
		t.Fatal("light regular glass returned !ok")
	}
	if u.Rect != [4]float32{0, 0, 200, 120} {
		t.Errorf("Rect = %v, want [0 0 200 120]", u.Rect)
	}
	if u.Radii != [4]float32{32, 32, 32, 32} {
		t.Errorf("Radii = %v, want [32 32 32 32]", u.Radii)
	}
	// min(100, 60) = 60, bezel = min(36, 30) = 30 -> scaled by 2 = 60
	if u.P0[0] != 60 || math.Abs(float64(u.P0[1]-96)) > 1e-4 || u.P0[2] != 0.35 || u.P0[3] != 1 {
		t.Errorf("P0 = %v, want [60 96 0.35 1]", u.P0)
	}
	if math.Abs(float64(u.P1[3]-0.4)) > 0.01 {
		t.Errorf("P1 tint alpha = %v, want ~0.4", u.P1)
	}
	if u.P2 != [4]float32{0.541, 1, 1.2, 1} {
		t.Errorf("light P2 = %v", u.P2)
	}
	if math.Abs(float64(u.Blur-2.1)) > 1e-4 {
		t.Errorf("Blur = %v, want 2.1", u.Blur)
	}

	dark, ok := ComputeGlassUniforms(&reg, 100, 60, 1, true)
	if !ok || dark.P2 != [4]float32{0.15, 0.51, 2, 2.2} {
		t.Errorf("dark P2 = %v (ok=%v)", dark.P2, ok)
	}

	clr := reg
	clr.Glass = driver.GlassClear
	clr.Corners = driver.Corners{TopLeft: 100, TopRight: 100, BottomRight: 100, BottomLeft: 100}
	cu, ok := ComputeGlassUniforms(&clr, 100, 60, 1, true)
	if !ok {
		t.Fatal("clear glass returned !ok")
	}
	if cu.Blur != 1 || cu.P2 != [4]float32{0.125, 1.082, 1, 1} {
		t.Errorf("clear Blur=%v P2=%v", cu.Blur, cu.P2)
	}
	// Corners.Fit clamps 100px radii on a 100x60 box down to 30px each.
	for i, r := range cu.Radii {
		if math.Abs(float64(r-30)) > 1e-4 {
			t.Errorf("fitted Radii[%d] = %v, want ~30", i, r)
		}
	}
}

func TestGlassFallbackHelpers(t *testing.T) {
	none := driver.DefaultViewStyle
	if GlassFallbackBackdrop(&none) != "" || GlassFallbackTint(&none, false) != "" || GlassFallbackRim(&none, false) != "" {
		t.Fatal("GlassNone produced fallback CSS")
	}

	reg := driver.DefaultViewStyle
	reg.Glass = driver.GlassRegular
	if got := GlassFallbackBackdrop(&reg); got == "" {
		t.Error("bare GlassRegular needs fallback backdrop-filter")
	}
	if got := GlassFallbackTint(&reg, false); got == "" {
		t.Error("bare GlassRegular needs fallback light tint")
	}
	if got := GlassFallbackTint(&reg, true); got == "" {
		t.Error("bare GlassRegular needs fallback dark tint")
	}
	if got := GlassFallbackRim(&reg, false); got == "" {
		t.Error("GlassRegular needs fallback rim")
	}

	// Explicit BackdropFilters and Background take precedence.
	custom := reg
	custom.Background = driver.RGBA(0x112233, 0.5)
	custom.Effects = &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 24}}}
	if got := GlassFallbackBackdrop(&custom); got != "blur(24px)" {
		t.Errorf("explicit backdrop = %q, want blur(24px)", got)
	}
	if got := GlassFallbackTint(&custom, false); got != Fill(custom.Background) {
		t.Errorf("explicit tint = %q, want %q", got, Fill(custom.Background))
	}

	clr := driver.DefaultViewStyle
	clr.Glass = driver.GlassClear
	if got := GlassFallbackBackdrop(&clr); got == "" || got == GlassFallbackBackdrop(&reg) {
		t.Errorf("GlassClear fallback = %q", got)
	}
}

func TestGlassBoxTransform(t *testing.T) {
	vs := driver.DefaultViewStyle
	x, y, sw, sh, ok := GlassBoxTransform(10, 20, 100, 60, &vs)
	if !ok || x != 10 || y != 20 || sw != 100 || sh != 60 {
		t.Fatalf("identity = (%v,%v,%v,%v,%v)", x, y, sw, sh, ok)
	}

	vs.Transform = driver.Transform{TranslateX: 15, TranslateY: -5, ScaleX: 2, ScaleY: 0.5}
	x, y, sw, sh, ok = GlassBoxTransform(10, 20, 100, 60, &vs)
	// ox=50, oy=30 -> x = 10 + 15 + 50*(1-2) = -25; y = 20 - 5 + 30*(1-0.5) = 30
	if !ok || x != -25 || y != 30 || sw != 200 || sh != 30 {
		t.Fatalf("translate+scale = (%v,%v,%v,%v,%v)", x, y, sw, sh, ok)
	}

	for _, bad := range []driver.Transform{
		{Rotate: 15},
		{RotateZ: 10},
		{RotateX: 20},
		{SkewX: 5},
		{Perspective: 400},
		{Then: driver.Matrix3D{1}},
	} {
		vs.Transform = bad
		if _, _, _, _, ok := GlassBoxTransform(0, 0, 100, 60, &vs); ok {
			t.Errorf("expected non-upright transform %+v to return false", bad)
		}
	}
}

func TestGlassUnsafeAncestor(t *testing.T) {
	safe := driver.DefaultViewStyle
	if GlassUnsafeAncestor(&safe) {
		t.Error("default style marked unsafe")
	}

	blend := driver.DefaultViewStyle
	blend.BlendMode = driver.BlendMultiply
	if !GlassUnsafeAncestor(&blend) {
		t.Error("BlendMultiply should be unsafe")
	}

	masked := driver.DefaultViewStyle
	masked.Effects = &driver.Effects{Mask: &driver.GradientPaint{}}
	if !GlassUnsafeAncestor(&masked) {
		t.Error("Mask should be unsafe")
	}

	translucentBackdrop := driver.DefaultViewStyle
	translucentBackdrop.Opacity = 0.8
	translucentBackdrop.Effects = &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 4}}}
	if !GlassUnsafeAncestor(&translucentBackdrop) {
		t.Error("translucent backdrop should be unsafe")
	}

	nestedGlass := driver.DefaultViewStyle
	nestedGlass.Glass = driver.GlassRegular
	if !GlassUnsafeAncestor(&nestedGlass) {
		t.Error("nested glass should be unsafe")
	}
}
