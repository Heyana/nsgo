//go:build linux && !android

package linux

import "github.com/nsgo-dev/nsgo/internal/driver"

func (p *canvasPlayer) atlas(op *driver.DrawOp) {
	imgSrc := p.drawing.Image(op)
	sprites := p.drawing.AtlasSprites(op)
	if imgSrc == nil || len(sprites) == 0 {
		return
	}
	if imgSrc.Offscreen != nil {
		p.atlasImages(op, sprites)
		return
	}
	slot := p.canvasImage(*imgSrc)
	if slot == nil || slot.pic == nil {
		p.waited = p.waited || (slot != nil && slot.load != nil)
		return
	}
	src := slot.surfaceOf(0)
	nat := natural(slot)
	if src == 0 || nat.Width <= 0 {
		return
	}
	pattern := cairoPatternCreateForSurface(src)
	defer cairoPatternDestroy(pattern)
	cairoPatternSetExtend(pattern, 3) // CAIRO_EXTEND_PAD
	if op.Sampling == driver.ImageSamplingNearest {
		const cairoFilterNearest = 3
		cairoPatternSetFilter(pattern, cairoFilterNearest)
	}
	cr := p.cr
	path := cairoCopyPath(cr)
	k := float64(slot.pic.width) / float64(nat.Width)
	for _, sprite := range sprites {
		cairoSave(cr)
		t := sprite.Transform
		c, s := float64(t.SCos), float64(t.SSin)
		m := cairoMatrix{xx: c, yx: s, xy: -s, yy: c, x0: float64(t.TX), y0: float64(t.TY)}
		cairoTransform(cr, &m)
		b := sprite.Rect
		cairoNewPath(cr)
		cairoRectangle(cr, 0, 0, float64(b.Size.Width), float64(b.Size.Height))
		source := cairoMatrix{xx: k, yy: k, x0: k * float64(b.Origin.X), y0: k * float64(b.Origin.Y)}
		cairoPatternSetMatrix(pattern, &source)
		p.shadowed(func(cr uintptr) {
			cairoSave(cr)
			cairoClip(cr)
			cairoSetSource(cr, pattern)
			cairoPaintWithAlpha(cr, p.st.alpha*float64(sprite.Alpha))
			cairoRestore(cr)
		})
		cairoRestore(cr)
	}
	cairoNewPath(cr)
	cairoAppendPath(cr, path)
	cairoPathDestroy(path)
}

// atlasImages draws each sprite as the DrawImageFrom it stands for, for sources the shared pattern does not cover: offscreen drawings.
func (p *canvasPlayer) atlasImages(op *driver.DrawOp, sprites driver.AtlasSprites) {
	alpha := p.st.alpha
	for _, sprite := range sprites {
		t, b := sprite.Transform, sprite.Rect
		c, s := float64(t.SCos), float64(t.SSin)
		m := cairoMatrix{xx: c, yx: s, xy: -s, yy: c, x0: float64(t.TX), y0: float64(t.TY)}
		cairoSave(p.cr)
		cairoTransform(p.cr, &m)
		p.st.alpha = alpha * float64(sprite.Alpha)
		w, h := float32(b.Size.Width), float32(b.Size.Height)
		p.image(&driver.DrawOp{Kind: driver.DrawImage, ImageIndex: op.ImageIndex, Sampling: op.Sampling, W: w, H: h,
			X1: float32(b.Origin.X), Y1: float32(b.Origin.Y), X2: w, Y2: h})
		cairoRestore(p.cr)
	}
	p.st.alpha = alpha
}
