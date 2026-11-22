//go:build js && wasm && nsgotest

package web

import (
	"os"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// Trusted app controls go through Ref, the mount command and the browser's
// real PiP API; the callback must agree with the document's actual owner.
func TestVideoPictureInPictureBrowserSession(t *testing.T) {
	if os.Getenv("NSGO_TEST_TARGET") != "web" {
		t.Fatal("requires the web target host")
	}
	doc := js.Global().Get("document")
	if !doc.Get("pictureInPictureEnabled").Truthy() {
		t.Skip("browser does not offer standard video PiP")
	}
	// Record NSGo's own canvas frames instead of depending on a network clip or
	// an external codec. The resulting WebM is a normal Video.Source.
	record := js.Global().Get("Function").New(`return new Promise((resolve, reject) => {
 const canvas = document.createElement('canvas'); canvas.width = 64; canvas.height = 48;
 const context = canvas.getContext('2d'); const stream = canvas.captureStream(10);
 const recorder = new MediaRecorder(stream, {mimeType: 'video/webm;codecs=vp8'});
 const chunks = []; let frame = 0;
 const timer = setInterval(() => { context.fillStyle = frame++ % 2 ? '#de4a31' : '#249cd6'; context.fillRect(0, 0, 64, 48); }, 40);
 recorder.ondataavailable = e => chunks.push(e.data);
 recorder.onerror = e => reject(e.error);
 recorder.onstop = () => { clearInterval(timer); stream.getTracks().forEach(t => t.stop()); resolve(URL.createObjectURL(new Blob(chunks, {type:'video/webm'}))); };
 recorder.start(); setTimeout(() => recorder.stop(), 600);
});`)
	var url, recordError string
	done := false
	ok := js.FuncOf(func(_ js.Value, args []js.Value) any { url = args[0].String(); done = true; return nil })
	fail := js.FuncOf(func(_ js.Value, args []js.Value) any { recordError = args[0].String(); done = true; return nil })
	record.Invoke().Call("then", ok, fail)
	pipWait(t, "recorded video", func() bool { return done })
	ok.Release()
	fail.Release()
	if recordError != "" {
		t.Fatal(recordError)
	}
	defer js.Global().Get("URL").Call("revokeObjectURL", url)
	var ref core.Ref
	var source *core.Signal[string]
	var visible *core.Signal[bool]
	var state core.PictureInPictureState
	native, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		source, visible = core.NewSignal(cx, url), core.NewSignal(cx, true)
		button := func(id string, fn func()) core.Node {
			return core.View{TestID: id, Style: core.Style{Width: core.Pt(160), Height: core.Pt(40)}, OnPress: func(core.PressEvent) { fn() }, Children: []core.Node{core.Text{Content: id}}}
		}
		return core.View{Children: []core.Node{
			core.Show{When: visible.Get, Then: func(*core.Scope) core.Node {
				return core.Video{Ref: &ref, SourceFunc: source.Get, Muted: true, Repeat: true, PictureInPicture: true, OnPictureInPicture: func(s core.PictureInPictureState) { state = s }, Style: core.Style{Width: core.Pt(240), Height: core.Pt(180)}}
			}},
			button("start-pip", ref.StartPictureInPicture), button("stop-pip", ref.StopPictureInPicture),
			button("clear-source", func() { source.Set("") }), button("restore-source", func() { source.Set(url) }),
			button("remove-video", func() { visible.Set(false) }),
		}}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	host := native.(*testSurface)
	defer host.s.engine.Close()
	host.Sync()
	var vd *video
	for _, v := range host.s.views {
		if v != nil && v.video != nil {
			vd = v.video
			break
		}
	}
	if vd == nil {
		t.Fatal("no real Video element")
	}
	press := func(id string) {
		t.Helper()
		el := doc.Call("querySelector", "[data-testid="+id+"]")
		r := el.Call("getBoundingClientRect")
		if err := host.Press(driver.Point{X: driver.Points(r.Get("left").Float() + 12), Y: driver.Points(r.Get("top").Float() + 12)}, 0, driver.Modifiers{}); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(label string, ready func() bool) {
		t.Helper()
		pipWait(t, label, func() bool {
			if state.Error != "" {
				t.Fatalf("%s: %+v", label, state)
			}
			return ready()
		})
	}
	wait("metadata readiness", func() bool { return state.Supported && state.Possible })
	enter := func() {
		t.Helper()
		press("start-pip")
		wait("actual browser entry and callback", func() bool { return doc.Get("pictureInPictureElement").Equal(vd.el) && state.Active })
	}
	enter()
	press("start-pip")
	if !doc.Get("pictureInPictureElement").Equal(vd.el) {
		t.Fatal("duplicate start ended PiP")
	}
	press("stop-pip")
	wait("app exit", func() bool { return !doc.Get("pictureInPictureElement").Truthy() && !state.Active })
	press("stop-pip")
	enter()
	// The browser, rather than an app command, ends this session.
	doc.Call("exitPictureInPicture")
	wait("browser exit callback", func() bool { return !state.Active && !doc.Get("pictureInPictureElement").Truthy() })
	enter()
	press("clear-source")
	wait("cleared source exits", func() bool { return !state.Active && !state.Possible && !doc.Get("pictureInPictureElement").Truthy() })
	press("restore-source")
	wait("replacement readiness", func() bool { return state.Possible })
	enter()
	press("remove-video")
	pipWait(t, "owner deletion exits", func() bool { return !doc.Get("pictureInPictureElement").Truthy() })
	t.Log("real browser PiP: Ref entry/exit, duplicate commands, native exit, source clear/reload, owner deletion")
	time.Sleep(30 * time.Millisecond) // Drain the removed owner's queued media events.
}
