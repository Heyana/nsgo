//go:build js && wasm

package web

import (
	"math"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvas2d"
	"github.com/nsgo-dev/nsgo/internal/platform/canvasmesh"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

// setDrawing keeps a canvas view's drawing, binds its animated arguments
// and plays it.
func (s *surface) setDrawing(v *view, d *driver.Drawing) {
	v.drawing = d
	s.animator.BindCanvas(v.id, d.Slots)
	s.paint(v)
	if d != nil {
		s.describeNodes(v, d.Nodes)
	}
}

// changedCanvases plays again the canvases whose bound arguments changed.
func (s *surface) changedCanvases() {
	s.animator.ChangedCanvases(func(id driver.NodeID) {
		if v := s.view(id); v != nil && v.kind == driver.KindCanvas {
			s.paint(v)
		}
	})
}

// changedNatives hands components the animated props that changed.
func (s *surface) changedNatives() {
	s.animator.ChangedNatives(func(id driver.NodeID, b driver.NativeBinding, num float64, col driver.Color) {
		if v := s.view(id); v != nil {
			driver.SetAnimatedProp(v.component, b, num, col)
		}
	})
}

// canvasRect is a sub-rectangle of a canvas in its drawing's points.
type canvasRect struct {
	x, y, w, h float64
}

func (r canvasRect) contains(o canvasRect) bool {
	if o.w <= 0 || o.h <= 0 {
		return true
	}
	return r.w > 0 && r.h > 0 && o.x >= r.x && o.y >= r.y && o.x+o.w <= r.x+r.w && o.y+o.h <= r.y+r.h
}

// paint plays a canvas view's drawing into its <canvas>, whose pixels follow the pixel ratio, enclosing scroll zooms and style transforms. A canvas whose full bitmap would pass maxZoomPixels or maxZoomSide draws only its visible part (with overscan) positioned inside the view so it stays sharp within the budget. The drawing's origin is inside the border.
func (s *surface) paint(v *view) {
	if v.drawing == nil {
		return
	}
	w, h := v.canvasSize()
	tile, ratio, sliced := s.canvasPlan(v, w, h)
	v.canvasRatio = ratio
	v.canvasTile = tile
	if sliced {
		if s.slicedCanvases == nil {
			s.slicedCanvases = map[*view]bool{}
		}
		s.slicedCanvases[v] = true
		v.canvasSliced = true
		setStyle(v.inner, map[string]string{
			"left":   webstyle.Px(driver.Points(tile.x)),
			"top":    webstyle.Px(driver.Points(tile.y)),
			"width":  webstyle.Px(driver.Points(tile.w)),
			"height": webstyle.Px(driver.Points(tile.h)),
		})
	} else {
		delete(s.slicedCanvases, v)
		if v.canvasSliced {
			v.canvasSliced = false
			setStyle(v.inner, map[string]string{"left": "0", "top": "0", "width": "100%", "height": "100%"})
		}
	}
	pw, ph := int(math.Ceil(tile.w*ratio)), int(math.Ceil(tile.h*ratio))
	// Setting the size clears the canvas and resets its context.
	v.inner.Set("width", pw)
	v.inner.Set("height", ph)
	if pw > 0 && ph > 0 {
		ctx := v.inner.Call("getContext", "2d")
		ctx.Call("setTransform", ratio, 0, 0, ratio, -tile.x*ratio, -tile.y*ratio)
		p := canvas2d.NewPlayer(ctx, ratio, math.Hypot(w, h))
		p.OriginX, p.OriginY = tile.x, tile.y
		p.Font = fontCSS
		p.NewCanvas = s.scratchCanvas
		p.Image = func(src driver.ImageSource, readable bool) (js.Value, float64, bool) {
			im := s.loadCanvasImage(src, v, readable)
			return im.img, im.density, im.loaded
		}
		p.MeshCanvas = s.scratchMeshCanvas
		p.Texture = func(src driver.ImageSource) canvasmesh.Texture {
			tex, _ := s.meshTexture(src, v)
			return tex
		}
		ops := s.animator.CanvasOps(v.id, v.drawing)
		for i := range ops {
			p.Play(&ops[i])
		}
	}
	s.canvasImages.endPass()
	s.setCanvasExact(v)
	s.requestGlassPaint()
}

// setCanvasExact draws a canvas inside zoomed scroll contents or an axis-aligned scale transform pixel for pixel while its bitmap is exactly as large as the screen shows it. Chrome composes nested scale transforms with filtering even when they add up to one pixel per pixel, which greys a sharp edge; nearest sampling of a bitmap at that size changes nothing else. During a zoom, under rotation/skew, or past the raster bounds, the bitmap is scaled for the screen and keeps smooth sampling.
func (s *surface) setCanvasExact(v *view) {
	zoom, zoomed := s.canvasZoom(v)
	tw, th := v.canvasTile.w, v.canvasTile.h
	shown := float64(s.env.PixelRatio) * zoom
	exact := zoomed && s.canvasAxisAligned(v) && tw > 0 && th > 0 &&
		math.Abs(v.canvasRatio-shown) < 1e-9 && tw*shown == math.Ceil(tw*shown) && th*shown == math.Ceil(th*shown)
	if exact == v.canvasExact {
		return
	}
	v.canvasExact = exact
	rendering := ""
	if exact {
		rendering = "pixelated"
	}
	v.inner.Get("style").Set("imageRendering", rendering)
}

// smoothCanvases returns the canvases under root to smooth sampling while a zoom changes how the screen shows their bitmaps; the replay after it sets them again.
func (s *surface) smoothCanvases(root js.Value) {
	canvases := root.Call("querySelectorAll", "canvas")
	for i := range canvases.Length() {
		if v := s.viewOf(canvases.Index(i).Get("parentElement")); v != nil && v.canvasExact {
			v.canvasExact = false
			v.inner.Get("style").Set("imageRendering", "")
		}
	}
}

// canvasSize is the size in points of a canvas view's <canvas>, inside its border.
func (v *view) canvasSize() (w, h float64) {
	return math.Max(float64(v.size.Width-v.border.Left-v.border.Right), 0), math.Max(float64(v.size.Height-v.border.Top-v.border.Bottom), 0)
}

const (
	// maxRasterZoom bounds how far scroll zoom and style transforms raise a canvas's pixels, as apple.RasterStep does.
	maxRasterZoom = 4
	// maxZoomPixels and maxZoomSide bound the bitmap a canvas replay may make: 64 MB, and within the side every browser accepts, since a larger canvas draws nothing in some browsers.
	maxZoomPixels = 4096 * 4096
	maxZoomSide   = 16384
	// skewAttr marks a view whose transform rotates, skews or tilts out of the plane.
	skewAttr = "data-nsgo-skew"
)

// boundCanvasRatio clamps fullRatio for a w×h point bitmap so its pixels stay within maxZoomPixels and maxZoomSide, never below baseRatio when baseRatio alone fits (or keeping baseRatio when even baseRatio alone exceeds the budget).
func boundCanvasRatio(w, h, baseRatio, fullRatio float64) float64 {
	if fullRatio <= baseRatio || w <= 0 || h <= 0 {
		return baseRatio
	}
	zoom := fullRatio / baseRatio
	zoom = min(zoom, math.Sqrt(maxZoomPixels/(w*h*baseRatio*baseRatio)), maxZoomSide/(max(w, h)*baseRatio))
	zoom = max(zoom, 1)
	return baseRatio * zoom
}

// canvasPlan returns the drawing-point rectangle, raster ratio and whether only a visible slice is drawn for canvas v of size w×h points.
func (s *surface) canvasPlan(v *view, w, h float64) (tile canvasRect, ratio float64, sliced bool) {
	zoom, _ := s.canvasZoom(v)
	base := float64(s.env.PixelRatio)
	if base <= 0 {
		base = 1
	}
	fullRatio := base * min(max(zoom, 1), maxRasterZoom)
	full := canvasRect{w: w, h: h}
	if w <= 0 || h <= 0 {
		return full, base, false
	}
	fw, fh := math.Ceil(w*fullRatio), math.Ceil(h*fullRatio)
	if fw*fh <= maxZoomPixels && fw <= maxZoomSide && fh <= maxZoomSide {
		return full, fullRatio, false
	}
	// Past the full-canvas bitmap budget: draw only the visible part at full resolution when the canvas is axis-aligned and laid out in the page.
	if s.canvasAxisAligned(v) {
		if vis, ok := s.visibleCanvasRect(v, w, h); ok {
			if vis.w <= 0 || vis.h <= 0 {
				return canvasRect{}, fullRatio, true
			}
			padded := padVisibleTile(vis, w, h, fullRatio)
			if padded.x == 0 && padded.y == 0 && padded.w >= w && padded.h >= h {
				return full, boundCanvasRatio(w, h, base, fullRatio), false
			}
			return padded, boundCanvasRatio(padded.w, padded.h, base, fullRatio), true
		}
	}
	return full, boundCanvasRatio(w, h, base, fullRatio), false
}

// padVisibleTile expands visible rect vis within [0, w]×[0, h] by up to half its extent on each side so small scrolls and pans do not repaint every frame, keeping the padded bitmap well within maxZoomPixels and maxZoomSide.
func padVisibleTile(vis canvasRect, w, h, ratio float64) canvasRect {
	x0, y0, x1, y1 := vis.x, vis.y, vis.x+vis.w, vis.y+vis.h
	const overscanPixels = maxZoomPixels / 2
	// Expand only if the visible area itself leaves headroom for overscan.
	vw, vh := x1-x0, y1-y0
	if vw*vh*ratio*ratio < overscanPixels {
		padX := math.Floor(vw / 2)
		padY := math.Floor(vh / 2)
		nx0 := max(x0-padX, 0)
		ny0 := max(y0-padY, 0)
		nx1 := min(x1+padX, w)
		ny1 := min(y1+padY, h)
		nw, nh := nx1-nx0, ny1-ny0
		if nw*nh*ratio*ratio <= overscanPixels && nw*ratio <= maxZoomSide && nh*ratio <= maxZoomSide {
			x0, y0, x1, y1 = nx0, ny0, nx1, ny1
		}
	}
	return canvasRect{x: x0, y: y0, w: x1 - x0, h: y1 - y0}
}

// visibleCanvasRect returns the visible sub-rectangle of canvas v (whose drawing space is w×h points) in v's drawing points, clipped by the viewport, host and every scroll or clipping ancestor.
func (s *surface) visibleCanvasRect(v *view, w, h float64) (canvasRect, bool) {
	if !v.el.Get("isConnected").Bool() || w <= 0 || h <= 0 {
		return canvasRect{}, false
	}
	s.dom.flush()
	box := v.el.Call("getBoundingClientRect")
	bw, bh := box.Get("width").Float(), box.Get("height").Float()
	totalW, totalH := float64(v.size.Width), float64(v.size.Height)
	if bw <= 0 || bh <= 0 || totalW <= 0 || totalH <= 0 {
		return canvasRect{}, false
	}
	sx, sy := bw/totalW, bh/totalH
	cL := box.Get("left").Float() + float64(v.border.Left)*sx
	cT := box.Get("top").Float() + float64(v.border.Top)*sy
	cR := cL + w*sx
	cB := cT + h*sy
	clipL, clipT, clipR, clipB := cL, cT, cR, cB
	win := js.Global()
	if iw, ih := win.Get("innerWidth").Float(), win.Get("innerHeight").Float(); iw > 0 && ih > 0 {
		clipL = max(clipL, 0)
		clipT = max(clipT, 0)
		clipR = min(clipR, iw)
		clipB = min(clipB, ih)
	}
	if s.host.Truthy() {
		hr := s.host.Call("getBoundingClientRect")
		if hr.Get("width").Float() > 0 && hr.Get("height").Float() > 0 {
			clipL = max(clipL, hr.Get("left").Float())
			clipT = max(clipT, hr.Get("top").Float())
			clipR = min(clipR, hr.Get("right").Float())
			clipB = min(clipB, hr.Get("bottom").Float())
		}
	}
	for el := v.el.Get("parentElement"); el.Truthy() && !el.Equal(s.host); el = el.Get("parentElement") {
		av := s.viewOf(el)
		if av == nil || (av.sc == nil && !av.style.Clip) {
			continue
		}
		r := el.Call("getBoundingClientRect")
		if r.Get("width").Float() > 0 && r.Get("height").Float() > 0 {
			clipL = max(clipL, r.Get("left").Float())
			clipT = max(clipT, r.Get("top").Float())
			clipR = min(clipR, r.Get("right").Float())
			clipB = min(clipB, r.Get("bottom").Float())
		}
	}
	if clipR <= clipL || clipB <= clipT {
		return canvasRect{}, true
	}
	lx0 := max(math.Floor((clipL-cL)/sx), 0)
	ly0 := max(math.Floor((clipT-cT)/sy), 0)
	lx1 := min(math.Ceil((clipR-cL)/sx), w)
	ly1 := min(math.Ceil((clipB-cT)/sy), h)
	if lx1 <= lx0 || ly1 <= ly0 {
		return canvasRect{}, true
	}
	return canvasRect{x: lx0, y: ly0, w: lx1 - lx0, h: ly1 - ly0}, true
}

// canvasScale is the pixels per point a canvas view draws at: the screen's pixel ratio times enclosing scroll zooms and style transforms, bounded by maxRasterZoom and the bitmap budget.
func (s *surface) canvasScale(v *view) float64 {
	w, h := v.canvasSize()
	_, ratio, _ := s.canvasPlan(v, w, h)
	return ratio
}

// canvasZoom is the product of the zooms of the scroll views and scaled style transforms v is in, nested ones included, and whether any of them scales.
func (s *surface) canvasZoom(v *view) (zoom float64, zoomed bool) {
	zoom = 1
	// Only zoomed scroll contents and scaled views carry the attribute, so this visits them alone rather than every ancestor.
	const attr = "[" + zoomAttr + "]"
	for el := v.el.Call("closest", attr); el.Truthy(); {
		if z, err := strconv.ParseFloat(el.Call("getAttribute", zoomAttr).String(), 64); err == nil && z > 0 {
			zoom *= z
			zoomed = true
		}
		parent := el.Get("parentElement")
		if !parent.Truthy() {
			break
		}
		el = parent.Call("closest", attr)
	}
	return zoom, zoomed
}

// canvasAxisAligned reports whether no ancestor (including v) rotates, skews or tilts out of the plane.
func (s *surface) canvasAxisAligned(v *view) bool {
	return !v.el.Call("closest", "["+skewAttr+"]").Truthy()
}

// followTransform updates v's transform zoom and skew attributes from its current style and animated values, replaying canvases inside when its scale changes. Animated and CSS-transitioned transforms debounce replay until the motion rests; static style changes replay at once.
func (s *surface) followTransform(v *view, animated bool) {
	vs := v.style
	if v.animated.Set != 0 {
		vs = v.animated.Apply(v.style)
	}
	scale := float64(vs.Transform.RasterScale())
	if scale <= 0 {
		scale = 1
	}
	skewed := !vs.Transform.AxisAligned()
	oldZoom := v.transformZoom
	if oldZoom == 0 {
		oldZoom = 1
	}
	oldSkewed := v.transformSkewed
	if scale == oldZoom && skewed == oldSkewed {
		return
	}
	v.transformZoom = scale
	v.transformSkewed = skewed
	attr := ""
	if scale != 1 {
		attr = strconv.FormatFloat(scale, 'g', -1, 64)
	}
	setAttr(v.el, zoomAttr, attr)
	skewVal := ""
	if skewed {
		skewVal = "1"
	}
	setAttr(v.el, skewAttr, skewVal)
	if !v.el.Get("isConnected").Bool() {
		return
	}
	s.smoothCanvases(v.el)
	tr := v.style.Transition
	transitioning := !animated && tr.Duration > 0 && (tr.Properties == 0 || tr.Properties&driver.TransitionTransform != 0)
	if animated || transitioning {
		wait := zoomRasterWait
		if transitioning {
			if ms := int((tr.Duration + tr.Delay).Milliseconds()) + 20; ms > wait {
				wait = ms
			}
		}
		s.rasterAfterTransform(v, wait)
		return
	}
	v.stopTransformRaster()
	s.refreshCanvasScale(v.el)
}

// rasterAfterTransform keeps CSS scaling cheap during a transform animation or transition, then replays canvases inside v once it rests.
func (s *surface) rasterAfterTransform(v *view, waitMs int) {
	if v.rasterEnd.IsUndefined() {
		v.rasterEnd = js.FuncOf(func(js.Value, []js.Value) any {
			v.rasterTimer = js.Undefined()
			if s.view(v.id) == v {
				s.refreshCanvasScale(v.el)
			}
			return nil
		})
	}
	if v.rasterTimer.Truthy() {
		js.Global().Call("clearTimeout", v.rasterTimer)
	}
	v.rasterTimer = js.Global().Call("setTimeout", v.rasterEnd, waitMs)
}

// stopTransformRaster cancels a waiting transform canvas replay and releases its callback.
func (v *view) stopTransformRaster() {
	if v.rasterTimer.Truthy() {
		js.Global().Call("clearTimeout", v.rasterTimer)
		v.rasterTimer = js.Undefined()
	}
	if !v.rasterEnd.IsUndefined() {
		v.rasterEnd.Release()
		v.rasterEnd = js.Func{}
	}
}

// checkSlicedCanvases repaints any visible-slice canvas whose visible area has moved outside the sub-rectangle currently painted in its <canvas>.
func (s *surface) checkSlicedCanvases() {
	for v := range s.slicedCanvases {
		if s.view(v.id) != v || v.drawing == nil {
			delete(s.slicedCanvases, v)
			continue
		}
		w, h := v.canvasSize()
		vis, ok := s.visibleCanvasRect(v, w, h)
		if !ok {
			continue
		}
		if !v.canvasTile.contains(vis) {
			s.paint(v)
		}
	}
}

// refreshCanvasScale replays the drawings under root whose raster scale or visible slice changed. DOM placement is authoritative, including kept content moved to a new scroll view and sticky wrappers around list cells.
func (s *surface) refreshCanvasScale(root js.Value) {
	canvases := root.Call("querySelectorAll", "canvas")
	for i := range canvases.Length() {
		v := s.viewOf(canvases.Index(i).Get("parentElement"))
		if v == nil || v.kind != driver.KindCanvas || v.drawing == nil {
			continue
		}
		w, h := v.canvasSize()
		tile, ratio, sliced := s.canvasPlan(v, w, h)
		if v.canvasRatio != ratio || v.canvasSliced != sliced || (sliced && !v.canvasTile.contains(tile)) {
			s.paint(v)
		} else {
			// A zoom within the raster bounds' cap keeps the bitmap but can bring it back to the screen's size.
			s.setCanvasExact(v)
		}
	}
}

// canvasImage is an image canvases draw, cached for the surface within its
// count and byte bounds.
type canvasImage struct {
	img         js.Value
	url         string
	object      bool
	density     float64
	loaded      bool
	failed      bool
	onload      js.Func
	onerror     js.Func
	waiting     map[*view]bool
	texture     canvasmesh.Texture
	readTexture bool
}

// scratchCanvas returns a new <canvas> of width by height pixels.
func (s *surface) scratchCanvas(width, height int) js.Value {
	c := s.doc.Call("createElement", "canvas")
	c.Set("width", width)
	c.Set("height", height)
	return c
}

func (im *canvasImage) releaseListeners() {
	if !im.onload.Value.IsUndefined() {
		im.img.Call("removeEventListener", "load", im.onload)
		im.onload.Release()
		im.onload = js.Func{}
	}
	if !im.onerror.Value.IsUndefined() {
		im.img.Call("removeEventListener", "error", im.onerror)
		im.onerror.Release()
		im.onerror = js.Func{}
	}
}

func (im *canvasImage) release() {
	if im == nil {
		return
	}
	im.releaseListeners()
	if im.img.Truthy() {
		im.img.Call("removeAttribute", "src")
	}
	if im.object && im.url != "" {
		js.Global().Get("URL").Call("revokeObjectURL", im.url)
		im.url, im.object = "", false
	}
	im.waiting = nil
	im.texture = canvasmesh.Texture{}
}

func (s *surface) keepCanvasImage(key string) {
	s.canvasImages.keep(key)
}

func (s *surface) releaseCanvasImage(key string) {
	s.canvasImages.release(key)
	for _, v := range s.views {
		if v != nil && v.gl != nil {
			v.gl.textures.trim()
		}
	}
}

func (s *surface) setImageCacheLimit(maxBytes int64) {
	s.canvasImages.setLimit(maxBytes)
	for _, v := range s.views {
		if v != nil && v.gl != nil {
			v.gl.textures.setLimit(maxBytes)
		}
	}
}

// loadImage returns src's image, loading it the first time, and has v
// draw again once it loads: a canvas plays again, a shader draws.
func (s *surface) loadImage(src driver.ImageSource, v *view) *canvasImage {
	return s.loadCanvasImage(src, v, false)
}

// loadCanvasImage is loadImage for a canvas draw; readable asks for an image whose pixels a tint can read.
func (s *surface) loadCanvasImage(src driver.ImageSource, v *view, readable bool) *canvasImage {
	if src.Offscreen != nil {
		canvas, density, ok := s.offscreenCanvas(src.Offscreen, v)
		return &canvasImage{img: canvas, density: density, loaded: ok}
	}
	if src.Pixels != nil {
		return s.canvasFrame(src)
	}
	if s.canvasImages.onEvict == nil {
		s.canvasImages.onEvict = func(im *canvasImage) { im.release() }
	}
	key := src.Key()
	// A cross-origin image can draw without CORS. Tinting needs access to its pixels, so the anonymous request has a separate cache entry.
	cors := readable && len(src.Data) == 0 && (strings.HasPrefix(src.URI, "https:") ||
		strings.HasPrefix(src.URI, "http:") || strings.HasPrefix(src.URI, "//"))
	if cors {
		key = "cors:" + key
	}
	im, ok := s.canvasImages.get(key)
	if !ok {
		url, object := imageURL(src)
		im = &canvasImage{
			img:     js.Global().Get("Image").New(),
			url:     url,
			object:  object,
			density: float64(src.Density()),
			waiting: map[*view]bool{},
		}
		encodedBytes := int64(len(src.Data))
		s.canvasImages.addPending(key, im, encodedBytes)
		finish := func(loaded bool) {
			im.releaseListeners()
			cur, ok := s.canvasImages.peek(key)
			if !ok || cur != im {
				return
			}
			im.loaded = loaded
			im.failed = !loaded
			waiting := im.waiting
			im.waiting = nil
			if loaded {
				for w := range waiting {
					switch {
					case s.view(w.id) != w:
					case w.kind == driver.KindShader:
						s.shadeLater(w)
					default:
						s.paint(w)
					}
				}
			}
			bytes := encodedBytes
			if loaded && bytes == 0 {
				nw := int64(im.img.Get("naturalWidth").Float())
				nh := int64(im.img.Get("naturalHeight").Float())
				if nw > 0 && nh > 0 {
					bytes = nw * nh * 4
				}
			}
			s.canvasImages.complete(key, bytes)
		}
		im.onload = js.FuncOf(func(js.Value, []js.Value) any { finish(true); return nil })
		im.onerror = js.FuncOf(func(js.Value, []js.Value) any { finish(false); return nil })
		im.img.Call("addEventListener", "load", im.onload)
		im.img.Call("addEventListener", "error", im.onerror)
		if cors {
			im.img.Set("crossOrigin", "anonymous")
		}
		im.img.Set("src", url)
	}
	if !im.loaded && !im.failed && im.waiting != nil {
		im.waiting[v] = true
	}
	return im
}

// scratchMeshCanvas returns the canvas meshes raster into and read textures through, made once for the surface.
func (s *surface) scratchMeshCanvas() js.Value {
	if !s.meshCanvas.Truthy() {
		s.meshCanvas = s.doc.Call("createElement", "canvas")
	}
	return s.meshCanvas
}

// meshTexture returns the pixels a mesh samples from src, and false while src loads. Pixels are used as they are; an encoded image's are read once it loads, and an offscreen drawing's once per version.
func (s *surface) meshTexture(src driver.ImageSource, v *view) (canvasmesh.Texture, bool) {
	switch {
	case src.Pixels != nil:
		return canvasmesh.RGBA(src.Pixels, src.Size), true
	case src.Offscreen != nil:
		off := src.Offscreen
		canvas, density, ok := s.offscreenCanvas(off, v)
		e := s.offscreen[off.ID]
		if !ok || e == nil || !e.canvas.Equal(canvas) {
			return canvasmesh.Texture{}, ok
		}
		if !e.readTexture {
			e.texture = canvas2d.ReadTexture(s.scratchMeshCanvas(), canvas, density)
			e.texture.Points = off.Size
			if src.Size.Width > 0 && src.Size.Height > 0 {
				e.texture.Points = src.Size
			}
			e.readTexture = true
		}
		return e.texture, true
	}
	im := s.loadImage(src, v)
	if im.loaded && !im.readTexture {
		im.texture = canvas2d.ReadTexture(s.scratchMeshCanvas(), im.img, im.density)
		im.readTexture = true
	}
	return im.texture, im.loaded
}

// fontCSS is the CSS font shorthand of fnt.
func fontCSS(fnt driver.Font) string {
	family := cssFamily(fnt.Family)
	style := ""
	if fnt.Italic {
		style = "italic "
	}
	if fnt.Variant&driver.FontSmallCaps != 0 {
		style += "small-caps "
	}
	return style + strconv.Itoa(int(fnt.Weight)) + " " + webstyle.Px(fnt.Size) + " " + family
}

// describeNodes gives a canvas's named parts to assistive technology: an
// element each inside the <canvas>, which browsers expose as its
// children at their bounds and never draw.
func (s *surface) describeNodes(v *view, nodes []driver.CanvasNode) {
	v.inner.Set("textContent", "")
	for i := range nodes {
		n := &nodes[i]
		el := s.doc.Call("createElement", "div")
		if role := roles[n.Role]; role != "" {
			el.Call("setAttribute", "role", role)
		}
		if n.Label != "" {
			el.Call("setAttribute", "aria-label", n.Label)
			el.Set("textContent", n.Label)
		}
		if n.TestID != "" {
			el.Call("setAttribute", "data-testid", n.TestID)
		}
		if n.Selected {
			el.Call("setAttribute", "aria-selected", "true")
		}
		el.Call("setAttribute", "data-canvas-node", "")
		el.Set("nsgoCenter", []any{float64(n.Bounds.Origin.X + n.Bounds.Size.Width/2),
			float64(n.Bounds.Origin.Y + n.Bounds.Size.Height/2)})
		setStyle(el, map[string]string{"position": "absolute", "left": webstyle.Px(n.Bounds.Origin.X),
			"top": webstyle.Px(n.Bounds.Origin.Y), "width": webstyle.Px(n.Bounds.Size.Width), "height": webstyle.Px(n.Bounds.Size.Height)})
		v.inner.Call("appendChild", el)
	}
}

// canvasNodePress presses at the center of the canvas node el is, as
// assistive technology activates one: the view under that point that
// takes presses gets the press, as a pointer there would give it.
func (s *surface) canvasNodePress(el js.Value) bool {
	if !el.Truthy() || el.Get("nsgoCenter").IsUndefined() {
		return false
	}
	center := el.Get("nsgoCenter")
	owner := el.Get("parentElement").Get("parentElement")
	box := owner.Call("getBoundingClientRect")
	x := box.Get("left").Float() + center.Index(0).Float()
	y := box.Get("top").Float() + center.Index(1).Float()
	if cv := s.viewOf(owner); cv != nil && cv.size.Width > 0 && cv.size.Height > 0 {
		sx := box.Get("width").Float() / float64(cv.size.Width)
		sy := box.Get("height").Float() / float64(cv.size.Height)
		x = box.Get("left").Float() + (float64(cv.border.Left)+center.Index(0).Float())*sx
		y = box.Get("top").Float() + (float64(cv.border.Top)+center.Index(1).Float())*sy
	}
	v := s.target(s.doc.Call("elementFromPoint", x, y))
	if v == nil {
		return true
	}
	at := v.el.Call("getBoundingClientRect")
	host := s.host.Call("getBoundingClientRect")
	ev := driver.Event{Count: 1,
		Point: driver.Point{X: driver.Points(x - at.Get("left").Float()), Y: driver.Points(y - at.Get("top").Float())},
		Page:  driver.Point{X: driver.Points(x - host.Get("left").Float()), Y: driver.Points(y - host.Get("top").Float())}}
	for _, k := range []driver.EventKind{driver.EventPressDown, driver.EventPressUp, driver.EventPress} {
		ev.Kind = k
		s.send(v, ev)
	}
	return true
}
