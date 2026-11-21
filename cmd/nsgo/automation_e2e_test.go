package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/automation"
)

// lockedBuffer is a buffer two goroutines write.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// TestAutomationEndToEnd builds testdata/automation as nsgo dev -automation does, runs it against a hub and drives it with posted events, the real menu bar, Dock/Jump List menu and alerts. It opens a window, so it runs only when asked, on macOS or Windows with a GUI session:
//
//	NSGO_AUTOMATION_E2E=1 go tool nsgo test -run TestAutomationEndToEnd ./cmd/nsgo
func TestAutomationEndToEnd(t *testing.T) {
	if (runtime.GOOS != "darwin" && runtime.GOOS != "windows") || os.Getenv("NSGO_AUTOMATION_E2E") == "" {
		t.Skip("set NSGO_AUTOMATION_E2E=1 on macOS or Windows with a GUI session: the test opens a window")
	}
	ctx := context.Background()
	p, err := load(filepath.Join("..", "..", "testdata", "automation"))
	if err != nil {
		t.Fatal(err)
	}
	p.prepareDev(devOptions{automation: true})
	var bin string
	switch runtime.GOOS {
	case "darwin":
		bundle, err := buildMacApp(ctx, p, t.TempDir(), runtime.GOARCH)
		if err != nil {
			t.Fatal(err)
		}
		bin = filepath.Join(bundle, "Contents", "MacOS", p.Name)
	case "windows":
		dir := t.TempDir()
		if err := buildWindows(ctx, p, dir, runtime.GOARCH); err != nil {
			t.Fatal(err)
		}
		bin = filepath.Join(dir, p.Name+".exe")
	}
	h := startHub(t)
	var out lockedBuffer
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "NSGO_DEV_SERVER="+h.srv.url(), "NSGO_DEV_TOKEN="+h.srv.token)
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
		if t.Failed() {
			t.Logf("the app's output:\n%s", out.String())
		}
	})
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
	for deadline := time.Now().Add(20 * time.Second); len(ws.Windows) == 0; time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the app showed no window")
		}
		must(automation.Windows, automation.WindowsParams{}, &ws)
	}
	if len(ws.Windows) != 1 || !ws.Windows[0].Default || ws.Windows[0].Kind != "window" {
		t.Fatalf("windows %+v", ws)
	}
	var snap automation.SnapshotResult
	must(automation.Snapshot, automation.SnapshotParams{}, &snap)
	if !strings.Contains(snap.Text, `- button "Add"`) {
		t.Fatalf("snapshot:\n%s", snap.Text)
	}
	must(automation.Press, automation.PressParams{Selector: "id:add"}, nil)
	shows(`text:"Count: 1"`)
	must(automation.Type, automation.TypeParams{Selector: "id:name", Text: "Ada"}, nil)
	shows(`text:"Hello, Ada"`)
	must(automation.Key, automation.KeyParams{Selector: "id:name", Key: "mod+a"}, nil)
	must(automation.Key, automation.KeyParams{Selector: "id:name", Key: "Backspace"}, nil)
	shows(`text:"Hello, "`)
	// The menu and its alert are the app function's, which the app's own surface holds.
	must(automation.Menu, automation.MenuParams{Path: []string{"Actions", "Reset"}}, nil)
	shows(`text:"Count: 0"`)
	// So is the Dock menu, which AppKit asks the app delegate for.
	must(automation.Menu, automation.MenuParams{Dock: true, Path: []string{"Add Ten"}}, nil)
	shows(`text:"Count: 10"`)
	must(automation.Menu, automation.MenuParams{Path: []string{"Actions", "Reset"}}, nil)
	shows(`text:"Count: 0"`)
	must(automation.Menu, automation.MenuParams{Path: []string{"Actions", "Ask"}}, nil)
	var alerts automation.AlertResult
	must(automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 1 || alerts.Alerts[0].Window != 0 || alerts.Alerts[0].Title != "Keep going?" {
		t.Fatalf("the app's alert: %+v", alerts)
	}
	dismissed := `text:"Answer: "`
	if runtime.GOOS == "windows" {
		// Pressing Escape on a Windows alert chooses its cancel action ("No").
		dismissed = `text:"Answer: No"`
	}
	yes, no, dismiss := 0, 1, -1
	must(automation.Alert, automation.AlertParams{Answer: &yes}, &alerts)
	shows(`text:"Answer: Yes"`)
	// The window's alert is a sheet on macOS and a drawn dialog on Windows, which a dismissal ends.
	must(automation.Press, automation.PressParams{Selector: "id:ask"}, nil)
	must(automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 1 || alerts.Alerts[0].Window != ws.Windows[0].ID {
		t.Fatalf("the window's alert: %+v", alerts)
	}
	must(automation.Alert, automation.AlertParams{Answer: &dismiss}, &alerts)
	shows(dismissed)
	must(automation.Press, automation.PressParams{Selector: "id:ask"}, nil)
	must(automation.Alert, automation.AlertParams{Window: &ws.Windows[0].ID, Answer: &no}, &alerts)
	shows(`text:"Answer: No"`)
	// Reset to Yes before dismissing the app's own alert so dismissal is observed as a state change on Windows too.
	must(automation.Press, automation.PressParams{Selector: "id:ask"}, nil)
	must(automation.Alert, automation.AlertParams{Window: &ws.Windows[0].ID, Answer: &yes}, &alerts)
	shows(`text:"Answer: Yes"`)
	// The app's own alert runs modal on macOS and in the root window on Windows, which a dismissal ends.
	must(automation.Menu, automation.MenuParams{Path: []string{"Actions", "Ask"}}, nil)
	own := uint32(0)
	must(automation.Alert, automation.AlertParams{Window: &own, Answer: &dismiss}, &alerts)
	shows(dismissed)
	if len(alerts.Alerts) != 0 {
		t.Fatalf("a dismissed alert still shows: %+v", alerts)
	}
	// The app took its token for the server out of its environment, as it started.
	shows(`text:"Token in the environment: false"`)
	// A scroll view shows all its content to selectors, so the scroll shows in where a row far down is placed.
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
	var shot automation.ScreenshotResult
	must(automation.Screenshot, automation.ScreenshotParams{}, &shot)
	if shot.Width < int(ws.Windows[0].Width) || shot.Height < int(ws.Windows[0].Height) || len(shot.PNG) == 0 {
		t.Fatalf("a %dx%d screenshot of a %gx%g window", shot.Width, shot.Height, ws.Windows[0].Width, ws.Windows[0].Height)
	}
}

// TestSimulatorAutomationEndToEnd builds testdata/automation for ios_simulator as nsgo dev -target ios_simulator -automation does, launches it with SIMCTL_CHILD_ environment variables and drives it through the automation hub.
//
//	NSGO_SIMULATOR_E2E=1 go tool nsgo test -run TestSimulatorAutomationEndToEnd ./cmd/nsgo
func TestSimulatorAutomationEndToEnd(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("NSGO_SIMULATOR_E2E") != "1" {
		t.Skip("set NSGO_SIMULATOR_E2E=1 on an Apple silicon Mac to drive the iOS simulator through automation")
	}
	if runtime.GOARCH != "arm64" {
		t.Fatal("NSGo's simulator binaries are arm64; run this check on an Apple silicon Mac")
	}
	ctx := context.Background()
	p, err := load(filepath.Join("..", "..", "testdata", "automation"))
	if err != nil {
		t.Fatal(err)
	}
	p.Dev.Target = "ios_simulator"
	p.prepareDev(devOptions{automation: true})
	bundle, err := buildIOSApp(ctx, p, iOS, t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	udid, err := bootSimulator(ctx, iOS, os.Getenv("NSGO_SIMULATOR_DEVICE"))
	if err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, "xcrun", "simctl", "install", udid, bundle); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		exec.Command("xcrun", "simctl", "terminate", udid, p.ID).Run()
		exec.Command("xcrun", "simctl", "uninstall", udid, p.ID).Run()
	})
	h := startHub(t)
	var out lockedBuffer
	cmd := exec.Command("xcrun", "simctl", "launch", "--console-pty", "--terminate-running-process", udid, p.ID)
	cmd.Env = append(os.Environ(), simulatorEnv(devOptions{server: h.srv, automation: true})...)
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
		if t.Failed() {
			t.Logf("the simulator app's output:\n%s", out.String())
		}
	})
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
	for deadline := time.Now().Add(30 * time.Second); len(ws.Windows) == 0; time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the simulator app showed no surface")
		}
		must(automation.Windows, automation.WindowsParams{}, &ws)
	}
	if len(ws.Windows) != 1 || !ws.Windows[0].Default || ws.Windows[0].ID != 0 || ws.Windows[0].Kind != "app" {
		t.Fatalf("windows %+v", ws)
	}
	var snap automation.SnapshotResult
	must(automation.Snapshot, automation.SnapshotParams{}, &snap)
	if !strings.Contains(snap.Text, `- button "Add"`) {
		t.Fatalf("snapshot:\n%s", snap.Text)
	}
	must(automation.Press, automation.PressParams{Selector: "id:add"}, nil)
	shows(`text:"Count: 1"`)
	must(automation.Type, automation.TypeParams{Selector: "id:name", Text: "Ada"}, nil)
	shows(`text:"Hello, Ada"`)
	must(automation.Key, automation.KeyParams{Selector: "id:name", Key: "mod+a"}, nil)
	must(automation.Key, automation.KeyParams{Selector: "id:name", Key: "Backspace"}, nil)
	shows(`text:"Hello, "`)
	// Alerts on iOS are answered by tapping their UIAlertController buttons.
	must(automation.Press, automation.PressParams{Selector: "id:ask"}, nil)
	var alerts automation.AlertResult
	must(automation.Alert, automation.AlertParams{}, &alerts)
	if len(alerts.Alerts) != 1 || alerts.Alerts[0].Window != 0 || alerts.Alerts[0].Title != "Keep going?" {
		t.Fatalf("the simulator alert: %+v", alerts)
	}
	yes, dismiss := 0, -1
	must(automation.Alert, automation.AlertParams{Answer: &yes}, &alerts)
	shows(`text:"Answer: Yes"`)
	must(automation.Press, automation.PressParams{Selector: "id:ask"}, nil)
	must(automation.Alert, automation.AlertParams{Answer: &dismiss}, &alerts)
	shows(`text:"Answer: No"`)
	if len(alerts.Alerts) != 0 {
		t.Fatalf("a dismissed alert still shows: %+v", alerts)
	}
	// The simulator app took its token out of its environment on startup.
	shows(`text:"Token in the environment: false"`)
	rowY := func() float64 {
		t.Helper()
		var found automation.FindResult
		must(automation.Find, automation.FindParams{Selector: "id:row-60"}, &found)
		return found.Elements[0].Bounds[1]
	}
	before := rowY()
	must(automation.Scroll, automation.ScrollParams{Selector: "id:list", By: 1500}, nil)
	if moved := before - rowY(); moved < 500 {
		t.Fatalf("the list scrolled %g points of 1500", moved)
	}
	var shot automation.ScreenshotResult
	must(automation.Screenshot, automation.ScreenshotParams{}, &shot)
	if shot.Width < int(ws.Windows[0].Width) || shot.Height < int(ws.Windows[0].Height) || len(shot.PNG) == 0 {
		t.Fatalf("a %dx%d screenshot of a %gx%g surface", shot.Width, shot.Height, ws.Windows[0].Width, ws.Windows[0].Height)
	}
}
