package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// ListAppTools asks for the running graph's app-defined tools.
const ListAppTools = "_nsgo/list-app-tools"

// AppToolListTimeout bounds discovery while the graph is busy.
const AppToolListTimeout = 250 * time.Millisecond

// AppTool is the published description of a tool the app registered.
type AppTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

var (
	ErrUnknownAppTool = errors.New("automation: unknown app tool")
	ErrAppToolInput   = errors.New("automation: expected a JSON object")
	appToolName       = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,128}$`)
)

// ValidateAppTool checks the published name and the MCP input shape.
func ValidateAppTool(tool AppTool) error {
	if !appToolName.MatchString(tool.Name) {
		return fmt.Errorf("automation: invalid tool name %q", tool.Name)
	}
	if Params[tool.Name] != nil {
		return fmt.Errorf("automation: tool name %q belongs to nsgo mcp", tool.Name)
	}
	var schema map[string]json.RawMessage
	var typ string
	if json.Unmarshal(tool.InputSchema, &schema) != nil || json.Unmarshal(schema["type"], &typ) != nil || typ != "object" {
		return fmt.Errorf("automation: tool %q needs an object input schema", tool.Name)
	}
	return nil
}
