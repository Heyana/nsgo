//go:build darwin && !ios

package macos

import (
	"math"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Closing a surface whose frame clock ran lets go of its view: the display link, which the run loop holds, holds the view as its target.
func TestClosingASurfaceReleasesItsDisplayLink(t *testing.T) {
	onMain(func() {
		loadAppKit()
		drained := func(fn func()) {
			pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
			fn()
			pool.Send(sel("drain"))
		}
		var window objc.ID
		var s *surface
		drained(func() {
			window = objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
				apple.Rect(0, 0, 200, 100), uint(0), uint(2), false)
			window.Send(sel("setReleasedWhenClosed:"), false)
			s = newSurface(Options{Width: 200, Height: 100}, false)
			s.window = window
			window.Send(sel("setContentView:"), s.native)
		})
		defer window.Send(selRelease)
		native := s.native
		defer func() {
			delete(views, native)
			native.Send(selRelease)
		}()
		retained := func() uint { return uint(native.Send(sel("retainCount"))) }
		start := retained()
		drained(s.startTicking)
		if s.link == 0 {
			t.Skip("no display link before macOS 14")
		}
		if retained() <= start {
			t.Fatal("the display link does not hold its target")
		}
		drained(s.close)
		if s.link != 0 {
			t.Error("a closed surface keeps its display link")
		}
		if got := retained(); got != start {
			t.Errorf("a closed surface's view keeps retain count %d, want %d", got, start)
		}
	})
}

// Closing a surface lets go of the views still fading out of it, as the ticks that would end their fades stop with it.
func TestClosingASurfaceReleasesItsGhosts(t *testing.T) {
	onMain(func() {
		loadAppKit()
		drained := func(fn func()) {
			pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
			fn()
			pool.Send(sel("drain"))
		}
		var s *surface
		var leaving objc.ID
		drained(func() {
			s = newSurface(Options{Width: 200, Height: 100}, false)
			leaving = objc.ID(apple.Class("NSView")).Send(selNew)
			s.native.Send(selAddSubview, leaving)
		})
		native := s.native
		defer func() {
			delete(views, native)
			native.Send(selRelease)
		}()
		defer leaving.Send(selRelease)
		retained := func() uint { return uint(leaving.Send(sel("retainCount"))) }
		// The test's reference and the surface view's.
		start := retained()
		// What vanish does as a layout animation deletes the view.
		leaving.Send(sel("retain"))
		s.ghosts = append(s.ghosts, &ghost{native: leaving, opacity: 1, tr: &driver.LayoutTransition{Curve: driver.LayoutLinear, Duration: time.Hour}})
		drained(s.close)
		if len(s.ghosts) != 0 {
			t.Error("a closed surface keeps the views fading out of it")
		}
		if leaving.Send(selSuperview) != 0 {
			t.Error("a closed surface leaves a view fading out in place")
		}
		// Only the test's reference is left.
		if got := retained(); got != start-1 {
			t.Errorf("a view fading out of a closed surface keeps retain count %d, want %d", got, start-1)
		}
	})
}

func TestTransitionsRunBesideAnimatedValues(t *testing.T) {
	onMain(func() {
		loadAppKit()
		s := &surface{native: objc.ID(classView).Send(selNew), animator: driver.NewAnimator(nil), epoch: time.Now()}
		defer s.native.Send(selRelease)

		tr := driver.Transition{Duration: time.Second, Easing: driver.EaseOut}
		style := driver.ViewStyle{
			Opacity:      1,
			Background:   driver.RGBA(0x112233, 0.8),
			BorderWidth:  1,
			CornerRadius: 4,
			Material:     driver.BackdropThin,
			Effects:      &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 8}}},
			Transform:    driver.Transform{Scale: 1},
			Transition:   tr,
		}
		// Batch 1: create view, initial style, frame + bind TranslateX to animated node 10.
		s.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
			{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
			{Kind: driver.ItemFrame, ID: 1, Frame: driver.Bounds{Size: driver.Size{Width: 120, Height: 80}}},
			{Kind: driver.ItemViewStyle, ID: 1, ViewStyle: &style},
			{Kind: driver.ItemAnimation, Animation: []driver.AnimationOp{
				{Kind: driver.AnimCreateValue, Node: 10, Value: 15},
				{Kind: driver.AnimBind, View: 1, Prop: driver.PropTranslateX, Node: 10},
			}},
		}})
		v := s.views[1]
		defer s.delete(v, false)
		layer := v.native.Send(selLayer)
		animFor := func(k string) objc.ID {
			return layer.Send(sel("animationForKey:"), apple.NSString("nsgo."+k))
		}
		if animFor("opacity") != 0 || animFor("backgroundColor") != 0 || animFor("transform") != 0 {
			t.Fatal("initial batch started transitions")
		}
		if v.drawn.Transform.TranslateX != 15 {
			t.Fatalf("initial animated TranslateX = %v, want 15", v.drawn.Transform.TranslateX)
		}

		// Batch 2: change unbound Opacity, Background, BorderWidth, CornerRadius and unbound Transform.Scale while TranslateX stays bound.
		style.Opacity = 0.25
		style.Background = driver.RGBA(0x446688, 0.9)
		style.BorderWidth = 3
		style.CornerRadius = 12
		style.Transform.Scale = 0.5
		s.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
			{Kind: driver.ItemViewStyle, ID: 1, ViewStyle: &style},
		}})
		for _, k := range []string{"opacity", "borderWidth", "cornerRadius"} {
			if animFor(k) == 0 {
				t.Errorf("unbound %q did not start a transition on macOS", k)
			}
		}
		if animFor("transform") != 0 {
			t.Error("bound transform started a Core Animation transition")
		}
		if v.tint == 0 || v.tint.Send(selLayer).Send(sel("animationForKey:"), apple.NSString("nsgo.backgroundColor")) == 0 {
			t.Error("backdrop tint did not start a background transition beside animated TranslateX")
		}
		if v.transformTr == nil || !s.transitions[v] {
			t.Fatal("unbound Scale change beside bound TranslateX did not start a frame-stepped transform transition")
		}

		// Advance TranslateX via the animator and step halfway through the Scale transition.
		s.animator.Apply([]driver.AnimationOp{{Kind: driver.AnimSet, Node: 10, Value: 42}})
		apple.WithoutAnimation(func() {
			s.applyAnimated()
			s.stepTransitions(s.clock() + 500*time.Millisecond)
		})
		if animFor("opacity") == 0 || animFor("borderWidth") == 0 {
			t.Error("animated value frame cancelled unbound Core Animation transitions")
		}
		if v.drawn.Transform.TranslateX != 42 {
			t.Errorf("TranslateX = %v, want 42", v.drawn.Transform.TranslateX)
		}
		if sc := v.drawn.Transform.Scale; sc <= 0.5 || sc >= 1 || math.IsNaN(float64(sc)) {
			t.Errorf("mid-transition Scale = %v, want strictly between 0.5 and 1", sc)
		}

		// Binding Opacity mid-transition cancels nsgo.opacity while leaving nsgo.borderWidth running.
		s.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
			{Kind: driver.ItemAnimation, Animation: []driver.AnimationOp{
				{Kind: driver.AnimCreateValue, Node: 11, Value: 0.7},
				{Kind: driver.AnimBind, View: 1, Prop: driver.PropOpacity, Node: 11},
			}},
		}})
		if animFor("opacity") != 0 {
			t.Error("newly bound Opacity left nsgo.opacity running")
		}
		if animFor("borderWidth") == 0 {
			t.Error("binding Opacity cancelled unrelated nsgo.borderWidth transition")
		}
		if math.Abs(float64(v.drawn.Opacity-0.7)) > 1e-4 {
			t.Errorf("bound Opacity = %v, want 0.7", v.drawn.Opacity)
		}
	})
}
