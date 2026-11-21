package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestWebOfflineRelease exercises packaged apps with HTTP caching disabled.
// A live old page can still load its assets while a new release installs.
func TestWebOfflineRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two web releases")
	}
	if _, err := findBrowser(); err != nil {
		t.Skip(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	sum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	appDir := t.TempDir()
	writeFiles(t, appDir, map[string]string{
		"go.mod":    "module example.com/offline\n\ngo 1.27.3\n\nrequire github.com/nsgo-dev/nsgo v0.0.0\n\nreplace github.com/nsgo-dev/nsgo => " + root + "\n",
		"go.sum":    string(sum),
		"nsgo.yaml": "name: Example\nid: com.example.offline\nweb:\n  offline: true\n",
		"main.go": `package main

import (
	_ "embed"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

//go:embed payload.bin
var payload []byte

func main() {
	nsgo.Run(func(*core.Scope) core.Node {
		return core.Text{Content: "Release " + core.App().Version + string(payload[:1]) }
	})
}
`,
	})
	payload, err := os.Create(filepath.Join(appDir, "payload.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := payload.WriteString("!"); err != nil {
		payload.Close()
		t.Fatal(err)
	}
	if err := payload.Truncate(20 << 20); err != nil {
		payload.Close()
		t.Fatal(err)
	}
	if err := payload.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOWORK", "off")
	p, err := load(appDir)
	if err != nil {
		t.Fatal(err)
	}
	p.Release = true
	build := func(version, asset string) (string, webApp) {
		t.Helper()
		p.Version = version
		writeFiles(t, appDir, map[string]string{"resources/sound clip.bin": asset})
		dir := t.TempDir()
		if err := buildWeb(t.Context(), p, dir, false); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "nsgo.json"))
		if err != nil {
			t.Fatal(err)
		}
		var app webApp
		if err := json.Unmarshal(data, &app); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(dir, app.Wasm))
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() < 30<<20 {
			t.Fatalf("test module is only %d bytes", info.Size())
		}
		return dir, app
	}
	oldDir, oldApp := build("1.0.0", "old-generation")
	newDir, newApp := build("2.0.0", "new-generation")
	if oldApp.Wasm == newApp.Wasm {
		t.Fatal("both releases use the same module")
	}
	p.Web.Offline = false
	plainDir, plainApp := build("2.0.0", "new-generation")
	if plainApp.Wasm != newApp.Wasm || plainApp.Loader != newApp.Loader {
		t.Fatal("enabling offline delivery changed the app module or loader")
	}
	if _, err := os.Stat(filepath.Join(plainDir, offlineWorkerName)); !os.IsNotExist(err) {
		t.Fatalf("a default build has a service worker: %v", err)
	}
	plainPage, err := os.ReadFile(filepath.Join(plainDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plainPage), `href="manifest.webmanifest"`) {
		t.Fatal("a default build's page lost its existing manifest")
	}

	var deployed atomic.Bool
	var oldDownloads atomic.Int64
	oldFiles := http.StripPrefix("/game/", http.FileServer(http.Dir(oldDir)))
	newFiles := http.StripPrefix("/game/", http.FileServer(http.Dir(newDir)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", strictPolicy)
		if r.URL.Path == "/observer" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<!doctype html><title>Observer</title>")
			return
		}
		if deployed.Load() {
			newFiles.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/game/"+oldApp.Wasm {
			oldDownloads.Add(1)
		}
		oldFiles.ServeHTTP(w, r)
	}))
	defer server.Close()
	browser, err := startBrowser(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer browser.close()

	open := func(path string) (*webPage, string) {
		t.Helper()
		raw, err := browser.call("", "Target.createTarget", map[string]any{"url": "about:blank"})
		if err != nil {
			t.Fatal(err)
		}
		var target struct{ TargetID string }
		if err := json.Unmarshal(raw, &target); err != nil {
			t.Fatal(err)
		}
		raw, err = browser.call("", "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true})
		if err != nil {
			t.Fatal(err)
		}
		var attached struct{ SessionID string }
		if err := json.Unmarshal(raw, &attached); err != nil {
			t.Fatal(err)
		}
		pg := &webPage{b: browser, session: attached.SessionID}
		if _, err := pg.call("Page.navigate", map[string]any{"url": server.URL + path}); err != nil {
			t.Fatal(err)
		}
		return pg, target.TargetID
	}
	closeTab := func(id string) {
		t.Helper()
		if _, err := browser.call("", "Target.closeTarget", map[string]any{"targetId": id}); err != nil {
			t.Fatal(err)
		}
	}
	eval := func(pg *webPage, expression string) any {
		t.Helper()
		raw, err := pg.call("Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true})
		if err != nil {
			t.Fatal(err)
		}
		var result struct{ Result struct{ Value any } }
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		return result.Result.Value
	}
	wait := func(pg *webPage, expression string, want any) {
		t.Helper()
		deadline := time.Now().Add(25 * time.Second)
		for {
			if got := eval(pg, expression); got == want {
				return
			} else if time.Now().After(deadline) {
				t.Fatalf("%s = %v, want %v", expression, got, want)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	cacheNames := `caches.keys().then(names => names.filter(name => name.startsWith("nsgo-offline:")).join(","))`
	assetURL := func(app webApp) string {
		return (&url.URL{Path: "/game/" + app.Resources["sound clip.bin"]}).EscapedPath()
	}
	readAsset := func(app webApp) string {
		return fmt.Sprintf("fetch(%q).then(r => r.text())", assetURL(app))
	}
	observer, _ := open("/observer")
	old, oldID := open("/game/")
	wait(old, `document.body?.innerText`, "Release 1.0.0!")
	wait(old, `Boolean(navigator.serviceWorker.controller)`, true)
	initialCache, _ := eval(observer, cacheNames).(string)
	if initialCache == "" || strings.Contains(initialCache, ",") {
		t.Fatalf("expected one installed offline cache, got %q", initialCache)
	}
	downloads := oldDownloads.Load()
	returning, returningID := open("/game/index.html?second=1")
	wait(returning, `document.body?.innerText`, "Release 1.0.0!")
	if got := oldDownloads.Load(); got != downloads {
		t.Fatalf("second visit downloaded the module again (%d -> %d)", downloads, got)
	}
	closeTab(returningID)

	deployed.Store(true)
	current, currentID := open("/game/?update=1")
	wait(current, `document.body?.innerText`, "Release 2.0.0!")
	wait(current, readAsset(newApp), "new-generation")
	wait(old, readAsset(oldApp), "old-generation")
	wait(observer, `navigator.serviceWorker.getRegistration("/game/").then(r => r?.waiting?.state)`, "installed")
	closeTab(oldID)
	closeTab(currentID)
	wait(observer, `navigator.serviceWorker.getRegistration("/game/").then(r => r?.active?.state === "activated" && !r?.waiting)`, true)
	newCache, _ := eval(observer, cacheNames).(string)
	if newCache == "" || newCache == initialCache || strings.Contains(newCache, ",") {
		t.Fatalf("new cache = %q, old cache = %q", newCache, initialCache)
	}

	server.Close()
	offline, _ := open("/game/index.html?offline=1")
	wait(offline, `document.body?.innerText`, "Release 2.0.0!")
	wait(offline, readAsset(newApp), "new-generation")
	rangeRead := fmt.Sprintf(`fetch(%q, { headers: { Range: "bytes=4-7" } }).then(async r => r.status + " " + await r.text())`, assetURL(newApp))
	wait(offline, rangeRead, "206 gene")
}
