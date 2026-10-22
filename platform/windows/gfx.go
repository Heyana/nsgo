//go:build windows

package windows

// The graphics devices of the main thread and the window's swap chain,
// as NSH's gfx.rs: one Direct3D 11 device (hardware, else WARP) and the
// Direct2D device and device context on it, and a flip-model swap chain
// the context draws into. When the device is lost (a driver update, a GPU
// reset), deviceLost drops it and bumps gfxGeneration: device resources
// (bitmaps, brushes, shadows) remember the generation they were made in
// and are made again.
//
// DirectWrite's shared factory is free-threaded; the engine goroutine
// lays text out with it.

import (
	"errors"
	"sync"
	"unsafe"
)

// gfx is the main thread's devices.
type gfx struct {
	d3d     *comObject // ID3D11Device: swap chains, and video frames later
	factory *comObject // ID2D1Factory1: geometries, stroke styles
	device  *comObject // ID2D1Device
	dc      *comObject // ID2D1DeviceContext every frame draws with
	// bake is a second context, for what is recorded apart from the
	// frame: shadows' command lists.
	bake *comObject
}

var (
	mainGfx *gfx
	// gfxGeneration is bumped whenever the devices are replaced.
	gfxGeneration uint64 = 1
)

// devices returns the main thread's devices, made on first use.
func devices() (*gfx, error) {
	if mainGfx != nil {
		return mainGfx, nil
	}
	g, err := newGfx()
	if err != nil {
		return nil, err
	}
	mainGfx = g
	return g, nil
}

// deviceLost drops the devices: the next devices makes new ones, and
// resources made so far are made again.
func deviceLost() {
	if mainGfx != nil {
		g := mainGfx
		releaseAll(&g.bake, &g.dc, &g.device, &g.factory, &g.d3d)
		mainGfx = nil
	}
	gfxGeneration++
}

func newGfx() (*gfx, error) {
	d3d, err := createD3D(d3dDriverHardware)
	if err != nil {
		if d3d, err = createD3D(d3dDriverWARP); err != nil {
			return nil, err
		}
	}
	g := &gfx{d3d: d3d}
	// Media Foundation's frames are drawn from another thread later: the
	// device's immediate context must take turns.
	if mt := d3d.query(&iidD3D11Multithread); mt != nil {
		mt.call(multithreadSetProtected, 1)
		mt.release()
	}
	const singleThreaded = 0
	if hr, _, _ := d2d1CreateFactory.Call(singleThreaded, uintptr(unsafe.Pointer(&iidD2D1Factory1)), 0,
		uintptr(unsafe.Pointer(&g.factory))); failed(hr) {
		d3d.release()
		return nil, hresultError("Direct2D could not start", hr)
	}
	dxgi := d3d.query(&iidDXGIDevice)
	if dxgi == nil {
		g.release()
		return nil, errors.New("the Direct3D device has no DXGI device")
	}
	hr := g.factory.call(d2dFactoryCreateDevice, dxgi.ptr(), uintptr(unsafe.Pointer(&g.device)))
	dxgi.release()
	if failed(hr) {
		g.release()
		return nil, hresultError("no Direct2D device", hr)
	}
	const optionsNone = 0
	if hr := g.device.call(d2dDeviceCreateDeviceContext, optionsNone, uintptr(unsafe.Pointer(&g.dc))); failed(hr) {
		g.release()
		return nil, hresultError("no Direct2D device context", hr)
	}
	if hr := g.device.call(d2dDeviceCreateDeviceContext, optionsNone, uintptr(unsafe.Pointer(&g.bake))); failed(hr) {
		g.release()
		return nil, hresultError("no Direct2D device context", hr)
	}
	return g, nil
}

func (g *gfx) release() {
	releaseAll(&g.bake, &g.dc, &g.device, &g.factory, &g.d3d)
}

// createD3D makes a Direct3D 11 device of driver with BGRA support, for
// Direct2D, and video support, for Media Foundation, which some drivers
// lack.
func createD3D(driver uintptr) (*comObject, error) {
	var device *comObject
	var hr uintptr
	for _, flags := range []uintptr{d3dBGRASupport | d3dVideoSupport, d3dBGRASupport} {
		hr, _, _ = d3d11CreateDevice.Call(0, driver, 0, flags, 0, 0, d3dSDKVersion,
			uintptr(unsafe.Pointer(&device)), 0, 0)
		if !failed(hr) && device != nil {
			return device, nil
		}
	}
	return nil, hresultError("no Direct3D device", hr)
}

// dwriteFactory is DirectWrite's shared factory, nil when DirectWrite
// cannot start.
var dwriteFactory = sync.OnceValue(func() *comObject {
	var f *comObject
	if hr, _, _ := dwriteCreateFactory.Call(dwFactoryShared, uintptr(unsafe.Pointer(&iidDWriteFactory)),
		uintptr(unsafe.Pointer(&f))); failed(hr) {
		return nil
	}
	return f
})

// targetProperties are the properties of a bitmap the context draws into
// at scale pixels per point.
func targetProperties(scale float32, premultiplied bool) bitmapProperties1 {
	alpha := uint32(d2dAlphaIgnore)
	if premultiplied {
		alpha = d2dAlphaPremultiplied
	}
	return bitmapProperties1{format: dxgiFormatBGRA, alphaMode: alpha, dpiX: 96 * scale, dpiY: 96 * scale,
		options: d2dBitmapTarget | d2dBitmapCannotDraw}
}

// swapChain is a window's flip-model swap chain and the bitmap of its back
// buffer.
type swapChain struct {
	swap          *comObject // IDXGISwapChain1
	target        *comObject // ID2D1Bitmap1, made on demand
	targetScale   float32
	width, height uint32
	generation    uint64
}

// newSwapChain makes a swap chain for hwnd of width by height pixels: two
// buffers, flipped and discarded (sequential before Windows 10).
func newSwapChain(g *gfx, hwnd uintptr, width, height uint32) (*swapChain, error) {
	factory, err := adapterFactory(g)
	if err != nil {
		return nil, err
	}
	defer factory.release()
	width, height = max(width, 1), max(height, 1)
	desc := swapChainDesc1{width: width, height: height, format: dxgiFormatBGRA, sampleCount: 1,
		bufferUsage: dxgiUsageTarget, bufferCount: 2, swapEffect: dxgiFlipDiscard, alphaMode: dxgiAlphaIgnore}
	var swap *comObject
	hr := factory.call(dxgiFactoryCreateSwapChainForHwnd, g.d3d.ptr(), hwnd, uintptr(unsafe.Pointer(&desc)), 0, 0,
		uintptr(unsafe.Pointer(&swap)))
	if failed(hr) {
		desc.swapEffect = dxgiFlipSequential
		hr = factory.call(dxgiFactoryCreateSwapChainForHwnd, g.d3d.ptr(), hwnd, uintptr(unsafe.Pointer(&desc)), 0, 0,
			uintptr(unsafe.Pointer(&swap)))
	}
	if failed(hr) {
		return nil, hresultError("no swap chain", hr)
	}
	factory.call(dxgiFactoryMakeWindowAssociation, hwnd, dxgiMWANoAltEnter)
	// One frame queued at most: input is drawn by the next vertical blank.
	if d1 := g.d3d.query(&iidDXGIDevice1); d1 != nil {
		d1.call(dxgiDeviceSetMaxFrameLatency, 1)
		d1.release()
	}
	return &swapChain{swap: swap, width: width, height: height, generation: gfxGeneration}, nil
}

// adapterFactory returns the DXGI factory that made the device's adapter,
// which the caller releases.
func adapterFactory(g *gfx) (*comObject, error) {
	dxgi := g.d3d.query(&iidDXGIDevice)
	if dxgi == nil {
		return nil, errors.New("the Direct3D device has no DXGI device")
	}
	defer dxgi.release()
	var adapter, factory *comObject
	if hr := dxgi.call(dxgiDeviceGetAdapter, uintptr(unsafe.Pointer(&adapter))); failed(hr) {
		return nil, hresultError("no DXGI adapter", hr)
	}
	hr := adapter.call(dxgiGetParent, uintptr(unsafe.Pointer(&iidDXGIFactory2)), uintptr(unsafe.Pointer(&factory)))
	adapter.release()
	if failed(hr) {
		return nil, hresultError("no DXGI 1.2 factory", hr)
	}
	return factory, nil
}

// stale reports whether s was made on devices that are gone.
func (s *swapChain) stale() bool { return s.generation != gfxGeneration }

func (s *swapChain) release() {
	releaseAll(&s.target, &s.swap)
}

// resize resizes the buffers to width by height pixels; the context must
// hold no target.
func (s *swapChain) resize(width, height uint32) error {
	width, height = max(width, 1), max(height, 1)
	if width == s.width && height == s.height {
		return nil
	}
	releaseAll(&s.target)
	const formatUnknown = 0
	if hr := s.swap.call(swapResizeBuffers, 0, uintptr(width), uintptr(height), formatUnknown, 0); failed(hr) {
		return hresultError("the swap chain could not resize", hr)
	}
	s.width, s.height = width, height
	return nil
}

// targetBitmap returns the back buffer as a bitmap the context draws into
// at scale, the window's DPI.
func (s *swapChain) targetBitmap(g *gfx, scale float32) (*comObject, error) {
	if s.target != nil && s.targetScale == scale {
		return s.target, nil
	}
	releaseAll(&s.target)
	var surface *comObject
	if hr := s.swap.call(swapGetBuffer, 0, uintptr(unsafe.Pointer(&iidDXGISurface)), uintptr(unsafe.Pointer(&surface))); failed(hr) {
		return nil, hresultError("no back buffer", hr)
	}
	defer surface.release()
	props := targetProperties(scale, false)
	if hr := g.dc.call(dcCreateBitmapFromDxgiSurface, surface.ptr(), uintptr(unsafe.Pointer(&props)),
		uintptr(unsafe.Pointer(&s.target))); failed(hr) {
		return nil, hresultError("no back buffer bitmap", hr)
	}
	s.targetScale = scale
	return s.target, nil
}

// present shows the frame drawn, at the next vertical blank.
func (s *swapChain) present() uintptr {
	return s.swap.call(swapPresent, 1, 0)
}
