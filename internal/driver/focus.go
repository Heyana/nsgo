package driver

// Pressable reports whether the mask takes presses: a press, a long press, or a press's down or up.
func (m EventMask) Pressable() bool {
	return m.Has(EventPress) || m.Has(EventLongPress) || m.Has(EventPressDown) || m.Has(EventPressUp)
}

// FocusStop reports whether a non-input view takes keyboard focus. Platforms also require it to be mounted, visible and in the active modal.
func (p *ViewProps) FocusStop(events EventMask) bool {
	return !p.Disabled && !p.NoFocus && (p.Focusable || events.Pressable())
}

// FocusesViews reports whether the platform supports keyboard focus on ordinary views beside text inputs. On mobile platforms this includes explicit Ref.Focus requests without changing how touches focus.
func (e *Environment) FocusesViews() bool {
	return e.OS == OSMacOS || e.OS == OSWindows || e.OS == OSLinux || e.OS == OSWeb || e.OS == OSIOS || e.OS == OSAndroid
}
