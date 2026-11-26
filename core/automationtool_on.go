//go:build nsgoautomation || nsgotest || nsgocheck

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/nsgo-dev/nsgo/internal/automation"
)

type automationToolRegistry struct {
	tools map[string]ownedAutomationTool
}

type ownedAutomationTool struct {
	scope *Scope
	tool  AutomationTool
}

// RegisterAutomationTool is automation's registration for a graph and scope.
func RegisterAutomationTool(cx *Scope, tool AutomationTool) error {
	if cx == nil || cx.disposed {
		return errors.New("automation: registration needs a live scope")
	}
	cx.rt.check()
	if err := automation.ValidateAppTool(automation.AppTool{Name: tool.Name, InputSchema: tool.InputSchema}); err != nil {
		return err
	}
	if tool.Handler == nil {
		return fmt.Errorf("automation: tool %q needs a handler", tool.Name)
	}
	r := &cx.rt.automation
	if _, exists := r.tools[tool.Name]; exists {
		return fmt.Errorf("automation: tool %q is already registered", tool.Name)
	}
	if r.tools == nil {
		r.tools = map[string]ownedAutomationTool{}
	}
	tool.InputSchema = bytes.Clone(tool.InputSchema)
	r.tools[tool.Name] = ownedAutomationTool{scope: cx, tool: tool}
	cx.addCleanup(func() { delete(r.tools, tool.Name) })
	return nil
}

// CallAutomationTool is automation's call on the graph goroutine.
func CallAutomationTool(cx *Scope, ctx context.Context, name string, args json.RawMessage) (result json.RawMessage, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cx == nil || cx.disposed {
		return nil, errors.New("automation: call needs a live scope")
	}
	cx.rt.check()
	if cx.rt.aborted {
		return nil, errors.New("automation: app graph has stopped")
	}
	registered, ok := cx.rt.automation.tools[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", automation.ErrUnknownAppTool, name)
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("%w for tool %q", automation.ErrAppToolInput, name)
	}
	cx.rt.run(registered.scope, func() {
		defer func() {
			if failure := recover(); failure != nil {
				err = fmt.Errorf("automation: tool %q panicked: %v", name, failure)
			}
		}()
		value, failure := registered.tool.Handler(ctx, args)
		if failure != nil {
			err = failure
			return
		}
		result, err = json.Marshal(value)
	})
	return result, err
}

func (s *devSurface) AutomationTools() []automation.AppTool {
	tools := s.t.rt.automation.tools
	out := make([]automation.AppTool, 0, len(tools))
	for _, name := range slices.Sorted(maps.Keys(tools)) {
		t := tools[name].tool
		out = append(out, automation.AppTool{Name: name, Description: t.Description, InputSchema: bytes.Clone(t.InputSchema)})
	}
	return out
}

func (s *devSurface) CallAutomationTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	return CallAutomationTool(s.t.root, ctx, name, args)
}
