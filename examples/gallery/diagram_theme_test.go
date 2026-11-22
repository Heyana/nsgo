package main

import (
	"os"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

func TestDocumentDiagramsFollowTheAppPalette(t *testing.T) {
	first, second := core.DefaultTheme(), core.DefaultTheme()
	first.Light.Text, first.Light.Surface, first.Light.Border = core.RGB(0x183a57), core.RGB(0xe4f1d8), core.RGB(0x365b23)
	first.Light.Accent, first.Light.Success = core.RGB(0x9f3165), core.RGB(0x237d66)
	second.Light.Text, second.Light.Surface, second.Light.Border = core.RGB(0x6f2841), core.RGB(0xf7dbc0), core.RGB(0x8f5620)
	second.Light.Accent, second.Light.Success = core.RGB(0x345bad), core.RGB(0x8e6221)
	const markdown = "```mermaid\nflowchart LR\n A[First] --> B[Second]\n```\n\n```mermaid\npie\n \"One\" : 2\n \"Two\" : 3\n```"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.SetTheme(first)
		return core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), Padding: core.All(16), Gap: 12}, Children: []core.Node{
			base.Button{TestID: "change-palette", OnPress: func(core.PressEvent) { cx.SetTheme(second) }, Children: []core.Node{core.Text{Content: "Change palette"}}},
			ui.TextView{TestID: "document", Markdown: markdown, RenderDiagram: renderThemedDiagram},
		}}
	}, nsgotest.Size(760, 680), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.Settle()
	canvases := func() (ids []driver.NodeID, colors map[core.Color]bool) {
		colors = make(map[core.Color]bool)
		var visit func(*nsgotest.View)
		visit = func(v *nsgotest.View) {
			if v.Kind == nsgotest.KindCanvas {
				ids = append(ids, v.ID)
				for _, op := range v.Ops {
					if op.Kind == driver.DrawFillColor || op.Kind == driver.DrawStrokeColor {
						colors[op.Color] = true
					}
				}
			}
			for _, child := range v.Children {
				visit(child)
			}
		}
		visit(app.Find("id:document"))
		return
	}
	check := func(p core.Palette) []driver.NodeID {
		t.Helper()
		ids, colors := canvases()
		if len(ids) != 2 {
			t.Fatalf("document has %d diagram canvases", len(ids))
		}
		for _, want := range []core.Color{p.Text, p.Surface, p.Border, p.Accent, p.Success} {
			if !colors[want] {
				t.Errorf("diagrams did not paint palette color %v; colors=%v", want, colors)
			}
		}
		return ids
	}
	before := check(first.Light)
	if os.Getenv("NSGO_TEST_TARGET") != "" {
		app.Screenshot("diagram-palette/before")
	}
	app.Press("id:change-palette")
	app.Settle()
	if after := check(second.Light); !slices.Equal(before, after) {
		t.Fatalf("palette change replaced diagrams: %v -> %v", before, after)
	}
	if os.Getenv("NSGO_TEST_TARGET") != "" {
		app.Screenshot("diagram-palette/after")
		// Check actual native pixels as well as the recorded drawing.
		img := app.Capture()
		r, g, b := uint32(second.Light.Surface>>24), uint32(second.Light.Surface>>16)&255, uint32(second.Light.Surface>>8)&255
		found := 0
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				pr, pg, pb, pa := img.At(x, y).RGBA()
				if pa == 65535 && pr>>8 == r && pg>>8 == g && pb>>8 == b {
					found++
				}
			}
		}
		if found < 20 {
			t.Fatalf("native capture has only %d pixels in the changed node surface color", found)
		}
	}
}
