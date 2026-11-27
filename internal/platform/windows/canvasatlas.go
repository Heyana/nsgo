//go:build windows

package windows

import (
	"math"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type atlasRectU struct{ left, top, right, bottom uint32 }

type atlasBatchSprite struct {
	dest   rectF
	source atlasRectU
	tint   colorF
}

func (c *canvasPlayer) atlas(op *driver.DrawOp) {
	imgSrc := c.drawing.Image(op)
	sprites := c.drawing.AtlasSprites(op)
	if imgSrc == nil || len(sprites) == 0 {
		return
	}
	if imgSrc.Offscreen != nil {
		c.atlasImages(op, sprites)
		return
	}
	s := c.p.w.canvasImage(*imgSrc)
	if s == nil || s.pic == nil {
		c.waited = c.waited || (s != nil && s.load != nil)
		return
	}
	bitmap := s.bitmapOf(c.p.dc, 0)
	if bitmap == nil {
		return
	}
	scale := float32(s.pic.width) / max(float32(natural(s).Width), 1e-3)
	if c.batchAtlas(bitmap, sprites, scale, s.pic.width, s.pic.height, op.Sampling) {
		return
	}
	for _, sprite := range sprites {
		source, dest := atlasRects(sprite, scale)
		transform := atlasMatrix(sprite.Transform).then(c.st.ctm)
		c.paintBitmap(bitmap, source, dest, c.st.alpha*sprite.Alpha, transform, op.Sampling)
	}
}

// atlasImages draws each sprite as the DrawImageFrom it stands for, for sources the sprite batch does not cover: offscreen drawings.
func (c *canvasPlayer) atlasImages(op *driver.DrawOp, sprites driver.AtlasSprites) {
	ctm, alpha := c.st.ctm, c.st.alpha
	for _, sprite := range sprites {
		b := sprite.Rect
		c.st.ctm, c.st.alpha = atlasMatrix(sprite.Transform).then(ctm), alpha*sprite.Alpha
		w, h := float32(b.Size.Width), float32(b.Size.Height)
		c.image(&driver.DrawOp{Kind: driver.DrawImage, ImageIndex: op.ImageIndex, Sampling: op.Sampling, W: w, H: h,
			X1: float32(b.Origin.X), Y1: float32(b.Origin.Y), X2: w, Y2: h})
	}
	c.st.ctm, c.st.alpha = ctm, alpha
}

func atlasMatrix(t driver.RSXform) matrix {
	scos, ssin := float64(t.SCos), float64(t.SSin)
	return matrix{scos, ssin, -ssin, scos, float64(t.TX), float64(t.TY)}
}

func atlasRects(sprite driver.AtlasSprite, scale float32) (source, dest rectF) {
	r := sprite.Rect
	w, h := float32(r.Size.Width), float32(r.Size.Height)
	return canvasImageCrop(float32(r.Origin.X), float32(r.Origin.Y), w, h, scale), rectF{0, 0, w, h}
}

func (c *canvasPlayer) batchAtlas(bitmap *comObject, sprites driver.AtlasSprites, scale float32, width, height int, sampling driver.ImageSampling) bool {
	st := c.st
	sh := st.shadow
	if st.composite != driver.CompositeSourceOver || visible(sh.Color) && (sh.X != 0 || sh.Y != 0 || sh.R != 0) {
		return false
	}
	dc := c.p.dc.query(&iidD2D1DeviceContext3)
	if dc == nil {
		return false
	}
	defer dc.release()

	spritesForD2D := make([]atlasBatchSprite, 0, len(sprites))
	canvas := st.ctm.then(c.world)
	hasOpacity := false
	for _, sprite := range sprites {
		native, ok := nativeAtlasSprite(sprite, canvas, scale, c.p.scale, width, height)
		if !ok {
			return false
		}
		opacity := st.alpha * sprite.Alpha
		native.tint = colorF{1, 1, 1, opacity}
		hasOpacity = hasOpacity || opacity != 1
		spritesForD2D = append(spritesForD2D, native)
	}
	var batch *comObject
	if failed(dc.call(dcCreateSpriteBatch, uintptr(unsafe.Pointer(&batch)))) || batch == nil {
		return false
	}
	defer batch.release()
	first := &spritesForD2D[0]
	stride := unsafe.Sizeof(*first)
	var tint *colorF
	if hasOpacity {
		tint = &first.tint
	}
	if failed(batch.call(spriteBatchAdd, uintptr(len(spritesForD2D)),
		uintptr(unsafe.Pointer(&first.dest)), uintptr(unsafe.Pointer(&first.source)), uintptr(unsafe.Pointer(tint)), 0,
		stride, stride, stride, 0)) {
		return false
	}
	c.p.setTransform(identity)
	aa := dc.call(rtGetAntialiasMode)
	dc.call(rtSetAntialiasMode, d2dAntialiasAliased)
	dc.call(dcDrawSpriteBatch, batch.ptr(), 0, uintptr(len(spritesForD2D)), bitmap.ptr(), imageInterpolation(sampling), 0)
	dc.call(rtSetAntialiasMode, aa)
	c.p.setTransform(c.world)
	return true
}

func nativeAtlasSprite(sprite driver.AtlasSprite, canvas matrix, scale, dpiScale float32, width, height int) (atlasBatchSprite, bool) {
	source, dest := atlasRects(sprite, scale)
	if !(source.left >= 0 && source.top >= 0 && source.right <= float32(width) && source.bottom <= float32(height) &&
		source.left < source.right && source.top < source.bottom && integralAtlasRect(source, 1)) {
		return atlasBatchSprite{}, false
	}
	transform := atlasMatrix(sprite.Transform).then(canvas)
	if !transform.axisAligned() {
		return atlasBatchSprite{}, false
	}
	m := transform.d2d()
	dest = rectF{m.dx, m.dy, m.dx + m.m11*dest.right, m.dy + m.m22*dest.bottom}
	if !integralAtlasRect(dest, dpiScale) || dest.left >= dest.right || dest.top >= dest.bottom ||
		(dest.right-dest.left)*dpiScale > source.right-source.left || (dest.bottom-dest.top)*dpiScale > source.bottom-source.top {
		// The batch rasterizer is aliased. Upscaling a crop can also
		// sample neighboring atlas entries differently from DrawBitmap.
		return atlasBatchSprite{}, false
	}
	return atlasBatchSprite{dest: dest, source: atlasRectU{uint32(source.left), uint32(source.top), uint32(source.right), uint32(source.bottom)}}, true
}

func integralAtlasRect(r rectF, scale float32) bool {
	return atlasPixel(r.left*scale) && atlasPixel(r.top*scale) && atlasPixel(r.right*scale) && atlasPixel(r.bottom*scale)
}

func atlasPixel(v float32) bool {
	f := float64(v)
	return !math.IsInf(f, 0) && f == math.Trunc(f)
}

// atlasListTransform permits replaying a command list containing aliased
// sprites only when the view preserves the recording's device pixel grid.
func atlasListTransform(m matrix, scale float32) bool {
	native := m.d2d()
	return native.m11 == 1 && native.m12 == 0 && native.m21 == 0 && native.m22 == 1 &&
		atlasPixel(native.dx*scale) && atlasPixel(native.dy*scale)
}
