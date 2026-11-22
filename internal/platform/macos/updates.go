//go:build darwin && !ios && !nsgo_steam

package macos

import (
	"log"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

const sparkleEnabled = true

var sparkleConfig updates.Config
var sparkleFailure string
var sparkleDelegate objc.ID
var sparkleFound, sparkleNotes string

// startSparkle starts the platform updater before the engine's first view.
func startSparkle(c updates.Config) {
	sparkleConfig = c
	sparkleFailure = ""
	sparkleFound, sparkleNotes = "", ""
	if !c.Checks(false) {
		return
	}
	main := objc.ID(apple.Class("NSBundle")).Send(sel("mainBundle"))
	path := apple.GoString(main.Send(sel("privateFrameworksPath"))) + "/Sparkle.framework"
	framework := objc.ID(apple.Class("NSBundle")).Send(sel("bundleWithPath:"), apple.NSString(path))
	controller := objc.Class(0)
	if framework != 0 && objc.Send[bool](framework, sel("load")) {
		controller = apple.Class("SPUStandardUpdaterController")
	}
	if controller == 0 {
		sparkleFailure = "updates: this direct build's Sparkle.framework could not be loaded"
		log.Printf("nsgo: %s: %s", sparkleFailure, path)
		return
	}
	cls := register("NsgoUpdaterDelegate", apple.Class("NSObject"), []*objc.Protocol{objc.GetProtocol("SPUUpdaterDelegate")}, []objc.MethodDef{
		{Cmd: sel("updater:didFindValidUpdate:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID, item objc.ID) {
			sparkleFound = apple.GoString(item.Send(sel("displayVersionString")))
			sparkleNotes = updates.NotesText(apple.GoString(item.Send(sel("itemDescription"))))
			reportSparkle()
		}},
		{Cmd: sel("updaterDidNotFindUpdate:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) { sparkleFound, sparkleNotes = "", ""; reportSparkle() }},
		{Cmd: sel("updater:didAbortWithError:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID, _ objc.ID) { apple.OnMain(reportSparkle) }},
		{Cmd: sel("updaterSettingsChanged:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) { apple.OnMain(reportSparkle) }},
	})
	sparkleDelegate = objc.ID(cls).Send(selAlloc).Send(sel("init"))
	sparkle = objc.ID(controller).Send(selAlloc).Send(sel("initWithStartingUpdater:updaterDelegate:userDriverDelegate:"), true, sparkleDelegate, objc.ID(0))
	center := objc.ID(apple.Class("NSNotificationCenter")).Send(sel("defaultCenter"))
	center.Send(sel("addObserver:selector:name:object:"), sparkleDelegate, sel("updaterSettingsChanged:"), apple.NSString("NSUserDefaultsDidChangeNotification"), objc.ID(0))
}

func sparkleStatus() driver.UpdaterState {
	appStore := sparkleConfig.Channel == "store"
	state := sparkleConfig.State(appStore)
	if appStore {
		return state
	}
	state.NativePreferences = true
	if sparkleFailure != "" {
		state.Managed = driver.UpdateManagedUnavailable
	}
	if sparkle == 0 {
		return state
	}
	updater := sparkle.Send(sel("updater"))
	state.AutomaticChecks = objc.Send[bool](updater, sel("automaticallyChecksForUpdates"))
	state.AutomaticDownloads = objc.Send[bool](updater, sel("automaticallyDownloadsUpdates"))
	if date := updater.Send(sel("lastUpdateCheckDate")); date != 0 {
		state.LastCheck = time.UnixMilli(int64(objc.Send[float64](date, sel("timeIntervalSince1970")) * 1000))
	}
	state.Available, state.Notes = sparkleFound, sparkleNotes
	return state
}

func reportSparkle() {
	state := sparkleStatus()
	for surface := range surfaces {
		if surface.engine != nil {
			surface.engine.Dispatch(driver.Event{Kind: driver.EventUpdate, Data: &state})
		}
	}
}

func (s *surface) setUpdatePreference(p *driver.UpdatePreference) {
	if sparkle == 0 {
		return
	}
	updater := sparkle.Send(sel("updater"))
	if p.AutomaticChecks != nil {
		updater.Send(sel("setAutomaticallyChecksForUpdates:"), *p.AutomaticChecks)
	}
	if p.AutomaticDownloads != nil {
		updater.Send(sel("setAutomaticallyDownloadsUpdates:"), *p.AutomaticDownloads)
	}
	reportSparkle()
}

func (s *surface) checkForUpdates(background bool, id uint32) {
	if sparkleConfig.Channel == "store" {
		apple.CheckAppStore(sparkleConfig, true, id, s.engine.Dispatch)
		return
	}
	state := sparkleStatus()
	update := driver.Update{App: sparkleConfig.App, Current: sparkleConfig.Version, Managed: state.Managed, Store: state.Store, Err: sparkleFailure}
	ev := driver.Event{Kind: driver.EventUpdate, Count: id, Data: &update}
	if sparkle != 0 {
		ev.Finished = true
		if background {
			sparkle.Send(sel("updater")).Send(sel("checkForUpdatesInBackground"))
		} else {
			sparkle.Send(sel("checkForUpdates:"), objc.ID(0))
		}
	}
	s.engine.Dispatch(ev)
}
