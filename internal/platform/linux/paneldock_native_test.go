//go:build linux && !android

package linux

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestX11DockedPanelsFollowNativeGeometry(t *testing.T) {
	if os.Getenv("NSGO_X11_DOCK_TEST") != "1" {
		t.Skip("needs an isolated X11 display")
	}
	type panelCase struct {
		panel *core.Window
		cx    *core.Scope
		dock  driver.PanelDock
	}
	ready := make(chan []panelCase, 1)
	var appScope *core.Scope
	own := startTestWindow(t, core.AppRoot(func(cx *core.Scope) {
		appScope = cx
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		var cases []panelCase
		for _, display := range cx.Env().Displays() {
			for _, edge := range []core.Placement{core.PlaceTop, core.PlaceBottom, core.PlaceLeft, core.PlaceRight} {
				c := panelCase{dock: driver.PanelDock{Edge: windowDockEdge(edge), Display: display.ID, Margin: 10}}
				c.panel = cx.OpenPanel(core.PanelOptions{Width: 120, Height: 60, NoActivate: true, Dock: core.PanelDock{Edge: edge, Display: display.ID, Margin: 10}}, func(cx *core.Scope) core.Node {
					c.cx = cx
					return core.View{}
				})
				cases = append(cases, c)
			}
		}
		ready <- cases
	}), Options{ID: "dev.nsgo.test.edge1377", Width: 320, Height: 200, Hidden: true})
	defer own.onLoop(t, own.close)
	cases := <-ready
	var x11 bool
	own.onLoop(t, func() { x11 = drawsOnX11() })
	if !x11 || len(cases) == 0 {
		t.Fatal("the opt-in fixture requires X11 outputs")
	}
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	var geometry func(uintptr, uintptr, *uintptr, *int32, *int32, *uint32, *uint32, *uint32, *uint32) int32
	var syncX func(uintptr, int32) int32
	purego.RegisterLibFunc(&geometry, lib, "XGetGeometry")
	purego.RegisterLibFunc(&syncX, lib, "XSync")
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s/edge%d", c.dock.Display, c.dock.Edge), func(t *testing.T) {
			appScope.Post(c.panel.Show)
			for _, size := range []core.Size{{Width: 120, Height: 60}, {Width: 160, Height: 80}, {Width: 80, Height: 40}} {
				appScope.Post(func() { c.cx.SetWindowFrame(core.WindowFrame{Width: size.Width, Height: size.Height}) })
				var got, want driver.WindowFrame
				var allocated bool
				for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
					own.onLoop(t, func() {
						windowHandles.Range(func(_, value any) bool {
							w := value.(*window)
							if w.opener != own || w.dock != c.dock {
								return true
							}
							display, xid, ok := w.x11Window()
							if !ok {
								return true
							}
							syncX(display, 0)
							var root uintptr
							var x, y int32
							var width, height, border, depth uint32
							geometry(display, xid, &root, &x, &y, &width, &height, &border, &depth)
							scale := driver.Points(max(gtkWidgetGetScaleFactor(w.win), 1))
							got = driver.WindowFrame{X: driver.Points(x) / scale, Y: driver.Points(y) / scale, Width: driver.Points(width) / scale, Height: driver.Points(height) / scale, Move: true}
							want, _ = driver.DockWindow(size, displays(), c.dock)
							allocated = gtkWidgetGetWidth(w.surface) == int32(size.Width) && gtkWidgetGetHeight(w.surface) == int32(size.Height)
							return true
						})
					})
					if got == want && got.Width > 0 && allocated {
						break
					}
					if time.Now().After(deadline) {
						t.Fatalf("native X11 frame%+v, want%+v", got, want)
					}
				}
			}
			appScope.Post(c.panel.Hide)
		})
	}
}

// TestLayerShellDocksPanelsOnMixedScaleOutputs captures only an explicitly owned headless compositor, including the second output's actual 2x pixels.
func TestLayerShellDocksPanelsOnMixedScaleOutputs(t *testing.T) {
	if os.Getenv("NSGO_LAYER_SHELL_TEST") != "1" {
		t.Skip("needs an isolated headless layer-shell compositor and grim")
	}
	grim, err := exec.LookPath("grim")
	if err != nil {
		t.Fatal("the opt-in layer-shell fixture requires grim")
	}
	wtype, err := exec.LookPath("wtype")
	if err != nil {
		t.Fatal("the opt-in layer-shell fixture requires wtype")
	}
	type panelCase struct {
		display core.DisplayInfo
		edge    core.Placement
		panel   *core.Window
		resize  func(core.Size)
	}
	type setup struct {
		cx        *core.Scope
		cases     []panelCase
		available bool
	}
	ready := make(chan setup, 1)
	keys := make(chan struct{}, 1)
	const body = uint32(0x406080)
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		cx.OpenWindow(core.WindowOptions{Title: "nsgo-dock-keyboard", Width: 320, Height: 200}, func(cx *core.Scope) core.Node {
			cx.OnKeyWith("a", core.KeyOptions{When: core.KeyAlways}, func() { keys <- struct{}{} })
			return core.View{Style: core.Style{Width: core.Pt(320), Height: core.Pt(200)}}
		})
		s := setup{cx: cx, available: cx.Env().CanDockPanels()}
		for _, display := range cx.Env().Displays() {
			for _, edge := range []core.Placement{core.PlaceTop, core.PlaceBottom, core.PlaceLeft, core.PlaceRight} {
				var resize func(core.Size)
				p := cx.OpenPanel(core.PanelOptions{Width: 200, Height: 120, FitContent: true, NoActivate: true, Dock: core.PanelDock{Edge: edge, Display: display.ID, Margin: 10}}, func(cx *core.Scope) core.Node {
					size := core.NewSignal(cx, core.Size{Width: 120, Height: 60})
					resize = size.Set
					return core.View{StyleFunc: func() core.Style {
						s := size.Get()
						return core.Style{Width: core.Pt(s.Width), Height: core.Pt(s.Height), Background: core.RGB(body)}
					}}
				})
				s.cases = append(s.cases, panelCase{display, edge, p, resize})
			}
		}
		ready <- s
	})
	own := startTestWindow(t, app, Options{ID: "dev.nsgo.test.edge1377", Title: "nsgo-layer-dock", Width: 320, Height: 200, Hidden: true})
	defer own.onLoop(t, own.close)
	s := <-ready
	var x11 bool
	own.onLoop(t, func() { x11 = drawsOnX11() })
	if x11 {
		t.Fatal("the opt-in layer-shell fixture must use GDK_BACKEND=wayland")
	}
	if !s.available {
		t.Fatalf("native docking is unavailable: %s", loadLayerShell().issue)
	}
	if len(s.cases) < 8 {
		t.Fatal("the opt-in fixture requires two headless outputs")
	}
	firstScale := s.cases[0].display.Scale
	mixed := false
	for _, c := range s.cases {
		if !strings.HasPrefix(c.display.ID, "HEADLESS-") {
			t.Fatal("refusing to capture an output outside the owned headless compositor")
		}
		mixed = mixed || c.display.Scale != firstScale
	}
	if !mixed {
		t.Fatal("the opt-in fixture requires different output scales")
	}
	keyboard := func(t *testing.T) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, wtype, "-s", "100", "-k", "a").CombinedOutput(); err != nil {
			t.Fatalf("type on the owned seat: %v: %s", err, out)
		}
		select {
		case <-keys:
		case <-ctx.Done():
			t.Fatal("the original window did not receive the native keyboard press")
		}
	}
	keyboard(t)
	for _, c := range s.cases {
		t.Run(fmt.Sprintf("%s/edge%d", c.display.ID, c.edge), func(t *testing.T) {
			if c.panel == nil {
				t.Fatal("the panel did not open")
			}
			s.cx.Post(c.panel.Show)
			var native *window
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
				own.onLoop(t, func() {
					windowHandles.Range(func(_, value any) bool {
						w := value.(*window)
						if w.opener == own && w.dock.Display == c.display.ID && w.dock.Edge == windowDockEdge(c.edge) {
							native = w
						}
						return true
					})
				})
				if native != nil {
					break
				}
			}
			if native == nil {
				t.Fatal("the docked native surface was not attached")
			}
			own.onLoop(t, func() {
				if !native.dockLayer || native.dockIssue != "" {
					t.Errorf("layer role: initialized%v issue%q", native.dockLayer, native.dockIssue)
				}
				if gtkWindowIsActive(native.win) != 0 {
					t.Error("a nonactivating layer took native focus")
				}
			})
			for _, size := range []core.Size{{Width: 120, Height: 60}, {Width: 160, Height: 80}, {Width: 80, Height: 40}} {
				s.cx.Post(func() { c.resize(size) })
				expectDockedPixels(t, grim, c.display, c.edge, size, body)
				keyboard(t)
			}
			s.cx.Post(c.panel.Hide)
			for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
				hidden := false
				own.onLoop(t, func() { hidden = gtkWidgetGetVisible(native.win) == 0 })
				if hidden {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("the native layer stayed visible after Hide")
				}
			}
			s.cx.Post(c.panel.Show)
			expectDockedPixels(t, grim, c.display, c.edge, core.Size{Width: 80, Height: 40}, body)
			keyboard(t)
			s.cx.Post(c.panel.Hide)
		})
	}
}

// Run this opt-in fixture without the optional library, or with libwayland preloaded as an embedding host can load it before NSGo starts.
func TestUnavailableLayerShellKeepsDockedPanelsHidden(t *testing.T) {
	wantIssue := os.Getenv("NSGO_LAYER_SHELL_UNAVAILABLE")
	if wantIssue == "" {
		t.Skip("needs an isolated Wayland compositor and an unavailable layer-shell loader")
	}
	assertDockUnavailable(t, "", false, wantIssue)
}

func TestLayerShellMissingDisplayKeepsThePanelHidden(t *testing.T) {
	if os.Getenv("NSGO_LAYER_SHELL_TEST") != "1" {
		t.Skip("needs an isolated headless layer-shell compositor")
	}
	assertDockUnavailable(t, "nsgo-test-missing-output", true, "display is unavailable")
}

func assertDockUnavailable(t *testing.T, display string, available bool, wantIssue string) {
	t.Helper()
	issues := make(chan error, 1)
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		if cx.Env().CanDockPanels() != available {
			t.Errorf("docking capability: want %v", available)
		}
		cx.OpenWindow(core.WindowOptions{Width: 120, Height: 60}, func(*core.Scope) core.Node { return core.View{} })
		panel := cx.OpenPanel(core.PanelOptions{Width: 120, Height: 60, Dock: core.PanelDock{Edge: core.PlaceTop, Display: display}}, func(*core.Scope) core.Node { return core.View{} })
		if panel == nil {
			issues <- fmt.Errorf("the panel could not be created")
			return
		}
		panel.OnPlacementError(cx, func(err error) { issues <- err })
		panel.Show()
	})
	own := startTestWindow(t, app, Options{ID: "dev.nsgo.test.edge1377", Width: 320, Height: 200, Hidden: true})
	defer own.onLoop(t, own.close)
	select {
	case err := <-issues:
		if err == nil || !strings.Contains(err.Error(), wantIssue) {
			t.Fatalf("placement error%v, want %q", err, wantIssue)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the unavailable panel reported no placement error")
	}
	own.onLoop(t, func() {
		var ordinary, docked bool
		windowHandles.Range(func(_, value any) bool {
			w := value.(*window)
			if w.opener == own {
				if w.dock.Edge == driver.WindowPlaceDefault {
					ordinary = gtkWidgetGetVisible(w.win) != 0
				} else {
					docked = true
					if gtkWidgetGetVisible(w.win) != 0 {
						t.Error("the unavailable panel became an ordinary visible toplevel")
					}
				}
			}
			return true
		})
		if !ordinary || !docked {
			t.Errorf("native windows: ordinary%v docked%v", ordinary, docked)
		}
	})
}

func expectDockedPixels(t *testing.T, grim string, display core.DisplayInfo, edge core.Placement, size core.Size, body uint32) {
	t.Helper()
	x, y := (display.Frame.Size.Width-size.Width)/2, driver.Points(10)
	switch edge {
	case core.PlaceBottom:
		y = display.Frame.Size.Height - size.Height - 10
	case core.PlaceLeft:
		x, y = 10, (display.Frame.Size.Height-size.Height)/2
	case core.PlaceRight:
		x, y = display.Frame.Size.Width-size.Width-10, (display.Frame.Size.Height-size.Height)/2
	}
	scale := float64(display.Scale)
	pixel := func(v driver.Points) int { return int(math.Round(float64(v) * scale)) }
	want := image.Rect(pixel(x), pixel(y), pixel(x+size.Width), pixel(y+size.Height))
	var got image.Rectangle
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		data, err := exec.CommandContext(ctx, grim, "-o", display.ID, "-").Output()
		cancel()
		if err != nil {
			t.Fatalf("capture owned output: %v", err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		got = matchingPanelBounds(img, body)
		if got == want {
			break
		}
	}
	if got != want {
		t.Fatalf("native output pixels: panel%v, want%v at scale%v", got, want, display.Scale)
	}
}

func windowDockEdge(edge core.Placement) driver.WindowPlacement {
	switch edge {
	case core.PlaceTop:
		return driver.WindowPlaceTop
	case core.PlaceBottom:
		return driver.WindowPlaceBottom
	case core.PlaceLeft:
		return driver.WindowPlaceLeft
	default:
		return driver.WindowPlaceRight
	}
}

func matchingPanelBounds(img image.Image, rgb uint32) image.Rectangle {
	var bounds image.Rectangle
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a == 0xffff && uint32(r>>8)<<16|uint32(g>>8)<<8|uint32(b>>8) == rgb {
				p := image.Rect(x, y, x+1, y+1)
				if bounds.Empty() {
					bounds = p
				} else {
					bounds = bounds.Union(p)
				}
			}
		}
	}
	return bounds
}
