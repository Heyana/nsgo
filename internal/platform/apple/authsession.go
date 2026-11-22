//go:build darwin

package apple

import (
	"net/url"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

// Auth sessions for macOS and iOS through AuthenticationServices' ASWebAuthenticationSession, which shows the provider's page in a browser sheet sharing the user's Safari cookies (unless ephemeral) and ends once the page redirects to the app's callback scheme, which the app need not register. The iOS backend keeps tvOS unavailable rather than using its different authentication flow.

const authenticationServicesPath = "/System/Library/Frameworks/AuthenticationServices.framework/AuthenticationServices"

var loadAuthenticationServices = sync.OnceValue(func() bool {
	if _, err := purego.Dlopen(authenticationServicesPath, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		return false
	}
	return Class("ASWebAuthenticationSession") != 0
})

// webAuth is an auth session of a surface's: its start's request until it opens, then the ASWebAuthenticationSession and the window it presents over.
type webAuth struct {
	request driver.AuthRequest
	session objc.ID
	anchor  func() objc.ID
}

// webAuths are the sessions by number, touched only on the main thread.
var webAuths = map[uint32]*webAuth{}

// authPresenter is the presentation context provider of every session, which answers each with its surface's window. A session holds it weakly, so it lives as long as the process.
var authPresenter = sync.OnceValue(func() objc.ID {
	var protocols []*objc.Protocol
	if p := objc.GetProtocol("ASWebAuthenticationPresentationContextProviding"); p != nil {
		protocols = append(protocols, p)
	}
	return objc.ID(RegisterClass("NsgoAuthPresenter", Class("NSObject"), protocols, []objc.MethodDef{
		{Cmd: sel("presentationAnchorForWebAuthenticationSession:"), Fn: func(_ objc.ID, _ objc.SEL, session objc.ID) objc.ID {
			for _, a := range webAuths {
				if a.session == session {
					return a.anchor()
				}
			}
			return 0
		}},
	})).Send(sel("new"))
})

// ASWebAuthenticationSessionErrorDomain and its code for a sheet the user closed.
const (
	webAuthErrorDomain   = "com.apple.AuthenticationServices.WebAuthenticationSession"
	webAuthCanceledLogin = 1
)

// HandleAuthSession carries out r for auth session id on the main thread: a start checks the session can catch the redirect and reports it, an open shows the URL over the window anchor returns, and a cancel ends it. dispatch runs on any goroutine.
func HandleAuthSession(id uint32, r driver.AuthRequest, anchor func() objc.ID, dispatch func(driver.Event)) {
	switch r.Command {
	case driver.AuthStart:
		https := strings.EqualFold(r.Scheme, "https")
		switch {
		case !loadAuthenticationServices():
			dispatch(driver.AuthEnd(id, driver.AuthUnavailable, ""))
		case r.Scheme == "":
			dispatch(driver.AuthEnd(id, driver.AuthUnavailable, "the system's session needs a callback scheme"))
		case https && Class("ASWebAuthenticationSessionCallback") == 0:
			dispatch(driver.AuthEnd(id, driver.AuthUnavailable, "HTTPS callbacks need iOS 17.4 or macOS 14.4"))
		default:
			webAuths[id] = &webAuth{request: r, anchor: anchor}
			dispatch(driver.Event{Kind: driver.EventAuthSession, Count: id, Data: &driver.AuthEvent{Phase: driver.AuthRedirect, Redirect: driver.AuthRedirectURI(r)}})
		}
	case driver.AuthOpen:
		a := webAuths[id]
		if a == nil || a.session != 0 {
			return
		}
		openWebAuth(id, a, r.URL, dispatch)
	case driver.AuthCancel:
		a := webAuths[id]
		delete(webAuths, id)
		if a != nil && a.session != 0 {
			// The completion handler then runs with a cancel, which the core no longer hears.
			a.session.Send(sel("cancel"))
		}
	}
}

func openWebAuth(id uint32, a *webAuth, rawURL string, dispatch func(driver.Event)) {
	u := objc.ID(Class("NSURL")).Send(sel("URLWithString:"), NSString(rawURL))
	if u == 0 {
		delete(webAuths, id)
		dispatch(driver.AuthEnd(id, driver.AuthFailed, "the authorization URL is not one Foundation reads"))
		return
	}
	done := objcblock.New(func(_ objc.Block, callback, failure objc.ID) {
		// Read the answer here, as it is gone by the time the main thread runs.
		ev := driver.Event{Kind: driver.EventAuthSession, Count: id, Data: &driver.AuthEvent{Phase: driver.AuthEnded,
			Callback: GoString(callback.Send(sel("absoluteString")))}}
		if callback == 0 {
			ev = driver.AuthEnd(id, driver.AuthFailed, GoString(failure.Send(sel("localizedDescription"))))
			if failure != 0 && objc.Send[int](failure, sel("code")) == webAuthCanceledLogin && GoString(failure.Send(sel("domain"))) == webAuthErrorDomain {
				ev = driver.AuthEnd(id, driver.AuthCancelled, "")
			}
		}
		OnMain(func() {
			if webAuths[id] == a {
				delete(webAuths, id)
			}
			a.session.Send(selRelease)
			dispatch(ev)
		})
	})
	defer done.Release()
	var callback objc.ID
	if strings.EqualFold(a.request.Scheme, "https") {
		callback = httpsAuthCallback(a.request)
		if callback == 0 {
			delete(webAuths, id)
			dispatch(driver.AuthEnd(id, driver.AuthFailed, "the HTTPS callback URL is invalid"))
			return
		}
	}
	session := objc.ID(Class("ASWebAuthenticationSession")).Send(sel("alloc"))
	if callback != 0 {
		session = session.Send(sel("initWithURL:callback:completionHandler:"), u, callback, done)
	} else {
		session = session.Send(sel("initWithURL:callbackURLScheme:completionHandler:"), u, NSString(a.request.Scheme), done)
	}
	if session == 0 {
		delete(webAuths, id)
		dispatch(driver.AuthEnd(id, driver.AuthFailed, "ASWebAuthenticationSession refused the URL"))
		return
	}
	a.session = session
	session.Send(sel("setPresentationContextProvider:"), authPresenter())
	session.Send(sel("setPrefersEphemeralWebBrowserSession:"), a.request.Ephemeral)
	if !objc.Send[bool](session, sel("start")) {
		// A session that does not start never calls its completion handler.
		delete(webAuths, id)
		session.Send(selRelease)
		dispatch(driver.AuthEnd(id, driver.AuthFailed, "ASWebAuthenticationSession did not start: the app may be in the background, or no window presents it"))
	}
}

// httpsAuthCallback matches the same canonical path the prepared redirect gives the app, including escaping and dot segments, rather than the original text.
func httpsAuthCallback(request driver.AuthRequest) objc.ID {
	u, err := url.Parse(driver.AuthRedirectURI(request))
	if err != nil {
		return 0
	}
	return objc.ID(Class("ASWebAuthenticationSessionCallback")).Send(sel("callbackWithHTTPSHost:path:"), NSString(u.Host), NSString(u.Path))
}
