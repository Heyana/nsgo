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
	"sync"

	"github.com/nsgo-dev/nsgo/internal/automation"
)

type automationToolRegistry struct {
	tools map[string]ownedAutomationTool
}

type ownedAutomationTool struct {
	scope    *Scope
	tool     AutomationTool
	deferred *deferredAutomationTool
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
	if (tool.Handler == nil) == (tool.Start == nil) {
		return fmt.Errorf("automation: tool %q needs exactly one of Handler and Start", tool.Name)
	}
	r := &cx.rt.automation
	if _, exists := r.tools[tool.Name]; exists {
		return fmt.Errorf("automation: tool %q is already registered", tool.Name)
	}
	if r.tools == nil {
		r.tools = map[string]ownedAutomationTool{}
	}
	tool.InputSchema = bytes.Clone(tool.InputSchema)
	registered := ownedAutomationTool{scope: cx, tool: tool}
	if tool.Start != nil {
		registered.deferred = &deferredAutomationTool{}
	}
	r.tools[tool.Name] = registered
	cx.addCleanup(func() {
		delete(r.tools, tool.Name)
		if registered.deferred != nil {
			registered.deferred.dispose(tool.Name)
		}
	})
	return nil
}

// CallAutomationTool is automation's synchronous call on the graph goroutine.
func CallAutomationTool(cx *Scope, ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	registered, args, err := lookupAutomationTool(cx, ctx, name, args)
	if err != nil {
		return nil, err
	}
	if registered.deferred != nil {
		return nil, fmt.Errorf("automation: tool %q is deferred; use automation.CallAsync", name)
	}
	return registered.call(ctx, args)
}

// CallAutomationToolAsync is automation's non-waiting call on the graph goroutine.
func CallAutomationToolAsync(cx *Scope, ctx context.Context, name string, args json.RawMessage, done func(json.RawMessage, error)) {
	registered, args, err := lookupAutomationTool(cx, ctx, name, args)
	if err != nil {
		done(nil, err)
		return
	}
	if registered.deferred == nil {
		done(registered.call(ctx, args))
		return
	}
	registered.start(ctx, args, done)
}

func lookupAutomationTool(cx *Scope, ctx context.Context, name string, args json.RawMessage) (ownedAutomationTool, json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return ownedAutomationTool{}, nil, err
	}
	if cx == nil || cx.disposed {
		return ownedAutomationTool{}, nil, errors.New("automation: call needs a live scope")
	}
	cx.rt.check()
	if cx.rt.aborted {
		return ownedAutomationTool{}, nil, errors.New("automation: app graph has stopped")
	}
	registered, ok := cx.rt.automation.tools[name]
	if !ok {
		return ownedAutomationTool{}, nil, fmt.Errorf("%w: %q", automation.ErrUnknownAppTool, name)
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil || fields == nil {
		return ownedAutomationTool{}, nil, fmt.Errorf("%w for tool %q", automation.ErrAppToolInput, name)
	}
	return registered, args, nil
}

func (registered ownedAutomationTool) call(ctx context.Context, args json.RawMessage) (result json.RawMessage, err error) {
	registered.scope.rt.run(registered.scope, func() {
		defer catchAutomationToolPanic(registered.tool.Name, &err)
		value, failure := registered.tool.Handler(ctx, args)
		if failure != nil {
			err = failure
			return
		}
		result, err = json.Marshal(value)
	})
	return result, err
}

func catchAutomationToolPanic(name string, err *error) {
	if failure := recover(); failure != nil {
		*err = fmt.Errorf("automation: tool %q panicked: %v", name, failure)
	}
}

// One cleanup per registration owns the outstanding calls. The mutex protects
// their state, not the graph, and is never held over app code or delivery.
type deferredAutomationTool struct {
	mu       sync.Mutex
	pending  map[*pendingAutomationCall]struct{}
	disposed error
}

type pendingAutomationCall struct {
	owner *deferredAutomationTool
	name  string
	ctx   context.Context

	cancel   context.CancelFunc
	stop     func() bool
	done     func(json.RawMessage, error)
	replying bool
}

func (registered ownedAutomationTool) start(ctx context.Context, args json.RawMessage, done func(json.RawMessage, error)) {
	r := registered.deferred
	callCtx, cancel := context.WithCancel(ctx)
	p := &pendingAutomationCall{owner: r, name: registered.tool.Name, ctx: ctx, cancel: cancel, done: done}
	r.mu.Lock()
	if r.pending == nil {
		r.pending = make(map[*pendingAutomationCall]struct{})
	}
	r.pending[p] = struct{}{}
	if ctx.Done() != nil {
		p.stop = context.AfterFunc(ctx, func() { p.finish(nil, ctx.Err()) })
	}
	r.mu.Unlock()

	var err error
	registered.scope.rt.run(registered.scope, func() {
		defer catchAutomationToolPanic(p.name, &err)
		if err = callCtx.Err(); err == nil {
			registered.tool.Start(callCtx, args, p.reply)
		}
	})
	if err != nil {
		p.finish(nil, err)
	}
}

func (p *pendingAutomationCall) reply(value any, err error) {
	r := p.owner
	r.mu.Lock()
	if p.done == nil || p.replying || r.disposed != nil || p.ctx.Err() != nil {
		r.mu.Unlock()
		return
	}
	p.replying = true
	r.mu.Unlock()

	var result json.RawMessage
	if err == nil {
		func() {
			defer catchAutomationToolPanic(p.name, &err)
			result, err = json.Marshal(value)
		}()
	}
	p.finish(result, err)
}

func (p *pendingAutomationCall) finish(result json.RawMessage, err error) {
	r := p.owner
	r.mu.Lock()
	if p.done == nil {
		r.mu.Unlock()
		return
	}
	if r.disposed != nil {
		result, err = nil, r.disposed
	} else if canceled := p.ctx.Err(); canceled != nil {
		result, err = nil, canceled
	}
	done, stop, cancel := p.done, p.stop, p.cancel
	p.done, p.stop, p.cancel, p.ctx = nil, nil, nil, nil
	delete(r.pending, p)
	r.mu.Unlock()

	if stop != nil {
		stop()
	}
	cancel()
	done(result, err)
}

func (r *deferredAutomationTool) dispose(name string) {
	r.mu.Lock()
	r.disposed = fmt.Errorf("automation: registering scope for tool %q was disposed", name)
	pending := r.pending
	r.pending = nil
	r.mu.Unlock()
	// No call can change this detached map.
	for p := range pending {
		p.finish(nil, nil)
	}
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

func (s *devSurface) CallAutomationToolAsync(ctx context.Context, name string, args json.RawMessage, done func(json.RawMessage, error)) {
	CallAutomationToolAsync(s.t.root, ctx, name, args, done)
}
