//go:build ios && nsgotest

package watchos

import "github.com/nsgo-dev/nsgo/internal/driver"

// AnimatedForTest returns the animated values the view with testID last took, read on the main thread, and whether it has any.
func AnimatedForTest(testID string) (driver.AnimatedValues, bool) {
	var (
		v  driver.AnimatedValues
		ok bool
	)
	onMainSync(func() {
		for _, vw := range s.views {
			if vw != nil && !vw.deleted && vw.props.TestID == testID && vw.animated != nil {
				v, ok = *vw.animated, true
				return
			}
		}
	})
	return v, ok
}

// RotateCrownForTest drives a Digital Crown rotation sequence through hostCrown
// inside a scripted test scope: the first delta begins the gesture, subsequent
// deltas update it, and a trailing idle report ends it.
func RotateCrownForTest(deltas ...float64) {
	if len(deltas) == 0 {
		return
	}
	scriptedOnMain(func() {
		for i, d := range deltas {
			phase := crownChanged
			if i == 0 {
				phase = crownBegan
			}
			hostCrown(phase, d, d*60)
		}
		hostCrown(crownEnded, 0, 0)
	})
}

// WatchHIDAvailableForTest reports whether the watchOS simulator native HID
// touch path is active on the current test host.
func WatchHIDAvailableForTest() bool {
	return watchHIDReady()
}
