//go:build darwin && !ios

package macos

import (
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// The surface's frame clock steps the native animation graph, the layout
// animations and the scroll views once a frame while one runs.

var (
	selTick      = sel("nsgoTick:")
	selWatch     = sel("nsgoWatch:")
	selStallTick = sel("nsgoStallTick:")
)

const (
	// frameBatches is how long a frame applies batches for while something
	// animates, at least one whole batch, as NSH's FRAME_BATCHES: a
	// scroll's frames are not delayed by batches landing between them,
	// and a burst of batches spreads over frames.
	frameBatches = 4 * time.Millisecond
	// stallAfter is how long the display link may go without a frame
	// while frames are wanted before a 60 Hz timer ticks in its place: the
	// link stops while the window is occluded or minimized or the screen
	// is locked, and animations must still end.
	stallAfter = 100 * time.Millisecond
)

// clock returns the frame clock's time.
func (s *surface) clock() time.Duration {
	if s.epoch.IsZero() {
		s.epoch = time.Now()
	}
	return time.Since(s.epoch)
}

// frameRateRange preserves the rate native interactions need and applies
// the app's preference when its callbacks are the only frame work.
func (s *surface) frameRateRange() apple.CAFrameRateRange {
	if s.pace > 0 {
		rate := float32(s.pace)
		return apple.CAFrameRateRange{Minimum: rate, Maximum: rate, Preferred: rate}
	}
	maximum := float32(60)
	screen := s.window.Send(sel("screen"))
	if screen == 0 {
		screen = objc.ID(apple.Class("NSScreen")).Send(sel("mainScreen"))
	}
	if screen != 0 {
		if n := objc.Send[int](screen, sel("maximumFramesPerSecond")); n >= 1 {
			maximum = float32(n)
		}
	}
	if s.preferredRate.Preferred > 0 && !s.animating() && s.selecting == nil && s.perf == nil {
		return s.preferredRate.Clamped(maximum)
	}
	return apple.CAFrameRateRange{Minimum: min(maximum, 80), Maximum: maximum, Preferred: maximum}
}

func (s *surface) applyFrameRate() {
	if s.link == 0 {
		return
	}
	rate := s.frameRateRange()
	if rate != s.appliedRate {
		s.link.Send(sel("setPreferredFrameRateRange:"), rate)
		s.appliedRate = rate
	}
}

// startTicking runs the frame clock until nothing animates: AppKit's
// display link where there is one (macOS 14), else a timer at the usual
// 60 Hz, both in the run loop's common modes so they keep running while a
// menu tracks or a window resizes.
func (s *surface) startTicking() {
	defer s.publishPace()
	if s.ticking {
		s.setPace()
		return
	}
	s.ticking = true
	s.pace = s.wantedPace()
	if s.link != 0 {
		// The window may have moved to another screen meanwhile.
		s.applyFrameRate()
		s.link.Send(sel("setPaused:"), false)
		s.watch()
		return
	}
	if objc.Send[bool](s.native, sel("respondsToSelector:"), sel("displayLinkWithTarget:selector:")) {
		// A link needs the view in a window: without one, the next mount
		// asks again.
		link := s.native.Send(sel("displayLinkWithTarget:selector:"), s.native, selTick)
		if link == 0 {
			s.ticking = false
			return
		}
		s.link = link.Send(sel("retain"))
		s.appliedRate = apple.CAFrameRateRange{}
		s.applyFrameRate()
		s.link.Send(sel("addToRunLoop:forMode:"), mainLoop(), apple.CommonModes())
		s.watch()
		return
	}
	s.timer = repeating(1.0/60, s.native, selTick)
}

// wantedPace is the rate the frame clock need run at: the animations' own
// when they are all it steps, else the screen's highest.
func (s *surface) wantedPace() float64 {
	if s.panelRepickAfter != 0 || s.panelMotion.Active() || len(s.motions) > 0 || len(s.transitions) > 0 || len(s.ghosts) > 0 || len(s.live) > 0 || s.frameWanted.Load() || s.selecting != nil || s.perf != nil {
		return 0
	}
	return s.animator.FrameRate()
}

// setPace moves the running link to the rate wanted now. The timers keep
// theirs: the animator holds a slowed run to its rate on any clock.
func (s *surface) setPace() {
	pace := s.wantedPace()
	if pace != s.pace || s.preferredRate.Preferred > 0 {
		s.pace = pace
		s.applyFrameRate()
	}
	s.publishPace()
	// The app asked for a frame as the clock slowed, seeing it still at
	// the full rate.
	if pace != 0 && s.frameWanted.Load() {
		s.setPace()
	}
}

// publishPace tells the engine goroutine whether the clock ticks at the
// display's full rate, so that a frame the app asks for needs no hop to
// the main thread (RequestFrame).
func (s *surface) publishPace() { s.fullRate.Store(s.ticking && s.pace == 0) }

// repeating returns a retained timer that sends action to target every
// interval seconds in the main run loop's common modes.
func repeating(interval float64, target objc.ID, action objc.SEL) objc.ID {
	t := objc.ID(apple.Class("NSTimer")).Send(sel("timerWithTimeInterval:target:selector:userInfo:repeats:"),
		interval, target, action, objc.ID(0), true).Send(sel("retain"))
	mainLoop().Send(sel("addTimer:forMode:"), t, apple.CommonModes())
	return t
}

func mainLoop() objc.ID { return objc.ID(apple.Class("NSRunLoop")).Send(sel("mainRunLoop")) }

func invalidate(t *objc.ID) {
	if *t != 0 {
		t.Send(sel("invalidate"))
		t.Send(selRelease)
		*t = 0
	}
}

// watch starts the timer that checks, while the link runs, that it ticks.
func (s *surface) watch() {
	s.ticked = time.Now()
	if s.watcher == 0 {
		s.watcher = repeating(stallAfter.Seconds(), s.native, selWatch)
	}
}

func (s *surface) stopTicking() {
	if !s.ticking {
		return
	}
	s.ticking = false
	s.publishPace()
	invalidate(&s.watcher)
	invalidate(&s.stall)
	if s.link != 0 {
		s.link.Send(sel("setPaused:"), true)
		return
	}
	invalidate(&s.timer)
}

func surfaceTick(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		s := v.surface
		// The link is back.
		invalidate(&s.stall)
		var period time.Duration
		if s.perf != nil {
			period = s.framePeriod()
		}
		s.tick(period, true)
	}
}

// surfaceWatch ticks with a timer once the display link stalled.
func surfaceWatch(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		if s := v.surface; s.ticking && s.stall == 0 && time.Since(s.ticked) >= stallAfter {
			s.stall = repeating(1.0/60, s.native, selStallTick)
		}
	}
}

func surfaceStallTick(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.surface.tick(time.Second/60, false)
	}
}

// framePeriod is the display link's period, or 60 Hz where there is none.
func (s *surface) framePeriod() time.Duration {
	if s.link != 0 {
		stamp := objc.Send[float64](s.link, selTimestamp)
		target := objc.Send[float64](s.link, selTargetTimestamp)
		if target > stamp {
			return time.Duration((target - stamp) * float64(time.Second))
		}
	}
	return time.Second / 60
}

// tick steps one frame: the native animations first, then the frame the app waits for, at the time the display link's frame shows where linked says the link ticks, and at the engine's where a timer stands in for it.
func (s *surface) tick(period time.Duration, linked bool) {
	defer s.reportGeometry()
	last := s.ticked
	s.ticked = time.Now()
	dirty := s.panelMotion.Active() || s.animator.Running() || len(s.motions) > 0 || len(s.transitions) > 0 || len(s.ghosts) > 0 || len(s.images) > 0 || len(s.live) > 0
	var start time.Time
	if s.perf != nil {
		start = time.Now()
	}
	// A scroll reported from here to the frame below rides with that frame
	// (reportScroll).
	s.stepping = true
	s.pickPresentedPanel(linked)
	// The batches that waited for this frame, before it steps.
	if s.held() {
		s.applyBatches(frameBatches, false)
		dirty = true
	}
	s.stepPanelFrame(s.ticked)
	if len(s.live) > 0 {
		s.tickScrolls(mediaTime())
	}
	selecting := s.autoscrollSelection(min(max(s.ticked.Sub(last), time.Second/240), time.Second/30))
	now := s.clock()
	if s.animator.Running() {
		s.animator.Step(now)
	}
	apple.WithoutAnimation(func() {
		s.stepMotions(now)
		s.stepTransitions(now)
		s.applyAnimated()
	})
	if dirty || selecting {
		s.staleEffects()
	}
	s.drawShaders()
	s.startImages()
	if len(s.motions) > 0 {
		s.revisit()
	}
	if s.frameWanted.Swap(false) && s.engine != nil {
		var at time.Time
		if linked && s.link != 0 {
			at = apple.LinkFrameTime(s.link)
		}
		s.engine.FrameAt(at)
	}
	s.stepping = false
	if s.perf != nil {
		s.perf.RecordWork(time.Since(start), dirty)
		s.perf.Vsync(time.Now(), period)
	}
	apple.YieldFrame()
	if !s.animating() && !s.frameWanted.Load() && !s.held() && !selecting && s.perf == nil {
		s.stopTicking()
		// The app asked for a frame as the clock stopped, seeing it still
		// ticking.
		if s.frameWanted.Load() {
			s.startTicking()
		}
		return
	}
	s.setPace()
}

// animating reports whether something moves on the frame clock: the
// animation graph, a layout animation, or a scroll view moving or fading
// its indicator. Batches wait for the start of a frame meanwhile.
func (s *surface) animating() bool {
	return s.panelRepickAfter != 0 || s.panelMotion.Active() || s.animator.Running() || len(s.motions) > 0 || len(s.transitions) > 0 || len(s.ghosts) > 0 || len(s.live) > 0 || s.geometryAnimating()
}

// held reports whether batches wait for a frame.
func (s *surface) held() bool { return len(s.waiting) > 0 || s.batches.Len() > 0 }

// applyAnimated draws the animated values that changed.
func (s *surface) applyAnimated() {
	apple.ApplyAnimated(s.animator, func(id driver.NodeID) *view {
		if int(id) < len(s.views) {
			return s.views[id]
		}
		return nil
	}, func(v *view) *driver.AnimatedValues {
		return &v.animated
	}, func(v *view) {
		// A transform transition stepped beside bound transform values ends once none is bound.
		if v.transformTr != nil && v.animated.Set&driver.AnimatedTransformMask == 0 {
			v.transformTr = nil
			delete(s.transitions, v)
		}
		v.restyle()
	}, (*view).redraw, (*view).redrawAnimated, func(v *view) driver.NativeComponent {
		return v.component
	}, s.repinBound)
}

// updateTransformTransition starts, keeps or clears v's frame-stepped transform transition when a transform property is bound alongside an unbound transform change: Core Animation's single transform layer property cannot run a transition and an animated value at once.
func (v *view) updateTransformTransition(prev driver.ViewStyle) {
	s := v.surface
	styled := v.styled || v.born != s.batch
	bound := v.animated.Set
	if m := s.motions[v]; m != nil && m.Appear != nil && m.Appear.Property == driver.LayoutScale {
		bound |= 1 << driver.PropScale
	}
	if !styled || bound&driver.AnimatedTransformMask == 0 || s.motionReduced() {
		v.transformTr = nil
		delete(s.transitions, v)
		return
	}
	v.transformTr = driver.StartStyleTransition(v.transformTr, prev, v.style, s.clock(), driver.TransitionTransform, v.animated)
	if v.transformTr != nil {
		if s.transitions == nil {
			s.transitions = map[*view]bool{}
		}
		s.transitions[v] = true
		s.startTicking()
	} else {
		delete(s.transitions, v)
	}
}

// stepTransitions advances active frame-stepped transform transitions.
func (s *surface) stepTransitions(now time.Duration) {
	for v := range s.transitions {
		if int(v.id) >= len(s.views) || s.views[v.id] != v || v.transformTr == nil {
			delete(s.transitions, v)
			continue
		}
		v.restyleAt(false, now)
	}
}

// restyle draws v's style with its animated values and what a layout animation does to it, without starting new transitions.
func (v *view) restyle() { v.restyleWith(false) }

// restyleWith draws v's style with its animated values and layout appearance animation. When transition is true (from ItemViewStyle), unbound properties run v.style.Transition.
func (v *view) restyleWith(transition bool) { v.restyleAt(transition, v.surface.clock()) }

func (v *view) restyleAt(transition bool, now time.Duration) {
	base := v.style
	if v.transformTr != nil {
		var done bool
		base, done = v.transformTr.At(now)
		if done {
			v.transformTr = nil
			delete(v.surface.transitions, v)
		}
	}
	bound := v.animated.Set
	vs := base
	if bound != 0 {
		vs = v.animated.Apply(base)
	}
	if !transition {
		vs.Transition = driver.Transition{}
	}
	if m := v.surface.motions[v]; m != nil && m.Appear != nil {
		p, _ := m.Appear.At(now - m.AppearStart)
		if m.Appear.Property == driver.LayoutScale {
			sx, sy := vs.Transform.Scales()
			vs.Transform.ScaleX, vs.Transform.ScaleY = sx*float32(p), sy*float32(p)
			bound |= 1 << driver.PropScale
		} else {
			vs.Opacity *= float32(p)
			bound |= 1 << driver.PropOpacity
		}
	}
	v.styleLayer(&vs, bound)
}

// styleLayer draws vs with v's layer, only what changed since it was
// last drawn when that is all an animation moves.
func (v *view) styleLayer(vs *driver.ViewStyle, bound uint16) {
	v.styleTint(vs)
	if v.holdClip(vs) {
		// The holder clips the children: the view's own layer, its shadow and outline, stays unclipped.
		drawn := *vs
		drawn.Clip = false
		vs = &drawn
	}
	if blursByRadius(vs, drawsGlass(vs)) && vs.Background != 0 {
		// The tint draws the background over the blur, which would blur the layer's own and show it twice.
		drawn := *vs
		drawn.Background = 0
		vs = &drawn
	}
	v.followScale(vs)
	// A view created by an earlier batch shows the default style.
	var shown *driver.ViewStyle
	if v.styled || v.born != v.surface.batch {
		shown = &v.drawn
	}
	layer := v.native.Send(selLayer)
	if added := bound &^ v.bound; added != 0 {
		apple.ClearBoundAnimations(layer, added)
	}
	v.bound = bound
	apple.RestyleLayer(layer, shown, vs, v.native, bound)
	// RestyleLayer may create per-side rings after setBackdrop ran; move them above the backdrop now that they exist.
	apple.MoveBorders(layer, v.borderHost())
	v.drawn, v.styled = *vs, true
	v.transform = vs.Transform
	if vs.Transform != (driver.Transform{}) || vs.Animation.Keyframes != 0 {
		v.transformed = true
	}
}

// layoutHost binds the surface's views, placement and clock to the layout animations apple shares with UIKit and AppKit.
func (s *surface) layoutHost() apple.LayoutHost[*view] {
	return apple.LayoutHost[*view]{
		Motions: s.motions, Ghosts: &s.ghosts,
		ID:     func(v *view) driver.NodeID { return v.id },
		Native: func(v *view) objc.ID { return v.native },
		Framed: func(v *view) bool { return v.framed },
		Frame:  func(v *view) driver.Bounds { return v.frame },
		Shift:  func(v *view) driver.Point { return v.shift },
		Place:  (*view).place, Restyle: (*view).restyle,
		Now: s.clock, Tick: s.startTicking,
		Start: func(*driver.LayoutTransition) time.Duration { return s.clock() },
	}
}

// layoutBatch is what a batch with a layout animation tracks.
type layoutBatch = apple.LayoutBatch

// setFrame moves v to the item's frame, animating from where it shows now when the batch animates layout or the item carries a transition.
func (s *surface) setFrame(v *view, it *driver.MountItem, lb *layoutBatch) {
	s.layoutHost().SetFrame(v, it, lb)
}

// place sets v's frame and what follows it.
func (v *view) place(f driver.Bounds) {
	resized := f.Size != v.frame.Size
	v.frame, v.framed = f, true
	f.Origin.X += v.shift.X
	f.Origin.Y += v.shift.Y
	v.native.Send(selSetFrame, apple.Rect(float64(f.Origin.X), float64(f.Origin.Y),
		float64(f.Size.Width), float64(f.Size.Height)))
	if v.transform != (driver.Transform{}) {
		apple.SetTransform(v.native.Send(selLayer), v.transform)
	}
	apple.LayoutDecor(v.native.Send(selLayer))
	if v.backdrop != 0 && !v.glass {
		v.fitBackdrop(&v.style)
	}
	if resized {
		v.sizeContent()
		if v.sc != nil {
			v.sizeScroll()
		}
		if (v.clipHeld || v.sc == nil) && v.content != v.native {
			// As last drawn, animated corners and all: only the view's own layer leaves the clip out. The perspective's centre follows the size too.
			held := v.drawn
			held.Clip = v.clipHeld
			v.fitHolder(&held)
		}
	}
	if v.image != nil {
		v.image.Layout(apple.CGSize{Width: float64(f.Size.Width), Height: float64(f.Size.Height)})
		v.surface.images = append(v.surface.images, v)
	}
}

// appear starts v appearing, as the batch that created it animates.
func (s *surface) appear(v *view, lb *layoutBatch, parent driver.NodeID) {
	s.layoutHost().Appear(v, lb, parent)
}

// match moves v, which the batch created, from where old, which it deletes, shows in v's parent, on the batch's Update rather than appearing, and hides old at once (driver.ItemMatch).
func (s *surface) match(v, old *view, lb *layoutBatch) {
	s.layoutHost().Match(v, old, lb)
}

// vanish keeps a deleted view's native in place, fading or shrinking it away, unless an ancestor the batch deleted already does. It reports whether it kept the view.
func (s *surface) vanish(v *view, lb *layoutBatch) bool {
	return s.layoutHost().Vanish(v, lb)
}

// inGhost reports whether native is inside a view going away, which keeps its subviews until it goes.
func (s *surface) inGhost(native objc.ID) bool {
	return apple.InLayoutGhost(s.ghosts, native)
}

// stepMotions moves the layout animations to now.
func (s *surface) stepMotions(now time.Duration) {
	s.ghosts = apple.StepLayoutMotions(now, s.motions, s.ghosts, func(v *view) bool {
		return s.views[v.id] == v
	}, (*view).place, (*view).restyle)
}

// releaseGhosts lets go of the views still fading out as the surface
// closes, as the ticks that would end them have stopped.
func (s *surface) releaseGhosts() {
	for _, g := range s.ghosts {
		g.Release()
	}
	clear(s.ghosts)
	s.ghosts = nil
}
