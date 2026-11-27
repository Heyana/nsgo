package driver

import (
	"math"
	"slices"
	"testing"
)

// svgTestEngine measures each rune at advancePerRune * fontSize (1.0 em per
// rune, distinct from the 0.6 em nil-engine approximation) so tests can prove
// that SVG text layout actually queries the Drawing's platform TextEngine.
type svgTestEngine struct {
	charWidth float32
	ascent    float32
	descent   float32
	calls     int
}

type svgTestPrepared struct {
	text    string
	size    Points
	charW   float32
	ascent  float32
	descent float32
}

func (e *svgTestEngine) Prepare(t *AttributedText) PreparedText {
	e.calls++
	return svgTestPrepared{
		text:    t.Text,
		size:    t.Style.Font.Size,
		charW:   e.charWidth,
		ascent:  e.ascent,
		descent: e.descent,
	}
}

func (p svgTestPrepared) Measure(Points) Size {
	n := len([]rune(p.text))
	return Size{
		Width:  Points(float32(n) * p.charW * float32(p.size)),
		Height: Points((p.ascent + p.descent) * float32(p.size)),
	}
}

func (p svgTestPrepared) Layout(s Size) TextLayout { return svgTestLayout{p, s} }
func (svgTestPrepared) Release()                   {}

type svgTestLayout struct {
	p svgTestPrepared
	s Size
}

func (l svgTestLayout) Release() {}

func (l svgTestLayout) TextLines() []TextLine {
	return []TextLine{{
		Bounds:    Bounds{Size: l.s},
		Ascender:  Points(l.p.ascent * float32(l.p.size)),
		Descender: Points(l.p.descent * float32(l.p.size)),
	}}
}

func TestSVGTextUsesPlatformTextEngineAndEmitsNativeTextOps(t *testing.T) {
	s := mustParse(t, []byte(`<svg viewBox="0 0 200 100">
		<text x="100" y="50" text-anchor="middle" dominant-baseline="central"
			font-family="Inter, sans-serif" font-size="20" font-weight="bold" font-style="italic"
			fill="#112233">Hi</text>
	</svg>`))
	if !s.HasText() {
		t.Fatal("HasText() = false, want true")
	}

	eng := &svgTestEngine{charWidth: 1.0, ascent: 0.75, descent: 0.25}
	d := NewDrawing(eng)
	d.DrawSVGIn(s, 10, 20, 400, 200)

	if eng.calls == 0 {
		t.Fatal("DrawSVGIn did not query the Drawing's TextEngine")
	}

	// Verify viewBox 200x100 scaled uniformly 2x into (10, 20, 400, 200).
	scales := kindsOf(d.Ops, DrawScale)
	if len(scales) != 1 || scales[0].X != 2 || scales[0].Y != 2 {
		t.Fatalf("scales = %+v, want single 2x2 scale", scales)
	}
	translates := kindsOf(d.Ops, DrawTranslate)
	if len(translates) < 2 || translates[0].X != 10 || translates[0].Y != 20 {
		t.Fatalf("translates = %+v, want origin (10, 20)", translates)
	}

	texts := kindsOf(d.Ops, DrawFillText)
	if len(texts) != 1 {
		t.Fatalf("DrawFillText ops = %d, want 1 (%+v)", len(texts), d.Ops)
	}
	op := texts[0]
	if op.Text != "Hi" {
		t.Errorf("text = %q, want %q", op.Text, "Hi")
	}
	if op.Font.Family != "Inter, sans-serif" || op.Font.Size != 20 || op.Font.Weight != FontWeightBold || !op.Font.Italic {
		t.Errorf("font = %+v", op.Font)
	}
	// With svgTestEngine (1.0 em per rune), "Hi" at 20pt is 40pt wide:
	// text-anchor="middle" shifts X1 by -20 (NOT the -12 approximation).
	// Ascent=15, Descent=5 -> BaselineMiddle shifts Y1 by (15-5)/2 = 5.
	if op.X != 100 || op.Y != 50 || op.X1 != -20 || op.Y1 != 5 {
		t.Errorf("position = (%v,%v) offset (%v,%v), want (100,50) offset (-20,5)", op.X, op.Y, op.X1, op.Y1)
	}
	if op.TextAlign != TextAlignCenter || op.TextBaseline != BaselineMiddle {
		t.Errorf("align=%v baseline=%v", op.TextAlign, op.TextBaseline)
	}

	fills := kindsOf(d.Ops, DrawFillColor)
	if len(fills) != 1 || fills[0].Color != RGB(0x112233) {
		t.Errorf("fill colors = %+v", fills)
	}
}

func TestSVGTspanFlowAnchorsAndChunks(t *testing.T) {
	s := mustParse(t, []byte(`<svg viewBox="0 0 200 100">
		<text x="100" y="30" text-anchor="middle" font-size="10" fill="#000">
			AB <tspan font-weight="bold" fill="#ff0000">CD</tspan>
			<tspan x="20" dy="1.5em" text-anchor="start" fill="#0000ff">Line2</tspan>
		</text>
	</svg>`))

	// 1.0 em per rune -> each character at 10pt is 10pt wide.
	eng := &svgTestEngine{charWidth: 1.0, ascent: 0.8, descent: 0.2}
	d := NewDrawing(eng)
	d.DrawSVG(s, 0, 0)

	texts := kindsOf(d.Ops, DrawFillText)
	if len(texts) != 3 {
		t.Fatalf("texts = %+v, want 3 runs", texts)
	}
	// Chunk 1: "AB " (30pt) + "CD" (20pt) = 50pt total, centered at x=100 -> ax = -25.
	if texts[0].Text != "AB " || texts[0].X != 100 || texts[0].Y != 30 || texts[0].X1 != -25 {
		t.Errorf("run 0 = %+v, want AB  at (100,30) X1=-25", texts[0])
	}
	if texts[1].Text != "CD" || texts[1].Font.Weight != FontWeightBold || texts[1].X != 130 || texts[1].Y != 30 || texts[1].X1 != -25 {
		t.Errorf("run 1 = %+v, want CD at (130,30) X1=-25", texts[1])
	}
	// Chunk 2 starts a new chunk at x=20, y=30+15=45, text-anchor="start" -> X1=0.
	if texts[2].Text != "Line2" || texts[2].X != 20 || texts[2].Y != 45 || texts[2].X1 != 0 {
		t.Errorf("run 2 = %+v, want Line2 at (20,45) X1=0", texts[2])
	}
}

func TestSVGTspanInlineCenterExactWidth(t *testing.T) {
	s := mustParse(t, []byte(`<svg viewBox="0 0 200 100"><text x="100" y="40" text-anchor="middle" font-size="10">AB<tspan font-weight="bold">CDE</tspan></text></svg>`))
	eng := &svgTestEngine{charWidth: 1.0, ascent: 0.8, descent: 0.2}
	d := NewDrawing(eng)
	d.DrawSVG(s, 0, 0)

	texts := kindsOf(d.Ops, DrawFillText)
	if len(texts) != 2 {
		t.Fatalf("texts = %+v, want 2", texts)
	}
	// "AB" (20) + "CDE" (30) = 50 -> centered at 100 means X1 = -25.
	// Run 0 at X=100, X1=-25 -> drawn at 75.
	// Run 1 at X=120, X1=-25 -> drawn at 95.
	if texts[0].X != 100 || texts[0].X1 != -25 {
		t.Errorf("run 0 = %+v, want X=100 X1=-25", texts[0])
	}
	if texts[1].X != 120 || texts[1].X1 != -25 {
		t.Errorf("run 1 = %+v, want X=120 X1=-25", texts[1])
	}
}

func TestSVGTextEndAnchorAndBaselines(t *testing.T) {
	s := mustParse(t, []byte(`<svg viewBox="0 0 200 200" font-size="10">
		<text x="100" y="20" text-anchor="end">ABCD</text>
		<text x="10" y="40" dominant-baseline="text-before-edge">Top</text>
		<text x="10" y="60" dominant-baseline="hanging">Hang</text>
		<text x="10" y="80" dominant-baseline="middle">Mid</text>
		<text x="10" y="100" dominant-baseline="text-after-edge">Bot</text>
		<text x="10" y="120" dominant-baseline="ideographic">Ideo</text>
	</svg>`))
	eng := &svgTestEngine{charWidth: 1.0, ascent: 0.8, descent: 0.2}
	d := NewDrawing(eng)
	d.DrawSVG(s, 0, 0)

	texts := kindsOf(d.Ops, DrawFillText)
	if len(texts) != 6 {
		t.Fatalf("got %d texts, want 6", len(texts))
	}
	// end anchor on 4 chars * 10pt = -40
	if texts[0].X1 != -40 || texts[0].TextAlign != TextAlignEnd {
		t.Errorf("end anchor = %+v", texts[0])
	}
	wantY1 := []float32{0, 8, 6.4, 3, -2, -2}
	for i, want := range wantY1 {
		if math.Abs(float64(texts[i].Y1-want)) > 1e-4 {
			t.Errorf("text %d (%q) Y1 = %v, want %v", i, texts[i].Text, texts[i].Y1, want)
		}
	}
}

func TestSVGTextStrokeFillOpacityGradientsAndCurrentColor(t *testing.T) {
	s := mustParse(t, []byte(`<svg viewBox="0 0 200 100">
		<defs>
			<linearGradient id="g" x1="0%" y1="0%" x2="100%" y2="0%">
				<stop offset="0" stop-color="#ff0000"/>
				<stop offset="1" stop-color="#0000ff"/>
			</linearGradient>
		</defs>
		<g opacity="0.5">
			<text x="10" y="20" font-size="16" fill="currentColor" stroke="#00ff00" stroke-width="2" stroke-dasharray="3 1">Tinted</text>
		</g>
		<text x="10" y="60" font-size="20" fill="url(#g)">Grad</text>
	</svg>`))

	eng := &svgTestEngine{charWidth: 1.0, ascent: 0.8, descent: 0.2}
	d := NewDrawing(eng)
	d.SetFillColor(RGB(0xabcdef)).DrawSVG(s, 0, 0)

	fills := kindsOf(d.Ops, DrawFillText)
	strokes := kindsOf(d.Ops, DrawStrokeText)
	if len(fills) != 2 || len(strokes) != 1 {
		t.Fatalf("fills=%d strokes=%d, want 2 and 1", len(fills), len(strokes))
	}
	// First text uses currentColor (0xabcdef from SetFillColor) with opacity 0.5 via DrawAlpha.
	alphas := kindsOf(d.Ops, DrawAlpha)
	if len(alphas) != 1 || alphas[0].A != 0.5 {
		t.Errorf("alphas = %+v, want single 0.5 alpha for currentColor text", alphas)
	}
	fillColors := kindsOf(d.Ops, DrawFillColor)
	// First DrawFillColor is d.SetFillColor(0xabcdef), second is the text's currentColor (0xabcdef).
	if len(fillColors) < 2 || fillColors[1].Color != RGB(0xabcdef) {
		t.Errorf("fillColors = %+v, want currentColor=0xabcdef", fillColors)
	}
	// Stroke has opacity 0.5 multiplied into #00ff00 -> 0x00ff0080 and line width 2.
	strokeColors := kindsOf(d.Ops, DrawStrokeColor)
	if len(strokeColors) != 1 || strokeColors[0].Color != 0x00ff0080 {
		t.Errorf("strokeColors = %+v, want 0x00ff0080", strokeColors)
	}
	dashes := kindsOf(d.Ops, DrawLineDash)
	if len(dashes) != 2 || dashes[0].Dash == nil || !slices.Equal(*dashes[0].Dash, LineDash{3, 1}) || dashes[1].Dash != nil {
		t.Errorf("dashes = %+v", dashes)
	}
	// Second text ("Grad", 4 chars * 20pt = 80pt wide, 20pt tall) gets an objectBoundingBox gradient mapped to its measured box.
	grads := kindsOf(d.Ops, DrawFillGradient)
	if len(grads) != 1 || grads[0].Gradient == nil {
		t.Fatalf("grads = %+v, want 1 text gradient", grads)
	}
	gm := grads[0].Gradient.Transform
	// x=10, y=60-16=44, w=80, h=20
	if gm != [6]float32{80, 0, 0, 20, 10, 44} {
		t.Errorf("text bbox gradient transform = %v, want [80 0 0 20 10 44]", gm)
	}
}

func TestSVGTextTransformsClipUseAndPaintersOrder(t *testing.T) {
	s := mustParse(t, []byte(`<svg viewBox="0 0 100 100">
		<defs>
			<clipPath id="cp"><rect x="0" y="0" width="50" height="50"/></clipPath>
			<text id="lbl" x="5" y="15" font-size="12">Used</text>
		</defs>
		<rect x="0" y="0" width="100" height="100" fill="#eee"/>
		<g clip-path="url(#cp)" transform="translate(10, 20) rotate(90)">
			<text x="0" y="10" font-size="10">Rotated</text>
		</g>
		<circle cx="50" cy="50" r="10" fill="#f00"/>
		<use href="#lbl" x="20" y="30" fill="#00f"/>
	</svg>`))

	d := NewDrawing(&svgTestEngine{charWidth: 1.0, ascent: 0.8, descent: 0.2})
	d.DrawSVG(s, 0, 0)

	// Verify painter's order: DrawRect (bg) -> DrawClip (rotated clip path) -> DrawTransform -> DrawFillText("Rotated") -> DrawRestore (clip) -> DrawEllipse -> DrawTransform -> DrawFillText("Used").
	var order []DrawKind
	for _, op := range d.Ops {
		switch op.Kind {
		case DrawRect, DrawClip, DrawTransform, DrawFillText, DrawEllipse:
			order = append(order, op.Kind)
		}
	}
	wantOrder := []DrawKind{
		DrawRect,      // background rect
		DrawClip,      // clip active (rotated clip rect becomes path lines)
		DrawTransform, // translate(10,20)*rotate(90)
		DrawFillText,  // "Rotated"
		DrawEllipse,   // circle
		DrawTransform, // use translate(20,30)
		DrawFillText,  // "Used"
	}
	if !slices.Equal(order, wantOrder) {
		t.Fatalf("painter order = %v, want %v", order, wantOrder)
	}
	texts := kindsOf(d.Ops, DrawFillText)
	if texts[0].Text != "Rotated" || texts[1].Text != "Used" {
		t.Errorf("texts = %q, %q", texts[0].Text, texts[1].Text)
	}
}

func TestSVGTextWhitespaceAndFontShorthand(t *testing.T) {
	s := mustParse(t, []byte(`<svg viewBox="0 0 200 100">
		<text x="0" y="20" style="font: italic small-caps 600 18px/1.2 'Georgia'">
			Hello
			   world
		</text>
		<text x="0" y="50" xml:space="preserve">  A  B  </text>
		<text x="0" y="80" font-size="20">Big<tspan font-size="50%">Half</tspan><tspan font-size="0">Zero</tspan></text>
	</svg>`))

	d := NewDrawing(&svgTestEngine{charWidth: 1.0, ascent: 0.8, descent: 0.2})
	d.DrawSVG(s, 0, 0)

	texts := kindsOf(d.Ops, DrawFillText)
	if len(texts) != 4 {
		t.Fatalf("texts = %+v, want 4", texts)
	}
	if texts[0].Text != "Hello world" {
		t.Errorf("collapsed = %q, want %q", texts[0].Text, "Hello world")
	}
	if !texts[0].Font.Equal(Font{Family: "Georgia", Size: 18, Weight: FontWeightSemibold, Italic: true, Variant: FontSmallCaps}) {
		t.Errorf("shorthand font = %+v", texts[0].Font)
	}
	if texts[1].Text != "  A  B  " {
		t.Errorf("preserved = %q, want %q", texts[1].Text, "  A  B  ")
	}
	if texts[2].Text != "Big" || texts[2].Font.Size != 20 {
		t.Errorf("big = %+v", texts[2])
	}
	if texts[3].Text != "Half" || texts[3].Font.Size != 10 {
		t.Errorf("half = %+v", texts[3])
	}
}

func TestSVGPreserveAspectRatio(t *testing.T) {
	t.Run("none stretches non-uniformly", func(t *testing.T) {
		s := mustParse(t, []byte(`<svg viewBox="0 0 10 10" preserveAspectRatio="none"><rect width="10" height="10"/></svg>`))
		d := &Drawing{}
		d.DrawSVGIn(s, 5, 10, 40, 20)
		scales := kindsOf(d.Ops, DrawScale)
		if len(scales) != 1 || scales[0].X != 4 || scales[0].Y != 2 {
			t.Fatalf("scales = %+v, want 4x2", scales)
		}
		tr := kindsOf(d.Ops, DrawTranslate)
		if tr[0].X != 5 || tr[0].Y != 10 {
			t.Fatalf("translate = %+v, want (5,10)", tr[0])
		}
	})
	t.Run("xMaxYMin meet aligns right-top", func(t *testing.T) {
		s := mustParse(t, []byte(`<svg viewBox="0 0 10 10" preserveAspectRatio="xMaxYMin meet"><rect width="10" height="10"/></svg>`))
		d := &Drawing{}
		d.DrawSVGIn(s, 0, 0, 50, 20)
		// scale = min(5, 2) = 2 -> drawn 20x20 inside 50x20 -> ox = 30, oy = 0.
		tr := kindsOf(d.Ops, DrawTranslate)
		if tr[0].X != 30 || tr[0].Y != 0 {
			t.Fatalf("translate = %+v, want (30,0)", tr[0])
		}
	})
	t.Run("slice covers and clips to target rect", func(t *testing.T) {
		s := mustParse(t, []byte(`<svg viewBox="0 0 10 10" preserveAspectRatio="xMidYMid slice"><rect width="10" height="10"/></svg>`))
		d := &Drawing{}
		d.DrawSVGIn(s, 4, 8, 40, 20)
		// scale = max(4, 2) = 4 -> drawn 40x40 inside (4,8,40,20) -> ox=4, oy=8-10=-2, clipped to (4,8,40,20).
		clips := kindsOf(d.Ops, DrawClip)
		if len(clips) != 1 {
			t.Fatalf("clips = %d, want 1", len(clips))
		}
		rects := kindsOf(d.Ops, DrawRect)
		if rects[0].X != 4 || rects[0].Y != 8 || rects[0].W != 40 || rects[0].H != 20 {
			t.Fatalf("clip rect = %+v, want (4,8,40,20)", rects[0])
		}
		scales := kindsOf(d.Ops, DrawScale)
		if scales[0].X != 4 || scales[0].Y != 4 {
			t.Fatalf("scale = %+v, want 4x4", scales[0])
		}
		tr := kindsOf(d.Ops, DrawTranslate)
		if tr[0].X != 4 || tr[0].Y != -2 {
			t.Fatalf("translate = %+v, want (4,-2)", tr[0])
		}
	})
}

func TestDrawSVGPreservesCurrentPathAndHandlesEdgeCases(t *testing.T) {
	s := mustParse(t, []byte(`<svg viewBox="0 0 24 24"><rect width="24" height="24" fill="red"/></svg>`))

	// Nil SVG and non-positive target sizes are safe no-ops.
	d := &Drawing{}
	d.DrawSVG(nil, 0, 0).DrawSVGIn(nil, 0, 0, 10, 10).DrawSVGIn(s, 0, 0, 0, 10).DrawSVGIn(s, 0, 0, 10, -5)
	if len(d.Ops) != 0 {
		t.Fatalf("no-op calls recorded %d ops", len(d.Ops))
	}

	// Drawing an SVG leaves a pre-existing current path intact for queries and Stroke.
	d = &Drawing{}
	d.BeginPath().Rect(0, 0, 100, 100).DrawSVGIn(s, 10, 10, 24, 24)
	if !d.IsPointInPath(50, 50, NonZero) || d.IsPointInPath(150, 50, NonZero) {
		t.Fatal("DrawSVGIn lost the caller's current path")
	}
	beforeStroke := len(d.Ops)
	d.Stroke()
	if len(d.Ops) <= beforeStroke+1 {
		t.Fatal("Stroke did not restore the caller's current path after DrawSVGIn")
	}

	// Malformed text attributes return errors.
	if _, err := ParseSVG([]byte(`<svg viewBox="0 0 10 10"><text x="notanumber">Hi</text></svg>`)); err == nil {
		t.Error("ParseSVG accepted bad text x coordinate")
	}
}
