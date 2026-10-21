//go:build darwin

package apple

import (
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
)

// StyleLayer draws vs with a view's CALayer. upward says the layer's y
// axis points up, as AppKit's do, so shadows offset the other way.
func StyleLayer(layer objc.ID, vs *core.ViewStyle, upward bool) {
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
		return
	}
	color(selSetShadowColor, vs.ShadowColor)
	dy := float64(vs.ShadowOffset.Y)
	if upward {
		dy = -dy
	}
	layer.Send(selSetShadowOffset, CGSize{Width: float64(vs.ShadowOffset.X), Height: dy})
	layer.Send(selSetShadowRadius, float64(vs.ShadowRadius))
	layer.Send(selSetShadowOpacity, float32(1))
}
