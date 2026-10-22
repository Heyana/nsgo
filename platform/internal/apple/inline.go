//go:build darwin

package apple

import (
	"structs"
	"sync"

	"github.com/ebitengine/purego"

	"github.com/oneo-me/nsgo/core"
)

// What CoreText draws besides glyphs: attachments' placeholders sized by
// run delegates, span backgrounds and strikethrough, which CoreText has no
// attribute for.

var (
	ctRunDelegateCreate     func(callbacks *runDelegateCallbacks, refCon uintptr) uintptr
	ctFontGetXHeight        func(uintptr) float64
	ctFontGetUnderlineThick func(uintptr) float64

	attrRunDelegate uintptr
)

func loadInline(ct uintptr) {
	reg := purego.RegisterLibFunc
	reg(&ctRunDelegateCreate, ct, "CTRunDelegateCreate")
	reg(&ctFontGetXHeight, ct, "CTFontGetXHeight")
	reg(&ctFontGetUnderlineThick, ct, "CTFontGetUnderlineThickness")
	attrRunDelegate = constant(ct, "kCTRunDelegateAttributeName")
	delegateCallbacks = runDelegateCallbacks{
		version:    1, // kCTRunDelegateVersion1
		dealloc:    purego.NewCallback(func(ref uintptr) { extents.Delete(ref) }),
		getAscent:  purego.NewCallback(func(ref uintptr) cgFloat { return cgFloat{V: extentOf(ref).above} }),
		getDescent: purego.NewCallback(func(ref uintptr) cgFloat { return cgFloat{V: extentOf(ref).below} }),
		getWidth:   purego.NewCallback(func(ref uintptr) cgFloat { return cgFloat{V: extentOf(ref).width} }),
	}
}

// runDelegateCallbacks is CTRunDelegateCallbacks. It lives in a global, so
// the collector never moves it.
type runDelegateCallbacks struct {
	_                                        structs.HostLayout
	version                                  int
	dealloc, getAscent, getDescent, getWidth uintptr
}

var delegateCallbacks runDelegateCallbacks

// cgFloat returns a CGFloat from a callback: purego returns floats from
// callbacks only inside structs, which the ABI returns in a float
// register.
type cgFloat struct {
	_ structs.HostLayout
	V float64
}

// extents holds each run delegate's metrics by its refCon, until
// CoreText deallocates the delegate.
var (
	extents    sync.Map // uintptr → extent
	nextExtent uintptr
	extentMu   sync.Mutex
)

type extent struct{ above, below, width float64 }

func extentOf(ref uintptr) extent {
	e, _ := extents.Load(ref)
	x, _ := e.(extent)
	return x
}

// runDelegate returns a new CTRunDelegate of e. The caller releases it.
func runDelegate(e extent) uintptr {
	extentMu.Lock()
	nextExtent++
	ref := nextExtent
	extentMu.Unlock()
	extents.Store(ref, e)
	return ctRunDelegateCreate(&delegateCallbacks, ref)
}

// fontExtent returns the metrics an attachment is placed against in st's
// font.
func fontExtent(st core.TextStyle) core.FontExtent {
	f := Font(st.Font)
	defer Release(f)
	return core.FontExtent{
		Ascent:  core.Points(ctFontGetAscent(f)),
		Descent: core.Points(ctFontGetDescent(f)),
		XHeight: core.Points(ctFontGetXHeight(f)),
	}
}

// decoration is a UTF-16 range drawn on: a span's background box, or a
// strikethrough.
type decoration struct {
	start, end int
	color      core.Color
	strike     bool
	radius     float64
	// ascent and descent bound a background; offset is a strike's height
	// above the baseline and thickness its width.
	ascent, descent, offset, thickness float64
}

// decorations lists what t's spans draw besides glyphs, in UTF-16
// offsets.
func decorations(t *core.AttributedText, toUTF16 []int) []decoration {
	type run struct {
		start, end int
		style      core.TextStyle
		background core.TextBackground
	}
	var runs []run
	if len(t.Spans) == 0 {
		runs = append(runs, run{0, len(t.Text), t.Style, core.TextBackground{}})
	}
	for _, sp := range t.Spans {
		runs = append(runs, run{sp.Start, sp.End, sp.Style, sp.Background})
	}
	var out []decoration
	for _, r := range runs {
		start, end := min(max(r.start, 0), len(t.Text)), min(max(r.end, 0), len(t.Text))
		strike := r.style.Decoration&core.TextDecorationLineThrough != 0
		if end <= start || (!strike && r.background.Color == 0) {
			continue
		}
		f := Font(r.style.Font)
		d := decoration{start: toUTF16[start], end: toUTF16[end],
			ascent: ctFontGetAscent(f), descent: ctFontGetDescent(f)}
		if r.background.Color != 0 {
			bg := d
			bg.color, bg.radius = r.background.Color, float64(r.background.Radius)
			out = append(out, bg)
		}
		if strike {
			d.strike, d.color = true, r.style.Color
			d.thickness = max(ctFontGetUnderlineThick(f), 1)
			d.offset = ctFontGetXHeight(f) / 2
			out = append(out, d)
		}
		Release(f)
	}
	return out
}

// box is a decoration placed in a text's box: from its top left.
type box struct {
	rect   CGRect
	color  core.Color
	radius float64
	strike bool
}

// place puts l's decorations on its lines, and its attachments.
func (l *Layout) place(decos []decoration, atts []core.TextAttachment, fonts []core.FontExtent, toUTF16 []int) {
	lines, baselines, bases := l.placed()
	for _, d := range decos {
		for i, line := range lines {
			r := ctLineGetStringRange(line)
			from, to := r.Location+bases[i], r.Location+r.Length+bases[i]
			a, b := max(d.start, from), min(d.end, to)
			// Hanging spaces and breaks at a line's end are not painted.
			for b > a && l.blankAt(b-1) {
				b--
			}
			if a >= b {
				continue
			}
			x0 := ctLineOffsetForIndex(line, a-bases[i], nil)
			x1 := ctLineOffsetForIndex(line, b-bases[i], nil)
			if x1 < x0 {
				x0, x1 = x1, x0
			}
			o := baselines[i]
			if d.strike {
				l.over = append(l.over, box{rect: Rect(o.X+x0, o.Y-d.offset-d.thickness/2, x1-x0, d.thickness), color: d.color})
				continue
			}
			h := d.ascent + d.descent
			l.under = append(l.under, box{rect: Rect(o.X+x0, o.Y-d.ascent, x1-x0, h), color: d.color,
				radius: min(d.radius, h/2)})
		}
	}
	l.attachments = make([]attachmentBox, len(atts))
	for k, at := range atts {
		idx := toUTF16[at.Offset]
		for i, line := range lines {
			r := ctLineGetStringRange(line)
			if idx < r.Location+bases[i] || idx >= r.Location+r.Length+bases[i] {
				continue
			}
			var asc, desc, lead float64
			width := ctLineGetTypographicBounds(line, &asc, &desc, &lead)
			x := ctLineOffsetForIndex(line, idx-bases[i], nil)
			if x+float64(at.Size.Width) > width+0.5 {
				break // behind the ellipsis
			}
			o := baselines[i]
			top := o.Y - float64(at.Ascent(fonts[k]))
			l.attachments[k] = attachmentBox{core.Bounds{
				Origin: core.Point{X: core.Points(o.X + x), Y: core.Points(top)}, Size: at.Size}, true}
			break
		}
	}
}

type attachmentBox struct {
	box   core.Bounds
	shown bool
}

// blankAt reports whether UTF-16 offset i is a space or a line break.
func (l *Layout) blankAt(i int) bool {
	if i < 0 || i >= len(l.toUTF8)-1 {
		return false
	}
	c := l.text[l.toUTF8[i]]
	return c == ' ' || c == '\n' || c == '\r' || c == '\t'
}

// Attachment implements core.AttachmentLayout.
func (l *Layout) Attachment(i int) (core.Bounds, bool) {
	if l == nil || i < 0 || i >= len(l.attachments) {
		return core.Bounds{}, false
	}
	return l.attachments[i].box, l.attachments[i].shown
}

// DrawBackgrounds fills the backgrounds of l's spans in box of a flipped
// context, behind the selection and the glyphs.
func (l *Layout) DrawBackgrounds(ctx uintptr, at CGRect) {
	if l == nil || ctx == 0 {
		return
	}
	fillBoxes(ctx, l.under, at)
}

func fillBoxes(ctx uintptr, boxes []box, at CGRect) {
	for _, b := range boxes {
		r := b.rect
		r.Origin.X += at.Origin.X
		r.Origin.Y += at.Origin.Y
		color := CGColor(b.color)
		cgContextSaveGState(ctx)
		cgContextSetFillColor(ctx, color)
		if b.radius > 0 {
			path := cgPathCreateWithRoundedRect(r, b.radius, b.radius, 0)
			cgContextAddPath(ctx, path)
			Release(path)
		} else {
			cgContextAddRect(ctx, r)
		}
		cgContextFillPath(ctx)
		cgContextRestoreGState(ctx)
		Release(color)
	}
}
