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
	Handler     func(context.Context, json.RawMessage) (any, error)
}
