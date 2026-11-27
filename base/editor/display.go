package editor

import (
	"sort"
	"unicode"
	"unicode/utf8"
)

// dispRow is a display row: part of a line (from byte start to end), or a
// block decoration row when block != nil.
type dispRow struct {
	line, start, end int
	block            *blockSpec
}

// display lays the text out in rows: a line is a row unless soft wrap
// breaks it, a fold shows its first line and hides the others, and a block
// decoration replaces or hides whole lines or inserts a row between lines.
// With none of those, rows are lines and nothing is laid out, so 200 000
// lines cost nothing to show.
type display struct {
	identity bool
	lines    int
	rows     []dispRow
	// first is each line's first row; a hidden line's is the row of the
	// fold or block that hides it (or the nearest row when collapsed to zero
	// rows).
	first []int
}

// layout lays r out with the folds (sorted, outermost, not overlapping)
// hiding their lines, blocks (sorted, non-overlapping replacing/hidden
// blocks) replacing, hiding or inserting rows between lines, wrapping rows
// at wrap columns (none when 0; lineWrap overrides wrap per line when
// non-nil).
func layout(r *rope, folds []foldRange, blocks []blockSpec, wrap, tab int, lineWrap func(int) int) *display {
	n := r.lines()
	if len(folds) == 0 && len(blocks) == 0 && wrap <= 0 {
		return &display{identity: true, lines: n}
	}
	d := &display{lines: n, rows: make([]dispRow, 0, n+len(blocks)), first: make([]int, n)}
	f := 0
	bi := 0
	for line := 0; line < n; line++ {
		// Emit inserted blocks placed before this line.
		for bi < len(blocks) && blocks[bi].startLine < line {
			bi++
		}
		for bi < len(blocks) && blocks[bi].startLine == line && blocks[bi].inserted && !blocks[bi].after {
			b := blocks[bi]
			d.rows = append(d.rows, dispRow{line: line, block: &b})
			bi++
		}

		// Check if a replacing or hidden block starts on this line.
		if bi < len(blocks) && blocks[bi].startLine == line && !blocks[bi].inserted {
			b := blocks[bi]
			bi++
			endLine := min(max(b.endLine, line), n-1)
			row := len(d.rows)
			if !b.hidden {
				d.rows = append(d.rows, dispRow{line: line, block: &b})
			}
			for l := line; l <= endLine; l++ {
				d.first[l] = row
			}
			// Emit any inserted-after blocks attached to lines inside the
			// replaced/hidden range only if they belong to endLine and are
			// explicitly after it.
			for bi < len(blocks) && blocks[bi].startLine <= endLine {
				if blocks[bi].inserted && blocks[bi].after && blocks[bi].startLine == endLine {
					ab := blocks[bi]
					d.rows = append(d.rows, dispRow{line: endLine, block: &ab})
				}
				bi++
			}
			line = endLine
			continue
		}

		row := len(d.rows)
		d.first[line] = row
		text := r.line(line)
		wcols := wrap
		if wrap > 0 && lineWrap != nil {
			wcols = lineWrap(line)
		}
		if wcols <= 0 {
			d.rows = append(d.rows, dispRow{line: line, start: 0, end: len(text)})
		} else {
			for _, w := range wrapLine(text, wcols, tab) {
				d.rows = append(d.rows, dispRow{line: line, start: w[0], end: w[1]})
			}
		}

		// Emit inserted blocks placed after this line (unless folded below).
		emitAfter := func(at int) {
			for bi < len(blocks) && blocks[bi].startLine <= at {
				if blocks[bi].inserted && blocks[bi].after && blocks[bi].startLine == at {
					ab := blocks[bi]
					d.rows = append(d.rows, dispRow{line: at, block: &ab})
				}
				bi++
			}
		}

		for f < len(folds) && folds[f].end < line {
			f++
		}
		if f < len(folds) && folds[f].start == line && folds[f].end > line {
			// A folded line shows as one row.
			d.rows = d.rows[:row+1]
			d.rows[row].end = len(text)
			endFold := min(folds[f].end, n-1)
			for hidden := line + 1; hidden <= endFold; hidden++ {
				d.first[hidden] = row
			}
			// Skip blocks inside folded lines, except inserted-after on endFold.
			emitAfter(endFold)
			line = endFold
			continue
		}
		emitAfter(line)
	}
	// Clamp any trailing hidden lines' first index to a valid row when rows exist.
	if len(d.rows) > 0 {
		last := len(d.rows) - 1
		for i, r := range d.first {
			if r > last {
				d.first[i] = last
			}
		}
	}
	return d
}

// wrapLine breaks text into rows of at most wrap columns: after white
// space where a row would overflow, else between characters.
func wrapLine(text string, wrap, tab int) [][2]int {
	var out [][2]int
	start, col, lastSpace := 0, 0, -1
	for i, r := range text {
		w := runeCols(r, col, tab)
		if col+w > wrap && i > start {
			cut := i
			if lastSpace > start {
				cut = lastSpace
			}
			out = append(out, [2]int{start, cut})
			start, lastSpace = cut, -1
			col = colAt(text[start:i], len(text[start:i]), tab)
		}
		col += w
		if r == ' ' || r == '\t' {
			lastSpace = i + 1
		}
	}
	return append(out, [2]int{start, len(text)})
}

func (d *display) count() int {
	if d.identity {
		return d.lines
	}
	return len(d.rows)
}

func (d *display) row(i int, r *rope) dispRow {
	if d.identity {
		return dispRow{line: i, start: 0, end: len(r.line(i))}
	}
	return d.rows[i]
}

// rowOf is the row p shows in.
func (d *display) rowOf(p pos) int {
	if d.identity {
		return min(max(p.line, 0), max(d.lines-1, 0))
	}
	if len(d.rows) == 0 || len(d.first) == 0 {
		return 0
	}
	row := d.first[min(max(p.line, 0), len(d.first)-1)]
	if row >= len(d.rows) {
		return len(d.rows) - 1
	}
	if d.rows[row].block != nil || d.rows[row].line != p.line {
		return row
	}
	for row+1 < len(d.rows) && d.rows[row+1].block == nil && d.rows[row+1].line == p.line && d.rows[row+1].start <= p.col {
		row++
	}
	return row
}

// lineRow is the first row of line, or the row hiding it.
func (d *display) lineRow(line int) int {
	if d.identity {
		return min(max(line, 0), max(d.lines-1, 0))
	}
	if len(d.first) == 0 {
		return 0
	}
	return d.first[min(max(line, 0), len(d.first)-1)]
}

// visibleLine reports whether line shows (no fold or block hides it).
func (d *display) visibleLine(line int) bool {
	if d.identity {
		return line >= 0 && line < d.lines
	}
	if len(d.rows) == 0 || line < 0 || line >= len(d.first) {
		return false
	}
	row := d.first[line]
	if row < 0 || row >= len(d.rows) {
		return false
	}
	return d.rows[row].block == nil && d.rows[row].line == line
}

// rowsOfLines is the rows of lines from to to, sorted.
func (d *display) rowAt(line int) int { return d.lineRow(line) }

// colAt is the display column at byte b of text: tabs to the next stop,
// wide characters two columns.
func colAt(text string, b, tab int) int {
	col := 0
	for _, r := range text[:min(b, len(text))] {
		col += runeCols(r, col, tab)
	}
	return col
}

func runeCols(r rune, col, tab int) int {
	switch {
	case r == '\t':
		return tab - col%tab
	case wide(r):
		return 2
	}
	return 1
}

// wide reports whether r takes two columns: East Asian wide and full-width
// characters.
func wide(r rune) bool {
	return r >= 0x1100 && (r <= 0x115f || r == 0x2329 || r == 0x232a ||
		r >= 0x2e80 && r <= 0xa4cf && r != 0x303f || r >= 0xac00 && r <= 0xd7a3 ||
		r >= 0xf900 && r <= 0xfaff || r >= 0xfe30 && r <= 0xfe4f || r >= 0xff00 && r <= 0xff60 ||
		r >= 0xffe0 && r <= 0xffe6 || r >= 0x1f300 && r <= 0x1f64f || r >= 0x1f900 && r <= 0x1f9ff ||
		r >= 0x20000 && r <= 0x3fffd)
}

// byteAtCol is the byte of text from start nearest column col (of the
// whole line), within start to end.
func byteAtCol(text string, start, end, col, tab int) int {
	c := colAt(text, start, tab)
	for i, r := range text[start:end] {
		w := runeCols(r, c, tab)
		if col < c+(w+1)/2 {
			return start + i
		}
		c += w
	}
	return end
}

// wordChar reports whether r is part of a word in l.
func (l *language) wordChar(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || l.word != "" && containsRune(l.word, r)
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

// wordAt is the word around byte col of text, or col to col.
func (l *language) wordAt(text string, col int) (int, int) {
	start, end := col, col
	for start > 0 {
		r, n := utf8.DecodeLastRuneInString(text[:start])
		if !l.wordChar(r) {
			break
		}
		start -= n
	}
	for end < len(text) {
		r, n := utf8.DecodeRuneInString(text[end:])
		if !l.wordChar(r) {
			break
		}
		end += n
	}
	return start, end
}

// searchRows is the index of the first row of rows at or after line.
func searchRows(rows []dispRow, line int) int {
	return sort.Search(len(rows), func(i int) bool { return rows[i].line >= line })
}
