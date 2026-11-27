// Package imageloadtest checks a backend's imageload.HTTP against a
// local server, so each backend's tests hold its system client to the
// same behavior: status, headers both ways, the body, progress and
// cancelling.
package imageloadtest

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

// CheckHTTP runs get's checks.
func CheckHTTP(t *testing.T, get func(ctx context.Context, url string, header imageload.Header, progress imageload.Progress) (*imageload.Response, error)) {
	t.Helper()
	body := make([]byte, 256<<10)
	for i := range body {
		body[i] = byte(i)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow":
			<-r.Context().Done()
		case "/missing":
			http.NotFound(w, r)
		case "/big":
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.Write(body)
		default:
			w.Header().Set("ETag", `"v1"`)
			w.Header().Add("Cache-Control", "max-age=60")
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(r.Header.Get("If-None-Match") + " png"))
		}
	}))
	defer srv.Close()

	resp, err := get(context.Background(), srv.URL+"/a.png?x=1", imageload.Header{"If-None-Match": {`"v0"`}}, nil)
	if err != nil || resp.Status != 200 || string(resp.Body) != `"v0" png` || resp.Header.Get("Etag") != `"v1"` ||
		resp.Header.Get("cache-control") != "max-age=60" {
		t.Fatalf("get = %+v, %v", resp, err)
	}
	var loaded, total int64
	resp, err = get(context.Background(), srv.URL+"/big", imageload.Header{}, func(n, of int64) { loaded, total = n, of })
	if err != nil || resp.Status != 200 || len(resp.Body) != len(body) || resp.Body[len(body)-1] != body[len(body)-1] ||
		loaded != int64(len(body)) || total != int64(len(body)) {
		t.Fatalf("a big body = %d bytes, %v, progress %d of %d", len(resp.Body), err, loaded, total)
	}
	if resp, err := get(context.Background(), srv.URL+"/missing", imageload.Header{}, nil); err != nil || resp.Status != 404 {
		t.Fatalf("a missing image = %+v, %v", resp, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := get(ctx, srv.URL+"/slow", imageload.Header{}, nil); err != context.DeadlineExceeded || time.Since(start) > 5*time.Second {
		t.Fatalf("a cancelled download = %v after %v", err, time.Since(start))
	}
}

// CheckHTTPNoRedirect verifies that getNoRedirect returns a 3xx redirect
// response with its Location header without following it to the target.
func CheckHTTPNoRedirect(t *testing.T, getNoRedirect func(ctx context.Context, url string, header imageload.Header, progress imageload.Progress) (*imageload.Response, error)) {
	t.Helper()
	var targetHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/target", http.StatusFound)
		case "/target":
			targetHits.Add(1)
			w.Write([]byte("target"))
		}
	}))
	defer srv.Close()

	resp, err := getNoRedirect(context.Background(), srv.URL+"/redirect", imageload.Header{}, nil)
	if err != nil {
		t.Fatalf("getNoRedirect: %v", err)
	}
	if resp.Status != http.StatusFound || resp.Header.Get("Location") != "/target" {
		t.Fatalf("getNoRedirect = status %d location %q, want 302 /target", resp.Status, resp.Header.Get("Location"))
	}
	if targetHits.Load() != 0 {
		t.Fatalf("getNoRedirect followed redirect to /target (%d hits)", targetHits.Load())
	}
}

var (
	slowRun   atomic.Int64
	slowStart = time.Now().UnixNano()
)

// Slow has imageload answer every http(s) source with body after delay, as a slow network does, until the test ends, and returns n distinct addresses for it. Each call's addresses are new, also to a disk cache an earlier test process filled, so imageload's caches hold none of them yet.
func Slow(t *testing.T, n int, delay time.Duration, body []byte) []string {
	saved := imageload.HTTP
	t.Cleanup(func() { imageload.HTTP = saved })
	imageload.HTTP = func(ctx context.Context, url string, header imageload.Header, progress imageload.Progress) (*imageload.Response, error) {
		select {
		case <-time.After(delay):
			return &imageload.Response{Status: 200, Body: bytes.Clone(body)}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	run := slowRun.Add(1)
	uris := make([]string, n)
	for i := range uris {
		uris[i] = fmt.Sprintf("https://slow.invalid/%d-%d/%d.png", slowStart, run, i)
	}
	return uris
}
