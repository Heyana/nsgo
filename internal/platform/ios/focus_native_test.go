//go:build ios && nsgotest

package ios

import (
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The remote has no UIKey. Exercise the mounted view's native press responders separately from the keyboard HID regression, including its long-press timer.
func TestTVRemoteSelectKeepsItsPressLifecycle(t *testing.T) {
	if !onTV() {
		t.Skip("Apple TV remote")
	}
	var ref core.Ref
	var phases []string
	var got []core.PressEvent
	shortcuts := 0
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, func() { shortcuts++ })
		record := func(phase string) func(core.PressEvent) {
			return func(e core.PressEvent) { phases = append(phases, phase); got = append(got, e) }
		}
		return core.View{Ref: &ref, TestID: "remote", Style: core.Style{Width: core.Pt(180), Height: core.Pt(50)},
			DelayLongPress: 30 * time.Millisecond,
			OnPressDown:    record("down"), OnPressUp: record("up"), OnPress: record("press"), OnLongPress: record("long")}
	})
	app.Post(ref.Focus)
	app.WaitUntil(5*time.Second, func() bool { return app.Find("id:remote").Focused })
	id := app.Find("id:remote").ID
	var press, presses objc.ID
	onMainSync(func() {
		class := register("NsgoRemoteSelectTestPress", apple.Class("NSObject"), nil, []objc.MethodDef{
			{Cmd: sel("type"), Fn: func(objc.ID, objc.SEL) int { return pressSelect }},
			{Cmd: sel("key"), Fn: func(objc.ID, objc.SEL) objc.ID { return 0 }},
		})
		press = objc.ID(class).Send(selNew)
		presses = objc.ID(apple.Class("NSSet")).Send(sel("setWithObject:"), press).Send(sel("retain"))
	})
	defer onMainSync(func() { presses.Send(selRelease); press.Send(selRelease) })
	send := func(selector string) {
		onMainSync(func() { runner.views[id].native.Send(sel(selector), presses, objc.ID(0)) })
		app.Settle()
	}
	onMainSync(func() {
		runner.views[id].native.Send(sel("pressesBegan:withEvent:"), presses, objc.ID(0))
		runner.views[id].native.Send(sel("pressesEnded:withEvent:"), presses, objc.ID(0))
	})
	app.Settle()
	if len(phases) != 3 || phases[0] != "down" || phases[1] != "up" || phases[2] != "press" {
		t.Fatalf("remote tap phases=%v", phases)
	}
	send("pressesBegan:withEvent:")
	app.WaitUntil(5*time.Second, func() bool { return len(phases) == 5 })
	send("pressesEnded:withEvent:")
	if len(phases) != 6 || phases[3] != "down" || phases[4] != "long" || phases[5] != "up" || shortcuts != 0 {
		t.Fatalf("remote hold phases=%v, shortcuts=%d", phases, shortcuts)
	}
	for _, e := range got {
		if e.Modifiers != (core.Modifiers{}) || e.Position != (core.Point{}) || e.Clicks != 1 {
			t.Fatalf("remote press=%+v, want unmodified at the origin with one click", e)
		}
	}
}
