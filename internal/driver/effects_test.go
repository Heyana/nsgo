package driver

import (
	"math"
	"testing"
	"time"
)

func segmentEnd(s Segment) [2]float64 {
	if s.Kind == SegmentCubic {
		return s.P[2]
	}
	return s.P[0]
}

func near(a, b [2]float64) bool { return math.Abs(a[0]-b[0]) < 1e-4 && math.Abs(a[1]-b[1]) < 1e-4 }

func TestCircularCornersAreQuarterCircles(t *testing.T) {
	v := RoundedRect(nil, 0, 0, 100, 60, AllCorners(10), false, false)
	// Move, then per corner a line and one arc, then close.
	if len(v) != 1+1+3*2+1 {
		t.Fatalf("%d segments", len(v))
	}
	if !near(segmentEnd(v[0]), [2]float64{90, 0}) || !near(segmentEnd(v[1]), [2]float64{100, 10}) {
		t.Fatalf("first corner = %v %v", v[0], v[1])
	}
	// The standard quarter circle's handles are 0.5523 r long.
	if c := v[1].P[0]; math.Abs(c[0]-(90+5.5228475)) > 1e-4 || math.Abs(c[1]) > 1e-6 {
		t.Errorf("handle = %v", c)
	}
}

func TestContinuousCornersSpreadAlongTheEdges(t *testing.T) {
	v := RoundedRect(nil, 0, 0, 100, 60, AllCorners(10), true, false)
	// The curve starts 1.6 r before the corner and ends 1.6 r after it.
	if !near(segmentEnd(v[0]), [2]float64{84, 0}) || !near(segmentEnd(v[3]), [2]float64{100, 16}) {
		t.Fatalf("corner from %v to %v", segmentEnd(v[0]), segmentEnd(v[3]))
	}
	// Its first handle lies on the edge, so the edge flows into it.
	if v[1].P[0][1] != 0 || v[3].P[1][0] != 100 {
		t.Errorf("handles leave the edges: %v, %v", v[1].P, v[3].P)
	}
	// A pill has no room to spread: its ends stay circular.
	pill := RoundedRect(nil, 0, 0, 100, 60, AllCorners(30), true, false)
	circle := RoundedRect(nil, 0, 0, 100, 60, AllCorners(30), false, false)
	if len(pill) != len(circle) {
		t.Fatalf("pill has %d segments, circle %d", len(pill), len(circle))
	}
	for i := range pill {
		for j := range 3 {
			if !near(pill[i].P[j], circle[i].P[j]) {
				t.Fatalf("segment %d differs: %v, %v", i, pill[i], circle[i])
			}
		}
	}
}

func TestReversedOutlinesAndFittedCorners(t *testing.T) {
	v := RoundedRect(nil, 0, 0, 100, 60, Corners{TopLeft: 10}, false, true)
	want := [][2]float64{{10, 0}, {0, 10}, {0, 60}, {100, 60}, {100, 0}}
	for i, w := range want {
		if !near(segmentEnd(v[i]), w) {
			t.Fatalf("segment %d ends at %v, want %v", i, segmentEnd(v[i]), w)
		}
	}
	if c := (Corners{TopLeft: 80, TopRight: 80}).Fit(Size{Width: 100, Height: 200}); c.TopLeft != 50 || c.TopRight != 50 {
		t.Errorf("fit = %+v, want both halved to the width", c)
	}
}

func TestLinearGradientLineReachesTheCorners(t *testing.T) {
	g := Gradient{Angle: 90, Stops: []ColorStop{{Color: RGB(0xff0000)}, {Color: RGB(0x0000ff)}}, Space: ColorSpaceSRGB}
	p, ok := g.Paint(Size{Width: 200, Height: 100})
	if !ok || p.X0 != 0 || p.X1 != 1 || p.Y0 != 0.5 || p.Y1 != 0.5 {
		t.Fatalf("to the right: %+v", p)
	}
	if len(p.Stops) != 2 || p.Stops[0] != (GradientStop{0, RGB(0xff0000)}) || p.Stops[1] != (GradientStop{1, RGB(0x0000ff)}) {
		t.Errorf("stops = %v", p.Stops)
	}
	// 45 degrees on a square: the line runs corner to corner.
	g.Angle = 45
	p, _ = g.Paint(Size{Width: 100, Height: 100})
	if math.Abs(float64(p.X0)) > 1e-5 || math.Abs(float64(p.Y0-1)) > 1e-5 || math.Abs(float64(p.X1-1)) > 1e-5 {
		t.Errorf("45 degrees: %+v", p)
	}
	// To a corner, the line is perpendicular to the other diagonal.
	g.ToCorner = ToTopRight
	p, _ = g.Paint(Size{Width: 200, Height: 100})
	dx, dy := float64(p.X1-p.X0)*200, float64(p.Y1-p.Y0)*100
	if dot := dx*200 + dy*100; math.Abs(dot) > 1e-2 {
		t.Errorf("to top right is not perpendicular to the diagonal: %v, %v", dx, dy)
	}
}

func TestStopsResolveAsCSS(t *testing.T) {
	red, green, blue := RGB(0xff0000), RGB(0x00ff00), RGB(0x0000ff)
	g := Gradient{Angle: 180, Space: ColorSpaceSRGB, Stops: []ColorStop{
		{Color: red}, {Color: green, At: Pt(50)}, {Color: blue, At: Pct(25)}}}
	p, _ := g.Paint(Size{Width: 100, Height: 100})
	// 50 points of a 100 point line is half; the blue stop before it
	// moves up to it.
	if got := []float32{p.Stops[0].Offset, p.Stops[1].Offset, p.Stops[2].Offset}; got[1] != 0.5 || got[2] != 0.5 {
		t.Errorf("offsets = %v", got)
	}
	// Repeating stripes lay out their copies over the whole line.
	g = Gradient{Angle: 180, Repeating: true, Space: ColorSpaceSRGB, Stops: []ColorStop{
		{Color: red, At: Pct(0)}, {Color: red, At: Pct(10)}, {Color: blue, At: Pct(10)}, {Color: blue, At: Pct(20)}}}
	p, _ = g.Paint(Size{Width: 100, Height: 100})
	if n := len(p.Stops); n < 20 || p.Stops[n-1].Offset != 1 {
		t.Errorf("%d stops, last at %v", n, p.Stops[n-1].Offset)
	}
	// Radial stops past the ending shape grow it.
	g = Gradient{Kind: GradientRadial, Circle: true, Extent: ExtentClosestSide, Space: ColorSpaceSRGB,
		Stops: []ColorStop{{Color: red}, {Color: blue, At: Pct(200)}}}
	p, _ = g.Paint(Size{Width: 100, Height: 100})
	// Its 200% reaches past the box, so it is cut at the corners, where
	// the color is halfway between.
	if math.Abs(float64(p.RX)-math.Sqrt2/2) > 1e-5 || p.Stops[1].Offset != 1 || p.Stops[1].Color == RGB(0x0000ff) {
		t.Errorf("radial = %+v", p)
	}
}

func TestOklabMixesWithoutGrey(t *testing.T) {
	blue, yellow := RGB(0x0000ff), RGB(0xffff00)
	srgb := MixColors(blue, yellow, 0.5, ColorSpaceSRGB)
	if srgb != RGB(0x808080) {
		t.Fatalf("sRGB middle = %08x", srgb)
	}
	lab := MixColors(blue, yellow, 0.5, ColorSpaceOklab)
	r, g, b, _ := lab.Components()
	if spread := max(r, g, b) - min(r, g, b); spread < 0.05 {
		t.Errorf("Oklab middle %08x is grey", lab)
	}
	// Sampled stops follow the curve.
	p, _ := (&Gradient{Angle: 90, Stops: []ColorStop{{Color: blue}, {Color: yellow}}}).Paint(Size{Width: 10, Height: 10})
	if len(p.Stops) < 4 {
		t.Errorf("Oklab gradient sampled into %d stops", len(p.Stops))
	}
	// A transparent end takes the other's color, premultiplied.
	if c := MixColors(0, RGB(0xff0000), 0.5, ColorSpaceSRGB); c != RGBA(0xff0000, 0.5) {
		t.Errorf("transparent to red = %08x", c)
	}
}

func TestColorMatrices(t *testing.T) {
	apply := func(m ColorMatrix, r, g, b float32) [3]float32 {
		return [3]float32{m[0]*r + m[1]*g + m[2]*b + m[4], m[5]*r + m[6]*g + m[7]*b + m[9], m[10]*r + m[11]*g + m[12]*b + m[14]}
	}
	m, _ := Filter{Kind: FilterGrayscale, Amount: 1}.Matrix()
	if c := apply(m, 1, 0, 0); math.Abs(float64(c[0]-0.2126)) > 1e-4 || c[0] != c[1] || c[1] != c[2] {
		t.Errorf("grey red = %v", c)
	}
	m, _ = Filter{Kind: FilterInvert, Amount: 1}.Matrix()
	if c := apply(m, 1, 0.25, 0); c != [3]float32{0, 0.75, 1} {
		t.Errorf("inverted = %v", c)
	}
	m, ok, blur, shadows := SplitFilters([]Filter{{Kind: FilterBrightness, Amount: 2}, {Kind: FilterBlur, Amount: 4},
		{Kind: FilterBrightness, Amount: 0.25}, {Kind: FilterDropShadow, Shadow: BoxShadow{Blur: 3}}})
	if !ok || blur != 4 || len(shadows) != 1 || math.Abs(float64(m[0]-0.5)) > 1e-6 {
		t.Errorf("split = %v %v %v %v", m, ok, blur, shadows)
	}
}

func TestKeyframesRunTheirCycles(t *testing.T) {
	spin := KeyframeAnimation{Keyframes: KeyframesSpin}
	if k, ok := spin.At(250 * time.Millisecond); !ok || math.Abs(float64(k.Rotate-90)) > 1e-3 {
		t.Errorf("spin at a quarter = %+v", k)
	}
	if k, _ := spin.At(10*time.Second + 500*time.Millisecond); math.Abs(float64(k.Rotate-180)) > 1e-3 {
		t.Errorf("spin repeats: %+v", k)
	}
	ping := KeyframeAnimation{Keyframes: KeyframesPing, Iterations: 1}
	if k, ok := ping.At(900 * time.Millisecond); !ok || k.Opacity != 0 || k.Scale != 2 {
		t.Errorf("ping past three quarters = %+v", k)
	}
	if _, ok := ping.At(2 * time.Second); ok {
		t.Error("ping runs past its one iteration")
	}
	bounce := KeyframeAnimation{Keyframes: KeyframesBounce}
	if k, _ := bounce.At(0); k.TranslateY != -0.25 {
		t.Errorf("bounce starts at %+v", k)
	}
}

func TestTransformMatrices(t *testing.T) {
	m := Transform{Rotate: 90}.Matrix(Point{X: 50, Y: 50})
	if x, y := m.Apply(100, 50); math.Abs(x-50) > 1e-9 || math.Abs(y-100) > 1e-9 {
		t.Errorf("rotated (100, 50) = %v, %v; want (50, 100), clockwise around the center", x, y)
	}
	if _, _, _, _, _, _, ok := m.Affine(); !ok {
		t.Error("a rotation in the plane is not affine")
	}
	tilt := Transform{RotateY: 30, Perspective: 400}.Matrix(Point{X: 50, Y: 50})
	if _, _, _, _, _, _, ok := tilt.Affine(); ok {
		t.Error("a rotation out of the plane with perspective is affine")
	}
	flat := Transform{Rotate: 30, ScaleX: 2, TranslateX: 5}.Matrix(Point{X: 50, Y: 50})
	inv, ok := flat.Invert()
	if x, y := inv.Apply(flat.Apply(10, 20)); !ok || math.Abs(x-10) > 1e-6 || math.Abs(y-20) > 1e-6 {
		t.Errorf("inverse maps back to %v, %v", x, y)
	}
	skew := Transform{SkewX: 45}.Matrix(Point{})
	if x, _ := skew.Apply(0, 10); math.Abs(x-10) > 1e-9 {
		t.Errorf("skewed x = %v", x)
	}
}

func TestMixStylesFadeThroughTheVisibleColor(t *testing.T) {
	// A transparent start takes the target's hue, so it does not pass
	// through black.
	got := MixStyles(ViewStyle{Background: RGBA(0xff0000, 0)}, ViewStyle{Background: RGBA(0x0000ff, 1)}, 0.5,
		TransitionBackground).Background
	if got != RGBA(0x0000ff, 0.5) {
		t.Fatalf("mix = %08x", got)
	}
	shadow := func(blur Points) *Effects { return &Effects{Shadows: []BoxShadow{{Blur: blur, Color: RGB(0)}}} }
	mid := MixStyles(ViewStyle{Effects: shadow(0)}, ViewStyle{Effects: shadow(10)}, 0.5, TransitionShadow)
	if mid.Effects.Shadows[0].Blur != 5 {
		t.Errorf("box shadow blur = %v halfway", mid.Effects.Shadows[0].Blur)
	}
}

// TestMixStylesScalesFromUnsetAsOne moves a view that sets no scale to one
// that does, as a button's pressed style: the transition shrinks it from
// its shown size, not from nothing.
func TestMixStylesScalesFromUnsetAsOne(t *testing.T) {
	from, to := ViewStyle{}, ViewStyle{Transform: Transform{Scale: 0.97, ScaleX: 0.5}}
	mid := MixStyles(from, to, 0.5, TransitionTransform)
	if sx, sy := mid.Transform.Scales(); sx != 0.75 || sy != 0.985 {
		t.Fatalf("halfway scales = %v, %v; want 0.75, 0.985", sx, sy)
	}
	// And one that stops setting a scale grows back to its shown size.
	mid = MixStyles(to, ViewStyle{}, 0.5, TransitionTransform)
	if sx, sy := mid.Transform.Scales(); sx != 0.75 || sy != 0.985 {
		t.Fatalf("halfway scales back = %v, %v; want 0.75, 0.985", sx, sy)
	}
	// Two styles that set none keep none.
	if got := MixStyles(from, from, 0.5, TransitionTransform).Transform; got.Scale != 0 || got.ScaleX != 0 || got.ScaleY != 0 {
		t.Fatalf("unset scales = %+v", got)
	}
}
