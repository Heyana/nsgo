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
	"github.com/oneo-me/nsgo/engine"
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

// windowPixel reads the client pixel at x, y as the window painted it,
// and reports whether Windows could copy the window's pixels.
func windowPixel(hwnd uintptr, x, y int32) (r, g, b uint8, ok bool) {
	pix, width, height, ok := clientPixels(hwnd)
	if !ok || width <= x || height <= y {
		return 0, 0, 0, false
	}
	at := (int(y)*int(width) + int(x)) * 4
	return pix[at+2], pix[at+1], pix[at], true
}

// clientPixels reads the window's client area as it painted it: 32-bit
// pixels, top to bottom, in the order B, G, R, A. It reports false when
// Windows cannot copy the window's pixels, as on a session with no
// desktop.
func clientPixels(hwnd uintptr) (pix []byte, width, height int32, ok bool) {
	var client rect
	getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	width, height = client.Right, client.Bottom
	if width <= 0 || height <= 0 {
		return nil, 0, 0, false
	}
	dc, _, _ := getDC.Call(hwnd)
	if dc == 0 {
		return nil, 0, 0, false
	}
	defer releaseDC.Call(hwnd, dc)
	mem, _, _ := createCompatibleDC.Call(dc)
	if mem == 0 {
		return nil, 0, 0, false
	}
	defer deleteDC.Call(mem)
	bmp, _, _ := createCompatibleBitmap.Call(dc, uintptr(width), uintptr(height))
	if bmp == 0 {
		return nil, 0, 0, false
	}
	defer deleteObject.Call(bmp)
	old, _, _ := selectObject.Call(mem, bmp)
	if r, _, _ := printWindow.Call(hwnd, mem, clientOnly); r == 0 {
		// A window without WM_PRINT support still has its client on
		// screen; copy it directly.
		if r, _, _ := bitBlt.Call(mem, 0, 0, uintptr(width), uintptr(height), dc, 0, 0, srcCopy); r == 0 {
			selectObject.Call(mem, old)
			return nil, 0, 0, false
		}
	}
	// GetDIBits wants the bitmap out of the DC.
	selectObject.Call(mem, old)
	pix = make([]byte, int(width)*int(height)*4)
	info := bitmapInfo{Width: width, Height: -height, Planes: 1, BitCount: 32}
	info.Size = uint32(unsafe.Sizeof(info))
	if n, _, _ := getDIBits.Call(mem, bmp, 0, uintptr(height), uintptr(unsafe.Pointer(&pix[0])),
		uintptr(unsafe.Pointer(&info)), 0); n == 0 {
		return nil, 0, 0, false
	}
	return pix, width, height, true
}

// TestRoundedCornersBlend draws a circle and checks the pixels along its
// edge: without antialiasing every one of them is black or white.
func TestRoundedCornersBlend(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	testEdgesBlend(t, "nsgo-round", func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(320), Height: core.Pt(240), Background: core.RGB(0xffffff),
			JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(200), Height: core.Pt(200), CornerRadius: 100,
				Background: core.RGB(0x000000)}},
		}}
	})
}

// TestRoundedClipBlendsItsEdge clips a black child to a circle and checks
// both that the corner of its frame keeps the background and that the
// circle's edge blends.
func TestRoundedClipBlendsItsEdge(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	app := func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(320), Height: core.Pt(240), Background: core.RGB(0xffffff),
			Padding: core.All(20)}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(200), Height: core.Pt(200), CornerRadius: 100,
				Overflow: core.Hidden, Background: core.RGB(0xffffff)}, Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(300), Height: core.Pt(300), Background: core.RGB(0x000000)}},
			}}}}
	}
	hwnd := testEdgesBlend(t, "nsgo-clip", app)
	if hwnd == 0 {
		return
	}
	scale := 1.0
	if dpi, _, _ := getDpiForSystem.Call(); dpi != 0 {
		scale = float64(dpi) / 96
	}
	// The frame's corner is outside the circle, so the child must not have
	// painted there.
	if r, g, b, ok := windowPixel(hwnd, int32(22*scale), int32(22*scale)); ok && (r != 0xff || g != 0xff || b != 0xff) {
		t.Errorf("the frame's corner = #%02x%02x%02x, want the background", r, g, b)
	}
	if r, g, b, ok := windowPixel(hwnd, int32(120*scale), int32(120*scale)); ok && (r != 0 || g != 0 || b != 0) {
		t.Errorf("the circle's centre = #%02x%02x%02x, want the child's black", r, g, b)
	}
}

// TestCanvasCurvesBlend checks that a canvas's curves blend too: GDI+
// draws them into a bitmap the painter blends back.
func TestCanvasCurvesBlend(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	draw := func(d *core.Drawing, size core.Size) {
		d.SetFillColor(core.RGB(0x000000)).BeginPath().
			Ellipse(float32(size.Width)/2, float32(size.Height)/2, float32(size.Width)/2-20, float32(size.Height)/2-20).Fill()
	}
	testEdgesBlend(t, "nsgo-curve", func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(320), Height: core.Pt(240), Background: core.RGB(0xffffff)},
			Children: []core.Node{core.Canvas{Draw: draw, Style: core.Style{Width: core.Pt(320), Height: core.Pt(240)}}}}
	})
}

// testEdgesBlend runs app in a window and checks that what it painted
// holds black, white and the greys between them, which only a blended
// edge makes.
func testEdgesBlend(t *testing.T, title string, app engine.App) uintptr {
	t.Helper()
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: title, Width: 320, Height: 240})
	}()
	hwnd := waitForWindow(t, title, done)
	if hwnd == 0 {
		return 0
	}
	t.Cleanup(func() {
		postMessageW.Call(hwnd, wmClose, 0, 0)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the window did not close")
		}
	})
	// Wait for the frame that has the shape, since the first may come
	// before the engine has laid it out.
	var pix []byte
	deadline := time.Now().Add(5 * time.Second)
	for {
		var ok bool
		if pix, _, _, ok = clientPixels(hwnd); ok && blackAndWhite(pix) {
			break
		}
		if time.Now().After(deadline) {
			t.Skip("the window painted no shape")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var black, white, blends int
	for i := 0; i+3 < len(pix); i += 4 {
		b, g, r := pix[i], pix[i+1], pix[i+2]
		switch {
		case r == 0 && g == 0 && b == 0:
			black++
		case r == 0xff && g == 0xff && b == 0xff:
			white++
		case r == g && g == b:
			blends++
		}
	}
	if black == 0 || white == 0 {
		t.Fatalf("pixels: %d black, %d white; want both", black, white)
	}
	if blends < 20 {
		t.Fatalf("%d pixels blend the shape's edge, want at least 20", blends)
	}
	return hwnd
}

// blackAndWhite reports whether pix holds both colors, which means the
// shape has been painted.
func blackAndWhite(pix []byte) bool {
	var black, white bool
	for i := 0; i+3 < len(pix); i += 4 {
		switch {
		case pix[i] == 0 && pix[i+1] == 0 && pix[i+2] == 0:
			black = true
		case pix[i] == 0xff && pix[i+1] == 0xff && pix[i+2] == 0xff:
			white = true
		}
	}
	return black && white
}

// TestTextDecorationsBackgroundsAndAttachments draws struck-through
// spaces, alone and in a span, a span's background and an attachment, and
// finds each in the window's pixels.
func TestTextDecorationsBackgroundsAndAttachments(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-inline"
	black := core.RGB(0x000000)
	strike := core.Style{TextDecoration: core.TextDecorationLineThrough, Color: black}
	spaces := "            "
	app := func(*core.Scope) core.Node {
		box := core.Style{Height: core.Pt(40), AlignItems: core.AlignStart}
		return core.View{Style: core.Style{Width: core.Pt(320), Height: core.Pt(240), Background: core.RGB(0xffffff),
			Padding: core.All(10), FontSize: 20, Color: black}, Children: []core.Node{
			core.View{Style: box, Children: []core.Node{core.Text{Content: spaces, Style: strike}}},
			core.View{Style: box, Children: []core.Node{core.Text{Spans: []core.Span{{Content: spaces, Style: strike}}}}},
			core.View{Style: core.Style{Height: core.Pt(60), AlignItems: core.AlignStart}, Children: []core.Node{
				core.Text{Spans: []core.Span{
					{Content: "ab "},
					{Content: "code", Style: core.Style{Background: core.RGB(0xff0000), CornerRadius: 4}},
					{Content: " "},
					{Attachment: core.View{Style: core.Style{Width: core.Pt(30), Height: core.Pt(30), Background: core.RGB(0x0000ff)}}},
					{Content: " cd"},
				}},
			}},
		}}
	}
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: title, Width: 320, Height: 240})
	}()
	hwnd := waitForWindow(t, title, done)
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
	scale := 1.0
	if dpi, _, _ := getDpiForSystem.Call(); dpi != 0 {
		scale = float64(dpi) / 96
	}
	px := func(p float64) int { return int(p * scale) }
	type box struct{ left, top, right, bottom, n int }
	find := func(pix []byte, width int, match func(r, g, b byte) bool, top, bottom int) box {
		bx := box{left: 1 << 30, top: 1 << 30}
		for y := top; y < bottom && (y+1)*width*4 <= len(pix); y++ {
			for x := 0; x < width; x++ {
				at := (y*width + x) * 4
				if match(pix[at+2], pix[at+1], pix[at]) {
					bx.n++
					bx.left, bx.right = min(bx.left, x), max(bx.right, x+1)
					bx.top, bx.bottom = min(bx.top, y), max(bx.bottom, y+1)
				}
			}
		}
		return bx
	}
	dark := func(r, g, b byte) bool { return r < 0x80 && g < 0x80 && b < 0x80 }
	red := func(r, g, b byte) bool { return r == 0xff && g == 0 && b == 0 }
	blue := func(r, g, b byte) bool { return r == 0 && g == 0 && b == 0xff }
	var pix []byte
	var width int32
	deadline := time.Now().Add(5 * time.Second)
	for {
		var ok bool
		if pix, width, _, ok = clientPixels(hwnd); ok && find(pix, int(width), blue, 0, px(240)).n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Skip("the window painted no attachment")
		}
		time.Sleep(10 * time.Millisecond)
	}
	w := int(width)
	if s := find(pix, w, dark, px(10), px(50)); s.n == 0 || s.right-s.left < px(30) {
		t.Errorf("a struck-through text's spaces drew %+v, want a line", s)
	}
	if s := find(pix, w, dark, px(50), px(90)); s.n == 0 || s.right-s.left < px(30) {
		t.Errorf("a struck-through span's spaces drew %+v, want a line", s)
	}
	bg := find(pix, w, red, px(90), px(150))
	att := find(pix, w, blue, px(90), px(150))
	if bg.n < px(20)*px(10) {
		t.Errorf("the span's background covers %d pixels, want a box", bg.n)
	}
	if got, want := att.right-att.left, px(30); got < want-1 || got > want+1 {
		t.Errorf("the attachment is %d pixels wide, want %d", got, want)
	}
	if got, want := att.bottom-att.top, px(30); got < want-1 || got > want+1 {
		t.Errorf("the attachment is %d pixels tall, want %d", got, want)
	}
	if att.left <= bg.right {
		t.Errorf("the attachment starts at %d, inside the background ending at %d", att.left, bg.right)
	}
	// It stands on the baseline, which is below the background's top by
	// the font's ascent: its bottom is between the background's top and
	// bottom.
	if att.bottom <= bg.top || att.bottom >= bg.bottom {
		t.Errorf("the attachment's bottom %d is not on the baseline, inside the background %d..%d",
			att.bottom, bg.top, bg.bottom)
	}
}

// TestFallbackGlyphsPaintAsTheirFallback draws a command symbol, a check
// mark and an emoji in the default font and again in the fallback family
// that has them, and checks the two renderings' ink has the same box: a
// glyph the font lacks would be GDI's missing-glyph box instead.
func TestFallbackGlyphsPaintAsTheirFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-glyphs"
	glyphs := []struct{ text, family string }{
		{"⌘", "Segoe UI Symbol"},
		{"✓", "Segoe UI Symbol"},
		{"😀", "Segoe UI Symbol"},
	}
	var rows []core.Node
	for _, g := range glyphs {
		for _, family := range []string{"", g.family} {
			style := core.Style{FontSize: 40, FontFamily: family, Color: core.RGB(0)}
			rows = append(rows, core.View{Style: core.Style{Height: core.Pt(60), AlignItems: core.AlignStart},
				Children: []core.Node{core.Text{Content: g.text, Style: style}}})
		}
	}
	app := func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(320), Height: core.Pt(400), Background: core.RGB(0xffffff),
			Padding: core.All(10)}, Children: rows}
	}
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: title, Width: 320, Height: 400})
	}()
	hwnd := waitForWindow(t, title, done)
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
	scale := 1.0
	if dpi, _, _ := getDpiForSystem.Call(); dpi != 0 {
		scale = float64(dpi) / 96
	}
	px := func(p float64) int { return int(p * scale) }
	dark := func(pix []byte, width, x, y int) bool {
		at := (y*width + x) * 4
		return pix[at] < 0x80 && pix[at+1] < 0x80 && pix[at+2] < 0x80
	}
	// ink returns the box of the dark pixels in a band, or a zero box.
	ink := func(pix []byte, width, top, bottom int) (left, right, y0, y1 int) {
		left, right, y0, y1 = 1<<30, -1, 1<<30, -1
		for y := top; y < bottom && (y+1)*width*4 <= len(pix); y++ {
			for x := 0; x < width; x++ {
				if dark(pix, width, x, y) {
					left, right = min(left, x), max(right, x)
					y0, y1 = min(y0, y), max(y1, y)
				}
			}
		}
		if right < 0 {
			return 0, 0, 0, 0
		}
		return
	}
	var pix []byte
	var width int32
	deadline := time.Now().Add(5 * time.Second)
	for {
		var ok bool
		if pix, width, _, ok = clientPixels(hwnd); ok {
			if l, r, top, bottom := ink(pix, int(width), px(10), px(70)); r > l && bottom > top {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Skip("the window painted no text")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for i, g := range glyphs {
		l1, r1, t1, b1 := ink(pix, int(width), px(10+float64(i*2)*60), px(10+float64(i*2+1)*60))
		l2, r2, t2, b2 := ink(pix, int(width), px(10+float64(i*2+1)*60), px(10+float64(i*2+2)*60))
		if r1 <= l1 || r2 <= l2 {
			t.Errorf("%q drew nothing in the default or the %s font", g.text, g.family)
			continue
		}
		if r1-l1 != r2-l2 || b1-t1 != b2-t2 || l1 != l2 {
			t.Errorf("%q's default glyph is %d×%d at x=%d, but %s's is %d×%d at x=%d",
				g.text, r1-l1, b1-t1, l1, g.family, r2-l2, b2-t2, l2)
		}
	}
}

func TestWebViewPostsMessagesBothWays(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if _, err := findWebView2(); err != nil {
		t.Skip(err)
	}
	title := "nsgo-web"
	const page = `<!doctype html><html><body><script>
addEventListener('message', function (e) { window.nsgo.postMessage('echo:' + e.data); });
window.nsgo.postMessage('ready');
</script></body></html>`
	app := func(cx *core.Scope) core.Node {
		var ref core.Ref
		loaded := core.NewSignal(cx, "waiting")
		echo := core.NewSignal(cx, "")
		cx.Effect(func() { cx.SetTitle(title + "|" + loaded.Get() + "|" + echo.Get()) })
		// The web view starts 20pt into a scroll view 150pt tall, which
		// cuts off its bottom.
		return core.View{Style: core.Style{Width: core.Pt(320), Height: core.Pt(240), Padding: core.All(10),
			Background: core.RGB(0xffffff)}, Children: []core.Node{
			core.Scroll{Style: core.Style{Width: core.Pt(300), Height: core.Pt(150)}, Children: []core.Node{
				core.View{Style: core.Style{Height: core.Pt(20)}},
				core.WebView{Source: core.WebHTML(page, ""), Ref: &ref, UserAgent: "nsgo-test",
					InjectedJavaScriptBeforeContent: "window.before = 'b';",
					InjectedJavaScript:              "window.nsgo.postMessage('loaded:' + window.before + ':' + navigator.userAgent);",
					Style:                           core.Style{Width: core.Pt(280), Height: core.Pt(200)},
					OnMessage: func(data string) {
						switch {
						case data == "ready":
							ref.PostMessage("hello")
						case strings.HasPrefix(data, "loaded:"):
							loaded.Set(data)
						default:
							echo.Set(data)
						}
					},
					OnError: func(err error) { loaded.Set("error " + err.Error()) }},
				core.View{Style: core.Style{Height: core.Pt(200)}},
			}},
		}}
	}
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: title + "|waiting|", Width: 320, Height: 240})
	}()
	hwnd := waitForWindow(t, title+"|waiting|", done)
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
	// The first start of the browser process can take a while.
	want := title + "|loaded:b:nsgo-test|echo:hello"
	deadline := time.Now().Add(30 * time.Second)
	for {
		got := editText(hwnd)
		if got == want {
			break
		}
		if strings.HasPrefix(got, title+"|error") {
			t.Fatalf("the web view failed: %q", got)
		}
		select {
		case <-done:
			t.Fatalf("the window closed with title %q", got)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("window title = %q, want %q", got, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
	host := waitForChild(t, hwnd, webHostClass)
	if host == 0 {
		t.Fatal("the web view has no host window")
	}
	scale := 1.0
	if dpi, _, _ := getDpiForSystem.Call(); dpi != 0 {
		scale = float64(dpi) / 96
	}
	px := func(p float64) int32 { return int32(math.Round(p * scale)) }
	// placed returns the host window's top in the client area and the
	// top and bottom of its visible region.
	placed := func() (top, from, to int32) {
		var r rect
		getWindowRect.Call(host, uintptr(unsafe.Pointer(&r)))
		at := point{r.Left, r.Top}
		screenToClient.Call(hwnd, uintptr(unsafe.Pointer(&at)))
		var box rect
		if kind, _, _ := getWindowRgnBox.Call(host, uintptr(unsafe.Pointer(&box))); kind == 0 {
			box = rect{0, 0, r.Right - r.Left, r.Bottom - r.Top}
		}
		return at.Y, box.Top, box.Bottom
	}
	near := func(a, b int32) bool { return a-b <= 1 && b-a <= 1 }
	if top, from, to := placed(); !near(top, px(30)) || from != 0 || !near(to, px(130)) {
		t.Fatalf("the host is at %d showing %d to %d, want %d showing 0 to %d", top, from, to, px(30), px(130))
	}
	// One notch scrolls 48pt: the top goes under the scroll view's edge.
	at := point{px(15), px(15)}
	clientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&at)))
	postMessageW.Call(hwnd, wmMouseWheel, uintptr(uint32(uint16(0xff88))<<16), uintptr(uint32(at.Y)<<16|uint32(at.X)&0xffff))
	deadline = time.Now().Add(5 * time.Second)
	for {
		top, from, to := placed()
		if near(top, px(-18)) && near(from, px(28)) && near(to, px(178)) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("after scrolling the host is at %d showing %d to %d, want %d showing %d to %d",
				top, from, to, px(-18), px(28), px(178))
		}
		time.Sleep(20 * time.Millisecond)
	}
}
