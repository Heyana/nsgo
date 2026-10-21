package core

import (
	"strconv"
	"testing"
)

// fixedText measures every rune as 10 by 20 points, wrapping at maxWidth.
type fixedText struct{}

func (fixedText) Measure(text string, font Font, maxWidth Points) Size {
	w := Points(len([]rune(text)) * 10)
	lines := Points(1)
	if !maxWidth.IsAuto() && w > maxWidth && maxWidth > 0 {
		perLine := max(int(maxWidth/10), 1)
		n := len([]rune(text))
		lines = Points((n + perLine - 1) / perLine)
		w = Points(perLine * 10)
	}
	return Size{w, 20 * lines}
}

func frames(b *MountBatch) map[NodeID]Bounds {
	out := map[NodeID]Bounds{}
	for _, it := range b.Items {
		if it.Kind == ItemFrame {
			out[it.ID] = it.Frame
		}
	}
	return out
}

func TestSignalsRunEffects(t *testing.T) {
	cx := NewScope()
	count := NewSignal(cx, 1)
	var seen []int
	dispose := cx.Effect(func() { seen = append(seen, count.Get()) })
	count.Set(2)
	count.Set(2)
	cx.Batch(func() {
		count.Set(3)
		count.Set(4)
	})
	dispose()
	count.Set(5)
	if got, want := seen, []int{1, 2, 4}; len(got) != len(want) || got[0] != 1 || got[1] != 2 || got[2] != 4 {
		t.Fatalf("effect runs = %v, want %v", got, want)
	}
}

func TestColumnStretchesAndPads(t *testing.T) {
	cx := NewScope()
	tree := NewTree(cx, fixedText{})
	tree.Mount(View(
		Text("hello"),
		Input().Style(Style{Height: 40}),
	).Style(Style{Padding: All(10), Gap: 5}))
	tree.Resize(Size{200, 300})
	f := frames(tree.Commit())

	if got, want := f[1], (Bounds{Size: Size{200, 300}}); got != want {
		t.Errorf("root = %v, want %v", got, want)
	}
	if got, want := f[2], (Bounds{Point{10, 10}, Size{180, 20}}); got != want {
		t.Errorf("text = %v, want %v", got, want)
	}
	if got, want := f[3], (Bounds{Point{10, 35}, Size{180, 40}}); got != want {
		t.Errorf("input = %v, want %v", got, want)
	}
}

func TestRowGrowsAndCenters(t *testing.T) {
	cx := NewScope()
	tree := NewTree(cx, fixedText{})
	tree.Mount(View(
		Text("ab"),
		View().Style(Style{FlexGrow: 1, Height: 10}),
	).Style(Style{Direction: Row, Width: 100, AlignItems: AlignCenter}))
	tree.Resize(Size{100, 50})
	f := frames(tree.Commit())

	if got, want := f[2], (Bounds{Point{0, 15}, Size{20, 20}}); got != want {
		t.Errorf("text = %v, want %v", got, want)
	}
	if got, want := f[3], (Bounds{Point{20, 20}, Size{80, 10}}); got != want {
		t.Errorf("grown view = %v, want %v", got, want)
	}
}

func TestCommitSendsOnlyChanges(t *testing.T) {
	cx := NewScope()
	tree := NewTree(cx, fixedText{})
	count := NewSignal(cx, 0)
	tree.Mount(View(
		TextFn(func() string { return strconv.Itoa(count.Get()) }),
		View().OnPress(func() { count.Update(func(n int) int { return n + 1 }) }).Style(Style{Height: 30}),
	))
	tree.Resize(Size{100, 100})
	first := tree.Commit()
	if len(first.Items) == 0 || first.Items[0].Kind != ItemCreate {
		t.Fatalf("first batch = %v", first.Items)
	}
	if tree.Commit() != nil {
		t.Fatal("an unchanged tree commits again")
	}

	if got := tree.HitTest(Point{50, 40}); got != 3 {
		t.Fatalf("hit test = %d, want 3", got)
	}
	tree.Press(3)
	batch := tree.Commit()
	var props, frameItems int
	for _, it := range batch.Items {
		switch it.Kind {
		case ItemProps:
			props++
			if it.ID != 2 || it.Props.Text != "1" {
				t.Errorf("props item = %+v", it)
			}
		case ItemFrame:
			frameItems++
		default:
			t.Errorf("unexpected item %+v", it)
		}
	}
	if props != 1 || frameItems != 0 {
		t.Errorf("props = %d, frames = %d; want 1 and 0 (same width)", props, frameItems)
	}
}

func TestInputEditIsNotMountedBack(t *testing.T) {
	cx := NewScope()
	tree := NewTree(cx, fixedText{})
	name := NewSignal(cx, "")
	tree.Mount(View(Input().Value(name.Get).OnChange(name.Set)))
	tree.Resize(Size{100, 100})
	tree.Commit()

	tree.Change(2, "Go")
	if name.Get() != "Go" {
		t.Fatalf("signal = %q", name.Get())
	}
	if b := tree.Commit(); b != nil {
		t.Fatalf("edit mounted back: %+v", b.Items)
	}
}
