//go:build ios

package ios

import (
	"os"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// openedURL is the web link that last opened the app (Environment's
// OpenedURL): an App Clip's invocation or a universal link, which UIKit
// hands the scene as an activity of browsing the web, as it connects or
// when the link opens the running app again.
var openedURL string

// takeActivity keeps the link of activity, when it browses the web, and
// reports whether it did.
func takeActivity(activity objc.ID) bool {
	if activity == 0 || apple.GoString(activity.Send(sel("activityType"))) != "NSUserActivityTypeBrowsingWeb" {
		return false
	}
	url := activity.Send(sel("webpageURL"))
	if url == 0 {
		return false
	}
	openedURL = apple.GoString(url.Send(sel("absoluteString")))
	return true
}

// takeConnectionActivities keeps the link a scene's connection options
// open it with. Without one, an App Clip takes _XCAppClipURL's, through
// which Xcode, and nsgo dev -clip, open it in the simulator, where no
// link can.
func takeConnectionActivities(options objc.ID) {
	if options != 0 {
		if activities := options.Send(sel("userActivities")); activities != 0 {
			apple.ForEach(activities, func(a objc.ID) { takeActivity(a) })
		}
	}
	if openedURL == "" && appClip() {
		openedURL = os.Getenv("_XCAppClipURL")
	}
}

// sceneContinueActivity is the scene delegate's
// scene:continueUserActivity:, as a link opens the running app.
func sceneContinueActivity(_ objc.ID, _ objc.SEL, _, activity objc.ID) {
	if takeActivity(activity) {
		// Env.OpenedURL is the app's, in every scene.
		for _, s := range scenes {
			s.refresh()
		}
	}
}

// isAppClip is whether the app runs as an App Clip, whose Info.plist
// holds NSAppClip; 0 until read, then 1 or 2.
var isAppClip uint8

func appClip() bool {
	if isAppClip == 0 {
		isAppClip = 2
		bundle := objc.ID(apple.Class("NSBundle")).Send(sel("mainBundle"))
		if bundle.Send(sel("objectForInfoDictionaryKey:"), apple.NSString("NSAppClip")) != 0 {
			isAppClip = 1
		}
	}
	return isAppClip == 1
}

// launchLinks are links of the app's own URL schemes that came before its engine started, as the one a cold launch opens it with, which go to it once it starts (core.Scope.OnOpenLinks keeps them for the first handler).
var launchLinks []string

// contextLinks are the links of a set of UIOpenURLContexts: those of the app's URL schemes (url_schemes, CFBundleURLTypes), not files another app shares.
func contextLinks(contexts objc.ID) []string {
	var links []string
	if contexts == 0 {
		return nil
	}
	apple.ForEach(contexts, func(c objc.ID) {
		u := c.Send(sel("URL"))
		if u == 0 || objc.Send[bool](u, sel("isFileURL")) {
			return
		}
		links = append(links, apple.GoString(u.Send(sel("absoluteString"))))
	})
	return links
}

// takeConnectionLinks keeps the links a scene's connection options open it with, as a link of the app's scheme launches it.
func takeConnectionLinks(options objc.ID) {
	if options != 0 {
		contexts := options.Send(sel("URLContexts"))
		launchLinks = append(launchLinks, contextLinks(contexts)...)
		importContextFiles(contexts)
	}
}

// sceneOpenURLContexts is the scene delegate's scene:openURLContexts:, for documents and links opened while the app runs.
func sceneOpenURLContexts(_ objc.ID, _ objc.SEL, _, contexts objc.ID) {
	importContextFiles(contexts)
	links := contextLinks(contexts)
	if len(links) == 0 {
		return
	}
	if runner == nil || runner.engine == nil {
		launchLinks = append(launchLinks, links...)
		return
	}
	runner.engine.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: links})
}

// sendLaunchLinks sends the links that came before the engine started to it.
func (s *surface) sendLaunchLinks() {
	if len(launchLinks) > 0 {
		s.engine.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: launchLinks})
		launchLinks = nil
	}
}
