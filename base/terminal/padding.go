package terminal

import (
	"math"

	"github.com/nsgo-dev/nsgo/base/terminal/internal/vt"
	"github.com/nsgo-dev/nsgo/core"
)

// PaddingColor is how View colors the space outside its grid.
type PaddingColor uint8

const (
	// PaddingBackground uses the terminal's background, including OSC 11.
	PaddingBackground PaddingColor = iota
	// PaddingExtend extends the nearest cell's background. On the primary
	// screen, top/bottom extension is suppressed for prompt rows, Powerline
	// rows or rows with any default-background cell.
	PaddingExtend
	// PaddingExtendAlways extends every edge without those heuristics.
	PaddingExtendAlways
)

func (f *frame) paddingRow(y, rows, cols int, cells []vt.Cell, styleOf func(vt.Cell, int) resolved, size core.Size) {
	v, d := f.v, f.d
	if v.p.PaddingColor == PaddingBackground || len(cells) == 0 {
		return
	}
	cols = min(cols, len(cells))
	bg := color(f.colors.Background)
	cellColor := func(x int) core.Color {
		c := cells[x]
		if c.Wide == vt.SpacerTail && x > 0 {
			x--
			c = cells[x]
		}
		s := styleOf(c, x)
		if c.HasBackground {
			b := c.Background
			if c.Palette {
				b = f.colors.Palette[b.R]
			}
			s.bg = color(b)
		}
		if c.Codepoint == 0x2588 && !s.invisible {
			return s.fg
		}
		return s.bg
	}
	fill := func(c core.Color, x, y, w, h float32) {
		if w > 0 && h > 0 && (!v.p.Transparent || c != bg) {
			pixelRect(d, c, x, y, w, h, v.ratio)
		}
	}
	ry := v.padY + float32(y)*v.cellH
	fill(cellColor(0), 0, ry, v.padX, v.cellH)
	rx := v.padX + float32(cols)*v.cellW
	fill(cellColor(cols-1), rx, ry, float32(size.Width)-rx, v.cellH)
	if y != 0 && y != rows-1 {
		return
	}
	if v.p.PaddingColor != PaddingExtendAlways && !v.t.term.AltScreen() {
		if f.rs.RowPrompt() != vt.NoPrompt {
			return
		}
		for x, c := range cells[:cols] {
			if c.Codepoint >= 0xe0b0 && c.Codepoint <= 0xe0b7 || cellColor(x) == bg {
				return
			}
		}
	}
	for x := range cols {
		left, right := v.padX+float32(x)*v.cellW, v.padX+float32(x+1)*v.cellW
		if x == 0 {
			left = 0
		}
		if x == cols-1 {
			right = float32(size.Width)
		}
		if y == 0 {
			fill(cellColor(x), left, 0, right-left, v.padY)
		}
		if y == rows-1 {
			bottom := ry + v.cellH
			fill(cellColor(x), left, bottom, right-left, float32(size.Height)-bottom)
		}
	}
}

// Cell backgrounds and extended padding share the same pixel edges. Adjacent
// fractional rectangles otherwise each leave a translucent edge, which shows
// the base color through as a hairline even when both colors are identical.
func pixelRect(d *core.Drawing, c core.Color, x, y, w, h, ratio float32) {
	ratio = max(ratio, 1)
	snap := func(n float32) float32 { return float32(math.Round(float64(n*ratio))) / ratio }
	x0, y0, x1, y1 := snap(x), snap(y), snap(x+w), snap(y+h)
	d.SetFillColor(c).FillRect(x0, y0, x1-x0, y1-y0)
}
