//go:build darwin && !ios && nsgotest

package macos

import (
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Task progress is drawn into the real Dock tile and removed again when the app clears it. This exercises AppKit's dockTile content view rather than only the mount protocol and pure progress conversion tests.
func TestTaskProgressDockTileNative(t *testing.T) {
	onMain(func() {
		loadAppKit()
		app := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
		tile := app.Send(sel("dockTile"))
		setDockProgress(driver.TaskProgress{State: driver.TaskProgressNormal, Value: .4, Call: 1})
		content := tile.Send(sel("contentView"))
		if content == 0 {
			t.Fatal("Dock tile did not receive a progress content view")
		}
		if objc.Send[objc.ID](content, sel("subviews")).Send(sel("count")) < 2 {
			t.Fatal("Dock progress tile did not retain the icon and progress bar views")
		}
		setDockProgress(driver.TaskProgress{State: driver.TaskProgressIndeterminate, Call: 2})
		if tile.Send(sel("contentView")) == 0 {
			t.Fatal("indeterminate progress removed the Dock tile")
		}
		setDockProgress(driver.TaskProgress{State: driver.TaskProgressNone, Call: 3})
		if tile.Send(sel("contentView")) != 0 {
			t.Fatal("clearing task progress did not restore the plain Dock tile")
		}
	})
}
