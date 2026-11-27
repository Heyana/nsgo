//go:build windows

package windows

import (
	"errors"
	"image"
	"runtime"
	"time"
	"unsafe"
)

var (
	captureRoInitialize   = combase.NewProc("RoInitialize")
	captureRoUninitialize = combase.NewProc("RoUninitialize")
	captureWinRTDevice    = d3d11.NewProc("CreateDirect3D11DeviceFromDXGIDevice")
	iidCaptureItemInterop = parseGUID("3628e81b-3cac-4c60-b7f4-23ce0e0c3356")
	iidCaptureItem        = parseGUID("79c3f95b-31f7-4ec2-a464-632ef5d30760")
	iidCapturePoolStatics = parseGUID("589b103f-6bbc-5df5-a991-02e28b3b66d5")
	iidCaptureSession2    = parseGUID("2c39ae40-7d2e-5044-804e-8b6799d4cf9e")
	iidCaptureClosable    = parseGUID("30d5a829-7fa4-4026-83bb-d75bae4ea99e")
	iidCaptureSurface     = parseGUID("a9b3d012-3df2-4ee3-b8d1-8695f457d3c1")
	iidCaptureTexture     = parseGUID("6f15aaf2-d208-4e89-9ab4-489535d34f9c")
)

const (
	captureCreateForWindow  = 3 // IGraphicsCaptureItemInterop
	captureItemSize         = 7 // IGraphicsCaptureItem
	captureCreateFreeThread = 6 // IDirect3D11CaptureFramePoolStatics2
	captureNextFrame        = 7 // IDirect3D11CaptureFramePool
	captureCreateSession    = 10
	captureStart            = 6 // IGraphicsCaptureSession
	captureSetCursor        = 7 // IGraphicsCaptureSession2
	captureGetSurface       = 6 // IDirect3D11CaptureFrame
	captureGetContentSize   = 8
	captureClose            = 6  // IClosable
	captureGetInterface     = 3  // IDirect3DDxgiInterfaceAccess
	captureTextureDesc      = 10 // ID3D11Texture2D
	captureMap              = 14 // ID3D11DeviceContext
	captureUnmap            = 15
	captureCopyResource     = 47
)

// captureCompositedWindow takes one frame from Windows Graphics Capture. Its
// window item confines the frame to hwnd, including DirectComposition and
// the window's swap chain. The UI thread never waits for the frame or GPU.
func captureCompositedWindow(hwnd uintptr) (*image.RGBA, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const multiThreaded = 1
	if hr, _, _ := captureRoInitialize.Call(multiThreaded); failed(hr) {
		return nil, hresultError("windows: cannot initialize capture", hr)
	}
	defer captureRoUninitialize.Call()

	factory := activationFactory("Windows.Graphics.Capture.GraphicsCaptureItem", &iidCaptureItemInterop)
	if factory == nil {
		return nil, errors.New("windows: window capture is unavailable")
	}
	defer factory.release()
	var item *comObject
	if hr := factory.call(captureCreateForWindow, hwnd, uintptr(unsafe.Pointer(&iidCaptureItem)), uintptr(unsafe.Pointer(&item))); failed(hr) {
		return nil, hresultError("windows: cannot capture this window", hr)
	}
	defer item.release()
	var size point
	if hr := item.call(captureItemSize, uintptr(unsafe.Pointer(&size))); failed(hr) || size.X <= 0 || size.Y <= 0 {
		return nil, errors.New("windows: capture has no frame size")
	}

	device, err := createD3D(d3dDriverHardware)
	if err != nil {
		if device, err = createD3D(d3dDriverWARP); err != nil {
			return nil, err
		}
	}
	defer device.release()
	dxgi := device.query(&iidDXGIDevice)
	if dxgi == nil {
		return nil, errors.New("windows: capture has no DXGI device")
	}
	defer dxgi.release()
	var winrt *comObject
	if hr, _, _ := captureWinRTDevice.Call(dxgi.ptr(), uintptr(unsafe.Pointer(&winrt))); failed(hr) {
		return nil, hresultError("windows: cannot use the capture device", hr)
	}
	defer winrt.release()
	poolFactory := activationFactory("Windows.Graphics.Capture.Direct3D11CaptureFramePool", &iidCapturePoolStatics)
	if poolFactory == nil {
		return nil, errors.New("windows: capture frame pool is unavailable")
	}
	defer poolFactory.release()
	// SizeInt32 is an eight-byte value passed in one integer register.
	extent := uintptr(uint64(uint32(size.X)) | uint64(uint32(size.Y))<<32)
	var pool *comObject
	if hr := poolFactory.call(captureCreateFreeThread, winrt.ptr(), dxgiFormatBGRA, 2, extent, uintptr(unsafe.Pointer(&pool))); failed(hr) {
		return nil, hresultError("windows: cannot create the capture frame pool", hr)
	}
	defer closeCaptureObject(pool)
	var session *comObject
	if hr := pool.call(captureCreateSession, item.ptr(), uintptr(unsafe.Pointer(&session))); failed(hr) {
		return nil, hresultError("windows: cannot start a window capture session", hr)
	}
	defer closeCaptureObject(session)
	if cursor := session.query(&iidCaptureSession2); cursor != nil {
		cursor.call(captureSetCursor, 0)
		cursor.release()
	}
	if hr := session.call(captureStart); failed(hr) {
		return nil, hresultError("windows: cannot start capture", hr)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		var frame *comObject
		if hr := pool.call(captureNextFrame, uintptr(unsafe.Pointer(&frame))); failed(hr) {
			return nil, hresultError("windows: cannot read the captured frame", hr)
		}
		if frame != nil {
			defer closeCaptureObject(frame)
			return captureFramePixels(device, frame)
		}
		if time.Now().After(deadline) {
			return nil, errors.New("windows: capture produced no frame")
		}
		time.Sleep(8 * time.Millisecond)
	}
}

func closeCaptureObject(o *comObject) {
	if close := o.query(&iidCaptureClosable); close != nil {
		close.call(captureClose)
		close.release()
	}
	o.release()
}

func captureFramePixels(device, frame *comObject) (*image.RGBA, error) {
	var size point
	if hr := frame.call(captureGetContentSize, uintptr(unsafe.Pointer(&size))); failed(hr) || size.X <= 0 || size.Y <= 0 {
		return nil, errors.New("windows: the captured frame is empty")
	}
	var surface *comObject
	if hr := frame.call(captureGetSurface, uintptr(unsafe.Pointer(&surface))); failed(hr) {
		return nil, hresultError("windows: cannot get the captured surface", hr)
	}
	defer surface.release()
	access := surface.query(&iidCaptureSurface)
	if access == nil {
		return nil, errors.New("windows: the captured surface has no texture")
	}
	defer access.release()
	var texture *comObject
	if hr := access.call(captureGetInterface, uintptr(unsafe.Pointer(&iidCaptureTexture)), uintptr(unsafe.Pointer(&texture))); failed(hr) {
		return nil, hresultError("windows: cannot get the capture texture", hr)
	}
	defer texture.release()
	var desc texture2DDesc
	texture.call(captureTextureDesc, uintptr(unsafe.Pointer(&desc)))
	const staging, cpuRead = 3, 0x20000
	desc.usage, desc.bindFlags, desc.cpuAccess, desc.misc = staging, 0, cpuRead, 0
	var readable *comObject
	if hr := device.call(d3dCreateTexture2D, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&readable))); failed(hr) {
		return nil, hresultError("windows: cannot create a readable capture texture", hr)
	}
	defer readable.release()
	var context *comObject
	device.call(d3dGetImmediateContext, uintptr(unsafe.Pointer(&context)))
	defer context.release()
	context.call(captureCopyResource, readable.ptr(), texture.ptr())
	var mapped struct {
		bits                 *byte
		rowPitch, depthPitch uint32
	}
	const mapRead = 1
	if hr := context.call(captureMap, readable.ptr(), 0, mapRead, 0, uintptr(unsafe.Pointer(&mapped))); failed(hr) {
		return nil, hresultError("windows: cannot copy the captured pixels", hr)
	}
	defer context.call(captureUnmap, readable.ptr(), 0)
	width := min(int(desc.width), int(size.X))
	height := min(int(desc.height), int(size.Y))
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	src := unsafe.Slice(mapped.bits, int(mapped.rowPitch)*height)
	for y := range height {
		s, dst := src[y*int(mapped.rowPitch):], img.Pix[y*img.Stride:]
		for x := 0; x < width*4; x += 4 {
			dst[x], dst[x+1], dst[x+2], dst[x+3] = s[x+2], s[x+1], s[x], s[x+3]
		}
	}
	return img, nil
}
