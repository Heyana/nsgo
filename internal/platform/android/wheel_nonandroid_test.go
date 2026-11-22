//go:build !android || !nsgotest

package android

import "testing"

func wheelAndTouchpadMomentumNative(t *testing.T) {
	t.Skip("run with nsgo test -target android")
}

func pinchPositionNative(t *testing.T) {
	t.Skip("run with nsgo test -target android")
}
