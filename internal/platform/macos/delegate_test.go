//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/ebitengine/purego/objc"
)

// TestWindowDelegateRegisters checks that loading AppKit registers NSGo's window delegate, which every app does at launch, and that it answers each full screen transition's failure: two branches once both defined windowDidFailToEnterFullScreen:, and every macOS app panicked as it started.
func TestWindowDelegateRegisters(t *testing.T) {
	onMain(func() {
		loadAppKit()
		if classWindowDelegate == 0 {
			t.Fatal("loading AppKit registered no window delegate")
		}
		for _, name := range []string{"windowShouldClose:", "windowDidFailToEnterFullScreen:", "windowDidFailToExitFullScreen:", "windowShouldZoom:toFrame:", "windowWillClose:"} {
			if !objc.Send[bool](objc.ID(classWindowDelegate), sel("instancesRespondToSelector:"), sel(name)) {
				t.Errorf("the window delegate does not answer %s", name)
			}
		}
	})
}
