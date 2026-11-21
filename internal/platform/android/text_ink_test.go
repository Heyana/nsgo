package android

import "testing"

// TestTextNodeInkOverflowOnJVM runs the real TextNode.java on the JVM and
// checks that decoration lines and text shadows past a text's box draw
// unclipped, apply shadows to custom decorations, size tinted layers to the
// ink bounds, and invalidate the parent when overflow appears or clears.
func TestTextNodeInkOverflowOnJVM(t *testing.T) {
	runJavaAlgorithm(t, "textink", "TextInkCheck", nil, "TextNode", "Protocol")
}
