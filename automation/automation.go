// Package automation exposes tools from a Go app or library to nsgo mcp.
package automation

import (
	"context"
	"encoding/json"

	"github.com/nsgo-dev/nsgo/core"
)

// Tool describes an app tool. InputSchema is a JSON Schema object with
// type "object". Set exactly one of Handler and Start. Both run on the
// owning scope's graph goroutine, receive a JSON object and answer with
// a JSON-encodable value or an error.
//
// Handler returns its answer directly. It must not wait for work posted
// back to the graph. Start returns without waiting and calls reply when
// its answer is ready, from any goroutine. Replies after the first reply,
// cancellation or scope cleanup are ignored. Start's context is canceled
// when the call finishes, including on scope cleanup.
type Tool = core.AutomationTool

// Register gives a tool to cx's graph until cx is cleaned up.
// Names are unique on the graph and cannot replace built-in MCP tools.
// Call it on the graph goroutine. Registration is active in automation and test builds.
func Register(cx *core.Scope, tool Tool) error { return core.RegisterAutomationTool(cx, tool) }

// Call runs a synchronous (Handler) tool on cx's graph and returns its JSON
// result. Use CallAsync for a deferred (Start) tool.
// Call it on the graph goroutine, such as inside nsgotest.App.Post.
func Call(cx *core.Scope, ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	return core.CallAutomationTool(cx, ctx, name, args)
}

// CallAsync starts a registered tool on cx's graph and delivers its JSON
// result or error to done once. Call it on the graph goroutine, such as
// inside nsgotest.App.Post. Use ctx to bound the wait; cleaning up the
// registering scope ends a pending call with an error.
//
// The non-nil done callback may run before CallAsync returns or later, on
// any goroutine. It must not block or panic. Post back to the graph before
// accessing graph state from done.
func CallAsync(cx *core.Scope, ctx context.Context, name string, args json.RawMessage, done func(json.RawMessage, error)) {
	core.CallAutomationToolAsync(cx, ctx, name, args, done)
}
