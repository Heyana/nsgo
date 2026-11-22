package formula

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

const wrappingFormula = `a+b+c+d+e+f+g+h=i+j+k+l+m+n+o+p`

func TestDisplayLayoutsKeepTheirWidthConstraintsSeparate(t *testing.T) {
	r := Default()
	key := layoutKey{source: wrappingFormula, display: true}
	natural := r.parsed(key, true)
	key.maxWidth = 600
	narrow := r.parsed(key, true)
	key.maxWidth = 1200
	wide := r.parsed(key, true)
	if narrow.failed || wide.failed || narrow.em.Width >= wide.em.Width || narrow.em.Ascent+narrow.em.Descent <= wide.em.Ascent+wide.em.Descent {
		t.Fatalf("narrow=%+v wide=%+v natural=%+v", narrow.em, wide.em, natural.em)
	}
	if r.parsed(key, true) != wide || r.parsed(natural.key, true) != natural {
		t.Fatal("a constrained layout replaced another frame in the renderer cache")
	}
	// With no safe operator to cut, preserve the full ink and let a containing scroll handle it instead of clipping or shrinking it.
	unbreakable := r.parsed(layoutKey{source: `abcdefghijklmnopqrstuv`, display: true, maxWidth: 100}, true)
	if unbreakable.failed || unbreakable.em.Width <= 1 {
		t.Fatalf("unbreakable formula lost its overflow: %+v", unbreakable.em)
	}
}

func TestDisplayFormulaWrapsOnResizeAndFontChange(t *testing.T) {
	r := Default()
	var width, font *core.Signal[core.Points]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		width, font = core.NewSignal(cx, core.Points(150)), core.NewSignal(cx, core.Points(20))
		return core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}, Children: []core.Node{
			core.View{StyleFunc: func() core.Style { return core.Style{Width: core.Pt(width.Get()), FontSize: font.Get()} }, Children: []core.Node{
				View{Source: wrappingFormula, Display: true, Renderer: r, TestID: "display"},
				View{Source: `x^2`, Renderer: r, TestID: "inline"},
			}},
		}}
	}, nsgotest.Size(600, 600))
	waitForFont(app, r, "id:display", "id:inline")
	narrow, inline := app.Find("id:display").Frame.Size, app.Find("id:inline").Frame.Size
	for _, op := range app.Find("id:display").Ops {
		if op.Kind == driver.DrawScale && (math.Abs(float64(op.X-20)) > .5 || op.X != op.Y) {
			t.Fatalf("wrapping changed the formula's font scale: %+v", op)
		}
	}
	app.Screenshot("formula-wrap/narrow")
	app.Post(func() { width.Set(340) })
	app.Settle()
	wide := app.Find("id:display").Frame.Size
	if narrow.Height <= wide.Height || narrow.Width > 151 || wide.Width > 341 {
		t.Fatalf("resize did not reflow: narrow=%v wide=%v", narrow, wide)
	}
	if got := app.Find("id:inline").Frame.Size; math.Abs(float64(got.Width-inline.Width)) > 1 || math.Abs(float64(got.Height-inline.Height)) > 1 {
		t.Fatalf("resizing the block changed an inline formula's font geometry: %v -> %v", inline, got)
	}
	app.Screenshot("formula-wrap/wide")
	app.Post(func() { width.Set(150) })
	app.Settle()
	if got := app.Find("id:display").Frame.Size; got != narrow {
		t.Fatalf("shrinking back retained the wide layout: %v want %v", got, narrow)
	}
	app.Post(func() { font.Set(30) })
	app.Settle()
	large := app.Find("id:display").Frame.Size
	if large.Height <= narrow.Height || large.Width > 151 {
		t.Fatalf("larger text was stretched or kept old line breaks: %v -> %v", narrow, large)
	}
	app.Screenshot("formula-wrap/large-font")
	app.SetEnvironment(core.Environment{FontScale: 1.5})
	if got := app.Find("id:display").Frame.Size; got.Height <= large.Height || got.Width > 151 {
		t.Fatalf("system text size did not reflow the display: %v -> %v", large, got)
	}
}

func TestTextViewDisplayWrapUsesEachPaddedViewport(t *testing.T) {
	r := Default()
	var padding *core.Signal[core.Points]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		padding = core.NewSignal(cx, core.Points(0))
		block := func(id string, width core.Points) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(width), FontSize: 20}, Children: []core.Node{
				textview.TextView{Markdown: "$$\n" + wrappingFormula + "\n$$",
					Classes: textview.Classes{textview.PartMathBlockScroll: "px-4"},
					StyleOf: func(part textview.Part) core.Style {
						if part != textview.PartMathBlockScroll {
							return core.Style{}
						}
						s := core.Style{BorderWidth: 2}
						if p := padding.Get(); p > 0 {
							s.Padding = core.Edges{Left: p, Right: p}
						}
						return s
					},
					RenderMath: func(_ *core.Scope, m textview.Math) core.Node {
						return View{Source: m.TeX, Display: m.Display, Renderer: r, TestID: id}
					}},
			}}
		}
		return core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}, Children: []core.Node{block("narrow", 180), block("wide", 340)}}
	}, nsgotest.Size(600, 700))
	waitForFont(app, r, "id:narrow", "id:wide")
	narrow, wide := app.Find("id:narrow"), app.Find("id:wide")
	if math.Abs(float64(narrow.Frame.Size.Width-144)) > 1 || math.Abs(float64(wide.Frame.Size.Width-304)) > 1 || narrow.Frame.Size.Height <= wide.Frame.Size.Height {
		t.Fatalf("viewport constraints leaked or ignored class padding: narrow=%v wide=%v", narrow.Frame.Size, wide.Frame.Size)
	}
	text := narrow.Parent
	for text.Kind != nsgotest.KindText {
		text = text.Parent
	}
	if len(text.Spans) != 1 || text.Spans[0].Copy != "$$\n"+wrappingFormula+"\n$$" {
		t.Fatalf("wrapped display lost its source copy: %+v", text.Spans)
	}
	app.Screenshot("formula-wrap/textview")
	// The viewport's outer frame stays fixed; only its padding changes.
	app.Post(func() { padding.Set(30) })
	app.Settle()
	if got := app.Find("id:narrow").Frame.Size; got.Width > 117 || got.Height < narrow.Frame.Size.Height {
		t.Fatalf("padding-only change did not update wrapping: before=%v after=%v", narrow.Frame.Size, got)
	}
}
