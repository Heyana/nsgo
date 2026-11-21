// Package codescan reads QR codes and barcodes from a camera's frames in Go, for the platforms whose system reads none (Android, Windows and Linux): a frame's luminance, its Y plane or converted from BGRA, goes in and the first code of the kinds a driver.ScanRequest asks for comes out. It reads every kind but PDF417, which gozxing has no reader for.
package codescan

import (
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/aztec"
	"github.com/makiuchi-d/gozxing/datamatrix"
	"github.com/makiuchi-d/gozxing/oned"
	"github.com/makiuchi-d/gozxing/qrcode"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// readers are gozxing's reader of each kind it reads.
var readers = map[driver.CodeKind]func() gozxing.Reader{
	driver.CodeQR:         qrcode.NewQRCodeReader,
	driver.CodeAztec:      func() gozxing.Reader { return aztec.NewAztecReader() },
	driver.CodeDataMatrix: func() gozxing.Reader { return datamatrix.NewDataMatrixReader() },
	driver.CodeEAN13:      oned.NewEAN13Reader,
	driver.CodeEAN8:       oned.NewEAN8Reader,
	driver.CodeUPCE:       oned.NewUPCEReader,
	driver.CodeCode128:    oned.NewCode128Reader,
	driver.CodeCode39:     oned.NewCode39Reader,
	driver.CodeCode93:     oned.NewCode93Reader,
	driver.CodeITF:        oned.NewITFReader,
}

// linear are the kinds read along a line across the code, which a frame turned a quarter reads across the other way.
var linear = map[driver.CodeKind]bool{
	driver.CodeEAN13: true, driver.CodeEAN8: true, driver.CodeUPCE: true,
	driver.CodeCode128: true, driver.CodeCode39: true, driver.CodeCode93: true, driver.CodeITF: true,
}

// Reads reports whether codescan reads codes of kind k.
func Reads(k driver.CodeKind) bool { return readers[k] != nil }

// HasReader reports whether codescan reads at least one kind r asks for.
func HasReader(r driver.ScanRequest) bool {
	for k := driver.CodeQR; k <= driver.CodeITF; k++ {
		if r.Wants(k) && readers[k] != nil {
			return true
		}
	}
	return false
}

// maxDecodeSide caps the longer side of a BGRA camera frame before decoding, matching Android's 960 px limit so high-resolution webcams decode quickly.
const maxDecodeSide = 960

// LuminanceBGRA converts a BGRA pixel buffer (B, G, R, A byte order) of width by height pixels and row byte stride into a luminance plane, downscaling so the longer side is at most maxSide when maxSide > 0.
func LuminanceBGRA(bgra []byte, width, height, stride, maxSide int) (lum []byte, outW, outH int) {
	if width <= 0 || height <= 0 {
		return nil, 0, 0
	}
	if stride <= 0 {
		stride = width * 4
	}
	if stride < width*4 || len(bgra) < (height-1)*stride+width*4 {
		return nil, 0, 0
	}
	outW, outH = width, height
	if maxSide > 0 {
		if long := max(width, height); long > maxSide {
			outW = max(width*maxSide/long, 1)
			outH = max(height*maxSide/long, 1)
		}
	}
	lum = make([]byte, outW*outH)
	for y := range outH {
		sy := y * height / outH
		row := bgra[sy*stride:]
		dst := lum[y*outW:]
		for x := range outW {
			sx := x * width / outW
			off := sx * 4
			b, g, r := uint32(row[off]), uint32(row[off+1]), uint32(row[off+2])
			dst[x] = byte((77*r + 150*g + 29*b) >> 8)
		}
	}
	return lum, outW, outH
}

// DecodeBGRA converts a BGRA camera frame to luminance (capped at 960 px on its longer side) and decodes the first matching code for r.
func DecodeBGRA(bgra []byte, width, height, stride int, r driver.ScanRequest) *driver.ScannedCode {
	lum, w, h := LuminanceBGRA(bgra, width, height, stride, maxDecodeSide)
	if len(lum) == 0 {
		return nil
	}
	return Decode(lum, w, h, r)
}

// Decode returns the first code of the kinds r asks for in a frame of width by height pixels whose luminance is lum, a byte a pixel, row by row; nil when it finds none.
func Decode(lum []byte, width, height int, r driver.ScanRequest) *driver.ScannedCode {
	if width <= 0 || height <= 0 || len(lum) < width*height {
		return nil
	}
	hints := map[gozxing.DecodeHintType]interface{}{gozxing.DecodeHintType_TRY_HARDER: true}
	var turned []byte
	for k := driver.CodeQR; k <= driver.CodeITF; k++ {
		if !r.Wants(k) || readers[k] == nil {
			continue
		}
		if text, ok := read(readers[k](), lum, width, height, hints); ok {
			return &driver.ScannedCode{Kind: k, Text: text}
		}
		if linear[k] {
			if turned == nil {
				turned = turn(lum, width, height)
			}
			if text, ok := read(readers[k](), turned, height, width, hints); ok {
				return &driver.ScannedCode{Kind: k, Text: text}
			}
		}
	}
	return nil
}

// read reads one code with reader from a frame of luminance lum.
func read(reader gozxing.Reader, lum []byte, width, height int, hints map[gozxing.DecodeHintType]interface{}) (string, bool) {
	source, err := gozxing.NewPlanarYUVLuminanceSource(lum, width, height, 0, 0, width, height, false)
	if err != nil {
		return "", false
	}
	bmp, err := gozxing.NewBinaryBitmap(gozxing.NewHybridBinarizer(source))
	if err != nil {
		return "", false
	}
	result, err := reader.Decode(bmp, hints)
	if err != nil {
		return "", false
	}
	return result.GetText(), true
}

// turn returns a frame of width by height pixels turned a quarter clockwise, height by width.
func turn(lum []byte, width, height int) []byte {
	out := make([]byte, width*height)
	for y := range height {
		for x := range width {
			out[x*height+height-1-y] = lum[y*width+x]
		}
	}
	return out
}
