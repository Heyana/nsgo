//go:build windows

package windows

import (
	"context"
	"errors"
	"fmt"
	"image"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

// TextEngine shapes and measures text with DirectWrite without a window.
type TextEngine struct{}

// Prepare implements driver.TextEngine.
func (TextEngine) Prepare(t *driver.AttributedText) driver.PreparedText {
	return (*window)(nil).Prepare(t)
}

// PlatformTextEngine returns DirectWrite's text engine when available, or nil.
func PlatformTextEngine() driver.TextEngine {
	if dwriteFactory() == nil {
		return nil
	}
	return TextEngine{}
}

// RegisterFont registers font data with DirectWrite for the process.
func RegisterFont(data []byte) error {
	if len(data) == 0 {
		return errors.New("windows: empty font data")
	}
	return addFonts(data)
}

func (p *painter) canvasImage(src driver.ImageSource) *imageSlot {
	if p.resolveImage != nil {
		return p.resolveImage(src)
	}
	if p.w == nil {
		return nil
	}
	return p.w.canvasImage(src)
}

// RenderDrawing plays a recording into owned premultiplied RGBA pixels without
// opening a window. Its synchronous image resolver never touches window caches.
func RenderDrawing(d *driver.Drawing, size driver.Size, scale float64) (*image.RGBA, error) {
	width, height, err := driver.RenderPixelSize(d, size, scale)
	if err != nil {
		return nil, fmt.Errorf("windows: %w", err)
	}
	g, err := newGfx()
	if err != nil {
		return nil, fmt.Errorf("windows: %w", err)
	}
	defer g.release()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r := &drawingRenderer{
		ctx:        ctx,
		g:          g,
		pics:       map[string]*picture{},
		images:     map[renderImageKey]*imageSlot{},
		offscreens: map[*driver.OffscreenCanvas]*offscreenEntry{},
		visiting:   map[*driver.OffscreenCanvas]bool{},
	}
	defer r.release()

	fscale := float32(scale)
	props := bitmapProperties1{
		format:    dxgiFormatBGRA,
		alphaMode: d2dAlphaPremultiplied,
		dpiX:      96 * fscale,
		dpiY:      96 * fscale,
		options:   d2dBitmapTarget,
	}
	var target *comObject
	if hr := g.dc.callSize(dcCreateBitmap, uint32(width), uint32(height), 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&target))); failed(hr) || target == nil {
		return nil, hresultError("windows: cannot create the render bitmap", hr)
	}
	defer target.release()

	g.dc.call(dcSetTarget, target.ptr())
	g.dc.callF(rtSetDpi, 0b11, f32(96*fscale), f32(96*fscale))
	g.dc.call(rtBeginDraw)
	clearCol := colorF{}
	g.dc.call(rtClear, uintptr(unsafe.Pointer(&clearCol)))

	p := &painter{
		g:                g,
		dc:               g.dc,
		scale:            fscale,
		m:                identity,
		alpha:            1,
		targetWidth:      uint32(width),
		targetHeight:     uint32(height),
		targetAlpha:      true,
		resolveImage:     r.image,
		resolveOffscreen: r.offscreen,
	}
	black := colorF{a: 1}
	g.dc.call(rtCreateSolidColorBrush, uintptr(unsafe.Pointer(&black)), 0, uintptr(unsafe.Pointer(&p.brush)))
	r.p = p
	p.setTransform(identity)

	c := p.newCanvasPlayer(nil, d, identity, float32(size.Width), float32(size.Height))
	c.off, c.pixelScale = target, fscale
	c.playAll(d.Ops)
	p.release()

	hr := g.dc.call(rtEndDraw, 0, 0)
	g.dc.call(dcSetTarget, 0)
	if r.err != nil {
		return nil, r.err
	}
	if failed(hr) {
		return nil, hresultError("windows: render EndDraw", hr)
	}
	bgra := bitmapPixels(g.dc, target, uint32(width), uint32(height), fscale)
	if bgra == nil {
		return nil, errors.New("windows: cannot read render bitmap pixels")
	}
	return driver.BGRAtoRGBA(bgra, width, height, width*4), nil
}

type renderImageKey struct {
	source string
	size   driver.Size
}

type drawingRenderer struct {
	ctx        context.Context
	g          *gfx
	p          *painter
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
		e.release()
	}
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
			r.err = errors.New("windows: cannot render pixel image")
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
			r.err = fmt.Errorf("windows: render image: %w", err)
			return nil
		}
		if pic == nil || len(pic.frames) == 0 {
			r.err = errors.New("windows: cannot render image")
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
		r.err = errors.New("windows: offscreen drawing contains a cycle")
		return nil
	}
	scale := float64(off.EffectiveScale())
	pw, ph, err := driver.RenderPixelSize(off.Drawing, off.Size, scale)
	if err != nil {
		r.err = fmt.Errorf("windows: %w", err)
		return nil
	}
	fscale := float32(scale)
	props := bitmapProperties1{
		format:    dxgiFormatBGRA,
		alphaMode: d2dAlphaPremultiplied,
		dpiX:      96 * fscale,
		dpiY:      96 * fscale,
		options:   d2dBitmapTarget,
	}
	var bmp *comObject
	if hr := r.g.dc.callSize(dcCreateBitmap, uint32(pw), uint32(ph), 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&bmp))); failed(hr) || bmp == nil {
		r.err = hresultError("windows: cannot create nested offscreen bitmap", hr)
		return nil
	}
	r.visiting[off] = true
	p := r.p
	world := p.m
	oldScale := p.scale
	p.flatten(func() {
		var prevTarget *comObject
		p.dc.call(dcGetTarget, uintptr(unsafe.Pointer(&prevTarget)))
		p.dc.call(dcSetTarget, bmp.ptr())
		p.dc.callF(rtSetDpi, 0b11, f32(96*fscale), f32(96*fscale))
		pushed, split := p.pushed, p.split
		p.pushed, p.split = nil, nil
		p.scale = fscale
		p.setTransform(identity)
		clearCol := colorF{}
		p.dc.call(rtClear, uintptr(unsafe.Pointer(&clearCol)))
		c := p.newCanvasPlayer(nil, off.Drawing, identity, float32(off.Size.Width), float32(off.Size.Height))
		c.off, c.pixelScale = bmp, fscale
		c.playAll(off.Drawing.Ops)
		for len(p.pushed) > 0 {
			p.pop()
		}
		p.pushed, p.split = pushed, split
		p.scale = oldScale
		p.dc.call(dcSetTarget, prevTarget.ptr())
		p.dc.callF(rtSetDpi, 0b11, f32(96*oldScale), f32(96*oldScale))
		prevTarget.release()
	})
	delete(r.visiting, off)
	p.setTransform(world)
	if r.err != nil {
		bmp.release()
		return nil
	}
	e := &offscreenEntry{
		version:    off.Version,
		generation: gfxGeneration,
		bitmap:     bmp,
		scale:      fscale,
		points:     off.Size,
		width:      uint32(pw),
		height:     uint32(ph),
	}
	r.offscreens[off] = e
	return e
}

func (w *window) renderDrawing(count uint32, req *driver.RenderDrawingRequest) {
	if req == nil {
		w.engine.Dispatch(driver.RenderDrawingResult(count, nil, errors.New("windows: no drawing")))
		return
	}
	go func() {
		img, err := RenderDrawing(req.Drawing, req.Size, req.Scale)
		w.engine.Dispatch(driver.RenderDrawingResult(count, img, err))
	}()
}
