// Package mobile holds what only iOS and Android have: the system's tab
// bar and a bar above the software keyboard, as NSH's nsh/mobile. It
// builds on every platform, so one app runs everywhere: elsewhere its
// components take no room, and IsAvailable says whether they show.
package mobile

import "github.com/nsgo-dev/nsgo/core"

type (
	// TabBar is the system's tab bar for an app's top-level sections:
	// UITabBar on iOS (Liquid Glass from iOS 26), a Material 3 navigation
	// bar on Android, each in the system's look. The app gives the tabs,
	// says which is selected, hears the user's choice with OnChange and
	// shows the selected tab's content; it places the bar with its style,
	// whose height (or width on a vertical edge) is the system's where the
	// style leaves it open. Set Edge (or EdgeFunc) to TabBarLeading or
	// TabBarTrailing to draw the bar vertically on that edge, or
	// TabBarAutomatic to follow the system's choice (core.Env.VerticalBarEdge).
	// Elsewhere it takes no room and shows nothing, and the app shows its
	// tabs with base.Tabs.
	TabBar = core.TabBar
	// TabBarItem is one tab of a TabBar.
	TabBarItem = core.TabBarItem
	// TabBarEdge is the edge along which a TabBar lays out its tabs.
	TabBarEdge = core.TabBarEdge
	// TabBadge shows text or a dot on a tab. The zero value clears it.
	TabBadge = core.TabBadge
	// InputAccessoryView is a bar that rides above the software keyboard
	// while an Input naming its ID in InputAccessoryViewID has focus. It
	// is as wide as the surface and as tall as its content, and shows
	// nothing where it is written.
	InputAccessoryView = core.InputAccessoryView
)

// The edges a TabBar may sit on: the bottom by default, a vertical leading or
// trailing edge, or following the system's choice (core.Env.VerticalBarEdge).
const (
	TabBarBottom    = core.TabBarBottom
	TabBarLeading   = core.TabBarLeading
	TabBarTrailing  = core.TabBarTrailing
	TabBarAutomatic = core.TabBarAutomatic
)

// Haptic is feedback the device plays under the user's finger, from
// HapticSelection to HapticError.
type Haptic = core.Haptic

// The haptics PlayHaptic plays: a selection changing, an impact of a
// weight, and a task's outcome.
const (
	HapticSelection = core.HapticSelection
	HapticLight     = core.HapticLight
	HapticMedium    = core.HapticMedium
	HapticHeavy     = core.HapticHeavy
	HapticSoft      = core.HapticSoft
	HapticRigid     = core.HapticRigid
	HapticSuccess   = core.HapticSuccess
	HapticWarning   = core.HapticWarning
	HapticError     = core.HapticError
)

// PlayHaptic plays h: through UIKit's feedback generators on iOS, and
// on Android through the view's haptic feedback, its nearest constant,
// where the user's touch feedback setting allows it. Devices without
// haptics, and every other platform, play nothing.
func PlayHaptic(cx *core.Scope, h Haptic) { cx.PlayHaptic(h) }

// Immersive requests that the window hide its system bars. Android hides its status and navigation bars and reveals them briefly on an edge swipe. HarmonyOS asks the system to hide its status bar, navigation bar and navigation indicator. iOS hides the status bar and lets UIKit hide the home indicator. Pass false to return to the window's regular settings. A shown screen or modal can override it in its StatusBar. Other targets ignore it.
func Immersive(cx *core.Scope, on bool) {
	toggle := core.ToggleOff
	if on {
		toggle = core.ToggleOn
	}
	cx.SetStatusBar(core.StatusBarSetting{Immersive: toggle})
}

// HapticEvent is one event in a HapticPattern.
type HapticEvent = core.HapticEvent

// HapticPattern is a sequence of timed pulses.
type HapticPattern = core.HapticPattern

// PlayHapticPattern plays pattern in place of the current one. An empty
// pattern stops playback. Env.CanPlayHapticPattern reports support.
func PlayHapticPattern(cx *core.Scope, pattern HapticPattern) { cx.PlayHapticPattern(pattern) }

// IsAvailable reports whether the app runs on iOS or Android, where this package's components show; a watch, which reports iOS, and a web build that runs as one (core.Env.Simulated) do not.
func IsAvailable(cx *core.Scope) bool {
	os := cx.Env().OS()
	return (os == core.OSIOS || os == core.OSAndroid) && cx.Env().Idiom() != core.IdiomWatch && !cx.Env().Simulated()
}

// IsAppClip reports whether the app runs as an iOS App Clip, the part of
// an app nsgo.yaml's app_clip builds, which the system runs from a link,
// a code or an NFC tag without installing the app. An app whose clip is
// its own package shows the clip's screens when it does.
func IsAppClip(cx *core.Scope) bool { return cx.Env().AppClip() }

// OpenedURL returns the last link that invoked the iOS or Android app, or the current address of a whole-page web app. It is available before the root renders and follows later invocations and browser address changes. It returns "" when no invocation was supplied or the surface is embedded in a containing page.
func OpenedURL(cx *core.Scope) string { return cx.Env().OpenedURL() }

// OnOpenLinks runs fn with declared URL-scheme and associated HTTPS links the system asks the app to open. nsgo.yaml's url_schemes registers schemes, and app_links registers Android HTTPS hosts. They reach the running app, and the link the app launched to open waits for the first handler. A link carries whatever the page that made it chose, so treat its contents as untrusted input and confirm with the user before acting on it. It is desktop.OnOpenLinks for phones, and either hears the same links.
func OnOpenLinks(cx *core.Scope, fn func(links []string)) { cx.OnOpenLinks(fn) }

// SharedItem is text (Text) or a file (Path and Name) received from another
// app. MIME is the item's media type. Copy the file out of the app's cache
// if it needs to outlive the cache.
type SharedItem = core.SharedItem

// OnShared calls fn for each share the system delivers while cx lives. Shares that launch the app wait for its first handler. On Android, nsgo.yaml's android.share_types declares which media types the app receives. On iOS, apple.share_types declares the UTIs the share extension accepts; it queues each share in the App Group until the containing app is next active. Files arrive as cache copies on both: move or copy one to persistent storage if it needs to outlive the cache.
func OnShared(cx *core.Scope, fn func(items []SharedItem)) { cx.OnShared(fn) }

// ExitApp leaves the app on Android as its back button does when nothing
// in the app takes it: the task's root activity goes to the background,
// any other finishes. iOS apps never leave themselves, so it does nothing
// there, as elsewhere.
func ExitApp(cx *core.Scope) { cx.ExitApp() }

// InstallPackage hands the APK at path, in the app's own storage, to
// Android's package installer, which asks the user to confirm. done,
// when not nil, learns whether the installer started, with the error
// when it did not: a missing file, an app whose manifest does not ask
// for android.permission.REQUEST_INSTALL_PACKAGES (nsgo.yaml's
// android.permissions; an embedding host's own manifest), or a failed
// launch. That it started says nothing of whether the user installed
// the package. Where the user has not let the app install packages
// (CanInstallPackages), the installer sends them to the setting first.
// The installer reads the file through a content URI the runtime grants
// it, so the app keeps it until the installer is done and deletes it
// itself. Elsewhere it answers with an error. It never installs
// silently.
func InstallPackage(cx *core.Scope, path string, done func(started bool, err error)) {
	cx.InstallPackage(path, done)
}

// CanInstallPackages calls done with whether the user lets the app
// install packages on Android, as PackageManager.canRequestPackageInstalls
// says: false until they allow it in the setting OpenInstallSettings
// opens, and always for an app whose manifest does not ask for
// REQUEST_INSTALL_PACKAGES. Elsewhere it answers false.
func CanInstallPackages(cx *core.Scope, done func(allowed bool)) { cx.CanInstallPackages(done) }

// OpenInstallSettings opens Android's setting that lets the app install
// packages, for the user to allow it, and calls done with whether it
// opened. Elsewhere it answers false.
func OpenInstallSettings(cx *core.Scope, done func(opened bool)) { cx.OpenInstallSettings(done) }
