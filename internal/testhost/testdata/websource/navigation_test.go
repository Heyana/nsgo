package websource_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestAppleWebViewTracksSameDocumentNavigation(t *testing.T) {
	switch nsgotest.Target() {
	case "macos", "ios", "ios_simulator":
	default:
		t.Skip("checks WKWebView navigation state")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<!doctype html><body>page</body>`)
	}))
	defer server.Close()
	app, page := startPage(t, core.WebSrc(server.URL+"/"))
	wait := func(matched func(webEvent) bool) {
		t.Helper()
		app.WaitUntil(5*time.Second, func() bool {
			page.mu.Lock()
			defer page.mu.Unlock()
			return len(page.errors) != 0 || slices.ContainsFunc(page.events, matched)
		})
		page.mu.Lock()
		defer page.mu.Unlock()
		if len(page.errors) != 0 {
			t.Fatalf("page errors: %v", page.errors)
		}
	}
	wait(func(e webEvent) bool { return e.kind == "load" })
	app.Settle()
	page.mu.Lock()
	initial := slices.Clone(page.events)
	page.events = nil
	page.mu.Unlock()
	var states []bool
	for _, e := range initial {
		if e.kind == "navigation" {
			states = append(states, e.nav.Loading)
		}
	}
	if !slices.Equal(states, []bool{true, false}) {
		t.Fatalf("the full load reported navigation states %v, want [true false]: %+v", states, initial)
	}

	for _, step := range []struct {
		script, path, title string
		back, forward       bool
	}{
		{`document.title = 'Initial'`, "/", "Initial", false, false},
		{`history.pushState({}, '', '/pushed')`, "/pushed", "Initial", true, false},
		{`history.replaceState({}, '', '/replaced')`, "/replaced", "Initial", true, false},
		{`location.hash = 'part'`, "/replaced#part", "Initial", true, false},
		{`history.back()`, "/replaced", "Initial", true, true},
		{`history.back()`, "/", "Initial", false, true},
		{`history.forward()`, "/replaced", "Initial", true, true},
		{`document.title = 'New title'`, "/replaced", "New title", true, true},
	} {
		t.Log(step.script)
		page.scope.Post(func() { page.ref.InjectJavaScript(step.script) })
		want := core.WebNavigation{URL: server.URL + step.path, Title: step.title, CanGoBack: step.back, CanGoForward: step.forward}
		wait(func(e webEvent) bool { return e.kind == "navigation" && e.nav == want })
		page.mu.Lock()
		events := slices.Clone(page.events)
		page.events = nil
		page.mu.Unlock()
		for _, e := range events {
			if e.kind != "navigation" || e.nav.Loading {
				t.Fatalf("%s started a full load: %+v", step.script, events)
			}
		}
	}
}
