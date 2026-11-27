//go:build ios

package ios

import (
	"slices"
	"sync"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Focus on an Apple TV, as the desktops' keyboard focus: views that take presses or are Focusable are the focus engine's items with the text fields, unless disabled, hidden, NoFocus or under a newer modal or input-capturing Topmost overlay. The remote moves the focus, its select button presses the focused view and holding it long-presses, and its Menu button goes back where the app has somewhere to go back from, else leaves the app as tvOS's apps do. A modal or capture overlay takes the focus as it comes and gives it back as it goes.
// iPhones and iPads use explicit first-responder focus for hardware keyboards; touches continue to press controls without focusing them.

// onTV reports whether the app runs on an Apple TV, whose UIKit lacks
// what only touchscreens have and moves a focus instead.
var onTV = sync.OnceValue(func() bool {
	const idiomTV = 2 // UIUserInterfaceIdiomTV
	return objc.Send[int](objc.ID(apple.Class("UIDevice")).Send(sel("currentDevice")), sel("userInterfaceIdiom")) == idiomTV
})

// focusMethods are what every view answers the focus engine with.
func focusMethods() []objc.MethodDef {
	return []objc.MethodDef{
		{Cmd: sel("canBecomeFocused"), Fn: func(self objc.ID, cmd objc.SEL) bool {
			v := views[self]
			if v == nil || v.native != self {
				return apple.SendSuper[bool](self, cmd)
			}
			return v.focusable()
		}},
		{Cmd: sel("didUpdateFocusInContext:withAnimationCoordinator:"), Fn: func(self objc.ID, cmd objc.SEL, context, coordinator objc.ID) {
			apple.Super(self, cmd, context, coordinator)
			v := views[self]
			if v == nil || v.native != self {
				return
			}
			switch self {
			case context.Send(sel("nextFocusedView")):
				v.surface.focused = v
				v.dispatch(driver.EventFocus)
			case context.Send(sel("previouslyFocusedView")):
				if v.surface.focused == v {
					v.surface.focused = nil
				}
				v.dispatch(driver.EventBlur)
			}
		}},
	}
}

// pressMethods take the remote's select button on the focused view.
func pressMethods() []objc.MethodDef {
	return []objc.MethodDef{
		{Cmd: sel("pressesBegan:withEvent:"), Fn: func(self objc.ID, cmd objc.SEL, presses, event objc.ID) {
			v := views[self]
			if v == nil || !v.selects(presses) {
				apple.Super(self, cmd, presses, event)
				return
			}
			v.pressing.Store(true)
			v.longFired.Store(false)
			// A press from the remote is at the view's origin, with no keys, as one from a screen reader.
			v.pressAt = driver.Event{Index: -1, Count: 1}
			v.beginPress()
		}},
		{Cmd: sel("pressesEnded:withEvent:"), Fn: func(self objc.ID, cmd objc.SEL, presses, event objc.ID) {
			v := views[self]
			if v == nil || !v.selects(presses) {
				apple.Super(self, cmd, presses, event)
				return
			}
			pressed := v.pressing.Swap(false)
			v.pressed(driver.EventPressUp)
			if pressed && !v.longFired.Load() {
				v.pressed(driver.EventPress)
			}
		}},
		{Cmd: sel("pressesCancelled:withEvent:"), Fn: func(self objc.ID, cmd objc.SEL, presses, event objc.ID) {
			v := views[self]
			if v == nil || !v.selects(presses) {
				apple.Super(self, cmd, presses, event)
				return
			}
			if v.pressing.Swap(false) {
				v.pressed(driver.EventPressUp)
			}
		}},
	}
}

// selects reports whether presses hold the select button and v, focused,
// takes it.
func (v *view) selects(presses objc.ID) bool {
	if !onTV() || !v.pressable() || !v.focusable() {
		return false
	}
	remote := false
	apple.ForEach(presses, func(p objc.ID) {
		remote = remote || p.Send(sel("key")) == 0 && objc.Send[int](p, sel("type")) == pressSelect
	})
	return remote
}

// keyboardResponderMethods let an explicitly focused pressable receive hardware keys on iPhone and iPad without adopting UITextInput or showing a keyboard.
func keyboardResponderMethods() []objc.MethodDef {
	return []objc.MethodDef{
		{Cmd: sel("canBecomeFirstResponder"), Fn: func(self objc.ID, _ objc.SEL) bool {
			v := views[self]
			return v != nil && !onTV() && v.focusable()
		}},
		{Cmd: sel("becomeFirstResponder"), Fn: becomeKeyboardResponder},
		{Cmd: sel("resignFirstResponder"), Fn: resignKeyboardResponder},
	}
}

func becomeKeyboardResponder(self objc.ID, cmd objc.SEL) bool {
	ok := apple.SendSuper[bool](self, cmd)
	if v := views[self]; ok && v != nil && !onTV() && v.focusable() {
		v.surface.focused = v
		v.dispatch(driver.EventFocus)
	}
	return ok
}

func resignKeyboardResponder(self objc.ID, cmd objc.SEL) bool {
	ok := apple.SendSuper[bool](self, cmd)
	if v := views[self]; ok && v != nil && !onTV() && v.surface.focused == v {
		v.surface.focused = nil
		v.dispatch(driver.EventBlur)
	}
	return ok
}

// activateKey runs only after the keymap and never while a native editor has focus.
func (s *surface) activateKey(name string, mods driver.Modifiers) bool {
	if name != "enter" && name != "space" || s.editing() {
		return false
	}
	v := s.focused
	if v == nil || s.views[v.id] != v || !v.focusable() || !v.pressable() {
		return false
	}
	for _, kind := range []driver.EventKind{driver.EventPressDown, driver.EventPressUp, driver.EventPress} {
		v.report(driver.Event{Kind: kind, Index: -1, Count: 1, Modifiers: mods})
	}
	return true
}

// focusKeyUse returns the keys the focused view uses itself, which the keymap's bindings that yield them leave it (driver.FindKey): a field's editing keys, the focused view's, a pressable view's Return and Space or a slider's arrows.
func (s *surface) focusKeyUse() driver.KeyUse {
	if s.editing() {
		return driver.KeyUseText
	}
	if v := s.focused; v != nil && s.views[v.id] == v {
		return driver.FocusKeyUse(v.kind, v.props.Role, v.pressable())
	}
	return 0
}

// hasPress reports whether presses, an NSSet of UIPress, hold one of kind.
func hasPress(presses objc.ID, kind int) bool {
	found := false
	apple.ForEach(presses, func(p objc.ID) {
		found = found || objc.Send[int](p, sel("type")) == kind
	})
	return found
}

// beginPress reports v pressed down, and long pressed once it is held.
func (v *view) beginPress() {
	v.pressed(driver.EventPressDown)
	if v.events.Has(driver.EventLongPress) {
		e, long := v.surface.engine, v.pressAt
		long.Kind, long.ID = driver.EventLongPress, v.id
		time.AfterFunc(v.longPressDelay(), func() {
			if v.pressing.Load() && v.longFired.CompareAndSwap(false, true) {
				e.Dispatch(long)
			}
		})
	}
}

// focusable reports whether v may take the focus.
func (v *view) focusable() bool {
	if v.native == 0 || !v.props.FocusStop(v.events) ||
		v.kind == driver.KindInput || v.native.Send(sel("window")) == 0 || hiddenWithin(v.native) {
		return false
	}
	capture := v.surface.topmostCapture()
	inCapture := capture != nil && objc.Send[bool](v.native, sel("isDescendantOfView:"), capture.native)
	top := v.surface.topModal()
	inModal := top != nil && objc.Send[bool](v.native, sel("isDescendantOfView:"), top.native)
	return tvFocusAllowed(inCapture, capture != nil, inModal, top != nil, v.inTopmostOverlay())
}

// inTopmostOverlay reports whether v lies inside a Topmost overlay.
func (v *view) inTopmostOverlay() bool {
	for n := v.native; n != 0 && n != v.surface.native; n = n.Send(selSuperview) {
		if o := views[n]; o != nil && o.props.Topmost {
			return true
		}
	}
	return false
}

// focusView moves tvOS's focus engine, or the first responder on iPhone and iPad. A plain UIView opens no software keyboard.
func (s *surface) focusView(v *view) {
	if !onTV() {
		v.native.Send(selBecomeFirst)
		return
	}
	system := objc.ID(apple.Class("UIFocusSystem")).Send(sel("focusSystemForEnvironment:"), v.native)
	if system != 0 {
		system.Send(sel("requestFocusUpdateToEnvironment:"), v.native)
		system.Send(sel("updateFocusIfNeeded"))
	}
}

// hiddenWithin reports whether view or a view around it is hidden.
func hiddenWithin(view objc.ID) bool {
	for n := view; n != 0; n = n.Send(selSuperview) {
		if objc.Send[bool](n, sel("isHidden")) {
			return true
		}
	}
	return false
}

// topModal is the newest modal shown in the surface, not a sheet, which
// UIKit presents and focuses apart.
func (s *surface) topModal() *view {
	for i := len(s.modals) - 1; i >= 0; i-- {
		if v := s.modals[i]; s.views[v.id] == v && v.native.Send(sel("window")) != 0 {
			return v
		}
	}
	return nil
}

// topmostCapture is the newest visible input-capturing Topmost overlay shown in the surface, such as the layout audit's ruler.
func (s *surface) topmostCapture() *view {
	for i := len(s.topmostCaptures) - 1; i >= 0; i-- {
		if v := s.topmostCaptures[i]; s.views[v.id] == v && v.native.Send(sel("window")) != 0 &&
			!hiddenWithin(v.native) && isTopmostCaptureProps(v.props) {
			return v
		}
	}
	return nil
}

// topFocusScope is the newest active capture overlay, else the newest modal.
func (s *surface) topFocusScope() *view {
	if c := s.topmostCapture(); c != nil {
		return c
	}
	return s.topModal()
}

// updateModals follows the modals and capture overlays a batch showed and closed, moving the focus into the newest as it comes and back to what had it as it goes, and focuses the views a batch brought when nothing has the focus.
func (s *surface) updateModals() {
	// The newest modal as the last batch left them, which may be gone.
	var top *view
	if n := len(s.modals); n > 0 {
		top = s.modals[n-1]
	}
	prev := s.focusScope
	s.modals = slices.DeleteFunc(s.modals, func(v *view) bool {
		return s.views[v.id] != v || v.native.Send(sel("window")) == 0
	})
	s.topmostCaptures = slices.DeleteFunc(s.topmostCaptures, func(v *view) bool {
		return s.views[v.id] != v || !isTopmostCaptureProps(v.props)
	})
	for _, v := range s.presenting {
		if s.views[v.id] == v && v.sheet == nil && !slices.Contains(s.modals, v) {
			v.focusBefore = s.focused
			s.modals = append(s.modals, v)
		}
	}
	if !onTV() {
		// Keep the modal scope for explicit hardware-keyboard focus too.
		if top != nil && !slices.Contains(s.modals, top) && !s.editing() &&
			(s.focused == nil || s.views[s.focused.id] != s.focused) {
			if back := top.focusBefore; back != nil && s.views[back.id] == back && back.focusable() {
				s.focusView(back)
			}
		}
		return
	}
	needsRefocus := s.updateTVScrolls()
	now := s.topFocusScope()
	s.focusScope = now
	switch {
	case now != prev:
		if prev != nil && !slices.Contains(s.modals, prev) && !slices.Contains(s.topmostCaptures, prev) {
			s.focusBack = prev.focusBefore
		}
		s.refocus(true)
	case needsRefocus:
		s.refocus(true)
	case s.focused == nil || s.views[s.focused.id] != s.focused:
		// Nothing may have the focus yet, or what had it went: UIKit
		// focuses only as the window appears. A field's own text view
		// may have it, which keeps it.
		s.refocus(false)
	}
}

// refocus has the focus system move the focus where the controller
// prefers, after this batch's views are in place; unless moving, only
// where nothing has it. The system moves none while the app is inactive,
// as it is launching, and focuses the views itself as it becomes active.
func (s *surface) refocus(moving bool) {
	c := s.viewController()
	if c == 0 {
		return
	}
	apple.OnMain(func() {
		system := objc.ID(apple.Class("UIFocusSystem")).Send(sel("focusSystemForEnvironment:"), c)
		if system == 0 || !moving && system.Send(sel("focusedItem")) != 0 {
			return
		}
		system.Send(sel("requestFocusUpdateToEnvironment:"), c)
		system.Send(sel("updateFocusIfNeeded"))
		if system.Send(sel("focusedItem")) != 0 {
			s.focusBack = nil
		}
	})
}

// preferredFocus is where the focus goes when the controller's view asks: into the newest capture overlay or modal, back to what had it before one, else UIKit's choice.
func (s *surface) preferredFocus() objc.ID {
	if top := s.topFocusScope(); top != nil {
		return top.native
	}
	if b := s.focusBack; b != nil && s.views[b.id] == b && b.canTakeTVFocus() {
		return b.native
	}
	return 0
}

// controllerPreferredFocus is where the root view controller asks the
// focus to go: surface.preferredFocus, else UIKit's choice.
func controllerPreferredFocus(self objc.ID, cmd objc.SEL) objc.ID {
	if s := controllerSurface(self); s != nil {
		if v := s.preferredFocus(); v != 0 {
			return objc.ID(apple.Class("NSArray")).Send(sel("arrayWithObject:"), v)
		}
	}
	return apple.SendSuper[objc.ID](self, cmd)
}

// menuGoesBack reports whether the remote's Menu button in presses goes
// back in s, rather than leaving the app: a screen is above another or a
// modal is open (ItemHistory), or the focus is in a modal or capture overlay.
func (s *surface) menuGoesBack(presses objc.ID) bool {
	return onTV() && hasPress(presses, pressMenu) && tvMenuGoesBack(s.history, s.topModal() != nil, s.topmostCapture() != nil)
}
