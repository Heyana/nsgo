//go:build nsgoautomation || nsgotest || nsgocheck

package nsgotest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/automation"
	"github.com/nsgo-dev/nsgo/core"
	protocol "github.com/nsgo-dev/nsgo/internal/automation"
)

func TestAppAutomationTool(t *testing.T) {
	var cx, owner *core.Scope
	var registration error
	var count *core.Signal[string]
	schema := json.RawMessage(`{"type":"object"}`)
	tool := automation.Tool{
		Name: "set_count", InputSchema: schema,
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var input struct{ Count string }
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, err
			}
			count.Set(input.Count)
			return input.Count, nil
		},
	}
	app := Start(t, func(scope *core.Scope) core.Node {
		cx = scope
		count = core.NewSignal(cx, "0")
		owner = cx.Child()
		registration = automation.Register(owner, tool)
		return core.Text{ContentFunc: count.Get}
	})
	if registration != nil {
		t.Fatal(registration)
	}
	call := func(scope *core.Scope, args string) (json.RawMessage, error) {
		t.Helper()
		var result json.RawMessage
		var err error
		app.Post(func() { result, err = automation.Call(scope, t.Context(), "set_count", json.RawMessage(args)) })
		app.Settle()
		return result, err
	}
	if result, err := call(cx, `{"count":"3"}`); err != nil || string(result) != `"3"` {
		t.Fatalf("set_count returned %s, %v", result, err)
	}
	if view := app.Query("text:3"); len(view) != 1 {
		t.Fatalf("set_count did not update the view: %+v", view)
	}
	if _, err := call(cx, `[]`); !errors.Is(err, protocol.ErrAppToolInput) {
		t.Fatalf("array arguments: %v", err)
	}
	if _, err := call(cx, `{"count":4}`); err == nil || !strings.Contains(err.Error(), "cannot unmarshal") {
		t.Fatalf("handler error: %v", err)
	}

	var duplicate, reserved, invalid error
	app.Post(func() {
		duplicate = automation.Register(cx, tool)
		tool.Name = "status"
		reserved = automation.Register(cx, tool)
		tool.Name, tool.InputSchema = "other", json.RawMessage(`{"type":"array"}`)
		invalid = automation.Register(cx, tool)
	})
	app.Settle()
	if duplicate == nil || reserved == nil || invalid == nil {
		t.Fatalf("invalid registrations: duplicate %v, reserved %v, schema %v", duplicate, reserved, invalid)
	}
	var isolated error
	other := Start(t, func(scope *core.Scope) core.Node {
		_, isolated = automation.Call(scope, t.Context(), "set_count", nil)
		return core.Text{}
	})
	other.Settle()
	if !errors.Is(isolated, protocol.ErrUnknownAppTool) {
		t.Fatalf("another graph found set_count: %v", isolated)
	}
	app.Post(owner.Dispose)
	app.Settle()
	if _, err := call(cx, `{"count":"4"}`); !errors.Is(err, protocol.ErrUnknownAppTool) {
		t.Fatalf("set_count survived owner disposal: %v", err)
	}
}
