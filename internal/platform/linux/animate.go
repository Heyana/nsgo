//go:build linux && !android

package linux

import (
	"runtime"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/scrollbar"
)

const (
	// stallMillis is how often a running clock checks that the frame clock ticks, and how long it goes without a tick before the check ticks in its place.
	stallMillis    = 100
	scrollDuration = 300 * time.Millisecond

	allProperties = driver.TransitionOpacity | driver.TransitionBackground | driver.TransitionTransform |
		driver.TransitionBorder | driver.TransitionShadow
)

// transition is a view's style animating from one ViewStyle to the next.
type transition struct {
	from, to driver.ViewStyle
	start    time.Time // after the delay
	duration time.Duration
	curve    driver.Curve
	props    driver.TransitionProperty
	shown    driver.ViewStyle
}

// styleOf returns the style v shows now: mid transition, or its own,
// with the values the animator binds over it.
func (w *window) styleOf(v *view) *driver.ViewStyle { return v.shownStyle() }

func (v *view) shownStyle() *driver.ViewStyle {
	s := &v.style
	if v.transition != nil {
		s = &v.transition.shown
	}
	if v.animated != nil {
		out := v.animated.Apply(*s)
		return &out
	}
	return s
}

// setStyle applies v's new style, animating to it when it has a
// transition.
func (w *window) setStyle(v *view, s *driver.ViewStyle) {
	from := *w.styleOf(v)
	// The first style shows at once, unless the view was created by an
	// earlier batch: then it changes from the default style.
	styled := v.styled || v.born != w.batch
	if s.Animation != v.style.Animation || !styled {
		v.keyframeStart = time.Now()
		if s.Animation.Keyframes != 0 {
			w.animate(v)
		}
	}
	v.style, v.styled, v.transition = *s, true, nil
	w.setCursor(v)
	t := s.Transition
	if !styled || t.Duration <= 0 || w.motionReduced() {
		return
	}
	props := t.Properties
	if props == 0 {
		props = allProperties
	}
	first, last := driver.MixStyles(from, *s, 0, props), driver.MixStyles(from, *s, 1, props)
	if first == last {
		return
	}
	v.transition = &transition{from: from, to: *s, start: time.Now().Add(t.Delay), duration: t.Duration,
		curve: t.Easing, props: props, shown: first}
	w.animate(v)
}

// scrollAnimated scrolls v to offset over a moment, or at once when the
// user turned animation effects off.
func (w *window) scrollAnimated(v *view, offset driver.Point) {
	if w.motionReduced() {
		w.scrollTo(v, offset)
		return
	}
	v.scrolling = &scrollbar.OffsetAnimation{From: v.offset, To: offset, Start: time.Now()}
	w.animate(v)
}

// animate ticks v's animations each frame until they end.
func (w *window) animate(v *view) {
	for _, a := range w.animating {
		if a == v {
			return
		}
	}
	w.animating = append(w.animating, v)
	w.startClock()
}

// startClock ticks every frame until nothing animates: no transition or scroll, no animator run, no layout animation, no animated image and no frame the app waits for. It ticks from the surface's GdkFrameClock, at the display's rate, and while that clock stands still, as it does for a window not shown, a timer checking it every stallMillis ticks in its place, so animations still end and the app's frames still come.
func (w *window) startClock() {
	if !w.ticking && !w.closing {
		w.tickID = gtkWidgetAddTickCallback(w.surface, cbTick, w.handle, 0)
		w.watchID = gTimeoutAdd(stallMillis, cbTickWatch, w.handle)
		w.ticked = time.Now()
		w.ticking = true
		w.clockOn.Store(true)
		startYielding()
	}
}

// stopClock removes the frame clock's callback and the timer watching it but the one running now, which ends by returning 0.
func (w *window) stopClock(fromFrameClock bool) {
	if fromFrameClock {
		gSourceRemove(w.watchID)
	} else {
		gtkWidgetRemoveTickCallback(w.surface, w.tickID)
	}
	w.tickID, w.watchID = 0, 0
	w.ticking = false
	stopYielding()
}

// reportScroll reports a scroll view's offset. While the app waits for a
// display frame, the report rides with it: the engine handles both in one
// pass, where a report that woke it on its own cost a second pass and
// commit every frame of an app scrolling in its frames.
func (w *window) reportScroll(ev driver.Event) {
	if w.engine == nil {
		return
	}
	scrollbar.Report(w.engine, ev, w.frameWanted.Load())
}

// animatorReport hands what the animator reports to the engine.
func (w *window) animatorReport(ev driver.Event) { w.engine.Dispatch(ev) }

// animations applies a batch's changes to the animation graph.
func (w *window) animations(ops []driver.AnimationOp) {
	w.animator.Apply(ops)
	if w.animator.Running() {
		w.startClock()
	}
}

// showAnimated draws the views whose bound values changed with them.
func (w *window) showAnimated() {
	w.animator.Changed(func(id driver.NodeID, vals driver.AnimatedValues) {
		v := w.view(id)
		if v == nil {
			return
		}
		if vals.Set == 0 {
			v.animated = nil
		} else {
			v.animated = &vals
		}
		w.moved(v)
	})
	if ch := canvases.Load(); ch != nil {
		ch.changedCanvases(w)
	}
	if nh := natives.Load(); nh != nil {
		nh.changedNatives(w)
	}
	if sh := shaders.Load(); sh != nil {
		sh.changedShaders(w)
	}
	w.repinBound()
	if w.animator.Running() {
		if sh := shaders.Load(); sh != nil {
			sh.staleEffects(w)
		}
	}
}

// monotonic is the animator's clock: time since the window opened.
func (w *window) monotonic(now time.Time) time.Duration { return now.Sub(w.epoch) }

// tickCallback is a frame of the surface's GdkFrameClock, in its update phase: it moves every animation to now and runs the frame the app waits for at the clock's frame time.
func tickCallback(widget, clock, data uintptr) int32 {
	w := windowOf(data)
	if w == nil || w.closing {
		return 0
	}
	w.ticked = time.Now()
	done := w.measuring(true)
	w.tick(frameTime(clock), true)
	done()
	runtime.Gosched()
	// The callback stays while something animates.
	return boolInt(w.ticking)
}

// tickWatchCallback ticks in the frame clock's place while it has not ticked for stallMillis.
func tickWatchCallback(data uintptr) int32 {
	w := windowOf(data)
	if w == nil || w.closing {
		return 0
	}
	if time.Since(w.ticked) >= stallMillis*time.Millisecond {
		done := w.measuring(true)
		w.tick(time.Time{}, false)
		done()
		runtime.Gosched()
	}
	return boolInt(w.ticking)
}

// frameTime is the frame time of a GdkFrameClock, when its frame began on g_get_monotonic_time's clock in microseconds, on time.Now's base.
func frameTime(clock uintptr) time.Time {
	return driver.DisplayTime(time.Duration(gdkFrameClockGetFrameTime(clock))*time.Microsecond, time.Duration(gGetMonotonicTime())*time.Microsecond)
}

// step moves v's animations to now and reports whether one goes on.
func (w *window) step(v *view, now time.Time) bool {
	keep := false
	if t := v.transition; t != nil {
		if now.Before(t.start) {
			return true
		}
		elapsed := now.Sub(t.start)
		w.moved(v)
		if elapsed >= t.duration {
			v.transition = nil
		} else {
			p := t.curve.At(float64(elapsed) / float64(t.duration))
			t.shown = driver.MixStyles(t.from, t.to, p, t.props)
			keep = true
		}
	}
	if a := v.style.Animation; a.Keyframes != 0 {
		w.moved(v)
		if _, running := a.At(now.Sub(v.keyframeStart)); running {
			keep = true
		}
	}
	if velocity, ok := v.wheel.Settled(now); ok {
		w.settle(v, velocity)
	} else if v.wheel.Turning() && v.scrolling == nil {
		keep = true
	}
	if w.stepGlide(v, now) {
		keep = true
		if v.refreshing {
			gtkWidgetQueueDraw(v.widget)
		}
	}
	if s := v.scrolling; s != nil {
		elapsed := now.Sub(s.Start)
		if elapsed >= scrollDuration {
			v.scrolling = nil
			w.scrollTo(v, s.To)
			if s.Momentum {
				w.scrollEvent(v, driver.EventMomentumEnd)
			}
			w.restScroll(v)
		} else {
			p := float64(elapsed) / float64(scrollDuration)
			at := driver.Point{
				X: driver.Points(float64(s.From.X) + float64(s.To.X-s.From.X)*p),
				Y: driver.Points(float64(s.From.Y) + float64(s.To.Y-s.From.Y)*p),
			}
			w.scrollTo(v, at)
			keep = true
		}
	}
	if zm := v.zooming; zm != nil {
		x := min(float64(now.Sub(zm.start))/float64(scrollbar.Animation), 1)
		at, z := zm.ease.Step(&v.scroll, v.frame.Size, v.size, scrollbar.EaseOut(x))
		v.zoom = z
		w.moveOffset(v, at)
		if x >= 1 {
			v.zooming = nil
			w.restScroll(v)
		} else {
			keep = true
		}
	}
	return keep
}

// tick moves every animation to now and repaints, and runs the frame the app waits for at at, the zero time for the engine's clock; fromFrameClock says the frame clock's callback runs it, rather than the timer standing in for it.
func (w *window) tick(at time.Time, fromFrameClock bool) {
	defer w.reportGeometry()
	now := time.Now()
	live := w.animating[:0]
	for _, v := range w.animating {
		if !v.deleted && w.step(v, now) {
			live = append(live, v)
		}
	}
	clear(w.animating[len(live):])
	w.animating = live
	running := false
	if w.animator.Running() {
		w.animator.Step(w.monotonic(now))
		running = w.animator.Running()
	}
	w.showAnimated()
	if w.stepLayout(now) {
		running = true
	}
	if w.stepImages(now) {
		running = true
	}
	if w.frameWanted.Swap(false) && w.engine != nil {
		w.engine.FrameAt(at)
	}
	// A frame the engine asks for as the clock stops either finds it
	// stopped and starts it, or keeps it going here.
	if len(live) == 0 && !running {
		w.clockOn.Store(false)
		if w.frameWanted.Load() {
			w.clockOn.Store(true)
		} else {
			w.stopClock(fromFrameClock)
		}
	}
}
