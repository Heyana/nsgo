package nsgotest

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/nsgo-dev/nsgo/internal/automation"
	"github.com/nsgo-dev/nsgo/internal/devtools"
)

// appToolSurface additionally exposes the tools of its graph.
type appToolSurface interface {
	devtools.Surface
	AutomationTools() []automation.AppTool
	CallAutomationToolAsync(context.Context, string, json.RawMessage, func(json.RawMessage, error))
}

func (g *agent) toolSurface() appToolSurface {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, f := range g.surfaces {
		if s, ok := f.native.(appToolSurface); ok && !f.closed {
			return s
		}
	}
	return nil
}

// appTool reads or calls a tool on the graph goroutine, using any live surface.
func (g *agent) appTool(req automation.Request) (any, *automation.Error) {
	list := req.Method == automation.ListAppTools
	s := g.toolSurface()
	if s == nil {
		if list {
			return []automation.AppTool{}, nil
		}
		return nil, automation.Errorf(automation.UnknownMethod, "no app tool %q", req.Method)
	}

	limit := maxWait
	if list {
		limit = automation.AppToolListTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	type reply struct {
		result any
		err    error
	}
	done := make(chan reply, 1)
	s.Post(func() {
		if ctx.Err() != nil {
			return
		}
		if list {
			done <- reply{result: s.AutomationTools()}
			return
		}
		s.CallAutomationToolAsync(ctx, req.Method, req.Params, func(result json.RawMessage, err error) {
			if ctx.Err() == nil {
				done <- reply{result, err}
			}
		})
	})
	select {
	case r := <-done:
		if r.err == nil {
			return r.result, nil
		}
		code := automation.Failed
		switch {
		case errors.Is(r.err, automation.ErrUnknownAppTool):
			code = automation.UnknownMethod
		case errors.Is(r.err, automation.ErrAppToolInput):
			code = automation.InvalidRequest
		case errors.Is(r.err, context.DeadlineExceeded):
			code = automation.Timeout
		}
		return nil, automation.Errorf(code, "%v", r.err)
	case <-ctx.Done():
		return nil, automation.Errorf(automation.Timeout, "%s took longer than %v", req.Method, limit)
	}
}
