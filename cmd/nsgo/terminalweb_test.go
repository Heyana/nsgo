//go:build !js && !android && !ios

package main

import (
	"context"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	terminalview "github.com/nsgo-dev/nsgo/base/terminal"
)

// TestTerminalBrowserSession runs a real local shell in a PTY and transports
// its mirror over a real WebSocket to the browser's own libghostty-vt module.
func TestTerminalBrowserSession(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("runs /bin/sh and a browser")
	}
	if _, err := findBrowser(); err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	token := rand.Text()
	failures := make(chan error, 1)
	fail := func(err error) {
		select {
		case failures <- err:
		default:
		}
	}
	var received, sent atomic.Int64
	server := websocket.Server{
		Handshake: func(_ *websocket.Config, r *http.Request) error {
			origin, err := url.Parse(r.Header.Get("Origin"))
			if err != nil || origin.Hostname() != "127.0.0.1" || r.URL.Path != "/"+token {
				return fmt.Errorf("unexpected test socket origin or path")
			}
			return nil
		},
		Handler: func(conn *websocket.Conn) {
			defer conn.Close()
			term, err := terminalview.New(terminalview.Options{Command: []string{"/bin/sh", "-c", "printf '\\033]2;WebSocket remote\\007\\033[32mREMOTE_READY\\033[0m\\r\\n'; exec /bin/sh -i"}, Env: []string{"PS1=socket$ "}})
			if err != nil {
				fail(err)
				return
			}
			defer func() { term.Close(); <-term.Done() }()
			term.Resize(80, 24)
			mirror, err := term.NewMirror()
			if err != nil {
				fail(err)
				return
			}
			defer mirror.Close()
			closed := make(chan struct{})
			go func() {
				defer close(closed)
				buf := make([]byte, 4096)
				for {
					n, err := conn.Read(buf)
					if n > 0 {
						received.Add(int64(n))
						term.Send(buf[:n])
					}
					if err != nil {
						return
					}
				}
			}()
			conn.PayloadType = websocket.BinaryFrame
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-closed:
					return
				case <-ticker.C:
				}
				frame, _ := mirror.Frame()
				if len(frame) == 0 {
					continue
				}
				if _, err := conn.Write(frame); err != nil {
					return
				}
				sent.Add(int64(len(frame)))
			}
		},
	}
	srv := httptest.NewServer(server)
	defer func() { cancel(); srv.Close() }()
	p, err := loadForTest(filepath.Join("..", "..", "internal", "testhost", "testdata", "terminalweb"), nil)
	if err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(t.TempDir(), "tests.xml")
	address := "ws" + strings.TrimPrefix(srv.URL, "http") + "/" + token
	if err := runTests(ctx, p, testOptions{target: "web", junit: report, timeout: time.Minute, env: []string{"NSGO_TERMINAL_WS=" + address}, screenshots: os.Getenv("NSGO_TERMINAL_WEB_SCREENSHOTS")}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-failures:
		t.Fatal(err)
	default:
	}
	data, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	var suites junitSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatal(err)
	}
	if suites.Tests != 1 || suites.Failures != 0 {
		t.Fatalf("browser results: %s", data)
	}
	for _, suite := range suites.Suites {
		for _, c := range suite.Cases {
			if c.Skipped != nil {
				t.Fatalf("browser terminal test skipped: %+v", c.Skipped)
			}
		}
	}
	if received.Load() == 0 || sent.Load() == 0 {
		t.Fatalf("WebSocket did not carry bidirectional session data: received %d, sent %d", received.Load(), sent.Load())
	}
}
