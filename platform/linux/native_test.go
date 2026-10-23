//go:build linux && !android

package linux

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego"

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
		// Close the window for good: its close request only stops the
		// loop, and a tiling window manager shares the screen between the
		// windows left open.
		gtkWindowDestroy(w.win)
		for gMainContextIteration(0, 0) != 0 {
		}
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

// gMainContextIteration runs what the default main context has ready.
var gMainContextIteration = func() func(uintptr, int32) int32 {
	var fn func(uintptr, int32) int32
	if openLibs() == nil {
		purego.RegisterLibFunc(&fn, glibLib, "g_main_context_iteration")
	}
	return fn
}()

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

	// What the views drew is in the surface, not only in the engine's
	// model.
	waitPixels(t, w, "button pixel #123456", func(p pixels) bool {
		pix := p.at(60, 50)
		return pix[2] == 0x12 && pix[1] == 0x34 && pix[0] == 0x56
	})
}

// pixels is the surface as GTK draws it, premultiplied ARGB32.
type pixels struct {
	data   []byte
	stride int
	width  int
	height int
}

// at returns the BGRA bytes of the pixel at x, y.
func (p pixels) at(x, y int) []byte {
	if x < 0 || y < 0 || x >= p.width || y >= p.height {
		return []byte{0, 0, 0, 0}
	}
	return p.data[y*p.stride+x*4 : y*p.stride+x*4+4]
}

// surfacePixels draws the surface, natives included, into an image.
func (w *window) surfacePixels(t *testing.T) pixels {
	t.Helper()
	var p pixels
	w.onLoop(t, func() {
		width, height := int(gtkWidgetGetWidth(w.surface)), int(gtkWidgetGetHeight(w.surface))
		if width <= 0 || height <= 0 {
			return
		}
		surface := cairoImageSurfaceCreate(0, int32(width), int32(height))
		cr := cairoCreate(surface)
		w.paint(cr)
		cairoSurfaceFlush(surface)
		stride := int(cairoImageSurfaceGetStride(surface))
		data := (*[1 << 30]byte)(cPointer(cairoImageSurfaceGetData(surface)))[: stride*height : stride*height]
		p = pixels{data: append([]byte(nil), data...), stride: stride, width: width, height: height}
		cairoDestroy(cr)
		cairoSurfaceDestroy(surface)
	})
	return p
}

// waitPixels waits until the drawn surface satisfies ok: GTK lays out and
// draws a batch on its next frame.
func waitPixels(t *testing.T, w *window, what string, ok func(pixels) bool) pixels {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		p := w.surfacePixels(t)
		if p.width > 0 && ok(p) {
			return p
		}
		if time.Now().After(deadline) {
			t.Fatalf("want %s", what)
		}
		time.Sleep(50 * time.Millisecond)
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

	waitPixels(t, w, "a red span background and a blue attachment", func(p pixels) bool {
		span := p.at(int(textAt.X)+int(bg.x+bg.w/2), int(textAt.Y)+int(bg.y+bg.h/2))
		attached := p.at(int(button.Origin.X+20), int(button.Origin.Y+15))
		return span[2] == 0xff && span[1] == 0 && span[0] == 0 && attached[0] == 0xff && attached[1] == 0 && attached[2] == 0
	})

	// The attachment takes its own presses.
	w.onLoop(t, func() { w.press(core.Point{X: button.Origin.X + 20, Y: button.Origin.Y + 15}) })
	waitForTitle(t, w, title+"|1")
}

// TestWebViewTalksToThePage opens a window with a web view showing
// inline HTML, receives the message the page posts and posts one back,
// which the page answers. It skips where WebKitGTK 6.0 is missing.
func TestWebViewTalksToThePage(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if err := openWebKit(); err != nil {
		t.Skip(err)
	}
	const page = `<!doctype html><html><body><script>
		addEventListener('message', (e) => window.nsgo.postMessage('echo:' + e.data));
		window.nsgo.postMessage('ready');
	</script></body></html>`
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		var ref core.Ref
		last := core.NewSignal(cx, "")
		loaded := core.NewSignal(cx, 0)
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%d|%s", title, loaded.Get(), last.Get())) })
		return core.View{Style: core.Style{Width: core.Pt(360), Height: core.Pt(240), Padding: core.All(10)},
			Children: []core.Node{core.WebView{Source: core.WebHTML(page, ""), Ref: &ref,
				Style:   core.Style{Width: core.Pt(300), Height: core.Pt(200)},
				OnLoad:  func(core.WebNavigation) { loaded.Update(func(n int) int { return n + 1 }) },
				OnError: func(err error) { last.Set("error:" + err.Error()) },
				OnMessage: func(data string) {
					last.Set(data)
					if data == "ready" {
						ref.PostMessage("ping")
					}
				}}}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	waitForTitle(t, w, title+"|1|echo:ping")

	// The page fills the web view's frame, hosted in its widget.
	waitHosted(t, w, func(v *view) bool { return v.web != nil && v.web.widget != 0 },
		core.Bounds{Origin: core.Point{X: 10, Y: 10}, Size: core.Size{Width: 300, Height: 200}})
}

// waitHosted waits until the native widget of the view that is matches
// is a child of the view's widget, mapped at want in the surface.
func waitHosted(t *testing.T, w *window, is func(*view) bool, want core.Bounds) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var got core.Bounds
		var hosted, mapped bool
		w.onLoop(t, func() {
			for _, v := range w.views {
				if v == nil || !is(v) || v.hosted() == nil || v.hosted().child == 0 {
					continue
				}
				child := v.hosted().child
				hosted = gtkWidgetGetParent(child) == v.widget
				mapped = gtkWidgetGetMapped(child) != 0
				var r grapheneRect
				if gtkWidgetComputeBounds(child, w.surface, &r) != 0 {
					got = core.Bounds{Origin: core.Point{X: core.Points(r.x), Y: core.Points(r.y)},
						Size: core.Size{Width: core.Points(r.w), Height: core.Points(r.h)}}
				}
			}
		})
		if hosted && mapped && got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("native hosted %t, mapped %t at %v, want %v", hosted, mapped, got, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitTitle waits until the window's title satisfies ok.
func waitTitle(t *testing.T, w *window, ok func(string) bool) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		got := w.titleOf(t)
		if ok(got) {
			return got
		} else if time.Now().After(deadline) {
			t.Fatalf("window title = %q", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// aiff is half a second of 8 kHz mono silence, which GStreamer plays
// with its base and bad plug-ins alone.
func aiff() []byte {
	const rate, frames = 8000, 4000
	be := binary.BigEndian
	comm := make([]byte, 18)
	be.PutUint16(comm[0:], 1)      // channels
	be.PutUint32(comm[2:], frames) // sample frames
	be.PutUint16(comm[6:], 16)     // bits
	// 8000 as an 80-bit extended float: exponent 16383+12, mantissa
	// 8000 << (63-12).
	be.PutUint16(comm[8:], 16383+12)
	be.PutUint64(comm[10:], rate<<(63-12))
	ssnd := make([]byte, 8+frames*2)
	chunk := func(id string, data []byte) []byte {
		b := append([]byte(id), 0, 0, 0, 0)
		be.PutUint32(b[4:], uint32(len(data)))
		return append(b, data...)
	}
	body := append([]byte("AIFF"), chunk("COMM", comm)...)
	body = append(body, chunk("SSND", ssnd)...)
	return chunk("FORM", body)
}

// TestVideoReportsLoadProgressAndEnd plays a short sound through a
// Video, as NSH's Linux backend does, and reads its events.
func TestVideoReportsLoadProgressAndEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	path := filepath.Join(t.TempDir(), "silence.aiff")
	if err := os.WriteFile(path, aiff(), 0o644); err != nil {
		t.Fatal(err)
	}
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		state := core.NewSignal(cx, "")
		progress, ended := core.NewSignal(cx, 0), core.NewSignal(cx, 0)
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%s|%d|%d", title, state.Get(), progress.Get(), ended.Get())) })
		return core.View{Style: core.Style{Width: core.Pt(360), Height: core.Pt(240)}, Children: []core.Node{
			core.Video{Source: path, ProgressInterval: 50 * time.Millisecond,
				Style:   core.Style{Width: core.Pt(200), Height: core.Pt(100)},
				OnLoad:  func(i core.VideoInfo) { state.Set(fmt.Sprintf("load %v", i.Duration.Round(100*time.Millisecond))) },
				OnError: func(err error) { state.Set("error " + err.Error()) },
				OnProgress: func(time.Duration, time.Duration) {
					progress.Update(func(n int) int { return n + 1 })
				},
				OnEnd: func() { ended.Update(func(n int) int { return n + 1 }) }},
		}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	got := waitTitle(t, w, func(s string) bool { return strings.HasSuffix(s, "|1") || strings.Contains(s, "|error ") })
	if strings.Contains(got, "No installed GStreamer plug-in") {
		// GTK plays sound through autoaudiosink, of gst-plugins-good.
		t.Skip(got)
	}
	if !strings.HasPrefix(got, title+"|load 500ms|") {
		t.Fatalf("title = %q, want a load of 500ms and an end", got)
	}
	var p, e int
	fmt.Sscanf(got[strings.LastIndex(got, "ms|")+3:], "%d|%d", &p, &e)
	if p == 0 {
		t.Errorf("title = %q: no progress while playing", got)
	}
}

// TestVideoReportsWhatGStreamerCannotPlay opens bytes that are no media.
func TestVideoReportsWhatGStreamerCannotPlay(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	path := filepath.Join(t.TempDir(), "noise.mp4")
	if err := os.WriteFile(path, []byte(strings.Repeat("not a video ", 512)), 0o644); err != nil {
		t.Fatal(err)
	}
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		state := core.NewSignal(cx, "")
		cx.Effect(func() { cx.SetTitle(title + "|" + state.Get()) })
		return core.Video{Source: path, Style: core.Style{Width: core.Pt(200), Height: core.Pt(100)},
			OnLoadStart: func() { state.Set("start") },
			OnLoad:      func(core.VideoInfo) { state.Set("load") },
			OnError:     func(err error) { state.Set("error " + err.Error()) }}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	got := waitTitle(t, w, func(s string) bool { return strings.Contains(s, "|error ") || strings.HasSuffix(s, "|load") })
	if !strings.Contains(got, "|error ") || strings.Contains(got, "\n") {
		t.Fatalf("title = %q, want one line of error", got)
	}
}

// TestWebViewWithoutWebKitShowsAPlaceholder runs where WebKitGTK 6.0 is
// missing, as NSH's Linux backend built without it does.
func TestWebViewWithoutWebKitShowsAPlaceholder(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if openWebKit() == nil {
		t.Skip("WebKitGTK 6.0 is installed")
	}
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		cx.SetTitle(title)
		return core.View{Style: core.Style{Padding: core.All(10)}, Children: []core.Node{
			core.WebView{Source: core.WebSrc("https://example.com"), Style: core.Style{Width: core.Pt(300), Height: core.Pt(200)}},
		}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	waitForTitle(t, w, title)
	waitHosted(t, w, func(v *view) bool { return v.web != nil && v.web.widget == 0 },
		core.Bounds{Origin: core.Point{X: 10, Y: 10}, Size: core.Size{Width: 300, Height: 200}})
}

// brokenVideo writes bytes GStreamer cannot play: GtkVideo then shows its
// black frame and an error icon.
func brokenVideo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "broken.mp4")
	if err := os.WriteFile(path, []byte(strings.Repeat("not a video ", 512)), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// dark reports a pixel drawn by GtkVideo's black frame.
func dark(p []byte) bool { return p[3] == 0xff && p[0] < 0x30 && p[1] < 0x30 && p[2] < 0x30 }

// The tests below keep what they read in the window's top left 130 × 130
// points: a tiling window manager may give a test window little room.

// TestLaterViewsCoverHostedNatives covers part of a video with a later
// sibling, as a dialog covers one: the sibling is drawn and picked over
// the video, which still shows beside it.
func TestLaterViewsCoverHostedNatives(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	path := brokenVideo(t)
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		cx.SetTitle(title)
		return core.View{Style: core.Style{Width: core.Pt(360), Height: core.Pt(240), Background: core.RGB(0xffffff)},
			Children: []core.Node{
				core.Video{Source: path, Controls: true, Style: core.Style{Width: core.Pt(140), Height: core.Pt(120)}},
				core.View{TestID: "cover", Style: core.Style{Position: core.Absolute, Left: core.Pt(70), Top: core.Pt(0),
					Width: core.Pt(70), Height: core.Pt(120), Background: core.RGB(0xff0000)}},
			}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	waitForTitle(t, w, title)
	waitPixels(t, w, "the cover red over the video, the video dark beside it", func(p pixels) bool {
		cover, video := p.at(105, 60), p.at(20, 15)
		return cover[2] == 0xff && cover[1] == 0 && cover[0] == 0 && dark(video)
	})
	// GTK gives presses there to the cover too, not to the video.
	var picked, cover uintptr
	w.onLoop(t, func() {
		picked = gtkWidgetPick(w.surface, 105, 60, gtkPickDefault)
		for _, v := range w.views {
			if v != nil && v.props.TestID == "cover" {
				cover = v.widget
			}
		}
	})
	if picked == 0 || picked != cover {
		t.Fatalf("picked %#x, want the cover %#x", picked, cover)
	}
}

// TestRoundedParentsClipHostedNatives puts a video in a rounded parent
// that clips: the parent's corners cut the video's off.
func TestRoundedParentsClipHostedNatives(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	path := brokenVideo(t)
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		cx.SetTitle(title)
		return core.View{Style: core.Style{Width: core.Pt(360), Height: core.Pt(240), Padding: core.All(10),
			Background: core.RGB(0xffffff)},
			Children: []core.Node{core.View{
				Style: core.Style{Width: core.Pt(120), Height: core.Pt(120), CornerRadius: 40, Overflow: core.Hidden},
				Children: []core.Node{core.Video{Source: path, Controls: true,
					Style: core.Style{Width: core.Pt(120), Height: core.Pt(120)}}},
			}}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	waitForTitle(t, w, title)
	waitPixels(t, w, "the video dark inside the corners and cut off at them", func(p pixels) bool {
		corner, inside := p.at(12, 12), p.at(70, 20)
		white := corner[0] == 0xff && corner[1] == 0xff && corner[2] == 0xff
		return white && dark(inside)
	})
}

// TestWheelScrollsContentAndClipsIt turns the wheel over a scroll view:
// its content moves inside it, clipped, and the move is reported.
func TestWheelScrollsContentAndClipsIt(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		offset := core.NewSignal(cx, core.Points(0))
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%v", title, offset.Get())) })
		return core.View{Style: core.Style{Width: core.Pt(360), Height: core.Pt(240), Padding: core.All(5),
			Background: core.RGB(0xffffff)},
			Children: []core.Node{core.Scroll{HideIndicator: true, OnScroll: func(p core.Point) { offset.Set(p.Y) },
				Style: core.Style{Width: core.Pt(40), Height: core.Pt(40)},
				Children: []core.Node{
					core.View{Style: core.Style{Height: core.Pt(40), Background: core.RGB(0xff0000)}},
					core.View{Style: core.Style{Height: core.Pt(100), Background: core.RGB(0x0000ff)}},
				}}}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	waitForTitle(t, w, title+"|0")
	waitPixels(t, w, "red content at the top", func(p pixels) bool { return p.at(20, 10)[2] == 0xff })
	w.onLoop(t, func() { w.wheel(core.Point{X: 20, Y: 20}, 0, 60) })
	waitForTitle(t, w, title+"|60")
	waitPixels(t, w, "blue content scrolled up, white below the scroll view", func(p pixels) bool {
		in, below := p.at(20, 10), p.at(20, 52)
		return in[0] == 0xff && in[2] == 0 && below[0] == 0xff && below[1] == 0xff && below[2] == 0xff
	})
}
