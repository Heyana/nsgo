package core

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// prepareBrowserAuth runs the platform's two-step handshake, without supplying its callback: that must arrive through the app's actual open-link event.
func prepareBrowserAuth(t *testing.T, scope *Scope, opts AuthSessionOptions, done func(string, error)) (uint32, func()) {
	t.Helper()
	var redirect string
	cancel := scope.AuthSession(func(uri string) string {
		redirect = uri
		return "https://provider.example/authorize"
	}, opts, done)
	tree := scope.tree()
	batch := tree.Commit()
	var start *driver.MountItem
	for i := range batch.Items {
		if it := &batch.Items[i]; it.Kind == driver.ItemAuthSession && it.Auth.Command == driver.AuthStart {
			start = it
		}
	}
	if start == nil {
		t.Fatal("no auth start request")
	}
	uri := driver.AuthRedirectURI(*start.Auth)
	tree.Dispatch(driver.Event{Kind: driver.EventAuthSession, Count: start.Count,
		Data: &driver.AuthEvent{Phase: driver.AuthRedirect, Redirect: uri, OpenLinks: true}})
	opened := false
	for _, it := range tree.Commit().Items {
		if it.Kind == driver.ItemAuthSession && it.Auth.Command == driver.AuthOpen {
			opened = it.Auth.URL == "https://provider.example/authorize" && it.Auth.Scheme == opts.Scheme && it.Auth.Path == start.Auth.Path
		}
	}
	if !opened || redirect != uri {
		t.Fatalf("authorization URL did not receive and open the prepared redirect %q", uri)
	}
	return start.Count, cancel
}

func TestAuthSessionCompletesFromItsOwnSchemeLink(t *testing.T) {
	h := newHost(t, Size{}, nil)
	var ordinary []string
	h.root.OnOpenLinks(func(links []string) { ordinary = append(ordinary, links...) })
	var result string
	calls := 0
	id, _ := prepareBrowserAuth(t, h.root, AuthSessionOptions{Scheme: "com.example.app", Path: "/before/../登录/%2F"}, func(callback string, err error) {
		if err != nil {
			t.Error(err)
		}
		result = callback
		calls++
	})
	callback := "COM.Example.APP:/登录/%252F?code=a%2Bb&state=s1#token=synthetic"
	unrelated := []string{"other:/登录/%252F", "com.example.app:/elsewhere", "com.example.app://other/登录/%252F", "com.example.app:opaque"}
	h.tree.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: append(slices.Clone(unrelated), callback)})
	if calls != 1 || result != callback || !slices.Equal(ordinary, unrelated) {
		t.Fatalf("calls=%d callback=%q ordinary=%v", calls, result, ordinary)
	}
	// The callback carries the provider's code: it must not become the app's address either.
	if got := h.root.AppURL(); got != unrelated[len(unrelated)-1] {
		t.Fatalf("app address %q, want the last ordinary link", got)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: []string{"other:/next"}})
	prepareBrowserAuth(t, h.root, AuthSessionOptions{Scheme: "com.example.app", Path: "/only"}, func(string, error) {})
	h.tree.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: []string{"com.example.app:/only?code=secret"}})
	if got := h.root.AppURL(); got != "other:/next" {
		t.Fatalf("a claimed callback alone became the app address %q", got)
	}
	// A late native answer cannot answer again, and a late link is ordinary.
	h.tree.Dispatch(driver.Event{Kind: driver.EventAuthSession, Count: id, Data: &driver.AuthEvent{Phase: driver.AuthEnded, Callback: callback}})
	h.tree.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: []string{callback}})
	if calls != 1 || ordinary[len(ordinary)-1] != callback || len(h.root.rt.authLinks) != 0 {
		t.Fatal("a completed session still claimed links or answered twice")
	}
}

func TestAuthSessionLinkCancellationTimeoutAndOwnerDisposal(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		h := newHost(t, Size{}, nil)
		owner := h.root.Child()
		var result error
		calls := 0
		_, cancel := prepareBrowserAuth(t, owner, AuthSessionOptions{Scheme: "app", Path: "/oauth", Timeout: time.Second}, func(_ string, err error) {
			result = err
			calls++
		})
		owner.Dispose()
		if len(h.root.rt.authLinks) != 1 {
			t.Fatal("a session did not outlive its region owner")
		}
		want := AuthCancelled
		if timeout {
			want = AuthTimeout
			h.clock.Advance(time.Second)
			h.drain()
		} else {
			cancel()
		}
		if calls != 1 || !errors.Is(result, want) || len(h.root.rt.authLinks) != 0 {
			t.Fatalf("timeout=%v: calls=%d error=%v claims=%d", timeout, calls, result, len(h.root.rt.authLinks))
		}
		h.tree.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: []string{"app:/oauth?late=true"}})
		if calls != 1 {
			t.Fatal("a cancelled session caught its late callback")
		}
	}
}

func TestAuthSessionReplacesOnlyTheSameBrowserRedirect(t *testing.T) {
	h := newHost(t, Size{}, nil)
	var first error
	prepareBrowserAuth(t, h.root, AuthSessionOptions{Scheme: "app", Path: "/first"}, func(_ string, err error) { first = err })
	otherCalls, latestCalls := 0, 0
	prepareBrowserAuth(t, h.root, AuthSessionOptions{Scheme: "app", Path: "/other"}, func(string, error) { otherCalls++ })
	prepareBrowserAuth(t, h.root, AuthSessionOptions{Scheme: "app", Path: "/first"}, func(string, error) { latestCalls++ })
	if !errors.Is(first, AuthCancelled) || otherCalls != 0 {
		t.Fatalf("replacement error=%v, other calls=%d", first, otherCalls)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: []string{"app:/first?code=latest", "app:/other?code=other"}})
	if latestCalls != 1 || otherCalls != 1 {
		t.Fatalf("latest=%d other=%d", latestCalls, otherCalls)
	}
}

func TestAuthSessionCallbackReachesAnOpenedSurface(t *testing.T) {
	var scope *Scope
	var win *Window
	w := newWindows(t, func(cx *Scope) {
		cx.SetShutdownMode(ShutdownOnExplicit)
		win = cx.OpenWindow(WindowOptions{Title: "sign-in"}, func(child *Scope) Node { scope = child; return View{} })
	})
	var callback string
	prepareBrowserAuth(t, scope, AuthSessionOptions{Scheme: "app", Path: "/oauth"}, func(uri string, err error) {
		if scope.CurrentWindow() != win || err != nil {
			t.Errorf("callback lost its surface: %v", err)
		}
		callback = uri
	})
	w.app.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: []string{"app:/oauth?code=child"}})
	if callback != "app:/oauth?code=child" {
		t.Fatalf("main-surface delivery did not reach the session: %q", callback)
	}
	prepareBrowserAuth(t, scope, AuthSessionOptions{Scheme: "app", Path: "/oauth", Timeout: time.Hour}, func(string, error) {
		t.Error("a destroyed surface's session answered")
	})
	win.Close()
	if len(w.root.rt.authLinks) != 0 || len(win.t.auths) != 0 {
		t.Fatal("closing the surface retained a callback claim")
	}
}
