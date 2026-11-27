//go:build darwin

package apple

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	maxAtlasCropsPerImage = 1024
	maxAtlasCropPixels    = 8 << 20
)

var atlasCropPixels int // main thread, with the canvas images that own the crops

func (im *loadedImage) release() {
	for _, crop := range im.atlasCrops {
		Release(crop)
	}
	atlasCropPixels -= im.atlasPixels
	im.atlasPixels, im.atlasCrops = 0, nil
	im.meshPixels = nil
	Release(im.image)
	im.image = 0
	Release(im.canvasTint)
	im.canvasTint = 0
	if im.metal != 0 {
		im.metal.Send(selRelease)
		im.metal = 0
	}
}

// crop returns a retained tile, with temporary true when the caller owns
// it. The cache shares the lifetime of its decoded source image.
func (im *loadedImage) crop(b driver.Bounds) (tile uintptr, temporary bool) {
	if tile = im.atlasCrops[b]; tile != 0 {
		return tile, false
	}
	k := im.width / max(float64(im.points.Width), 1e-9)
	x, y := float64(b.Origin.X)*k, float64(b.Origin.Y)*k
	w, h := float64(b.Size.Width)*k, float64(b.Size.Height)*k
	tile = cgImageCreateWithImageInRect(im.image, Rect(x, y, w, h))
	area := (math.Ceil(x+w) - math.Floor(x)) * (math.Ceil(y+h) - math.Floor(y))
	if tile == 0 || len(im.atlasCrops) >= maxAtlasCropsPerImage || area > float64(maxAtlasCropPixels-atlasCropPixels) {
		return tile, true
	}
	if im.atlasCrops == nil {
		im.atlasCrops = map[driver.Bounds]uintptr{}
	}
	im.atlasCrops[b] = tile
	im.atlasPixels += int(area)
	atlasCropPixels += int(area)
	return tile, false
}

func (p *player) atlas(op *driver.DrawOp) {
	if op.Image == nil || op.Atlas == nil || len(*op.Atlas) == 0 {
		return
	}
	if op.Image.Offscreen != nil {
		p.atlasImages(op)
		return
	}
	var im *loadedImage
	if p.host.image != nil {
		im = p.host.image(*op.Image, p.host.Ratio)
		if im == nil || im.image == 0 {
			return
		}
	} else {
		im = canvasImage(*op.Image, p.host)
		if im == nil || im.image == 0 {
			p.waited = p.waited || im == nil || !im.failed
			return
		}
	}
	if p.atlasFast(op, im) {
		return
	}
	p.atlasCG(op, im)
}

func (p *player) atlasCG(op *driver.DrawOp, im *loadedImage) {
	st := p.st()
	flat := !st.shadow && !st.composite.Unbounded()
	for _, sprite := range *op.Atlas {
		flat = flat && sprite.Transform.SSin == 0 && sprite.Transform.SCos > 0
	}
	ctx := p.ctx
	if flat {
		// Flip the coordinate system once. Axis-aligned sprites can all be
		// drawn in it without changing the transform between them.
		cgContextSaveGState(ctx)
		cgContextScaleCTM(ctx, 1, -1)
		setImageSampling(ctx, op.Sampling)
		alpha := st.alpha
		for _, sprite := range *op.Atlas {
			tile, temporary := im.crop(sprite.Rect)
			if tile == 0 {
				continue
			}
			if next := st.alpha * float64(sprite.Alpha); next != alpha {
				alpha = next
				cgContextSetAlpha(ctx, alpha)
			}
			t := sprite.Transform
			w, h := float64(sprite.Rect.Size.Width)*float64(t.SCos), float64(sprite.Rect.Size.Height)*float64(t.SCos)
			cgContextDrawImage(ctx, Rect(float64(t.TX), -float64(t.TY)-h, w, h), tile)
			if temporary {
				Release(tile)
			}
		}
		cgContextRestoreGState(ctx)
		return
	}
	for _, sprite := range *op.Atlas {
		tile, temporary := im.crop(sprite.Rect)
		if tile == 0 {
			continue
		}
		w, h := float64(sprite.Rect.Size.Width), float64(sprite.Rect.Size.Height)
		t := sprite.Transform
		c, s := float64(t.SCos), float64(t.SSin)
		cgContextSaveGState(ctx)
		cgContextSetAlpha(ctx, st.alpha*float64(sprite.Alpha))
		p.layered(false, func() {
			setImageSampling(ctx, op.Sampling)
			// Local sprite transform followed by the image's vertical flip.
			cgContextConcatCTM(ctx, affine{A: c, B: s, C: s, D: -c, X: float64(t.TX) - s*h, Y: float64(t.TY) + c*h})
			cgContextDrawImage(ctx, Rect(0, 0, w, h), tile)
		})
		cgContextRestoreGState(ctx)
		if temporary {
			Release(tile)
		}
	}
}

// atlasImages draws each sprite as the DrawImageFrom it stands for, for sources the crop cache does not cover: offscreen drawings.
func (p *player) atlasImages(op *driver.DrawOp) {
	ctx, alpha := p.ctx, p.st().alpha
	for _, sprite := range *op.Atlas {
		t, b := sprite.Transform, sprite.Rect
		c, s := float64(t.SCos), float64(t.SSin)
		cgContextSaveGState(ctx)
		cgContextConcatCTM(ctx, affine{A: c, B: s, C: -s, D: c, X: float64(t.TX), Y: float64(t.TY)})
		cgContextSetAlpha(ctx, alpha*float64(sprite.Alpha))
		w, h := float32(b.Size.Width), float32(b.Size.Height)
		p.image(&driver.DrawOp{Kind: driver.DrawImage, Image: op.Image, Sampling: op.Sampling, W: w, H: h,
			X1: float32(b.Origin.X), Y1: float32(b.Origin.Y), X2: w, Y2: h})
		cgContextRestoreGState(ctx)
	}
}
