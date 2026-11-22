// Package tui draws NSGo's mount protocol into a terminal's cells. Layout
// remains in points: one cell is 8 by 16 points, independent of the font
// configured in the terminal. Components choose their own styles as on
// every other backend.
package tui

import (
	"math"
	"strings"
	"unicode"

	"github.com/clipperhouse/displaywidth"
	"github.com/clipperhouse/uax29/v2/graphemes"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

const cellWidth, cellHeight driver.Points = 8, 16

type textEngine struct{}

type glyph struct {
	text                     string
	start, end, units, width int
	x, y                     driver.Points
	style                    driver.TextStyle
	hidden                   bool
}

type preparedText struct {
	text   driver.AttributedText
	glyphs []glyph
}

type textLayout struct {
	driver.CaretLayout
	glyphs []glyph
	lines  []driver.TextLine
	size   driver.Size
}

// safeText keeps application text from writing terminal control sequences.
// Newlines and tabs are handled by layout rather than sent to the terminal.
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return '\ufffd'
		}
		return r
	}, s)
}

func (textEngine) Prepare(t *driver.AttributedText) driver.PreparedText {
	p := &preparedText{text: *t}
	p.text.Spans = append([]driver.TextSpan(nil), t.Spans...)
	it := graphemes.FromString(t.Text)
	units := 0
	for it.Next() {
		value := it.Value()
		g := glyph{text: safeText(value), start: it.Start(), end: it.End(), units: units, style: t.Style}
		for _, span := range t.Spans {
			if g.start >= span.Start && g.start < span.End {
				g.style, g.hidden = span.Style, span.Hidden
				break
			}
		}
		if value == "\n" || value == "\r\n" {
			g.text = "\n"
		}
		if value == "\t" {
			g.text = "\t"
		}
		g.width = max(displaywidth.String(g.text), 1)
		p.glyphs = append(p.glyphs, g)
		units += driver.UTF16Len(value)
	}
	return p
}

func (*preparedText) Release()                     {}
func (*textLayout) Release()                       {}
func (l *textLayout) TextLines() []driver.TextLine { return append([]driver.TextLine(nil), l.lines...) }

func (p *preparedText) Measure(width driver.Points) driver.Size   { return p.layout(width).size }
func (p *preparedText) Layout(size driver.Size) driver.TextLayout { return p.layout(size.Width) }

func (p *preparedText) layout(width driver.Points) *textLayout {
	l := &textLayout{}
	columns := math.MaxInt
	if !width.IsAuto() {
		columns = max(int(width/cellWidth), 1)
	}
	lineHeight := max(cellHeight, driver.Points(math.Ceil(float64(p.text.Style.LineHeight/cellHeight)))*cellHeight)
	row, x, first, start, startUnits := 0, 0, 0, 0, 0
	finish := func(end, endUnits int) {
		left := driver.Points(0)
		lineWidth := driver.Points(x) * cellWidth
		if !width.IsAuto() {
			switch p.text.Style.Align {
			case driver.TextAlignCenter:
				left = max(0, (width-lineWidth)/2)
			case driver.TextAlignRight:
				left = max(0, width-lineWidth)
			case driver.TextAlignEnd:
				if !p.text.Style.RTL {
					left = max(0, width-lineWidth)
				}
			case driver.TextAlignStart:
				if p.text.Style.RTL {
					left = max(0, width-lineWidth)
				}
			}
		}
		box := driver.Bounds{Origin: driver.Point{X: left, Y: driver.Points(row) * lineHeight}, Size: driver.Size{Width: lineWidth, Height: lineHeight}}
		cl := driver.CaretLine{Start: startUnits, End: endUnits, Box: box}
		for i := first; i < len(l.glyphs); i++ {
			g := &l.glyphs[i]
			g.x += left
			cl.Clusters = append(cl.Clusters, driver.CaretCluster{Start: g.units, End: g.units + driver.UTF16Len(p.text.Text[g.start:g.end]), Left: g.x, Right: g.x + driver.Points(g.width)*cellWidth})
		}
		l.CaretLayout = append(l.CaretLayout, cl)
		l.lines = append(l.lines, driver.TextLine{Text: p.text.Text[start:end], Start: start, End: end, Bounds: box, Ascender: 12, Descender: 4, CapHeight: 12, XHeight: 8})
		l.size.Width = max(l.size.Width, lineWidth)
		l.size.Height += lineHeight
		row++
		x = 0
		first = len(l.glyphs)
		start = end
		startUnits = endUnits
	}
	truncated := false
	for _, original := range p.glyphs {
		g := original
		if g.text == "\n" {
			finish(g.end, g.units+driver.UTF16Len(p.text.Text[g.start:g.end]))
			if p.text.MaxLines > 0 && row >= p.text.MaxLines {
				truncated = g.end < len(p.text.Text)
				break
			}
			continue
		}
		if g.text == "\t" {
			g.width = 4 - x%4
			g.text = strings.Repeat(" ", g.width)
		}
		if x > 0 && x+g.width > columns {
			finish(g.start, g.units)
			if p.text.MaxLines > 0 && row >= p.text.MaxLines {
				truncated = true
				break
			}
		}
		g.x, g.y = driver.Points(x)*cellWidth, driver.Points(row)*lineHeight
		l.glyphs = append(l.glyphs, g)
		x += g.width
	}
	if p.text.MaxLines == 0 || row < p.text.MaxLines {
		finish(len(p.text.Text), driver.UTF16Len(p.text.Text))
	} else if truncated && p.text.Ellipsize != driver.EllipsizeClip && len(l.glyphs) > 0 {
		last := &l.glyphs[len(l.glyphs)-1]
		line := &l.lines[len(l.lines)-1]
		caret := &l.CaretLayout[len(l.CaretLayout)-1]
		if width.IsAuto() || last.x-line.Bounds.Origin.X+driver.Points(last.width+1)*cellWidth <= width {
			ellipsis := *last
			ellipsis.x += driver.Points(last.width) * cellWidth
			ellipsis.units += driver.UTF16Len(p.text.Text[last.start:last.end])
			ellipsis.start, ellipsis.end = last.end, last.end
			l.glyphs = append(l.glyphs, ellipsis)
			last = &l.glyphs[len(l.glyphs)-1]
			caret.Clusters = append(caret.Clusters, driver.CaretCluster{Start: last.units, End: last.units, Left: last.x, Right: last.x + cellWidth})
		}
		last.text, last.width, last.hidden = "…", 1, false
		line.Bounds.Size.Width = last.x - line.Bounds.Origin.X + cellWidth
		caret.Box.Size.Width = line.Bounds.Size.Width
		if len(caret.Clusters) > 0 {
			caret.Clusters[len(caret.Clusters)-1].Right = last.x + cellWidth
		}
		l.size.Width = 0
		for _, line := range l.lines {
			l.size.Width = max(l.size.Width, line.Bounds.Size.Width)
		}
	}
	return l
}
