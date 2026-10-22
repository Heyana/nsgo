//go:build windows

package windows

import (
	"time"

	"github.com/oneo-me/nsgo/core"
)

var (
	setTimer  = user32.NewProc("SetTimer")
	killTimer = user32.NewProc("KillTimer")
)

const (
	wmTimer        = 0x0113
	animationTimer = 1
	// frameMillis asks for about 60 frames a second; the system's timer
	// granularity may make it fewer.
	frameMillis    = 16
	scrollDuration = 300 * time.Millisecond

	allProperties = core.TransitionOpacity | core.TransitionBackground | core.TransitionTransform |
		core.TransitionBorder | core.TransitionShadow
)

// transition is a view's style animating from one ViewStyle to the next.
type transition struct {
	from, to core.ViewStyle
	start    time.Time // after the delay
	duration time.Duration
	curve    core.Curve
	props    core.TransitionProperty
	shown    core.ViewStyle // at the last frame
}

// scrolling is a scroll view's offset animating to a point.
type scrolling struct {
	from, to core.Point
	start    time.Time
}

// styleOf returns the style v shows now: mid transition, or its own.
func (w *window) styleOf(v *view) *core.ViewStyle {
	if v.transition != nil {
		return &v.transition.shown
	}
	return &v.style
}

// setStyle applies v's new style, animating to it when it has a
// transition. Windows turns its own animations off when the user turns
// off animation effects, so these snap then.
func (w *window) setStyle(v *view, s *core.ViewStyle) {
	from := *w.styleOf(v)
	styled := v.styled
	v.style, v.styled, v.transition = *s, true, nil
	t := s.Transition
	if !styled || t.Duration <= 0 || w.env.ReduceMotion {
		return
	}
	props := t.Properties
	if props == 0 {
		props = allProperties
	}
	first, last := mix(from, *s, 0, props), mix(from, *s, 1, props)
	if first == last {
		return
	}
	v.transition = &transition{from: from, to: *s, start: time.Now().Add(t.Delay), duration: t.Duration,
		curve: t.Easing, props: props, shown: first}
	w.animate(v)
}

// scrollAnimated scrolls v to offset over a moment, or at once when the
// user turned animation effects off.
func (w *window) scrollAnimated(v *view, offset core.Point) {
	if w.env.ReduceMotion {
		w.scrollTo(v, offset)
		return
	}
	v.scrolling = &scrolling{from: v.offset, to: offset, start: time.Now()}
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
	if !w.ticking {
		setTimer.Call(w.hwnd, animationTimer, frameMillis, 0)
		w.ticking = true
	}
}

// tick moves every animation to now and repaints.
func (w *window) tick() {
	now := time.Now()
	scrolled := false
	live := w.animating[:0]
	for _, v := range w.animating {
		if v.deleted {
			continue
		}
		if t := v.transition; t != nil {
			x := 0.0
			if now.After(t.start) {
				x = min(float64(now.Sub(t.start))/float64(t.duration), 1)
			}
			t.shown = mix(t.from, t.to, t.curve.At(x), t.props)
			if x >= 1 {
				v.transition = nil
			}
		}
		if sc := v.scrolling; sc != nil {
			x := min(float64(now.Sub(sc.start))/float64(scrollDuration), 1)
			e := core.Points(core.EaseInOut.At(x))
			at := core.Point{X: sc.from.X + (sc.to.X-sc.from.X)*e, Y: sc.from.Y + (sc.to.Y-sc.from.Y)*e}
			scrolled = w.scrollTo(v, at) || scrolled
			if x >= 1 {
				v.scrolling = nil
			}
		}
		if v.transition != nil || v.scrolling != nil {
			live = append(live, v)
		}
	}
	clear(w.animating[len(live):])
	w.animating = live
	if len(live) == 0 {
		killTimer.Call(w.hwnd, animationTimer)
		w.ticking = false
	}
	if scrolled {
		w.placeEdits()
	}
	invalidateRect.Call(w.hwnd, 0, 0)
}

// mix returns the style at progress t from from to to, moving props and
// taking everything else from to.
func mix(from, to core.ViewStyle, t float64, props core.TransitionProperty) core.ViewStyle {
	out := to
	if props&core.TransitionOpacity != 0 {
		out.Opacity = from.Opacity + (to.Opacity-from.Opacity)*float32(t)
	}
	if props&core.TransitionBackground != 0 {
		out.Background = mixColor(from.Background, to.Background, t)
	}
	if props&core.TransitionTransform != 0 {
		out.Transform = mixTransform(from.Transform, to.Transform, t)
	}
	if props&core.TransitionBorder != 0 {
		out.BorderWidth = mixPoints(from.BorderWidth, to.BorderWidth, t)
		out.BorderColor = mixColor(from.BorderColor, to.BorderColor, t)
		out.CornerRadius = mixPoints(from.CornerRadius, to.CornerRadius, t)
	}
	if props&core.TransitionShadow != 0 {
		out.ShadowColor = mixColor(from.ShadowColor, to.ShadowColor, t)
		out.ShadowRadius = mixPoints(from.ShadowRadius, to.ShadowRadius, t)
		out.ShadowOffset = core.Point{X: mixPoints(from.ShadowOffset.X, to.ShadowOffset.X, t),
			Y: mixPoints(from.ShadowOffset.Y, to.ShadowOffset.Y, t)}
	}
	return out
}

func mixPoints(a, b core.Points, t float64) core.Points { return a + (b-a)*core.Points(t) }

func mixFloat(a, b float32, t float64) float32 { return a + (b-a)*float32(t) }

// mixColor mixes two 0xRRGGBBAA colors; none fades from or to the other
// color's transparent self rather than through black.
func mixColor(a, b core.Color, t float64) core.Color {
	if a == b {
		return b
	}
	if a&0xff == 0 {
		a = b &^ 0xff
	}
	if b&0xff == 0 {
		b = a &^ 0xff
	}
	var out core.Color
	for shift := 0; shift < 32; shift += 8 {
		x, y := float64(a>>shift&0xff), float64(b>>shift&0xff)
		out |= core.Color(x+(y-x)*t+0.5) << shift
	}
	return out
}

// mixTransform mixes each part of two transforms, scales resolved.
func mixTransform(a, b core.Transform, t float64) core.Transform {
	ax, ay := a.Scales()
	bx, by := b.Scales()
	return core.Transform{
		TranslateX: mixPoints(a.TranslateX, b.TranslateX, t),
		TranslateY: mixPoints(a.TranslateY, b.TranslateY, t),
		ScaleX:     mixFloat(ax, bx, t),
		ScaleY:     mixFloat(ay, by, t),
		Rotate:     mixFloat(a.Rotate, b.Rotate, t),
	}
}
