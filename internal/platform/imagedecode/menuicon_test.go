package imagedecode

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestRasterizeSVGAndTintAndEncodePNG(t *testing.T) {
	svg, err := driver.ParseSVG([]byte(`<svg viewBox="0 0 24 24"><rect x="2" y="2" width="20" height="20" fill="currentColor"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if !svg.Monochrome() {
		t.Fatal("currentColor SVG should report Monochrome")
	}
	img, err := RasterizeSVG(svg, 16, 16, driver.RGB(0x112233), BGRA)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 16 || img.Height != 16 || len(img.Frames) != 1 {
		t.Fatalf("got %dx%d frames=%d", img.Width, img.Height, len(img.Frames))
	}
	// Center pixel (8, 8) should be solid BGRA (0x33, 0x22, 0x11, 0xff).
	c := (8*16 + 8) * 4
	pix := img.Frames[0].Pix
	if pix[c] != 0x33 || pix[c+1] != 0x22 || pix[c+2] != 0x11 || pix[c+3] != 0xff {
		t.Fatalf("center BGRA = %x %x %x %x, want 33 22 11 ff", pix[c], pix[c+1], pix[c+2], pix[c+3])
	}

	TintPremultiplied(pix, BGRA, driver.RGB(0xffffff))
	if pix[c] != 0xff || pix[c+1] != 0xff || pix[c+2] != 0xff || pix[c+3] != 0xff {
		t.Fatalf("tinted center BGRA = %x %x %x %x, want ff ff ff ff", pix[c], pix[c+1], pix[c+2], pix[c+3])
	}

	encoded, err := EncodePNG(pix, 16, 16, BGRA)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if dec.Bounds().Dx() != 16 || dec.Bounds().Dy() != 16 {
		t.Fatalf("decoded PNG size = %v, want 16x16", dec.Bounds())
	}
	r, g, b, a := dec.At(8, 8).RGBA()
	if r != 0xffff || g != 0xffff || b != 0xffff || a != 0xffff {
		t.Fatalf("decoded PNG center = %x %x %x %x, want opaque white", r, g, b, a)
	}
}

func TestFitPremultipliedPreservesAspectAndCenters(t *testing.T) {
	// 32x16 solid red RGBA image downscaled into 16x16 should occupy rows 4..11 (height 8) and leave top/bottom transparent.
	src := make([]byte, 32*16*4)
	for i := 0; i < len(src); i += 4 {
		src[i], src[i+1], src[i+2], src[i+3] = 200, 10, 20, 255
	}
	dst := FitPremultiplied(src, 32, 16, 16, 16)
	if len(dst) != 16*16*4 {
		t.Fatalf("len = %d, want %d", len(dst), 16*16*4)
	}
	// Top row (0, 8) is transparent letterbox.
	top := (0*16 + 8) * 4
	if dst[top+3] != 0 {
		t.Fatalf("top letterbox alpha = %d, want 0", dst[top+3])
	}
	// Center pixel (8, 8) is solid red.
	mid := (8*16 + 8) * 4
	if dst[mid] != 200 || dst[mid+1] != 10 || dst[mid+2] != 20 || dst[mid+3] != 255 {
		t.Fatalf("center = %v, want [200 10 20 255]", dst[mid:mid+4])
	}
}
