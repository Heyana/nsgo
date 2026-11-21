//go:build linux && !android

package linux

import (
	"slices"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Focus stays in the newest modal. Ordinary widgets become focusable only when Tab or an explicit command chooses them, so GTK's automatic focus on mapping a window does not focus a button on the app's behalf.
func (w *window) focusScope() *view {
	root := w.view(w.root)
	scope := root
	var walk func(*view)
	walk = func(v *view) {
		if v.style.Hidden {
			return
		}
		if v.kind == driver.KindModal {
			scope = v
		}
		for _, c := range v.children {
			walk(c)
		}
	}
	if root != nil {
		walk(root)
	}
	return scope
}

// focusableView reports whether v is a keyboard stop: an enabled field or a stop view that shows within the newest modal.
func (w *window) focusableView(v *view) bool { return w.focusableIn(v, w.focusScope()) }

// focusableIn is focusableView for a focus scope the caller found once, as Tab does for every view.
func (w *window) focusableIn(v, scope *view) bool {
	if v == nil || v.deleted {
		return false
	}
	inside := false
	for p := v; p != nil; p = p.parent {
		if p.style.Hidden {
			return false
		}
		inside = inside || p == scope
		if p == w.view(w.root) {
			if v.kind == driver.KindInput {
				return inside && !v.props.Disabled && !v.input.Disabled
			}
			return inside && v.props.FocusStop(v.events)
		}
	}
	return false
}

func (w *window) focusedView() *view {
	if w.controlFocus != nil {
		return w.controlFocus
	}
	return w.focusedEntry()
}

// focusCommand applies ItemFocus. A single-line field is focused without GTK's select-on-focus: it keeps a selection set in the same batch, or else gets the caret at the end of its text.
func (w *window) focusCommand(v *view, selectionSet bool) {
	if v.entry == 0 || v.textView != 0 {
		w.focusView(v)
		return
	}
	if w.focus == v || !w.focusableView(v) {
		return
	}
	w.setControlFocus(nil)
	if gtkEntryGrabFocusWithoutSelecting(v.entry) != 0 && !selectionSet {
		end := int32(utf8.RuneCountInString(w.editorText(v)))
		gtkEditableSelectRegion(v.entry, end, end)
	}
}

// focusView grabs the actual GTK widget; focus reports then drive the engine's focus and focus-visible styles as they do for text fields.
func (w *window) focusView(v *view) {
	if !w.focusableView(v) {
		return
	}
	if v.entry != 0 {
		w.focusEntry(v)
		return
	}
	gtkWidgetSetFocusable(v.widget, 1)
	if gtkWidgetGrabFocus(v.widget) != 0 {
		w.setControlFocus(v)
	}
}

func (w *window) setControlFocus(v *view) {
	old := w.controlFocus
	if old == v {
		return
	}
	w.controlFocus = v
	if old != nil && !old.deleted {
		gtkWidgetSetFocusable(old.widget, 0)
		if old.events.Has(driver.EventBlur) {
			w.engine.Dispatch(driver.Event{Kind: driver.EventBlur, ID: old.id})
		}
	}
	if v != nil && v.events.Has(driver.EventFocus) {
		w.engine.Dispatch(driver.Event{Kind: driver.EventFocus, ID: v.id})
	}
}

func controlFocusCallback(widget, pspec, data uintptr) {
	r, ok := lookupWidget(widget)
	if !ok || r.v == nil {
		return
	}
	w, v := r.w, r.v
	if top := w.toplevel(); top != 0 && gtkWindowGetFocus(top) == widget {
		w.setControlFocus(v)
	} else if w.controlFocus == v {
		w.setControlFocus(nil)
	}
}

// keyboardStop reports whether v is a keyboard stop whatever modal shows: in the window, not hidden, an enabled field or a stop view.
func (w *window) keyboardStop(v *view) bool { return w.focusableIn(v, w.view(w.root)) }

// coverFocus takes the focus off a field or stop that is no longer a keyboard stop. One a modal covers is kept in covered to get it back as the modal goes. A batch runs it before its focus requests, so a modal that focuses its own stop as it comes still gives the focus back to the stop it covered.
func (w *window) coverFocus() {
	v := w.focusedView()
	if v == nil || w.focusableView(v) {
		return
	}
	w.focusEntry(nil)
	if w.keyboardStop(v) {
		w.covered = append(w.covered, v)
	}
}

// checkFocus runs after each batch's focus requests. Covered views that went, hid or stopped being stops are forgotten; one a closed modal uncovered gets the focus back if nothing has it by then, else it is forgotten too. The newest is checked first, so modals closing one after another each give back what they covered.
func (w *window) checkFocus() {
	w.coverFocus()
	w.covered = slices.DeleteFunc(w.covered, func(v *view) bool { return !w.keyboardStop(v) })
	scope := w.focusScope()
	for n := len(w.covered); n > 0 && w.focusableIn(w.covered[n-1], scope); n-- {
		v := w.covered[n-1]
		w.covered = w.covered[:n-1]
		if w.focusedView() == nil {
			w.focusView(v)
		}
	}
}

func (w *window) tab(back bool) bool {
	scope := w.focusScope()
	var stops []*view
	var walk func(*view)
	walk = func(v *view) {
		if v.style.Hidden {
			return
		}
		if w.focusableIn(v, scope) {
			stops = append(stops, v)
		}
		for _, c := range v.children {
			walk(c)
		}
	}
	if scope != nil {
		walk(scope)
	}
	if len(stops) == 0 {
		return false
	}
	next := 0
	if back {
		next = len(stops) - 1
	}
	if i := slices.Index(stops, w.focusedView()); i >= 0 {
		if back {
			next = (i + len(stops) - 1) % len(stops)
		} else {
			next = (i + 1) % len(stops)
		}
	}
	w.focusView(stops[next])
	return true
}

// controlKey presses the focused pressable stop on Return or Space, once per physical press, after the keymap has had the key. A focusable view without press handlers leaves the key alone.
func (w *window) controlKey(keyval, state uint32, repeat bool) bool {
	v := w.controlFocus
	if !w.focusableView(v) || !v.events.Pressable() ||
		(keyval != gdkReturn && keyval != gdkKPEnter && keyval != gdkSpace) {
		return false
	}
	if !repeat {
		for _, kind := range []driver.EventKind{driver.EventPressDown, driver.EventPressUp, driver.EventPress} {
			if v.events.Has(kind) {
				w.engine.Dispatch(driver.Event{Kind: kind, ID: v.id, Index: -1, Count: 1, Modifiers: modifiers(state)})
			}
		}
	}
	return true
}
