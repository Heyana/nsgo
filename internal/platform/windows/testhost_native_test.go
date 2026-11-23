//go:build windows && nsgotest

package windows

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// TestTestHostChoosesNativeMenus chooses a menu bar item and an item of a
// context menu tracking, as nsgo test -target does.
func TestTestHostChoosesNativeMenus(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	var root *core.Scope
	chose := make(chan string, 4)
	app := func(cx *core.Scope) core.Node {
		root = cx
		cx.SetApplicationMenu([]core.MenuItem{{Label: "File", Items: []core.MenuItem{
			{Label: "New", OnSelect: func() { chose <- "New" }}}}})
		return core.View{Style: core.Style{FlexGrow: 1}}
	}
	hwnd := startApp(t, "nsgo-menus", app, 300, 200)
	var ts *testSurface
	var newID uint32
	for deadline := time.Now().Add(5 * time.Second); newID == 0 && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		onMain(t, hwnd, func(w *window) {
			ts = &testSurface{w: w}
			if len(w.chrome.entries) > 0 {
				newID = w.chrome.entries[len(w.chrome.entries)-1].id
			}
		})
	}
	if newID == 0 {
		t.Fatal("no menu bar")
	}
	if err := ts.ChooseMenu(newID); err != nil {
		t.Fatal(err)
	}
	if got := <-chose; got != "New" {
		t.Errorf("chose %q", got)
	}
	root.Post(func() {
		root.PopupMenu([]core.MenuItem{{Label: "Copy", OnSelect: func() { chose <- "Copy" }}}, core.Point{X: 20, Y: 20}, nil)
	})
	var copyID uint32
	for deadline := time.Now().Add(5 * time.Second); copyID == 0 && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		onMain(t, hwnd, func(w *window) {
			if w.popupTracking && len(w.popupShown) > 0 {
				copyID = w.popupShown[0].ID
			}
		})
	}
	if copyID == 0 {
		t.Fatal("no context menu tracks")
	}
	if err := ts.ChoosePopup(copyID); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-chose:
		if got != "Copy" {
			t.Errorf("chose %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Error("the context menu's item was not chosen")
	}
}

// TestTestHostCommitsCompositionBeforeBlur moves the focus from a field while an input method composes in it, by a click on another field, from the app and by disabling the field, and checks that the field reports the committed text, then the composition's end, then its blur; a click in the composing field itself and making it read-only commit without a blur. A machine without an input method skips it.
func TestTestHostCommitsCompositionBeforeBlur(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	for by, want := range map[string]string{"click": "focus,change,end,blur", "app": "focus,change,end,blur", "disable": "focus,change,end,blur", "same": "focus,change,end", "readonly": "focus,change,end"} {
		t.Run(by, func(t *testing.T) {
			title := "nsgo-ime-blur-" + by
			var root *core.Scope
			second := &core.Ref{}
			var disabled, readonly *core.Signal[bool]
			app := func(cx *core.Scope) core.Node {
				root = cx
				value, events := core.NewSignal(cx, ""), core.NewSignal(cx, "")
				disabled, readonly = core.NewSignal(cx, false), core.NewSignal(cx, false)
				add := func(event string) {
					events.Update(func(old string) string {
						if old == "" {
							return event
						}
						return old + "," + event
					})
				}
				cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%s|%s", title, value.Get(), events.Get())) })
				field := core.Style{Width: driver.Pt(300), Height: driver.Pt(30)}
				return core.View{Style: core.Style{Width: driver.Pt(320), Height: driver.Pt(120), Padding: driver.All(10), Gap: 10}, Children: []core.Node{
					core.Input{Value: value.Get, AutoFocus: true, Disabled: disabled.Get, ReadonlyFunc: readonly.Get, Style: field,
						OnChange: func(v string) { value.Set(v); add("change") },
						OnCompositionChange: func(c core.Composition) {
							if c.Composing {
								add("composition")
							} else {
								add("end")
							}
						},
						OnFocus: func() { add("focus") },
						OnBlur:  func() { add("blur") }},
					core.Input{Ref: second, Style: field},
				}}
			}
			hwnd := startApp(t, title, app, 320, 120)
			waitForTitle(t, hwnd, title+"||focus", 5*time.Second)
			var ts *testSurface
			onMain(t, hwnd, func(w *window) { ts = &testSurface{w: w} })
			if err := ts.Compose("ni"); err != nil {
				if errors.Is(err, testhost.ErrNoInputMethod) {
					t.Skip(err)
				}
				t.Fatal(err)
			}
			waitForTitle(t, hwnd, title+"||focus,composition", 5*time.Second)
			switch by {
			case "click":
				if err := ts.Press(driver.Point{X: 20, Y: 65}, 0, driver.Modifiers{}); err != nil {
					t.Fatal(err)
				}
			case "same":
				if err := ts.Press(driver.Point{X: 20, Y: 25}, 0, driver.Modifiers{}); err != nil {
					t.Fatal(err)
				}
			case "app":
				root.Post(second.Focus)
			case "disable":
				root.Post(func() { disabled.Set(true) })
			case "readonly":
				root.Post(func() { readonly.Set(true) })
			}
			order := func() (string, bool) {
				parts := strings.SplitN(windowText(hwnd), "|", 3)
				if len(parts) != 3 {
					return "", false
				}
				var events []string
				for _, ev := range strings.Split(parts[2], ",") {
					if ev != "composition" {
						events = append(events, ev)
					}
				}
				return strings.Join(events, ","), parts[1] != ""
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				if got, committed := order(); committed && got == want {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("window title = %q, want the committed text and %s", windowText(hwnd), want)
				}
				time.Sleep(20 * time.Millisecond)
			}
			// Nothing follows, such as a late blur.
			time.Sleep(200 * time.Millisecond)
			if got, _ := order(); got != want {
				t.Fatalf("events %s after %s", got, want)
			}
		})
	}
}

// TestTestHostPreciseScroll verifies that testSurface.Scroll delivers exact point distances and touchpad phases to OnWheel as well as exact point distances to both vertical and horizontal native scroll views.
func TestTestHostPreciseScroll(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	var mu sync.Mutex
	var wheelEvents []core.WheelEvent
	var vOffset, hOffset core.Point
	app := func(*core.Scope) core.Node {
		return core.View{
			Style: core.Style{Width: driver.Pt(300), Height: driver.Pt(340), Gap: 10},
			Children: []core.Node{
				core.View{
					Style: core.Style{Width: driver.Pt(280), Height: driver.Pt(100)},
					OnWheel: func(e core.WheelEvent) {
						mu.Lock()
						wheelEvents = append(wheelEvents, e)
						mu.Unlock()
					},
				},
				core.Scroll{
					Style: core.Style{Width: driver.Pt(280), Height: driver.Pt(100)},
					OnScroll: func(e core.ScrollEvent) {
						mu.Lock()
						vOffset = e.Offset
						mu.Unlock()
					},
					Children: []core.Node{core.View{Style: core.Style{Height: driver.Pt(1000)}}},
				},
				core.Scroll{
					Horizontal: true,
					Style:      core.Style{Width: driver.Pt(280), Height: driver.Pt(100)},
					OnScroll: func(e core.ScrollEvent) {
						mu.Lock()
						hOffset = e.Offset
						mu.Unlock()
					},
					Children: []core.Node{core.View{Style: core.Style{Width: driver.Pt(1000), Height: driver.Pt(100)}}},
				},
			},
		}
	}
	hwnd := startApp(t, "nsgo-precise-scroll", app, 300, 340)
	var ts *testSurface
	onMain(t, hwnd, func(w *window) { ts = &testSurface{w: w} })
	ts.Sync()

	// TouchpadScroll passes negated content deltas as the offset delta.
	if err := ts.Scroll(driver.Point{X: 100, Y: 50}, driver.Point{X: -40, Y: -60}); err != nil {
		t.Fatal(err)
	}
	ts.Sync()

	mu.Lock()
	gotWheel := append([]core.WheelEvent(nil), wheelEvents...)
	mu.Unlock()
	if len(gotWheel) < 3 {
		t.Fatalf("got %d wheel events, want GestureBegan, GestureChanged..., GestureEnded: %+v", len(gotWheel), gotWheel)
	}
	if gotWheel[0].Phase != core.GestureBegan || !gotWheel[0].Touchpad || gotWheel[0].Delta != (core.Point{}) {
		t.Fatalf("first wheel event = %+v, want GestureBegan Touchpad:true zero delta", gotWheel[0])
	}
	last := gotWheel[len(gotWheel)-1]
	if last.Phase != core.GestureEnded || !last.Touchpad || last.Delta != (core.Point{}) {
		t.Fatalf("last wheel event = %+v, want GestureEnded Touchpad:true zero delta", last)
	}
	var moved core.Point
	for i, ev := range gotWheel[1 : len(gotWheel)-1] {
		if ev.Phase != core.GestureChanged || !ev.Touchpad {
			t.Fatalf("middle wheel event[%d] = %+v, want GestureChanged Touchpad:true", i, ev)
		}
		moved.X += ev.Delta.X
		moved.Y += ev.Delta.Y
	}
	if math.Abs(float64(moved.X-40)) > 0.5 || math.Abs(float64(moved.Y-60)) > 0.5 {
		t.Fatalf("OnWheel moved %v, want {40 60}", moved)
	}

	if err := ts.Scroll(driver.Point{X: 100, Y: 160}, driver.Point{Y: 80}); err != nil {
		t.Fatal(err)
	}
	ts.Sync()
	mu.Lock()
	gotV := vOffset
	mu.Unlock()
	if math.Abs(float64(gotV.Y-80)) > 0.5 {
		t.Fatalf("vertical scroll offset = %v, want Y 80", gotV)
	}

	if err := ts.Scroll(driver.Point{X: 100, Y: 270}, driver.Point{X: 75}); err != nil {
		t.Fatal(err)
	}
	ts.Sync()
	mu.Lock()
	gotH := hOffset
	mu.Unlock()
	if math.Abs(float64(gotH.X-75)) > 0.5 {
		t.Fatalf("horizontal scroll offset = %v, want X 75", gotH)
	}
}

// TestTestHostKeyPositionsHoldTheirOwnSide holds Shift by its right-hand position and by name, with keys pressed between, and checks that GetKeyState reads each side of Shift down only while a key holding that side does: the right-hand position never holds the left-hand Shift, and Shift's name never the right-hand one. It then activates the French (AZERTY) layout on the window's thread, where the key at QWERTY's W position goes down as the layout's Z (#1242).
func TestTestHostKeyPositionsHoldTheirOwnSide(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	hwnd := startApp(t, "nsgo-key-positions", func(*core.Scope) core.Node { return core.View{Style: core.Style{FlexGrow: 1}} }, 200, 100)
	var ts *testSurface
	onMain(t, hwnd, func(w *window) { ts = &testSurface{w: w} })
	step := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	shifts := func(when string, left, right bool) {
		t.Helper()
		var l, r, either bool
		onMain(t, hwnd, func(*window) { l, r, either = keyDown(vkLShift), keyDown(vkRShift), keyDown(vkShift) })
		if l != left || r != right || either != (left || right) {
			t.Errorf("%s: GetKeyState reads the left, right and either Shift %v, %v, %v, want %v, %v, %v", when, l, r, either, left, right, left || right)
		}
	}
	step(ts.KeyCodeDown("ShiftRight", "shift"))
	shifts("ShiftRight held", false, true)
	step(ts.Key("a"))
	shifts("a pressed with ShiftRight held", false, true)
	step(ts.KeyCodeUp("ShiftRight", "shift"))
	shifts("ShiftRight up", false, false)
	step(ts.KeyDown("shift"))
	step(ts.KeyCodeDown("ShiftRight", "shift"))
	shifts("shift and ShiftRight held", true, true)
	step(ts.KeyUp("shift"))
	shifts("shift up, ShiftRight held", false, true)
	step(ts.Key("a"))
	shifts("a pressed with ShiftRight held", false, true)
	step(ts.KeyCodeUp("ShiftRight", "shift"))
	shifts("both up", false, false)

	n, _, _ := getKeyboardLayoutList.Call(0, 0)
	enabled := make([]uintptr, n)
	if n > 0 {
		n, _, _ = getKeyboardLayoutList.Call(n, uintptr(unsafe.Pointer(&enabled[0])))
		enabled = enabled[:n]
	}
	klid, err := syscall.UTF16PtrFromString("0000040C")
	if err != nil {
		t.Fatal(err)
	}
	const klfNoTellShell = 0x80
	french, _, _ := loadKeyboardLayoutW.Call(uintptr(unsafe.Pointer(klid)), klfNoTellShell)
	if french == 0 {
		t.Skip("Windows cannot load the French keyboard layout")
	}
	if !slices.Contains(enabled, french) {
		t.Cleanup(func() { unloadKeyboardLayout.Call(french) })
	}
	var previous uintptr
	onMain(t, hwnd, func(*window) { previous, _, _ = activateKeyboardLayout.Call(french, 0) })
	t.Cleanup(func() {
		onMain(t, hwnd, func(*window) { activateKeyboardLayout.Call(previous, 0) })
	})
	step(ts.KeyCodeDown("KeyW", "w"))
	var z, w bool
	onMain(t, hwnd, func(*window) { z, w = keyDown('Z'), keyDown('W') })
	step(ts.KeyCodeUp("KeyW", "w"))
	if !z || w {
		t.Errorf("on AZERTY, KeyW holds Z %v and W %v, want Z alone", z, w)
	}
}
