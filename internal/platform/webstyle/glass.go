package webstyle

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// GlassUniforms holds the shader parameters for Liquid Glass, matching Windows' glass shader (internal/platform/windows/glass.go) and MyGo's glass material measurements of macOS 27's NSGlassEffectView.
type GlassUniforms struct {
	Rect  [4]float32 // x, y, width, height in output pixels
	Radii [4]float32 // fitted corner radii in pixels: top-left, top-right, bottom-right, bottom-left
	P0    [4]float32 // bezel, refraction, rim, rim width (pixels / factors)
	P1    [4]float32 // straight RGBA tint
	P2    [4]float32 // low, high, curve, saturation
	P3    [4]float32 // light direction (x, y, 0, 0)
	Blur  float32    // backdrop blur sigma in CSS points
}

// ComputeGlassUniforms computes the Liquid Glass shader parameters for a box of w by h CSS points at device pixel scale. It returns false when vs has no glass effect or the box has no area.
func ComputeGlassUniforms(vs *driver.ViewStyle, w, h, scale float32, dark bool) (GlassUniforms, bool) {
	if vs == nil || vs.Glass == driver.GlassNone || w <= 0 || h <= 0 {
		return GlassUniforms{}, false
	}
	if scale <= 0 {
		scale = 1
	}
	pw := float32(math.Ceil(float64(w * scale)))
	ph := float32(math.Ceil(float64(h * scale)))
	if pw <= 0 || ph <= 0 {
		return GlassUniforms{}, false
	}
	corners := vs.Corners
	if corners == (driver.Corners{}) && vs.CornerRadius > 0 {
		corners = driver.AllCorners(vs.CornerRadius)
	}
	corners = corners.Fit(driver.Size{Width: driver.Points(w), Height: driver.Points(h)})
	m := min(w, h)
	bezel := min(36, m/2)
	u := GlassUniforms{
		Rect: [4]float32{0, 0, pw, ph},
		Radii: [4]float32{
			float32(corners.TopLeft) * scale,
			float32(corners.TopRight) * scale,
			float32(corners.BottomRight) * scale,
			float32(corners.BottomLeft) * scale,
		},
		P0: [4]float32{bezel * scale, 1.6 * bezel * scale, 0.35, 1},
		P3: [4]float32{0, 1, 0, 0},
	}
	if vs.Background != 0 {
		r, g, b, a := vs.Background.Components()
		u.P1 = [4]float32{float32(r), float32(g), float32(b), float32(a)}
	}
	// Larger panes blur more, matching Windows' glass.go.
	blur := min(max(m*0.035, 1), 10)
	switch {
	case vs.Glass == driver.GlassClear:
		blur = 1
		u.P2 = [4]float32{0.125, 1.082, 1, 1}
	case dark:
		u.P2 = [4]float32{0.15, 0.51, 2, 2.2}
	default:
		u.P2 = [4]float32{0.541, 1, 1.2, 1}
	}
	u.Blur = blur
	return u, true
}

// GlassFallbackBackdrop returns the CSS backdrop-filter for a glass view when the glass shader is unavailable: the style's own BackdropFilters when set, else a material blur matching GlassEffect.
func GlassFallbackBackdrop(vs *driver.ViewStyle) string {
	if vs == nil {
		return ""
	}
	if vs.Effects != nil {
		if f := Filter(vs.Effects.BackdropFilters); f != "" {
			return f
		}
	}
	switch vs.Glass {
	case driver.GlassClear:
		return "blur(2px) saturate(1.2)"
	case driver.GlassRegular:
		return "blur(16px) saturate(1.8)"
	default:
		return ""
	}
}

// GlassFallbackTint returns the CSS background-color for a glass view falling back to CSS: the view's own Background when set, else a subtle translucent tint matching GlassEffect and appearance.
func GlassFallbackTint(vs *driver.ViewStyle, dark bool) string {
	if vs == nil {
		return ""
	}
	if vs.Background != 0 {
		return Fill(vs.Background)
	}
	switch vs.Glass {
	case driver.GlassClear:
		if dark {
			return "rgba(255,255,255,0.06)"
		}
		return "rgba(255,255,255,0.08)"
	case driver.GlassRegular:
		if dark {
			return "rgba(30,30,34,0.45)"
		}
		return "rgba(255,255,255,0.45)"
	default:
		return ""
	}
}

// GlassFallbackRim returns a subtle inset specular rim box-shadow for a glass view falling back to CSS, or "" when vs has no glass effect.
func GlassFallbackRim(vs *driver.ViewStyle, dark bool) string {
	if vs == nil || vs.Glass == driver.GlassNone {
		return ""
	}
	if dark {
		return "inset 0 0 0 1px rgba(255,255,255,0.18)"
	}
	return "inset 0 0 0 1px rgba(255,255,255,0.45)"
}

// GlassBoxTransform checks whether vs's transform keeps the glass box upright (axis-aligned, flat, no rotation/skew/perspective/matrix) and returns its transformed CSS bounds (x, y, sw, sh) from layout origin (lx, ly) and layout size (w, h).
func GlassBoxTransform(lx, ly, w, h float64, vs *driver.ViewStyle) (x, y, sw, sh float64, ok bool) {
	if w <= 0 || h <= 0 {
		return 0, 0, 0, 0, false
	}
	if vs == nil {
		return lx, ly, w, h, true
	}
	t := vs.Transform
	if t.Rotate != 0 || t.RotateZ != 0 || !t.Flat() || t.SkewX != 0 || t.SkewY != 0 ||
		t.Perspective != 0 || t.Then != (driver.Matrix3D{}) {
		return 0, 0, 0, 0, false
	}
	sx, sy := t.Scales()
	if sx <= 0 || sy <= 0 {
		return 0, 0, 0, 0, false
	}
	ox := w*0.5 + float64(vs.Origin.X)
	oy := h*0.5 + float64(vs.Origin.Y)
	x = lx + float64(t.TranslateX) + ox*(1-float64(sx))
	y = ly + float64(t.TranslateY) + oy*(1-float64(sy))
	sw = w * float64(sx)
	sh = h * float64(sy)
	return x, y, sw, sh, true
}

// GlassUnsafeAncestor reports whether an ancestor view with style vs makes placing a nested HTML-in-Canvas drawable inside it unsafe: Chrome hangs when a nested drawable sits under blend/mask/translucent backdrop layers, and nesting glass inside glass risks recursive capture.
func GlassUnsafeAncestor(vs *driver.ViewStyle) bool {
	if vs == nil {
		return false
	}
	if vs.Glass != driver.GlassNone || vs.BlendMode != driver.BlendNormal {
		return true
	}
	if e := vs.Effects; e != nil {
		if e.Mask != nil {
			return true
		}
		if vs.Opacity < 1 && len(e.BackdropFilters) > 0 {
			return true
		}
	}
	return false
}
