package automation

import (
	"bytes"
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

// AppToolImage is the wire form of an image returned by an app tool.
type AppToolImage struct {
	Data     []byte `json:"data"`
	MIMEType string `json:"mimeType,omitempty"`
}

// AppToolResult is the wire form of an explicit app tool result.
type AppToolResult struct {
	Text   string         `json:"text,omitempty"`
	Images []AppToolImage `json:"images,omitempty"`
}

type appToolResultEnvelope struct {
	Result *AppToolResult `json:"_nsgoAppResult"`
}

// EncodeAppToolResult marshals r into the internal protocol's tagged form.
func EncodeAppToolResult(r AppToolResult) ([]byte, error) {
	return json.Marshal(appToolResultEnvelope{Result: &r})
}

// DecodeAppToolResult decodes data when it is an encoded AppToolResult.
func DecodeAppToolResult(data []byte) (AppToolResult, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || len(fields) != 1 {
		return AppToolResult{}, false, nil
	}
	raw, ok := fields["_nsgoAppResult"]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return AppToolResult{}, false, nil
	}
	var inner map[string]json.RawMessage
	if err := json.Unmarshal(raw, &inner); err != nil || inner == nil {
		return AppToolResult{}, false, nil
	}
	for key := range inner {
		if key != "text" && key != "images" {
			return AppToolResult{}, false, nil
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var r AppToolResult
	if err := dec.Decode(&r); err != nil {
		return AppToolResult{}, false, err
	}
	return r, true, nil
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
