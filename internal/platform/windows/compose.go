//go:build windows

package windows

// Window composition, as NSH's compose.rs: web views between what the
// window draws below them and what it draws above them.
//
// A window without web views or a material draws its whole tree into its
// flip-model swap chain and has no composition at all. A material makes
// the composition before its first frame, so DWM's backdrop shows where
// the window's content is transparent; a window without a material makes
// it with its first web view. The DirectComposition target's root holds,
// bottom to top, the window's own content (the base), each web view's
// visual (a portal, which WebView2 draws the page into) and after each a
// layer, a composition swap chain the window's size with premultiplied
// alpha, holding what is drawn after that web view in paint order:
//
//	top     layer 1       later siblings, modals, dialogs
//	        web view 1
//	        layer 0       what is drawn between the two web views
//	        web view 0
//	bottom  base          everything drawn before the first web view
//
// The tree is still drawn in one pass (paint.go): reaching a web view
// records where it goes, and the next thing drawn first moves the context
// to the next layer. A layer nothing is drawn into is neither drawn,
// presented nor shown. A portal is clipped to its view's clipping
// ancestors (window pixels) and its own rounded corners, transformed as
// its view is and faded with its opacity; the root is arranged again only
// when the order of its visuals changes, and a frame's changes are one
// Commit.
//
// The base's and the layers' swap chains are made on the Direct3D device
// and again after it is lost; the DirectComposition device has no rendering
// device of its own, as no DirectComposition surfaces are used, so it
// survives a loss.

import (
	"errors"
	"math"
	"syscall"
	"unsafe"
)

var (
	dcomp                     = syscall.NewLazyDLL("dcomp.dll")
	dcompositionCreateDevice2 = dcomp.NewProc("DCompositionCreateDevice2")
)

var (
	iidDCompDesktopDevice = parseGUID("5f4633fe-1e08-4cb8-8c75-ce24333f5602")
	iidDCompVisual3       = parseGUID("2775f462-b6c1-4015-b0be-b3e7d6a4976d")
)

// Vtable indexes, from dcomp.h (each overloaded setter's float form is
// the second of its pair).
const (
	// IDCompositionDevice2, then IDCompositionDesktopDevice
	dcompCommit              = 3
	dcompCreateVisual        = 6
	dcompCreateRectangleClip = 22
	dcompCreateTargetForHwnd = 24

	// IDCompositionTarget
	dcompTargetSetRoot = 3

	// IDCompositionVisual, then IDCompositionVisual3
	visualSetTransformMatrix = 8
	visualSetClip            = 13
	visualSetClipRect        = 14
	visualSetContent         = 15
	visualAddVisual          = 16
	visualRemoveVisual       = 17
	visualRemoveAllVisuals   = 18
	visualSetOpacity         = 30

	// IDCompositionRectangleClip, the float setters
	clipSetLeft               = 4
	clipSetTop                = 6
	clipSetRight              = 8
	clipSetBottom             = 10
	clipSetTopLeftRadiusX     = 12
	clipSetTopLeftRadiusY     = 14
	clipSetTopRightRadiusX    = 16
	clipSetTopRightRadiusY    = 18
	clipSetBottomLeftRadiusX  = 20
	clipSetBottomLeftRadiusY  = 22
	clipSetBottomRightRadiusX = 24
	clipSetBottomRightRadiusY = 26

	// IDXGIFactory2
	dxgiFactoryCreateSwapChainForComposition = 24

	// ID2D1Geometry
	geometryGetBounds = 4
)

const (
	dxgiAlphaPremultiplied = 1
	dxgiScalingStretch     = 0
)

// placement is where a frame put a web view: what its portal is set to.
type placement struct {
	// m maps the web view's pixels (its points times the window's scale)
	// to window pixels.
	m             matrix
	width, height int32 // in pixels
	// clip is what its clipping ancestors leave visible, in window pixels.
	clip rectF
	// radius is its corners' in pixels when it clips its content, else 0.
	radius float32
	// alpha is the opacity it is drawn with, its ancestors' included.
	alpha float32
}

// bounds returns its window rectangle in pixels as if it were only
// translated: WebView2's bounds, which pop-ups and the IME are placed by.
func (p placement) bounds() rect {
	left, top := int32(math.Round(p.m.e)), int32(math.Round(p.m.f))
	return rect{left, top, left + p.width, top + p.height}
}

// composition is a window's DirectComposition tree, made with its first
// web view and gone with its last.
type composition struct {
	w      *window
	device *comObject // IDCompositionDesktopDevice
	target *comObject // IDCompositionTarget
	root   *comObject // IDCompositionVisual2
	// base holds the window's own swap chain, below web views and the
	// layers above them.
	base *comObject // IDCompositionVisual2
	// layers are those made so far: layer i holds what is drawn after
	// the i-th web view something is drawn after.
	layers []*layer
	// order is the root's visuals as last arranged, bottom to top.
	order []*comObject
	users int // the portals alive
}

// layer is what the window draws above a web view: a composition swap
// chain the window's size.
type layer struct {
	visual        *comObject // IDCompositionVisual2
	swap          *comObject // IDXGISwapChain1
	target        *comObject // ID2D1Bitmap1 of its back buffer
	scale         float32    // the target's
	width, height uint32
	generation    uint64 // the devices' the swap chain was made on
}

// compose returns w's composition, made for a web view if it has none.
func (w *window) compose() (*composition, error) {
	if w.composition != nil {
		return w.composition, nil
	}
	if err := dcompositionCreateDevice2.Find(); err != nil {
		return nil, err
	}
	c := &composition{w: w}
	if hr, _, _ := dcompositionCreateDevice2.Call(0, uintptr(unsafe.Pointer(&iidDCompDesktopDevice)),
		uintptr(unsafe.Pointer(&c.device))); failed(hr) || c.device == nil {
		return nil, hresultError("DirectComposition could not start", hr)
	}
	const behindChildren = 0
	if hr := c.device.call(dcompCreateTargetForHwnd, w.hwnd, behindChildren, uintptr(unsafe.Pointer(&c.target))); failed(hr) {
		c.release()
		return nil, hresultError("no composition target for the window", hr)
	}
	if hr := c.device.call(dcompCreateVisual, uintptr(unsafe.Pointer(&c.root))); failed(hr) {
		c.release()
		return nil, hresultError("no composition visual", hr)
	}
	if hr := c.device.call(dcompCreateVisual, uintptr(unsafe.Pointer(&c.base))); failed(hr) {
		c.release()
		return nil, hresultError("no composition visual", hr)
	}
	c.target.call(dcompTargetSetRoot, c.root.ptr())
	c.device.call(dcompCommit)
	w.composition = c
	return c, nil
}

// setBase shows swap as the window's content, below the portals.
func (c *composition) setBase(swap *comObject) {
	if c.base != nil && swap != nil {
		c.base.call(visualSetContent, swap.ptr())
	}
}

func (c *composition) release() {
	for _, l := range c.layers {
		releaseAll(&l.target, &l.swap, &l.visual)
	}
	c.layers, c.order = nil, nil
	releaseAll(&c.base, &c.root, &c.target, &c.device)
	if c.w != nil && c.w.composition == c {
		c.w.composition = nil
	}
}

// layerTarget returns the bitmap layer index is drawn into this frame,
// making, resizing or making again its swap chain as needed, at width by
// height pixels and scale.
func (c *composition) layerTarget(g *gfx, index int, width, height uint32, scale float32) (*comObject, error) {
	for len(c.layers) <= index {
		l := &layer{}
		if hr := c.device.call(dcompCreateVisual, uintptr(unsafe.Pointer(&l.visual))); failed(hr) {
			return nil, hresultError("no composition visual", hr)
		}
		c.layers = append(c.layers, l)
	}
	l := c.layers[index]
	width, height = max(width, 1), max(height, 1)
	if l.generation != gfxGeneration {
		releaseAll(&l.target, &l.swap)
	}
	if l.swap == nil {
		swap, err := compositionSwapChain(g, width, height)
		if err != nil {
			return nil, err
		}
		l.visual.call(visualSetContent, swap.ptr())
		l.swap, l.width, l.height, l.generation = swap, width, height, gfxGeneration
	}
	if l.width != width || l.height != height {
		// The context draws into another target now, and the bitmap of
		// the old buffers goes first.
		releaseAll(&l.target)
		const formatUnknown = 0
		if hr := l.swap.call(swapResizeBuffers, 0, uintptr(width), uintptr(height), formatUnknown, 0); failed(hr) {
			return nil, hresultError("the layer could not resize", hr)
		}
		l.width, l.height = width, height
	}
	if l.scale != scale {
		releaseAll(&l.target)
		l.scale = scale
	}
	if l.target != nil {
		return l.target, nil
	}
	var surface *comObject
	if hr := l.swap.call(swapGetBuffer, 0, uintptr(unsafe.Pointer(&iidDXGISurface)), uintptr(unsafe.Pointer(&surface))); failed(hr) {
		return nil, hresultError("no layer buffer", hr)
	}
	defer surface.release()
	props := targetProperties(scale, true)
	if hr := g.dc.call(dcCreateBitmapFromDxgiSurface, surface.ptr(), uintptr(unsafe.Pointer(&props)),
		uintptr(unsafe.Pointer(&l.target))); failed(hr) {
		l.target = nil
		return nil, hresultError("no layer bitmap", hr)
	}
	return l.target, nil
}

// present shows what was drawn into the first used layers, without
// waiting: the window's own swap chain waits for the vertical blank once
// for all of them.
func (c *composition) present(used int) uintptr {
	for _, l := range c.layers[:min(used, len(c.layers))] {
		if l.swap != nil {
			if hr := l.swap.call(swapPresent, 0, 0); failed(hr) {
				return hr
			}
		}
	}
	return 0
}

// arrange stacks the portals' visuals, bottom to top, with the layers
// drawn this frame, layers[i] being how many web views are below layer
// i, and commits the frame's changes, the portals' placements with them.
func (c *composition) arrange(portals []*comObject, layers []int) {
	order := make([]*comObject, 0, len(portals)+len(layers)+1)
	if c.base != nil {
		order = append(order, c.base)
	}
	next := 0
	for i, p := range portals {
		order = append(order, p)
		for next < len(layers) && layers[next] <= i+1 {
			if next < len(c.layers) {
				order = append(order, c.layers[next].visual)
			}
			next++
		}
	}
	for ; next < min(len(layers), len(c.layers)); next++ {
		order = append(order, c.layers[next].visual)
	}
	same := len(order) == len(c.order)
	for i := 0; same && i < len(order); i++ {
		same = order[i] == c.order[i]
	}
	if !same {
		c.root.call(visualRemoveAllVisuals)
		var below *comObject
		for _, v := range order {
			// Above the one added before it; with no reference, insertAbove
			// puts a visual below all its siblings.
			c.root.call(visualAddVisual, v.ptr(), 1, below.ptr())
			below = v
		}
		c.order = order
	}
	c.device.call(dcompCommit)
}

// deviceLost drops what lives on the lost device: the layers' swap
// chains.
func (c *composition) deviceLost() {
	for _, l := range c.layers {
		releaseAll(&l.target, &l.swap)
		l.visual.call(visualSetContent, 0)
	}
	if c.base != nil {
		c.base.call(visualSetContent, 0)
	}
}

// compositionSwapChain makes a swap chain for composition of width by
// height pixels, premultiplied.
func compositionSwapChain(g *gfx, width, height uint32) (*comObject, error) {
	factory, err := adapterFactory(g)
	if err != nil {
		return nil, err
	}
	defer factory.release()
	desc := swapChainDesc1{width: width, height: height, format: dxgiFormatBGRA, sampleCount: 1,
		bufferUsage: dxgiUsageTarget, bufferCount: 2, scaling: dxgiScalingStretch, swapEffect: dxgiFlipSequential,
		alphaMode: dxgiAlphaPremultiplied}
	var swap *comObject
	if hr := factory.call(dxgiFactoryCreateSwapChainForComposition, g.d3d.ptr(), uintptr(unsafe.Pointer(&desc)), 0,
		uintptr(unsafe.Pointer(&swap))); failed(hr) || swap == nil {
		return nil, hresultError("no composition swap chain", hr)
	}
	return swap, nil
}

// portal is a web view's visuals: an outer one with its ancestors' clip,
// in window pixels, and its opacity, holding an inner one with its
// transform and rounded corners, which WebView2 draws the page into.
type portal struct {
	c            *composition
	outer, inner *comObject // IDCompositionVisual2
	corners      *comObject // IDCompositionRectangleClip
	applied      *placement
}

// newPortal makes a web view's visuals in w's composition, made if need
// be.
func (w *window) newPortal() (*portal, error) {
	c, err := w.compose()
	if err != nil {
		return nil, err
	}
	p := &portal{c: c}
	if failed(c.device.call(dcompCreateVisual, uintptr(unsafe.Pointer(&p.outer)))) ||
		failed(c.device.call(dcompCreateVisual, uintptr(unsafe.Pointer(&p.inner)))) ||
		failed(c.device.call(dcompCreateRectangleClip, uintptr(unsafe.Pointer(&p.corners)))) {
		releaseAll(&p.corners, &p.inner, &p.outer)
		if c.users == 0 && !c.w.baseComposed {
			c.release()
		}
		return nil, errors.New("no composition visuals for the web view")
	}
	p.inner.call(visualSetClip, p.corners.ptr())
	p.outer.call(visualAddVisual, p.inner.ptr(), 1, 0)
	c.users++
	return p, nil
}

// place puts the visuals at pl, committed by arrange.
func (p *portal) place(pl placement) {
	if p.applied != nil && *p.applied == pl {
		return
	}
	p.applied = &pl
	t := pl.m.d2d()
	p.inner.call(visualSetTransformMatrix, uintptr(unsafe.Pointer(&t)))
	c := p.corners
	set := func(method int, v float32) { c.callF(method, 1, f32(v)) }
	set(clipSetLeft, 0)
	set(clipSetTop, 0)
	set(clipSetRight, float32(pl.width))
	set(clipSetBottom, float32(pl.height))
	for _, m := range []int{clipSetTopLeftRadiusX, clipSetTopLeftRadiusY, clipSetTopRightRadiusX, clipSetTopRightRadiusY,
		clipSetBottomLeftRadiusX, clipSetBottomLeftRadiusY, clipSetBottomRightRadiusX, clipSetBottomRightRadiusY} {
		set(m, pl.radius)
	}
	clip := pl.clip
	p.outer.call(visualSetClipRect, uintptr(unsafe.Pointer(&clip)))
	if v3 := p.outer.query(&iidDCompVisual3); v3 != nil {
		v3.callF(visualSetOpacity, 1, f32(min(max(pl.alpha, 0), 1)))
		v3.release()
	}
}

// hidden notes that the web view was not drawn: its visual is out of the
// tree until it is drawn again, as the next arrangement leaves it out.
func (p *portal) hidden() { p.applied = nil }

// release takes the visuals out of the composition, which goes with its
// last portal.
func (p *portal) release() {
	if p.c == nil {
		return
	}
	c := p.c
	p.c = nil
	if c.root != nil {
		c.root.call(visualRemoveVisual, p.outer.ptr())
		c.order = nil // arranged again from scratch
		c.device.call(dcompCommit)
	}
	releaseAll(&p.corners, &p.inner, &p.outer)
	if c.users--; c.users == 0 && !c.w.baseComposed {
		c.release()
		invalidateRect.Call(c.w.hwnd, 0, 0)
	}
}

// mapBounds returns the bounds of r mapped by m.
func mapBounds(m matrix, r rectF) rectF {
	out := rectF{float32(math.Inf(1)), float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.Inf(-1))}
	for _, c := range [4][2]float32{{r.left, r.top}, {r.right, r.top}, {r.right, r.bottom}, {r.left, r.bottom}} {
		x, y := m.apply(float64(c[0]), float64(c[1]))
		out.left, out.top = min(out.left, float32(x)), min(out.top, float32(y))
		out.right, out.bottom = max(out.right, float32(x)), max(out.bottom, float32(y))
	}
	return out
}
