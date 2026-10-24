package layout

import "testing"

// Taffy's hand-written relayout, root, position, block margin and initial
// containing block tests (tests/hand_written).

func TestRelayout(t *testing.T) {
	node1 := newLeaf(Style{Size: pxSize(8, 80)})
	node0 := newNode(Style{AlignSelf: alignOf(AlignCenter)}, node1)
	node := newNode(Style{Size: Size[Length]{Percent(1), Percent(1)}}, node0)
	computeLayout(node, definiteSpace(100))
	initial := []Point[float32]{node.layout().Location, node0.layout().Location, node1.layout().Location}
	for range 9 {
		computeLayout(node, definiteSpace(100))
		for i, n := range []*testNode{node, node0, node1} {
			if l := n.layout().Location; l != initial[i] {
				t.Fatalf("node %d moved from %v to %v", i, initial[i], l)
			}
		}
	}
}

func TestToggleDisplayNone(t *testing.T) {
	hidden := Style{Display: DisplayNone, Size: pxSize(100, 100)}
	flex := Style{Display: DisplayFlex, Size: pxSize(100, 100)}
	grid := Style{Display: DisplayGrid, Size: pxSize(100, 100)}
	check := func(t *testing.T, step string, n *testNode, w, h float32) {
		t.Helper()
		if l := n.layout(); l.Location != (Point[float32]{}) || l.Size != (Size[float32]{w, h}) {
			t.Errorf("%s: location %v size %v, want 0,0 %vx%v", step, l.Location, l.Size, w, h)
		}
	}

	t.Run("root", func(t *testing.T) {
		node := newLeaf(hidden)
		computeLayout(node, maxContentSpaces)
		check(t, "hidden", node, 0, 0)
		node.setStyle(flex)
		computeLayout(node, maxContentSpaces)
		check(t, "shown", node, 100, 100)
		node.setStyle(hidden)
		computeLayout(node, maxContentSpaces)
		check(t, "hidden again", node, 0, 0)
	})

	t.Run("root with children", func(t *testing.T) {
		child := newLeaf(Style{Size: pxSize(800, 100)})
		parent := newNode(Style{Size: pxSize(800, 100)}, child)
		root := newNode(Style{}, parent)
		computeLayout(root, maxContentSpaces)
		check(t, "shown", child, 800, 100)
		root.setStyle(Style{Display: DisplayNone})
		computeLayout(root, maxContentSpaces)
		check(t, "hidden", child, 0, 0)
		root.setStyle(Style{})
		computeLayout(root, maxContentSpaces)
		check(t, "shown again: parent", parent, 800, 100)
		check(t, "shown again: child", child, 800, 100)
	})

	for _, container := range []Style{flex, grid} {
		name := displayName(container.Display)
		t.Run(name+" child", func(t *testing.T) {
			node := newLeaf(hidden)
			root := newNode(container, node)
			computeLayout(root, maxContentSpaces)
			check(t, "hidden", node, 0, 0)
			node.setStyle(container)
			computeLayout(root, maxContentSpaces)
			check(t, "shown", node, 100, 100)
			node.setStyle(hidden)
			computeLayout(root, maxContentSpaces)
			check(t, "hidden again", node, 0, 0)
		})
		t.Run(name+" container", func(t *testing.T) {
			node := newNode(hidden, newLeaf(hidden))
			computeLayout(node, maxContentSpaces)
			check(t, "hidden", node, 0, 0)
			node.setStyle(container)
			computeLayout(node, maxContentSpaces)
			check(t, "shown", node, 100, 100)
			node.setStyle(hidden)
			computeLayout(node, maxContentSpaces)
			check(t, "hidden again", node, 0, 0)
		})
	}
}

func TestRelayoutIsStableWithRounding(t *testing.T) {
	inner := newLeaf(Style{MinSize: Size[Length]{Width: Px(300)}})
	wrapper := newNode(Style{Size: Size[Length]{Width: Px(150)}, JustifyContent: contentOf(ContentEnd)}, inner)
	outer := newNode(Style{
		Size:     Size[Length]{Width: Percent(1)},
		Position: PositionRelative,
		Inset:    Rect[Length]{Left: Px(1.5), Right: Auto, Top: Auto, Bottom: Auto},
	}, wrapper)
	root := newNode(Style{Size: pxSize(1920, 1080)}, outer)
	nodes := []*testNode{root, outer, wrapper, inner}
	want := []Layout{
		{Size: Size[float32]{1920, 1080}},
		{Location: Point[float32]{2, 0}, Size: Size[float32]{1920, 1080}},
		{Size: Size[float32]{150, 1080}},
		{Location: Point[float32]{-150, 0}, Size: Size[float32]{300, 1080}},
	}
	for pass := range 6 {
		if pass > 0 {
			root.markDirty()
		}
		computeLayout(root, maxContentSpaces)
		for i, n := range nodes {
			l := n.layout()
			if l.Location != want[i].Location || l.Size != want[i].Size {
				t.Errorf("pass %d node %d: location %v size %v, want %v %v", pass, i, l.Location, l.Size,
					want[i].Location, want[i].Size)
			}
		}
	}
}

// relayoutAndFresh lays out the tree build makes with before, restyles the
// first node build returns with after and lays the tree out again. It
// returns the unrounded layouts of build's nodes alongside those of a fresh
// tree built with after. build returns the root last.
func relayoutAndFresh(before, after Style, build func(Style) []*testNode) (relaid, fresh []Layout) {
	layouts := func(ns []*testNode) []Layout {
		var ls []Layout
		for _, n := range ns {
			ls = append(ls, n.state.Unrounded)
		}
		return ls
	}
	ns := build(before)
	root := ns[len(ns)-1]
	computeLayout(root, maxContentSpaces)
	ns[0].setStyle(after)
	computeLayout(root, maxContentSpaces)

	fs := build(after)
	computeLayout(fs[len(fs)-1], maxContentSpaces)
	return layouts(ns), layouts(fs)
}

func sameLayouts(t *testing.T, relaid, fresh []Layout) {
	t.Helper()
	for i := range relaid {
		if relaid[i] != fresh[i] {
			t.Errorf("node %d relaid %+v, fresh %+v", i, relaid[i], fresh[i])
		}
	}
}

func TestRelayoutRestyled(t *testing.T) {
	// <div style="display: block; width: 100px; height: 200px">
	//   <div style="display: block; height: 50px">
	//     <div style="display: block; height: 10px; margin-top: 20px"></div>
	//   </div>
	// </div>
	// The outer box then becomes the container. The inner box's margin
	// collapses through the middle box only while the middle box shares its
	// parent's block formatting context: as a flex or grid item it contains
	// the margin.
	blockParentAs := func(t *testing.T, container Style) {
		size := pxSize(100, 200)
		container.Size = size
		relaid, fresh := relayoutAndFresh(Style{Display: DisplayBlock, Size: size}, container,
			func(rootStyle Style) []*testNode {
				inner := newLeaf(Style{
					Display: DisplayBlock,
					Size:    Size[Length]{Height: Px(10)},
					Margin:  Rect[Length]{Top: Px(20)},
				})
				middle := newNode(Style{Display: DisplayBlock, Size: Size[Length]{Height: Px(50)}}, inner)
				root := newNode(rootStyle, middle)
				return []*testNode{root, middle, inner, root}
			})
		if y := fresh[1].Location.Y; y != 0 {
			t.Errorf("item at y %v, want 0", y)
		}
		if y := fresh[2].Location.Y; y != 20 {
			t.Errorf("item's child at y %v, want 20", y)
		}
		sameLayouts(t, relaid, fresh)
	}
	t.Run("block child does not keep collapsed margin as flex item", func(t *testing.T) {
		blockParentAs(t, Style{Display: DisplayFlex, FlexDirection: Column})
	})
	t.Run("block child does not keep collapsed margin as grid item", func(t *testing.T) {
		blockParentAs(t, Style{Display: DisplayGrid})
	})

	// A grid container measures its first item with its min-width applied;
	// once the container is a flex container, the item's flex base size is
	// measured without it, so both items have a base size of zero and grow
	// equally.
	t.Run("flex base size is not answered by an inherent size measurement", func(t *testing.T) {
		container := func(d Display) Style { return Style{Display: d, Size: Size[Length]{Width: Px(100)}} }
		relaid, fresh := relayoutAndFresh(container(DisplayGrid), container(DisplayFlex),
			func(rootStyle Style) []*testNode {
				first := newLeaf(Style{
					Display:  DisplayGrid,
					FlexGrow: 1,
					Size:     Size[Length]{Height: Px(20)},
					MinSize:  Size[Length]{Width: Px(40)},
				})
				second := newLeaf(Style{Display: DisplayBlock, FlexGrow: 1, Size: Size[Length]{Height: Px(20)}})
				root := newNode(rootStyle, first, second)
				return []*testNode{root, first, second, root}
			})
		if fresh[1].Size.Width != 50 || fresh[2].Size.Width != 50 {
			t.Errorf("widths %v and %v, want 50", fresh[1].Size.Width, fresh[2].Size.Width)
		}
		sameLayouts(t, relaid, fresh)
	})

	// Once the middle block's height is auto, the item's percentage height
	// no longer resolves (CSS 2.1 §10.5): it is 19px tall, not 53.32px.
	t.Run("a percentage height is not reused after its containing block loses its height", func(t *testing.T) {
		block := func(h Length) Style { return Style{Display: DisplayBlock, Size: Size[Length]{Px(78), h}} }
		relaid, fresh := relayoutAndFresh(block(Px(62)), block(Auto), func(middleStyle Style) []*testNode {
			leaf := newLeaf(Style{Display: DisplayBlock, Size: Size[Length]{Height: Px(19)}})
			item := newNode(Style{Display: DisplayFlex, Size: Size[Length]{Px(31), Percent(0.86)}}, leaf)
			middle := newNode(middleStyle, item)
			root := newNode(Style{Display: DisplayFlex, FlexDirection: Column, Size: Size[Length]{Width: Px(78)}},
				middle)
			return []*testNode{middle, item, leaf, root}
		})
		if h := fresh[1].Size.Height; h != 19 {
			t.Errorf("item height %v, want 19", h)
		}
		sameLayouts(t, relaid, fresh)
	})
}

func TestRootConstraints(t *testing.T) {
	t.Run("root with percentage size", func(t *testing.T) {
		node := newLeaf(Style{Size: Size[Length]{Percent(1), Percent(1)}})
		computeLayout(node, Size[AvailableSpace]{Definite(100), Definite(200)})
		wantSize(t, "root", node, 100, 200)
	})
	t.Run("root with no size", func(t *testing.T) {
		node := newLeaf(Style{})
		computeLayout(node, definiteSpace(100))
		wantSize(t, "root", node, 0, 0)
	})
	t.Run("root with larger size", func(t *testing.T) {
		node := newLeaf(Style{Size: pxSize(200, 200)})
		computeLayout(node, definiteSpace(100))
		wantSize(t, "root", node, 200, 200)
	})
	t.Run("root padding and border larger than definite size", func(t *testing.T) {
		ten := Rect[Length]{Px(10), Px(10), Px(10), Px(10)}
		root := newNode(Style{Size: pxSize(10, 10), Padding: ten, Border: ten}, newLeaf(Style{}))
		computeLayout(root, maxContentSpaces)
		wantSize(t, "root", root, 40, 40)
	})
}

func TestPositionVariants(t *testing.T) {
	for _, d := range displays {
		for _, tc := range []struct {
			position Position
			want     Point[float32]
		}{
			{PositionStatic, Point[float32]{}},
			{PositionSticky, Point[float32]{}},
			{PositionRelative, Point[float32]{5, 7}},
		} {
			child := newLeaf(Style{
				Position: tc.position,
				Size:     pxSize(10, 10),
				Inset:    Rect[Length]{Left: Px(5), Right: Auto, Top: Px(7), Bottom: Auto},
			})
			root := newNode(Style{Display: d, Size: pxSize(100, 100)}, child)
			computeLayout(root, maxContentSpaces)
			if l := child.layout().Location; l != tc.want {
				t.Errorf("position %d in %s: location %v, want %v", tc.position, displayName(d), l, tc.want)
			}
		}
	}
	if !PositionSticky.isPositioned() || PositionSticky.isOutOfFlow() || PositionStatic.isPositioned() {
		t.Error("sticky must be positioned but in flow, and static not positioned")
	}
}

// TestLayoutMarginExcludesCollapsedMargins checks that margin collapsing
// moves boxes but Layout.Margin is each box's own margin.
func TestLayoutMarginExcludesCollapsedMargins(t *testing.T) {
	block := func(top, bottom float32, height Length) Style {
		return Style{
			Display: DisplayBlock,
			Size:    Size[Length]{Auto, height},
			Margin:  Rect[Length]{Top: Px(top), Bottom: Px(bottom)},
		}
	}
	inner := newLeaf(block(13, 20, Px(10)))
	outer := newNode(block(8, 5, Auto), inner)
	sibling := newLeaf(block(30, 0, Px(10)))
	root := newNode(Style{Display: DisplayFlowRoot, Size: Size[Length]{Width: Px(100)}}, outer, sibling)
	computeLayout(root, maxContentSpaces)

	// The inner box's margins collapse through the outer box's edges.
	if l := outer.layout(); l.Location.Y != 13 || l.Size.Height != 10 || l.Margin.Top != 8 || l.Margin.Bottom != 5 {
		t.Errorf("outer: y %v height %v margins %v/%v", l.Location.Y, l.Size.Height, l.Margin.Top, l.Margin.Bottom)
	}
	if l := inner.layout(); l.Location.Y != 0 || l.Margin.Top != 13 || l.Margin.Bottom != 20 {
		t.Errorf("inner: y %v margins %v/%v", l.Location.Y, l.Margin.Top, l.Margin.Bottom)
	}
	// The sibling follows the largest adjoining margin (5, 20, 30).
	if l := sibling.layout(); l.Location.Y != 53 || l.Margin.Top != 30 {
		t.Errorf("sibling: y %v margin %v", l.Location.Y, l.Margin.Top)
	}
}

func TestInitialContainingBlock(t *testing.T) {
	viewport := Size[AvailableSpace]{Definite(800), Definite(600)}
	rootStyle := Style{
		Display: DisplayBlock,
		Margin:  Rect[Length]{Left: Px(10), Right: Px(10), Top: Px(20), Bottom: Px(20)},
		Border:  Rect[Length]{Px(5), Px(5), Px(5), Px(5)},
	}
	zeroInset := Rect[Length]{Px(0), Px(0), Px(0), Px(0)}
	autoInset := Rect[Length]{Auto, Auto, Auto, Auto}

	t.Run("root is placed at its margin", func(t *testing.T) {
		root := newLeaf(rootStyle)
		computeLayout(root, viewport)
		if l := root.layout(); l.Location != (Point[float32]{10, 20}) || l.Size.Width != 780 {
			t.Errorf("location %v width %v", l.Location, l.Size.Width)
		}
	})
	for _, p := range []Position{PositionAbsolute, PositionFixed} {
		t.Run("inset zero fills viewport", func(t *testing.T) {
			oof := newLeaf(Style{Position: p, Inset: zeroInset})
			root := newNode(rootStyle, oof)
			computeLayout(root, viewport)
			// Relative to the root's border box, at (10, 20) in the viewport.
			if l := oof.layout(); l.Location != (Point[float32]{-10, -20}) || l.Size != (Size[float32]{800, 600}) {
				t.Errorf("position %d: location %v size %v", p, l.Location, l.Size)
			}
		})
		t.Run("bottom right insets resolve against viewport", func(t *testing.T) {
			oof := newLeaf(Style{
				Position: p,
				Inset:    Rect[Length]{Left: Auto, Top: Auto, Right: Px(0), Bottom: Px(0)},
				Size:     pxSize(50, 50),
			})
			root := newNode(rootStyle, oof)
			computeLayout(root, viewport)
			if l := oof.layout(); l.Location != (Point[float32]{800 - 50 - 10, 600 - 50 - 20}) {
				t.Errorf("position %d: location %v", p, l.Location)
			}
		})
	}
	t.Run("static position is unaffected", func(t *testing.T) {
		oof := newLeaf(Style{Position: PositionAbsolute, Inset: autoInset, Size: pxSize(50, 50)})
		root := newNode(rootStyle, oof)
		computeLayout(root, viewport)
		if l := oof.layout(); l.Location != (Point[float32]{5, 5}) {
			t.Errorf("location %v", l.Location)
		}
	})
	t.Run("fixed root is positioned by its insets", func(t *testing.T) {
		root := newLeaf(Style{
			Display:  DisplayBlock,
			Position: PositionFixed,
			Inset:    Rect[Length]{Left: Auto, Top: Percent(0.1), Right: Px(10), Bottom: Auto},
			Size:     pxSize(100, 50),
		})
		computeLayout(root, viewport)
		if l := root.layout(); l.Location != (Point[float32]{800 - 100 - 10, 60}) || l.Size != (Size[float32]{100, 50}) {
			t.Errorf("location %v size %v", l.Location, l.Size)
		}
	})
}
