//go:build windows

package windows

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

const (
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
)

func (w *window) appWindow() *window {
	if w.hostWindow != nil {
		return w.hostWindow
	}
	return w
}

func (w *window) requestQuit() {
	w = w.appWindow()
	if !w.embedded && !w.destroying {
		w.engine.Dispatch(driver.Event{Kind: driver.EventQuitRequest})
	}
}

func (w *window) finishQuit() {
	if !w.primary || w.destroying || w.opts.test != nil {
		return
	}
	w.destroying = true
	if w.popupTracking {
		endMenu.Call()
	}
	postMessageW.Call(w.hwnd, wmClose, 0, 0)
}

func (w *window) endSession() {
	if w.sessionEnding || w.destroying || w.engine == nil {
		return
	}
	w.sessionEnding = true
	w.engine.CloseForForcedQuit()
	engine.SyncStorage()
	w.finishQuit()
}
