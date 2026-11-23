//go:build windows

package windows

import (
	"encoding/binary"
	"fmt"
	"testing"
)

// TestClipboardImageReadsAnotherAppsDIB: an app that puts a bare DIB on the
// clipboard, as every Windows app that is not ours does, must read back as
// an image. The DIB carries no file header of its own, so it is wrapped as
// one before it is decoded.
func TestClipboardImageReadsAnotherAppsDIB(t *testing.T) {
	for _, header := range []uint32{40, 124} {
		for _, bitfields := range []bool{false, true} {
			t.Run(fmt.Sprintf("header %d bitfields %v", header, bitfields), func(t *testing.T) {
				masks := uint32(0)
				if bitfields {
					masks = 12
				}
				dib := make([]byte, header+masks+4*4)
				binary.LittleEndian.PutUint32(dib, header)
				binary.LittleEndian.PutUint32(dib[4:], 2) // width
				binary.LittleEndian.PutUint32(dib[8:], 2) // height
				binary.LittleEndian.PutUint16(dib[12:], 1)
				binary.LittleEndian.PutUint16(dib[14:], 32)
				if bitfields {
					binary.LittleEndian.PutUint32(dib[16:], 3)
					binary.LittleEndian.PutUint32(dib[40:], 0x00ff0000)
					binary.LittleEndian.PutUint32(dib[44:], 0x0000ff00)
					binary.LittleEndian.PutUint32(dib[48:], 0x000000ff)
					binary.LittleEndian.PutUint32(dib[52:], 0xff000000)
				}
				img := pngOf(bmpOfDIB(dib))
				if img == nil || len(img.PNG) == 0 || img.Size.Width != 2 || img.Size.Height != 2 {
					t.Fatalf("reads as %+v, want a 2x2 PNG", img)
				}
			})
		}
	}
}

// bitfieldsDIB is a 2x2 BI_BITFIELDS DIB whose color masks are the plain BGRA
// ones and whose alpha mask is alpha.
func bitfieldsDIB(header, alpha uint32) []byte {
	extra := uint32(0)
	if header == 40 {
		extra = 12 // the three masks sit between the header and the pixels
	}
	dib := make([]byte, header+extra+4*4)
	binary.LittleEndian.PutUint32(dib, header)
	binary.LittleEndian.PutUint32(dib[4:], 2) // width
	binary.LittleEndian.PutUint32(dib[8:], 2) // height
	binary.LittleEndian.PutUint16(dib[12:], 1)
	binary.LittleEndian.PutUint16(dib[14:], 32)
	binary.LittleEndian.PutUint32(dib[16:], 3)
	binary.LittleEndian.PutUint32(dib[40:], 0x00ff0000)
	binary.LittleEndian.PutUint32(dib[44:], 0x0000ff00)
	binary.LittleEndian.PutUint32(dib[48:], 0x000000ff)
	binary.LittleEndian.PutUint32(dib[52:], alpha)
	return dib
}

// TestClipboardImageRejectsTruncatedBitfieldsDIB: a DIB that says
// BI_BITFIELDS but ends before its three masks do has no pixels to wrap; it
// must be refused instead of sliced past its end.
func TestClipboardImageRejectsTruncatedBitfieldsDIB(t *testing.T) {
	for _, size := range []uint32{40, 48, 51} {
		dib := bitfieldsDIB(40, 0xff000000)[:size]
		if got := bmpOfDIB(dib); got != nil {
			t.Fatalf("a %d-byte BI_BITFIELDS DIB wraps as %d bytes, want nil", size, len(got))
		}
	}
}

// TestClipboardImageChecksWideHeaderAlphaMask: a V4/V5 header keeps its alpha
// mask at bytes 52-56, inside the header. A mask that is not the plain one
// says the pixels are not BGRA, and the DIB must be refused.
func TestClipboardImageChecksWideHeaderAlphaMask(t *testing.T) {
	for _, header := range []uint32{108, 124} {
		for _, alpha := range []uint32{0xff000000, 0} {
			t.Run(fmt.Sprintf("header %d alpha %#x", header, alpha), func(t *testing.T) {
				img := pngOf(bmpOfDIB(bitfieldsDIB(header, alpha)))
				if img == nil || len(img.PNG) == 0 || img.Size.Width != 2 || img.Size.Height != 2 {
					t.Fatalf("reads as %+v, want a 2x2 PNG", img)
				}
			})
		}
		for _, alpha := range []uint32{0xff0000ff, 0x000000ff} {
			t.Run(fmt.Sprintf("header %d wrong alpha %#x", header, alpha), func(t *testing.T) {
				if got := bmpOfDIB(bitfieldsDIB(header, alpha)); got != nil {
					t.Fatalf("a DIB with alpha mask %#x wraps as %d bytes, want nil", alpha, len(got))
				}
			})
		}
	}
}
