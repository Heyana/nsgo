//go:build windows

package windows

import (
	"image/png"
	"os"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
	"github.com/nsgo-dev/nsgo/internal/shader"
)

// shadingPrograms compiles the shading sample's programs and links the
// canvas passes.
func shadingPrograms(t testing.TB) (sdf, solid, uv *driver.ShaderProgram) {
	t.Helper()
	(*window)(nil).EnableCanvasShaders()
	var progs []*driver.ShaderProgram
	for _, src := range []string{canvastest.SDFShader, canvastest.SolidShader, canvastest.UVShader} {
		ir, err := shader.CompileFragment([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		progs = append(progs, &driver.ShaderProgram{IR: ir})
	}
	return progs[0], progs[1], progs[2]
}

// TestCanvasShading plays the shading sample through Direct3D (#2260).
// NSGO_CANVAS_PNG names a directory to write it to.
func TestCanvasShading(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	sdf, solid, uv := shadingPrograms(t)
	w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
	s := canvastest.Shading(w, sdf, solid, uv)
	img := drawImage(t, g, s.Width, s.Height, func(p *painter) {
		p.canvas(&view{drawing: s.Drawing}, float32(s.Width), float32(s.Height))
	}, w)
	if dir := os.Getenv("NSGO_CANVAS_PNG"); dir != "" {
		if f, err := os.Create(dir + "/windows-shading.png"); err == nil {
			png.Encode(f, img)
			f.Close()
		}
	}
	for _, f := range s.Check(img) {
		t.Error(f)
	}
}

// sdfGlyphRun draws 500 glyphs of the SDF disc in rows, through DrawAtlas,
// shaded by paint unless it is the zero paint.
func sdfGlyphRun(paint driver.ShaderPaint) *driver.Drawing {
	disc := driver.ImageSource{Pixels: canvastest.SDFDisc(), Size: driver.Size{Width: 32, Height: 32}}
	const n = 500
	rects := make([]driver.Bounds, n)
	xforms := make([]driver.RSXform, n)
	for i := range n {
		rects[i] = driver.Bounds{Size: driver.Size{Width: 32, Height: 32}}
		xforms[i] = driver.RSXform{SCos: 0.5, TX: float32(i%25) * 16, TY: float32(i/25) * 16}
	}
	d := driver.NewDrawing(nil)
	if driver.PaintShader(paint) != nil {
		d.SetShader(paint)
	}
	return d.DrawAtlas(disc, rects, xforms, nil)
}

// TestShadedGlyphRun draws the benchmark's 500 glyphs in one op.
func TestShadedGlyphRun(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	sdf, _, _ := shadingPrograms(t)
	d := sdfGlyphRun(driver.NewShaderPaint(&driver.Shader{Program: sdf, Lanes: []float64{1, 0, 0, 1, 0}}))
	if len(d.Ops) != 1 {
		t.Fatalf("%d ops", len(d.Ops))
	}
	w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
	img := drawImage(t, g, 400, 320, func(p *painter) { p.canvas(&view{drawing: d}, 400, 320) }, w)
	for _, at := range [][2]int{{8, 8}, {24*16 + 8, 19*16 + 8}} {
		if c := img.RGBAAt(at[0]*canvastest.Scale, at[1]*canvastest.Scale); c.R < 200 || c.G > 60 {
			t.Errorf("glyph at %v is %v", at, c)
		}
	}
	if c := img.RGBAAt(1, 1); c.G < 200 {
		t.Errorf("between glyphs %v", c)
	}
}

func benchmarkGlyphs(b *testing.B, paint driver.ShaderPaint) {
	g, err := devices()
	if err != nil {
		b.Skip(err)
	}
	d := sdfGlyphRun(paint)
	const width, height = 400, 320
	pw, ph := uint32(width*canvastest.Scale), uint32(height*canvastest.Scale)
	props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied, dpiX: 96 * canvastest.Scale,
		dpiY: 96 * canvastest.Scale, options: d2dBitmapTarget}
	var target *comObject
	if failed(g.dc.callSize(dcCreateBitmap, pw, ph, 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&target)))) {
		b.Fatal("no target bitmap")
	}
	defer target.release()
	w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
	v := &view{drawing: d}
	frame := func() {
		g.dc.call(dcSetTarget, target.ptr())
		g.dc.callF(rtSetDpi, 0b11, f32(96*canvastest.Scale), f32(96*canvastest.Scale))
		g.dc.call(rtBeginDraw)
		white := colorF{1, 1, 1, 1}
		g.dc.call(rtClear, uintptr(unsafe.Pointer(&white)))
		p := newPainter(w, g)
		p.setTransform(identity)
		// The canvas plays its drawing, as a frame whose drawing changed does.
		v.canvasCache.release()
		p.canvas(v, width, height)
		p.release()
		g.dc.call(rtEndDraw, 0, 0)
		g.dc.call(dcSetTarget, 0)
	}
	frame() // uploads the atlas
	b.ResetTimer()
	for b.Loop() {
		frame()
	}
	v.canvasCache.release()
}

// BenchmarkPlayAtlas plays 500 glyphs from an atlas unshaded, and
// BenchmarkPlayShadedAtlas the same through the SDF program in one pass.
func BenchmarkPlayAtlas(b *testing.B) { benchmarkGlyphs(b, driver.ShaderPaint{}) }

func BenchmarkPlayShadedAtlas(b *testing.B) {
	sdf, _, _ := shadingPrograms(b)
	benchmarkGlyphs(b, driver.NewShaderPaint(&driver.Shader{Program: sdf, Lanes: []float64{1, 0, 0, 1, 0}}))
}
