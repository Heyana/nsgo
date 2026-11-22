package watchos

import (
	"net/url"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// authSessions prepares redirects in Go and sends the open/cancel to the SwiftUI host. The host retains each native session until its completion. Its methods run on the host's main thread.
type authSessions struct{ pending map[uint32]*authRequest }

type authRequest struct {
	request driver.AuthRequest
	opened  bool
}

func (a *authSessions) handle(id uint32, r driver.AuthRequest, w *writer, dispatch func(driver.Event)) {
	switch r.Command {
	case driver.AuthStart:
		if r.Scheme == "" {
			dispatch(driver.AuthEnd(id, driver.AuthUnavailable, "watchOS needs a callback scheme or an associated HTTPS domain"))
			return
		}
		if a.pending == nil {
			a.pending = map[uint32]*authRequest{}
		}
		a.pending[id] = &authRequest{request: r}
		dispatch(driver.Event{Kind: driver.EventAuthSession, Count: id, Data: &driver.AuthEvent{Phase: driver.AuthRedirect, Redirect: driver.AuthRedirectURI(r)}})
	case driver.AuthOpen:
		p := a.pending[id]
		if p == nil || p.opened {
			return
		}
		p.opened = true
		w.op(opAuthSession)
		w.u32(id)
		w.byte(byte(driver.AuthOpen))
		w.str(r.URL)
		w.str(p.request.Scheme)
		// The host's HTTPS callback matches the host and normalized path of the prepared redirect, as the Apple backend's does (httpsAuthCallback).
		host, path := p.request.Host, p.request.Path
		if u, err := url.Parse(driver.AuthRedirectURI(p.request)); err == nil && u.Host != "" {
			host, path = u.Host, u.Path
		}
		w.str(host)
		w.str(path)
		w.flag(p.request.Ephemeral)
	case driver.AuthCancel:
		p := a.pending[id]
		delete(a.pending, id)
		if p != nil && p.opened {
			w.op(opAuthSession)
			w.u32(id)
			w.byte(byte(driver.AuthCancel))
		}
	}
}

// finished reads the host's outcome: callback (0), cancelled (1), failed (2) or unavailable (3). Late completions of a cancelled request are ignored.
func (a *authSessions) finished(id uint32, outcome uint8, value string, dispatch func(driver.Event)) {
	p := a.pending[id]
	if p == nil || !p.opened {
		return
	}
	delete(a.pending, id)
	var ev driver.Event
	switch outcome {
	case 0:
		ev = driver.Event{Kind: driver.EventAuthSession, Count: id, Data: &driver.AuthEvent{Phase: driver.AuthEnded, Callback: value}}
	case 1:
		ev = driver.AuthEnd(id, driver.AuthCancelled, "")
	case 3:
		ev = driver.AuthEnd(id, driver.AuthUnavailable, value)
	default:
		ev = driver.AuthEnd(id, driver.AuthFailed, value)
	}
	dispatch(ev)
}
