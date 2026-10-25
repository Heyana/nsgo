//go:build darwin

package apple

import (
	"image"
	"image/png"
	"os"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/canvastest"
)

// play draws d into a white w by h bitmap at canvastest.Scale, top down
// as a canvas view's content layer is drawn. NSGO_CANVAS_PNG names a
// directory to write it to.
func play(t *testing.T, name string, d *driver.Drawing, w, h int) *image.RGBA {
	t.Helper()
	cg := load("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
	var (
		colorSpace func() uintptr
		bitmap     func(data unsafe.Pointer, w, h, bits, row, space uintptr, info uint32) uintptr
	)
	purego.RegisterLibFunc(&colorSpace, cg, "CGColorSpaceCreateDeviceRGB")
	purego.RegisterLibFunc(&bitmap, cg, "CGBitmapContextCreate")
	const scale = canvastest.Scale
	img := image.NewRGBA(image.Rect(0, 0, w*scale, h*scale))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	space := colorSpace()
	defer Release(space)
	const premultipliedLast = 1
	ctx := bitmap(unsafe.Pointer(&img.Pix[0]), uintptr(w*scale), uintptr(h*scale), 8, uintptr(img.Stride), space,
		premultipliedLast)
	defer Release(ctx)
	cgContextScaleCTM(ctx, scale, scale)
	DrawTopDown(ctx, float64(h), true, func() { Play(ctx, d.Ops, CanvasHost{Ratio: scale}) })
	if dir := os.Getenv("NSGO_CANVAS_PNG"); dir != "" {
		if f, err := os.Create(dir + "/apple-" + name + ".png"); err == nil {
			png.Encode(f, img)
			f.Close()
		}
	}
	return img
}

// TestCanvasSamples plays the shared samples, once their images loaded.
func TestCanvasSamples(t *testing.T) {
	logo := canvastest.Logo("../../..")
	if logo == nil {
		t.Fatal("no logo")
	}
	loaded := make(chan bool, 1)
	canvasImage(driver.ImageSource{Data: logo}, CanvasHost{Ratio: 2, Redraw: func() { loaded <- true }, Owner: t})
	deadline := time.Now().Add(5 * time.Second)
	for len(loaded) == 0 && time.Now().Before(deadline) {
		// The decode finishes on the main thread, which the test is not.
		runMain()
		time.Sleep(time.Millisecond)
	}
	if len(loaded) == 0 {
		t.Fatal("the image did not load")
	}
	for _, s := range canvastest.Samples(TextEngine{}, logo) {
		t.Run(s.Name, func(t *testing.T) {
			// The sample's image source must be the bytes loaded above.
			for i := range s.Drawing.Ops {
				if op := &s.Drawing.Ops[i]; op.Kind == driver.DrawImage {
					op.Image = &driver.ImageSource{Data: logo}
				}
			}
			for _, f := range s.Check(play(t, s.Name, s.Drawing, s.Width, s.Height)) {
				t.Error(f)
			}
		})
	}
}
