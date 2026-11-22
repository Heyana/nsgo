//go:build darwin

package formula

import (
	"fmt"
	"image"
	"math"
	"runtime"
	"slices"
	"testing"
	"unsafe"

	"github.com/caelyreth/untie/display"
	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Replay the actual formula rules through the native canvas player. At a
// column inside a rule, its minor edges must leave no antialiased rows.
func TestFormulaRuleNativePixels(t *testing.T) {
	blurredControls := 0
	for _, source := range []string{`\frac{123}{456}`, `\sqrt{x+1}`} {
		l := Default().parsed(layoutKey{source: source}, true)
		for _, fontSize := range []float32{11, 13, 17, 23.8} {
			for _, ratio := range []float32{1, 1.5, 2, 3} {
				t.Run(fmt.Sprintf("%s/%gpt/%gx", source, fontSize, ratio), func(t *testing.T) {
					for _, run := range l.runs {
						if run.path != nil {
							continue
						}
						var control *image.RGBA
						for _, snap := range []bool{false, true} {
							d := driver.NewDrawing(nil)
							d.SetFillColor(core.RGB(0x000000)).Translate(4, 4).Scale(fontSize, fontSize)
							grid := display.PixelGrid{}
							if snap {
								grid = display.PixelGrid{Scale: fontSize, PixelRatio: ratio}
							}
							run.draw(d, grid)
							img := ruleBitmap(t, d, ratio)
							if !snap {
								control = img
							}
							if snap && run.rect.Height*fontSize*ratio > 3 {
								if !slices.Equal(img.Pix, control.Pix) {
									t.Error("a thick rule changed pixels")
								}
								continue
							}
							x := int((4 + (run.rect.X+run.rect.Width/2)*fontSize) * ratio)
							black, partial := 0, 0
							for y := range img.Bounds().Dy() {
								v := img.RGBAAt(x, y).R
								if v < 2 {
									black++
								} else if v < 253 {
									partial++
								}
							}
							if snap {
								want := max(1, int(math.Round(float64(run.rect.Height*fontSize*ratio))))
								if black != want || partial != 0 {
									t.Errorf("snapped rule: %d black and %d partial rows, want %d black and no partial rows", black, partial, want)
								}
							} else if partial > 0 {
								blurredControls++
							}
						}
					}
				})
			}
		}
	}
	if blurredControls == 0 {
		t.Fatal("unsnapped controls did not reproduce blurred rule edges")
	}
	t.Logf("unsnapped controls had partial rows in %d cases; snapped rules had none", blurredControls)
}

// ruleBitmap uses the same Core Graphics player as AppKit, into an independent
// bitmap at the requested density; it does not resample through the display.
func ruleBitmap(t *testing.T, d *core.Drawing, ratio float32) *image.RGBA {
	t.Helper()
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	defer purego.Dlclose(cg)
	var spaceCreate func() uintptr
	var create func(unsafe.Pointer, uintptr, uintptr, uintptr, uintptr, uintptr, uint32) uintptr
	var scale func(uintptr, float64, float64)
	purego.RegisterLibFunc(&spaceCreate, cg, "CGColorSpaceCreateDeviceRGB")
	purego.RegisterLibFunc(&create, cg, "CGBitmapContextCreate")
	purego.RegisterLibFunc(&scale, cg, "CGContextScaleCTM")
	const size = 128
	pixels := int(size * ratio)
	img := image.NewRGBA(image.Rect(0, 0, pixels, pixels))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	space := spaceCreate()
	defer apple.Release(space)
	ctx := create(unsafe.Pointer(&img.Pix[0]), uintptr(pixels), uintptr(pixels), 8, uintptr(img.Stride), space, 1)
	if ctx == 0 {
		t.Fatal("no Core Graphics bitmap context")
	}
	defer apple.Release(ctx)
	scale(ctx, float64(ratio), float64(ratio))
	apple.DrawTopDown(ctx, size, true, func(baseUp bool) {
		apple.Play(ctx, d.Ops, apple.CanvasHost{Ratio: float64(ratio), BaseUp: baseUp})
	})
	runtime.KeepAlive(img)
	return img
}
