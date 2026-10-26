package editor

import (
	"sort"
	"unicode"
	"unicode/utf8"
)

// dispRow is a display row: part of a line, from byte start to end.
type dispRow struct{ line, start, end int }

// display lays the text out in rows: a line is a row unless soft wrap
// breaks it, and a fold shows its first line and hides the others. With
// neither, rows are lines and nothing is laid out, so 200 000 lines cost
// nothing to show.
type display struct {
	identity bool
	lines    int
	rows     []dispRow
	// first is each line's first row; a hidden line's is the row of the
	// fold that hides it.
	first []int
}

// layout lays r out with the folds (sorted, outermost, not overlapping)
// hiding their lines, wrapping rows at wrap columns (none when 0).
func layout(r *rope, folds []foldRange, wrap, tab int) *display {
	n := r.lines()
	if len(folds) == 0 && wrap <= 0 {
		return &display{identity: true, lines: n}
	}
	d := &display{lines: n, rows: make([]dispRow, 0, n), first: make([]int, n)}
	f := 0
	for line := 0; line < n; line++ {
		row := len(d.rows)
		d.first[line] = row
		text := r.line(line)
		if wrap <= 0 {
			d.rows = append(d.rows, dispRow{line, 0, len(text)})
		} else {
			for _, w := range wrapLine(text, wrap, tab) {
				d.rows = append(d.rows, dispRow{line, w[0], w[1]})
			}
		}
		for f < len(folds) && folds[f].end < line {
			f++
		}
		if f < len(folds) && folds[f].start == line && folds[f].end > line {
			// A folded line shows as one row.
			d.rows = d.rows[:row+1]
			d.rows[row].end = len(text)
			for hidden := line + 1; hidden <= folds[f].end && hidden < n; hidden++ {
				d.first[hidden] = row
			}
			line = folds[f].end
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
		return dispRow{i, 0, len(r.line(i))}
	}
	return d.rows[i]
}

// rowOf is the row p shows in.
func (d *display) rowOf(p pos) int {
	if d.identity {
		return p.line
	}
	row := d.first[min(p.line, len(d.first)-1)]
	for row+1 < len(d.rows) && d.rows[row+1].line == p.line && d.rows[row+1].start <= p.col {
		row++
	}
	return row
}

// lineRow is the first row of line, or the row hiding it.
func (d *display) lineRow(line int) int {
	if d.identity {
		return line
	}
	return d.first[line]
}

// visibleLine reports whether line shows (no fold hides it).
func (d *display) visibleLine(line int) bool {
	return d.identity || d.rows[d.first[line]].line == line
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
