package main

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nsgo-dev/nsgo/internal/automation"
)

type mcpAppTools struct {
	server *mcp.Server
	bridge mcpBridge
	mu     sync.Mutex
	tools  map[string]automation.AppTool
}

func (a *mcpAppTools) receive(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method == "tools/list" {
			a.refresh(ctx)
		}
		return next(ctx, method, req)
	}
}

// watch notices registrations after startup and changes from a rebuild, even
// if the client waits for a list-change notification before listing again.
func (a *mcpAppTools) watch(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			a.refresh(ctx)
		}
	}
}

func (a *mcpAppTools) refresh(ctx context.Context) {
	if !a.mu.TryLock() {
		return
	}
	defer a.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 3*automation.AppToolListTimeout)
	defer cancel()
	data, err := a.bridge.call(ctx, automation.ListAppTools, nil)
	var catalog []automation.AppTool
	switch {
	case err != nil && err.Code == automation.NoApp:
		// The session ended, so its tools are gone.
	case err != nil || json.Unmarshal(data, &catalog) != nil:
		// Keep the current list while the app is busy.
		return
	}
	tools := make(map[string]automation.AppTool, len(catalog))
	for _, tool := range catalog {
		if automation.ValidateAppTool(tool) != nil {
			continue
		}
		prior, exists := a.tools[tool.Name]
		if !exists || prior.Description != tool.Description || !bytes.Equal(prior.InputSchema, tool.InputSchema) {
			if !a.add(tool) {
				continue
			}
		}
		tools[tool.Name] = tool
	}
	var removed []string
	for name := range a.tools {
		if _, ok := tools[name]; !ok {
			removed = append(removed, name)
		}
	}
	if len(removed) > 0 {
		a.server.RemoveTools(removed...)
	}
	a.tools = tools
}

func (a *mcpAppTools) add(tool automation.AppTool) (added bool) {
	// The SDK panics on invalid MCP-specific schema annotations. Keep the
	// catalog available when an app supplies one.
	defer func() {
		if recover() != nil {
			added = false
		}
	}()
	a.server.AddTool(&mcp.Tool{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			result, err := a.bridge.call(ctx, tool.Name, req.Params.Arguments)
			if err != nil {
				var r mcp.CallToolResult
				r.SetError(err)
				return &r, nil
			}
			return mcpResult(tool.Name, result), nil
		})
	return true
}
