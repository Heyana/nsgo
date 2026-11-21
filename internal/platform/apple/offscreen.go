//go:build darwin

package apple

import (
	"sync"
	"sync/atomic"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	cgOffscreenBitmapCreate    func(data uintptr, w, h, bits, row uintptr, space uintptr, info uint32) uintptr
	cgBitmapContextCreateImage func(ctx uintptr) uintptr
)

func loadOffscreen(cg uintptr) {
	purego.RegisterLibFunc(&cgOffscreenBitmapCreate, cg, "CGBitmapContextCreate")
	purego.RegisterLibFunc(&cgBitmapContextCreateImage, cg, "CGBitmapContextCreateImage")
}

// maxOffscreenPixels bounds a single offscreen bitmap (16 Mi pixels = 64 MiB).
const maxOffscreenPixels = 1 << 24

type offscreenEntry struct {
	version       uint64
	image         uintptr
	width, height float64
	points        driver.Size
}

var (
	offscreenMu        sync.Mutex
	offscreenCache     = map[uint64]*offscreenEntry{}
	offscreenRendering = map[uint64]bool{}
	offscreenDropped   = map[uint64]bool{}
	offscreenRenders   atomic.Uint64
)

// ReleaseOffscreen drops the cached CGImage for offscreen resource id.
func ReleaseOffscreen(id uint64) {
	offscreenMu.Lock()
	if e := offscreenCache[id]; e != nil {
		delete(offscreenCache, id)
		Release(e.image)
	}
	if offscreenRendering[id] {
		offscreenDropped[id] = true
	}
	offscreenMu.Unlock()
}

// offscreenImage returns a retained CGImage and metadata for off, rendering
// off.Drawing only on first use or when off.Version changes. The caller must
// call Release on the returned image when non-zero.
func offscreenImage(src driver.ImageSource, host CanvasHost) (img uintptr, width, height float64, points driver.Size, waited bool) {
	off := src.Offscreen
	if off == nil {
		return 0, 0, 0, driver.Size{}, false
	}
	points = off.Size
	if src.Size.Width > 0 && src.Size.Height > 0 {
		points = src.Size
	}

	offscreenMu.Lock()
	if e := offscreenCache[off.ID]; e != nil && e.version == off.Version {
		cfRetain(e.image)
		img, width, height = e.image, e.width, e.height
		offscreenMu.Unlock()
		return img, width, height, points, false
	}
	if offscreenRendering[off.ID] {
		offscreenMu.Unlock()
		return 0, 0, 0, driver.Size{}, false
	}
	offscreenRendering[off.ID] = true
	offscreenMu.Unlock()

	defer func() {
		offscreenMu.Lock()
		delete(offscreenRendering, off.ID)
		delete(offscreenDropped, off.ID)
		offscreenMu.Unlock()
	}()

	pw, ph := off.PixelSize()
	if pw <= 0 || ph <= 0 || pw*ph > maxOffscreenPixels {
		return 0, 0, 0, driver.Size{}, false
	}
	const premultipliedFirst, byteOrder32Little = 2, 2 << 12
	ctx := cgOffscreenBitmapCreate(0, uintptr(pw), uintptr(ph), 8, uintptr(pw*4), srgb, premultipliedFirst|byteOrder32Little)
	if ctx == 0 {
		return 0, 0, 0, driver.Size{}, false
	}
	scale := float64(off.EffectiveScale())
	// Top down in logical points; the bitmap context's base space points up.
	cgContextTranslateCTM(ctx, 0, float64(ph))
	cgContextScaleCTM(ctx, scale, -scale)
	var ops []driver.DrawOp
	if off.Drawing != nil {
		ops = off.Drawing.Ops
	}
	subHost := CanvasHost{Ratio: scale, Redraw: host.Redraw, Owner: host.Owner, BaseUp: true}
	p := newPlayer(ctx, ops, driver.CanvasSplit{}, subHost)
	p.run()
	offscreenRenders.Add(1)
	img = cgBitmapContextCreateImage(ctx)
	Release(ctx)
	if img == 0 {
		return 0, 0, 0, driver.Size{}, p.waited
	}
	width, height = float64(pw), float64(ph)
	if !p.waited {
		offscreenMu.Lock()
		if !offscreenDropped[off.ID] {
			if old := offscreenCache[off.ID]; old != nil {
				Release(old.image)
			}
			cfRetain(img)
			offscreenCache[off.ID] = &offscreenEntry{
				version: off.Version,
				image:   img,
				width:   width,
				height:  height,
				points:  off.Size,
			}
		}
		offscreenMu.Unlock()
	}
	return img, width, height, points, p.waited
}

// offscreenDecoded renders (or retrieves from cache) src's offscreen canvas as
// a decoded image for ImageView, applying blur if requested.
func offscreenDecoded(src driver.ImageSource, r request) decoded {
	img, w, h, points, _ := offscreenImage(src, CanvasHost{Ratio: r.ratio})
	if img == 0 {
		return decoded{}
	}
	scale := float64(src.Offscreen.EffectiveScale())
	if r.blur > 0 {
		radius := r.blur * r.ratio
		if ppp := (request{mode: r.mode, view: r.view, scale: scale}).pointsPerPixel(w, h); ppp > 0 {
			radius = r.blur / ppp
		}
		if out := blur(img, radius); out != 0 {
			Release(img)
			img = out
		}
	}
	return decoded{image: img, width: w, height: h, scale: scale, points: points}
}
