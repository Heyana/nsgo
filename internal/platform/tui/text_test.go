package tui

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestCellTextGraphemesAndCarets(t *testing.T) {
	p := (textEngine{}).Prepare(&driver.AttributedText{Text: "A界e\u0301👩‍💻Z"})
	if got := p.Measure(driver.Auto); got != (driver.Size{Width: 56, Height: 16}) {
		t.Fatalf("unbounded: %+v", got)
	}
	l := p.Layout(driver.Size{Width: 32}).(*textLayout)
	if len(l.lines) != 2 || l.lines[0].Text != "A界e\u0301" || l.lines[1].Text != "👩‍💻Z" {
		t.Fatalf("grapheme wrapping: %+v", l.lines)
	}
	if got := l.HitTest(driver.Point{X: 16, Y: 16}); got.Offset != 9 {
		t.Fatalf("emoji UTF-16 caret: %+v", got)
	}
	if box, ok := l.CaretRect(driver.Caret{Offset: 4}); !ok || box.Origin.Y != 16 {
		t.Fatalf("wrapped caret: %+v %v", box, ok)
	}
	if boxes := l.RangeRects(1, 3, nil); len(boxes) != 1 || boxes[0].Size.Width != 24 {
		t.Fatalf("selection: %+v", boxes)
	}
}

func TestCellTextBreaksTabsAndSafeControls(t *testing.T) {
	p := (textEngine{}).Prepare(&driver.AttributedText{Text: "a\tb\n\n\x1b[31m\r\n"})
	l := p.Layout(driver.Size{Width: 160}).(*textLayout)
	if len(l.lines) != 4 || l.lines[0].Bounds.Size.Width != 40 || l.size.Height != 64 {
		t.Fatalf("lines/tabs: %+v", l.lines)
	}
	for _, g := range l.glyphs {
		if g.text == "\x1b" || g.text == "\r" {
			t.Fatalf("unsafe glyph: %q", g.text)
		}
	}
	if got := safeText("\x1b]0;bad\a\x7f"); got != "�]0;bad��" {
		t.Fatalf("sanitization: %q", got)
	}
}

func TestCellTextStylesAndAlignmentAreImmutable(t *testing.T) {
	text := driver.AttributedText{Text: "ab", Style: driver.TextStyle{Align: driver.TextAlignCenter, LineHeight: 17}, Spans: []driver.TextSpan{{Start: 1, End: 2, Style: driver.TextStyle{Color: 0xff0000ff}, Hidden: true}}}
	p := (textEngine{}).Prepare(&text)
	text.Spans[0].Hidden = false
	l := p.Layout(driver.Size{Width: 64}).(*textLayout)
	if l.glyphs[0].x != 24 || !l.glyphs[1].hidden || l.lines[0].Bounds.Size.Height != 32 {
		t.Fatalf("style/layout: %+v", l)
	}
}

func TestCellTextLineLimitKeepsWideEllipsisGeometry(t *testing.T) {
	for _, mode := range []driver.EllipsizeMode{driver.EllipsizeTail, driver.EllipsizeClip} {
		p := (textEngine{}).Prepare(&driver.AttributedText{Text: "界界界", MaxLines: 1, Ellipsize: mode})
		l := p.Layout(driver.Size{Width: 32}).(*textLayout)
		if len(l.lines) != 1 || l.size.Height != 16 {
			t.Fatalf("line limit: %+v", l)
		}
		last := l.glyphs[len(l.glyphs)-1]
		if mode == driver.EllipsizeTail && (last.text != "…" || last.width != 1 || l.size.Width != 24) {
			t.Fatalf("wide ellipsis: %+v size%+v", last, l.size)
		}
		if mode == driver.EllipsizeClip && (last.text != "界" || last.width != 2 || l.size.Width != 32) {
			t.Fatalf("wide clip: %+v size%+v", last, l.size)
		}
	}
}

func TestCellTextEllipsisAfterHardBreak(t *testing.T) {
	l := (textEngine{}).Prepare(&driver.AttributedText{Text: "abc\nnext", MaxLines: 1}).Layout(driver.Size{Width: 80}).(*textLayout)
	var text string
	for _, g := range l.glyphs {
		text += g.text
	}
	if text != "abc…" || l.size.Width != 32 {
		t.Fatalf("hard-break ellipsis: %q %+v", text, l.size)
	}
}
