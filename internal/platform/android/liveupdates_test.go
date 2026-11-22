package android

import "testing"

// The shipped engine writes real frames to two child processes and rejects ended owners, so a press cannot cross an embedded engine's lifetime.
func TestLiveUpdateEngineOwnership(t *testing.T) {
	runJavaEngineCheck(t, "LiveActionCheck")
}
