//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestReadFrontmostNamesTheWorkspacesApp(t *testing.T) {
	if !loadWindowList() {
		t.Skip("no window server")
	}
	onMain(func() {
		loadAppKit()
		withPool(func() {
			front := objc.ID(apple.Class("NSWorkspace")).Send(sel("sharedWorkspace")).Send(sel("frontmostApplication"))
			if front == 0 {
				t.Skip("no application in front, as outside a login session")
			}
			app := readFrontmost()
			if want := int(int32(apple.Msg(front, sel("processIdentifier")))); app.PID != want || app.PID <= 0 {
				t.Errorf("PID = %d, want %d", app.PID, want)
			}
			if app.ID == "" || app.Name == "" {
				t.Errorf("frontmost app %+v lacks its ID or name", app)
			}
		})
	})
}

// An activation or a change of Space reads the application in front again: a stale value gives way to what NSWorkspace says, from either notification.
func TestFrontmostFollowsWorkspaceNotifications(t *testing.T) {
	if !loadWindowList() {
		t.Skip("no window server")
	}
	onMain(func() {
		loadAppKit()
		withPool(func() {
			workspace := objc.ID(apple.Class("NSWorkspace")).Send(sel("sharedWorkspace"))
			if workspace.Send(sel("frontmostApplication")) == 0 {
				t.Skip("no application in front, as outside a login session")
			}
			frontmostApp()
			center := workspace.Send(sel("notificationCenter"))
			stale := driver.FrontmostAppInfo{ID: "stale", Name: "stale", PID: -1}
			for _, name := range []string{"NSWorkspaceDidActivateApplicationNotification", "NSWorkspaceActiveSpaceDidChangeNotification"} {
				frontmost = stale
				center.Send(sel("postNotificationName:object:"), apple.Global(appKitPath, name), workspace)
				if want := readFrontmost(); frontmost != want {
					t.Errorf("after %s, frontmost = %+v, want %+v", name, frontmost, want)
				}
			}
		})
	})
}

// Emitting KVO through Foundation tests the real observer subscription and deferred cache refresh without changing the user's presentation options. Actual fullscreen-player acceptance still needs an interactive desktop.
func TestFrontmostFollowsSystemPresentationKVO(t *testing.T) {
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		frontmostApp()
		app := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
		key := apple.NSString("currentSystemPresentationOptions")
		frontmost = driver.FrontmostAppInfo{ID: "stale", PID: -1}
		app.Send(sel("willChangeValueForKey:"), key)
		app.Send(sel("didChangeValueForKey:"), key)
		loop := objc.ID(apple.Class("NSRunLoop")).Send(sel("mainRunLoop"))
		until := objc.ID(apple.Class("NSDate")).Send(sel("dateWithTimeIntervalSinceNow:"), 0.05)
		loop.Send(sel("runUntilDate:"), until)
		if want := readFrontmost(); frontmost != want {
			t.Fatal("presentation notification retained a stale cache")
		}
	})
}
