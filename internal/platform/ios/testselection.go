//go:build ios && (nsgotest || nsgoautomation)

package ios

import (
	"errors"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// SelectRange moves the focused UITextInput's actual selection, including on a
// physical device where editing arrows cannot be injected as simulator HID.
func (t *testSurface) SelectRange(selection driver.TextRange) (driver.TextRange, error) {
	w, err := t.window()
	if err != nil {
		return driver.TextRange{}, err
	}
	var actual driver.TextRange
	onMainSync(func() {
		in := inputOf(t.focusedEditor(w))
		if in == nil || !in.focused {
			err = errors.New("no text field has the keyboard focus")
			return
		}
		in.programmatic(false, func() { in.selectRange(selection) })
		var ok bool
		actual, ok = in.selection()
		if !ok {
			err = errors.New("the focused field has no native selection")
			return
		}
		// UIKit can omit its editor delegate for a programmatic selection.
		// Report through the same path after reading the actual UITextRange;
		// field.State deduplicates a delegate callback that already ran.
		in.selectionMoved()
	})
	return actual, err
}
