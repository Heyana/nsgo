//go:build darwin

package apple

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// CollectText collects visible text in native subtree order. text reports
// whether a managed view is text, including text with an empty label.
func CollectText(native objc.ID, out *[]string, text func(objc.ID) (string, bool)) {
	for _, child := range Subviews(native) {
		if objc.Send[bool](child, sel("isHidden")) {
			continue
		}
		if label, isText := text(child); isText {
			if label != "" {
				*out = append(*out, label)
			}
			continue
		}
		CollectText(child, out, text)
	}
}

// CustomAccessibilityAction resolves VoiceOver's label to the action's name.
// An action without a label uses its name as the spoken label.
func CustomAccessibilityAction(action objc.ID, actions []driver.AccessibilityAction, perform func(string) bool) bool {
	label := GoString(action.Send(sel("name")))
	for _, action := range actions {
		if action.Label == label || (action.Label == "" && action.Name == label) {
			return perform(action.Name)
		}
	}
	return false
}
