//go:build darwin && !ios

package macos

import (
	"sync"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

var (
	// frontmost is the application in front at its last activation, Space or presentation change. Every surface reports it; only the main thread uses it.
	frontmost      driver.FrontmostAppInfo
	frontmostWatch sync.Once
)

// frontmostApp follows activations, Spaces and the active application's presentation options, including fullscreen changes that create no Space.
func frontmostApp() driver.FrontmostAppInfo {
	frontmostWatch.Do(func() {
		observeApp()
		app := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
		app.Send(sel("addObserver:forKeyPath:options:context:"), appObserver,
			apple.NSString("currentSystemPresentationOptions"), uint(0), uintptr(0))
		frontmost = readFrontmost()
		center := objc.ID(apple.Class("NSWorkspace")).Send(sel("sharedWorkspace")).Send(sel("notificationCenter"))
		for _, name := range []string{"NSWorkspaceDidActivateApplicationNotification", "NSWorkspaceActiveSpaceDidChangeNotification"} {
			apple.Observe(center, apple.Global(appKitPath, name), func(objc.ID) { updateFrontmost() })
		}
	})
	return frontmost
}

// readFrontmost asks NSWorkspace for the active app, and AppKit for its actual fullscreen presentation state. Display-sized bounds alone cannot distinguish fullscreen from a zoomed window with auto-hidden system UI.
func readFrontmost() (app driver.FrontmostAppInfo) {
	front := objc.ID(apple.Class("NSWorkspace")).Send(sel("sharedWorkspace")).Send(sel("frontmostApplication"))
	if front == 0 {
		return app
	}
	app.ID = apple.GoString(front.Send(sel("bundleIdentifier")))
	if app.ID == "" {
		if url := front.Send(sel("executableURL")); url != 0 {
			app.ID = apple.GoString(url.Send(sel("path")))
		}
	}
	app.Name = apple.GoString(front.Send(sel("localizedName")))
	app.PID = int(int32(apple.Msg(front, sel("processIdentifier"))))
	const presentationFullScreen = 1 << 10
	system := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
	app.FullScreen = objc.Send[uint](system, sel("currentSystemPresentationOptions"))&presentationFullScreen != 0
	return app
}

// updateFrontmost is called on AppKit's thread after native notifications.
func updateFrontmost() {
	if app := readFrontmost(); app != frontmost {
		frontmost = app
		for s := range surfaces {
			s.refresh()
		}
	}
}
