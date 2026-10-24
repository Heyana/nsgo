package core

import "testing"

func wantFrames(t *testing.T, h *host, cases map[string]Bounds) {
	t.Helper()
	for id, want := range cases {
		if got := h.find(id).frame; got != want {
			t.Errorf("%s = %v, want %v", id, got, want)
		}
	}
}

func TestGridTracksAndPlacement(t *testing.T) {
	h := newHost(t, Size{Width: 400, Height: 300}, func(cx *Scope) Node {
		return View{Style: Style{
			Display:             DisplayGrid,
			GridTemplateColumns: "100px 1fr 2fr",
			GridAutoRows:        "50px",
			RowGap:              10,
		}, TestID: "grid", Children: []Node{
			View{Style: Style{GridColumn: GridLines{Start: GridSpan(2)}}, TestID: "wide"},
			View{TestID: "auto"},
			View{Style: Style{GridColumn: GridLines{Start: GridLine(-2), End: GridLine(-1)}}, TestID: "last"},
			View{Style: Style{GridRow: GridLines{Start: GridLine(3)}, GridColumn: GridLines{Start: GridLine(1)},
				JustifySelf: AlignCenter, Width: Pt(40)}, TestID: "placed"},
		}}
	})
	h.commit()
	wantFrames(t, h, map[string]Bounds{
		"grid":   {Size: Size{Width: 400, Height: 170}},
		"wide":   {Origin: Point{X: 0, Y: 0}, Size: Size{Width: 200, Height: 50}},
		"auto":   {Origin: Point{X: 200, Y: 0}, Size: Size{Width: 200, Height: 50}},
		"last":   {Origin: Point{X: 200, Y: 60}, Size: Size{Width: 200, Height: 50}},
		"placed": {Origin: Point{X: 30, Y: 120}, Size: Size{Width: 40, Height: 50}},
	})
}

func TestGridRepeatAndNamedLines(t *testing.T) {
	h := newHost(t, Size{Width: 300, Height: 100}, func(cx *Scope) Node {
		return View{Style: Style{
			Display:             DisplayGrid,
			GridTemplateColumns: "[start] repeat(auto-fill, 100px) [end]",
			GridTemplateRows:    "40px",
		}, Children: []Node{
			View{Style: Style{GridColumn: GridLines{Start: GridNamed("start", 0), End: GridNamed("end", 0)}},
				TestID: "span"},
		}}
	})
	h.commit()
	wantFrames(t, h, map[string]Bounds{"span": {Origin: Point{X: 0, Y: 0}, Size: Size{Width: 300, Height: 40}}})
}

func TestBlockAutoMarginsAndCollapsing(t *testing.T) {
	h := newHost(t, Size{Width: 300, Height: 300}, func(cx *Scope) Node {
		return View{Style: Style{Display: DisplayBlock, Width: Pt(300)}, Children: []Node{
			View{Style: Style{Display: DisplayBlock, Width: Pt(100), Height: Pt(20), AutoMargin: SidesHorizontal},
				TestID: "centered"},
			View{Style: Style{Display: DisplayBlock, Height: Pt(10), Margin: Edges{Bottom: 20}}, TestID: "first"},
			// Adjoining vertical margins collapse to the larger.
			View{Style: Style{Display: DisplayBlock, Height: Pt(10), Margin: Edges{Top: 30}}, TestID: "second"},
		}}
	})
	h.commit()
	wantFrames(t, h, map[string]Bounds{
		"centered": {Origin: Point{X: 100, Y: 0}, Size: Size{Width: 100, Height: 20}},
		"first":    {Origin: Point{X: 0, Y: 20}, Size: Size{Width: 300, Height: 10}},
		"second":   {Origin: Point{X: 0, Y: 60}, Size: Size{Width: 300, Height: 10}},
	})
}

func TestFlexAutoMarginCenters(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		return View{Style: Style{Direction: Row, Height: Pt(100)}, Children: []Node{
			View{Style: Style{Width: Pt(50), Height: Pt(20), AutoMargin: SidesAll}, TestID: "box"},
		}}
	})
	h.commit()
	wantFrames(t, h, map[string]Bounds{"box": {Origin: Point{X: 75, Y: 40}, Size: Size{Width: 50, Height: 20}}})
}

func TestRightToLeft(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		return View{Style: Style{Direction: Row, Padding: Edges{Left: 10}}, Children: []Node{
			View{Style: Style{Width: Pt(50), Height: Pt(10)}, TestID: "first"},
			View{Style: Style{Width: Pt(50), Height: Pt(10), Margin: Edges{Left: 5}}, TestID: "second"},
		}}
	})
	h.commit()
	wantFrames(t, h, map[string]Bounds{
		"first":  {Origin: Point{X: 10, Y: 0}, Size: Size{Width: 50, Height: 10}},
		"second": {Origin: Point{X: 65, Y: 0}, Size: Size{Width: 50, Height: 10}},
	})
	// Right to left, a row starts at the right and left and right swap.
	h.tree.SetEnvironment(Environment{RTL: true})
	h.commit()
	wantFrames(t, h, map[string]Bounds{
		"first":  {Origin: Point{X: 140, Y: 0}, Size: Size{Width: 50, Height: 10}},
		"second": {Origin: Point{X: 85, Y: 0}, Size: Size{Width: 50, Height: 10}},
	})
}

func TestOverflowHiddenKeepsScrollContent(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		return Scroll{Style: Style{Height: Pt(100)}, TestID: "scroll", Children: []Node{
			View{Style: Style{Height: Pt(50), Overflow: Hidden}, Children: []Node{
				View{Style: Style{Height: Pt(500)}},
			}},
			View{Style: Style{Height: Pt(80), BoxSizing: ContentBox, Padding: Edges{Top: 10}}},
		}}
	})
	h.commit()
	// The clipped child's overflow does not count; content-box sizing adds
	// the padding to the second child's height.
	if got := h.find("scroll").scroll.content; got != (Size{Width: 100, Height: 140}) {
		t.Errorf("content = %v, want 100x140", got)
	}
}

func TestGridTracksValid(t *testing.T) {
	for _, g := range []GridTracks{"", "1fr", "repeat(3, minmax(0, 1fr))", "[a] 10px [b] fit-content(20%)"} {
		if !g.Valid() {
			t.Errorf("%q is not valid", g)
		}
	}
	if GridTracks("1 fr banana").Valid() {
		t.Error(`"1 fr banana" is valid`)
	}
}

// TestGridMeasuresTextAtItsContentWidth is a table cell's case: a padded
// text in an auto-sized grid track. The track's width is its border box;
// measuring the text at that width lets it fit one line, while drawing it
// in the content box wraps it to two, overflowing the row.
func TestGridMeasuresTextAtItsContentWidth(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		return View{Style: Style{Display: DisplayGrid, GridTemplateColumns: "1fr 100px"}, Children: []Node{
			Text{Content: "aaaa bbbb", Style: Style{Padding: All(10), FontSize: 17}, TestID: "cell"},
			View{Style: Style{GridColumn: GridLines{Start: GridLine(2)}, Height: Pt(10)}},
		}}
	})
	h.commit()
	// Two 40.8 pt lines of the approximate 17 pt font, in 20 pt of padding.
	if got := h.find("cell").frame.Size.Height; got != 61 {
		t.Errorf("cell height = %v, want 61: the text wraps to two lines at its content width", got)
	}
}
