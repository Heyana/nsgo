//go:build darwin

package apple

import (
	"image"
	"image/png"
	"math"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/canvastest"
)

// bitmap returns a context drawing into a white w by h image at
// canvastest.Scale, which the caller releases. The image must stay
// alive while the context draws.
func bitmap(w, h int) (uintptr, *image.RGBA) {
	cg := load("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
	var (
		colorSpace func() uintptr
		create     func(data unsafe.Pointer, w, h, bits, row, space uintptr, info uint32) uintptr
	)
	purego.RegisterLibFunc(&colorSpace, cg, "CGColorSpaceCreateDeviceRGB")
	purego.RegisterLibFunc(&create, cg, "CGBitmapContextCreate")
	const scale = canvastest.Scale
	img := image.NewRGBA(image.Rect(0, 0, w*scale, h*scale))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	space := colorSpace()
	defer Release(space)
	const premultipliedLast = 1
	ctx := create(unsafe.Pointer(&img.Pix[0]), uintptr(w*scale), uintptr(h*scale), 8, uintptr(img.Stride), space,
		premultipliedLast)
	cgContextScaleCTM(ctx, scale, scale)
	return ctx, img
}

// play draws d into a white w by h bitmap at canvastest.Scale, top down
// as a canvas view's content layer is drawn. NSGO_CANVAS_PNG names a
// directory to write it to.
func play(t *testing.T, name string, d *driver.Drawing, w, h int) *image.RGBA {
	t.Helper()
	ctx, img := bitmap(w, h)
	defer Release(ctx)
	DrawTopDown(ctx, float64(h), true, func() { Play(ctx, d.Ops, CanvasHost{Ratio: canvastest.Scale}) })
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

// BenchmarkPlayChart plays the Stocks demo's intraday chart five hours
// into its session: an area, its line, the average and 300 volume bars
// under a pulsing point, clipped to the point as its animation's frames
// draw it again.
func BenchmarkPlayChart(b *testing.B) {
	const n, w, h = 300, 524, 340
	y := func(i int) float32 { return 120 + 60*float32(math.Sin(float64(i)/9)) }
	x := func(i int) float32 { return float32(i) * 1.6 }
	d := driver.NewDrawing(TextEngine{})
	d.SetFillColor(driver.RGBA(0x1a7f37, 0.25)).BeginPath().MoveTo(0, 250)
	for i := range n {
		d.LineTo(x(i), y(i))
	}
	d.LineTo(x(n-1), 250).ClosePath().Fill()
	for _, line := range []func(int) float32{y, func(i int) float32 { return y(i)/2 + 60 }} {
		d.BeginPath().MoveTo(x(0), line(0))
		for i := 1; i < n; i++ {
			d.LineTo(x(i), line(i))
		}
		d.SetStrokeColor(driver.RGB(0x1a7f37)).SetLineWidth(1.5).Stroke()
	}
	for i := range n {
		c := driver.RGB(0x1a7f37)
		if i%3 == 0 {
			c = driver.RGB(0xcf222e)
		}
		d.SetFillColor(c).FillRect(x(i)-0.4, 330-float32(i%40), 0.8, float32(i%40))
	}
	d.SetFillColor(driver.RGB(0x1a7f37)).BeginPath().Arc(x(n-1), y(n-1), 3, 0, 2*math.Pi, false).Fill()
	ctx, img := bitmap(w, h)
	defer Release(ctx)
	defer runtime.KeepAlive(img)
	cgContextClipToRect(ctx, Rect(float64(x(n-1))-15, float64(y(n-1))-15, 31, 31))
	for b.Loop() {
		Play(ctx, d.Ops, CanvasHost{Ratio: canvastest.Scale})
	}
	b.ReportMetric(float64(len(d.Ops)), "ops")
}
