package core

import (
	"fmt"
	"math"
	goruntime "runtime"
	"testing"
	"weak"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

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

// Insets scroll with the content, so what fits between them does not fill the
// view, while what does not fit keeps its size.
func TestScrollContentStopsAtItsInsets(t *testing.T) {
	var height *Signal[Points]
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		height = NewSignal(cx, Points(40))
		return Scroll{Style: Style{Height: Pt(100)}, TestID: "scroll",
			Behavior: ScrollBehavior{ContentInset: Edges{Top: 30}}, Children: []Node{
				View{StyleFunc: func() Style { return Style{Height: Pt(height.Get())} }},
			}}
	})
	content := func(b *driver.MountBatch) driver.Size {
		t.Helper()
		for _, it := range b.Items {
			if it.Kind == driver.ItemScrollContent {
				return it.Size
			}
		}
		t.Fatal("no content size sent")
		return driver.Size{}
	}
	if got, want := content(h.commit()), (driver.Size{Width: 100, Height: 70}); got != want {
		t.Fatalf("fitting content = %v, want %v", got, want)
	}
	height.Set(90)
	if got, want := content(h.commit()), (driver.Size{Width: 100, Height: 90}); got != want {
		t.Fatalf("taller content = %v, want %v", got, want)
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

// TestGridBaselineRowsOfWrappingTexts is a history list's case (#280): rows of a title, a smaller status and a message in a baseline-aligned grid. Texts report no baseline of their own, so a row aligns its texts' one-line boxes by their bottoms; a text that wraps at its min-content width must not push its row's other cells down by its wrapped height.
func TestGridBaselineRowsOfWrappingTexts(t *testing.T) {
	for name, rows := range map[string][][3]string{
		"latin":    {{"Optimize app data", "skipped", "Safari is running"}, {"Flush DNS cache", "done", "Flushed"}, {"Reset QuickLook", "done", "Reset"}},
		"cjk":      {{"优化应用数据库", "已跳过", "Safari 仍在运行"}, {"刷新 DNS 缓存", "完成", "已刷新"}, {"重置快速查看", "完成", "已重置"}},
		"one word": {{"Alpha", "ok", "Beta"}, {"Gamma", "ok", "Delta"}, {"Epsilon", "ok", "Zeta"}},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHost(t, Size{Width: 600, Height: 300}, func(cx *Scope) Node {
				var cells []Node
				for r, row := range rows {
					for c, content := range row {
						size := Points(15)
						if c == 1 {
							size = 12
						}
						cells = append(cells, Text{TestID: fmt.Sprintf("r%dc%d", r, c), Content: content, Style: Style{FontSize: size}})
					}
				}
				return View{Style: Style{Display: DisplayGrid, GridTemplateColumns: "max-content max-content 1fr",
					ColumnGap: 12, RowGap: 8, AlignItems: AlignBaseline}, Children: cells}
			})
			h.commit()
			var prevBottom Points
			for r := range rows {
				top, bottom := Points(math.MaxFloat32), Points(0)
				for c := range 3 {
					f := h.find(fmt.Sprintf("r%dc%d", r, c)).frame
					top, bottom = min(top, f.Origin.Y), max(bottom, f.Origin.Y+f.Size.Height)
					if c > 0 {
						if prev := h.find(fmt.Sprintf("r%dc0", r)).frame; f.Origin.Y+f.Size.Height != prev.Origin.Y+prev.Size.Height {
							t.Errorf("row %d: cell %d = %v, cell 0 = %v: their bottoms differ", r, c, f, prev)
						}
					}
				}
				if r > 0 && top != prevBottom+8 {
					t.Errorf("row %d starts at %v, want %v: the row before ends at %v plus the gap", r, top, prevBottom+8, prevBottom)
				}
				prevBottom = bottom
			}
		})
	}
}

// A row of cells, the first a fixed size or its content's, holding a
// text that the signal it returns sets.
func cellRow(t *testing.T, fixed bool) (*host, *Signal[string]) {
	var value *Signal[string]
	h := newHost(t, Size{Width: 400, Height: 300}, func(cx *Scope) Node {
		value = NewSignal(cx, "9.07")
		cell := Style{Padding: All(4)}
		if fixed {
			cell.Width, cell.Height = Pt(56), Pt(24)
		}
		return View{TestID: "row", Style: Style{Direction: Row, Gap: 8}, Children: []Node{
			View{TestID: "cell", Style: cell, Children: []Node{Text{TestID: "value", ContentFunc: value.Get}}},
			View{TestID: "next", Style: Style{Width: Pt(40), Height: Pt(20)}},
		}}
	})
	h.commit()
	return h, value
}

// resolved resolves what changed, as a commit does first, so the test
// can see what that marked for layout.
func (h *host) resolved() { h.tree.resolve(h.tree.view, textFields{}, false) }

func TestAChangeInAFixedCellLaysOutTheCellAlone(t *testing.T) {
	h, value := cellRow(t, true)
	value.Set("10.14") // wider, and still inside the cell
	h.resolved()
	if h.tree.view.layoutDirty || h.find("row").layoutDirty {
		t.Fatal("a change inside a fixed cell marked the views around it")
	}
	if !h.find("cell").queued {
		t.Fatal("the cell is not laid out on its own")
	}
	// The text keeps the width the cell stretches it to.
	if got := kinds(h.commit()); got[driver.ItemText] != 1 || len(got) != 1 {
		t.Errorf("items %v, want the text's alone", got)
	}
}

func TestATextThatMeasuresAsBeforeLaysNothingOut(t *testing.T) {
	h, value := cellRow(t, false)
	value.Set("9.14") // as wide: the approximate engine's characters are
	h.resolved()
	if h.tree.view.layoutDirty || h.find("cell").layoutDirty || h.find("value").layoutDirty {
		t.Fatal("a text that measures as before marked layout")
	}
	if got := kinds(h.commit()); got[driver.ItemText] != 1 || len(got) != 1 {
		t.Errorf("items %v, want the text's alone", got)
	}
	value.Set("+9.14")
	h.resolved()
	if !h.tree.view.layoutDirty {
		t.Fatal("a wider text in a cell of its size laid nothing out")
	}
	if w := h.find("cell").frame.Size.Width; h.commit() != nil && h.find("cell").frame.Size.Width <= w {
		t.Error("the cell did not grow with its text")
	}
}

func TestARestyledFixedCellLaysOutItsParent(t *testing.T) {
	var wide *Signal[bool]
	h := newHost(t, Size{Width: 400, Height: 300}, func(cx *Scope) Node {
		wide = NewSignal(cx, false)
		return View{TestID: "row", Style: Style{Direction: Row}, Children: []Node{
			View{TestID: "cell", StyleFunc: func() Style {
				w := Points(56)
				if wide.Get() {
					w = 100
				}
				return Style{Width: Pt(w), Height: Pt(24)}
			}},
			View{TestID: "next", Style: Style{Width: Pt(40), Height: Pt(20)}},
		}}
	})
	h.commit()
	wide.Set(true)
	h.commit()
	if x := h.find("next").frame.Origin.X; x != 100 {
		t.Errorf("the cell after a resized fixed cell is at %v, want 100", x)
	}
}

func TestAFixedCellWhoseOverflowChangesLaysOutItsParent(t *testing.T) {
	var tall *Signal[bool]
	h := newHost(t, Size{Width: 400, Height: 300}, func(cx *Scope) Node {
		tall = NewSignal(cx, false)
		return Scroll{TestID: "scroll", Children: []Node{
			View{TestID: "cell", Style: Style{Width: Pt(100), Height: Pt(50)}, Children: []Node{
				View{StyleFunc: func() Style {
					h := Points(20)
					if tall.Get() {
						h = 900
					}
					return Style{Height: Pt(h)}
				}},
			}},
		}}
	})
	h.commit()
	tall.Set(true)
	h.commit()
	if c := h.find("scroll").scroll.content.Height; c != 900 {
		t.Errorf("the scroll view's content is %v tall, want the overflow's 900", c)
	}
}

func TestAModalInsideAFixedViewShows(t *testing.T) {
	var open *Signal[bool]
	h := newHost(t, Size{Width: 320, Height: 200}, func(cx *Scope) Node {
		open = NewSignal(cx, false)
		// The view is a relayout boundary; the root places the modal.
		return View{Style: Style{Width: Pt(320), Height: Pt(200), Padding: All(10)}, Children: []Node{
			Input{Style: Style{Width: Pt(300), Height: Pt(40)}},
			Show{When: open.Get, Then: func(*Scope) Node {
				return Modal{TestID: "modal", Style: Style{Background: RGB(0xff0000)}}
			}},
		}}
	})
	h.commit()
	open.Set(true)
	h.commit()
	if f := h.find("modal").frame; f.Size != (Size{Width: 320, Height: 200}) {
		t.Errorf("the modal is laid out at %v, want the surface's size", f)
	}
}

func TestNestedRelayoutBoundariesBothLayOut(t *testing.T) {
	var a, x *Signal[Points]
	h := newHost(t, Size{Width: 400, Height: 400}, func(cx *Scope) Node {
		a, x = NewSignal(cx, Points(10)), NewSignal(cx, Points(0))
		inner := View{Style: Style{Width: Pt(100), Height: Pt(100)}, Children: []Node{
			View{TestID: "dot", StyleFunc: func() Style {
				return Style{Position: Absolute, Left: Pt(x.Get()), Width: Pt(1), Height: Pt(1)}
			}},
		}}
		return View{Style: Style{Width: Pt(300), Height: Pt(300)}, Children: []Node{
			View{Style: Style{Width: Pt(200), Height: Pt(200)}, Children: []Node{inner}},
			View{StyleFunc: func() Style { return Style{Width: Pt(a.Get()), Height: Pt(10)} }},
		}}
	})
	h.commit()
	h.root.Batch(func() {
		x.Set(50)
		a.Set(20)
	})
	// The commit checks itself against a full layout (CheckCommits).
	h.commit()
}

// Relayout boundaries laid out on their own are freed once removed: the queue of boundaries keeps none it has laid out, so views shown and hidden again and again do not each stay behind in its unused slots.
func TestRemovedRelayoutBoundariesAreFreed(t *testing.T) {
	var shown *Signal[bool]
	var x *Signal[Points]
	h := newHost(t, Size{Width: 400, Height: 400}, func(cx *Scope) Node {
		shown, x = NewSignal(cx, true), NewSignal(cx, Points(0))
		boundary := func(id string) Node {
			return View{TestID: id, Style: Style{Width: Pt(100), Height: Pt(100)}, Children: []Node{
				View{StyleFunc: func() Style {
					return Style{Position: Absolute, Left: Pt(x.Get()), Width: Pt(1), Height: Pt(1)}
				}},
			}}
		}
		return View{Children: []Node{
			Show{When: shown.Get, Then: func(*Scope) Node {
				return Group{boundary("a"), boundary("b")}
			}},
		}}
	})
	h.commit()
	x.Set(50) // lays a and b out on their own
	h.commit()
	a, b := weak.Make(h.find("a")), weak.Make(h.find("b"))
	for _, on := range []bool{false, true, false} {
		shown.Set(on)
		h.commit()
	}
	goruntime.GC()
	if a.Value() != nil || b.Value() != nil {
		t.Fatal("a boundary removed two showings ago stays in memory")
	}
	goruntime.KeepAlive(h.tree)
}

func TestLayoutRoundsToDevicePixels(t *testing.T) {
	const ratio = 2.625
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		return View{Style: Style{AlignItems: AlignStart}, Children: []Node{
			View{Style: Style{Margin: Edges{Top: 4}, Height: Pt(30)}, TestID: "scroll", Children: []Node{
				View{Style: Style{Height: Pt(30), Width: Pt(30), BorderWidth: 1}, TestID: "chip"},
			}},
			View{Style: Style{Height: Pt(28.2)}, TestID: "row"},
		}}
	})
	h.tree.SetEnvironment(Environment{PixelRatio: ratio})
	h.commit()
	onPixels := func(v Points) bool {
		p := float64(v) * ratio
		return p-float64(int(p+0.5)) < 1e-3 && float64(int(p+0.5))-p < 1e-3
	}
	for _, id := range []string{"scroll", "chip", "row"} {
		f := h.find(id).frame
		for _, v := range []Points{f.Origin.X, f.Origin.Y, f.Size.Width, f.Size.Height} {
			if !onPixels(v) {
				t.Errorf("%s = %v, off the device pixels", id, f)
			}
		}
	}
	// 4 points down is 10.5 pixels: the scroll view and the chip as tall
	// as it are one height, not a pixel apart.
	if s, c := h.find("scroll").frame.Size.Height, h.find("chip").frame.Size.Height; s != c {
		t.Errorf("scroll view %v tall, chip %v", s, c)
	}
}

func TestTextRoundsUpToADevicePixel(t *testing.T) {
	for _, c := range []struct {
		p     Points
		ratio float32
		want  Points
	}{
		{56 / 2.625, 2.625, 56 / 2.625},
		{21.4, 2.625, 57 / 2.625},
		{20.5, 1, 21},
		{20, 1, 20},
		{20.25, 2, 20.5},
	} {
		if got := ceil(c.p, c.ratio); float64(got)*float64(c.ratio)-float64(c.want)*float64(c.ratio) > 1e-3 ||
			float64(c.want)*float64(c.ratio)-float64(got)*float64(c.ratio) > 1e-3 {
			t.Errorf("ceil(%v, %v) = %v, want %v", c.p, c.ratio, got, c.want)
		}
	}
}

// fixedEngine measures every text at one size, as a platform's engine reports what its layout measured, fractions kept.
type fixedEngine struct{ size Size }

func (e fixedEngine) Prepare(t *driver.AttributedText) driver.PreparedText {
	return fixedText{ApproxTextEngine{}.Prepare(t), e.size}
}

type fixedText struct {
	driver.PreparedText
	size Size
}

func (f fixedText) Measure(Points) Size { return f.size }

// TestAMeasuredTextRoundsUpToADevicePixel is the text engines' contract: they return the fractional size they measured, and the core alone rounds it up, to a device pixel rather than a whole point.
func TestAMeasuredTextRoundsUpToADevicePixel(t *testing.T) {
	for _, c := range []struct {
		ratio  float32
		pixels Size // the frame's size in device pixels
	}{
		{1, Size{Width: 101, Height: 21}},
		{1.5, Size{Width: 151, Height: 31}},
		{2, Size{Width: 201, Height: 41}},
		{3, Size{Width: 301, Height: 61}},
	} {
		h := newHost(t, Size{Width: 400, Height: 400}, nil)
		h.tree = NewTree(h.root, fixedEngine{Size{Width: 100.2, Height: 20.3}})
		h.tree.Resize(Size{Width: 400, Height: 400}, Geometry{})
		h.tree.Mount(func(cx *Scope) Node {
			return View{Style: Style{AlignItems: AlignStart}, Children: []Node{Text{TestID: "text", Content: "measured"}}}
		})
		h.tree.SetEnvironment(Environment{PixelRatio: c.ratio})
		h.commit()
		f := h.find("text").frame.Size
		if w, ht := float64(f.Width*Points(c.ratio)), float64(f.Height*Points(c.ratio)); math.Abs(w-float64(c.pixels.Width)) > 1e-3 || math.Abs(ht-float64(c.pixels.Height)) > 1e-3 {
			t.Errorf("at %v pixels a point, a text measured 100.2 by 20.3 is laid out %v, %v by %v pixels; want %v", c.ratio, f, w, ht, c.pixels)
		}
	}
}

func TestFramesMovedByWholePixelsAreNotSentAgain(t *testing.T) {
	const ratio = 2.625
	var top *Signal[Points]
	h := newHost(t, Size{Width: 200, Height: 2000}, func(cx *Scope) Node {
		top = NewSignal(cx, Points(0))
		rows := make([]Node, 40)
		for i := range rows {
			rows[i] = View{Style: Style{Height: Pt(28)}, Children: []Node{View{Style: Style{Height: Pt(10)}}}}
		}
		return View{Children: []Node{
			View{StyleFunc: func() Style { return Style{Margin: Edges{Top: top.Get()}} }, TestID: "box", Children: rows},
		}}
	})
	h.tree.SetEnvironment(Environment{PixelRatio: ratio})
	h.commit()
	box := h.find("box")
	for px := 1; px < 400; px++ {
		top.Set(Points(float32(px) / ratio))
		for _, it := range h.commit().Items {
			if it.Kind == driver.ItemFrame && it.ID != box.id && it.ID != box.parent.id {
				t.Fatalf("pixel %d: view %d's frame %v sent again", px, it.ID, it.Frame)
			}
		}
	}
}

// TestFarEdgesRoundAgainWhenMovedByWholePixels is #809: a box 3,000,000 points tall, a DataTable's scroll extent, keeps its far edge on float32's quarter points, so moved by whole pixels from a half pixel at 3.5 pixels a point it rounds to another pixel count, and its frame must follow, as a full layout (CheckCommits) finds it.
func TestFarEdgesRoundAgainWhenMovedByWholePixels(t *testing.T) {
	const ratio = 3.5
	var top *Signal[Points]
	h := newHost(t, Size{Width: 200, Height: 800}, func(cx *Scope) Node {
		top = NewSignal(cx, Points(157.5/ratio))
		return View{Children: []Node{
			View{StyleFunc: func() Style { return Style{Height: Pt(top.Get())} }},
			View{Style: Style{AlignItems: AlignStart}, Children: []Node{
				View{Style: Style{Width: Pt(100), Height: Pt(3_000_000)}, Children: []Node{View{Style: Style{Height: Pt(3_000_000)}, TestID: "far"}}},
			}},
		}}
	})
	h.tree.SetEnvironment(Environment{PixelRatio: ratio})
	h.commit()
	for px := 1; px < 20; px++ {
		top.Set(Points((157.5 + float32(px)) / ratio))
		h.commit() // panics where the commit kept a frame a full layout rounds otherwise
	}
	if f := h.find("far").frame; f.Size.Height < 2_999_999 {
		t.Errorf("far = %v", f)
	}
}

func TestColumnFitsHeightAtItsItemsUsedWidth(t *testing.T) {
	h := newHost(t, Size{Width: 240, Height: 200}, func(*Scope) Node {
		return View{Style: Style{AlignItems: AlignEnd}, Children: []Node{
			View{Children: []Node{
				View{Style: Style{Width: Pt(80), Height: Pt(1)}},
				Text{TestID: "text", Content: "one two three", Style: Style{Width: Pct(50), FontSize: 10}},
			}},
			View{TestID: "next", Style: Style{Height: Pt(10)}},
		}}
	})
	h.commit()
	text, next := h.find("text").frame, h.find("next").frame
	if bottom := text.Origin.Y + text.Size.Height; bottom > next.Origin.Y {
		t.Fatalf("text ends at %v, next item starts at %v", bottom, next.Origin.Y)
	}
}

// TestATextShrinksInARowWithFlexShrink is the Sizes section of layout.md (#308): a text in a row shrinks below its one-line width and wraps only with FlexShrink, an unset minimum being 0 already, so MinWidth: Pt(0) alone leaves it overflowing the row. With an unset minimum it shrinks into less than its widest word and breaks its words; AutoMinimum stops it at its widest word, as CSS's automatic minimum does.
func TestATextShrinksInARowWithFlexShrink(t *testing.T) {
	for _, c := range []struct {
		name  string
		style Style
		want  Size
	}{
		{"FlexShrink", Style{FlexGrow: 1, FlexShrink: 1}, Size{Width: 20, Height: 96}},
		{"MinWidth 0 alone", Style{FlexGrow: 1, MinWidth: Pt(0)}, Size{Width: 114, Height: 12}},
		{"AutoMinimum", Style{FlexGrow: 1, FlexShrink: 1, MinWidth: AutoMinimum}, Size{Width: 24, Height: 48}},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
				return View{Style: Style{Direction: Row, Gap: 10, AlignItems: AlignStart, FontSize: 10}, Children: []Node{
					View{Style: Style{Width: Pt(70), Height: Pt(10)}},
					Text{Content: "aaaa bbbb cccc dddd", Style: c.style, TestID: "text"},
				}}
			})
			h.commit()
			if got := h.find("text").frame.Size; got != c.want {
				t.Errorf("text = %v, want %v", got, c.want)
			}
		})
	}
}
