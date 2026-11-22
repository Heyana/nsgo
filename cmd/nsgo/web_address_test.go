package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWebAddressBase(t *testing.T) {
	for _, c := range []struct{ raw, want string }{{"", ""}, {"/", "/"}, {"/app", "/app/"}, {"/app/", "/app/"}} {
		p := &project{}
		p.Web.Base = c.raw
		got, err := p.webBase()
		if err != nil || got != c.want {
			t.Errorf("base%q=%q,%v", c.raw, got, err)
		}
	}
	for _, bad := range []string{"relative/", "https://evil.example/", "//evil.example/", "/x?query", "/x?", "/x#hash", "/x#", "/bad path", "/\\evil/"} {
		p := &project{}
		p.Web.Base = bad
		if _, err := p.webBase(); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	var out bytes.Buffer
	if err := indexHTML.Execute(&out, map[string]any{"Name": "Address", "Base": "/app/"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `<base href="/app/">`) {
		t.Fatal(out.String())
	}
}
func TestWebAppAddressSurvivesReloadAndSharedLink(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a browser app")
	}
	if _, err := findBrowser(); err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	p, err := load("../../internal/testhost/testdata/address")
	if err != nil {
		t.Fatal(err)
	}
	p.Release = true
	dir := t.TempDir()
	if err := buildWeb(ctx, p, dir, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "outside.html"), []byte("<!doctype html><p>Outside the app</p>"), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(webFiles(dir))
	defer server.Close()
	b, err := startBrowser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	open := func() *webPage {
		data, err := b.call("", "Target.createTarget", map[string]any{"url": "about:blank"})
		if err != nil {
			t.Fatal(err)
		}
		var target struct{ TargetID string }
		json.Unmarshal(data, &target)
		data, err = b.call("", "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true})
		if err != nil {
			t.Fatal(err)
		}
		var attached struct{ SessionID string }
		json.Unmarshal(data, &attached)
		return &webPage{b: b, session: attached.SessionID}
	}
	evaluate := func(pg *webPage, code string) any {
		data, err := pg.call("Runtime.evaluate", map[string]any{"expression": code, "returnByValue": true})
		if err != nil {
			t.Fatal(err)
		}
		var result struct{ Result struct{ Value any } }
		json.Unmarshal(data, &result)
		return result.Result.Value
	}
	wait := func(pg *webPage, code string) {
		t.Helper()
		for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(30 * time.Millisecond) {
			if evaluate(pg, code) == true {
				return
			}
		}
		t.Fatalf("browser never satisfied%s;body=%v", code, evaluate(pg, "document.body.innerText"))
	}
	navigate := func(pg *webPage, path string) {
		t.Helper()
		if _, err := pg.call("Page.navigate", map[string]any{"url": server.URL + path}); err != nil {
			t.Fatal(err)
		}
	}
	shown := func(text string) string { return fmt.Sprintf("document.body.innerText.includes(%q)", text) }
	pg := open()
	navigate(pg, "/outside.html")
	wait(pg, shown("Outside the app"))
	navigate(pg, "/library/start?from=shared")
	wait(pg, shown("initial="+server.URL+"/library/start?from=shared"))
	at, ok := evaluate(pg, `(()=>{const r=document.querySelector('[data-testid="push"]').getBoundingClientRect();return [r.x+r.width/2,r.y+r.height/2]})()`).([]any)
	if !ok || len(at) != 2 {
		t.Fatal("no push control")
	}
	for _, kind := range []string{"mousePressed", "mouseReleased"} {
		if err := pg.mouse(kind, at[0].(float64), at[1].(float64), "left", 1, 1); err != nil {
			t.Fatal(err)
		}
	}
	wait(pg, shown("current="+server.URL+"/films/42?list=two"))
	if _, err := pg.call("Page.reload", map[string]any{"ignoreCache": true}); err != nil {
		t.Fatal(err)
	}
	wait(pg, shown("initial="+server.URL+"/films/42?list=two"))
	shared := open()
	navigate(shared, "/albums/84?shared=1")
	wait(shared, shown("initial="+server.URL+"/albums/84?shared=1"))
	evaluate(pg, "history.back()")
	wait(pg, shown("Outside the app"))
}
