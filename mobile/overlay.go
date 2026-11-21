package mobile

import "github.com/nsgo-dev/nsgo/core"

type (
	// Overlay is a window over other apps on Android, the surface OpenOverlay opens: Show, Hide, Focus and Close it, and Visible says whether it shows. Its methods do nothing on a nil Overlay, which OpenOverlay returns where there is none.
	Overlay = core.Window
	// OverlayOptions are an overlay's size, place, input modes and what hears it end.
	OverlayOptions = core.OverlayOptions
	// OverlayEnd is why the platform closed an overlay itself (OverlayOptions.OnEnd).
	OverlayEnd = core.OverlayEnd
)

// The permissions OverlayPermission answers, core.OverlayPermission values.
const (
	OverlayDenied      = core.OverlayDenied
	OverlayGranted     = core.OverlayGranted
	OverlayUnavailable = core.OverlayUnavailable
)

// Why the platform closes an overlay itself: the permission missing as it opened, or the app's manifest not asking for it, the user revoking it, the system or the user stopping its service, or the platform failing to show it.
const (
	OverlayEndDenied      = core.OverlayEndDenied
	OverlayEndUnavailable = core.OverlayEndUnavailable
	OverlayEndRevoked     = core.OverlayEndRevoked
	OverlayEndRemoved     = core.OverlayEndRemoved
	OverlayEndFailed      = core.OverlayEndFailed
)

// OpenOverlay opens a window over other apps on Android showing content, a tree of its own on the app's one engine, which belongs to cx itself rather than the running owner and closes as cx goes; opened with the app's own scope it lasts as long as the app's process even when opened from a button or callback handler. It takes touches and leaves the keys and the touches around it to the app in front, unless its options say otherwise, and a foreground service keeps the app's process while it shows. The app lists android.permission.SYSTEM_ALERT_WINDOW in nsgo.yaml's android.permissions, and the user grants it (RequestOverlayPermission): without it the overlay ends at once, OverlayEndDenied. The app has one overlay at a time: OpenOverlay returns nil while another is open, and on every other platform, where desktop.OpenPanel floats a window instead.
func OpenOverlay(cx *core.Scope, opts OverlayOptions, content func(cx *core.Scope) core.Node) *Overlay {
	return cx.OpenOverlay(opts, content)
}

// OverlayPermission calls done with whether the app may show overlays on Android, as Settings.canDrawOverlays says, without asking the user; unavailable for an app whose manifest does not ask for SYSTEM_ALERT_WINDOW, and elsewhere.
func OverlayPermission(cx *core.Scope, done func(core.OverlayPermission)) { cx.OverlayPermission(done) }

// RequestOverlayPermission opens Android's setting that lets the app draw over other apps, for the user to grant it, and calls done with the permission as the app comes back to the front; at once where the permission is granted already or unavailable. Ask as the user does something that needs it.
func RequestOverlayPermission(cx *core.Scope, done func(core.OverlayPermission)) {
	cx.RequestOverlayPermission(done)
}
