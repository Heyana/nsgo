package windowstate_test

import (
	"math"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestPanelPlacementKeepsNativeFocus(t *testing.T)     { checkPanelPlacement(t, false) }
func TestPanelPlacementOnMixedScaleDisplays(t *testing.T) { checkPanelPlacement(t, true) }

func TestWindowStateReadsNativeVisibility(t *testing.T) {
	var scope *core.Scope
	app := nsgotest.StartApp(t, func(cx *core.Scope) {
		scope = cx
		cx.SetMainView(func(*core.Scope) core.Node { return core.View{TestID: "main"} })
	})
	window := app.Surface("id:main")
	app.WaitUntil(5*time.Second, func() bool { return window.WindowState().Visible })
	initial := window.WindowState()
	if initial.Frame.Width <= 0 || initial.Frame.Height <= 0 {
		t.Fatalf("native window has no frame: %+v", initial)
	}
	app.Post(func() { scope.MainWindow().Hide() })
	app.WaitUntil(5*time.Second, func() bool { return !window.WindowState().Visible })
	if state := window.WindowState(); state.Focused {
		t.Fatalf("hidden native window kept the keyboard: %+v", state)
	}
	app.Post(func() { scope.MainWindow().Show() })
	app.WaitUntil(5*time.Second, func() bool { return window.WindowState().Visible })
}

func checkPanelPlacement(t *testing.T, mixedScale bool) {
	t.Helper()
	switch nsgotest.Target() {
	case "macos", "windows", "linux":
	default:
		t.Skip("requires native desktop windows")
	}
	var scope *core.Scope
	var panel *core.Window
	var displays []core.DisplayInfo
	app := nsgotest.StartApp(t, func(cx *core.Scope) {
		scope = cx
		displays = cx.Env().Displays()
		cx.SetMainView(func(cx *core.Scope) core.Node {
			value := core.NewSignal(cx, "")
			return core.View{TestID: "main", Children: []core.Node{
				core.Input{TestID: "field", AutoFocus: true, Value: value.Get, OnChange: value.Set,
					Style: core.Style{Width: core.Pt(240), Height: core.Pt(40)}},
			}}
		})
		panel = cx.OpenPanel(core.PanelOptions{Width: 120, Height: 60, NoActivate: true}, func(*core.Scope) core.Node {
			return core.View{TestID: "panel", Style: core.Style{Background: core.RGB(0x334455)}}
		})
	})
	if len(displays) == 0 {
		t.Fatal("the native environment reported no display")
	}
	display := displays[0]
	if mixedScale {
		found := false
		for _, candidate := range displays[1:] {
			if candidate.Scale != display.Scale {
				display = candidate
				found = true
				break
			}
		}
		if !found {
			t.Skip("requires a second native display with a different scale")
		}
	}
	area := display.VisibleFrame
	if area.Size.Width < 500 || area.Size.Height < 300 {
		t.Skip("the native display's work area is too small for this fixture")
	}
	main := app.Surface("id:main")
	popup := app.Surface("id:panel")
	app.Post(func() { scope.MainWindow().Focus() })
	main.Type("id:field", "A")
	app.WaitUntil(5*time.Second, func() bool { return main.WindowState().Focused })
	if state := popup.WindowState(); state.Visible || state.Focused {
		t.Fatalf("new hidden panel state: %+v", state)
	}
	anchor := core.Bounds{Origin: core.Point{X: area.Origin.X + 160, Y: area.Origin.Y + 80}, Size: core.Size{Width: 120, Height: 20}}
	check := func(x, y core.Points, visible bool) {
		t.Helper()
		app.WaitUntil(5*time.Second, func() bool {
			s := popup.WindowState()
			return s.Visible == visible && (!visible || s.Frame.Move && near(s.Frame.X, x) && near(s.Frame.Y, y))
		})
		state := popup.WindowState()
		if state.Focused || !main.WindowState().Focused {
			t.Fatalf("panel changed native focus: panel%+v main%+v", state, main.WindowState())
		}
		if visible && (!near(state.Frame.Width, 120) || !near(state.Frame.Height, 60)) {
			t.Fatalf("native panel size: %+v", state.Frame)
		}
	}
	app.Post(func() {
		if err := panel.ShowAt(anchor, core.PlaceBottom); err != nil {
			t.Error(err)
		}
	})
	check(anchor.Origin.X, anchor.Origin.Y+26, true)
	// Moving preserves visibility and the other window's keyboard focus.
	anchor.Origin.X += 70
	anchor.Origin.Y += 40
	moved := anchor
	app.Post(func() {
		if err := panel.MoveTo(moved, core.PlaceRight); err != nil {
			t.Error(err)
		}
	})
	x, y := moved.Origin.X+126, moved.Origin.Y-20
	check(x, y, true)
	app.Post(panel.Hide)
	check(x, y, false)
	app.Post(panel.Focus)
	check(x, y, true)
	app.Post(panel.Toggle)
	check(x, y, false)
	app.Post(panel.Toggle)
	check(x, y, true)
	main.Type("id:field", "b")
	if got := main.Find("id:field").Text; got != "Ab" {
		t.Fatalf("keyboard/caret after panel placement: %q", got)
	}
}

func near(a, b core.Points) bool { return math.Abs(float64(a-b)) <= 1 }
