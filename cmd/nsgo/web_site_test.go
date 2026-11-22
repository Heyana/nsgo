package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublishedSiteLoadsFromACDNAndOpensRealLinks(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a published web app")
	}
	if _, err := findBrowser(); err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	p, err := load("../../internal/testhost/testdata/site")
	if err != nil {
		t.Fatal(err)
	}
	p.Release = true
	dir := t.TempDir()
	if err := buildWeb(ctx, p, dir, false); err != nil {
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
	var resources atomic.Int32
	files := webFiles(dir)
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if strings.HasSuffix(r.URL.Path, ".svg") {
			resources.Add(1)
		}
		files.ServeHTTP(w, r)
	}))
	defer cdn.Close()
	page := fmt.Sprintf(`<!doctype html><title>Original</title><script type="module">import {mount} from %q; mount(null, %q);</script>`, cdn.URL+"/"+app.Loader, cdn.URL+"/nsgo.json")
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/article" {
			fmt.Fprint(w, "<!doctype html><title>Article target</title>")
			return
		}
		fmt.Fprint(w, page)
	}))
	defer site.Close()
	b, err := startBrowser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	raw, err := b.call("", "Target.createTarget", map[string]any{"url": "about:blank"})
	if err != nil {
		t.Fatal(err)
	}
	var target struct{ TargetID string }
	if err := json.Unmarshal(raw, &target); err != nil {
		t.Fatal(err)
	}
	raw, err = b.call("", "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true})
	if err != nil {
		t.Fatal(err)
	}
	var attached struct{ SessionID string }
	if err := json.Unmarshal(raw, &attached); err != nil {
		t.Fatal(err)
	}
	pg := &webPage{b: b, session: attached.SessionID}
	eval := func(expression string) any {
		t.Helper()
		raw, err := pg.call("Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true})
		if err != nil {
			t.Fatal(err)
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
				t.Fatalf("page did not reach %s; body=%v", expression, eval("document.body.innerText"))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if _, err := pg.call("Page.navigate", map[string]any{"url": site.URL + "/"}); err != nil {
		t.Fatal(err)
	}
	wait(`document.title === "Site" && document.querySelector('meta[name="description"]')?.content === "Site summary" && !!document.querySelector('a[data-testid="article"]')`)
	if got := eval(`document.querySelector('[data-testid="heading"]').tagName`); got != "H1" {
		t.Fatalf("heading tag=%v", got)
	}
	if got := eval(`getComputedStyle(document.querySelector('a')).cursor`); got == "pointer" {
		t.Fatal("link changed the app's pointer")
	}
	deadline := time.Now().Add(10 * time.Second)
	for resources.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if resources.Load() == 0 {
		t.Fatal("asset did not load from the CDN origin")
	}
	point := eval(`(() => { const r=document.querySelector('a').getBoundingClientRect(); return [r.x+r.width/2,r.y+r.height/2]; })()`).([]any)
	x, y := point[0].(float64), point[1].(float64)
	if err := pg.mouse("mousePressed", x, y, "middle", 4, 1); err != nil {
		t.Fatal(err)
	}
	if err := pg.mouse("mouseReleased", x, y, "middle", 0, 1); err != nil {
		t.Fatal(err)
	}
	opened := false
	deadline = time.Now().Add(10 * time.Second)
	for !opened && time.Now().Before(deadline) {
		raw, err := b.call("", "Target.getTargets", nil)
		if err != nil {
			t.Fatal(err)
		}
		var targets struct {
			TargetInfos []struct{ TargetID, URL string }
		}
		if err := json.Unmarshal(raw, &targets); err != nil {
			t.Fatal(err)
		}
		for _, target := range targets.TargetInfos {
			opened = opened || target.URL == site.URL+"/article"
		}
		if !opened {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !opened || eval("document.title") != "Site" {
		t.Fatal("middle click did not open a separate page without pressing the app's link")
	}
	if err := pg.mouse("mousePressed", x, y, "left", 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := pg.mouse("mouseReleased", x, y, "left", 0, 1); err != nil {
		t.Fatal(err)
	}
	wait(`document.title === "Article" && document.querySelector('meta[name="description"]').content === "Article summary"`)
	if eval("location.href") != site.URL+"/" {
		t.Fatal("ordinary click navigated outside the app")
	}
	eval(`document.querySelector('[data-testid="back"]').click()`)
	wait(`document.title === "Site" && document.querySelector('meta[name="description"]').content === "Site summary"`)
}
