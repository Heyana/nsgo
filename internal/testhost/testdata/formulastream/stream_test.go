package formulastream_test

import (
	"math"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base/formula"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestGrowingFormulaHasNoRecoveryInk(t *testing.T) {
	var source *core.Signal[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		source = core.NewSignal(cx, `x+`)
		return core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(24), Gap: 16, FontSize: 32, Color: core.RGB(0x000000), Background: core.RGB(0xffffff)}, Children: []core.Node{
			core.Text{Content: "Streaming"},
			formula.View{TestID: "stream", SourceFunc: source.Get, Streaming: true, Display: true, ErrorStyle: core.Style{Color: core.RGB(0x00ff00)}},
			core.Text{Content: "Ordinary"},
			formula.View{TestID: "ordinary", SourceFunc: source.Get, Display: true, ErrorStyle: core.Style{Color: core.RGB(0x00ff00)}},
		}}
	}, nsgotest.Size(600, 440))
	app.WaitUntil(10*time.Second, func() bool { return len(app.Find("id:stream").Ops) > 0 && len(app.Find("id:ordinary").Ops) > 0 })
	initial := app.Find("id:stream").Frame.Size
	for i, src := range []string{`x+\fr`, `x+\frac{1}`, `x+\frac{1}{`, `x+\frac{1}{2}`} {
		app.Post(func() { source.Set(src) })
		app.Settle()
		for _, op := range app.Find("id:stream").Ops {
			if op.Kind == driver.DrawFillColor && op.Color == core.RGB(0x00ff00) {
				t.Fatalf("%q paints recovery ink", src)
			}
		}
		if i == 0 {
			found := false
			for _, op := range app.Find("id:ordinary").Ops {
				found = found || op.Kind == driver.DrawFillColor && op.Color == core.RGB(0x00ff00)
			}
			if !found {
				t.Fatal("ordinary formula has no visible-error control")
			}
			app.Screenshot("formula-stream-partial")
		}
	}
	if v, ordinary := app.Find("id:stream"), app.Find("id:ordinary"); v.Kind != nsgotest.KindCanvas || v.Frame.Size == initial || math.Abs(float64(v.Frame.Size.Width-ordinary.Frame.Size.Width)) > 1 || math.Abs(float64(v.Frame.Size.Height-ordinary.Frame.Size.Height)) > 1 {
		t.Fatalf("completed formula geometry = %+v, initial %+v", v.Frame.Size, initial)
	}
	app.Screenshot("formula-stream-complete")
}
