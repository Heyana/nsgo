//go:build windows

package windows

import (
	"fmt"
	"math"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestEdgeDockedPanelsUseNativeMonitorWorkAreas drives core panels through
// the real window harness. Expectations come from Win32 work-area pixels,
// independently of DockWindow's logical-coordinate placement.
func TestEdgeDockedPanelsUseNativeMonitorWorkAreas(t *testing.T) {
	if testing.Short() {
		t.Skip("opens native panels")
	}
	type setup struct {
		cx *core.Scope
	}
	ready := make(chan setup, 1)
	const title = "nsgo-edge-dock1377"
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		if !cx.Env().CanDockPanels() {
			t.Error("Windows did not advertise native panel docking")
		}
		cx.OpenWindow(core.WindowOptions{Title: title + "-keyboard", Width: 320, Height: 160},
			func(*core.Scope) core.Node { return core.View{} })
		ready <- setup{cx}
	})
	root := startAppWith(t, app, Options{Title: title, Width: 320, Height: 160, Hidden: true})
	var s setup
	select {
	case s = <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("the app did not create its window scope")
	}
	var all []displayPlacement
	var keyboard uintptr
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		onMain(t, root, func(own *window) {
			all = monitorLayout()
			for hwnd, w := range windows {
				if w.hostWindow == own && w.kind == driver.SurfaceWindow {
					keyboard = hwnd
				}
			}
		})
		if keyboard != 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the keyboard window did not open")
		}
	}
	// An independent native control distinguishes an unavailable desktop
	// from an NSGo show regression. A disconnected Windows session can
	// keep even WS_VISIBLE windows invisible through their desktop parent.
	var controlVisible, keyboardVisible bool
	var controlStyle uintptr
	var controlError error
	onMain(t, root, func(*window) {
		instance, _, _ := getModuleHandleW.Call(0)
		control, _, err := createWindowExW.Call(wsExNoActivate, uintptr(unsafe.Pointer(wide("STATIC"))),
			uintptr(unsafe.Pointer(wide("NSGo dock desktop availability"))), wsOverlappedWindow|wsVisible,
			20, 20, 160, 90, 0, 0, instance, 0)
		if control == 0 {
			controlError = err
			return
		}
		defer destroyWindow.Call(control)
		controlStyle, _, _ = getWindowLongPtrW.Call(control, gwlStyle)
		shown, _, _ := isWindowVisible.Call(control)
		controlVisible = shown != 0
		shown, _, _ = isWindowVisible.Call(keyboard)
		keyboardVisible = shown != 0
	})
	if controlError != nil {
		t.Skipf("native desktop cannot create an independent Win32 control: %v", controlError)
	}
	if !controlVisible {
		t.Skipf("native desktop cannot expose an independent Win32 control (style=%#x); real panel visibility and focus acceptance need an interactive Windows desktop", controlStyle)
	}
	if !keyboardVisible {
		t.Fatal("the NSGo keyboard window stayed hidden on a visible native desktop")
	}
	if len(all) == 0 {
		t.Fatal("Win32 reported no monitor work area")
	}
	// Check an explicit selection of every actual monitor and the default
	// primary selection. No test invents a display the native API lacks.
	type target struct {
		on      displayPlacement
		request string
	}
	var targets []target
	for _, on := range all {
		targets = append(targets, target{on, on.display.ID})
		if on.display.Main {
			targets = append(targets, target{on, ""})
		}
	}
	t.Logf("native monitors: %d; explicit/default selections: %d", len(all), len(targets))
	for _, on := range targets {
		for _, edge := range []core.Placement{core.PlaceTop, core.PlaceBottom, core.PlaceLeft, core.PlaceRight} {
			label := on.request
			if label == "" {
				label = "default-primary"
			}
			t.Run(fmt.Sprintf("%s/edge%d", label, edge), func(t *testing.T) {
				type panelState struct {
					panel  *core.Window
					resize func(core.Size)
				}
				created := make(chan panelState, 1)
				issues := make(chan error, 1)
				s.cx.Post(func() {
					var resize func(core.Size)
					panel := s.cx.OpenPanel(core.PanelOptions{Width: 240, Height: 160, FitContent: true, NoActivate: true,
						Dock: core.PanelDock{Edge: edge, Display: on.request, Margin: 10}}, func(cx *core.Scope) core.Node {
						size := core.NewSignal(cx, core.Size{Width: 120, Height: 60})
						resize = size.Set
						return core.View{StyleFunc: func() core.Style {
							s := size.Get()
							return core.Style{Width: core.Pt(s.Width), Height: core.Pt(s.Height), Background: core.RGB(0x406080)}
						}}
					})
					panel.OnPlacementError(s.cx, func(err error) {
						select {
						case issues <- err:
						default:
						}
					})
					created <- panelState{panel, resize}
				})
				var c panelState
				select {
				case c = <-created:
				case <-time.After(5 * time.Second):
					t.Fatal("the engine did not create the docked panel")
				}
				if c.panel == nil || c.resize == nil {
					t.Fatal("the panel or its content was not created")
				}
				defer s.cx.Post(c.panel.Close)
				var native *window
				var focused, foreground uintptr
				onMain(t, root, func(*window) {
					setFocus.Call(keyboard)
					focused, _, _ = getFocus.Call()
					foreground, _, _ = getForegroundWindow.Call()
				})
				if focused != keyboard {
					t.Fatal("the keyboard window did not accept native focus")
				}
				check := func(size core.Size, visible bool) {
					t.Helper()
					work := on.on.info.Work
					pixel := func(v core.Points) int32 { return int32(math.Round(float64(v) * on.on.scale)) }
					width, height, margin := pixel(size.Width), pixel(size.Height), pixel(10)
					x, y := work.Left+(work.Right-work.Left-width)/2, work.Top+(work.Bottom-work.Top-height)/2
					switch edge {
					case core.PlaceTop:
						y = work.Top + margin
					case core.PlaceBottom:
						y = work.Bottom - height - margin
					case core.PlaceLeft:
						x = work.Left + margin
					case core.PlaceRight:
						x = work.Right - width - margin
					}
					want := rect{x, y, x + width, y + height}
					var got rect
					var shown bool
					for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
						onMain(t, root, func(own *window) {
							if native == nil {
								for _, w := range windows {
									if w.hostWindow == own && w.kind == driver.SurfacePanel && w.dock.Display == on.request && w.dock.Edge == driver.WindowPlacement(edge) {
										native = w
									}
								}
							}
							if native != nil {
								getWindowRect.Call(native.hwnd, uintptr(unsafe.Pointer(&got)))
								v, _, _ := isWindowVisible.Call(native.hwnd)
								shown = v != 0
							}
						})
						near := func(a, b int32) bool { return a >= b-1 && a <= b+1 }
						if native != nil && shown == visible && (!visible || near(got.Left, want.Left) && near(got.Top, want.Top) && got.Right-got.Left == width && got.Bottom-got.Top == height) {
							break
						}
						select {
						case err := <-issues:
							t.Fatalf("native placement failed: %v", err)
						default:
						}
						if time.Now().After(deadline) {
							t.Fatalf("native frame%+v visible%v, want%+v visible%v; work%+v scale%v", got, shown, want, visible, work, on.on.scale)
						}
					}
					onMain(t, root, func(*window) {
						const exStyle = ^uintptr(19) // GWL_EXSTYLE.
						ex, _, _ := getWindowLongPtrW.Call(native.hwnd, exStyle)
						if ex&wsExNoActivate == 0 {
							t.Error("the docked panel lost WS_EX_NOACTIVATE")
						}
						focus, _, _ := getFocus.Call()
						front, _, _ := getForegroundWindow.Call()
						if focus != focused || front != foreground {
							t.Error("showing or resizing the docked panel changed native keyboard focus or the foreground window")
						}
					})
					t.Logf("native frame%+v visible%v, work%+v scale%v", got, shown, work, on.on.scale)
				}
				s.cx.Post(c.panel.Show)
				for _, size := range []core.Size{{Width: 120, Height: 60}, {Width: 160, Height: 80}, {Width: 80, Height: 40}} {
					s.cx.Post(func() { c.resize(size) })
					check(size, true)
				}
				onMain(t, root, func(*window) {
					result, _, _ := user32.NewProc("SendMessageW").Call(native.hwnd, wmMouseActivate, keyboard, wmLButtonDown<<16|1)
					if result != maNoActivate {
						t.Errorf("native mouse activation result = %d, want MA_NOACTIVATE", result)
					}
				})
				s.cx.Post(c.panel.Focus)
				check(core.Size{Width: 80, Height: 40}, true)
				s.cx.Post(c.panel.Hide)
				check(core.Size{Width: 80, Height: 40}, false)
				s.cx.Post(func() { c.resize(core.Size{Width: 140, Height: 70}) })
				s.cx.Post(c.panel.Show)
				check(core.Size{Width: 140, Height: 70}, true)
				s.cx.Post(c.panel.Toggle)
				check(core.Size{Width: 140, Height: 70}, false)
				s.cx.Post(c.panel.Toggle)
				check(core.Size{Width: 140, Height: 70}, true)
				s.cx.Post(c.panel.Close)
				for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
					closed := false
					onMain(t, root, func(*window) { closed = windows[native.hwnd] == nil })
					if closed {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("the docked panel did not close")
					}
				}
			})
		}
	}
}
