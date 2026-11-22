package main

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestStepWaitsForTheDialogCommit(t *testing.T) {
	app := nsgotest.Start(t, Root, nsgotest.Size(600, 480), nsgotest.Environment(core.Environment{ReduceMotion: true}))
	app.Press("id:open")
	app.Find(`text:"1 / 3"`)
	if got := app.Find("id:guide").Bounds().Size; got.Width != 600 || got.Height != 480 {
		t.Fatalf("dialog size = %+v, want the full 600 by 480 point window", got)
	}
}

// TestWaitTakesTheFramesOfADialogClosing closes a dialog from the app's Open: the wait moves the clock past the motion the core times its leaving by, as headless.
func TestWaitTakesTheFramesOfADialogClosing(t *testing.T) {
	var open *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		open = core.NewSignal(cx, false)
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			ui.Dialog{TestID: "dialog", Open: open.Get, Title: "Hi", OnOpenChange: func(o bool, _ base.Reason) { open.Set(o) }},
		}}
	}, nsgotest.Size(600, 400))
	app.Post(func() { open.Set(true) })
	app.WaitFor("id:dialog", time.Second)
	app.Post(func() { open.Set(false) })
	app.WaitForGone("id:dialog", 3*time.Second)
	app.Post(func() { open.Set(true) })
	app.WaitFor("id:dialog", time.Second)
	// A step takes them too.
	app.Key("Escape")
	if app.Exists("id:dialog") {
		t.Fatal("the dialog stayed after Escape closed it")
	}
}

func TestSettleBoundsAnAppThatKeepsChanging(t *testing.T) {
	if nsgotest.Target() == "" {
		t.Skip("checks the target's real-time settle deadline")
	}
	begin, stop, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(stop) }); <-done }
	var updates atomic.Int32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		text := core.NewSignal(cx, "0")
		go func() {
			defer close(done)
			select {
			case <-stop:
				return
			case <-begin:
			}
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					cx.Post(func() { text.Set(strconv.Itoa(int(updates.Add(1)))) })
				}
			}
		}()
		return core.Text{ContentFunc: text.Get}
	})
	t.Cleanup(finish)
	close(begin)
	start := time.Now()
	app.Settle()
	elapsed := time.Since(start)
	finish()
	if elapsed < 4*time.Second || elapsed > 6*time.Second || updates.Load() < 10 {
		t.Fatalf("Settle took %s over %d updates, want the five-second bound", elapsed, updates.Load())
	}
}
