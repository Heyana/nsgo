//go:build ios && nsgotest

package ios

// TVClipboardUnavailableForTest exposes the backend's public-tvOS pasteboard gate.
func TVClipboardUnavailableForTest() bool { return generalPasteboard() == 0 }
