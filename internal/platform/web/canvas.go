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

// paint plays a canvas view's drawing into its <canvas>, whose pixels follow the pixel ratio and enclosing scroll zooms. The drawing's origin is inside the border.
func (s *surface) paint(v *view) {
	if v.drawing == nil {
		return
	}
	ratio := s.canvasScale(v)
	v.canvasRatio = ratio
	w, h := v.canvasSize()
	// Setting the size clears the canvas and resets its context.
	v.inner.Set("width", math.Ceil(w*ratio))
	v.inner.Set("height", math.Ceil(h*ratio))
	ctx := v.inner.Call("getContext", "2d")
	ctx.Call("scale", ratio, ratio)
	p := canvas2d.NewPlayer(ctx, ratio, math.Hypot(w, h))
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
	s.setCanvasExact(v)
}

// setCanvasExact draws a canvas inside zoomed scroll contents pixel for pixel while its bitmap is exactly as large as the screen shows it. Chrome composes nested scale transforms with filtering even when they add up to one pixel per pixel, which greys a sharp edge; nearest sampling of a bitmap at that size changes nothing else. During a zoom, or past the raster bounds, the bitmap is scaled for the screen and keeps smooth sampling.
func (s *surface) setCanvasExact(v *view) {
	zoom, zoomed := s.canvasZoom(v)
	w, h := v.canvasSize()
	shown := float64(s.env.PixelRatio) * zoom
	exact := zoomed && math.Abs(v.canvasRatio-shown) < 1e-9 && w*shown == math.Ceil(w*shown) && h*shown == math.Ceil(h*shown)
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
	// maxRasterZoom bounds how far scroll zoom raises a canvas's pixels, as apple.RasterStep does.
	maxRasterZoom = 4
	// maxZoomPixels and maxZoomSide bound the bitmap scroll zoom may make: 64 MB, and within the side every browser accepts, since a larger canvas draws nothing in some browsers. A canvas larger than that at the screen's pixel ratio alone keeps that ratio.
	maxZoomPixels = 4096 * 4096
	maxZoomSide   = 16384
)

// canvasScale is the pixels per point a canvas view draws at: the screen's pixel ratio times the zoom of the scroll views it is in, nested ones included, with the zoom between 1 and maxRasterZoom and the bitmap within maxZoomPixels and maxZoomSide.
func (s *surface) canvasScale(v *view) float64 {
	zoom, _ := s.canvasZoom(v)
	ratio := float64(s.env.PixelRatio)
	if ratio <= 0 {
		ratio = 1
	}
	zoom = min(max(zoom, 1), maxRasterZoom)
	if w, h := v.canvasSize(); zoom > 1 && w > 0 && h > 0 {
		zoom = min(zoom, math.Sqrt(maxZoomPixels/(w*h*ratio*ratio)), maxZoomSide/(max(w, h)*ratio))
		zoom = max(zoom, 1)
	}
	return ratio * zoom
}

// canvasZoom is the product of the zooms of the scroll views v is in, nested ones included, and whether any of them is zoomed.
func (s *surface) canvasZoom(v *view) (zoom float64, zoomed bool) {
	zoom = 1
	// Only zoomed scroll contents carry the attribute, so this visits them alone rather than every ancestor.
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

// refreshCanvasScale replays the drawings under root whose raster scale changed. DOM placement is authoritative, including kept content moved to a new scroll view and sticky wrappers around list cells.
func (s *surface) refreshCanvasScale(root js.Value) {
	canvases := root.Call("querySelectorAll", "canvas")
	for i := range canvases.Length() {
		v := s.viewOf(canvases.Index(i).Get("parentElement"))
		if v == nil || v.kind != driver.KindCanvas || v.drawing == nil {
			continue
		}
		if v.canvasRatio != s.canvasScale(v) {
			s.paint(v)
		} else {
			// A zoom within the raster bounds' cap keeps the bitmap but can bring it back to the screen's size.
			s.setCanvasExact(v)
		}
	}
}

// canvasImage is an image canvases draw, loaded once for the surface.
type canvasImage struct {
	img         js.Value
	density     float64
	loaded      bool
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
	key := src.Key()
	// A cross-origin image can draw without CORS. Tinting needs access to
	// its pixels, so the anonymous request has a separate cache entry.
	cors := readable && len(src.Data) == 0 && (strings.HasPrefix(src.URI, "https:") ||
		strings.HasPrefix(src.URI, "http:") || strings.HasPrefix(src.URI, "//"))
	if cors {
		key = "cors:" + key
	}
	im := s.canvasImages[key]
	if im == nil {
		if s.canvasImages == nil {
			s.canvasImages = map[string]*canvasImage{}
		}
		im = &canvasImage{img: js.Global().Get("Image").New(), density: float64(src.Density()),
			waiting: map[*view]bool{}}
		s.canvasImages[key] = im
		var done js.Func
		done = js.FuncOf(func(_ js.Value, args []js.Value) any {
			im.img.Set("onload", js.Null())
			im.img.Set("onerror", js.Null())
			done.Release()
			waiting := im.waiting
			im.waiting = nil
			if args[0].Get("type").String() != "load" {
				return nil
			}
			im.loaded = true
			for v := range waiting {
				switch {
				case s.view(v.id) != v:
				case v.kind == driver.KindShader:
					s.shadeLater(v)
				default:
					s.paint(v)
				}
			}
			return nil
		})
		im.img.Set("onload", done)
		im.img.Set("onerror", done)
		if cors {
			im.img.Set("crossOrigin", "anonymous")
		}
		url, _ := imageURL(src)
		im.img.Set("src", url)
	}
	if !im.loaded && im.waiting != nil {
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
	box := el.Get("parentElement").Call("getBoundingClientRect")
	x := box.Get("left").Float() + center.Index(0).Float()
	y := box.Get("top").Float() + center.Index(1).Float()
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
