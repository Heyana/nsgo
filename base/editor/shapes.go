package editor

import (
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
	node     core.Node
}

// byteSpan is a range of bytes in a line.
type byteSpan struct{ a, b int }

// replSpan is a replaced range of a line and the view drawn in it.
type replSpan struct {
	a, b int
	node core.Node
}

// shapesIn is line's shapes, clipped to its text, merged and ordered. A
// column a Replace covers stays a column even when a Hidden range covers
// it too.
func (e *ed) shapesIn(line int) []shape {
	r := e.st.r()
	if line < 0 || line >= r.lines() {
		return nil
	}
	l := r.line(line)
	var hidden []byteSpan
	var replaced []replSpan
	for i := range e.marks {
		m := &e.marks[i]
		d := m.deco
		if d == nil || !d.Hidden && d.Replace == nil {
			continue
		}
		if m.to.line < line || m.from.line > line {
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
		// A range across lines keeps its columns and hides its text, as
		// one line's box holds the view.
		if d.Replace != nil && m.from.line == m.to.line {
			replaced = append(replaced, replSpan{from, to, d.Replace})
			continue
		}
		if d.Hidden {
			hidden = append(hidden, byteSpan{from, to})
		}
	}
	replaced = mergeReplSpans(replaced)
	cover := make([]byteSpan, len(replaced))
	for i, s := range replaced {
		cover[i] = byteSpan{s.a, s.b}
	}
	hidden = cutByteSpans(mergeByteSpans(hidden), cover)
	out := make([]shape, 0, len(replaced)+len(hidden))
	for _, s := range replaced {
		out = append(out, shape{s.a, s.b, false, s.node})
	}
	for _, s := range hidden {
		out = append(out, shape{s.a, s.b, true, nil})
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
	l := r.line(line)
	from = min(max(from, 0), len(l))
	to = min(max(to, 0), len(l))
	if from > to {
		from, to = to, from
	}
	shapes := e.shapesIn(line)
	shown := func(limit int) int {
		col, i, si := 0, 0, 0
		for i < limit {
			for si < len(shapes) && shapes[si].to <= i {
				si++
			}
			if si < len(shapes) && shapes[si].from <= i && shapes[si].hidden {
				i = min(shapes[si].to, limit)
				continue
			}
			c, n := utf8.DecodeRuneInString(l[i:limit])
			col += runeCols(c, col, e.st.tab)
			i += n
		}
		return col
	}
	return shown(to) - shown(from)
}

// byteAtShownCol is the byte of line from to to nearest shown column
// target: a hidden range has no column of its own, so a target inside it
// lands after it.
func (e *ed) byteAtShownCol(line, from, to, target int) int {
	r := e.st.r()
	if line < 0 || line >= r.lines() {
		return 0
	}
	l := r.line(line)
	from = min(max(from, 0), len(l))
	to = min(max(to, from), len(l))
	col := e.shownCols(line, 0, from)
	if target <= col {
		return from
	}
	shapes := e.shapesIn(line)
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
	line, from, to, row int
	cols                float32
	node                core.Node
}

// replacementList is the replacements of the rows made, in order. A range
// split by soft wrap draws on the row its start is in, as one line's box
// holds one view.
func (e *ed) replacementList() []replacement {
	e.textRev.Get()
	e.marksRev.Get()
	s, end := e.windowRows()
	r := e.st.r()
	var out []replacement
	for i := range e.marks {
		m := &e.marks[i]
		d := m.deco
		if d == nil || d.Replace == nil || m.from.line != m.to.line || m.from.line >= r.lines() {
			continue
		}
		row := e.st.disp.rowOf(m.from)
		if row < s || row >= end {
			continue
		}
		dr := e.st.disp.row(row, r)
		if dr.line != m.from.line || dr.start > m.from.col || dr.end < m.to.col {
			continue
		}
		cols := e.shownCols(m.from.line, m.from.col, m.to.col)
		if cols <= 0 {
			continue
		}
		out = append(out, replacement{line: m.from.line, from: m.from.col, to: m.to.col, row: row,
			cols: float32(cols), node: d.Replace})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].row != out[j].row {
			return out[i].row < out[j].row
		}
		return out[i].from < out[j].from
	})
	return out
}

// sameReplacements compares replacements by where they go, not by the views
// they hold, which need not compare.
func sameReplacements(a, b []replacement) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].line != b[i].line || a[i].from != b[i].from || a[i].to != b[i].to ||
			a[i].row != b[i].row || a[i].cols != b[i].cols {
			return false
		}
	}
	return true
}

// replacementLayer draws the rows' replacements over their text, each in a
// box as wide as the columns it took and as tall as the line.
func (e *ed) replacementLayer() core.Node {
	list := core.NewMemoFunc(e.cx, e.replacementList, sameReplacements)
	return core.Each(func() []int {
		out := make([]int, len(list.Get()))
		for i := range out {
			out[i] = i
		}
		return out
	}, func(i int) int { return i }, func(cx *core.Scope, i func() int, _ func() int) core.Node {
		return core.View{
			StyleFunc: func() core.Style {
				all := list.Get()
				if i() >= len(all) {
					return core.Style{Display: core.DisplayNone}
				}
				r := all[i()]
				return merge(at(pad+float32(e.shownCols(r.line, 0, r.from))*e.charW, float32(r.row)*e.lh, r.cols*e.charW, e.lh),
					core.Style{Overflow: core.Hidden})
			},
			Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
				if all := list.Get(); i() < len(all) {
					return all[i()].node
				}
				return core.Group{}
			})},
		}
	})
}
