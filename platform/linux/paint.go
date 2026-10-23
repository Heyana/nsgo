//go:build linux && !android

package linux

import (
	"math"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/platform/internal/scrollbar"
)

// painter draws a view's Cairo content (an image) into a context.
type painter struct {
	w  *window
	cr uintptr
}

const (
	operatorSourceOver = 2
	operatorClear      = 0
)

// snapshotView draws v into its widget's snapshot, as NSH's NshView
// does: its shadow and background, its content (Cairo nodes for images,
// canvases and text), its children (clipped to its rounded bounds when
// it clips), its scroll indicator, then its border over the content, as
// UIKit draws layer borders. The view's opacity fades all of it, natives
// included.
func (w *window) snapshotView(v *view, snap uintptr) {
	s := w.styleOf(v)
	opacity := s.Opacity * v.layoutOpacity()
	if s.Hidden || opacity <= 0 {
		return
	}
	size := v.shownFrame().Size
	width, height := float32(size.Width), float32(size.Height)
	if opacity < 1 {
		gtkSnapshotPushOpacity(snap, float64(opacity))
	}
	bounds := roundedRect(0, 0, width, height, float32(s.CornerRadius))
	filled := s.Background&0xff != 0
	shadowed := s.ShadowColor&0xff != 0 && (s.ShadowRadius > 0 || s.ShadowOffset != core.Point{})
	// A shadow takes the box's shape when the view fills it, else the
	// shape of what it draws.
	contentShadow := shadowed && !filled
	if contentShadow {
		sh := gskShadow{rgba(s.ShadowColor), float32(s.ShadowOffset.X), float32(s.ShadowOffset.Y),
			float32(s.ShadowRadius) * shadowBlurPerUnit}
		gtkSnapshotPushShadow(snap, &sh, 1)
	}
	if width > 0 && height > 0 {
		if shadowed && filled {
			c := rgba(s.ShadowColor)
			gtkSnapshotAppendOutset(snap, &bounds, &c, float32(s.ShadowOffset.X), float32(s.ShadowOffset.Y),
				0, float32(s.ShadowRadius)*shadowBlurPerUnit)
		}
		if filled {
			fillRounded(snap, &bounds, s.Background)
		}
	}
	clip := s.Clip || v.kind == core.KindScroll || v.kind == core.KindImage || v.kind == core.KindInput
	if clip {
		gtkSnapshotPushRoundedClip(snap, &bounds)
	}
	w.snapshotContent(v, s, snap)
	for c := gtkWidgetGetFirstChild(v.widget); c != 0; c = gtkWidgetGetNextSibling(c) {
		// A view inserted since the last layout has no allocation until
		// the next frame's; a snapshot taken between skips it.
		if r, ok := lookupWidget(c); ok && r.v != nil && gtkWidgetGetWidth(c) == 0 && gtkWidgetGetHeight(c) == 0 &&
			r.v.frame.Size.Width >= 1 && r.v.frame.Size.Height >= 1 {
			continue
		}
		gtkWidgetSnapshotChild(v.widget, c, snap)
	}
	if v.kind == core.KindScroll && w.showsBar(v) {
		snapshotIndicator(v, snap)
	}
	if clip {
		gtkSnapshotPop(snap)
	}
	snapshotBorder(snap, &bounds, s)
	if contentShadow {
		gtkSnapshotPop(snap)
	}
	if opacity < 1 {
		gtkSnapshotPop(snap)
	}
}

// snapshotContent draws what v shows inside its box with Cairo, as the
// backend's images, canvases and Pango text are drawn.
func (w *window) snapshotContent(v *view, s *core.ViewStyle, snap uintptr) {
	size := v.shownFrame().Size
	width, height := float64(size.Width), float64(size.Height)
	r := rect{w: width, h: height, r: float64(s.CornerRadius)}
	box := grapheneRect{0, 0, float32(width), float32(height)}
	switch {
	case width <= 0 || height <= 0:
	case v.kind == core.KindImage && (v.source.pic != nil || v.placeholder.pic != nil):
		cr := gtkSnapshotAppendCairo(snap, &box)
		(&painter{w: w, cr: cr}).drawImage(v, s.TintColor, r)
		cairoDestroy(cr)
	case v.kind == core.KindCanvas && v.drawing != nil:
		cr := gtkSnapshotAppendCairo(snap, &box)
		w.playCanvas(v.drawing, cr, r)
		cairoDestroy(cr)
	}
	if l, ok := v.text.(*textLayout); ok && l != nil && l.text != "" {
		// Glyphs may paint a little outside their view, as before.
		x, y := float32(v.content.Origin.X), float32(v.content.Origin.Y)
		grown := grapheneRect{min(0, x) - textOverhang, min(0, y) - textOverhang,
			max(float32(width), x+float32(l.width)) - min(0, x) + 2*textOverhang,
			max(float32(height), y+float32(l.height)) - min(0, y) + 2*textOverhang}
		cr := gtkSnapshotAppendCairo(snap, &grown)
		l.draw(cr, v.content.Origin.X, v.content.Origin.Y)
		cairoDestroy(cr)
	}
}

// fillRounded paints c inside r's corners.
func fillRounded(snap uintptr, r *gskRoundedRect, c core.Color) {
	color := rgba(c)
	if r.corner[0].w > 0 {
		gtkSnapshotPushRoundedClip(snap, r)
		gtkSnapshotAppendColor(snap, &color, &r.bounds)
		gtkSnapshotPop(snap)
		return
	}
	gtkSnapshotAppendColor(snap, &color, &r.bounds)
}

// snapshotBorder draws the border inside outline, with a width and color
// per side when ViewStyle.Borders sets them; GSK meets the sides on the
// diagonals, as CSS does.
func snapshotBorder(snap uintptr, outline *gskRoundedRect, s *core.ViewStyle) {
	if outline.bounds.w <= 0 || outline.bounds.h <= 0 {
		return
	}
	var widths [4]float32
	var colors [4]gdkRGBA
	if s.Borders == (core.Edges{}) {
		if s.BorderWidth <= 0 || s.BorderColor&0xff == 0 {
			return
		}
		c := rgba(s.BorderColor)
		b := float32(s.BorderWidth)
		widths, colors = [4]float32{b, b, b, b}, [4]gdkRGBA{c, c, c, c}
	} else {
		width := func(points core.Points, col core.Color) float32 {
			if points <= 0 || col&0xff == 0 {
				return 0
			}
			return float32(math.Max(float64(points), 1))
		}
		b, c := s.Borders, s.BorderColors
		widths = [4]float32{width(b.Top, c.Top), width(b.Right, c.Right), width(b.Bottom, c.Bottom), width(b.Left, c.Left)}
		if widths == ([4]float32{}) {
			return
		}
		colors = [4]gdkRGBA{rgba(c.Top), rgba(c.Right), rgba(c.Bottom), rgba(c.Left)}
	}
	gtkSnapshotAppendBorder(snap, outline, &widths, &colors)
}

// snapshotIndicator draws a scroll bar's thumb while the pointer is over
// the scroll view or drags the thumb.
func snapshotIndicator(v *view, snap uintptr) {
	thumb, ok := v.bar().Thumb()
	if !ok {
		return
	}
	r := roundedRect(float32(thumb.Origin.X), float32(thumb.Origin.Y), float32(thumb.Size.Width), float32(thumb.Size.Height), float32(scrollbar.Thickness)/2)
	fillRounded(snap, &r, core.RGBA(0x000000, indicatorAlpha))
}

// paint draws the surface as GTK shows it, natives included, into cr:
// the window's pixels without reading the screen back.
func (w *window) paint(cr uintptr) {
	width, height := gtkWidgetGetWidth(w.surface), gtkWidgetGetHeight(w.surface)
	paintable := gtkWidgetPaintableNew(w.surface)
	snap := gtkSnapshotNew()
	gdkPaintableSnapshot(paintable, snap, float64(width), float64(height))
	if node := gtkSnapshotFreeToNode(snap); node != 0 {
		gskRenderNodeDraw(node, cr)
		gskRenderNodeUnref(node)
	}
	gObjectUnref(paintable)
}

// rect is a rectangle in points.
type rect struct {
	x, y, w, h, r float64
}

// roundRect adds a rounded rectangle to the current path.
func roundRect(cr uintptr, r rect) {
	if r.w <= 0 || r.h <= 0 {
		return
	}
	radius := math.Min(math.Min(r.r, r.w/2), r.h/2)
	if radius <= 0 {
		cairoRectangle(cr, r.x, r.y, r.w, r.h)
		return
	}
	x, y, w, h := r.x, r.y, r.w, r.h
	cairoNewSubPath(cr)
	cairoArc(cr, x+w-radius, y+radius, radius, -math.Pi/2, 0)
	cairoArc(cr, x+w-radius, y+h-radius, radius, 0, math.Pi/2)
	cairoArc(cr, x+radius, y+h-radius, radius, math.Pi/2, math.Pi)
	cairoArc(cr, x+radius, y+radius, radius, math.Pi, 3*math.Pi/2)
	cairoClosePath(cr)
}

// setColor sets c as Cairo's source.
func setColor(cr uintptr, c core.Color) {
	r, g, b, a := c.Components()
	cairoSetSourceRGBA(cr, r, g, b, a)
}
