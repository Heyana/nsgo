//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// TestFlipPointFlipsAppKitYToTopLeftDown guards the coordinate conversion:
// an AppKit point whose y grows up must land in the app's space, y down
// from the main screen's top left, as Monitor frames are. Keeping y up, or
// dropping the main screen's X offset, fails this.
func TestFlipPointFlipsAppKitYToTopLeftDown(t *testing.T) {
	// Main screen at the AppKit origin, 900 points tall. The point sits
	// 300 up from the bottom, so 600 down from the top.
	got := flipPoint(apple.CGPoint{X: 120, Y: 300}, 0, 900)
	want := driver.Point{X: 120, Y: 600}
	if got != want {
		t.Fatalf("flipPoint = %+v, want %+v (y-up 300 must flip to y-down 600)", got, want)
	}

	// Main screen shifted left in AppKit: its X offset subtracts, its top
	// sets the y-down origin. Near the top, y-down is small.
	got = flipPoint(apple.CGPoint{X: 200, Y: 880}, -100, 900)
	want = driver.Point{X: 300, Y: 20}
	if got != want {
		t.Fatalf("flipPoint = %+v, want %+v", got, want)
	}
}
