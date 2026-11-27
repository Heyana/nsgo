//go:build linux && !android

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nsgo-dev/nsgo/internal/automation"
)

// TestLinuxAutomationEndToEnd exercises both commands in a real GTK window. Run with NSGO_AUTOMATION_E2E=1 xvfb-run -a go tool nsgo test -run TestLinuxAutomationEndToEnd ./cmd/nsgo.
func TestLinuxAutomationEndToEnd(t *testing.T) {
	if os.Getenv("NSGO_AUTOMATION_E2E") == "" {
		t.Skip("set NSGO_AUTOMATION_E2E=1 in a Linux desktop session or under xvfb-run")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(filepath.Join(root, "testdata"), ".automation-linux-")
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

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	var output lockedBuffer
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("nsgo mcp output:\n%s", output.String())
		}
	})
	command := exec.CommandContext(ctx, "go", "tool", "nsgo", "mcp", "-target", "native", "-port", "0", dir)
	command.Dir = root
	command.Stderr = &output
	command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir, "XDG_DATA_HOME="+dir, "XDG_CACHE_HOME="+dir, "XDG_STATE_HOME="+dir)
	client := mcp.NewClient(&mcp.Implementation{Name: "linux-automation-test"}, nil)
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

	// The MCP process can accept requests while dev is still compiling.
	for {
		status := read(automation.Status, nil)
		var live automation.StatusResult
		if err := json.Unmarshal([]byte(status), &live); err != nil {
			t.Fatal(err)
		}
		if live.BuildError != "" {
			t.Fatalf("building the GTK app: %s", live.BuildError)
		}
		if live.App {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("the GTK app did not connect: %s", status)
		case <-time.After(100 * time.Millisecond):
		}
	}
	windows := read(automation.Windows, nil)
	var list automation.WindowsResult
	if err := json.Unmarshal([]byte(windows), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Windows) != 1 || !list.Windows[0].Default || list.Windows[0].Width <= 0 {
		t.Fatalf("windows: %+v", list.Windows)
	}
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
		t.Fatal("nsgo mcp on Linux did not list set_count")
	}
	read("set_count", map[string]any{"count": 3})
	show(`text:"Count: 3"`)
	read(automation.Menu, map[string]any{"path": []string{"Actions", "Reset"}})
	show(`text:"Count: 0"`)
	if snapshot := read(automation.Snapshot, nil); !strings.Contains(snapshot, "id=add") {
		t.Fatalf("snapshot:\n%s", snapshot)
	}
	read(automation.Press, map[string]any{"selector": "id:add"})
	show(`text:"Count: 1"`)
	read(automation.Type, map[string]any{"selector": "id:name", "text": "Ada"})
	show(`text:"Hello, Ada"`)
	read(automation.Key, map[string]any{"selector": "id:name", "key": "mod+a"})
	read(automation.Key, map[string]any{"selector": "id:name", "key": "Backspace"})
	show(`text:"Hello, "`)
	read(automation.Menu, map[string]any{"path": []string{"Actions", "Reset"}})
	show(`text:"Count: 0"`)
	read(automation.Press, map[string]any{"selector": "id:ask"})
	if alerts := read(automation.Alert, nil); !strings.Contains(alerts, "Keep going?") {
		t.Fatalf("alerts:\n%s", alerts)
	}
	read(automation.Alert, map[string]any{"answer": 0})
	show(`text:"Answer: Yes"`)
	show(`text:"Token in the environment: false"`)

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
	var logs automation.LogsResult
	if err := json.Unmarshal([]byte(read(automation.Logs, map[string]any{"lines": 20})), &logs); err != nil {
		t.Fatalf("logs: %v", err)
	}

	screenshot := call(automation.Screenshot, nil)
	shot, ok := screenshot[0].(*mcp.ImageContent)
	if !ok || shot.MIMEType != "image/png" {
		t.Fatalf("screenshot returned %T", screenshot[0])
	}
	size, err := png.DecodeConfig(bytes.NewReader(shot.Data))
	if err != nil || size.Width < int(list.Windows[0].Width) || size.Height < int(list.Windows[0].Height) {
		t.Fatalf("screenshot size %dx%d of window %+v: %v", size.Width, size.Height, list.Windows[0], err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readSession(dir); !os.IsNotExist(err) {
		t.Fatalf("the dev session did not end with MCP: %v", err)
	}
}
