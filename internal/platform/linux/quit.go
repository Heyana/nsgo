//go:build linux && !android

package linux

import "github.com/nsgo-dev/nsgo/internal/driver"

func (w *window) requestClose() {
	switch {
	case w.embedded:
		w.close()
	case w.primary:
		w.requestQuit()
	case w.engine != nil && !w.closing:
		w.engine.Dispatch(driver.Event{Kind: driver.EventWindowClose})
	}
}

func (w *window) requestQuit() {
	if w.opener != nil {
		w = w.opener
	}
	if !w.embedded && !w.closing {
		w.engine.Dispatch(driver.Event{Kind: driver.EventQuitRequest})
	}
}

func (w *window) finishQuit() {
	if w.primary && w.opts.test == nil {
		w.close()
	}
}
