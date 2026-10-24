//go:build linux && !android

package linux

import (
	"math"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/scrollbar"
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
// does: its shadows and background, its gradients and inset shadows, its
// content (Cairo nodes for images, canvases and text), its children
// (clipped to its shape when it clips), its scroll indicator, then its
// border over the content, as UIKit draws layer borders, and its outline.
// Its opacity, filters and mask take all of it, natives included.
func (w *window) snapshotView(v *view, snap uintptr) {
	s := w.styleOf(v)
	k, _ := v.keyframe()
	opacity := s.Opacity * v.layoutOpacity() * k.Opacity
	if s.Hidden || opacity <= 0 {
		return
	}
	if s.BackfaceHidden && v.parent != nil && facingAway(w.placement(v, 0, 0)) {
		return
	}
	size := v.shownFrame().Size
	width, height := float32(size.Width), float32(size.Height)
	pops := 0
	if opacity < 1 {
		gtkSnapshotPushOpacity(snap, float64(opacity))
		pops++
	}
	e := s.Effects
	if e == nil {
		e = &driver.Effects{}
	}
	pops += pushFilters(snap, e.Filters)
	if e.Mask != nil {
		gtkSnapshotPushMask(snap, maskAlpha)
		appendGradient(snap, e.Mask, width, height)
		gtkSnapshotPop(snap)
		pops++
	}
	sh := newShape(s, width, height)
	filled := s.Background&0xff != 0 || len(e.Gradients) > 0
	shadowed := s.ShadowColor&0xff != 0 && (s.ShadowRadius > 0 || s.ShadowOffset != driver.Point{})
	// A shadow takes the box's shape when the view fills it, else the
	// shape of what it draws.
	contentShadow := shadowed && !filled
	if contentShadow {
		shadow := gskShadow{rgba(s.ShadowColor), float32(s.ShadowOffset.X), float32(s.ShadowOffset.Y),
			float32(s.ShadowRadius) * shadowBlurPerUnit}
		gtkSnapshotPushShadow(snap, &shadow, 1)
		pops++
	}
	if width > 0 && height > 0 {
		if shadowed && filled {
			c := rgba(s.ShadowColor)
			gtkSnapshotAppendOutset(snap, &sh.rect, &c, float32(s.ShadowOffset.X), float32(s.ShadowOffset.Y),
				0, float32(s.ShadowRadius)*shadowBlurPerUnit)
		}
		// The first shadow is on top.
		for i := len(e.Shadows) - 1; i >= 0; i-- {
			if b := e.Shadows[i]; !b.Inset {
				c := rgba(b.Color)
				gtkSnapshotAppendOutset(snap, &sh.rect, &c, float32(b.Offset.X), float32(b.Offset.Y), float32(b.Spread), float32(b.Blur))
			}
		}
		if s.Background&0xff != 0 {
			sh.fill(snap, s.Background)
		}
		if len(e.Gradients) > 0 {
			sh.pushClip(snap)
			for i := len(e.Gradients) - 1; i >= 0; i-- {
				appendGradient(snap, &e.Gradients[i], width, height)
			}
			gtkSnapshotPop(snap)
		}
		w.snapshotInsets(snap, s, &sh, e.Shadows)
	}
	clip := s.Clip || v.kind == driver.KindScroll || v.kind == driver.KindImage || v.kind == driver.KindInput
	if clip {
		sh.pushClip(snap)
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
	if v.kind == driver.KindScroll && w.showsBar(v) {
		snapshotIndicator(v, snap)
	}
	if clip {
		gtkSnapshotPop(snap)
	}
	snapshotBorder(snap, &sh, s)
	if o := s.Outline; o.Visible() && width > 0 && height > 0 {
		sh.stroke(snap, -(o.Offset + o.Width/2), o.Width, o.Color, o.Style)
	}
	for range pops {
		gtkSnapshotPop(snap)
	}
}

// snapshotInsets draws the inset shadows inside the padding box, the
// first on top.
func (w *window) snapshotInsets(snap uintptr, s *driver.ViewStyle, sh *shape, shadows []driver.BoxShadow) {
	inset := false
	for _, b := range shadows {
		inset = inset || b.Inset
	}
	if !inset {
		return
	}
	widths := s.Borders
	if widths == (driver.Edges{}) {
		widths = driver.All(s.BorderWidth)
	}
	inner := driver.InnerRadii(sh.radii, widths)
	padding := gskRoundedRect{bounds: grapheneRect{float32(widths.Left), float32(widths.Top),
		sh.w - float32(widths.Left+widths.Right), sh.h - float32(widths.Top+widths.Bottom)}}
	for i, r := range inner {
		padding.corner[i] = grapheneSize{float32(r[0]), float32(r[1])}
	}
	for i := len(shadows) - 1; i >= 0; i-- {
		if b := shadows[i]; b.Inset {
			c := rgba(b.Color)
			gtkSnapshotAppendInset(snap, &padding, &c, float32(b.Offset.X), float32(b.Offset.Y), float32(b.Spread), float32(b.Blur))
		}
	}
}

// snapshotContent draws what v shows inside its box with Cairo, as the
// backend's images, canvases and Pango text are drawn.
func (w *window) snapshotContent(v *view, s *driver.ViewStyle, snap uintptr) {
	size := v.shownFrame().Size
	width, height := float64(size.Width), float64(size.Height)
	r := rect{w: width, h: height, r: float64(s.CornerRadius)}
	box := grapheneRect{0, 0, float32(width), float32(height)}
	switch {
	case width <= 0 || height <= 0:
	case v.kind == driver.KindImage && (v.source.pic != nil || v.placeholder.pic != nil):
		w.snapshotImage(v, s.TintColor, snap, width, height)
	case v.kind == driver.KindCanvas && v.drawing != nil:
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
		if sh := l.shadow; sh.Color != 0 {
			s := gskShadow{rgba(sh.Color), float32(sh.Offset.X), float32(sh.Offset.Y), float32(sh.Radius) * shadowBlurPerUnit}
			gtkSnapshotPushShadow(snap, &s, 1)
		}
		cr := gtkSnapshotAppendCairo(snap, &grown)
		l.draw(cr, v.content.Origin.X, v.content.Origin.Y)
		cairoDestroy(cr)
		if l.shadow.Color != 0 {
			gtkSnapshotPop(snap)
		}
	}
}

// fillRounded paints c inside r's corners.
func fillRounded(snap uintptr, r *gskRoundedRect, c driver.Color) {
	color := rgba(c)
	if r.corner[0].w > 0 {
		gtkSnapshotPushRoundedClip(snap, r)
		gtkSnapshotAppendColor(snap, &color, &r.bounds)
		gtkSnapshotPop(snap)
		return
	}
	gtkSnapshotAppendColor(snap, &color, &r.bounds)
}

// snapshotBorder draws the border inside the box, with a width and color
// per side when ViewStyle.Borders sets them; GSK meets the sides on the
// diagonals, as CSS does. Dashes, dots and continuous corners are a
// stroked path, in the widest side's width and the top's color.
func snapshotBorder(snap uintptr, sh *shape, s *driver.ViewStyle) {
	if sh.w <= 0 || sh.h <= 0 {
		return
	}
	var widths [4]float32
	var colors [4]gdkRGBA
	widest, top := s.BorderWidth, s.BorderColor
	if s.Borders == (driver.Edges{}) {
		if s.BorderWidth <= 0 || s.BorderColor&0xff == 0 {
			return
		}
		c := rgba(s.BorderColor)
		b := float32(s.BorderWidth)
		widths, colors = [4]float32{b, b, b, b}, [4]gdkRGBA{c, c, c, c}
	} else {
		width := func(points driver.Points, col driver.Color) float32 {
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
		widest, top = max(b.Top, b.Right, b.Bottom, b.Left), c.Top
	}
	if s.BorderStyle == driver.BorderDashed || s.BorderStyle == driver.BorderDotted || sh.continuous {
		sh.stroke(snap, widest/2, widest, top, s.BorderStyle)
		return
	}
	gtkSnapshotAppendBorder(snap, &sh.rect, &widths, &colors)
}

// snapshotIndicator draws a scroll bar's thumb while the pointer is over
// the scroll view or drags the thumb.
func snapshotIndicator(v *view, snap uintptr) {
	thumb, ok := v.bar().Thumb()
	if !ok {
		return
	}
	r := roundedRect(float32(thumb.Origin.X), float32(thumb.Origin.Y), float32(thumb.Size.Width), float32(thumb.Size.Height), float32(scrollbar.Thickness)/2)
	fillRounded(snap, &r, driver.RGBA(0x000000, indicatorAlpha))
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
func setColor(cr uintptr, c driver.Color) {
	r, g, b, a := c.Components()
	cairoSetSourceRGBA(cr, r, g, b, a)
}
