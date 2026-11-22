package core

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestEdgeDockedPanelKeepsItsPlacementContract(t *testing.T) {
	var panel *Window
	var inside *Scope
	opts := PanelDock{Edge: PlaceBottom, Display: "screen-2", Margin: 12}
	w := newWindows(t, func(cx *Scope) {
		panel = cx.OpenPanel(PanelOptions{Width: 120, Height: 60, NoActivate: true, Dock: opts}, func(cx *Scope) Node { inside = cx; return View{} })
	})
	opened := w.take(0, driver.ItemOpenSurface)
	if len(opened) != 1 || opened[0].Surface.Dock != (driver.PanelDock{Edge: driver.WindowPlaceBottom, Display: "screen-2", Margin: 12}) || !opened[0].Surface.NoActivate {
		t.Fatalf("panel config: %+v", opened)
	}
	if err := panel.ShowAt(Bounds{}, PlaceTop); err == nil {
		t.Error("a docked panel accepted a rectangle placement")
	}
	if err := panel.MoveTo(Bounds{}, PlaceLeft); err == nil {
		t.Error("a docked panel accepted a rectangle move")
	}
	panel.showAt(99)
	w.commit()
	commands := w.take(panel.t.id, driver.ItemWindow)
	if len(commands) != 1 || commands[0].Window.Action != driver.WindowShow || commands[0].Window.Anchor != 0 {
		t.Fatalf("tray changed the dock: %+v", commands)
	}
	panel.t.SetEnvironment(Environment{CanDockPanels: true, CanPlaceWindows: true})
	if !inside.Env().CanDockPanels() || inside.Env().CanPlaceWindows() {
		t.Fatal("a docked panel did not distinguish docking from arbitrary placement")
	}
	inside.SetWindowFrame(WindowFrame{X: 999, Y: 999, Move: true, Width: 200})
	w.commit()
	commands = w.take(panel.t.id, driver.ItemWindow)
	if len(commands) != 1 || commands[0].Window.Action != driver.WindowFitSize || commands[0].Window.Frame != (WindowFrame{Width: 200, Height: 60}) {
		t.Fatalf("docked size change: %+v", commands)
	}
}

func TestPanelDockNormalizesMarginsAndLeavesZeroUndocked(t *testing.T) {
	for _, margin := range []Points{-4, Points(math.NaN()), Points(math.Inf(1))} {
		w := newWindows(t, func(cx *Scope) {
			cx.OpenPanel(PanelOptions{Dock: PanelDock{Edge: PlaceTop, Margin: margin}}, func(*Scope) Node { return View{} })
		})
		opened := w.take(0, driver.ItemOpenSurface)
		if len(opened) != 1 || opened[0].Surface.Dock.Margin != 0 {
			t.Fatalf("margin %v: %+v", margin, opened)
		}
	}
	var panel *Window
	w := newWindows(t, func(cx *Scope) { panel = cx.OpenPanel(PanelOptions{}, func(*Scope) Node { return View{} }) })
	if err := panel.ShowAt(Bounds{Origin: Point{X: 50, Y: 70}}, PlaceRight); err != nil {
		t.Fatal(err)
	}
	w.commit()
	commands := w.take(panel.t.id, driver.ItemWindow)
	if len(commands) != 1 || commands[0].Window.Placement != driver.WindowPlaceRight {
		t.Fatalf("ordinary panel: %+v", commands)
	}
}
