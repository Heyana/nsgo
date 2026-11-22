package driver

import (
	"slices"
	"time"
)

// Backends that draw their own views (Windows, Linux) step a style transition themselves: each frame shows the style MixStyles returns. Apple and the web also use StyleTransition to step composite properties (transform, or box-shadow on the web) when some sub-properties are bound by AnimatedValues while others transition.

// StyleTransition steps a subset of ViewStyle properties on a backend's frame clock from one style to another.
type StyleTransition struct {
	from, to ViewStyle
	start    time.Duration
	delay    time.Duration
	duration time.Duration
	curve    Curve
	props    TransitionProperty
}

// StartStyleTransition starts, keeps or retargets a transition over props from prevStyle to nextStyle at now, ignoring differences covered by anim. It returns nil when nextStyle carries no transition for props or no unbound property in props changed.
func StartStyleTransition(prev *StyleTransition, prevStyle, nextStyle ViewStyle, now time.Duration, props TransitionProperty, anim AnimatedValues) *StyleTransition {
	tr := nextStyle.Transition
	if tr.Duration <= 0 || props == 0 {
		return nil
	}
	if tr.Properties != 0 {
		props &= tr.Properties
		if props == 0 {
			return nil
		}
	}
	from := prevStyle
	if prev != nil {
		// Keep an in-flight transition running when its target for props did not change, updating non-props fields on to so At preserves them.
		if prev.props == props {
			if _, done := prev.At(now); !done {
				a, b := MixStyles(prev.to, nextStyle, 0, props), MixStyles(prev.to, nextStyle, 1, props)
				if sameStyleProps(anim.Apply(a), anim.Apply(b), props) {
					prev.to = nextStyle
					return prev
				}
			}
		}
		from, _ = prev.At(now)
	}
	first, last := MixStyles(from, nextStyle, 0, props), MixStyles(from, nextStyle, 1, props)
	if sameStyleProps(anim.Apply(first), anim.Apply(last), props) {
		return nil
	}
	return &StyleTransition{
		from: from, to: nextStyle, start: now,
		delay: tr.Delay, duration: tr.Duration, curve: tr.Easing, props: props,
	}
}

// Props returns the transition property groups st steps.
func (st *StyleTransition) Props() TransitionProperty { return st.props }

// At returns the style shown at now and whether the transition has finished.
func (st *StyleTransition) At(now time.Duration) (ViewStyle, bool) {
	elapsed := now - st.start - st.delay
	if elapsed <= 0 {
		return MixStyles(st.from, st.to, 0, st.props), false
	}
	if elapsed >= st.duration {
		return st.to, true
	}
	t := st.curve.At(float64(elapsed) / float64(st.duration))
	return MixStyles(st.from, st.to, t, st.props), false
}

// sameStyleProps reports whether a and b match on props, comparing Effects.Shadows by value rather than pointer.
func sameStyleProps(a, b ViewStyle, props TransitionProperty) bool {
	if props&TransitionShadow != 0 {
		var sa, sb []BoxShadow
		if a.Effects != nil {
			sa = a.Effects.Shadows
		}
		if b.Effects != nil {
			sb = b.Effects.Shadows
		}
		if !slices.Equal(sa, sb) {
			return false
		}
	}
	a.Effects, b.Effects = nil, nil
	return a == b
}

// MixStyles returns the style shown t of the way from from to to, moving
// only the properties props lists and taking to's for the rest.
func MixStyles(from, to ViewStyle, t float64, props TransitionProperty) ViewStyle {
	out := to
	if props&TransitionOpacity != 0 {
		out.Opacity = mixFloat(from.Opacity, to.Opacity, t)
	}
	if props&TransitionBackground != 0 {
		out.Background = mixColor(from.Background, to.Background, t)
	}
	if props&TransitionTransform != 0 {
		a, b := from.Transform, to.Transform
		out.Transform = Transform{
			TranslateX: mixPoints(a.TranslateX, b.TranslateX, t), TranslateY: mixPoints(a.TranslateY, b.TranslateY, t),
			Scale: mixScale(a.Scale, b.Scale, t), ScaleX: mixScale(a.ScaleX, b.ScaleX, t), ScaleY: mixScale(a.ScaleY, b.ScaleY, t),
			Rotate: mixFloat(a.Rotate, b.Rotate, t), RotateX: mixFloat(a.RotateX, b.RotateX, t), RotateY: mixFloat(a.RotateY, b.RotateY, t),
			RotateZ: mixFloat(a.RotateZ, b.RotateZ, t),
			SkewX:   mixFloat(a.SkewX, b.SkewX, t), SkewY: mixFloat(a.SkewY, b.SkewY, t),
			Perspective: mixPoints(a.Perspective, b.Perspective, t), Then: mixMatrix(a.Then, b.Then, t),
		}
		out.Origin = Point{X: mixPoints(from.Origin.X, to.Origin.X, t), Y: mixPoints(from.Origin.Y, to.Origin.Y, t)}
	}
	if props&TransitionBorder != 0 {
		out.BorderWidth = mixPoints(from.BorderWidth, to.BorderWidth, t)
		out.BorderColor = mixColor(from.BorderColor, to.BorderColor, t)
		out.Borders = mixEdges(from.Borders, to.Borders, t)
		out.BorderColors = EdgeColors{Top: mixColor(from.BorderColors.Top, to.BorderColors.Top, t),
			Right:  mixColor(from.BorderColors.Right, to.BorderColors.Right, t),
			Bottom: mixColor(from.BorderColors.Bottom, to.BorderColors.Bottom, t),
			Left:   mixColor(from.BorderColors.Left, to.BorderColors.Left, t)}
		out.CornerRadius = mixPoints(from.CornerRadius, to.CornerRadius, t)
		if from.Corners != (Corners{}) || to.Corners != (Corners{}) {
			a, b := from.Radii(Size{Width: 1 << 20, Height: 1 << 20}), to.Radii(Size{Width: 1 << 20, Height: 1 << 20})
			out.CornerRadius = 0
			out.Corners = Corners{mixPoints(a.TopLeft, b.TopLeft, t), mixPoints(a.TopRight, b.TopRight, t),
				mixPoints(a.BottomRight, b.BottomRight, t), mixPoints(a.BottomLeft, b.BottomLeft, t)}
		}
		if from.Outline.Width > 0 || to.Outline.Width > 0 {
			o := to.Outline
			if o.Width == 0 {
				o = from.Outline
			}
			o.Width = mixPoints(from.Outline.Width, to.Outline.Width, t)
			o.Offset = mixPoints(from.Outline.Offset, to.Outline.Offset, t)
			o.Color = mixColor(from.Outline.Color, to.Outline.Color, t)
			out.Outline = o
		}
	}
	if props&TransitionShadow != 0 {
		out.ShadowColor = mixColor(from.ShadowColor, to.ShadowColor, t)
		out.ShadowRadius = mixPoints(from.ShadowRadius, to.ShadowRadius, t)
		out.ShadowOffset = Point{X: mixPoints(from.ShadowOffset.X, to.ShadowOffset.X, t),
			Y: mixPoints(from.ShadowOffset.Y, to.ShadowOffset.Y, t)}
		// Box shadows move layer by layer when the layers match.
		if a, b := from.Effects, to.Effects; a != nil && b != nil && len(a.Shadows) == len(b.Shadows) && len(a.Shadows) > 0 {
			e := *b
			e.Shadows = make([]BoxShadow, len(b.Shadows))
			for i, s := range b.Shadows {
				f := a.Shadows[i]
				if f.Inset != s.Inset {
					e.Shadows[i] = s
					continue
				}
				e.Shadows[i] = BoxShadow{Offset: Point{X: mixPoints(f.Offset.X, s.Offset.X, t), Y: mixPoints(f.Offset.Y, s.Offset.Y, t)},
					Blur: mixPoints(f.Blur, s.Blur, t), Spread: mixPoints(f.Spread, s.Spread, t),
					Color: mixColor(f.Color, s.Color, t), Inset: s.Inset}
			}
			out.Effects = &e
		}
	}
	return out
}

func mixPoints(a, b Points, t float64) Points { return a + (b-a)*Points(t) }

func mixFloat(a, b float32, t float64) float32 { return a + (b-a)*float32(t) }

// mixScale mixes two scales, whose zero is unset and so 1: moving to or
// from a style that sets one shrinks or grows from the shown size, not
// from nothing.
func mixScale(a, b float32, t float64) float32 {
	if a == b {
		return a
	}
	if a == 0 {
		a = 1
	}
	if b == 0 {
		b = 1
	}
	return mixFloat(a, b, t)
}

func mixEdges(a, b Edges, t float64) Edges {
	return Edges{Top: mixPoints(a.Top, b.Top, t), Right: mixPoints(a.Right, b.Right, t),
		Bottom: mixPoints(a.Bottom, b.Bottom, t), Left: mixPoints(a.Left, b.Left, t)}
}

// mixColor blends two colors in straight alpha, taking the visible color
// of a transparent end rather than fading through black.
func mixColor(a, b Color, t float64) Color {
	if a == b {
		return a
	}
	return MixColors(a, b, float32(t), ColorSpaceSRGB)
}

// Matrix returns the step's transform for a box of size, around its
// center: translated, turned, then scaled.
func (k Keyframe) Matrix(size Size) Matrix3D {
	t := Transform{TranslateY: Points(k.TranslateY) * size.Height, Rotate: k.Rotate, Scale: k.Scale}
	return t.Matrix(Point{X: size.Width / 2, Y: size.Height / 2})
}

// mixMatrix mixes two Then matrices entry by entry, a zero one standing
// for the identity. That is exact for moves and scales and close for
// small turns; a CSS list's own functions are not matched one by one.
func mixMatrix(a, b Matrix3D, t float64) Matrix3D {
	if a == b {
		return a
	}
	if a == (Matrix3D{}) {
		a = Identity3D
	}
	if b == (Matrix3D{}) {
		b = Identity3D
	}
	var out Matrix3D
	for i := range out {
		out[i] = a[i] + (b[i]-a[i])*t
	}
	return out
}
