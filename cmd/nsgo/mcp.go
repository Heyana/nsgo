package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nsgo-dev/nsgo/internal/automation"
)

// nsgo mcp: an MCP server on standard input and output, so an agent drives the app nsgo dev -automation runs for the directory: the one its session file names, else one it starts with mcp's flags and stops when the client goes. Each tool is a method of package internal/automation, its input that method's params.

// mcpInstructions is what the server tells clients.
const mcpInstructions = `Drives the NSGo app of this directory as it runs under nsgo dev -automation, on macOS and Apple devices and simulators (iOS, tvOS and watchOS), as a user does: native pointer, touch, accessibility and key events, the real menus and alerts.

Start with windows, then snapshot: it outlines what a window shows, an element a line, such as - button "Add" [view:12] id=add focused. Name what to act on with a selector: id:<test ID>, label:<label>, text:<text>, role:<role> <name>, or the [view:N] ref a snapshot printed; a >> b looks inside a, and >> nth=0 keeps the first match. Act with press, type, key, scroll, drag, hover, menu and alert, passing snapshot: true to see the window after the step, and wait for what work on other goroutines shows. A step answers not settled when the app kept changing for 5 seconds.

Saving a Go file rebuilds and restarts the app: status says whether it builds and its last build error, and its run count grows with each start, which makes earlier view refs stale. logs reads the app's output, reload builds and starts it again, and screenshot captures a window as a PNG. The app runs with its real data.`

// mcpBridge reaches nsgo dev -automation: it sends an automation request and returns its result.
type mcpBridge interface {
	call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *automation.Error)
}

// mcpTools are the tools nsgo mcp serves, with what each does.
var mcpTools = []struct {
	name, doc string
	add       func(*mcp.Server, mcpBridge, string, string)
}{
	{automation.Status, "Whether the app builds, its last build error, whether it runs, and its run count", addMCPTool[automation.StatusParams]},
	{automation.Windows, "The app's windows: IDs, titles, sizes, and the default one requests act on", addMCPTool[automation.WindowsParams]},
	{automation.Snapshot, "An outline of what a window shows, an element a line, with each element's view ref", addMCPTool[automation.SnapshotParams]},
	{automation.Find, "The elements a selector matches: refs, roles, names, texts, states and frames", addMCPTool[automation.FindParams]},
	{automation.Press, "Press what a selector names, as a click does; long or right with mode", addMCPTool[automation.PressParams]},
	{automation.Type, "Type into the text field a selector names, optionally clearing it first and pressing Return after", addMCPTool[automation.TypeParams]},
	{automation.Key, "Press a key or a shortcut, such as Escape or mod+k, in the focused view or a field", addMCPTool[automation.KeyParams]},
	{automation.Scroll, "Scroll the scroll view a selector names, or the one around it, by points or to an offset", addMCPTool[automation.ScrollParams]},
	{automation.Drag, "Drag from what a selector names through points given as offsets", addMCPTool[automation.DragParams]},
	{automation.Hover, "Move the mouse over what a selector names", addMCPTool[automation.HoverParams]},
	{automation.Menu, "Choose an application menu item by its labels, such as File, New, or an item of the context menu showing", addMCPTool[automation.MenuParams]},
	{automation.Alert, "List the alerts showing, or answer one by its button's index", addMCPTool[automation.AlertParams]},
	{automation.Wait, "Wait until a selector matches a view shown, or until nothing does", addMCPTool[automation.WaitParams]},
	{automation.Screenshot, "Capture a window as a PNG", addMCPTool[automation.ScreenshotParams]},
	{automation.Logs, "The app's latest output lines", addMCPTool[automation.LogsParams]},
	{automation.Reload, "Build the app and start it again", addMCPTool[automation.ReloadParams]},
}

// newMCPServer returns the MCP server answering through b.
func newMCPServer(b mcpBridge) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "nsgo", Version: toolVersion()}, &mcp.ServerOptions{Instructions: mcpInstructions})
	for _, t := range mcpTools {
		t.add(s, b, t.name, t.doc)
	}
	return s
}

// addMCPTool adds the tool of method, whose input is the method's params In.
func addMCPTool[In any](s *mcp.Server, b mcpBridge, method, doc string) {
	mcp.AddTool(s, &mcp.Tool{Name: method, Description: doc}, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		params, err := json.Marshal(in)
		if err != nil {
			return nil, nil, err
		}
		result, aerr := b.call(ctx, method, params)
		if aerr != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: aerr.Error()}}}, nil, nil
		}
		return mcpResult(method, result), nil, nil
	})
}

// mcpResult is a method's result as a tool's: a screenshot as an image, an outline as its text, a step as what it left, and the rest as JSON.
func mcpResult(method string, result json.RawMessage) *mcp.CallToolResult {
	text := func(s string) *mcp.CallToolResult {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
	}
	switch method {
	case automation.Screenshot:
		var r automation.ScreenshotResult
		if json.Unmarshal(result, &r) == nil {
			return &mcp.CallToolResult{Content: []mcp.Content{
				&mcp.ImageContent{Data: r.PNG, MIMEType: "image/png"},
				&mcp.TextContent{Text: fmt.Sprintf("%d×%d pixels", r.Width, r.Height)},
			}}
		}
	case automation.Snapshot:
		var r automation.SnapshotResult
		if json.Unmarshal(result, &r) == nil {
			return text(fmt.Sprintf("window %d\n%s", r.Window, r.Text))
		}
	case automation.Press, automation.Type, automation.Key, automation.Scroll, automation.Drag, automation.Hover, automation.Menu, automation.Wait:
		var r automation.StepResult
		if json.Unmarshal(result, &r) == nil {
			s := "done"
			if !r.Settled {
				s = "done; the app was still changing 5 seconds later"
			}
			if r.Snapshot != "" {
				s += "\n\n" + r.Snapshot
			}
			return text(s)
		}
	}
	var out bytes.Buffer
	if json.Indent(&out, result, "", "  ") != nil {
		return text(string(result))
	}
	return text(out.String())
}

// serveMCP serves MCP on standard input and output for the app in p.Dir, with args, mcp's arguments, for the nsgo dev -automation it starts when none runs.
func serveMCP(ctx context.Context, p *project, args []string) error {
	// The transport owns standard output: whatever else prints goes to standard error.
	stdout := os.Stdout
	os.Stdout = os.Stderr
	defer func() { os.Stdout = stdout }()
	b := &sessionBridge{dir: p.Dir, args: args}
	defer b.stop()
	if _, live := b.live(); !live {
		if err := b.start(); err != nil {
			return err
		}
	}
	return newMCPServer(b).Run(ctx, &mcp.IOTransport{Reader: os.Stdin, Writer: stdout})
}

// sessionBridge reaches the nsgo dev -automation the session file in dir names, starting one with args when none runs.
type sessionBridge struct {
	dir  string
	args []string

	mu    sync.Mutex
	child *exec.Cmd
	done  chan struct{}
	err   error
}

// sessionWait is how long a tool waits for nsgo dev -automation to write its session before it answers that the app is not up yet.
const sessionWait = 90 * time.Second

// live returns the session of a running nsgo dev -automation for the directory.
func (b *sessionBridge) live() (*automationSession, bool) {
	s, err := readSession(b.dir)
	if err != nil || !sessionLive(s) {
		return nil, false
	}
	return s, true
}

// start starts nsgo dev -automation with the bridge's arguments, its output on standard error.
func (b *sessionBridge) start() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.child != nil {
		select {
		case <-b.done:
		default:
			return nil
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, append([]string{"dev", "-automation"}, b.args...)...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting nsgo dev -automation: %w", err)
	}
	done := make(chan struct{})
	b.child, b.done = cmd, done
	go func() {
		err := cmd.Wait()
		b.mu.Lock()
		b.err = err
		b.mu.Unlock()
		close(done)
	}()
	return nil
}

// stop stops the nsgo dev the bridge started, as Control+C does.
func (b *sessionBridge) stop() {
	b.mu.Lock()
	cmd, done := b.child, b.done
	b.mu.Unlock()
	if cmd == nil {
		return
	}
	cmd.Process.Signal(os.Interrupt)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		<-done
	}
}

// session returns the running session, waiting while the nsgo dev the bridge started comes up.
func (b *sessionBridge) session(ctx context.Context) (*automationSession, *automation.Error) {
	deadline := time.Now().Add(sessionWait)
	for {
		if s, ok := b.live(); ok {
			return s, nil
		}
		b.mu.Lock()
		done, err := b.done, b.err
		b.mu.Unlock()
		if done == nil {
			if err := b.start(); err != nil {
				return nil, automation.Errorf(automation.NoApp, "%v", err)
			}
			continue
		}
		select {
		case <-done:
			return nil, automation.Errorf(automation.NoApp, "nsgo dev -automation ended (%v): its output is on standard error", err)
		default:
		}
		if time.Now().After(deadline) {
			return nil, automation.Errorf(automation.NoApp, "nsgo dev -automation has not come up after %v: it may still be building", sessionWait)
		}
		select {
		case <-ctx.Done():
			return nil, automation.Errorf(automation.NoApp, "%v", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (b *sessionBridge) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *automation.Error) {
	s, aerr := b.session(ctx)
	if aerr != nil {
		return nil, aerr
	}
	body, _ := json.Marshal(automation.Request{ID: 1, Method: method, Params: params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL+"/_nsgo/automation", bytes.NewReader(body))
	if err != nil {
		return nil, automation.Errorf(automation.Failed, "%v", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, automation.Errorf(automation.NoApp, "nsgo dev -automation does not answer: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, automation.Errorf(automation.NoApp, "nsgo dev -automation refused the request: %s", resp.Status)
	}
	var a automation.Answer
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return nil, automation.Errorf(automation.Failed, "nsgo dev -automation's answer: %v", err)
	}
	if a.Error != nil {
		return nil, a.Error
	}
	return a.Result, nil
}
