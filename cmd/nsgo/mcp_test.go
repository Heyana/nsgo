package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nsgo-dev/nsgo/internal/automation"
)

// fakeBridge answers as nsgo dev -automation would, recording the calls.
type fakeBridge struct {
	calls    []string
	png      []byte
	appTools []automation.AppTool
}

func (f *fakeBridge) call(_ context.Context, method string, params json.RawMessage) (json.RawMessage, *automation.Error) {
	f.calls = append(f.calls, method+" "+string(params))
	var result any
	switch method {
	case automation.ListAppTools:
		result = f.appTools
	case "set_count":
		result = params
	case automation.Screenshot:
		result = automation.ScreenshotResult{PNG: f.png, Width: 2, Height: 1}
	case automation.Press:
		var p automation.PressParams
		json.Unmarshal(params, &p)
		if p.Selector == "id:none" {
			return nil, automation.Errorf(automation.NoMatch, "no view shown matches %q", p.Selector)
		}
		result = automation.StepResult{Settled: true, Snapshot: "- button \"Add\" [view:3]"}
	default:
		result = automation.StatusResult{Target: "native", App: true, Run: 1}
	}
	data, _ := json.Marshal(result)
	return data, nil
}

// connectMCP connects a client to an nsgo mcp server answering through b.
func connectMCP(t *testing.T, b mcpBridge) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	server, _ := newMCPServer(b)
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func TestMCPSessionBridge(t *testing.T) {
	h := startHub(t)
	dir := t.TempDir()
	if err := h.writeSession(dir); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if code := <-fakeApp(ctx, t, h, func(req automation.Request) automation.Answer {
		if req.Method != automation.Snapshot {
			return automation.Reply(req.ID, nil, automation.Errorf(automation.InvalidRequest, "unexpected method %s", req.Method))
		}
		return automation.Reply(req.ID, automation.SnapshotResult{Text: "- button \"Add\" [view:3]"}, nil)
	}); code != http.StatusOK {
		t.Fatalf("app connection: %d", code)
	}
	b := &sessionBridge{dir: dir}
	if s, ok := b.live(); !ok || s.Token != h.bridgeToken {
		t.Fatalf("running session was not found: %+v, %v", s, ok)
	}
	cs := connectMCP(t, b)
	for _, method := range []string{automation.Status, automation.Snapshot} {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: method, Arguments: map[string]any{}})
		if err != nil || result.IsError {
			t.Fatalf("%s: %+v, %v", method, result, err)
		}
		if method == automation.Snapshot && result.Content[0].(*mcp.TextContent).Text != "window 0\n- button \"Add\" [view:3]" {
			t.Errorf("snapshot: %+v", result.Content)
		}
	}
	if b.child != nil {
		t.Error("MCP started another dev process instead of reusing the session")
	}
	s, err := readSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Token = "wrong-token"
	if sessionLive(s) {
		t.Error("a live PID bypassed the session token check")
	}
}

func TestMCPTools(t *testing.T) {
	cs := connectMCP(t, &fakeBridge{})
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		if tool.Name == automation.Press {
			schema, _ := json.Marshal(tool.InputSchema)
			if !strings.Contains(string(schema), "an nsgotest selector") || !strings.Contains(string(schema), `"required":["selector"]`) {
				t.Errorf("press's input schema: %s", schema)
			}
		}
	}
	slices.Sort(names)
	want := []string{"alert", "drag", "find", "hover", "key", "logs", "menu", "press", "reload", "screenshot", "scroll", "snapshot", "status", "type", "wait", "windows"}
	if !slices.Equal(names, want) {
		t.Errorf("tools %v, want %v", names, want)
	}
	if !strings.Contains(cs.InitializeResult().Instructions, "selector") {
		t.Error("the server tells clients nothing of selectors")
	}
}

func TestMCPResults(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.White)
	var pic bytes.Buffer
	png.Encode(&pic, img)
	b := &fakeBridge{png: pic.Bytes()}
	cs := connectMCP(t, b)
	ctx := context.Background()
	shot, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: automation.Screenshot, Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	shown, ok := shot.Content[0].(*mcp.ImageContent)
	if !ok || shown.MIMEType != "image/png" {
		t.Fatalf("screenshot %+v", shot.Content)
	}
	if cfg, err := png.DecodeConfig(bytes.NewReader(shown.Data)); err != nil || cfg.Width != 2 {
		t.Fatalf("the screenshot's image: %+v %v", cfg, err)
	}
	press, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: automation.Press, Arguments: map[string]any{"selector": "id:add", "snapshot": true}})
	if err != nil || press.IsError {
		t.Fatalf("press: %+v %v", press, err)
	}
	if text := press.Content[0].(*mcp.TextContent).Text; !strings.HasPrefix(text, "done\n\n- button") {
		t.Errorf("press answered %q", text)
	}
	if !slices.Contains(b.calls, `press {"selector":"id:add","snapshot":true}`) {
		t.Errorf("the bridge was asked %q", b.calls)
	}
	failed, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: automation.Press, Arguments: map[string]any{"selector": "id:none"}})
	if err != nil || !failed.IsError || !strings.HasPrefix(failed.Content[0].(*mcp.TextContent).Text, "no_match: ") {
		t.Fatalf("a failed press: %+v %v", failed, err)
	}
	// The schema requires a selector: the SDK refuses the call before the bridge sees it.
	n := len(b.calls)
	if r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: automation.Press, Arguments: map[string]any{}}); err == nil && !r.IsError || len(b.calls) != n {
		t.Errorf("a press without a selector was taken: %q", b.calls)
	}
}

func TestMCPAppToolCatalog(t *testing.T) {
	b := &fakeBridge{}
	cs := connectMCP(t, b)
	ctx := t.Context()
	tool := automation.AppTool{
		Name: "set_count", Description: "Set the count",
		InputSchema: json.RawMessage(`{"type":"object","required":["count"]}`),
	}
	list := func() []*mcp.Tool {
		t.Helper()
		result, err := cs.ListTools(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		return result.Tools
	}
	has := func(name, description string) bool {
		for _, tool := range list() {
			if tool.Name == name && tool.Description == description {
				return true
			}
		}
		return false
	}
	if has(tool.Name, tool.Description) {
		t.Fatal("the server listed an unregistered tool")
	}
	b.appTools = []automation.AppTool{tool, {
		Name: automation.Status, Description: "Replace status",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, {
		Name: "bad_header", Description: "A schema with an invalid header",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","x-mcp-header":"bad header"}}}`),
	}}
	if !has(tool.Name, tool.Description) || has(automation.Status, "Replace status") || has("bad_header", "A schema with an invalid header") {
		t.Fatal("the tool catalog was not added alongside the built-in tools")
	}
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool.Name, Arguments: map[string]int{"count": 3}})
	if err != nil || result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, `"count": 3`) {
		t.Fatalf("app tool call: %+v, %v", result, err)
	}
	b.appTools[0].Description = "Change the count"
	if !has(tool.Name, "Change the count") {
		t.Fatal("the changed tool definition was not published")
	}
	b.appTools = nil
	if has(tool.Name, "Change the count") {
		t.Fatal("the removed tool is still published")
	}
}
