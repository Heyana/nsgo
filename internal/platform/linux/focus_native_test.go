//go:build linux && !android

package linux

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func programmaticFocusApp(title string) func(*core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		var button, plain, explicit, field core.Ref
		focused, pressed := core.NewSignal(cx, "ready"), core.NewSignal(cx, 0)
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%s|%d", title, focused.Get(), pressed.Get())) })
		cx.OnKeyWith("mod+f", core.KeyOptions{When: core.KeyAlways}, field.Focus)
		cx.OnKeyWith("mod+g", core.KeyOptions{When: core.KeyAlways}, func() {
			field.Focus()
			field.SetSelection(0, 0)
		})
		cx.OnKeyWith("mod+b", core.KeyOptions{When: core.KeyAlways}, button.Focus)
		cx.OnKeyWith("mod+p", core.KeyOptions{When: core.KeyAlways}, plain.Focus)
		cx.OnKeyWith("mod+v", core.KeyOptions{When: core.KeyAlways}, explicit.Focus)
		cx.OnKeyWith("mod+n", core.KeyOptions{When: core.KeyAlways}, field.Blur)
		cx.OnKeyWith("mod+m", core.KeyOptions{When: core.KeyAlways}, func() { focused.Set(focused.Get() + "-checked") })
		cx.OnKeyWith("mod+q", core.KeyOptions{When: core.KeyAlways}, cx.Quit)
		return core.View{Style: core.Style{Width: core.Pt(200), Height: core.Pt(200)}, Children: []core.Node{
			core.Input{TestID: "field", Ref: &field, DefaultValue: "hello", Style: core.Style{Height: core.Pt(30)},
				OnFocus: func() { focused.Set("field") }},
			core.View{TestID: "button", Ref: &button, Style: core.Style{Width: core.Pt(100), Height: core.Pt(40),
				Background: core.RGB(0xcc0000), FocusVisible: &core.Style{Background: core.RGB(0x123456)}},
				OnFocus: func() { focused.Set("button") }, OnBlur: func() { focused.Set("blur") },
				OnPress: func(core.PressEvent) { pressed.Update(func(n int) int { return n + 1 }) }},
			core.View{TestID: "explicit", Ref: &explicit, Style: core.Style{Height: core.Pt(30)},
				Focusable: func() bool { return true }, OnFocus: func() { focused.Set("explicit") }},
			core.View{TestID: "plain", Ref: &plain, Style: core.Style{Height: core.Pt(30)}},
			core.View{TestID: "trigger", NoFocus: true, Style: core.Style{Height: core.Pt(30)},
				OnPress: func(core.PressEvent) { button.Focus() }},
		}}
	}
}

func TestRefFocusUsesNativeKeyboardStops(t *testing.T) {
	const title = "nsgo-ref-focus"
	w := startTestWindow(t, programmaticFocusApp(title), Options{Title: title, Width: 200, Height: 200})
	waitForTitle(t, w, title+"|ready|0")
	// The platform reports a key binding by its ID, as it does a matched press.
	key := func(name string) {
		t.Helper()
		w.onLoop(t, func() {
			for _, b := range w.keys {
				if b.Key == name {
					w.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: b.ID})
					return
				}
			}
			t.Errorf("no binding of %s", name)
		})
	}
	key("mod+f")
	waitForTitle(t, w, title+"|field|0")
	w.onLoop(t, func() {
		v := w.focusedEntry()
		if v == nil {
			t.Error("programmatic focus did not focus the field")
			return
		}
		if start, end := w.selection(v); start != 5 || end != 5 {
			t.Errorf("programmatic focus selection = %d-%d, want 5-5", start, end)
		}
	})
	key("mod+b")
	waitForTitle(t, w, title+"|button|0")
	key("mod+g")
	waitForTitle(t, w, title+"|field|0")
	w.onLoop(t, func() {
		v := w.focusedEntry()
		if v == nil {
			t.Error("programmatic focus did not focus the field")
			return
		}
		if start, end := w.selection(v); start != 0 || end != 0 {
			t.Errorf("focus with SetSelection(0, 0) left %d-%d", start, end)
		}
	})
	key("mod+b")
	waitForTitle(t, w, title+"|button|0")
	var button *view
	w.onLoop(t, func() {
		button = w.focusedView()
		if button == nil || button.props.TestID != "button" || gtkWindowGetFocus(w.toplevel()) != button.widget {
			t.Error("Ref.Focus did not grab the button's GTK widget")
		}
		// A click on empty space or on a view that is no stop leaves a focused control focused, in GTK too, and Return still presses it.
		for _, at := range []driver.Point{{X: 190, Y: 190}, {X: 50, Y: 115}} {
			w.pointerDown(at, 1, 0, 0, 1)
			w.pointerUp(at)
			if w.focusedView() != button || gtkWindowGetFocus(w.toplevel()) != button.widget {
				t.Errorf("a click at %v dropped the focused button", at)
			}
		}
		if !w.controlKey(gdkReturn, 0, false) {
			t.Error("Return did not press the button after the clicks")
		}
		// The keymap has Return before the focused button: a binding that yields it to the button leaves it, one that does not takes it.
		keys := w.keys
		w.keys = []driver.KeyBinding{{Key: "enter", Yields: driver.KeyUsePress}}
		if w.shortcut(gdkReturn, 0, false) {
			t.Error("a binding that yields Return to a focused button took it")
		}
		w.keys = []driver.KeyBinding{{Key: "enter"}}
		if !w.shortcut(gdkReturn, 0, false) {
			t.Error("a binding that does not yield Return left it to the focused button")
		}
		w.keys = keys
		for _, key := range []uint32{gdkSpace, gdkReturn} {
			keyPressedCallback(0, key, key, 0, w.handle)
			// Repeated keydowns must not press again.
			keyPressedCallback(0, key, key, 0, w.handle)
			keyReleasedCallback(0, key, key, 0, w.handle)
		}
	})
	waitForTitle(t, w, title+"|button|3")
	w.onLoop(t, func() {
		if button.style.Background != driver.RGB(0x123456) {
			t.Errorf("focus-visible background = %#x", button.style.Background)
		}
	})
	key("mod+p")
	key("mod+n")
	key("mod+m")
	waitForTitle(t, w, title+"|button-checked|3")
	key("mod+v")
	waitForTitle(t, w, title+"|explicit|3")
	w.onLoop(t, func() {
		v := w.focusedView()
		if v == nil || v.props.TestID != "explicit" || gtkWindowGetFocus(w.toplevel()) != v.widget {
			t.Error("Ref.Focus did not focus the explicit stop")
		}
		if !w.tab(false) || w.focusedView() == nil || w.focusedView().props.TestID != "field" {
			t.Error("Tab did not wrap to the input")
		}
		if !w.tab(true) || w.focusedView() != v {
			t.Error("Shift+Tab did not return to the explicit stop")
		}
		// A click outside a focused field still blurs it.
		if !w.tab(false) || w.focusedView() == nil || w.focusedView().props.TestID != "field" {
			t.Error("Tab did not focus the input")
			return
		}
		w.pointerDown(driver.Point{X: 190, Y: 190}, 1, 0, 0, 1)
		w.pointerUp(driver.Point{X: 190, Y: 190})
		if w.focusedView() != nil || gtkWindowGetFocus(w.toplevel()) != 0 {
			t.Error("a click on empty space left the input focused")
		}
	})
}

func TestEmbeddedClickKeepsControlFocus(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	host := startTestWindow(t, func(*core.Scope) core.Node { return core.View{} }, Options{Width: 200, Height: 300})
	var embedded *View
	host.onLoop(t, func() {
		var err error
		embedded, err = NewView(func(*core.Scope) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(160), Height: core.Pt(100)}, Children: []core.Node{
				core.View{TestID: "button", Style: core.Style{Width: core.Pt(60), Height: core.Pt(30)}, OnPress: func(core.PressEvent) {}},
			}}
		}, Options{})
		if err != nil {
			t.Error(err)
			return
		}
		gtkWidgetSetSizeRequest(embedded.Handle(), 160, 100)
		gtkBoxAppend(host.box, embedded.Handle())
	})
	if embedded == nil {
		t.Fatal("the embedded view did not open")
	}
	surface := embedded.Handle()
	t.Cleanup(func() {
		host.onLoop(t, func() {
			embedded.Close()
			gtkWidgetUnparent(surface)
		})
	})
	var button *view
	for deadline := time.Now().Add(5 * time.Second); button == nil; time.Sleep(10 * time.Millisecond) {
		host.onLoop(t, func() {
			for _, v := range embedded.w.views {
				if v != nil && v.props.TestID == "button" && v.widget != 0 {
					button = v
				}
			}
		})
		if button == nil && time.Now().After(deadline) {
			t.Fatal("the embedded app did not mount")
		}
	}
	host.onLoop(t, func() {
		w := embedded.w
		click := func() {
			gesture := gtkGestureClickNew()
			clickPressedCallback(gesture, 1, 150, 90, w.handle)
			gObjectUnref(gesture)
		}
		w.focusView(button)
		if w.focusedView() != button || gtkWindowGetFocus(w.toplevel()) != button.widget {
			t.Error("focusView did not focus the embedded button")
			return
		}
		// The press on empty space leaves the focused control's GTK focus alone instead of moving it to the surface.
		click()
		if w.focusedView() != button || gtkWindowGetFocus(w.toplevel()) != button.widget {
			t.Error("a click on empty space blurred the embedded button")
		}
		// With nothing focused, the press still gives the surface the keys.
		w.focusEntry(nil)
		click()
		if gtkWindowGetFocus(w.toplevel()) != surface {
			t.Error("a click did not focus the embedded surface")
		}
	})
}

// modalFocus is an app with an opener before a field and a dialog the opener's press shows, with a stop that closes it, one that opens a second dialog over it, and an optional field that focuses itself as it mounts.
type modalFocus struct {
	root                                           *core.Scope
	open, nested, autoField, removed, hidden, stop *core.Signal[bool]
}

func (m *modalFocus) app(title string) func(*core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		m.root = cx
		m.open, m.nested, m.autoField = core.NewSignal(cx, false), core.NewSignal(cx, false), core.NewSignal(cx, false)
		m.removed, m.hidden, m.stop = core.NewSignal(cx, false), core.NewSignal(cx, false), core.NewSignal(cx, true)
		var opener, field core.Ref
		focused, presses := core.NewSignal(cx, "none"), core.NewSignal(cx, 0)
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%s|%d", title, focused.Get(), presses.Get())) })
		cx.OnKeyWith("mod+b", core.KeyOptions{When: core.KeyAlways}, opener.Focus)
		cx.OnKeyWith("mod+f", core.KeyOptions{When: core.KeyAlways}, field.Focus)
		// OnFocus names the view that took the focus, OnBlur the one that lost it.
		heard := func(id string) (func(), func()) {
			return func() { focused.Set(id) }, func() { focused.Set("-" + id) }
		}
		stop := func(id string, press func()) core.Node {
			onFocus, onBlur := heard(id)
			return core.View{TestID: id, Style: core.Style{Width: core.Pt(80), Height: core.Pt(30)},
				OnFocus: onFocus, OnBlur: onBlur, OnPress: func(core.PressEvent) { press() }}
		}
		openerFocus, openerBlur := heard("opener")
		fieldFocus, fieldBlur := heard("field")
		autoFocus, autoBlur := heard("auto")
		return core.View{Style: core.Style{Width: core.Pt(240), Height: core.Pt(240)}, Children: []core.Node{
			core.View{StyleFunc: func() core.Style {
				if m.hidden.Get() {
					return core.Style{Display: core.DisplayNone}
				}
				return core.Style{}
			}, Children: []core.Node{
				core.Show{When: func() bool { return !m.removed.Get() }, Then: func(*core.Scope) core.Node {
					return core.View{TestID: "opener", Ref: &opener, Style: core.Style{Width: core.Pt(120), Height: core.Pt(40)},
						Focusable: m.stop.Get, OnFocus: openerFocus, OnBlur: openerBlur,
						OnPress: func(core.PressEvent) {
							presses.Update(func(n int) int { return n + 1 })
							m.open.Set(true)
						}}
				}},
			}},
			core.Input{TestID: "field", Ref: &field, Style: core.Style{Height: core.Pt(30)}, OnFocus: fieldFocus, OnBlur: fieldBlur},
			core.Show{When: m.open.Get, Then: func(*core.Scope) core.Node {
				return core.Modal{Children: []core.Node{core.View{Style: core.Style{Width: core.Pt(200), Height: core.Pt(140)}, Children: []core.Node{
					stop("close", func() { m.open.Set(false) }),
					stop("inner", func() { m.nested.Set(true) }),
					core.Show{When: m.autoField.Get, Then: func(*core.Scope) core.Node {
						return core.Input{TestID: "auto", AutoFocus: true, Style: core.Style{Height: core.Pt(30)}, OnFocus: autoFocus, OnBlur: autoBlur}
					}},
				}}}}
			}},
			core.Show{When: m.nested.Get, Then: func(*core.Scope) core.Node {
				return core.Modal{Children: []core.Node{stop("close2", func() { m.nested.Set(false) })}}
			}},
		}}
	}
}

func (m *modalFocus) set(s *core.Signal[bool], on bool) { m.root.Post(func() { s.Set(on) }) }

// startModalFocus opens the app and waits for it to show.
func startModalFocus(t *testing.T, name string) (*window, *modalFocus, func(string)) {
	t.Helper()
	title := "nsgo-modal-focus-" + name
	m := &modalFocus{}
	w := startTestWindow(t, m.app(title), Options{Title: title, Width: 240, Height: 240})
	waitForTitle(t, w, title+"|none|0")
	return w, m, func(want string) { t.Helper(); waitForTitle(t, w, title+"|"+want) }
}

// bound reports a key binding by its ID, as the platform reports a matched press.
func (w *window) bound(t *testing.T, name string) {
	t.Helper()
	w.onLoop(t, func() {
		for _, b := range w.keys {
			if b.Key == name {
				w.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: b.ID})
				return
			}
		}
		t.Errorf("no binding of %s", name)
	})
}

// pressReturn presses and releases Return on the window's key controller.
func (w *window) pressReturn(t *testing.T) {
	t.Helper()
	w.onLoop(t, func() {
		keyPressedCallback(0, gdkReturn, gdkReturn, 0, w.handle)
		keyReleasedCallback(0, gdkReturn, gdkReturn, 0, w.handle)
	})
}

// focusedID returns the test ID of the view with the keyboard focus, checked against the GTK window's own focus.
func (w *window) focusedID(t *testing.T) string {
	t.Helper()
	var id string
	w.onLoop(t, func() {
		v := w.focusedView()
		if v == nil {
			return
		}
		id = v.props.TestID
		// A field's focus is on the text widget inside its editor.
		for f := gtkWindowGetFocus(w.toplevel()); f != v.widget; f = gtkWidgetGetParent(f) {
			if f == 0 {
				t.Errorf("%s has the focus but not its GTK widget", id)
				return
			}
		}
	})
	return id
}

// coveredIDs returns the test IDs of the views kept to get the focus back.
func (w *window) coveredIDs(t *testing.T) []string {
	t.Helper()
	var ids []string
	w.onLoop(t, func() {
		for _, v := range w.covered {
			ids = append(ids, v.props.TestID)
		}
	})
	return ids
}

// waitForModals waits until n modals show.
func (w *window) waitForModals(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var got int
		w.onLoop(t, func() {
			var walk func(*view)
			walk = func(v *view) {
				if v.kind == driver.KindModal && !v.deleted {
					got++
				}
				for _, c := range v.children {
					walk(c)
				}
			}
			if r := w.view(w.root); r != nil {
				walk(r)
			}
		})
		if got == n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d modals show, want %d", got, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestControlFocusReturnsAfterModal(t *testing.T) {
	w, m, want := startModalFocus(t, "control")
	w.bound(t, "mod+b")
	want("opener|0")
	// Return presses the focused opener, whose dialog covers it.
	w.pressReturn(t)
	want("-opener|1")
	if id, covered := w.focusedID(t), w.coveredIDs(t); id != "" || !slices.Equal(covered, []string{"opener"}) {
		t.Fatalf("under the dialog the focus is on %q and %q are kept, want none and the opener", id, covered)
	}
	// Tab reaches the dialog's own stop, and Return on it closes the dialog.
	w.onLoop(t, func() { w.tab(false) })
	want("close|1")
	w.pressReturn(t)
	want("opener|1")
	if id, covered := w.focusedID(t), w.coveredIDs(t); id != "opener" || len(covered) != 0 {
		t.Fatalf("after the dialog the focus is on %q and %q are kept, want the opener and none", id, covered)
	}
	// Return presses the opener again.
	w.pressReturn(t)
	want("-opener|2")
	m.set(m.open, false)
	want("opener|2")
}

func TestFocusReturnsThroughNestedModals(t *testing.T) {
	w, m, want := startModalFocus(t, "nested")
	w.bound(t, "mod+b")
	want("opener|0")
	w.pressReturn(t)
	want("-opener|1")
	w.onLoop(t, func() { w.tab(false); w.tab(false) })
	want("inner|1")
	w.pressReturn(t)
	want("-inner|1")
	if covered := w.coveredIDs(t); !slices.Equal(covered, []string{"opener", "inner"}) {
		t.Fatalf("under two dialogs %q are kept, want the opener and the inner stop", covered)
	}
	w.onLoop(t, func() { w.tab(false) })
	want("close2|1")
	w.pressReturn(t)
	want("inner|1")
	m.set(m.open, false)
	want("opener|1")
	if id := w.focusedID(t); id != "opener" {
		t.Fatalf("after both dialogs the focus is on %q", id)
	}
}

func TestFocusReturnsPastAModalThatFocusesItsField(t *testing.T) {
	w, m, want := startModalFocus(t, "autofocus")
	w.bound(t, "mod+b")
	want("opener|0")
	m.root.Post(func() { m.autoField.Set(true); m.open.Set(true) })
	want("auto|0")
	if covered := w.coveredIDs(t); !slices.Equal(covered, []string{"opener"}) {
		t.Fatalf("a dialog that focused its field kept %q, want the opener", covered)
	}
	m.set(m.open, false)
	want("opener|0")
}

func TestFieldFocusReturnsAfterModal(t *testing.T) {
	w, m, want := startModalFocus(t, "field")
	w.bound(t, "mod+f")
	want("field|0")
	m.set(m.open, true)
	want("-field|0")
	if id := w.focusedID(t); id != "" {
		t.Fatalf("under the dialog the focus is on %q, want none: keys would reach the covered field", id)
	}
	m.set(m.open, false)
	want("field|0")
	if id := w.focusedID(t); id != "field" {
		t.Fatalf("after the dialog the focus is on %q", id)
	}
}

func TestCoveredFocusIsForgottenWhenItGoes(t *testing.T) {
	for _, gone := range []string{"removed", "hidden", "no stop"} {
		t.Run(gone, func(t *testing.T) {
			w, m, want := startModalFocus(t, strings.ReplaceAll(gone, " ", "-"))
			w.bound(t, "mod+b")
			want("opener|0")
			m.set(m.open, true)
			want("-opener|0")
			m.root.Post(func() {
				switch gone {
				case "removed":
					m.removed.Set(true)
				case "hidden":
					m.hidden.Set(true)
				default:
					m.stop.Set(false)
				}
			})
			deadline := time.Now().Add(10 * time.Second)
			for len(w.coveredIDs(t)) != 0 {
				if time.Now().After(deadline) {
					t.Fatalf("the %s opener is still kept", gone)
				}
				time.Sleep(10 * time.Millisecond)
			}
			m.root.Post(func() { m.open.Set(false); m.hidden.Set(false); m.stop.Set(true) })
			w.waitForModals(t, 0)
			want("-opener|0")
			if id := w.focusedID(t); id != "" {
				t.Fatalf("the %s opener's dialog gave the focus to %q", gone, id)
			}
		})
	}
}
