package driver

import "testing"

func TestSplitCanvasEndsAtTheFirstBoundOp(t *testing.T) {
	d := &Drawing{}
	d.SetFillColor(RGB(0xff0000)).FillRect(0, 0, 500, 300)
	d.BeginPath().Arc(100, 50, 10, 0, 6, false).Bind(ArgX, testNode(1))
	ring := len(d.Ops) - 1
	d.Fill()
	s := SplitCanvas(d)
	if s.At != ring || s.Path {
		t.Fatalf("split %+v, want at the ring's arc (%d) without the path before it", s, ring)
	}
	if !s.Static(d) {
		t.Error("a background under a moving ring is not a static part")
	}
	for i := range d.Ops {
		op := &d.Ops[i]
		want := i < ring && (op.Kind.Paints() || op.Kind.BuildsPath() || op.Kind == DrawBeginPath)
		if got := s.Skips(i, op); got != want {
			t.Errorf("op %d (%v) skipped %v, want %v", i, op.Kind, got, want)
		}
	}
}

func TestSplitCanvasKeepsAPathUsedAfterIt(t *testing.T) {
	d := &Drawing{}
	d.BeginPath().Rect(0, 0, 50, 50).Fill()
	d.SetFillColor(RGB(0)).Bind(ArgColor, testNode(1)).Fill()
	if s := SplitCanvas(d); !s.Path {
		t.Errorf("split %+v drops the path the bound fill paints", s)
	}
	clip := &Drawing{}
	clip.BeginPath().Rect(0, 0, 50, 50).Clip().FillRect(0, 0, 100, 100)
	clip.BeginPath().Rect(0, 0, 5, 5).Bind(ArgX, testNode(1)).Fill()
	if s := SplitCanvas(clip); !s.Path {
		t.Errorf("split %+v drops the path a clip before it uses", s)
	}
}

func TestSplitCanvasWithoutBindingsHasNoStaticPart(t *testing.T) {
	d := &Drawing{}
	d.FillRect(0, 0, 10, 10)
	if s := SplitCanvas(d); s.At != len(d.Ops) || s.Static(d) {
		t.Errorf("split %+v of an unbound drawing", s)
	}
}
