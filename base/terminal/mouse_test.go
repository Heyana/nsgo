package terminal

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/terminal/internal/vt"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func mouseApp(t *testing.T, preference func(*core.Scope) func() bool) (*nsgotest.App, *Terminal, *recordingConn, *Ref) {
	t.Helper()
	conn := newRecordingConn()
	term, err := New(Options{Conn: conn})
	if err != nil {
		if !vt.Supported {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	ref := &Ref{}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		var enabled func() bool
		if preference != nil {
			enabled = preference(cx)
		}
		return View{Terminal: term, Ref: ref, TestID: "terminal", MouseReporting: enabled,
			Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
	}, nsgotest.Size(640, 320), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.WaitUntil(5*time.Second, func() bool { return len(app.Query("id:terminal-screen")) == 1 && ref.v.cols > 0 })
	return app, term, conn, ref
}

func mouseAt(app *nsgotest.App, ref *Ref, col, row int) core.Point {
	var point core.Point
	app.Post(func() {
		v := ref.v
		point = core.Point{X: core.Points(v.padX + (float32(col)+.5)*v.cellW), Y: core.Points(float64(v.padY+(float32(row)+.5)*v.cellH) + v.scrollY)}
	})
	app.Settle()
	return point
}

// The marker shares the actual input queue, so an empty expected report is checked after earlier writes have drained rather than by a timing sleep.
func mouseInput(t *testing.T, app *nsgotest.App, term *Terminal, conn *recordingConn, want string) {
	t.Helper()
	const marker = "\x00mouse-input-barrier\x00"
	term.Send([]byte(marker))
	var got string
	app.WaitUntil(5*time.Second, func() bool { got += conn.take(); return strings.Contains(got, marker) })
	got = strings.TrimSuffix(got, marker)
	if got != want {
		t.Fatalf("program input = %q, want %q", got, want)
	}
}

func mouseMode(app *nsgotest.App, term *Terminal, sequence string, capture bool) {
	term.Feed([]byte(sequence))
	app.WaitUntil(5*time.Second, func() bool { return app.Find("id:terminal-screen").Events.Has(driver.EventWheel) == capture })
	app.Settle()
}

func TestViewReportsRequestedMouseEvents(t *testing.T) {
	app, term, conn, ref := mouseApp(t, nil)
	mouseMode(app, term, "\x1b[?1000h\x1b[?1006h", true)
	point := mouseAt(app, ref, 25, 2)
	for _, pair := range []struct {
		button core.MouseButton
		code   int
	}{{core.MouseButtonLeft, 0}, {core.MouseButtonRight, 2}, {core.MouseButtonMiddle, 1}, {core.MouseButtonBack, 128}, {core.MouseButtonForward, 129}} {
		app.MouseDownAt("id:terminal-screen", point, pair.button)
		mouseInput(t, app, term, conn, fmt.Sprintf("\x1b[<%d;26;3M", pair.code))
		app.MouseUpAt("id:terminal-screen", point, pair.button)
		mouseInput(t, app, term, conn, fmt.Sprintf("\x1b[<%d;26;3m", pair.code))
	}
	mods := core.Modifiers{Control: true, Alt: true}
	app.MouseDownAtWith("id:terminal-screen", point, core.MouseButtonLeft, mods)
	mouseInput(t, app, term, conn, "\x1b[<24;26;3M")
	app.MouseUpAtWith("id:terminal-screen", point, core.MouseButtonLeft, mods)
	mouseInput(t, app, term, conn, "\x1b[<24;26;3m")
	// 1000 excludes motion even with a button held; 1002 includes drags, and 1003 includes hover. The encoder deduplicates within a cell.
	app.HoverAt("id:terminal-screen", mouseAt(app, ref, 2, 1))
	mouseInput(t, app, term, conn, "")
	mouseMode(app, term, "\x1b[?1000l\x1b[?1002h", true)
	point = mouseAt(app, ref, 2, 1)
	app.MouseDownAt("id:terminal-screen", point, core.MouseButtonLeft)
	mouseInput(t, app, term, conn, "\x1b[<0;3;2M")
	app.HoverAt("id:terminal-screen", mouseAt(app, ref, 3, 1))
	mouseInput(t, app, term, conn, "\x1b[<32;4;2M")
	app.MouseUpAt("id:terminal-screen", mouseAt(app, ref, 3, 1), core.MouseButtonLeft)
	mouseInput(t, app, term, conn, "\x1b[<0;4;2m")
	mouseMode(app, term, "\x1b[?1002l\x1b[?1003h", true)
	point = mouseAt(app, ref, 4, 2)
	app.HoverAt("id:terminal-screen", point)
	mouseInput(t, app, term, conn, "\x1b[<35;5;3M")
	point.X += .2
	app.HoverAt("id:terminal-screen", point)
	mouseInput(t, app, term, conn, "")
}

func TestViewWheelReportsAndAlternateScreenKeys(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("model discrete-wheel reports; real touchpad covered by TestTerminalNativeMouseAndTouchpad")
	}
	app, term, conn, _ := mouseApp(t, nil)
	id := app.Find("id:terminal-screen").ID
	mouseMode(app, term, "\x1b[?1000h\x1b[?1006h", true)
	app.Wheel("id:terminal-screen", core.Point{Y: 40}, core.Modifiers{})
	// Wheel's point is the content's center; only the button/count matter here, while the mouse test checks exact cell conversion.
	const marker = "\x00wheel-barrier\x00"
	term.Send([]byte(marker))
	var got string
	app.WaitUntil(5*time.Second, func() bool { got += conn.take(); return strings.Contains(got, marker) })
	if strings.Count(got, "\x1b[<65;") != 3 {
		t.Fatalf("wheel reports = %q, want three down reports", got)
	}
	mouseMode(app, term, "\x1b[?1000l\x1b[?1049h\x1b[?1007h\x1b[?1h", true)
	app.Wheel("id:terminal-screen", core.Point{Y: -40}, core.Modifiers{})
	mouseInput(t, app, term, conn, strings.Repeat("\x1bOA", 3))
	mouseMode(app, term, "\x1b[?1l", true)
	app.Wheel("id:terminal-screen", core.Point{Y: 40}, core.Modifiers{})
	mouseInput(t, app, term, conn, strings.Repeat("\x1b[B", 3))
	mouseMode(app, term, "\x1b[?1007l", false)
	mouseInput(t, app, term, conn, "")
	if got := app.Find("id:terminal-screen"); got.ID != id || got.Events.Has(driver.EventWheel) {
		t.Fatalf("normal scrolling remounted the hit layer or kept interception: %+v", got)
	}
}

func TestMouseReportingPreferenceRestoresSelection(t *testing.T) {
	var enabled *core.Signal[bool]
	app, term, conn, ref := mouseApp(t, func(cx *core.Scope) func() bool { enabled = core.NewSignal(cx, true); return enabled.Get })
	mouseMode(app, term, "hello world\x1b[?1000h\x1b[?1006h", true)
	id := app.Find("id:terminal-screen").ID
	app.Post(func() { enabled.Set(false) })
	app.Settle()
	if got := app.Find("id:terminal-screen"); got.ID != id || got.Events.Has(driver.EventWheel) {
		t.Fatal("reporting preference did not restore native scrolling in place")
	}
	point := mouseAt(app, ref, 1, 0)
	app.PressAt("id:terminal-screen", point)
	app.PressAt("id:terminal-screen", point)
	mouseInput(t, app, term, conn, "")
	var selected bool
	app.Post(func() { selected = ref.Selected() })
	app.Settle()
	if !selected {
		t.Fatal("disabling reporting did not restore selection")
	}
	app.Post(ref.Copy)
	app.Settle()
	if got := app.Clipboard; !strings.Contains(got, "hell") {
		t.Fatalf("selection after disabling reporting = %q", got)
	}
}

func TestMousePixelProtocolUsesRenderedCoordinates(t *testing.T) {
	app, term, conn, ref := mouseApp(t, nil)
	mouseMode(app, term, "\x1b[?1000h\x1b[?1006l\x1b[?1016h", true)
	point := core.Point{X: 36.25, Y: 20.25}
	var wantX, wantY int
	app.Post(func() {
		_, _, x, y := ref.v.cellAt(point)
		wantX, wantY = int(x)+1, int(y)+1
	})
	app.Settle()
	app.MouseDownAt("id:terminal-screen", point, core.MouseButtonLeft)
	const marker = "\x00pixel-barrier\x00"
	term.Send([]byte(marker))
	var got string
	app.WaitUntil(5*time.Second, func() bool { got += conn.take(); return strings.Contains(got, marker) })
	var x, y int
	if _, err := fmt.Sscanf(strings.TrimSuffix(got, marker), "\x1b[<0;%d;%dM", &x, &y); err != nil || math.Abs(float64(x-wantX)) > 1 || math.Abs(float64(y-wantY)) > 1 {
		t.Fatalf("pixel report=%q, want rendered coordinates %d,%d", got, wantX, wantY)
	}
	app.MouseUpAt("id:terminal-screen", point, core.MouseButtonLeft)
	mouseInput(t, app, term, conn, fmt.Sprintf("\x1b[<0;%d;%dm", x, y))
}

func TestShiftKeepsSelectionWhileMouseReportsEnabled(t *testing.T) {
	app, term, conn, ref := mouseApp(t, nil)
	mouseMode(app, term, "hello world\x1b[?1000h\x1b[?1006h", true)
	point := mouseAt(app, ref, 1, 0)
	app.PressAtWith("id:terminal-screen", point, core.Modifiers{Shift: true})
	app.PressAtWith("id:terminal-screen", point, core.Modifiers{Shift: true})
	mouseInput(t, app, term, conn, "")
	var selected bool
	app.Post(func() { selected = ref.Selected() })
	app.Settle()
	if !selected {
		t.Fatal("Shift did not retain selection under mouse capture")
	}
}

func TestTerminalNativeMouseAndTouchpad(t *testing.T) {
	if os.Getenv("NSGO_TEST_TARGET") != "macos" {
		t.Skip("actual AppKit mouse and touchpad acceptance")
	}
	app, term, conn, ref := mouseApp(t, nil)
	mouseMode(app, term, "\x1b[?1000h\x1b[?1006h", true)
	point := mouseAt(app, ref, 2, 1)
	app.MouseDownAt("id:terminal-screen", point, core.MouseButtonLeft)
	app.MouseUpAt("id:terminal-screen", point, core.MouseButtonLeft)
	mouseInput(t, app, term, conn, "\x1b[<0;3;2M\x1b[<0;3;2m")
	var height core.Points
	app.Post(func() { height = core.Points(ref.v.cellH) })
	app.Settle()
	app.TouchpadScroll("id:terminal-screen", core.Point{}, core.Point{Y: 2 * height})
	const marker = "\x00native-wheel-barrier\x00"
	term.Send([]byte(marker))
	var got string
	app.WaitUntil(5*time.Second, func() bool { got += conn.take(); return strings.Contains(got, marker) })
	if strings.Count(got, "\x1b[<65;") != 2 {
		t.Fatalf("native touchpad reports = %q", got)
	}
	mouseMode(app, term, "\x1b[?1000l\x1b[?1049h\x1b[?1007h\x1b[?1h", true)
	app.TouchpadScroll("id:terminal-screen", core.Point{}, core.Point{Y: -2 * height})
	mouseInput(t, app, term, conn, strings.Repeat("\x1bOA", 2))
	term.Feed([]byte("\x1b[HMouse buttons and precision wheel reached the session.\r\n"))
	app.Settle()
	app.Screenshot("terminal-mouse/native")
}

func TestWheelStepsPreserveFractionAndDirection(t *testing.T) {
	var pending float64
	for _, tc := range []struct {
		delta float64
		want  int
	}{{.4, 0}, {.4, 0}, {.4, 1}, {-.5, 0}, {-.8, -1}, {2.7, 2}} {
		if got := wheelSteps(&pending, tc.delta); got != tc.want {
			t.Fatalf("delta %v: got %d want %d, residual %v", tc.delta, got, tc.want, pending)
		}
	}
}
