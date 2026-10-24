//go:build ios

package ios

import (
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/apple"
)

// Text and canvas views draw into a content layer of their own, as on
// macOS: a plain CALayer whose delegate draws in drawLayer:inContext:,
// below the view's sublayers as a backing store's contents would be. The
// layers a style adds under the children (gradients, fills of shaped
// corners) then sit under the text too, where drawRect: would have drawn
// below them.

var (
	contentDrawer  objc.ID
	contentActions objc.ID
	// drawers maps content layers to their views.
	drawers = map[objc.ID]*view{}

	selContentsAreFlipped = sel("contentsAreFlipped")
	selSetNeedsDisplayL   = sel("setNeedsDisplay")
)

// contentZ puts a content layer below the view's other sublayers.
const contentZ = -1

func registerContent() {
	drawer := register("NsgoContentDrawer", objc.GetClass("NSObject"), nil, []objc.MethodDef{{
		Cmd: sel("drawLayer:inContext:"),
		Fn:  drawContent,
	}})
	contentDrawer = objc.ID(drawer).Send(selNew)
	null := objc.ID(objc.GetClass("NSNull")).Send(sel("null"))
	dict := objc.ID(objc.GetClass("NSMutableDictionary")).Send(selNew)
	for _, k := range []string{"contents", "bounds", "position", "frame", "hidden", "contentsScale",
		"onOrderIn", "onOrderOut", "sublayers"} {
		dict.Send(sel("setObject:forKey:"), null, apple.NSString(k))
	}
	contentActions = dict
}

// drawContent draws a view's text or canvas into its content layer, top
// down.
func drawContent(_ objc.ID, _ objc.SEL, layer objc.ID, ctx uintptr) {
	v := drawers[layer]
	if v == nil || ctx == 0 || (v.layout == nil && v.drawing == nil) {
		return
	}
	h := float64(v.frame.Size.Height)
	apple.DrawTopDown(ctx, h, !apple.MsgBool(layer, selContentsAreFlipped), func() {
		if v.drawing != nil {
			apple.Play(ctx, v.drawing)
			return
		}
		box := v.box
		v.layout.DrawBackgrounds(ctx, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y), 0, 0))
		v.drawSelection(ctx)
		v.layout.Draw(ctx, h, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y),
			float64(box.Size.Width), float64(box.Size.Height)))
	})
}

// newContent gives text or canvas view v its content layer.
func (v *view) newContent() {
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	layer.Send(sel("setActions:"), contentActions)
	layer.Send(sel("setDelegate:"), contentDrawer)
	layer.Send(sel("setNeedsDisplayOnBoundsChange:"), true)
	// Canvases are drawn on Core Animation's threads: paths and gradients
	// cost more than glyphs.
	layer.Send(sel("setDrawsAsynchronously:"), v.kind == driver.KindCanvas)
	layer.Send(sel("setContentsScale:"), float64(v.surface.env.PixelRatio))
	layer.Send(sel("setZPosition:"), float64(contentZ))
	v.native.Send(selLayer).Send(sel("addSublayer:"), layer)
	layer.Send(selRelease)
	v.contentLayer = layer
	drawers[layer] = v
}

// redraw draws v's content again.
func (v *view) redraw() {
	if v.contentLayer != 0 {
		v.contentLayer.Send(selSetNeedsDisplayL)
	}
}

// sizeContent fits v's content layer to its frame.
func (v *view) sizeContent() {
	if v.contentLayer != 0 {
		apple.WithoutAnimation(func() {
			v.contentLayer.Send(selSetFrame, apple.Rect(0, 0, float64(v.frame.Size.Width), float64(v.frame.Size.Height)))
		})
	}
}

// forgetContent drops v's content layer, which goes with its view.
func (v *view) forgetContent() {
	if v.contentLayer != 0 {
		delete(drawers, v.contentLayer)
		v.contentLayer = 0
	}
}

// A view's backdrop blur is a UIVisualEffectView under its children with
// the system material its style names, and Liquid Glass on iOS 26 and
// later when it asks for glass. UIKit sets the blur's strength by
// material: the radius only turns it on.

// blurStyle is the UIBlurEffectStyle of a backdrop material.
func blurStyle(m driver.BackdropMaterial) int {
	return 6 + int(m) // systemUltraThinMaterial and on
}

// setBackdrop gives v the backdrop its style asks for, or takes it away.
func (v *view) setBackdrop(vs *driver.ViewStyle) {
	glass := vs.Glass != driver.GlassNone && objc.GetClass("UIGlassEffect") != 0
	want := vs.Effects.BackdropBlur() > 0 || glass
	if !want || glass != v.glass {
		if v.backdrop != 0 {
			v.backdrop.Send(sel("removeFromSuperview"))
			v.backdrop = 0
		}
		if !want {
			return
		}
	}
	var effect objc.ID
	if glass {
		effect = objc.ID(objc.GetClass("UIGlassEffect")).Send(sel("effectWithStyle:"), int(vs.Glass)-1)
	} else {
		effect = objc.ID(objc.GetClass("UIBlurEffect")).Send(sel("effectWithStyle:"), blurStyle(vs.Material))
	}
	if v.backdrop == 0 {
		v.backdrop = objc.ID(objc.GetClass("UIVisualEffectView")).Send(selAlloc).Send(sel("initWithEffect:"), effect)
		v.backdrop.Send(selSetFrame, objc.Send[apple.CGRect](v.native, selBounds))
		const flexible = 2 | 16 // width and height
		v.backdrop.Send(sel("setAutoresizingMask:"), flexible)
		v.backdrop.Send(sel("setUserInteractionEnabled:"), false)
		v.native.Send(sel("insertSubview:atIndex:"), v.backdrop, 0)
		v.backdrop.Send(selRelease)
		v.glass = glass
	} else {
		v.backdrop.Send(sel("setEffect:"), effect)
	}
	radius := float64(max(vs.CornerRadius, vs.Corners.TopLeft))
	layer := v.backdrop.Send(selLayer)
	layer.Send(sel("setCornerRadius:"), radius)
	if vs.ContinuousCorners {
		layer.Send(sel("setCornerCurve:"), apple.NSString("continuous"))
	}
	v.backdrop.Send(sel("setClipsToBounds:"), radius > 0)
}
