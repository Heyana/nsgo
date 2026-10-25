//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/apple"
)

// Text and canvas views draw into a content layer of their own: a plain
// CALayer whose delegate draws in drawLayer:inContext:, below the view's
// sublayers as a backing store's contents would be. No view implements
// drawRect:, so AppKit gives none a backing store and never asks one to
// draw. AppKit's display pass recorded every drawRect: into a display list
// and replayed it, and scrolling a clip view marked every drawRect: view
// of its document for display. A content layer draws again only when its
// content or its size changes, and only while some of it can be seen.

var (
	// contentDrawer is the delegate of every content layer, which does
	// not retain it.
	contentDrawer objc.ID
	// contentActions turns off implicit animations of content layers.
	contentActions objc.ID
	// drawers maps content layers to their views.
	drawers = map[objc.ID]*view{}

	selContentsAreFlipped       = sel("contentsAreFlipped")
	selSetContentsScale         = sel("setContentsScale:")
	selSetNeedsDisplayLayer     = sel("setNeedsDisplay")
	selSetNeedsDisplayOnResize  = sel("setNeedsDisplayOnBoundsChange:")
	selSetZPosition, selActions = sel("setZPosition:"), sel("setActions:")
)

// contentZ puts a content layer below the view's other sublayers (its
// children and its border rings), where a backing store's contents draw.
const contentZ = -1

func registerContent() {
	drawer := register("NsgoContentDrawer", objc.GetClass("NSObject"), nil, []objc.MethodDef{{
		Cmd: sel("drawLayer:inContext:"),
		Fn:  drawContent,
	}})
	contentDrawer = objc.ID(drawer).Send(selNew)
	null := objc.ID(objc.GetClass("NSNull")).Send(sel("null"))
	keys := []string{"contents", "bounds", "position", "frame", "hidden", "contentsScale",
		"onOrderIn", "onOrderOut", "sublayers"}
	dict := objc.ID(objc.GetClass("NSMutableDictionary")).Send(selNew)
	for _, k := range keys {
		dict.Send(sel("setObject:forKey:"), null, apple.NSString(k))
	}
	contentActions = dict
}

// drawContent draws a view's text or canvas into its content layer, top
// down as a flipped view's drawRect: did.
func drawContent(_ objc.ID, _ objc.SEL, layer objc.ID, ctx uintptr) {
	v := drawers[layer]
	if v == nil || ctx == 0 || (v.layout == nil && v.drawing == nil) {
		return
	}
	// The layer is as large as the view (sizeContent).
	h := float64(v.frame.Size.Height)
	apple.DrawTopDown(ctx, h, !apple.MsgBool(layer, selContentsAreFlipped), func() {
		if v.drawing != nil {
			apple.Play(ctx, v.surface.animator.CanvasOps(v.id, v.drawing), apple.CanvasHost{
				Ratio: float64(v.surface.env.PixelRatio), Redraw: v.redraw, Owner: v})
			return
		}
		box := v.box
		v.layout.DrawBackgrounds(ctx, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y), 0, 0))
		v.drawSelection(ctx)
		v.layout.Draw(ctx, h, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y),
			float64(box.Size.Width), float64(box.Size.Height)))
		if v.animated.Has(driver.PropTextColor) {
			// A bound color paints over the whole text.
			apple.Tint(ctx, apple.Rect(0, 0, float64(v.frame.Size.Width), h), v.animated.TextColor)
		}
	})
}

// redraw draws v's content again: now if some of v can be seen, else
// once it shows (surface.revisit). Lists keep rows out of sight, and
// hidden screens stay mounted.
func (v *view) redraw() {
	if v.contentLayer == 0 && v.layout == nil && v.drawing == nil && v.shader == nil {
		return
	}
	if v.shown() {
		v.display()
		return
	}
	if !v.stale {
		v.stale = true
		v.surface.stale = append(v.surface.stale, v)
	}
}

// drawLayer returns v's content layer, made on first use at v's size.
func (v *view) drawLayer() objc.ID {
	if v.contentLayer != 0 {
		return v.contentLayer
	}
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	layer.Send(selActions, contentActions)
	layer.Send(selSetDelegate, contentDrawer)
	layer.Send(selSetNeedsDisplayOnResize, true)
	// Canvases are drawn on Core Animation's threads, as NSH's are: paths
	// and gradients cost more than glyphs.
	layer.Send(sel("setDrawsAsynchronously:"), v.drawing != nil)
	layer.Send(selSetContentsScale, float64(v.surface.env.PixelRatio))
	layer.Send(selSetZPosition, float64(contentZ))
	layer.Send(selSetFrame, apple.Rect(0, 0, float64(v.frame.Size.Width), float64(v.frame.Size.Height)))
	v.native.Send(selLayer).Send(sel("addSublayer:"), layer)
	layer.Send(selRelease) // its host keeps it
	v.contentLayer = layer
	drawers[layer] = v
	return layer
}

// shown reports whether some of v can be seen: in the tree, neither it
// nor an ancestor hidden, and not outside what the scroll views around it
// show. It reads the batches' state, as NSH's does, rather than ask
// AppKit for a window, hidden ancestors and a visible CGRect for every
// text a frame changes. Transforms are left out: one at most draws a view
// early or late.
func (v *view) shown() bool {
	s := v.surface
	// r is v's placed frame in the space of n's parent's content.
	r := v.frame
	r.Origin.X += v.shift.X
	r.Origin.Y += v.shift.Y
	for n := v; ; n = n.up {
		if n == nil || s.views[n.id] != n || n.style.Hidden {
			return false
		}
		if n.id == s.root {
			return true
		}
		p := n.up
		if p == nil {
			return false
		}
		if p.sc != nil {
			// The content shows from the offset, as large as the view.
			o, size := p.sc.shown, p.frame.Size
			x0, y0 := max(r.Origin.X, o.X), max(r.Origin.Y, o.Y)
			x1 := min(r.Origin.X+r.Size.Width, o.X+size.Width)
			y1 := min(r.Origin.Y+r.Size.Height, o.Y+size.Height)
			if x1 <= x0 || y1 <= y0 {
				return false
			}
			r = driver.Bounds{Origin: driver.Point{X: x0 - o.X, Y: y0 - o.Y},
				Size: driver.Size{Width: x1 - x0, Height: y1 - y0}}
		}
		r.Origin.X += p.frame.Origin.X + p.shift.X
		r.Origin.Y += p.frame.Origin.Y + p.shift.Y
	}
}

// revisit draws the views whose content changed out of sight and that
// show now: after batches, scrolls and layout animations. Deleted views
// are dropped; views still out of sight wait.
func (s *surface) revisit() {
	if len(s.stale) == 0 {
		return
	}
	waiting := s.stale[:0]
	for _, v := range s.stale {
		if s.views[v.id] != v {
			v.stale = false
			continue
		}
		if !v.shown() {
			waiting = append(waiting, v)
			continue
		}
		v.stale = false
		v.display()
	}
	clear(s.stale[len(waiting):])
	s.stale = waiting
}

// display draws v's content, which can be seen: a content layer when Core
// Animation next displays, a shader once the batch or the frame is
// applied.
func (v *view) display() {
	if v.shader != nil {
		v.surface.shadeLater(v)
		return
	}
	apple.Msg(v.drawLayer(), selSetNeedsDisplayLayer)
}

// sizeContent keeps v's content layer over its bounds; a new size draws
// it again (needsDisplayOnBoundsChange).
func (v *view) sizeContent() {
	if v.shader != nil {
		v.sizeShader()
	}
	if v.contentLayer == 0 {
		return
	}
	v.contentLayer.Send(selSetFrame, apple.Rect(0, 0, float64(v.frame.Size.Width), float64(v.frame.Size.Height)))
}

// rescale draws every content layer again at the window's backing scale.
func (s *surface) rescale() {
	for _, v := range s.views {
		if v != nil && v.shader != nil {
			v.sizeShader()
		}
		if v == nil || v.contentLayer == 0 {
			continue
		}
		v.contentLayer.Send(selSetContentsScale, float64(s.env.PixelRatio))
		v.redraw()
	}
}

// forgetContent drops v's content layer, which goes with its view.
func (v *view) forgetContent() {
	if v.contentLayer != 0 {
		delete(drawers, v.contentLayer)
	}
}
