package core

import (
	"context"
	"encoding/json"
)

// AutomationImage is automation's image returned by an app-owned MCP tool.
type AutomationImage struct {
	Data     []byte
	MIMEType string
}

// AutomationResult is automation's text and image result returned by an app-owned MCP tool.
type AutomationResult struct {
	Text   string
	Images []AutomationImage
}

// AutomationTool is automation's declaration of an app-owned MCP tool.
type AutomationTool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Handler     func(context.Context, json.RawMessage) (any, error)
}
