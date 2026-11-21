package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestDevProfileFlags(t *testing.T) {
	flags := flag.NewFlagSet("dev", flag.ContinueOnError)
	dev := bindDevFlags(flags)
	if err := flags.Parse([]string{"-profile", "cpu"}); err != nil {
		t.Fatal(err)
	}
	p := &project{Dir: t.TempDir()}
	profile, err := dev.options().profile.forProject(p)
	if err != nil {
		t.Fatal(err)
	}
	if profile.kind != "cpu" || profile.duration != 10*time.Second || profile.output != p.cache("profiles", "cpu.pprof") {
		t.Errorf("wrong defaults: %+v", profile)
	}
	p.Dev.Target = "web"
	if _, err := dev.options().profile.forProject(p); err == nil {
		t.Fatal("web accepted a CPU profile")
	}
	heap, err := (devProfileOptions{kind: "heap", output: "browser.pprof"}).forProject(p)
	if err != nil || heap.output != filepath.Join(p.Dir, "browser.pprof") || heap.duration != 0 {
		t.Errorf("web heap profile = %+v, %v", heap, err)
	}
}

func TestDevProfileUpload(t *testing.T) {
	s, err := startDevServer(0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	output := filepath.Join(t.TempDir(), "heap.pprof")
	if err := os.WriteFile(output, []byte("old profile"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.profile = &devProfile{opts: devProfileOptions{kind: "heap", output: output}}
	s.mu.Unlock()
	request := func(method string, body io.Reader, authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/_nsgo/profile", body)
		r.Header.Set("Content-Type", "application/octet-stream")
		if authorized {
			r.Header.Set(tokenHeader, s.token)
		}
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, r)
		return w
	}
	if got := request(http.MethodGet, nil, false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET = %d", got.Code)
	}
	if got := request(http.MethodPost, strings.NewReader("secret"), false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST = %d", got.Code)
	}
	got := request(http.MethodGet, nil, true)
	var spec struct {
		Kind     string
		Duration time.Duration
	}
	if err := json.Unmarshal(got.Body.Bytes(), &spec); err != nil || got.Code != http.StatusOK || spec.Kind != "heap" || spec.Duration != 0 {
		t.Fatalf("profile request = %d, %+v, %v", got.Code, spec, err)
	}
	check := func(want string) {
		t.Helper()
		data, err := os.ReadFile(output)
		if err != nil || string(data) != want {
			t.Fatalf("saved profile = %q, %v, want %q", data, err, want)
		}
	}
	partial := io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(errors.New("connection closed")))
	if got := request(http.MethodPost, partial, true); got.Code == http.StatusNoContent {
		t.Fatal("an interrupted upload succeeded")
	}
	check("old profile")
	if got := request(http.MethodPost, strings.NewReader("completed profile"), true); got.Code != http.StatusNoContent {
		t.Fatalf("complete upload = %d, %s", got.Code, got.Body)
	}
	check("completed profile")
	if got := request(http.MethodGet, nil, true); got.Code != http.StatusNoContent {
		t.Fatalf("the server requested another profile after completion: %d", got.Code)
	}
	request(http.MethodPost, strings.NewReader("later capture"), true)
	check("completed profile")
	entries, err := os.ReadDir(filepath.Dir(output))
	if err != nil || len(entries) != 1 {
		t.Errorf("profile files = %v, %v", entries, err)
	}
}
