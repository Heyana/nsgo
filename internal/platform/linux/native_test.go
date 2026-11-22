//go:build linux && !android

package linux

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	alphaclips "github.com/nsgo-dev/nsgo/internal/platform/videoalpha/clips"
)

// videoRateClip contains a picture and a 440 Hz tone. Regenerate with:
//
//	ffmpeg -f lavfi -i testsrc2=size=32x24:rate=12:duration=8 -f lavfi -i sine=frequency=440:sample_rate=24000:duration=8 -c:v libvpx -crf 40 -b:v 0 -c:a libopus -b:a 12k -vbr off testdata/video-rate.webm
//
//go:embed testdata/video-rate.webm
var videoRateClip []byte

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
			Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240), Padding: driver.All(10),
				Direction: core.Row, Gap: 10, Background: driver.RGB(0xffffff)},
			Children: []core.Node{
				core.View{OnPress: func(core.PressEvent) { count.Update(func(n int) int { return n + 1 }) },
					Style: core.Style{Width: driver.Pt(100), Height: driver.Pt(80), Background: driver.RGB(0x123456)}},
				core.Input{Value: field.Get, OnChange: field.Set, AutoFocus: true,
					OnSubmit: func(string) { submitted.Update(func(n int) int { return n + 1 }) },
					Style:    core.Style{Width: driver.Pt(200), Height: driver.Pt(30)}},
			},
		}
	}
}

// testWindowCalls runs the native tests' windows, one after another, on the locked OS thread whose first window initializes GTK, as GTK must be called only on that thread. A window's call returns once it has closed, so the next test's window waits for it.
var testWindowCalls = sync.OnceValue(func() chan func() {
	calls := make(chan func())
	go func() {
		runtime.LockOSThread()
		serveLoop(func() bool {
			fn, ok := <-calls
			if !ok {
				return false
			}
			fn()
			return true
		})
	}()
	return calls
})

// startTestWindow opens the window on the shared GTK thread and returns it once its engine runs. The test skips where no display can open one, and fails when the previous test's window still holds the thread.
func startTestWindow(t *testing.T, app engine.App, opts Options) *window {
	t.Helper()
	type result struct {
		w   *window
		err error
	}
	ready := make(chan result, 1)
	done := make(chan struct{})
	open := func() {
		defer close(done)
		w, err := start(app, opts)
		ready <- result{w, err}
		if err != nil {
			return
		}
		defer func() {
			w.stop()
			// Close the window for good: its close request only stops the loop, and a tiling window manager shares the screen between the windows left open.
			gtkWindowDestroy(w.win)
			for gMainContextIteration(0, 0) != 0 {
			}
		}()
		w.run()
	}
	select {
	case testWindowCalls() <- open:
	case <-time.After(10 * time.Second):
		t.Fatal("the previous GTK window did not stop")
	}
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

// onLoop runs fn on the window's GTK thread and waits for it, after applying the mount batches the engine already handed over. Loop calls run between GLib dispatches, so a batch whose idle has not been dispatched yet would otherwise land after fn, and a step after engine.Sync would not see it.
func (w *window) onLoop(t *testing.T, fn func()) {
	t.Helper()
	if !runOnThread(postLoopCall, 10*time.Second, "panic on the GTK thread", func() {
		w.applyBatches()
		fn()
	}) {
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
	w.onLoop(t, func() { w.press(driver.Point{X: 60, Y: 50}) })
	waitForTitle(t, w, title+"|1||0|1")

	// The native GtkEntry reports its changes to the engine.
	w.onLoop(t, func() {
		for entry, v := range w.entries {
			if v.kind == driver.KindInput && entry == v.entry {
				gtkEditableSetText(entry, cstr("hi"))
			}
		}
	})
	waitForTitle(t, w, title+"|1|hi|0|1")

	// Return submits the focused field.
	w.onLoop(t, func() {
		for entry, v := range w.entries {
			if v.kind == driver.KindInput && entry == v.entry {
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

// TestWindowCloseReleasesEditorProviders opens and closes windows with a field and a multiline input, the app's own window and one it opens beside a window that stays, and checks that the display-wide CSS providers their editors add go with them while the window left open keeps its own (#859).
func TestWindowCloseReleasesEditorProviders(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	inputs := func() core.Node {
		return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(120)}, Children: []core.Node{
			core.Input{DefaultValue: "field", Style: core.Style{Width: driver.Pt(160), Height: driver.Pt(30)}},
			core.Input{DefaultValue: "lines", Multiline: true, Style: core.Style{Width: driver.Pt(160), Height: driver.Pt(60)}},
		}}
	}
	before := editorProviders.Load()
	waitFor := func(what string, want int64) {
		t.Helper()
		for deadline := time.Now().Add(10 * time.Second); editorProviders.Load() != want; time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s: editor providers = %d, want %d", what, editorProviders.Load(), want)
			}
		}
	}
	// The app's own window, opened and closed again and again.
	for i := range 3 {
		w := startTestWindow(t, func(*core.Scope) core.Node { return inputs() }, Options{Title: "nsgo-editor-providers", Width: 200, Height: 120})
		waitFor(fmt.Sprintf("window %d open", i), before+2)
		w.onLoop(t, w.close)
		waitFor(fmt.Sprintf("window %d closed", i), before)
	}
	// Windows the app opens: the one that closes takes its providers, the one that stays keeps its.
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		cx.OpenWindow(core.WindowOptions{Title: "nsgo-editors-stay", Width: 200, Height: 120}, func(*core.Scope) core.Node { return inputs() })
		cx.OpenWindow(core.WindowOptions{Title: "nsgo-editors-go", Width: 200, Height: 120}, func(*core.Scope) core.Node { return inputs() })
	})
	w := startTestWindow(t, app, Options{Title: "nsgo-app", Width: 120, Height: 80, Hidden: true})
	waitFor("both opened windows open", before+4)
	var stays, goes *window
	w.onLoop(t, func() {
		windowHandles.Range(func(_, v any) bool {
			if o := v.(*window); !o.primary && o.win != 0 {
				switch goString(gtkWindowGetTitle(o.win)) {
				case "nsgo-editors-stay":
					stays = o
				case "nsgo-editors-go":
					goes = o
				}
			}
			return true
		})
	})
	if stays == nil || goes == nil {
		t.Fatal("the opened windows were not found")
	}
	w.onLoop(t, func() { gtkWindowClose(goes.win) })
	waitFor("one opened window closed", before+2)
	w.onLoop(t, func() {
		n := 0
		for _, v := range stays.views {
			if v != nil && v.provider != 0 {
				n++
				if v.entry == 0 {
					t.Error("the window left open lost an editor")
				}
			}
		}
		if n != 2 {
			t.Errorf("the window left open has %d editor providers, want 2", n)
		}
	})
	w.onLoop(t, func() { gtkWindowClose(stays.win) })
	waitFor("both opened windows closed", before)
	w.onLoop(t, w.close)
	waitFor("the app closed", before)
	// A window the app opened that is still open when the app's own window stops goes with it, and its providers too.
	app = core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		cx.OpenWindow(core.WindowOptions{Title: "nsgo-editors-left", Width: 200, Height: 120}, func(*core.Scope) core.Node { return inputs() })
	})
	w = startTestWindow(t, app, Options{Title: "nsgo-app-left", Width: 120, Height: 80, Hidden: true})
	waitFor("the window left open open", before+2)
	var left *window
	w.onLoop(t, func() {
		windowHandles.Range(func(_, v any) bool {
			if o := v.(*window); !o.primary && o.win != 0 && goString(gtkWindowGetTitle(o.win)) == "nsgo-editors-left" {
				left = o
			}
			return true
		})
	})
	if left == nil {
		t.Fatal("the opened window was not found")
	}
	w.onLoop(t, w.close)
	waitFor("the app closed with a window left open", before)
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, ok := windowHandles.Load(left.handle); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the window left open outlived the app")
		}
	}
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
	w.onLoop(t, func() { p = w.drawnPixels() })
	return p
}

// drawnPixels draws the surface into an image on the loop, from the state the window holds at that moment; it is empty while the surface has no size.
func (w *window) drawnPixels() pixels {
	width, height := int(gtkWidgetGetWidth(w.surface)), int(gtkWidgetGetHeight(w.surface))
	if width <= 0 || height <= 0 {
		return pixels{}
	}
	surface := cairoImageSurfaceCreate(0, int32(width), int32(height))
	cr := cairoCreate(surface)
	w.paint(cr)
	cairoSurfaceFlush(surface)
	stride := int(cairoImageSurfaceGetStride(surface))
	data := (*[1 << 30]byte)(cPointer(cairoImageSurfaceGetData(surface)))[: stride*height : stride*height]
	p := pixels{data: append([]byte(nil), data...), stride: stride, width: width, height: height}
	cairoDestroy(cr)
	cairoSurfaceDestroy(surface)
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
		return core.View{Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240), Padding: driver.All(10),
			Background: driver.RGB(0xffffff), FontSize: 16, Color: driver.RGB(0x000000)},
			Children: []core.Node{core.Text{Spans: []core.Span{
				{Content: "text "},
				{Content: "x      x", Style: core.Style{Background: driver.RGB(0xff0000), CornerRadius: 4}},
				{Content: " and "},
				{Style: core.Style{VerticalAlign: driver.VerticalAlignMiddle},
					Attachment: core.View{TestID: "button", OnPress: func(core.PressEvent) { pressed.Update(func(n int) int { return n + 1 }) },
						Style: core.Style{Width: driver.Pt(40), Height: driver.Pt(30), Background: driver.RGB(0x0000ff)}}},
				{Content: " after"},
			}}},
		}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	waitForTitle(t, w, title+"|0")

	var button driver.Bounds
	var bg rect
	var textAt driver.Point
	w.onLoop(t, func() {
		for _, v := range w.views {
			if v == nil || v.kind != driver.KindText {
				continue
			}
			b, _ := w.surfaceBounds(v)
			textAt = driver.Point{X: b.Origin.X + v.content.Origin.X, Y: b.Origin.Y + v.content.Origin.Y}
			if l, ok := v.text.(*textLayout); ok && len(l.under) > 0 {
				bg = l.under[0]
			}
			for _, c := range v.children {
				button, _ = w.surfaceBounds(c)
			}
		}
	})
	if button.Size != (driver.Size{Width: 40, Height: 30}) {
		t.Fatalf("attachment frame = %v, want a 40x30 child of the text", button)
	}
	if bg.w == 0 || button.Origin.X < textAt.X+driver.Points(bg.x+bg.w) {
		t.Fatalf("background %+v, attachment %v: want the attachment after the span", bg, button)
	}

	waitPixels(t, w, "a red span background and a blue attachment", func(p pixels) bool {
		span := p.at(int(textAt.X)+int(bg.x+bg.w/2), int(textAt.Y)+int(bg.y+bg.h/2))
		attached := p.at(int(button.Origin.X+20), int(button.Origin.Y+15))
		return span[2] == 0xff && span[1] == 0 && span[0] == 0 && attached[0] == 0xff && attached[1] == 0 && attached[2] == 0
	})

	// The attachment takes its own presses.
	w.onLoop(t, func() { w.press(driver.Point{X: button.Origin.X + 20, Y: button.Origin.Y + 15}) })
	waitForTitle(t, w, title+"|1")
}

// TestWebViewTalksToThePage opens a window with a web view showing inline HTML, receives the message the page posts and posts one back, which the page answers. It skips where WebKitGTK 6.0 is missing. WebKit keeps the GTK thread that creates its first view, so this test runs in its own process, as the other WebKit tests do.
func TestWebViewTalksToThePage(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("NSGO_TEST_WEBKIT_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		dir := t.TempDir()
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBKIT_PROCESS="+t.Name(),
			"XDG_DATA_HOME="+filepath.Join(dir, "data"), "XDG_CACHE_HOME="+filepath.Join(dir, "cache"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return
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
		return core.View{Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240), Padding: driver.All(10)},
			Children: []core.Node{core.WebView{Source: core.WebHTML(page, ""), Ref: &ref,
				Style:   core.Style{Width: driver.Pt(300), Height: driver.Pt(200)},
				OnLoad:  func(driver.WebNavigation) { loaded.Update(func(n int) int { return n + 1 }) },
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
		driver.Bounds{Origin: driver.Point{X: 10, Y: 10}, Size: driver.Size{Width: 300, Height: 200}})
}

// TestWebMessagesComeOnlyFromTheSourcesPage loads a page with a frame of another origin, which posts to WebKit's handler itself and relays a message through the page, and which then goes to a page of the frame's origin. A web view reaches the app only from its source's page by default, one taking any origin from every page it shows, and neither from the frame. It runs in a process of its own, as TestWebViewsOutliveEachOther does.
func TestWebMessagesComeOnlyFromTheSourcesPage(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("NSGO_TEST_WEBKIT_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		dir := t.TempDir()
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBKIT_PROCESS="+t.Name(),
			"XDG_DATA_HOME="+filepath.Join(dir, "data"), "XDG_CACHE_HOME="+filepath.Join(dir, "cache"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return
	}
	if err := openWebKit(); err != nil {
		t.Skip(err)
	}
	page, other := webOriginServers(t)
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		own, anyOrigin := core.NewSignal(cx, ""), core.NewSignal(cx, "")
		cx.Effect(func() { cx.SetTitle(title + "|" + own.Get() + "|" + anyOrigin.Get()) })
		heard := func(s *core.Signal[string]) func(core.WebMessage) {
			return func(m core.WebMessage) { s.Update(func(v string) string { return v + m.Data + "@" + m.Origin + ";" }) }
		}
		frame := core.Style{Width: driver.Pt(150), Height: driver.Pt(100)}
		return core.View{Style: core.Style{Width: driver.Pt(320), Height: driver.Pt(120), Direction: core.Row}, Children: []core.Node{
			core.WebView{Source: core.WebSrc(page.URL + "/"), Style: frame, OnMessageFrom: heard(own),
				OnLoad: func(n core.WebNavigation) {
					if strings.HasSuffix(n.URL, "/next") {
						own.Update(func(v string) string { return v + "loaded" })
					}
				}},
			core.WebView{Source: core.WebSrc(page.URL + "/"), Style: frame, MessageOrigins: []string{"*"}, OnMessageFrom: heard(anyOrigin)},
		}}
	}
	w := startTestWindow(t, app, Options{ID: "nsgo-test-web-origins", Title: title, Width: 320, Height: 120})
	from, to := page.URL, other.URL
	waitForTitle(t, w, title+"|main@"+from+";relayed@"+from+";loaded|main@"+from+";relayed@"+from+";other@"+to+";")
}

// webOriginServers serves the pages of TestWebMessagesComeOnlyFromTheSourcesPage from two origins: page's posts, holds a frame of other's and goes to other's once the frame relayed a message through it.
func webOriginServers(t *testing.T) (page, other *httptest.Server) {
	other = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/frame":
			fmt.Fprint(rw, `<!doctype html><script>
try { window.webkit.messageHandlers.nsgo.postMessage('frame'); } catch (e) {}
try { window.nsgo.postMessage('frame'); } catch (e) {}
parent.postMessage('relay', '*');
</script>`)
		case "/next":
			fmt.Fprint(rw, `<!doctype html><script>window.nsgo.postMessage('other');</script>`)
		}
	}))
	t.Cleanup(other.Close)
	page = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(rw, `<!doctype html><script>
addEventListener('message', (e) => {
  if (e.data !== 'relay') return;
  window.nsgo.postMessage('relayed');
  location.href = '%[1]s/next';
});
window.nsgo.postMessage('main');
</script><iframe src="%[1]s/frame"></iframe>`, other.URL)
	}))
	t.Cleanup(page.Close)
	return page, other
}

// webSchemesAccess lets a web view reach every host and none of the pages a page makes itself.
type webSchemesAccess struct{}

func (webSchemesAccess) Allows(core.AccessRequest) bool { return true }
func (webSchemesAccess) WebHosts() []string             { return []string{"*"} }
func (webSchemesAccess) WebSchemes() []string           { return nil }

// TestWebOwnPagesReloadAndReturn keeps the app's HTML and data pages exempt from page schemes when reloading and traversing history.
func TestWebOwnPagesReloadAndReturn(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("NSGO_TEST_WEBKIT_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		dir := t.TempDir()
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBKIT_PROCESS="+t.Name(),
			"XDG_DATA_HOME="+filepath.Join(dir, "data"), "XDG_CACHE_HOME="+filepath.Join(dir, "cache"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return
	}
	if err := openWebKit(); err != nil {
		t.Skip(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "text/html")
		fmt.Fprint(rw, "<p>next</p>")
	}))
	defer server.Close()
	title := "nsgo-native-" + t.Name()
	var root *core.Scope
	var refs [2]core.Ref
	errors := make(chan error, 8)
	app := func(cx *core.Scope) core.Node {
		root = cx
		cx.ProvideAccess(webSchemesAccess{})
		loads, errs := core.NewSignal(cx, [2]int{}), core.NewSignal(cx, "")
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%v|%s", title, loads.Get(), errs.Get())) })
		var views []core.Node
		for i, src := range []core.WebSource{core.WebHTML("<p>own</p>", ""), core.WebSrc("data:text/html,<p>own</p>")} {
			views = append(views, core.WebView{Source: src, Ref: &refs[i],
				Style: core.Style{Width: core.Pt(150), Height: core.Pt(100)},
				OnLoadStart: func(n core.WebNavigation) {
					phase := loads.Get()[i] + 1
					want := src.Src
					if want == "" {
						want = "about:blank"
					}
					if phase == 3 || phase == 6 {
						want = server.URL + "/next"
					}
					if n.URL != want {
						t.Errorf("view %d phase %d start URL = %q, want %q", i, phase, n.URL, want)
					}
				},
				OnLoad: func(n core.WebNavigation) {
					phase := loads.Get()[i] + 1
					if (phase == 3 || phase == 6) && !n.CanGoBack {
						t.Errorf("view %d phase %d cannot go back", i, phase)
					}
					if (phase == 4 || phase == 5) && !n.CanGoForward {
						t.Errorf("view %d cannot go forward after going back", i)
					}
					refs[i].Evaluate("document.body.textContent.trim()", func(body string, err error) {
						want := "\"own\""
						if phase == 3 || phase == 6 {
							want = "\"next\""
						}
						if err != nil || body != want {
							t.Errorf("view %d phase %d body = %s, %v; want %s", i, phase, body, err, want)
						}
						loads.Update(func(v [2]int) [2]int { v[i]++; return v })
					})
				},
				OnError: func(err error) { errors <- err; errs.Update(func(s string) string { return s + err.Error() + ";" }) },
			})
		}
		return core.View{Style: core.Style{Direction: core.Row}, Children: views}
	}
	w := startTestWindow(t, app, Options{ID: "nsgo-test-web-own-history", Title: title, Width: 300, Height: 100})
	wait := func(n int) { waitForTitle(t, w, fmt.Sprintf("%s|[%d %d]|", title, n, n)) }
	command := func(fn func(*core.Ref)) {
		root.Post(func() {
			for i := range refs {
				fn(&refs[i])
			}
		})
	}
	wait(1)
	command(func(r *core.Ref) { r.Reload() })
	wait(2)
	command(func(r *core.Ref) { r.InjectJavaScript(fmt.Sprintf("location.href = %q", server.URL+"/next")) })
	wait(3)
	command(func(r *core.Ref) { r.GoBack() })
	wait(4)
	command(func(r *core.Ref) { r.Reload() })
	wait(5)
	command(func(r *core.Ref) { r.GoForward() })
	wait(6)
	command(func(r *core.Ref) { r.GoBack() })
	wait(7)
	// A subframe reload cannot prime permission for the top level's next page-made about: navigation, even from an app-owned about:/data: page.
	command(func(r *core.Ref) {
		r.InjectJavaScript(`{
const f = document.createElement('iframe');
let reloaded = false;
f.onload = () => {
  if (!reloaded) { reloaded = true; f.contentWindow.location.reload(); return; }
  location.href = 'about:blank';
};
f.src = 'about:blank';
document.body.append(f);
}`)
	})
	for range refs {
		select {
		case err := <-errors:
			if e, ok := err.(*core.WebError); !ok || e.URL != "about:blank" {
				t.Errorf("page navigation error = %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the page-made about: navigation was not refused")
		}
	}
}

// TestWebHistoryStepWithinThePageGrantsNothing goes back from a pushState entry to the app's own page, a step within one document that starts no navigation, so the exemption the step was given must not pass to the page's next navigation, to an about: page the access refuses.
func TestWebHistoryStepWithinThePageGrantsNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("NSGO_TEST_WEBKIT_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		dir := t.TempDir()
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBKIT_PROCESS="+t.Name(),
			"XDG_DATA_HOME="+filepath.Join(dir, "data"), "XDG_CACHE_HOME="+filepath.Join(dir, "cache"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return
	}
	if err := openWebKit(); err != nil {
		t.Skip(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "text/html")
		fmt.Fprint(rw, "<p>own</p>")
	}))
	defer server.Close()
	title := "nsgo-native-" + t.Name()
	var root *core.Scope
	var ref core.Ref
	errors := make(chan error, 4)
	app := func(cx *core.Scope) core.Node {
		root = cx
		cx.ProvideAccess(webSchemesAccess{})
		loads := core.NewSignal(cx, 0)
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%d", title, loads.Get())) })
		return core.WebView{Source: core.WebSrc(server.URL + "/own"), Ref: &ref,
			Style: core.Style{Width: core.Pt(150), Height: core.Pt(100)},
			OnLoad: func(n core.WebNavigation) {
				// Refusing about:blank may reload the original page when WebKit cannot restore it from its history cache.
				if n.URL != server.URL+"/own" {
					t.Errorf("the page loaded %s", n.URL)
				}
				if loads.Get() == 0 {
					loads.Set(1)
				}
			},
			OnError: func(err error) { errors <- err },
		}
	}
	w := startTestWindow(t, app, Options{ID: "nsgo-test-web-history-step", Title: title, Width: 150, Height: 100})
	waitForTitle(t, w, fmt.Sprintf("%s|1", title))
	root.Post(func() {
		ref.Evaluate(`addEventListener('popstate', () => { location.href = 'about:blank' }); history.pushState(null, '', '#step'); 1`, func(_ string, err error) {
			if err != nil {
				t.Errorf("pushState: %v", err)
			}
			ref.GoBack()
		})
	})
	select {
	case err := <-errors:
		if e, ok := err.(*core.WebError); !ok || e.URL != "about:blank" {
			t.Errorf("page navigation error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the page's about: navigation was not refused")
	}
}

// TestWebPagesAboutPageIsRefusedAtTheTopLevel has the app's page, under an Access whose WebSchemes allow none, send itself to about:blank once its srcdoc frame has loaded. WebKitGTK shows an about: page as soon as its navigation action is used and says no action's frame, so the view refuses it as it commits: the about: page posts nothing and reports no load, the refusal reaches OnError once, and the app's page loads again with its frame. It skips where WebKitGTK 6.0 is missing, in a process of its own as WebKit takes the first thread that uses it.
func TestWebPagesAboutPageIsRefusedAtTheTopLevel(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("NSGO_TEST_WEBKIT_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		dir := t.TempDir()
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBKIT_PROCESS="+t.Name(),
			"XDG_DATA_HOME="+filepath.Join(dir, "data"), "XDG_CACHE_HOME="+filepath.Join(dir, "cache"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return
	}
	if err := openWebKit(); err != nil {
		t.Skip(err)
	}
	const page = `<!doctype html><iframe id="f" srcdoc="<p>frame</p>"></iframe><script>
onload = () => {
  const frame = document.getElementById('f').contentDocument.body.innerHTML;
  if (sessionStorage.left) return window.nsgo.postMessage('again ' + frame);
  sessionStorage.left = '1';
  window.nsgo.postMessage('first ' + frame);
  location.href = 'about:blank';
};
</script>`
	// /next answers only once the test has seen its start, so a start held until the page commits fails the test rather than arriving late.
	release := make(chan struct{})
	next := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/next" {
			select {
			case <-release:
			case <-time.After(20 * time.Second):
			}
		}
		if r.URL.Path == "/fail" {
			conn, _, err := rw.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close() // no response: the provisional load fails before commit
			}
			return
		}
		rw.Header().Set("Content-Type", "text/html")
		fmt.Fprint(rw, "<p>next</p>")
	}))
	defer next.Close()
	title := "nsgo-native-" + t.Name()
	var root *core.Scope
	var ref core.Ref
	app := func(cx *core.Scope) core.Node {
		root = cx
		heard, errs, loads := core.NewSignal(cx, ""), core.NewSignal(cx, ""), core.NewSignal(cx, "")
		starts, states, own := core.NewSignal(cx, ""), core.NewSignal(cx, ""), core.NewSignal(cx, "")
		cx.Effect(func() {
			cx.SetTitle(title + "|" + heard.Get() + "|" + errs.Get() + "|" + loads.Get() + "|" + starts.Get() + "|" + states.Get() + "|" + own.Get())
		})
		cx.ProvideAccess(webSchemesAccess{})
		add := func(s *core.Signal[string], text string) { s.Update(func(v string) string { return v + text + ";" }) }
		return core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
			core.WebView{Source: core.WebHTML(page, "https://localhost/"), Ref: &ref, MessageOrigins: []string{"*"},
				InjectedJavaScript: "if (location.protocol === 'about:') window.nsgo.postMessage('about');",
				Style:              core.Style{Width: driver.Pt(200), Height: driver.Pt(120)},
				OnMessage:          func(data string) { add(heard, data) },
				OnError: func(err error) {
					if e, ok := err.(*core.WebError); ok {
						add(errs, e.URL)
					}
				},
				OnLoad:                  func(n core.WebNavigation) { add(loads, n.URL) },
				OnLoadStart:             func(n core.WebNavigation) { add(starts, n.URL) },
				OnNavigationStateChange: func(n core.WebNavigation) { add(states, fmt.Sprintf("%s %t", n.URL, n.Loading)) }},
			// The app's own baseless HTML must still report its about:blank load.
			core.WebView{Source: core.WebHTML("<p>own</p>", ""),
				Style:                   core.Style{Width: driver.Pt(100), Height: driver.Pt(120)},
				OnLoadStart:             func(n core.WebNavigation) { add(own, "start "+n.URL) },
				OnLoad:                  func(n core.WebNavigation) { add(own, "load "+n.URL) },
				OnNavigationStateChange: func(n core.WebNavigation) { add(own, fmt.Sprintf("state %s %t", n.URL, n.Loading)) },
				OnError:                 func(err error) { add(own, "error "+err.Error()) }},
		}}
	}
	w := startTestWindow(t, app, Options{ID: "nsgo-test-web-schemes", Title: title, Width: 200, Height: 120})
	waitForTitle(t, w, title+"|first <p>frame</p>;again <p>frame</p>;|about:blank;|https://localhost/;https://localhost/;"+
		"|https://localhost/;https://localhost/;|https://localhost/ true;https://localhost/ false;https://localhost/ true;https://localhost/ false;"+
		"|start about:blank;state about:blank true;load about:blank;state about:blank false;")
	// A frame's about: action followed by the page's own top-level navigation must not hold that navigation's start: both start and loading state name the destination before the response, and the load follows in order.
	q := regexp.QuoteMeta
	head := q(title + "|first <p>frame</p>;again <p>frame</p>;|about:blank;")
	own := q("|start about:blank;state about:blank true;load about:blank;state about:blank false;")
	waitMatch := func(pattern string) {
		t.Helper()
		re := regexp.MustCompile("^" + pattern + "$")
		waitTitle(t, w, re.MatchString)
	}
	frameThen := func(url string) {
		root.Post(func() {
			ref.InjectJavaScript(fmt.Sprintf(`{
const f = document.createElement('iframe');
f.src = 'about:blank';
f.onload = () => { location.href = %q; };
document.body.append(f);
}`, url))
		})
	}
	frameThen(next.URL + "/next")
	waitMatch(head + q("|https://localhost/;https://localhost/;") +
		q("|https://localhost/;https://localhost/;"+next.URL+"/next;") +
		q("|https://localhost/ true;https://localhost/ false;https://localhost/ true;https://localhost/ false;") + q(next.URL+"/next true;") + own)
	close(release)
	waitMatch(head + q("|https://localhost/;https://localhost/;"+next.URL+"/next;") +
		q("|https://localhost/;https://localhost/;"+next.URL+"/next;") +
		q("|https://localhost/ true;https://localhost/ false;https://localhost/ true;https://localhost/ false;") + q(next.URL+"/next true;") + q(next.URL+"/next false;") + own)
	// Nor must it swallow the start of a load that fails before commit.
	frameThen(next.URL + "/fail")
	waitMatch(q(title+"|first <p>frame</p>;again <p>frame</p>;|about:blank;"+next.URL+"/fail;") + q("|https://localhost/;https://localhost/;"+next.URL+"/next;") +
		q("|https://localhost/;https://localhost/;"+next.URL+"/next;"+next.URL+"/fail;") +
		q("|https://localhost/ true;https://localhost/ false;https://localhost/ true;https://localhost/ false;") + q(next.URL+"/next true;") + q(next.URL+"/next false;") + q(next.URL+"/fail true;") + own)
}

// TestWebPageNavigationReportsDestinationAtStart has a restricted and an unrestricted view's page navigate with location.href and no frame's action before it, and checks that the start and the loading state name the destination while its response is still held, that the app's reload and history commands keep their start URLs, that a restricted view's refused blob: page reports no start, and that a provisional failure after a step within the page reports its own URL. It skips where WebKitGTK 6.0 is missing, in a process of its own as WebKit takes the first thread that uses it.
func TestWebPageNavigationReportsDestinationAtStart(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("NSGO_TEST_WEBKIT_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		dir := t.TempDir()
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBKIT_PROCESS="+t.Name(),
			"XDG_DATA_HOME="+filepath.Join(dir, "data"), "XDG_CACHE_HOME="+filepath.Join(dir, "cache"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return
	}
	if err := openWebKit(); err != nil {
		t.Skip(err)
	}
	for _, restricted := range []bool{false, true} {
		t.Run(fmt.Sprintf("restricted=%t", restricted), func(t *testing.T) {
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/next" {
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				}
				if r.URL.Path == "/fail" {
					conn, _, err := rw.(http.Hijacker).Hijack()
					if err == nil {
						conn.Close()
					}
					return
				}
				rw.Header().Set("Content-Type", "text/html")
				fmt.Fprint(rw, "<p>page</p>")
			}))
			t.Cleanup(server.Close)
			t.Cleanup(unblock)
			source, next, failed := server.URL+"/source", server.URL+"/next", server.URL+"/fail"
			title := fmt.Sprintf("nsgo-native-navigation-%t", restricted)
			var root *core.Scope
			var ref core.Ref
			var steppingWithinPage bool
			app := func(cx *core.Scope) core.Node {
				root = cx
				if restricted {
					cx.ProvideAccess(webSchemesAccess{})
				}
				events := core.NewSignal(cx, "")
				add := func(event string) {
					// A blob: URL is unique to its page, so it is written by its scheme alone.
					if i := strings.Index(event, "blob:"); i >= 0 {
						rest := ""
						if j := strings.IndexByte(event[i:], ' '); j >= 0 {
							rest = event[i+j:]
						}
						event = event[:i] + "blob" + rest
					}
					events.Update(func(v string) string { return v + event + ";" })
				}
				cx.Effect(func() { cx.SetTitle(title + "|" + events.Get()) })
				return core.WebView{Source: core.WebSrc(source), Ref: &ref,
					Style:       core.Style{Width: core.Pt(200), Height: core.Pt(120)},
					OnLoadStart: func(n core.WebNavigation) { add("start " + n.URL) },
					OnNavigationStateChange: func(n core.WebNavigation) {
						if steppingWithinPage && !n.Loading && (n.URL == source || n.URL == source+"#step") {
							return
						}
						add(fmt.Sprintf("state %s %t", n.URL, n.Loading))
					},
					OnLoad: func(n core.WebNavigation) { add("load " + n.URL) },
					OnError: func(err error) {
						if e, ok := err.(*core.WebError); ok {
							add("error " + e.URL)
						} else {
							add("error " + err.Error())
						}
					},
				}
			}
			w := startTestWindow(t, app, Options{ID: title, Title: title, Width: 200, Height: 120})
			start := func(url string) string { return "start " + url + ";state " + url + " true;" }
			load := func(url string) string { return "load " + url + ";state " + url + " false;" }
			want := title + "|" + start(source) + load(source)
			waitForTitle(t, w, want)
			// No frame's navigation action comes first: location.href alone must name the destination while its response is still held.
			root.Post(func() { ref.InjectJavaScript(fmt.Sprintf("location.href = %q", next)) })
			want += start(next)
			waitForTitle(t, w, want)
			unblock()
			want += load(next)
			waitForTitle(t, w, want)
			for _, step := range []struct {
				command func()
				url     string
			}{{ref.Reload, next}, {ref.GoBack, source}, {ref.GoForward, next}, {ref.GoBack, source}} {
				root.Post(step.command)
				want += start(step.url) + load(step.url)
				waitForTitle(t, w, want)
			}
			if restricted {
				// The access refuses pages a page makes itself, which WebKitGTK tells apart only at the main-frame response: the refusal must not be preceded by a start for a page that never loads.
				root.Post(func() {
					ref.InjectJavaScript(`location.href = URL.createObjectURL(new Blob(['<p>blob</p>'], {type: 'text/html'}))`)
				})
				want += "error blob;"
				waitForTitle(t, w, want)
			}
			// The app's history step within its own document starts no load. Its grant must not make the page's next load report its start at WEBKIT_LOAD_STARTED, with the URI of the page still shown, even in an unrestricted view.
			root.Post(func() {
				// A same-document update can be combined with the following load's state. This test checks the full-load events and failing URL.
				steppingWithinPage = true
				ref.Evaluate(fmt.Sprintf(`addEventListener('popstate', () => { location.href = %q }); history.pushState(null, '', '#step'); 1`, failed), func(_ string, err error) {
					if err != nil {
						t.Errorf("pushState: %v", err)
						return
					}
					ref.GoBack()
				})
			})
			waitForTitle(t, w, want+start(failed)+"error "+failed+";")
		})
	}
}

// TestWebViewsOutliveEachOther opens an Incognito web view and one in the app's session, both serving an app-owned scheme, and one that serves none. Each page keeps its scheme and downloads as the others unmount and mount again, the page without a handler gets an error rather than a load that never ends, and a view that unmounts mid-download cancels it. It skips where WebKitGTK 6.0 is missing. It runs in a process of its own, as the other WebKit tests do.
func TestWebViewsOutliveEachOther(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("NSGO_TEST_WEBKIT_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		// The app's website data and downloads go to a directory of the test's.
		dir := t.TempDir()
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBKIT_PROCESS="+t.Name(),
			"XDG_DATA_HOME="+filepath.Join(dir, "data"), "XDG_CACHE_HOME="+filepath.Join(dir, "cache"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return
	}
	if err := openWebKit(); err != nil {
		t.Skip(err)
	}
	// B's page downloads a file as it loads, which its view saves. WebKit's network process cannot fetch app-owned schemes, so the file is a data: URL.
	const page = `<!doctype html><html><body><script>
		window.nsgo.postMessage(location.host);
		if (location.host === 'b' && location.pathname === '/') {
			const a = document.createElement('a');
			a.href = 'data:text/plain,saved';
			a.download = 'saved.txt';
			document.body.append(a);
			a.click();
		}
	</script></body></html>`
	// D's page downloads a file that never ends.
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/slow" {
			rw.Header().Set("Content-Type", "text/html")
			fmt.Fprint(rw, `<a id="a" href="/slow" download="slow.bin"></a><script>document.getElementById('a').click()</script>`)
			return
		}
		rw.Header().Set("Content-Type", "application/octet-stream")
		rw.Header().Set("Content-Length", "1048576")
		rw.Write(make([]byte, 1024))
		rw.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-time.After(20 * time.Second):
		}
	}))
	defer server.Close()
	title := "nsgo-native-" + t.Name()
	var root *core.Scope
	var showA, showB, showD *core.Signal[bool]
	app := func(cx *core.Scope) core.Node {
		root = cx
		showA, showB, showD = core.NewSignal(cx, true), core.NewSignal(cx, true), core.NewSignal(cx, false)
		started := core.NewSignal(cx, 0)
		a, b := core.NewSignal(cx, 0), core.NewSignal(cx, 0)
		failed := core.NewSignal(cx, false)
		saved := core.NewSignal(cx, 0)
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|a%d|b%d|%t|d%d|s%d", title, a.Get(), b.Get(), failed.Get(), saved.Get(), started.Get()))
		})
		frame := core.Style{Width: driver.Pt(100), Height: driver.Pt(100)}
		served := func(name string, incognito bool, count *core.Signal[int]) core.Node {
			return core.Func(func(cx *core.Scope) core.Node {
				if err := cx.ServeScheme("app", func(string) []byte { return []byte(page) }); err != nil {
					t.Error(err)
				}
				web := core.WebView{Source: core.WebSrc("app://" + name + "/"), Incognito: incognito, Style: frame,
					OnMessage: func(data string) {
						if data == name {
							count.Update(func(n int) int { return n + 1 })
						}
					}}
				if name == "b" {
					web.OnDownload = func(d core.WebDownload) {
						if d.State != core.WebDownloadFinished {
							return
						}
						if body, err := os.ReadFile(d.Path); err != nil || string(body) != "saved" || d.Filename != "saved.txt" {
							t.Errorf("download %+v saved %q, %v", d, body, err)
						}
						saved.Update(func(n int) int { return n + 1 })
					}
				}
				return web
			})
		}
		return core.View{Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240), Direction: core.Row}, Children: []core.Node{
			core.Show{When: showA.Get, Then: func(*core.Scope) core.Node { return served("a", true, a) }},
			core.Show{When: showB.Get, Then: func(*core.Scope) core.Node { return served("b", false, b) }},
			core.WebView{Source: core.WebSrc("app://c/"), Style: frame, OnError: func(error) { failed.Set(true) }},
			core.Show{When: showD.Get, Then: func(*core.Scope) core.Node {
				return core.WebView{Source: core.WebSrc(server.URL + "/"), Style: frame, OnDownload: func(d core.WebDownload) {
					if d.State == core.WebDownloadStarted {
						started.Update(func(n int) int { return n + 1 })
					}
				}}
			}},
		}}
	}
	w := startTestWindow(t, app, Options{ID: "nsgo-test-webviews", Title: title, Width: 360, Height: 240})
	waitForTitle(t, w, title+"|a1|b1|true|d1|s0")

	// The app's session outlives its view: B mounts again into it, and A still serves its scheme.
	root.Post(func() { showB.Set(false) })
	waitTitle(t, w, func(s string) bool { return webPagesOf(t, w) == 2 })
	root.Post(func() { showB.Set(true) })
	waitForTitle(t, w, title+"|a1|b2|true|d2|s0")

	// An Incognito view's own session goes with it, and the view mounts again with a new one, while B keeps serving.
	root.Post(func() { showA.Set(false) })
	waitTitle(t, w, func(s string) bool { return webPagesOf(t, w) == 2 })
	root.Post(func() { showA.Set(true) })
	waitForTitle(t, w, title+"|a2|b2|true|d2|s0")

	// A view that unmounts cancels the download it was saving.
	root.Post(func() { showD.Set(true) })
	waitForTitle(t, w, title+"|a2|b2|true|d2|s1")
	root.Post(func() { showD.Set(false) })
	select {
	case <-cancelled:
	case <-time.After(10 * time.Second):
		t.Fatal("the download went on after its view unmounted")
	}
	// D's page is gone; the others stay.
	waitTitle(t, w, func(s string) bool { return webPagesOf(t, w) == 3 })
}

// webPagesOf returns how many pages w shows.
func webPagesOf(t *testing.T, w *window) int {
	t.Helper()
	n := 0
	w.onLoop(t, func() {
		for _, v := range w.views {
			if v != nil && !v.deleted && v.web != nil && v.web.widget != 0 {
				n++
			}
		}
	})
	return n
}

// waitHosted waits until the native widget of the view that is matches
// is a child of the view's widget, mapped at want in the surface.
func waitHosted(t *testing.T, w *window, is func(*view) bool, want driver.Bounds) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var got driver.Bounds
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
					got = driver.Bounds{Origin: driver.Point{X: driver.Points(r.x), Y: driver.Points(r.y)},
						Size: driver.Size{Width: driver.Points(r.w), Height: driver.Points(r.h)}}
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

// gstreamerNeverPrerolled marks a failure in which GStreamer neither prepared the stream nor reported an error, before NSGo had anything to do.
const gstreamerNeverPrerolled = "gstreamer: the stream was never prepared and reported no error"

// TestVideoReportsLoadProgressAndEnd plays a short sound through a Video and reads its events. At the end, unrelated configuration stays stopped; an explicit replay or seek starts it again.
func TestVideoReportsLoadProgressAndEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	// GTK and GStreamer keep state beyond a window's lifetime. Exercise this first pipeline independently of the other native tests, as the WebKit regressions do, while retaining every playback assertion.
	if os.Getenv("NSGO_TEST_GSTREAMER_PROCESS") != t.Name() {
		// Now and then in CI GStreamer never prerolls the file: GstPlay stays in READY with no error, so GTK never prepares the stream and NSGo is never asked anything (#727). A PulseAudio daemon that does not answer, stopped with SIGSTOP, fails the test exactly so, as pulsesink waits for it; CI's daemon stalls now and then. A run that fails that way, and only that way, gets one more process; any other failure, or the same one twice, fails the test.
		var failures []string
		for attempt := 1; ; attempt++ {
			dots := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
			// GstPlay writes the pipeline at each of its state changes there, so a failure names the last one it reached.
			cmd.Env = append(os.Environ(), "NSGO_TEST_GSTREAMER_PROCESS="+t.Name(), "GST_DEBUG_DUMP_DOT_DIR="+dots)
			out, err := cmd.CombinedOutput()
			if err == nil {
				if strings.Contains(string(out), "--- SKIP") {
					t.Skipf("%s", out)
				}
				if len(failures) > 0 {
					t.Logf("passed on attempt %d after GStreamer never prerolled:\n%s", attempt, strings.Join(failures, "\n"))
				}
				return
			}
			var states []string
			if entries, _ := os.ReadDir(dots); len(entries) > 0 {
				for _, e := range entries {
					states = append(states, e.Name())
				}
			}
			failures = append(failures, fmt.Sprintf("attempt %d: %v\n%s\nGstPlay state changes: %q", attempt, err, out, states))
			if attempt == 2 || !strings.Contains(string(out), gstreamerNeverPrerolled) {
				t.Fatal(strings.Join(failures, "\n"))
			}
		}
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
		return core.View{Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240)}, Children: []core.Node{
			core.Video{Source: path, ProgressInterval: 50 * time.Millisecond,
				Style:   core.Style{Width: driver.Pt(200), Height: driver.Pt(100)},
				OnLoad:  func(i core.VideoInfo) { state.Set(fmt.Sprintf("load %v", i.Duration.Round(100*time.Millisecond))) },
				OnError: func(err error) { state.Set("error " + err.Error()) },
				OnProgress: func(time.Duration, time.Duration) {
					progress.Update(func(n int) int { return n + 1 })
				},
				OnEnd: func() { ended.Update(func(n int) int { return n + 1 }) }},
		}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	// A timeout may precede every media callback. Keep the native state in the failure report so CI distinguishes that from a missed end.
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		w.onLoop(t, func() {
			for _, v := range w.views {
				if v == nil || v.video == nil || v.video.stream == 0 {
					continue
				}
				s := v.video.stream
				t.Logf("video: loaded=%t failed=%t requested=%t prepared=%d playing=%d ended=%d",
					v.video.loaded, v.video.failed, v.video.playing, gtkMediaStreamIsPrepared(s), gtkMediaStreamGetPlaying(s), gtkMediaStreamGetEnded(s))
				msg := "none"
				if e := gtkMediaStreamGetError(s); e != 0 {
					msg = goString((*gError)(cPointer(e)).message)
				} else if gtkMediaStreamIsPrepared(s) == 0 {
					t.Log(gstreamerNeverPrerolled)
				}
				t.Logf("media: audio=%d video=%d duration=%d time=%d error=%s",
					gtkMediaStreamHasAudio(s), gtkMediaStreamHasVideo(s), gtkMediaStreamGetDuration(s), gtkMediaStreamGetTimestamp(s), msg)
			}
		})
	})
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
	configure := func(change func(*driver.VideoConfig)) {
		t.Helper()
		found := false
		w.onLoop(t, func() {
			for _, v := range w.views {
				if v != nil && v.video != nil {
					c := v.video.config
					change(&c)
					w.configureVideo(v, &c)
					found = true
					break
				}
			}
		})
		if !found {
			t.Fatal("no video")
		}
	}
	for _, change := range []func(*driver.VideoConfig){
		func(c *driver.VideoConfig) { c.Volume = 0.3 },
		func(c *driver.VideoConfig) { c.Muted = true },
		func(c *driver.VideoConfig) { c.Rate = 1.25 },
		func(c *driver.VideoConfig) { c.ResizeMode = driver.Cover },
		func(c *driver.VideoConfig) { c.Controls = true },
		func(c *driver.VideoConfig) { c.ProgressInterval = 30 * time.Millisecond },
	} {
		configure(change)
		time.Sleep(150 * time.Millisecond)
		if after := w.titleOf(t); after != got {
			t.Fatalf("a finished video reported new events after configuration: %q -> %q", got, after)
		}
	}
	configure(func(c *driver.VideoConfig) { c.Paused = true })
	configure(func(c *driver.VideoConfig) { c.Paused = false })
	waitTitle(t, w, func(s string) bool { return strings.HasSuffix(s, "|2") })
	w.onLoop(t, func() {
		for _, v := range w.views {
			if v != nil && v.video != nil {
				v.video.seek(200 * time.Millisecond)
			}
		}
	})
	waitTitle(t, w, func(s string) bool { return strings.HasSuffix(s, "|3") })
	// GtkVideo's controls play a finished stream again by setting it playing, which the app does not see; a configuration that follows must not stop it.
	w.onLoop(t, func() {
		for _, v := range w.views {
			if v != nil && v.video != nil {
				gtkMediaStreamSetPlaying(v.video.stream, 1)
				c := v.video.config
				c.Volume = 0.6
				w.configureVideo(v, &c)
			}
		}
	})
	waitTitle(t, w, func(s string) bool { return strings.HasSuffix(s, "|4") })
}

func TestVideoChangesPlaybackRateWithoutReloading(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	path := filepath.Join(t.TempDir(), "video.webm")
	if err := os.WriteFile(path, videoRateClip, 0o600); err != nil {
		t.Fatal(err)
	}
	type sample struct {
		wall time.Time
		clip time.Duration
	}
	loads := make(chan core.VideoInfo, 8)
	positions := make(chan sample, 256)
	errs := make(chan error, 1)
	ends := make(chan struct{}, 1)
	var cx *core.Scope
	var rate *core.Signal[float32]
	var paused *core.Signal[bool]
	var ref core.Ref
	w := startTestWindow(t, func(scope *core.Scope) core.Node {
		cx = scope
		rate, paused = core.NewSignal(cx, float32(2)), core.NewSignal(cx, false)
		return core.Video{
			Source: path, Controls: true, Repeat: true, Muted: true, Ref: &ref,
			RateFunc: rate.Get, PausedFunc: paused.Get, ProgressInterval: 30 * time.Millisecond,
			Style:   core.Style{Width: driver.Pt(200), Height: driver.Pt(100)},
			OnLoad:  func(info core.VideoInfo) { loads <- info },
			OnError: func(err error) { errs <- err },
			OnEnd:   func() { ends <- struct{}{} },
			OnProgress: func(pos, _ time.Duration) {
				select {
				case positions <- sample{time.Now(), pos}:
				default:
				}
			},
		}
	}, Options{Title: "nsgo-native-video-rate", Width: 360, Height: 240})
	var duration time.Duration
	select {
	case info := <-loads:
		if !info.RateChangeable {
			if bindVideoGST() {
				t.Fatal("GstPlay loaded a seekable clip without rate support")
			}
			t.Skip("GstPlay or scaletempo is unavailable")
		}
		duration = info.Duration
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(8 * time.Second):
		t.Fatal("video never loaded")
	}
	next := func(after time.Time) sample {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for {
			select {
			case s := <-positions:
				if !s.wall.Before(after) && s.clip > 0 {
					return s
				}
			case err := <-errs:
				t.Fatal(err)
			case <-deadline:
				t.Fatal("no video progress")
			}
		}
	}
	// Progress reports come while GTK's stream is set playing, before GStreamer's pipeline has reached PLAYING after a load, a rate change or a seek: until then they repeat the position it prerolled at. On a busy CI runner the audio sink can take seconds to start, so measure only from the first report at which the position moves.
	moving := func(after time.Time) sample {
		t.Helper()
		prev := next(after)
		for deadline := prev.wall.Add(10 * time.Second); ; {
			s := next(after)
			if s.clip > prev.clip {
				return s
			}
			if s.wall.After(deadline) {
				t.Fatalf("video position stayed at %v for 10s", s.clip)
			}
			prev = s
		}
	}
	measure := func(want float64) sample {
		t.Helper()
		begin := moving(time.Now().Add(180 * time.Millisecond))
		end := next(begin.wall.Add(700 * time.Millisecond))
		got := float64(end.clip-begin.clip) / float64(end.wall.Sub(begin.wall))
		if got < want*0.72 || got > want*1.28 {
			t.Fatalf("position advanced %.2f× wall time, want %.1f× (%v to %v)", got, want, begin.clip, end.clip)
		}
		return end
	}
	initial := measure(2)
	cx.Post(func() { rate.Set(0.5) })
	if slow := measure(0.5); slow.clip < initial.clip {
		t.Fatalf("changing rate lost position: %v to %v", initial.clip, slow.clip)
	}
	cx.Post(func() { paused.Set(true) })
	w.engine.Sync()
	w.onLoop(t, func() {
		for _, v := range w.views {
			if v != nil && v.video != nil && gtkMediaStreamGetPlaying(v.video.stream) != 0 {
				t.Error("the video did not pause")
			}
		}
	})
	cx.Post(func() { ref.Seek(4 * time.Second); paused.Set(false) })
	if resumed := measure(0.5); resumed.clip < 4*time.Second {
		t.Fatalf("resuming after seek started at %v, want at least 4s", resumed.clip)
	}
	if duration < 7*time.Second {
		t.Fatalf("video duration = %v, want about 8s", duration)
	}
	cx.Post(func() { ref.Seek(duration - 350*time.Millisecond) })
	deadline := time.Now().Add(5 * time.Second)
	atEnd := false
	for {
		s := next(time.Time{})
		if atEnd && s.clip < time.Second {
			break
		}
		atEnd = atEnd || s.clip > duration-time.Second
		if time.Now().After(deadline) {
			t.Fatal("the movie did not repeat")
		}
	}
	measure(0.5)
	select {
	case info := <-loads:
		t.Fatalf("changing speed or playback reopened the source: %+v", info)
	case <-ends:
		t.Fatal("a repeating video reported an end")
	default:
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
		return core.Video{Source: path, Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100)},
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

// TestVideoShowsWhatIsBehindItsTransparentPixels plays a VP9 WebM with alpha, transparent around a blue square, over a red view: the red shows through around the square. Without GStreamer's codecalpha plug-in it reports an error with MediaErrorAlphaUnsupported instead, and the test skips.
func TestVideoShowsWhatIsBehindItsTransparentPixels(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	path := filepath.Join(t.TempDir(), "alpha.webm")
	if err := os.WriteFile(path, alphaclips.VP9Alpha, 0o644); err != nil {
		t.Fatal(err)
	}
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		state := core.NewSignal(cx, "")
		cx.Effect(func() { cx.SetTitle(title + "|" + state.Get()) })
		return core.View{Style: core.Style{Width: driver.Pt(100), Height: driver.Pt(100), Background: driver.RGB(0xff0000)},
			Children: []core.Node{core.Video{Source: path, Repeat: true, Muted: true,
				Style:  core.Style{Width: driver.Pt(100), Height: driver.Pt(100)},
				OnLoad: func(core.VideoInfo) { state.Set("load") },
				OnError: func(err error) {
					var reason core.MediaErrorReason
					if me, ok := err.(*core.MediaError); ok {
						reason = me.Reason
					}
					state.Set("error " + reason.String() + " " + err.Error())
				}}},
		}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	got := waitTitle(t, w, func(s string) bool { return strings.Contains(s, "|error ") || strings.HasSuffix(s, "|load") })
	if strings.Contains(got, "|error ") {
		if gstFeature("vp9alphadecodebin") {
			t.Fatalf("title = %q, but GStreamer has vp9alphadecodebin", got)
		}
		if !strings.Contains(got, "|error alpha-unsupported ") || !strings.Contains(got, "codecalpha") {
			t.Fatalf("title = %q, want the alpha refused and the missing plug-in named", got)
		}
		t.Skip(got)
	}
	// The clip's square covers its middle half: 25 to 75 points here.
	red := func(p []byte) bool { return p[2] > 200 && p[1] < 60 && p[0] < 60 && p[3] == 255 }
	blue := func(p []byte) bool { return p[0] > 200 && p[1] < 60 && p[2] < 60 && p[3] == 255 }
	var last pixels
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		last = w.surfacePixels(t)
		if red(last.at(8, 8)) && red(last.at(92, 92)) && blue(last.at(50, 50)) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("want red around a blue square, have %v and %v at its corners and %v in its middle (BGRA) in %dx%d", last.at(8, 8), last.at(92, 92), last.at(50, 50), last.width, last.height)
		}
	}
}

// TestWebViewWithoutWebKitShowsAPlaceholder shows a web view where WebKitGTK 6.0 cannot be loaded, as NSH's Linux backend built without it does. It runs in a process of its own in which WebKit counts as missing, so it runs where WebKit is installed too and never loads it into the test process.
func TestWebViewWithoutWebKitShowsAPlaceholder(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("NSGO_TEST_WEBKIT_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBKIT_PROCESS="+t.Name())
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return
	}
	webkitOnce.Do(func() { webkitErr = fmt.Errorf("WebKitGTK 6.0 is hidden from %s", t.Name()) })
	if openWebKit() == nil {
		t.Fatal("WebKitGTK 6.0 loaded although the test hid it")
	}
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		cx.SetTitle(title)
		return core.View{Style: core.Style{Padding: driver.All(10)}, Children: []core.Node{
			core.WebView{Source: core.WebSrc("https://example.com"), Style: core.Style{Width: driver.Pt(300), Height: driver.Pt(200)}},
		}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	waitForTitle(t, w, title)
	waitHosted(t, w, func(v *view) bool { return v.web != nil && v.web.widget == 0 },
		driver.Bounds{Origin: driver.Point{X: 10, Y: 10}, Size: driver.Size{Width: 300, Height: 200}})
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
		return core.View{Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240), Background: driver.RGB(0xffffff)},
			Children: []core.Node{
				core.Video{Source: path, Controls: true, Style: core.Style{Width: driver.Pt(140), Height: driver.Pt(120)}},
				core.View{TestID: "cover", Style: core.Style{Position: core.Absolute, Left: driver.Pt(70), Top: driver.Pt(0),
					Width: driver.Pt(70), Height: driver.Pt(120), Background: driver.RGB(0xff0000)}},
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
		return core.View{Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240), Padding: driver.All(10),
			Background: driver.RGB(0xffffff)},
			Children: []core.Node{core.View{
				Style: core.Style{Width: driver.Pt(120), Height: driver.Pt(120), CornerRadius: 40, Overflow: core.Hidden},
				Children: []core.Node{core.Video{Source: path, Controls: true,
					Style: core.Style{Width: driver.Pt(120), Height: driver.Pt(120)}}},
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
		offset := core.NewSignal(cx, driver.Points(0))
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%v", title, offset.Get())) })
		return core.View{Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240), Padding: driver.All(5),
			Background: driver.RGB(0xffffff)},
			Children: []core.Node{core.Scroll{HideIndicator: true, OnScroll: func(e core.ScrollEvent) { offset.Set(e.Offset.Y) },
				Style: core.Style{Width: driver.Pt(40), Height: driver.Pt(40)},
				Children: []core.Node{
					core.View{Style: core.Style{Height: driver.Pt(40), Background: driver.RGB(0xff0000)}},
					core.View{Style: core.Style{Height: driver.Pt(100), Background: driver.RGB(0x0000ff)}},
				}}}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	waitForTitle(t, w, title+"|0")
	waitPixels(t, w, "red content at the top", func(p pixels) bool { return p.at(20, 10)[2] == 0xff })
	w.onLoop(t, func() { w.wheel(driver.Point{X: 20, Y: 20}, 0, 60) })
	waitForTitle(t, w, title+"|60")
	waitPixels(t, w, "blue content scrolled up, white below the scroll view", func(p pixels) bool {
		in, below := p.at(20, 10), p.at(20, 52)
		return in[0] == 0xff && in[2] == 0 && below[0] == 0xff && below[1] == 0xff && below[2] == 0xff
	})
}

// TestTouchpadGlides lifts the touchpad's fingers at a velocity: the view
// the turns moved glides on, slows down and rests, and fingers landing on
// another scroll stop it where it is.
func TestTouchpadGlides(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		offset, ended := core.NewSignal(cx, driver.Points(0)), core.NewSignal(cx, 0)
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%d|%v", title, ended.Get(), offset.Get())) })
		return core.Scroll{HideIndicator: true, OnScroll: func(e core.ScrollEvent) { offset.Set(e.Offset.Y) },
			OnMomentumEnd: func(core.ScrollEvent) { ended.Update(func(n int) int { return n + 1 }) },
			Style:         core.Style{Width: driver.Pt(100), Height: driver.Pt(100)},
			Children:      []core.Node{core.View{Style: core.Style{Height: driver.Pt(100000)}}}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 100, Height: 100})
	waitForTitle(t, w, title+"|0|0")
	glide := func(stop bool) {
		w.onLoop(t, func() {
			w.wheel(driver.Point{X: 20, Y: 20}, 0, 10)
			w.wheelGlide(0, 2)
			if stop {
				w.stopWheelGlide()
			}
		})
	}
	glide(false)
	deadline := time.Now().Add(10 * time.Second)
	for !strings.HasPrefix(w.titleOf(t), title+"|1|") {
		if time.Now().After(deadline) {
			t.Fatalf("window title = %q, want the glide ended", w.titleOf(t))
		}
		time.Sleep(10 * time.Millisecond)
	}
	var at float64
	if _, err := fmt.Sscanf(strings.TrimPrefix(w.titleOf(t), title+"|1|"), "%g", &at); err != nil || at < 200 {
		t.Fatalf("glided to %v (%v), want well past the turn's 10", at, err)
	}
	glide(true)
	waitForTitle(t, w, fmt.Sprintf("%s|2|%v", title, at+10))
	// At the end of the content it stops, with no bounce past it.
	var end driver.Points
	w.onLoop(t, func() {
		v := w.wheeled
		end = v.size.Height - v.frame.Size.Height
		w.scrollTo(v, driver.Point{Y: end - 20})
		w.wheelGlide(0, 2)
	})
	waitForTitle(t, w, fmt.Sprintf("%s|3|%v", title, end))
}

// TestShieldedTouchpadGlides drives a touchpad scroll through the
// hosted-view shield path (no GtkEventControllerScroll involved): the
// scroll view glides on after the stop event, and a shielded touchpad
// hold stops the glide in place.
func TestShieldedTouchpadGlides(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		offset, ended := core.NewSignal(cx, driver.Points(0)), core.NewSignal(cx, 0)
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%d|%v", title, ended.Get(), offset.Get())) })
		return core.Scroll{HideIndicator: true, OnScroll: func(e core.ScrollEvent) { offset.Set(e.Offset.Y) },
			OnMomentumEnd: func(core.ScrollEvent) { ended.Update(func(n int) int { return n + 1 }) },
			Style:         core.Style{Width: driver.Pt(100), Height: driver.Pt(100)},
			Children:      []core.Node{core.View{Style: core.Style{Height: driver.Pt(100000)}}}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 100, Height: 100})
	waitForTitle(t, w, title+"|0|0")
	flick := func(base uint32, stopWithHold bool) {
		w.onLoop(t, func() {
			now := time.Now()
			at := driver.Point{X: 20, Y: 20}
			w.shieldScrollEvent(0, at, 0, 16, true, true, false, base, now)
			w.shieldScrollEvent(0, at, 0, 16, true, true, false, base+8, now.Add(8*time.Millisecond))
			w.shieldScrollEvent(0, at, 0, 16, true, true, false, base+16, now.Add(16*time.Millisecond))
			w.shieldScrollEvent(0, at, 0, 0, true, true, true, base+16, now.Add(16*time.Millisecond))
			if stopWithHold {
				w.shieldTouchpadHold(gdkTouchpadPhaseBegin, base+24, now.Add(24*time.Millisecond))
				w.shieldTouchpadHold(gdkTouchpadPhaseEnd, base+32, now.Add(32*time.Millisecond))
			}
		})
	}
	flick(1000, false)
	deadline := time.Now().Add(10 * time.Second)
	for !strings.HasPrefix(w.titleOf(t), title+"|1|") {
		if time.Now().After(deadline) {
			t.Fatalf("window title = %q, want the shielded glide ended", w.titleOf(t))
		}
		time.Sleep(10 * time.Millisecond)
	}
	var at float64
	if _, err := fmt.Sscanf(strings.TrimPrefix(w.titleOf(t), title+"|1|"), "%g", &at); err != nil || at < 200 {
		t.Fatalf("shielded glide reached %v (%v), want well past the deltas' 48", at, err)
	}
	flick(2000, true)
	waitForTitle(t, w, fmt.Sprintf("%s|2|%v", title, at+48))
}

// TestWheelSettlesOnPagesWithinInsets turns the wheel over a paging
// scroll view with a content inset: it settles on the next page once the
// wheel rests, reporting the momentum, and the inset widens its offsets.
func TestWheelSettlesOnPagesWithinInsets(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		offset, rested := core.NewSignal(cx, driver.Points(0)), core.NewSignal(cx, 0)
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%v|%d", title, offset.Get(), rested.Get())) })
		return core.View{Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240), Background: driver.RGB(0xffffff)},
			Children: []core.Node{core.Scroll{HideIndicator: true, ContentOffset: driver.Point{Y: -10},
				Behavior:      driver.ScrollBehavior{Paging: true, ContentInset: driver.Edges{Top: 10}},
				OnScroll:      func(e core.ScrollEvent) { offset.Set(e.Offset.Y) },
				OnMomentumEnd: func(core.ScrollEvent) { rested.Update(func(n int) int { return n + 1 }) },
				Style:         core.Style{Width: driver.Pt(100), Height: driver.Pt(100)},
				Children:      []core.Node{core.View{Style: core.Style{Height: driver.Pt(400), Background: driver.RGB(0x0000ff)}}}}}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	waitForTitle(t, w, title+"|-10|0")
	w.onLoop(t, func() { w.wheel(driver.Point{X: 20, Y: 20}, 0, 30) })
	waitForTitle(t, w, title+"|100|1")
}

// TestTouchAreasDecideWhatTakesAPress presses a menu outside its parent,
// a small target by its hit slop and a button under a veil that takes no
// pointer.
func TestTouchAreasDecideWhatTakesAPress(t *testing.T) {
	w := startTestWindow(t, touchAreasApp("areas"), Options{Title: "areas|start", Width: 320, Height: 200})
	waitForTitle(t, w, "areas|none")
	for _, c := range []struct {
		at   driver.Point
		want string
	}{{driver.Point{X: 60, Y: 60}, "menu"}, {driver.Point{X: 225, Y: 25}, "slop"}, {driver.Point{X: 200, Y: 140}, "under"}} {
		w.onLoop(t, func() { w.press(c.at) })
		waitForTitle(t, w, "areas|"+c.want)
	}
}

// TestLongPressWaitsForTheViewsDelay holds a press on a view whose long
// press takes 300 ms: it becomes a long press and its release presses
// nothing, while a quick press stays a press.
func TestLongPressWaitsForTheViewsDelay(t *testing.T) {
	app := func(cx *core.Scope) core.Node {
		last := core.NewSignal(cx, "none")
		cx.Effect(func() { cx.SetTitle("long|" + last.Get()) })
		return core.View{Style: core.Style{Width: driver.Pt(320), Height: driver.Pt(200)}, Children: []core.Node{
			core.View{DelayLongPress: 300 * time.Millisecond, Style: core.Style{Width: driver.Pt(100), Height: driver.Pt(100)},
				OnPress:     func(core.PressEvent) { last.Set("press") },
				OnLongPress: func(core.PressEvent) { last.Set("long") }},
		}}
	}
	w := startTestWindow(t, app, Options{Title: "long|start", Width: 320, Height: 200})
	waitForTitle(t, w, "long|none")
	at := driver.Point{X: 50, Y: 50}
	w.onLoop(t, func() { w.pointerDown(at, 1, 0, 0, 1) })
	waitForTitle(t, w, "long|long")
	w.onLoop(t, func() { w.pointerUp(at) })
	time.Sleep(100 * time.Millisecond)
	waitForTitle(t, w, "long|long")
	w.onLoop(t, func() { w.pointerDown(at, 1, 0, 0, 1); w.pointerUp(at) })
	waitForTitle(t, w, "long|press")
}

// TestTheResponderFollowsADrag drags on a view that takes the touches as
// they start: it is granted them, follows the move and is released.
func TestTheResponderFollowsADrag(t *testing.T) {
	w := startTestWindow(t, responderApp("responder"), Options{Title: "responder|start", Width: 320, Height: 200})
	waitForTitle(t, w, "responder|")
	w.onLoop(t, func() { w.pointerDown(driver.Point{X: 20, Y: 20}, 1, 0, 0, 1) })
	waitForTitle(t, w, "responder|grant 0,0")
	w.onLoop(t, func() { w.movePointer(driver.Point{X: 50, Y: 30}, gdkButton1) })
	waitForTitle(t, w, "responder|move 30,10")
	w.onLoop(t, func() { w.pointerUp(driver.Point{X: 50, Y: 30}) })
	waitForTitle(t, w, "responder|release 30,10")
}

// responderApp is a view that takes the touches as they start and logs
// its responder's events in the title.
func responderApp(title string) func(*core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		log := core.NewSignal(cx, "")
		cx.Effect(func() { cx.SetTitle(title + "|" + log.Get()) })
		say := func(what string) func(core.ResponderEvent) {
			return func(e core.ResponderEvent) {
				log.Set(fmt.Sprintf("%s %.0f,%.0f", what, e.Gesture.Delta.X, e.Gesture.Delta.Y))
			}
		}
		return core.View{Style: core.Style{Width: driver.Pt(320), Height: driver.Pt(200), Background: driver.RGB(0xffffff)},
			Children: []core.Node{core.View{Style: core.Style{Width: driver.Pt(100), Height: driver.Pt(100)},
				Responder: &core.Responder{Claims: []core.TouchClaim{{}}, OnGrant: say("grant"), OnMove: say("move"),
					OnRelease: say("release")}}}}
	}
}

func touchAreasApp(title string) func(*core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		last := core.NewSignal(cx, "none")
		cx.Effect(func() { cx.SetTitle(title + "|" + last.Get()) })
		press := func(name string) func(core.PressEvent) { return func(core.PressEvent) { last.Set(name) } }
		return core.View{Style: core.Style{Width: driver.Pt(320), Height: driver.Pt(200), Background: driver.RGB(0xffffff)},
			Children: []core.Node{
				// A menu 30 points below its field, outside it.
				core.View{Style: core.Style{Position: core.Absolute, Left: driver.Pt(10), Top: driver.Pt(10), Width: driver.Pt(100),
					Height: driver.Pt(30)}, Children: []core.Node{
					core.View{OnPress: press("menu"), Style: core.Style{Position: core.Absolute, Top: driver.Pt(30),
						Width: driver.Pt(100), Height: driver.Pt(40), Background: driver.RGB(0x123456)}},
				}},
				// A 10-point target with 20 points of slop.
				core.View{OnPress: press("slop"), HitSlop: driver.All(20), Style: core.Style{Position: core.Absolute,
					Left: driver.Pt(200), Top: driver.Pt(20), Width: driver.Pt(10), Height: driver.Pt(10), Background: driver.RGB(0x123456)}},
				// A veil over a button lets its presses through.
				core.View{OnPress: press("under"), Style: core.Style{Position: core.Absolute, Left: driver.Pt(150),
					Top: driver.Pt(120), Width: driver.Pt(100), Height: driver.Pt(40), Background: driver.RGB(0x123456)}},
				core.View{OnPress: press("veil"), PointerEvents: driver.PointerEventsNone, Style: core.Style{Position: core.Absolute,
					Left: driver.Pt(140), Top: driver.Pt(110), Width: driver.Pt(120), Height: driver.Pt(60)}},
			}}
	}
}

// TestPointerMovesReportPositions moves the pointer over a view that
// follows moves: OnMouseMove hears it in the view and the surface.
func TestPointerMovesReportPositions(t *testing.T) {
	app := func(cx *core.Scope) core.Node {
		hover := core.NewSignal(cx, core.MouseEvent{})
		cx.Effect(func() {
			h := hover.Get()
			cx.SetTitle(fmt.Sprintf("moves|%.0f,%.0f|%.0f,%.0f", h.Position.X, h.Position.Y,
				h.PagePosition.X, h.PagePosition.Y))
		})
		return core.View{Style: core.Style{Width: driver.Pt(320), Height: driver.Pt(200), Padding: driver.All(10)},
			Children: []core.Node{
				core.View{Style: core.Style{Width: driver.Pt(100), Height: driver.Pt(80)},
					OnMouseMove: hover.Set},
			}}
	}
	w := startTestWindow(t, app, Options{Title: "moves|start", Width: 320, Height: 200})
	waitForTitle(t, w, "moves|0,0|0,0")
	w.onLoop(t, func() { motionCallback(0, 40, 30, w.handle) })
	waitForTitle(t, w, "moves|30,20|40,30")
}

// TestHoverFollowsThePointer moves the pointer onto a view and its
// child, off them and out of the window, as GTK's motion controller
// reports it, and checks the hover state style shows while it is over.
func TestHoverFollowsThePointer(t *testing.T) {
	app := func(cx *core.Scope) core.Node {
		in, out := core.NewSignal(cx, 0), core.NewSignal(cx, 0)
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("hover|%d|%d", in.Get(), out.Get())) })
		return core.View{Style: core.Style{Width: driver.Pt(320), Height: driver.Pt(200), Padding: driver.All(10)},
			Children: []core.Node{
				core.View{TestID: "target", Style: core.Style{Width: driver.Pt(100), Height: driver.Pt(80), Padding: driver.All(20),
					Background: driver.RGB(0x123456), Hover: &core.Style{Background: driver.RGB(0x654321)}},
					OnMouseEnter: func(core.MouseEvent) { in.Update(func(n int) int { return n + 1 }) },
					OnMouseLeave: func(core.MouseEvent) { out.Update(func(n int) int { return n + 1 }) },
					Children:     []core.Node{core.View{Style: core.Style{Width: driver.Pt(20), Height: driver.Pt(20)}}}},
			}}
	}
	w := startTestWindow(t, app, Options{Title: "hover|0|0", Width: 320, Height: 200})
	waitForTitle(t, w, "hover|0|0")
	move := func(x, y float64) { w.onLoop(t, func() { motionCallback(0, x, y, w.handle) }) }
	background := func() (c driver.Color) {
		w.onLoop(t, func() {
			for _, v := range w.views {
				if v != nil && v.props.TestID == "target" {
					c = v.style.Background
				}
			}
		})
		return c
	}
	move(15, 15)
	waitForTitle(t, w, "hover|1|0")
	// Over its child, the view stays hovered.
	move(40, 40)
	time.Sleep(50 * time.Millisecond)
	waitForTitle(t, w, "hover|1|0")
	deadline := time.Now().Add(5 * time.Second)
	for background() != driver.RGB(0x654321) {
		if time.Now().After(deadline) {
			t.Fatalf("background = %x, want the hover style's", background())
		}
		time.Sleep(10 * time.Millisecond)
	}
	move(200, 150)
	waitForTitle(t, w, "hover|1|1")
	move(15, 15)
	waitForTitle(t, w, "hover|2|1")
	w.onLoop(t, func() { leaveCallback(0, w.handle) })
	waitForTitle(t, w, "hover|2|2")
}

// TestStickyListRowsStayPinned turns the wheel over a list whose first
// row sticks: the row stays at the top, moved natively from its frame.
func TestStickyListRowsStayPinned(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		offset := core.NewSignal(cx, driver.Points(0))
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%v", title, offset.Get())) })
		return core.View{Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240), Padding: driver.All(5),
			Background: driver.RGB(0xffffff)},
			Children: []core.Node{core.List{HideIndicator: true, OnScroll: func(e core.ScrollEvent) { offset.Set(e.Offset.Y) },
				Count: func() int { return 20 }, StickyHeaderIndices: []int{0}, EstimatedSize: 20,
				Style: core.Style{Width: driver.Pt(40), Height: driver.Pt(100)},
				Row: func(cx *core.Scope, r core.ListRow) core.Node {
					return core.View{StyleFunc: func() core.Style {
						if r.Index() == 0 {
							return core.Style{Height: driver.Pt(20), Background: driver.RGB(0xff0000)}
						}
						return core.Style{Height: driver.Pt(20), Background: driver.RGB(0x0000ff)}
					}}
				}}}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	waitForTitle(t, w, title+"|0")
	w.onLoop(t, func() { w.wheel(driver.Point{X: 20, Y: 20}, 0, 60) })
	waitForTitle(t, w, title+"|60")
	waitPixels(t, w, "the sticky row at the top over blue rows", func(p pixels) bool {
		top, below := p.at(20, 12), p.at(20, 40)
		return top[2] == 0xff && top[0] == 0 && below[0] == 0xff && below[2] == 0
	})
}

// TestVideoRefusesAlphaItCannotKeep opens HEVC with alpha, whose alpha layer GStreamer cannot decode: the video reports an error with MediaErrorAlphaUnsupported that says so instead of showing an opaque picture.
func TestVideoRefusesAlphaItCannotKeep(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	path := filepath.Join(t.TempDir(), "alpha.mov")
	if err := os.WriteFile(path, alphaclips.HEVCAlpha, 0o644); err != nil {
		t.Fatal(err)
	}
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		state := core.NewSignal(cx, "")
		cx.Effect(func() { cx.SetTitle(title + "|" + state.Get()) })
		return core.Video{Source: path, Style: core.Style{Width: driver.Pt(100), Height: driver.Pt(100)},
			OnLoad: func(core.VideoInfo) { state.Set("load") },
			OnError: func(err error) {
				var reason core.MediaErrorReason
				if me, ok := err.(*core.MediaError); ok {
					reason = me.Reason
				}
				state.Set("error " + reason.String() + " " + err.Error())
			}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	got := waitTitle(t, w, func(s string) bool { return strings.Contains(s, "|error ") || strings.HasSuffix(s, "|load") })
	if !strings.Contains(got, "|error alpha-unsupported "+path+": GStreamer cannot decode the alpha channel of HEVC with alpha") {
		t.Fatalf("title = %q, want the alpha refused", got)
	}
}
