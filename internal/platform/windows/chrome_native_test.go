//go:build windows

package windows

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestHiddenTitleBarNoSystemCaptionButtonsOnMaximize checks that a hidden
// title bar suppresses WM_NCPAINT, WM_NCUAHDRAWCAPTION and WM_NCUAHDRAWFRAME
// so Windows/DWM never paints the system caption buttons on maximize or title
// updates (which would flash during complex layouts and linger on Mica).
func TestHiddenTitleBarNoSystemCaptionButtonsOnMaximize(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-native-no-syscaption"
	app := func(c *core.Scope) core.Node {
		c.SetTitleBar(core.TitleBarHidden, 48)
		return core.View{Style: core.Style{FlexGrow: 1}}
	}
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: title, Width: 500, Height: 340, Material: driver.WindowMaterialMica})
	}()
	hwnd := waitForWindow(t, title, done)
	if hwnd == 0 {
		return
	}
	defer func() {
		postMessageW.Call(hwnd, wmClose, 0, 0)
		<-done
	}()
	for deadline := time.Now().Add(3 * time.Second); ; {
		ready := false
		onMain(t, hwnd, func(w *window) { ready = w.hidden() })
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("title bar did not hide")
		}
		time.Sleep(20 * time.Millisecond)
	}

	onMain(t, hwnd, func(w *window) {
		for _, msg := range []struct {
			name string
			id   uint32
		}{
			{"WM_NCPAINT", wmNCPaint},
			{"WM_NCUAHDRAWCAPTION", wmNCUAHDrawCaption},
			{"WM_NCUAHDRAWFRAME", wmNCUAHDrawFrame},
		} {
			if r, _, _ := sendMessage.Call(w.hwnd, uintptr(msg.id), 1, 0); r != 0 {
				t.Errorf("%s returned %d, want 0 while title bar is hidden", msg.name, r)
			}
		}
		setForegroundWindow.Call(w.hwnd)
		w.toggleMaximize()
	})
	for deadline := time.Now().Add(3 * time.Second); ; {
		zoomed := false
		onMain(t, hwnd, func(w *window) { zoomed = w.zoomed() })
		if zoomed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("window did not maximize")
		}
		time.Sleep(20 * time.Millisecond)
	}
	onMain(t, hwnd, func(w *window) {
		setWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(wide(title+"-updated"))))
		sendMessage.Call(w.hwnd, wmNCActivate, 0, 0)
		sendMessage.Call(w.hwnd, wmNCActivate, 1, 0)
		w.frame()
	})
	time.Sleep(150 * time.Millisecond)

	// On an active desktop session with Mica, check the top strip above NSGo's
	// custom caption buttons (where Windows 11 draws its 46px-wide system
	// caption glyphs when WM_NCPAINT reaches DefWindowProc).
	var mcr rect
	var morg point
	var scale float64
	var hasMica bool
	onMain(t, hwnd, func(w *window) {
		getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&mcr)))
		clientToScreen.Call(w.hwnd, uintptr(unsafe.Pointer(&morg)))
		scale = w.scale
		hasMica = w.material != driver.WindowMaterialNone
	})
	if !hasMica {
		return
	}
	// Sample screen pixels via BitBlt in the strip where native DWM glyphs sit:
	// with captionHeight=48 and captionButtonSize=32, NSGo's custom buttons sit
	// at y=[8..40] and their 10pt glyphs at y=[19..29]; native DWM glyphs sit
	// at y=[10..20] around x=right-115 (minimize) and x=right-69 (restore).
	// Specifically, x=[right-128..right-105] is left of NSGo's custom minimize
	// button (which starts at right-112, glyph at right-101..right-91), so any
	// dark pixels in x=[right-128..right-105], y=[6..20] are native DWM glyphs.
	px := func(pt float64) int32 { return int32(pt * scale) }
	x0 := morg.X + mcr.Right - px(128)
	y0 := morg.Y + px(6)
	rw, rh := px(22), px(14)
	if rw <= 0 || rh <= 0 {
		return
	}
	screenDC, _, _ := captureGetDC.Call(0)
	if screenDC == 0 {
		return
	}
	defer captureReleaseDC.Call(0, screenDC)
	memDC, _, _ := captureCreateDC.Call(screenDC)
	defer captureDeleteDC.Call(memDC)
	bmp, _, _ := captureCreateBitmap.Call(screenDC, uintptr(rw), uintptr(rh))
	defer captureDeleteObject.Call(bmp)
	old, _, _ := captureSelect.Call(memDC, bmp)
	bitBlt := gdi32.NewProc("BitBlt")
	bitBlt.Call(memDC, 0, 0, uintptr(rw), uintptr(rh), screenDC, uintptr(x0), uintptr(y0), 0x00CC0020|0x40000000)
	captureSelect.Call(memDC, old)
	type bmiHeader struct {
		Size, Width, Height    int32
		Planes, BitCount       uint16
		Compression, SizeImage uint32
		XPels, YPels           int32
		ClrUsed, ClrImp        uint32
	}
	hdr := bmiHeader{Size: 40, Width: rw, Height: -rh, Planes: 1, BitCount: 32}
	pix := make([]byte, int(rw)*int(rh)*4)
	captureDIBits.Call(memDC, bmp, 0, uintptr(rh), uintptr(unsafe.Pointer(&pix[0])), uintptr(unsafe.Pointer(&hdr)), 0)
	// Ignore disconnected sessions where BitBlt returns all black.
	var maxLuma, minLuma int = 0, 255
	for i := 0; i < len(pix); i += 4 {
		luma := (int(pix[i+2])*299 + int(pix[i+1])*587 + int(pix[i])*114) / 1000
		if luma > maxLuma {
			maxLuma = luma
		}
		if luma < minLuma {
			minLuma = luma
		}
	}
	if maxLuma < 16 {
		t.Log("screen capture is blank: skipping maximized pixel check")
		return
	}
	// A native DWM minimize glyph has high contrast (>80 luma delta) against the
	// uniform Mica background in this empty region.
	if maxLuma-minLuma > 40 {
		t.Errorf("native system caption button pixels detected above custom buttons on maximize (luma range %d..%d)", minLuma, maxLuma)
	}
}

// TestHiddenTitleBarCaptionHitTestAndClientButtons checks that WM_NCHITTEST
// keeps HTMAXBUTTON for Windows 11 Snap Layouts while answering HTCLIENT for
// minimize and close (preceding WindowDrag), and that client mouse messages
// drive hover, cancel-on-drag-off and minimize without leaking to app views.
func TestHiddenTitleBarCaptionHitTestAndClientButtons(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-native-caption-hit"
	var viewPressed bool
	app := func(c *core.Scope) core.Node {
		c.SetTitleBar(core.TitleBarHidden, 48)
		return core.View{
			Style: core.Style{FlexGrow: 1, Background: core.RGB(0xf5f5f5)},
			Desktop: core.DesktopViewProps{
				WindowDrag: core.WindowDragOn,
			},
			OnPress: func(core.PressEvent) { viewPressed = true },
		}
	}
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: title, Width: 500, Height: 340})
	}()
	hwnd := waitForWindow(t, title, done)
	if hwnd == 0 {
		return
	}
	defer func() {
		postMessageW.Call(hwnd, wmClose, 0, 0)
		<-done
	}()
	for deadline := time.Now().Add(3 * time.Second); ; {
		ready := false
		onMain(t, hwnd, func(w *window) { ready = w.buttons() })
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("title bar buttons not active")
		}
		time.Sleep(20 * time.Millisecond)
	}

	pack := func(x, y int32) uintptr {
		return uintptr(uint32(uint16(int16(y)))<<16 | uint32(uint16(int16(x))))
	}
	onMain(t, hwnd, func(w *window) {
		var cr rect
		var orig point
		getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&cr)))
		clientToScreen.Call(w.hwnd, uintptr(unsafe.Pointer(&orig)))
		cy := w.px(24)
		closeX := cr.Right - w.px(captionMargin+captionButtonSize/2)
		maxX := cr.Right - w.px(captionMargin+captionButtonSize+captionButtonGap+captionButtonSize/2)
		minX := cr.Right - w.px(captionMargin+2*(captionButtonSize+captionButtonGap)+captionButtonSize/2)
		dragX := w.px(40)

		if got := w.ncHitTest(pack(orig.X+maxX, orig.Y+cy)); got != htMaxButton {
			t.Errorf("ncHitTest(max) = %d, want %d (HTMAXBUTTON)", got, htMaxButton)
		}
		if got := w.ncHitTest(pack(orig.X+closeX, orig.Y+cy)); got != htClient {
			t.Errorf("ncHitTest(close) = %d, want %d (HTCLIENT)", got, htClient)
		}
		if got := w.ncHitTest(pack(orig.X+minX, orig.Y+cy)); got != htClient {
			t.Errorf("ncHitTest(min) = %d, want %d (HTCLIENT)", got, htClient)
		}
		if got := w.ncHitTest(pack(orig.X+dragX, orig.Y+cy)); got != htCaption {
			t.Errorf("ncHitTest(drag) = %d, want %d (HTCAPTION)", got, htCaption)
		}

		// Non-client hover messages over HTMAXBUTTON must be consumed so DefWindowProc
		// never paints native hover chrome or tooltips.
		if !w.ncMouse(wmNCMouseMove, htMaxButton) {
			t.Error("ncMouse(WM_NCMOUSEMOVE, HTMAXBUTTON) was not consumed")
		}
		if w.chrome.hover != htMaxButton {
			t.Errorf("hover after HTMAXBUTTON move = %d, want %d", w.chrome.hover, htMaxButton)
		}
		if !w.ncMouse(wmNCMouseHover, htMaxButton) {
			t.Error("ncMouse(WM_NCMOUSEHOVER, HTMAXBUTTON) was not consumed")
		}
		if !w.ncMouse(wmNCMouseLeave, 0) {
			t.Error("ncMouse(WM_NCMOUSELEAVE) was not consumed")
		}

		// Client hover + press + drag-off cancel on Close must not close or leak to views.
		if !w.captionMouse(wmMouseMove, pack(closeX, cy)) || w.chrome.hover != htClose {
			t.Errorf("captionMouse hover close failed: hover=%d", w.chrome.hover)
		}
		if !w.captionMouse(wmLButtonDown, pack(closeX, cy)) || w.chrome.pressed != htClose {
			t.Errorf("captionMouse press close failed: pressed=%d", w.chrome.pressed)
		}
		if !w.captionMouse(wmMouseMove, pack(dragX, cy)) || w.chrome.hover != 0 {
			t.Errorf("captionMouse drag off close did not clear hover: hover=%d", w.chrome.hover)
		}
		if !w.captionMouse(wmLButtonUp, pack(dragX, cy)) || w.chrome.pressed != 0 {
			t.Errorf("captionMouse release off close failed: pressed=%d", w.chrome.pressed)
		}
		if w.destroying {
			t.Error("canceling close press closed the window")
		}

		// Clicking Minimize minimizes the window.
		if !w.captionMouse(wmLButtonDown, pack(minX, cy)) {
			t.Error("captionMouse press minimize not handled")
		}
		if !w.captionMouse(wmLButtonUp, pack(minX, cy)) {
			t.Error("captionMouse release minimize not handled")
		}
	})
	for deadline := time.Now().Add(3 * time.Second); ; {
		minimized := false
		onMain(t, hwnd, func(w *window) { minimized = isIconicWindow(w.hwnd) })
		if minimized {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("clicking minimize caption button did not minimize the window")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if viewPressed {
		t.Error("caption button click leaked to underlying view")
	}
}
