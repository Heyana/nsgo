package mobile

import "github.com/nsgo-dev/nsgo/core"

// AppClipOverlayPosition locates the system's full-app recommendation.
type AppClipOverlayPosition = core.AppClipOverlayPosition

const (
	// AppClipOverlayBottom puts the recommendation at the screen's bottom.
	AppClipOverlayBottom = core.AppClipOverlayBottom
	// AppClipOverlayBottomRaised leaves room for a bottom bar.
	AppClipOverlayBottomRaised = core.AppClipOverlayBottomRaised
)

// ShowAppClipOverlay asks the App Store to recommend this App Clip's full app, using StoreKit's App Clip configuration. Call it from an explicit user action. StoreKit decides whether it can load and display the offer; this does not install an app or report a successful presentation.
//
// The returned dismiss function is safe to call repeatedly or from another goroutine. The current scope's cleanup also dismisses its own request, without dismissing a newer request. Outside a real iOS App Clip, or when a confined owner is refused presentation, this is a no-op.
func ShowAppClipOverlay(cx *core.Scope, position AppClipOverlayPosition) (dismiss func()) {
	return cx.ShowAppClipOverlay(position)
}
