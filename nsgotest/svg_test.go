package nsgotest_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestCanvasDrawSVGVectorsGradientsAndNativeText(t *testing.T) {
	diagram, err := core.ParseSVG([]byte(`<svg viewBox="0 0 200 100">
		<defs>
			<linearGradient id="bg" x1="0%" y1="0%" x2="100%" y2="0%">
				<stop offset="0%" stop-color="#e0f2fe"/>
				<stop offset="100%" stop-color="#bae6fd"/>
			</linearGradient>
		</defs>
		<rect x="4" y="4" width="192" height="92" rx="8" fill="url(#bg)" stroke="#0284c7" stroke-width="2"/>
		<g transform="translate(20, 20)">
			<circle cx="16" cy="30" r="12" fill="currentColor"/>
		</g>
		<text x="110" y="46" text-anchor="middle" font-family="Inter" font-size="18" font-weight="bold" fill="#0f172a">
			Sales <tspan font-weight="normal" fill="#0369a1">Q4</tspan>
		</text>
		<text x="110" y="72" text-anchor="middle" font-size="12" fill="none" stroke="#334155" stroke-width="1">Outline</text>
	</svg>`))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	app := nsgotest.Start(t, func(_ *core.Scope) core.Node {
		return core.Canvas{
			TestID: "chart",
			Style:  core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Draw: func(d *core.Drawing, size core.Size) {
				d.SetFillColor(core.RGB(0x0ea5e9)).DrawSVGIn(diagram, 0, 0, float32(size.Width), float32(size.Height))
			},
		}
	}, nsgotest.Size(400, 200))

	canvas := app.Find("chart")
	if canvas == nil {
		t.Fatal("canvas not found")
	}

	var (
		scales       []driver.DrawOp
		roundRects   []driver.DrawOp
		ellipses     []driver.DrawOp
		fillGrads    []driver.DrawOp
		fillTexts    []driver.DrawOp
		strokeTexts  []driver.DrawOp
		tintedCircle bool
	)
	for i, op := range canvas.Ops {
		switch op.Kind {
		case driver.DrawScale:
			scales = append(scales, op)
		case driver.DrawRoundRect:
			roundRects = append(roundRects, op)
		case driver.DrawEllipse:
			ellipses = append(ellipses, op)
		case driver.DrawFillGradient:
			fillGrads = append(fillGrads, op)
		case driver.DrawFillText:
			fillTexts = append(fillTexts, op)
		case driver.DrawStrokeText:
			strokeTexts = append(strokeTexts, op)
		case driver.DrawFillColor:
			if op.Color == core.RGB(0x0ea5e9) && i > 0 {
				tintedCircle = true
			}
		}
	}

	if len(scales) != 1 || scales[0].X != 2 || scales[0].Y != 2 {
		t.Fatalf("scales = %+v, want 2x2 viewBox scaling into 400x200", scales)
	}
	if len(roundRects) != 1 || len(ellipses) != 1 || len(fillGrads) != 1 {
		t.Fatalf("vector ops: roundRects=%d ellipses=%d grads=%d, want 1 each", len(roundRects), len(ellipses), len(fillGrads))
	}
	if ellipses[0].X != 36 || ellipses[0].Y != 50 {
		t.Errorf("translated circle = (%v,%v), want (36,50)", ellipses[0].X, ellipses[0].Y)
	}
	if !tintedCircle {
		t.Error("currentColor circle did not receive the drawing's fill color")
	}
	if len(fillTexts) != 2 {
		t.Fatalf("DrawFillText ops = %+v, want 2 spans ('Sales ', 'Q4')", fillTexts)
	}
	if fillTexts[0].Text != "Sales " || fillTexts[0].Font.Weight != core.FontWeightBold || fillTexts[0].Font.Family != "Inter" {
		t.Errorf("span 0 = %+v", fillTexts[0])
	}
	if fillTexts[1].Text != "Q4" || fillTexts[1].Font.Weight != core.FontWeightNormal || fillTexts[1].X <= fillTexts[0].X {
		t.Errorf("span 1 = %+v", fillTexts[1])
	}
	if fillTexts[0].X1 >= 0 || fillTexts[0].X1 != fillTexts[1].X1 {
		t.Errorf("centered spans offsets = %v and %v, want matching negative offset", fillTexts[0].X1, fillTexts[1].X1)
	}
	if len(strokeTexts) != 1 || strokeTexts[0].Text != "Outline" {
		t.Fatalf("DrawStrokeText ops = %+v, want 1 'Outline'", strokeTexts)
	}
}

func TestCanvasDrawSVGAtIntrinsicSizeAndMalformedInput(t *testing.T) {
	if _, err := core.ParseSVG([]byte(`<svg viewBox="0 0 10 10"><path d="M0 0 L"/></svg>`)); err == nil {
		t.Fatal("expected error for malformed path")
	}
	if _, err := core.ParseSVG([]byte(`<not-svg/>`)); err == nil {
		t.Fatal("expected error for non-svg root")
	}

	badge := core.MustParseSVG([]byte(`<svg width="80" height="30" viewBox="0 0 80 30">
		<rect width="80" height="30" rx="6" fill="#2563eb"/>
		<text x="40" y="19" text-anchor="middle" font-size="12" fill="#ffffff">OK</text>
	</svg>`))

	app := nsgotest.Start(t, func(_ *core.Scope) core.Node {
		return core.Canvas{
			TestID: "badge-canvas",
			Style:  core.Style{Width: core.Pt(200), Height: core.Pt(100)},
			Draw: func(d *core.Drawing, _ core.Size) {
				d.DrawSVG(badge, 15, 25)
			},
		}
	}, nsgotest.Size(200, 100))

	canvas := app.Find("badge-canvas")
	if canvas == nil {
		t.Fatal("badge-canvas not found")
	}
	var gotTranslate, gotScale, gotText bool
	for _, op := range canvas.Ops {
		switch op.Kind {
		case driver.DrawTranslate:
			if op.X == 15 && op.Y == 25 {
				gotTranslate = true
			}
		case driver.DrawScale:
			if op.X == 1 && op.Y == 1 {
				gotScale = true
			}
		case driver.DrawFillText:
			if op.Text == "OK" && op.X == 40 && op.Y == 19 {
				gotText = true
			}
		}
	}
	if !gotTranslate || !gotScale || !gotText {
		t.Fatalf("DrawSVG ops missing expected transform/text: translate=%v scale=%v text=%v", gotTranslate, gotScale, gotText)
	}
}
