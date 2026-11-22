package diagram

import (
	"strings"
	"testing"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

const wideFlowchart = "flowchart LR\n A[Receive request] --> B[Review details] --> C[Prepare response] --> D[Approve response] --> E[Send response]"

func TestViewFitsItsContainerAndRestoresDirection(t *testing.T) {
	var width *core.Signal[core.Points]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		width = core.NewSignal(cx, core.Points(280))
		return core.View{Style: core.Style{Padding: core.All(20)}, Children: []core.Node{
			core.View{TestID: "container", StyleFunc: func() core.Style {
				return core.Style{Width: core.Pt(width.Get()), Padding: core.All(12)}
			}, Children: []core.Node{View{TestID: "fit", Source: wideFlowchart}}},
		}}
	}, nsgotest.Size(1400, 1000))
	narrow := app.Find("id:fit").Frame.Size
	if narrow.Width > 256 || narrow.Height <= narrow.Width {
		t.Fatalf("a flowchart in a narrow padded container did not run down: %+v", narrow)
	}
	for _, op := range app.Find("id:fit").Ops {
		if op.Kind == driver.DrawScale && op.X != 1 {
			t.Fatalf("fitting stretched the labels horizontally by %v", op.X)
		}
	}
	app.Post(func() { width.Set(1200) })
	app.Settle()
	wide := app.Find("id:fit").Frame.Size
	if wide.Width <= wide.Height || wide.Height >= narrow.Height {
		t.Fatalf("widening the container kept the narrow layout: narrow %+v, wide %+v", narrow, wide)
	}
	app.Post(func() { width.Set(280) })
	app.Settle()
	if got := app.Find("id:fit").Frame.Size; got != narrow {
		t.Fatalf("narrowing again changed the layout: %+v, want %+v", got, narrow)
	}
	app.Screenshot("diagram-fit-container")
}

func TestTextViewDiagramFitsItsViewport(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.All(16)}, Children: []core.Node{
			textview.TextView{TestID: "doc", Markdown: "```mermaid\n" + wideFlowchart + "\n```", RenderDiagram: RenderDiagram},
		}}
	}, nsgotest.Size(320, 1000))
	narrow := app.Find("label:Diagram").Frame.Size
	if narrow.Width > 288 || narrow.Height <= narrow.Width {
		t.Fatalf("the diagram ignored its text viewport: %+v", narrow)
	}
	app.Resize(1400, 1000)
	wide := app.Find("label:Diagram").Frame.Size
	if wide.Width <= wide.Height || wide.Height >= narrow.Height {
		t.Fatalf("the diagram kept its narrow text viewport: narrow %+v, wide %+v", narrow, wide)
	}
	app.Screenshot("diagram-fit-textview")
}

func TestTextViewDiagramWidthsStayIndependent(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		doc := func(id string, width core.Points) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(width)}, Children: []core.Node{
				textview.TextView{TestID: id, Markdown: "```mermaid\n" + wideFlowchart + "\n```", RenderDiagram: RenderDiagram},
			}}
		}
		return core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{doc("narrow", 280), doc("wide", 1100)}}
	}, nsgotest.Size(1400, 1000))
	narrow, wide := app.Find("id:narrow >> label:Diagram").Frame.Size, app.Find("id:wide >> label:Diagram").Frame.Size
	if narrow.Width > 280 || narrow.Height <= narrow.Width || wide.Width <= wide.Height {
		t.Fatalf("two copies shared a viewport or layout: narrow %+v, wide %+v", narrow, wide)
	}
}

func TestTextViewDiagramHonorsItsOwnWidth(t *testing.T) {
	var width *core.Signal[core.Points]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		width = core.NewSignal(cx, core.Points(240))
		return textview.TextView{Markdown: "```mermaid\n" + wideFlowchart + "\n```", RenderDiagram: func(cx *core.Scope, block textview.CodeBlock) core.Node {
			return View{Source: block.Code, TestID: "styled", StyleFunc: func() core.Style { return core.Style{Width: core.Pt(width.Get())} }}
		}}
	}, nsgotest.Size(1400, 1000))
	narrow := app.Find("id:styled").Frame.Size
	if narrow.Width != 240 || narrow.Height <= narrow.Width {
		t.Fatalf("the styled diagram used its viewport instead of its own width: %+v", narrow)
	}
	app.Post(func() { width.Set(1100) })
	app.Settle()
	wide := app.Find("id:styled").Frame.Size
	if wide.Width != 1100 || wide.Width <= wide.Height {
		t.Fatalf("the styled diagram did not follow its new width: %+v", wide)
	}
}

func TestTextViewKeepsAnUnfittableDiagramScrollable(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{Markdown: "```mermaid\nflowchart LR\n A[" + strings.Repeat("W", 80) + "]\n```", RenderDiagram: RenderDiagram}
	}, nsgotest.Size(320, 600))
	diagram := app.Find("label:Diagram")
	scroll := diagram.Parent
	for scroll != nil && scroll.Kind != nsgotest.KindScroll {
		scroll = scroll.Parent
	}
	if scroll == nil || diagram.Frame.Size.Width <= scroll.Frame.Size.Width || scroll.Content.Width < diagram.Frame.Size.Width {
		t.Fatalf("an unbreakable label lost its horizontal scroll: diagram %+v, scroll %+v", diagram.Frame, scroll)
	}
}
