//go:build nsgoautomation || nsgotest || nsgocheck

package nsgotest

import (
	"bytes"
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

func TestAppAutomationToolResult(t *testing.T) {
	var cx *core.Scope
	schema := json.RawMessage(`{"type":"object"}`)
	pngBytes := []byte("fake-png-data")
	jpegBytes := []byte("fake-jpeg-data")

	app := Start(t, func(scope *core.Scope) core.Node {
		cx = scope
		for _, tool := range []automation.Tool{
			{
				Name: "image_value", InputSchema: schema,
				Handler: func(context.Context, json.RawMessage) (any, error) {
					return automation.Result{
						Text: "snapshot",
						Images: []automation.Image{
							{Data: pngBytes},
							{Data: jpegBytes, MIMEType: "image/jpeg"},
						},
					}, nil
				},
			},
			{
				Name: "image_ptr", InputSchema: schema,
				Handler: func(context.Context, json.RawMessage) (any, error) {
					return &automation.Result{
						Images: []automation.Image{{Data: pngBytes, MIMEType: "image/png"}},
					}, nil
				},
			},
			{
				Name: "image_nil_ptr", InputSchema: schema,
				Handler: func(context.Context, json.RawMessage) (any, error) {
					var res *automation.Result
					return res, nil
				},
			},
			{
				Name: "plain_png_struct", InputSchema: schema,
				Handler: func(context.Context, json.RawMessage) (any, error) {
					return struct {
						PNG []byte `json:"png"`
					}{PNG: pngBytes}, nil
				},
			},
		} {
			if err := automation.Register(cx, tool); err != nil {
				t.Fatal(err)
			}
		}
		return core.Text{Content: "ready"}
	})

	call := func(name string) json.RawMessage {
		t.Helper()
		var raw json.RawMessage
		var err error
		app.Post(func() { raw, err = automation.Call(cx, t.Context(), name, nil) })
		app.Settle()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return raw
	}

	valRes, ok, err := automation.DecodeResult(call("image_value"))
	if err != nil || !ok {
		t.Fatalf("image_value: ok=%v err=%v", ok, err)
	}
	if valRes.Text != "snapshot" || len(valRes.Images) != 2 ||
		!bytes.Equal(valRes.Images[0].Data, pngBytes) || valRes.Images[0].MIMEType != "" ||
		!bytes.Equal(valRes.Images[1].Data, jpegBytes) || valRes.Images[1].MIMEType != "image/jpeg" {
		t.Fatalf("image_value decoded = %+v", valRes)
	}

	ptrRes, ok, err := automation.DecodeResult(call("image_ptr"))
	if err != nil || !ok || ptrRes.Text != "" || len(ptrRes.Images) != 1 || !bytes.Equal(ptrRes.Images[0].Data, pngBytes) {
		t.Fatalf("image_ptr decoded = %+v, ok=%v, err=%v", ptrRes, ok, err)
	}

	nilRaw := call("image_nil_ptr")
	if string(nilRaw) != "null" {
		t.Fatalf("image_nil_ptr raw = %s, want null", nilRaw)
	}
	if _, ok, err := automation.DecodeResult(nilRaw); ok || err != nil {
		t.Fatalf("image_nil_ptr DecodeResult = ok:%v err:%v", ok, err)
	}

	plainRaw := call("plain_png_struct")
	if _, ok, err := automation.DecodeResult(plainRaw); ok || err != nil {
		t.Fatalf("plain_png_struct DecodeResult = ok:%v err:%v", ok, err)
	}
	var decoded struct {
		PNG []byte `json:"png"`
	}
	if err := json.Unmarshal(plainRaw, &decoded); err != nil || !bytes.Equal(decoded.PNG, pngBytes) {
		t.Fatalf("plain_png_struct raw = %s, decoded=%+v, err=%v", plainRaw, decoded, err)
	}
}
