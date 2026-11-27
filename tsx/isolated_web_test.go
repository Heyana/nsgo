//go:build js

package tsx_test

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	sjs "syscall/js"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/bundler"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

func sandboxedIframeCount() int {
	return sjs.Global().Get("document").Call("querySelectorAll", `iframe[sandbox="allow-scripts"]`).Length()
}

// TestWebIsolationOpaqueOriginAndSecurityBoundaries verifies that tsx.View and
// tsx.Load run a sandboxed package on the web inside a real opaque-origin
// <iframe sandbox="allow-scripts"> with CSP lockdown: the package cannot reach
// the host page's window, DOM, localStorage, cookies, clipboard, XMLHttpRequest
// or WebSocket, while Go-backed capability-gated APIs work over the bridge and
// the iframe is removed cleanly on disposal.
func TestWebIsolationOpaqueOriginAndSecurityBoundaries(t *testing.T) {
	// Seed a host secret in the page's localStorage and window global to prove
	// the isolated package cannot read either.
	const secretKey = "__nsgo_web_isolation_test_secret__"
	ls := sjs.Global().Get("localStorage")
	ls.Call("setItem", secretKey, "host-secret-value")
	sjs.Global().Set("__nsgoHostGlobalSecret", "top-secret")
	t.Cleanup(func() {
		ls.Call("removeItem", secretKey)
		sjs.Global().Delete("__nsgoHostGlobalSecret")
	})

	beforeFrames := sandboxedIframeCount()

	const probeSource = `import { mount, signal, Text, Clipboard, Storage, host } from "nsgo";

function probeSecurity() {
	const r = {};
	r.origin = String(window.origin);
	try {
		r.parentDoc = String(window.parent.document.title);
	} catch (e) {
		r.parentDoc = "blocked:" + e.name;
	}
	try {
		r.parentSecret = String(window.parent.__nsgoHostGlobalSecret);
	} catch (e) {
		r.parentSecret = "blocked:" + e.name;
	}
	r.frameElement = window.frameElement === null ? "null" : "exposed";
	try {
		r.localStorage = String(window.localStorage.getItem("__nsgo_web_isolation_test_secret__"));
	} catch (e) {
		r.localStorage = "blocked:" + e.name;
	}
	try {
		r.sessionStorage = String(window.sessionStorage.length);
	} catch (e) {
		r.sessionStorage = "blocked:" + e.name;
	}
	try {
		r.cookie = String(document.cookie);
	} catch (e) {
		r.cookie = "blocked:" + e.name;
	}
	r.xhr = typeof XMLHttpRequest;
	r.ws = typeof WebSocket;
	r.eventSource = typeof EventSource;
	r.worker = typeof Worker;
	r.broadcast = typeof BroadcastChannel;
	r.indexedDB = typeof indexedDB;
	r.caches = typeof caches;
	try {
		new Function("return 1")();
		r.evalBlocked = "no";
	} catch (e) {
		r.evalBlocked = "blocked:" + e.name;
	}
	return r;
}

export async function audit() {
	const r = probeSecurity();
	try {
		r.browserClipboard = String(await navigator.clipboard.readText());
	} catch (e) {
		r.browserClipboard = "blocked:" + (e && e.name ? e.name : String(e));
	}
	try {
		r.allowedFetch = await (await fetch("https://api.example.com/ok")).text();
	} catch (e) {
		r.allowedFetch = "err:" + (e.code || e.message);
	}
	try {
		await fetch("https://evil.example.com/");
		r.deniedFetch = "unexpected-ok";
	} catch (e) {
		r.deniedFetch = (e.code || "") + ":" + (e.permission || "");
	}
	try {
		await Clipboard.readText();
		r.deniedClipboard = "unexpected-ok";
	} catch (e) {
		r.deniedClipboard = (e.code || "") + ":" + (e.permission || "");
	}
	try {
		r.hostGreet = await host.call("greet", "isolated");
	} catch (e) {
		r.hostGreet = "err:" + (e.code || e.message);
	}
	await Storage.setItem("k", "v1");
	r.storageVal = await Storage.getItem("k");
	return r;
}

const clicks = signal(0);
const viewAudit = signal("");
audit().then((res) => viewAudit.set(JSON.stringify(res)));
mount(() => (
	<Text testID="isolated-view" onPress={() => clicks.set(clicks() + 1)}>
		{() => clicks() + "|" + viewAudit()}
	</Text>
));
`

	caps := pkg.Capabilities{
		Network: &pkg.Network{Hosts: []string{"api.example.com"}},
		Host:    []string{"greet"},
	}
	p := verified(t, probeSource, caps)

	checkAudit := func(t *testing.T, label string, m map[string]any) {
		t.Helper()
		if m["origin"] != "null" {
			t.Errorf("%s: window.origin = %v, want opaque origin \"null\"", label, m["origin"])
		}
		if got, _ := m["parentDoc"].(string); !strings.HasPrefix(got, "blocked:") {
			t.Errorf("%s: window.parent.document was not blocked: %v", label, got)
		}
		if got, _ := m["parentSecret"].(string); !strings.HasPrefix(got, "blocked:") {
			t.Errorf("%s: window.parent secret was not blocked: %v", label, got)
		}
		if m["frameElement"] != "null" {
			t.Errorf("%s: window.frameElement = %v, want null", label, m["frameElement"])
		}
		if got, _ := m["localStorage"].(string); !strings.HasPrefix(got, "blocked:") {
			t.Errorf("%s: localStorage was not blocked: %v", label, got)
		}
		if got, _ := m["sessionStorage"].(string); !strings.HasPrefix(got, "blocked:") {
			t.Errorf("%s: sessionStorage was not blocked: %v", label, got)
		}
		if got, _ := m["cookie"].(string); !strings.HasPrefix(got, "blocked:") {
			t.Errorf("%s: document.cookie was not blocked: %v", label, got)
		}
		for _, k := range []string{"xhr", "ws", "eventSource", "worker", "broadcast", "indexedDB", "caches"} {
			if m[k] != "undefined" {
				t.Errorf("%s: typeof %s = %v, want undefined", label, k, m[k])
			}
		}
		if got, _ := m["evalBlocked"].(string); !strings.HasPrefix(got, "blocked:") {
			t.Errorf("%s: new Function was not blocked: %v", label, got)
		}
		if got, _ := m["browserClipboard"].(string); !strings.HasPrefix(got, "blocked:") {
			t.Errorf("%s: navigator.clipboard was not blocked: %v", label, got)
		}
		if m["allowedFetch"] != "https://api.example.com/ok" {
			t.Errorf("%s: allowedFetch = %v, want https://api.example.com/ok", label, m["allowedFetch"])
		}
		if m["deniedFetch"] != "ERR_ACCESS_DENIED:network" {
			t.Errorf("%s: deniedFetch = %v, want ERR_ACCESS_DENIED:network", label, m["deniedFetch"])
		}
		if m["deniedClipboard"] != "ERR_ACCESS_DENIED:clipboard" {
			t.Errorf("%s: deniedClipboard = %v, want ERR_ACCESS_DENIED:clipboard", label, m["deniedClipboard"])
		}
		if m["hostGreet"] != "hi isolated" {
			t.Errorf("%s: hostGreet = %v, want hi isolated", label, m["hostGreet"])
		}
		if m["storageVal"] != "v1" {
			t.Errorf("%s: storageVal = %v, want v1", label, m["storageVal"])
		}
	}

	fetcher := func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(req.URL.String())),
			Request:    req,
		}, nil
	}

	// 1. Test isolated tsx.View: rendering, signal updates on press, capabilities, and iframe disposal.
	t.Run("View", func(t *testing.T) {
		var shown *core.Signal[bool]
		app := nsgotest.Start(t, tsx.WithFetcher(func(cx *core.Scope) core.Node {
			shown = core.NewSignal(cx, true)
			return core.Show{
				When: shown.Get,
				Then: func(cx *core.Scope) core.Node {
					return tsx.View{Package: p, Sandbox: tsx.Sandbox{Host: greeter()}}
				},
			}
		}, fetcher))

		if got := sandboxedIframeCount(); got != beforeFrames+1 {
			t.Fatalf("sandboxed iframe count while mounted = %d, want %d", got, beforeFrames+1)
		}
		app.WaitUntil(10*time.Second, func() bool {
			return strings.Contains(app.Find("id:isolated-view").Text, `"storageVal":"v1"`)
		})
		txt := app.Find("id:isolated-view").Text
		prefix, rawJSON, ok := strings.Cut(txt, "|")
		if !ok || prefix != "0" {
			t.Fatalf("initial view text = %q", txt)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(rawJSON), &m); err != nil {
			t.Fatalf("unmarshal audit JSON %q: %v", rawJSON, err)
		}
		checkAudit(t, "View", m)

		app.Press("id:isolated-view")
		if !strings.HasPrefix(app.Find("id:isolated-view").Text, "1|") {
			t.Fatalf("after press, view text = %q, want prefix 1|", app.Find("id:isolated-view").Text)
		}

		// Dispose the view and verify the opaque-origin iframe is removed from the DOM.
		app.Post(func() { shown.Set(false) })
		app.Settle()
		if got := sandboxedIframeCount(); got != beforeFrames {
			t.Fatalf("sandboxed iframe count after view disposal = %d, want %d", got, beforeFrames)
		}
	})

	// 2. Test isolated tsx.Load: export listing, async Plugin.Call, capabilities, and iframe cleanup on Close.
	t.Run("Load", func(t *testing.T) {
		var plugin *tsx.Plugin
		var loadErr error
		app := nsgotest.Start(t, tsx.WithFetcher(func(cx *core.Scope) core.Node {
			plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{Host: greeter()})
			return core.View{}
		}, fetcher))
		if loadErr != nil {
			t.Fatalf("tsx.Load isolated failed: %v", loadErr)
		}
		if got := sandboxedIframeCount(); got != beforeFrames+1 {
			t.Fatalf("sandboxed iframe count while plugin loaded = %d, want %d", got, beforeFrames+1)
		}
		var result any
		var callErr error
		done := false
		app.Post(func() {
			plugin.Call("audit", nil, func(v any, err error) {
				result, callErr, done = v, err, true
			})
		})
		app.WaitUntil(10*time.Second, func() bool { return done })
		if callErr != nil {
			t.Fatalf("plugin.Call(audit) failed: %v", callErr)
		}
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("audit result = %#v, want map[string]any", result)
		}
		checkAudit(t, "Load", m)

		app.Post(plugin.Close)
		app.Settle()
		if got := sandboxedIframeCount(); got != beforeFrames {
			t.Fatalf("sandboxed iframe count after plugin.Close = %d, want %d", got, beforeFrames)
		}
	})
}

// TestWebUnisolatedOptInStillUsesPageOrigin verifies that Sandbox.WebUnisolated
// continues to run a package in the page's origin when explicitly requested.
func TestWebUnisolatedOptInStillUsesPageOrigin(t *testing.T) {
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(`export function origin() { return String(window.origin); }`)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	root, _ := pkg.GenerateKey()
	release, _ := pkg.GenerateKey()
	cert, err := pkg.Certify(root, release.Public().(ed25519.PublicKey), []string{"com.example.*"}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	data, err := pkg.Build(pkg.Contents{App: pkg.App{ID: "com.example.unisolated", Build: 1}, Source: code, Capabilities: pkg.Capabilities{}}, release, cert)
	if err != nil {
		t.Fatal(err)
	}
	trust := pkg.NewTrust()
	trust.AddRoot(root.Public().(ed25519.PublicKey))
	p, err := trust.Verify(data)
	if err != nil {
		t.Fatal(err)
	}

	var plugin *tsx.Plugin
	var loadErr error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{WebUnisolated: true})
		return core.View{}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	var got any
	var callErr error
	done := false
	app.Post(func() {
		plugin.Call("origin", nil, func(v any, err error) {
			got, callErr, done = v, err, true
		})
	})
	app.WaitUntil(5*time.Second, func() bool { return done })
	if callErr != nil {
		t.Fatal(callErr)
	}
	pageOrigin := sjs.Global().Get("window").Get("origin").String()
	if got != pageOrigin {
		t.Fatalf("WebUnisolated origin = %v, want page origin %q", got, pageOrigin)
	}
	app.Post(plugin.Close)
	app.Settle()
	var afterErr error
	app.Post(func() {
		plugin.Call("origin", nil, func(_ any, err error) { afterErr = err })
	})
	app.Settle()
	if !errors.Is(afterErr, tsx.ErrPluginClosed) {
		t.Fatalf("call after close = %v, want ErrPluginClosed", afterErr)
	}
}
