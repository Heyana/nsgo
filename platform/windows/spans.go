//go:build windows

package windows

import (
	"math"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

var (
	getTextExtentExPointW = gdi32.NewProc("GetTextExtentExPointW")
	getGlyphOutlineW      = gdi32.NewProc("GetGlyphOutlineW")
)

// drawnText is a TextLayout of this backend: plain or with spans.
type drawnText interface {
	core.TextLayout
	draw(p *painter, x, y int32)
}

// glyphMetrics is GDI's GLYPHMETRICS.
type glyphMetrics struct {
	BlackBoxX, BlackBoxY uint32
	OriginX, OriginY     int32
	CellIncX, CellIncY   int16
}

// glyphTop returns how far the ink of r rises above the baseline in the
// font selected into dc, or fallback when GDI cannot tell.
func glyphTop(dc uintptr, r rune, fallback int32) int32 {
	const ggoMetrics = 0
	var gm glyphMetrics
	identity := [4]uint32{1 << 16, 0, 0, 1 << 16} // MAT2: 1.0 on the diagonal
	if n, _, _ := getGlyphOutlineW.Call(dc, uintptr(r), ggoMetrics, uintptr(unsafe.Pointer(&gm)), 0, 0,
		uintptr(unsafe.Pointer(&identity))); int32(n) == -1 || gm.OriginY <= 0 {
		return fallback
	}
	return gm.OriginY
}

// fontExtent returns the metrics an attachment is placed against in f.
func (w *window) fontExtent(f *font) core.FontExtent {
	pt := func(px int32) core.Points { return core.Points(float64(px) / w.scale) }
	return core.FontExtent{Ascent: pt(f.ascent), Descent: pt(f.height - f.ascent), XHeight: pt(f.xHeight)}
}

// inline is an attachment's placeholder, in pixels: what its line
// reserves above and below the baseline, and how far its top is above it.
type inline struct {
	index        int // in the text's attachments
	size         core.Size
	above, below int32
	top          int32
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
	// span is the index of its span in the text, or -1; bg is that
	// span's background.
	span int
	bg   core.TextBackground
	// att is the attachment whose placeholder the piece is, alone.
	att *inline
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
	// attachments counts the text's attachments.
	attachments int
}

func (w *window) prepareSpans(t *core.AttributedText) core.PreparedText {
	st := &spanText{w: w, align: t.Style.Align, maxLines: t.MaxLines, attachments: len(t.Attachments)}
	if t.Style.LineHeight > 0 {
		st.step = max(w.px(t.Style.LineHeight), 1)
	}
	byKey := map[fontKey]*font{}
	add := func(start, end int, style core.TextStyle, span int, bg core.TextBackground) {
		key := fontKey{style.Font, style.Decoration}
		f := byKey[key]
		if f == nil {
			f = w.fonts.acquire(key)
			byKey[key] = f
			st.fonts = append(st.fonts, f)
		}
		if bg.Color&0xff == 0 {
			bg = core.TextBackground{}
		}
		st.split(t, start, end, f, style.Color, w.px(style.LetterSpacing), span, bg)
	}
	at := 0
	for i, sp := range t.Spans {
		start, end := min(max(sp.Start, at), len(t.Text)), min(max(sp.End, at), len(t.Text))
		if start > at {
			add(at, start, t.Style, -1, core.TextBackground{})
		}
		if end > start {
			add(start, end, sp.Style, i, sp.Background)
		}
		at = max(at, end)
	}
	if at < len(t.Text) || len(st.pieces) == 0 {
		add(at, len(t.Text), t.Style, -1, core.TextBackground{})
	}
	st.measure()
	return st
}

// split adds t.Text[start:end] as pieces in one style.
func (st *spanText) split(t *core.AttributedText, start, end int, f *font, color core.Color, extra int32,
	span int, bg core.TextBackground) {
	text := t.Text
	for i := start; i < end; {
		r, size := utf8.DecodeRuneInString(text[i:end])
		if r == core.AttachmentRune {
			if k := attachmentAt(t, i); k >= 0 {
				st.pieces = append(st.pieces, piece{kind: pieceWord, text: []uint16{uint16(r)}, offsets: []int{i},
					font: f, color: color, extra: extra, span: span, bg: bg, att: st.w.inline(t.Attachments[k], k, f)})
				i += size
				continue
			}
		}
		kind := pieceWord
		switch r {
		case '\n':
			kind = pieceBreak
		case ' ', '\t':
			kind = pieceSpace
		}
		n := len(st.pieces)
		if kind == pieceBreak || n == 0 || st.pieces[n-1].kind != kind || st.pieces[n-1].font != f ||
			st.pieces[n-1].color != color || st.pieces[n-1].extra != extra || st.pieces[n-1].att != nil || i == start {
			st.pieces = append(st.pieces, piece{kind: kind, font: f, color: color, extra: extra, span: span, bg: bg})
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

// attachmentAt returns the index of t's attachment at byte offset i, or
// -1.
func attachmentAt(t *core.AttributedText, i int) int {
	for k, a := range t.Attachments {
		if a.Offset == i {
			return k
		}
	}
	return -1
}

// inline returns the placeholder of attachment a, the kth of its text,
// set in font f.
func (w *window) inline(a core.TextAttachment, k int, f *font) *inline {
	font := w.fontExtent(f)
	top, height := w.px(a.Ascent(font)), w.px(a.Size.Height)
	return &inline{index: k, size: a.Size, above: max(top, 0), below: max(height-top, 0), top: top}
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
		if p.att != nil {
			p.ends = []int32{max(st.w.px(p.att.size.Width), 0)}
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
	ascent, descent int32 // its fonts'
	// above and below are what its attachments reserve around the
	// baseline.
	above, below int32
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
			if a := r.p.att; a != nil {
				cur.above = max(cur.above, a.above)
				cur.below = max(cur.below, a.below)
			}
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

// extent returns how far line l reaches above and below its baseline: its
// fonts centered in the line height, grown where attachments reach past
// them.
func (st *spanText) extent(l *textLine) (above, below int32) {
	above, below = l.ascent, l.descent
	if st.step > 0 {
		above = (st.step-l.ascent-l.descent)/2 + l.ascent
		below = st.step - above
	}
	return max(above, l.above), max(below, l.below)
}

func (st *spanText) height(l *textLine) int32 {
	above, below := st.extent(l)
	return above + below
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
	l := &spanLayout{fonts: &st.w.fonts, attachments: make([]attachmentBox, st.attachments)}
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
		above, below := st.extent(line)
		h := above + below
		baseline := top + above
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
			if a := r.p.att; a != nil {
				l.attachments[a.index] = attachmentBox{shown: true, box: core.Bounds{
					Origin: st.w.point(dx+r.x, baseline-a.top), Size: a.size}}
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
		l.backgrounds = st.backgrounds(l.backgrounds, runs, dx, baseline)
		l.lines = append(l.lines, lineBox{top: top, bottom: top + h, first: first, last: len(l.runs)})
		top += h
	}
	return l
}

// backgrounds appends the boxes of the span backgrounds of a line's runs
// to out: one per span, from its font's ascent to its descent, cut to the
// line's ink, so the spaces hanging at its end are not painted.
func (st *spanText) backgrounds(out []backgroundBox, runs []run, dx, baseline int32) []backgroundBox {
	end := len(runs)
	for end > 0 && (runs[end-1].p.kind != pieceWord || runs[end-1].to <= runs[end-1].from) {
		end--
	}
	for i := 0; i < end; {
		r := runs[i]
		j := i + 1
		for j < end && runs[j].p.span == r.p.span && runs[j].p.bg == r.p.bg {
			j++
		}
		if bg := r.p.bg; bg.Color&0xff != 0 {
			last := runs[j-1]
			f := r.p.font
			b := rect{dx + r.x, baseline - f.ascent, dx + last.x + last.p.width(last.from, last.to), baseline + f.height - f.ascent}
			if b.Right > b.Left {
				radius := min(float64(st.w.px(bg.Radius)), float64(b.Bottom-b.Top)/2)
				out = append(out, backgroundBox{r: b, color: bg.Color, radius: max(radius, 0)})
			}
		}
		i = j
	}
	return out
}

// ellipsize shortens line to end with "…" within width and returns the
// ellipsis's run.
func (st *spanText) ellipsize(line *textLine, width int32) *run {
	if len(line.runs) == 0 {
		return nil
	}
	lastRun := line.runs[len(line.runs)-1]
	e := &piece{kind: pieceWord, text: []uint16{'…'}, font: lastRun.p.font, color: lastRun.p.color,
		offsets: []int{lastRun.p.offsets[max(lastRun.to-1, 0)]}, span: -1}
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

// backgroundBox is a span's background on one line, relative to the
// content's origin, in pixels.
type backgroundBox struct {
	r      rect
	color  core.Color
	radius float64
}

type attachmentBox struct {
	box   core.Bounds // in the content box, in points
	shown bool
}

type lineBox struct {
	top, bottom int32
	first, last int // runs
}

// spanLayout is spanned text laid out in runs, ready to draw.
type spanLayout struct {
	fonts       *fontCache
	held        []*font
	runs        []laidRun
	lines       []lineBox
	backgrounds []backgroundBox
	attachments []attachmentBox
}

// Attachment implements core.AttachmentLayout.
func (l *spanLayout) Attachment(i int) (core.Bounds, bool) {
	if i < 0 || i >= len(l.attachments) {
		return core.Bounds{}, false
	}
	return l.attachments[i].box, l.attachments[i].shown
}

// Release implements core.TextLayout on the main thread.
func (l *spanLayout) Release() {
	for _, f := range l.held {
		l.fonts.release(f)
	}
}

// draw paints the span backgrounds, then the glyphs; attachments are
// the text view's children, drawn after it. GDI fonts draw underlines
// and strikethroughs themselves.
func (l *spanLayout) draw(p *painter, x, y int32) {
	for _, b := range l.backgrounds {
		p.paintShape(b.r.offset(x, y), b.radius, b.color)
	}
	dc := p.dc
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
