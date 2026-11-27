//go:build js && wasm

package web

import (
	"math"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvas2d"
	"github.com/nsgo-dev/nsgo/internal/platform/canvasmesh"
)

const maxOffscreenPixels = 1 << 24

type offscreenEntry struct {
	version uint64
	canvas  js.Value
	scale   float64
	// texture is canvas's pixels, read the first time a mesh samples this version.
	texture     canvasmesh.Texture
	readTexture bool
}

func (s *surface) releaseOffscreen(id uint64) {
	if e := s.offscreen[id]; e != nil {
		if e.canvas.Truthy() {
			// Resetting dimensions lets the browser release the backing buffer promptly.
			e.canvas.Set("width", 0)
			e.canvas.Set("height", 0)
		}
		delete(s.offscreen, id)
	}
}

// offscreenCanvas returns the cached offscreen <canvas> and backing density
// for off, rendering off.Drawing via canvas2d only on first use or when
// off.Version changes.
func (s *surface) offscreenCanvas(off *driver.OffscreenCanvas, v *view) (js.Value, float64, bool) {
	if off == nil {
		return js.Null(), 1, false
	}
	if e := s.offscreen[off.ID]; e != nil && e.version == off.Version && e.canvas.Truthy() {
		return e.canvas, e.scale, true
	}
	if s.offscreenRendering[off.ID] {
		return js.Null(), 1, false
	}
	pw, ph := off.PixelSize()
	if pw <= 0 || ph <= 0 || pw*ph > maxOffscreenPixels {
		return js.Null(), 1, false
	}
	if s.offscreenRendering == nil {
		s.offscreenRendering = map[uint64]bool{}
	}
	s.offscreenRendering[off.ID] = true
	defer delete(s.offscreenRendering, off.ID)

	var canvas js.Value
	if old := s.offscreen[off.ID]; old != nil && old.canvas.Truthy() {
		canvas = old.canvas
	} else {
		canvas = s.doc.Call("createElement", "canvas")
	}
	// Setting width/height clears the canvas and resets its 2D context state.
	canvas.Set("width", pw)
	canvas.Set("height", ph)
	ctx := canvas.Call("getContext", "2d")
	scale := float64(off.EffectiveScale())
	ctx.Call("scale", scale, scale)

	waited := false
	p := canvas2d.NewPlayer(ctx, scale, math.Hypot(float64(off.Size.Width), float64(off.Size.Height)))
	p.Drawing = off.Drawing
	p.Font = fontCSS
	p.NewCanvas = s.scratchCanvas
	p.Image = func(src driver.ImageSource, readable bool) (js.Value, float64, bool) {
		im := s.loadCanvasImage(src, v, readable)
		if !im.loaded {
			waited = true
		}
		return im.img, im.density, im.loaded
	}
	p.MeshCanvas = s.scratchMeshCanvas
	p.Texture = func(src driver.ImageSource) canvasmesh.Texture {
		tex, ok := s.meshTexture(src, v)
		waited = waited || !ok
		return tex
	}
	if off.Drawing != nil {
		for i := range off.Drawing.Ops {
			p.Play(&off.Drawing.Ops[i])
		}
	}

	if s.offscreen == nil {
		s.offscreen = map[uint64]*offscreenEntry{}
	}
	version := off.Version
	if waited {
		// Re-render on the next paint once inner async images finish loading.
		version = 0
	}
	s.offscreen[off.ID] = &offscreenEntry{
		version: version,
		canvas:  canvas,
		scale:   scale,
	}
	return canvas, scale, true
}
