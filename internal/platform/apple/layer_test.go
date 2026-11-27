//go:build darwin

package apple

import (
	"math"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// An animation's frames that move or fade a view set only its transform
// and opacity; any other change draws the whole style again.
func TestOnlyMovesSetsJustTransformAndOpacity(t *testing.T) {
	base := driver.ViewStyle{Background: driver.RGB(0x336699), CornerRadius: 8, Opacity: 1,
		Effects: &driver.Effects{Shadows: []driver.BoxShadow{{Blur: 4, Color: driver.RGB(0)}}}}
	moved := base
	moved.Transform = driver.Transform{TranslateX: 12, Rotate: 30}
	moved.Opacity = 0.5
	recolored := base
	recolored.Background = driver.RGB(0x993366)
	filtered := base
	filtered.Effects = &driver.Effects{Filters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 2}}}
	faded := filtered
	faded.Opacity = 0.5
	turned := filtered
	turned.Transform.Rotate = 10

	for _, c := range []struct {
		name string
		a, b driver.ViewStyle
		want bool
	}{
		{"moved and faded", base, moved, true},
		{"recolored", base, recolored, false},
		{"other effects", base, filtered, false},
		// iOS draws an opacity under filters with them.
		{"faded under filters", filtered, faded, false},
		{"turned under filters", filtered, turned, true},
	} {
		if got := onlyMoves(&c.a, &c.b); got != c.want {
			t.Errorf("%s: onlyMoves = %v, want %v", c.name, got, c.want)
		}
	}
}

// A transition's turn stays on the transform origin all the way, though
// the anchor point sits at the corner as AppKit keeps it: its steps hold
// the origin still and turn no further than Core Animation can tween. A
// new target goes on from the angle reached, not the last one drawn.
func TestTransformTransitionTurnsAroundTheOrigin(t *testing.T) {
	// The media clock stands still unless the test moves it, so how long the steps take to run does not shift the angles it checks.
	clock, now := caCurrentMediaTime, caCurrentMediaTime()
	caCurrentMediaTime = func() float64 { return now }
	defer func() { caCurrentMediaTime = clock }()
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	defer ForgetTransform(layer)
	layer.Send(sel("setAnchorPoint:"), CGPoint{})
	layer.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: 40, Height: 40}})
	transition := driver.Transition{Duration: time.Hour, Properties: driver.TransitionTransform}
	closed := driver.ViewStyle{Opacity: 1}
	open := driver.ViewStyle{Opacity: 1, Transform: driver.Transform{Rotate: 180}, Transition: transition}
	StyleLayer(layer, nil, &closed, 0)

	steps := func() []transform3D {
		values := layer.Send(sel("animationForKey:"), NSString("nsgo.transform")).Send(sel("values"))
		out := make([]transform3D, objc.Send[int](values, sel("count")))
		for i := range out {
			out[i] = objc.Send[transform3D](values.Send(sel("objectAtIndex:"), i), sel("CATransform3DValue"))
		}
		return out
	}
	angle := func(m transform3D) float64 { return math.Atan2(m.M12, m.M11) * 180 / math.Pi }
	StyleLayer(layer, &closed, &open, 0)
	got := steps()
	if len(got) < 3 {
		t.Fatalf("%d steps turn half a circle", len(got))
	}
	for i, m := range got {
		// (20, 20) is the origin, multiplied as CATransform3D's row vectors.
		if x, y := 20*m.M11+20*m.M21+m.M41, 20*m.M12+20*m.M22+m.M42; math.Abs(x-20) > 1e-6 || math.Abs(y-20) > 1e-6 {
			t.Errorf("step %d shows the origin at (%g, %g)", i, x, y)
		}
		if i > 0 {
			if d := math.Abs(math.Mod(angle(m)-angle(got[i-1])+540, 360) - 180); d > turnStep(math.Hypot(20, 20))+1e-6 {
				t.Errorf("step %d turns %g degrees", i, d)
			}
		}
	}
	if a := angle(got[len(got)-1]); math.Abs(math.Abs(a)-180) > 1e-6 {
		t.Errorf("the last step is at %g degrees", a)
	}

	// Halfway through its hour, the turn goes back.
	now += transition.Duration.Seconds() / 2
	back := closed
	back.Transition = transition
	StyleLayer(layer, &open, &back, 0)
	if a := angle(steps()[0]); math.Abs(a-90) > 1e-3 {
		t.Errorf("the turn back starts at %g degrees, want 90", a)
	}
}

func TestStyleLayerBoundRunsTransitionsOnUnboundProperties(t *testing.T) {
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	defer ForgetTransform(layer)
	defer ForgetDecor(layer)
	layer.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: 100, Height: 60}})

	animFor := func(k string) objc.ID {
		return layer.Send(sel("animationForKey:"), NSString("nsgo."+k))
	}

	tr := driver.Transition{Duration: time.Second, Easing: driver.EaseOut}
	// Translucent background stays on layer itself (no rim cover sublayer), so layer holds every transition key including backgroundColor.
	start := driver.ViewStyle{
		Opacity:      1,
		Background:   driver.RGBA(0x112233, 0.5),
		BorderWidth:  1,
		BorderColor:  driver.RGBA(0x222222, 0.5),
		CornerRadius: 4,
		ShadowColor:  driver.RGB(0x000000),
		ShadowRadius: 2,
		ShadowOffset: driver.Point{X: 1, Y: 1},
		Transform:    driver.Transform{TranslateX: 0},
		Transition:   tr,
	}
	RestyleLayer(layer, nil, &start, 0, 1<<driver.PropTranslateX)
	if animFor("opacity") != 0 || animFor("backgroundColor") != 0 {
		t.Fatal("initial style started a transition")
	}

	// With TranslateX bound, every unbound property transitions while transform does not.
	next := driver.ViewStyle{
		Opacity:      0.3,
		Background:   driver.RGBA(0x445566, 0.5),
		BorderWidth:  4,
		BorderColor:  driver.RGBA(0xabcdef, 0.5),
		CornerRadius: 16,
		ShadowColor:  driver.RGB(0x330000),
		ShadowRadius: 10,
		ShadowOffset: driver.Point{X: 4, Y: 6},
		Transform:    driver.Transform{TranslateX: 25},
		Transition:   tr,
	}
	RestyleLayer(layer, &start, &next, 0, 1<<driver.PropTranslateX)
	for _, k := range []string{"opacity", "backgroundColor", "borderWidth", "borderColor", "cornerRadius", "shadowColor", "shadowRadius", "shadowOffset"} {
		if animFor(k) == 0 {
			t.Errorf("unbound %q did not start a transition", k)
		}
	}
	if animFor("transform") != 0 {
		t.Error("bound transform started a transition")
	}

	// An animated frame changing only TranslateX (with Transition cleared) takes the onlyMoves fast path and leaves all running transitions intact.
	moved := next
	moved.Transition = driver.Transition{}
	moved.Transform.TranslateX = 40
	if !onlyMoves(&next, &moved) {
		t.Fatal("onlyMoves returned false when only Transform and Transition differed")
	}
	RestyleLayer(layer, &next, &moved, 0, 1<<driver.PropTranslateX)
	if animFor("opacity") == 0 || animFor("backgroundColor") == 0 {
		t.Error("animated transform frame removed unbound transitions")
	}

	// Binding Opacity cancels only nsgo.opacity, leaving nsgo.backgroundColor running.
	ClearBoundAnimations(layer, 1<<driver.PropOpacity)
	if animFor("opacity") != 0 {
		t.Error("ClearBoundAnimations left nsgo.opacity running")
	}
	if animFor("backgroundColor") == 0 {
		t.Error("ClearBoundAnimations removed unbound nsgo.backgroundColor")
	}

	// With BorderColor and ShadowColor bound, borderWidth/cornerRadius and shadowRadius/shadowOffset still transition.
	layer2 := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer2.Send(selRelease)
	defer ForgetTransform(layer2)
	defer ForgetDecor(layer2)
	layer2.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: 100, Height: 60}})
	bound2 := uint16(1<<driver.PropBorderColor | 1<<driver.PropShadowColor)
	RestyleLayer(layer2, nil, &start, 0, bound2)
	RestyleLayer(layer2, &start, &next, 0, bound2)
	anim2 := func(k string) objc.ID {
		return layer2.Send(sel("animationForKey:"), NSString("nsgo."+k))
	}
	for _, k := range []string{"opacity", "backgroundColor", "borderWidth", "cornerRadius", "shadowRadius", "shadowOffset"} {
		if anim2(k) == 0 {
			t.Errorf("partial group: unbound %q did not start a transition", k)
		}
	}
	for _, k := range []string{"borderColor", "shadowColor", "shadowOpacity"} {
		if anim2(k) != 0 {
			t.Errorf("partial group: bound %q started a transition", k)
		}
	}

	// Per-side borders: bound BorderColor suppresses fillColor transitions while keeping path transitions.
	layer3 := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer3.Send(selRelease)
	defer ForgetDecor(layer3)
	layer3.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: 100, Height: 60}})
	b1 := driver.ViewStyle{Opacity: 1, Borders: driver.Edges{Top: 1, Right: 1, Bottom: 1, Left: 1},
		BorderColors: driver.AllColors(driver.RGB(0x111111)), Transition: tr}
	b2 := driver.ViewStyle{Opacity: 1, Borders: driver.Edges{Top: 4, Right: 4, Bottom: 4, Left: 4},
		BorderColors: driver.AllColors(driver.RGB(0xeeeeee)), Transition: tr}
	RestyleLayer(layer3, nil, &b1, 0, 1<<driver.PropBorderColor)
	RestyleLayer(layer3, &b1, &b2, 0, 1<<driver.PropBorderColor)
	side := rings[layer3].sides[0]
	if side.Send(sel("animationForKey:"), NSString("nsgo.path")) == 0 {
		t.Error("per-side border path did not transition when only BorderColor was bound")
	}
	if side.Send(sel("animationForKey:"), NSString("nsgo.fillColor")) != 0 {
		t.Error("per-side border fillColor transitioned while BorderColor was bound")
	}

	// Rimmed cover layer: unbound background animates on d.cover and ClearBoundAnimations stops it.
	layer4 := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer4.Send(selRelease)
	defer ForgetDecor(layer4)
	layer4.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: 100, Height: 60}})
	r1 := driver.ViewStyle{Opacity: 1, Background: driver.RGB(0x112233), BorderWidth: 1, BorderColor: driver.RGB(0x222222), CornerRadius: 4, Transition: tr}
	r2 := r1
	r2.Background = driver.RGB(0x445566)
	RestyleLayer(layer4, nil, &r1, 0, 1<<driver.PropTranslateX)
	RestyleLayer(layer4, &r1, &r2, 0, 1<<driver.PropTranslateX)
	cover := decors[layer4].cover
	if cover == 0 || cover.Send(sel("animationForKey:"), NSString("nsgo.backgroundColor")) == 0 {
		t.Fatal("rimmed cover did not transition background when only TranslateX was bound")
	}
	ClearBoundAnimations(layer4, 1<<driver.PropBackground)
	if cover.Send(sel("animationForKey:"), NSString("nsgo.backgroundColor")) != 0 {
		t.Error("ClearBoundAnimations left rimmed cover background transition running")
	}
}

func TestSetTransformGeometryMatchesLayerTransform(t *testing.T) {
	readTransform := func(layer objc.ID) transform3D {
		return objc.Send[transform3D](layer, sel("transform"))
	}

	transforms := []struct {
		name string
		tr   driver.Transform
		pure bool
	}{
		{"identity", driver.Transform{}, true},
		{"translate", driver.Transform{TranslateX: 14, TranslateY: -22}, true},
		{"translate scale1", driver.Transform{TranslateX: 8, Scale: 1}, true},
		{"scale", driver.Transform{Scale: 1.35}, false},
		{"non-uniform scale", driver.Transform{ScaleX: 0.8, ScaleY: 1.4, TranslateX: 5, TranslateY: -3}, false},
		{"rotate", driver.Transform{Rotate: 37.5, TranslateX: -10, TranslateY: 12}, false},
		{"3d perspective", driver.Transform{Perspective: 600, RotateX: 25, RotateY: -40, RotateZ: 15, Scale: 1.1}, false},
		{"skew", driver.Transform{SkewX: 12, SkewY: -8}, false},
		{"then", driver.Transform{Rotate: 20, Then: driver.Translate3D(4, 6, -2)}, false},
	}

	anchors := []CGPoint{
		{X: 0, Y: 0},       // AppKit backing layers
		{X: 0.5, Y: 0.5},   // UIKit backing layers
		{X: 0.25, Y: 0.75}, // custom anchor
	}
	origins := []struct {
		offset driver.Point
		z      driver.Points
	}{
		{},
		{offset: driver.Point{X: -18, Y: 11}, z: 24},
	}

	for _, anchor := range anchors {
		for _, orig := range origins {
			layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
			defer layer.Send(selRelease)
			defer ForgetTransform(layer)
			defer ForgetDecor(layer)

			layer.Send(sel("setAnchorPoint:"), anchor)
			size := driver.Size{Width: 120, Height: 80}
			layer.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: float64(size.Width), Height: float64(size.Height)}})

			base := driver.ViewStyle{Opacity: 1, Origin: orig.offset, OriginZ: orig.z}
			StyleLayer(layer, nil, &base, 0)

			var geom TransformGeometry
			geom.SetSize(size)

			// First non-pure transform populates Anchor once without reading bounds.
			boundsBefore, anchorBefore := layerBoundsReads, layerAnchorReads
			first := driver.Transform{Rotate: 15}
			SetTransformGeometry(layer, first, &geom)
			if layerBoundsReads != boundsBefore {
				t.Fatalf("anchor=%v: first cached transform read bounds %d times, want 0", anchor, layerBoundsReads-boundsBefore)
			}
			if layerAnchorReads != anchorBefore+1 {
				t.Fatalf("anchor=%v: first cached transform read anchor %d times, want 1", anchor, layerAnchorReads-anchorBefore)
			}
			if !geom.AnchorKnown || geom.Anchor != anchor {
				t.Fatalf("geom.Anchor = %v (known=%v), want %v", geom.Anchor, geom.AnchorKnown, anchor)
			}

			// Every subsequent transform must match layerTransform exactly with zero ObjC reads.
			boundsBefore, anchorBefore = layerBoundsReads, layerAnchorReads
			for _, tc := range transforms {
				SetTransformGeometry(layer, tc.tr, &geom)
				got := readTransform(layer)
				if layerBoundsReads != boundsBefore || layerAnchorReads != anchorBefore {
					t.Fatalf("anchor=%v %s: cached SetTransformGeometry made ObjC reads (bounds=%d, anchor=%d)",
						anchor, tc.name, layerBoundsReads-boundsBefore, layerAnchorReads-anchorBefore)
				}
				want, _ := layerTransform(layer, tc.tr, orig.offset, float64(orig.z))
				// Reset expected baseline after reference layerTransform call.
				boundsBefore, anchorBefore = layerBoundsReads, layerAnchorReads
				if got != want {
					t.Errorf("anchor=%v orig=%+v %s:\n got  %+v\n want %+v", anchor, orig, tc.name, got, want)
				}
			}

			// Resizing updates geom.Size and stays exact with zero ObjC reads.
			resized := driver.Size{Width: 260, Height: 140}
			layer.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: float64(resized.Width), Height: float64(resized.Height)}})
			geom.SetSize(resized)
			boundsBefore, anchorBefore = layerBoundsReads, layerAnchorReads
			tr := driver.Transform{Perspective: 500, Rotate: 45, RotateY: 30, Scale: 1.25}
			SetTransformGeometry(layer, tr, &geom)
			if layerBoundsReads != boundsBefore || layerAnchorReads != anchorBefore {
				t.Fatalf("after resize: cached SetTransformGeometry made ObjC reads")
			}
			got := readTransform(layer)
			want, _ := layerTransform(layer, tr, orig.offset, float64(orig.z))
			if got != want {
				t.Errorf("after resize: got %+v, want %+v", got, want)
			}

			// ClearSize falls back to reading bounds while keeping cached anchor.
			resized2 := driver.Size{Width: 90, Height: 45}
			layer.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: float64(resized2.Width), Height: float64(resized2.Height)}})
			geom.ClearSize()
			boundsBefore, anchorBefore = layerBoundsReads, layerAnchorReads
			SetTransformGeometry(layer, tr, &geom)
			if layerBoundsReads != boundsBefore+1 || layerAnchorReads != anchorBefore {
				t.Fatalf("after ClearSize: reads = (bounds=%d, anchor=%d), want (1, 0)",
					layerBoundsReads-boundsBefore, layerAnchorReads-anchorBefore)
			}
			got = readTransform(layer)
			want, _ = layerTransform(layer, tr, orig.offset, float64(orig.z))
			if got != want {
				t.Errorf("after ClearSize: got %+v, want %+v", got, want)
			}
		}
	}

	// Pure translations never read bounds or anchor even without geometry.
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	defer ForgetTransform(layer)
	boundsBefore, anchorBefore := layerBoundsReads, layerAnchorReads
	SetTransform(layer, driver.Transform{TranslateX: 30, TranslateY: -12})
	SetTransform(layer, driver.Transform{})
	if layerBoundsReads != boundsBefore || layerAnchorReads != anchorBefore {
		t.Fatalf("pure translation read layer geometry (bounds=%d, anchor=%d)",
			layerBoundsReads-boundsBefore, layerAnchorReads-anchorBefore)
	}
}

func TestRestyleLayerGeometryFastPath(t *testing.T) {
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	defer ForgetTransform(layer)
	defer ForgetDecor(layer)
	layer.Send(sel("setAnchorPoint:"), CGPoint{})
	layer.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: 100, Height: 60}})

	var geom TransformGeometry
	geom.SetSize(driver.Size{Width: 100, Height: 60})

	start := driver.ViewStyle{Opacity: 1, Background: driver.RGB(0x224466), CornerRadius: 8}
	if full := RestyleLayerGeometry(layer, nil, &start, 0, 0, &geom); !full {
		t.Fatal("initial RestyleLayerGeometry reported fast path")
	}

	// Frame 1 primes Anchor on the first rotation.
	frame1 := start
	frame1.Opacity = 0.8
	frame1.Transform = driver.Transform{Rotate: 25, Scale: 1.1}
	if full := RestyleLayerGeometry(layer, &start, &frame1, 0, 1<<driver.PropRotate, &geom); full {
		t.Fatal("animated transform+opacity frame took full restyle path")
	}

	// Frame 2 must take fast path with zero bounds/anchor reads.
	frame2 := frame1
	frame2.Opacity = 0.6
	frame2.Transform = driver.Transform{Rotate: 50, Scale: 1.2}
	boundsBefore, anchorBefore := layerBoundsReads, layerAnchorReads
	if full := RestyleLayerGeometry(layer, &frame1, &frame2, 0, 1<<driver.PropRotate, &geom); full {
		t.Fatal("second animated frame took full restyle path")
	}
	if layerBoundsReads != boundsBefore || layerAnchorReads != anchorBefore {
		t.Fatalf("steady-state animated frame made ObjC reads (bounds=%d, anchor=%d)",
			layerBoundsReads-boundsBefore, layerAnchorReads-anchorBefore)
	}
	if got := objc.Send[float32](layer, sel("opacity")); math.Abs(float64(got-0.6)) > 1e-5 {
		t.Errorf("opacity = %v, want 0.6", got)
	}
	want, _ := layerTransform(layer, frame2.Transform, driver.Point{}, 0)
	if got := objc.Send[transform3D](layer, sel("transform")); got != want {
		t.Errorf("transform = %+v, want %+v", got, want)
	}

	// Changing opaque background without rim takes the fast recolor path (updates cover or layer directly, zero bounds/anchor reads).
	recolored := frame2
	recolored.Background = driver.RGB(0x664422)
	boundsBefore, anchorBefore = layerBoundsReads, layerAnchorReads
	if full := RestyleLayerGeometry(layer, &frame2, &recolored, 0, 1<<driver.PropBackground, &geom); full {
		t.Fatal("opaque-to-opaque recolor took full restyle path")
	}
	if layerBoundsReads != boundsBefore || layerAnchorReads != anchorBefore {
		t.Fatalf("fast recolor made ObjC reads (bounds=%d, anchor=%d)",
			layerBoundsReads-boundsBefore, layerAnchorReads-anchorBefore)
	}

	// Crossing opaque to translucent takes the full restyle path.
	translucent := recolored
	translucent.Background = driver.RGBA(0x664422, 0.5)
	if full := RestyleLayerGeometry(layer, &recolored, &translucent, 0, 1<<driver.PropBackground, &geom); !full {
		t.Fatal("opaque-to-translucent recolor reported fast path")
	}

	// Changing corner radius takes the full restyle path.
	rounded := translucent
	rounded.CornerRadius = 16
	if full := RestyleLayerGeometry(layer, &translucent, &rounded, 0, 0, &geom); !full {
		t.Fatal("corner radius change reported fast path")
	}

	// Opaque background with outer shadows updates its cover sublayer on the fast path without rebuilding shadows.
	shadowed := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer shadowed.Send(selRelease)
	defer ForgetTransform(shadowed)
	defer ForgetDecor(shadowed)
	shadowed.Send(sel("setAnchorPoint:"), CGPoint{})
	shadowed.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: 52, Height: 52}})
	var sgeom TransformGeometry
	sgeom.SetSize(driver.Size{Width: 52, Height: 52})
	s1 := driver.ViewStyle{
		Opacity:      1,
		Background:   driver.RGB(0x2266cc),
		CornerRadius: 26,
		Effects:      &driver.Effects{Shadows: []driver.BoxShadow{{Offset: driver.Point{Y: 4}, Blur: 10, Color: driver.RGBA(0x2266cc, 0.35)}}},
	}
	if full := RestyleLayerGeometry(shadowed, nil, &s1, 0, 0, &sgeom); !full {
		t.Fatal("initial shadowed RestyleLayerGeometry reported fast path")
	}
	cover := decors[shadowed].cover
	if cover == 0 {
		t.Fatal("opaque shadowed view did not create cover sublayer")
	}
	s2 := s1
	s2.Background = driver.RGB(0x22aa66)
	s2.Transform = driver.Transform{Scale: 1.05}
	// Prime anchor once on first scale.
	if full := RestyleLayerGeometry(shadowed, &s1, &s2, 0, 1<<driver.PropBackground|1<<driver.PropScale, &sgeom); full {
		t.Fatal("opaque shadowed background+scale animation took full restyle path")
	}
	s3 := s2
	s3.Background = driver.RGB(0x33bb77)
	s3.Transform = driver.Transform{Scale: 1.08}
	boundsBefore, anchorBefore = layerBoundsReads, layerAnchorReads
	if full := RestyleLayerGeometry(shadowed, &s2, &s3, 0, 1<<driver.PropBackground|1<<driver.PropScale, &sgeom); full {
		t.Fatal("second opaque shadowed background+scale animation took full restyle path")
	}
	if layerBoundsReads != boundsBefore || layerAnchorReads != anchorBefore {
		t.Fatalf("shadowed fast path made ObjC reads (bounds=%d, anchor=%d)",
			layerBoundsReads-boundsBefore, layerAnchorReads-anchorBefore)
	}
	if decors[shadowed].cover != cover || decors[shadowed].covers != s3.Background {
		t.Fatalf("cover sublayer not updated in place: cover=%v covers=%#x", decors[shadowed].cover, decors[shadowed].covers)
	}
}

func TestSameClipStyle(t *testing.T) {
	base := driver.ViewStyle{
		Opacity:           1,
		Background:        driver.RGB(0x112233),
		CornerRadius:      12,
		ContinuousCorners: true,
		BorderWidth:       2,
		ChildPerspective:  800,
	}
	moved := base
	moved.Opacity = 0.4
	moved.Transform = driver.Transform{TranslateX: 20, Rotate: 45, Scale: 1.2}
	if !SameClipStyle(&base, &moved) {
		t.Error("SameClipStyle = false when only Opacity and Transform changed")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*driver.ViewStyle)
	}{
		{"CornerRadius", func(s *driver.ViewStyle) { s.CornerRadius = 6 }},
		{"Corners", func(s *driver.ViewStyle) { s.Corners.TopLeft = 4 }},
		{"ContinuousCorners", func(s *driver.ViewStyle) { s.ContinuousCorners = false }},
		{"BorderWidth", func(s *driver.ViewStyle) { s.BorderWidth = 3 }},
		{"Borders", func(s *driver.ViewStyle) { s.Borders.Top = 1 }},
		{"ChildPerspective", func(s *driver.ViewStyle) { s.ChildPerspective = 400 }},
	} {
		changed := base
		tc.mutate(&changed)
		if SameClipStyle(&base, &changed) {
			t.Errorf("SameClipStyle = true after changing %s", tc.name)
		}
	}
}

func BenchmarkSetTransform(b *testing.B) {
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	defer ForgetTransform(layer)
	layer.Send(sel("setAnchorPoint:"), CGPoint{})
	layer.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: 100, Height: 60}})

	tr := driver.Transform{TranslateX: 12, TranslateY: -8, Rotate: 35, Scale: 1.15}

	b.Run("uncached", func(b *testing.B) {
		WithoutAnimation(func() {
			SetTransform(layer, tr)
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				tr.Rotate = float32(i % 360)
				SetTransform(layer, tr)
			}
		})
	})

	b.Run("cached", func(b *testing.B) {
		var geom TransformGeometry
		geom.SetSize(driver.Size{Width: 100, Height: 60})
		WithoutAnimation(func() {
			SetTransformGeometry(layer, tr, &geom)
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				tr.Rotate = float32(i % 360)
				SetTransformGeometry(layer, tr, &geom)
			}
		})
	})
}

func BenchmarkRestyleLayerAnimated(b *testing.B) {
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	defer ForgetTransform(layer)
	defer ForgetDecor(layer)
	layer.Send(sel("setAnchorPoint:"), CGPoint{})
	layer.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: 100, Height: 60}})

	base := driver.ViewStyle{Opacity: 1, Background: driver.RGB(0x336699), CornerRadius: 12}
	var geom TransformGeometry
	geom.SetSize(driver.Size{Width: 100, Height: 60})
	RestyleLayerGeometry(layer, nil, &base, 0, 0, &geom)
	shown := base
	shown.Transform = driver.Transform{Rotate: 1, Scale: 1.05}
	RestyleLayerGeometry(layer, &base, &shown, 0, 1<<driver.PropRotate, &geom)

	b.Run("uncached", func(b *testing.B) {
		prev := shown
		WithoutAnimation(func() {
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				next := prev
				next.Transform.Rotate = float32((i + 2) % 360)
				RestyleLayer(layer, &prev, &next, 0, 1<<driver.PropRotate)
				prev = next
			}
		})
	})

	b.Run("cached", func(b *testing.B) {
		prev := shown
		WithoutAnimation(func() {
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				next := prev
				next.Transform.Rotate = float32((i + 2) % 360)
				RestyleLayerGeometry(layer, &prev, &next, 0, 1<<driver.PropRotate, &geom)
				prev = next
			}
		})
	})
}
