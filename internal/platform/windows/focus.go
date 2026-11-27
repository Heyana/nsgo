//go:build windows

package windows

import "github.com/nsgo-dev/nsgo/internal/driver"

// focusedView is the keyboard's field or ordinary view. Windows gives the native focus to the surface; the backend routes it within the tree.
func (w *window) focusedView() *view {
	if in := w.focusedField(); in != nil {
		return in.v
	}
	return w.controlFocus
}

// focusableView reports whether v is a keyboard stop: a field or a stop view that shows within the newest modal.
func (w *window) focusableView(v *view) bool { return w.focusableIn(v, w.focusScope()) }

// focusableIn is focusableView for a focus scope the caller found once, as Tab does for every view.
func (w *window) focusableIn(v, scope *view) bool {
	if v == nil || v.deleted || !w.shown(v) || !isAncestor(scope, v) {
		return false
	}
	if v.field != nil {
		return !v.props.Disabled && v.field.focusable()
	}
	return v.props.FocusStop(v.events)
}

// focusView handles Ref.Focus and accessibility focus for either kind of stop, without moving to an invalid target or through a modal.
func (w *window) focusView(v *view) {
	if !w.focusableView(v) || w.focusedView() == v {
		return
	}
	if v.field != nil {
		w.setFocus(v.field)
		return
	}
	w.axFocusView, w.axFocusPart = 0, 0
	w.setFocus(nil)
	w.restore = nil
	w.controlFocus = v
	w.report(v, driver.Event{Kind: driver.EventFocus, ID: v.id})
	w.uiaFocusChanged(v)
	w.revealView(v)
}

func (w *window) blurControl() {
	if v := w.controlFocus; v != nil {
		w.controlFocus = nil
		if !v.deleted {
			w.report(v, driver.Event{Kind: driver.EventBlur, ID: v.id})
		}
	}
	w.restoreControl = nil
}

// checkControlFocus follows stops hidden, disabled or covered by a modal. A modal gives back the stop it covered when it closes.
func (w *window) checkControlFocus() {
	v := w.controlFocus
	switch {
	case v != nil && (v.deleted || !w.shown(v) || !v.props.FocusStop(v.events)):
		w.blurControl()
	case v != nil && !isAncestor(w.focusScope(), v):
		w.blurControl()
		w.restoreControl = v
	case w.focusedView() == nil && w.restoreControl != nil:
		r := w.restoreControl
		if r.deleted || !w.shown(r) || !r.props.FocusStop(r.events) {
			w.restoreControl = nil
		} else if w.focusableView(r) {
			w.focusView(r)
		}
	}
}

// focusKeyUse returns the keys the view with the keyboard focus uses itself, which the keymap's bindings that yield them leave it (driver.FindKey).
func (w *window) focusKeyUse() driver.KeyUse {
	if w.editing() {
		return driver.KeyUseText
	}
	if v := w.controlFocus; v != nil && !v.deleted {
		return driver.FocusKeyUse(v.kind, v.props.Role, v.events.Pressable())
	}
	return 0
}

// controlKey presses the focused pressable stop on Return or Space, once per physical press, after the keymap has had the key. A focusable view without press handlers leaves the key alone.
func (w *window) controlKey(vk, lParam uintptr) bool {
	v := w.controlFocus
	if !w.focusableView(v) || !v.events.Pressable() ||
		(vk != vkReturn && vk != vkSpace) {
		return false
	}
	if !repeated(lParam) {
		mods := modifiers()
		for _, kind := range []driver.EventKind{driver.EventPressDown, driver.EventPressUp, driver.EventPress} {
			w.report(v, driver.Event{Kind: kind, ID: v.id, Index: -1, Count: 1, Modifiers: mods})
		}
	}
	return true
}
