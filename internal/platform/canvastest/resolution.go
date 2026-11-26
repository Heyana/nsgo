package canvastest

import (
	"fmt"
	"image"
	"image/color"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ResolutionSize is the side, in points, of the canvas Resolution draws.
const ResolutionSize = 8

// Resolution draws a checkerboard of 1-point squares, ResolutionSize on a
// side with black in its top left, at a ResolutionScale of 0.5 stretched
// with sampling: at Scale, each square is one pixel of the backing store
// and two of the view.
func Resolution(text driver.TextEngine, sampling driver.ImageSampling) *driver.Drawing {
	d := driver.NewDrawing(text)
	d.Resolution, d.Upscale = 0.5, sampling
	for y := range ResolutionSize {
		for x := range ResolutionSize {
			c := checkerColor(x, y)
			d.SetFillColor(driver.RGB(uint32(c.R)<<16|uint32(c.G)<<8|uint32(c.B))).FillRect(float32(x), float32(y), 1, 1)
		}
	}
	return d
}

// ResolutionPixels is how many pixels a side of Resolution's backing store
// has at Scale.
const ResolutionPixels = ResolutionSize * Scale / 2

// CheckResolution returns what img, Resolution's drawing stretched over
// ResolutionSize × Scale pixels from at, gets wrong: nearest, each square
// fills its 2 × 2 pixels with its color; smooth, most pixels inside blend
// two squares' colors.
func CheckResolution(img image.Image, at image.Point, sampling driver.ImageSampling) []string {
	const side = ResolutionSize * Scale
	var failed []string
	blended, inside := 0, 0
	for y := range side {
		for x := range side {
			got := color.RGBAModel.Convert(img.At(at.X+x, at.Y+y)).(color.RGBA)
			if sampling == driver.ImageSamplingNearest {
				if want := checkerColor(x/2, y/2); got != want && len(failed) < 8 {
					failed = append(failed, fmt.Sprintf("nearest pixel (%d, %d) = %v, want %v", x, y, got, want))
				}
				continue
			}
			if x > 0 && y > 0 && x < side-1 && y < side-1 {
				inside++
				if got.R > 16 && got.R < 239 {
					blended++
				}
			}
		}
	}
	if sampling != driver.ImageSamplingNearest && blended*2 < inside {
		failed = append(failed, fmt.Sprintf("smooth stretch blended %d of %d pixels, want most", blended, inside))
	}
	return failed
}
