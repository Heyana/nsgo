// Package automation exposes tools from a Go app or library to nsgo mcp.
package automation

import (
	"context"
	"encoding/json"

	"github.com/nsgo-dev/nsgo/core"
)

// Tool describes an app tool. InputSchema is a JSON Schema object with
// type "object". Handler runs on the owning scope's graph goroutine.
// It reads a JSON object and returns a JSON-encodable result or an error.
type Tool = core.AutomationTool

// Register gives a tool to cx's graph until cx is cleaned up.
// Names are unique on the graph and cannot replace built-in MCP tools.
// Call it on the graph goroutine. Registration is active in automation and test builds.
func Register(cx *core.Scope, tool Tool) error { return core.RegisterAutomationTool(cx, tool) }

// Call runs a registered tool on cx's graph and returns its JSON result.
// Call it on the graph goroutine, such as inside nsgotest.App.Post.
func Call(cx *core.Scope, ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	return core.CallAutomationTool(cx, ctx, name, args)
}
