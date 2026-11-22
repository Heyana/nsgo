package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
)

func TestWebWasmCompressedCopies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.wasm")
	wasm := append([]byte("\x00asm\x01\x00\x00\x00"), bytes.Repeat([]byte{0}, 4096)...)
	if err := os.WriteFile(path, wasm, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeWebWasmCopies(path, wasm); err != nil {
		t.Fatal(err)
	}
	handler := webFiles(dir)
	for _, tc := range []struct{ accept, want string }{
		{"br, gzip", "br"},
		{"br;q=0.5, gzip", "gzip"},
		{"BR;q=0, gzip;q=0, *;q=1", ""},
		{"br;q=0.5, identity", ""},
		{"deflate", ""},
		{"", ""},
	} {
		t.Run(tc.accept, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/app.wasm", nil)
			req.Header.Set("Accept-Encoding", tc.accept)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != tc.want ||
				rec.Header().Get("Content-Type") != "application/wasm" || rec.Header().Get("Vary") != "Accept-Encoding" {
				t.Fatalf("request yielded %d %v", rec.Code, rec.Header())
			}
			if n := rec.Header().Get("Content-Length"); n != strconv.Itoa(rec.Body.Len()) {
				t.Errorf("Content-Length = %q with %d bytes sent", n, rec.Body.Len())
			}
			var body io.Reader = rec.Body
			switch tc.want {
			case "br":
				body = brotli.NewReader(body)
			case "gzip":
				gz, err := gzip.NewReader(body)
				if err != nil {
					t.Fatal(err)
				}
				defer gz.Close()
				body = gz
			}
			if got, err := io.ReadAll(body); err != nil || !bytes.Equal(got, wasm) {
				t.Fatalf("read %d bytes, want original %d-byte wasm: %v", len(got), len(wasm), err)
			}
		})
	}
}

func TestWebWasmLoadProgress(t *testing.T) {
	if _, err := findBrowser(); err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	b, err := startBrowser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()

	// A valid wasm module with one large custom section. The stub Go host
	// still receives a real WebAssembly instance from the loader.
	wasm := []byte("\x00asm\x01\x00\x00\x00\x00")
	wasm = binary.AppendUvarint(wasm, 32768)
	wasm = append(wasm, make([]byte, 32768)...)

	for _, tc := range []struct {
		name             string
		config           string
		gzip             bool
		midway, complete string
	}{
		{
			name:     "default bar",
			config:   `{wasm:"app.wasm"}`,
			midway:   `(() => { const p = document.querySelector("progress"); return p && p.value > 0 && p.value < p.max })()`,
			complete: `window.ran && !document.querySelector("progress")`,
		},
		{
			name:     "custom host with gzip",
			config:   fmt.Sprintf(`{wasm:"app.wasm", wasmSize:%d, host:"host.js"}`, len(wasm)),
			gzip:     true,
			midway:   fmt.Sprintf(`!document.querySelector("progress") && window.progress.some(p => p.loaded > 0 && p.loaded < p.total && p.total === %d)`, len(wasm)),
			complete: fmt.Sprintf(`window.ran && window.progress.at(-2).loaded === %d && window.progress.at(-1) === null`, len(wasm)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := make(chan struct{})
			finish := sync.OnceFunc(func() { close(release) })
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/":
					w.Header().Set("Content-Type", "text/html")
					fmt.Fprint(w, `<script type="module" src="main.js"></script>`)
				case "/main.js":
					w.Header().Set("Content-Type", "text/javascript")
					fmt.Fprintf(w, `import {mount} from "./nsgo.js";
globalThis.Go = class { argv = []; importObject = {}; run(instance) { window.ran = instance instanceof WebAssembly.Instance; } };
window.progress = [];
mount(null, null, {config:%s}).catch(e => { window.failure = e.message; });`, tc.config)
				case "/host.js":
					w.Header().Set("Content-Type", "text/javascript")
					fmt.Fprint(w, `export function onProgress(value) { window.progress.push(value); }`)
				case "/nsgo.js":
					w.Header().Set("Content-Type", "text/javascript")
					w.Write(loaderJS)
				case "/app.wasm":
					w.Header().Set("Content-Type", "application/wasm")
					controller := http.NewResponseController(w)
					var body io.Writer = w
					var gzipFlush func() error
					if tc.gzip {
						w.Header().Set("Content-Encoding", "gzip")
						gz := gzip.NewWriter(w)
						defer gz.Close()
						body = gz
						gzipFlush = gz.Flush
					} else {
						w.Header().Set("Content-Length", strconv.Itoa(len(wasm)))
					}
					if _, err := body.Write(wasm[:8192]); err != nil {
						return
					}
					if gzipFlush != nil && gzipFlush() != nil {
						return
					}
					if err := controller.Flush(); err != nil {
						return
					}
					select {
					case <-release:
						body.Write(wasm[8192:])
					case <-r.Context().Done():
					}
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			defer finish()
			raw, err := b.call("", "Target.createTarget", map[string]any{"url": "about:blank"})
			if err != nil {
				t.Fatal(err)
			}
			var target struct{ TargetID string }
			if err := json.Unmarshal(raw, &target); err != nil {
				t.Fatal(err)
			}
			defer b.call("", "Target.closeTarget", map[string]any{"targetId": target.TargetID})
			raw, err = b.call("", "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true})
			if err != nil {
				t.Fatal(err)
			}
			var attached struct{ SessionID string }
			if err := json.Unmarshal(raw, &attached); err != nil {
				t.Fatal(err)
			}
			page := &webPage{b: b, session: attached.SessionID}
			// Navigate once attached: Page.navigate returns when the page commits, so a cold browser's slow first load does not eat into the polling deadlines below.
			if _, err := page.call("Page.navigate", map[string]any{"url": srv.URL}); err != nil {
				t.Fatal(err)
			}
			wait := func(expression string) {
				t.Helper()
				for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
					raw, err := page.call("Runtime.evaluate", map[string]any{
						"expression":    "window.failure || Boolean(" + expression + ")",
						"returnByValue": true,
					})
					if err != nil {
						t.Fatal(err)
					}
					var response struct {
						Result struct{ Value any }
					}
					if err := json.Unmarshal(raw, &response); err != nil {
						t.Fatal(err)
					}
					switch value := response.Result.Value.(type) {
					case bool:
						if value {
							return
						}
					case string:
						t.Fatal(value)
					}
				}
				t.Fatalf("the page did not reach %s", expression)
			}
			wait("!window.ran && (" + tc.midway + ")")
			finish()
			wait(tc.complete)
		})
	}
}

func TestWebFilesServesAddressRoutesAndDeploymentBase(t *testing.T) {
	dir := t.TempDir()
	for file, data := range map[string]string{"index.html": "app entry", "nsgo.json": `{"base":"/my-app/"}`, "page.css": "body{}"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	handler := webFiles(dir)
	for _, c := range []struct {
		path, accept, method string
		status               int
		body                 string
	}{
		{"/my-app/films/42", "text/html", "GET", 200, "app entry"},
		{"/my-app/page.css", "text/css", "GET", 200, "body{}"},
		{"/my-app/missing.js", "*/*", "GET", 404, ""},
		{"/my-app/films/42", "application/json", "GET", 404, ""},
		{"/my-app/films/42", "text/html", "POST", 404, ""},
	} {
		req := httptest.NewRequest(c.method, c.path, nil)
		req.Header.Set("Accept", c.accept)
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		if out.Code != c.status || c.body != "" && out.Body.String() != c.body {
			t.Errorf("%s %s=>%d %q", c.method, c.path, out.Code, out.Body.String())
		}
	}
}
