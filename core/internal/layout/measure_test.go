package layout

import "testing"

// Taffy's hand-written measure and caching tests (tests/hand_written).

// fixedContent is TestNodeContext::fixed.
func fixedContent(w, h float32) func(Size[Opt], Size[AvailableSpace]) Size[float32] {
	return func(Size[Opt], Size[AvailableSpace]) Size[float32] { return Size[float32]{w, h} }
}

// aspectContent is TestNodeContext::aspect_ratio.
func aspectContent(width, heightRatio float32) func(Size[Opt], Size[AvailableSpace]) Size[float32] {
	return func(known Size[Opt], _ Size[AvailableSpace]) Size[float32] {
		w := known.Width.Or(width)
		return Size[float32]{w, known.Height.Or(w * heightRatio)}
	}
}

// textContent is TestNodeContext::ahem_text, horizontal.
func textContent(text string) func(Size[Opt], Size[AvailableSpace]) Size[float32] {
	return (&ahemText{text: text}).measure
}

func leafWith(s Style, content func(Size[Opt], Size[AvailableSpace]) Size[float32]) *testNode {
	n := newLeaf(s)
	n.content = content
	return n
}

func wantSize(t *testing.T, what string, n *testNode, w, h float32) {
	t.Helper()
	if got := n.layout().Size; got != (Size[float32]{w, h}) {
		t.Errorf("%s size %v, want %vx%v", what, got, w, h)
	}
}

func TestMeasure(t *testing.T) {
	hundred := fixedContent(100, 100)

	t.Run("measure root", func(t *testing.T) {
		node := leafWith(Style{}, hundred)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "node", node, 100, 100)
	})
	t.Run("measure child", func(t *testing.T) {
		child := leafWith(Style{}, hundred)
		node := newNode(Style{}, child)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "node", node, 100, 100)
		wantSize(t, "child", child, 100, 100)
	})
	t.Run("measure child constraint", func(t *testing.T) {
		child := leafWith(Style{}, hundred)
		node := newNode(Style{Size: Size[Length]{Width: Px(50)}}, child)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "node", node, 50, 100)
		wantSize(t, "child", child, 100, 100)
	})
	t.Run("measure child constraint padding parent", func(t *testing.T) {
		child := leafWith(Style{}, hundred)
		node := newNode(Style{
			Size:    Size[Length]{Width: Px(50)},
			Padding: Rect[Length]{Px(10), Px(10), Px(10), Px(10)},
		}, child)
		computeLayout(node, maxContentSpaces)
		if l := node.layout(); l.Location != (Point[float32]{}) {
			t.Errorf("node location %v", l.Location)
		}
		wantSize(t, "node", node, 50, 120)
		if l := child.layout(); l.Location != (Point[float32]{10, 10}) {
			t.Errorf("child location %v", l.Location)
		}
		wantSize(t, "child", child, 100, 100)
	})
	t.Run("measure child with flex grow", func(t *testing.T) {
		child0 := newLeaf(Style{Size: pxSize(50, 50)})
		child1 := leafWith(Style{FlexGrow: 1}, fixedContent(50, 50))
		node := newNode(Style{Size: Size[Length]{Width: Px(100)}}, child0, child1)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "child1", child1, 50, 50)
	})
	t.Run("measure child with flex shrink", func(t *testing.T) {
		child0 := newLeaf(Style{Size: pxSize(50, 50), FlexShrink: Some(0)})
		child1 := leafWith(Style{}, fixedContent(100, 50))
		node := newNode(Style{Size: Size[Length]{Width: Px(100)}}, child0, child1)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "child1", child1, 100, 50)
	})
	t.Run("remeasure child after growing", func(t *testing.T) {
		child0 := newLeaf(Style{Size: pxSize(50, 50)})
		child1 := leafWith(Style{FlexGrow: 1}, aspectContent(10, 2))
		node := newNode(Style{Size: Size[Length]{Width: Px(100)}, AlignItems: alignOf(AlignStart)}, child0, child1)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "child1", child1, 50, 100)
	})
	t.Run("remeasure child after shrinking", func(t *testing.T) {
		child0 := newLeaf(Style{Size: pxSize(50, 50), FlexShrink: Some(0)})
		child1 := leafWith(Style{}, aspectContent(100, 2))
		node := newNode(Style{Size: Size[Length]{Width: Px(100)}, AlignItems: alignOf(AlignStart)}, child0, child1)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "child1", child1, 100, 200)
	})
	t.Run("remeasure child after stretching", func(t *testing.T) {
		child := leafWith(Style{}, func(known Size[Opt], _ Size[AvailableSpace]) Size[float32] {
			h := known.Height.Or(50)
			return Size[float32]{known.Width.Or(h), h}
		})
		// Taffy's custom measure function here skips test_measure_function's
		// count and early return; neither affects the result.
		node := newNode(Style{Size: pxSize(100, 100)}, child)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "child", child, 100, 100)
	})
	t.Run("width overrides measure", func(t *testing.T) {
		child := leafWith(Style{Size: Size[Length]{Width: Px(50)}}, hundred)
		node := newNode(Style{}, child)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "child", child, 50, 100)
	})
	t.Run("height overrides measure", func(t *testing.T) {
		child := leafWith(Style{Size: Size[Length]{Height: Px(50)}}, hundred)
		node := newNode(Style{}, child)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "child", child, 100, 50)
	})
	t.Run("flex basis overrides measure", func(t *testing.T) {
		child0 := newLeaf(Style{FlexBasis: Px(50), FlexGrow: 1})
		child1 := leafWith(Style{FlexBasis: Px(50), FlexGrow: 1}, hundred)
		node := newNode(Style{Size: pxSize(200, 100)}, child0, child1)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "child0", child0, 100, 100)
		wantSize(t, "child1", child1, 100, 100)
	})
	t.Run("stretch overrides measure", func(t *testing.T) {
		child := leafWith(Style{}, fixedContent(50, 50))
		node := newNode(Style{Size: pxSize(100, 100)}, child)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "child", child, 50, 100)
	})
	t.Run("explicit min main size skips min-content measurement", func(t *testing.T) {
		// Taffy asserts on the leaf's LayoutInput; the leaf's Measure sees the
		// same available width, as the leaf has no size of its own.
		child := leafWith(Style{MinSize: Size[Length]{Width: Px(10)}}, fixedContent(10, 10))
		child.onMeasure = func(_ Size[Opt], available Size[AvailableSpace]) {
			if available.Width.isMinContent() {
				t.Error("measured under a min-content constraint")
			}
		}
		root := newNode(Style{}, child)
		computeLayout(root, maxContentSpaces)
	})
	t.Run("measure absolute child", func(t *testing.T) {
		child := leafWith(Style{Position: PositionAbsolute}, fixedContent(50, 50))
		node := newNode(Style{Size: pxSize(100, 100)}, child)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "child", child, 50, 50)
	})
	t.Run("ignore invalid measure", func(t *testing.T) {
		child := newLeaf(Style{FlexGrow: 1})
		node := newNode(Style{Size: pxSize(100, 100)}, child)
		computeLayout(node, maxContentSpaces)
		wantSize(t, "child", child, 100, 100)
	})
}

func TestCaching(t *testing.T) {
	const text = "HH​HH​HH​HH"
	grid := func(s Style) Style {
		s.Display = DisplayGrid
		return s
	}
	wantCount := func(t *testing.T, leaf *testNode, want int) {
		t.Helper()
		if leaf.count != want {
			t.Errorf("measured %d times, want %d", leaf.count, want)
		}
	}

	t.Run("measure count flexbox", func(t *testing.T) {
		leaf := leafWith(Style{}, fixedContent(50, 50))
		node := newNode(Style{}, leaf)
		for range 100 {
			node = newNode(Style{}, node)
		}
		computeLayout(node, maxContentSpaces)
		wantCount(t, leaf, 7)
	})
	t.Run("measure count grid", func(t *testing.T) {
		leaf := leafWith(grid(Style{}), fixedContent(50, 50))
		node := newNode(Style{}, leaf)
		for range 100 {
			node = newNode(Style{}, node)
		}
		computeLayout(node, maxContentSpaces)
		wantCount(t, leaf, 7)
	})
	t.Run("measure count nested grid", func(t *testing.T) {
		// The leaf's min-content (20), max-content (80) and final (50) widths
		// differ, so measuring its height at the first two can never hit the
		// cache.
		leaf := leafWith(Style{}, textContent(text))
		inner := newNode(grid(Style{}), leaf)
		middle := newNode(grid(Style{}), inner)
		outer := newNode(grid(Style{Size: Size[Length]{Width: Px(50)}}), middle)
		computeLayout(outer, maxContentSpaces)
		wantSize(t, "leaf", leaf, 50, 20)
		wantCount(t, leaf, 6)
	})
	t.Run("measure count grid item with known width", func(t *testing.T) {
		leaf := leafWith(Style{Size: Size[Length]{Width: Px(40)}}, textContent(text))
		g := newNode(grid(Style{}), leaf)
		computeLayout(g, maxContentSpaces)
		wantSize(t, "grid", g, 40, 20)
		wantSize(t, "leaf", leaf, 40, 20)
		wantCount(t, leaf, 3)
	})
	t.Run("measure count grid item in fixed track", func(t *testing.T) {
		leaf := leafWith(Style{
			JustifySelf: alignOf(AlignStart),
			GridColumn:  Line[GridPlacement]{Start: GridLine(1)},
		}, textContent(text))
		sibling := newLeaf(Style{Size: pxSize(50, 50), GridColumn: Line[GridPlacement]{Start: GridLine(2)}})
		g := newNode(grid(Style{GridTemplateColumns: []GridTemplateComponent{
			{Track: Track(Px(100))}, {Track: Track(Auto)},
		}}), leaf, sibling)
		computeLayout(g, maxContentSpaces)
		wantSize(t, "grid", g, 150, 50)
		wantSize(t, "leaf", leaf, 80, 50)
		wantCount(t, leaf, 3)
	})
	t.Run("measure count flex in grid", func(t *testing.T) {
		for _, tc := range []struct {
			dir  FlexDirection
			want int
		}{{Row, 10}, {Column, 11}} {
			leaf := leafWith(Style{}, textContent(text))
			flex := newNode(Style{FlexDirection: tc.dir}, leaf)
			g := newNode(grid(Style{Size: Size[Length]{Width: Px(50)}}), flex)
			computeLayout(g, maxContentSpaces)
			wantSize(t, "leaf", leaf, 50, 20)
			wantCount(t, leaf, tc.want)
		}
	})
	t.Run("grid block item height not polluted by width measure", func(t *testing.T) {
		var items []*testNode
		for row := int16(1); row <= 3; row++ {
			leaf := leafWith(Style{}, fixedContent(100, 19))
			items = append(items, newNode(Style{
				Display:    DisplayBlock,
				GridRow:    Line[GridPlacement]{GridLine(row), GridLine(row + 1)},
				GridColumn: Line[GridPlacement]{GridLine(1), GridLine(2)},
			}, leaf))
		}
		container := newNode(grid(Style{
			GridTemplateColumns: []GridTemplateComponent{{Track: Track(Percent(0.6))}, {Track: Track(Auto)}},
			Size:                Size[Length]{Width: Px(1000)},
		}), items...)
		computeLayout(container, definiteSpace(1000))
		for i, item := range items {
			if l := item.layout(); l.Size.Height != 19 || l.Location.Y != 19*float32(i) {
				t.Errorf("item %d: height %v at y %v", i, l.Size.Height, l.Location.Y)
			}
		}
	})
	t.Run("grid item spanning only unaffected tracks is not measured", func(t *testing.T) {
		leaf := leafWith(Style{GridColumn: Line[GridPlacement]{GridLine(2), GridLine(3)}},
			textContent("HH HH HH HH"))
		g := newNode(grid(Style{
			Size: pxSize(400, 50),
			GridTemplateColumns: []GridTemplateComponent{
				{Track: Track(Px(100))}, {Track: MinMax(Px(0), Fr(1))},
			},
			GridTemplateRows: []GridTemplateComponent{{Track: Track(Px(50))}},
		}), leaf)
		computeLayout(g, maxContentSpaces)
		if w := leaf.layout().Size.Width; w != 300 {
			t.Errorf("leaf width %v, want 300", w)
		}
		wantCount(t, leaf, 1)
	})
}
