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

func TestWebKitGTKReportsSameDocumentNavigation(t *testing.T) {
	if nsgotest.Target() != "linux" {
		t.Skip("checks WebKitGTK navigation notifications")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<!doctype html><body>navigation</body>`)
	}))
	defer server.Close()
	app, page := startPage(t, core.WebSrc(server.URL+"/"))
	app.WaitUntil(10*time.Second, func() bool {
		page.mu.Lock()
		defer page.mu.Unlock()
		return slices.ContainsFunc(page.events, func(e webEvent) bool { return e.kind == "load" })
	})
	app.Settle()
	page.mu.Lock()
	initial := page.events
	page.events = nil
	page.mu.Unlock()
	var states []bool
	for _, event := range initial {
		if event.kind == "navigation" {
			states = append(states, event.nav.Loading)
		}
	}
	if !slices.Equal(states, []bool{true, false}) {
		t.Fatalf("the full load reported navigation states %v, want [true false]: %+v", states, initial)
	}

	for _, step := range []struct {
		script, path, title string
		back, forward       bool
	}{
		{`history.pushState({}, '', '/pushed')`, "/pushed", "", true, false},
		{`history.replaceState({}, '', '/replaced')`, "/replaced", "", true, false},
		{`location.hash = 'part'`, "/replaced#part", "", true, false},
		{`history.back()`, "/replaced", "", true, true},
		{`history.back()`, "/", "", false, true},
		{`history.forward()`, "/replaced", "", true, true},
		{`document.title = 'Updated'`, "/replaced", "Updated", true, true},
	} {
		t.Log(step.script)
		page.scope.Post(func() { page.ref.InjectJavaScript(step.script) })
		want := core.WebNavigation{URL: server.URL + step.path, Title: step.title, CanGoBack: step.back, CanGoForward: step.forward}
		app.WaitUntil(5*time.Second, func() bool {
			page.mu.Lock()
			defer page.mu.Unlock()
			return len(page.errors) != 0 || slices.ContainsFunc(page.events, func(e webEvent) bool {
				return e.kind == "navigation" && e.nav == want
			})
		})
		page.mu.Lock()
		events, failures := slices.Clone(page.events), slices.Clone(page.errors)
		page.events = nil
		page.mu.Unlock()
		if len(failures) != 0 {
			t.Fatalf("%s failed: %v", step.script, failures)
		}
		for _, event := range events {
			if event.kind != "navigation" || event.nav.Loading {
				t.Fatalf("%s reported a full load: %+v", step.script, events)
			}
		}
	}
}
