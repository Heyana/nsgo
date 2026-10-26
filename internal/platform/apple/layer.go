//go:build darwin

package apple

import (
	"math"
	"strings"
	"structs"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/internal/driver"
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
func StyleLayer(layer objc.ID, vs *driver.ViewStyle, upward bool) {
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
	color := func(sel objc.SEL, c driver.Color) {
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
	setBorders(layer, vs, &tr)
	applyDecor(layer, vs, upward)
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
func transitionKeys(p driver.TransitionProperty) []string {
	if p == 0 {
		p = ^driver.TransitionProperty(0)
	}
	var keys []string
	add := func(bit driver.TransitionProperty, ks ...string) {
		if p&bit != 0 {
			keys = append(keys, ks...)
		}
	}
	add(driver.TransitionOpacity, "opacity")
	add(driver.TransitionBackground, "backgroundColor")
	add(driver.TransitionTransform, "transform")
	add(driver.TransitionBorder, "borderWidth", "borderColor", "cornerRadius")
	add(driver.TransitionShadow, "shadowColor", "shadowOpacity", "shadowOffset", "shadowRadius")
	return keys
}

// timing returns the CAMediaTimingFunction of a curve.
func timing(c driver.Curve) objc.ID {
	fn := objc.ID(objc.GetClass("CAMediaTimingFunction"))
	if c == (driver.Curve{}) {
		return fn.Send(sel("functionWithName:"), NSString("linear"))
	}
	return fn.Send(sel("functionWithControlPoints::::"), c.X1, c.Y1, c.X2, c.Y2)
}

// transform3D is CATransform3D, which multiplies row vectors.
type transform3D struct {
	_                                                                              structs.HostLayout
	M11, M12, M13, M14, M21, M22, M23, M24, M31, M32, M33, M34, M41, M42, M43, M44 float64
}

// SetTransform sets a layer's transform to t around its transform
// origin: its center, moved by its style's Origin. It works from wherever
// the anchor point is, since AppKit keeps its layers' at their corner.
func SetTransform(layer objc.ID, t driver.Transform) {
	size := objc.Send[CGRect](layer, selBounds).Size
	anchor := objc.Send[CGPoint](layer, sel("anchorPoint"))
	ax, ay := anchor.X*size.Width, anchor.Y*size.Height
	origin := driver.Point{X: driver.Points(size.Width / 2), Y: driver.Points(size.Height / 2)}
	if d := decors[layer]; d != nil {
		origin.X += d.vs.Origin.X
		origin.Y += d.vs.Origin.Y
	}
	m := toCA(driver.Translate3D(-ax, -ay, 0).Mul(t.Matrix(origin)).Mul(driver.Translate3D(ax, ay, 0)))
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
			Super(self, cmd, m)
		}},
		{Cmd: sel("setAffineTransform:"), Fn: func(self objc.ID, cmd objc.SEL, m affine) {
			if h, ok := held[self]; ok {
				Super(self, sel("setTransform:"), h)
				return
			}
			Super(self, cmd, m)
		}},
	}
}

// ForgetTransform forgets the transform SetTransform gave a layer that is
// going away.
func ForgetTransform(layer objc.ID) { delete(held, layer) }

// SetSublayerTranslation moves layer's sublayers by (dx, dy) points, as a
// scroll view shows an offset its bounds do not have yet.
func SetSublayerTranslation(layer objc.ID, dx, dy float64) {
	layer.Send(sel("setSublayerTransform:"), transform3D{M11: 1, M22: 1, M33: 1, M44: 1, M41: dx, M42: dy})
}

// RotateLayer turns layer by radians around its anchor point.
func RotateLayer(layer objc.ID, radians float64) {
	c, s := math.Cos(radians), math.Sin(radians)
	layer.Send(sel("setTransform:"), transform3D{M11: c, M12: s, M21: -s, M22: c, M33: 1, M44: 1})
}

// MediaTime returns Core Animation's clock: the time since the system
// started, on which display links and NSEvent timestamps count too.
func MediaTime() time.Duration { return time.Duration(caCurrentMediaTime() * float64(time.Second)) }
