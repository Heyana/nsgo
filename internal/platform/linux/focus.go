//go:build linux && !android

package linux

import (
	"slices"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Focus stays in the newest modal. Every active Tab stop keeps GTK focusable
// enabled so AT-SPI reports it as focusable to screen readers such as Orca;
// NsgoView's focus vfunc declines GTK's directional traversal so mapping a
// window does not focus a button or field on the app's behalf.
func (v *view) focusKind() driver.ViewKind { return v.kind }
func (v *view) focusHidden() bool          { return v.style.Hidden }
func (v *view) focusDeleted() bool         { return v.deleted }
func (v *view) focusChildren() []*view     { return v.children }
func (v *view) focusStopSelf() bool {
	return viewCanFocusStop(v.kind, &v.props, v.events, v.input.Disabled)
}

func (w *window) focusScope() *view {
	if w.focusScopeValid {
		return w.focusScopeCache
	}
	w.focusScopeCache = activeFocusScope(w.view(w.root))
	w.focusScopeValid = true
	return w.focusScopeCache
}

func (w *window) invalidateFocusScope() {
	w.focusScopeValid = false
	w.focusScopeCache = nil
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
			return inside && v.focusStopSelf()
		}
	}
	return false
}

// inputFocusWidget returns the native GTK widget that takes keyboard focus for
// a KindInput view: its GtkTextView when multiline, or its GtkEntry delegate.
func (w *window) inputFocusWidget(v *view) uintptr {
	if v == nil || v.entry == 0 {
		return 0
	}
	if v.textView != 0 {
		return v.textView
	}
	return gtkEditableGetDelegate(v.entry)
}

func (w *window) setViewFocusable(v *view, on bool) {
	if v == nil || v.widget == 0 || v.focusable == on {
		return
	}
	v.focusable = on
	gtkWidgetSetFocusable(v.widget, boolInt(on))
}

func (w *window) setEditorFocusable(v *view, on bool) {
	if v == nil || v.editorFocusable == on {
		return
	}
	editor := w.inputFocusWidget(v)
	if editor == 0 {
		v.editorFocusable = false
		return
	}
	v.editorFocusable = on
	gtkWidgetSetFocusable(editor, boolInt(on))
}

func (w *window) clearFocusableSubtree(v *view) {
	if v == nil || v.deleted {
		return
	}
	w.setViewFocusable(v, false)
	w.setEditorFocusable(v, false)
	for _, c := range v.children {
		w.clearFocusableSubtree(c)
	}
}

func (w *window) inRootTree(v *view) bool {
	root := w.view(w.root)
	if root == nil {
		return false
	}
	for p := v; p != nil; p = p.parent {
		if p == root {
			return true
		}
	}
	return false
}

// syncFocusable updates GTK's focusable flag across the mounted tree (and
// clears any views detached from it) so AT-SPI matches the active Tab stops.
func (w *window) syncFocusable(detached []*view) {
	for _, v := range detached {
		if v != nil && !v.deleted && !w.inRootTree(v) {
			w.clearFocusableSubtree(v)
		}
	}
	root := w.view(w.root)
	scope := activeFocusScope(root)
	w.focusScopeCache = scope
	w.focusScopeValid = true
	if root == nil {
		return
	}
	syncFocusSubtree(root, scope, false, false, func(v *view, viewFocus, editorFocus bool) {
		w.setViewFocusable(v, viewFocus)
		w.setEditorFocusable(v, editorFocus)
	})
}

// syncFocusableIncremental updates GTK's focusable flag only for views (and
// subtrees) touched by a batch that did not change the active modal focus
// scope, avoiding a full-tree walk on every tick or row-binding scroll batch.
func (w *window) syncFocusableIncremental(detached, singles, subtrees []*view) {
	for _, v := range detached {
		if v != nil && !v.deleted && !w.inRootTree(v) {
			w.clearFocusableSubtree(v)
		}
	}
	root := w.view(w.root)
	if root == nil {
		return
	}
	scope := w.focusScope()
	ancestorState := func(v *view) (inRoot, insideScope, parentHidden bool) {
		for p := v.parent; p != nil; p = p.parent {
			if p.deleted {
				return false, false, false
			}
			if p.style.Hidden {
				parentHidden = true
			}
			if p == scope {
				insideScope = true
			}
			if p == root {
				inRoot = true
				break
			}
		}
		if v == root {
			inRoot = true
		}
		return inRoot, insideScope, parentHidden
	}
	set := func(v *view, viewFocus, editorFocus bool) {
		w.setViewFocusable(v, viewFocus)
		w.setEditorFocusable(v, editorFocus)
	}
	for _, v := range singles {
		if v == nil || v.deleted {
			continue
		}
		inRoot, inside, hidden := ancestorState(v)
		if !inRoot {
			set(v, false, false)
			continue
		}
		stop := !hidden && !v.style.Hidden && (inside || v == scope) && v.focusStopSelf()
		viewFocus, editorFocus := widgetFocusableTargets(v.kind, stop)
		set(v, viewFocus, editorFocus)
	}
	for _, v := range subtrees {
		if v == nil || v.deleted {
			continue
		}
		inRoot, inside, hidden := ancestorState(v)
		if !inRoot {
			w.clearFocusableSubtree(v)
			continue
		}
		syncFocusSubtree(v, scope, inside, hidden, set)
	}
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
	w.setEditorFocusable(v, true)
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
	w.setViewFocusable(v, true)
	if gtkWidgetGrabFocus(v.widget) != 0 {
		w.setControlFocus(v)
	}
}

func (w *window) setControlFocus(v *view) {
	if v != nil {
		w.focusedPartCanvas, w.focusedPartID = nil, ""
	}
	old := w.controlFocus
	if old == v {
		return
	}
	w.controlFocus = v
	if old != nil && !old.deleted {
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
	if len(w.covered) == 0 {
		return
	}
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
