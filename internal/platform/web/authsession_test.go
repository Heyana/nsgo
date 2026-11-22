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

// Real popups and navigation exercise the browser's URL serialization and
// close detection. No provider account, cookies or external network is used.
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
