package core

import (
	"math"
	"slices"
	"testing"
)

// ops parses doc and returns the drawing operations it records.
func svgOps(t *testing.T, doc string) []DrawOp {
	t.Helper()
	s, err := ParseSVG([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return s.ops
}

func opKinds(ops []DrawOp) []DrawKind {
	out := make([]DrawKind, len(ops))
	for i, op := range ops {
		out[i] = op.Kind
	}
	return out
}

func TestSVGPathCommands(t *testing.T) {
	ops := svgOps(t, `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<path d="M2 2h4v4H2z"/>
	</svg>`)
	want := []DrawKind{DrawBeginPath, DrawMoveTo, DrawLineTo, DrawLineTo, DrawLineTo, DrawClosePath,
		DrawStrokeColor, DrawLineWidth, DrawLineCap, DrawLineJoin, DrawStroke}
	if got := opKinds(ops); !slices.Equal(got, want) {
		t.Fatalf("ops = %v, want %v", got, want)
	}
	points := [][2]float32{{2, 2}, {6, 2}, {6, 6}, {2, 6}}
	for i, p := range points {
		if op := ops[1+i]; op.X != p[0] || op.Y != p[1] {
			t.Errorf("point %d = %g,%g, want %g,%g", i, op.X, op.Y, p[0], p[1])
		}
	}
	if ops[6].Color != 0 || ops[7].W != 2 || ops[8].Cap != CapRound || ops[9].Join != JoinRound {
		t.Errorf("stroke = %+v %+v %+v %+v", ops[6], ops[7], ops[8], ops[9])
	}
}

func TestSVGPathCurves(t *testing.T) {
	ops := svgOps(t, `<svg viewBox="0 0 24 24"><path d="M0 0 C1 1 2 2 3 3 S5 5 6 6 Q7 7 8 8 T10 10"/></svg>`)
	if got := opKinds(ops); !slices.Equal(got, []DrawKind{DrawBeginPath, DrawMoveTo, DrawCubicTo, DrawCubicTo, DrawQuadTo, DrawQuadTo, DrawFillColor, DrawFill}) {
		t.Fatalf("ops = %v", got)
	}
	// S reflects C's second control point about (3, 3).
	if op := ops[3]; op.X1 != 4 || op.Y1 != 4 || op.X2 != 5 || op.Y2 != 5 || op.X != 6 || op.Y != 6 {
		t.Errorf("smooth cubic = %+v", op)
	}
	// T reflects Q's control point about (8, 8).
	if op := ops[5]; op.X1 != 9 || op.Y1 != 9 || op.X != 10 || op.Y != 10 {
		t.Errorf("smooth quad = %+v", op)
	}
}

func TestSVGPathArcs(t *testing.T) {
	// A half circle of radius 5 from (0, 0) to (10, 0).
	ops := svgOps(t, `<svg viewBox="0 0 24 24"><path d="M0 0 A5 5 0 0 1 10 0"/></svg>`)
	if got := opKinds(ops); !slices.Equal(got, []DrawKind{DrawBeginPath, DrawMoveTo, DrawCubicTo, DrawCubicTo, DrawFillColor, DrawFill}) {
		t.Fatalf("ops = %v", got)
	}
	last := ops[3]
	if math.Abs(float64(last.X-10)) > 0.01 || math.Abs(float64(last.Y)) > 0.01 {
		t.Fatalf("arc ends at %g,%g, want 10,0", last.X, last.Y)
	}
	// The sweep flag picks the other half: 1 goes over the top, 0 under it.
	ops = svgOps(t, `<svg viewBox="0 0 24 24"><path d="M0 0 A5 5 0 0 0 10 0"/></svg>`)
	below := false
	for _, op := range ops {
		if op.Kind == DrawCubicTo && (op.Y1 > 0 || op.Y2 > 0) {
			below = true
		}
	}
	if !below {
		t.Fatal("sweep 0 should pass below the chord")
	}
	// Rotated, relative arcs and flags without separators, as galaxy.svg has.
	ops = svgOps(t, `<svg viewBox="0 0 24 24"><path d="M16.005 15.108a5.041 6.52 28.25 00-8.008-6.217"/></svg>`)
	var lastCubic DrawOp
	cubics := 0
	for _, op := range ops {
		if op.Kind == DrawCubicTo {
			cubics++
			lastCubic = op
		}
	}
	if cubics < 2 {
		t.Fatalf("arc drew %d cubics", cubics)
	}
	if math.Abs(float64(lastCubic.X-7.997)) > 0.01 || math.Abs(float64(lastCubic.Y-8.891)) > 0.01 {
		t.Errorf("relative arc ends at %g,%g, want 7.997,8.891", lastCubic.X, lastCubic.Y)
	}
	for _, op := range ops {
		if math.IsNaN(float64(op.X)) || math.IsInf(float64(op.X), 0) {
			t.Fatalf("non-finite op %+v", op)
		}
	}
}

func TestSVGShapes(t *testing.T) {
	ops := svgOps(t, `<svg viewBox="0 0 24 24">
		<circle cx="4" cy="4" r="2"/>
		<ellipse cx="10" cy="4" rx="3" ry="2"/>
		<line x1="0" y1="10" x2="10" y2="10"/>
		<polyline points="0,12 4,16 8,12"/>
		<polygon points="12,12 16,16 20,12"/>
		<rect x="1" y="18" width="6" height="4" rx="1"/>
	</svg>`)
	want := []DrawKind{
		DrawBeginPath, DrawEllipse, DrawFillColor, DrawFill,
		DrawBeginPath, DrawEllipse, DrawFillColor, DrawFill,
		DrawBeginPath, DrawMoveTo, DrawLineTo, DrawFillColor, DrawFill,
		DrawBeginPath, DrawMoveTo, DrawLineTo, DrawLineTo, DrawFillColor, DrawFill,
		DrawBeginPath, DrawMoveTo, DrawLineTo, DrawLineTo, DrawClosePath, DrawFillColor, DrawFill,
		DrawBeginPath, DrawRoundRect, DrawFillColor, DrawFill,
	}
	if got := opKinds(ops); !slices.Equal(got, want) {
		t.Fatalf("ops = %v, want %v", got, want)
	}
	if op := ops[1]; op.R != 2 || op.RY != 2 {
		t.Errorf("circle = %+v", op)
	}
	if op := ops[27]; op.Kind != DrawRoundRect || op.R != 1 {
		t.Errorf("rect = %+v", op)
	}
}

func TestSVGDrawFitsCentersAndColors(t *testing.T) {
	s := MustParseSVG([]byte(`<svg width="24" height="24" viewBox="0 0 24 24"><rect width="24" height="24" fill="currentColor"/></svg>`))
	if w, h := s.Size(); w != 24 || h != 24 {
		t.Fatalf("size = %gx%g", w, h)
	}
	d := &Drawing{}
	s.Draw(d, Size{Width: 48, Height: 24}, RGB(0x123456))
	want := []DrawOp{
		{Kind: DrawSave},
		{Kind: DrawTranslate, X: 12, Y: 0},
		{Kind: DrawScale, X: 1, Y: 1},
		{Kind: DrawTranslate, X: 0, Y: 0},
		{Kind: DrawBeginPath},
		{Kind: DrawRect, W: 24, H: 24},
		{Kind: DrawFillColor, Color: RGB(0x123456)},
		{Kind: DrawFill},
		{Kind: DrawRestore},
	}
	if len(d.Ops) != len(want) {
		t.Fatalf("ops = %+v", d.Ops)
	}
	for i := range want {
		if d.Ops[i] != want[i] {
			t.Errorf("op %d = %+v, want %+v", i, d.Ops[i], want[i])
		}
	}
	// A zero color paints black, not the transparent zero value.
	d = &Drawing{}
	s.Draw(d, Size{Width: 24, Height: 24}, 0)
	if d.Ops[6].Color != RGB(0x000000) {
		t.Errorf("zero color = %#x", d.Ops[6].Color)
	}
}

func TestSVGViewBoxAndStyles(t *testing.T) {
	s := MustParseSVG([]byte(`<svg viewBox="2 4 10 20" fill="none">
		<g stroke="currentColor" stroke-width="3" stroke-linecap="square" stroke-linejoin="bevel">
			<path d="M0 0L1 1"/>
		</g>
		<path d="M0 0L1 1" fill="#f00" stroke="none"/>
	</svg>`))
	if w, h := s.Size(); w != 10 || h != 20 {
		t.Fatalf("size from viewBox = %gx%g", w, h)
	}
	d := &Drawing{}
	s.Draw(d, Size{Width: 20, Height: 20}, RGB(0x00ff00))
	// The group strokes in the icon color, with its width, cap and join.
	var cap LineCap
	var join LineJoin
	var width float32
	var strokes, fills int
	for _, op := range d.Ops {
		switch op.Kind {
		case DrawStroke:
			strokes++
		case DrawFill:
			fills++
		case DrawLineWidth:
			width = op.W
		case DrawLineCap:
			cap = op.Cap
		case DrawLineJoin:
			join = op.Join
		case DrawStrokeColor:
			if op.Color != RGB(0x00ff00) {
				t.Errorf("stroke color = %#x", op.Color)
			}
		}
	}
	if strokes != 1 || fills != 1 || width != 3 || cap != CapSquare || join != JoinBevel {
		t.Errorf("strokes %d fills %d width %g cap %v join %v", strokes, fills, width, cap, join)
	}
	// The viewBox's origin shifts the drawing.
	if op := d.Ops[3]; op.Kind != DrawTranslate || op.X != -2 || op.Y != -4 {
		t.Errorf("viewBox translate = %+v", op)
	}
}

func TestSVGRejectsBadDocuments(t *testing.T) {
	for _, doc := range []string{
		``,
		`<div/>`,
		`<svg/>`,
		`<svg viewBox="0 0 24 24"><path d="M0 0 L"/></svg>`,
		`<svg viewBox="0 0 24 24"><rect width="x"/></svg>`,
	} {
		if _, err := ParseSVG([]byte(doc)); err == nil {
			t.Errorf("ParseSVG(%q) succeeded", doc)
		}
	}
}

func TestIconInheritsColorAndSize(t *testing.T) {
	s := MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="8"/></svg>`))
	var tint *Signal[Color]
	h := newHost(t, Size{200, 100}, func(cx *Scope) Node {
		tint = NewSignal(cx, RGB(0x112233))
		return View{Style: Style{AlignItems: AlignStart}, Children: []Node{
			View{
				StyleFunc: func() Style {
					return Style{Color: tint.Get(), FontSize: 20, AlignItems: AlignStart}
				},
				Children: []Node{
					Icon{SVG: s, TestID: "icon"},
					Icon{SVG: s, TestID: "wide", Style: Style{Width: Pt(48), Height: Pt(24)}},
				},
			},
		}}
	})
	drawing := func(b *MountBatch, id NodeID) *Drawing {
		for _, it := range b.Items {
			if it.Kind == ItemCanvas && it.ID == id {
				return it.Drawing
			}
		}
		return nil
	}
	b := h.commit()
	icon := h.find("icon")
	if got := icon.frame.Size; got != (Size{Width: 20, Height: 20}) {
		t.Fatalf("icon size = %v, want its font size", got)
	}
	d := drawing(b, icon.id)
	if d == nil {
		t.Fatal("the icon did not draw")
	}
	found := false
	for _, op := range d.Ops {
		if op.Kind == DrawStrokeColor {
			found = true
			if op.Color != RGB(0x112233) {
				t.Errorf("stroke color = %#x, want the inherited text color", op.Color)
			}
		}
	}
	if !found {
		t.Fatal("no stroke color")
	}
	wide := h.find("wide")
	d = drawing(b, wide.id)
	if d == nil {
		t.Fatal("the wide icon did not draw")
	}
	// 24 units in a 24-point-high box, centered in the 48-point width.
	if op := d.Ops[1]; op.Kind != DrawTranslate || op.X != 12 || op.Y != 0 {
		t.Errorf("wide translate = %+v", op)
	}
	if op := d.Ops[2]; op.Kind != DrawScale || op.X != 1 || op.Y != 1 {
		t.Errorf("wide scale = %+v", op)
	}

	tint.Set(RGB(0xff8800))
	b = h.commit()
	d = drawing(b, icon.id)
	if d == nil {
		t.Fatal("a color change did not redraw")
	}
	for _, op := range d.Ops {
		if op.Kind == DrawStrokeColor && op.Color != RGB(0xff8800) {
			t.Errorf("stroke color after change = %#x", op.Color)
		}
	}
}

func TestIconChangesDocument(t *testing.T) {
	square := MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><rect width="24" height="24"/></svg>`))
	wide := MustParseSVG([]byte(`<svg viewBox="0 0 48 24"><rect width="48" height="24"/></svg>`))
	var doc *Signal[*SVG]
	h := newHost(t, Size{200, 100}, func(cx *Scope) Node {
		doc = NewSignal(cx, square)
		return View{Style: Style{AlignItems: AlignStart}, Children: []Node{
			Icon{SVGFunc: doc.Get, TestID: "icon"},
		}}
	})
	b := h.commit()
	icon := h.find("icon")
	if got, want := icon.frame.Size, (Size{Width: DefaultFontSize, Height: DefaultFontSize}); got != want {
		t.Fatalf("icon = %v, want its font size %v", got, want)
	}
	doc.Set(wide)
	b = h.commit()
	if got, want := icon.frame.Size, (Size{Width: DefaultFontSize * 2, Height: DefaultFontSize}); got != want {
		t.Fatalf("after the document changed, icon = %v, want %v", got, want)
	}
	found := false
	for _, it := range b.Items {
		if it.Kind == ItemCanvas && it.ID == icon.id {
			found = true
		}
	}
	if !found {
		t.Fatal("a new document did not draw")
	}
}
