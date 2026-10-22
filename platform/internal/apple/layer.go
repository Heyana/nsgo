//go:build darwin

package apple

import (
	"math"
	"strings"
	"structs"

	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/core"
)

var (
	sel = objc.RegisterName

	selSetBackgroundColor = sel("setBackgroundColor:")
	selSetCornerRadius    = sel("setCornerRadius:")
	selSetBorderWidth     = sel("setBorderWidth:")
	selSetBorderColor     = sel("setBorderColor:")
	selSetOpacity         = sel("setOpacity:")
	selSetMasksToBounds   = sel("setMasksToBounds:")
	selSetShadowColor     = sel("setShadowColor:")
	selSetShadowOffset    = sel("setShadowOffset:")
	selSetShadowRadius    = sel("setShadowRadius:")
	selSetShadowOpacity   = sel("setShadowOpacity:")
	selValueForKey        = sel("valueForKey:")
	selBounds             = sel("bounds")

	caCurrentMediaTime func() float64
)

// StyleLayer draws vs with a view's CALayer. upward says the layer's y
// axis points up, as AppKit's do, so shadows offset the other way. A
// style with a transition animates what it lists from the values shown.
func StyleLayer(layer objc.ID, vs *core.ViewStyle, upward bool) {
	tr := vs.Transition
	var keys []string
	var from []objc.ID
	if tr.Duration > 0 {
		shown := layer.Send(sel("presentationLayer"))
		if shown == 0 {
			shown = layer
		}
		keys = transitionKeys(tr.Properties)
		for _, k := range keys {
			from = append(from, shown.Send(selValueForKey, NSString(k)))
		}
	}
	color := func(sel objc.SEL, c core.Color) {
		if c == 0 {
			layer.Send(sel, uintptr(0))
			return
		}
		ref := CGColor(c)
		layer.Send(sel, ref)
		Release(ref)
	}
	color(selSetBackgroundColor, vs.Background)
	layer.Send(selSetCornerRadius, float64(vs.CornerRadius))
	layer.Send(selSetBorderWidth, float64(vs.BorderWidth))
	color(selSetBorderColor, vs.BorderColor)
	layer.Send(selSetOpacity, vs.Opacity)
	layer.Send(selSetMasksToBounds, vs.Clip)
	if vs.ShadowColor == 0 {
		layer.Send(selSetShadowOpacity, float32(0))
	} else {
		color(selSetShadowColor, vs.ShadowColor)
		dy := float64(vs.ShadowOffset.Y)
		if upward {
			dy = -dy
		}
		layer.Send(selSetShadowOffset, CGSize{Width: float64(vs.ShadowOffset.X), Height: dy})
		layer.Send(selSetShadowRadius, float64(vs.ShadowRadius))
		layer.Send(selSetShadowOpacity, float32(1))
	}
	SetTransform(layer, vs.Transform)
	for i, k := range keys {
		to := layer.Send(selValueForKey, NSString(k))
		if from[i] == to || (from[i] != 0 && objc.Send[bool](from[i], sel("isEqual:"), to)) {
			continue
		}
		start := from[i]
		if start == 0 && strings.HasSuffix(k, "Color") {
			clear := CGColor(0)
			start = objc.ID(clear).Send(sel("autorelease"))
		}
		anim := objc.ID(objc.GetClass("CABasicAnimation")).Send(sel("animationWithKeyPath:"), NSString(k))
		anim.Send(sel("setFromValue:"), start)
		anim.Send(sel("setDuration:"), tr.Duration.Seconds())
		anim.Send(sel("setTimingFunction:"), timing(tr.Easing))
		if tr.Delay > 0 {
			anim.Send(sel("setBeginTime:"), caCurrentMediaTime()+tr.Delay.Seconds())
			anim.Send(sel("setFillMode:"), NSString("backwards"))
		}
		layer.Send(selAddAnimation, anim, NSString("nsgo."+k))
	}
}

// transitionKeys returns the CALayer keys of the properties a transition
// animates.
func transitionKeys(p core.TransitionProperty) []string {
	if p == 0 {
		p = ^core.TransitionProperty(0)
	}
	var keys []string
	add := func(bit core.TransitionProperty, ks ...string) {
		if p&bit != 0 {
			keys = append(keys, ks...)
		}
	}
	add(core.TransitionOpacity, "opacity")
	add(core.TransitionBackground, "backgroundColor")
	add(core.TransitionTransform, "transform")
	add(core.TransitionBorder, "borderWidth", "borderColor", "cornerRadius")
	add(core.TransitionShadow, "shadowColor", "shadowOpacity", "shadowOffset", "shadowRadius")
	return keys
}

// timing returns the CAMediaTimingFunction of a curve.
func timing(c core.Curve) objc.ID {
	fn := objc.ID(objc.GetClass("CAMediaTimingFunction"))
	if c == (core.Curve{}) {
		return fn.Send(sel("functionWithName:"), NSString("linear"))
	}
	return fn.Send(sel("functionWithControlPoints::::"), c.X1, c.Y1, c.X2, c.Y2)
}

// transform3D is CATransform3D, which multiplies row vectors.
type transform3D struct {
	_                                                                              structs.HostLayout
	M11, M12, M13, M14, M21, M22, M23, M24, M31, M32, M33, M34, M41, M42, M43, M44 float64
}

// SetTransform sets a layer's transform to t around the layer's center:
// translate, then rotate, then scale. It works from wherever the anchor
// point is, since AppKit keeps its layers' at their corner.
func SetTransform(layer objc.ID, t core.Transform) {
	sx, sy := t.Scales()
	rad := float64(t.Rotate) * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	// The rotation and scale, for column vectors.
	a, b := cos*float64(sx), -sin*float64(sy)
	c, d := sin*float64(sx), cos*float64(sy)
	size := objc.Send[CGRect](layer, selBounds).Size
	anchor := objc.Send[CGPoint](layer, sel("anchorPoint"))
	cx, cy := (0.5-anchor.X)*size.Width, (0.5-anchor.Y)*size.Height
	m := transform3D{M11: a, M12: c, M21: b, M22: d, M33: 1, M44: 1,
		M41: float64(t.TranslateX) + cx - (a*cx + b*cy),
		M42: float64(t.TranslateY) + cy - (c*cx + d*cy)}
	if m == (transform3D{M11: 1, M22: 1, M33: 1, M44: 1}) {
		delete(held, layer)
	} else {
		held[layer] = m
	}
	setting = true
	layer.Send(sel("setTransform:"), m)
	setting = false
}

var (
	// held are the transforms SetTransform gave layers; setting says it
	// is setting one.
	held    = map[objc.ID]transform3D{}
	setting bool
)

// HeldTransformMethods are the methods of a CALayer subclass that keeps
// the transform SetTransform gave it: AppKit sets its views' layers back
// to its own transform whenever it syncs their geometry.
func HeldTransformMethods() []objc.MethodDef {
	return []objc.MethodDef{
		{Cmd: sel("setTransform:"), Fn: func(self objc.ID, cmd objc.SEL, m transform3D) {
			if h, ok := held[self]; ok && !setting {
				m = h
			}
			self.SendSuper(cmd, m)
		}},
		{Cmd: sel("setAffineTransform:"), Fn: func(self objc.ID, cmd objc.SEL, m affine) {
			if h, ok := held[self]; ok {
				self.SendSuper(sel("setTransform:"), h)
				return
			}
			self.SendSuper(cmd, m)
		}},
	}
}

// ForgetTransform forgets the transform SetTransform gave a layer that is
// going away.
func ForgetTransform(layer objc.ID) { delete(held, layer) }
