package formula

import (
	"math"
	"testing"

	"github.com/caelyreth/untie/display"

	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func hasFill(v *nsgotest.View, color core.Color) bool {
	for _, op := range v.Ops {
		if op.Kind == driver.DrawFillColor && op.Color == color {
			return true
		}
	}
	return false
}

func TestStreamingKeepsInkAndRestoresErrorsWhenFinished(t *testing.T) {
	const errorColor = core.Color(0x00ff00ff)
	var source *core.Signal[string]
	var streaming *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		source = core.NewSignal(cx, `x+`)
		streaming = core.NewSignal(cx, true)
		return core.View{Children: []core.Node{
			View{TestID: "stream", SourceFunc: source.Get, StreamingFunc: streaming.Get, Display: true, ErrorStyle: core.Style{Color: errorColor}},
			View{TestID: "ordinary", SourceFunc: source.Get, Display: true, ErrorStyle: core.Style{Color: errorColor}},
		}}
	})
	waitForFont(app, Default(), "id:stream", "id:ordinary")
	id := app.Find("id:stream").ID
	first := app.Find("id:stream").Frame.Size
	for _, src := range []string{`x+\f`, `x+\fr`, `x+\frac`, `x+\frac{1}`, `x+\frac{1}{`, `x+\frac{1}{2}`} {
		source.Set(src)
		app.Settle()
		v := app.Find("id:stream")
		if v.ID != id || v.Kind != nsgotest.KindCanvas || len(v.Ops) == 0 {
			t.Fatalf("%q lost the live formula: %+v", src, v)
		}
		if hasFill(v, errorColor) {
			t.Errorf("%q paints streaming errors", src)
		}
		if src == `x+\fr` && !hasFill(app.Find("id:ordinary"), errorColor) {
			t.Error("the ordinary formula lost its visible error control")
		}
	}
	if v, ordinary := app.Find("id:stream"), app.Find("id:ordinary"); v.Frame.Size == first || math.Abs(float64(v.Frame.Size.Width-ordinary.Frame.Size.Width)) > 1 || math.Abs(float64(v.Frame.Size.Height-ordinary.Frame.Size.Height)) > 1 {
		t.Errorf("completed formula size %+v, initial %+v, ordinary %+v", v.Frame.Size, first, app.Find("id:ordinary").Frame.Size)
	}
	// Changing only the streaming state must reveal a remaining error, even when another view shares the renderer's cached layout.
	source.Set(`x+\unknown`)
	app.Settle()
	if hasFill(app.Find("id:stream"), errorColor) || !hasFill(app.Find("id:ordinary"), errorColor) {
		t.Fatal("streaming visibility contaminated the ordinary cached layout")
	}
	streaming.Set(false)
	app.Settle()
	if !hasFill(app.Find("id:stream"), errorColor) || app.Find("id:stream").ID != id {
		t.Error("finishing did not reveal the error in the same canvas")
	}
}

func TestStreamingOmitsRecoveryPathsAndKeepsAuthoredColor(t *testing.T) {
	const src = `\color{blue}{x}+\fr`
	l := Default().parsed(layoutKey{source: src}, true)
	visible, hidden := 0, 0
	for _, run := range l.runs {
		if run.paint.Role == display.RoleError || run.paint.Role == display.RolePlaceholder {
			hidden++
		} else {
			visible++
		}
	}
	if hidden == 0 || visible == 0 {
		t.Fatalf("the partial fixture has %d visible and %d recovery runs", visible, hidden)
	}
	var source *core.Signal[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		source = core.NewSignal(cx, src)
		return View{TestID: "stream", SourceFunc: source.Get, Streaming: true}
	})
	waitForFont(app, Default(), "id:stream")
	v := app.Find("id:stream")
	fills := 0
	for _, op := range v.Ops {
		if op.Kind == driver.DrawFill {
			fills++
		}
	}
	if fills != visible || !hasFill(v, core.RGB(0x0000ff)) {
		t.Errorf("stream draws %d fills, want %d visible runs with authored blue", fills, visible)
	}
	source.Set("")
	app.Settle()
	for _, op := range app.Find("id:stream").Ops {
		if op.Kind == driver.DrawFill {
			t.Error("an empty streaming formula draws placeholder ink")
		}
	}
}

func TestStreamingTextViewFormulaFollowsTheReply(t *testing.T) {
	var reply *core.Signal[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		reply = core.NewSignal(cx, `Result: $x+\fr`)
		return textview.TextView{MarkdownFunc: reply.Get, Options: textview.Options{Streaming: true},
			RenderMath: func(_ *core.Scope, m textview.Math) core.Node {
				return View{TestID: "reply-math", Source: m.TeX, Display: m.Display, Streaming: true, ErrorStyle: core.Style{Color: core.RGB(0x00ff00)}}
			}}
	})
	waitForFont(app, Default(), "id:reply-math")
	initial := app.Find("id:reply-math").Frame.Size
	for _, src := range []string{`x+\frac{1}`, `x+\frac{1}{`, `x+\frac{1}{2}`} {
		reply.Set("Result: $" + src)
		app.Settle()
		v := app.Find("id:reply-math")
		if v.Kind != nsgotest.KindCanvas || len(v.Ops) == 0 || hasFill(v, core.RGB(0x00ff00)) {
			t.Fatalf("reply %q has no clean typeset formula", src)
		}
	}
	if app.Find("id:reply-math").Frame.Size == initial {
		t.Error("the reply's completed fraction kept its partial geometry")
	}
}
