package driver

import "testing"

// moved returns d's ops with op i's argument arg set to v, as a bound
// argument's new value plays it.
func moved(d *Drawing, i int, arg DrawArg, v float64) []DrawOp {
	ops := append([]DrawOp(nil), d.Ops...)
	ops[i] = DrawSlot{Op: i, Arg: arg}.Set(ops[i], v, 0)
	return ops
}

func TestCanvasDamageCoversWhatAnArgumentMoves(t *testing.T) {
	d := &Drawing{}
	d.SetFillColor(RGB(0xff0000)).FillRect(0, 0, 500, 300) // a static background
	d.BeginPath().Arc(100, 50, 10, 0, 6, false)
	ring := len(d.Ops) - 1
	d.Fill()
	got, ok := CanvasDamage(d.Ops, moved(d, ring, ArgX, 140))
	if !ok {
		t.Fatal("a moving circle redraws the whole canvas")
	}
	if want := (Bounds{Origin: Point{X: 90, Y: 40}, Size: Size{Width: 60, Height: 20}}); got != want {
		t.Errorf("damage %v, want the circle's old and new places %v", got, want)
	}
	if got, ok := CanvasDamage(d.Ops, d.Ops); !ok || got != (Bounds{}) {
		t.Errorf("an unchanged drawing damages %v", got)
	}
}

func TestCanvasDamageFollowsStateItPaintsWith(t *testing.T) {
	d := &Drawing{}
	d.Translate(10, 0)
	shift := len(d.Ops) - 1
	d.SetLineWidth(4).BeginPath().Rect(0, 0, 20, 20).Stroke()
	got, ok := CanvasDamage(d.Ops, moved(d, shift, ArgX, 30))
	if !ok || got.Origin.X > 8 || got.Origin.X+got.Size.Width < 52 {
		t.Errorf("a moved transform damages %v (%v), want both strokes", got, ok)
	}
	d = &Drawing{}
	d.SetFillColor(RGB(0)).Bind(ArgColor, testNode(1)).FillRect(5, 5, 10, 10)
	ops := append([]DrawOp(nil), d.Ops...)
	ops[0].Color = RGB(0xffffff)
	if got, ok := CanvasDamage(d.Ops, ops); !ok || got != (Bounds{Origin: Point{X: 5, Y: 5}, Size: Size{Width: 10, Height: 10}}) {
		t.Errorf("a new fill color damages %v (%v), want the rectangle it fills", got, ok)
	}
}

func TestCanvasDamageRedrawsAllWhereItCannotTell(t *testing.T) {
	clip := &Drawing{}
	clip.BeginPath().Rect(0, 0, 10, 10)
	i := len(clip.Ops) - 1
	clip.Clip().FillRect(0, 0, 100, 100)
	if _, ok := CanvasDamage(clip.Ops, moved(clip, i, ArgW, 50)); ok {
		t.Error("a moved clip does not redraw all")
	}
	comp := &Drawing{}
	comp.SetComposite(CompositeCopy).BeginPath().Rect(0, 0, 10, 10)
	i = len(comp.Ops) - 1
	comp.Fill()
	if _, ok := CanvasDamage(comp.Ops, moved(comp, i, ArgX, 20)); ok {
		t.Error("painting under copy does not redraw all")
	}
}

func TestCanvasDamageWidensForShadowsAndText(t *testing.T) {
	d := &Drawing{}
	d.SetShadow(RGB(0), 5, 5, 4).BeginPath().Rect(0, 0, 10, 10)
	i := len(d.Ops) - 1
	d.Fill()
	got, _ := CanvasDamage(d.Ops, moved(d, i, ArgX, 1))
	if got.Origin.X+got.Size.Width < 11+5+8 {
		t.Errorf("a shadowed rectangle damages %v, which misses its shadow", got)
	}
	d = &Drawing{}
	d.FillText("abc", 10, 20, Font{Size: 10})
	got, _ = CanvasDamage(d.Ops, moved(d, 0, ArgX, 30))
	if got.Origin.X > -5 || got.Origin.X+got.Size.Width < 30+45 {
		t.Errorf("a moved text damages %v", got)
	}
}
