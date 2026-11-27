//go:build linux && !android

package linux

import (
	"context"
	"errors"
	"fmt"
	"image"
	"sync"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

// PlatformTextEngine returns Pango's text engine without opening a GTK display,
// or nil when Pango and Cairo libraries are unavailable.
func PlatformTextEngine() driver.TextEngine {
	if err := openCanvasLibs(); err != nil {
		return nil
	}
	return &headlessTextEngine{gen: fontGeneration.Load()}
}

type headlessTextEngine struct {
	mu  sync.Mutex
	w   window
	gen uint64
}

func (e *headlessTextEngine) Prepare(t *driver.AttributedText) driver.PreparedText {
	e.mu.Lock()
	defer e.mu.Unlock()
	if g := fontGeneration.Load(); g != e.gen {
		e.w.releaseTextContexts()
		clear(e.w.extents)
		e.gen = g
	}
	return &headlessPreparedText{e: e, p: e.w.Prepare(t)}
}

type headlessPreparedText struct {
	e *headlessTextEngine
	p driver.PreparedText
}

func (p *headlessPreparedText) Measure(maxWidth driver.Points) driver.Size {
	p.e.mu.Lock()
	defer p.e.mu.Unlock()
	return p.p.Measure(maxWidth)
}

func (p *headlessPreparedText) Layout(size driver.Size) driver.TextLayout {
	p.e.mu.Lock()
	defer p.e.mu.Unlock()
	return p.p.Layout(size)
}

func (p *headlessPreparedText) Release() {
	p.e.mu.Lock()
	defer p.e.mu.Unlock()
	p.p.Release()
}

func (p *canvasPlayer) canvasImage(src driver.ImageSource) *imageSlot {
	if p.resolveImage != nil {
		return p.resolveImage(src)
	}
	if p.w == nil {
		return nil
	}
	return p.w.canvasImage(src, p.v)
}

func (p *canvasPlayer) offscreenSurface(off *driver.OffscreenCanvas) *offscreenEntry {
	if p.resolveOffscreen != nil {
		return p.resolveOffscreen(off)
	}
	if p.w == nil {
		return nil
	}
	return p.w.offscreenSurface(p.v, off)
}

// RenderDrawing plays a recording into owned premultiplied RGBA pixels using
// Cairo and Pango without opening a window or GTK display.
func RenderDrawing(d *driver.Drawing, size driver.Size, scale float64) (*image.RGBA, error) {
	if err := openCanvasLibs(); err != nil {
		return nil, fmt.Errorf("linux: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fontconfig.mu.Lock()
	textCtx := newTextContext(false)
	fontconfig.mu.Unlock()
	defer gObjectUnref(textCtx)

	r := &drawingRenderer{
		ctx:        ctx,
		textCtx:    textCtx,
		pics:       map[string]*picture{},
		images:     map[renderImageKey]*imageSlot{},
		offscreens: map[*driver.OffscreenCanvas]*offscreenEntry{},
		visiting:   map[*driver.OffscreenCanvas]bool{},
	}
	defer r.release()

	surf, width, height, err := r.renderSurface(d, size, scale)
	if err != nil {
		return nil, err
	}
	defer cairoSurfaceDestroy(surf)
	cairoSurfaceFlush(surf)
	data := cairoImageSurfaceGetData(surf)
	stride := int(cairoImageSurfaceGetStride(surf))
	if data == 0 || stride < width*4 {
		return nil, errors.New("linux: render surface has no pixels")
	}
	rows := unsafe.Slice((*byte)(cPointer(data)), stride*height)
	return driver.BGRAtoRGBA(rows, width, height, stride), nil
}

type renderImageKey struct {
	source string
	size   driver.Size
}

type drawingRenderer struct {
	ctx        context.Context
	textCtx    uintptr
	pics       map[string]*picture
	images     map[renderImageKey]*imageSlot
	offscreens map[*driver.OffscreenCanvas]*offscreenEntry
	visiting   map[*driver.OffscreenCanvas]bool
	err        error
}

func (r *drawingRenderer) release() {
	for _, s := range r.images {
		s.reset(driver.ImageSource{})
	}
	for _, e := range r.offscreens {
		e.destroy()
	}
}

func (r *drawingRenderer) renderSurface(d *driver.Drawing, size driver.Size, scale float64) (uintptr, int, int, error) {
	width, height, err := driver.RenderPixelSize(d, size, scale)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("linux: %w", err)
	}
	surf := cairoImageSurfaceCreate(0, int32(width), int32(height))
	if surf == 0 {
		return 0, 0, 0, errors.New("linux: cannot create the render surface")
	}
	cr := cairoCreate(surf)
	cairoScale(cr, scale, scale)
	p := &canvasPlayer{
		drawing:          d,
		cr:               cr,
		size:             rect{w: float64(size.Width), h: float64(size.Height)},
		scale:            scale,
		textContext:      r.textCtx,
		resolveImage:     r.image,
		resolveOffscreen: r.offscreen,
		st:               canvasState{fill: driver.RGB(0x000000), stroke: driver.RGB(0x000000), width: 1, miter: 10, alpha: 1},
	}
	cairoGetMatrix(cr, &p.base)
	cairoNewPath(cr)
	for i := range d.Ops {
		p.play(&d.Ops[i])
		if r.err != nil {
			break
		}
	}
	for range p.stack {
		cairoRestore(cr)
	}
	cairoDestroy(cr)
	if r.err != nil {
		cairoSurfaceDestroy(surf)
		return 0, 0, 0, r.err
	}
	return surf, width, height, nil
}

func (r *drawingRenderer) image(src driver.ImageSource) *imageSlot {
	if r.err != nil {
		return nil
	}
	if src.Pixels != nil {
		key := renderImageKey{source: fmt.Sprintf("px:%p:%d", src.Pixels, src.Version), size: src.Size}
		if s := r.images[key]; s != nil {
			return s
		}
		s := &imageSlot{}
		s.showPixels(src)
		if s.pic == nil {
			r.err = errors.New("linux: cannot render pixel image")
			return nil
		}
		r.images[key] = s
		return s
	}
	srcKey := src.Key()
	slotKey := renderImageKey{source: srcKey, size: src.Size}
	if s := r.images[slotKey]; s != nil {
		return s
	}
	pic := r.pics[srcKey]
	if pic == nil {
		data, err := imageload.Load(r.ctx, src, nil)
		if err == nil {
			pic, err = decode(r.ctx, data)
		}
		if err != nil {
			r.err = fmt.Errorf("linux: render image: %w", err)
			return nil
		}
		if pic == nil {
			r.err = errors.New("linux: cannot render image")
			return nil
		}
		r.pics[srcKey] = pic
	}
	s := &imageSlot{src: src, pic: pic}
	r.images[slotKey] = s
	return s
}

func (r *drawingRenderer) offscreen(off *driver.OffscreenCanvas) *offscreenEntry {
	if r.err != nil || off == nil {
		return nil
	}
	if e := r.offscreens[off]; e != nil {
		return e
	}
	if r.visiting[off] {
		r.err = errors.New("linux: offscreen drawing contains a cycle")
		return nil
	}
	r.visiting[off] = true
	surf, width, height, err := r.renderSurface(off.Drawing, off.Size, float64(off.EffectiveScale()))
	delete(r.visiting, off)
	if err != nil {
		r.err = err
		return nil
	}
	cairoSurfaceFlush(surf)
	e := &offscreenEntry{
		version: off.Version,
		surface: surf,
		width:   width,
		height:  height,
		points:  off.Size,
	}
	r.offscreens[off] = e
	return e
}

func (w *window) renderDrawing(count uint32, req *driver.RenderDrawingRequest) {
	if req == nil {
		w.engine.Dispatch(driver.RenderDrawingResult(count, nil, errors.New("linux: no drawing")))
		return
	}
	go func() {
		img, err := RenderDrawing(req.Drawing, req.Size, req.Scale)
		w.engine.Dispatch(driver.RenderDrawingResult(count, img, err))
	}()
}
