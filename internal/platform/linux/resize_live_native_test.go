//go:build linux && !android

package linux

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestCompositorResizeShowsEachSizeLaidOut has Hyprland resize a floating
// window step by step, keeping its left edge, as a drag of its right edge
// does, and reads the middle row of
// what the compositor shows at each size: a 200-point red column, a blue
// fill and a 24-point green strip end the window wherever it is shown,
// and past it is what was there before the window, never a strip the
// window drew without its layout, while the app takes most of a frame to
// lay out each size (#1375). It needs a Hyprland session, grim and
// NSGO_LIVE_RESIZE=1, and turns the compositor's animations off while it
// runs.
func TestCompositorResizeShowsEachSizeLaidOut(t *testing.T) {
	if os.Getenv("NSGO_LIVE_RESIZE") != "1" || os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") == "" {
		t.Skip("set NSGO_LIVE_RESIZE=1 in a Hyprland session")
	}
	grim, err := exec.LookPath("grim")
	if err != nil {
		t.Skip("needs grim")
	}
	hyprctl := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("hyprctl", args...).Output()
		if err == nil && strings.Contains(string(out), "error:") {
			err = fmt.Errorf("%s", out)
		}
		if err != nil {
			t.Fatalf("hyprctl %v: %v", args, err)
		}
		return string(out)
	}
	var animations struct{ Bool bool }
	if err := json.Unmarshal([]byte(hyprctl("-j", "getoption", "animations:enabled")), &animations); err != nil {
		t.Fatal(err)
	}
	animate := func(on bool) { hyprctl("eval", fmt.Sprintf("hl.config({ animations = { enabled = %t } })", on)) }
	animate(false)
	t.Cleanup(func() { animate(animations.Bool) })

	const (
		red, blue, green = 0xff0000, 0x0000ff, 0x00ff00
		left, strip      = 200, 24
		x0, y0, h        = 100, 100, 300
		from, to, step   = 400, 800, 10
	)
	app := func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1}, Children: []core.Node{
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
	const title = "nsgo-live-resize1375"
	startTestWindow(t, app, Options{Title: title, Width: from, Height: h})
	for deadline := time.Now().Add(5 * time.Second); !strings.Contains(hyprctl("clients"), "title: "+title); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("Hyprland does not list the window")
		}
	}
	// Hyprland's Lua dispatchers: a resize keeps the window's center, so
	// each goes with a move, in one request, to keep its left edge.
	window := `window = "title:^` + title + `$"`
	frame := func(width int) {
		hyprctl("--batch", fmt.Sprintf("dispatch hl.dsp.window.resize({ x = %d, y = %d, relative = false, %s }) ; "+
			"dispatch hl.dsp.window.move({ x = %d, y = %d, relative = false, %s })", width, h, window, x0, y0, window))
	}
	hyprctl("dispatch", "hl.dsp.window.float({ action = \"enable\", "+window+" })")
	frame(from)
	hyprctl("dispatch", "hl.dsp.focus({ "+window+" })")
	time.Sleep(500 * time.Millisecond)

	// row reads the screen's row through the window's middle, from its
	// left edge to past its widest.
	row := func() []uint32 {
		out, err := exec.Command(grim, "-g", fmt.Sprintf("%d,%d %dx1", x0, y0+h/2, to+40), "-").Output()
		if err != nil {
			t.Fatalf("grim: %v", err)
		}
		img, err := png.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		px := make([]uint32, b.Dx())
		for x := range px {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y).RGBA()
			px[x] = r>>8<<16 | g>>8<<8 | bl>>8
		}
		return px
	}
	near := func(c, want uint32) bool {
		for shift := 0; shift < 24; shift += 8 {
			if d := int(c>>shift&0xff) - int(want>>shift&0xff); d < -40 || d > 40 {
				return false
			}
		}
		return true
	}
	ours := func(c uint32) bool { return near(c, red) || near(c, blue) || near(c, green) }
	// What is behind the window where it grows, read before it grows.
	behind := row()
	if !near(behind[10], red) {
		t.Skipf("the window is not on screen where Hyprland was asked to put it: #%06x", behind[10])
	}
	// check finds the extent of the layout shown and says what is wrong:
	// a red column of another width, green not ending it, or anything but
	// what was behind between it and the size asked for, short of the
	// border Hyprland draws there.
	check := func(size int) string {
		px := row()
		shown := 0
		for shown < len(px) && (ours(px[shown]) || shown+1 < len(px) && ours(px[shown+1])) {
			shown++
		}
		edge := 0
		for edge < shown && near(px[edge], red) {
			edge++
		}
		if edge < left-2 || edge > left+2 {
			return fmt.Sprintf("red ends at %d, want %d", edge, left)
		}
		greenFrom := shown
		for greenFrom > edge && near(px[greenFrom-1], green) {
			greenFrom--
		}
		if n := shown - greenFrom; n < strip-2 || n > strip+1 {
			return fmt.Sprintf("green %d wide at %d of %d shown", n, greenFrom, shown)
		}
		for x := shown + 2; x < size-4 && x < len(px); x++ {
			if x >= from && !near(px[x], behind[x]) {
				return fmt.Sprintf("x %d is #%06x past the layout shown to %d at size %d, was #%06x", x, px[x], shown, size, behind[x])
			}
		}
		return ""
	}
	samples := 0
	var bad []string
	sizes := []int{}
	for s := from + step; s <= to; s += step {
		sizes = append(sizes, s)
	}
	for s := to - step; s >= from; s -= step {
		sizes = append(sizes, s)
	}
	for _, size := range sizes {
		frame(size)
		for range 2 {
			samples++
			if problem := check(size); problem != "" {
				bad = append(bad, fmt.Sprintf("size %d: %s", size, problem))
			}
		}
	}
	t.Logf("%d samples at %d sizes: %d showed a stretched frame or background", samples, len(sizes), len(bad))
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
