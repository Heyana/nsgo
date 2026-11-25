// Package automation exposes tools from a Go app or library to nsgo mcp.
package automation

import (
	"context"
	"encoding/json"

	"github.com/nsgo-dev/nsgo/core"
)

// Tool describes an app tool. InputSchema is a JSON Schema object with
// type "object". Handler runs on the owning scope's graph goroutine.
// It reads a JSON object and returns a JSON-encodable value, Result, *Result,
// or an error.
type Tool = core.AutomationTool

// Image is an image returned by an app tool to nsgo mcp.
// An empty MIMEType defaults to "image/png".
type Image = core.AutomationImage

// Result is an explicit text and image result returned by an app tool to nsgo mcp.
type Result = core.AutomationResult

// Register gives a tool to cx's graph until cx is cleaned up.
// Names are unique on the graph and cannot replace built-in MCP tools.
// Call it on the graph goroutine. Registration is active in automation and test builds.
func Register(cx *core.Scope, tool Tool) error { return core.RegisterAutomationTool(cx, tool) }

// Call runs a registered tool on cx's graph and returns its raw result.
// Call it on the graph goroutine, such as inside nsgotest.App.Post.
func Call(cx *core.Scope, ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	return core.CallAutomationTool(cx, ctx, name, args)
}

// DecodeResult decodes a Result returned by Call, reporting whether raw
// carried an explicit Result rather than an ordinary JSON value.
func DecodeResult(raw json.RawMessage) (Result, bool, error) {
	return core.DecodeAutomationResult(raw)
}
