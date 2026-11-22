package formula

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A formula's view and recorded ink follow the fitted font, including when the text grows again.
func TestFormulaFollowsFittedText(t *testing.T) {
	var width *core.Signal[core.Points]
	const source = `\frac{a+b}{c+d}`
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		width = core.NewSignal[core.Points](cx, 150)
		return core.View{Style: core.Style{FontSize: 32, AlignItems: core.AlignStart, Padding: core.All(8)}, Children: []core.Node{
			core.Text{TestID: "reference-text", Spans: []core.Span{{Attachment: View{TestID: "reference", Source: source}}}},
			core.Text{TestID: "fitted-text", AdjustsFontSizeToFit: true, MinimumFontScale: 0.25, MaxLines: 1,
				StyleFunc: func() core.Style { return core.Style{Width: core.Pt(width.Get()), Height: core.Pt(100)} },
				Spans:     []core.Span{{Content: "before "}, {Content: "$" + source + "$", Attachment: View{TestID: "fitted", Source: source}}, {Content: " after"}},
			},
		}}
	}, nsgotest.Size(700, 500), nsgotest.Environment(core.Environment{FontScale: 1}))
	waitForFont(app, Default(), "id:reference", "id:fitted")
	check := func(shrunk bool) {
		t.Helper()
		text := app.Find("id:fitted-text")
		size := text.TextStyle.Font.Size
		reference := app.Find("id:reference")
		fitted := app.Find("id:fitted")
		if shrunk && (size >= 32 || size < 8) {
			t.Fatalf("fitted font %v, want between its minimum and original", size)
		}
		if !shrunk && size != 32 {
			t.Fatalf("font %v did not restore", size)
		}
		ratio := float64(size / 32)
		for axis, pair := range map[string][2]core.Points{"width": {reference.Frame.Size.Width, fitted.Frame.Size.Width}, "height": {reference.Frame.Size.Height, fitted.Frame.Size.Height}} {
			if math.Abs(float64(pair[1])-float64(pair[0])*ratio) > 1 {
				t.Errorf("formula %s %v, reference %v at fitted font %v", axis, pair[1], pair[0], size)
			}
		}
		var inkScale float32
		for _, op := range fitted.Ops {
			if op.Kind == driver.DrawScale {
				inkScale = op.X
				break
			}
		}
		if math.Abs(float64(inkScale)-float64(size)) > 1 {
			t.Errorf("drawing uses scale %v, fitted font %v", inkScale, size)
		}
		if len(text.Spans) < 2 || text.Spans[1].Copy != "$"+source+"$" {
			t.Error("fitting lost the formula's copied source")
		}
	}
	check(true)
	app.Screenshot("formula-fitted-small")
	app.Post(func() { width.Set(420) })
	app.Settle()
	check(false)
	app.Screenshot("formula-fitted-restored")
	app.Post(func() { width.Set(150) })
	app.Settle()
	check(true)
}
