//go:build linux && !android

package linux

import "github.com/nsgo-dev/nsgo/internal/driver"

// webCreateCallback reports a new-window request without creating a WebKitWebView.
func webCreateCallback(page, action, data uintptr) uintptr {
	w, v := webOf(data, page)
	if v != nil {
		url := goString(webkitURIRequestGetURI(webkitNavigationActionGetRequest(action)))
		e := v.web.config.OpenWindowEvent(driver.WebWindowRequest{URL: url,
			UserInitiated: webkitNavigationActionIsUserGesture(action)})
		e.ID = v.id
		w.report(v, e)
	}
	return 0
}
