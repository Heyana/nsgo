//go:build darwin

package apple

import (
	"runtime"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestBackdropTintTransitionsAndAnimatedBackgrounds(t *testing.T) {
	// BackdropTint styles inside WithoutAnimation's CATransaction. When that transaction is the thread's outermost one, its commit ends at once the animations it added to a layer outside any window, as this one is, so the transitions the test reads would be gone or not depending on whether an earlier test left an implicit transaction open on the thread the test runs on. An outer transaction, held on one thread as transactions are per thread, keeps them on the layer as a window's layer keeps them.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	transaction := objc.ID(Class("CATransaction"))
	transaction.Send(sel("begin"))
	defer transaction.Send(sel("commit"))

	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	defer ForgetTransform(layer)

	animation := func() objc.ID {
		return layer.Send(sel("animationForKey:"), NSString("nsgo.backgroundColor"))
	}
	assertColor := func(got objc.ID, want driver.Color) {
		t.Helper()
		expected := CGColor(want)
		defer Release(expected)
		if got == 0 || !objc.Send[bool](got, sel("isEqual:"), expected) {
			t.Errorf("tint color %v does not match %#x", got, want)
		}
	}

	var tint BackdropTint
	style := driver.ViewStyle{
		Opacity:    1,
		Background: driver.RGBA(0, 0.2),
		Transition: driver.Transition{
			Duration:   time.Second,
			Delay:      100 * time.Millisecond,
			Properties: driver.TransitionBackground,
			Easing:     driver.Curve{X1: 0.2, Y1: 0.3, X2: 0.4, Y2: 0.5},
		},
	}

	// Initial style paints immediately with no transition.
	tint.Style(layer, &style, false)
	if animation() != 0 {
		t.Error("initial style starts a transition")
	}
	assertColor(layer.Send(sel("backgroundColor")), style.Background)

	// Background change starts a Core Animation transition from the previous color.
	from := style.Background
	style.Background = driver.RGBA(0x224466, 0.8)
	tint.Style(layer, &style, false)
	anim := animation()
	if anim == 0 {
		t.Fatal("a background change starts no tint transition")
	}
	assertColor(anim.Send(sel("fromValue")), from)
	if got := objc.Send[float64](anim, sel("duration")); got != 1 {
		t.Errorf("duration = %v, want 1 second", got)
	}
	if objc.Send[float64](anim, sel("beginTime")) == 0 || GoString(anim.Send(sel("fillMode"))) != "backwards" {
		t.Error("the tint transition lost its delay")
	}

	// An unrelated animated property frame (restyle with empty Transition, same Background) leaves the running tint transition intact.
	before := objc.Send[float64](anim, sel("beginTime"))
	unrelated := style
	unrelated.Opacity = 0.5
	unrelated.Transition = driver.Transition{}
	tint.Style(layer, &unrelated, false)
	if animation() == 0 || objc.Send[float64](animation(), sel("beginTime")) != before {
		t.Error("an unrelated animated property replaced or removed the tint transition")
	}

	// An animated background binding cancels an active transition even when its color already matches the model style's target color.
	sameTarget := style
	sameTarget.Transition = driver.Transition{}
	tint.Style(layer, &sameTarget, true)
	if animation() != 0 {
		t.Error("an animated background matching the target left the transition running")
	}
	assertColor(layer.Send(sel("backgroundColor")), style.Background)

	// Start another transition, then override with animated background frames.
	style.Background = driver.RGB(0x112233)
	tint.Style(layer, &style, false)
	if animation() == 0 {
		t.Fatal("second background change did not start a transition")
	}
	bound := style
	bound.Transition = driver.Transition{}
	bound.Background = driver.RGB(0xff0000)
	tint.Style(layer, &bound, true)
	if animation() != 0 {
		t.Error("an animated background left the style transition running")
	}
	assertColor(layer.Send(sel("backgroundColor")), bound.Background)

	bound.Background = driver.RGB(0x00ff00)
	tint.Style(layer, &bound, true)
	if animation() != 0 {
		t.Error("a subsequent animated background frame started a transition")
	}
	assertColor(layer.Send(sel("backgroundColor")), bound.Background)

	// Unbinding returns immediately to the model background.
	unbound := style
	unbound.Transition = driver.Transition{}
	tint.Style(layer, &unbound, false)
	if animation() != 0 {
		t.Error("unbinding animated background started a transition")
	}
	assertColor(layer.Send(sel("backgroundColor")), style.Background)

	// A transition excluding background updates immediately without animating.
	style.Transition.Properties = driver.TransitionOpacity
	style.Background = driver.RGB(0x0000ff)
	tint.Style(layer, &style, false)
	if animation() != 0 {
		t.Error("a transition excluding background animates the tint")
	}
	assertColor(layer.Send(sel("backgroundColor")), style.Background)

	// Default transition (Properties == 0) includes background.
	style.Transition.Properties = 0
	style.Background = driver.RGB(0xffffff)
	tint.Style(layer, &style, false)
	if animation() == 0 {
		t.Error("the default transition excludes the tint")
	}

	// Clearing background without a transition removes any active animation and clears the layer.
	style.Transition = driver.Transition{}
	style.Background = 0
	tint.Style(layer, &style, false)
	if animation() != 0 || layer.Send(sel("backgroundColor")) != 0 {
		t.Error("clearing background without a transition left a tint or animation")
	}

	// Resetting (after backdrop removal/recreation) prevents an initial transition.
	tint.Reset()
	style.Background = driver.RGB(0x884422)
	style.Transition = driver.Transition{Duration: time.Second}
	tint.Style(layer, &style, false)
	if animation() != 0 {
		t.Error("a recreated tint started a transition")
	}
	assertColor(layer.Send(sel("backgroundColor")), style.Background)
}

func TestBackdropBackgroundHandoffAndClearOuterAnimation(t *testing.T) {
	outer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer outer.Send(selRelease)
	defer ForgetTransform(outer)

	tintLayer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer tintLayer.Send(selRelease)
	defer ForgetTransform(tintLayer)

	assertColor := func(got objc.ID, want driver.Color) {
		t.Helper()
		expected := CGColor(want)
		defer Release(expected)
		if got == 0 || !objc.Send[bool](got, sel("isEqual:"), expected) {
			t.Errorf("layer color %v does not match %#x", got, want)
		}
	}

	// Outer layer starts a background transition; entering blur mode cancels it so it cannot paint behind the blur.
	initial := driver.ViewStyle{Opacity: 1, Background: driver.RGB(0x112233)}
	StyleLayer(outer, nil, &initial, 0)
	next := driver.ViewStyle{
		Opacity:    1,
		Background: driver.RGB(0x445566),
		Transition: driver.Transition{Duration: time.Second, Properties: driver.TransitionBackground},
	}
	StyleLayer(outer, &initial, &next, 0)
	if outer.Send(sel("animationForKey:"), NSString("nsgo.backgroundColor")) == 0 {
		t.Fatal("setup: outer layer did not start a background transition")
	}
	ClearBackgroundAnimation(outer)
	if outer.Send(sel("animationForKey:"), NSString("nsgo.backgroundColor")) != 0 {
		t.Fatal("ClearBackgroundAnimation left an active outer background transition")
	}

	// Paint the tint layer and keep outer transparent while blur is active.
	var tint BackdropTint
	tintColor := driver.RGBA(0x336699, 0.75)
	tintStyle := driver.ViewStyle{Opacity: 1, Background: tintColor}
	tint.Style(tintLayer, &tintStyle, false)
	outerCleared := driver.ViewStyle{Opacity: 1}
	StyleLayer(outer, &next, &outerCleared, 0)
	if outer.Send(sel("backgroundColor")) != 0 {
		t.Fatal("outer layer kept a background while blur was active")
	}

	// Removing the blur with the same background and a transition hands off the tint's color so outer does not flash from transparent to tintColor.
	HandoffBackground(tintLayer, outer)
	assertColor(outer.Send(sel("backgroundColor")), tintColor)
	sameWithTransition := driver.ViewStyle{
		Opacity:    1,
		Background: tintColor,
		Transition: driver.Transition{Duration: time.Second, Properties: driver.TransitionBackground},
	}
	StyleLayer(outer, &outerCleared, &sameWithTransition, 0)
	if outer.Send(sel("animationForKey:"), NSString("nsgo.backgroundColor")) != 0 {
		t.Error("removing blur with unchanged background started a spurious transition from clear")
	}

	// Removing the blur while also changing background transitions from the tint's color.
	StyleLayer(outer, &sameWithTransition, &outerCleared, 0)
	HandoffBackground(tintLayer, outer)
	changedWithTransition := driver.ViewStyle{
		Opacity:    1,
		Background: driver.RGB(0xaa2244),
		Transition: driver.Transition{Duration: time.Second, Properties: driver.TransitionBackground},
	}
	StyleLayer(outer, &outerCleared, &changedWithTransition, 0)
	anim := outer.Send(sel("animationForKey:"), NSString("nsgo.backgroundColor"))
	if anim == 0 {
		t.Fatal("removing blur with a new background did not transition outer layer")
	}
	assertColor(anim.Send(sel("fromValue")), tintColor)
	assertColor(outer.Send(sel("backgroundColor")), changedWithTransition.Background)
}
