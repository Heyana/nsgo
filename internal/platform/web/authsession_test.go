//go:build js && wasm && nsgotest

package web

import (
	"net/url"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

type authTestPlatform struct{ core.ApproxTextEngine }

func (authTestPlatform) Mount(*driver.MountBatch)  {}
func (authTestPlatform) RequestFrame()             {}
func (authTestPlatform) SubmitGPU(driver.GPUFrame) {}

// Real popups and navigation exercise the browser's URL serialization and close detection. No provider account, cookies or external network is used.
func TestAuthPopupCallbackAndClose(t *testing.T) {
	events := make(chan driver.Event, 16)
	e := engine.Start(authTestPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{Event: func(_ uint32, ev driver.Event) {
		if ev.Kind == driver.EventAuthSession {
			events <- ev
		}
	}})
	defer e.Close()
	s := &surface{engine: e}
	next := func(id uint32, phase driver.AuthPhase) *driver.AuthEvent {
		t.Helper()
		select {
		case ev := <-events:
			report := ev.Data.(*driver.AuthEvent)
			if ev.Count != id || report.Phase != phase {
				t.Fatalf("session %d phase %v: %+v", ev.Count, report.Phase, report)
			}
			return report
		case <-time.After(5 * time.Second):
			t.Fatal("the browser did not finish the auth request")
			return nil
		}
	}
	start := func(id uint32, path string) string {
		t.Helper()
		doc := js.Global().Get("document")
		button := doc.Call("createElement", "button")
		button.Set("textContent", "Sign in to the test provider")
		button.Set("style", "position:fixed;top:0;left:0;width:100px;height:60px;z-index:1000")
		doc.Get("body").Call("appendChild", button)
		defer button.Call("remove")
		var ls listeners
		defer ls.release()
		ls.on(button, "click", func(event js.Value) {
			if event.Get("isTrusted").Bool() {
				s.authSession(id, driver.AuthRequest{Command: driver.AuthStart, Path: path})
			}
		})
		if err := new(testSurface).do(map[string]any{"op": "press", "x": 30, "y": 30, "hold": 0}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.authSession(id, driver.AuthRequest{Command: driver.AuthCancel}) })
		return next(id, driver.AuthRedirect).Redirect
	}
	for index, path := range []string{"/nsgo-auth-callback", "/登录/戻る", "/nsgo-auth/../callback", "/literal%2Fcallback"} {
		id := uint32(index + 1)
		t.Logf("callback path %q", path)
		redirect := start(id, path)
		query := url.Values{"code": {"synthetic-code+&"}, "state": {"synthetic-state+&"}}
		callback := js.Global().Get("URL").New(redirect + "?" + query.Encode()).Get("href").String()
		s.authSession(id, driver.AuthRequest{Command: driver.AuthOpen, URL: callback})
		report := next(id, driver.AuthEnded)
		if report.Error != "" || report.Callback != callback {
			t.Fatalf("callback for %q: %+v", path, report)
		}
		parsed, err := url.Parse(report.Callback)
		if err != nil || parsed.Query().Get("code") != query.Get("code") || parsed.Query().Get("state") != query.Get("state") {
			t.Fatalf("the callback lost code or state for %q", path)
		}
		if path == "/literal%2Fcallback" && parsed.Path != path {
			t.Fatal("a literal percent was interpreted as an escaped separator")
		}
		if _, exists := webAuths[id]; exists {
			t.Fatal("completed popup retained its session")
		}
	}
	const id = 5
	start(id, "/nsgo-auth-cancel")
	popup := webAuths[id].popup
	s.authSession(id, driver.AuthRequest{Command: driver.AuthOpen, URL: js.Global().Get("location").Get("origin").String() + "/nsgo-auth-wait"})
	popup.Call("close")
	if report := next(id, driver.AuthEnded); report.Error != driver.AuthCancelled || report.Callback != "" {
		t.Fatalf("closed popup: %+v", report)
	}
	select {
	case ev := <-events:
		t.Fatalf("a finished popup answered twice: %+v", ev)
	case <-time.After(300 * time.Millisecond):
	}
}
func TestAuthRedirectChannelAndMatch(t *testing.T) {
	for _, test := range []struct {
		href, redirect string
		match          bool
	}{
		{"https://app.example/oauth/callback?code=ok&state=s", "https://app.example/oauth/callback", true},
		{"https://app.example/oauth/callback#done", "https://app.example/oauth/callback", true},
		{"https://app.example/other?code=ok", "https://app.example/oauth/callback", false},
		{"", "https://app.example/oauth/callback", false},
	} {
		if got := authRedirectMatches(test.href, test.redirect); got != test.match {
			t.Errorf("authRedirectMatches(%q, %q) = %v, want %v", test.href, test.redirect, got, test.match)
		}
	}
	if got, want := authChannelName("https://app.example/oauth/callback?code=ignored"), "nsgo-auth:https://app.example/oauth/callback"; got != want {
		t.Errorf("authChannelName = %q, want %q", got, want)
	}
}

// fakePopup replaces window.open for one session with a popup the test drives: after foreign its location throws on reading, as another origin's does (a revoked Proxy throws natively), and it reads as closed once closed is set, as a popup that COOP severed does.
type fakePopup struct{ obj js.Value }

func (f *fakePopup) foreign() {
	revocable := js.Global().Get("Proxy").Call("revocable", js.Global().Get("Object").New(), js.Global().Get("Object").New())
	revocable.Call("revoke")
	f.obj.Set("location", revocable.Get("proxy"))
}

func installFakePopup(t *testing.T) *fakePopup {
	t.Helper()
	g := js.Global()
	original := g.Get("open")
	f := &fakePopup{obj: g.Get("Object").New()}
	f.obj.Set("closed", false)
	f.obj.Set("location", map[string]any{"href": "about:blank"})
	closeFn := js.FuncOf(func(js.Value, []js.Value) any { return nil })
	f.obj.Set("close", closeFn)
	open := js.FuncOf(func(js.Value, []js.Value) any { return f.obj })
	g.Set("open", open)
	t.Cleanup(func() {
		g.Set("open", original)
		closeFn.Release()
		open.Release()
	})
	return f
}

func TestAuthPopupSeveredByCOOP(t *testing.T) {
	events := make(chan driver.Event, 16)
	e := engine.Start(authTestPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{Event: func(_ uint32, ev driver.Event) {
		if ev.Kind == driver.EventAuthSession {
			events <- ev
		}
	}})
	defer e.Close()
	s := &surface{engine: e}
	next := func(id uint32, within time.Duration) *driver.AuthEvent {
		t.Helper()
		select {
		case ev := <-events:
			if ev.Count != id {
				t.Fatalf("event for session %d, want %d", ev.Count, id)
			}
			return ev.Data.(*driver.AuthEvent)
		case <-time.After(within):
			return nil
		}
	}
	start := func(id uint32, path string) (*fakePopup, string) {
		t.Helper()
		popup := installFakePopup(t)
		s.authSession(id, driver.AuthRequest{Command: driver.AuthStart, Path: path})
		report := next(id, 5*time.Second)
		if report == nil || report.Phase != driver.AuthRedirect {
			t.Fatalf("no redirect: %+v", report)
		}
		s.authSession(id, driver.AuthRequest{Command: driver.AuthOpen, URL: "https://provider.example/authorize"})
		t.Cleanup(func() { s.authSession(id, driver.AuthRequest{Command: driver.AuthCancel}) })
		return popup, report.Redirect
	}
	defer func(wait time.Duration) { severedPopupWait = wait }(severedPopupWait)

	// The provider's page severs the popup as it loads; its redirect page brings the callback back seconds later over the channel and is asked to close.
	severedPopupWait = time.Minute
	popup, redirect := start(11, "/nsgo-auth-coop")
	popup.foreign()
	time.Sleep(300 * time.Millisecond)
	popup.obj.Set("closed", true)
	if report := next(11, 1500*time.Millisecond); report != nil {
		t.Fatalf("a severed popup ended the session before its callback: %+v", report)
	}
	relay := js.Global().Get("BroadcastChannel").New(authChannelName(redirect))
	defer relay.Call("close")
	closed := make(chan struct{}, 1)
	onMessage := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if args[0].Get("data").String() == authCloseMessage {
			closed <- struct{}{}
		}
		return nil
	})
	defer onMessage.Release()
	relay.Set("onmessage", onMessage)
	callback := redirect + "?code=synthetic&state=s"
	relay.Call("postMessage", callback)
	report := next(11, 5*time.Second)
	if report == nil || report.Phase != driver.AuthEnded || report.Error != "" || report.Callback != callback {
		t.Fatalf("severed popup's callback: %+v", report)
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("the redirect page was not asked to close")
	}

	// A severed popup whose callback never comes ends cancelled after the wait.
	severedPopupWait = 700 * time.Millisecond
	popup, _ = start(12, "/nsgo-auth-coop-abandoned")
	popup.foreign()
	time.Sleep(300 * time.Millisecond)
	popup.obj.Set("closed", true)
	if report := next(12, 400*time.Millisecond); report != nil {
		t.Fatalf("ended before the wait: %+v", report)
	}
	if report := next(12, 3*time.Second); report == nil || report.Error != driver.AuthCancelled {
		t.Fatalf("abandoned severed popup: %+v", report)
	}

	// Closed while still on the page's own origin, the popup was closed by the user: the session ends cancelled at once.
	severedPopupWait = time.Minute
	popup, _ = start(13, "/nsgo-auth-closed")
	popup.obj.Set("closed", true)
	if report := next(13, time.Second); report == nil || report.Error != driver.AuthCancelled {
		t.Fatalf("closed same-origin popup: %+v", report)
	}
}
