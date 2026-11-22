//go:build windows

package windows

import "unsafe"

// bitmapPixels copies a Direct2D bitmap's premultiplied BGRA rows to Go memory.
// A CPU-readable staging bitmap keeps mapped GPU storage out of its callers.
func bitmapPixels(dc, source *comObject, width, height uint32, scale float32) []byte {
	if width == 0 || height == 0 || uint64(width)*uint64(height) > maxOffscreenPixels {
		return nil
	}
	props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied, dpiX: 96 * scale,
		dpiY: 96 * scale, options: d2dBitmapCPURead | d2dBitmapCannotDraw}
	var readable *comObject
	if failed(dc.callSize(dcCreateBitmap, width, height, 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&readable)))) {
		return nil
	}
	defer readable.release()
	dc.call(rtFlush, 0, 0)
	if failed(readable.call(bitmapCopyFromBitmap, 0, source.ptr(), 0)) {
		return nil
	}
	var mapped struct {
		pitch uint32
		bits  *byte
	}
	if failed(readable.call(bitmapMap, d2dMapRead, uintptr(unsafe.Pointer(&mapped)))) {
		return nil
	}
	defer readable.call(bitmapUnmap)
	if mapped.bits == nil || mapped.pitch < width*4 {
		return nil
	}
	rows := unsafe.Slice(mapped.bits, int(mapped.pitch)*int(height))
	pix := make([]byte, int(width)*int(height)*4)
	for y := range int(height) {
		copy(pix[y*int(width)*4:(y+1)*int(width)*4], rows[y*int(mapped.pitch):])
	}
	return pix
}
