package android

import "testing"

// Keep the entry points visible to the tool's host-side test discovery. Their bodies require the Android test host and skip on other targets.
func TestVerticalListTouchStopsMomentum(t *testing.T) { listTouchStopsMomentum(t, false) }

func TestHorizontalListTouchStopsMomentum(t *testing.T) { listTouchStopsMomentum(t, true) }

func TestZoomScrollTouchStopsMomentum(t *testing.T) { zoomScrollTouchStopsMomentum(t) }
