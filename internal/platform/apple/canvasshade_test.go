//go:build darwin

package apple

import (
	"runtime"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
	"github.com/nsgo-dev/nsgo/internal/shader"
)

// shadingPrograms compiles the shading sample's programs and waits for
// their canvas pipelines.
func shadingPrograms(t testing.TB) (sdf, solid, uv *driver.ShaderProgram) {
	t.Helper()
	if !loadMetal() {
		t.Skip("no Metal device")
	}
	EnableCanvasShaders()
	var progs []*driver.ShaderProgram
	for _, src := range []string{canvastest.SDFShader, canvastest.SolidShader, canvastest.UVShader} {
		ir, err := shader.CompileFragment([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		p := &driver.ShaderProgram{IR: ir}
		canvasPipelines[p] = compileCanvasMetal(p)
		progs = append(progs, p)
	}
	deadline := time.Now().Add(10 * time.Second)
	for _, p := range progs {
		for !canvasPipelines[p].ready && time.Now().Before(deadline) {
			runMain()
			time.Sleep(time.Millisecond)
		}
		if canvasPipelines[p].state == 0 {
			t.Fatal("a canvas pipeline did not compile")
		}
	}
	return progs[0], progs[1], progs[2]
}

// TestCanvasShading plays the shading sample through Metal (#2260).
func TestCanvasShading(t *testing.T) {
	sdf, solid, uv := shadingPrograms(t)
	s := canvastest.Shading(TextEngine{}, sdf, solid, uv)
	for _, flipped := range []bool{false, true} {
		for _, f := range s.Check(play(t, s.Name, s.Drawing, s.Width, s.Height, flipped)) {
			t.Errorf("flipped %v: %s", flipped, f)
		}
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

func benchmarkGlyphs(b *testing.B, paint driver.ShaderPaint) {
	d := glyphRun(paint)
	ctx, img := bitmap(400, 320)
	defer runtime.KeepAlive(img) // the context draws into its pixels
	defer Release(ctx)
	host := CanvasHost{Ratio: canvastest.Scale}
	Play(ctx, d.Ops, host) // decodes the atlas
	b.ResetTimer()
	for b.Loop() {
		Play(ctx, d.Ops, host)
	}
}

// BenchmarkPlayAtlas plays 500 glyphs from an atlas unshaded, and
// BenchmarkPlayShadedAtlas the same through the SDF program in one pass.
func BenchmarkPlayAtlas(b *testing.B) { benchmarkGlyphs(b, driver.ShaderPaint{}) }

func BenchmarkPlayShadedAtlas(b *testing.B) {
	sdf, _, _ := shadingPrograms(b)
	benchmarkGlyphs(b, driver.NewShaderPaint(&driver.Shader{Program: sdf, Lanes: []float64{1, 0, 0, 1, 0}}))
}

// TestShadedGlyphRun draws the benchmark's 500 glyphs in one op.
func TestShadedGlyphRun(t *testing.T) {
	sdf, _, _ := shadingPrograms(t)
	d := glyphRun(driver.NewShaderPaint(&driver.Shader{Program: sdf, Lanes: []float64{1, 0, 0, 1, 0}}))
	if len(d.Ops) != 1 {
		t.Fatalf("%d ops", len(d.Ops))
	}
	img := play(t, "glyph-run", d, 400, 320, false)
	for _, at := range [][2]int{{8, 8}, {24*16 + 8, 19*16 + 8}} {
		if c := img.RGBAAt(at[0]*canvastest.Scale, at[1]*canvastest.Scale); c.R < 200 || c.G > 60 {
			t.Errorf("glyph at %v is %v", at, c)
		}
	}
	if c := img.RGBAAt(1, 1); c.G < 200 {
		t.Errorf("between glyphs %v", c)
	}
}
