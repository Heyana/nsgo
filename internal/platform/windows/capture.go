//go:build windows

package windows

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"math"
	"runtime"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	captureGetDC        = user32.NewProc("GetDC")
	captureReleaseDC    = user32.NewProc("ReleaseDC")
	capturePrintWindow  = user32.NewProc("PrintWindow")
	captureCreateDC     = gdi32.NewProc("CreateCompatibleDC")
	captureCreateBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	captureDeleteDC     = gdi32.NewProc("DeleteDC")
	captureSelect       = gdi32.NewProc("SelectObject")
	captureDIBits       = gdi32.NewProc("GetDIBits")
	captureDeleteObject = gdi32.NewProc("DeleteObject")
	captureClientRect   = user32.NewProc("GetClientRect")
)

func (w *window) capture(id driver.NodeID, count uint32) {
	w.post(func() {
		hwnd := w.hwnd
		dispatch := w.engine.Dispatch
		var client rect
		if ok, _, _ := captureClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client))); ok == 0 || client.empty() {
			dispatch(driver.CaptureResult(count, nil, errors.New("windows: capture has no client area")))
			return
		}
		clientRegion := image.Rect(0, 0, int(client.Right), int(client.Bottom))
		if id != 0 {
			view := w.view(id)
			if view == nil {
				dispatch(driver.CaptureResult(count, nil, errors.New("windows: capture view is gone")))
				return
			}
			b, visible := w.surfaceBounds(view)
			if !visible {
				dispatch(driver.CaptureResult(count, nil, errors.New("windows: capture view is outside the window")))
				return
			}
			pixels := func(p driver.Points) float64 { return float64(p) * w.scale }
			r := image.Rectangle{
				Min: image.Pt(int(math.Floor(pixels(b.Origin.X))), int(math.Floor(pixels(b.Origin.Y)))),
				Max: image.Pt(int(math.Ceil(pixels(b.Origin.X+b.Size.Width))), int(math.Ceil(pixels(b.Origin.Y+b.Size.Height)))),
			}
			clientRegion = clientRegion.Intersect(r)
		}
		if clientRegion.Empty() {
			dispatch(driver.CaptureResult(count, nil, errors.New("windows: capture has no visible pixels")))
			return
		}
		embedded := w.embedded
		windowRegion := clientRegion
		useComposited := !embedded
		if useComposited {
			var frame rect
			var origin point
			const extendedFrameBounds = 9
			hr, _, _ := dwmGetWindowAttribute.Call(hwnd, extendedFrameBounds, uintptr(unsafe.Pointer(&frame)), unsafe.Sizeof(frame))
			if ok, _, _ := clientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&origin))); failed(hr) || ok == 0 {
				useComposited = false
			} else {
				windowRegion = clientRegion.Add(image.Pt(int(origin.X-frame.Left), int(origin.Y-frame.Top)))
			}
		}
		go func() {
			var img *image.RGBA
			var err error
			region := clientRegion
			if useComposited {
				img, err = captureCompositedWindow(hwnd)
				if err == nil {
					region = windowRegion
				} else {
					// Fall back to PrintWindow (which still composites Direct2D, shaders and GPU views)
					// when Windows Graphics Capture is unavailable on the host or session.
					img, err = captureClient(hwnd)
				}
			} else {
				img, err = captureClient(hwnd)
			}
			var data []byte
			if err == nil {
				data, err = encodeCapture(img, &region)
			}
			dispatch(driver.CaptureResult(count, data, err))
		}()
	})
}

// captureClient copies this window's client area with its presented content.
func captureClient(hwnd uintptr) (*image.RGBA, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	failure := errors.New("windows: cannot copy the window's pixels")
	var client rect
	if ok, _, _ := captureClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client))); ok == 0 ||
		client.Right <= 0 || client.Bottom <= 0 {
		return nil, failure
	}
	width, height := client.Right, client.Bottom
	dc, _, _ := captureGetDC.Call(hwnd)
	if dc == 0 {
		return nil, failure
	}
	defer captureReleaseDC.Call(hwnd, dc)
	mem, _, _ := captureCreateDC.Call(dc)
	if mem == 0 {
		return nil, failure
	}
	defer captureDeleteDC.Call(mem)
	bmp, _, _ := captureCreateBitmap.Call(dc, uintptr(width), uintptr(height))
	if bmp == 0 {
		return nil, failure
	}
	defer captureDeleteObject.Call(bmp)
	old, _, _ := captureSelect.Call(mem, bmp)
	const renderClient = 1 | 2 // PW_CLIENTONLY | PW_RENDERFULLCONTENT
	ok, _, _ := capturePrintWindow.Call(hwnd, mem, renderClient)
	captureSelect.Call(mem, old)
	if ok == 0 {
		return nil, failure
	}
	img := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	type bitmapHeader struct {
		Size                         uint32
		Width, Height                int32
		Planes, BitCount             uint16
		Compression, SizeImage       uint32
		XPelsPerMeter, YPelsPerMeter int32
		ClrUsed, ClrImportant        uint32
	}
	info := bitmapHeader{Width: width, Height: -height, Planes: 1, BitCount: 32}
	info.Size = uint32(unsafe.Sizeof(info))
	if n, _, _ := captureDIBits.Call(mem, bmp, 0, uintptr(height), uintptr(unsafe.Pointer(&img.Pix[0])),
		uintptr(unsafe.Pointer(&info)), 0); n != uintptr(height) {
		return nil, failure
	}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+2], img.Pix[i+3] = img.Pix[i+2], img.Pix[i], 0xff
	}
	return img, nil
}

func encodeCapture(img *image.RGBA, region *image.Rectangle) ([]byte, error) {
	if region != nil {
		img = img.SubImage(*region).(*image.RGBA)
	}
	if img.Bounds().Empty() {
		return nil, errors.New("windows: capture has no visible pixels")
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}
