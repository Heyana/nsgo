//go:build js && wasm

package web

import (
	"net/url"
	"strconv"
	"strings"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Auth sessions on the web open the provider in a popup window, which keeps the page and the app's state where a redirect of the page itself would lose them. The popup opens blank as the session starts, while the press that started it still lets a page open one, and goes to the authorization URL once the app built it. The provider redirects to a page of the app's origin, the redirect, which the page reads from the popup, as it may once the popup is back on its origin, and closes; a popup the user closes ends the session cancelled. The page served at the redirect shows only for that moment.

// webAuth is a popup an auth session shows.
type webAuth struct {
	popup    js.Value
	redirect string
	stop     chan struct{}
}

// webAuths are the sessions by number; the page runs them on one thread.
var webAuths = map[uint32]*webAuth{}

// authSession carries out an auth session's request.
func (s *surface) authSession(id uint32, r driver.AuthRequest) {
	dispatch := s.engine.Dispatch
	switch r.Command {
	case driver.AuthStart:
		loc := js.Global().Get("location")
		path := (&url.URL{Path: r.Path}).EscapedPath()
		redirect := loc.Get("origin").String() + path
		switch {
		case r.Scheme == "":
		case strings.EqualFold(r.Scheme, "https") && strings.EqualFold(r.Host, loc.Get("host").String()):
			redirect = "https://" + loc.Get("host").String() + path
		default:
			dispatch(driver.AuthEnd(id, driver.AuthUnavailable, "the web catches only redirects to the page's own origin"))
			return
		}
		// Navigation serializes Unicode and resolves dot segments. Give the app
		// that same URL, so follow compares the actual callback to its redirect.
		redirect = js.Global().Get("URL").New(redirect).Get("href").String()
		popup := js.Global().Call("open", "", "nsgo-auth-"+strconv.FormatUint(uint64(id), 10), "popup,width=520,height=680")
		if !popup.Truthy() {
			dispatch(driver.AuthEnd(id, driver.AuthFailed, "the browser blocked the sign-in popup: start a session from a press"))
			return
		}
		webAuths[id] = &webAuth{popup: popup, redirect: redirect, stop: make(chan struct{})}
		dispatch(driver.Event{Kind: driver.EventAuthSession, Count: id, Data: &driver.AuthEvent{Phase: driver.AuthRedirect, Redirect: redirect}})
	case driver.AuthOpen:
		a := webAuths[id]
		if a == nil {
			return
		}
		a.popup.Get("location").Set("href", r.URL)
		go a.follow(id, dispatch)
	case driver.AuthCancel:
		if a := webAuths[id]; a != nil {
			a.end(id)
		}
	}
}

// follow reads where the popup is five times a second until it is back at the redirect or closed.
func (a *webAuth) follow(id uint32, dispatch func(driver.Event)) {
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-a.stop:
			return
		case <-tick.C:
		}
		if a.popup.Get("closed").Bool() {
			a.end(id)
			dispatch(driver.AuthEnd(id, driver.AuthCancelled, ""))
			return
		}
		href := popupHref(a.popup)
		if base, _, _ := strings.Cut(strings.SplitN(href, "#", 2)[0], "?"); base == a.redirect {
			a.end(id)
			dispatch(driver.Event{Kind: driver.EventAuthSession, Count: id, Data: &driver.AuthEvent{Phase: driver.AuthEnded, Callback: href}})
			return
		}
	}
}

// popupHref is where popup is, "" while it shows another origin's page, whose location the page may not read. Reading it throws then, which only a call turns into a panic Go recovers from, so it reads through Reflect.get.
func popupHref(popup js.Value) (href string) {
	defer func() {
		if recover() != nil {
			href = ""
		}
	}()
	get := js.Global().Get("Reflect").Get("get")
	return get.Invoke(get.Invoke(popup, "location"), "href").String()
}

// end closes the popup and forgets the session.
func (a *webAuth) end(id uint32) {
	if webAuths[id] != a {
		return
	}
	delete(webAuths, id)
	close(a.stop)
	a.popup.Call("close")
}
