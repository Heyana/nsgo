//go:build windows && (nsgotest || nsgoautomation)

package windows

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// A surface acting as the user does, with Win32 messages posted to the window and PrintWindow captures, for the test host (testhost.go) and for nsgo dev's automation (automation.go), whose live surfaces wrap the running app's windows with no test options: test is nil.

type testSurface struct {
	w    *window
	test *testOpen
	// held are the keys KeyDown holds, by virtual key; the window's thread reads and writes it.
	held map[uintptr]bool
}

var (
	_ testhost.Surface        = (*testSurface)(nil)
	_ testhost.KeyHoldSurface = (*testSurface)(nil)
	_ testhost.WindowSurfaces = (*testSurface)(nil)
	_ testhost.CloseWindow    = (*testSurface)(nil)
)

var (
	hostSendMessage       = user32.NewProc("SendMessageW")
	hostGetKeyboardState  = user32.NewProc("GetKeyboardState")
	hostSetKeyboardState  = user32.NewProc("SetKeyboardState")
	hostGetDC             = user32.NewProc("GetDC")
	hostReleaseDC         = user32.NewProc("ReleaseDC")
	hostPrintWindow       = user32.NewProc("PrintWindow")
	hostCreateDC          = gdi32.NewProc("CreateCompatibleDC")
	hostCreateBitmap      = gdi32.NewProc("CreateCompatibleBitmap")
	hostDeleteDC          = gdi32.NewProc("DeleteDC")
	hostSelectObject      = gdi32.NewProc("SelectObject")
	hostGetDIBits         = gdi32.NewProc("GetDIBits")
	hostDeleteObject      = gdi32.NewProc("DeleteObject")
	hostGetClientRectProc = user32.NewProc("GetClientRect")
)

const (
	hostMouseLeave  = 0x02A3
	hostLButtonUp   = 0x0202
	hostRButtonUp   = 0x0205
	hostKeyUp       = 0x0101
	pwRenderContent = 1 | 2
)

// onMain runs fn on the window's thread and waits for it.
func (t *testSurface) onMain(fn func()) {
	if t.w.closed == nil {
		runOnThread(t.w.post, 10*time.Second, "panic on the window thread", fn)
		return
	}
	// A live window can close while the automation acts on it: stop waiting once it does.
	select {
	case <-t.w.closed:
		return
	default:
	}
	done := make(chan struct{})
	t.w.post(func() {
		defer close(done)
		fn()
	})
	select {
	case <-done:
	case <-t.w.closed:
	case <-time.After(10 * time.Second):
	}
}

func (t *testSurface) WindowSurface(id uint32) testhost.Surface {
	if t.test == nil {
		return testhost.Live(t.w.engine.Graph(), id)
	}
	var opened *window
	t.onMain(func() { opened = t.test.windows[id] })
	if opened == nil {
		return nil
	}
	return &testSurface{w: opened, test: t.test}
}

func (t *testSurface) RequestQuit(id uint32, forced bool) {
	t.w.engine.Dispatch(driver.Event{Kind: driver.EventQuitRequest, Count: id, Forced: forced})
}

func (t *testSurface) RequestClose() error {
	if t.w.hwnd == 0 || (t.w.primary && t.w.opts.Hidden) {
		return errors.New("this surface has no window to close")
	}
	t.onMain(func() { hostSendMessage.Call(t.w.hwnd, wmClose, 0, 0) })
	return nil
}

// lParam is at, in client pixels, as mouse messages carry it.
func (t *testSurface) lParam(at driver.Point) uintptr {
	x, y := t.w.px(at.X), t.w.px(at.Y)
	return uintptr(uint32(uint16(y))<<16 | uint32(uint16(x)))
}

func (t *testSurface) post(message, wParam uintptr, at driver.Point) {
	postMessageW.Call(t.w.hwnd, message, wParam, t.lParam(at))
}

func (t *testSurface) Press(at driver.Point, hold time.Duration, mods driver.Modifiers) error {
	var buttons uintptr
	if mods.Shift {
		buttons |= 0x0004 // MK_SHIFT
	}
	if mods.Control {
		buttons |= 0x0008 // MK_CONTROL
	}
	// Send each message on the window's thread while its keyboard state holds the modifiers, so GetKeyState and native controls see them too.
	send := func(message, buttons uintptr) {
		t.onMain(func() {
			var saved [256]byte
			hostGetKeyboardState.Call(uintptr(unsafe.Pointer(&saved)))
			state := saved
			for _, key := range []uintptr{vkControl, vkShift, vkMenu, vkLWin, vkRWin, 0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5} {
				state[key] &^= 0x80
			}
			for _, key := range []struct {
				code uintptr
				held bool
			}{{vkControl, mods.Control}, {vkShift, mods.Shift}, {vkMenu, mods.Alt}, {vkLWin, mods.Platform}} {
				if key.held {
					state[key.code] |= 0x80
				}
			}
			hostSetKeyboardState.Call(uintptr(unsafe.Pointer(&state)))
			defer hostSetKeyboardState.Call(uintptr(unsafe.Pointer(&saved)))
			hostSendMessage.Call(t.w.hwnd, message, buttons, t.lParam(at))
		})
	}
	send(wmMouseMove, buttons)
	send(wmLButtonDown, buttons|mkLButton)
	time.Sleep(max(hold+50*time.Millisecond, 50*time.Millisecond))
	send(hostLButtonUp, buttons)
	return nil
}

func (t *testSurface) RightClick(at driver.Point) error {
	t.post(wmMouseMove, 0, at)
	t.post(wmRButtonDown, mkRButton, at)
	time.Sleep(30 * time.Millisecond)
	t.post(hostRButtonUp, 0, at)
	return nil
}

// MouseDown implements testhost.MouseButtonSurface for all five mouse buttons.
func (t *testSurface) MouseDown(at driver.Point, button driver.MouseButton, mods driver.Modifiers) error {
	return t.mouseButtonStep(at, button, mods, true)
}

// MouseUp implements testhost.MouseButtonSurface for all five mouse buttons.
func (t *testSurface) MouseUp(at driver.Point, button driver.MouseButton, mods driver.Modifiers) error {
	return t.mouseButtonStep(at, button, mods, false)
}

func (t *testSurface) mouseButtonStep(at driver.Point, button driver.MouseButton, mods driver.Modifiers, down bool) error {
	var msg uintptr
	var wParam uintptr
	if mods.Shift {
		wParam |= 0x0004 // MK_SHIFT
	}
	if mods.Control {
		wParam |= 0x0008 // MK_CONTROL
	}
	switch button {
	case driver.MouseButtonLeft:
		if down {
			msg, wParam = wmLButtonDown, wParam|mkLButton
		} else {
			msg = wmLButtonUp
		}
	case driver.MouseButtonRight:
		if down {
			msg, wParam = wmRButtonDown, wParam|mkRButton
		} else {
			msg = wmRButtonUp
		}
	case driver.MouseButtonMiddle:
		if down {
			msg, wParam = wmMButtonDown, wParam|mkMButton
		} else {
			msg = wmMButtonUp
		}
	case driver.MouseButtonBack:
		if down {
			msg, wParam = wmXButtonDown, wParam|mkXButton1|(1<<16)
		} else {
			msg = wmXButtonUp
			wParam = 1 << 16
		}
	case driver.MouseButtonForward:
		if down {
			msg, wParam = wmXButtonDown, wParam|mkXButton2|(xButtonForward<<16)
		} else {
			msg = wmXButtonUp
			wParam = xButtonForward << 16
		}
	default:
		return fmt.Errorf("unsupported mouse button %s", button)
	}
	t.onMain(func() {
		var saved [256]byte
		hostGetKeyboardState.Call(uintptr(unsafe.Pointer(&saved)))
		state := saved
		for _, key := range []struct {
			code uintptr
			held bool
		}{{vkControl, mods.Control}, {vkShift, mods.Shift}, {vkMenu, mods.Alt}, {vkLWin, mods.Platform}} {
			if key.held {
				setKey(&state, key.code, true)
			}
		}
		hostSetKeyboardState.Call(uintptr(unsafe.Pointer(&state)))
		defer hostSetKeyboardState.Call(uintptr(unsafe.Pointer(&saved)))
		hostSendMessage.Call(t.w.hwnd, msg, wParam, t.lParam(at))
	})
	return nil
}

// BlurWindow implements testhost.BlurWindowSurface, releasing held keys and mouse buttons as WM_KILLFOCUS does.
func (t *testSurface) BlurWindow() error {
	t.onMain(func() {
		t.w.releaseKeys()
		t.w.releaseMouseButtons()
	})
	return nil
}

func (t *testSurface) Drag(path []driver.Point) error {
	if len(path) == 0 {
		return errors.New("an empty drag")
	}
	t.post(wmMouseMove, 0, path[0])
	t.post(wmLButtonDown, mkLButton, path[0])
	time.Sleep(100 * time.Millisecond)
	at := path[0]
	for _, to := range path[1:] {
		dx, dy := to.X-at.X, to.Y-at.Y
		n := int(max(absPoints(dx), absPoints(dy))/10) + 1
		for i := 1; i <= n; i++ {
			f := driver.Points(i) / driver.Points(n)
			t.post(wmMouseMove, mkLButton, driver.Point{X: at.X + dx*f, Y: at.Y + dy*f})
			time.Sleep(16 * time.Millisecond)
		}
		at = to
	}
	t.post(hostLButtonUp, 0, at)
	return nil
}

// Scroll delivers a precise touchpad-style scroll gesture at at by delta (the scroll-offset change in points, opposite to content movement): it begins, moves at most 20 points every 16 ms, and ends without momentum, carrying exact point distances to both native scroll views and OnWheel.
func (t *testSurface) Scroll(at driver.Point, delta driver.Point) error {
	steps := preciseScrollSteps(delta)
	var g preciseScroll
	t.onMain(func() { g = t.w.beginPreciseScroll(at) })
	time.Sleep(16 * time.Millisecond)
	for _, step := range steps {
		t.onMain(func() { t.w.stepPreciseScroll(&g, at, step) })
		time.Sleep(16 * time.Millisecond)
	}
	t.onMain(func() { t.w.endPreciseScroll(&g, at) })
	return nil
}

func (t *testSurface) Hover(at driver.Point) error {
	if at.X < 0 || at.Y < 0 {
		postMessageW.Call(t.w.hwnd, hostMouseLeave, 0, 0)
		return nil
	}
	t.post(wmMouseMove, 0, at)
	return nil
}

func (t *testSurface) Type(text string) error {
	for _, r := range text {
		if r == '\n' {
			if err := t.Key("enter"); err != nil {
				return err
			}
			continue
		}
		for _, u := range utf16.Encode([]rune{r}) {
			postMessageW.Call(t.w.hwnd, wmChar, uintptr(u), 1)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

var (
	immSetCompositionStringW = imm32.NewProc("ImmSetCompositionStringW")
	immSetOpenStatus         = imm32.NewProc("ImmSetOpenStatus")
	immGetOpenStatus         = imm32.NewProc("ImmGetOpenStatus")
)

const scsSetStr = 0x0009 // SCS_SETSTR: GCS_COMPREAD | GCS_COMPSTR

// Compose has the window's IME compose text, through IMM as the IME's own composition goes: the IME answers with WM_IME_COMPOSITION. A machine whose input language has no IME composes nothing.
func (t *testSurface) Compose(text string) error {
	var err error
	t.onMain(func() {
		if t.w.focusedField() == nil {
			err = errors.New("no field has the keyboard")
			return
		}
		t.w.withContext(func(himc uintptr) {
			if text == "" {
				immNotifyIME.Call(himc, niCompositionStr, cpsCancel, 0)
				return
			}
			if open, _, _ := immGetOpenStatus.Call(himc); open == 0 {
				immSetOpenStatus.Call(himc, 1)
			}
			u := utf16.Encode([]rune(text))
			if ok, _, _ := immSetCompositionStringW.Call(himc, scsSetStr, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)*2), 0, 0); ok == 0 {
				err = testhost.ErrNoInputMethod
			}
		})
	})
	time.Sleep(30 * time.Millisecond)
	return err
}

// Commit composes text and has the IME complete it, which commits it.
func (t *testSurface) Commit(text string) error {
	if err := t.Compose(text); err != nil {
		return err
	}
	t.onMain(func() {
		t.w.withContext(func(himc uintptr) { immNotifyIME.Call(himc, niCompositionStr, cpsComplete, 0) })
	})
	time.Sleep(30 * time.Millisecond)
	return nil
}

// virtualKeys are the virtual-key codes of the keys tests name.
var virtualKeys = map[string]uintptr{
	"enter": vkReturn, "return": vkReturn, "escape": 0x1B, "backspace": 0x08, "tab": 0x09, "space": 0x20,
	"left": 0x25, "up": 0x26, "right": 0x27, "down": 0x28, "home": 0x24, "end": 0x23, "pageup": 0x21,
	"pagedown": 0x22, "delete": 0x2E,
	"shift": vkShift, "ctrl": vkControl, "control": vkControl, "mod": vkControl, "alt": vkMenu, "option": vkMenu,
	"meta": vkLWin, "super": vkLWin, "win": vkLWin,
}

// keyStroke is a key a test names: its virtual key and the modifiers its chord holds for it.
type keyStroke struct {
	vk                     uintptr
	ctrl, shift, alt, meta bool
}

// parseKey reads a key a test names, a chord's modifiers before it ("mod+shift+k").
func parseKey(key string) (keyStroke, error) {
	var k keyStroke
	parts := strings.Split(strings.ToLower(key), "+")
	name := parts[len(parts)-1]
	for _, m := range parts[:len(parts)-1] {
		switch m {
		case "mod", "ctrl", "control":
			k.ctrl = true
		case "shift":
			k.shift = true
		case "alt", "option":
			k.alt = true
		case "meta", "super", "win":
			k.meta = true
		default:
			return k, fmt.Errorf("unknown modifier %q in %q", m, key)
		}
	}
	vk, ok := virtualKeys[name]
	if !ok && len(name) == 1 {
		c := strings.ToUpper(name)[0]
		if c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			vk, ok = uintptr(c), true
		}
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(name, "f")); !ok && err == nil && name[0] == 'f' && n >= 1 && n <= 24 {
		vk, ok = uintptr(0x6F+n), true
	}
	if !ok {
		return k, fmt.Errorf("unknown key %q", key)
	}
	k.vk = vk
	return k, nil
}

// sidedKeys are the left-hand virtual keys GetKeyState also reads for a modifier, which a held modifier holds too.
var sidedKeys = map[uintptr]uintptr{vkShift: vkLShift, vkControl: 0xA2, vkMenu: 0xA4}

// setKey holds vk in a keyboard state, or lets it up.
func setKey(state *[256]byte, vk uintptr, down bool) {
	for _, code := range []uintptr{vk, sidedKeys[vk]} {
		if code == 0 {
			continue
		}
		if down {
			state[code] |= 0x80
		} else {
			state[code] &^= 0x80
		}
	}
}

// Key presses key with the modifiers held in the thread's keyboard state, in the order the message loop and the window procedure take a real key: the focused view's OnKeyDown, then a keymap binding or menu shortcut, then the focused field's editing, and else the window's own key handling. The key then comes up (WM_KEYUP) whoever took it, with the chord's modifiers still held, so a view that heard it go down hears it come up.
func (t *testSurface) Key(key string) error { return t.stroke(key, true, true) }

// KeyDown presses key and holds it, a modifier through the keys after it, until its KeyUp; a key KeyDown already holds goes down again as an auto-repeat.
func (t *testSurface) KeyDown(key string) error { return t.stroke(key, true, false) }

// KeyUp lets a key KeyDown holds come up.
func (t *testSurface) KeyUp(key string) error { return t.stroke(key, false, true) }

// stroke sends key going down, coming up, or both, keeping the keys KeyDown holds in the thread's keyboard state between calls, as GetKeyState reads a real keyboard's. A chord's modifiers are held for its key alone.
func (t *testSurface) stroke(key string, down, up bool) error {
	k, err := parseKey(key)
	if err != nil {
		return err
	}
	t.onMain(func() {
		if !down && !t.held[k.vk] {
			err = fmt.Errorf("key %q is not held", key)
			return
		}
		if t.held == nil {
			t.held = map[uintptr]bool{}
		}
		var base [256]byte
		hostGetKeyboardState.Call(uintptr(unsafe.Pointer(&base)))
		state := base
		setKey(&state, vkControl, k.ctrl || t.held[vkControl])
		setKey(&state, vkShift, k.shift || t.held[vkShift])
		setKey(&state, vkMenu, k.alt || t.held[vkMenu])
		setKey(&state, vkLWin, k.meta || t.held[vkLWin])
		if down {
			// A modifier pressed alone is down as its own key goes down.
			setKey(&state, k.vk, true)
			hostSetKeyboardState.Call(uintptr(unsafe.Pointer(&state)))
			lParam := uintptr(1)
			if t.held[k.vk] {
				lParam |= 1 << 30
			}
			t.held[k.vk] = true
			w := t.w
			if !w.keyDown(k.vk, lParam) && !w.shortcut(k.vk, lParam) && (w.focusedField() == nil || !w.editKey(k.vk, k.ctrl || t.held[vkControl], k.shift || t.held[vkShift], k.alt || t.held[vkMenu])) {
				hostSendMessage.Call(w.hwnd, wmKeyDown, k.vk, lParam)
			}
		}
		if up {
			setKey(&state, k.vk, false)
			hostSetKeyboardState.Call(uintptr(unsafe.Pointer(&state)))
			delete(t.held, k.vk)
			hostSendMessage.Call(t.w.hwnd, hostKeyUp, k.vk, 1<<31|1<<30|1)
		}
		// The chord's modifiers come up with its key; the keys KeyDown holds stay down.
		for _, vk := range []uintptr{vkControl, vkShift, vkMenu, vkLWin} {
			setKey(&base, vk, t.held[vk])
		}
		setKey(&base, k.vk, t.held[k.vk])
		hostSetKeyboardState.Call(uintptr(unsafe.Pointer(&base)))
	})
	return err
}

// Back is Escape, twice in a field, whose first only leaves it.
func (t *testSurface) Back() error {
	var editing bool
	t.onMain(func() { editing = t.w.focusedField() != nil })
	if editing {
		t.Key("escape")
	}
	return t.Key("escape")
}

// Answer clicks the button at index of the alert the window draws, or presses Escape for -1, as a user would.
func (t *testSurface) Answer(id uint32, index int) error {
	var err error
	deadline := time.Now().Add(3 * time.Second)
	for {
		drawn := false
		t.onMain(func() {
			w := t.w
			if !w.alerting || w.alertShown.ID != id {
				// The engine's batch may have queued the alert while wmAlert is still in the message queue.
				if slices.ContainsFunc(w.alerts, func(a driver.AlertConfig) bool { return a.ID == id }) {
					return
				}
				err = fmt.Errorf("no alert %d shows", id)
				return
			}
			s := w.alert
			if s == nil || len(s.buttons) == 0 {
				return // not drawn yet
			}
			drawn = true
			if index < 0 {
				if !s.cancelable {
					err = errors.New("the alert cannot be dismissed")
					return
				}
				postMessageW.Call(w.hwnd, wmKeyDown, vkEscape, 1)
				return
			}
			if index >= len(s.buttons) {
				err = fmt.Errorf("alert %d has no button %d", id, index)
				return
			}
			r := s.buttons[index]
			x := int32(float64((r.left+r.right)/2) * w.scale)
			y := int32(float64((r.top+r.bottom)/2) * w.scale)
			at := uintptr(uint32(y)<<16 | uint32(x)&0xffff)
			postMessageW.Call(w.hwnd, wmLButtonDown, mkLButton, at)
			postMessageW.Call(w.hwnd, wmLButtonUp, 0, at)
		})
		if err != nil || drawn {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the alert %d is not drawn", id)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Screenshot copies the window's client area as it composed it.
func (t *testSurface) Screenshot() ([]byte, error) {
	var img *image.RGBA
	t.onMain(func() {
		hwnd := t.w.hwnd
		var client rect
		hostGetClientRectProc.Call(hwnd, uintptr(unsafe.Pointer(&client)))
		width, height := client.Right, client.Bottom
		if width <= 0 || height <= 0 {
			return
		}
		dc, _, _ := hostGetDC.Call(hwnd)
		defer hostReleaseDC.Call(hwnd, dc)
		mem, _, _ := hostCreateDC.Call(dc)
		defer hostDeleteDC.Call(mem)
		bmp, _, _ := hostCreateBitmap.Call(dc, uintptr(width), uintptr(height))
		defer hostDeleteObject.Call(bmp)
		old, _, _ := hostSelectObject.Call(mem, bmp)
		hostPrintWindow.Call(hwnd, mem, pwRenderContent)
		hostSelectObject.Call(mem, old)
		pix := make([]byte, int(width)*int(height)*4)
		type bitmapHeader struct {
			Size                         uint32
			Width, Height                int32
			Planes, BitCount             uint16
			Compression, SizeImage       uint32
			XPelsPerMeter, YPelsPerMeter int32
			ClrUsed, ClrImportant        uint32
			Colors                       [1]uint32
		}
		info := bitmapHeader{Width: width, Height: -height, Planes: 1, BitCount: 32}
		info.Size = uint32(unsafe.Sizeof(info)) - 4
		if n, _, _ := hostGetDIBits.Call(mem, bmp, 0, uintptr(height), uintptr(unsafe.Pointer(&pix[0])),
			uintptr(unsafe.Pointer(&info)), 0); n == 0 {
			return
		}
		img = image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
		for i := 0; i < len(pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = pix[i+2], pix[i+1], pix[i], 0xff
		}
	})
	if img == nil {
		return nil, errors.New("Windows could not copy the window's pixels")
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (t *testSurface) PointerDelta(dx, dy float64) {
	t.onMain(func() { t.w.pointerMotion(dx, dy) })
}

// Sync returns once the window's engine has handed its commits to the platform.
func (t *testSurface) Sync() {
	if t.w.engine != nil {
		t.w.engine.Sync()
	}
}

// Animating counts transitions, layout animations and animated values that end.
func (t *testSurface) Animating() bool {
	var busy bool
	t.onMain(func() {
		busy = t.w.batches.Len() > 0 || t.w.animator.Ending()
		for _, v := range t.w.views {
			busy = busy || v != nil && (v.transition != nil || v.moving != nil || v.fading != nil)
		}
	})
	return busy
}

func (t *testSurface) Resize(size driver.Size) error {
	t.onMain(func() {
		const noMove, noZOrder = 0x0002, 0x0004
		frame := rect{Right: t.w.px(size.Width), Bottom: t.w.px(size.Height)}
		adjustWindowRect.Call(uintptr(unsafe.Pointer(&frame)), wsOverlappedWindow|wsClipChildren, 0)
		setWindowPos.Call(t.w.hwnd, 0, 0, 0, uintptr(frame.Right-frame.Left), uintptr(frame.Bottom-frame.Top), noMove|noZOrder)
	})
	return nil
}

func (t *testSurface) SetEnvironment(env driver.Environment) error {
	if t.test == nil {
		return errors.New("a running app's window follows the system's settings")
	}
	t.test.pin.Set(&env)
	t.w.engine.Repin()
	return nil
}

// ChooseMenu implements testhost.Surface: the menu bar's command, as WM_COMMAND brings it.
func (t *testSurface) ChooseMenu(id uint32) error {
	var err error
	t.onMain(func() {
		for i, r := range t.w.chrome.entries {
			if r.id == id {
				postMessageW.Call(t.w.hwnd, wmCommand, uintptr(i+1), 0)
				return
			}
		}
		err = fmt.Errorf("the menu bar has no item %d", id)
	})
	return err
}

// ChooseDockMenu implements testhost.Surface: the running app hears the launch item id's Jump List task makes, as choosing the task does; an item that is no task, as one in a submenu, is not in the list.
func (t *testSurface) ChooseDockMenu(id uint32) error {
	var err error
	t.onMain(func() {
		for _, e := range jumpTasks {
			if e.ID == id && jumpTask(e) {
				t.w.launched(launch{Task: e.Label})
				return
			}
		}
		err = fmt.Errorf("the Jump List has no task %d", id)
	})
	return err
}

// ChoosePopup implements testhost.Surface: the context menu tracking ends, and its item id is carried out after, as a click on it would. For 0 the menu tracking and those queued to show are cancelled, and it returns once the tracking has ended, so the next input reaches a window; with none showing it does nothing.
func (t *testSurface) ChoosePopup(id uint32) error {
	if id == 0 {
		tracking := true
		t.onMain(func() {
			w := t.w
			// Queued menus would track once the one tracking returns; they close unshown, as one replaced before it showed does.
			for _, p := range w.popups {
				w.engine.Dispatch(driver.Event{Kind: driver.EventMenuClosed, Count: p.id})
			}
			w.popups = nil
			if w.popupTracking {
				w.popupChoice = &id
				endMenu.Call()
			}
			tracking = w.popupTracking
		})
		for deadline := time.Now().Add(3 * time.Second); tracking && time.Now().Before(deadline); {
			time.Sleep(10 * time.Millisecond)
			t.onMain(func() { tracking = t.w.popupTracking })
		}
		if tracking {
			return errors.New("the context menu kept tracking after it was cancelled")
		}
		return nil
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		tracking := false
		t.onMain(func() {
			if tracking = t.w.popupTracking; tracking {
				t.w.popupChoice = &id
				endMenu.Call()
			}
		})
		if tracking {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no context menu shows")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
