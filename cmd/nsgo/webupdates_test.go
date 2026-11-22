package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Use two actual published apps, not a fake registration: accepting the
// updater's alert must reload the page and start the newly deployed module.
func TestPublishedWebUpdateReloadsNewRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two published web apps")
	}
	if _, err := findBrowser(); err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	sum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":    "module example.com/webupdate\n\ngo 1.27.3\n\nrequire github.com/nsgo-dev/nsgo v0.0.0\nreplace github.com/nsgo-dev/nsgo => " + root + "\n",
		"go.sum":    string(sum),
		"nsgo.yaml": "name: WebUpdate\nid: dev.nsgo.WebUpdate1423\nweb:\n  offline: true\n",
		"main.go": `package main
import (
 "github.com/nsgo-dev/nsgo"
 "github.com/nsgo-dev/nsgo/base"
 "github.com/nsgo-dev/nsgo/core"
 "github.com/nsgo-dev/nsgo/updater"
)
func main(){ nsgo.Run(func(cx *core.Scope) core.Node {
 return core.View{Children:[]core.Node{
  core.Text{Content:"Release "+core.App().Version},
  base.Button{OnPress:func(core.PressEvent){updater.Check(cx)},Children:[]core.Node{core.Text{Content:"Check"}}},
 }}
}) }
`,
	})
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOWORK", "off")
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	p.Release = true
	build := func(version string) string {
		t.Helper()
		p.Version = version
		out := t.TempDir()
		if err := buildWeb(ctx, p, out, false); err != nil {
			t.Fatal(err)
		}
		return out
	}
	old, newer := build("1.0.0"), build("2.0.0")
	var deployed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", strictPolicy)
		files := old
		if deployed.Load() {
			files = newer
		}
		http.FileServer(http.Dir(files)).ServeHTTP(w, r)
	}))
	defer server.Close()
	browser, err := startBrowser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer browser.close()
	page, closePage, err := openWebPage(browser, webPageOptions{url: server.URL + "/", width: 640, height: 480, out: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer closePage()
	eval := func(expression string) any {
		t.Helper()
		raw, err := page.call("Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true})
		if err != nil {
			return nil
		}
		var result struct{ Result struct{ Value any } }
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		return result.Result.Value
	}
	wait := func(expression string) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for eval(expression) != true {
			if time.Now().After(deadline) {
				t.Fatalf("browser did not satisfy %s; text=%v", expression, eval("document.body.innerText"))
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	wait(`document.body.innerText.includes("Release 1.0.0")`)
	wait(`Boolean(navigator.serviceWorker.controller)`)
	deployed.Store(true)
	eval(`[...document.querySelectorAll("button,[role=button]")].find(b=>b.textContent==="Check").click()`)
	wait(`document.body.innerText.includes("WebUpdate 2.0.0 is available")`)
	eval(`[...document.querySelectorAll("button,[role=button]")].find(b=>b.textContent==="Later").click()`)
	if eval(`document.body.innerText.includes("Release 1.0.0")`) != true {
		t.Fatal("Later replaced the running release")
	}
	eval(`[...document.querySelectorAll("button,[role=button]")].find(b=>b.textContent==="Check").click()`)
	wait(`document.body.innerText.includes("WebUpdate 2.0.0 is available")`)
	eval(`[...document.querySelectorAll("button,[role=button]")].find(b=>b.textContent==="Update").click()`)
	wait(`document.body.innerText.includes("Release 2.0.0")`)
	eval(`[...document.querySelectorAll("button,[role=button]")].find(b=>b.textContent==="Check").click()`)
	wait(`document.body.innerText.includes("You’re up to date")`)
}
