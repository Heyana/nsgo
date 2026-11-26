package canvasshade

import (
	"image"
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

func shader() driver.ShaderPaint {
	return driver.NewShaderPaint(&driver.Shader{Program: &driver.ShaderProgram{IR: &shaderir.Program{}}, Lanes: []float64{7}})
}

func TestPlanShaderRectUnderTransform(t *testing.T) {
	op := driver.DrawOp{Kind: driver.DrawShader, Shader: shader(), X: 10, Y: 20, W: 30, H: 40}
	// Two pixels a point, then moved by (5, 6) pixels.
	m := Affine{A: 2, D: 2, E: 5, F: 6}
	p, ok := Plan(&op, m, image.Rect(0, 0, 1000, 1000), Image{})
	if !ok {
		t.Fatal("planned nothing")
	}
	if want := image.Rect(25, 46, 85, 126); p.Bounds != want {
		t.Fatalf("bounds %v, want %v", p.Bounds, want)
	}
	if p.Size != [2]float64{30, 40} || p.Scale != 2 {
		t.Fatalf("size %v scale %v", p.Size, p.Scale)
	}
	last := p.Vertices[4] // the far corner
	if last.X != 60 || last.Y != 80 || last.PosX != 30 || last.PosY != 40 || last.DstX != 60 || last.DstY != 80 {
		t.Fatalf("far corner %+v", last)
	}
	lanes := p.Lanes(1, func(int) (float64, float64) { return 3, 4 })
	if want := []float64{30, 40, 2, 3, 4, 7}; !equal(lanes, want) {
		t.Fatalf("lanes %v, want %v", lanes, want)
	}
}

func TestPlanClipsAndSkips(t *testing.T) {
	op := driver.DrawOp{Kind: driver.DrawShader, Shader: shader(), X: 0, Y: 0, W: 100, H: 100}
	p, ok := Plan(&op, Affine{A: 1, D: 1}, image.Rect(10, 10, 20, 30), Image{})
	if !ok || p.Bounds != image.Rect(10, 10, 20, 30) {
		t.Fatalf("clipped to %v, %v", p.Bounds, ok)
	}
	if v := p.Vertices[0]; v.X != -10 || v.Y != -10 || v.PosX != 0 {
		t.Fatalf("first corner %+v", v)
	}
	if _, ok := Plan(&op, Affine{A: 1, D: 1, E: 500}, image.Rect(0, 0, 100, 100), Image{}); ok {
		t.Fatal("planned a pass outside the clip")
	}
	plain := driver.DrawOp{Kind: driver.DrawShader, X: 0, Y: 0, W: 10, H: 10}
	if _, ok := Plan(&plain, Affine{A: 1, D: 1}, image.Rect(0, 0, 100, 100), Image{}); ok {
		t.Fatal("planned an op with no shader")
	}
}

func TestPlanAtlasSprites(t *testing.T) {
	img := driver.ImageSource{URI: "atlas.png"}
	sprites := driver.AtlasSprites{
		{Rect: driver.Bounds{Origin: driver.Point{X: 8, Y: 0}, Size: driver.Size{Width: 8, Height: 4}}, Transform: driver.RSXform{SCos: 0, SSin: 2, TX: 50, TY: 10}, Alpha: 0.5},
	}
	op := driver.DrawOp{Kind: driver.DrawAtlas, Image: &img, Atlas: &sprites, Shader: shader()}
	p, ok := Plan(&op, Affine{A: 1, D: 1}, image.Rect(0, 0, 200, 200), Image{W: 16, H: 4, PixelsW: 32, PixelsH: 8})
	if !ok {
		t.Fatal("planned nothing")
	}
	if len(p.Images) != 1 || p.Images[0].URI != "atlas.png" || p.Size != [2]float64{16, 4} || p.Scale != 2 {
		t.Fatalf("images %v size %v scale %v", p.Images, p.Size, p.Scale)
	}
	// Rotated a quarter turn and doubled: the sprite's (8, 4) corner, its
	// right bottom, lands at (50 - 8, 10 + 16).
	far := p.Vertices[4]
	x, y := far.X+float32(p.Bounds.Min.X), far.Y+float32(p.Bounds.Min.Y)
	if math.Abs(float64(x-42)) > 1e-4 || math.Abs(float64(y-26)) > 1e-4 || far.PosX != 16 || far.PosY != 4 || far.DstX != 32 || far.DstY != 8 {
		t.Fatalf("far corner at %v, %v: %+v", x, y, far)
	}
	if far.A != 0.5 || far.R != 0.5 {
		t.Fatalf("sprite color %+v", far)
	}
}

func TestPackFlipsForOpenGL(t *testing.T) {
	op := driver.DrawOp{Kind: driver.DrawShader, Shader: shader(), X: 0, Y: 0, W: 10, H: 10}
	p, _ := Plan(&op, Affine{A: 1, D: 1}, image.Rect(0, 0, 10, 10), Image{})
	up, down := p.Pack(nil, false), p.Pack(nil, true)
	if len(up) != 6*Floats || up[0] != -1 || up[1] != 1 || down[1] != -1 {
		t.Fatalf("packed %v and %v", up[:2], down[:2])
	}
}

func TestAffineInvert(t *testing.T) {
	m := Affine{A: 0.8, B: 0.6, C: -1.2, D: 1.6, E: 3, F: -4}
	id := m.Then(m.Invert())
	for _, v := range [...][2]float64{{id.A, 1}, {id.B, 0}, {id.C, 0}, {id.D, 1}, {id.E, 0}, {id.F, 0}} {
		if math.Abs(v[0]-v[1]) > 1e-9 {
			t.Fatalf("m after its inverse is %+v", id)
		}
	}
}

func equal(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
