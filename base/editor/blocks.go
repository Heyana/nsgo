package editor

import (
	"math"
	"slices"
	"sort"

	"github.com/nsgo-dev/nsgo/core"
)

// blockSpec is a resolved Block decoration: either replacing or hiding lines
// startLine..endLine, or inserted before/after startLine when inserted is
// true.
type blockSpec struct {
	startLine, endLine int
	inserted           bool
	after              bool
	hidden             bool
	deco               *Decoration
}

// blockPosKey identifies a block's line anchor so its measured height can be
// carried across Decoration rebuilds without a 1-frame jump back to e.lh.
type blockPosKey struct {
	startLine, endLine int
	inserted, after    bool
}

// lineMetric is a line's resolved vertical and horizontal metrics when a
// Decoration's LineStyle sets FontSize, LineHeight, FontFamily, FontWeight,
// FontStyle or Color.
type lineMetric struct {
	height    float32
	charW     float32
	font      core.Font
	textStyle core.Style
}

// blockPlacement is a visible Block decoration rendered in blockLayer.
type blockPlacement struct {
	deco *Decoration
	row  int
}

// collectBlocks extracts block decorations from e.marks, sorted by line and
// placement (inserted-before, replacing/hidden, inserted-after), keeping the
// first replacing/hidden block when two overlap.
func (e *ed) collectBlocks() []blockSpec {
	var raw []blockSpec
	n := e.st.r().lines()
	for i := range e.marks {
		m := &e.marks[i]
		d := m.deco
		if d == nil || !d.Block {
			continue
		}
		if d.Replace == nil && !d.Hidden {
			continue
		}
		if m.to.less(m.from) {
			continue
		}
		if m.from == m.to {
			if d.Replace == nil {
				continue
			}
			line := min(max(m.from.line, 0), max(n-1, 0))
			raw = append(raw, blockSpec{
				startLine: line,
				endLine:   line,
				inserted:  true,
				after:     m.after,
				deco:      d,
			})
			continue
		}
		startLine := min(max(m.from.line, 0), max(n-1, 0))
		endLine := min(max(lastLine(m), startLine), max(n-1, 0))
		raw = append(raw, blockSpec{
			startLine: startLine,
			endLine:   endLine,
			hidden:    d.Replace == nil && d.Hidden,
			deco:      d,
		})
	}
	if len(raw) == 0 {
		return nil
	}
	rank := func(b blockSpec) int {
		switch {
		case b.inserted && !b.after:
			return 0
		case !b.inserted:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(raw, func(i, j int) bool {
		if raw[i].startLine != raw[j].startLine {
			return raw[i].startLine < raw[j].startLine
		}
		return rank(raw[i]) < rank(raw[j])
	})
	out := make([]blockSpec, 0, len(raw))
	coveredUntil := -1
	for _, b := range raw {
		if !b.inserted {
			if b.startLine <= coveredUntil {
				continue // overlaps an earlier replacing/hidden block
			}
			coveredUntil = b.endLine
		}
		out = append(out, b)
	}
	return out
}

// hasLineTextStyle reports whether st overrides any font, height or color
// property of a line's text.
func hasLineTextStyle(st core.Style) bool {
	return st.LineHeight > 0 || st.FontSize > 0 || st.FontFamily != "" ||
		st.FontWeight != 0 || st.FontStyle != 0 || st.Color != 0
}

// lineTextFields returns only the text-styling fields of st, for merging onto
// a line's core.Text without pulling in box backgrounds or borders.
func lineTextFields(st core.Style) core.Style {
	return core.Style{
		Color:      st.Color,
		FontFamily: st.FontFamily,
		FontSize:   st.FontSize,
		FontWeight: st.FontWeight,
		FontStyle:  st.FontStyle,
		LineHeight: st.LineHeight,
	}
}

// hasLineBoxStyle reports whether st has any non-text box style (background,
// border, padding, etc.) that lineRects should draw behind the line.
func hasLineBoxStyle(st core.Style) bool {
	st.Color = 0
	st.FontFamily = ""
	st.FontSize = 0
	st.FontWeight = 0
	st.FontStyle = 0
	st.LineHeight = 0
	return !st.IsZero()
}

// collectLineMetrics extracts per-line text metrics from LineStyle decorations.
// Returns nil when no decoration sets line text/height overrides.
func (e *ed) collectLineMetrics() map[int]lineMetric {
	var raw map[int]core.Style
	n := e.st.r().lines()
	for i := range e.marks {
		m := &e.marks[i]
		d := m.deco
		if d == nil || d.Block || !hasLineTextStyle(d.LineStyle) {
			continue
		}
		if m.to.less(m.from) {
			continue
		}
		fromLine := min(max(m.from.line, 0), max(n-1, 0))
		toLine := min(max(lastLine(m), fromLine), max(n-1, 0))
		tf := lineTextFields(d.LineStyle)
		if raw == nil {
			raw = map[int]core.Style{}
		}
		for l := fromLine; l <= toLine; l++ {
			raw[l] = merge(raw[l], tf)
		}
	}
	if len(raw) == 0 {
		return nil
	}
	out := make(map[int]lineMetric, len(raw))
	for l, st := range raw {
		font := e.font
		if st.FontFamily != "" {
			font.Family = st.FontFamily
		}
		if st.FontSize > 0 {
			font.Size = st.FontSize
		}
		if st.FontWeight != 0 {
			font.Weight = st.FontWeight
		}
		if st.FontStyle == core.FontStyleItalic {
			font.Italic = true
		}
		cw := e.charW
		if font != e.font {
			cw = e.measureFontCharW(font)
		}
		h := e.lh
		switch {
		case st.LineHeight > 0:
			h = float32(st.LineHeight)
		case st.FontSize > 0:
			h = max(e.lh*float32(st.FontSize)/float32(e.font.Size), float32(st.FontSize))
		}
		st.LineHeight = core.Points(h)
		out[l] = lineMetric{height: h, charW: cw, font: font, textStyle: st}
	}
	return out
}

// measureFontCharW caches the single-column width for a custom line font.
func (e *ed) measureFontCharW(font core.Font) float32 {
	if e.fontWidths == nil {
		e.fontWidths = map[core.Font]float32{}
	}
	if w, ok := e.fontWidths[font]; ok {
		return w
	}
	w := max(e.cx.MeasureText("0000000000", font).Width/10, 1)
	e.fontWidths[font] = w
	return w
}

// lineCharW returns the single-column width in points for line.
func (e *ed) lineCharW(line int) float32 {
	if e.lineMetrics != nil {
		if lm, ok := e.lineMetrics[line]; ok {
			return lm.charW
		}
	}
	return e.charW
}

// lineFontSize returns the font size in points for line.
func (e *ed) lineFontSize(line int) core.Points {
	if e.lineMetrics != nil {
		if lm, ok := e.lineMetrics[line]; ok {
			return lm.font.Size
		}
	}
	return e.font.Size
}

// lineTextStyle returns the text style overrides for line, or zero.
func (e *ed) lineTextStyle(line int) core.Style {
	if e.lineMetrics != nil {
		if lm, ok := e.lineMetrics[line]; ok {
			return lm.textStyle
		}
	}
	return core.Style{}
}

// blockRowHeight returns the current height in points of a block widget row.
func (e *ed) blockRowHeight(b *blockSpec) float32 {
	if b.deco != nil && b.deco.Height > 0 {
		return float32(b.deco.Height)
	}
	if e.blockH != nil {
		if h, ok := e.blockH[b.deco]; ok && h > 0 {
			return h
		}
	}
	if e.blockPosH != nil {
		key := blockPosKey{b.startLine, b.endLine, b.inserted, b.after}
		if h, ok := e.blockPosH[key]; ok && h > 0 {
			return h
		}
	}
	return e.lh
}

// syncLayout refreshes block specs, line metrics, display layout (when
// needed) and row prefix-sum geometry. It returns whether display rows or
// row geometry changed.
func (e *ed) syncLayout(forceRelayout bool) bool {
	blocks := e.collectBlocks()
	metrics := e.collectLineMetrics()

	// Prune blockH to live block decorations and carry measured heights by
	// position key when decorations are recreated across renders.
	if len(blocks) == 0 {
		e.blockH = nil
		e.blockPosH = nil
	} else {
		nextH := make(map[*Decoration]float32, len(blocks))
		nextPosH := make(map[blockPosKey]float32, len(blocks))
		for _, b := range blocks {
			if b.deco == nil || b.hidden || b.deco.Height > 0 {
				continue
			}
			key := blockPosKey{b.startLine, b.endLine, b.inserted, b.after}
			if e.blockH != nil {
				if h, ok := e.blockH[b.deco]; ok && h > 0 {
					nextH[b.deco] = h
					nextPosH[key] = h
					continue
				}
			}
			if e.blockPosH != nil {
				if h, ok := e.blockPosH[key]; ok && h > 0 {
					nextH[b.deco] = h
					nextPosH[key] = h
				}
			}
		}
		e.blockH = nextH
		e.blockPosH = nextPosH
	}

	blocksChanged := !slices.Equal(e.blocks, blocks)
	metricsChanged := !mapsEqualLineMetric(e.lineMetrics, metrics)
	e.blocks = blocks
	e.lineMetrics = metrics
	e.st.blocks = blocks

	if len(metrics) > 0 && e.p.SoftWrap {
		avail := float32(e.viewport.GetUntracked().Width) - e.gutterWidth() - 2*pad
		e.st.lineWrap = func(line int) int {
			return max(int(avail/e.lineCharW(line)), 8)
		}
	} else {
		e.st.lineWrap = nil
	}

	if forceRelayout || blocksChanged || (metricsChanged && e.p.SoftWrap) {
		e.st.relayout()
	}

	geoChanged := e.rebuildGeometry()
	if metricsChanged {
		e.measure()
	}
	return blocksChanged || metricsChanged || geoChanged
}

func mapsEqualLineMetric(a, b map[int]lineMetric) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || va.height != vb.height || va.charW != vb.charW || va.font != vb.font ||
			va.textStyle.Color != vb.textStyle.Color || va.textStyle.FontStyle != vb.textStyle.FontStyle {
			return false
		}
	}
	return true
}

// rebuildGeometry updates uniform/tops from e.st.disp, e.blocks and
// e.lineMetrics. Returns true if uniform or tops changed.
func (e *ed) rebuildGeometry() bool {
	hasVarHeight := len(e.blocks) > 0
	if !hasVarHeight && e.lineMetrics != nil {
		for _, lm := range e.lineMetrics {
			if lm.height != e.lh {
				hasVarHeight = true
				break
			}
		}
	}
	if !hasVarHeight {
		changed := !e.uniform || len(e.tops) > 0
		e.uniform = true
		e.tops = nil
		return changed
	}
	n := e.st.disp.count()
	oldUniform := e.uniform
	oldTops := slices.Clone(e.tops)
	e.uniform = false
	if cap(e.tops) < n+1 {
		e.tops = make([]float32, n+1)
	}
	e.tops = e.tops[:n+1]
	r := e.st.r()
	y := float32(0)
	for i := range n {
		e.tops[i] = y
		d := e.st.disp.row(i, r)
		var h float32
		switch {
		case d.block != nil:
			h = e.blockRowHeight(d.block)
		case e.lineMetrics != nil:
			if lm, ok := e.lineMetrics[d.line]; ok {
				h = lm.height
			} else {
				h = e.lh
			}
		default:
			h = e.lh
		}
		y += h
	}
	e.tops[n] = y
	return oldUniform || !slices.Equal(oldTops, e.tops)
}

// rowTop returns the Y offset of display row i in the rows' box.
func (e *ed) rowTop(i int) float32 {
	if i <= 0 {
		return 0
	}
	if e.uniform {
		return float32(i) * e.lh
	}
	if len(e.tops) == 0 {
		return 0
	}
	if i >= len(e.tops) {
		return e.tops[len(e.tops)-1]
	}
	return e.tops[i]
}

// rowHeight returns the height in points of display row i.
func (e *ed) rowHeight(i int) float32 {
	if e.uniform {
		return e.lh
	}
	if i < 0 || i+1 >= len(e.tops) {
		return e.lh
	}
	return e.tops[i+1] - e.tops[i]
}

// totalRowsHeight returns the sum of all display rows' heights.
func (e *ed) totalRowsHeight() float32 {
	if e.uniform {
		return float32(e.st.disp.count()) * e.lh
	}
	if len(e.tops) == 0 {
		return 0
	}
	return e.tops[len(e.tops)-1]
}

// rowAtY returns the display row containing y: -1 when y < 0 or there are no
// rows, and disp.count() when y >= totalRowsHeight().
func (e *ed) rowAtY(y float32) int {
	n := e.st.disp.count()
	if n == 0 || y < 0 {
		return -1
	}
	if e.uniform {
		row := int(y / e.lh)
		if row >= n {
			return n
		}
		return row
	}
	if y >= e.tops[n] {
		return n
	}
	return sort.Search(n, func(i int) bool { return e.tops[i+1] > y })
}

// rowAtYClamped returns the display row at y, clamped to [0, disp.count()].
func (e *ed) rowAtYClamped(y float32) int {
	n := e.st.disp.count()
	if n == 0 || y <= 0 {
		return 0
	}
	r := e.rowAtY(y)
	if r < 0 {
		return 0
	}
	if r > n {
		return n
	}
	return r
}

// pageStep returns the row delta for MovePageUp/Down from row, moving by
// roughly one viewport height (always at least 1 row in dir).
func (e *ed) pageStep(row, dir int) int {
	n := e.st.disp.count()
	if n == 0 {
		return 0
	}
	vpH := max(float32(e.viewport.GetUntracked().Height)-e.lh, e.lh)
	if e.uniform {
		return dir * max(int(vpH/e.lh), 1)
	}
	targetY := e.rowTop(row) + float32(dir)*vpH
	target := e.rowAtY(targetY)
	if dir < 0 {
		if target < 0 {
			target = 0
		}
		if target >= row {
			target = max(row-1, 0)
		}
	} else {
		if target >= n {
			target = n - 1
		}
		if target <= row {
			target = min(row+1, n-1)
		}
	}
	return target - row
}

// blockList returns the visible Block widget rows in the active window.
func (e *ed) blockList() []blockPlacement {
	e.textRev.Get()
	e.marksRev.Get()
	if len(e.blocks) == 0 {
		return nil
	}
	s, end := e.windowRows()
	r := e.st.r()
	var out []blockPlacement
	for row := s; row < end; row++ {
		d := e.st.disp.row(row, r)
		if d.block != nil && d.block.deco != nil && d.block.deco.Replace != nil {
			out = append(out, blockPlacement{deco: d.block.deco, row: row})
		}
	}
	return out
}

// blockLayer renders visible Block decorations in the rows' box, keyed by
// *Decoration so unchanged widgets stay mounted as rows shift.
func (e *ed) blockLayer() core.Node {
	list := core.NewMemoFunc(e.cx, e.blockList, slices.Equal[[]blockPlacement])
	return core.Each(list.Get, func(b blockPlacement) *Decoration { return b.deco }, func(cx *core.Scope, item func() blockPlacement, _ func() int) core.Node {
		deco := item().deco
		return core.View{
			Class: joinClasses(deco.LineClass, deco.Class),
			StyleFunc: func() core.Style {
				e.textRev.Get()
				e.marksRev.Get()
				b := item()
				st := core.Style{
					Position: core.Absolute,
					Left:     core.Pt(0),
					Right:    core.Pt(0),
					Top:      core.Pt(core.Points(e.rowTop(b.row))),
					Cursor:   core.CursorDefault,
				}
				if deco.Height > 0 {
					st.Height = core.Pt(deco.Height)
					st.Overflow = core.Hidden
				}
				return merge(st, deco.LineStyle, deco.Style)
			},
			OnLayout: func(bounds core.Bounds) {
				if deco.Height > 0 {
					return
				}
				h := float32(bounds.Size.Height)
				if h <= 0 {
					return
				}
				prev := float32(0)
				if e.blockH != nil {
					prev = e.blockH[deco]
				}
				if math.Abs(float64(h-prev)) <= 0.5 {
					return
				}
				if e.blockH == nil {
					e.blockH = map[*Decoration]float32{}
				}
				e.blockH[deco] = h
				if e.blockPosH == nil {
					e.blockPosH = map[blockPosKey]float32{}
				}
				for _, bs := range e.blocks {
					if bs.deco == deco {
						e.blockPosH[blockPosKey{bs.startLine, bs.endLine, bs.inserted, bs.after}] = h
						break
					}
				}
				if e.rebuildGeometry() {
					e.textRev.Update(inc)
				}
			},
			Children: []core.Node{deco.Replace},
		}
	})
}
