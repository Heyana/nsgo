//go:build !android || !nsgotest

package android

import "testing"

func listTouchStopsMomentum(t *testing.T, _ bool) { t.Skip("requires the Android test host") }

func zoomScrollTouchStopsMomentum(t *testing.T) { t.Skip("requires the Android test host") }
