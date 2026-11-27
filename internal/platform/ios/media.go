//go:build ios

package ios

import (
	"log"
	"math"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// The surface's frame clock, a CADisplayLink, steps the native animation
// graph and the layout animations once a display frame while either
// runs.

var (
	selTick     = sel("nsgoTick:")
	selDuration = sel("duration")
)

func (s *surface) clock() time.Duration {
	if s.epoch.IsZero() {
		s.epoch = time.Now()
	}
	return time.Since(s.epoch)
}

// layoutStart is when a layout change on tr starts: now, or for the
// keyboard's curve when the keyboard started to move, however late the
// batch arrived.
func (s *surface) layoutStart(tr *driver.LayoutTransition) time.Duration {
	now := s.clock()
	if tr.Curve == driver.LayoutKeyboard && s.keyboardStart > 0 && now-s.keyboardStart < tr.End() {
		return s.keyboardStart
	}
	return now
}

func (s *surface) startTicking() {
	defer s.publishPace()
	if s.ticking {
		s.setPace()
		return
	}
	s.ticking = true
	s.pace = s.wantedPace()
	if s.link == 0 {
		// A link needs the view in a window: without one, the next mount
		// asks again.
		link := objc.ID(apple.Class("CADisplayLink")).Send(sel("displayLinkWithTarget:selector:"), s.native, selTick)
		if link == 0 {
			s.ticking = false
			return
		}
		s.link = link.Send(sel("retain"))
		s.appliedRate, s.appliedFPS = apple.CAFrameRateRange{}, -1
		s.applyFrameRate()
		s.link.Send(sel("addToRunLoop:forMode:"), objc.ID(apple.Class("NSRunLoop")).Send(sel("mainRunLoop")),
			apple.CommonModes())
		return
	}
	s.applyFrameRate()
	s.link.Send(sel("setPaused:"), false)
}

func (s *surface) screenRate() int {
	screen := s.window.Send(sel("screen"))
	if screen == 0 {
		screen = objc.ID(apple.Class("UIScreen")).Send(sel("mainScreen"))
	}
	return max(objc.Send[int](screen, sel("maximumFramesPerSecond")), 0)
}

// applyFrameRate preserves the link's usual pace for native motion. A
// link with only app callbacks can take its explicit preferred range.
func (s *surface) applyFrameRate() {
	if s.link == 0 {
		return
	}
	rate := apple.CAFrameRateRange{}
	fps := -1
	if s.preferredRate.Preferred > 0 && !s.animator.Running() && len(s.motions) == 0 &&
		len(s.ghosts) == 0 && len(s.borrowedFrames) == 0 && s.selecting == nil && s.perf == nil {
		maximum := s.screenRate()
		if maximum == 0 {
			maximum = 60
		}
		rate = s.preferredRate.Clamped(float32(maximum))
	} else if s.pace > 0 {
		fps = max(int(math.Ceil(s.pace)), 1)
	} else {
		fps = s.screenRate()
	}
	if rate == s.appliedRate && fps == s.appliedFPS {
		return
	}
	s.appliedRate, s.appliedFPS = rate, fps
	if fps < 0 {
		s.link.Send(sel("setPreferredFrameRateRange:"), rate)
	} else {
		// A ProMotion iPhone otherwise defaults to 60 Hz.
		s.link.Send(sel("setPreferredFramesPerSecond:"), fps)
	}
}

// wantedPace is the rate the frame clock need run at, as macOS's: the
// animations' own when they are all it steps, else the screen's highest.
// Batches apply as they land, so none waits for a slowed frame.
func (s *surface) wantedPace() float64 {
	if len(s.motions) > 0 || len(s.transitions) > 0 || len(s.ghosts) > 0 || s.frameWanted.Load() || len(s.borrowedFrames) > 0 || s.selecting != nil || s.perf != nil || s.pipFramePending {
		return 0
	}
	return s.animator.FrameRate()
}

// setPace moves the running link to the rate wanted now.
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

func (s *surface) stopTicking() {
	if s.ticking {
		s.ticking = false
		s.publishPace()
		s.link.Send(sel("setPaused:"), true)
	}
}

func surfaceTick(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.surface.tick()
	}
}

func (s *surface) tick() {
	defer s.reportGeometry()
	if s.perf != nil {
		s.perf.Vsync(time.Now(), time.Duration(apple.MsgFloat64(s.link, selDuration)*float64(time.Second)))
		defer s.measuring(s.animator.Running() || len(s.motions) > 0 || len(s.transitions) > 0 || len(s.ghosts) > 0 || s.frameWanted.Load())()
	}
	now := s.clock()
	contentAnimated := s.animator.Running() || len(s.motions) > 0 || len(s.transitions) > 0
	if s.animator.Running() {
		s.animator.Step(now)
	}
	apple.WithoutAnimation(func() {
		s.stepMotions(now)
		s.stepTransitions(now)
		s.pullBackResizedScrolls()
		s.applyAnimated()
	})
	if sh := shaders.Load(); sh != nil {
		if s.animator.Running() || len(s.motions) > 0 || len(s.transitions) > 0 {
			sh.staleEffects(s)
		}
		sh.drawShaders(s)
	}
	if contentAnimated {
		s.dirtyPictureInPictures()
	}
	s.pipFramePending = s.drawPictureInPictures()
	s.startImages()
	if wanted := s.frameWanted.Swap(false); wanted || len(s.borrowedFrames) != 0 {
		at := apple.LinkFrameTime(s.link)
		if wanted && s.engine != nil {
			s.engine.FrameAt(at)
		}
		for _, scene := range s.borrowedFrames {
			if scene.frameWanted.Swap(false) && scene.engine != nil {
				scene.engine.FrameAt(at)
			}
		}
		clear(s.borrowedFrames)
		s.borrowedFrames = s.borrowedFrames[:0]
	}
	apple.YieldFrame()
	// A monitor closes a frame at every refresh.
	if !s.pipFramePending && !s.animator.Running() && len(s.motions) == 0 && len(s.transitions) == 0 && len(s.ghosts) == 0 && !s.frameWanted.Load() && s.perf == nil && !s.geometryAnimating() {
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
	}, (*view).redraw, func(v *view) {
		if ch := canvases.Load(); ch != nil {
			ch.redrawAnimated(v)
		}
	}, func(v *view) driver.NativeComponent {
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
	drawn := outerLayerStyle(vs, v.tintLayer() != 0)
	if v.holdClip(&drawn) {
		// The holder clips the children: the view's own layer, its shadow and outline, stays unclipped.
		v.native.Send(sel("setClipsToBounds:"), false)
		drawn.Clip = false
	}
	v.followScale(&drawn)
	// A view created by an earlier batch shows the default style.
	var shown *driver.ViewStyle
	if v.styled || v.born != v.surface.batch {
		shown = &v.drawn
	}
	if v.layer == 0 {
		v.layer = objc.ID(apple.Msg(v.native, selLayer))
	}
	layer := v.layer
	if added := bound &^ v.bound; added != 0 {
		apple.ClearBoundAnimations(layer, added)
	}
	v.bound = bound
	if apple.RestyleLayerGeometry(layer, shown, &drawn, 0, bound, &v.layerGeom) {
		// RestyleLayer may create per-side rings after setBackdrop ran; move them above the backdrop now that they exist.
		apple.MoveBorders(layer, v.borderHost())
	}
	v.drawn, v.styled = drawn, true
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
		Start:   s.layoutStart,
		Disable: func(v *view) { v.native.Send(sel("setUserInteractionEnabled:"), false) },
	}
}

// layoutBatch is what a batch with a layout animation tracks.
type layoutBatch = apple.LayoutBatch

// setFrame moves v to the item's frame, animating from where it shows now when the batch animates layout or the item carries a transition.
func (s *surface) setFrame(v *view, it *driver.MountItem, lb *layoutBatch) {
	s.layoutHost().SetFrame(v, it, lb)
}

// place sets v's frame and what follows it. Bounds and center, unlike
// the frame, hold with a transform, which a style or a screen's swipe
// sets; the bounds' origin is a scroll view's offset. A view with neither
// takes its frame in one message: each with a struct costs purego about
// two microseconds, most of what placing a pushed screen's views cost.
func (v *view) place(f driver.Bounds) {
	resized := f.Size != v.frame.Size
	v.frame, v.framed = f, true
	if v.surface.accessories[v.id] == nil {
		v.layerGeom.SetSize(f.Size)
	} else {
		v.layerGeom.ClearSize()
	}
	transformed := v.styled && v.drawn.Transform != (driver.Transform{})
	if !transformed && v.bar == nil && v.kind != driver.KindScroll {
		v.native.Send(selSetFrame, apple.Rect(float64(f.Origin.X+v.shift.X), float64(f.Origin.Y+v.shift.Y),
			float64(f.Size.Width), float64(f.Size.Height)))
	} else {
		b := objc.Send[apple.CGRect](v.native, selBounds)
		b.Size = apple.CGSize{Width: float64(f.Size.Width), Height: float64(f.Size.Height)}
		v.native.Send(sel("setBounds:"), b)
		v.native.Send(sel("setCenter:"), apple.CGPoint{X: float64(f.Origin.X + v.shift.X + f.Size.Width/2),
			Y: float64(f.Origin.Y + v.shift.Y + f.Size.Height/2)})
	}
	if v.layer == 0 {
		v.layer = objc.ID(apple.Msg(v.native, selLayer))
	}
	layer := v.layer
	if transformed {
		// Its origin is a point of the old size.
		apple.SetTransformGeometry(layer, v.drawn.Transform, &v.layerGeom)
	}
	apple.LayoutDecor(layer)
	if resized && v.container != 0 {
		// As last drawn, animated corners and all: only the view's own layer leaves the clip out. The perspective's centre follows the size too.
		held := v.drawn
		held.Clip = v.clipHeld
		v.fitHolder(&held)
	}
	if v.backdrop != 0 {
		v.fitBackdrop(&v.style, float64(f.Size.Width), float64(f.Size.Height))
	}
	if v.componentView != 0 {
		v.componentView.Send(selSetFrame, apple.Rect(0, 0, float64(f.Size.Width), float64(f.Size.Height)))
	}
	v.sizeContent()
	if resized && v.sc != nil {
		v.surface.resizedScrolls = append(v.surface.resizedScrolls, v)
		if v.hasEdges() {
			// Each step of a layout animation, as the keyboard moves a composer.
			v.placeEdges()
		}
		v.updateHandle()
	}
	if v.image != nil {
		v.image.Layout(apple.CGSize{Width: float64(f.Size.Width), Height: float64(f.Size.Height)})
		v.surface.images = append(v.surface.images, v)
	}
	if v.video != nil {
		if vh := videos.Load(); vh != nil {
			vh.layout(v)
		}
	}
	if v.shader != nil {
		if sh := shaders.Load(); sh != nil {
			sh.sizeShader(v)
		}
	}
	if g := gpus.Load(); g != nil && v.gpu != nil {
		g.sizeView(v)
	}
	if field := v.surface.accessories[v.id]; field != nil && field.input != nil {
		// A taller bar shows only once the keyboard asks for it again.
		field.input.field.Send(sel("reloadInputViews"))
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

// configureNative makes a native view's component on its first item and
// passes it the props that changed.
func (s *surface) configureNative(v *view, c *driver.NativeConfig) {
	if c.Bind {
		s.animator.BindNative(v.id, c.Bindings)
	}
	if v.component == nil && !v.componentTried {
		v.componentTried = true
		factory := s.components[c.Component]
		if factory == nil {
			factory = s.hostFactory(c.Component)
		}
		if factory == nil && c.Component == driver.TabBarComponent {
			factory = newTabBar
		}
		if factory == nil && c.Component == driver.SwitchComponent {
			factory = newSwitch
		}
		if factory == nil {
			if !s.unknown[c.Component] {
				s.unknown[c.Component] = true
				log.Printf("nsgo: no native component %q is registered", c.Component)
			}
			return
		}
		e := s.engine
		id := v.id
		v.component = factory(driver.NewNativeContext(
			func(name string, data any) {
				e.Dispatch(driver.Event{Kind: driver.EventNative, ID: id, Text: name, Data: data})
			},
			func(size driver.Size) { e.Dispatch(driver.Event{Kind: driver.EventNativeSize, ID: id, Size: size}) },
		))
		if v.component == nil {
			return
		}
		var native objc.ID
		switch h := v.component.View().(type) {
		case objc.ID:
			native = h
		case uintptr:
			native = objc.ID(h)
		}
		if native != 0 {
			native.Send(selSetFrame, objc.Send[apple.CGRect](v.native, selBounds))
			v.children().Send(selAddSubview, native)
			v.componentView = native
			s.directTree(native)
		}
	}
	if v.component == nil {
		return
	}
	for name, value := range c.Props {
		v.component.SetProp(name, value)
	}
	if _, ok := v.component.(*uiSwitch); ok {
		v.updateAccessibility()
	}
}

// setAccessory makes the InputAccessoryView id the bar above input v's
// keyboard, or removes the bar for 0.
func (s *surface) setAccessory(v *view, id driver.NodeID) {
	if v.accessory == id {
		return
	}
	if old := v.accessory; old != 0 && s.accessories[old] == v {
		delete(s.accessories, old)
	}
	v.accessory = id
	bar := objc.ID(0)
	if id != 0 && int(id) < len(s.views) && s.views[id] != nil {
		bar = s.views[id].native
		const flexibleWidth = 2
		bar.Send(sel("setAutoresizingMask:"), uint(flexibleWidth))
		s.views[id].layerGeom.ClearSize()
		s.accessories[id] = v
	}
	v.input.setBar(bar)
}
