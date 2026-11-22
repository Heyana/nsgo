//go:build ios && nsgotest

package ios_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/ios"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A pan gate that goes away mid-drag lets go of its touch and its scroll handoff (#1950).
func TestPanGateReleasesWhatItHoldsWhenItGoesAway(t *testing.T) {
	const wait = 5 * time.Second
	pannable := func(id string) core.View {
		return core.View{TestID: id, OnPan: func(core.PanEvent) {}, Style: core.Style{Width: core.Pt(120), Height: core.Pt(120)}}
	}
	var hide func()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		shown := core.NewSignal(cx, true)
		hide = func() { shown.Set(false) }
		return core.View{Children: []core.Node{
			pannable("kept"),
			core.Show{When: shown.Get, Then: func(*core.Scope) core.Node { return pannable("deleted") }},
		}}
	})
	app.WaitFor("id:deleted", wait)

	remove, held := ios.HoldPanForTest("kept")
	if remove == nil {
		t.Fatal("the kept view has no pan gate")
	}
	remove()
	if held() {
		t.Error("removing a view's pan kept its touch or scroll handoff")
	}

	_, held = ios.HoldPanForTest("deleted")
	if held == nil {
		t.Fatal("the deleted view has no pan gate")
	}
	app.Post(hide)
	app.WaitUntil(wait, func() bool { return !held() })
}
