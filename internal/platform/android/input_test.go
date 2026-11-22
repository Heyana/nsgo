package android

import "testing"

// TestInputFieldCompositionEndsBeforeBlur compiles and runs the real Android
// InputField.java on the host JVM to verify that losing focus (or becoming
// disabled or read-only) while composing ends the IME composition before
// View's focus listener reports EVENT_BLUR (#1563).
func TestInputFieldCompositionEndsBeforeBlur(t *testing.T) {
	runJavaAlgorithm(t, "inputfield", "InputFieldCheck", nil, "Protocol", "InputField")
}
