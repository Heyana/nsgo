//go:build darwin

package apple

import (
	"runtime"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// appleWebOpenWindow gives the app the request and returns nil to WebKit.
func appleWebOpenWindow(self objc.ID, _ objc.SEL, _, _, action, _ objc.ID) objc.ID {
	w := webHandlers[self]
	if w == nil {
		return 0
	}
	url := GoString(action.Send(sel("request")).Send(sel("URL")).Send(sel("absoluteString")))
	// WebKit exposes the navigation type, but not its user gesture flag.
	// On macOS, the pop-up policy also admits window.open only when
	// WebKit associated it with a gesture. iOS has compatibility
	// exceptions to that policy, so there we report only links and forms.
	const other, link, form = -1, 0, 1 // WKNavigationType
	kind := objc.Send[int](action, sel("navigationType"))
	gesture := kind == link || kind == form || kind == other && runtime.GOOS == "darwin"
	if !fileNavigable(url, w.fileDir, w.config.Restricted) {
		w.Dispatch(driver.Event{Kind: driver.EventError, Text: driver.RefusedNavigation(url),
			Navigation: &driver.WebNavigation{URL: url}})
		return 0
	}
	w.Dispatch(w.config.OpenWindowEvent(driver.WebWindowRequest{URL: url, UserInitiated: gesture}))
	return 0
}
