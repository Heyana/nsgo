//go:build darwin

package apple

import (
	"encoding/binary"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// CoreGraphics bitmap and color-space inspection calls used by the fast
// DrawAtlas blitter. Pointer/integer calls go through purego.SyscallN so
// checking the context on every frame allocates nothing.
var (
	cgBitmapData        func(ctx uintptr) uintptr
	cgBitmapWidth       func(ctx uintptr) uintptr
	cgBitmapHeight      func(ctx uintptr) uintptr
	cgBitmapBytesPerRow func(ctx uintptr) uintptr
	cgBitmapBPC         func(ctx uintptr) uintptr
	cgBitmapBPP         func(ctx uintptr) uintptr
	cgBitmapInfo        func(ctx uintptr) uint32
	cgBitmapColorSpace  func(ctx uintptr) uintptr
	cgColorSpaceName    func(cs uintptr) uintptr
	cgColorSpaceModel   func(cs uintptr) int32
	cfEqual             func(a, b uintptr) bool
	cgSRGBName          uintptr
	cgDeviceRGBName     uintptr
)

func loadAtlas(cf, cg uintptr) {
	registerSyscall(&cgBitmapData, cg, "CGBitmapContextGetData")
	registerSyscall(&cgBitmapWidth, cg, "CGBitmapContextGetWidth")
	registerSyscall(&cgBitmapHeight, cg, "CGBitmapContextGetHeight")
	registerSyscall(&cgBitmapBytesPerRow, cg, "CGBitmapContextGetBytesPerRow")
	registerSyscall(&cgBitmapBPC, cg, "CGBitmapContextGetBitsPerComponent")
	registerSyscall(&cgBitmapBPP, cg, "CGBitmapContextGetBitsPerPixel")
	registerSyscall(&cgBitmapColorSpace, cg, "CGBitmapContextGetColorSpace")
	registerSyscall(&cgColorSpaceName, cg, "CGColorSpaceGetName")

	infoSym := symbol(cg, "CGBitmapContextGetBitmapInfo")
	cgBitmapInfo = func(ctx uintptr) uint32 {
		r, _, _ := purego.SyscallN(infoSym, ctx)
		return uint32(r)
	}
	modelSym := symbol(cg, "CGColorSpaceGetModel")
	cgColorSpaceModel = func(cs uintptr) int32 {
		r, _, _ := purego.SyscallN(modelSym, cs)
		return int32(r)
	}
	equalSym := symbol(cf, "CFEqual")
	cfEqual = func(a, b uintptr) bool {
		r, _, _ := purego.SyscallN(equalSym, a, b)
		return byte(r) != 0
	}
	if cgContextGetClipBoundingBox == nil {
		purego.RegisterLibFunc(&cgContextGetClipBoundingBox, cg, "CGContextGetClipBoundingBox")
	}
	cgSRGBName = constant(cg, "kCGColorSpaceSRGB")
	cgDeviceRGBName = CFString("kCGColorSpaceDeviceRGB")
}

// atlasScratchMinSprites is the minimum sprite count for the Tier 2 scratch
// bitmap path (used for async display-list layers, clipped states, and wide
// color-space contexts). Below this threshold—where a few sparse sprites can
// span a full-screen bounding box—CoreGraphics' per-sprite cached crops avoid
// uploading a large scratch image per frame.
const atlasScratchMinSprites = 64

// atlasMaxScratchSide caps one side of a Tier 2 scratch bitmap.
const atlasMaxScratchSide = 4096

// atlasFast composites op.Atlas in pure Go instead of issuing one CoreGraphics
// CGContextDrawImage call per sprite.
//
// Tier 1 writes directly into CGBitmapContextGetData(p.ctx) with zero per-sprite
// FFI calls, zero image copies, and zero heap allocations when the destination is
// a compatible 8-bit sRGB/DeviceRGB bitmap context without an active canvas clip.
//
// Tier 2 renders all sprites into one tight-bounds premultiplied BGRA bitmap and
// draws it once in device space through CoreGraphics, reducing display-list
// layers (CALayer drawsAsynchronously), clipped drawings, and wide-gamut contexts
// from O(N) CGImage draws to one.
// Test hooks for comparing Tier 1 / Tier 2 against the CoreGraphics reference path.
var (
	disableAtlasFast  bool
	forceAtlasScratch bool
)

func (p *player) atlasFast(op *driver.DrawOp, imgSrc *driver.ImageSource, sprites driver.AtlasSprites, im *loadedImage) bool {
	if disableAtlasFast {
		return false
	}
	st := p.st()
	if st.shadow || st.composite != driver.CompositeSourceOver {
		return false
	}
	if len(sprites) == 0 {
		return true
	}
	if st.alpha <= 0 {
		return true
	}

	ctm := cgContextGetCTM(p.ctx)
	det := ctm.A*ctm.D - ctm.B*ctm.C
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return true
	}

	// Tier 1: direct write into a compatible CGBitmapContext without canvas clip.
	if !st.clipped && !forceAtlasScratch {
		if dstPix, dstW, dstH, dstStride, dstBGRA, ok := atlasBitmapTarget(p.ctx); ok {
			srcPix, srcW, srcH, srcStride, points, srcBGRA := atlasSourcePixels(imgSrc, im)
			if len(srcPix) == 0 {
				return false
			}
			clipX0, clipY0, clipX1, clipY1, empty := atlasClipInTarget(p.ctx, ctm, 0, dstH, dstW, dstH)
			if empty {
				return true
			}
			return blitAtlasInto(dstPix, dstStride, dstBGRA, 0, dstH, clipX0, clipY0, clipX1, clipY1,
				ctm, st.alpha, op.Sampling == driver.ImageSamplingNearest,
				srcPix, srcW, srcH, srcStride, points, srcBGRA, sprites)
		}
	}

	// Tier 2: single-blit scratch bitmap for async/display-list/clipped/wide-gamut contexts.
	if !forceAtlasScratch && len(sprites) < atlasScratchMinSprites {
		return false
	}
	return p.atlasScratch(op, imgSrc, sprites, im, ctm, st.alpha)
}

func (p *player) atlasScratch(op *driver.DrawOp, imgSrc *driver.ImageSource, sprites driver.AtlasSprites, im *loadedImage, ctm affine, stateAlpha float64) bool {
	srcPix, srcW, srcH, srcStride, points, srcBGRA := atlasSourcePixels(imgSrc, im)
	if len(srcPix) == 0 {
		return false
	}

	// Compute the tight device-space bounding box of all visible sprites.
	minDX, minDY := math.Inf(1), math.Inf(1)
	maxDX, maxDY := math.Inf(-1), math.Inf(-1)
	kx := float64(srcW) / max(float64(points.Width), 1e-9)
	ky := float64(srcH) / max(float64(points.Height), 1e-9)
	anyVisible := false
	for i := range sprites {
		sp := &sprites[i]
		a := stateAlpha * float64(sp.Alpha)
		if !(a > 0) {
			continue
		}
		rw, rh := float64(sp.Rect.Size.Width), float64(sp.Rect.Size.Height)
		if !(rw > 0) || !(rh > 0) || math.IsInf(rw, 0) || math.IsInf(rh, 0) {
			continue
		}
		rx, ry := float64(sp.Rect.Origin.X), float64(sp.Rect.Origin.Y)
		sx0, sy0 := rx*kx, ry*ky
		if sx0 < 0 || sy0 < 0 || sx0+rw*kx > float64(srcW)+1e-4 || sy0+rh*ky > float64(srcH)+1e-4 {
			return false
		}
		t := sp.Transform
		c, s, tx, ty := float64(t.SCos), float64(t.SSin), float64(t.TX), float64(t.TY)
		// Map the four local corners (0,0), (rw,0), (0,rh), (rw,rh) to CG device space.
		ux0, uy0 := tx, ty
		ux1, uy1 := c*rw+tx, s*rw+ty
		ux2, uy2 := -s*rh+tx, c*rh+ty
		ux3, uy3 := c*rw-s*rh+tx, s*rw+c*rh+ty
		for _, u := range [4][2]float64{{ux0, uy0}, {ux1, uy1}, {ux2, uy2}, {ux3, uy3}} {
			dx := ctm.A*u[0] + ctm.C*u[1] + ctm.X
			dy := ctm.B*u[0] + ctm.D*u[1] + ctm.Y
			minDX, minDY = min(minDX, dx), min(minDY, dy)
			maxDX, maxDY = max(maxDX, dx), max(maxDY, dy)
		}
		anyVisible = true
	}
	if !anyVisible {
		return true
	}

	// Intersect with the context's clip bounding box when finite.
	clip := cgContextGetClipBoundingBox(p.ctx)
	if clip.Size.Width <= 0 || clip.Size.Height <= 0 || math.IsNaN(clip.Size.Width) || math.IsNaN(clip.Size.Height) {
		return true
	}
	if math.Abs(clip.Origin.X) < 1e7 && math.Abs(clip.Origin.Y) < 1e7 && clip.Size.Width < 1e7 && clip.Size.Height < 1e7 {
		cMinX, cMinY := math.Inf(1), math.Inf(1)
		cMaxX, cMaxY := math.Inf(-1), math.Inf(-1)
		for _, k := range [4][2]float64{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
			ux := clip.Origin.X + k[0]*clip.Size.Width
			uy := clip.Origin.Y + k[1]*clip.Size.Height
			dx := ctm.A*ux + ctm.C*uy + ctm.X
			dy := ctm.B*ux + ctm.D*uy + ctm.Y
			cMinX, cMinY = min(cMinX, dx), min(cMinY, dy)
			cMaxX, cMaxY = max(cMaxX, dx), max(cMaxY, dy)
		}
		minDX, minDY = max(minDX, cMinX), max(minDY, cMinY)
		maxDX, maxDY = min(maxDX, cMaxX), min(maxDY, cMaxY)
	}

	dxMin := int(math.Floor(minDX))
	dyMin := int(math.Floor(minDY))
	dxMax := int(math.Ceil(maxDX))
	dyMax := int(math.Ceil(maxDY))
	bw, bh := dxMax-dxMin, dyMax-dyMin
	if bw <= 0 || bh <= 0 {
		return true
	}
	if bw > atlasMaxScratchSide || bh > atlasMaxScratchSide || int64(bw)*int64(bh) > maxOffscreenPixels {
		return false
	}

	const premultipliedFirst, byteOrder32Little = 2, 2 << 12
	scratch := cgOffscreenBitmapCreate(0, uintptr(bw), uintptr(bh), 8, uintptr(bw*4), srgb, premultipliedFirst|byteOrder32Little)
	if scratch == 0 {
		return false
	}
	data := cgBitmapData(scratch)
	bpr := int(cgBitmapBytesPerRow(scratch))
	if data == 0 || bpr < bw*4 {
		Release(scratch)
		return false
	}
	dstPix := unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), bh*bpr)
	ok := blitAtlasInto(dstPix, bpr, true, dxMin, dyMax, 0, 0, bw, bh,
		ctm, stateAlpha, op.Sampling == driver.ImageSamplingNearest,
		srcPix, srcW, srcH, srcStride, points, srcBGRA, sprites)
	if !ok {
		Release(scratch)
		return false
	}
	img := cgBitmapContextCreateImage(scratch)
	Release(scratch)
	if img == 0 {
		return false
	}
	defer Release(img)

	cgContextSaveGState(p.ctx)
	cgContextConcatCTM(p.ctx, ctm.invert())
	cgContextSetAlpha(p.ctx, 1)
	setImageSampling(p.ctx, driver.ImageSamplingNearest)
	cgContextDrawImage(p.ctx, Rect(float64(dxMin), float64(dyMin), float64(bw), float64(bh)), img)
	cgContextRestoreGState(p.ctx)
	return true
}

// atlasBitmapTarget inspects ctx and returns its pixel slice if it is a direct
// 8-bit sRGB or DeviceRGB bitmap context in premultiplied BGRA or RGBA order.
func atlasBitmapTarget(ctx uintptr) (pix []byte, w, h, stride int, dstBGRA, ok bool) {
	data := cgBitmapData(ctx)
	if data == 0 {
		return nil, 0, 0, 0, false, false
	}
	if cgBitmapBPC(ctx) != 8 || cgBitmapBPP(ctx) != 32 {
		return nil, 0, 0, 0, false, false
	}
	info := cgBitmapInfo(ctx)
	const (
		alphaPremulLast     = 1
		alphaPremulFirst    = 2
		alphaNoneSkipLast   = 5
		alphaNoneSkipFirst  = 6
		byteOrder32Little   = 2 << 12
		byteOrder32Big      = 4 << 12
		bgraPremul          = byteOrder32Little | alphaPremulFirst
		bgraNoneSkip        = byteOrder32Little | alphaNoneSkipFirst
		rgbaPremulDefault   = alphaPremulLast
		rgbaPremulBig       = byteOrder32Big | alphaPremulLast
		rgbaNoneSkipDefault = alphaNoneSkipLast
		rgbaNoneSkipBig     = byteOrder32Big | alphaNoneSkipLast
	)
	switch info {
	case bgraPremul, bgraNoneSkip:
		dstBGRA = true
	case rgbaPremulDefault, rgbaPremulBig, rgbaNoneSkipDefault, rgbaNoneSkipBig:
		dstBGRA = false
	default:
		return nil, 0, 0, 0, false, false
	}
	cs := cgBitmapColorSpace(ctx)
	if cs == 0 {
		return nil, 0, 0, 0, false, false
	}
	if cs != srgb {
		if name := cgColorSpaceName(cs); name != 0 {
			if name != cgSRGBName && !cfEqual(name, cgSRGBName) && !cfEqual(name, cgDeviceRGBName) {
				return nil, 0, 0, 0, false, false
			}
		} else if cgColorSpaceModel(cs) != 1 { // kCGColorSpaceModelRGB (unnamed DeviceRGB)
			return nil, 0, 0, 0, false, false
		}
	}
	w = int(cgBitmapWidth(ctx))
	h = int(cgBitmapHeight(ctx))
	stride = int(cgBitmapBytesPerRow(ctx))
	if w <= 0 || h <= 0 || stride < w*4 {
		return nil, 0, 0, 0, false, false
	}
	pix = unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), h*stride)
	return pix, w, h, stride, dstBGRA, true
}

// atlasClipInTarget transforms ctx's clip bounding box into top-down target
// pixel coordinates anchored at device (dxMin, dyMax) and clamps to [0, w) x [0, h).
func atlasClipInTarget(ctx uintptr, ctm affine, dxMin, dyMax, w, h int) (x0, y0, x1, y1 int, empty bool) {
	clip := cgContextGetClipBoundingBox(ctx)
	if clip.Size.Width <= 0 || clip.Size.Height <= 0 || math.IsNaN(clip.Size.Width) || math.IsNaN(clip.Size.Height) {
		return 0, 0, 0, 0, true
	}
	if math.Abs(clip.Origin.X) >= 1e7 || math.Abs(clip.Origin.Y) >= 1e7 || clip.Size.Width >= 1e7 || clip.Size.Height >= 1e7 {
		return 0, 0, w, h, false
	}
	mA, mC, mX := ctm.A, ctm.C, ctm.X-float64(dxMin)
	mB, mD, mY := -ctm.B, -ctm.D, float64(dyMax)-ctm.Y
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, k := range [4][2]float64{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		ux := clip.Origin.X + k[0]*clip.Size.Width
		uy := clip.Origin.Y + k[1]*clip.Size.Height
		px := mA*ux + mC*uy + mX
		py := mB*ux + mD*uy + mY
		minX, minY = min(minX, px), min(minY, py)
		maxX, maxY = max(maxX, px), max(maxY, py)
	}
	x0 = max(0, int(math.Floor(minX)))
	y0 = max(0, int(math.Floor(minY)))
	x1 = min(w, int(math.Ceil(maxX)))
	y1 = min(h, int(math.Ceil(maxY)))
	return x0, y0, x1, y1, x0 >= x1 || y0 >= y1
}

// atlasSourcePixels returns premultiplied source pixels for imgSrc without
// re-decoding: directly from imgSrc.Pixels when present, or from im's cached
// BGRA pixel copy.
func atlasSourcePixels(imgSrc *driver.ImageSource, im *loadedImage) (pix []byte, w, h, stride int, points driver.Size, bgra bool) {
	if p := imgSrc.Pixels; p != nil {
		w, h = p.Rect.Dx(), p.Rect.Dy()
		if w <= 0 || h <= 0 || len(p.Pix) < (h-1)*p.Stride+w*4 {
			return nil, 0, 0, 0, driver.Size{}, false
		}
		points = imgSrc.Size
		if points.Width <= 0 || points.Height <= 0 {
			points = driver.Size{Width: driver.Points(w), Height: driver.Points(h)}
		}
		return p.Pix, w, h, p.Stride, points, false
	}
	w, h = cgImageGetWidth(im.image), cgImageGetHeight(im.image)
	if w <= 0 || h <= 0 {
		return nil, 0, 0, 0, driver.Size{}, false
	}
	if len(im.meshPixels) != w*h*4 {
		im.meshPixels = imagePixels(im.image, w, h)
	}
	if len(im.meshPixels) < w*h*4 {
		return nil, 0, 0, 0, driver.Size{}, false
	}
	return im.meshPixels, w, h, w * 4, im.points, true
}

const atlasFixed = 1 << 32

type preparedSprite struct {
	dx0, dx1 int // destination pixel columns [dx0, dx1)
	dy0, dy1 int // destination pixel rows [dy0, dy1)

	cropX0, cropY0 int // source crop origin in texels
	cropX1, cropY1 int // source crop exclusive end in texels
	minU, maxU     int64
	minV, maxV     int64

	// Axis-aligned stepping (when axisAligned is true):
	uStart, vStart int64
	du, dv         int64

	// General affine inverse transform (when axisAligned is false):
	// lx = ia*(x + 0.5 - tx) + ie*(y + 0.5 - ty)
	// ly = ib*(x + 0.5 - tx) + id*(y + 0.5 - ty)
	ia, ib, ie, id float64
	tx, ty         float64
	rw, rh         float64
	sx0, sy0       float64
	kx, ky         float64
	duX, dvX       int64

	a255        uint32 // effective alpha in [1..255]
	axisAligned bool
}

var atlasPreparedPool = sync.Pool{
	New: func() any {
		s := make([]preparedSprite, 0, 256)
		return &s
	},
}

func blitAtlasInto(
	dstPix []byte, dstStride int, dstBGRA bool,
	dxMin, dyMax int, clipX0, clipY0, clipX1, clipY1 int,
	ctm affine, stateAlpha float64, nearest bool,
	srcPix []byte, srcW, srcH, srcStride int, points driver.Size, srcBGRA bool,
	sprites []driver.AtlasSprite,
) bool {
	if clipX0 >= clipX1 || clipY0 >= clipY1 {
		return true
	}
	kx := float64(srcW) / max(float64(points.Width), 1e-9)
	ky := float64(srcH) / max(float64(points.Height), 1e-9)

	// Map canvas user space -> top-down target buffer pixel space.
	mA, mC, mX := ctm.A, ctm.C, ctm.X-float64(dxMin)
	mB, mD, mY := -ctm.B, -ctm.D, float64(dyMax)-ctm.Y

	bufPtr := atlasPreparedPool.Get().(*[]preparedSprite)
	prepared := (*bufPtr)[:0]
	defer func() {
		*bufPtr = prepared[:0]
		atlasPreparedPool.Put(bufPtr)
	}()

	var totalPixels int64
	minRow, maxRow := clipY1, clipY0

	for i := range sprites {
		sp := &sprites[i]
		effAlpha := stateAlpha * float64(sp.Alpha)
		if !(effAlpha > 0) {
			continue
		}
		if effAlpha > 1 {
			effAlpha = 1
		}
		a255 := uint32(math.Round(effAlpha * 255))
		if a255 == 0 {
			continue
		}
		rw, rh := float64(sp.Rect.Size.Width), float64(sp.Rect.Size.Height)
		if !(rw > 0) || !(rh > 0) || math.IsInf(rw, 0) || math.IsInf(rh, 0) {
			continue
		}
		rx, ry := float64(sp.Rect.Origin.X), float64(sp.Rect.Origin.Y)
		sx0, sy0 := rx*kx, ry*ky
		sw, sh := rw*kx, rh*ky
		if sx0 < 0 || sy0 < 0 || sx0+sw > float64(srcW)+1e-4 || sy0+sh > float64(srcH)+1e-4 {
			return false
		}
		cropX0 := max(0, int(math.Floor(sx0)))
		cropY0 := max(0, int(math.Floor(sy0)))
		cropX1 := min(srcW, int(math.Ceil(sx0+sw)))
		cropY1 := min(srcH, int(math.Ceil(sy0+sh)))
		if cropX0 >= cropX1 || cropY0 >= cropY1 {
			continue
		}

		t := sp.Transform
		c, s, stx, sty := float64(t.SCos), float64(t.SSin), float64(t.TX), float64(t.TY)
		// Combined transform from sprite local (lx, ly) to target buffer (px, py).
		a := mA*c + mC*s
		b := mB*c + mD*s
		e := -mA*s + mC*c
		d := -mB*s + mD*c
		tx := mA*stx + mC*sty + mX
		ty := mB*stx + mD*sty + mY

		det := a*d - b*e
		if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
			continue
		}

		var minU, maxU, minV, maxV int64
		if nearest {
			minU = int64(cropX0) * atlasFixed
			maxU = int64(cropX1)*atlasFixed - 1
			minV = int64(cropY0) * atlasFixed
			maxV = int64(cropY1)*atlasFixed - 1
		} else {
			minU = int64(cropX0) * atlasFixed
			maxU = int64(cropX1-1) * atlasFixed
			minV = int64(cropY0) * atlasFixed
			maxV = int64(cropY1-1) * atlasFixed
		}

		if b == 0 && e == 0 {
			x0f, x1f := tx, tx+a*rw
			if x0f > x1f {
				x0f, x1f = x1f, x0f
			}
			y0f, y1f := ty, ty+d*rh
			if y0f > y1f {
				y0f, y1f = y1f, y0f
			}
			dx0 := max(clipX0, int(math.Ceil(x0f-0.5)))
			dx1 := min(clipX1, int(math.Ceil(x1f-0.5)))
			dy0 := max(clipY0, int(math.Ceil(y0f-0.5)))
			dy1 := min(clipY1, int(math.Ceil(y1f-0.5)))
			if dx0 >= dx1 || dy0 >= dy1 {
				continue
			}
			duF := kx / a
			dvF := ky / d
			uBase := sx0 + (float64(dx0)+0.5-tx)*duF
			vBase := sy0 + (float64(dy0)+0.5-ty)*dvF
			if !nearest {
				uBase -= 0.5
				vBase -= 0.5
			}
			// Guard fixed-point range for extreme sub-pixel scales.
			if math.Abs(uBase) > 1e7 || math.Abs(vBase) > 1e7 || math.Abs(duF) > 1e7 || math.Abs(dvF) > 1e7 {
				return false
			}
			prepared = append(prepared, preparedSprite{
				dx0: dx0, dx1: dx1, dy0: dy0, dy1: dy1,
				cropX0: cropX0, cropY0: cropY0, cropX1: cropX1, cropY1: cropY1,
				minU: minU, maxU: maxU, minV: minV, maxV: maxV,
				uStart:      int64(math.Round(uBase * atlasFixed)),
				vStart:      int64(math.Round(vBase * atlasFixed)),
				du:          int64(math.Round(duF * atlasFixed)),
				dv:          int64(math.Round(dvF * atlasFixed)),
				a255:        a255,
				axisAligned: true,
			})
			totalPixels += int64(dx1-dx0) * int64(dy1-dy0)
			minRow, maxRow = min(minRow, dy0), max(maxRow, dy1)
			continue
		}

		// General 2D affine sprite (rotated or skewed).
		p0x, p0y := tx, ty
		p1x, p1y := tx+a*rw, ty+b*rw
		p2x, p2y := tx+e*rh, ty+d*rh
		p3x, p3y := tx+a*rw+e*rh, ty+b*rw+d*rh
		x0f := min(min(p0x, p1x), min(p2x, p3x))
		x1f := max(max(p0x, p1x), max(p2x, p3x))
		y0f := min(min(p0y, p1y), min(p2y, p3y))
		y1f := max(max(p0y, p1y), max(p2y, p3y))
		dx0 := max(clipX0, int(math.Ceil(x0f-0.5)))
		dx1 := min(clipX1, int(math.Ceil(x1f-0.5)))
		dy0 := max(clipY0, int(math.Ceil(y0f-0.5)))
		dy1 := min(clipY1, int(math.Ceil(y1f-0.5)))
		if dx0 >= dx1 || dy0 >= dy1 {
			continue
		}
		invDet := 1 / det
		ia, ie := d*invDet, -e*invDet
		ib, id := -b*invDet, a*invDet
		duXF, dvXF := ia*kx, ib*ky
		if math.Abs(duXF) > 1e7 || math.Abs(dvXF) > 1e7 {
			return false
		}
		prepared = append(prepared, preparedSprite{
			dx0: dx0, dx1: dx1, dy0: dy0, dy1: dy1,
			cropX0: cropX0, cropY0: cropY0, cropX1: cropX1, cropY1: cropY1,
			minU: minU, maxU: maxU, minV: minV, maxV: maxV,
			ia: ia, ib: ib, ie: ie, id: id,
			tx: tx, ty: ty, rw: rw, rh: rh,
			sx0: sx0, sy0: sy0, kx: kx, ky: ky,
			duX:         int64(math.Round(duXF * atlasFixed)),
			dvX:         int64(math.Round(dvXF * atlasFixed)),
			a255:        a255,
			axisAligned: false,
		})
		totalPixels += int64(dx1-dx0) * int64(dy1-dy0)
		minRow, maxRow = min(minRow, dy0), max(maxRow, dy1)
	}

	if len(prepared) == 0 || minRow >= maxRow {
		return true
	}

	swapRB := srcBGRA != dstBGRA
	const bandRows = 64
	rows := maxRow - minRow
	bands := (rows + bandRows - 1) / bandRows
	workers := 1
	if totalPixels >= 128<<10 && len(prepared) >= 32 {
		workers = min(runtime.GOMAXPROCS(0), bands)
	}
	if workers <= 1 {
		blitAtlasBand(dstPix, dstStride, srcPix, srcStride, swapRB, nearest, prepared, minRow, maxRow)
		return true
	}
	var next atomic.Int32
	work := func() {
		for {
			i := int(next.Add(1)) - 1
			if i >= bands {
				return
			}
			y0 := minRow + i*bandRows
			y1 := min(maxRow, y0+bandRows)
			blitAtlasBand(dstPix, dstStride, srcPix, srcStride, swapRB, nearest, prepared, y0, y1)
		}
	}
	var wg sync.WaitGroup
	for range workers - 1 {
		wg.Go(work)
	}
	work()
	wg.Wait()
	return true
}

func blitAtlasBand(
	dstPix []byte, dstStride int,
	srcPix []byte, srcStride int,
	swapRB, nearest bool,
	sprites []preparedSprite,
	bandY0, bandY1 int,
) {
	for i := range sprites {
		sp := &sprites[i]
		if sp.dy1 <= bandY0 || sp.dy0 >= bandY1 {
			continue
		}
		y0 := max(sp.dy0, bandY0)
		y1 := min(sp.dy1, bandY1)
		if sp.axisAligned {
			if nearest {
				blitAxisAlignedNearest(dstPix, dstStride, srcPix, srcStride, swapRB, sp, y0, y1)
			} else {
				blitAxisAlignedLinear(dstPix, dstStride, srcPix, srcStride, swapRB, sp, y0, y1)
			}
		} else {
			blitAffine(dstPix, dstStride, srcPix, srcStride, swapRB, nearest, sp, y0, y1)
		}
	}
}

func blitAxisAlignedNearest(
	dstPix []byte, dstStride int,
	srcPix []byte, srcStride int,
	swapRB bool,
	sp *preparedSprite,
	y0, y1 int,
) {
	uStart, du := sp.uStart, sp.du
	minU, maxU := sp.minU, sp.maxU
	minV, maxV := sp.minV, sp.maxV
	v := sp.vStart + int64(y0-sp.dy0)*sp.dv
	dv := sp.dv
	a255 := sp.a255
	cols := sp.dx1 - sp.dx0
	dstColOff := sp.dx0 * 4

	for y := y0; y < y1; y++ {
		vc := min(max(v, minV), maxV)
		v += dv
		sy := int(vc >> 32)
		srcRow := srcPix[sy*srcStride:]
		dstRow := dstPix[y*dstStride+dstColOff : y*dstStride+dstColOff+cols*4]

		u := uStart
		lastSX := -1
		var cachedPx uint32
		for off := 0; off < len(dstRow); off += 4 {
			uc := min(max(u, minU), maxU)
			u += du
			sx := int(uc >> 32)
			if sx != lastSX {
				lastSX = sx
				px := binary.LittleEndian.Uint32(srcRow[sx*4 : sx*4+4])
				if px != 0 {
					if swapRB {
						px = swapRB32(px)
					}
					if a255 != 255 {
						px = mulAlpha32(px, a255)
					}
				}
				cachedPx = px
			}
			if cachedPx != 0 {
				blendOver32(dstRow[off:off+4:off+4], cachedPx)
			}
		}
	}
}

func blitAxisAlignedLinear(
	dstPix []byte, dstStride int,
	srcPix []byte, srcStride int,
	swapRB bool,
	sp *preparedSprite,
	y0, y1 int,
) {
	uStart, du := sp.uStart, sp.du
	minU, maxU := sp.minU, sp.maxU
	minV, maxV := sp.minV, sp.maxV
	v := sp.vStart + int64(y0-sp.dy0)*sp.dv
	dv := sp.dv
	a255 := sp.a255
	maxSX := sp.cropX1 - 1
	maxSY := sp.cropY1 - 1
	cols := sp.dx1 - sp.dx0
	dstColOff := sp.dx0 * 4

	for y := y0; y < y1; y++ {
		vc := min(max(v, minV), maxV)
		v += dv
		sy := int(vc >> 32)
		fy := uint32(vc>>24) & 255
		row0 := srcPix[sy*srcStride:]
		row1 := row0
		if sy < maxSY {
			row1 = srcPix[(sy+1)*srcStride:]
		}
		dstRow := dstPix[y*dstStride+dstColOff : y*dstStride+dstColOff+cols*4]

		u := uStart
		if fy == 0 {
			for off := 0; off < len(dstRow); off += 4 {
				uc := min(max(u, minU), maxU)
				u += du
				sx := int(uc >> 32)
				fx := uint32(uc>>24) & 255
				p0 := binary.LittleEndian.Uint32(row0[sx*4 : sx*4+4])
				var px uint32
				if fx == 0 || sx >= maxSX {
					px = p0
				} else {
					p1 := binary.LittleEndian.Uint32(row0[sx*4+4 : sx*4+8])
					if p0|p1 != 0 {
						px = mixPixels32(p0, p1, fx)
					}
				}
				if px == 0 {
					continue
				}
				if swapRB {
					px = swapRB32(px)
				}
				if a255 != 255 {
					px = mulAlpha32(px, a255)
				}
				if px != 0 {
					blendOver32(dstRow[off:off+4:off+4], px)
				}
			}
			continue
		}
		for off := 0; off < len(dstRow); off += 4 {
			uc := min(max(u, minU), maxU)
			u += du
			sx := int(uc >> 32)
			fx := uint32(uc>>24) & 255
			si := sx * 4
			t0 := binary.LittleEndian.Uint32(row0[si : si+4])
			b0 := binary.LittleEndian.Uint32(row1[si : si+4])
			var px uint32
			if fx == 0 || sx >= maxSX {
				if t0|b0 != 0 {
					px = mixPixels32(t0, b0, fy)
				}
			} else {
				t1 := binary.LittleEndian.Uint32(row0[si+4 : si+8])
				b1 := binary.LittleEndian.Uint32(row1[si+4 : si+8])
				if t0|t1|b0|b1 != 0 {
					top := mixPixels32(t0, t1, fx)
					bot := mixPixels32(b0, b1, fx)
					px = mixPixels32(top, bot, fy)
				}
			}
			if px == 0 {
				continue
			}
			if swapRB {
				px = swapRB32(px)
			}
			if a255 != 255 {
				px = mulAlpha32(px, a255)
			}
			if px != 0 {
				blendOver32(dstRow[off:off+4:off+4], px)
			}
		}
	}
}

func blitAffine(
	dstPix []byte, dstStride int,
	srcPix []byte, srcStride int,
	swapRB, nearest bool,
	sp *preparedSprite,
	y0, y1 int,
) {
	minU, maxU := sp.minU, sp.maxU
	minV, maxV := sp.minV, sp.maxV
	maxSX := sp.cropX1 - 1
	maxSY := sp.cropY1 - 1
	a255 := sp.a255
	duX, dvX := sp.duX, sp.dvX

	for y := y0; y < y1; y++ {
		dy := float64(y) + 0.5 - sp.ty
		lxBase := sp.ie*dy - sp.ia*sp.tx
		lyBase := sp.id*dy - sp.ib*sp.tx

		// Clip x in [sp.dx0, sp.dx1) against 0 <= lx < rw and 0 <= ly < rh at x+0.5.
		startF, endF := float64(sp.dx0), float64(sp.dx1)
		startF, endF = clipLinearInterval(startF, endF, sp.ia, lxBase+0.5*sp.ia, 0, sp.rw)
		if startF >= endF {
			continue
		}
		startF, endF = clipLinearInterval(startF, endF, sp.ib, lyBase+0.5*sp.ib, 0, sp.rh)
		if startF >= endF {
			continue
		}
		x0 := max(sp.dx0, int(math.Ceil(startF)))
		x1 := min(sp.dx1, int(math.Ceil(endF)))
		// Trim edge rounding to exact [0, rw) x [0, rh) membership.
		for x0 < x1 {
			xf := float64(x0) + 0.5 - sp.tx
			lx := sp.ia*xf + sp.ie*dy
			ly := sp.ib*xf + sp.id*dy
			if lx >= 0 && lx < sp.rw && ly >= 0 && ly < sp.rh {
				break
			}
			x0++
		}
		for x1 > x0 {
			xf := float64(x1-1) + 0.5 - sp.tx
			lx := sp.ia*xf + sp.ie*dy
			ly := sp.ib*xf + sp.id*dy
			if lx >= 0 && lx < sp.rw && ly >= 0 && ly < sp.rh {
				break
			}
			x1--
		}
		if x0 >= x1 {
			continue
		}

		xf0 := float64(x0) + 0.5 - sp.tx
		uF := sp.sx0 + (sp.ia*xf0+sp.ie*dy)*sp.kx
		vF := sp.sy0 + (sp.ib*xf0+sp.id*dy)*sp.ky
		if !nearest {
			uF -= 0.5
			vF -= 0.5
		}
		u := int64(math.Round(uF * atlasFixed))
		v := int64(math.Round(vF * atlasFixed))
		dstRow := dstPix[y*dstStride+x0*4 : y*dstStride+x1*4]

		if nearest {
			for off := 0; off < len(dstRow); off += 4 {
				uc := min(max(u, minU), maxU)
				vc := min(max(v, minV), maxV)
				u += duX
				v += dvX
				si := int(vc>>32)*srcStride + int(uc>>32)*4
				px := binary.LittleEndian.Uint32(srcPix[si : si+4])
				if px == 0 {
					continue
				}
				if swapRB {
					px = swapRB32(px)
				}
				if a255 != 255 {
					px = mulAlpha32(px, a255)
				}
				if px != 0 {
					blendOver32(dstRow[off:off+4:off+4], px)
				}
			}
			continue
		}

		for off := 0; off < len(dstRow); off += 4 {
			uc := min(max(u, minU), maxU)
			vc := min(max(v, minV), maxV)
			u += duX
			v += dvX
			sx, sy := int(uc>>32), int(vc>>32)
			fx, fy := uint32(uc>>24)&255, uint32(vc>>24)&255
			si := sy*srcStride + sx*4
			right := 4
			if sx >= maxSX {
				right = 0
			}
			down := srcStride
			if sy >= maxSY {
				down = 0
			}
			t0 := binary.LittleEndian.Uint32(srcPix[si : si+4])
			t1 := binary.LittleEndian.Uint32(srcPix[si+right : si+right+4])
			b0 := binary.LittleEndian.Uint32(srcPix[si+down : si+down+4])
			b1 := binary.LittleEndian.Uint32(srcPix[si+down+right : si+down+right+4])
			if t0|t1|b0|b1 == 0 {
				continue
			}
			px := mixPixels32(mixPixels32(t0, t1, fx), mixPixels32(b0, b1, fx), fy)
			if px == 0 {
				continue
			}
			if swapRB {
				px = swapRB32(px)
			}
			if a255 != 255 {
				px = mulAlpha32(px, a255)
			}
			if px != 0 {
				blendOver32(dstRow[off:off+4:off+4], px)
			}
		}
	}
}

// clipLinearInterval narrows [lo, hi) to where minV <= slope*x + intercept < maxV.
func clipLinearInterval(lo, hi, slope, intercept, minV, maxV float64) (float64, float64) {
	if slope == 0 {
		if intercept < minV || intercept >= maxV {
			return hi, hi
		}
		return lo, hi
	}
	a := (minV - intercept) / slope
	b := (maxV - intercept) / slope
	if a > b {
		a, b = b, a
	}
	return max(lo, a), min(hi, b)
}

func swapRB32(px uint32) uint32 {
	return px&0xff00ff00 | (px&0x000000ff)<<16 | (px>>16)&0x000000ff
}

func mixPixels32(a, b, f uint32) uint32 {
	const lane = 0x00ff00ff
	lo := ((a&lane)*(256-f) + (b&lane)*f + 0x00800080) >> 8 & lane
	hi := (((a>>8)&lane)*(256-f) + ((b>>8)&lane)*f + 0x00800080) >> 8 & lane
	return lo | hi<<8
}

// mulAlpha32 scales every premultiplied 8-bit channel of px by a255/255 with
// exact rounding, in one 64-bit SWAR step.
func mulAlpha32(px, a255 uint32) uint32 {
	const mask = 0x00ff00ff00ff00ff
	s64 := uint64(px&0x00ff00ff) | uint64(px&0xff00ff00)<<24
	t := s64*uint64(a255) + 0x0080008000800080
	t = (t + ((t >> 8) & mask)) >> 8 & mask
	return uint32(t&0x00ff00ff) | uint32((t>>24)&0xff00ff00)
}

// blendOver32 composites premultiplied src over premultiplied dst in place.
// Works identically for both RGBA and BGRA little-endian byte orders because
// alpha occupies bits 24..31 in both.
func blendOver32(dst []byte, src uint32) {
	sa := src >> 24
	if sa == 255 {
		binary.LittleEndian.PutUint32(dst, src)
		return
	}
	d32 := binary.LittleEndian.Uint32(dst)
	if d32 == 0 {
		binary.LittleEndian.PutUint32(dst, src)
		return
	}
	const mask = 0x00ff00ff00ff00ff
	invA := uint64(255 - sa)
	s64 := uint64(src&0x00ff00ff) | uint64(src&0xff00ff00)<<24
	d64 := uint64(d32&0x00ff00ff) | uint64(d32&0xff00ff00)<<24
	t := d64*invA + 0x0080008000800080
	t = (t + ((t >> 8) & mask)) >> 8 & mask
	out := s64 + t
	if out&0xff00ff00ff00ff00 != 0 {
		out = clampLanes64(out)
	}
	binary.LittleEndian.PutUint32(dst, uint32(out&0x00ff00ff)|uint32((out>>24)&0xff00ff00))
}

func clampLanes64(v uint64) uint64 {
	var out uint64
	for shift := uint(0); shift < 64; shift += 16 {
		c := (v >> shift) & 0xffff
		if c > 255 {
			c = 255
		}
		out |= c << shift
	}
	return out
}
