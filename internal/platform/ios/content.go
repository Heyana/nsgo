//go:build ios

package ios

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
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
	drawer := register("NsgoContentDrawer", apple.Class("NSObject"), nil, []objc.MethodDef{{
		Cmd: sel("drawLayer:inContext:"),
		Fn:  drawContent,
	}})
	contentDrawer = objc.ID(drawer).Send(selNew)
	null := objc.ID(apple.Class("NSNull")).Send(sel("null"))
	dict := objc.ID(apple.Class("NSMutableDictionary")).Send(selNew)
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
	// The layer holds the view and the ink of its text past it, and a
	// text's may be larger still (sizeContent).
	h, f := float64(v.frame.Size.Height), v.contentFrame()
	if v.layout != nil {
		f = v.laidFrame
	}
	apple.DrawTopDown(ctx, f.Size.Height, !apple.MsgBool(layer, selContentsAreFlipped), func(baseUp bool) {
		apple.TranslateCTM(ctx, -f.Origin.X, -f.Origin.Y)
		if v.drawing != nil {
			ops := v.surface.animator.CanvasOps(v.id, v.drawing)
			apple.Play(ctx, ops, apple.CanvasHost{
				Ratio: v.contentScale(), Redraw: v.redraw, Owner: v, BaseUp: baseUp})
			v.played = append(v.played[:0], ops...)
			return
		}
		box := v.box
		v.layout.DrawBackgrounds(ctx, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y), 0, 0))
		v.drawSelection(ctx)
		v.layout.Draw(ctx, h, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y),
			float64(box.Size.Width), float64(box.Size.Height)), baseUp)
		if v.animated.Has(driver.PropTextColor) {
			// A bound color paints over the whole text.
			apple.Tint(ctx, f, v.animated.TextColor)
		}
	})
}

// newContent gives text or canvas view v its content layer.
func (v *view) newContent() {
	layer := objc.ID(apple.Class("CALayer")).Send(selNew)
	layer.Send(sel("setActions:"), contentActions)
	layer.Send(sel("setDelegate:"), contentDrawer)
	layer.Send(sel("setNeedsDisplayOnBoundsChange:"), true)
	// Canvases are drawn on Core Animation's threads: paths and gradients
	// cost more than glyphs.
	layer.Send(sel("setDrawsAsynchronously:"), v.kind == driver.KindCanvas)
	layer.Send(sel("setContentsScale:"), v.contentScale())
	layer.Send(sel("setZPosition:"), apple.Z(contentZ))
	v.native.Send(selLayer).Send(sel("addSublayer:"), layer)
	layer.Send(selRelease)
	v.contentLayer, v.laidFrame, v.needFrame = layer, apple.CGRect{}, apple.CGRect{}
	drawers[layer] = v
}

// redrawAnimated draws again what a canvas's animated arguments changed,
// as on macOS: the area apple.Damaged finds, or all of it.
func (v *view) redrawAnimated() {
	if v.contentLayer == 0 || v.drawing == nil || len(v.played) == 0 {
		v.redraw()
		return
	}
	area, whole := apple.Damaged(v.played, v.surface.animator.CanvasOps(v.id, v.drawing))
	switch {
	case whole:
		v.redraw()
	case area.Size.Width > 0:
		v.contentLayer.Send(sel("setNeedsDisplayInRect:"), area)
	}
}

// redraw draws v's content again.
func (v *view) redraw() {
	if v.contentLayer != 0 {
		v.contentLayer.Send(selSetNeedsDisplayL)
	}
}

// sizeContent fits v's content layer to its frame and its text's ink. A
// frame as before is not sent: a table's cells keep theirs as their texts
// change, and the CGRect costs purego a reflected call each time. A
// text's layer keeps a larger frame (apple.KeptContent) and draws again
// within it, unless its view is rounded: Core Animation then blends the
// corners' edges a level apart.
func (v *view) sizeContent() {
	if v.contentLayer == 0 {
		return
	}
	f := v.contentFrame()
	keeps := v.layout != nil && !v.style.Rounded()
	if f == v.needFrame && (keeps || f == v.laidFrame) {
		return
	}
	v.needFrame = f
	if keeps && v.laidFrame != (apple.CGRect{}) {
		if f = apple.KeptContent(v.laidFrame, f); f == v.laidFrame {
			v.redraw()
			return
		}
	}
	if f != v.laidFrame {
		v.laidFrame = f
		apple.WithoutAnimation(func() {
			v.contentLayer.Send(selSetFrame, f)
		})
	}
}

// contentFrame is the frame of v's content layer: its bounds, grown to
// hold its text's ink past them.
func (v *view) contentFrame() apple.CGRect {
	w, h := float64(v.frame.Size.Width), float64(v.frame.Size.Height)
	if v.layout == nil {
		return apple.Rect(0, 0, w, h)
	}
	b := v.box
	return v.layout.ContentFrame(w, h, apple.Rect(float64(b.Origin.X), float64(b.Origin.Y), float64(b.Size.Width), float64(b.Size.Height)))
}

// forgetContent drops v's content layer, which goes with its view.
func (v *view) forgetContent() {
	if v.contentLayer != 0 {
		delete(drawers, v.contentLayer)
		v.contentLayer = 0
	}
}

// A view's backdrop is a UIVisualEffectView under its children with the
// system material its style names for a plain blur or a named material,
// Liquid Glass on iOS 26 and later when it asks for glass, or a
// CABackdropLayer with Core Animation's gaussianBlur and colorMatrix filters
// when color filters are set alone or over an unnamed-material blur, matching
// macOS. The background shows over the backdrop, where CSS has it, in a tint
// view over the CABackdropLayer (and left out of the view's own layer, which
// the backdrop layer would otherwise filter) or in UIVisualEffectView's
// contentView.

// hasGlass reports whether UIKit has Liquid Glass (iOS 26 and later).
func hasGlass() bool { return apple.Class("UIGlassEffect") != 0 }

// fitBackdrop lays v's backdrop out for style vs at size w by h.
func (v *view) fitBackdrop(vs *driver.ViewStyle, w, h float64) {
	apple.FitBackdrop(v.backdrop, vs, w, h)
	if v.backdropLayer != 0 || v.tint != 0 {
		bounds := objc.Send[apple.CGRect](v.backdrop, selBounds)
		if v.backdropLayer != 0 {
			// A layer of its own would ease to its frame on its own.
			apple.WithoutAnimation(func() { v.backdropLayer.Send(selSetFrame, bounds) })
		}
		if v.tint != 0 {
			v.tint.Send(selSetFrame, bounds)
		}
	}
}

// setBackdrop gives v the backdrop its style asks for, or takes it away.
func (v *view) setBackdrop(vs *driver.ViewStyle) {
	mode := chooseBackdrop(vs, hasGlass())
	if mode == backdropNone || mode != v.backdropKind {
		if v.backdrop != 0 {
			apple.MoveBorders(v.native.Send(selLayer), 0)
			v.backdrop.Send(sel("removeFromSuperview"))
			v.backdrop, v.backdropLayer, v.tint = 0, 0, 0
		}
		v.backdropKind = backdropNone
		if mode == backdropNone {
			return
		}
	}
	const flexible = 2 | 16 // width and height
	if mode == backdropFiltered {
		if v.backdrop == 0 {
			v.backdrop = objc.ID(apple.Class("UIView")).Send(selAlloc).Send(sel("initWithFrame:"), objc.Send[apple.CGRect](v.native, selBounds))
			v.backdrop.Send(sel("setAutoresizingMask:"), flexible)
			v.backdrop.Send(sel("setUserInteractionEnabled:"), false)
			v.backdrop.Send(selSetOpaque, false)
			v.backdropLayer = objc.ID(apple.Class("CABackdropLayer")).Send(selNew)
			v.backdrop.Send(selLayer).Send(sel("addSublayer:"), v.backdropLayer)
			v.backdropLayer.Send(selRelease)
			v.tint = objc.ID(apple.Class("UIView")).Send(selAlloc).Send(sel("initWithFrame:"), objc.Send[apple.CGRect](v.backdrop, selBounds))
			v.tint.Send(sel("setAutoresizingMask:"), flexible)
			v.tint.Send(sel("setUserInteractionEnabled:"), false)
			v.tint.Send(selSetOpaque, false)
			v.backdrop.Send(selAddSubview, v.tint)
			v.tint.Send(selRelease)
			v.native.Send(sel("insertSubview:atIndex:"), v.backdrop, 0)
			v.backdrop.Send(selRelease)
			v.backdropKind = mode
		}
		apple.WithoutAnimation(func() {
			v.backdropLayer.Send(sel("setFilters:"), apple.BackdropFilters(vs.Effects.BackdropFilters))
		})
		b := objc.Send[apple.CGRect](v.native, selBounds)
		v.fitBackdrop(vs, b.Size.Width, b.Size.Height)
		v.tint.Send(sel("setBackgroundColor:"), uiColor(vs.Background))
		apple.MoveBorders(v.native.Send(selLayer), v.borderHost())
		return
	}
	var effect objc.ID
	if mode == backdropGlass {
		effect = objc.ID(apple.Class("UIGlassEffect")).Send(sel("effectWithStyle:"), int(vs.Glass)-1)
	} else {
		effect = objc.ID(apple.Class("UIBlurEffect")).Send(sel("effectWithStyle:"), blurStyle(vs.Material))
	}
	if v.backdrop == 0 {
		v.backdrop = objc.ID(apple.Class("UIVisualEffectView")).Send(selAlloc).Send(sel("initWithEffect:"), effect)
		v.backdrop.Send(selSetFrame, objc.Send[apple.CGRect](v.native, selBounds))
		v.backdrop.Send(sel("setAutoresizingMask:"), flexible)
		v.backdrop.Send(sel("setUserInteractionEnabled:"), false)
		v.native.Send(sel("insertSubview:atIndex:"), v.backdrop, 0)
		v.backdrop.Send(selRelease)
		v.backdropKind = mode
	} else {
		v.backdrop.Send(sel("setEffect:"), effect)
	}
	b := objc.Send[apple.CGRect](v.native, selBounds)
	v.fitBackdrop(vs, b.Size.Width, b.Size.Height)
	if mode != backdropGlass {
		v.backdrop.Send(sel("contentView")).Send(sel("setBackgroundColor:"), uiColor(vs.Background))
	}
	apple.MoveBorders(v.native.Send(selLayer), v.borderHost())
}

// borderHost is the layer v's per-side borders go in, above its backdrop and below its children: the glass's own layer, the tint over a filter backdrop, else the blur's content view, which draws the background; zero, v's own layer, without a backdrop.
func (v *view) borderHost() objc.ID {
	switch v.backdropKind {
	case backdropGlass:
		return v.backdrop.Send(selLayer)
	case backdropFiltered:
		return v.tint.Send(selLayer)
	case backdropMaterial:
		return v.backdrop.Send(sel("contentView")).Send(selLayer)
	}
	return 0
}
