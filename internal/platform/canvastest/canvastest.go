// Package canvastest holds drawings that exercise a canvas player, and
// what their pixels must show, so each backend's tests check their
// player the same way: they draw each Sample on white at twice its
// points and run its probes.
package canvastest

import (
	"image"
	"image/color"
	"math"
	"os"

	"github.com/oneo-me/nsgo/internal/driver"
)

// Sample is a drawing and the pixels it must make.
type Sample struct {
	Name          string
	Width, Height int // points
	Drawing       *driver.Drawing
	Probes        []Probe
}

// Probe is a point, or a rectangle when W and H are set, in points, and
// what its pixel must be: some pixel of the rectangle, or all of them
// with All.
type Probe struct {
	X, Y float64
	What string
	OK   func(c color.RGBA) bool
	W, H float64
	All  bool
}

func pt(x, y float64, what string, ok func(color.RGBA) bool) Probe {
	return Probe{X: x, Y: y, What: what, OK: ok}
}

func area(x, y, w, h float64, what string, ok func(color.RGBA) bool) Probe {
	return Probe{X: x, Y: y, W: w, H: h, What: what, OK: ok}
}

// Scale is the pixels a point the samples are drawn at.
const Scale = 2

// Check returns the probes of s that img fails, img being s drawn at
// Scale on white.
func (s *Sample) Check(img *image.RGBA) []string {
	var failed []string
	for _, p := range s.Probes {
		c := img.RGBAAt(int(p.X*Scale), int(p.Y*Scale))
		ok := p.OK(c)
		for y := p.Y; ok == p.All && y < p.Y+p.H; y += 0.5 {
			for x := p.X; ok == p.All && x < p.X+p.W; x += 0.5 {
				if c2 := img.RGBAAt(int(x*Scale), int(y*Scale)); p.OK(c2) != p.All {
					ok, c = !p.All, c2
				}
			}
		}
		if !ok {
			failed = append(failed, p.What+": "+colorString(c))
		}
	}
	return failed
}

func colorString(c color.RGBA) string {
	const hex = "0123456789abcdef"
	b := []byte{'#'}
	for _, v := range []uint8{c.R, c.G, c.B, c.A} {
		b = append(b, hex[v>>4], hex[v&15])
	}
	return string(b)
}

func red(c color.RGBA) bool   { return c.R > 180 && c.G < 90 && c.B < 90 }
func green(c color.RGBA) bool { return c.G > 180 && c.R < 90 && c.B < 90 }
func blue(c color.RGBA) bool  { return c.B > 180 && c.R < 90 && c.G < 90 }
func white(c color.RGBA) bool { return c.R > 230 && c.G > 230 && c.B > 230 }
func dark(c color.RGBA) bool  { return c.R < 110 && c.G < 110 && c.B < 110 }
func inked(c color.RGBA) bool { return !white(c) }
func clear(c color.RGBA) bool { return c.A < 20 || white(c) }

// Samples returns the drawings, recorded with the platform's text engine
// as a canvas's are, logo being an image the last one draws.
func Samples(text driver.TextEngine, logo []byte) []*Sample {
	stops := []driver.GradientStop{{Offset: 0, Color: driver.RGB(0xff0000)}, {Offset: 1, Color: driver.RGB(0x0000ff)}}
	var out []*Sample

	d := driver.NewDrawing(text)
	d.SetFillGradient(driver.CanvasLinear(10, 0, 110, 0, stops...)).BeginPath().Rect(10, 10, 100, 40).Fill()
	d.SetFillGradient(driver.CanvasRadial(170, 30, 0, 170, 30, 30, stops...)).BeginPath().Rect(140, 0, 60, 60).Fill()
	d.SetFillGradient(driver.CanvasConic(0, 250, 30, stops...)).BeginPath().Rect(220, 0, 60, 60).Fill()
	d.Save().SetShadow(driver.RGBA(0x000000, 0.8), 10, 10, 0).SetFillColor(driver.RGB(0x00ff00)).
		BeginPath().Rect(10, 80, 40, 40).Fill().Restore()
	d.Save().SetShadow(driver.RGBA(0x000000, 1), 0, 0, 12).SetFillColor(driver.RGB(0x00ff00)).
		BeginPath().Rect(120, 80, 30, 30).Fill().Restore()
	out = append(out, &Sample{Name: "gradients-shadows", Width: 300, Height: 140, Drawing: d, Probes: []Probe{
		pt(12, 30, "linear start", red), pt(108, 30, "linear end", blue),
		pt(170, 30, "radial center", red), pt(141, 1, "radial corner", blue),
		pt(278, 31, "conic just after its start", red), pt(278, 27, "conic just before its end", func(c color.RGBA) bool { return c.B > 150 }),
		pt(52, 122, "hard shadow", dark), pt(30, 100, "shape over its shadow", green),
		pt(153, 95, "blurred shadow near the shape", func(c color.RGBA) bool { return c.R < 230 }),
		pt(175, 95, "blurred shadow fades", func(c color.RGBA) bool { return c.R > 200 }),
	}})

	d = driver.NewDrawing(text)
	d.SetStrokeColor(driver.RGB(0)).SetLineWidth(4).SetLineDash(10, 10).
		BeginPath().MoveTo(0, 10).LineTo(100, 10).Stroke().SetLineDash()
	d.Scale(5, 5).SetTransform(1, 0, 0, 1, 120, 0).SetFillColor(driver.RGB(0xff0000)).FillRect(0, 0, 20, 20)
	d.ResetTransform()
	d.Save().BeginPath().Rect(0, 40, 100, 60).Clip()
	d.SetFillColor(driver.RGB(0x0000ff)).FillRect(0, 40, 100, 60)
	d.SetComposite(driver.CompositeDestinationIn).BeginPath().Arc(50, 70, 20, 0, 2*math.Pi, false).Fill()
	d.Restore()
	d.Save().BeginPath().Rect(110, 40, 80, 60).Clip()
	d.SetFillColor(driver.RGB(0xff0000)).FillRect(110, 40, 80, 60)
	d.SetComposite(driver.CompositeMultiply).SetFillColor(driver.RGB(0x00ff00)).FillRect(150, 40, 40, 60)
	d.Restore()
	out = append(out, &Sample{Name: "dashes-transforms-composites", Width: 200, Height: 120, Drawing: d, Probes: []Probe{
		pt(5, 10, "dash", dark), pt(15, 10, "gap", white),
		pt(130, 10, "set transform square", red), pt(145, 10, "past the 20 point square", white),
		pt(50, 70, "destination-in inside", blue), pt(10, 45, "destination-in clears outside", clear),
		pt(130, 70, "red alone", red), pt(170, 70, "red multiplied by green", dark),
	}})

	d = driver.NewDrawing(text)
	d.SetLineWidth(2).SetStrokeColor(driver.RGB(0)).BeginPath().MoveTo(10, 10).ArcTo(60, 10, 60, 60, 20).
		LineTo(60, 60).Stroke()
	d.SetFillColor(driver.RGB(0x00ff00)).BeginPath().RoundRectCorners(80, 10, 60, 50, 25, 0, 0, 0).Fill()
	d.SetFillColor(driver.RGB(0x0000ff)).BeginPath().EllipseArc(190, 35, 40, 10, math.Pi/2, 0, 2*math.Pi, false).Fill()
	heart := driver.MustParsePath("M 0 0 L 40 0 L 40 40 Z")
	d.Save().Translate(10, 70).SetFillColor(driver.RGB(0xff0000)).FillPath(heart, driver.NonZero).Restore()
	out = append(out, &Sample{Name: "shapes", Width: 240, Height: 120, Drawing: d, Probes: []Probe{
		pt(40, 10, "arcTo's line", dark), pt(60, 45, "arcTo's second line", dark), pt(58, 12, "arcTo cuts the corner", white),
		pt(84, 14, "rounded corner", white), pt(136, 14, "square corner", green),
		pt(190, 70, "ellipse turned upright", blue), pt(225, 35, "ellipse turned off its width", white),
		pt(45, 80, "path filled under the translation", red), pt(15, 105, "outside the path", white),
	}})

	d = driver.NewDrawing(text)
	font := driver.Font{Size: 40, Weight: 700}
	d.SetFillGradient(driver.CanvasLinear(0, 0, 200, 0, stops...)).FillText("MMMM", 0, 40, font)
	d.SetStrokeColor(driver.RGB(0x00aa00)).SetLineWidth(2).StrokeText("O", 10, 100, font)
	d.SetTextAlign(driver.TextAlignEnd).SetTextBaseline(driver.BaselineTop).SetFillColor(driver.RGB(0)).
		FillText("M", 300, 60, font)
	if logo != nil {
		d.DrawImageIn(driver.ImageSource{Data: logo}, 120, 60, 40, 40)
	}
	probes := []Probe{
		area(0, 20, 12, 18, "gradient text starts red", func(c color.RGBA) bool { return c.R > 150 && c.B < 120 }),
		area(0, 0, 200, 40, "gradient text ends blue", func(c color.RGBA) bool { return c.B > 120 && c.R < 150 }),
		area(8, 60, 40, 42, "stroked text", func(c color.RGBA) bool { return c.G > 100 && c.R < 100 }),
		{X: 301, Y: 55, W: 18, H: 50, What: "end-aligned text ends at its point", OK: white, All: true},
		area(270, 62, 30, 28, "end-aligned text, hung from its top", inked),
	}
	if logo != nil {
		probes = append(probes, Probe{X: 140, Y: 80, What: "image", OK: inked})
	}
	out = append(out, &Sample{Name: "text-images", Width: 320, Height: 120, Drawing: d, Probes: probes})
	return out
}

// Logo reads the gallery's logo, which the image sample draws, from the
// module's root given as a path from the test's directory.
func Logo(root string) []byte {
	data, err := os.ReadFile(root + "/examples/gallery/logo.png")
	if err != nil {
		return nil
	}
	return data
}
