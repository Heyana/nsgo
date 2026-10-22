//go:build windows

package windows

import (
	"math"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

var getTextExtentExPointW = gdi32.NewProc("GetTextExtentExPointW")

// drawnText is a TextLayout of this backend: plain or with spans.
type drawnText interface {
	core.TextLayout
	draw(dc uintptr, x, y int32)
}

type pieceKind uint8

const (
	pieceWord pieceKind = iota
	pieceSpace
	pieceBreak
)

// piece is a word, a run of spaces or a line break in one style.
type piece struct {
	kind    pieceKind
	text    []uint16
	offsets []int   // each unit's UTF-8 offset in the whole text
	ends    []int32 // the width up to the end of each unit, in pixels
	font    *font
	color   core.Color
	extra   int32
}

func (p *piece) width(from, to int) int32 {
	if to <= from {
		return 0
	}
	w := p.ends[to-1]
	if from > 0 {
		w -= p.ends[from-1]
	}
	return w
}

// fit returns how far from from the piece fits in avail, keeping
// surrogate pairs whole.
func (p *piece) fit(from int, avail int32) int {
	k := from
	for k < len(p.text) && p.width(from, k+1) <= avail {
		k++
	}
	if k > from && k < len(p.text) && utf16.IsSurrogate(rune(p.text[k])) && p.text[k] >= 0xdc00 {
		k--
	}
	return k
}

// spanText is text with styled spans. DrawText takes one font, so it is
// broken into lines and drawn here run by run.
type spanText struct {
	w        *window
	pieces   []piece
	fonts    []*font
	align    core.TextAlign
	step     int32 // a line height in pixels; 0 is each line's fonts'
	maxLines int
}

func (w *window) prepareSpans(t *core.AttributedText) core.PreparedText {
	st := &spanText{w: w, align: t.Style.Align, maxLines: t.MaxLines}
	if t.Style.LineHeight > 0 {
		st.step = max(w.px(t.Style.LineHeight), 1)
	}
	byKey := map[fontKey]*font{}
	add := func(start, end int, style core.TextStyle) {
		key := fontKey{style.Font, style.Decoration}
		f := byKey[key]
		if f == nil {
			f = w.fonts.acquire(key)
			byKey[key] = f
			st.fonts = append(st.fonts, f)
		}
		st.split(t.Text, start, end, f, style.Color, w.px(style.LetterSpacing))
	}
	at := 0
	for _, sp := range t.Spans {
		start, end := min(max(sp.Start, at), len(t.Text)), min(max(sp.End, at), len(t.Text))
		if start > at {
			add(at, start, t.Style)
		}
		if end > start {
			add(start, end, sp.Style)
		}
		at = max(at, end)
	}
	if at < len(t.Text) || len(st.pieces) == 0 {
		add(at, len(t.Text), t.Style)
	}
	st.measure()
	return st
}

// split adds text[start:end] as pieces in one style.
func (st *spanText) split(text string, start, end int, f *font, color core.Color, extra int32) {
	for i := start; i < end; {
		r, size := utf8.DecodeRuneInString(text[i:end])
		kind := pieceWord
		switch r {
		case '\n':
			kind = pieceBreak
		case ' ', '\t':
			kind = pieceSpace
		}
		n := len(st.pieces)
		if kind == pieceBreak || n == 0 || st.pieces[n-1].kind != kind || st.pieces[n-1].font != f ||
			st.pieces[n-1].color != color || st.pieces[n-1].extra != extra || i == start {
			st.pieces = append(st.pieces, piece{kind: kind, font: f, color: color, extra: extra})
			n++
		}
		p := &st.pieces[n-1]
		if kind != pieceBreak {
			if r == '\t' {
				r = ' '
			}
			for _, u := range utf16.AppendRune(nil, r) {
				p.text = append(p.text, u)
				p.offsets = append(p.offsets, i)
			}
		} else {
			p.offsets = append(p.offsets, i)
		}
		i += size
	}
}

// measure finds each unit's extent, on the engine thread.
func (st *spanText) measure() {
	w := st.w
	if w.measureDC == 0 {
		w.measureDC, _, _ = createCompatibleDC.Call(0)
	}
	dc := w.measureDC
	for i := range st.pieces {
		p := &st.pieces[i]
		if len(p.text) == 0 {
			continue
		}
		p.ends = make([]int32, len(p.text))
		old, _, _ := selectObject.Call(dc, p.font.handle)
		setTextCharacterExtra.Call(dc, uintptr(p.extra))
		var size [2]int32
		getTextExtentExPointW.Call(dc, uintptr(unsafe.Pointer(&p.text[0])), uintptr(len(p.text)), 0, 0,
			uintptr(unsafe.Pointer(&p.ends[0])), uintptr(unsafe.Pointer(&size)))
		setTextCharacterExtra.Call(dc, 0)
		selectObject.Call(dc, old)
	}
}

type run struct {
	p        *piece
	from, to int
	x        int32
}

type textLine struct {
	runs            []run
	width           int32 // without trailing spaces
	ascent, descent int32
}

// wrap breaks the pieces into lines at most limit pixels wide (< 0: no
// limit), at spaces, or inside words longer than a line.
func (st *spanText) wrap(limit int32) []textLine {
	var lines []textLine
	var cur textLine
	var x int32
	last := st.fonts[0]
	words := false
	flush := func() {
		for _, r := range cur.runs {
			if r.p.kind == pieceWord {
				cur.width = r.x + r.p.width(r.from, r.to)
			}
		}
		f := last
		for _, r := range cur.runs {
			f = r.p.font
			cur.ascent = max(cur.ascent, f.ascent)
			cur.descent = max(cur.descent, f.height-f.ascent)
		}
		if len(cur.runs) == 0 {
			cur.ascent, cur.descent = f.ascent, f.height-f.ascent
		}
		lines = append(lines, cur)
		cur, x, words = textLine{}, 0, false
	}
	for i := range st.pieces {
		p := &st.pieces[i]
		last = p.font
		n := len(p.text)
		switch p.kind {
		case pieceBreak:
			flush()
		case pieceSpace:
			cur.runs = append(cur.runs, run{p, 0, n, x})
			x += p.width(0, n)
		case pieceWord:
			if limit >= 0 && words && x+p.width(0, n) > limit {
				flush()
			}
			from := 0
			for limit >= 0 && x+p.width(from, n) > limit {
				k := p.fit(from, limit-x)
				if k == from {
					if len(cur.runs) > 0 {
						flush()
						continue
					}
					k = from + 1 // at least one unit a line
				}
				cur.runs = append(cur.runs, run{p, from, k, x})
				words = true
				flush()
				from = k
			}
			if from < n {
				cur.runs = append(cur.runs, run{p, from, n, x})
				x += p.width(from, n)
				words = true
			}
		}
	}
	flush()
	return lines
}

func (st *spanText) height(l *textLine) int32 {
	if st.step > 0 {
		return st.step
	}
	return l.ascent + l.descent
}

func (st *spanText) Measure(maxWidth core.Points) core.Size {
	limit := int32(-1)
	if !maxWidth.IsAuto() {
		limit = int32(float64(maxWidth) * st.w.scale)
	}
	lines := st.wrap(limit)
	if st.maxLines > 0 && len(lines) > st.maxLines {
		lines = lines[:st.maxLines]
	}
	var width, height int32
	for i := range lines {
		width = max(width, lines[i].width)
		height += st.height(&lines[i])
	}
	if limit >= 0 {
		width = min(width, limit)
	}
	return core.Size{Width: core.Points(float64(width) / st.w.scale), Height: core.Points(float64(height) / st.w.scale)}
}

func (st *spanText) Layout(size core.Size) core.TextLayout {
	width := int32(math.Ceil(float64(size.Width) * st.w.scale))
	lines := st.wrap(width)
	cut := st.maxLines > 0 && len(lines) > st.maxLines
	if cut {
		lines = lines[:st.maxLines]
	}
	l := &spanLayout{fonts: &st.w.fonts}
	held := map[*font]bool{}
	hold := func(f *font) {
		if !held[f] {
			held[f] = true
			st.w.fonts.retain(f)
			l.held = append(l.held, f)
		}
	}
	var top int32
	for i := range lines {
		line := &lines[i]
		var ellipsis *run
		if cut && i == len(lines)-1 {
			ellipsis = st.ellipsize(line, width)
		}
		h := st.height(line)
		baseline := top + (h-line.ascent-line.descent)/2 + line.ascent
		var dx int32
		switch st.align {
		case core.TextAlignCenter:
			dx = (width - line.width) / 2
		case core.TextAlignEnd:
			dx = width - line.width
		}
		first := len(l.runs)
		runs := line.runs
		if ellipsis != nil {
			runs = append(runs, *ellipsis)
		}
		for _, r := range runs {
			if r.to <= r.from {
				continue
			}
			hold(r.p.font)
			ends := make([]int32, r.to-r.from)
			for k := range ends {
				ends[k] = r.p.width(r.from, r.from+k+1)
			}
			l.runs = append(l.runs, laidRun{
				text: r.p.text[r.from:r.to], offsets: r.p.offsets[r.from:r.to], ends: ends,
				x: dx + r.x, baseline: baseline, font: r.p.font, color: r.p.color, extra: r.p.extra,
			})
		}
		l.lines = append(l.lines, lineBox{top: top, bottom: top + h, first: first, last: len(l.runs)})
		top += h
	}
	return l
}

// ellipsize shortens line to end with "…" within width and returns the
// ellipsis's run.
func (st *spanText) ellipsize(line *textLine, width int32) *run {
	if len(line.runs) == 0 {
		return nil
	}
	lastRun := line.runs[len(line.runs)-1]
	e := &piece{kind: pieceWord, text: []uint16{'…'}, font: lastRun.p.font, color: lastRun.p.color,
		offsets: []int{lastRun.p.offsets[max(lastRun.to-1, 0)]}}
	st.measurePiece(e)
	avail := width - e.width(0, 1)
	for len(line.runs) > 0 {
		r := &line.runs[len(line.runs)-1]
		if r.p.kind == pieceWord {
			if k := r.p.fit(r.from, avail-r.x); k > r.from {
				r.to = k
				break
			}
		}
		line.runs = line.runs[:len(line.runs)-1]
	}
	var x int32
	if n := len(line.runs); n > 0 {
		r := line.runs[n-1]
		x = r.x + r.p.width(r.from, r.to)
	}
	line.width = x + e.width(0, 1)
	return &run{e, 0, 1, x}
}

func (st *spanText) measurePiece(p *piece) {
	pieces := st.pieces
	st.pieces = []piece{*p}
	st.measure()
	p.ends = st.pieces[0].ends
	st.pieces = pieces
}

func (st *spanText) Release() {
	for _, f := range st.fonts {
		st.w.fonts.release(f)
	}
}

type laidRun struct {
	text        []uint16
	offsets     []int
	ends        []int32 // relative to x
	x, baseline int32   // relative to the content's origin, in pixels
	font        *font
	color       core.Color
	extra       int32
}

type lineBox struct {
	top, bottom int32
	first, last int // runs
}

// spanLayout is spanned text laid out in runs, ready to draw.
type spanLayout struct {
	fonts *fontCache
	held  []*font
	runs  []laidRun
	lines []lineBox
}

// Release implements core.TextLayout on the main thread.
func (l *spanLayout) Release() {
	for _, f := range l.held {
		l.fonts.release(f)
	}
}

func (l *spanLayout) draw(dc uintptr, x, y int32) {
	setTextAlign.Call(dc, taBaseline)
	for _, r := range l.runs {
		if r.color&0xff == 0 {
			continue
		}
		old, _, _ := selectObject.Call(dc, r.font.handle)
		setTextColor.Call(dc, colorRef(r.color))
		setTextCharacterExtra.Call(dc, uintptr(r.extra))
		textOutW.Call(dc, uintptr(x+r.x), uintptr(y+r.baseline), uintptr(unsafe.Pointer(&r.text[0])), uintptr(len(r.text)))
		setTextCharacterExtra.Call(dc, 0)
		selectObject.Call(dc, old)
	}
	setTextAlign.Call(dc, 0)
}

// indexAt returns the UTF-8 offset of the character at x, y, in pixels
// from the content's origin, or -1.
func (l *spanLayout) indexAt(x, y int32) int {
	for _, line := range l.lines {
		if y < line.top || y >= line.bottom {
			continue
		}
		for _, r := range l.runs[line.first:line.last] {
			if x < r.x {
				continue
			}
			for i, e := range r.ends {
				if x-r.x < e {
					return r.offsets[i]
				}
			}
		}
	}
	return -1
}
