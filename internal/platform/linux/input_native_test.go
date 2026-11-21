//go:build linux && !android

package linux

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// editingApp is a multiline field capped at 12 characters and a single
// line. The title shows the note (line breaks as /), its content height
// being above one line's, its selection, and the line's submits and blurs.
func editingApp(title string) engine.App {
	return func(cx *core.Scope) core.Node {
		note, line := core.NewSignal(cx, ""), core.NewSignal(cx, "")
		height := core.NewSignal(cx, driver.Points(0))
		sel := core.NewSignal(cx, driver.TextRange{})
		submitted, blurred := core.NewSignal(cx, 0), core.NewSignal(cx, 0)
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|%s|%v|%d-%d|%d|%d", title, strings.ReplaceAll(note.Get(), "\n", "/"),
				height.Get() > 30, sel.Get().Start, sel.Get().End, submitted.Get(), blurred.Get()))
		})
		return core.View{
			Style: core.Style{Width: driver.Pt(320), Height: driver.Pt(240), Padding: driver.All(10), Gap: 10,
				Background: driver.RGB(0xffffff)},
			Children: []core.Node{
				core.Input{Value: note.Get, OnChange: note.Set, Multiline: true, MaxLength: 12, TestID: "note",
					OnSelectionChange:   sel.Set,
					OnContentSizeChange: func(s driver.Size) { height.Set(s.Height) },
					Style:               core.Style{Width: driver.Pt(300), MinHeight: driver.Pt(30), FontSize: 16}},
				core.Input{Value: line.Get, OnChange: line.Set, TestID: "line",
					OnSubmit: func(string) { submitted.Update(func(n int) int { return n + 1 }) },
					OnBlur:   func() { blurred.Update(func(n int) int { return n + 1 }) },
					Style:    core.Style{Width: driver.Pt(300), Height: driver.Pt(30), FontSize: 16}},
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
			} else if v.kind == driver.KindInput {
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
		w.selectRange(note, driver.TextRange{Start: 1, End: 3})
		if s, e := w.selection(note); s != 1 || e != 3 {
			t.Errorf("selection %d-%d, want 1-3", s, e)
		}
		w.focusEntry(line)
		gtkEditableSetText(line.entry, cstr("x"))
		gSignalEmitByName(line.entry, cstr("activate"))
	})
	waitForTitle(t, w, title+"|one/twothree|true|12-12|1|1")
}

// TestTextViewTakesTheLineHeight checks that a multiline input's lines, and its placeholder's, are as tall as its style's line height, as a text's are (#310).
func TestTextViewTakesTheLineHeight(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-line-height"
	style := core.Style{Width: driver.Pt(300), Height: driver.Pt(100), FontSize: 16, LineHeight: 26}
	app := func(cx *core.Scope) core.Node {
		cx.SetTitle(title)
		return core.View{Style: core.Style{Width: driver.Pt(320), Height: driver.Pt(240), Background: driver.RGB(0xffffff)}, Children: []core.Node{
			core.Input{Multiline: true, DefaultValue: "HHHH\nHHHH\nHHHH", Style: style},
			core.Input{Multiline: true, Placeholder: "HHHH", Style: style},
		}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 320, Height: 240})
	waitForTitle(t, w, title)
	var (
		lineYrange        func(tv uintptr, iter *textIter, y, h *int32)
		measure           func(widget uintptr, orientation, forSize int32, min, nat, minBaseline, natBaseline *int32)
		gtkLabelGetYalign func(uintptr) float32
	)
	purego.RegisterLibFunc(&lineYrange, gtkLib, "gtk_text_view_get_line_yrange")
	purego.RegisterLibFunc(&measure, gtkLib, "gtk_widget_measure")
	purego.RegisterLibFunc(&gtkLabelGetYalign, gtkLib, "gtk_label_get_yalign")
	// The title is set as the app is built, which can be before the tree with its editors is mounted.
	deadline := time.Now().Add(10 * time.Second)
	var views []*view
	for len(views) != 2 {
		views = views[:0]
		w.onLoop(t, func() {
			for _, v := range w.entries {
				if v.textView != 0 && !slices.Contains(views, v) {
					views = append(views, v)
				}
			}
		})
		if len(views) != 2 {
			if time.Now().After(deadline) {
				t.Fatalf("%d text views, want 2", len(views))
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	// GTK lays a text view's lines out at its width once it has one, wrapping them until then.
	for laidOut := false; !laidOut; {
		w.onLoop(t, func() {
			laidOut = gtkWidgetGetWidth(views[0].textView) >= 290 && gtkWidgetGetWidth(views[1].textView) >= 290
		})
		if !laidOut && time.Now().After(deadline) {
			t.Fatal("the text views were not laid out at their width")
		}
		time.Sleep(50 * time.Millisecond)
	}
	w.onLoop(t, func() {
		for _, v := range views {
			if v.input.Placeholder != "" {
				var lo, nat, b1, b2 int32
				measure(v.hint, 1, 300, &lo, &nat, &b1, &b2)
				if nat != 26 {
					t.Errorf("the placeholder is %d px tall, want one line of 26", nat)
				}
				if a := gtkLabelGetYalign(v.hint); a != 0 {
					t.Errorf("the placeholder is aligned %v down its box, want at its top as the text view's first line", a)
				}
				continue
			}
			for k, at := range []int32{0, 5, 10} {
				var iter textIter
				var y, h int32
				gtkTextBufferGetIterAtOffset(v.buffer, &iter, at)
				lineYrange(v.textView, &iter, &y, &h)
				if y != int32(k)*26 || h != 26 {
					t.Errorf("line %d at %d, %d px tall; want at %d, 26 tall", k, y, h, k*26)
				}
			}
		}
	})
}

// TestKeysCaretAndPasteInAField focuses a field that reports its caret
// and pastes: a plain key binding leaves the field its typing, one that
// asks fires there and once while held, the caret is reported, and a
// paste is reported with the clipboard's text, kept out of the field.
func TestKeysCaretAndPasteInAField(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-keys"
	app := func(cx *core.Scope) core.Node {
		x, down := core.NewSignal(cx, 0), core.NewSignal(cx, 0)
		caret := core.NewSignal(cx, false)
		pasted := core.NewSignal(cx, "")
		cx.OnKey("x", func() { x.Update(func(n int) int { return n + 1 }) })
		cx.OnKeyWith("down", core.KeyOptions{When: core.KeyAlways}, func() { down.Update(func(n int) int { return n + 1 }) })
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|%d|%d|%v|%s", title, x.Get(), down.Get(), caret.Get(), pasted.Get()))
		})
		return core.View{Style: core.Style{Padding: driver.All(20)}, Children: []core.Node{
			core.Input{Style: core.Style{Height: driver.Pt(30)}, PasteBehavior: driver.PasteReport,
				OnCaretMove: func(e core.CaretEvent) { caret.Set(e.Bounds.Size.Height > 0) },
				OnPaste:     func(e driver.PasteEvent) { pasted.Set(e.Text) }},
		}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 320, Height: 200})
	waitForTitle(t, w, title+"|0|0|false|")
	var field *view
	w.onLoop(t, func() {
		for _, v := range w.entries {
			if v.kind == driver.KindInput {
				field = v
			}
		}
		w.focusEntry(field)
	})
	waitForTitle(t, w, title+"|0|0|true|")
	w.onLoop(t, func() {
		keyPressedCallback(0, 'x', 53, 0, w.handle) // the field types it
		keyReleasedCallback(0, 'x', 53, 0, w.handle)
		keyPressedCallback(0, gdkDown, 116, 0, w.handle)
		keyPressedCallback(0, gdkDown, 116, 0, w.handle) // held: once
	})
	waitForTitle(t, w, title+"|0|1|true|")
	w.onLoop(t, func() {
		w.setClipboard("pasted text")
		gSignalEmitByName(gtkEditableGetDelegate(field.entry), cstr("paste-clipboard"))
	})
	waitForTitle(t, w, title+"|0|1|true|pasted text")
	w.onLoop(t, func() {
		if got := w.editorText(field); got != "" {
			t.Errorf("the field reads %q, want the paste kept out", got)
		}
	})
}

// TestADoubleClickOnADragRegionIsTheTitleBars presses a window drag region one to three times in a row with the seat's pointer. The second press is the title bar's double click, which the window carries out itself; the others wait to move the window until the pointer is dragged past the threshold, so a move does not take the pointer before a double click can be told, and a release or a new press ends that wait. A transparent window has no title bar to double-click, so all its presses wait to move it, and a press of another button is not the window's.
func TestADoubleClickOnADragRegionIsTheTitleBars(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	var (
		defaultSeat func(display uintptr) uintptr
		seatPointer func(seat uintptr) uintptr
	)
	purego.RegisterLibFunc(&defaultSeat, gtkLib, "gdk_display_get_default_seat")
	purego.RegisterLibFunc(&seatPointer, gtkLib, "gdk_seat_get_pointer")
	app := func(cx *core.Scope) core.Node {
		cx.Effect(func() { cx.SetTitle("drag|ready") })
		return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100)},
			Desktop: core.DesktopViewProps{WindowDrag: driver.WindowDragOn}}
	}
	for _, transparent := range []bool{false, true} {
		t.Run(fmt.Sprintf("transparent=%t", transparent), func(t *testing.T) {
			w := startTestWindow(t, app, Options{Title: "drag|start", Width: 200, Height: 100, Transparent: transparent})
			waitForTitle(t, w, "drag|ready")
			at := driver.Point{X: 50, Y: 50}
			var pointer uintptr
			w.onLoop(t, func() {
				if seat := defaultSeat(gdkDisplayGetDefault()); seat != 0 {
					pointer = seatPointer(seat)
				}
			})
			if pointer == 0 {
				t.Skip("the display has no pointer")
			}
			w.onLoop(t, func() {
				for clicks := 1; clicks <= 3; clicks++ {
					w.pointerDown(at, 1, pointer, 0, clicks)
					double := clicks == 2 && !transparent
					if !w.pressConsumed || (w.windowMove != nil) == double {
						t.Errorf("press %d of a row: taken %v, waiting to move the window %v; want taken, waiting %v", clicks, w.pressConsumed, w.windowMove != nil, !double)
					}
					if !double {
						dragUpdateCallback(0, windowMoveSlop/2, 0, w.handle)
						if w.windowMove == nil {
							t.Errorf("press %d of a row moved the window within the drag threshold", clicks)
						}
					}
					w.pointerUp(at)
					if w.windowMove != nil {
						t.Errorf("press %d of a row still waits to move the window after its release", clicks)
					}
				}
				w.pointerDown(at, 1, pointer, 0, 1)
				w.pointerDown(at, 2, pointer, 0, 1)
				if w.pressConsumed || w.windowMove != nil {
					t.Error("a middle press on the drag region was taken to move the window, or the first press's wait outlived it")
				}
				w.pointerUp(at)
			})
		})
	}
}

// TestKeysComeUpWhereTheyWentDown holds keys on a focused view that hears them and on none, for an app that reads the keys held. Each up reaches the view its down did, a modifier released alone comes up without its own modifier unless its other key stays down, the keys held come up as the window loses the keyboard and press anew after it, and a down no view hears is still held.
func TestKeysComeUpWhereTheyWentDown(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-keys-up"
	app := func(cx *core.Scope) core.Node {
		log := core.NewSignal(cx, "")
		mark := func(prefix string, e core.KeyEvent) {
			s := prefix + e.Key
			if e.Modifiers.Shift {
				s += "+S"
			}
			if e.Repeat {
				s += "!"
			}
			log.Update(func(l string) string {
				if l != "" {
					l += ","
				}
				return l + s
			})
		}
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|%s|%s", title, log.Get(), strings.Join(core.KeysDown(cx), " ")))
		})
		return core.View{Style: core.Style{Padding: driver.All(20)}, Children: []core.Node{
			core.View{TestID: "pad", Focusable: func() bool { return true }, Style: core.Style{Width: driver.Pt(100), Height: driver.Pt(40)},
				OnKeyDown: func(e core.KeyEvent) bool { mark("d:", e); return false },
				OnKeyUp:   func(e core.KeyEvent) { mark("u:", e) }},
		}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 320, Height: 200})
	waitForTitle(t, w, title+"||")
	const shiftL, shiftR = 0xffe1, 0xffe2
	w.onLoop(t, func() {
		if !w.heldKeys {
			t.Fatal("reading the keys held did not ask for every key")
		}
		for _, v := range w.views {
			if v != nil && v.props.TestID == "pad" {
				w.setControlFocus(v)
			}
		}
		keyPressedCallback(0, shiftL, 50, 0, w.handle)
		keyPressedCallback(0, shiftR, 62, gdkShift, w.handle)
		keyPressedCallback(0, 'a', 38, gdkShift, w.handle)
		keyReleasedCallback(0, 'a', 38, gdkShift, w.handle)
		// GDK's state at a modifier's release still holds it: the left Shift comes up with the right one held, the right one alone.
		keyReleasedCallback(0, shiftL, 50, gdkShift, w.handle)
		keyReleasedCallback(0, shiftR, 62, gdkShift, w.handle)
	})
	ups := "d:shift+S,d:shift+S,d:a+S,u:a+S,u:shift+S,u:shift"
	waitForTitle(t, w, title+"|"+ups+"|")
	w.onLoop(t, func() {
		keyPressedCallback(0, 'b', 56, 0, w.handle)
		keyPressedCallback(0, 'b', 56, 0, w.handle)
	})
	ups += ",d:b,d:b!"
	waitForTitle(t, w, title+"|"+ups+"|b")
	w.onLoop(t, func() {
		// A window under test is seldom active; where it is, the flush its deactivation runs is run directly.
		if w.win != 0 && gtkWindowIsActive(w.win) != 0 {
			w.releaseKeys()
		} else {
			activityCallback(0, 0, 0)
		}
		if len(w.held) != 0 {
			t.Errorf("%d keys still held after the window lost the keyboard", len(w.held))
		}
		// Back, the key goes down anew rather than repeating, and its release elsewhere is not heard again.
		keyReleasedCallback(0, 'b', 56, 0, w.handle)
		keyPressedCallback(0, 'b', 56, 0, w.handle)
		keyReleasedCallback(0, 'b', 56, 0, w.handle)
	})
	ups += ",u:b,d:b,u:b"
	waitForTitle(t, w, title+"|"+ups+"|")
	w.onLoop(t, func() {
		w.setControlFocus(nil)
		keyPressedCallback(0, 'c', 54, 0, w.handle)
	})
	waitForTitle(t, w, title+"|"+ups+"|c")
	w.onLoop(t, func() { keyReleasedCallback(0, 'c', 54, 0, w.handle) })
	waitForTitle(t, w, title+"|"+ups+"|")
}

// TestClicksRestartOnAReplacedView presses where a pressed view has just been replaced, with GTK's count of the surface's presses going on (#1034). The new view takes the press as a first click and counts its own quick presses from there, as GTK's count of the surface does not restart when the views drawn in it change.
func TestClicksRestartOnAReplacedView(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	app := func(cx *core.Scope) core.Node {
		replaced, log := core.NewSignal(cx, false), core.NewSignal(cx, "")
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("clicks|%t|%s", replaced.Get(), log.Get())) })
		pressed := func(name string) func(core.PressEvent) {
			return func(e core.PressEvent) {
				log.Update(func(s string) string { return s + fmt.Sprintf(" %s%d", name, e.Clicks) })
				replaced.Set(true)
			}
		}
		style := core.Style{Width: driver.Pt(100), Height: driver.Pt(60)}
		page := core.Show{When: replaced.Get,
			Then: func(*core.Scope) core.Node {
				return core.View{TestID: "second", Style: style, OnPress: pressed("second")}
			},
			Else: func(*core.Scope) core.Node {
				return core.View{TestID: "first", Style: style, OnPress: pressed("first")}
			}}
		return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100)}, Children: []core.Node{page}}
	}
	w := startTestWindow(t, app, Options{Title: "clicks|start", Width: 200, Height: 100})
	waitForTitle(t, w, "clicks|false|")
	at := driver.Point{X: 50, Y: 30}
	under := func() *view {
		var v *view
		w.onLoop(t, func() {
			if root := w.view(w.root); root != nil {
				v = hit(root, at, func(v *view) bool { return v.events.Has(driver.EventPress) })
			}
		})
		return v
	}
	first := under()
	if first == nil {
		t.Fatal("no pressable view under the press")
	}
	want := ""
	for n, name := range []string{"first", "second", "second", "second"} {
		w.onLoop(t, func() {
			w.pointerDown(at, 1, 0, 0, n+1)
			w.pointerUp(at)
		})
		clicks := n + 1
		if n > 0 {
			clicks = n
		}
		want += fmt.Sprintf(" %s%d", name, clicks)
		waitForTitle(t, w, "clicks|true|"+want)
		// The engine mounts the second view after the title changes; press it only once it is there.
		for deadline := time.Now().Add(10 * time.Second); under() == first; time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatal("the pressed view was not replaced")
			}
		}
	}
}
