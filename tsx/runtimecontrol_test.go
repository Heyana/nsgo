package tsx_test

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

func TestPluginExtendsOnlyTheWaitingCall(t *testing.T) {
	app, plugin := deadlinePlugin(t, `
let complete;
export function hold() { return new Promise(resolve => complete = resolve); }
export function wait() { return new Promise(() => {}); }
export function finish() { complete(42); }`, time.Second, 350*time.Millisecond)
	var budget *tsx.CallBudget
	var extended, heldDone bool
	var held any
	var heldErr, otherErr error
	app.Post(func() {
		budget = plugin.CallWithBudget("hold", nil, func(value any, err error) { held, heldErr, heldDone = value, err, true })
		extended = budget.Extend(time.Second)
		plugin.Call("wait", nil, func(_ any, err error) { otherErr = err })
	})
	app.Settle()
	if !extended {
		t.Fatal("the pending call's deadline could not be extended")
	}
	app.WaitUntil(3*time.Second, func() bool { return otherErr != nil })
	if !errors.Is(otherErr, tsx.ErrBudget) || heldDone {
		t.Fatalf("other call error = %v, extended call completed = %v", otherErr, heldDone)
	}
	if _, err := callPlugin(t, app, plugin, "finish"); err != nil {
		t.Fatal(err)
	}
	app.WaitUntil(time.Second, func() bool { return heldDone })
	if heldErr != nil || number(held) != 42 {
		t.Fatalf("extended call = %v, %v, want 42", held, heldErr)
	}
	app.Post(func() { extended = budget.Extend(time.Second) })
	app.Settle()
	if extended {
		t.Fatal("a completed call still accepted a deadline extension")
	}
	if elapsed, ok := plugin.JSTime(); ok != (runtime.GOOS != "js") || ok && elapsed <= 0 {
		t.Fatalf("JSTime = %v, %v", elapsed, ok)
	}
}

func TestViewRuntimeStopsWithoutUnmounting(t *testing.T) {
	p := verified(t, `import { mount, signal, Text } from "nsgo";
const n = signal(0);
mount(() => <Text testID="runtime" onPress={() => n.set(n() + 1)}>{String(n())}</Text>);`, pkg.Capabilities{})
	var view *tsx.ViewRuntime
	var shown *core.Signal[bool]
	var stops []error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		shown = core.NewSignal(cx, true)
		return core.Show{When: shown.Get, Then: tsx.View{Package: p, Sandbox: tsx.Sandbox{
			WebUnisolated: true, OnViewRuntime: func(v *tsx.ViewRuntime) { view = v },
			OnStop: func(err error) { stops = append(stops, err) },
		}}.Render}
	})
	if view == nil {
		t.Fatal("view runtime was not published")
	}
	before, supported := view.JSTime()
	engineBefore, engineSupported := view.EngineTime()
	app.Press("id:runtime")
	if got := app.Find("id:runtime").Text; got != "1" {
		t.Fatalf("before interruption the view showed %q", got)
	}
	if after, ok := view.JSTime(); supported != (runtime.GOOS != "js") || ok != supported || ok && (before <= 0 || after <= before) {
		t.Fatalf("JSTime changed from %v to %v, supported %v", before, after, ok)
	}

	if after, ok := view.EngineTime(); engineSupported != supported || ok != supported || ok && (engineBefore < before || after <= engineBefore) {
		t.Fatalf("EngineTime changed from %v to %v, supported %v", engineBefore, after, ok)
	}

	why := errors.New("test stopped the view")
	done := make(chan struct{})
	go func() {
		view.Interrupt(why)
		close(done)
	}()
	<-done
	app.Settle()
	app.Press("id:runtime")
	if got := app.Find("id:runtime").Text; got != "1" || !view.Stopped() || !errors.Is(view.Err(), why) {
		t.Fatalf("the stopped view shows %q with error %v", got, view.Err())
	}
	app.Post(func() { shown.Set(false) })
	app.Settle()
	if len(stops) != 1 || !errors.Is(stops[0], why) {
		t.Fatalf("stop callbacks = %v", stops)
	}
}

func TestViewCallTotalBoundsWorkFromAnEvent(t *testing.T) {
	p := verified(t, `import { mount, signal, Text } from "nsgo";
const ticks = signal(0);
mount(() => <Text testID="ticks" onPress={() => setInterval(() => ticks.set(ticks() + 1), 2)}>{String(ticks())}</Text>);`, pkg.Capabilities{})
	limits := tsx.Limits{Load: time.Second, Call: time.Second, CallTotal: 350 * time.Millisecond}
	var view *tsx.ViewRuntime
	app := nsgotest.Start(t, tsx.View{Package: p, Sandbox: tsx.Sandbox{
		Limits: &limits, WebUnisolated: true, OnViewRuntime: func(v *tsx.ViewRuntime) { view = v },
	}}.Render)
	app.Press("id:ticks")
	runUntil(app, time.Now(), limits.CallTotal+50*time.Millisecond)
	before := app.Find("id:ticks").Text
	app.Advance(30 * time.Millisecond)
	after := app.Find("id:ticks").Text
	if before == "0" || after != before || view.Stopped() {
		t.Fatalf("timer count changed from %q to %q, view error = %v", before, after, view.Err())
	}
}

func TestViewRuntimeCanInterruptItsLoadingEntry(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("the browser does not let Go preempt a running JavaScript entry")
	}
	p := verified(t, `for (;;) {}`, pkg.Capabilities{})
	why := errors.New("stop during load")
	handles := make(chan *tsx.ViewRuntime, 1)
	elapsed := make(chan time.Duration, 1)
	limits := tsx.Limits{Load: 2 * time.Second}
	go func() {
		var view *tsx.ViewRuntime
		select {
		case view = <-handles:
		case <-time.After(time.Second):
			elapsed <- 0
			return
		}
		start := time.Now()
		for time.Since(start) < time.Second {
			if ran, _ := view.JSTime(); ran > 5*time.Millisecond {
				view.Interrupt(why)
				elapsed <- ran
				return
			}
			time.Sleep(time.Millisecond)
		}
		view.Interrupt(why)
		elapsed <- 0
	}()
	var view *tsx.ViewRuntime
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.Text{TestID: "host", Content: "host"},
			tsx.View{Package: p, Sandbox: tsx.Sandbox{Limits: &limits, OnViewRuntime: func(v *tsx.ViewRuntime) {
				view = v
				handles <- v
			}}},
		}}
	})
	if took := <-elapsed; took <= 0 || view == nil || !errors.Is(view.Err(), why) || app.Find("id:host").Text != "host" {
		t.Fatalf("measured %v before interruption, error = %v", took, view.Err())
	}
}

func TestRuntimeReportsNormalOwnerDisposal(t *testing.T) {
	p := verified(t, `export function wait() { return new Promise(() => {}); }`, pkg.Capabilities{})
	var owner *core.Scope
	var plugin *tsx.Plugin
	var loadErr, callErr error
	var shown *core.Signal[bool]
	var view *tsx.ViewRuntime
	var stopErr error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		shown = core.NewSignal(cx, true)
		owner = cx.Child()
		owner.Run(func() { plugin, loadErr = tsx.Load(owner, p, tsx.Sandbox{WebUnisolated: true}) })
		return core.Show{When: shown.Get, Then: tsx.View{Package: p, Sandbox: tsx.Sandbox{
			WebUnisolated: true, OnViewRuntime: func(v *tsx.ViewRuntime) { view = v },
			OnStop: func(err error) { stopErr = err },
		}}.Render}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	app.Post(func() {
		plugin.Call("wait", nil, func(_ any, err error) { callErr = err })
		owner.Dispose()
		shown.Set(false)
	})
	app.Settle()
	if !errors.Is(callErr, tsx.ErrPluginClosed) || !errors.Is(plugin.Err(), tsx.ErrPluginClosed) {
		t.Errorf("disposed plugin call error = %v, stop error = %v", callErr, plugin.Err())
	}
	if view == nil || !errors.Is(view.Err(), tsx.ErrViewClosed) || !errors.Is(stopErr, tsx.ErrViewClosed) {
		t.Errorf("disposed view reason = %v, callback reason = %v", view.Err(), stopErr)
	}
}
