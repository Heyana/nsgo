package editor

import (
	"sort"
	"strings"
)

// chunkLines is the most lines a chunk of the rope holds before it splits.
const chunkLines = 512

// rope is the text as lines in chunks: finding a line is a binary search
// over the chunks' first lines, and an edit splices the lines of the
// chunks it touches, so 200 000 lines cost an edit a few chunks. Lines
// hold no line breaks; a text of n breaks has n+1 lines.
type rope struct {
	chunks [][]string
	// starts[i] is the first line of chunk i; starts[len(chunks)] the
	// line count.
	starts []int
}

func newRope(text string) *rope {
	r := &rope{}
	r.setLines(strings.Split(text, "\n"))
	return r
}

func (r *rope) setLines(lines []string) {
	r.chunks = r.chunks[:0]
	for len(lines) > chunkLines {
		r.chunks = append(r.chunks, lines[:chunkLines:chunkLines])
		lines = lines[chunkLines:]
	}
	r.chunks = append(r.chunks, lines)
	r.reindex(0)
}

func (r *rope) reindex(from int) {
	if len(r.starts) != len(r.chunks)+1 {
		r.starts = make([]int, len(r.chunks)+1)
		from = 0
	}
	n := 0
	if from > 0 {
		n = r.starts[from]
	}
	for i := from; i < len(r.chunks); i++ {
		r.starts[i] = n
		n += len(r.chunks[i])
	}
	r.starts[len(r.chunks)] = n
}

// lines is the line count.
func (r *rope) lines() int { return r.starts[len(r.chunks)] }

// chunkOf is the chunk holding line i.
func (r *rope) chunkOf(i int) int {
	return sort.Search(len(r.chunks), func(c int) bool { return r.starts[c+1] > i })
}

// line is line i.
func (r *rope) line(i int) string {
	c := r.chunkOf(i)
	return r.chunks[c][i-r.starts[c]]
}

// splice replaces lines from to to (excluded) with with, rechunking only
// the chunks it touches.
func (r *rope) splice(from, to int, with []string) {
	if from == to && len(with) == 0 {
		return
	}
	n := r.lines()
	first := r.chunkOf(min(from, n-1))
	last := max(first, r.chunkOf(min(max(to-1, from), n-1)))
	base := r.starts[first]
	var old []string
	for _, c := range r.chunks[first : last+1] {
		old = append(old, c...)
	}
	joined := make([]string, 0, len(old)-(to-from)+len(with))
	joined = append(joined, old[:from-base]...)
	joined = append(joined, with...)
	joined = append(joined, old[min(to-base, len(old)):]...)
	var parts [][]string
	for len(joined) > chunkLines {
		parts = append(parts, joined[:chunkLines:chunkLines])
		joined = joined[chunkLines:]
	}
	if len(joined) > 0 || len(parts) == 0 {
		parts = append(parts, joined)
	}
	tail := append([][]string(nil), r.chunks[last+1:]...)
	r.chunks = append(append(r.chunks[:first], parts...), tail...)
	// An empty chunk stays only as the whole text's one line.
	if len(r.chunks) > 1 {
		kept := r.chunks[:0]
		for _, c := range r.chunks {
			if len(c) > 0 {
				kept = append(kept, c)
			}
		}
		r.chunks = kept
	}
	r.reindex(first)
}

// text is the whole text.
func (r *rope) text() string {
	var b strings.Builder
	for i, c := range r.chunks {
		for j, l := range c {
			if i > 0 || j > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(l)
		}
	}
	return b.String()
}

// slice is the text from a to b.
func (r *rope) slice(a, b pos) string {
	if a.line == b.line {
		l := r.line(a.line)
		return l[min(a.col, len(l)):min(b.col, len(l))]
	}
	var s strings.Builder
	l := r.line(a.line)
	s.WriteString(l[min(a.col, len(l)):])
	for i := a.line + 1; i < b.line; i++ {
		s.WriteByte('\n')
		s.WriteString(r.line(i))
	}
	s.WriteByte('\n')
	l = r.line(b.line)
	s.WriteString(l[:min(b.col, len(l))])
	return s.String()
}

// pos is a place in the text: a line and a byte column on a char
// boundary.
type pos struct{ line, col int }

func (a pos) less(b pos) bool { return a.line < b.line || a.line == b.line && a.col < b.col }

func minPos(a, b pos) pos {
	if b.less(a) {
		return b
	}
	return a
}

func maxPos(a, b pos) pos {
	if a.less(b) {
		return b
	}
	return a
}

// clamp is p within the text, on a char boundary.
func (r *rope) clamp(p pos) pos {
	if p.line < 0 {
		return pos{}
	}
	if p.line >= r.lines() {
		last := r.lines() - 1
		return pos{last, len(r.line(last))}
	}
	l := r.line(p.line)
	c := max(min(p.col, len(l)), 0)
	for c > 0 && c < len(l) && !startsRune(l[c]) {
		c--
	}
	return pos{p.line, c}
}

func startsRune(b byte) bool { return b&0xc0 != 0x80 }

// end is the end of the text.
func (r *rope) end() pos {
	last := r.lines() - 1
	return pos{last, len(r.line(last))}
}
