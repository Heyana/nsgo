//go:build windows

package windows

import (
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ICoreWebView2 and ICoreWebView2NewWindowRequestedEventArgs.
const (
	webAddNewWindowRequested = 44
	windowArgsGetURI         = 3
	windowArgsPutHandled     = 6
	windowArgsGetUserGesture = 8
)

var iidNewWindowRequested = parseGUID("d4c185fe-c81c-4989-97af-2d3fa7ab5651")

func (wv *webView) listenWindowRequests() bool {
	h := newHandler(&iidNewWindowRequested, func(_ uintptr, args unsafe.Pointer) {
		a := (*comObject)(args)
		// Set Handled even when no app callback is listening. Otherwise
		// WebView2 creates a popup outside our navigation policy.
		a.call(windowArgsPutHandled, 1)
		if !wv.closed {
			wv.report(wv.config.OpenWindowEvent(driver.WebWindowRequest{
				URL: a.getString(windowArgsGetURI), UserInitiated: a.getBool(windowArgsGetUserGesture)}))
		}
	})
	defer h.release()
	var token eventToken
	return !failed(wv.web.call(webAddNewWindowRequested, h.ptr(), uintptr(unsafe.Pointer(&token))))
}
