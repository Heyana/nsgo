//go:build linux

package linux

import (
	"math"

	"github.com/oneo-me/nsgo/core"
)

// painter draws one frame into a cairo context.
type painter struct {
	w  *window
	cr uintptr
}

const (
	operatorSourceOver = 2
	operatorClear      = 0
	fillRuleEvenOdd    = 1
)

// paint fills the surface and draws the tree.
func (w *window) paint(cr uintptr, size core.Size) {
	bg := core.RGB(0xffffff)
	if root := w.view(w.root); root != nil {
		bg = background(root)
	}
	setColor(cr, bg)
	cairoRectangle(cr, 0, 0, float64(size.Width), float64(size.Height))
	cairoFill(cr)
	if root := w.view(w.root); root != nil {
		p := &painter{w: w, cr: cr}
		p.draw(root, 0, 0)
	}
}

// draw paints v and its children; x and y are the origin of v's parent's
// content in the window.
func (p *painter) draw(v *view, x, y core.Points) {
	s := p.w.styleOf(v)
	if s.Hidden || s.Opacity <= 0 {
		return
	}
	if s.Opacity < 1 {
		cairoSave(p.cr)
		cairoPushGroup(p.cr)
		p.drawView(v, s, x, y)
		cairoPopGroupToSource(p.cr)
		cairoIdentityMatrix(p.cr)
		cairoPaintWithAlpha(p.cr, float64(s.Opacity))
		cairoRestore(p.cr)
		return
	}
	p.drawView(v, s, x, y)
}

// drawView paints one view with s.
func (p *painter) drawView(v *view, s *core.ViewStyle, x, y core.Points) {
	w := p.w
	x, y = x+v.frame.Origin.X, y+v.frame.Origin.Y
	width, height := float64(v.frame.Size.Width), float64(v.frame.Size.Height)
	radius := float64(s.CornerRadius)
	r := rect{x: float64(x), y: float64(y), w: width, h: height, r: radius}

	if s.Transform != (core.Transform{}) {
		cairoSave(p.cr)
		p.applyTransform(s.Transform, r)
		defer cairoRestore(p.cr)
	}
	p.drawShadow(r, s)
	p.fill(r, s.Background)
	switch v.kind {
	case core.KindImage:
		if s.Clip && radius > 0 {
			cairoSave(p.cr)
			roundRect(p.cr, r)
			cairoClip(p.cr)
			p.drawImage(v, s.TintColor, r)
			cairoRestore(p.cr)
		} else {
			p.drawImage(v, s.TintColor, r)
		}
	case core.KindCanvas:
		if v.drawing != nil {
			w.playCanvas(v.drawing, p.cr, r)
		}
	}

	if l, ok := v.text.(*textLayout); ok && l != nil {
		l.draw(p.cr, x+v.content.Origin.X, y+v.content.Origin.Y)
	}

	if len(v.children) > 0 {
		if s.Clip || v.kind == core.KindScroll {
			cairoSave(p.cr)
			roundRect(p.cr, r)
			cairoClip(p.cr)
			for _, c := range v.children {
				p.draw(c, x-v.offset.X, y-v.offset.Y)
			}
			cairoRestore(p.cr)
		} else {
			for _, c := range v.children {
				p.draw(c, x-v.offset.X, y-v.offset.Y)
			}
		}
	}
	if v.kind == core.KindScroll && !v.scroll.HideIndicator {
		p.drawIndicator(v, x, y)
	}
	p.drawBorder(r, s)
}

// applyTransform turns the drawing around r's centre, translates it and
// scales it, as CSS does.
func (p *painter) applyTransform(t core.Transform, r rect) {
	sx, sy := t.Scales()
	cx, cy := r.x+r.w/2, r.y+r.h/2
	cairoTranslate(p.cr, cx+float64(t.TranslateX), cy+float64(t.TranslateY))
	cairoRotate(p.cr, float64(t.Rotate)*math.Pi/180)
	cairoScale(p.cr, float64(sx), float64(sy))
	cairoTranslate(p.cr, -cx, -cy)
}

// drawShadow paints an approximation of a blurred shadow under r. Cairo
// has no shadow filter, so a few growing rounded rectangles fade out.
func (p *painter) drawShadow(r rect, s *core.ViewStyle) {
	if s.ShadowColor&0xff == 0 || s.ShadowRadius <= 0 && s.ShadowOffset == (core.Point{}) {
		return
	}
	spread := float64(s.ShadowRadius)
	layers := 6
	if spread <= 0 {
		layers = 1
	}
	hex := uint32(s.ShadowColor) >> 8
	baseAlpha := float64(s.ShadowColor&0xff) / 255
	for i := layers; i >= 1; i-- {
		grow := spread * float64(i) / float64(layers)
		alpha := baseAlpha / float64(layers)
		c := core.RGBA(hex, float32(alpha))
		rr := rect{
			x: r.x + float64(s.ShadowOffset.X) - grow,
			y: r.y + float64(s.ShadowOffset.Y) - grow,
			w: r.w + 2*grow,
			h: r.h + 2*grow,
			r: max(r.r+grow, 0),
		}
		p.fill(rr, c)
	}
}

// fill paints r with c's visible alpha, inside its corners.
func (p *painter) fill(r rect, c core.Color) {
	if c&0xff == 0 || r.w <= 0 || r.h <= 0 {
		return
	}
	setColor(p.cr, c)
	roundRect(p.cr, r)
	cairoFill(p.cr)
}

// drawBorder paints the border inside r, with a width and color per side
// when ViewStyle.Borders sets them.
func (p *painter) drawBorder(r rect, s *core.ViewStyle) {
	if s.Borders == (core.Edges{}) {
		if s.BorderWidth <= 0 || s.BorderColor&0xff == 0 {
			return
		}
		p.ring(r, s.BorderWidth, s.BorderColor)
		return
	}
	b, c := s.Borders, s.BorderColors
	width := func(points core.Points, col core.Color) float64 {
		if points <= 0 || col&0xff == 0 {
			return 0
		}
		return math.Max(float64(points), 1)
	}
	top, right := width(b.Top, c.Top), width(b.Right, c.Right)
	bottom, left := width(b.Bottom, c.Bottom), width(b.Left, c.Left)
	if top == 0 && right == 0 && bottom == 0 && left == 0 {
		return
	}
	// Each side is the ring between r and its inside, cut to the side's
	// trapezoid, so the sides meet on the diagonals as CSS's do.
	in := rect{x: r.x + left, y: r.y + top, w: math.Max(r.w-left-right, 0), h: math.Max(r.h-top-bottom, 0),
		r: math.Max(r.r-(left+top+right+bottom)/4, 0)}
	sides := []struct {
		width float64
		color core.Color
		pts   [4][2]float64
	}{
		{top, c.Top, [4][2]float64{{r.x, r.y}, {r.x + r.w, r.y}, {in.x + in.w, in.y}, {in.x, in.y}}},
		{right, c.Right, [4][2]float64{{r.x + r.w, r.y}, {r.x + r.w, r.y + r.h}, {in.x + in.w, in.y + in.h}, {in.x + in.w, in.y}}},
		{bottom, c.Bottom, [4][2]float64{{r.x + r.w, r.y + r.h}, {r.x, r.y + r.h}, {in.x, in.y + in.h}, {in.x + in.w, in.y + in.h}}},
		{left, c.Left, [4][2]float64{{r.x, r.y + r.h}, {r.x, r.y}, {in.x, in.y}, {in.x, in.y + in.h}}},
	}
	for _, side := range sides {
		if side.width == 0 || side.color&0xff == 0 {
			continue
		}
		cairoSave(p.cr)
		roundRect(p.cr, r)
		cairoClip(p.cr)
		cairoMoveTo(p.cr, side.pts[0][0], side.pts[0][1])
		for _, pt := range side.pts[1:] {
			cairoLineTo(p.cr, pt[0], pt[1])
		}
		cairoClosePath(p.cr)
		cairoClip(p.cr)
		p.ringPath(r, in)
		setColor(p.cr, side.color)
		cairoSetFillRule(p.cr, fillRuleEvenOdd)
		cairoFill(p.cr)
		cairoSetFillRule(p.cr, 0)
		cairoRestore(p.cr)
	}
}

// ring paints the border of width points inside r.
func (p *painter) ring(r rect, width core.Points, c core.Color) {
	in := rect{x: r.x + float64(width), y: r.y + float64(width),
		w: math.Max(r.w-2*float64(width), 0), h: math.Max(r.h-2*float64(width), 0),
		r: math.Max(r.r-float64(width), 0)}
	p.ringPath(r, in)
	setColor(p.cr, c)
	cairoSetFillRule(p.cr, fillRuleEvenOdd)
	cairoFill(p.cr)
	cairoSetFillRule(p.cr, 0)
}

// ringPath adds outer and inner rounded rectangles to the current path;
// the even-odd fill rule leaves the ring between them.
func (p *painter) ringPath(outer, inner rect) {
	roundRect(p.cr, outer)
	if inner.w > 0 && inner.h > 0 {
		roundRect(p.cr, inner)
	}
}

// drawIndicator paints a scroll bar's thumb along v's scrolling edge; x
// and y are v's origin in the window.
func (p *painter) drawIndicator(v *view, x, y core.Points) {
	const thickness, inset, minLength = 4.0, 2.0, 24.0
	if v.size == (core.Size{}) {
		return
	}
	frame := rect{x: float64(x), y: float64(y), w: float64(v.frame.Size.Width), h: float64(v.frame.Size.Height)}
	var thumb rect
	if v.scroll.Horizontal {
		if v.size.Width <= 0 || v.size.Width <= v.frame.Size.Width {
			return
		}
		length := math.Max(float64(v.frame.Size.Width)*float64(v.frame.Size.Width)/float64(v.size.Width), minLength)
		at := float64(v.offset.X) / float64(v.size.Width-v.frame.Size.Width) * (float64(v.frame.Size.Width) - length)
		thumb = rect{x: frame.x + at, y: frame.y + frame.h - thickness - inset, w: length, h: thickness, r: thickness / 2}
	} else {
		if v.size.Height <= 0 || v.size.Height <= v.frame.Size.Height {
			return
		}
		length := math.Max(float64(v.frame.Size.Height)*float64(v.frame.Size.Height)/float64(v.size.Height), minLength)
		at := float64(v.offset.Y) / float64(v.size.Height-v.frame.Size.Height) * (float64(v.frame.Size.Height) - length)
		thumb = rect{x: frame.x + frame.w - thickness - inset, y: frame.y + at, w: thickness, h: length, r: thickness / 2}
	}
	p.fill(thumb, core.RGBA(0x000000, 0.35))
}

// rect is a rectangle in the window's points.
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
