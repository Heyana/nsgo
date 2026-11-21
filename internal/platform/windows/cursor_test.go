//go:build windows

package windows

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestCursorAtTakesASpansCursorAheadOfItsText finds a span's cursor over its characters, the text's own over the others, and none off the text.
func TestCursorAtTakesASpansCursorAheadOfItsText(t *testing.T) {
	w := testWindow()
	style := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 16, Weight: 400}, Color: driver.RGB(0)}
	at := &driver.AttributedText{Text: "plain link", Style: style, Spans: []driver.TextSpan{
		{Start: 0, End: 6, Style: style},
		{Start: 6, End: 10, Style: style, Cursor: driver.CursorPointer},
	}}
	p := w.Prepare(at)
	defer p.Release()
	size := p.Measure(driver.Auto)
	l := p.Layout(size).(*textLayout)
	defer l.Release()

	shown := driver.ViewStyle{Opacity: 1}
	root := &view{id: 1, kind: driver.KindView, style: shown, frame: driver.Bounds{Size: driver.Size{Width: 400, Height: 200}}}
	content := driver.Bounds{Origin: driver.Point{X: 2, Y: 3}, Size: size}
	text := &view{id: 2, kind: driver.KindText, parent: root, style: shown, text: l, content: content, spans: at.Spans,
		frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 20}, Size: driver.Size{Width: size.Width + 4, Height: size.Height + 6}}}
	text.style.Cursor = driver.CursorHelp
	root.children = []*view{text}
	w.root, w.views = 1, []*view{nil, root, text}

	y := 20 + 3 + size.Height/2
	if c, _, ok := w.cursorAt(driver.Point{X: 10 + 2 + 2, Y: y}); c != driver.CursorHelp || !ok {
		t.Errorf("over the plain span: %v, %v; want the text's help cursor", c, ok)
	}
	if c, _, ok := w.cursorAt(driver.Point{X: 10 + 2 + size.Width - 2, Y: y}); c != driver.CursorPointer || !ok {
		t.Errorf("over the link: %v, %v; want the span's pointer", c, ok)
	}
	if c, _, ok := w.cursorAt(driver.Point{X: 300, Y: 150}); ok {
		t.Errorf("off the text: %v; want none", c)
	}
}
