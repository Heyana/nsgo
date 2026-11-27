//go:build js && wasm

package web

import (
	"errors"
	"fmt"
	stdimage "image"
	"math"
	"strings"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvas2d"
	"github.com/nsgo-dev/nsgo/internal/platform/canvasmesh"
)

func (s *surface) renderDrawing(count uint32, req *driver.RenderDrawingRequest) {
	answer := func(img *stdimage.RGBA, err error) {
		s.engine.Dispatch(driver.RenderDrawingResult(count, img, err))
	}
	if req == nil {
		answer(nil, errors.New("web: no drawing"))
		return
	}
	width, height, err := driver.RenderPixelSize(req.Drawing, req.Size, req.Scale)
	if err != nil {
		answer(nil, fmt.Errorf("web: %w", err))
		return
	}
	sources, err := collectRenderImages(req.Drawing)
	if err != nil {
		answer(nil, err)
		return
	}
	r := &webDrawingRenderer{
		s:          s,
		images:     map[string]*renderWebImage{},
		offscreens: map[*driver.OffscreenCanvas]webOffscreenSurface{},
		visiting:   map[*driver.OffscreenCanvas]bool{},
	}
	finish := func() {
		defer r.release()
		img, err := r.render(req.Drawing, req.Size, req.Scale, width, height)
		answer(img, err)
	}
	if len(sources) == 0 {
		finish()
		return
	}
	pending := len(sources)
	done := false
	fail := func(msg string) {
		if done {
			return
		}
		done = true
		r.release()
		answer(nil, errors.New(msg))
	}
	for key, src := range sources {
		url, object := imageURL(src)
		if url == "" {
			fail("web: render image has no source")
			return
		}
		im := &renderWebImage{
			img:     js.Global().Get("Image").New(),
			url:     url,
			object:  object,
			density: float64(src.Density()),
		}
		r.images[key] = im
		onDone := func(ok bool) {
			im.releaseListeners()
			if done {
				return
			}
			if !ok {
				fail("web: render image failed to load")
				return
			}
			im.loaded = true
			pending--
			if pending == 0 {
				done = true
				finish()
			}
		}
		im.onload = js.FuncOf(func(js.Value, []js.Value) any { onDone(true); return nil })
		im.onerror = js.FuncOf(func(js.Value, []js.Value) any { onDone(false); return nil })
		im.img.Call("addEventListener", "load", im.onload)
		im.img.Call("addEventListener", "error", im.onerror)
		if len(src.Data) == 0 && (strings.HasPrefix(src.URI, "https:") || strings.HasPrefix(src.URI, "http:") || strings.HasPrefix(src.URI, "//")) {
			im.img.Set("crossOrigin", "anonymous")
		}
		im.img.Set("src", url)
	}
}

type renderWebImage struct {
	img     js.Value
	url     string
	object  bool
	density float64
	loaded  bool
	onload  js.Func
	onerror js.Func
}

func (im *renderWebImage) releaseListeners() {
	if !im.onload.Value.IsUndefined() {
		im.img.Call("removeEventListener", "load", im.onload)
		im.onload.Release()
		im.onload = js.Func{}
	}
	if !im.onerror.Value.IsUndefined() {
		im.img.Call("removeEventListener", "error", im.onerror)
		im.onerror.Release()
		im.onerror = js.Func{}
	}
}

func (im *renderWebImage) release() {
	im.releaseListeners()
	if im.img.Truthy() {
		im.img.Call("removeAttribute", "src")
	}
	if im.object && im.url != "" {
		js.Global().Get("URL").Call("revokeObjectURL", im.url)
		im.url, im.object = "", false
	}
}

type webOffscreenSurface struct {
	canvas js.Value
	scale  float64
}

type webDrawingRenderer struct {
	s          *surface
	images     map[string]*renderWebImage
	offscreens map[*driver.OffscreenCanvas]webOffscreenSurface
	visiting   map[*driver.OffscreenCanvas]bool
	err        error
}

func (r *webDrawingRenderer) release() {
	for _, im := range r.images {
		im.release()
	}
	for _, off := range r.offscreens {
		if off.canvas.Truthy() {
			off.canvas.Set("width", 0)
			off.canvas.Set("height", 0)
		}
	}
}

func collectRenderImages(d *driver.Drawing) (map[string]driver.ImageSource, error) {
	out := map[string]driver.ImageSource{}
	visiting := map[*driver.OffscreenCanvas]bool{}
	var walk func(*driver.Drawing) error
	walk = func(cur *driver.Drawing) error {
		if cur == nil {
			return nil
		}
		for i := range cur.Ops {
			src := cur.Image(&cur.Ops[i])
			if src == nil || src.Pixels != nil {
				continue
			}
			if src.Offscreen != nil {
				if visiting[src.Offscreen] {
					return errors.New("web: offscreen drawing contains a cycle")
				}
				visiting[src.Offscreen] = true
				err := walk(src.Offscreen.Drawing)
				delete(visiting, src.Offscreen)
				if err != nil {
					return err
				}
				continue
			}
			if key := src.Key(); key != "" {
				out[key] = *src
			}
		}
		return nil
	}
	if err := walk(d); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *webDrawingRenderer) render(d *driver.Drawing, size driver.Size, scale float64, width, height int) (img *stdimage.RGBA, err error) {
	defer func() {
		if failure := recover(); failure != nil {
			if _, ok := failure.(js.Error); !ok {
				panic(failure)
			}
			err = errors.New("web: cannot read rendered canvas pixels")
		}
	}()
	canvas := r.playCanvas(d, size, scale, width, height)
	if r.err != nil {
		return nil, r.err
	}
	defer func() {
		canvas.Set("width", 0)
		canvas.Set("height", 0)
	}()
	ctx := canvas.Call("getContext", "2d")
	data := ctx.Call("getImageData", 0, 0, width, height).Get("data")
	raw := make([]byte, width*height*4)
	js.CopyBytesToGo(raw, data)
	out := stdimage.NewRGBA(stdimage.Rect(0, 0, width, height))
	for i := 0; i < len(raw); i += 4 {
		a := uint32(raw[i+3])
		out.Pix[i+0] = uint8((uint32(raw[i+0])*a + 127) / 255)
		out.Pix[i+1] = uint8((uint32(raw[i+1])*a + 127) / 255)
		out.Pix[i+2] = uint8((uint32(raw[i+2])*a + 127) / 255)
		out.Pix[i+3] = uint8(a)
	}
	return out, nil
}

func (r *webDrawingRenderer) playCanvas(d *driver.Drawing, size driver.Size, scale float64, width, height int) js.Value {
	canvas := r.s.doc.Call("createElement", "canvas")
	canvas.Set("width", width)
	canvas.Set("height", height)
	ctx := canvas.Call("getContext", "2d")
	ctx.Call("scale", scale, scale)
	p := canvas2d.NewPlayer(ctx, scale, math.Hypot(float64(size.Width), float64(size.Height)))
	p.Drawing = d
	p.Font = fontCSS
	p.NewCanvas = r.s.scratchCanvas
	p.MeshCanvas = r.s.scratchMeshCanvas
	p.Image = func(src driver.ImageSource, _ bool) (js.Value, float64, bool) {
		return r.resolveImage(src)
	}
	p.Texture = func(src driver.ImageSource) canvasmesh.Texture {
		return r.resolveTexture(src)
	}
	for i := range d.Ops {
		p.Play(&d.Ops[i])
		if r.err != nil {
			break
		}
	}
	return canvas
}

func (r *webDrawingRenderer) resolveImage(src driver.ImageSource) (js.Value, float64, bool) {
	if r.err != nil {
		return js.Null(), 1, false
	}
	if src.Pixels != nil {
		im := r.s.canvasFrame(src)
		return im.img, im.density, im.loaded
	}
	if off := src.Offscreen; off != nil {
		if entry, ok := r.offscreens[off]; ok {
			return entry.canvas, entry.scale, true
		}
		if r.visiting[off] {
			r.err = errors.New("web: offscreen drawing contains a cycle")
			return js.Null(), 1, false
		}
		offScale := float64(off.EffectiveScale())
		pw, ph, err := driver.RenderPixelSize(off.Drawing, off.Size, offScale)
		if err != nil {
			r.err = fmt.Errorf("web: %w", err)
			return js.Null(), 1, false
		}
		r.visiting[off] = true
		canvas := r.playCanvas(off.Drawing, off.Size, offScale, pw, ph)
		delete(r.visiting, off)
		if r.err != nil {
			canvas.Set("width", 0)
			canvas.Set("height", 0)
			return js.Null(), 1, false
		}
		r.offscreens[off] = webOffscreenSurface{canvas: canvas, scale: offScale}
		return canvas, offScale, true
	}
	if im := r.images[src.Key()]; im != nil && im.loaded {
		return im.img, im.density, true
	}
	return js.Null(), 1, false
}

func (r *webDrawingRenderer) resolveTexture(src driver.ImageSource) canvasmesh.Texture {
	if src.Pixels != nil {
		return canvasmesh.RGBA(src.Pixels, src.Size)
	}
	img, density, ok := r.resolveImage(src)
	if !ok || !img.Truthy() {
		return canvasmesh.Texture{}
	}
	var w, h int
	if src.Offscreen != nil {
		w, h = img.Get("width").Int(), img.Get("height").Int()
	} else {
		w, h = img.Get("naturalWidth").Int(), img.Get("naturalHeight").Int()
	}
	if w <= 0 || h <= 0 {
		return canvasmesh.Texture{}
	}
	c := r.s.scratchCanvas(w, h)
	ctx := c.Call("getContext", "2d")
	ctx.Call("drawImage", img, 0, 0)
	raw := make([]byte, w*h*4)
	js.CopyBytesToGo(raw, ctx.Call("getImageData", 0, 0, w, h).Get("data"))
	c.Set("width", 0)
	c.Set("height", 0)
	pts := src.Size
	if pts.Width <= 0 || pts.Height <= 0 {
		if src.Offscreen != nil {
			pts = src.Offscreen.Size
		} else {
			if density <= 0 {
				density = 1
			}
			pts = driver.Size{Width: driver.Points(float64(w) / density), Height: driver.Points(float64(h) / density)}
		}
	}
	return canvasmesh.Texture{Pix: raw, Width: w, Height: h, Stride: w * 4, Points: pts}
}
