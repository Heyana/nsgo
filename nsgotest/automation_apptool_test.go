//go:build nsgoautomation || nsgotest || nsgocheck

package nsgotest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nsgo-dev/nsgo/audio"
	"github.com/nsgo-dev/nsgo/automation"
	"github.com/nsgo-dev/nsgo/core"
	protocol "github.com/nsgo-dev/nsgo/internal/automation"
	"github.com/nsgo-dev/nsgo/internal/devtools"
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

	var duplicate, reserved, invalid, neither, both error
	app.Post(func() {
		duplicate = automation.Register(cx, tool)
		tool.Name = "status"
		reserved = automation.Register(cx, tool)
		tool.Name, tool.InputSchema = "other", json.RawMessage(`{"type":"array"}`)
		invalid = automation.Register(cx, tool)
		tool.Name, tool.InputSchema = "other", json.RawMessage(`{"type":"object"}`)
		tool.Start = func(context.Context, json.RawMessage, func(any, error)) {}
		both = automation.Register(cx, tool)
		tool.Start, tool.Handler = nil, nil
		neither = automation.Register(cx, tool)
	})
	app.Settle()
	if duplicate == nil || reserved == nil || invalid == nil || neither == nil || both == nil {
		t.Fatalf("invalid registrations: duplicate %v, reserved %v, schema %v, neither %v, both %v", duplicate, reserved, invalid, neither, both)
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

func TestAppAutomationToolAsync(t *testing.T) {
	var cx *core.Scope
	var started context.Context
	var reply func(any, error)
	var registration error
	app := Start(t, func(scope *core.Scope) core.Node {
		cx = scope
		registration = automation.Register(cx, automation.Tool{
			Name: "deferred", InputSchema: json.RawMessage(`{"type":"object"}`),
			Start: func(ctx context.Context, args json.RawMessage, done func(any, error)) {
				switch string(args) {
				case `{"post":true}`:
					cx.Post(func() {
						done("later", nil)
						done("duplicate", nil)
					})
				case `{"panic":true}`:
					panic("start failed")
				case `{"marshal":true}`:
					go done(make(chan int), nil)
				default:
					started, reply = ctx, done
				}
			},
		})
		return core.Text{}
	})
	if registration != nil {
		t.Fatal(registration)
	}
	var syncErr error
	app.Post(func() { _, syncErr = automation.Call(cx, t.Context(), "deferred", nil) })
	app.Settle()
	if syncErr == nil || !strings.Contains(syncErr.Error(), "CallAsync") || started != nil {
		t.Fatalf("synchronous call of deferred tool: %v, started %v", syncErr, started != nil)
	}

	type outcome struct {
		result json.RawMessage
		err    error
	}
	begin := func(ctx context.Context, args string) <-chan outcome {
		done := make(chan outcome, 2)
		app.Post(func() {
			automation.CallAsync(cx, ctx, "deferred", json.RawMessage(args), func(result json.RawMessage, err error) {
				done <- outcome{result, err}
			})
		})
		app.Settle()
		return done
	}
	receive := func(done <-chan outcome) outcome {
		t.Helper()
		select {
		case r := <-done:
			return r
		case <-time.After(2 * time.Second):
			t.Fatal("no reply")
			return outcome{}
		}
	}
	posted := begin(t.Context(), `{"post":true}`)
	if r := receive(posted); r.err != nil || string(r.result) != `"later"` {
		t.Fatalf("posted result: %s, %v", r.result, r.err)
	}
	select {
	case r := <-posted:
		t.Fatalf("duplicate reply delivered: %+v", r)
	default:
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := begin(ctx, "")
	if reply == nil || started == nil {
		t.Fatal("deferred tool did not start")
	}
	cancel()
	if r := receive(done); !errors.Is(r.err, context.Canceled) || len(r.result) != 0 {
		t.Fatalf("canceled call: %s, %v", r.result, r.err)
	}
	if started.Err() != context.Canceled {
		t.Fatalf("tool context not canceled: %v", started.Err())
	}
	reply("too late", nil)
	select {
	case r := <-done:
		t.Fatalf("late reply delivered: %+v", r)
	default:
	}
	for _, tc := range []struct{ args, want string }{
		{`{"panic":true}`, `tool "deferred" panicked: start failed`},
		{`{"marshal":true}`, "unsupported type: chan int"},
	} {
		if r := receive(begin(t.Context(), tc.args)); r.err == nil || !strings.Contains(r.err.Error(), tc.want) {
			t.Fatalf("%s: %s, %v", tc.args, r.result, r.err)
		}
	}
}

func TestAppAutomationToolAgent(t *testing.T) {
	if onTarget() {
		t.Skip("exercises the agent with the headless audio model")
	}
	synctest.Test(t, func(t *testing.T) {
		g := newAgent()
		t.Cleanup(g.close)
		// Don't start the unrelated idle model drain: it takes the agent's
		// request lock while the test advances fake time to the reply deadline.
		g.tick.Do(func() {})
		enabled, agent := devtools.Enabled, devtools.Automation
		devtools.Enabled, devtools.Automation = true, g
		t.Cleanup(func() { devtools.Enabled, devtools.Automation = enabled, agent })

		const soundURI = "asset:sound.wav"
		const duration = 80 * time.Millisecond
		var owner *core.Scope
		var pendingContext context.Context
		var lateReply func(any, error)
		app := Start(t, func(cx *core.Scope) core.Node {
			owner = cx.Child()
			text := core.NewSignal(cx, "ready")
			key := core.NewKey("tool owner", "wrong scope")
			owner.Post(func() { key.Provide(cx, "owning scope") })
			register := func(tool automation.Tool) {
				tool.InputSchema = json.RawMessage(`{"type":"object"}`)
				if err := automation.Register(owner, tool); err != nil {
					t.Error(err)
				}
			}
			register(automation.Tool{Name: "post_result", Start: func(_ context.Context, _ json.RawMessage, reply func(any, error)) {
				value := key.Use(cx)
				owner.Post(func() {
					text.Set("posted")
					reply(value, nil)
					reply("duplicate", nil)
				})
			}})
			register(automation.Tool{Name: "sound_duration", Start: func(_ context.Context, _ json.RawMessage, reply func(any, error)) {
				audio.LoadSound(owner, audio.Source{URI: soundURI}, func(sound *audio.Sound, err error) {
					if err != nil {
						reply(nil, err)
						return
					}
					reply(sound.Duration().Milliseconds(), nil)
				})
			}})
			register(automation.Tool{Name: "pending_result", Start: func(ctx context.Context, _ json.RawMessage, reply func(any, error)) {
				pendingContext, lateReply = ctx, reply
				text.Set("waiting")
			}})
			register(automation.Tool{Name: "immediate", Handler: func(context.Context, json.RawMessage) (any, error) {
				return "legacy", nil
			}})
			return core.Text{ContentFunc: text.Get}
		}, Environment(core.Environment{Audio: true}), func(a *App) {
			a.AudioDurations = map[string]time.Duration{soundURI: duration}
		})

		begin := func(name string) <-chan []byte {
			req, _ := json.Marshal(protocol.Request{ID: 9, Method: name, Params: json.RawMessage(`{}`)})
			done := make(chan []byte, 1)
			go func() { done <- g.Do(req) }()
			synctest.Wait() // The agent has queued its graph post.
			app.Settle()
			return done
		}
		answer := func(done <-chan []byte) protocol.Answer {
			t.Helper()
			var a protocol.Answer
			if err := json.Unmarshal(<-done, &a); err != nil || a.ID != 9 {
				t.Fatalf("agent reply: %+v, %v", a, err)
			}
			return a
		}
		for _, tc := range []struct{ name, want string }{
			{"post_result", `"owning scope"`}, {"sound_duration", "80"}, {"immediate", `"legacy"`},
		} {
			if a := answer(begin(tc.name)); a.Error != nil || string(a.Result) != tc.want {
				t.Fatalf("%s: %s, %v", tc.name, a.Result, a.Error)
			}
		}

		began := time.Now()
		pending := begin("pending_result")
		if pendingContext == nil || len(app.Query("text:waiting")) != 1 {
			t.Fatal("the app did not render while the tool was pending")
		}
		if deadline, ok := pendingContext.Deadline(); !ok || !deadline.Equal(began.Add(maxWait)) {
			t.Fatalf("tool deadline: %v, %v", deadline, ok)
		}
		if a := answer(pending); a.Error == nil || a.Error.Code != protocol.Timeout || time.Since(began) < maxWait {
			t.Fatalf("missing bounded Timeout: %+v", a)
		}
		lateReply("too late", nil)

		pending = begin("pending_result")
		app.Post(owner.Dispose)
		app.Settle()
		if a := answer(pending); a.Error == nil || a.Error.Code != protocol.Failed || !strings.Contains(a.Error.Message, "scope") {
			t.Fatalf("disposal did not fail the pending request: %+v", a)
		}
		if pendingContext.Err() != context.Canceled {
			t.Fatalf("disposed tool context: %v", pendingContext.Err())
		}
		lateReply("disposed", nil)
		if a := answer(begin("pending_result")); a.Error == nil || a.Error.Code != protocol.UnknownMethod {
			t.Fatalf("disposed tool remained callable: %+v", a)
		}
	})
}
