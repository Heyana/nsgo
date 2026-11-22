//go:build !js

package tsx_test

import (
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

var engineProbe struct {
	sync.Mutex
	run    func()
	scopes map[string]*core.Scope
}

type engineOwner struct {
	Name string
	Tick func() int
}

func (p engineOwner) Render(cx *core.Scope) core.Node {
	engineProbe.Lock()
	if engineProbe.scopes != nil {
		engineProbe.scopes[p.Name] = cx
	}
	engineProbe.Unlock()
	if p.Tick != nil {
		cx.Effect(func() {
			_ = p.Tick()
			engineProbe.Lock()
			run := engineProbe.run
			engineProbe.Unlock()
			if run != nil {
				run()
			}
		})
	}
	return nil
}

func init() {
	tsx.Register("app/engine-owner", map[string]core.Node{"Owner": engineOwner{}})
	tsx.RegisterFunctions("app/engine-cost", map[string]any{"work": func() {
		engineProbe.Lock()
		run := engineProbe.run
		engineProbe.Unlock()
		if run != nil {
			run()
		}
	}})
}

type engineSpan struct {
	jsBefore, jsAfter, engineBefore, engineAfter time.Duration
	available                                    bool
}

// sampleEngineSpans samples the handle from a host goroutine while the engine
// remains inside the work being attributed, rather than relying on a fast
// operation taking some minimum amount of time on a particular machine.
func sampleEngineSpans(views ...*tsx.ViewRuntime) []engineSpan {
	done := make(chan []engineSpan, 1)
	go func() {
		spans := make([]engineSpan, len(views))
		for i, view := range views {
			spans[i].jsBefore, _ = view.JSTime()
			spans[i].engineBefore, spans[i].available = view.EngineTime()
		}
		time.Sleep(5 * time.Millisecond)
		for i, view := range views {
			spans[i].jsAfter, _ = view.JSTime()
			spans[i].engineAfter, _ = view.EngineTime()
		}
		done <- spans
	}()
	return <-done
}

func checkEngineSpan(t *testing.T, span engineSpan, active bool) {
	t.Helper()
	if !span.available || span.jsBefore != span.jsAfter {
		t.Fatalf("outside-JS span = %+v", span)
	}
	if active && span.engineAfter <= span.engineBefore || !active && span.engineAfter != span.engineBefore {
		t.Fatalf("active=%v: engine time changed from %v to %v", active, span.engineBefore, span.engineAfter)
	}
}

func TestViewEngineTimeIncludesSynchronousGo(t *testing.T) {
	p := verified(t, `import { mount, Text } from "nsgo";
import { work } from "app/engine-cost";
mount(() => <Text testID="work" onPress={() => work()}>Go work</Text>);`, pkg.Capabilities{})
	var view *tsx.ViewRuntime
	var spans []engineSpan
	engineProbe.Lock()
	engineProbe.run = func() { spans = sampleEngineSpans(view) }
	engineProbe.Unlock()
	t.Cleanup(func() { engineProbe.Lock(); engineProbe.run = nil; engineProbe.Unlock() })
	app := nsgotest.Start(t, tsx.View{Package: p, Sandbox: tsx.Sandbox{
		Modules: []string{"app/engine-cost"}, OnViewRuntime: func(v *tsx.ViewRuntime) { view = v },
	}}.Render)
	app.Press("work")
	if len(spans) != 1 {
		t.Fatal("the registered Go function did not run")
	}
	checkEngineSpan(t, spans[0], true)
}

type engineCostText struct {
	phase string
	probe func(string)
}

func (e engineCostText) Prepare(text *core.AttributedText) core.PreparedText {
	if e.phase == "prepare" {
		e.probe(text.Text)
	}
	return engineCostPrepared{PreparedText: core.ApproxTextEngine{}.Prepare(text), engine: e, word: text.Text}
}

type engineCostPrepared struct {
	core.PreparedText
	engine engineCostText
	word   string
}

func (p engineCostPrepared) Measure(width core.Points) core.Size {
	if p.engine.phase == "measure" {
		p.engine.probe(p.word)
	}
	return p.PreparedText.Measure(width)
}

func (p engineCostPrepared) Layout(size core.Size) core.TextLayout {
	if p.engine.phase == "layout" {
		p.engine.probe(p.word)
	}
	return p.PreparedText.Layout(size)
}

func TestViewEngineTimeIncludesOwnedCommitAndLayout(t *testing.T) {
	p := verified(t, `import { mount, Text } from "nsgo";
mount(() => <Text testID="owned">owned engine work</Text>);`, pkg.Capabilities{})
	for _, phase := range []string{"prepare", "measure", "layout"} {
		t.Run(phase, func(t *testing.T) {
			var view *tsx.ViewRuntime
			var spans []engineSpan
			var once sync.Once
			engine := engineCostText{phase: phase, probe: func(word string) {
				if word == "owned engine work" {
					once.Do(func() { spans = sampleEngineSpans(view) })
				}
			}}
			app := nsgotest.Start(t, tsx.View{Package: p, Sandbox: tsx.Sandbox{
				OnViewRuntime: func(v *tsx.ViewRuntime) { view = v },
			}}.Render, nsgotest.TextEngine(engine))
			app.Find("owned")
			if len(spans) != 1 {
				t.Fatalf("no owned %s work was observed", phase)
			}
			checkEngineSpan(t, spans[0], true)
		})
	}
}

func TestViewEngineTimeDoesNotChargeHostOrSibling(t *testing.T) {
	p := verified(t, `import { mount, Text } from "nsgo";
mount(() => <Text>first view</Text>);`, pkg.Capabilities{})
	q := verified(t, `import { mount, Text } from "nsgo";
mount(() => <Text>second view</Text>);`, pkg.Capabilities{})
	var first, second *tsx.ViewRuntime
	seen := map[string][]engineSpan{}
	engine := engineCostText{phase: "prepare", probe: func(word string) {
		if (word == "host work" || word == "first view") && seen[word] == nil {
			seen[word] = sampleEngineSpans(first, second)
		}
	}}
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.Text{Content: "host work"},
			tsx.View{Package: p, Sandbox: tsx.Sandbox{OnViewRuntime: func(v *tsx.ViewRuntime) { first = v }}},
			tsx.View{Package: q, Sandbox: tsx.Sandbox{OnViewRuntime: func(v *tsx.ViewRuntime) { second = v }}},
		}}
	}, nsgotest.TextEngine(engine))
	app.Find("text:host work")
	for _, word := range []string{"host work", "first view"} {
		spans := seen[word]
		if len(spans) != 2 {
			t.Fatalf("no samples during %q", word)
		}
		checkEngineSpan(t, spans[0], word == "first view")
		checkEngineSpan(t, spans[1], false)
	}
}

func TestViewEngineTimeIncludesReactiveGoWork(t *testing.T) {
	p := verified(t, `import { mount, signal, Text } from "nsgo";
import { Owner } from "app/engine-owner";
const n = signal(0);
mount(() => <><Owner tick={n()}/><Text testID="tick" onPress={() => n.set(n() + 1)}>tick</Text></>);`, pkg.Capabilities{})
	var view *tsx.ViewRuntime
	app := nsgotest.Start(t, tsx.View{Package: p, Sandbox: tsx.Sandbox{
		Modules: []string{"app/engine-owner"}, OnViewRuntime: func(v *tsx.ViewRuntime) { view = v },
	}}.Render)
	var spans []engineSpan
	engineProbe.Lock()
	engineProbe.run = func() { spans = sampleEngineSpans(view) }
	engineProbe.Unlock()
	t.Cleanup(func() { engineProbe.Lock(); engineProbe.run = nil; engineProbe.Unlock() })
	app.Press("tick")
	if len(spans) != 1 {
		t.Fatal("the Go reactive effect did not run")
	}
	checkEngineSpan(t, spans[0], true)
}

func TestViewEngineTimeSwitchesNestedOwners(t *testing.T) {
	p := verified(t, `import { mount, Text } from "nsgo";
import { Owner } from "app/engine-owner";
import { work } from "app/engine-cost";
mount(() => <><Owner name="first"/><Text testID="work" onPress={() => work()}>work</Text></>);`, pkg.Capabilities{Host: []string{"probe"}})
	q := verified(t, `import { mount } from "nsgo";
import { Owner } from "app/engine-owner";
mount(() => <Owner name="second"/>);`, pkg.Capabilities{})
	engineProbe.Lock()
	engineProbe.scopes = map[string]*core.Scope{}
	engineProbe.Unlock()
	t.Cleanup(func() { engineProbe.Lock(); engineProbe.scopes = nil; engineProbe.run = nil; engineProbe.Unlock() })
	var first, second *tsx.ViewRuntime
	var host *core.Scope
	var samples [][]engineSpan
	channel := core.NewHost()
	channel.Handle("probe", func(*core.HostCall) {
		samples = append(samples, sampleEngineSpans(first, second))
	})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		host = cx
		return core.Group{
			tsx.View{Package: p, Sandbox: tsx.Sandbox{Host: channel, Modules: []string{"app/engine-owner", "app/engine-cost"}, OnViewRuntime: func(v *tsx.ViewRuntime) { first = v }}},
			tsx.View{Package: q, Sandbox: tsx.Sandbox{Modules: []string{"app/engine-owner"}, OnViewRuntime: func(v *tsx.ViewRuntime) { second = v }}},
		}
	})
	engineProbe.Lock()
	sibling := engineProbe.scopes["second"]
	owned := engineProbe.scopes["first"]
	engineProbe.run = func() {
		samples = append(samples, sampleEngineSpans(first, second))
		host.Run(func() { samples = append(samples, sampleEngineSpans(first, second)) })
		sibling.Run(func() { samples = append(samples, sampleEngineSpans(first, second)) })
		owned.PostHost("probe", nil)
		samples = append(samples, sampleEngineSpans(first, second))
	}
	engineProbe.Unlock()
	if sibling == nil {
		t.Fatal("the sibling scope was not mounted")
	}
	app.Press("work")
	if len(samples) != 5 {
		t.Fatal("nested owner work did not run")
	}
	for i, spans := range samples {
		checkEngineSpan(t, spans[0], i == 0 || i == 4)
		checkEngineSpan(t, spans[1], i == 2)
	}
	for _, view := range []*tsx.ViewRuntime{first, second} {
		checkEngineSpan(t, sampleEngineSpans(view)[0], false)
	}
}

func TestViewEngineTimeUnavailableWithoutRuntime(t *testing.T) {
	for _, view := range []*tsx.ViewRuntime{nil, {}} {
		if elapsed, ok := view.EngineTime(); elapsed != 0 || ok {
			t.Fatalf("empty view EngineTime = %v, %v", elapsed, ok)
		}
	}
}

func TestViewEngineTimeExcludesDeferredHostWorkAndDisposal(t *testing.T) {
	p := verified(t, `import { mount, Text } from "nsgo";
import { work } from "app/engine-cost";
mount(() => <Text testID="work" onPress={() => work()}>work</Text>);`, pkg.Capabilities{})
	var view *tsx.ViewRuntime
	var tick *core.Signal[int]
	var shown *core.Signal[bool]
	var spans []engineSpan
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		tick = core.NewSignal(cx, 0)
		shown = core.NewSignal(cx, true)
		cx.Effect(func() {
			if tick.Get() > 0 {
				spans = sampleEngineSpans(view)
			}
		})
		return core.Show{When: shown.Get, Then: tsx.View{Package: p, Sandbox: tsx.Sandbox{
			Modules: []string{"app/engine-cost"}, OnViewRuntime: func(v *tsx.ViewRuntime) { view = v },
		}}.Render}
	})
	engineProbe.Lock()
	engineProbe.run = func() { tick.Set(1) }
	engineProbe.Unlock()
	t.Cleanup(func() { engineProbe.Lock(); engineProbe.run = nil; engineProbe.Unlock() })
	app.Press("work")
	if len(spans) != 1 {
		t.Fatal("the deferred host effect did not run")
	}
	checkEngineSpan(t, spans[0], false)
	app.Post(func() { shown.Set(false) })
	app.Settle()
	if !view.Stopped() {
		t.Fatal("the removed view did not stop")
	}
	checkEngineSpan(t, sampleEngineSpans(view)[0], false)
	app.Post(func() { tick.Set(2) })
	app.Settle()
	checkEngineSpan(t, spans[0], false)
}
