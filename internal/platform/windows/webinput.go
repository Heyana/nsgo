//go:build windows

package windows

import (
	"encoding/json"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// sendInput delivers trusted user input to the WebView2 page: clicks through
// ICoreWebView2CompositionController::SendMouseInput and keys/text through
// WebView2's DevTools Input.dispatchKeyEvent, taking keyboard focus first as a
// real click does.
func (wv *webView) sendInput(in driver.WebInput) {
	if wv.closed || wv.failed || wv.comp == nil || wv.web == nil {
		return
	}
	wv.takeFocus()
	switch in.Kind {
	case driver.WebInputClick:
		scale := float64(wv.raster)
		if scale <= 0 {
			scale = wv.w.scale
		}
		x, y := webClickPoint(in.Point, scale, wv.zoomFactor())
		wv.sendSyntheticMouse(mouseMove, 0, x, y)
		wv.sendSyntheticMouse(mouseLeftDown, 0x1, x, y)
		wv.sendSyntheticMouse(mouseLeftUp, 0, x, y)
	case driver.WebInputText, driver.WebInputKey:
		for _, ev := range webKeyEventsForInput(in) {
			params, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			wv.devTools("Input.dispatchKeyEvent", string(params), "send key input", func(string, error) {})
		}
	}
}

func (wv *webView) zoomFactor() float64 {
	if wv.controller == nil {
		return 1
	}
	var zoom float64
	if failed(wv.controller.call(controllerGetZoomFactor, uintptr(unsafe.Pointer(&zoom)))) || zoom <= 0 {
		return 1
	}
	return zoom
}

func (wv *webView) sendSyntheticMouse(kind, keys uintptr, x, y int32) {
	if wv.comp == nil {
		return
	}
	args := append([]uintptr{kind, keys, 0}, pointArgs(x, y)...)
	wv.comp.call(compSendMouseInput, args...)
}
