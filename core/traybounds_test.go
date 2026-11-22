package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestTrayClickReportsCurrentBoundsWithoutChangingCallbacks(t *testing.T) {
	var tray *Tray
	var got Bounds
	var available bool
	var clicks int
	var panel *Window
	w := newWindows(t, func(cx *Scope) {
		panel = cx.OpenPanel(PanelOptions{}, func(*Scope) Node { return Text{Content: "popup"} })
		tray = cx.NewTray(TrayOptions{OnClick: func() {
			clicks++
			got, available = tray.ClickBounds()
			if available {
				if err := panel.ShowAt(got, PlaceBottom); err != nil {
					t.Fatal(err)
				}
			}
		}})
	})
	if _, ok := tray.ClickBounds(); ok {
		t.Fatal("a new tray has bounds before a native click")
	}
	want := Bounds{Origin: Point{X: -200, Y: 8}, Size: Size{Width: 62, Height: 24}}
	w.dispatch(0, driver.Event{Kind: driver.EventTray, Count: tray.cfg.ID, Data: want})
	if clicks != 1 || !available || got != want {
		t.Fatalf("clicks%d bounds%+v available%v", clicks, got, available)
	}
	if commands := w.take(panel.t.id, driver.ItemWindow); len(commands) != 1 ||
		commands[0].Window.AnchorBounds != want || commands[0].Window.Placement != driver.WindowPlaceBottom {
		t.Fatalf("popup did not receive the click's screen bounds: %+v", commands)
	}
	// A later click with unavailable geometry must not reuse stale bounds.
	w.dispatch(0, driver.Event{Kind: driver.EventTray, Count: tray.cfg.ID})
	if clicks != 2 || available || got != (Bounds{}) {
		t.Fatalf("unavailable click: clicks%d bounds%+v available%v", clicks, got, available)
	}
	w.dispatch(0, driver.Event{Kind: driver.EventTray, Count: tray.cfg.ID, Data: want})
	tray.Remove()
	if _, ok := tray.ClickBounds(); ok {
		t.Fatal("a removed tray retained available bounds")
	}
	var none *Tray
	if _, ok := none.ClickBounds(); ok {
		t.Fatal("a nil tray reported bounds")
	}
}

func TestTrayMenuReceivesItsOwnClickBounds(t *testing.T) {
	var first, second *Tray
	var got Bounds
	var available bool
	w := newWindows(t, func(cx *Scope) {
		first = cx.NewTray(TrayOptions{})
		second = cx.NewTray(TrayOptions{Menu: func() []MenuItem {
			got, available = second.ClickBounds()
			return nil
		}})
	})
	want := Bounds{Origin: Point{X: 900, Y: 700}, Size: Size{Width: 32, Height: 32}}
	w.dispatch(0, driver.Event{Kind: driver.EventTray, Count: second.cfg.ID, Index: 1, Data: want})
	if !available || got != want {
		t.Fatalf("secondary menu bounds%+v available%v", got, available)
	}
	if _, ok := first.ClickBounds(); ok {
		t.Fatal("a different tray received the click's bounds")
	}
}
