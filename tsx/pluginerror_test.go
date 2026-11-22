package tsx_test

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

func TestPluginCallbackErrorsReachTheirOwnHost(t *testing.T) {
	for _, c := range []struct {
		name, source string
		arm          bool
	}{
		{"entry timer", `setTimeout(() => { throw new Error("MARK"); }, 1);`, false},
		{"settled call timer", `export function arm() { setTimeout(() => { throw new Error("MARK"); }, 1); return "armed"; }`, true},
		{"settled timer microtask", `export function arm() { setTimeout(() => queueMicrotask(() => { throw new Error("MARK"); }), 1); return "armed"; }`, true},
		{"entry microtask", `queueMicrotask(() => { throw new Error("MARK"); });`, false},
		{"explicit report", `reportError(new Error("MARK"));`, false},
		{"frame", `requestAnimationFrame(() => { throw new Error("MARK"); });`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			packages := [2]*pkg.Package{}
			for i, marker := range []string{"first plugin", "second plugin"} {
				packages[i] = verified(t, strings.ReplaceAll(c.source, "MARK", marker)+` export function ping() { return "alive"; }`, pkg.Capabilities{})
			}
			var plugins [2]*tsx.Plugin
			var attributed [2][]error
			var appErrors []error
			var count *core.Signal[int]
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				count = core.NewSignal(cx, 0)
				for i, p := range packages {
					plugin, err := tsx.Load(cx, p, tsx.Sandbox{WebUnisolated: true, OnError: func(err error) {
						attributed[i] = append(attributed[i], err)
						count.Update(func(n int) int { return n + 1 })
					}})
					if err != nil {
						t.Fatal(err)
					}
					plugins[i] = plugin
				}
				return core.Text{Content: "Host stays mounted", TestID: "host"}
			}, nsgotest.Errors(func(err error) { appErrors = append(appErrors, err) }))
			if c.arm {
				for _, plugin := range plugins {
					if value, err := callPlugin(t, app, plugin, "arm"); err != nil || value != "armed" {
						t.Fatalf("arm = %v, %v", value, err)
					}
				}
			}
			app.Advance(30 * time.Millisecond)
			if len(appErrors) != 0 {
				t.Fatalf("plugin errors reached the app report (%d), attributed = %v", len(appErrors), attributed)
			}
			for i, marker := range []string{"first plugin", "second plugin"} {
				if len(attributed[i]) != 1 || !strings.Contains(attributed[i][0].Error(), marker) {
					t.Fatalf("plugin %d errors = %v", i, attributed[i])
				}
				if plugins[i].Stopped() {
					t.Fatal("an ordinary callback error stopped its plugin")
				}
				if value, err := callPlugin(t, app, plugins[i], "ping"); err != nil || value != "alive" {
					t.Fatalf("ping = %v, %v", value, err)
				}
			}
			if count.GetUntracked() != 2 || !app.Exists("host") {
				t.Fatal("the callbacks did not run on the engine with the host still mounted")
			}
		})
	}
}

func TestPluginCallbackErrorWithoutHandlerKeepsAppReport(t *testing.T) {
	p := verified(t, `setTimeout(() => { throw new Error("unhandled plugin timer"); }, 1);`, pkg.Capabilities{})
	var reported []error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		if _, err := tsx.Load(cx, p, tsx.Sandbox{WebUnisolated: true}); err != nil {
			t.Fatal(err)
		}
		return core.View{}
	}, nsgotest.Errors(func(err error) { reported = append(reported, err) }))
	app.Advance(10 * time.Millisecond)
	if len(reported) != 1 || !strings.Contains(reported[0].Error(), "unhandled plugin timer") {
		t.Fatalf("app report = %v", reported)
	}
}

func TestPluginCallbackSliceErrorsReachHostAfterCallSettles(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("the browser cannot interrupt JavaScript slices")
	}
	for _, c := range []struct {
		name, source string
		arm          bool
	}{
		{"entry timer", `setTimeout(() => { for (;;) {} }, 1);`, false},
		{"settled timer", `export function arm() { setTimeout(() => { for (;;) {} }, 1); return "armed"; }`, true},
		{"settled promise job", `let finish;
const waiting = new Promise(resolve => finish = resolve);
export function arm() { waiting.then(() => { for (;;) {} }); setTimeout(() => finish(), 1); return "armed"; }`, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := verified(t, c.source+` export function ping() { return "alive"; }`, pkg.Capabilities{})
			var plugin *tsx.Plugin
			var appErrors, attributed []error
			limits := tsx.Limits{Load: time.Second, Call: 20 * time.Millisecond}
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				var err error
				plugin, err = tsx.Load(cx, p, tsx.Sandbox{Limits: &limits, OnError: func(err error) { attributed = append(attributed, err) }})
				if err != nil {
					t.Fatal(err)
				}
				return core.View{}
			}, nsgotest.Errors(func(err error) { appErrors = append(appErrors, err) }))
			if c.arm {
				if _, err := callPlugin(t, app, plugin, "arm"); err != nil {
					t.Fatal(err)
				}
			}
			app.Advance(10 * time.Millisecond)
			app.Settle()
			if len(appErrors) != 0 || len(attributed) != 1 || !errors.Is(attributed[0], tsx.ErrBudget) {
				t.Fatalf("app errors = %d, plugin errors = %v", len(appErrors), attributed)
			}
			if value, err := callPlugin(t, app, plugin, "ping"); err != nil || value != "alive" {
				t.Fatalf("ping = %v, %v", value, err)
			}
		})
	}
}

func TestPluginCallbackHandlerKeepsLoadAndCallErrorsAtTheirCaller(t *testing.T) {
	var appErrors, attributed []error
	var loadErr error
	var plugin *tsx.Plugin
	bad := verified(t, `throw new Error("entry failed");`, pkg.Capabilities{})
	good := verified(t, `export function fail() { throw new Error("call failed"); }
export async function reject() { throw new Error("promise rejected"); }`, pkg.Capabilities{})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		sandbox := tsx.Sandbox{WebUnisolated: true, OnError: func(err error) { attributed = append(attributed, err) }}
		_, loadErr = tsx.Load(cx, bad, sandbox)
		var err error
		plugin, err = tsx.Load(cx, good, sandbox)
		if err != nil {
			t.Fatal(err)
		}
		return core.View{}
	}, nsgotest.Errors(func(err error) { appErrors = append(appErrors, err) }))
	if loadErr == nil || !strings.Contains(loadErr.Error(), "entry failed") {
		t.Fatalf("Load error = %v", loadErr)
	}
	for _, name := range []string{"fail", "reject"} {
		if _, err := callPlugin(t, app, plugin, name); err == nil {
			t.Fatalf("%s did not return its failure", name)
		}
	}
	if len(attributed) != 0 || len(appErrors) != 0 {
		t.Fatalf("load/call failures reached callbacks: plugin %v, app count %d", attributed, len(appErrors))
	}
}

func TestPluginCallbackHandlerKeepsPendingCallSliceFailure(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("the browser cannot interrupt JavaScript slices")
	}
	p := verified(t, `export function wait() { return new Promise(() => setTimeout(() => { for (;;) {} }, 1)); }`, pkg.Capabilities{})
	var plugin *tsx.Plugin
	var attributed, appErrors []error
	limits := tsx.Limits{Load: time.Second, Call: 20 * time.Millisecond}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		var err error
		plugin, err = tsx.Load(cx, p, tsx.Sandbox{Limits: &limits, OnError: func(err error) { attributed = append(attributed, err) }})
		if err != nil {
			t.Fatal(err)
		}
		return core.View{}
	}, nsgotest.Errors(func(err error) { appErrors = append(appErrors, err) }))
	if _, err := callPlugin(t, app, plugin, "wait"); !errors.Is(err, tsx.ErrBudget) {
		t.Fatalf("waiting call error = %v", err)
	}
	if len(attributed) != 0 || len(appErrors) != 0 {
		t.Fatalf("pending call failure reached callbacks: plugin %v, app count %d", attributed, len(appErrors))
	}
}

func TestPluginCallbackHandlerCanBreakARepeatedFailure(t *testing.T) {
	p := verified(t, `setInterval(() => { throw new Error("recurring failure"); }, 3);`, pkg.Capabilities{})
	var plugin *tsx.Plugin
	var attributed, stops, appErrors []error
	why := errors.New("plugin breaker opened")
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		var err error
		plugin, err = tsx.Load(cx, p, tsx.Sandbox{WebUnisolated: true, OnStop: func(err error) { stops = append(stops, err) }, OnError: func(err error) {
			attributed = append(attributed, err)
			if len(attributed) == 2 {
				plugin.Interrupt(why)
			}
		}})
		if err != nil {
			t.Fatal(err)
		}
		return core.View{}
	}, nsgotest.Errors(func(err error) { appErrors = append(appErrors, err) }))
	app.Advance(30 * time.Millisecond)
	if len(attributed) != 2 || !plugin.Stopped() || !errors.Is(plugin.Err(), why) || len(stops) != 1 || !errors.Is(stops[0], why) || len(appErrors) != 0 {
		t.Fatalf("breaker: errors %d, stop %v, notifications %v, app errors %d", len(attributed), plugin.Err(), stops, len(appErrors))
	}
}

func TestPluginErrorHandlerDoesNotReplaceAViewsBoundary(t *testing.T) {
	p := verified(t, `import { mount, Text } from "nsgo";
mount(() => <Text testID="fail" onPress={() => { throw new Error("view failed"); }}>Fail</Text>);`, pkg.Capabilities{})
	var attributed, caught, appErrors []error
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.Catch{Try: tsx.View{Package: p, Sandbox: tsx.Sandbox{WebUnisolated: true, OnError: func(err error) { attributed = append(attributed, err) }}}.Render,
			Fallback: func(_ *core.Scope, err error, _ func()) core.Node {
				caught = append(caught, err)
				return core.Text{Content: "View failed", TestID: "fallback"}
			}}
	}, nsgotest.Errors(func(err error) { appErrors = append(appErrors, err) }))
	app.Press("fail")
	if len(caught) != 1 || !strings.Contains(caught[0].Error(), "view failed") || len(attributed) != 0 || len(appErrors) != 0 || !app.Exists("fallback") {
		t.Fatalf("view routing: caught %v, plugin %v, app %d", caught, attributed, len(appErrors))
	}
}

func init() {
	tsx.RegisterFunctions("app/plugin-errors", map[string]any{"callback": func(fn func()) { fn() }})
}

func TestPluginCallbackHandlerKeepsNestedJavaScriptCatches(t *testing.T) {
	p := verified(t, `import { callback } from "app/plugin-errors";
let caught = false;
setTimeout(() => { try { callback(() => { throw new Error("nested failure"); }); } catch (error) { caught = true; } }, 1);
export function status() { return caught; }`, pkg.Capabilities{})
	var plugin *tsx.Plugin
	var attributed, appErrors []error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		var err error
		plugin, err = tsx.Load(cx, p, tsx.Sandbox{Modules: []string{"app/plugin-errors"}, WebUnisolated: true,
			OnError: func(err error) { attributed = append(attributed, err) }})
		if err != nil {
			t.Fatal(err)
		}
		return core.View{}
	}, nsgotest.Errors(func(err error) { appErrors = append(appErrors, err) }))
	app.Advance(10 * time.Millisecond)
	if value, err := callPlugin(t, app, plugin, "status"); err != nil || value != true {
		t.Fatalf("nested catch = %v, %v", value, err)
	}
	if len(attributed) != 0 || len(appErrors) != 0 {
		t.Fatalf("a caught exception was reported: plugin %v, app %d", attributed, len(appErrors))
	}
}

func TestPluginCallbackSliceDoesNotFailAnotherCallsResult(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("the browser cannot interrupt JavaScript slices")
	}
	p := verified(t, `let finish;
const waiting = new Promise(resolve => finish = resolve);
export function arm() { waiting.then(() => { for (;;) {} }); return "armed"; }
export function trigger() { finish(); return "triggered"; }`, pkg.Capabilities{})
	var plugin *tsx.Plugin
	var attributed, appErrors []error
	limits := tsx.Limits{Load: time.Second, Call: 20 * time.Millisecond}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		var err error
		plugin, err = tsx.Load(cx, p, tsx.Sandbox{Limits: &limits, OnError: func(err error) { attributed = append(attributed, err) }})
		if err != nil {
			t.Fatal(err)
		}
		return core.View{}
	}, nsgotest.Errors(func(err error) { appErrors = append(appErrors, err) }))
	for _, pair := range [][2]string{{"arm", "armed"}, {"trigger", "triggered"}} {
		if value, err := callPlugin(t, app, plugin, pair[0]); err != nil || value != pair[1] {
			t.Fatalf("%s returned %v, %v", pair[0], value, err)
		}
	}
	app.Settle()
	if len(appErrors) != 0 || len(attributed) != 1 || !errors.Is(attributed[0], tsx.ErrBudget) {
		t.Fatalf("app errors = %d, plugin errors = %v", len(appErrors), attributed)
	}
}
