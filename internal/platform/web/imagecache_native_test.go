//go:build js && wasm

package web

import (
	"image/color"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

func TestBrowserCanvasCacheReleasesImagesUnderBytePressure(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("requires a browser")
	}
	s := &surface{doc: doc, animator: driver.NewAnimator(nil)}
	v := s.create(1, driver.KindCanvas)
	t.Cleanup(func() { s.canvasImages.clear(); v.el.Call("remove") })
	sources := canvastest.CacheSources(20)
	s.setImageCacheLimit(2 * int64(len(sources[len(sources)-1].Data)))
	canvas := doc.Call("createElement", "canvas")
	canvas.Set("width", 2)
	canvas.Set("height", 2)
	load := func(src driver.ImageSource) *canvasImage {
		t.Helper()
		im := s.loadImage(src, v)
		for deadline := time.Now().Add(5 * time.Second); !im.loaded; {
			if im.failed || time.Now().After(deadline) {
				t.Fatal("image did not load")
			}
			time.Sleep(time.Millisecond)
		}
		canvas.Call("getContext", "2d").Call("drawImage", im.img, 0, 0)
		if got := rgbaAt(canvas, 0, 0); got != (color.RGBA{R: 255, A: 255}) {
			t.Fatalf("cache pressure changed the drawing: %v", got)
		}
		s.canvasImages.endPass()
		return im
	}
	first := load(sources[0])
	s.keepCanvasImage(sources[0].Key())
	for _, src := range sources[1:] {
		load(src)
		_, bytes := s.canvasImages.unkeptStats()
		if bytes > s.canvasImages.effectiveLimit() {
			t.Fatalf("retained %d unkept bytes above budget", bytes)
		}
	}
	if !first.img.Call("hasAttribute", "src").Bool() {
		t.Fatal("kept image was released under pressure")
	}
	s.setImageCacheLimit(0)
	if s.canvasImages.len() != 1 {
		t.Fatalf("zero budget retained %d entries, want the kept one", s.canvasImages.len())
	}
	s.releaseCanvasImage(sources[0].Key())
	if s.canvasImages.len() != 0 || first.img.Call("hasAttribute", "src").Bool() || first.object || first.url != "" {
		t.Fatal("last release retained the image or its blob URL")
	}
}
