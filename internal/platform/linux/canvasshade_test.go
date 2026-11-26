//go:build linux && !android

package linux

import (
	"image"
	"image/png"
	"os"
	"runtime"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
	"github.com/nsgo-dev/nsgo/internal/shader"
)

// shadingPrograms compiles the shading sample's programs, skipping where
// there is no display to make a GL context on.
func shadingPrograms(t testing.TB) (sdf, solid, uv *driver.ShaderProgram) {
	t.Helper()
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	// GTK stays on the thread that opened it.
	runtime.LockOSThread()
	if !gtkDisplayReady() {
		t.Skip("no display")
	}
	if restore := canvasGL(); restore == nil {
		t.Skip("no GL context for canvases")
	} else {
		restore()
	}
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

// playShaded plays d into a transparent image surface of w by h points
// at canvastest.Scale, as a canvas's raster is, and returns its pixels
// over white.
func playShaded(t testing.TB, name string, d *driver.Drawing, w, h int) *image.RGBA {
	win := &window{animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: canvastest.Scale}}
	width, height := w*canvastest.Scale, h*canvastest.Scale
	surface := cairoImageSurfaceCreate(0, int32(width), int32(height))
	defer cairoSurfaceDestroy(surface)
	cr := cairoCreate(surface)
	cairoScale(cr, canvastest.Scale, canvastest.Scale)
	win.playCanvas(&view{drawing: d}, cr, rect{w: float64(w), h: float64(h)})
	cairoDestroy(cr)
	cairoSurfaceFlush(surface)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	data, stride := cairoImageSurfaceGetData(surface), int(cairoImageSurfaceGetStride(surface))
	rows := unsafe.Slice((*byte)(cPointer(data)), stride*height)
	for y := range height {
		for x := range width {
			b := rows[y*stride+x*4:]
			img.Pix[y*img.Stride+x*4+0] = b[2] + 255 - b[3]
			img.Pix[y*img.Stride+x*4+1] = b[1] + 255 - b[3]
			img.Pix[y*img.Stride+x*4+2] = b[0] + 255 - b[3]
			img.Pix[y*img.Stride+x*4+3] = 255
		}
	}
	if dir := os.Getenv("NSGO_CANVAS_PNG"); dir != "" {
		if f, err := os.Create(dir + "/linux-" + name + ".png"); err == nil {
			png.Encode(f, img)
			f.Close()
		}
	}
	return img
}

// TestCanvasShading plays the shading sample through OpenGL (#2260).
func TestCanvasShading(t *testing.T) {
	sdf, solid, uv := shadingPrograms(t)
	s := canvastest.Shading(nil, sdf, solid, uv)
	for _, f := range s.Check(playShaded(t, s.Name, s.Drawing, s.Width, s.Height)) {
		t.Error(f)
	}
}

// glyphRun draws 500 glyphs of the SDF disc in rows, through DrawAtlas,
// shaded by paint unless it is the zero paint.
func glyphRun(paint driver.ShaderPaint) *driver.Drawing {
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
	sdf, _, _ := shadingPrograms(t)
	d := glyphRun(driver.NewShaderPaint(&driver.Shader{Program: sdf, Lanes: []float64{1, 0, 0, 1, 0}}))
	if len(d.Ops) != 1 {
		t.Fatalf("%d ops", len(d.Ops))
	}
	img := playShaded(t, "glyph-run", d, 400, 320)
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
	d := glyphRun(paint)
	win := &window{animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: canvastest.Scale}}
	surface := cairoImageSurfaceCreate(0, 400*canvastest.Scale, 320*canvastest.Scale)
	defer cairoSurfaceDestroy(surface)
	cr := cairoCreate(surface)
	defer cairoDestroy(cr)
	cairoScale(cr, canvastest.Scale, canvastest.Scale)
	v := &view{drawing: d}
	r := rect{w: 400, h: 320}
	win.playCanvas(v, cr, r) // decodes the atlas
	b.ResetTimer()
	for b.Loop() {
		win.playCanvas(v, cr, r)
	}
}

// BenchmarkPlayAtlas plays 500 glyphs from an atlas unshaded, and
// BenchmarkPlayShadedAtlas the same through the SDF program in one pass.
func BenchmarkPlayAtlas(b *testing.B) {
	if err := openLibs(); err != nil {
		b.Skip(err)
	}
	benchmarkGlyphs(b, driver.ShaderPaint{})
}

func BenchmarkPlayShadedAtlas(b *testing.B) {
	sdf, _, _ := shadingPrograms(b)
	benchmarkGlyphs(b, driver.NewShaderPaint(&driver.Shader{Program: sdf, Lanes: []float64{1, 0, 0, 1, 0}}))
}
