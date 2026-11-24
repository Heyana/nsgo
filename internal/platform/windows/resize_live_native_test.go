//go:build windows

package windows

import (
	"fmt"
	"os"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

var mouseEvent = user32.NewProc("mouse_event")

// TestEdgeDragShowsEachSizeLaidOut drags a window's right edge with the
// system's own pointer, through its modal sizing loop, and reads the
// screen at each size: a 200-point red column, a blue fill and a 24-point
// green strip at the right edge are where the layout at that size puts
// them, with neither a stretched frame, whose red would widen, nor a strip
// of background past the green, while the app takes most of a frame to lay
// out each size (#1375). It needs an interactive desktop
// session, and NSGO_LIVE_RESIZE=1.
func TestEdgeDragShowsEachSizeLaidOut(t *testing.T) {
	if os.Getenv("NSGO_LIVE_RESIZE") != "1" {
		t.Skip("set NSGO_LIVE_RESIZE=1 in an interactive desktop session")
	}
	const (
		red, blue, green, white = 0xff0000, 0x0000ff, 0x00ff00, 0xffffff
		left, strip             = 200, 24
	)
	app := func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, Height: core.Pct(100)}, Children: []core.Node{
			core.View{Style: core.Style{Width: driver.Pt(left), Background: driver.RGB(red)}},
			core.View{Style: core.Style{FlexGrow: 1, Background: driver.RGB(blue)}},
			// Each size takes the app 12 ms, within the wait, to style.
			core.View{StyleFunc: func() core.Style {
				_ = cx.Env().Window()
				for start := time.Now(); time.Since(start) < 12*time.Millisecond; {
				}
				return core.Style{Width: driver.Pt(strip), Background: driver.RGB(green)}
			}},
		}}
	}
	hwnd := startApp(t, "nsgo-live-resize1375", app, 400, 300)
	if _, ok := waitForPixel(t, hwnd, 2, 2, red, 5*time.Second); !ok {
		t.Skip("the window is not on screen: run in an interactive desktop session")
	}
	var scale float64
	onMain(t, hwnd, func(w *window) { scale = w.scale })
	var r rect
	getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	x, y := r.Right-3, (r.Top+r.Bottom)/2
	near := func(c, want uint32) bool {
		for shift := 0; shift < 24; shift += 8 {
			if d := int(c>>shift&0xff) - int(want>>shift&0xff); d < -40 || d > 40 {
				return false
			}
		}
		return true
	}
	// check reads the client area's middle row as the screen shows it and
	// says what is wrong with it, if anything. DWM can show the window at
	// its last extent for a composition after the client area has grown,
	// its border and what is behind it past there: the row is judged up to
	// the extent shown, where a strip of the window's white background or
	// a red column wider than 200 points is wrong, and the green strip
	// ending short of it, a layout of another size, is counted apart.
	check := func() (width int32, problem string, other bool) {
		pix, width, height, ok := screenPixels(hwnd)
		if !ok {
			return 0, "unreadable", false
		}
		// A size that changed while the screen was read says nothing.
		var after rect
		getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&after)))
		if after.Right != width || after.Bottom != height {
			return 0, "", false
		}
		row := int(height/2) * int(width) * 4
		at := func(x int32) uint32 {
			i := row + int(x)*4
			return uint32(pix[i+2])<<16 | uint32(pix[i+1])<<8 | uint32(pix[i])
		}
		ours := func(c uint32) bool {
			return near(c, red) || near(c, blue) || near(c, green) || near(c, white)
		}
		// A column blended between two of ours is ours too.
		shown := int32(0)
		for shown < width && (ours(at(shown)) || shown+1 < width && ours(at(shown+1))) {
			shown++
		}
		edge := int32(0)
		for edge < shown && near(at(edge), red) {
			edge++
		}
		if want := int32(left*scale + 0.5); edge < want-2 || edge > want+2 {
			return width, fmt.Sprintf("red ends at %d, want %d", edge, want), false
		}
		for x := edge; x < shown; x++ {
			if near(at(x), white) {
				return width, fmt.Sprintf("background at %d of %d, shown to %d", x, width, shown), false
			}
		}
		greenFrom := shown
		for greenFrom > edge && near(at(greenFrom-1), green) {
			greenFrom--
		}
		// Antialiasing blends the strip's last column into what is past it.
		want := int32(strip*scale + 0.5)
		return width, "", shown-greenFrom < want-2 || shown-greenFrom > want+1
	}
	setForegroundWindow.Call(hwnd)
	setCursorPos.Call(uintptr(x), uintptr(y))
	time.Sleep(200 * time.Millisecond)
	mouseEvent.Call(2, 0, 0, 0, 0) // MOUSEEVENTF_LEFTDOWN
	time.Sleep(200 * time.Millisecond)
	samples, others, sizes := 0, 0, map[int32]bool{}
	var bad []string
	// Out to 400 pixels wider and back, 10 pixels a step, faster than a
	// hand, reading the screen three times within a frame of each step,
	// once the sizing loop has had the pointer's move.
	for i := 1; i <= 80; i++ {
		dx := int32(i * 10)
		if i > 40 {
			dx = int32((80 - i) * 10)
		}
		setCursorPos.Call(uintptr(x+dx), uintptr(y))
		for _, wait := range []time.Duration{4, 4, 4} {
			time.Sleep(wait * time.Millisecond)
			width, problem, other := check()
			if width == 0 && problem == "" {
				continue
			}
			samples++
			sizes[width] = true
			if other {
				others++
			}
			if problem != "" {
				bad = append(bad, fmt.Sprintf("step %d +%dms: %s", i, wait, problem))
			}
		}
	}
	mouseEvent.Call(4, 0, 0, 0, 0) // MOUSEEVENTF_LEFTUP
	t.Logf("%d samples at %d widths, scale %.2f: %d showed a stretched frame or background, %d a layout of another size", samples, len(sizes), scale, len(bad), others)
	if len(sizes) < 20 {
		t.Fatalf("the drag resized the window to only %d widths: the pointer did not hold its edge", len(sizes))
	}
	for i, b := range bad {
		if i == 20 {
			t.Logf("... %d more", len(bad)-20)
			break
		}
		t.Log(b)
	}
	if len(bad) > 0 {
		t.Fail()
	}
}
