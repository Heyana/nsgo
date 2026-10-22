//go:build linux && !android

package linux

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/engine"
)

// nativeApp is the app a native test drives: a button and a field whose
// state the window's title shows, so the test can wait for the engine.
func nativeApp(title string) engine.App {
	return func(cx *core.Scope) core.Node {
		count := core.NewSignal(cx, 0)
		field := core.NewSignal(cx, "")
		submitted := core.NewSignal(cx, 0)
		keyed := core.NewSignal(cx, 0)
		cx.OnKey("mod+k", func() { keyed.Update(func(n int) int { return n + 1 }) })
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|%d|%s|%d|%d", title, count.Get(), field.Get(), submitted.Get(), keyed.Get()))
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

// startTestWindow opens the window on a GTK thread of its own and returns
// it once its engine runs. The test skips where no display can open one.
func startTestWindow(t *testing.T, app engine.App, opts Options) *window {
	t.Helper()
	type result struct {
		w   *window
		err error
	}
	ready := make(chan result, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.LockOSThread()
		w, err := start(app, opts)
		ready <- result{w, err}
		if err != nil {
			return
		}
		w.run()
		w.stop()
	}()
	var r result
	select {
	case r = <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("the window did not start")
	}
	if r.err != nil {
		<-done
		if strings.Contains(r.err.Error(), "no display") || strings.Contains(r.err.Error(), "GTK 4") {
			t.Skip(r.err)
		}
		t.Fatal(r.err)
	}
	t.Cleanup(func() {
		w := r.w
		if !w.closing {
			w.onLoop(t, func() { gtkWindowClose(w.win) })
		}
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("the window did not close")
		}
	})
	return r.w
}

// onLoop runs fn on the window's GTK thread and waits for it.
func (w *window) onLoop(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	w.post(func() {
		defer close(done)
		fn()
	})
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the GTK thread did not run the step")
	}
}

// title returns the window's GTK title.
func (w *window) titleOf(t *testing.T) string {
	t.Helper()
	var title string
	w.onLoop(t, func() { title = goString(gtkWindowGetTitle(w.win)) })
	return title
}

// waitForTitle waits for the engine to set the window's title.
func waitForTitle(t *testing.T, w *window, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if got := w.titleOf(t); got == want {
			return
		} else if time.Now().After(deadline) {
			t.Fatalf("window title = %q, want %q", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestWindowRunsAnAppAndTakesNativeInput opens a real GTK window, clicks
// its view, types into its GtkEntry and reads the pixels it painted.
func TestWindowRunsAnAppAndTakesNativeInput(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-native-" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	w := startTestWindow(t, nativeApp(title), Options{Title: title, Width: 360, Height: 240})
	waitForTitle(t, w, title+"|0||0|0")

	// The app's keymap takes a native key press before the view does.
	w.onLoop(t, func() { keyPressedCallback(0, 'k', 0, gdkControl, w.handle) })
	waitForTitle(t, w, title+"|0||0|1")

	// The button at (10, 10) to (110, 90) in the surface.
	w.onLoop(t, func() { w.press(core.Point{X: 60, Y: 50}) })
	waitForTitle(t, w, title+"|1||0|1")

	// The native GtkEntry reports its changes to the engine.
	w.onLoop(t, func() {
		for entry, v := range w.entries {
			if v.kind == core.KindInput {
				gtkEditableSetText(entry, cstr("hi"))
			}
		}
	})
	waitForTitle(t, w, title+"|1|hi|0|1")

	// Return submits the focused field.
	w.onLoop(t, func() {
		for entry, v := range w.entries {
			if v.kind == core.KindInput {
				gSignalEmitByName(entry, cstr("activate"))
			}
		}
	})
	waitForTitle(t, w, title+"|1|hi|1|1")

	// What Cairo drew is in the surface, not only in the engine's model.
	var r, g, b uint8
	w.onLoop(t, func() {
		surface := cairoImageSurfaceCreate(0, 360, 240)
		cr := cairoCreate(surface)
		w.paint(cr, core.Size{Width: 360, Height: 240})
		cairoSurfaceFlush(surface)
		stride := int(cairoImageSurfaceGetStride(surface))
		data := cairoImageSurfaceGetData(surface)
		pix := pixelAt(data, stride, 60, 50)
		r, g, b = pix[2], pix[1], pix[0]
		cairoDestroy(cr)
		cairoSurfaceDestroy(surface)
	})
	if r != 0x12 || g != 0x34 || b != 0x56 {
		t.Fatalf("button pixel = #%02x%02x%02x, want #123456", r, g, b)
	}
}

// pixelAt returns the four bytes of the premultiplied ARGB32 pixel at x,
// y.
func pixelAt(data uintptr, stride, x, y int) []byte {
	p := cPointer(data)
	row := (*[1 << 30]byte)(p)[y*stride : (y+1)*stride]
	return row[x*4 : x*4+4]
}

// TestTextDrawsBackgroundsAndAttachments opens a window with a text whose
// span has a background and whose attachment is a pressable view, reads
// the pixels painted and presses the attachment.
func TestTextDrawsBackgroundsAndAttachments(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		pressed := core.NewSignal(cx, 0)
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%d", title, pressed.Get())) })
		return core.View{Style: core.Style{Width: core.Pt(360), Height: core.Pt(240), Padding: core.All(10),
			Background: core.RGB(0xffffff), FontSize: 16, Color: core.RGB(0x000000)},
			Children: []core.Node{core.Text{Spans: []core.Span{
				{Content: "text "},
				{Content: "x      x", Style: core.Style{Background: core.RGB(0xff0000), CornerRadius: 4}},
				{Content: " and "},
				{Style: core.Style{VerticalAlign: core.VerticalAlignMiddle},
					Attachment: core.View{TestID: "button", OnPress: func() { pressed.Update(func(n int) int { return n + 1 }) },
						Style: core.Style{Width: core.Pt(40), Height: core.Pt(30), Background: core.RGB(0x0000ff)}}},
				{Content: " after"},
			}}},
		}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	waitForTitle(t, w, title+"|0")

	var button core.Bounds
	var bg rect
	var textAt core.Point
	w.onLoop(t, func() {
		for _, v := range w.views {
			if v == nil || v.kind != core.KindText {
				continue
			}
			b, _ := w.surfaceBounds(v)
			textAt = core.Point{X: b.Origin.X + v.content.Origin.X, Y: b.Origin.Y + v.content.Origin.Y}
			if l, ok := v.text.(*textLayout); ok && len(l.under) > 0 {
				bg = l.under[0]
			}
			for _, c := range v.children {
				button, _ = w.surfaceBounds(c)
			}
		}
	})
	if button.Size != (core.Size{Width: 40, Height: 30}) {
		t.Fatalf("attachment frame = %v, want a 40x30 child of the text", button)
	}
	if bg.w == 0 || button.Origin.X < textAt.X+core.Points(bg.x+bg.w) {
		t.Fatalf("background %+v, attachment %v: want the attachment after the span", bg, button)
	}

	var span, attached []byte
	w.onLoop(t, func() {
		surface := cairoImageSurfaceCreate(0, 360, 240)
		cr := cairoCreate(surface)
		w.paint(cr, core.Size{Width: 360, Height: 240})
		cairoSurfaceFlush(surface)
		stride := int(cairoImageSurfaceGetStride(surface))
		data := cairoImageSurfaceGetData(surface)
		span = append(span, pixelAt(data, stride, int(textAt.X)+int(bg.x+bg.w/2), int(textAt.Y)+int(bg.y+bg.h/2))...)
		attached = append(attached, pixelAt(data, stride, int(button.Origin.X+20), int(button.Origin.Y+15))...)
		cairoDestroy(cr)
		cairoSurfaceDestroy(surface)
	})
	if span[2] != 0xff || span[1] != 0 || span[0] != 0 {
		t.Errorf("span background pixel = %v, want red", span)
	}
	if attached[0] != 0xff || attached[1] != 0 || attached[2] != 0 {
		t.Errorf("attachment pixel = %v, want blue", attached)
	}

	// The attachment takes its own presses.
	w.onLoop(t, func() { w.press(core.Point{X: button.Origin.X + 20, Y: button.Origin.Y + 15}) })
	waitForTitle(t, w, title+"|1")
}
