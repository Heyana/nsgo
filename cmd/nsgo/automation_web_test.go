package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/net/websocket"

	"github.com/nsgo-dev/nsgo/internal/automation"
	projectcfg "github.com/nsgo-dev/nsgo/project"
)

// mockCDPServer serves a fake Chrome DevTools Protocol WebSocket endpoint for unit testing CDP page setup, binding dispatch, console forwarding and close.
type mockCDPServer struct {
	srv *httptest.Server

	mu         sync.Mutex
	ws         *websocket.Conn
	calls      []cdpMessage
	onCall     func(m cdpMessage) (any, error)
	closedFile string
}

func newMockCDPServer(t *testing.T, onCall func(m cdpMessage) (any, error)) *mockCDPServer {
	t.Helper()
	m := &mockCDPServer{onCall: onCall}
	m.srv = httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		m.mu.Lock()
		m.ws = ws
		m.mu.Unlock()
		for {
			var msg cdpMessage
			if err := websocket.JSON.Receive(ws, &msg); err != nil {
				return
			}
			m.mu.Lock()
			m.calls = append(m.calls, msg)
			m.mu.Unlock()
			var res any = map[string]any{}
			var callErr error
			switch msg.Method {
			case "Target.createBrowserContext":
				res = map[string]any{"browserContextId": "ctx-1"}
			case "Target.createTarget":
				res = map[string]any{"targetId": "target-1"}
			case "Target.attachToTarget":
				res = map[string]any{"sessionId": "session-1"}
			default:
				if m.onCall != nil {
					res, callErr = m.onCall(msg)
				}
			}
			resp := map[string]any{"id": msg.ID}
			if callErr != nil {
				resp["error"] = map[string]any{"message": callErr.Error()}
			} else {
				resp["result"] = res
			}
			if err := websocket.JSON.Send(ws, resp); err != nil {
				return
			}
			if msg.Method == "Browser.close" {
				if m.closedFile != "" {
					os.WriteFile(m.closedFile, nil, 0o600)
				}
				return
			}
		}
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *mockCDPServer) dial(t *testing.T) *cdp {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(m.srv.URL, "http")
	cfg, err := websocket.NewConfig(wsURL, cdpOrigin)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := cfg.DialContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	c := &cdp{ws: ws, pending: map[int64]chan cdpMessage{}}
	go c.read()
	t.Cleanup(c.close)
	return c
}

func (m *mockCDPServer) sendEvent(t *testing.T, session, method string, params any) {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	ws := m.ws
	m.mu.Unlock()
	if ws == nil {
		t.Fatal("mock CDP client not connected")
	}
	if err := websocket.JSON.Send(ws, cdpMessage{SessionID: session, Method: method, Params: raw}); err != nil {
		t.Fatal(err)
	}
}

func (m *mockCDPServer) recorded() []cdpMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]cdpMessage(nil), m.calls...)
}

func TestWebAutomationCDPSetupAndBridge(t *testing.T) {
	replies := make(chan string, 8)
	var mouseTypes []string
	var mouseMu sync.Mutex
	fakePNG := base64.StdEncoding.EncodeToString([]byte("fake-png-bytes"))

	mock := newMockCDPServer(t, func(msg cdpMessage) (any, error) {
		switch msg.Method {
		case "Input.dispatchMouseEvent":
			var p struct{ Type string }
			json.Unmarshal(msg.Params, &p)
			mouseMu.Lock()
			mouseTypes = append(mouseTypes, p.Type)
			mouseMu.Unlock()
		case "Page.captureScreenshot":
			return map[string]any{"data": fakePNG}, nil
		case "Runtime.evaluate":
			var p struct{ Expression string }
			json.Unmarshal(msg.Params, &p)
			replies <- p.Expression
		}
		return map[string]any{}, nil
	})

	b := mock.dial(t)
	var logs lockedBuffer
	targetURL := "http://localhost:8080/?nsgo-lang=de#nsgo-token=secret-tok"
	page, cleanup, err := openWebAutomation(b, targetURL, &logs)
	if err != nil {
		t.Fatalf("openWebAutomation: %v", err)
	}

	calls := mock.recorded()
	wantMethods := []string{
		"Target.createBrowserContext",
		"Target.createTarget",
		"Target.attachToTarget",
		"Runtime.enable",
		"Page.enable",
		"Runtime.addBinding",
		"Emulation.setDeviceMetricsOverride",
		"Page.navigate",
	}
	if len(calls) != len(wantMethods) {
		t.Fatalf("got %d setup calls, want %d: %+v", len(calls), len(wantMethods), calls)
	}
	for i, want := range wantMethods {
		if calls[i].Method != want {
			t.Errorf("call[%d] = %s, want %s", i, calls[i].Method, want)
		}
	}
	var addBind struct{ Name string }
	json.Unmarshal(calls[5].Params, &addBind)
	if addBind.Name != "nsgoTest" {
		t.Errorf("Runtime.addBinding name = %q, want nsgoTest", addBind.Name)
	}
	var metrics struct {
		Width, Height     int
		DeviceScaleFactor int
		Mobile            bool
	}
	json.Unmarshal(calls[6].Params, &metrics)
	if metrics.Width != webAutomationWidth || metrics.Height != webAutomationHeight || metrics.DeviceScaleFactor != webTestScale || metrics.Mobile {
		t.Errorf("Emulation.setDeviceMetricsOverride = %+v", metrics)
	}
	var nav struct{ URL string }
	json.Unmarshal(calls[7].Params, &nav)
	if nav.URL != targetURL {
		t.Errorf("Page.navigate url = %q, want %q", nav.URL, targetURL)
	}

	// Console output and uncaught exceptions are forwarded to the writer.
	mock.sendEvent(t, "session-1", "Runtime.consoleAPICalled", map[string]any{
		"args": []map[string]any{{"value": "hello"}, {"value": "from web"}},
	})
	mock.sendEvent(t, "session-1", "Runtime.exceptionThrown", map[string]any{
		"exceptionDetails": map[string]any{
			"text":      "Uncaught",
			"exception": map[string]any{"description": "ReferenceError: boom"},
		},
	})
	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		got := logs.String()
		if strings.Contains(got, "hello from web\n") && strings.Contains(got, "uncaught: Uncaught ReferenceError: boom\n") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("console output = %q", got)
		}
	}

	// A press op dispatches mouse events and replies via nsgoTestReply.
	waitReply := func() string {
		t.Helper()
		select {
		case expr := <-replies:
			return expr
		case <-time.After(3 * time.Second):
			t.Fatal("no nsgoTestReply received")
			return ""
		}
	}
	mock.sendEvent(t, "session-1", "Runtime.bindingCalled", map[string]any{
		"name":    "nsgoTest",
		"payload": `{"id":1,"op":"press","x":50,"y":60}`,
	})
	if expr := waitReply(); !strings.HasPrefix(expr, "nsgoTestReply(1, ") || strings.Contains(expr, `"error"`) {
		t.Errorf("press reply = %q", expr)
	}
	mouseMu.Lock()
	gotMouse := strings.Join(mouseTypes, ",")
	mouseMu.Unlock()
	if gotMouse != "mouseMoved,mousePressed,mouseReleased" {
		t.Errorf("press mouse sequence = %q", gotMouse)
	}

	// A screenshot op returns the captured base64 data.
	mock.sendEvent(t, "session-1", "Runtime.bindingCalled", map[string]any{
		"name":    "nsgoTest",
		"payload": `{"id":2,"op":"screenshot"}`,
	})
	if expr := waitReply(); !strings.HasPrefix(expr, "nsgoTestReply(2, ") || !strings.Contains(expr, fakePNG) {
		t.Errorf("screenshot reply = %q", expr)
	}

	// Main-frame navigation resets held modifier state.
	mock.sendEvent(t, "session-1", "Runtime.bindingCalled", map[string]any{
		"name":    "nsgoTest",
		"payload": `{"id":3,"op":"keyDown","key":"shift"}`,
	})
	waitReply()
	page.heldMu.Lock()
	heldBefore := page.held
	page.heldMu.Unlock()
	if heldBefore == 0 {
		t.Fatal("shift keyDown did not record held modifier")
	}
	mock.sendEvent(t, "session-1", "Page.frameNavigated", map[string]any{
		"frame": map[string]any{"id": "main", "parentId": ""},
	})
	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		page.heldMu.Lock()
		h := page.held
		page.heldMu.Unlock()
		if h == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("held modifiers not reset after navigation: %d", h)
		}
	}

	// An unknown op answers with an error in nsgoTestReply.
	mock.sendEvent(t, "session-1", "Runtime.bindingCalled", map[string]any{
		"name":    "nsgoTest",
		"payload": `{"id":4,"op":"bogus"}`,
	})
	if expr := waitReply(); !strings.HasPrefix(expr, "nsgoTestReply(4, ") || !strings.Contains(expr, "unknown op") {
		t.Errorf("unknown op reply = %q", expr)
	}

	cleanup()
	var disposed bool
	for _, c := range mock.recorded() {
		if c.Method == "Target.disposeBrowserContext" {
			disposed = true
		}
	}
	if !disposed {
		t.Error("cleanup did not dispose browser context")
	}
}

func TestWebAutomationSessionLifecycle(t *testing.T) {
	var navigated []string
	var navMu sync.Mutex
	mock := newMockCDPServer(t, func(msg cdpMessage) (any, error) {
		if msg.Method == "Page.navigate" {
			var p struct{ URL string }
			json.Unmarshal(msg.Params, &p)
			navMu.Lock()
			navigated = append(navigated, p.URL)
			navMu.Unlock()
		}
		return map[string]any{}, nil
	})
	t.Setenv("NSGO_TEST_BROWSER_PORT", fakeBrowserPort(t, mock.srv))
	closedMarker := filepath.Join(t.TempDir(), "closed")
	mock.closedFile = closedMarker
	t.Setenv("NSGO_TEST_BROWSER_CLOSED", closedMarker)
	fakeBrowser(t, `profile=$(cat "$NSGO_TEST_BROWSER_PROFILE")
printf '%s\n/devtools/browser/test\n' "$NSGO_TEST_BROWSER_PORT" > "$profile/DevToolsActivePort"
while [ ! -e "$NSGO_TEST_BROWSER_CLOSED" ]; do sleep 0.05; done
`)

	srv, err := startDevServer("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()
	p := &project{Config: projectcfg.Config{Dir: t.TempDir()}}
	s := webSession(devOptions{automation: true, server: srv, lang: "de"})

	ctx := t.Context()
	app1, err := s.start(ctx, p)
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	navMu.Lock()
	firstNavs := append([]string(nil), navigated...)
	navMu.Unlock()
	wantURL := srv.pageURL(srv.urlAt("localhost"), "/?nsgo-lang=de")
	if len(firstNavs) != 1 || firstNavs[0] != wantURL {
		t.Fatalf("navigated = %v, want [%s]", firstNavs, wantURL)
	}

	// Stopping the first run's wait and starting again (a rebuild) reuses the browser.
	waitDone := make(chan error, 1)
	go func() { waitDone <- app1.wait(ctx) }()
	app1.stop()
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("stopped wait returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("app1.stop did not end app1.wait")
	}

	app2, err := s.start(ctx, p)
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	defer app2.stop()
	navMu.Lock()
	secondNavs := len(navigated)
	navMu.Unlock()
	if secondNavs != 1 {
		t.Errorf("second start navigated again: %d total navigations", secondNavs)
	}

	// Closing the session sends Browser.close and removes the browser profile.
	s.close()
	checkBrowserProfileRemoved(t)
}

func TestWebAutomationSessionExplicitHost(t *testing.T) {
	var navigated []string
	var navMu sync.Mutex
	mock := newMockCDPServer(t, func(msg cdpMessage) (any, error) {
		if msg.Method == "Page.navigate" {
			var p struct{ URL string }
			json.Unmarshal(msg.Params, &p)
			navMu.Lock()
			navigated = append(navigated, p.URL)
			navMu.Unlock()
		}
		return map[string]any{}, nil
	})
	t.Setenv("NSGO_TEST_BROWSER_PORT", fakeBrowserPort(t, mock.srv))
	closedMarker := filepath.Join(t.TempDir(), "closed")
	mock.closedFile = closedMarker
	t.Setenv("NSGO_TEST_BROWSER_CLOSED", closedMarker)
	fakeBrowser(t, `profile=$(cat "$NSGO_TEST_BROWSER_PROFILE")
printf '%s\n/devtools/browser/test\n' "$NSGO_TEST_BROWSER_PORT" > "$profile/DevToolsActivePort"
while [ ! -e "$NSGO_TEST_BROWSER_CLOSED" ]; do sleep 0.05; done
`)

	srv, err := startDevServer("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()
	p := &project{Dir: t.TempDir()}
	s := webSession(devOptions{automation: true, server: srv})
	defer s.close()

	app, err := s.start(t.Context(), p)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer app.stop()

	navMu.Lock()
	got := append([]string(nil), navigated...)
	navMu.Unlock()
	wantURL := srv.pageURL(srv.url(), "/")
	if len(got) != 1 || got[0] != wantURL {
		t.Fatalf("navigated = %v, want [%s]", got, wantURL)
	}
}

func TestWebAutomationSessionReportsBrowserExit(t *testing.T) {
	mock := newMockCDPServer(t, nil)
	t.Setenv("NSGO_TEST_BROWSER_PORT", fakeBrowserPort(t, mock.srv))
	fakeBrowser(t, `profile=$(cat "$NSGO_TEST_BROWSER_PROFILE")
printf '%s\n/devtools/browser/test\n' "$NSGO_TEST_BROWSER_PORT" > "$profile/DevToolsActivePort"
sleep 0.2
exit 7
`)
	srv, err := startDevServer("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()
	p := &project{Config: projectcfg.Config{Dir: t.TempDir()}}
	s := webSession(devOptions{automation: true, server: srv})
	defer s.close()

	app, err := s.start(t.Context(), p)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer app.stop()

	select {
	case err := <-func() chan error {
		ch := make(chan error, 1)
		go func() { ch <- app.wait(t.Context()) }()
		return ch
	}():
		if err == nil || !strings.Contains(err.Error(), "automation browser exited") {
			t.Fatalf("wait err = %v, want automation browser exited", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait did not report browser exit")
	}
}

// TestWebAutomationEndToEnd builds testdata/automation as nsgo dev -target web -automation does, opens it in a real headless Chromium browser over CDP with the nsgoTest binding, and drives it end to end through the automation hub and nsgo mcp.
func TestWebAutomationEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an app for the web and drives it in a browser")
	}
	if _, err := findBrowser(); err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	p, err := load(filepath.Join("..", "..", "testdata", "automation"))
	if err != nil {
		t.Fatal(err)
	}
	p.prepareDev(devOptions{automation: true})
	dir := t.TempDir()
	if err := buildWeb(ctx, p, dir, true); err != nil {
		t.Fatal(err)
	}

	srv, err := startDevServer("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()
	srv.dir = p.Dir
	srv.serve(dir)

	h, err := newAutomationHub(srv, "web")
	if err != nil {
		t.Fatal(err)
	}
	sessionDir := t.TempDir()
	if err := h.writeSession(sessionDir); err != nil {
		t.Fatal(err)
	}
	defer h.removeSession()
	appConsole.teeTo(h.log)
	defer appConsole.teeTo(nil)

	b, err := startBrowser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()

	pageURL := srv.pageURL(srv.urlAt("localhost"), "/")
	pg, cleanupPage, err := openWebAutomation(b, pageURL, &appConsole)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupPage()

	do := func(method string, params, result any) *automation.Error {
		t.Helper()
		data, _ := json.Marshal(params)
		a := h.answer(automation.Request{Method: method, Params: data})
		if a.Error == nil && result != nil {
			if err := json.Unmarshal(a.Result, result); err != nil {
				t.Fatal(err)
			}
		}
		return a.Error
	}
	must := func(method string, params, result any) {
		t.Helper()
		if err := do(method, params, result); err != nil {
			t.Fatalf("%s %+v: %v", method, params, err)
		}
	}
	shows := func(selector string) {
		t.Helper()
		must(automation.Wait, automation.WaitParams{Selector: selector}, nil)
	}

	var ws automation.WindowsResult
	for deadline := time.Now().Add(60 * time.Second); len(ws.Windows) == 0; time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the web app showed no window")
		}
		if err := do(automation.Windows, automation.WindowsParams{}, &ws); err != nil && err.Code != automation.NoApp {
			t.Fatalf("windows: %v", err)
		}
	}
	if len(ws.Windows) != 1 || !ws.Windows[0].Default || ws.Windows[0].Kind != "app" || ws.Windows[0].ID == 0 || ws.Windows[0].Graph <= 0 {
		t.Fatalf("windows %+v", ws)
	}
	if ws.Windows[0].Width != webAutomationWidth || ws.Windows[0].Height != webAutomationHeight {
		t.Fatalf("window size = %gx%g, want %dx%d", ws.Windows[0].Width, ws.Windows[0].Height, webAutomationWidth, webAutomationHeight)
	}

	var status automation.StatusResult
	must(automation.Status, automation.StatusParams{}, &status)
	if status.Target != "web" || !status.App || status.Run != 1 || status.BuildError != "" {
		t.Fatalf("status = %+v", status)
	}

	// Drive through an in-memory MCP client backed by the session file, just as nsgo mcp does.
	cs := connectMCP(t, &sessionBridge{dir: sessionDir})
	snapRes, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: automation.Snapshot, Arguments: map[string]any{}})
	if err != nil || snapRes.IsError {
		t.Fatalf("mcp snapshot: %+v, %v", snapRes, err)
	}
	snapText := snapRes.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(snapText, `- button "Add"`) {
		t.Fatalf("snapshot:\n%s", snapText)
	}

	// Press, Type, Key.
	must(automation.Press, automation.PressParams{Selector: "id:add"}, nil)
	shows(`text:"Count: 1"`)
	must(automation.Type, automation.TypeParams{Selector: "id:name", Text: "Ada"}, nil)
	shows(`text:"Hello, Ada"`)
	must(automation.Key, automation.KeyParams{Selector: "id:name", Key: "mod+a"}, nil)
	must(automation.Key, automation.KeyParams{Selector: "id:name", Key: "Backspace"}, nil)
	shows(`text:"Hello, "`)

	// Native menus are unsupported on the web and report Unsupported cleanly.
	if err := do(automation.Menu, automation.MenuParams{Path: []string{"Actions", "Reset"}}, nil); err == nil || err.Code != automation.Unsupported {
		t.Fatalf("menu on web = %v, want unsupported", err)
	}

	// Alerts: list, answer Yes, answer No, dismiss via Escape.
	must(automation.Press, automation.PressParams{Selector: "id:ask"}, nil)
	var alerts automation.AlertResult
	must(automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 1 || alerts.Alerts[0].Window != ws.Windows[0].ID || alerts.Alerts[0].Title != "Keep going?" {
		t.Fatalf("alert = %+v", alerts)
	}
	yes, no, dismiss := 0, 1, -1
	must(automation.Alert, automation.AlertParams{Answer: &yes}, &alerts)
	shows(`text:"Answer: Yes"`)

	must(automation.Press, automation.PressParams{Selector: "id:ask"}, nil)
	must(automation.Alert, automation.AlertParams{Answer: &no}, &alerts)
	shows(`text:"Answer: No"`)

	must(automation.Press, automation.PressParams{Selector: "id:ask"}, nil)
	must(automation.Alert, automation.AlertParams{Answer: &dismiss}, &alerts)
	if len(alerts.Alerts) != 0 {
		t.Fatalf("dismissed alert still shows: %+v", alerts)
	}

	// Token is not leaked in the environment or the page's address.
	shows(`text:"Token in the environment: false"`)
	raw, err := pg.call("Runtime.evaluate", map[string]any{"expression": "location.href", "returnByValue": true})
	if err != nil {
		t.Fatal(err)
	}
	var href struct{ Result struct{ Value string } }
	json.Unmarshal(raw, &href)
	if strings.Contains(href.Result.Value, srv.token) {
		t.Errorf("page address %s carries the run's token", href.Result.Value)
	}

	// Scroll.
	rowY := func() float64 {
		t.Helper()
		var found automation.FindResult
		must(automation.Find, automation.FindParams{Selector: "id:row-60"}, &found)
		return found.Elements[0].Bounds[1]
	}
	before := rowY()
	must(automation.Scroll, automation.ScrollParams{Selector: "id:list", By: 1500}, nil)
	if moved := before - rowY(); moved < 1000 {
		t.Fatalf("the list scrolled %g points of 1500", moved)
	}

	// Screenshot via MCP returns a valid PNG matching the viewport scale.
	shotRes, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: automation.Screenshot, Arguments: map[string]any{}})
	if err != nil || shotRes.IsError {
		t.Fatalf("mcp screenshot: %+v, %v", shotRes, err)
	}
	imgContent, ok := shotRes.Content[0].(*mcp.ImageContent)
	if !ok || imgContent.MIMEType != "image/png" || len(imgContent.Data) == 0 {
		t.Fatalf("screenshot content: %+v", shotRes.Content)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(imgContent.Data))
	if err != nil || cfg.Width < webAutomationWidth || cfg.Height < webAutomationHeight {
		t.Fatalf("screenshot PNG = %dx%d, err=%v", cfg.Width, cfg.Height, err)
	}

	// Console output from the page reaches automation logs.
	if _, err := pg.call("Runtime.evaluate", map[string]any{"expression": `console.log("web-automation-log-check")`}); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		var logs automation.LogsResult
		must(automation.Logs, automation.LogsParams{Lines: 50}, &logs)
		if strings.Contains(strings.Join(logs.Lines, "\n"), "web-automation-log-check") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("console output never reached logs: %q", logs.Lines)
		}
	}

	// Reload: the page reloads with its sessionStorage token and persistent nsgoTest binding.
	srv.send("reload", "")
	for deadline := time.Now().Add(60 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		must(automation.Status, automation.StatusParams{}, &status)
		if status.App && status.Run >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("reloaded web app never reconnected: %+v", status)
		}
	}
	must(automation.Press, automation.PressParams{Selector: "id:add"}, nil)
	shows(`text:"Count: 1"`)
}

// TestWebMCPEndToEnd spawns the real nsgo mcp CLI with -target web, letting it start nsgo dev -automation -target web in headless Chromium, drives the app end to end over MCP stdio (including app-defined tools, input, scrolling, alerts, screenshots, logs and reload), and checks that closing MCP ends the dev session cleanly.
func TestWebMCPEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns nsgo mcp -target web and drives an app in a browser")
	}
	if _, err := findBrowser(); err != nil {
		t.Skip(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(filepath.Join(root, "testdata"), ".automation-web-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	for _, name := range []string{"nsgo.yaml", "main.go"} {
		code, err := os.ReadFile(filepath.Join(root, "testdata", "automation", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), code, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	var output lockedBuffer
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("nsgo mcp output:\n%s", output.String())
		}
	})
	command := exec.CommandContext(ctx, "go", "tool", "nsgo", "mcp", "-target", "web", "-port", "0", dir)
	command.Dir = root
	command.Stderr = &output
	client := mcp.NewClient(&mcp.Implementation{Name: "web-mcp-test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	call := func(method string, args map[string]any) []mcp.Content {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: method, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		if result.IsError || len(result.Content) == 0 {
			t.Fatalf("%s: %+v", method, result)
		}
		return result.Content
	}
	read := func(method string, args map[string]any) string {
		t.Helper()
		content := call(method, args)
		if body, ok := content[0].(*mcp.TextContent); ok {
			return body.Text
		}
		t.Fatalf("%s returned %T instead of text", method, content[0])
		return ""
	}
	show := func(selector string) {
		t.Helper()
		read(automation.Wait, map[string]any{"selector": selector})
	}

	// Wait for the child nsgo dev -automation -target web build and headless browser connection.
	for {
		status := read(automation.Status, nil)
		var live automation.StatusResult
		if err := json.Unmarshal([]byte(status), &live); err != nil {
			t.Fatal(err)
		}
		if live.BuildError != "" {
			t.Fatalf("building the web app: %s", live.BuildError)
		}
		if live.App {
			if live.Target != "web" || live.Run != 1 {
				t.Fatalf("status: %+v", live)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("the web app did not connect: %s", status)
		case <-time.After(100 * time.Millisecond):
		}
	}

	var list automation.WindowsResult
	for deadline := time.Now().Add(30 * time.Second); len(list.Windows) == 0; time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the web app showed no window")
		}
		if err := json.Unmarshal([]byte(read(automation.Windows, nil)), &list); err != nil {
			t.Fatal(err)
		}
	}
	if len(list.Windows) != 1 || !list.Windows[0].Default || list.Windows[0].Kind != "app" || list.Windows[0].Width != webAutomationWidth || list.Windows[0].Height != webAutomationHeight {
		t.Fatalf("windows: %+v", list.Windows)
	}

	// App-defined MCP tools appear in tools/list and update the web app's state.
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var hasSetCount bool
	for _, tool := range tools.Tools {
		if tool.Name == "set_count" && tool.Description == "Set the counter to a test value" {
			hasSetCount = true
			break
		}
	}
	if !hasSetCount {
		t.Fatal("nsgo mcp -target web did not list set_count")
	}
	read("set_count", map[string]any{"count": 3})
	show(`text:"Count: 3"`)

	if snapshot := read(automation.Snapshot, nil); !strings.Contains(snapshot, `- button "Add"`) || !strings.Contains(snapshot, "id=add") {
		t.Fatalf("snapshot:\n%s", snapshot)
	}
	read(automation.Press, map[string]any{"selector": "id:add"})
	show(`text:"Count: 4"`)
	read(automation.Type, map[string]any{"selector": "id:name", "text": "Ada"})
	show(`text:"Hello, Ada"`)
	read(automation.Key, map[string]any{"selector": "id:name", "key": "mod+a"})
	read(automation.Key, map[string]any{"selector": "id:name", "key": "Backspace"})
	show(`text:"Hello, "`)

	// Native menus report unsupported cleanly over MCP on the web.
	menuRes, err := session.CallTool(ctx, &mcp.CallToolParams{Name: automation.Menu, Arguments: map[string]any{"path": []string{"Actions", "Reset"}}})
	if err != nil || !menuRes.IsError || len(menuRes.Content) == 0 {
		t.Fatalf("menu on web: %+v, %v", menuRes, err)
	}
	if text := menuRes.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, string(automation.Unsupported)) {
		t.Fatalf("menu on web error = %q, want unsupported", text)
	}

	// Alerts: list, answer and dismiss.
	read(automation.Press, map[string]any{"selector": "id:ask"})
	if alerts := read(automation.Alert, nil); !strings.Contains(alerts, "Keep going?") {
		t.Fatalf("alerts:\n%s", alerts)
	}
	read(automation.Alert, map[string]any{"answer": 0})
	show(`text:"Answer: Yes"`)
	read(automation.Press, map[string]any{"selector": "id:ask"})
	read(automation.Alert, map[string]any{"answer": -1})
	var afterDismiss automation.AlertResult
	if err := json.Unmarshal([]byte(read(automation.Alert, nil)), &afterDismiss); err != nil || len(afterDismiss.Alerts) != 0 {
		t.Fatalf("alerts after dismiss: %+v, %v", afterDismiss, err)
	}

	// Token is absent from the app's environment.
	show(`text:"Token in the environment: false"`)

	// Scroll and find.
	rowY := func() float64 {
		t.Helper()
		var found automation.FindResult
		if err := json.Unmarshal([]byte(read(automation.Find, map[string]any{"selector": "id:row-60"})), &found); err != nil {
			t.Fatal(err)
		}
		if len(found.Elements) == 0 {
			t.Fatal("id:row-60 not found")
		}
		return found.Elements[0].Bounds[1]
	}
	before := rowY()
	read(automation.Scroll, map[string]any{"selector": "id:list", "by": 1500})
	if moved := before - rowY(); moved < 1000 {
		t.Fatalf("the list scrolled %g points of 1500", moved)
	}

	// Screenshot via MCP returns a PNG matching the viewport.
	screenshot := call(automation.Screenshot, nil)
	shot, ok := screenshot[0].(*mcp.ImageContent)
	if !ok || shot.MIMEType != "image/png" || len(shot.Data) == 0 {
		t.Fatalf("screenshot returned %+v", screenshot)
	}
	size, err := png.DecodeConfig(bytes.NewReader(shot.Data))
	if err != nil || size.Width < webAutomationWidth || size.Height < webAutomationHeight {
		t.Fatalf("screenshot size %dx%d: %v", size.Width, size.Height, err)
	}

	// Logs succeeds over MCP.
	var logs automation.LogsResult
	if err := json.Unmarshal([]byte(read(automation.Logs, map[string]any{"lines": 20})), &logs); err != nil {
		t.Fatalf("logs: %v", err)
	}

	// Reload rebuilds, reloads the page, reconnects with run >= 2 and remains interactive.
	read(automation.Reload, nil)
	for {
		status := read(automation.Status, nil)
		var live automation.StatusResult
		if err := json.Unmarshal([]byte(status), &live); err != nil {
			t.Fatal(err)
		}
		if live.BuildError != "" {
			t.Fatalf("rebuilding the web app: %s", live.BuildError)
		}
		if live.App && live.Run >= 2 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("the reloaded web app did not reconnect: %s", status)
		case <-time.After(100 * time.Millisecond):
		}
	}
	read(automation.Press, map[string]any{"selector": "id:add"})
	show(`text:"Count: 1"`)

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readSession(dir); !os.IsNotExist(err) {
		t.Fatalf("the dev session did not end with MCP: %v", err)
	}
}
