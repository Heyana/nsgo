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
