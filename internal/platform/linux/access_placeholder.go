package linux

// This file builds on every system, so tests check the multiline placeholder
// accessibility policy and transition logic on any host.

const (
	// placeholderLabelRole is the GTK accessible role nick for a multiline
	// input's visual placeholder label, keeping it out of AT-SPI.
	placeholderLabelRole = "presentation"

	// editorPlaceholderProperty is the GTK accessible property nick set on a
	// multiline GtkTextView (mapped to AT-SPI's placeholder-text attribute).
	editorPlaceholderProperty = "placeholder"
)

// editorPlaceholderAction is what GTK accessible call a placeholder update needs.
type editorPlaceholderAction uint8

const (
	editorPlaceholderNoop editorPlaceholderAction = iota
	editorPlaceholderSet
	editorPlaceholderReset
)

// editorPlaceholderState tracks the placeholder last written to a multiline
// GtkTextView's GtkAccessible so updates and clears emit only real changes.
type editorPlaceholderState struct {
	value string
	set   bool
}

// update computes the GTK accessible operation needed for placeholder and
// records the new state.
func (s *editorPlaceholderState) update(placeholder string) editorPlaceholderAction {
	if placeholder == s.value && (s.set || placeholder == "") {
		return editorPlaceholderNoop
	}
	s.value, s.set = placeholder, true
	if placeholder == "" {
		return editorPlaceholderReset
	}
	return editorPlaceholderSet
}

// reset forgets the last written placeholder when the native editor is dropped
// (for example, when switching between single-line and multiline).
func (s *editorPlaceholderState) reset() {
	*s = editorPlaceholderState{}
}
