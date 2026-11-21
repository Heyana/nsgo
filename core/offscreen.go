package core

import (
	"image"
	"math"
	"slices"
	"sync/atomic"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var nextOffscreenID atomic.Uint64

// OffscreenImage is a retained 2D drawing that platforms render once
// into an offscreen image surface and reuse across later frames without
// re-executing its drawing operations.
//
// Source returns an ImageSource directly usable in Drawing.DrawImage,
// Drawing.DrawImageIn, Drawing.DrawImageFrom, and Image. The offscreen
// drawing is recorded lazily on first use and re-recorded only when
// Invalidate or Resize is called (or when newly registered fonts become
// available). Call Release to drop the cached surface early; it is also
// released automatically when the scope that created it is disposed.
type OffscreenImage struct {
	id       uint64
	owner    *Scope
	tree     *Tree
	size     Size
	scale    float32
	draw     func(*Drawing)
	rev      *Signal[uint64]
	version  uint64
	dirty    bool
	released bool
	snapshot *driver.OffscreenCanvas
	frames   map[pixelKey]*image.RGBA
}

// NewOffscreenImage creates a retained offscreen image of logical size
// and backing scale (pixels per point; values <= 0 default to the
// current display pixel ratio, or 1 when unavailable) whose contents are
// recorded by draw. The offscreen image is automatically released when
// cx is disposed.
func (cx *Scope) NewOffscreenImage(size Size, scale float64, draw func(*Drawing)) *OffscreenImage {
	owner := cx.current()
	t := owner.tree()
	o := &OffscreenImage{
		id:    nextOffscreenID.Add(1),
		owner: owner,
		tree:  t,
		size:  size,
		scale: resolveOffscreenScale(t, scale),
		draw:  draw,
		rev:   NewSignal(owner, uint64(0)),
		dirty: true,
	}
	if t != nil {
		if t.offscreens == nil {
			t.offscreens = map[uint64]*OffscreenImage{}
		}
		t.offscreens[o.id] = o
	}
	owner.addCleanup(o.Release)
	return o
}

func resolveOffscreenScale(t *Tree, scale float64) float32 {
	if scale > 0 && !math.IsInf(scale, 0) {
		return float32(scale)
	}
	if t != nil {
		if r := t.root.Env().ratio.GetUntracked(); r > 0 {
			return r
		}
	}
	return 1
}

// ID returns the unique identifier of this offscreen image resource.
func (o *OffscreenImage) ID() uint64 {
	if o == nil {
		return 0
	}
	return o.id
}

// Size returns the logical size of the offscreen image in points.
func (o *OffscreenImage) Size() Size {
	if o == nil {
		return Size{}
	}
	return o.size
}

// Scale returns the backing pixel scale (pixels per point).
func (o *OffscreenImage) Scale() float64 {
	if o == nil {
		return 1
	}
	return float64(o.scale)
}

// Version returns the version of the most recently recorded snapshot, or
// 0 before Source has recorded the drawing.
func (o *OffscreenImage) Version() uint64 {
	if o == nil {
		return 0
	}
	return o.version
}

// Released reports whether Release has been called (directly or through
// scope disposal).
func (o *OffscreenImage) Released() bool {
	return o == nil || o.released
}

// Source returns an ImageSource representing this offscreen image.
// Reading Source inside a Canvas.Draw or Image.SourceFunc subscribes
// that view to Invalidate, Resize, and Release. While clean, repeated
// calls return the same snapshot without re-running the draw callback.
func (o *OffscreenImage) Source() ImageSource {
	if o == nil {
		return ImageSource{}
	}
	if o.rev != nil {
		_ = o.rev.Get()
	}
	if o.released || !(o.size.Width > 0) || !(o.size.Height > 0) {
		return ImageSource{}
	}
	if o.dirty || o.snapshot == nil {
		o.record()
	}
	if o.snapshot == nil {
		return ImageSource{}
	}
	return ImageSource{
		Offscreen: o.snapshot,
		Size:      o.snapshot.Size,
		Version:   o.snapshot.Version,
	}
}

func (o *OffscreenImage) record() {
	var text driver.TextEngine
	if o.tree != nil {
		text = o.tree.text
	}
	d := driver.NewDrawing(text)
	if o.draw != nil {
		if o.owner != nil && o.owner.rt != nil {
			Untracked(o.owner, func() struct{} {
				o.draw(d)
				return struct{}{}
			})
		} else {
			o.draw(d)
		}
	}
	if o.tree != nil {
		o.tree.pickDrawingScales(d)
		if o.tree.rt.restricted {
			o.tree.refuseDrawingImagesOwner(o.owner, d)
		}
	}
	d, o.frames = copyDrawingFrames(d, o.frames)
	o.version++
	o.snapshot = &driver.OffscreenCanvas{
		ID:      o.id,
		Version: o.version,
		Size:    o.size,
		Scale:   o.scale,
		Drawing: d,
	}
	o.dirty = false
}

// Invalidate marks the offscreen image dirty so its drawing is
// re-recorded and re-rendered on the next frame that uses it. Multiple
// invalidations before the next commit coalesce into one re-render.
func (o *OffscreenImage) Invalidate() {
	if o == nil || o.released || o.dirty {
		return
	}
	o.dirty = true
	if o.rev != nil {
		o.rev.Set(o.rev.GetUntracked() + 1)
	}
}

// Resize changes the logical size and backing scale (pixels per point;
// <= 0 defaults to the display pixel ratio or 1) and marks the offscreen
// image dirty when either changes.
func (o *OffscreenImage) Resize(size Size, scale float64) {
	if o == nil || o.released {
		return
	}
	s := resolveOffscreenScale(o.tree, scale)
	if size == o.size && s == o.scale {
		return
	}
	o.size, o.scale = size, s
	o.dirty = true
	if o.rev != nil {
		o.rev.Set(o.rev.GetUntracked() + 1)
	}
}

// Release frees the offscreen image and instructs the platform to drop
// its cached surface. Calling Release more than once is safe.
func (o *OffscreenImage) Release() {
	if o == nil || o.released {
		return
	}
	o.released = true
	o.dirty = false
	o.snapshot = nil
	o.frames = nil
	if o.tree != nil {
		delete(o.tree.offscreens, o.id)
		o.tree.commands = append(o.tree.commands, driver.MountItem{
			Kind:        driver.ItemReleaseOffscreen,
			OffscreenID: o.id,
		})
		o.tree.changed = true
		o.tree.rt.maybeFlush()
	}
	if o.rev != nil {
		o.rev.Set(o.rev.GetUntracked() + 1)
	}
}

// copyDrawingFrames returns d with any Pixels image sources copied once
// per (buffer, version), reusing entries from prev for unchanged frames.
func copyDrawingFrames(d *Drawing, prev map[pixelKey]*image.RGBA) (*Drawing, map[pixelKey]*image.RGBA) {
	if !slices.ContainsFunc(d.Ops, func(op driver.DrawOp) bool { return op.Image != nil && op.Image.Pixels != nil }) {
		return d, nil
	}
	out := *d
	out.Ops = slices.Clone(d.Ops)
	frames := map[pixelKey]*image.RGBA{}
	for i := range out.Ops {
		op := &out.Ops[i]
		if op.Image == nil || op.Image.Pixels == nil {
			continue
		}
		if op.Image.Pixels.Rect.Empty() {
			// A buffer with no pixels draws nothing.
			op.Image = nil
			continue
		}
		k := pixelKey{op.Image.Pixels, op.Image.Version}
		c := frames[k]
		if c == nil {
			if c = prev[k]; c == nil {
				c = driver.CopyPixels(k.from)
			}
			frames[k] = c
		}
		img := *op.Image
		img.Pixels = c
		op.Image = &img
	}
	return &out, frames
}
