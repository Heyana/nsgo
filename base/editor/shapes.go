package editor

import (
	"slices"
	"sort"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/core"
)

// A shape is what a decoration makes of its range's characters instead of
// drawing them: no columns at all, or the columns kept for a view. The
// editor lays its text out in columns of the font's 0, so a shape keeps that
// arithmetic exact: a hidden range takes no columns and the text after it
// moves left, a replaced one keeps its columns and its view fills them, and
// carets, selections and presses land where the characters still were.
type shape struct {
	from, to int
	hidden   bool
	deco     *Decoration
}

// byteSpan is a range of bytes in a line.
type byteSpan struct{ a, b int }

// replSpan is a replaced range of a line and the decoration whose view draws
// in it (nil when the decoration spans lines and shows no view).
type replSpan struct {
	a, b int
	deco *Decoration
}

// shapesIn is line's shapes, clipped to its text, merged and ordered. A
// column a Replace covers stays a column even when a Hidden range covers
// it too.
func (e *ed) shapesIn(line int) []shape {
	if !e.hasShapes {
		return nil
	}
	r := e.st.r()
	if line < 0 || line >= r.lines() {
		return nil
	}
	l := r.line(line)
	var hidden []byteSpan
	var replaced []replSpan
	for i := range e.marks {
		m := &e.marks[i]
		if m.from.line > line {
			break
		}
		d := m.deco
		if d == nil || !d.Hidden && d.Replace == nil {
			continue
		}
		if m.to.line < line {
			continue
		}
		from, to := 0, len(l)
		if m.from.line == line {
			from = min(max(m.from.col, 0), len(l))
		}
		if m.to.line == line {
			to = min(max(m.to.col, 0), len(l))
		}
		if from >= to {
			continue
		}
		// A range across lines keeps its columns and hides its text without
		// a view, as one line's box holds the view.
		if d.Replace != nil {
			var deco *Decoration
			if m.from.line == m.to.line {
				deco = d
			}
			replaced = append(replaced, replSpan{from, to, deco})
			continue
		}
		if d.Hidden {
			hidden = append(hidden, byteSpan{from, to})
		}
	}
	if len(replaced) == 0 && len(hidden) == 0 {
		return nil
	}
	replaced = mergeReplSpans(replaced)
	cover := make([]byteSpan, len(replaced))
	for i, s := range replaced {
		cover[i] = byteSpan{s.a, s.b}
	}
	hidden = cutByteSpans(mergeByteSpans(hidden), cover)
	out := make([]shape, 0, len(replaced)+len(hidden))
	for _, s := range replaced {
		out = append(out, shape{from: s.a, to: s.b, deco: s.deco})
	}
	for _, s := range hidden {
		out = append(out, shape{from: s.a, to: s.b, hidden: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].from < out[j].from })
	return out
}

// mergeByteSpans sorts spans and joins the ones that touch or overlap.
func mergeByteSpans(list []byteSpan) []byteSpan {
	if len(list) == 0 {
		return nil
	}
	sort.Slice(list, func(i, j int) bool { return list[i].a < list[j].a })
	out := []byteSpan{list[0]}
	for _, s := range list[1:] {
		last := &out[len(out)-1]
		if s.a <= last.b {
			last.b = max(last.b, s.b)
			continue
		}
		out = append(out, s)
	}
	return out
}

// mergeReplSpans sorts replaced spans and joins the ones that overlap,
// keeping the first one's view.
func mergeReplSpans(list []replSpan) []replSpan {
	if len(list) == 0 {
		return nil
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].a < list[j].a })
	out := []replSpan{list[0]}
	for _, s := range list[1:] {
		last := &out[len(out)-1]
		if s.a < last.b {
			last.b = max(last.b, s.b)
			continue
		}
		out = append(out, s)
	}
	return out
}

// cutByteSpans is list without the bytes cover takes.
func cutByteSpans(list, cover []byteSpan) []byteSpan {
	var out []byteSpan
	for _, s := range list {
		cur := s.a
		for _, c := range cover {
			if c.b <= cur || c.a >= s.b {
				continue
			}
			if c.a > cur {
				out = append(out, byteSpan{cur, c.a})
			}
			cur = max(cur, c.b)
			if cur >= s.b {
				break
			}
		}
		if cur < s.b {
			out = append(out, byteSpan{cur, s.b})
		}
	}
	return out
}

// shapeAt is the shape holding byte col.
func shapeAt(shapes []shape, col int) (shape, bool) {
	i := sort.Search(len(shapes), func(i int) bool { return shapes[i].to > col })
	if i < len(shapes) && shapes[i].from <= col {
		return shapes[i], true
	}
	return shape{}, false
}

// shownCols is the columns line shows from byte from to byte to: hidden
// shapes take none, replaced ones keep the columns their text took, and a
// tab counts to the next stop in the shown columns.
func (e *ed) shownCols(line, from, to int) int {
	r := e.st.r()
	if line < 0 || line >= r.lines() {
		return 0
	}
	return shownColsIn(r.line(line), e.shapesIn(line), e.st.tab, from, to)
}

func shownColsIn(l string, shapes []shape, tab, from, to int) int {
	from = min(max(from, 0), len(l))
	to = min(max(to, 0), len(l))
	if from > to {
		from, to = to, from
	}
	if from == to {
		return 0
	}
	col, fromCol, haveFrom := 0, 0, false
	i, si := 0, 0
	for i < to {
		for si < len(shapes) && shapes[si].to <= i {
			si++
		}
		if si < len(shapes) && shapes[si].from <= i && shapes[si].hidden {
			i = min(shapes[si].to, to)
			continue
		}
		if !haveFrom && i >= from {
			fromCol = col
			haveFrom = true
		}
		c, n := utf8.DecodeRuneInString(l[i:to])
		col += runeCols(c, col, tab)
		i += n
	}
	if !haveFrom {
		return 0
	}
	return col - fromCol
}

// byteAtShownCol is the byte of line from to to nearest shown column
// target: a hidden range has no column of its own, so a target at or inside
// it lands after it.
func (e *ed) byteAtShownCol(line, from, to, target int) int {
	r := e.st.r()
	if line < 0 || line >= r.lines() {
		return 0
	}
	l := r.line(line)
	from = min(max(from, 0), len(l))
	to = min(max(to, from), len(l))
	shapes := e.shapesIn(line)
	col := shownColsIn(l, shapes, e.st.tab, 0, from)
	i, si := from, 0
	for si < len(shapes) && shapes[si].to <= i {
		si++
	}
	for i < to {
		if si < len(shapes) && shapes[si].from <= i && shapes[si].hidden {
			i = min(shapes[si].to, to)
			for si < len(shapes) && shapes[si].to <= i {
				si++
			}
			continue
		}
		c, n := utf8.DecodeRuneInString(l[i:to])
		w := runeCols(c, col, e.st.tab)
		if target < col+(w+1)/2 {
			return i
		}
		col += w
		i += n
		for si < len(shapes) && shapes[si].to <= i {
			si++
		}
	}
	return to
}

// replacement is a Decoration.Replace drawn in its range's columns on a row.
type replacement struct {
	deco          *Decoration
	row           int
	leftCol, cols int
}

// replacementList is the replacements of the rows made, in order. A range
// across lines, or split by soft wrap, hides its text and shows no view.
func (e *ed) replacementList() []replacement {
	e.textRev.Get()
	e.marksRev.Get()
	if !e.hasShapes {
		return nil
	}
	s, end := e.windowRows()
	r := e.st.r()
	var out []replacement
	line := -1
	var l string
	var shapes []shape
	for row := s; row < end; row++ {
		dr := e.st.disp.row(row, r)
		if dr.line != line {
			line = dr.line
			l = r.line(line)
			shapes = e.shapesIn(line)
		}
		if len(shapes) == 0 {
			continue
		}
		base := -1
		for _, sh := range shapes {
			if sh.hidden || sh.deco == nil {
				continue
			}
			// A range split by soft wrap hides its text and shows no view.
			if sh.from < dr.start || sh.to > dr.end {
				continue
			}
			cols := shownColsIn(l, shapes, e.st.tab, sh.from, sh.to)
			if cols <= 0 {
				continue
			}
			if base < 0 {
				base = shownColsIn(l, shapes, e.st.tab, 0, dr.start)
			}
			leftCol := shownColsIn(l, shapes, e.st.tab, 0, sh.from) - base
			out = append(out, replacement{
				deco:    sh.deco,
				row:     row,
				leftCol: leftCol,
				cols:    cols,
			})
		}
	}
	return out
}

// replacementLayer draws the rows' replacements over their text, each in a
// box as wide as the columns it took and as tall as the line.
func (e *ed) replacementLayer() core.Node {
	list := core.NewMemoFunc(e.cx, e.replacementList, slices.Equal[[]replacement])
	return core.Each(list.Get, func(r replacement) *Decoration { return r.deco }, func(cx *core.Scope, rep func() replacement, _ func() int) core.Node {
		deco := rep().deco
		return core.View{
			StyleFunc: func() core.Style {
				r := rep()
				return merge(at(pad+float32(r.leftCol)*e.charW, float32(r.row)*e.lh, float32(r.cols)*e.charW, e.lh),
					core.Style{Overflow: core.Hidden, Cursor: core.CursorDefault})
			},
			Children: []core.Node{deco.Replace},
		}
	})
}
