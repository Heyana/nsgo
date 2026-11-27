//go:build js && wasm

package web

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestEmptyWebSourceIsOpaque(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-empty-websource")
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")
	var source *core.Signal[core.WebSource]
	var ref core.Ref
	var scope *core.Scope
	var starts, loads int
	var loaded core.WebNavigation
	s := open(func(cx *core.Scope) core.Node {
		scope, source = cx, core.NewSignal(cx, core.WebSource{})
		return core.WebView{SourceFunc: source.Get, Ref: &ref,
			Style:       core.Style{Width: core.Pt(100), Height: core.Pt(100)},
			OnLoadStart: func(core.WebNavigation) { starts++ },
			OnLoad:      func(nav core.WebNavigation) { loads++; loaded = nav }}
	}, Options{Mount: "#nsgo-empty-websource"}, nil)
	defer s.engine.Close()
	wait := func(want int) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); starts < want || loads < want; time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%d starts and %d loads, want %d", starts, loads, want)
			}
		}
		if starts < want || loads < want {
			t.Fatalf("%d starts and %d loads, want at least %d of each", starts, loads, want)
		}
	}
	blank := func() {
		t.Helper()
		frame := mount.Call("querySelector", "iframe")
		if loaded.URL != "about:blank" || loaded.Loading || frame.Call("hasAttribute", "srcdoc").Bool() ||
			frame.Call("getAttribute", "src").String() != "about:blank" {
			t.Fatalf("empty source did not load a blank document: %+v", loaded)
		}
		if strings.Contains(frame.Call("getAttribute", "sandbox").String(), "allow-same-origin") || !frame.Get("contentDocument").IsNull() {
			t.Fatal("the blank frame can reach the app's origin")
		}
	}
	wait(1)
	blank()
	scope.Post(func() { source.Set(core.WebHTML("<p>private</p>", "")) })
	wait(2)
	scope.Post(func() { source.Set(core.WebSource{}) })
	wait(3)
	blank()
	scope.Post(func() { ref.Load(core.WebSource{}) })
	wait(4)
	blank()
	// An about: page the app names itself takes the app's origin just the same unless the sandbox keeps it opaque.
	scope.Post(func() { ref.Load(core.WebSrc("about:blank?probe")) })
	wait(5)
	if frame := mount.Call("querySelector", "iframe"); strings.Contains(frame.Call("getAttribute", "sandbox").String(), "allow-same-origin") || !frame.Get("contentDocument").IsNull() {
		t.Fatal("an about: frame can reach the app's origin")
	}
}

// TestWebViewDropsTheBridgeOfAPageThatLeft loads HTML whose page goes to another, which has no bridge, and comes back as the app reloads it. An Evaluate made before the HTML's page handed its port waits for it; one the page had not answered as it left ends with an error, as does one made while the other page shows; and the HTML shown again answers again. The other page reaches no OnMessage. Run in a browser: nsgo test -target web.
func TestWebViewDropsTheBridgeOfAPageThatLeft(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-webview-test")
	setStyle(mount, map[string]string{"width": "200px", "height": "200px"})
	doc.Get("body").Call("appendChild", mount)
	ref := new(core.Ref)
	scope := make(chan *core.Scope, 1)
	var loads, starts int
	var answers, messages []string
	open(func(cx *core.Scope) core.Node {
		scope <- cx
		return core.WebView{Ref: ref, Source: core.WebHTML(`<script>window.nsgo.postMessage("html");</script>`, ""),
			MessageOrigins: []string{"*"},
			OnLoadStart:    func(core.WebNavigation) { starts++ },
			OnLoad:         func(core.WebNavigation) { loads++ },
			OnMessage:      func(m string) { messages = append(messages, m) },
			Style:          core.Style{Width: core.Pt(100), Height: core.Pt(100)}}
	}, Options{Mount: "#nsgo-webview-test"}, nil)
	cx := <-scope
	wait := func(what string, cond func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s: no answer after 5s (answers %q, %d loads)", what, answers, loads)
			}
		}
	}
	evaluate := func(script string) {
		cx.Post(func() {
			ref.Evaluate(script, func(json string, err error) {
				if err != nil {
					json = "error"
				}
				answers = append(answers, json)
			})
		})
	}
	answered := func(what, want string) {
		t.Helper()
		wait(what, func() bool { return len(answers) > 0 })
		if answers[0] != want {
			t.Fatalf("%s answered %q, want %q", what, answers[0], want)
		}
		answers = answers[1:]
	}

	// A load start ends what waits, so the Evaluate follows the HTML's.
	wait("the HTML's load start", func() bool { return starts == 1 })
	evaluate("1 + 1")
	answered("an Evaluate as the page loads", "2")
	wait("the HTML's load", func() bool { return loads == 1 })

	evaluate("new Promise(() => {})")
	time.Sleep(50 * time.Millisecond)
	cx.Post(func() {
		ref.InjectJavaScript(`location.href = "data:text/html,<script>window.nsgo && window.nsgo.postMessage('other');parent.postMessage('other', '*');</script>"`)
	})
	answered("an Evaluate the page left", "error")
	wait("the other page's load", func() bool { return loads == 2 })
	evaluate("3")
	answered("an Evaluate on the other page", "error")
	if starts != 2 {
		t.Errorf("%d load starts, want the source's and the page leaving", starts)
	}

	cx.Post(func() { ref.Reload() })
	wait("the HTML's load again", func() bool { return loads == 3 })
	evaluate("4")
	answered("an Evaluate on the HTML again", "4")
	if len(messages) != 2 || messages[0] != "html" || messages[1] != "html" {
		t.Errorf("OnMessage heard %q, want the HTML's page's own twice", messages)
	}
}

type restrictedWebTestAccess struct {
	hosts   []string
	schemes []string
}

func (a restrictedWebTestAccess) Allows(req core.AccessRequest) bool {
	if req.Permission != core.AccessWebView {
		return false
	}
	u, err := url.Parse(req.Resource)
	if err != nil {
		return false
	}
	return driver.AllowsHost(a.hosts, u.Hostname())
}

func (a restrictedWebTestAccess) WebHosts() []string   { return a.hosts }
func (a restrictedWebTestAccess) WebSchemes() []string { return a.schemes }

// TestRestrictedWebViewNavigationGrants verifies in a real browser that a
// sandboxed web view on the web refuses ungranted links, forms, meta
// refreshes, popups, top-level escapes and script navigations, allows
// navigations to granted hosts, enforces grants on data:text/html and
// NoJavaScript sources, and refuses ungranted Ref.Load targets.
func TestRestrictedWebViewNavigationGrants(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	allowedBase := os.Getenv("NSGO_BASE")
	if allowedBase == "" {
		allowedBase = js.Global().Get("location").Get("href").String()
	}
	allowedParsed, err := url.Parse(allowedBase)
	if err != nil || allowedParsed.Hostname() == "" {
		t.Fatalf("invalid NSGO_BASE %q: %v", allowedBase, err)
	}
	allowedHost := allowedParsed.Hostname()
	// Load a lightweight static file from the test runner's own server when verifying an allowed navigation.
	allowedDest := allowedParsed.Scheme + "://" + allowedParsed.Host + "/wasm_exec.js"
	topBefore := js.Global().Get("location").Get("href").String()

	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-webview-restricted-test")
	setStyle(mount, map[string]string{"width": "200px", "height": "200px"})
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")

	var mu sync.Mutex
	var loads int
	var webErrors []*core.WebError
	var openWindows []core.WebWindowRequest
	var answers []string

	ref := new(core.Ref)
	scope := make(chan *core.Scope, 1)
	var source *core.Signal[core.WebSource]
	var noJS *core.Signal[bool]

	initialHTML := `<!doctype html><html><head><title>Guarded</title>` +
		`<meta http-equiv="refresh" content="0;url=https://evil.invalid/meta">` +
		`</head><body data-stay="yes">` +
		`<a id="bad-link" href="https://evil.invalid/link">bad</a>` +
		`<a id="top-link" href="https://sub.allowed.invalid/escape" target="_top">top</a>` +
		`<a id="bad-blank" href="https://evil.invalid/blank" target="_blank">badblank</a>` +
		`<a id="ok-blank" href="https://sub.allowed.invalid/blank" target="_blank">okblank</a>` +
		`<a id="ok-link" href="` + allowedDest + `">ok</a>` +
		`<form id="bad-form" action="https://evil.invalid/form"><button id="bad-submit" type="submit">go</button></form>` +
		`</body></html>`

	s := open(func(cx *core.Scope) core.Node {
		cx.ProvideAccess(restrictedWebTestAccess{
			hosts:   []string{allowedHost, "*.allowed.invalid"},
			schemes: []string{"about"},
		})
		source = core.NewSignal(cx, core.WebHTML(initialHTML, ""))
		noJS = core.NewSignal(cx, false)
		scope <- cx
		return core.Dynamic(func(cx *core.Scope) core.Node {
			return core.WebView{
				Ref:          ref,
				SourceFunc:   source.Get,
				NoJavaScript: noJS.Get(),
				Style:        core.Style{Width: core.Pt(100), Height: core.Pt(100)},
				OnLoad: func(core.WebNavigation) {
					mu.Lock()
					loads++
					mu.Unlock()
				},
				OnError: func(err error) {
					var we *core.WebError
					if errors.As(err, &we) {
						mu.Lock()
						webErrors = append(webErrors, we)
						mu.Unlock()
					}
				},
				OnOpenWindow: func(req core.WebWindowRequest) {
					mu.Lock()
					openWindows = append(openWindows, req)
					mu.Unlock()
				},
			}
		})
	}, Options{Mount: "#nsgo-webview-restricted-test"}, nil)
	defer s.engine.Close()
	cx := <-scope

	wait := func(what string, cond func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				mu.Lock()
				defer mu.Unlock()
				t.Fatalf("%s: timed out (loads=%d, errors=%+v, openWindows=%+v, answers=%q)", what, loads, webErrors, openWindows, answers)
			}
		}
	}
	hasError := func(wantURL string) bool {
		mu.Lock()
		defer mu.Unlock()
		for _, we := range webErrors {
			if we.URL == wantURL && strings.Contains(we.Message, wantURL) {
				return true
			}
		}
		return false
	}
	eval := func(script string) string {
		t.Helper()
		mu.Lock()
		prev := len(answers)
		mu.Unlock()
		cx.Post(func() {
			ref.Evaluate(script, func(json string, err error) {
				if err != nil {
					json = "error:" + err.Error()
				}
				mu.Lock()
				answers = append(answers, json)
				mu.Unlock()
			})
		})
		wait("evaluate "+script, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return len(answers) > prev
		})
		mu.Lock()
		defer mu.Unlock()
		return answers[len(answers)-1]
	}

	// 1. Initial HTML loads and reports the stripped disallowed meta refresh via OnError.
	wait("initial load", func() bool { mu.Lock(); defer mu.Unlock(); return loads >= 1 })
	wait("refused meta refresh error", func() bool { return hasError("https://evil.invalid/meta") })

	// Sandbox attribute must omit popups, downloads, top-navigation and same-origin.
	frame := mount.Call("querySelector", "iframe")
	sb := frame.Call("getAttribute", "sandbox").String()
	for _, forbid := range []string{"allow-popups", "allow-popups-to-escape-sandbox", "allow-downloads", "allow-top-navigation", "allow-same-origin"} {
		if strings.Contains(sb, forbid) {
			t.Fatalf("restricted iframe sandbox %q unexpectedly grants %q", sb, forbid)
		}
	}
	if got := eval(`document.body.dataset.stay`); got != `"yes"` {
		t.Fatalf("initial document not active: %s", got)
	}

	// 2. Disallowed link click, target=_top escape, target=_blank popup, window.open and form submit
	//    must be blocked with the exact refused URL without leaving the page.
	blockedActions := []struct {
		name    string
		script  string
		wantURL string
	}{
		{"disallowed link", `document.getElementById("bad-link").click()`, "https://evil.invalid/link"},
		{"target=_top escape", `document.getElementById("top-link").click()`, "https://sub.allowed.invalid/escape"},
		{"disallowed target=_blank", `document.getElementById("bad-blank").click()`, "https://evil.invalid/blank"},
		{"disallowed window.open", `window.open("https://evil.invalid/open") === null`, "https://evil.invalid/open"},
		{"disallowed form submit button", `document.getElementById("bad-submit").click()`, "https://evil.invalid/form"},
		{"disallowed form.submit()", `document.getElementById("bad-form").submit()`, "https://evil.invalid/form"},
	}
	for _, tc := range blockedActions {
		cx.Post(func() { ref.InjectJavaScript(tc.script) })
		wait(tc.name+" error", func() bool { return hasError(tc.wantURL) })
		if got := eval(`document.body.dataset.stay`); got != `"yes"` {
			t.Fatalf("page left after %s: %s", tc.name, got)
		}
	}
	// Unverified script navigations (location.href, assign, replace, data:) from opaque srcdoc
	// are caught and aborted via beforeunload/pagehide, reporting OnError and restoring the page.
	scriptNavs := []struct {
		name   string
		script string
	}{
		{"disallowed location.href", `location.href = "https://evil.invalid/loc"`},
		{"disallowed location.assign", `location.assign("https://evil.invalid/assign")`},
		{"disallowed location.replace", `location.replace("https://evil.invalid/replace")`},
		{"disallowed data scheme", `location.href = "data:text/html,<p>nope</p>"`},
	}
	for _, tc := range scriptNavs {
		mu.Lock()
		prevErrs := len(webErrors)
		mu.Unlock()
		cx.Post(func() { ref.InjectJavaScript(tc.script) })
		wait(tc.name+" error", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return len(webErrors) > prevErrs
		})
		if got := eval(`document.body.dataset.stay`); got != `"yes"` {
			t.Fatalf("page not restored after %s: %s", tc.name, got)
		}
	}
	if loads != 1 {
		t.Fatalf("blocked navigations triggered extra loads: loads=%d", loads)
	}

	// 3. Allowed target=_blank and window.open route to OnOpenWindow without leaving the page.
	cx.Post(func() {
		ref.InjectJavaScript(`document.getElementById("ok-blank").click(); window.open("https://sub.allowed.invalid/win2");`)
	})
	wait("allowed OnOpenWindow", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(openWindows) >= 2
	})
	if got := eval(`document.body.dataset.stay`); got != `"yes"` {
		t.Fatalf("page left after allowed open-window requests: %s", got)
	}

	// 4. Programmatic Ref.Load to an ungranted host is refused and reports OnError without navigating.
	cx.Post(func() { ref.Load(core.WebSrc("https://evil.invalid/direct")) })
	wait("refused Ref.Load", func() bool { return hasError("https://evil.invalid/direct") })
	if got := eval(`document.body.dataset.stay`); got != `"yes"` {
		t.Fatalf("page left after refused Ref.Load: %s", got)
	}

	// 5. Navigating to a granted host via link succeeds cleanly and reports the destination URL.
	cx.Post(func() { ref.InjectJavaScript(`document.getElementById("ok-link").click()`) })
	wait("allowed navigation load", func() bool { mu.Lock(); defer mu.Unlock(); return loads >= 2 })
	if hasError(allowedDest) {
		t.Fatalf("allowed navigation to %q reported an error", allowedDest)
	}

	// 6. Restricted data:text/html source is decoded through the guarded srcdoc pipeline and blocks ungranted redirects without looping.
	mu.Lock()
	prevErrs := len(webErrors)
	mu.Unlock()
	cx.Post(func() {
		ref.Load(core.WebSrc(`data:text/html,<script>location.href="https://evil.invalid/from-data"</script><p>data</p>`))
	})
	wait("refused navigation from data:text/html", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(webErrors) > prevErrs
	})

	// 7. Restricted NoJavaScript HTML strips disallowed links and meta refreshes and omits allow-scripts.
	cx.Post(func() {
		noJS.Set(true)
		source.Set(core.WebHTML(`<meta http-equiv="refresh" content="0;url=https://evil.invalid/nojs-meta"><a href="https://evil.invalid/nojs-link">bad</a><a href="`+allowedDest+`">ok</a>`, ""))
	})
	wait("refused NoJavaScript meta refresh", func() bool { return hasError("https://evil.invalid/nojs-meta") })
	frame = mount.Call("querySelector", "iframe")
	sbNoJS := frame.Call("getAttribute", "sandbox").String()
	if strings.Contains(sbNoJS, "allow-scripts") {
		t.Fatalf("NoJavaScript iframe sandbox %q unexpectedly grants allow-scripts", sbNoJS)
	}
	srcdoc := frame.Get("srcdoc").String()
	if strings.Contains(srcdoc, "https://evil.invalid/nojs-link") || strings.Contains(srcdoc, "https://evil.invalid/nojs-meta") {
		t.Fatalf("NoJavaScript restricted srcdoc retained disallowed targets: %s", srcdoc)
	}
	if !strings.Contains(srcdoc, allowedDest) {
		t.Fatalf("NoJavaScript restricted srcdoc dropped allowed link %q", allowedDest)
	}

	// Host page must never have been navigated.
	if got := js.Global().Get("location").Get("href").String(); got != topBefore {
		t.Fatalf("host top-level location changed from %q to %q", topBefore, got)
	}
}
