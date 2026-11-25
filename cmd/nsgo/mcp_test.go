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
	calls      []string
	png        []byte
	appTools   []automation.AppTool
	appResults map[string]json.RawMessage
}

func (f *fakeBridge) call(_ context.Context, method string, params json.RawMessage) (json.RawMessage, *automation.Error) {
	f.calls = append(f.calls, method+" "+string(params))
	if raw, ok := f.appResults[method]; ok {
		return raw, nil
	}
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

func TestMCPAppToolCallBeforeList(t *testing.T) {
	b := &fakeBridge{}
	cs := connectMCP(t, b)
	ctx := t.Context()
	// The tool registers after the client connected, and the client calls it without listing first.
	b.appTools = []automation.AppTool{{Name: "set_count", Description: "Set the count", InputSchema: json.RawMessage(`{"type":"object"}`)}}
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "set_count", Arguments: map[string]int{"count": 3}})
	if err != nil || result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, `"count": 3`) {
		t.Fatalf("a call before any list: %+v, %v", result, err)
	}
	// A built-in tool needs no catalog read.
	n := len(b.calls)
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: automation.Status, Arguments: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(b.calls[n:], func(c string) bool { return strings.HasPrefix(c, automation.ListAppTools+" ") }) {
		t.Errorf("a built-in call read the catalog: %q", b.calls[n:])
	}
}

func TestMCPAppToolResults(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.White)
	var pic bytes.Buffer
	png.Encode(&pic, img)
	pngBytes := pic.Bytes()
	jpegBytes := []byte("fake-jpeg")

	encode := func(r automation.AppToolResult) json.RawMessage {
		t.Helper()
		raw, err := automation.EncodeAppToolResult(r)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	plainPNG, _ := json.Marshal(struct {
		PNG    []byte `json:"png"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}{PNG: pngBytes, Width: 2, Height: 1})

	schema := json.RawMessage(`{"type":"object"}`)
	b := &fakeBridge{
		appTools: []automation.AppTool{
			{Name: "image_only", Description: "Image only", InputSchema: schema},
			{Name: "text_and_images", Description: "Text and images", InputSchema: schema},
			{Name: "text_only", Description: "Text only", InputSchema: schema},
			{Name: "empty_result", Description: "Empty result", InputSchema: schema},
			{Name: "plain_png", Description: "Plain struct with png", InputSchema: schema},
			{Name: "corrupt_envelope", Description: "Corrupt envelope", InputSchema: schema},
		},
		appResults: map[string]json.RawMessage{
			"image_only":       encode(automation.AppToolResult{Images: []automation.AppToolImage{{Data: pngBytes}}}),
			"text_and_images":  encode(automation.AppToolResult{Text: "rendered", Images: []automation.AppToolImage{{Data: pngBytes, MIMEType: "image/png"}, {Data: jpegBytes, MIMEType: "image/jpeg"}}}),
			"text_only":        encode(automation.AppToolResult{Text: "just text"}),
			"empty_result":     encode(automation.AppToolResult{}),
			"plain_png":        plainPNG,
			"corrupt_envelope": json.RawMessage(`{"_nsgoAppResult":{"images":[{"data":123}]}}`),
		},
	}
	cs := connectMCP(t, b)
	ctx := t.Context()

	// Image-only result defaults empty MIMEType to image/png.
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "image_only", Arguments: map[string]any{}})
	if err != nil || res.IsError || len(res.Content) != 1 {
		t.Fatalf("image_only: %+v, %v", res, err)
	}
	ic, ok := res.Content[0].(*mcp.ImageContent)
	if !ok || ic.MIMEType != "image/png" || !bytes.Equal(ic.Data, pngBytes) {
		t.Fatalf("image_only content = %+v", res.Content[0])
	}

	// Text + multiple images preserves text first, then each image with its MIME type.
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "text_and_images", Arguments: map[string]any{}})
	if err != nil || res.IsError || len(res.Content) != 3 {
		t.Fatalf("text_and_images: %+v, %v", res, err)
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); !ok || tc.Text != "rendered" {
		t.Fatalf("text_and_images[0] = %+v", res.Content[0])
	}
	if ic, ok := res.Content[1].(*mcp.ImageContent); !ok || ic.MIMEType != "image/png" || !bytes.Equal(ic.Data, pngBytes) {
		t.Fatalf("text_and_images[1] = %+v", res.Content[1])
	}
	if ic, ok := res.Content[2].(*mcp.ImageContent); !ok || ic.MIMEType != "image/jpeg" || !bytes.Equal(ic.Data, jpegBytes) {
		t.Fatalf("text_and_images[2] = %+v", res.Content[2])
	}

	// Text-only explicit Result emits a single TextContent without JSON quoting.
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "text_only", Arguments: map[string]any{}})
	if err != nil || res.IsError || len(res.Content) != 1 {
		t.Fatalf("text_only: %+v, %v", res, err)
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); !ok || tc.Text != "just text" {
		t.Fatalf("text_only content = %+v", res.Content[0])
	}

	// Empty explicit Result emits empty Content.
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "empty_result", Arguments: map[string]any{}})
	if err != nil || res.IsError || len(res.Content) != 0 {
		t.Fatalf("empty_result: %+v, %v", res, err)
	}

	// A plain struct with a png field cannot spoof an image content item: it returns indented JSON text.
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "plain_png", Arguments: map[string]any{}})
	if err != nil || res.IsError || len(res.Content) != 1 {
		t.Fatalf("plain_png: %+v, %v", res, err)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("plain_png returned %T instead of *mcp.TextContent", res.Content[0])
	}
	var gotPlain struct {
		PNG    []byte `json:"png"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &gotPlain); err != nil || !bytes.Equal(gotPlain.PNG, pngBytes) || gotPlain.Width != 2 {
		t.Fatalf("plain_png JSON text = %q, err = %v", tc.Text, err)
	}

	// A corrupt envelope returns a tool error.
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "corrupt_envelope", Arguments: map[string]any{}})
	if err != nil || !res.IsError {
		t.Fatalf("corrupt_envelope: %+v, %v", res, err)
	}
}
