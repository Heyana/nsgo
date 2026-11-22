//go:build android && nsgotest

package android

import (
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

type nativeViewState struct {
	Nodes, Children int
	Alive           bool
	Text, Inputs    []string
	Drawings        int
	Awake           bool
	DisplayHolds    int
}

func TestDetachedViewReleasesWidgetsAndResyncsItsLiveGraph(t *testing.T) {
	var owner *core.Scope
	var count *core.Signal[int]
	var renders atomic.Int32
	window, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		renders.Add(1)
		owner = cx
		count = core.NewSignal(cx, 0)
		cx.KeepAwake("detach regression", core.KeepAwakeOptions{Display: true})
		value := core.NewSignal(cx, "")
		return core.View{Style: core.Style{Width: core.Pt(280), Height: core.Pt(180), Background: core.RGB(0xffffff), Gap: 8}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return "Count: " + strconv.Itoa(count.Get()) }, Style: core.Style{Height: core.Pt(20)}},
			core.Input{Value: value.Get, OnChange: value.Set, Style: core.Style{Height: core.Pt(40)}},
			core.Text{ContentFunc: func() string { return "Echo: " + value.Get() }},
			core.Canvas{Style: core.Style{Width: core.Pt(20), Height: core.Pt(20)}, Draw: func(d *core.Drawing, size core.Size) { d.FillRect(0, 0, float32(size.Width), float32(size.Height)) }},
		}}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := window.(*testSurface)
	s.Sync()
	pid := os.Getpid()
	state := func(op string) nativeViewState {
		t.Helper()
		data, err := s.ask(map[string]any{"op": op})
		if err != nil {
			t.Fatal(err)
		}
		var got nativeViewState
		if err := json.Unmarshal([]byte(data), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	await := func(text, input string) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			s.Sync()
			got := state("view-state")
			if got.Alive && got.Nodes > 0 && got.Drawings == 1 && got.Awake && slices.Contains(got.Text, text) && slices.Contains(got.Inputs, input) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("native state = %+v, want %q and input %q", got, text, input)
			}
		}
	}
	await("Count: 0", "")
	if err := s.Press(driver.Point{X: 40, Y: 40}, 0, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Type("before"); err != nil {
		t.Fatal(err)
	}
	await("Echo: before", "before")
	for i := 1; i <= 3; i++ {
		got := state("detach-view")
		if !got.Alive || got.Nodes != 0 || got.Children != 0 || got.Awake {
			t.Fatalf("detached native tree = %+v, want no widgets and a live process", got)
		}
		next := i
		var release func()
		owner.Post(func() {
			count.Set(next)
			release = owner.KeepAwake("detached request", core.KeepAwakeOptions{Display: true})
		})
		s.s.engine.Sync()
		// A frame/keyboard resize must not attach a view the host detached.
		time.Sleep(80 * time.Millisecond)
		if got := state("view-state"); !got.Alive || got.Nodes != 0 || got.Children != 0 || got.Awake || got.DisplayHolds != 2 {
			t.Fatalf("detached app recreated widgets: %+v", got)
		}
		release()
		if err := s.do(map[string]any{"op": "reattach-view"}); err != nil {
			t.Fatal(err)
		}
		await("Count: "+strconv.Itoa(i), "before")
		if renders.Load() != 1 || os.Getpid() != pid {
			t.Fatalf("reattach restarted the app: renders=%d pid=%d, want 1 and %d", renders.Load(), os.Getpid(), pid)
		}
	}
	if err := s.Type("-after"); err != nil {
		t.Fatal(err)
	}
	await("Echo: before-after", "before-after")
}
