package loopback

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// events collects what a session dispatches.
type events chan *driver.AuthEvent

func (e events) dispatch(ev driver.Event) { e <- ev.Data.(*driver.AuthEvent) }

func (e events) next(t *testing.T) *driver.AuthEvent {
	t.Helper()
	select {
	case ev := <-e:
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
		return nil
	}
}

func TestLoopbackTakesOneCallback(t *testing.T) {
	var s Sessions
	evs := make(events, 4)
	s.Handle(1, driver.AuthRequest{Command: driver.AuthStart, Path: "/cb"}, nil, "Done <here>", evs.dispatch)
	ev := evs.next(t)
	if ev.Phase != driver.AuthRedirect || !strings.HasPrefix(ev.Redirect, "http://127.0.0.1:") || !strings.HasSuffix(ev.Redirect, "/cb") {
		t.Fatalf("redirect %+v", ev)
	}
	redirect := ev.Redirect
	var opened string
	s.Handle(1, driver.AuthRequest{Command: driver.AuthOpen, URL: "https://provider.example/auth"}, func(u string, answer func(bool)) { opened = u; answer(true) }, "", evs.dispatch)
	if opened != "https://provider.example/auth" {
		t.Fatalf("opened %q", opened)
	}
	if r, err := http.Get(strings.TrimSuffix(redirect, "/cb") + "/favicon.ico"); err != nil || r.StatusCode != http.StatusNotFound {
		t.Fatalf("another path: %v, %v", r, err)
	}
	r, err := http.Get(redirect + "?code=abc&state=s1")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if !strings.Contains(string(body), "Done &lt;here&gt;") {
		t.Fatalf("page %q", body)
	}
	ev = evs.next(t)
	if ev.Phase != driver.AuthEnded || ev.Error != "" || ev.Callback != redirect+"?code=abc&state=s1" {
		t.Fatalf("end %+v", ev)
	}
	if s.Has(1) {
		t.Fatal("the session stayed")
	}
	if _, err := http.Get(redirect); err == nil {
		t.Fatal("still listening after the callback")
	}
}

func TestLoopbackCancelStopsListening(t *testing.T) {
	var s Sessions
	evs := make(events, 4)
	s.Handle(2, driver.AuthRequest{Command: driver.AuthStart, Path: "/"}, nil, "", evs.dispatch)
	redirect := evs.next(t).Redirect
	s.Handle(2, driver.AuthRequest{Command: driver.AuthCancel}, nil, "", evs.dispatch)
	if _, err := http.Get(redirect); err == nil {
		t.Fatal("still listening after a cancel")
	}
	select {
	case ev := <-evs:
		t.Fatalf("a cancel reported %+v", ev)
	default:
	}
}

func TestLoopbackNormalizesCallbackPaths(t *testing.T) {
	for _, test := range []struct{ path, encoded string }{
		{"/登录/戻る", "/%E7%99%BB%E5%BD%95/%E6%88%BB%E3%82%8B"},
		{"/auth/../callback", "/callback"},
		{"/literal%2Fcallback", "/literal%252Fcallback"},
	} {
		t.Run(test.path, func(t *testing.T) {
			var sessions Sessions
			defer sessions.Close()
			evs := make(events, 4)
			sessions.Handle(1, driver.AuthRequest{Command: driver.AuthStart, Path: test.path}, nil, "Done", evs.dispatch)
			redirect := evs.next(t).Redirect
			if !strings.HasSuffix(redirect, test.encoded) {
				t.Fatalf("redirect %q does not end in %q", redirect, test.encoded)
			}
			callback := redirect + "?code=synthetic&state=synthetic%2Bstate"
			response, err := http.Get(callback)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("callback returned %d", response.StatusCode)
			}
			if report := evs.next(t); report.Callback != callback || report.Error != "" {
				t.Fatalf("callback report %+v", report)
			}
		})
	}
}

func TestLoopbackFailsWithoutABrowser(t *testing.T) {
	var s Sessions
	evs := make(events, 4)
	s.Handle(3, driver.AuthRequest{Command: driver.AuthStart, Path: "/"}, nil, "", evs.dispatch)
	evs.next(t)
	s.Handle(3, driver.AuthRequest{Command: driver.AuthOpen, URL: "https://provider.example/"}, func(_ string, answer func(bool)) { answer(false) }, "", evs.dispatch)
	if ev := evs.next(t); ev.Phase != driver.AuthEnded || ev.Error != driver.AuthFailed {
		t.Fatalf("end %+v", ev)
	}
	if s.Has(3) {
		t.Fatal("the session stayed")
	}
}
