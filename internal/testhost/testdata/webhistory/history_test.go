package webhistory_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

type historyAccess struct{}

func (historyAccess) Allows(core.AccessRequest) bool { return true }
func (historyAccess) WebHosts() []string             { return []string{"127.0.0.1"} }
func (historyAccess) WebSchemes() []string           { return nil }

type historyPage struct {
	mu       sync.Mutex
	scope    *core.Scope
	ref      core.Ref
	loads    []core.WebNavigation
	errors   []error
	messages map[string]bool
}

func startHistory(t *testing.T, source core.WebSource) (*nsgotest.App, *historyPage) {
	t.Helper()
	switch nsgotest.Target() {
	case "macos", "ios", "ios_simulator", "android", "linux", "windows":
	default:
		t.Skip("checks native web-view page bodies, history and navigation policy")
	}
	p := &historyPage{messages: map[string]bool{}}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		p.scope = cx
		cx.ProvideAccess(historyAccess{})
		return core.WebView{
			Ref: &p.ref, Source: source, TestID: "history",
			Style:          core.Style{Width: core.Pt(260), Height: core.Pt(180)},
			MessageOrigins: []string{"*"},
			OnLoad: func(nav core.WebNavigation) {
				p.mu.Lock()
				defer p.mu.Unlock()
				p.loads = append(p.loads, nav)
			},
			OnError: func(err error) {
				p.mu.Lock()
				defer p.mu.Unlock()
				p.errors = append(p.errors, err)
			},
			OnMessage: func(message string) {
				p.mu.Lock()
				defer p.mu.Unlock()
				p.messages[message] = true
			},
		}
	}, nsgotest.Size(280, 200))
	return app, p
}

func (p *historyPage) evaluate(t *testing.T, app *nsgotest.App, script string) string {
	t.Helper()
	var result string
	var evalErr error
	done := false
	p.scope.Post(func() {
		p.ref.Evaluate(script, func(value string, err error) {
			p.mu.Lock()
			defer p.mu.Unlock()
			result, evalErr, done = value, err, true
		})
	})
	app.WaitUntil(10*time.Second, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return done
	})
	p.mu.Lock()
	defer p.mu.Unlock()
	if evalErr != nil {
		t.Fatalf("evaluate %s: %v", script, evalErr)
	}
	return result
}

func (p *historyPage) body(t *testing.T, app *nsgotest.App, want string) {
	t.Helper()
	if got := p.evaluate(t, app, "document.body.textContent.trim()"); got != strconv.Quote(want) {
		t.Fatalf("page body = %s, want %q", got, want)
	}
}

func (p *historyPage) load(t *testing.T, app *nsgotest.App, count int, body string) core.WebNavigation {
	t.Helper()
	app.WaitUntil(15*time.Second, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.loads) >= count || len(p.errors) > 0
	})
	p.mu.Lock()
	errors := append([]error(nil), p.errors...)
	var nav core.WebNavigation
	if len(p.loads) >= count {
		nav = p.loads[count-1]
	}
	p.mu.Unlock()
	if len(errors) != 0 {
		t.Fatalf("load %d: %v", count, errors)
	}
	p.body(t, app, body)
	t.Logf("load %d body %q, back=%v forward=%v", count, body, nav.CanGoBack, nav.CanGoForward)
	return nav
}

func (p *historyPage) refused(t *testing.T, app *nsgotest.App, target string) {
	t.Helper()
	app.WaitUntil(5*time.Second, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.errors) != 0
	})
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, err := range p.errors {
		web, ok := err.(*core.WebError)
		if !ok || web.URL != target || !strings.Contains(err.Error(), "may not navigate") {
			t.Fatalf("navigation error = %v, want refusal of %s", err, target)
		}
	}
}

func ownedHistory(t *testing.T, source core.WebSource) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<!doctype html><body>next</body>")
	}))
	defer server.Close()
	app, page := startHistory(t, source)
	page.load(t, app, 1, "owned")
	page.scope.Post(page.ref.Reload)
	page.load(t, app, 2, "owned")
	// Chromium's native history commands skip entries that navigate without
	// user activation. A stationary native drag touches the web view itself.
	app.Drag("history")
	page.evaluate(t, app, fmt.Sprintf("location.href=%q; true", server.URL+"/next"))
	if nav := page.load(t, app, 3, "next"); !nav.CanGoBack {
		t.Fatal("the next page lost the app-owned history entry")
	}
	page.scope.Post(page.ref.GoBack)
	if nav := page.load(t, app, 4, "owned"); !nav.CanGoForward {
		t.Fatal("the app-owned history entry lost its forward page")
	}
	page.scope.Post(page.ref.Reload)
	if nav := page.load(t, app, 5, "owned"); !nav.CanGoForward {
		t.Fatal("reloading a previous entry discarded forward history")
	}
	page.scope.Post(page.ref.GoForward)
	page.load(t, app, 6, "next")
	page.scope.Post(page.ref.GoBack)
	page.load(t, app, 7, "owned")

	// Chromium/WebKit may themselves reject a top-level data URL before the
	// navigation delegate sees it. Either way it must not replace the document.
	page.evaluate(t, app, "location.href='data:text/html,<body>refused</body>'; true")
	deadline := time.Now().Add(400 * time.Millisecond)
	app.WaitUntil(time.Second, func() bool { return time.Now().After(deadline) })
	page.body(t, app, "owned")
	page.mu.Lock()
	if len(page.loads) > 7 {
		t.Errorf("a page-created data URL loaded a document: %+v", page.loads[7:])
	}
	page.errors = nil
	page.mu.Unlock()

	// A frame's own reload must not give the parent page an app-load grant.
	page.evaluate(t, app, `(() => {
const frame = document.createElement('iframe');
let reloaded = false;
frame.onload = () => {
  if (!reloaded) { reloaded = true; frame.contentWindow.location.reload(); return; }
  window.nsgo.postMessage('iframe-reloaded');
  location.href = 'about:blank';
};
frame.src = 'about:blank';
document.body.append(frame);
return true;
})()`)
	app.WaitUntil(5*time.Second, func() bool {
		page.mu.Lock()
		defer page.mu.Unlock()
		return page.messages["iframe-reloaded"]
	})
	page.refused(t, app, "about:blank")
	page.body(t, app, "owned")
}

func TestRestrictedHTMLHistory(t *testing.T) {
	ownedHistory(t, core.WebHTML("<!doctype html><body>owned</body>", ""))
}

func TestRestrictedDataHistory(t *testing.T) {
	ownedHistory(t, core.WebSrc("data:text/html,"+url.PathEscape("<!doctype html><body>owned</body>")))
}

func TestSameDocumentHistoryGrantsNoNavigation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<!doctype html><body>owned</body>")
	}))
	defer server.Close()
	app, page := startHistory(t, core.WebSrc(server.URL+"/owned"))
	page.load(t, app, 1, "owned")
	app.Drag("history")
	page.evaluate(t, app, `addEventListener('popstate', () => { location.href='about:blank' }); history.pushState(null, '', '#step'); true`)
	page.scope.Post(page.ref.GoBack)
	page.refused(t, app, "about:blank")
	page.body(t, app, "owned")
}
