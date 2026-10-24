package layout

import "testing"

// Taffy's hand-written scroll size tests (tests/hand_written/scroll_size.rs):
// the maximum scroll offset is the content size less the padding box, so it
// must not depend on which side a border is on, and must agree across the
// layout algorithms.

const (
	scrollContent   float32 = 1000
	scrollContainer float32 = 200
	scrollBorder    float32 = 20
	scrollPadding   float32 = 20
)

func verticalEdges(top, bottom float32) Rect[Length] {
	return Rect[Length]{Left: Px(0), Right: Px(0), Top: Px(top), Bottom: Px(bottom)}
}

func scrollerWithPadding(display Display, border, padding Rect[Length]) Layout {
	child := newLeaf(Style{Size: pxSize(100, scrollContent)})
	node := newNode(Style{
		Display:   display,
		BoxSizing: BorderBox,
		Size:      pxSize(300, scrollContainer),
		Border:    border,
		Padding:   padding,
		Overflow:  Point[Overflow]{OverflowScroll, OverflowScroll},
	}, child)
	computeLayout(node, maxContentSpaces)
	return node.layout()
}

func measuredScrollLeaf(overflow Overflow, padding Rect[Length]) Layout {
	node := leafWith(Style{
		BoxSizing: BorderBox,
		Size:      pxSize(300, scrollContainer),
		Padding:   padding,
		Overflow:  Point[Overflow]{overflow, overflow},
	}, fixedContent(100, scrollContent))
	computeLayout(node, maxContentSpaces)
	return node.layout()
}

var verticalEdgePairs = [][2]float32{{20, 0}, {0, 20}, {20, 20}, {0, 0}}

func TestScrollSize(t *testing.T) {
	for _, d := range displays {
		t.Run(displayName(d), func(t *testing.T) {
			top := scrollerWithPadding(d, verticalEdges(scrollBorder, 0), verticalEdges(0, 0))
			bottom := scrollerWithPadding(d, verticalEdges(0, scrollBorder), verticalEdges(0, 0))
			if top.ScrollHeight() != bottom.ScrollHeight() {
				t.Errorf("border-top gave scroll height %v, border-bottom %v", top.ScrollHeight(),
					bottom.ScrollHeight())
			}
			for _, e := range verticalEdgePairs {
				l := scrollerWithPadding(d, verticalEdges(e[0], e[1]), verticalEdges(0, 0))
				if want := scrollContent - (scrollContainer - e[0] - e[1]); l.ScrollHeight() != want {
					t.Errorf("border %v: scroll height %v, want %v", e, l.ScrollHeight(), want)
				}
				if l.ScrollableOverflow.Bottom != scrollContent {
					t.Errorf("border %v: overflow bottom %v, want %v", e, l.ScrollableOverflow.Bottom, scrollContent)
				}

				l = scrollerWithPadding(d, verticalEdges(0, 0), verticalEdges(e[0], e[1]))
				if want := scrollContent + e[0] + e[1]; l.ScrollableOverflow.Bottom != want {
					t.Errorf("padding %v: overflow bottom %v, want %v", e, l.ScrollableOverflow.Bottom, want)
				}
				// The scrollable overflow runs from the padding box's origin to
				// the end of the content plus the end padding.
				if want := e[0] + scrollContent + e[1] - scrollContainer; l.ScrollHeight() != want {
					t.Errorf("padding %v: scroll height %v, want %v", e, l.ScrollHeight(), want)
				}

				// A box that is not a scroll container does not extend its
				// overflow by its own padding: only the content contributes.
				child := newLeaf(Style{Size: pxSize(100, scrollContent)})
				node := newNode(Style{
					Display:   d,
					BoxSizing: BorderBox,
					Size:      pxSize(300, scrollContainer),
					Padding:   verticalEdges(e[0], e[1]),
				}, child)
				computeLayout(node, maxContentSpaces)
				if want := e[0] + scrollContent; node.layout().ScrollableOverflow.Bottom != want {
					t.Errorf("non-scroll padding %v: overflow bottom %v, want %v", e,
						node.layout().ScrollableOverflow.Bottom, want)
				}
			}
			l := scrollerWithPadding(d, verticalEdges(scrollBorder, scrollBorder),
				verticalEdges(scrollPadding, scrollPadding))
			want := scrollPadding + scrollContent + scrollPadding - (scrollContainer - 2*scrollBorder)
			if l.ScrollHeight() != want {
				t.Errorf("borders and padding: scroll height %v, want %v", l.ScrollHeight(), want)
			}
		})
	}

	t.Run("leaf", func(t *testing.T) {
		for _, e := range verticalEdgePairs {
			if l := measuredScrollLeaf(OverflowScroll, verticalEdges(e[0], e[1])); l.ScrollableOverflow.Bottom !=
				e[0]+scrollContent+e[1] {
				t.Errorf("scroll container padding %v: overflow bottom %v", e, l.ScrollableOverflow.Bottom)
			}
			if l := measuredScrollLeaf(OverflowVisible, verticalEdges(e[0], e[1])); l.ScrollableOverflow.Bottom !=
				e[0]+scrollContent {
				t.Errorf("visible padding %v: overflow bottom %v", e, l.ScrollableOverflow.Bottom)
			}
		}
	})

	// Items in overflowing rows contribute their position, not only their
	// size, to the container's content size.
	t.Run("grid overflow includes item positions in overflowing tracks", func(t *testing.T) {
		c1 := newLeaf(Style{Size: pxSize(100, 500)})
		c2 := newLeaf(Style{Size: pxSize(100, 500)})
		node := newNode(Style{
			Display:  DisplayGrid,
			Size:     pxSize(300, scrollContainer),
			Overflow: Point[Overflow]{OverflowScroll, OverflowScroll},
		}, c1, c2)
		computeLayout(node, maxContentSpaces)
		l := node.layout()
		if l.ScrollableOverflow.Bottom != 1000 || l.ScrollHeight() != 1000-scrollContainer {
			t.Errorf("overflow bottom %v scroll height %v", l.ScrollableOverflow.Bottom, l.ScrollHeight())
		}
	})
}
