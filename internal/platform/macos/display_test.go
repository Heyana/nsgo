//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// TestMaxRefreshRateReadsEveryMode checks that each display's maximum comes from its modes at the current pixel size, not NSScreen's ceiling for the selected mode, which a 120 Hz panel set to 60 Hz reports as 60.
func TestMaxRefreshRateReadsEveryMode(t *testing.T) {
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		screens := objc.ID(apple.Class("NSScreen")).Send(sel("screens"))
		count := objc.Send[uint](screens, sel("count"))
		if count == 0 {
			t.Skip("no display")
		}
		bindDisplayMode()
		for i := uint(0); i < count; i++ {
			screen := screens.Send(sel("objectAtIndex:"), i)
			number := screenNumber(screen)
			if number == 0 {
				continue
			}
			mode := cgDisplayCopyMode(number)
			if mode == 0 {
				continue
			}
			key := displayModeKey{number, cgDisplayModeWidth(mode), cgDisplayModeHeight(mode)}
			current := cgDisplayModeRate(mode)
			apple.Release(mode)
			fastest := fastestDisplayMode(key)
			if fastest < current {
				t.Errorf("display %d: fastest mode %g Hz is below the current mode's %g Hz", number, fastest, current)
			}
			rate, maximum := displayRates(screen, number)
			if maximum < fastest || maximum < rate {
				t.Errorf("display %d reports %g Hz at most, below its fastest mode %g Hz or its rate %g Hz", number, maximum, fastest, rate)
			}
			t.Logf("display %d: %g Hz now, %g Hz at most (%dx%d)", number, rate, maximum, key.width, key.height)
		}
	})
}
