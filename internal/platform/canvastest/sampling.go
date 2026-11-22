package canvastest

import (
	"image"
	"image/color"
	"strconv"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Checkerboard has one texel per square, with black in its top left.
func Checkerboard(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetRGBA(x, y, checkerColor(x, y))
		}
	}
	return img
}

func checkerColor(x, y int) color.RGBA {
	level := uint8(255 * ((x + y) % 2))
	return color.RGBA{R: level, G: level, B: level, A: 255}
}

// DrawSampling draws a 16×16 checkerboard in four squares. The left side
// uses the default and restored smooth filter. The right side uses nearest
// before and after saving a different filter.
func DrawSampling(d *driver.Drawing, src driver.ImageSource, size float32) {
	d.DrawImageIn(src, 0, 0, size, size)
	d.Save().SetImageSmoothing(false).DrawImageIn(src, size, 0, size, size)
	d.Save().SetImageSmoothing(true).Restore()
	d.DrawImageFrom(src, 0, 0, 16, 16, size, size, size, size)
	d.Restore().DrawImageIn(src, 0, size, size, size)
}

// DrawAtlasSampling draws the same four sampling squares through DrawAtlas.
func DrawAtlasSampling(d *driver.Drawing, src driver.ImageSource, size float32) {
	rect := driver.Bounds{Size: driver.Size{Width: 16, Height: 16}}
	draw := func(x, y float32) {
		d.DrawAtlas(src, []driver.Bounds{rect}, []driver.RSXform{{SCos: size / 16, TX: x, TY: y}}, nil)
	}
	draw(0, 0)
	d.Save().SetImageSmoothing(false)
	draw(size, 0)
	d.Save().SetImageSmoothing(true).Restore()
	draw(size, size)
	d.Restore()
	draw(0, size)
}

// AtlasSampling checks DrawAtlas against the same nearest texel reference.
func AtlasSampling(text driver.TextEngine) *Sample {
	s := Sampling(text)
	s.Name = "atlas-sampling"
	s.Drawing = driver.NewDrawing(text)
	DrawAtlasSampling(s.Drawing, driver.ImageSource{Pixels: Checkerboard(16, 16)}, 16*8/Scale)
	return s
}

// Sampling checks every pixel of two nearest draws at 8× and looks for
// blended pixels in the corresponding smooth draws.
func Sampling(text driver.TextEngine) *Sample {
	const size = 16 * 8 / Scale
	d := driver.NewDrawing(text)
	DrawSampling(d, driver.ImageSource{Pixels: Checkerboard(16, 16)}, size)
	s := &Sample{Name: "image-sampling", Width: 2 * size, Height: 2 * size, Drawing: d}
	const cell = size / 16
	for _, top := range []int{0, size} {
		s.Probes = append(s.Probes, area(cell, float64(top+cell), cell, cell, "smooth draw has blended pixels", func(c color.RGBA) bool {
			return c.R > 0 && c.R < 255
		}))
		for y := range 16 {
			for x := range 16 {
				want := checkerColor(x, y)
				s.Probes = append(s.Probes, Probe{
					X: float64(size + x*cell), Y: float64(top + y*cell), W: cell, H: cell, All: true,
					What: "checkerboard texel (" + strconv.Itoa(x) + ", " + strconv.Itoa(y) + ")",
					OK:   func(c color.RGBA) bool { return c == want },
				})
			}
		}
	}
	return s
}

// SamplingDifference returns the first pixel different from the source
// texel it should duplicate at the given integer scale.
func SamplingDifference(img image.Image, at image.Point, ref *image.RGBA, scale int, tolerance uint8) (image.Point, bool) {
	for y := range ref.Bounds().Dy() * scale {
		for x := range ref.Bounds().Dx() * scale {
			p := at.Add(image.Pt(x, y))
			got := color.RGBAModel.Convert(img.At(p.X, p.Y)).(color.RGBA)
			want := ref.RGBAAt(x/scale, y/scale)
			delta := func(a, b uint8) uint8 { return max(a, b) - min(a, b) }
			if delta(got.R, want.R) > tolerance || delta(got.G, want.G) > tolerance || delta(got.B, want.B) > tolerance || got.A != want.A {
				return p, true
			}
		}
	}
	return image.Point{}, false
}
