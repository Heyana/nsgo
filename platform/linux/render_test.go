//go:build linux && !android

package linux

import (
	"testing"

	"github.com/oneo-me/nsgo/core"
)

// TestCanvasReplaysIntoTheSurface plays a recorded drawing into an image
// surface and reads the pixels back.
func TestCanvasReplaysIntoTheSurface(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	surface := cairoImageSurfaceCreate(0, 40, 40)
	defer cairoSurfaceDestroy(surface)
	cr := cairoCreate(surface)
	defer cairoDestroy(cr)
	d := &core.Drawing{}
	d.SetFillColor(core.RGB(0x00ff00))
	d.FillRect(10, 10, 20, 20)
	(&window{}).playCanvas(d, cr, rect{w: 40, h: 40})
	cairoSurfaceFlush(surface)
	data, stride := cairoImageSurfaceGetData(surface), int(cairoImageSurfaceGetStride(surface))
	if got := pixelAt(data, stride, 20, 20); got[1] != 0xff || got[0] != 0 || got[2] != 0 {
		t.Fatalf("filled pixel = %v, want green", got)
	}
	if got := pixelAt(data, stride, 5, 5); got[3] != 0 {
		t.Fatalf("outside pixel = %v, want transparent", got)
	}
}

// TestImageDrawsItsPixels draws a decoded picture into an image surface.
func TestImageDrawsItsPixels(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	surface := cairoImageSurfaceCreate(0, 20, 20)
	defer cairoSurfaceDestroy(surface)
	cr := cairoCreate(surface)
	defer cairoDestroy(cr)
	pix := make([]byte, 4*4*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i+2], pix[i+3] = 0xff, 0xff // red and opaque, in premultiplied BGRA
	}
	v := &view{
		image:  core.ImageConfig{ResizeMode: core.Stretch},
		source: imageSlot{pic: &picture{width: 4, height: 4, pix: pix}},
	}
	p := &painter{w: &window{}, cr: cr}
	p.drawImage(v, 0, rect{w: 20, h: 20})
	cairoSurfaceFlush(surface)
	data, stride := cairoImageSurfaceGetData(surface), int(cairoImageSurfaceGetStride(surface))
	if got := pixelAt(data, stride, 10, 10); got[2] != 0xff || got[0] != 0 || got[1] != 0 {
		t.Fatalf("picture pixel = %v, want red", got)
	}
}
