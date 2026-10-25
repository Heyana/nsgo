//go:build windows

package windows

import (
	"image"
	"image/png"
	"os"
	"testing"
	"unsafe"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/canvastest"
)

const (
	bitmapMap        = 14
	bitmapUnmap      = 15
	d2dBitmapCPURead = 4
	d2dMapRead       = 1
)

// TestCanvasSamples plays the shared samples into a Direct2D bitmap at
// canvastest.Scale and reads its pixels back. NSGO_CANVAS_PNG names a
// directory to write them to.
func TestCanvasSamples(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	logo := canvastest.Logo("../../..")
	w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
	pic, err := decode(logo)
	if err != nil {
		t.Fatal(err)
	}
	src := driver.ImageSource{Data: logo}
	w.canvasImages = map[string]*imageSlot{src.Key(): {src: src, pic: pic}}
	for _, s := range canvastest.Samples(w, logo) {
		t.Run(s.Name, func(t *testing.T) {
			for i := range s.Drawing.Ops {
				if op := &s.Drawing.Ops[i]; op.Kind == driver.DrawImage {
					op.Image = &src
				}
			}
			pw, ph := uint32(s.Width*canvastest.Scale), uint32(s.Height*canvastest.Scale)
			props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied, dpiX: 96 * canvastest.Scale,
				dpiY: 96 * canvastest.Scale, options: d2dBitmapTarget}
			var target, readable *comObject
			if failed(g.dc.callSize(dcCreateBitmap, pw, ph, 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&target)))) {
				t.Fatal("no target bitmap")
			}
			defer target.release()
			props.options = d2dBitmapCPURead | d2dBitmapCannotDraw
			if failed(g.dc.callSize(dcCreateBitmap, pw, ph, 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&readable)))) {
				t.Fatal("no readable bitmap")
			}
			defer readable.release()
			g.dc.call(dcSetTarget, target.ptr())
			g.dc.callF(rtSetDpi, 0b11, f32(96*canvastest.Scale), f32(96*canvastest.Scale))
			g.dc.call(rtBeginDraw)
			white := colorF{1, 1, 1, 1}
			g.dc.call(rtClear, uintptr(unsafe.Pointer(&white)))
			p := newPainter(w, g)
			p.setTransform(identity)
			p.canvas(&view{drawing: s.Drawing}, float32(s.Width), float32(s.Height))
			p.release()
			if hr := g.dc.call(rtEndDraw, 0, 0); failed(hr) {
				t.Fatal(hresultError("EndDraw", hr))
			}
			g.dc.call(dcSetTarget, 0)
			readable.call(bitmapCopyFromBitmap, 0, target.ptr(), 0)
			var mapped struct {
				pitch uint32
				bits  *byte
			}
			if failed(readable.call(bitmapMap, d2dMapRead, uintptr(unsafe.Pointer(&mapped)))) {
				t.Fatal("cannot map the pixels")
			}
			img := image.NewRGBA(image.Rect(0, 0, int(pw), int(ph)))
			rows := unsafe.Slice(mapped.bits, int(mapped.pitch)*int(ph))
			for y := range int(ph) {
				for x := range int(pw) {
					b := rows[y*int(mapped.pitch)+x*4:]
					// Premultiplied BGRA over the white cleared first; a
					// cleared pixel reads white.
					img.Pix[y*img.Stride+x*4+0] = b[2] + 255 - b[3]
					img.Pix[y*img.Stride+x*4+1] = b[1] + 255 - b[3]
					img.Pix[y*img.Stride+x*4+2] = b[0] + 255 - b[3]
					img.Pix[y*img.Stride+x*4+3] = 255
				}
			}
			readable.call(bitmapUnmap)
			if dir := os.Getenv("NSGO_CANVAS_PNG"); dir != "" {
				if f, err := os.Create(dir + "/windows-" + s.Name + ".png"); err == nil {
					png.Encode(f, img)
					f.Close()
				}
			}
			for _, f := range s.Check(img) {
				t.Error(f)
			}
		})
	}
}
