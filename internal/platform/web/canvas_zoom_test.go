//go:build js && wasm

package web

import (
	"math"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func canvasZoomSurface(t *testing.T) (*surface, *view) {
	t.Helper()
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("run with nsgo test -target web")
	}
	style := doc.Call("createElement", "style")
	style.Set("textContent", baseCSS)
	doc.Get("head").Call("appendChild", style)
	host := doc.Call("createElement", "div")
	doc.Get("body").Call("appendChild", host)
	s := &surface{doc: doc, host: host, animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: 2, ReduceMotion: true}}
	s.parked = s.element("div")
	host.Call("appendChild", s.parked)
	t.Cleanup(func() {
		defer style.Call("remove")
		if t.Failed() {
			for _, v := range s.views {
				if v != nil && v.sc != nil {
					t.Logf("scroll %d: zoom=%v, canvases=%d, timer=%v", v.id, v.sc.zoom, v.content.Call("querySelectorAll", "canvas").Length(), v.sc.rasterTimer)
				}
				if v != nil && v.drawing != nil {
					t.Logf("canvas %d: scale=%v, last=%v", v.id, s.canvasScale(v), v.canvasRatio)
				}
			}
		}
		for _, v := range s.views {
			if v == nil {
				continue
			}
			if v.sc != nil {
				v.sc.stopRaster()
				v.sc.stopZoom()
			}
			v.stopTransformRaster()
			v.listeners.release()
		}
		host.Call("remove")
	})
	f := driver.Bounds{Size: driver.Size{Width: 100, Height: 100}}
	c := driver.ScrollConfig{MinZoomScale: .5, MaxZoomScale: 8, ZoomScale: 1, HideIndicator: true}
	d := driver.NewDrawing(nil)
	d.SetFillColor(driver.RGB(0xffffff)).FillRect(0, 0, 100, 100)
	d.SetFillColor(driver.RGB(0x000000)).FillRect(10, 0, 1, 100)
	s.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindScroll},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemFrame, ID: 2, Frame: f},
		{Kind: driver.ItemScrollContent, ID: 2, Size: f.Size},
		{Kind: driver.ItemScrollConfig, ID: 2, Scroll: &c},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindScroll},
		{Kind: driver.ItemInsert, ID: 3, Parent: 1},
		{Kind: driver.ItemFrame, ID: 3, Frame: f},
		{Kind: driver.ItemScrollContent, ID: 3, Size: f.Size},
		{Kind: driver.ItemScrollConfig, ID: 3, Scroll: &c},
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 4, Parent: 2},
		{Kind: driver.ItemFrame, ID: 4, Frame: f},
		{Kind: driver.ItemCreate, ID: 5, View: driver.KindCanvas},
		{Kind: driver.ItemInsert, ID: 5, Parent: 4},
		{Kind: driver.ItemFrame, ID: 5, Frame: f},
		{Kind: driver.ItemCanvas, ID: 5, Drawing: d},
	}})
	return s, s.view(5)
}

func waitCanvasPixels(t *testing.T, v *view, size int) {
	t.Helper()
	waitCanvasBitmap(t, v, size, size)
}

func waitCanvasBitmap(t *testing.T, v *view, w, h int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if v.inner.Get("width").Int() == w && v.inner.Get("height").Int() == h {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("canvas bitmap is %dx%d, want %dx%d", v.inner.Get("width").Int(), v.inner.Get("height").Int(), w, h)
		}
	}
}

// mountCanvas mounts canvas id, w by h points, inside a new view holder into parent, in the order the core sends a new subtree: the canvas draws and is framed before its holder, and the holder reaches parent last.
func mountCanvas(s *surface, holder, id, parent driver.NodeID, w, h driver.Points) *view {
	d := driver.NewDrawing(nil)
	d.SetFillColor(driver.RGB(0x000000)).FillRect(0, 0, 1, float32(h))
	f := driver.Bounds{Size: driver.Size{Width: w, Height: h}}
	s.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: holder, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: id, View: driver.KindCanvas},
		{Kind: driver.ItemCanvas, ID: id, Drawing: d},
		{Kind: driver.ItemFrame, ID: id, Frame: f},
		{Kind: driver.ItemInsert, ID: id, Parent: holder},
		{Kind: driver.ItemFrame, ID: holder, Frame: f},
		{Kind: driver.ItemInsert, ID: holder, Parent: parent},
	}})
	return s.view(id)
}

func TestCanvasScrollZoomReachesNewSubtrees(t *testing.T) {
	s, existing := canvasZoomSurface(t)
	s.zoomCommand(s.view(2), &driver.ZoomRequest{Scale: 3}, false)
	// The zoom's replay has run, so it cannot be what sharpens the new canvas.
	waitCanvasPixels(t, existing, 600)
	// The canvas draws while its holder is still detached, so only the holder's insert can find the zoom.
	canvas := mountCanvas(s, 6, 7, 2, 100, 100)
	waitCanvasPixels(t, canvas, 600)
}

func TestCanvasScrollZoomBoundsBitmap(t *testing.T) {
	s, _ := canvasZoomSurface(t)
	s.zoomCommand(s.view(2), &driver.ZoomRequest{Scale: 4, Center: &driver.Point{}}, false)
	// Canvases whose full bitmap would exceed 4096×4096 or 16384 side draw only
	// their visible slice at full zoom resolution (ratio 2 * 4 = 8), staying
	// well within the bitmap budget.
	square := mountCanvas(s, 6, 7, 2, 1024, 1024)
	long := mountCanvas(s, 8, 9, 2, 4096, 10)
	large := mountCanvas(s, 10, 11, 2, 2500, 2500)
	for _, tc := range []struct {
		name string
		v    *view
	}{{"square", square}, {"long", long}, {"large", large}} {
		if !tc.v.canvasSliced {
			t.Errorf("%s: expected visible-slice mode", tc.name)
		}
		if tc.v.canvasRatio != 8 {
			t.Errorf("%s: canvasRatio = %v, want 8", tc.name, tc.v.canvasRatio)
		}
		bw, bh := tc.v.inner.Get("width").Int(), tc.v.inner.Get("height").Int()
		if bw <= 0 || bh <= 0 || bw*bh > maxZoomPixels || bw > maxZoomSide || bh > maxZoomSide {
			t.Errorf("%s: bitmap %dx%d out of budget", tc.name, bw, bh)
		}
	}
	// Scrolling to another part of a sliced canvas updates its painted slice.
	scroll := s.view(2)
	s.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemScrollContent, ID: 2, Size: driver.Size{Width: 2500, Height: 2500}},
	}})
	scroll.el.Set("scrollLeft", 4000)
	scroll.el.Set("scrollTop", 4000)
	s.scrolled(scroll)
	if large.canvasTile.x < 900 || large.canvasTile.y < 900 {
		t.Errorf("scrolled large canvas tile = %+v, want origin near (1000, 1000)", large.canvasTile)
	}
	// A rotated large canvas cannot use axis-aligned slicing, so it still
	// stays bounded within the bitmap budget.
	rot := mountCanvas(s, 12, 13, 2, 1024, 1024)
	s.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 12, ViewStyle: &driver.ViewStyle{Opacity: 1, Transform: driver.Transform{Rotate: 15}}},
	}})
	if rot.canvasSliced {
		t.Error("rotated canvas should not enter axis-aligned slice mode")
	}
	waitCanvasPixels(t, rot, 4096)
	// The fixture's small canvas still takes the whole zoom without slicing.
	waitCanvasPixels(t, s.view(5), 800)
	if s.view(5).canvasSliced {
		t.Error("small canvas should draw whole, not sliced")
	}
}

func TestCanvasScrollZoomStyleTransforms(t *testing.T) {
	s, canvas := canvasZoomSurface(t)
	waitCanvasPixels(t, canvas, 200)
	// Static style transform on holder (ID 4) sharpens canvas immediately.
	s.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 4, ViewStyle: &driver.ViewStyle{Opacity: 1, Transform: driver.Transform{Scale: 3}}},
	}})
	waitCanvasPixels(t, canvas, 600)
	if !canvas.canvasExact {
		t.Error("axis-aligned style transform should enable exact pixel rendering")
	}
	// Rotation keeps the higher-res bitmap but disables nearest-neighbor pixelated sampling.
	s.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 4, ViewStyle: &driver.ViewStyle{Opacity: 1, Transform: driver.Transform{Scale: 3, Rotate: 20}}},
	}})
	waitCanvasPixels(t, canvas, 600)
	if canvas.canvasExact {
		t.Error("rotated style transform must not use pixelated sampling")
	}
	// Combining style transform (1.5x) and scroll zoom (2x) multiplies to 3x (600px at ratio 2).
	s.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 4, ViewStyle: &driver.ViewStyle{Opacity: 1, Transform: driver.Transform{Scale: 1.5}}},
	}})
	s.zoomCommand(s.view(2), &driver.ZoomRequest{Scale: 2}, false)
	waitCanvasPixels(t, canvas, 600)
	// Animated scale on holder replays after rest.
	s.zoomCommand(s.view(2), &driver.ZoomRequest{Scale: 1}, false)
	s.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 4, ViewStyle: &driver.ViewStyle{Opacity: 1}},
	}})
	waitCanvasPixels(t, canvas, 200)
	s.animator.Apply([]driver.AnimationOp{
		{Kind: driver.AnimCreateValue, Node: 1, Value: 2.5},
		{Kind: driver.AnimBind, View: 4, Prop: driver.PropScale, Node: 1},
	})
	s.applyAnimated()
	waitCanvasPixels(t, canvas, 500)
}

func TestCanvasScrollZoomFollowsKeptPlacement(t *testing.T) {
	s, canvas := canvasZoomSurface(t)
	waitCanvasPixels(t, canvas, 200)
	s.zoomCommand(s.view(2), &driver.ZoomRequest{Scale: 3}, false)
	waitCanvasPixels(t, canvas, 600)
	s.zoomCommand(s.view(3), &driver.ZoomRequest{Scale: 2}, false)
	// Only the holder moves; its canvas sends no frame or drawing item.
	s.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemInsert, ID: 4, Parent: 3}}})
	waitCanvasPixels(t, canvas, 400)
	s.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemRemove, ID: 4}, {Kind: driver.ItemInsert, ID: 4, Parent: 1}}})
	waitCanvasPixels(t, canvas, 200)
	// A later screen-scale change still takes the current placement.
	s.env.PixelRatio = 3
	s.paint(canvas)
	waitCanvasPixels(t, canvas, 300)
}

func TestCanvasScrollZoomFollowsWheelAndConfiguration(t *testing.T) {
	s, canvas := canvasZoomSurface(t)
	scroll := s.view(2)
	e := js.Global().Get("WheelEvent").New("wheel", map[string]any{
		"ctrlKey": true, "deltaY": -100 * math.Log(3), "clientX": 0, "clientY": 0, "cancelable": true,
	})
	s.scrollWheelZoom(scroll, e)
	waitCanvasPixels(t, canvas, 600)
	// Configuration can clamp a live zoom without a ZoomTo command.
	s.zoomCommand(scroll, &driver.ZoomRequest{Scale: 8}, true)
	time.Sleep(30 * time.Millisecond)
	c := scroll.sc.config
	c.MaxZoomScale = 2
	s.configureScroll(scroll, &c)
	waitCanvasPixels(t, canvas, 400)
	time.Sleep(300 * time.Millisecond)
	waitCanvasPixels(t, canvas, 400)
	c.MaxZoomScale = c.MinZoomScale
	s.configureScroll(scroll, &c)
	waitCanvasPixels(t, canvas, 200)
	// Raster resolution is bounded, as on Apple, even at larger zooms.
	c.MinZoomScale, c.MaxZoomScale = 1, 8
	s.configureScroll(scroll, &c)
	s.zoomCommand(scroll, &driver.ZoomRequest{Scale: 8}, false)
	waitCanvasPixels(t, canvas, 800)
	// Removing the scroll while its next raster replay waits is safe.
	s.zoomCommand(scroll, &driver.ZoomRequest{Scale: 2}, false)
	s.zoomCommand(scroll, &driver.ZoomRequest{Scale: 1}, true)
	s.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemDelete, ID: 5}, {Kind: driver.ItemDelete, ID: 4}, {Kind: driver.ItemDelete, ID: 2},
	}})
	time.Sleep(200 * time.Millisecond)
}
