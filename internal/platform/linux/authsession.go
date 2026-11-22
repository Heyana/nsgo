//go:build linux && !android

package linux

import (
	"github.com/nsgo-dev/nsgo/desktop/locales"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/authscheme"
)

// authSession opens the default browser, catching a registered private scheme through the app's links or serving an empty scheme on loopback.
func (w *window) authSession(id uint32, r driver.AuthRequest) {
	if authscheme.Handle(id, r, w.launchURI, w.engine.Dispatch) {
		return
	}
	w.loopback.Handle(id, r, w.launchURI, locales.In(w.language).AuthSessionDone(), w.engine.Dispatch)
}
