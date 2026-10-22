//go:build windows

package windows

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

var (
	findWindowW   = user32.NewProc("FindWindowW")
	findWindowExW = user32.NewProc("FindWindowExW")
	printWindow   = user32.NewProc("PrintWindow")
	getDIBits     = gdi32.NewProc("GetDIBits")
)

const clientOnly = 1 // PrintWindow's PW_CLIENTONLY

// nativeApp is the app a native test drives: a button and a field whose
// state the window's title shows, so the test can wait for the engine.
func nativeApp(title string) func(*core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		count := core.NewSignal(cx, 0)
		field := core.NewSignal(cx, "")
		submitted := core.NewSignal(cx, 0)
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|%d|%s|%d", title, count.Get(), field.Get(), submitted.Get()))
		})
		return core.View{
			Style: core.Style{Width: core.Pt(360), Height: core.Pt(240), Padding: core.All(10),
				Direction: core.Row, Gap: 10, Background: core.RGB(0xffffff)},
			Children: []core.Node{
				core.View{OnPress: func() { count.Update(func(n int) int { return n + 1 }) },
					Style: core.Style{Width: core.Pt(100), Height: core.Pt(80), Background: core.RGB(0x123456)}},
				core.Input{Value: field.Get, OnChange: field.Set, AutoFocus: true,
					OnSubmit: func(string) { submitted.Update(func(n int) int { return n + 1 }) },
					Style:    core.Style{Width: core.Pt(200), Height: core.Pt(30)}},
			},
		}
	}
}

// TestWindowRunsAnAppAndTakesNativeInput opens a real window, clicks its
// view, types into its EDIT control and reads the pixels it painted. It
// skips where no desktop can create a window.
func TestWindowRunsAnAppAndTakesNativeInput(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-native-" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(nativeApp(title), Options{Title: title + "|0||0", Width: 360, Height: 240})
	}()

	hwnd := waitForWindow(t, title+"|0||0", done)
	if hwnd == 0 {
		return
	}
	t.Cleanup(func() {
		postMessageW.Call(hwnd, wmClose, 0, 0)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the window did not close")
		}
	})
	waitForText(t, hwnd, title+"|0||0", done)

	// The button at (10, 10) to (110, 90) in the client area.
	click(t, hwnd, 60, 50)
	waitForText(t, hwnd, title+"|1||0", done)

	// The native EDIT control reports its changes to the engine.
	edit := waitForChild(t, hwnd, "EDIT")
	if edit == 0 {
		t.Fatal("the input has no EDIT control")
	}
	for _, r := range "hi" {
		postMessageW.Call(edit, wmChar, uintptr(r), 1)
	}
	waitForText(t, hwnd, title+"|1|hi|0", done)

	// Return submits the focused field.
	postMessageW.Call(edit, wmKeyDown, vkReturn, 1)
	waitForText(t, hwnd, title+"|1|hi|1", done)

	// What GDI drew is in the window, not only in the engine's model.
	if r, g, b, ok := windowPixel(hwnd, 60, 50); ok {
		if r != 0x12 || g != 0x34 || b != 0x56 {
			t.Fatalf("button pixel = #%02x%02x%02x, want #123456", r, g, b)
		}
	}
}

func waitForWindow(t *testing.T, title string, done <-chan struct{}) uintptr {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		hwnd, _, _ := findWindowW.Call(0, uintptr(unsafe.Pointer(wide(title))))
		if hwnd != 0 {
			return hwnd
		}
		select {
		case <-done:
			t.Skip("no desktop could create the test window")
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("no window titled %q", title)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForText(t *testing.T, hwnd uintptr, want string, done <-chan struct{}) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := editText(hwnd); got == want {
			return
		}
		select {
		case <-done:
			t.Fatalf("the window closed with title %q, want %q", editText(hwnd), want)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("window title = %q, want %q", editText(hwnd), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForChild(t *testing.T, parent uintptr, class string) uintptr {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		child, _, _ := findWindowExW.Call(parent, 0, uintptr(unsafe.Pointer(wide(class))), 0)
		if child != 0 {
			return child
		}
		if time.Now().After(deadline) {
			return 0
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func click(t *testing.T, hwnd uintptr, x, y int32) {
	t.Helper()
	lParam := uintptr(uint32(y)<<16 | uint32(x)&0xffff)
	postMessageW.Call(hwnd, wmMouseMove, 0, lParam)
	postMessageW.Call(hwnd, wmLButtonDown, 1, lParam)
	time.Sleep(50 * time.Millisecond)
	postMessageW.Call(hwnd, wmLButtonUp, 0, lParam)
}

// windowPixel reads the client pixel at x, y as the window painted it.
// It skips when Windows cannot copy the window's pixels, as on a session
// with no desktop.
func windowPixel(hwnd uintptr, x, y int32) (r, g, b uint8, ok bool) {
	var client rect
	getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	width, height := client.Right, client.Bottom
	if width <= x || height <= y {
		return 0, 0, 0, false
	}
	dc, _, _ := getDC.Call(hwnd)
	if dc == 0 {
		return 0, 0, 0, false
	}
	defer releaseDC.Call(hwnd, dc)
	mem, _, _ := createCompatibleDC.Call(dc)
	if mem == 0 {
		return 0, 0, 0, false
	}
	defer deleteDC.Call(mem)
	bmp, _, _ := createCompatibleBitmap.Call(dc, uintptr(width), uintptr(height))
	if bmp == 0 {
		return 0, 0, 0, false
	}
	defer deleteObject.Call(bmp)
	old, _, _ := selectObject.Call(mem, bmp)
	if r, _, _ := printWindow.Call(hwnd, mem, clientOnly); r == 0 {
		// A window without WM_PRINT support still has its client on
		// screen; copy it directly.
		if r, _, _ := bitBlt.Call(mem, 0, 0, uintptr(width), uintptr(height), dc, 0, 0, srcCopy); r == 0 {
			selectObject.Call(mem, old)
			return 0, 0, 0, false
		}
	}
	// GetDIBits wants the bitmap out of the DC.
	selectObject.Call(mem, old)
	pix := make([]byte, int(width)*int(height)*4)
	info := bitmapInfo{Width: width, Height: -height, Planes: 1, BitCount: 32}
	info.Size = uint32(unsafe.Sizeof(info))
	if n, _, _ := getDIBits.Call(mem, bmp, 0, uintptr(height), uintptr(unsafe.Pointer(&pix[0])),
		uintptr(unsafe.Pointer(&info)), 0); n == 0 {
		return 0, 0, 0, false
	}
	at := (int(y)*int(width) + int(x)) * 4
	return pix[at+2], pix[at+1], pix[at], true
}
