package core

import (
	"context"
	"encoding/json"
)

// AutomationTool is automation's declaration of an app-owned MCP tool.
type AutomationTool struct {
	Name        string
	Description string
	InputSchema json.RawMessage

	// Set exactly one of Handler and Start.
	// Handler returns its result on the owning scope's graph goroutine.
	Handler func(context.Context, json.RawMessage) (any, error)

	// Start begins work on the owning scope's graph goroutine and returns
	// without waiting. Call reply with a JSON-encodable value or an error
	// from any goroutine. Only the first reply before cancellation or
	// scope cleanup is accepted.
	Start func(ctx context.Context, args json.RawMessage, reply func(any, error))
}
