package canvastest

import (
	"image"
	"image/color"
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The programs the shading sample draws with, which a backend's test
// compiles (shader.CompileFragment) and passes to Shading.
const (
	// SDFShader thresholds a signed distance field's red channel, read
	// smoothly from image0, at 0.5 less Outline, with an edge a pixel wide
	// whatever the scale: a glyph of SDF text in Color.
	SDFShader = `package main

var Color vec4
var Outline float

func dist(p vec2) float {
	q := p - 0.5
	i := floor(q)
	f := q - i
	a := image0At(i + vec2(0.5, 0.5)).r
	b := image0At(i + vec2(1.5, 0.5)).r
	c := image0At(i + vec2(0.5, 1.5)).r
	d := image0At(i + vec2(1.5, 1.5)).r
	return mix(mix(a, b, f.x), mix(c, d, f.x), f.y)
}

func Fragment(dst vec4, pos vec2) vec4 {
	d := dist(dst.xy)
	w := max(fwidth(d), 0.001)
	k := smoothstep(0.5-Outline-w, 0.5-Outline+w, d)
	return vec4(Color.rgb*Color.a, Color.a) * k
}
`
	// SolidShader fills with Color.
	SolidShader = `package main

var Color vec4

func Fragment(dst vec4, pos vec2) vec4 {
	return vec4(Color.rgb*Color.a, Color.a)
}
`
	// UVShader goes from blue at the rectangle's left to red at its right.
	UVShader = `package main

func Fragment(dst vec4, pos vec2) vec4 {
	u := pos.x / rectSize().x
	return vec4(u, 0, 1-u, 1)
}
`
)

// SDFDisc is a 32 by 32 signed distance field of a disc 10 texels in
// radius at its center, in its red channel: 0.5 on the edge, a texel
// adding 1/16 inside.
func SDFDisc() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			d := 10 - math.Hypot(float64(x)+0.5-16, float64(y)+0.5-16)
			v := uint8(math.Round(255 * min(max(0.5+d/16, 0), 1)))
			img.SetRGBA(x, y, color.RGBA{R: v, A: 255})
		}
	}
	return img
}

// ShadingPaints makes the shading sample's paints: SDFShader's in a
// straight color with an outline, SolidShader's in a straight color, and
// UVShader's.
type ShadingPaints struct {
	SDF   func(color [4]float64, outline float64) driver.ShaderPaint
	Solid func(color [4]float64) driver.ShaderPaint
	UV    func() driver.ShaderPaint
}

// Shading draws through programs compiled from SDFShader, SolidShader and
// UVShader (DrawShading).
func Shading(text driver.TextEngine, sdf, solid, uv *driver.ShaderProgram) *Sample {
	paint := func(p *driver.ShaderProgram, lanes ...float64) driver.ShaderPaint {
		return driver.NewShaderPaint(&driver.Shader{Program: p, Lanes: lanes})
	}
	d := driver.NewDrawing(text)
	DrawShading(d, ShadingPaints{
		SDF: func(c [4]float64, outline float64) driver.ShaderPaint {
			return paint(sdf, c[0], c[1], c[2], c[3], outline)
		},
		Solid: func(c [4]float64) driver.ShaderPaint { return paint(solid, c[0], c[1], c[2], c[3]) },
		UV:    func() driver.ShaderPaint { return paint(uv) },
	})
	return &Sample{Name: "shading", Width: ShadingWidth, Height: ShadingHeight, Drawing: d, Probes: ShadingProbes()}
}

// The shading sample's size in points.
const ShadingWidth, ShadingHeight = 300, 200

// DrawShading draws an SDF disc from an atlas at 1x, 4x and turned, again
// with other uniforms, and rectangles shaded among other ops, under a
// clip, an alpha, an operator and transforms, which ShadingProbes check.
func DrawShading(d *driver.Drawing, p ShadingPaints) {
	disc := driver.ImageSource{Pixels: SDFDisc(), Size: driver.Size{Width: 32, Height: 32}}
	tile := []driver.Bounds{{Size: driver.Size{Width: 32, Height: 32}}}
	d.SetShader(p.SDF([4]float64{1, 0, 0, 1}, 0))
	d.DrawAtlas(disc, tile, []driver.RSXform{{SCos: 1, TX: 10, TY: 10}}, nil)
	d.DrawAtlas(disc, tile, []driver.RSXform{{SCos: 4, TX: 60, TY: 10}}, nil)
	q := float32(2 * math.Sqrt2 / 2)
	d.DrawAtlas(disc, tile, []driver.RSXform{{SCos: q, SSin: q, TX: 230, TY: 10}}, nil)
	d.SetShader(p.SDF([4]float64{0, 1, 0, 1}, 0.2))
	d.DrawAtlas(disc, tile, []driver.RSXform{{SCos: 1, TX: 10, TY: 60}}, nil)
	d.ClearShader()

	bluePaint, redPaint, greenPaint := p.Solid([4]float64{0, 0, 1, 1}), p.Solid([4]float64{1, 0, 0, 1}), p.Solid([4]float64{0, 1, 0, 1})
	// Order: a fill over a shaded rectangle, and another over the fill.
	d.DrawShader(bluePaint, 10, 110, 40, 40)
	d.SetFillColor(driver.RGB(0xff0000)).FillRect(30, 110, 30, 40)
	d.DrawShader(greenPaint, 55, 110, 20, 40)
	// Clip.
	d.Save().BeginPath().Rect(100, 110, 20, 40).Clip()
	d.DrawShader(bluePaint, 90, 110, 40, 40)
	d.Restore()
	// Alpha.
	d.SetAlpha(0.5).DrawShader(redPaint, 140, 110, 30, 40).SetAlpha(1)
	// An operator: the shaded rectangle goes under the fill.
	d.SetFillColor(driver.RGB(0x0000ff)).FillRect(180, 110, 30, 40)
	d.SetComposite(driver.CompositeDestinationOver).DrawShader(redPaint, 195, 110, 30, 40)
	d.SetComposite(driver.CompositeSourceOver)
	// A turned square, a diamond.
	d.Save().Translate(270, 130).Rotate(math.Pi/4).DrawShader(greenPaint, -10, -10, 20, 20).Restore()
	// pos and rectSize in the rectangle's own points, stretched.
	d.Save().Translate(10, 160).Scale(2, 1).DrawShader(p.UV(), 0, 0, 50, 30).Restore()
}

// ShadingProbes are what DrawShading must show over white.
func ShadingProbes() []Probe {
	pink := func(c color.RGBA) bool { return c.R > 230 && c.G > 90 && c.G < 170 && c.B > 90 && c.B < 170 }
	return []Probe{
		pt(26, 26, "1x glyph's center", red), pt(26, 21, "1x glyph inside its edge", red),
		pt(26, 39, "1x glyph outside its edge", white),
		pt(124, 74, "4x glyph's center", red), pt(124, 74+37, "4x glyph two points inside its edge", red),
		pt(124, 74+43, "4x glyph two points outside its edge", white),
		pt(124+37, 74, "4x glyph's crisp side", red), pt(124-43, 74, "4x glyph's other side", white),
		pt(230, 55.25, "turned glyph's center", red), pt(230, 55.25+17, "turned glyph inside its edge", red),
		pt(230, 55.25+24, "turned glyph outside its edge", white),
		pt(26+11.5, 76, "outlined run reaches past the plain run's edge", green),
		pt(26+11.5, 26, "plain run's edge", white),
		pt(20, 130, "shaded rectangle", blue), pt(40, 130, "fill over the shaded rectangle", red),
		pt(65, 130, "shaded rectangle over the fill", green),
		pt(95, 130, "shaded outside the clip", white), pt(110, 130, "shaded inside the clip", blue),
		pt(125, 130, "shaded past the clip", white),
		pt(155, 130, "shaded at half alpha", pink),
		pt(200, 130, "shaded under the fill", blue), pt(220, 130, "shaded beside the fill", red),
		pt(282, 130, "turned square's corner", green), pt(279, 139, "turned square's side", white),
		pt(13, 175, "stretched rectangle's left", blue), pt(107, 175, "stretched rectangle's right", red),
	}
}

// CheckShading returns the probes a capture fails, the sample drawn on
// white with its top left corner at origin in points and scale pixels a
// point.
func CheckShading(img image.Image, origin driver.Point, scale float32) []string {
	var failed []string
	b := img.Bounds()
	for _, p := range ShadingProbes() {
		x := b.Min.X + int((float32(origin.X)+float32(p.X))*scale)
		y := b.Min.Y + int((float32(origin.Y)+float32(p.Y))*scale)
		if c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA); !p.OK(c) {
			failed = append(failed, p.What+": "+colorString(c))
		}
	}
	return failed
}
