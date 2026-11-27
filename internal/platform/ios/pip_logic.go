package ios

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type pipWakeMode uint8

const (
	pipWakeNone pipWakeMode = iota
	pipWakeDisplayLink
	pipWakeBackgroundTimer
)

const pipBackgroundFrameInterval = time.Second / 60

// choosePiPWake picks the custom PiP frame clock for a session: UIKit's
// CADisplayLink while active or inactive in the foreground, or a single
// background timer while the session continues in the background.
func choosePiPWake(closed bool, lifecycle driver.Lifecycle, hasLayer, continuesInBackground bool) pipWakeMode {
	if closed || !hasLayer {
		return pipWakeNone
	}
	if lifecycle != driver.LifecycleBackground {
		return pipWakeDisplayLink
	}
	if continuesInBackground {
		return pipWakeBackgroundTimer
	}
	return pipWakeNone
}

// nextPiPBackgroundDelay paces the background PiP frame timer to at most 60 Hz
// without starving the main run loop when rendering takes longer than a frame.
func nextPiPBackgroundDelay(elapsed time.Duration) time.Duration {
	return max(pipBackgroundFrameInterval-elapsed, time.Millisecond)
}
