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
	"strings"

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
		switch v := value.(type) {
		case AutomationResult:
			result, err = encodeAutomationResult(v)
		case *AutomationResult:
			if v == nil {
				result, err = json.Marshal(nil)
			} else {
				result, err = encodeAutomationResult(*v)
			}
		default:
			result, err = json.Marshal(value)
		}
	})
	return result, err
}

func encodeAutomationResult(r AutomationResult) (json.RawMessage, error) {
	wire := automation.AppToolResult{Text: r.Text}
	if len(r.Images) > 0 {
		wire.Images = make([]automation.AppToolImage, len(r.Images))
		for i, img := range r.Images {
			if len(img.Data) == 0 {
				return nil, fmt.Errorf("automation: image %d has no data", i)
			}
			if img.MIMEType != "" && !strings.HasPrefix(img.MIMEType, "image/") {
				return nil, fmt.Errorf("automation: image %d has MIME type %q, not an image/ type", i, img.MIMEType)
			}
			wire.Images[i] = automation.AppToolImage{Data: img.Data, MIMEType: img.MIMEType}
		}
	}
	return automation.EncodeAppToolResult(wire)
}

// DecodeAutomationResult is automation's decoder for explicit results returned by CallAutomationTool.
func DecodeAutomationResult(raw json.RawMessage) (AutomationResult, bool, error) {
	wire, ok, err := automation.DecodeAppToolResult(raw)
	if err != nil || !ok {
		return AutomationResult{}, ok, err
	}
	res := AutomationResult{Text: wire.Text}
	if len(wire.Images) > 0 {
		res.Images = make([]AutomationImage, len(wire.Images))
		for i, img := range wire.Images {
			res.Images[i] = AutomationImage{Data: img.Data, MIMEType: img.MIMEType}
		}
	}
	return res, true, nil
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
