package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestBrowserLaunchArgs(t *testing.T) {
	const clone = "--disable-features=MacAppCodeSignClone"
	for _, goos := range []string{"darwin", "linux", "windows"} {
		args := browserArgs(goos, "test profile")
		if got := slices.Contains(args, clone); got != (goos == "darwin") {
			t.Errorf("%s arguments include %s: %v", goos, clone, got)
		}
	}
}

func TestBrowserStartupReportsExitAndCleansItsProfile(t *testing.T) {
	fakeBrowser(t, "echo 'browser startup rejected' >&2\nexit 23\n")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := startBrowser(ctx)
	if err == nil || !strings.Contains(err.Error(), "exit status 23") || !strings.Contains(err.Error(), "browser startup rejected") {
		t.Fatalf("startup error = %v, want the exit status and browser stderr", err)
	}
	checkBrowserProfileRemoved(t)
}

func TestBrowserStartupHonorsCancellation(t *testing.T) {
	fakeBrowser(t, "exec sleep 60\n")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	var err error
	go func() {
		_, err = startBrowser(ctx)
		close(done)
	}()
	t.Cleanup(func() { cancel(); <-done })
	// Cancel after the child has started and recorded its profile. Under load, a fixed deadline can expire before its shell ever runs.
	ready := time.NewTicker(10 * time.Millisecond)
	defer ready.Stop()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	waiting := true
	for waiting {
		select {
		case <-done:
			t.Fatalf("browser exited before cancellation: %v", err)
		case <-timeout.C:
			t.Fatal("fake browser did not record its profile")
		case <-ready.C:
			profile, readErr := os.ReadFile(os.Getenv("NSGO_TEST_BROWSER_PROFILE"))
			waiting = readErr != nil || len(profile) == 0
		}
	}
	start := time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("canceled browser did not stop within 3s")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("startup error = %v, want context cancellation", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("canceled browser took %s to stop", elapsed)
	}
	checkBrowserProfileRemoved(t)
}

func TestBrowserStartupWaitsForTheCompletePortFile(t *testing.T) {
	srv := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		for {
			var m cdpMessage
			if err := websocket.JSON.Receive(ws, &m); err != nil {
				return
			}
			if m.Method == "Browser.close" {
				profile, _ := os.ReadFile(os.Getenv("NSGO_TEST_BROWSER_PROFILE"))
				os.WriteFile(filepath.Join(string(profile), "closed"), nil, 0o600)
				return
			}
			if err := websocket.JSON.Send(ws, map[string]any{"id": m.ID, "result": map[string]any{"product": "fake"}}); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("NSGO_TEST_BROWSER_PORT", fakeBrowserPort(t, srv))
	// A browser may create the port file before writing its second line.
	fakeBrowser(t, `profile=$(cat "$NSGO_TEST_BROWSER_PROFILE")
printf '%s\n' "$NSGO_TEST_BROWSER_PORT" > "$profile/DevToolsActivePort"
sleep 0.3
printf '%s\n/devtools/browser/test\n' "$NSGO_TEST_BROWSER_PORT" > "$profile/DevToolsActivePort"
while [ ! -e "$profile/closed" ]; do sleep 0.05; done
`)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	b, err := startBrowser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := b.call("", "Browser.getVersion", nil)
	if err != nil || !strings.Contains(string(result), "fake") {
		t.Fatalf("Browser.getVersion = %s, %v", result, err)
	}
	start := time.Now()
	cancel()
	b.close()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("closing took %s; the browser was killed rather than closed", elapsed)
	}
	if b.browser.ProcessState == nil {
		t.Error("the browser was not waited for")
	}
	checkBrowserProfileRemoved(t)
}

func TestBrowserStartupCancelsTheHandshake(t *testing.T) {
	connected := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(connected) })
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	t.Setenv("NSGO_TEST_BROWSER_PORT", fakeBrowserPort(t, srv))
	terminated := filepath.Join(t.TempDir(), "terminated")
	t.Setenv("NSGO_TEST_BROWSER_TERMINATED", terminated)
	fakeBrowser(t, `profile=$(cat "$NSGO_TEST_BROWSER_PROFILE")
trap 'touch "$NSGO_TEST_BROWSER_TERMINATED"; exit 0' TERM
printf '%s\n/devtools/browser/test\n' "$NSGO_TEST_BROWSER_PORT" > "$profile/DevToolsActivePort"
while :; do sleep 0.05; done
`)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := startBrowser(ctx); result <- err }()
	select {
	case <-connected:
	case err := <-result:
		t.Fatalf("startup ended before its handshake: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the browser handshake did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("startup error = %v, want context cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the browser handshake kept waiting after cancellation")
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if _, err := os.Stat(terminated); err != nil {
			t.Fatalf("the browser did not receive SIGTERM: %v", err)
		}
	}
	checkBrowserProfileRemoved(t)
}

// fakeBrowserPort returns the port srv listens on.
func fakeBrowserPort(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func fakeBrowser(t *testing.T, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake browser uses a POSIX shell")
	}
	dir := t.TempDir()
	t.Setenv("NSGO_TEST_BROWSER_PROFILE", filepath.Join(dir, "profile"))
	script := "#!/bin/sh\nfor arg do\ncase $arg in\n--user-data-dir=*) printf '%s' \"${arg#*=}\" > \"$NSGO_TEST_BROWSER_PROFILE\";;\nesac\ndone\n" + body
	path := filepath.Join(dir, "browser")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSGO_BROWSER", path)
}

func checkBrowserProfileRemoved(t *testing.T) {
	t.Helper()
	profile, err := os.ReadFile(os.Getenv("NSGO_TEST_BROWSER_PROFILE"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(string(profile)); !os.IsNotExist(err) {
		t.Fatalf("browser profile still exists: %v", err)
	}
}
