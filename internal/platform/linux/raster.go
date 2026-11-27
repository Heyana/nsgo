//go:build linux && !android

package linux

import (
	"math"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A canvas is rasterized with Cairo into an image surface at the
// surface's scale and shown as textures of tile pixels a side (one for a
// stretched raster, tileSide), as NSH's
// Linux canvases are (canvasRaster): a frame in which the canvas did not
// change appends the same textures, which GSK keeps on the GPU, and only
// a new drawing, size or scale plays it whole. A change of its bound
// arguments draws again only what driver.CanvasDamage says they cover:
// the area is cleared and the drawing played clipped to it, and only the
// tiles it touches become new textures. What the drawing's static part
// (driver.CanvasSplit) painted is kept from the last full play and
// copied back into the area, so only the painting after it plays again.
//
// A canvas whose drawing asks for fewer pixels (driver.Drawing.Resolution)
// rasterizes at that factor of the scale and GSK stretches its textures
// over its bounds with the drawing's Upscale filter.
//
// A canvas out of sight, outside a clipping ancestor or the window, is
// not drawn: what changed waits, and it draws once a layout shows it
// (revealCanvases).

var (
	cairoSurfaceSetDeviceScale func(surface uintptr, x, y float64)
	cairoSurfaceStatus         func(surface uintptr) int32
	gdkMemoryTextureNew        func(w, h, format int32, bytes, stride uintptr) uintptr
	gtkSnapshotAppendTexture   func(snap, texture uintptr, bounds *grapheneRect)
	// gtkSnapshotAppendScaledTexture (GTK 4.10) stretches a texture with
	// a GskScalingFilter.
	gtkSnapshotAppendScaledTexture func(snap, texture uintptr, filter int32, bounds *grapheneRect)
)

// The GskScalingFilter values.
const (
	gskScalingFilterLinear  = 0
	gskScalingFilterNearest = 1
)

func bindRaster(cairoLib uintptr) {
	reg := purego.RegisterLibFunc
	reg(&cairoSurfaceSetDeviceScale, cairoLib, "cairo_surface_set_device_scale")
	reg(&cairoSurfaceStatus, cairoLib, "cairo_surface_status")
	reg(&gdkMemoryTextureNew, gtkLib, "gdk_memory_texture_new")
	bindSyscall(&gtkSnapshotAppendTexture, gtkLib, "gtk_snapshot_append_texture")
	bindSyscall(&gtkSnapshotAppendScaledTexture, gtkLib, "gtk_snapshot_append_scaled_texture")
}

// tile is the side of a canvas's textures, in pixels.
const tile = 256

// maxStretched is the most pixels a side of a stretched raster
// (driver.Drawing.Resolution) shows as one texture: GSK filters each
// texture on its own, so a smooth stretch across tiles would show their
// seams.
const maxStretched = 4096

// maxRaster is the most pixels a side of a Cairo image surface has: a
// canvas wider or taller is not drawn.
const maxRaster = 32767

// canvasRaster is a canvas's pixels.
type canvasRaster struct {
	surface uintptr // ARGB32 at scale
	pw, ph  int
	scale   float64
	// side is the side of its textures, in pixels (tileSide); zero is
	// tile.
	side int
	// tiles are the surface's textures, row by row.
	tiles []uintptr
	// static is what the ops before the split painted, or 0.
	static uintptr
}

// canvasDraw is what of a canvas must be drawn again.
type canvasDraw struct {
	// split is where its static part ends, At 0 when none is kept.
	split driver.CanvasSplit
	// last are the ops as its bound arguments last changed them, for the
	// damage of the next change.
	last []driver.DrawOp
	// whole or area, in points, must be drawn again.
	whole bool
	area  driver.Bounds
	// hidden is set while it waits out of sight with something to draw.
	hidden bool
	raster *canvasRaster
}

// setCanvas takes v's new drawing, drawn whole.
func (w *window) setCanvas(v *view, d *driver.Drawing) {
	was := v.drawing
	v.drawing = d
	was.Release()
	c := &v.canvas
	c.split, c.last, c.whole, c.area = driver.CanvasSplit{}, c.last[:0], true, driver.Bounds{}
	if s := driver.SplitCanvas(d); s.Static(d) {
		c.split = s
	}
	w.animator.BindCanvas(v.id, d.Slots)
	w.redrawCanvas(v)
}

// redrawCanvas has v drawn again unless it waits out of sight.
func (w *window) redrawCanvas(v *view) {
	if !v.canvas.hidden {
		w.changed(v)
	}
}

// changedCanvases draws again what the canvases' bound arguments
// changed.
func (w *window) changedCanvases() {
	w.animator.ChangedCanvases(func(id driver.NodeID) {
		v := w.view(id)
		if v == nil || v.drawing == nil {
			return
		}
		c := &v.canvas
		ops := w.animator.CanvasOps(v.id, v.drawing)
		if !c.whole {
			if len(c.last) != len(ops) {
				c.whole = true
			} else if area, ok := driver.CanvasDamage(v.drawing, c.last, ops); !ok {
				c.whole = true
			} else if area.Size.Width > 0 && area.Size.Height > 0 {
				c.area = union(c.area, area)
			} else {
				c.last = append(c.last[:0], ops...)
				return
			}
		}
		c.last = append(c.last[:0], ops...)
		w.redrawCanvas(v)
	})
}

// union is the bounds covering a and b, either of which may be empty.
func union(a, b driver.Bounds) driver.Bounds {
	if a.Size.Width <= 0 || a.Size.Height <= 0 {
		return b
	}
	x0, y0 := min(a.Origin.X, b.Origin.X), min(a.Origin.Y, b.Origin.Y)
	x1 := max(a.Origin.X+a.Size.Width, b.Origin.X+b.Size.Width)
	y1 := max(a.Origin.Y+a.Size.Height, b.Origin.Y+b.Size.Height)
	return driver.Bounds{Origin: driver.Point{X: x0, Y: y0}, Size: driver.Size{Width: x1 - x0, Height: y1 - y0}}
}

// canvasRasterScale returns the pixels per point canvas view v rasterizes at: the window's pixel ratio times the stepped scale of v and every ancestor's style transform, keyframe/layout scale, and enclosing scroll zooms, capped at 4× and bounded within 4096 × 4096 pixels and maxRaster per side.
func (w *window) canvasRasterScale(v *view, width, height float64) float64 {
	base := max(float64(w.env.PixelRatio), 1)
	zoom := 1.0
	for cur := v; cur != nil; cur = cur.parent {
		s := cur.shownStyle().Transform.RasterScale()
		if k, ok := cur.keyframe(); ok && k.Scale > 0 {
			s *= float32(math.Abs(float64(k.Scale)))
		}
		if ls := cur.layoutScale(); ls > 0 {
			s *= ls
		}
		if step := driver.RasterStep(s); step > 1 {
			zoom = min(zoom*step, 4)
		}
		if cur.parent != nil && cur.parent.kind == driver.KindScroll && cur.parent.scroll.Zooms() {
			if step := driver.RasterStep(float32(cur.parent.zoomScale())); step > 1 {
				zoom = min(zoom*step, 4)
			}
		}
	}
	if zoom <= 1 || width <= 0 || height <= 0 {
		return base
	}
	const maxZoomPixels = 4096 * 4096
	const maxZoomSide = 16384
	s := base * zoom
	s = min(s, math.Sqrt(maxZoomPixels/(width*height)), float64(min(maxZoomSide, maxRaster))/max(width, height))
	return max(s, base)
}

// snapshotCanvas appends v's raster, drawn again where it changed,
// unless v is out of sight.
func (w *window) snapshotCanvas(v *view, snap uintptr, width, height float64) {
	c := &v.canvas
	if len(v.drawing.Ops) == 0 {
		return
	}
	pw, ph, scale := w.canvasPixels(v, width, height)
	r := c.raster
	fits := r != nil && r.scale == scale && r.pw == pw && r.ph == ph
	dirty := c.whole || c.area.Size.Width > 0
	if (!fits || dirty) && !w.inSight(v) {
		if !c.hidden {
			c.hidden = true
			w.hiddenCanvases = append(w.hiddenCanvases, v)
			w.watchRevealClock()
		}
		return
	}
	if !fits || dirty {
		if !fits {
			c.whole = true
		}
		w.rasterize(v, pw, ph, scale)
		c.whole, c.area = false, driver.Bounds{}
		if len(c.last) == 0 {
			c.last = append(c.last, w.animator.CanvasOps(v.id, v.drawing)...)
		}
	}
	r = c.raster
	if r == nil || len(r.tiles) != tiles(pw, ph, r.textureSide()) {
		return
	}
	appendTiles(snap, r, scale, v.drawing.ResolutionScale() < 1, v.drawing.Smooth())
}

// canvasPixels returns the pixels a side canvas view v, width × height
// points, rasterizes into, and the pixels per point: canvasRasterScale's
// times its drawing's resolution scale, at least a pixel a side.
func (w *window) canvasPixels(v *view, width, height float64) (pw, ph int, scale float64) {
	factor := v.drawing.ResolutionScale()
	scale = w.canvasRasterScale(v, width, height) * factor
	if factor < 1 {
		return driver.CanvasPixels(driver.Points(width), scale), driver.CanvasPixels(driver.Points(height), scale), scale
	}
	return int(math.Ceil(width * scale)), int(math.Ceil(height * scale)), scale
}

// appendTiles appends r's textures, at scale pixels a point, to snap:
// stretched with the filter smooth names when stretched is set, at GSK's
// own otherwise.
func appendTiles(snap uintptr, r *canvasRaster, scale float64, stretched, smooth bool) {
	pw, ph := r.pw, r.ph
	filter := int32(gskScalingFilterLinear)
	if !smooth {
		filter = gskScalingFilterNearest
	}
	i := 0
	side := r.textureSide()
	for y := 0; y < ph; y += side {
		for x := 0; x < pw; x += side {
			tw, th := min(side, pw-x), min(side, ph-y)
			at := grapheneRect{float32(float64(x) / scale), float32(float64(y) / scale),
				float32(float64(tw) / scale), float32(float64(th) / scale)}
			if stretched {
				gtkSnapshotAppendScaledTexture(snap, r.tiles[i], filter, &at)
			} else {
				gtkSnapshotAppendTexture(snap, r.tiles[i], &at)
			}
			i++
		}
	}
}

// rasterize plays v's drawing into its raster where it must be drawn
// again, into a new one at pw × ph pixels when the size or scale
// changed, and makes the textures of the tiles it touched.
func (w *window) rasterize(v *view, pw, ph int, scale float64) {
	c := &v.canvas
	r := c.raster
	if r == nil || r.scale != scale || r.pw != pw || r.ph != ph {
		r.release()
		c.raster = nil
		if pw <= 0 || ph <= 0 || pw > maxRaster || ph > maxRaster {
			return
		}
		surf := cairoImageSurfaceCreate(0, int32(pw), int32(ph))
		if surf == 0 {
			return
		}
		if cairoSurfaceStatus(surf) != 0 {
			cairoSurfaceDestroy(surf)
			return
		}
		cairoSurfaceSetDeviceScale(surf, scale, scale)
		r = &canvasRaster{surface: surf, pw: pw, ph: ph, scale: scale,
			side: tileSide(pw, ph, v.drawing.ResolutionScale() < 1)}
		c.raster = r
		c.whole = true
	}
	cr := cairoCreate(r.surface)
	split := c.split
	// The area drawn again, in whole pixels.
	x0, y0, x1, y1 := 0, 0, pw, ph
	over := false
	if c.whole {
		if split.At == 0 && r.static != 0 {
			cairoSurfaceDestroy(r.static)
			r.static = 0
		}
		cairoSetOperator(cr, operatorClear)
		cairoPaint(cr)
		cairoSetOperator(cr, operatorSourceOver)
	} else {
		const aa = 2
		a := c.area
		px := func(v float64, round func(float64) float64, hi int) int {
			return min(max(int(round(v*scale)), 0), hi)
		}
		x0, y0 = px(math.Floor(float64(a.Origin.X)-aa), math.Floor, pw), px(math.Floor(float64(a.Origin.Y)-aa), math.Floor, ph)
		x1 = px(math.Ceil(float64(a.Origin.X+a.Size.Width)+aa), math.Ceil, pw)
		y1 = px(math.Ceil(float64(a.Origin.Y+a.Size.Height)+aa), math.Ceil, ph)
		cairoRectangle(cr, float64(x0)/scale, float64(y0)/scale, float64(x1-x0)/scale, float64(y1-y0)/scale)
		cairoClip(cr)
		if r.static != 0 {
			cairoSetSourceSurface(cr, r.static, 0, 0)
			cairoSetOperator(cr, operatorSource)
			over = true
		} else {
			cairoSetOperator(cr, operatorClear)
		}
		cairoPaint(cr)
		cairoSetOperator(cr, operatorSourceOver)
		cairoSetSourceRGBA(cr, 0, 0, 0, 0)
	}
	ops := w.animator.CanvasOps(v.id, v.drawing)
	size := rect{w: float64(pw) / scale, h: float64(ph) / scale}
	switch {
	case over:
		w.playCanvasFrom(v, cr, size, ops, split, nil)
	case c.whole && split.At > 0:
		// A full play keeps what the static part painted.
		w.playCanvasFrom(v, cr, size, ops, driver.CanvasSplit{}, func(at int) {
			if at == split.At {
				r.keepStatic()
			}
		})
	default:
		w.playCanvasFrom(v, cr, size, ops, driver.CanvasSplit{}, nil)
	}
	cairoDestroy(cr)
	cairoSurfaceFlush(r.surface)
	r.texture(x0, y0, x1, y1)
}

// tiles is how many textures show pw × ph pixels.
func tiles(pw, ph, side int) int {
	return (pw + side - 1) / side * ((ph + side - 1) / side)
}

// textureSide is the side of r's textures, in pixels.
func (r *canvasRaster) textureSide() int {
	if r.side > 0 {
		return r.side
	}
	return tile
}

// tileSide is the side of the textures that show pw × ph pixels: one
// texture for a raster stretched to its view (maxStretched), tile else.
func tileSide(pw, ph int, stretched bool) int {
	if stretched && pw <= maxStretched && ph <= maxStretched {
		return max(pw, ph, 1)
	}
	return tile
}

// keepStatic copies the surface's pixels into static.
func (r *canvasRaster) keepStatic() {
	cairoSurfaceFlush(r.surface)
	if r.static == 0 {
		r.static = cairoImageSurfaceCreate(0, int32(r.pw), int32(r.ph))
		if r.static == 0 {
			return
		}
		if cairoSurfaceStatus(r.static) != 0 {
			cairoSurfaceDestroy(r.static)
			r.static = 0
			return
		}
		cairoSurfaceSetDeviceScale(r.static, r.scale, r.scale)
	}
	cr := cairoCreate(r.static)
	cairoSetSourceSurface(cr, r.surface, 0, 0)
	cairoSetOperator(cr, operatorSource)
	cairoPaint(cr)
	cairoDestroy(cr)
	cairoSurfaceFlush(r.static)
}

// texture makes the textures of the tiles that meet the pixels from
// (x0, y0) to (x1, y1), and of all of them the first time.
func (r *canvasRaster) texture(x0, y0, x1, y1 int) {
	data := cairoImageSurfaceGetData(r.surface)
	stride := int(cairoImageSurfaceGetStride(r.surface))
	if data == 0 {
		return
	}
	pix := unsafe.Slice((*byte)(cPointer(data)), stride*r.ph)
	fresh := len(r.tiles) == 0
	var row []byte
	i := 0
	side := r.textureSide()
	for y := 0; y < r.ph; y += side {
		for x := 0; x < r.pw; x += side {
			tw, th := min(side, r.pw-x), min(side, r.ph-y)
			if fresh {
				r.tiles = append(r.tiles, 0)
			}
			if r.tiles[i] != 0 && (x >= x1 || x+tw <= x0 || y >= y1 || y+th <= y0) {
				i++
				continue
			}
			var bytes uintptr
			if x == 0 && tw*4 == stride {
				bytes = gBytesNew(unsafe.Pointer(&pix[y*stride]), uintptr(th*stride))
			} else {
				row = row[:0]
				for yy := y; yy < y+th; yy++ {
					row = append(row, pix[yy*stride+x*4:yy*stride+(x+tw)*4]...)
				}
				bytes = gBytesNew(unsafe.Pointer(&row[0]), uintptr(len(row)))
			}
			// Cairo's ARGB32 is premultiplied and native-endian: BGRA in
			// memory on little-endian machines.
			const bgraPremultiplied = 0
			t := gdkMemoryTextureNew(int32(tw), int32(th), bgraPremultiplied, bytes, uintptr(tw*4))
			gBytesUnref(bytes)
			if r.tiles[i] != 0 {
				gObjectUnref(r.tiles[i])
			}
			r.tiles[i] = t
			i++
		}
	}
}

// release lets r's surfaces and textures go; r may be nil.
func (r *canvasRaster) release() {
	if r == nil {
		return
	}
	for _, t := range r.tiles {
		if t != 0 {
			gObjectUnref(t)
		}
	}
	r.tiles = nil
	if r.static != 0 {
		cairoSurfaceDestroy(r.static)
		r.static = 0
	}
	if r.surface != 0 {
		cairoSurfaceDestroy(r.surface)
		r.surface = 0
	}
}

// inSight reports whether some of v may show: false only when it lies
// wholly outside a clipping ancestor or the window.
func (w *window) inSight(v *view) bool {
	if v.widget == 0 || gtkWidgetGetMapped(v.widget) == 0 {
		return false
	}
	for a := gtkWidgetGetParent(v.widget); a != 0; a = gtkWidgetGetParent(a) {
		ref, ok := lookupWidget(a)
		if !ok {
			continue
		}
		root := ref.v == nil // the surface: the window's bounds
		if !root && !clips(ref.v) {
			continue
		}
		var b grapheneRect
		if gtkWidgetComputeBounds(v.widget, a, &b) == 0 {
			return true
		}
		aw, ah := float32(gtkWidgetGetWidth(a)), float32(gtkWidgetGetHeight(a))
		if b.x >= aw || b.y >= ah || b.x+b.w <= 0 || b.y+b.h <= 0 {
			return false
		}
		if root {
			return true
		}
	}
	return true
}

// clips reports whether v clips its children, as snapshotView does.
func clips(v *view) bool {
	return v.style.Clip || v.kind == driver.KindScroll || v.kind == driver.KindImage || v.kind == driver.KindInput
}

// watchRevealClock connects cbRevealLayout after GTK's frame clock layout
// phase so hidden canvases coming into sight draw even when w.surface itself
// is not redrawn on that frame.
func (w *window) watchRevealClock() {
	if w.surface == 0 {
		return
	}
	if gtkWidgetGetFrameClock == nil {
		bindSyscall(&gtkWidgetGetFrameClock, gtkLib, "gtk_widget_get_frame_clock")
	}
	if clock := gtkWidgetGetFrameClock(w.surface); clock != 0 && clock != w.revealClock {
		w.revealClock = clock
		const gConnectAfter = 1 << 0
		gSignalConnectData(clock, cstr("layout"), cbRevealLayout, w.handle, 0, gConnectAfter)
	}
}

func revealLayoutCallback(clock, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing || clock != w.revealClock || !w.revealDue {
		return
	}
	w.revealDue = false
	w.revealCanvases()
}

// redrawCanvasesUnder queues a redraw for any canvas view in v's subtree when
// an ancestor's transform scale changes (so canvasRasterScale updates cleanly
// even when reposition keeps ancestor render nodes cached).
func (w *window) redrawCanvasesUnder(v *view) {
	if v == nil || v.deleted {
		return
	}
	if v.kind == driver.KindCanvas && v.drawing != nil {
		w.redrawCanvas(v)
	}
	for _, c := range v.children {
		w.redrawCanvasesUnder(c)
	}
}

// revealCanvases draws the canvases waiting out of sight that a layout
// showed; the others wait on.
func (w *window) revealCanvases() {
	if len(w.hiddenCanvases) == 0 {
		return
	}
	waiting := w.hiddenCanvases[:0]
	for _, v := range w.hiddenCanvases {
		switch {
		case v.deleted || v.drawing == nil:
			v.canvas.hidden = false
		case !w.inSight(v):
			waiting = append(waiting, v)
		default:
			v.canvas.hidden = false
			w.changed(v)
		}
	}
	clear(w.hiddenCanvases[len(waiting):])
	w.hiddenCanvases = waiting
}
