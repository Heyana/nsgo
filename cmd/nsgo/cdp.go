package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/net/websocket"
)

// cdp is a connection to a browser over the Chrome DevTools protocol: one
// WebSocket, the sessions of the pages attached to it flattened into it.
type cdp struct {
	ws      *websocket.Conn
	mu      sync.Mutex
	next    int64
	pending map[int64]chan cdpMessage
	// on receives each event, on the reading goroutine.
	on func(cdpMessage)
	// browser is the browser's process; done closes once Wait finishes and exitErr holds its error.
	browser   *exec.Cmd
	done      chan struct{}
	exitErr   error
	stop      context.CancelFunc
	profile   string
	closeOnce sync.Once
}

type cdpMessage struct {
	ID        int64           `json:"id,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// browserPaths are where Chrome, Chromium and Edge install, by system.
var browserPaths = map[string][]string{
	"darwin": {
		"/Applications/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
	},
	"linux":   {"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge"},
	"windows": {`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`},
}

// cdpOrigin is the origin the tool connects to the browser from.
const cdpOrigin = "http://127.0.0.1"

const browserShutdownDelay = 10 * time.Second

// findBrowser finds a Chromium browser: NSGO_BROWSER, else an installed
// Chrome, Chromium or Edge. NSGo never downloads one.
func findBrowser() (string, error) {
	if b := os.Getenv("NSGO_BROWSER"); b != "" {
		return b, nil
	}
	for _, p := range browserPaths[runtime.GOOS] {
		if path, err := exec.LookPath(p); err == nil {
			return path, nil
		}
	}
	return "", errors.New("the web target needs Chrome, Chromium or Edge: install one, or set NSGO_BROWSER to its executable")
}

func browserArgs(goos, profile string) []string {
	args := []string{"--headless=new", "--remote-debugging-port=0", "--user-data-dir=" + profile,
		// Match the browser's device scale to the emulated test viewport: mismatched scales halve DOM wheel deltas while native scrolling still moves the full requested CSS distance.
		fmt.Sprintf("--force-device-scale-factor=%d", webTestScale),
		"--no-first-run", "--no-default-browser-check", "--disable-extensions", "--mute-audio",
		// The DevTools socket takes this tool, which names its origin.
		"--remote-allow-origins=" + cdpOrigin,
		// A fine, hovering pointer, as Playwright gives it: headless finds
		// no input device and would measure as a touch screen.
		"--blink-settings=primaryHoverType=2,availableHoverTypes=2,primaryPointerType=4,availablePointerTypes=4"}
	if goos == "darwin" {
		// Chrome's code-signature clone persists unless the browser exits cleanly.
		args = append(args, "--disable-features=MacAppCodeSignClone")
	}
	return append(args, "about:blank")
}

// startBrowser starts a headless browser with a profile of its own and
// connects to it.
func startBrowser(ctx context.Context) (*cdp, error) {
	path, err := findBrowser()
	if err != nil {
		return nil, err
	}
	profile, err := os.MkdirTemp("", "nsgo-browser-")
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(profile, "browser.log")
	log, err := os.Create(logPath)
	if err != nil {
		os.RemoveAll(profile)
		return nil, err
	}
	browserCtx, stop := context.WithCancel(ctx)
	cmd := exec.CommandContext(browserCtx, path, browserArgs(runtime.GOOS, profile)...)
	var connected atomic.Pointer[cdp]
	cmd.Cancel = func() error {
		if browser := connected.Load(); browser != nil && browser.requestClose() == nil {
			return nil
		}
		if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
			return cmd.Process.Signal(syscall.SIGTERM)
		}
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = browserShutdownDelay
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		stop()
		log.Close()
		os.RemoveAll(profile)
		return nil, err
	}
	log.Close()
	c := &cdp{pending: map[int64]chan cdpMessage{}, browser: cmd, done: make(chan struct{}), stop: stop, profile: profile}
	go func() {
		c.exitErr = cmd.Wait()
		close(c.done)
	}()
	failed := func(err error) (*cdp, error) {
		stop()
		<-c.done
		// Read only after Wait, when the child has stopped writing.
		if log, readErr := os.Open(logPath); readErr == nil {
			if info, statErr := log.Stat(); statErr == nil {
				log.Seek(max(info.Size()-8192, 0), 0)
			}
			data, _ := io.ReadAll(log)
			log.Close()
			if text := strings.TrimSpace(string(data)); text != "" {
				err = fmt.Errorf("%w\nbrowser stderr: %s", err, text)
			}
		}
		os.RemoveAll(profile)
		return nil, err
	}
	var url string
	// A cold browser competes with the package tests on hosted runners.
	// Wait for readiness, cancellation or exit, rather than hiding a crash
	// behind the same short timeout as a slow launch.
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()
	for url == "" {
		select {
		case <-startup.Done():
			return failed(fmt.Errorf("waiting for the browser's DevTools port: %w", startup.Err()))
		case <-c.done:
			if ctx.Err() != nil {
				return failed(fmt.Errorf("waiting for the browser's DevTools port: %w", ctx.Err()))
			}
			return failed(fmt.Errorf("the browser exited before opening its DevTools port: %v", c.exitErr))
		case <-poll.C:
		}
		data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
		if lines := strings.Split(strings.TrimSpace(string(data)), "\n"); err == nil && len(lines) == 2 {
			url = "ws://127.0.0.1:" + lines[0] + lines[1]
			break
		}
	}
	config, err := websocket.NewConfig(url, cdpOrigin)
	if err != nil {
		return failed(err)
	}
	ws, err := config.DialContext(startup)
	if err != nil {
		// DialError does not unwrap, so name the cancellation or deadline itself for errors.Is.
		if startup.Err() != nil {
			err = fmt.Errorf("connecting to the browser's DevTools port: %w", startup.Err())
		}
		return failed(err)
	}
	ws.MaxPayloadBytes = 256 << 20
	c.ws = ws
	connected.Store(c)
	go c.read()
	return c, nil
}

func (c *cdp) read() {
	for {
		var m cdpMessage
		if err := websocket.JSON.Receive(c.ws, &m); err != nil {
			c.mu.Lock()
			for id, ch := range c.pending {
				close(ch)
				delete(c.pending, id)
			}
			c.mu.Unlock()
			return
		}
		if m.ID != 0 {
			c.mu.Lock()
			ch := c.pending[m.ID]
			delete(c.pending, m.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- m
			}
			continue
		}
		c.mu.Lock()
		on := c.on
		c.mu.Unlock()
		if on != nil {
			on(m)
		}
	}
}

// call calls method in session ("" for the browser) and returns its
// result.
func (c *cdp) call(session, method string, params any) (json.RawMessage, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if params == nil {
		raw = []byte("{}")
	}
	c.mu.Lock()
	c.next++
	id := c.next
	ch := make(chan cdpMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	if err := websocket.JSON.Send(c.ws, cdpMessage{ID: id, SessionID: session, Method: method, Params: raw}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	select {
	case m, ok := <-ch:
		if !ok {
			return nil, errors.New("the browser closed")
		}
		if m.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, m.Error.Message)
		}
		return m.Result, nil
	case <-time.After(60 * time.Second):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("%s: no answer", method)
	}
}

// requestClose sends Browser.close without waiting for a response. The
// browser may close its socket before replying, so exec waits on the process.
func (c *cdp) requestClose() error {
	c.ws.SetWriteDeadline(time.Now().Add(time.Second))
	c.mu.Lock()
	c.next++
	id := c.next
	c.mu.Unlock()
	return websocket.JSON.Send(c.ws, cdpMessage{ID: id, Method: "Browser.close"})
}

func (c *cdp) close() {
	c.closeOnce.Do(func() {
		if c.stop != nil {
			c.stop()
		} else if c.ws != nil {
			c.call("", "Browser.close", nil)
		}
		if c.done != nil {
			select {
			case <-c.done:
			case <-time.After(3 * time.Second):
				if c.browser != nil && c.browser.Process != nil {
					c.browser.Process.Kill()
				}
				<-c.done
			}
		}
		if c.ws != nil {
			c.ws.Close()
		}
		if c.profile != "" {
			os.RemoveAll(c.profile)
		}
	})
}
