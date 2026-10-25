package driver

import (
	"math"
	"testing"
	"unicode/utf8"
)

func TestArcToTouchesBothLines(t *testing.T) {
	d := &Drawing{}
	d.BeginPath().MoveTo(0, 0).ArcTo(100, 0, 100, 100, 20)
	arc := d.Ops[len(d.Ops)-1]
	if arc.Kind != DrawArc || !close(float64(arc.X), 80) || !close(float64(arc.Y), 20) || arc.R != 20 || arc.CCW {
		t.Fatalf("arc = %+v, want clockwise around (80, 20)", arc)
	}
	if !close(float64(arc.A0), -math.Pi/2) || !close(float64(arc.A1), 0) {
		t.Fatalf("arc runs from %v to %v, want -π/2 to 0", arc.A0, arc.A1)
	}
	// Without a current point it only starts a subpath; collinear points
	// draw a line.
	d = &Drawing{}
	d.BeginPath().ArcTo(10, 10, 20, 20, 5)
	if op := d.Ops[len(d.Ops)-1]; op.Kind != DrawMoveTo || op.X != 10 {
		t.Fatalf("op = %+v, want a move to the first point", op)
	}
	d.ArcTo(20, 20, 30, 30, 5)
	if op := d.Ops[len(d.Ops)-1]; op.Kind != DrawLineTo || op.X != 20 {
		t.Fatalf("op = %+v, want a line to the corner", op)
	}
}

func TestArcToReadsTheCurrentPointUnderTheTransform(t *testing.T) {
	d := &Drawing{}
	d.BeginPath().MoveTo(0, 0).Translate(50, 0).ArcTo(50, 0, 50, 100, 10)
	// (0, 0) was recorded before the translation: it is (-50, 0) now.
	arc := d.Ops[len(d.Ops)-1]
	if arc.Kind != DrawArc || !close(float64(arc.X), 40) || !close(float64(arc.Y), 10) {
		t.Fatalf("arc = %+v, want around (40, 10)", arc)
	}
}

func TestEllipseArcsAreRotatedCubics(t *testing.T) {
	d := &Drawing{}
	d.BeginPath().EllipseArc(0, 0, 20, 10, math.Pi/2, 0, math.Pi, false)
	if d.Ops[1].Kind != DrawMoveTo || !close(float64(d.Ops[1].X), 0) || !close(float64(d.Ops[1].Y), 20) {
		t.Fatalf("start = %+v, want the rotated x radius at (0, 20)", d.Ops[1])
	}
	last := d.Ops[len(d.Ops)-1]
	if last.Kind != DrawCubicTo || !close(float64(last.X), 0) || !close(float64(last.Y), -20) {
		t.Fatalf("end = %+v, want (0, -20)", last)
	}
	if n := len(d.Ops) - 2; n != 2 {
		t.Fatalf("%d curves for a half turn, want 2", n)
	}
}

func TestRoundRectCornersScaleTogether(t *testing.T) {
	d := &Drawing{}
	d.BeginPath().RoundRectCorners(0, 0, 100, 50, 80, 0, 0, 20)
	// The left side needs 80 + 20 of 50: every radius halves.
	if op := d.Ops[1]; op.Kind != DrawMoveTo || op.X != 40 {
		t.Fatalf("start = %+v, want at x 40", op)
	}
	if !d.IsPointInPath(50, 25, NonZero) || d.IsPointInPath(2, 2, NonZero) {
		t.Fatal("the rounded corner should cut the top left")
	}
}

func TestPathsKeepTheCurrentPath(t *testing.T) {
	d := &Drawing{}
	d.BeginPath().Scale(2, 2).MoveTo(1, 1).LineTo(5, 1).ResetTransform().
		FillRect(0, 0, 10, 10).LineTo(10, 10).Stroke()
	// The current path is built again under the scale it was built in.
	var set []DrawOp
	for _, op := range d.Ops {
		if op.Kind == DrawSetTransform {
			set = append(set, op)
		}
	}
	// The app's reset, then the scale the path was built in.
	if len(set) != 2 || set[1].X1 != 2 || set[1].Y2 != 2 {
		t.Fatalf("set transforms = %+v, want the reset and the scale", set)
	}
	if !d.IsPointInStroke(10, 2) || !d.IsPointInStroke(10, 6) {
		t.Fatal("the stroke should run from (2, 2) to (10, 2) and on to (10, 10)")
	}
}

func TestPointInPathRulesAndStrokes(t *testing.T) {
	d := &Drawing{}
	d.BeginPath().Rect(0, 0, 100, 100).Rect(25, 25, 50, 50)
	if !d.IsPointInPath(50, 50, NonZero) || d.IsPointInPath(50, 50, EvenOdd) {
		t.Fatal("the inner square is a hole by the even-odd rule only")
	}
	if !d.IsPointInPath(10, 10, EvenOdd) || d.IsPointInPath(150, 50, NonZero) {
		t.Fatal("wrong outside the hole")
	}
	d.SetLineWidth(10).SetLineCap(CapButt).BeginPath().MoveTo(0, 200).LineTo(100, 200)
	if !d.IsPointInStroke(50, 204) || d.IsPointInStroke(50, 206) || d.IsPointInStroke(-3, 200) {
		t.Fatal("a butt stroke is 10 wide and ends at its points")
	}
	d.SetLineCap(CapSquare)
	if !d.IsPointInStroke(-3, 200) {
		t.Fatal("a square cap reaches half the width past the end")
	}
	// Strokes follow the transform: a scaled line width is wider.
	d.Scale(2, 2).SetLineWidth(10)
	if !d.IsPointInStroke(50, 209) {
		t.Fatal("the scaled stroke should be 20 wide")
	}
	p := MustParsePath("M 0 0 h 10 v 10 h -10 z")
	if !d.PathContains(p, 15, 15, NonZero) || d.PathContains(p, 25, 15, NonZero) {
		t.Fatal("the path should be filled under the scale")
	}
}

func TestTextsArePlacedByAlignAndBaseline(t *testing.T) {
	d := &Drawing{}
	font := Font{Size: 10}
	d.SetTextAlign(TextAlignCenter).SetTextBaseline(BaselineTop).FillText("abcd", 100, 50, font)
	op := d.Ops[len(d.Ops)-1]
	// Approximated: 4 characters of 6 points, an ascent of 8.
	if op.X != 100 || op.X1 != -12 || op.Y1 != 8 {
		t.Fatalf("text = %+v, want 12 left of 100 and 8 down", op)
	}
	d.Save().SetTextAlign(TextAlignEnd).Restore().FillText("ab", 0, 0, font)
	if op := d.Ops[len(d.Ops)-1]; op.X1 != -6 {
		t.Fatalf("restore lost the alignment: %+v", op)
	}
	if w := d.MeasureText("abc", font); w != 18 {
		t.Fatalf("width = %v, want 18", w)
	}
}

type testNode uint32

func (n testNode) AnimatedNodeID() uint32 { return uint32(n) }

func TestBoundArgumentsPlayWithTheirValues(t *testing.T) {
	a := NewAnimator(func(Event) {})
	a.Apply([]AnimationOp{{Kind: AnimCreateValue, Node: 1, Value: 30},
		{Kind: AnimCreateValue, Node: 2},
		{Kind: AnimCreateInterpolate, Node: 3, Input: 2,
			Interpolation: &Interpolation{Input: []float64{0, 1}, Colors: []Color{RGB(0xff0000), RGB(0x0000ff)}}}})
	d := &Drawing{}
	d.SetFillColor(RGB(0)).Bind(ArgColor, testNode(3)).BeginPath().Arc(50, 50, 10, 0, 6, false).Bind(ArgR, testNode(1)).Fill()
	a.BindCanvas(7, d.Slots)
	reported := 0
	a.ChangedCanvases(func(NodeID) { reported++ })
	ops := a.CanvasOps(7, d)
	if reported != 1 || ops[2].R != 30 || ops[0].Color != RGB(0xff0000) {
		t.Fatalf("ops = %+v after %d reports", ops, reported)
	}
	if d.Ops[2].R != 10 {
		t.Fatal("the recording changed")
	}
	a.Apply([]AnimationOp{{Kind: AnimSet, Node: 2, Value: 1}})
	a.ChangedCanvases(func(NodeID) { reported++ })
	if ops := a.CanvasOps(7, d); reported != 2 || ops[0].Color != RGB(0x0000ff) {
		t.Fatalf("color = %v after %d reports", ops[0].Color, reported)
	}
	a.ChangedCanvases(func(NodeID) { reported++ })
	if reported != 2 {
		t.Fatal("an unchanged canvas was reported")
	}
}

func TestLineDashesRepeatOddLists(t *testing.T) {
	d := &Drawing{}
	d.SetLineDash(4, 2, 1)
	if dash := *d.Ops[0].Dash; len(dash) != 6 || dash[3] != 4 {
		t.Fatalf("dash = %v", dash)
	}
	if d.SetLineDash(1, -1); len(d.Ops) != 1 {
		t.Fatal("a negative length should drop the call")
	}
}

func TestCompositeOps(t *testing.T) {
	if CompositeMultiply.Name() != "multiply" || CompositeDestinationAtop.Name() != "destination-atop" {
		t.Fatal("wrong names")
	}
	if b, ok := CompositeLuminosity.Blend(); !ok || b != BlendLuminosity {
		t.Fatal("luminosity is a blend")
	}
	if !CompositeCopy.Unbounded() || CompositeSourceAtop.Unbounded() {
		t.Fatal("wrong bounds")
	}
}

func close(a, b float64) bool { return math.Abs(a-b) < 1e-3 }

func TestCutRestCutsTheLastLine(t *testing.T) {
	text := "one two three ￼ four five six ￼"
	at := AttributedText{Text: text, MaxLines: 2, Ellipsize: EllipsizeMiddle,
		Spans:       []TextSpan{{Start: 0, End: len(text), Style: TextStyle{Color: RGB(0x123456)}}},
		Attachments: []TextAttachment{{Offset: 16}, {Offset: len(text) - 3}}}
	start := len("one two three ")
	fits := func(c *AttributedText) bool { return utf8.RuneCountInString(c.Text) <= 9 }
	cut := at.CutRest(start, fits)
	// Four characters on each side of the ellipsis, as the middle keeps.
	want := "one two three \n￼ fo…ix ￼"
	if cut.Text != want {
		t.Fatalf("cut = %q, want %q", cut.Text, want)
	}
	if cut.MaxLines != 2 || cut.Ellipsize != EllipsizeClip || len(cut.Origin) != 2 || cut.Origin[1] != 1 {
		t.Fatalf("cut = %+v", cut)
	}
	if a := cut.Attachments[1]; cut.Text[a.Offset:a.Offset+3] != "￼" {
		t.Fatalf("the last attachment moved to %d", a.Offset)
	}
	if len(cut.Spans) != 2 || cut.Spans[1].End != len(cut.Text) {
		t.Fatalf("spans = %+v", cut.Spans)
	}
}
