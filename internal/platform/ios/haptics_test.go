//go:build ios && !tvos && nsgotest

package ios

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestCoreHapticPatternsReplaceAndStop(t *testing.T) {
	var available bool
	onMainSync(func() { available = hapticPatternsAvailable() })
	if !available {
		t.Skip("Core Haptics needs supported physical hardware")
	}
	// The test runner can start before UIKit finishes entering the foreground.
	for deadline := time.Now().Add(5 * time.Second); ; {
		var background bool
		onMainSync(func() { background = appLifecycle() == driver.LifecycleBackground })
		if !background {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the app did not enter the foreground")
		}
		time.Sleep(25 * time.Millisecond)
	}
	first := &surface{env: driver.Environment{HapticPatterns: true}}
	second := &surface{env: driver.Environment{HapticPatterns: true}}
	t.Cleanup(func() {
		onMainSync(func() {
			first.stopHapticPattern()
			second.stopHapticPattern()
		})
	})
	pattern := driver.HapticPattern{
		{Duration: 60 * time.Millisecond, Intensity: .25, Sharpness: .4},
		{At: 120 * time.Millisecond, Duration: 60 * time.Millisecond, Intensity: .5, Sharpness: .6},
		{At: 240 * time.Millisecond, Duration: 60 * time.Millisecond, Intensity: 1, Sharpness: .8},
	}
	onMainSync(func() {
		first.playHapticPattern(&pattern)
		if hapticPlayback.owner != first || hapticPlayback.player == 0 {
			t.Error("Core Haptics did not start the three-pulse pattern")
			return
		}
		// Include the distinct transient initializer in the replacement.
		replacement := driver.HapticPattern{{At: time.Millisecond, Intensity: .5, Sharpness: .5}}
		second.playHapticPattern(&replacement)
		if hapticPlayback.owner != second || hapticPlayback.player == 0 {
			t.Error("Core Haptics did not replace playback with the transient")
			return
		}
		first.stopHapticPattern()
		if hapticPlayback.owner != second || hapticPlayback.player == 0 {
			t.Error("the previous owner stopped its replacement")
		}
		second.playHapticPattern(nil)
		if hapticPlayback.owner != nil || hapticPlayback.player != 0 {
			t.Error("Core Haptics kept the canceled player")
		}
	})
}
