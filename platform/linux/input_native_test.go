//go:build linux && !android

package linux

import (
	"fmt"
	"strings"
	"testing"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/engine"
)

// editingApp is a multiline field capped at 12 characters and a single
// line. The title shows the note (line breaks as /), its content height
// being above one line's, its selection, and the line's submits and blurs.
func editingApp(title string) engine.App {
	return func(cx *core.Scope) core.Node {
		note, line := core.NewSignal(cx, ""), core.NewSignal(cx, "")
		height := core.NewSignal(cx, core.Points(0))
		sel := core.NewSignal(cx, core.TextRange{})
		submitted, blurred := core.NewSignal(cx, 0), core.NewSignal(cx, 0)
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|%s|%v|%d-%d|%d|%d", title, strings.ReplaceAll(note.Get(), "\n", "/"),
				height.Get() > 30, sel.Get().Start, sel.Get().End, submitted.Get(), blurred.Get()))
		})
		return core.View{
			Style: core.Style{Width: core.Pt(320), Height: core.Pt(240), Padding: core.All(10), Gap: 10,
				Background: core.RGB(0xffffff)},
			Children: []core.Node{
				core.Input{Value: note.Get, OnChange: note.Set, Multiline: true, MaxLength: 12, TestID: "note",
					OnSelectionChange:   sel.Set,
					OnContentSizeChange: func(s core.Size) { height.Set(s.Height) },
					Style:               core.Style{Width: core.Pt(300), MinHeight: core.Pt(30), FontSize: 16}},
				core.Input{Value: line.Get, OnChange: line.Set, TestID: "line",
					OnSubmit: func(string) { submitted.Update(func(n int) int { return n + 1 }) },
					OnBlur:   func() { blurred.Update(func(n int) int { return n + 1 }) },
					Style:    core.Style{Width: core.Pt(300), Height: core.Pt(30), FontSize: 16}},
			},
		}
	}
}

// TestEditorsTakeLinesLimitsAndSubmit inserts into a GtkTextView, which
// MaxLength cuts, selects in it from the app, and submits a GtkEntry,
// which loses the focus.
func TestEditorsTakeLinesLimitsAndSubmit(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-editing"
	w := startTestWindow(t, editingApp(title), Options{Title: title, Width: 320, Height: 240})
	waitForTitle(t, w, title+"||false|0-0|0|0")
	var note, line *view
	w.onLoop(t, func() {
		for _, v := range w.entries {
			if v.textView != 0 {
				note = v
			} else if v.kind == core.KindInput {
				line = v
			}
		}
		w.focusEntry(note)
		gtkTextBufferInsertAtCursor(note.buffer, cstr("one\ntwo"), -1)
	})
	waitForTitle(t, w, title+"|one/two|true|7-7|0|0")
	w.onLoop(t, func() { gtkTextBufferInsertAtCursor(note.buffer, cstr("threefour"), -1) })
	waitForTitle(t, w, title+"|one/twothree|true|12-12|0|0")
	w.onLoop(t, func() {
		w.selectRange(note, core.TextRange{Start: 1, End: 3})
		if s, e := w.selection(note); s != 1 || e != 3 {
			t.Errorf("selection %d-%d, want 1-3", s, e)
		}
		w.focusEntry(line)
		gtkEditableSetText(line.entry, cstr("x"))
		gSignalEmitByName(line.entry, cstr("activate"))
	})
	waitForTitle(t, w, title+"|one/twothree|true|12-12|1|1")
}
