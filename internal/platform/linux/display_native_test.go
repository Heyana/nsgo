//go:build linux && !android

package linux

import (
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/core"
)

func TestDisplayFollowsAWindowAcrossWaylandOutputs(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() { cx.SetTitle(cx.Env().Display().ID) })
		return core.View{}
	}, Options{Title: "nsgo-window-display", Width: 320, Height: 220})
	var multiOutput bool
	w.onLoop(t, func() { multiOutput = !drawsOnX11() && len(w.env.Displays) > 1 })
	if !multiOutput {
		t.Skip("requires a Wayland compositor with multiple outputs")
	}

	var initial string
	for deadline := time.Now().Add(5 * time.Second); initial == ""; time.Sleep(10 * time.Millisecond) {
		w.onLoop(t, func() { initial = w.env.DisplayID })
		if time.Now().After(deadline) {
			t.Fatal("the window has no initial display")
		}
	}
	waitForTitle(t, w, initial)

	var next string
	w.onLoop(t, func() {
		var fullScreenOnMonitor func(uintptr, uintptr)
		purego.RegisterLibFunc(&fullScreenOnMonitor, gtkLib, "gtk_window_fullscreen_on_monitor")
		monitors := gdkDisplayGetMonitors(gdkDisplayGetDefault())
		for i, n := uint32(0), gListModelGetNItems(monitors); i < n; i++ {
			monitor := gListModelGetItem(monitors, i)
			id := monitorDisplayID(monitor)
			if id != initial {
				next = id
				fullScreenOnMonitor(w.win, monitor)
			}
			gObjectUnref(monitor)
			if next != "" {
				break
			}
		}
	})
	if next == "" {
		t.Fatal("the compositor reported no second output")
	}
	waitForTitle(t, w, next)
}
