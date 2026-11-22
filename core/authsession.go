package core

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// AuthError is why an auth session ended without its callback.
type AuthError string

const (
	// AuthCancelled: the user closed the session, or the app cancelled it.
	AuthCancelled AuthError = driver.AuthCancelled
	// AuthUnavailable: this platform has no session for the redirect asked for, or the scope's Access does not grant AccessAuthSession.
	AuthUnavailable AuthError = driver.AuthUnavailable
	// AuthTimeout: AuthSessionOptions.Timeout passed first.
	AuthTimeout AuthError = driver.AuthTimeout
	// AuthFailed: the session or its options failed; the error wraps it with what failed.
	AuthFailed AuthError = driver.AuthFailed
)

func (e AuthError) Error() string { return "nsgo: auth session: " + string(e) }

// AuthSessionOptions say what redirect an auth session catches.
type AuthSessionOptions struct {
	// Scheme is the app's own callback scheme, such as "com.example.app": the redirect is scheme:/path. Windows and Linux need it registered in nsgo.yaml's url_schemes; macOS's, iOS's and watchOS's system session catches it without registration. "https" with Host catches an HTTPS redirect to the app's associated domain, from iOS 17.4, macOS 14.4 and watchOS 10.4. Without one, desktops serve a loopback redirect, http://127.0.0.1:port/path on a random port, and the web one on the page's origin.
	Scheme string
	Host   string
	// Path is the redirect's path, "/" when empty. Platforms treat it as decoded text, escape it and resolve dot segments before reporting the redirect.
	Path string
	// Ephemeral asks the session to share no cookies or data with the user's browser (prefersEphemeralWebBrowserSession), where the platform can.
	Ephemeral bool
	// Timeout ends the session with AuthTimeout after it, 0 never. A loopback session cannot tell that the user closed the browser's tab, so give it a timeout or a way to cancel.
	Timeout time.Duration
}

// authSession is an auth session waiting for the platform.
type authSession struct {
	tree    *Tree
	owner   *Scope
	id      uint32
	url     func(redirect string) string
	done    func(callback string, err error)
	stop    func() bool
	ended   bool
	request driver.AuthRequest
	link    string
}

// AuthSession signs the user in with an OAuth provider in the system's authentication session (RFC 8252): it prepares the redirect opts say, asks url for the authorization URL to open with it, which is the redirect_uri the URL names, and calls done once with the URL the provider redirected to, its query holding the code and state, or with an AuthError: AuthCancelled when the user closes the session. The app makes the PKCE verifier and the state and checks them. macOS, iOS and watchOS open ASWebAuthenticationSession; desktops otherwise open the default browser and catch the registered scheme or serve an empty one on loopback, and the web a popup. Elsewhere, or for a redirect a platform cannot catch, done hears AuthUnavailable. cancel ends the session with AuthCancelled; a session outlives the owner that started it, done running under the root then, as other answers do. Reusing a browser callback URI cancels its previous session; distinct callback URIs can wait together.
func (cx *Scope) AuthSession(url func(redirect string) string, opts AuthSessionOptions, done func(callback string, err error)) (cancel func()) {
	if done == nil {
		done = func(string, error) {}
	}
	if err := checkAuthOptions(&opts); err != nil || url == nil {
		if url == nil {
			err = fmt.Errorf("%w: no authorization URL", AuthFailed)
		}
		done("", err)
		return func() {}
	}
	t := cx.tree()
	if t == nil || cx.denied(serviceAuthSession, "") {
		done("", AuthUnavailable)
		return func() {}
	}
	t.alertID++
	s := &authSession{tree: t, owner: cx.current(), id: t.alertID, url: url, done: done}
	if t.auths == nil {
		t.auths = map[uint32]*authSession{}
		// A session outlives the region that started it, but not its entire surface. Release its callback claim and timer as that surface ends.
		t.root.addCleanup(func() {
			for _, pending := range t.auths {
				pending.send(driver.AuthRequest{Command: driver.AuthCancel})
				pending.release()
			}
		})
	}
	t.auths[s.id] = s
	if opts.Timeout > 0 {
		root := t.root
		s.stop = cx.rt.clock.AfterFunc(opts.Timeout, func() {
			root.Post(func() { s.cancel(AuthTimeout) })
		})
	}
	s.request = driver.AuthRequest{Scheme: opts.Scheme, Host: opts.Host, Path: opts.Path, Ephemeral: opts.Ephemeral}
	s.send(driver.AuthRequest{Command: driver.AuthStart})
	return func() { s.cancel(AuthCancelled) }
}

// checkAuthOptions refuses a scheme or path a redirect cannot have, and fills the path in.
func checkAuthOptions(opts *AuthSessionOptions) error {
	if opts.Path == "" {
		opts.Path = "/"
	}
	if !strings.HasPrefix(opts.Path, "/") || strings.ContainsAny(opts.Path, "?# ") {
		return fmt.Errorf("%w: the path %q does not start with / or holds ?, # or a space", AuthFailed, opts.Path)
	}
	if opts.Scheme == "" {
		if opts.Host != "" {
			return fmt.Errorf("%w: a host needs the https scheme", AuthFailed)
		}
		return nil
	}
	for i, c := range opts.Scheme {
		letter := c|0x20 >= 'a' && c|0x20 <= 'z'
		if !letter && (i == 0 || !(c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.')) {
			return fmt.Errorf("%w: %q is no URL scheme", AuthFailed, opts.Scheme)
		}
	}
	switch https := strings.EqualFold(opts.Scheme, "https"); {
	case https && (opts.Host == "" || strings.ContainsAny(opts.Host, "/?#@ ")):
		return fmt.Errorf("%w: an https callback needs a host, not %q", AuthFailed, opts.Host)
	case !https && opts.Host != "":
		return fmt.Errorf("%w: a host needs the https scheme", AuthFailed)
	case strings.EqualFold(opts.Scheme, "http"):
		return fmt.Errorf("%w: an http callback is a loopback one, which an empty Scheme asks for", AuthFailed)
	}
	return nil
}

func (s *authSession) send(r driver.AuthRequest) {
	r.Scheme, r.Host, r.Path, r.Ephemeral = s.request.Scheme, s.request.Host, s.request.Path, s.request.Ephemeral
	s.tree.commands = append(s.tree.commands, driver.MountItem{Kind: driver.ItemAuthSession, Count: s.id, Auth: &r})
	s.tree.changed = true
}

// cancel ends the session with err, having the platform end it too.
func (s *authSession) cancel(err AuthError) {
	if s.ended {
		return
	}
	s.send(driver.AuthRequest{Command: driver.AuthCancel})
	s.end("", err)
}

// end ends the session, once.
func (s *authSession) end(callback string, err error) {
	if !s.release() {
		return
	}
	s.within(func() { s.done(callback, err) })
}

// release drops every resource that still claims this session, once.
func (s *authSession) release() bool {
	if s.ended {
		return false
	}
	s.ended = true
	delete(s.tree.auths, s.id)
	if links := s.tree.rt.authLinks; links[s.link] == s {
		delete(links, s.link)
	}
	if s.stop != nil {
		s.stop()
	}
	return true
}

// within runs fn under the session's owner, or the root once it is gone.
func (s *authSession) within(fn func()) {
	owner := s.owner
	if owner.disposed {
		owner = s.tree.root
	}
	s.tree.within(owner, fn)
}

// authEvent hands a platform's report to the session it is of.
func (t *Tree) authEvent(ev driver.Event) {
	s := t.auths[ev.Count]
	report, _ := ev.Data.(*driver.AuthEvent)
	if s == nil || report == nil {
		return
	}
	switch report.Phase {
	case driver.AuthRedirect:
		var u string
		s.within(func() { u = s.url(report.Redirect) })
		if s.ended {
			return // url cancelled it
		}
		if err := checkAuthURL(u); err != nil {
			s.send(driver.AuthRequest{Command: driver.AuthCancel})
			s.end("", fmt.Errorf("%w: %v", AuthFailed, err))
			return
		}
		if report.OpenLinks {
			key, ok := authLinkKey(report.Redirect)
			if !ok {
				s.cancel(AuthUnavailable)
				return
			}
			rt := t.rt
			if rt.authLinks == nil {
				rt.authLinks = map[string]*authSession{}
			}
			if previous := rt.authLinks[key]; previous != nil && previous != s {
				previous.cancel(AuthCancelled)
			}
			if s.ended {
				return
			}
			s.link, rt.authLinks[key] = key, s
		}
		s.send(driver.AuthRequest{Command: driver.AuthOpen, URL: u})
	case driver.AuthEnded:
		if report.Error == "" {
			s.end(report.Callback, nil)
			return
		}
		var err error = AuthError(report.Error)
		if report.Message != "" {
			err = fmt.Errorf("%w: %s", err, report.Message)
		}
		s.end("", err)
	}
}

// authLinkKey canonicalizes only a private, hierarchical callback URI. Query and fragment are the provider's result; their state remains the app's check.
func authLinkKey(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host != "" || u.User != nil || u.Opaque != "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "http" || scheme == "https" || scheme == "file" || !strings.HasPrefix(u.Path, "/") {
		return "", false
	}
	return driver.AuthRedirectURI(driver.AuthRequest{Scheme: scheme, Path: u.Path}), true
}

// openAuthLinks consumes the callbacks claimed by browser auth sessions and leaves ordinary or late links for desktop.OnOpenLinks and mobile.OnOpenLinks.
func (rt *runtime) openAuthLinks(links []string) []string {
	remaining := make([]string, 0, len(links))
	for _, link := range links {
		key, ok := authLinkKey(link)
		if s := rt.authLinks[key]; ok && s != nil && !s.ended {
			s.send(driver.AuthRequest{Command: driver.AuthCancel})
			s.end(link, nil)
		} else {
			remaining = append(remaining, link)
		}
	}
	return remaining
}

// checkAuthURL refuses an authorization URL that is no web address.
func checkAuthURL(u string) error {
	if err := CheckURL(u); err != nil {
		return err
	}
	parsed, _ := url.Parse(u)
	if s := strings.ToLower(parsed.Scheme); s != "https" && s != "http" {
		return fmt.Errorf("the authorization URL %q is no web address", u)
	}
	return nil
}
