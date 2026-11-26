package capture_test

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestCanvasShading draws the shared shading sample through programs: an
// SDF glyph crisp at 1x, 4x and turned, two runs with other uniforms, and
// shaded rectangles among other ops under a clip, an alpha, an operator
// and transforms (#2260).
func TestCanvasShading(t *testing.T) {
	target := nsgotest.Target()
	if target == "" {
		t.Skip("run with nsgo test -target")
	}
	if strings.HasPrefix(target, "android") || target == "harmonyos" {
		t.Skip("the target's canvases have no GPU path for programs yet")
	}
	sdf := core.MustCompileShader(canvastest.SDFShader)
	solid := core.MustCompileShader(canvastest.SolidShader)
	uv := core.MustCompileShader(canvastest.UVShader)
	paints := canvastest.ShadingPaints{
		SDF: func(c [4]float64, outline float64) core.ShaderPaint {
			return sdf.Paint(core.Uniforms{"Color": c[:], "Outline": outline})
		},
		Solid: func(c [4]float64) core.ShaderPaint { return solid.Paint(core.Uniforms{"Color": c[:]}) },
		UV:    func() core.ShaderPaint { return uv.Paint(nil) },
	}
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{Style: core.Style{Background: core.RGB(0xffffff), Width: core.Pct(100), Height: core.Pct(100)},
			Children: []core.Node{core.Canvas{
				TestID: "shading",
				Style:  core.Style{Width: core.Pt(canvastest.ShadingWidth), Height: core.Pt(canvastest.ShadingHeight)},
				Draw:   func(d *core.Drawing, _ core.Size) { canvastest.DrawShading(d, paints) },
			}}}
	}, nsgotest.Size(canvastest.ShadingWidth, canvastest.ShadingHeight))
	var scale float32
	app.Post(func() { scale = scope.Env().PixelRatio() })
	app.Settle()
	origin := app.Find("id:shading").Bounds().Origin
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		failed := canvastest.CheckShading(app.Capture(), origin, scale)
		if len(failed) == 0 {
			return
		}
		if time.Now().After(deadline) {
			for _, f := range failed {
				t.Error(f)
			}
			return
		}
	}
}
