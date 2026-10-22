//go:build windows

package windows

// Drawing the window's view tree with Direct2D, as NSH's render.rs: each
// frame draws the whole tree into the swap chain's back buffer and
// presents it. Each view draws, in this order:
//
//  1. its shadow and background,
//  2. its content, clipped to its bounds: text, image, canvas,
//  3. its children, clipped to its rounded bounds when it clips,
//  4. its scroll indicator,
//  5. its border, over the children,
//
// all inside an opacity layer when it is translucent. Coordinates are
// points: the context's DPI is the window's scale times 96. Transforms are
// 3x2 world transforms.
//
// A window with web views draws the same single pass, split at each web
// view (compose.go): reaching one records where it goes (compose), and
// the next thing drawn first moves the context to the next layer above it
// (realize: the clips and layers pushed so far are popped, the target
// changes, and they are pushed again). Without a composition, realize is
// one test of a nil pointer.

import (
	"math"
	"slices"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

// matrix is an affine transform: x' = a x + c y + e, y' = b x + d y + f,
// laid out as D2D1_MATRIX_3X2_F.
type matrix struct{ a, b, c, d, e, f float64 }

var identity = matrix{a: 1, d: 1}

func translation(x, y float64) matrix { return matrix{a: 1, d: 1, e: x, f: y} }

func scaling(x, y float64) matrix { return matrix{a: x, d: y} }

func (m matrix) apply(x, y float64) (float64, float64) {
	return m.a*x + m.c*y + m.e, m.b*x + m.d*y + m.f
}

// then returns m followed by n.
func (m matrix) then(n matrix) matrix {
	return matrix{
		a: m.a*n.a + m.b*n.c, b: m.a*n.b + m.b*n.d,
		c: m.c*n.a + m.d*n.c, d: m.c*n.b + m.d*n.d,
		e: m.e*n.a + m.f*n.c + n.e, f: m.e*n.b + m.f*n.d + n.f,
	}
}

func (m matrix) invert() matrix {
	det := m.a*m.d - m.b*m.c
	if det == 0 {
		return identity
	}
	return matrix{
		a: m.d / det, b: -m.b / det, c: -m.c / det, d: m.a / det,
		e: (m.c*m.f - m.d*m.e) / det, f: (m.b*m.e - m.a*m.f) / det,
	}
}

// axisAligned reports whether m only moves and grows, keeping rectangles
// rectangles the right way round.
func (m matrix) axisAligned() bool { return m.b == 0 && m.c == 0 && m.a > 0 && m.d > 0 }

// stretch returns how much m scales lengths, on average.
func (m matrix) stretch() float64 { return math.Sqrt(math.Abs(m.a*m.d - m.b*m.c)) }

func (m matrix) d2d() matrix3x2 {
	return matrix3x2{float32(m.a), float32(m.b), float32(m.c), float32(m.d), float32(m.e), float32(m.f)}
}

// bound returns the box around r mapped by m, rounded out.
func (m matrix) bound(r rect) rect {
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, c := range [4][2]int32{{r.Left, r.Top}, {r.Right, r.Top}, {r.Left, r.Bottom}, {r.Right, r.Bottom}} {
		x, y := m.apply(float64(c[0]), float64(c[1]))
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	return rect{int32(math.Floor(x0)), int32(math.Floor(y0)), int32(math.Ceil(x1)), int32(math.Ceil(y1))}
}

// transformOf returns t for a view of size: around its center,
// translated, then rotated, then scaled, in points.
func transformOf(t core.Transform, size core.Size) matrix {
	if t == (core.Transform{}) {
		return identity
	}
	sx, sy := t.Scales()
	cx, cy := float64(size.Width)/2, float64(size.Height)/2
	sin, cos := math.Sincos(float64(t.Rotate) * math.Pi / 180)
	m := translation(-cx, -cy).
		then(scaling(float64(sx), float64(sy))).
		then(matrix{a: cos, b: sin, c: -sin, d: cos}).
		then(translation(cx+float64(t.TranslateX), cy+float64(t.TranslateY)))
	if math.Abs(m.a-1) < 1e-9 && math.Abs(m.d-1) < 1e-9 && math.Abs(m.b) < 1e-9 && math.Abs(m.c) < 1e-9 &&
		math.Abs(m.e) < 1e-9 && math.Abs(m.f) < 1e-9 {
		return identity
	}
	return m
}

func colorOf(c core.Color) colorF {
	return colorF{float32(c>>24&0xff) / 255, float32(c>>16&0xff) / 255, float32(c>>8&0xff) / 255, float32(c&0xff) / 255}
}

func visible(c core.Color) bool { return c&0xff != 0 }

// infinite bounds a layer's content without limiting it.
var infinite = rectF{-1e7, -1e7, 1e7, 1e7}

// pushed is a layer or clip the painter pushed, with what it needs to
// push it again.
type pushed struct {
	clip    bool // an axis-aligned clip, else a layer
	m       matrix
	rect    rectF      // the clip's
	opacity float32    // the layer's
	mask    *comObject // the layer's geometric mask, held
}

// painter is one frame's drawing pass.
type painter struct {
	w      *window
	g      *gfx
	dc     *comObject
	scale  float32 // pixels per point
	m      matrix  // the world transform: the drawn view's points to the target's DIPs
	brush  *comObject
	pushed []pushed
	// alpha is the opacity of what is drawn now: its views', multiplied.
	alpha float32
	// composed are the web views reached in this pass, in paint order,
	// with where they go.
	composed []composed
	// split is the pass's layers when the window has a composition.
	split *split
}

// composed is a web view a pass reached.
type composed struct {
	web *webView
	at  placement
}

// split is the layers a window's pass draws into (realize).
type split struct {
	c             *composition
	width, height uint32 // the window's, in pixels
	// layers holds, for each layer started, how many web views are below
	// it.
	layers []int
	// pending is set when a web view was reached: the next drawing
	// starts a layer.
	pending bool
	// ended is the first failure of ending a target's drawing.
	ended uintptr
}

func newPainter(w *window, g *gfx) *painter {
	p := &painter{w: w, g: g, dc: g.dc, scale: float32(w.scale), m: identity, alpha: 1}
	black := colorF{a: 1}
	g.dc.call(rtCreateSolidColorBrush, uintptr(unsafe.Pointer(&black)), 0, uintptr(unsafe.Pointer(&p.brush)))
	return p
}

func (p *painter) release() {
	for len(p.pushed) > 0 {
		p.pop()
	}
	releaseAll(&p.brush)
}

// solid returns the pass's brush in color c.
func (p *painter) solid(c core.Color) *comObject {
	col := colorOf(c)
	p.brush.call(brushSetColor, uintptr(unsafe.Pointer(&col)))
	return p.brush
}

// setTransform makes m the world transform.
func (p *painter) setTransform(m matrix) {
	p.m = m
	t := m.d2d()
	p.dc.call(rtSetTransform, uintptr(unsafe.Pointer(&t)))
}

// pushLayer draws what follows at opacity, inside mask (in the current
// coordinates) unless it is nil.
func (p *painter) pushLayer(opacity float32, mask *comObject) {
	p.pushLayerRaw(opacity, mask)
	mask.addRef()
	p.pushed = append(p.pushed, pushed{m: p.m, opacity: opacity, mask: mask})
}

func (p *painter) pushLayerRaw(opacity float32, mask *comObject) {
	params := layerParameters1{contentBounds: infinite, maskAntialias: d2dAntialiasPerPrimitive,
		maskTransform: identity.d2d(), opacity: opacity}
	if mask != nil {
		params.mask = mask.ptr()
	}
	p.dc.call(dcPushLayer, uintptr(unsafe.Pointer(&params)), 0)
}

// pushClipRect clips what follows to r, in the current coordinates:
// exactly while the transform keeps rectangles upright, else through a
// geometry.
func (p *painter) pushClipRect(r rectF) {
	if p.m.b == 0 && p.m.c == 0 {
		// Aliased, as a pixel-aligned layer's bounds clip.
		p.dc.call(rtPushAxisAlignedClip, uintptr(unsafe.Pointer(&r)), d2dAntialiasAliased)
		p.pushed = append(p.pushed, pushed{clip: true, m: p.m, rect: r})
		return
	}
	g := p.roundedGeometry(r, 0)
	p.pushLayer(1, g)
	g.release()
}

// pushClipRounded clips what follows to r with its corners rounded by
// radius.
func (p *painter) pushClipRounded(r rectF, radius float32) {
	if radius <= 0 {
		p.pushClipRect(r)
		return
	}
	g := p.roundedGeometry(r, radius)
	p.pushLayer(1, g)
	g.release()
}

// pop pops the last push.
func (p *painter) pop() {
	n := len(p.pushed)
	if n == 0 {
		return
	}
	top := p.pushed[n-1]
	p.pushed = p.pushed[:n-1]
	if top.clip {
		p.dc.call(rtPopAxisAlignedClip)
		return
	}
	p.dc.call(rtPopLayer)
	top.mask.release()
}

// flatten pops every layer and clip pushed so far, runs between, and
// pushes them again as they were: exact for clips, while a translucent
// layer splits in two.
func (p *painter) flatten(between func()) {
	for i := len(p.pushed) - 1; i >= 0; i-- {
		if p.pushed[i].clip {
			p.dc.call(rtPopAxisAlignedClip)
		} else {
			p.dc.call(rtPopLayer)
		}
	}
	between()
	current := p.m
	for _, q := range p.pushed {
		p.setTransform(q.m)
		if q.clip {
			r := q.rect
			p.dc.call(rtPushAxisAlignedClip, uintptr(unsafe.Pointer(&r)), d2dAntialiasAliased)
		} else {
			p.pushLayerRaw(q.opacity, q.mask)
		}
	}
	p.setTransform(current)
}

// clipBounds returns what the clips pushed so far leave visible, in the
// target's DIPs.
func (p *painter) clipBounds() rectF {
	r := infinite
	for _, q := range p.pushed {
		switch {
		case q.clip:
			r = r.intersect(mapBounds(q.m, q.rect))
		case q.mask != nil:
			var b rectF
			t := q.m.d2d()
			if !failed(q.mask.call(geometryGetBounds, uintptr(unsafe.Pointer(&t)), uintptr(unsafe.Pointer(&b)))) {
				r = r.intersect(b)
			}
		}
	}
	return r
}

// compose records that web view wv, width by height points at world with
// corners of radius, is drawn here: what follows goes into the next
// layer.
func (p *painter) compose(wv *webView, world matrix, width, height, radius float32) {
	s := float64(p.scale)
	// The page's pixels are the view's points times the scale: the linear
	// part stays, the translation is in pixels.
	m := world
	m.e, m.f = m.e*s, m.f*s
	if world.a == 1 && world.d == 1 && world.b == 0 && world.c == 0 {
		// Only moved: on whole pixels, so that the page stays sharp.
		m.e, m.f = math.Round(m.e), math.Round(m.f)
	}
	c := p.clipBounds()
	sc := float32(s)
	clip := rectF{max(c.left*sc, 0), max(c.top*sc, 0), min(c.right*sc, 1e7), min(c.bottom*sc, 1e7)}
	w, h := int32(math.Round(float64(width)*s)), int32(math.Round(float64(height)*s))
	if w <= 0 || h <= 0 || mapBounds(m, rectF{0, 0, float32(w), float32(h)}).intersect(clip).empty() {
		return
	}
	p.composed = append(p.composed, composed{wv, placement{m: m, width: w, height: h, clip: clip,
		radius: radius * sc, alpha: p.alpha}})
	if p.split != nil {
		p.split.pending = true
	}
}

// realize is called before something is drawn: after a web view, the pass
// goes on in the next layer, made as needed, with its clips and layers
// pushed again there.
func (p *painter) realize() {
	s := p.split
	if s == nil || !s.pending {
		return
	}
	s.pending = false
	target, err := s.c.layerTarget(p.g, len(s.layers), s.width, s.height, p.scale)
	if err != nil {
		// Drawn below the web view instead.
		return
	}
	s.layers = append(s.layers, len(p.composed))
	p.flatten(func() {
		if hr := p.dc.call(rtEndDraw, 0, 0); failed(hr) && s.ended == 0 {
			s.ended = hr
		}
		p.dc.call(dcSetTarget, target.ptr())
		p.dc.call(rtBeginDraw)
		t := identity.d2d()
		p.dc.call(rtSetTransform, uintptr(unsafe.Pointer(&t)))
		clear := colorF{}
		p.dc.call(rtClear, uintptr(unsafe.Pointer(&clear)))
	})
}

// roundedGeometry returns r with corners of radius as a geometry the
// caller releases; nil when Direct2D cannot make it, which a layer takes
// as no mask.
func (p *painter) roundedGeometry(r rectF, radius float32) *comObject {
	radius = max(min(radius, (r.right-r.left)/2, (r.bottom-r.top)/2), 0)
	rr := roundedRect{rect: r, radiusX: radius, radiusY: radius}
	var g *comObject
	if failed(p.g.factory.call(d2dFactoryCreateRoundedRectangleGeometry, uintptr(unsafe.Pointer(&rr)),
		uintptr(unsafe.Pointer(&g)))) {
		return nil
	}
	return g
}

func (p *painter) fillRect(r rectF, c core.Color) {
	if visible(c) && !r.empty() {
		p.dc.call(rtFillRectangle, uintptr(unsafe.Pointer(&r)), p.solid(c).ptr())
	}
}

// fillRounded fills r with corners of radius.
func (p *painter) fillRounded(r rectF, radius float32, c core.Color) {
	if !visible(c) || r.empty() {
		return
	}
	radius = max(min(radius, (r.right-r.left)/2, (r.bottom-r.top)/2), 0)
	if radius <= 0 {
		p.fillRect(r, c)
		return
	}
	rr := roundedRect{rect: r, radiusX: radius, radiusY: radius}
	p.dc.call(rtFillRoundedRectangle, uintptr(unsafe.Pointer(&rr)), p.solid(c).ptr())
}

func (p *painter) fillGeometry(g *comObject, c core.Color) {
	if g != nil && visible(c) {
		p.dc.call(rtFillGeometry, g.ptr(), p.solid(c).ptr(), 0)
	}
}

// paint answers WM_PAINT.
func (w *window) paint() {
	var ps paintStruct
	beginPaint.Call(w.hwnd, uintptr(unsafe.Pointer(&ps)))
	defer endPaint.Call(w.hwnd, uintptr(unsafe.Pointer(&ps)))
	w.frame()
}

// frame draws the whole tree into the swap chain and presents it.
func (w *window) frame() {
	var client rect
	getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
	if client.empty() {
		return
	}
	g, err := devices()
	if err != nil {
		return
	}
	if w.swap != nil && w.swap.stale() {
		w.swap.release()
		w.swap = nil
	}
	if w.swap == nil {
		if w.swap, err = newSwapChain(g, w.hwnd, uint32(client.Right), uint32(client.Bottom)); err != nil {
			return
		}
	}
	if err := w.swap.resize(uint32(client.Right), uint32(client.Bottom)); err != nil {
		w.lost()
		return
	}
	scale := float32(w.scale)
	target, err := w.swap.targetBitmap(g, scale)
	if err != nil {
		w.lost()
		return
	}
	dc := g.dc
	dc.call(dcSetTarget, target.ptr())
	dc.callF(rtSetDpi, 0b11, f32(96*scale), f32(96*scale))
	dc.call(rtBeginDraw)
	p := newPainter(w, g)
	// Only a window with web views has a composition: without one the
	// frame is the one swap chain.
	c := w.composition
	if c != nil {
		p.split = &split{c: c, width: uint32(client.Right), height: uint32(client.Bottom)}
	}
	p.setTransform(identity)
	white := colorF{1, 1, 1, 1}
	dc.call(rtClear, uintptr(unsafe.Pointer(&white)))
	if root := w.view(w.root); root != nil {
		p.draw(root, identity, root.frame.Origin.X, root.frame.Origin.Y)
	}
	p.drawCaptionButtons(client.Right)
	p.release()
	hr := dc.call(rtEndDraw, 0, 0)
	dc.call(dcSetTarget, 0)
	var layers []int
	if p.split != nil {
		layers = p.split.layers
		if hr == 0 {
			hr = p.split.ended
		}
	}
	if hr == d2dErrRecreateTarget {
		w.lost()
		return
	}
	if c != nil {
		if hr := c.present(len(layers)); failed(hr) {
			w.lost()
			return
		}
	}
	if hr := w.swap.present(); hr == dxgiErrDeviceRemoved || hr == dxgiErrDeviceReset {
		w.lost()
		return
	}
	w.composedPlaced(p.composed, layers)
}

// composedPlaced tells the web views the frame just shown drew where they
// go, and those it no longer drew, and stacks them with the layers drawn,
// layers[i] being how many web views are below layer i.
func (w *window) composedPlaced(drawn []composed, layers []int) {
	old := w.composed
	w.composed = w.composed[:0:0]
	for _, c := range drawn {
		w.composed = append(w.composed, c.web)
	}
	if len(old) == 0 && len(drawn) == 0 {
		return
	}
	for _, wv := range old {
		if !slices.Contains(w.composed, wv) {
			wv.composeAt(nil)
		}
	}
	visuals := make([]*comObject, 0, len(drawn))
	for _, c := range drawn {
		c.web.composeAt(&c.at)
		if c.web.portal != nil {
			visuals = append(visuals, c.web.portal.outer)
		}
	}
	if w.composition != nil {
		w.composition.arrange(visuals, layers)
	}
}

// lost drops the swap chains and the devices, and draws again on new
// ones.
func (w *window) lost() {
	if w.swap != nil {
		w.swap.release()
		w.swap = nil
	}
	if w.composition != nil {
		w.composition.deviceLost()
	}
	deviceLost()
	invalidateRect.Call(w.hwnd, 0, 0)
}

// draw paints v and its subtree; parent is the world transform of v's
// parent's coordinates, and x, y where v's origin goes in them.
func (p *painter) draw(v *view, parent matrix, x, y core.Points) {
	s := p.w.styleOf(v)
	if s.Hidden || s.Opacity <= 0 {
		return
	}
	world := transformOf(s.Transform, v.frame.Size).then(translation(float64(x), float64(y))).then(parent)
	// Drawn above a web view reached before it, if any.
	p.realize()
	p.setTransform(world)
	if s.Opacity < 1 {
		p.pushLayer(s.Opacity, nil)
	}
	alpha := p.alpha
	p.alpha *= s.Opacity
	p.drawContent(v, s, world)
	p.alpha = alpha
	if s.Opacity < 1 {
		p.pop()
	}
}

// drawContent paints v at world, the transform already set.
func (p *painter) drawContent(v *view, s *core.ViewStyle, world matrix) {
	width, height := float32(v.frame.Size.Width), float32(v.frame.Size.Height)
	radius := max(min(float32(s.CornerRadius), width/2, height/2), 0)
	bounds := rectF{0, 0, width, height}
	p.drawBackground(v, s, width, height, radius)
	clip := s.Clip || v.kind == core.KindScroll
	if clip {
		p.pushClipRounded(bounds, radius)
	} else {
		// The view's own content stays inside it.
		p.pushClipRect(bounds)
	}
	switch v.kind {
	case core.KindImage:
		b := p.w.borderEdges(s)
		p.drawImage(v, s.TintColor, rectF{b[3], b[0], width - b[1], height - b[2]})
	case core.KindCanvas:
		if v.drawing != nil {
			p.canvas(v, width, height)
		}
	case core.KindWeb:
		if wv := v.web; wv != nil && wv.composed() {
			corners := float32(0)
			if s.Clip {
				corners = radius
			}
			p.compose(wv, world, width, height, corners)
		} else if wv != nil && wv.failed {
			p.drawWebPlaceholder(width, height)
		}
	case core.KindVideo:
		if v.video != nil {
			p.drawVideo(v.video, width, height)
		}
	case core.KindInput:
		if v.field != nil {
			v.field.draw(p, world)
		}
	}
	if v.text != nil {
		v.text.draw(p, float32(v.content.Origin.X), float32(v.content.Origin.Y))
	}
	if !clip {
		p.pop()
	}
	for _, c := range v.children {
		p.draw(c, world, c.frame.Origin.X-v.offset.X, c.frame.Origin.Y-v.offset.Y)
	}
	indicator := v.kind == core.KindScroll && p.w.showsBar(v)
	if indicator || p.w.borderEdges(s) != ([4]float32{}) {
		// A scroll indicator, a border over a web view.
		p.realize()
	}
	p.setTransform(world)
	if indicator {
		p.drawIndicator(v)
	}
	if clip {
		p.pop()
	}
	p.drawBorder(s, width, height, radius)
}

// drawIndicator paints a scroll bar's thumb while the pointer is over
// the scroll view or drags the thumb.
func (p *painter) drawIndicator(v *view) {
	thumb, ok := v.bar().Thumb()
	if !ok || !p.w.showsBar(v) {
		return
	}
	bar := rectF{float32(thumb.Origin.X), float32(thumb.Origin.Y),
		float32(thumb.Origin.X + thumb.Size.Width), float32(thumb.Origin.Y + thumb.Size.Height)}
	p.fillRect(bar, core.RGB(0x8c8c8c))
}

// drawBackground paints v's shadow, under a fill only, as iOS draws a
// shadow of a layer with a background, then its background.
func (p *painter) drawBackground(v *view, s *core.ViewStyle, width, height, radius float32) {
	if width <= 0 || height <= 0 || !visible(s.Background) {
		return
	}
	if visible(s.ShadowColor) {
		p.drawShadow(v, s, width, height, radius)
	}
	p.fillRounded(rectF{0, 0, width, height}, radius, s.Background)
}

// shadowKey is what a shadow's drawing depends on.
type shadowKey struct {
	width, height, radius, sigma, scale float32
	color                               core.Color
}

// shadow is a view's shadow: its shape filled in its color, recorded,
// blurred by an effect.
type shadow struct {
	key        shadowKey
	generation uint64
	list       *comObject // ID2D1CommandList
	blur       *comObject // ID2D1Effect
	image      *comObject // the image drawn: the blur's output, or the list
}

func (sh *shadow) release() {
	if sh != nil {
		releaseAll(&sh.image, &sh.blur, &sh.list)
	}
}

// drawShadow draws v's shadow: React Native's shadowRadius is the blur's
// standard deviation, as Core Animation's shadowRadius is half CSS's blur.
func (p *painter) drawShadow(v *view, s *core.ViewStyle, width, height, radius float32) {
	key := shadowKey{width, height, radius, max(float32(s.ShadowRadius), 0), p.scale, s.ShadowColor}
	if v.shadow == nil || v.shadow.key != key || v.shadow.generation != gfxGeneration {
		v.shadow.release()
		v.shadow = p.makeShadow(key)
		if v.shadow == nil {
			return
		}
	}
	at := pointF{float32(s.ShadowOffset.X), float32(s.ShadowOffset.Y)}
	p.dc.call(dcDrawImage, v.shadow.image.ptr(), uintptr(unsafe.Pointer(&at)), 0, d2dInterpolationLinear, 0)
}

// makeShadow records key's shape on the second context and blurs it.
func (p *painter) makeShadow(key shadowKey) *shadow {
	sh := &shadow{key: key, generation: gfxGeneration}
	bake := p.g.bake
	if failed(bake.call(dcCreateCommandList, uintptr(unsafe.Pointer(&sh.list)))) {
		return nil
	}
	bake.call(dcSetTarget, sh.list.ptr())
	bake.callF(rtSetDpi, 0b11, f32(96*key.scale), f32(96*key.scale))
	bake.call(rtBeginDraw)
	t := identity.d2d()
	bake.call(rtSetTransform, uintptr(unsafe.Pointer(&t)))
	col := colorOf(key.color)
	var brush *comObject
	bake.call(rtCreateSolidColorBrush, uintptr(unsafe.Pointer(&col)), 0, uintptr(unsafe.Pointer(&brush)))
	if brush != nil {
		rr := roundedRect{rect: rectF{0, 0, key.width, key.height}, radiusX: key.radius, radiusY: key.radius}
		bake.call(rtFillRoundedRectangle, uintptr(unsafe.Pointer(&rr)), brush.ptr())
		brush.release()
	}
	hr := bake.call(rtEndDraw, 0, 0)
	bake.call(dcSetTarget, 0)
	if failed(hr) || failed(sh.list.call(commandListClose)) {
		sh.release()
		return nil
	}
	if key.sigma <= 0 {
		sh.list.addRef()
		sh.image = sh.list
		return sh
	}
	if failed(p.dc.call(dcCreateEffect, uintptr(unsafe.Pointer(&clsidGaussianBlur)), uintptr(unsafe.Pointer(&sh.blur)))) {
		sh.release()
		return nil
	}
	const standardDeviation = 0
	sigma := key.sigma
	sh.blur.call(effectSetValue, standardDeviation, d2dPropertyFloat, uintptr(unsafe.Pointer(&sigma)), 4)
	sh.blur.call(effectSetInput, 0, sh.list.ptr(), 1)
	sh.blur.call(effectGetOutput, uintptr(unsafe.Pointer(&sh.image)))
	if sh.image == nil {
		sh.release()
		return nil
	}
	return sh
}

// borderEdges returns the width of each side's drawn border in points,
// top, right, bottom, left: a side without a visible color has none, and
// one that has is at least a pixel wide.
func (w *window) borderEdges(s *core.ViewStyle) [4]float32 {
	side := func(width core.Points, c core.Color) float32 {
		if width <= 0 || !visible(c) {
			return 0
		}
		return max(float32(width), float32(1/w.scale))
	}
	if s.Borders == (core.Edges{}) {
		d := side(s.BorderWidth, s.BorderColor)
		return [4]float32{d, d, d, d}
	}
	b, c := s.Borders, s.BorderColors
	return [4]float32{side(b.Top, c.Top), side(b.Right, c.Right), side(b.Bottom, c.Bottom), side(b.Left, c.Left)}
}

// drawBorder paints a view's border inside its bounds, over its
// children, as NSH's draw_borders does with solid borders.
func (p *painter) drawBorder(s *core.ViewStyle, width, height, radius float32) {
	widths := p.w.borderEdges(s)
	if widths == ([4]float32{}) || width <= 0 || height <= 0 {
		return
	}
	colors := [4]core.Color{s.BorderColor, s.BorderColor, s.BorderColor, s.BorderColor}
	if s.Borders != (core.Edges{}) {
		c := s.BorderColors
		colors = [4]core.Color{c.Top, c.Right, c.Bottom, c.Left}
	}
	uniform := widths[0] == widths[1] && widths[1] == widths[2] && widths[2] == widths[3] &&
		colors[0] == colors[1] && colors[1] == colors[2] && colors[2] == colors[3]
	if bw := widths[0]; uniform && (radius <= 0 || radius >= bw) {
		// Stroking the middle line is exact while the inner corners stay
		// round, or square.
		half := bw / 2
		line := rectF{half, half, max(width-half, half), max(height-half, half)}
		if radius <= 0 {
			p.dc.callF(rtDrawRectangle, 0b100, uintptr(unsafe.Pointer(&line)), p.solid(colors[0]).ptr(), f32(bw), 0)
		} else {
			rr := roundedRect{rect: line, radiusX: radius - half, radiusY: radius - half}
			p.dc.callF(rtDrawRoundedRectangle, 0b100, uintptr(unsafe.Pointer(&rr)), p.solid(colors[0]).ptr(), f32(bw), 0)
		}
		return
	}
	// The ring between the border box and the padding box.
	outer := corners{{radius, radius}, {radius, radius}, {radius, radius}, {radius, radius}}
	figures := []roundedFigure{{rectF{0, 0, width, height}, outer}}
	if inner, c := paddingBox(width, height, radius, widths); !inner.empty() {
		figures = append(figures, roundedFigure{inner, c})
	}
	ring := p.path(figures)
	if ring == nil {
		return
	}
	defer ring.release()
	first, same := core.Color(0), true
	for i := range 4 {
		if widths[i] > 0 {
			if first == 0 {
				first = colors[i]
			} else if colors[i] != first {
				same = false
			}
		}
	}
	if same {
		p.fillGeometry(ring, first)
		return
	}
	// Sides that differ: the ring cut to each side's part of the box.
	for i, quad := range sideQuads(width, height, widths) {
		if widths[i] <= 0 || !visible(colors[i]) {
			continue
		}
		clip := p.polygon(quad[:])
		if clip == nil {
			continue
		}
		p.pushLayer(1, clip)
		clip.release()
		p.fillGeometry(ring, colors[i])
		p.pop()
	}
}

// corners are the horizontal and vertical radii of a box's corners,
// clockwise from the top left.
type corners [4][2]float32

type roundedFigure struct {
	r rectF
	c corners
}

// paddingBox returns the box inside borders of widths (top, right,
// bottom, left) of a width by height box with corners of radius, and its
// corners: the outer ones less the borders on each axis.
func paddingBox(width, height, radius float32, widths [4]float32) (rectF, corners) {
	top, right, bottom, left := widths[0], widths[1], widths[2], widths[3]
	r := rectF{left, top, max(width-right, left), max(height-bottom, top)}
	c := func(x, y float32) [2]float32 { return [2]float32{max(radius-x, 0), max(radius-y, 0)} }
	return r, corners{c(left, top), c(right, top), c(right, bottom), c(left, bottom)}
}

// fitCorners scales c down as CSS does where adjacent corners overlap on
// a width by height box.
func fitCorners(width, height float32, c corners) corners {
	f := float32(1)
	for _, side := range [4][2]float32{
		{c[0][0] + c[1][0], width}, {c[1][1] + c[2][1], height},
		{c[2][0] + c[3][0], width}, {c[3][1] + c[0][1], height},
	} {
		if side[0] > 0 {
			f = min(f, side[1]/side[0])
		}
	}
	if f < 1 {
		for i := range c {
			c[i][0] *= f
			c[i][1] *= f
		}
	}
	return c
}

// sideQuads returns each side's part of a width by height box with border
// widths, clockwise from the top: corners split along the lines from the
// outer corners through the inner ones, as CSS joins differing borders.
func sideQuads(width, height float32, widths [4]float32) [4][4]pointF {
	t, r, b, l := widths[0], widths[1], widths[2], widths[3]
	end := func(cx, cy, dx, dy float32) pointF {
		if dx == 0 && dy == 0 {
			dx, dy = 1, 1
			if cx != 0 {
				dx = -1
			}
			if cy != 0 {
				dy = -1
			}
		}
		k := float32(math.Inf(1))
		if dx != 0 {
			k = width / 2 / float32(math.Abs(float64(dx)))
		}
		if dy != 0 {
			k = min(k, height/2/float32(math.Abs(float64(dy))))
		}
		return pointF{cx + dx*k, cy + dy*k}
	}
	tl := end(0, 0, l, t)
	tr := end(width, 0, -r, t)
	br := end(width, height, -r, -b)
	bl := end(0, height, l, -b)
	return [4][4]pointF{
		{{0, 0}, {width, 0}, tr, tl},
		{{width, 0}, {width, height}, br, tr},
		{{width, height}, {0, height}, bl, br},
		{{0, height}, {0, 0}, tl, bl},
	}
}

// pathGeometry opens a path geometry; fill returns it, closed, or nil.
func (p *painter) pathGeometry(mode uintptr, fill func(sink *comObject)) *comObject {
	var path, sink *comObject
	if failed(p.g.factory.call(d2dFactoryCreatePathGeometry, uintptr(unsafe.Pointer(&path)))) {
		return nil
	}
	if failed(path.call(pathOpen, uintptr(unsafe.Pointer(&sink)))) {
		path.release()
		return nil
	}
	sink.call(sinkSetFillMode, mode)
	fill(sink)
	hr := sink.call(sinkClose)
	sink.release()
	if failed(hr) {
		path.release()
		return nil
	}
	return path
}

// path returns rounded rectangles as one geometry filled alternately: one
// inside another makes a ring.
func (p *painter) path(figures []roundedFigure) *comObject {
	return p.pathGeometry(d2dFillAlternate, func(sink *comObject) {
		for _, f := range figures {
			addRounded(sink, f.r, f.c)
		}
	})
}

// polygon returns a closed polygon.
func (p *painter) polygon(pts []pointF) *comObject {
	return p.pathGeometry(d2dFillAlternate, func(sink *comObject) {
		sink.callPoint(sinkBeginFigure, pts[0].x, pts[0].y, d2dFigureFilled)
		sink.call(sinkAddLines, uintptr(unsafe.Pointer(&pts[1])), uintptr(len(pts)-1))
		sink.call(sinkEndFigure, d2dFigureClosed)
	})
}

// addRounded adds r with corners c to an open sink as a closed figure.
func addRounded(sink *comObject, r rectF, c corners) {
	x, y, w, h := r.left, r.top, r.right-r.left, r.bottom-r.top
	c = fitCorners(w, h, c)
	round := func(k [2]float32) bool { return k[0] > 0 && k[1] > 0 }
	for i := range c {
		if !round(c[i]) {
			c[i] = [2]float32{}
		}
	}
	line := func(px, py float32) {
		pt := pointF{px, py}
		sink.call(sinkAddLines, uintptr(unsafe.Pointer(&pt)), 1)
	}
	arc := func(px, py float32, k [2]float32) {
		if round(k) {
			a := arcSegment{point: pointF{px, py}, width: k[0], height: k[1], sweep: d2dSweepClockwise}
			sink.call(sinkAddArc, uintptr(unsafe.Pointer(&a)))
		}
	}
	sink.callPoint(sinkBeginFigure, x+c[0][0], y, d2dFigureFilled)
	line(x+w-c[1][0], y)
	arc(x+w, y+c[1][1], c[1])
	line(x+w, y+h-c[2][1])
	arc(x+w-c[2][0], y+h, c[2])
	line(x+c[3][0], y+h)
	arc(x, y+h-c[3][1], c[3])
	line(x, y+c[0][1])
	arc(x+c[0][0], y, c[0])
	sink.call(sinkEndFigure, d2dFigureClosed)
}
