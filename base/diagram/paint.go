package diagram

// paint follows limumygo, limu's drawing on MyGo (github.com/firede/limu/limumygo, MIT): each item's parts, the markers at its ends and its label, in the colors and measures the look settles for its style. NSGo's canvas strokes, caps, joins and dashes paths as SVG does and strokes text, so a box is a rounded rectangle path, a line is stroked whole and a label's halo is its text stroked, where MyGo needs ways around them.

import (
	"math"

	"github.com/firede/limu/scene"
	"github.com/firede/limu/style"
	"github.com/firede/limu/text"

	"github.com/nsgo-dev/nsgo/core"
)

// miterLimit is SVG's initial stroke-miterlimit, which limu's scenes leave as it is.
const miterLimit = 4

// painter draws a scene's items on a canvas, in the scene's units.
type painter struct {
	d      *core.Drawing
	look   *style.Look
	m      *measurer
	icons  map[string]*core.SVG
	images map[string]core.Size
}

// paint draws sc on d, in the order of its items, in look's colors, measuring its labels' spans with m.
func paint(d *core.Drawing, sc *scene.Scene, look *style.Look, m *measurer, icons map[string]*core.SVG, images map[string]core.Size) {
	p := painter{d: d, look: look, m: m, icons: icons, images: images}
	for i := range sc.Items {
		it := &sc.Items[i]
		if !look.Draws(string(it.Role)) {
			continue
		}
		s := look.Resolve(string(it.Role), it.Style, it.Author)
		p.parts(it, s)
		p.ends(it, s)
		p.picture(it, s)
		p.label(it, s)
	}
}

// color is c at opacity, as the canvas takes it: limu draws an item's colors at the item's opacity, where SVG draws the item as a group at it.
func color(c style.Color, opacity float64) core.Color {
	a := float64(c.A) / 255
	if opacity < 1 {
		a *= max(opacity, 0)
	}
	return core.RGBA(uint32(c.R)<<16|uint32(c.G)<<8|uint32(c.B), float32(a))
}

// limuColor is c, packed as 0xRRGGBBAA, as limu takes it.
func limuColor(c core.Color) style.Color {
	return style.Color{R: uint8(c >> 24), G: uint8(c >> 16), B: uint8(c >> 8), A: uint8(c)}
}

// path is a path of the scene on the canvas.
func path(sp *scene.Path) *core.Path {
	out := &core.Path{}
	sp.Each(func(op scene.Op, a []float64) {
		f := func(i int) float32 { return float32(a[i]) }
		switch op {
		case scene.MoveTo:
			out.MoveTo(f(0), f(1))
		case scene.LineTo:
			out.LineTo(f(0), f(1))
		case scene.QuadTo:
			out.QuadTo(f(0), f(1), f(2), f(3))
		case scene.CubeTo:
			out.CubicTo(f(0), f(1), f(2), f(3), f(4), f(5))
		case scene.Close:
			out.ClosePath()
		}
	})
	return out
}

// parts draws the parts of an item's outline, paths and boxes, a box's corners rounded by the style's radius when it is Rounded, each filled and stroked as its Paint says.
func (p painter) parts(it *scene.Item, s style.Resolved) {
	for _, part := range it.Parts {
		fill, stroke := color(s.Fill, s.Opacity), color(s.Stroke, s.Opacity)
		switch part.Paint {
		case scene.StrokeOnly:
			fill = 0
		case scene.FillOnly:
			stroke = 0
		case scene.Solid:
			fill = color(s.Line, s.Opacity)
		case scene.Hollow:
			fill = color(s.Hollow, s.Opacity)
		case scene.Shade:
			stroke = 0
			fill = color(shade(s.Shade, !p.look.ThemeOnly && it.Author.Fill.Set()), s.Opacity)
		case scene.Dot:
			stroke = 0
			fill = color(s.Marker, s.Opacity)
		}
		sp := part.Path
		if sp == nil {
			if part.Box.W < 0 || part.Box.H < 0 {
				continue
			}
			radius := 0.0
			if part.Rounded {
				radius = min(max(s.Radius, 0), part.Box.W/2, part.Box.H/2)
			}
			sp = new(scene.Path).RoundRect(part.Box, radius)
		}
		cp := path(sp)
		if visible(fill) {
			p.d.SetFillColor(fill).FillPath(cp, core.NonZero)
		}
		p.stroke(cp, stroke, s, s.Dash, 0)
	}
}

// shade is the color of a part painted Shade: the background limu's look picks, and, when its item's author set a fill, that background at 60% opacity over the parts below, as Pele draws an entity's stripes.
func shade(bg style.Color, author bool) style.Color {
	if author {
		bg.A = uint8(float64(bg.A)*0.6 + 0.5)
	}
	return bg
}

// visible reports whether c paints anything.
func visible(c core.Color) bool { return c&0xff > 0 }

// stroke strokes cp in c with the style's width, caps and dashes, and its joins, or join when it is set.
func (p painter) stroke(cp *core.Path, c core.Color, s style.Resolved, dash []float64, join style.Join) {
	if !visible(c) || !(s.StrokeWidth > 0) {
		return
	}
	if !join.Set() {
		join = s.Join
	}
	p.d.Save()
	p.d.SetStrokeColor(c).SetLineWidth(float32(s.StrokeWidth)).SetLineCap(lineCap(s.Cap)).SetLineJoin(lineJoin(join)).SetMiterLimit(miterLimit)
	if ds := dashes(dash); ds != nil {
		p.d.SetLineDash(ds...)
	}
	p.d.StrokePath(cp)
	p.d.Restore()
}

// dashes is a dash pattern as the canvas takes it, or nil for one SVG draws solid: none, one with a length below zero or not finite, or one with no length above zero.
func dashes(dash []float64) []float32 {
	total := 0.0
	for _, l := range dash {
		if !(l >= 0) || l > math.MaxFloat32 {
			return nil
		}
		total += l
	}
	if !(total > 0) || total > math.MaxFloat32 {
		return nil
	}
	out := make([]float32, len(dash))
	for i, l := range dash {
		out[i] = float32(l)
	}
	return out
}

func lineCap(c style.Cap) core.LineCap {
	switch c {
	case style.CapRound:
		return core.CapRound
	case style.CapSquare:
		return core.CapSquare
	}
	return core.CapButt
}

func lineJoin(j style.Join) core.LineJoin {
	switch j {
	case style.JoinRound:
		return core.JoinRound
	case style.JoinBevel:
		return core.JoinBevel
	}
	return core.JoinMiter
}

// ends draws the markers at the ends of an edge: filled in the stroke's color without its opacity and stroked at it, as Pele draws them, with the corners its type turns, or else the edge's.
func (p painter) ends(it *scene.Item, s style.Resolved) {
	fill, line := color(s.Marker, s.Opacity), color(s.Stroke, s.Opacity)
	if !visible(fill) && !visible(line) {
		return
	}
	for _, m := range it.Ends {
		join := m.Type.Join()
		for _, part := range m.Outline() {
			sp := part.Path
			if sp == nil {
				sp = new(scene.Path).RoundRect(part.Box, 0)
			}
			cp := path(sp)
			switch part.Paint {
			case scene.FillOnly, scene.Solid:
				p.d.SetFillColor(fill).FillPath(cp, core.NonZero)
			case scene.FillAndStroke:
				p.d.SetFillColor(fill).FillPath(cp, core.NonZero)
				p.stroke(cp, line, s, nil, join)
			case scene.StrokeOnly:
				p.stroke(cp, line, s, nil, join)
			case scene.Hollow:
				p.d.SetFillColor(color(s.Hollow, s.Opacity)).FillPath(cp, core.NonZero)
				p.stroke(cp, line, s, nil, join)
			}
		}
	}
}

// label draws an item's label about its At as limu's SVG places it: each line centered on it, or starting or ending there as its Align says, its baseline 0.35 of the label's size below the middle of its line, and the whole turned by Rotate degrees. A span is set in its face, a Sub or Sup span 1.2 times smaller and lowered a fifth or raised a third of the size, and an Underline span underlined. A label's halo, which keeps it clear of the lines under it, is its text stroked first.
func (p painter) label(it *scene.Item, s style.Resolved) {
	l := it.Label
	if l == nil || l.Empty() {
		return
	}
	c := color(s.Text, s.Opacity)
	if !visible(c) {
		return
	}
	halo := color(s.Halo, s.Opacity)
	haloed := visible(halo) && s.HaloWidth > 0
	d := p.d
	d.Save()
	defer d.Restore()
	if it.Rotate != 0 {
		d.Translate(float32(it.At.X), float32(it.At.Y)).Rotate(float32(it.Rotate*math.Pi/180)).Translate(float32(-it.At.X), float32(-it.At.Y))
	}
	d.SetTextAlign(core.TextAlignLeft).SetTextBaseline(core.BaselineAlphabetic)
	if haloed {
		d.SetStrokeColor(halo).SetLineWidth(float32(s.HaloWidth)).SetLineJoin(core.JoinRound)
	}
	type run struct {
		text       string
		icon       string
		font       core.Font
		x, y, w    float64
		underlined bool
	}
	// set measures a span as the layout did, at its size, or 1.2 times smaller, lowered or raised, for a Sub or Sup span.
	set := func(sp text.Span) run {
		size, dy := l.Size, 0.0
		if sp.Face&(text.Sub|text.Sup) != 0 {
			dy = size / 5
			if sp.Face&text.Sup != 0 {
				dy = -size / 3
			}
			size /= 1.2
		}
		if sp.Icon != "" {
			return run{icon: sp.Icon, w: sp.Width}
		}
		return run{text: sp.Text, font: font(size, sp.Face), y: dy, w: p.m.Width(sp.Text, size, sp.Face&(text.Bold|text.Italic|text.Mono)), underlined: sp.Face&text.Underline != 0}
	}
	top := it.At.Y - l.Height/2
	var runs []run
	for _, line := range l.Lines {
		y := top + l.LineHeight/2 + l.Size*0.35
		runs = runs[:0]
		width := 0.0
		for _, sp := range line {
			if sp.Text == "" && sp.Icon == "" {
				continue
			}
			r := set(sp)
			r.x, r.y = width, y+r.y
			runs = append(runs, r)
			width += r.w
		}
		x := it.At.X
		switch it.Align {
		case scene.Start:
		case scene.End:
			x -= width
		default:
			x -= width / 2
		}
		if haloed {
			for _, r := range runs {
				if r.icon == "" {
					d.StrokeText(r.text, float32(x+r.x), float32(r.y), r.font)
				}
			}
		}
		d.SetFillColor(c)
		for _, r := range runs {
			if r.icon != "" {
				side := l.Size * 1.1
				p.icon(r.icon, scene.Rect{X: x + r.x, Y: top + l.LineHeight/2 - side/2, W: side, H: side}, s)
			} else {
				d.FillText(r.text, float32(x+r.x), float32(r.y), r.font)
			}
			if r.underlined {
				// A fifteenth of the size thick and a tenth of it below the baseline.
				d.FillRect(float32(x+r.x), float32(r.y+float64(r.font.Size)/10), float32(r.w), float32(max(1, float64(r.font.Size)/15)))
			}
		}
		top += l.LineHeight
	}
}
