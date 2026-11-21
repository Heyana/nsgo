//go:build darwin

package apple

import (
	"math"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvasmesh"
)

func (p *player) vertices(op *driver.DrawOp) {
	if op.Image == nil {
		return
	}
	if draw, ok := op.ImageRect(); ok {
		p.image(&draw)
		return
	}
	var texture canvasmesh.Texture
	if src := *op.Image; src.Pixels != nil {
		texture = canvasmesh.RGBA(src.Pixels, src.Size)
	} else {
		im := canvasImage(src, p.host)
		if im == nil || im.image == 0 {
			p.waited = p.waited || im == nil || !im.failed
			return
		}
		texture = im.meshTexture()
	}
	if len(texture.Pix) == 0 {
		return
	}
	m := cgContextGetCTM(p.ctx)
	pix := canvasmesh.Raster(op, texture, max(math.Hypot(m.A, m.B), math.Hypot(m.C, m.D)))
	if pix == nil {
		return
	}
	img := rgbaImage(pix.Pix, pix.Rect.Dx(), pix.Rect.Dy())
	if img != 0 {
		defer Release(img)
		p.drawImage(img, float64(op.X), float64(op.Y), float64(op.W), float64(op.H), op.Sampling)
	}
}

func (im *loadedImage) meshTexture() canvasmesh.Texture {
	w, h := int(im.width), int(im.height)
	if im.meshPixels == nil && w > 0 && h > 0 {
		bitmapsLoaded.Do(loadBitmaps)
		const premultipliedFirst, byteOrder32Little = 2, 2 << 12
		ctx := cgBitmapContextCreate(0, uintptr(w), uintptr(h), 8, uintptr(w*4), srgb, premultipliedFirst|byteOrder32Little)
		if ctx != 0 {
			defer Release(ctx)
			cgContextDrawImage(ctx, Rect(0, 0, float64(w), float64(h)), im.image)
			data := cgBitmapContextGetData(ctx)
			if data != 0 {
				im.meshPixels = append([]byte(nil), unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), w*h*4)...)
			}
		}
	}
	return canvasmesh.Texture{Pix: im.meshPixels, Width: w, Height: h, Stride: w * 4, Points: im.points, BGRA: true}
}
