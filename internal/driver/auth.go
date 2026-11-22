package driver

import (
	"net/url"
	"strings"
)

// AuthCommand is what an ItemAuthSession asks of its session.
type AuthCommand uint8

const (
	// AuthStart prepares the session's redirect, which the platform reports with AuthRedirect: Scheme, Host and Path say what the provider redirects to, Ephemeral whether the session shares no cookies with the browser.
	AuthStart AuthCommand = iota
	// AuthOpen opens URL, the authorization URL the app built for the redirect, and ends the session with AuthEnded once the provider redirects or the session ends otherwise.
	AuthOpen
	// AuthCancel ends the session at once, reporting nothing more.
	AuthCancel
)

// AuthRequest is an ItemAuthSession's request.
type AuthRequest struct {
	Command AuthCommand
	// Scheme is the app's callback scheme, "https" with Host for an HTTPS callback; "" for a loopback redirect where the platform serves one.
	Scheme string
	Host   string
	// Path is the redirect's path, starting with "/".
	Path      string
	Ephemeral bool
	// URL is AuthOpen's authorization URL.
	URL string
}

// AuthPhase is what an EventAuthSession reports of its session.
type AuthPhase uint8

const (
	// AuthRedirect carries the redirect URI the session catches, which the app builds the authorization URL for.
	AuthRedirect AuthPhase = iota
	// AuthEnded ends the session with Callback, or Error when it ended without one. A platform reports it once, last.
	AuthEnded
)

// The errors a session ends with.
const (
	AuthCancelled   = "cancelled"
	AuthUnavailable = "unavailable"
	AuthTimeout     = "timeout"
	AuthFailed      = "failed"
)

// AuthEvent is an EventAuthSession's report.
type AuthEvent struct {
	Phase    AuthPhase
	Redirect string
	// OpenLinks says EventOpenLinks carries this redirect's callback rather than a native session reporting AuthEnded. The core claims that URI across the app's surfaces until the session ends.
	OpenLinks bool
	// Callback is the URL the provider redirected to, query and fragment included.
	Callback string
	// Error is one of the Auth errors, "" for none, and Message what the platform said of a failure.
	Error   string
	Message string
}

// AuthRedirectURI is the redirect of a session that the system catches by its scheme, r being its AuthStart: scheme:/path for an app's own scheme, https://host/path for an HTTPS one.
func AuthRedirectURI(r AuthRequest) string {
	base := &url.URL{Scheme: strings.ToLower(r.Scheme), Host: r.Host}
	u := base.ResolveReference(&url.URL{Path: r.Path})
	// Keep private callbacks in scheme:/path form. A path starting with // needs the empty authority to keep those slashes in the path.
	u.OmitHost = u.Host == "" && !strings.HasPrefix(u.Path, "//")
	return u.String()
}

// AuthEnd is the event ending session id with err, one of the Auth errors, and message.
func AuthEnd(id uint32, err, message string) Event {
	return Event{Kind: EventAuthSession, Count: id, Data: &AuthEvent{Phase: AuthEnded, Error: err, Message: message}}
}

// NoAuthSession is how a platform without auth sessions answers an ItemAuthSession: its start ends unavailable, and the rest needs no answer. ok reports whether there is an event to dispatch.
func NoAuthSession(it *MountItem) (ev Event, ok bool) {
	if it.Auth == nil || it.Auth.Command != AuthStart {
		return Event{}, false
	}
	return AuthEnd(it.Count, AuthUnavailable, ""), true
}
