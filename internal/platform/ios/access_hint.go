package ios

import (
	"unicode"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// This file builds on every system, so tests check an editor's spoken hint on any host.

// editorHint returns the accessibilityHint VoiceOver reads for an input's editor.
// A single-line UITextField exposes its placeholder natively; a multiline UITextView
// draws a hidden-from-accessibility label and exposes its current placeholder here
// while empty, ahead of any explicit description or hint (HelpText), matching
// HelpText's sentence joining and omitting duplicates.
func editorHint(p *driver.ViewProps, multiline bool, placeholder, text string) string {
	help := ""
	if p != nil {
		help = p.HelpText()
	}
	if !multiline || placeholder == "" || text != "" {
		return help
	}
	if help == "" || help == placeholder {
		return placeholder
	}
	if r, _ := utf8.DecodeLastRuneInString(placeholder); unicode.IsLetter(r) || unicode.IsDigit(r) {
		placeholder += "."
	}
	return placeholder + " " + help
}
