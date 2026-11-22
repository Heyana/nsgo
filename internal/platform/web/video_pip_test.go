//go:build js && wasm

package web

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Controlled promises cover transitions a real browser cannot reliably delay.
func TestVideoPictureInPicturePendingOwnership(t *testing.T) {
	for _, invalidate := range []string{"stop", "source", "disable", "release"} {
		t.Run(invalidate, func(t *testing.T) {
			p, doc, resolve, _, exits, cleanup := pipFixture(t)
			defer cleanup()
			p.command(driver.PictureInPictureStart)
			p.command(driver.PictureInPictureStart)
			if !p.pending {
				t.Fatal("start did not wait for the browser")
			}
			switch invalidate {
			case "stop":
				p.command(driver.PictureInPictureStop)
			case "source":
				p.configure(true, true)
			case "disable":
				p.configure(false, false)
			case "release":
				p.released = true
				p.invalidate()
			}
			doc.Set("pictureInPictureElement", p.el)
			p.entered()
			resolve.Invoke(js.Undefined())
			pipWait(t, "stale start settlement", func() bool { return !p.pending && !p.exiting })
			if *exits != 1 || p.active() {
				t.Fatalf("late start survived %s: exits=%d active=%v", invalidate, *exits, p.active())
			}
			if invalidate == "source" || invalidate == "stop" {
				// A browser-controls entry after the old request settled is legitimate.
				doc.Set("pictureInPictureElement", p.el)
				p.entered()
				if !p.state.Active {
					t.Fatal("a new native entry was rejected")
				}
			}
		})
	}
}

func TestVideoPictureInPictureErrorsAndOtherOwner(t *testing.T) {
	p, doc, _, reject, exits, cleanup := pipFixture(t)
	defer cleanup()
	p.el.Set("readyState", 0)
	p.command(driver.PictureInPictureStart)
	if !p.state.Supported || p.state.Possible || p.state.Error == "" {
		t.Fatalf("not-ready state: %+v", p.state)
	}
	p.el.Set("readyState", 1)
	p.command(driver.PictureInPictureStart)
	reject.Invoke(js.Global().Get("Error").New("denied by browser"))
	pipWait(t, "rejection", func() bool { return !p.pending })
	if p.state.Error != "denied by browser" || !p.state.Supported {
		t.Fatalf("rejection state: %+v", p.state)
	}
	doc.Set("pictureInPictureElement", js.Global().Get("Object").New())
	p.command(driver.PictureInPictureStop)
	p.configure(false, false)
	if *exits != 0 {
		t.Fatal("stopped another video's session")
	}
	doc.Set("pictureInPictureEnabled", false)
	p.configure(true, false)
	p.command(driver.PictureInPictureStart)
	if p.state.Supported || p.state.Error != "" {
		t.Fatalf("unsupported state: %+v", p.state)
	}
}

func TestVideoPictureInPictureSynchronousFailure(t *testing.T) {
	p, _, _, _, _, cleanup := pipFixture(t)
	defer cleanup()
	p.el.Set("requestPictureInPicture", js.Global().Get("Function").New("throw new DOMException('activation required', 'NotAllowedError')"))
	p.command(driver.PictureInPictureStart)
	if p.pending || !p.state.Supported || p.state.Error == "" {
		t.Fatalf("synchronous DOM exception lost: pending=%v state=%+v", p.pending, p.state)
	}
}

func TestVideoPictureInPictureStopIsSerialized(t *testing.T) {
	p, doc, _, _, _, cleanup := pipFixture(t)
	defer cleanup()
	var resolve js.Value
	executor := js.FuncOf(func(_ js.Value, args []js.Value) any { resolve = args[0]; return nil })
	defer executor.Release()
	pending := js.Global().Get("Promise").New(executor)
	exits := 0
	exit := js.FuncOf(func(js.Value, []js.Value) any { exits++; return pending })
	defer exit.Release()
	doc.Set("exitPictureInPicture", exit)
	doc.Set("pictureInPictureElement", p.el)
	p.command(driver.PictureInPictureStop)
	p.command(driver.PictureInPictureStop)
	p.command(driver.PictureInPictureStart)
	if exits != 1 || p.pending {
		t.Fatalf("exit overlap: exits=%d pending=%v", exits, p.pending)
	}
	doc.Set("pictureInPictureElement", js.Null())
	resolve.Invoke(js.Undefined())
	pipWait(t, "exit", func() bool { return !p.exiting })
}

func TestVideoPictureInPictureClearingSourceInvalidates(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("needs nsgo test -target web")
	}
	s := &surface{doc: doc}
	v := &view{el: doc.Call("createElement", "div")}
	s.createVideo(v)
	defer func() { v.video.release(); v.listeners.release() }()
	c := driver.VideoConfig{Source: "data:video/webm;base64,", Paused: true, PictureInPicture: true, Volume: 1, Rate: 1}
	s.setVideo(v, &c)
	generation := v.video.pip.generation
	c.Source = ""
	s.setVideo(v, &c)
	if v.video.pip.generation <= generation || v.video.src != "" {
		t.Fatal("clearing the source kept the previous generation")
	}
}

func pipFixture(t *testing.T) (*videoPictureInPicture, js.Value, js.Value, js.Value, *int, func()) {
	t.Helper()
	doc, el := js.Global().Get("Object").New(), js.Global().Get("Object").New()
	el.Set("ownerDocument", doc)
	el.Set("readyState", 1)
	el.Set("videoWidth", 64)
	doc.Set("pictureInPictureEnabled", true)
	doc.Set("pictureInPictureElement", js.Null())
	var resolve, reject js.Value
	executor := js.FuncOf(func(_ js.Value, args []js.Value) any { resolve, reject = args[0], args[1]; return nil })
	promise := js.Global().Get("Promise").New(executor)
	start := js.FuncOf(func(js.Value, []js.Value) any { return promise })
	exits := 0
	exit := js.FuncOf(func(js.Value, []js.Value) any {
		exits++
		doc.Set("pictureInPictureElement", js.Null())
		return js.Global().Get("Promise").Call("resolve")
	})
	el.Set("requestPictureInPicture", start)
	doc.Set("exitPictureInPicture", exit)
	p := &videoPictureInPicture{el: el}
	p.configure(true, false)
	return p, doc, resolve, reject, &exits, func() { executor.Release(); start.Release(); exit.Release() }
}

func pipWait(t *testing.T, label string, ready func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ready(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", label)
		}
	}
}
