//go:build windows

package windows

import (
	"math"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A canvas plays its drawing into Direct2D command lists, as NSH's do,
// and a frame draws the lists: a drawing is played once, not on every
// frame the window draws. A drawing with bound arguments is two lists:
// its static part (driver.CanvasSplit), played once, and the rest,
// played again when an argument changes, which plays the static part's
// state (transforms, styles, the path, clips) without painting it. A
// rest that would combine with what the static part drew, under a
// composite operation other than source-over or with DrawClear, is
// played whole on each change instead. Command lists are vector: they
// stay sharp under any transform.
//
// Lists are played on a context of their own (gfx.record), so the frame's
// layers stay pushed while they are made.

// canvasCache is a canvas's lists.
type canvasCache struct {
	drawing *driver.Drawing
	split   driver.CanvasSplit // At 0 when the drawing is one list
	atlas   bool               // a recording can contain aliased sprites
	// shaded plays the drawing on each frame, as its shaded ops render
	// at the pixels the frame shows (canvasshade.go).
	shaded bool
	// mixes reports whether the drawing uses DrawClear or non-source-over composites, which rasterize into an offscreen bitmap scaled with world.
	mixes bool
	// head is the static part's list, or the whole drawing's; tail the
	// rest's, nil until played for the arguments' current values.
	head, tail *comObject
	scale      float32
	generation uint64
	// last are the ops as the bound arguments last changed them, for the
	// damage of the next change; spare is the buffer it was before.
	last, spare []driver.DrawOp
}

// setDrawing takes v's new drawing.
func (c *canvasCache) setDrawing(d *driver.Drawing) {
	c.release()
	c.drawing, c.split, c.atlas, c.mixes, c.shaded, c.last = d, driver.CanvasSplit{}, false, false, false, nil
	if d == nil {
		return
	}
	c.shaded = d.Shaded() && shadeCanvas != nil
	for _, op := range d.Ops {
		if op.Kind == driver.DrawAtlas {
			c.atlas = true
			break
		}
	}
	c.mixes = mixesAfter(d.Ops, 0)
	if s := driver.SplitCanvas(d); s.Static(d) && !mixesAfter(d.Ops, s.At) {
		c.split = s
	}
}

// mixesAfter reports whether the ops from at on combine with what those
// before drew: a composite operation other than source-over set before
// at or from at on, or a DrawClear from at on.
func mixesAfter(ops []driver.DrawOp, at int) bool {
	for i, op := range ops {
		switch {
		case op.Kind == driver.DrawComposite && op.Composite != driver.CompositeSourceOver:
			return true
		case op.Kind == driver.DrawClear && i >= at:
			return true
		}
	}
	return false
}

// argsChanged drops the list the arguments' old values played.
func (c *canvasCache) argsChanged() {
	if c.split.At > 0 {
		releaseAll(&c.tail)
	} else {
		releaseAll(&c.head)
	}
}

func (c *canvasCache) release() { releaseAll(&c.head, &c.tail) }

// offscreenScale is the pixels per point an offscreen canvas bitmap draws at under world transform world: baseScale times driver.RasterStep of world's stretch, bounded to 4096 × 4096 pixels and a 16384-pixel side.
func offscreenScale(width, height, baseScale float32, world matrix) float32 {
	if baseScale <= 0 {
		baseScale = 1
	}
	step := driver.RasterStep(world.maxScale())
	if step <= 1 || width <= 0 || height <= 0 {
		return baseScale
	}
	const maxPixels = 4096 * 4096
	const maxSide = 16384
	w, h := float64(width), float64(height)
	s := float64(baseScale) * step
	s = min(s, math.Sqrt(maxPixels/(w*h)), maxSide/max(w, h))
	return max(float32(s), baseScale)
}

// canvas draws v's drawing from its lists, played as needed; it plays
// the drawing into the frame where a list cannot be made.
func (p *painter) canvas(v *view, width, height float32) {
	c := &v.canvasCache
	if c.drawing != v.drawing {
		c.setDrawing(v.drawing)
	}
	scale := p.scale
	if c.mixes {
		scale = offscreenScale(width, height, p.scale, p.m)
	}
	if c.scale != scale || c.generation != gfxGeneration {
		c.release()
		c.scale, c.generation = scale, gfxGeneration
	}
	ops := p.w.animator.CanvasOps(v.id, v.drawing)
	if c.shaded || c.atlas && !atlasListTransform(p.m, p.scale) {
		p.playCanvas(v, ops, width, height)
		return
	}
	if c.head == nil {
		if c.split.At > 0 {
			c.head = p.recordCanvas(v, ops[:c.split.At], driver.CanvasSplit{}, width, height, scale)
		} else {
			c.head = p.recordCanvas(v, ops, driver.CanvasSplit{}, width, height, scale)
		}
	}
	if c.split.At > 0 && c.tail == nil {
		c.tail = p.recordCanvas(v, ops, c.split, width, height, scale)
	}
	if c.head == nil || c.split.At > 0 && c.tail == nil {
		p.playCanvas(v, ops, width, height)
		return
	}
	p.setTransform(p.m)
	for _, list := range []*comObject{c.head, c.tail} {
		if list != nil {
			p.dc.call(dcDrawImage, list.ptr(), 0, 0, d2dInterpolationLinear, 0)
		}
	}
}

// playCanvas plays ops into the frame.
func (p *painter) playCanvas(v *view, ops []driver.DrawOp, width, height float32) {
	mixes := false
	for i := range ops {
		if op := &ops[i]; op.Kind == driver.DrawClear || op.Kind == driver.DrawComposite && op.Composite != 0 {
			mixes = true
			break
		}
	}
	if mixes && p.canvasOffscreen(v, ops, width, height, offscreenScale(width, height, p.scale, p.m)) {
		return
	}
	c := p.newCanvasPlayer(v, p.m, width, height)
	c.playAll(ops)
}

// recordCanvas plays ops into a new command list on the recording
// context, leaving out what split skips; nil when it cannot.
func (p *painter) recordCanvas(v *view, ops []driver.DrawOp, split driver.CanvasSplit, width, height, scale float32) *comObject {
	rec := p.g.record
	var list *comObject
	if failed(rec.call(dcCreateCommandList, uintptr(unsafe.Pointer(&list)))) {
		return nil
	}
	rec.call(dcSetTarget, list.ptr())
	rec.callF(rtSetDpi, 0b11, f32(96*scale), f32(96*scale))
	rec.call(rtBeginDraw)
	// The brush is the device's, which any of its contexts draws with.
	r := &painter{w: p.w, g: p.g, dc: rec, scale: scale, m: identity, alpha: 1, brush: p.brush}
	r.setTransform(identity)
	mixes := false
	for i := range ops {
		if op := &ops[i]; !split.Skips(i, op) &&
			(op.Kind == driver.DrawClear || op.Kind == driver.DrawComposite && op.Composite != 0) {
			mixes = true
			break
		}
	}
	if !(mixes && split.At == 0 && r.canvasOffscreen(v, ops, width, height, scale)) {
		c := r.newCanvasPlayer(v, identity, width, height)
		c.split = split
		c.playAll(ops)
	}
	for len(r.pushed) > 0 {
		r.pop()
	}
	hr := rec.call(rtEndDraw, 0, 0)
	rec.call(dcSetTarget, 0)
	if failed(hr) || failed(list.call(commandListClose)) {
		list.release()
		return nil
	}
	return list
}
