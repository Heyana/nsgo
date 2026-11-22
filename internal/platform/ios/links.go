//go:build ios

package ios

import (
	"os"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// openedURL is the link that last opened the app (Environment's OpenedURL): an App Clip invocation, universal link or registered URL scheme, received when a scene connects or opens a running app.
var openedURL string

// recordOpenedURL keeps the latest invocation and refreshes all running scenes. During cold startup they have no engine yet; their initial environment reads the saved value before the app mounts.
func recordOpenedURL(link string) bool {
	if link == "" {
		return false
	}
	openedURL = link
	for _, s := range scenes {
		if s == nil {
			continue
		}
		if s.engine == nil {
			s.env.OpenedURL = link
			continue
		}
		s.refresh()
	}
	return true
}

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
	return recordOpenedURL(apple.GoString(url.Send(sel("absoluteString"))))
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
		recordOpenedURL(os.Getenv("_XCAppClipURL"))
	}
}

// sceneContinueActivity is the scene delegate's
// scene:continueUserActivity:, as a link opens the running app.
func sceneContinueActivity(_ objc.ID, _ objc.SEL, _, activity objc.ID) {
	takeActivity(activity)
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
		links := contextLinks(contexts)
		if len(links) > 0 {
			recordOpenedURL(links[len(links)-1])
		}
		launchLinks = append(launchLinks, links...)
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
	recordOpenedURL(links[len(links)-1])
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
