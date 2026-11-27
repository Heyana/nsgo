package editor

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestBlockDecorationReplacesWholeLinesAndMeasuresHeight(t *testing.T) {
	var ref Ref
	var setShowBlock func(bool)
	var setFixedHeight func(core.Points)
	var btnPresses int
	var sels []Selection
	doc := "# Title\n| a | b |\n| - | - |\n| 1 | 2 |\nafter"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		showBlock := core.NewSignal(cx, true)
		fixedH := core.NewSignal(cx, core.Points(0))
		setShowBlock = showBlock.Set
		setFixedHeight = fixedH.Set
		return Editor{
			Ref: &ref, TestID: "editor", Label: "Markdown", Language: "markdown",
			DefaultValue: doc, LineHeight: 20,
			Style: core.Style{Width: core.Pt(400), Height: core.Pt(240)},
			Decorations: func() []Decoration {
				if !showBlock.Get() {
					return nil
				}
				return []Decoration{{
					Range:  Range{Start: Position{Line: 1, Column: 0}, End: Position{Line: 4, Column: 0}},
					Block:  true,
					Height: fixedH.Get(),
					Replace: core.View{
						TestID: "table-widget",
						Style:  core.Style{Height: core.Pt(72)},
						Children: []core.Node{
							core.View{
								TestID:  "table-btn",
								Style:   core.Style{Width: core.Pt(40), Height: core.Pt(24)},
								OnPress: func(core.PressEvent) { btnPresses++ },
							},
						},
					},
				}}
			},
			OnSelectionChange: func(s []Selection) { sels = s },
		}
	}, nsgotest.Environment(core.Environment{Idiom: core.IdiomDesktop}))

	app.Settle()
	e := ref.e

	// Replaced lines 1..3 are hidden; line 0 and line 4 remain visible.
	app.Find("text:# Title")
	app.Find("text:after")
	for _, hidden := range []string{"| a | b |", "| - | - |", "| 1 | 2 |"} {
		if n := len(app.Query("text:" + hidden)); n != 0 {
			t.Fatalf("replaced block line %q still visible (%d)", hidden, n)
		}
	}

	// Auto-measured block sits at y=20 with height 72, pushing line 4 to y=92.
	widget := app.Find("id:table-widget")
	if got := widget.Bounds().Size.Height; math.Abs(float64(got-72)) > 0.5 {
		t.Fatalf("auto-measured block height = %v, want 72", got)
	}
	if top := e.rowTop(2); math.Abs(float64(top-92)) > 0.5 {
		t.Fatalf("row 2 (line 4) top = %v, want 92", top)
	}

	// Gutter shows line 1 and line 5 (at y=92) and hides 2..4.
	app.Find(`text:"1"`)
	l5 := app.Find(`text:"5"`)
	for _, num := range []string{"2", "3", "4"} {
		if n := len(app.Query(`text:"` + num + `"`)); n != 0 {
			t.Fatalf("gutter still shows replaced line number %s", num)
		}
	}
	if y := l5.Parent.Frame.Origin.Y; math.Abs(float64(y-92)) > 0.5 {
		t.Fatalf("gutter line 5 Y = %v, want 92", y)
	}

	// Interactive child inside block widget receives presses.
	app.Press("id:table-btn")
	if btnPresses != 1 {
		t.Fatalf("table-btn presses = %d, want 1", btnPresses)
	}

	// Clicking line 4 at y=102 places caret on line 4.
	if got := e.posAt(core.Point{X: core.Points(pad + 2*e.charW), Y: 102}); got != (pos{4, 2}) {
		t.Fatalf("posAt on line 4 = %+v, want {4 2}", got)
	}

	// Vertical navigation: Down from line 0 enters line 1 (block start); Down again exits to line 4.
	ref.SetSelections([]Selection{{Anchor: Position{0, 2}, Head: Position{0, 2}}})
	ref.Run(MoveDown)
	if len(sels) != 1 || sels[0].Head.Line != 1 {
		t.Fatalf("MoveDown from line 0 -> %+v, want line 1", sels)
	}
	ref.Run(MoveDown)
	if len(sels) != 1 || sels[0].Head.Line != 4 {
		t.Fatalf("MoveDown past block -> %+v, want line 4", sels)
	}
	// Up from line 4 enters line 3 (block end); Up again exits to line 0.
	ref.Run(MoveUp)
	if len(sels) != 1 || sels[0].Head.Line != 3 {
		t.Fatalf("MoveUp into block -> %+v, want line 3", sels)
	}
	ref.Run(MoveUp)
	if len(sels) != 1 || sels[0].Head.Line != 0 {
		t.Fatalf("MoveUp past block -> %+v, want line 0", sels)
	}

	// Explicit Height overrides auto measurement.
	setFixedHeight(96)
	app.Settle()
	if top := e.rowTop(2); math.Abs(float64(top-116)) > 0.5 {
		t.Fatalf("row 2 top with Height=96 = %v, want 116", top)
	}

	// Removing block decoration reveals lines 1..3 and restores uniform layout.
	setShowBlock(false)
	app.Settle()
	app.Find("text:| a | b |")
	if !e.uniform || e.tops != nil {
		t.Fatalf("expected uniform fast path after removing block, uniform=%v tops=%v", e.uniform, e.tops)
	}
	if top := e.rowTop(4); math.Abs(float64(top-80)) > 0.5 {
		t.Fatalf("line 4 top restored = %v, want 80", top)
	}
}

func TestBlockDecorationInsertedBeforeAndAfterLines(t *testing.T) {
	var ref Ref
	var copyClicks, headerMounts int
	var sels []Selection
	doc := "intro\n```go\nfmt.Println()\n```\noutro"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return Editor{
			Ref: &ref, TestID: "editor", DefaultValue: doc, LineHeight: 20,
			Style: core.Style{Width: core.Pt(400), Height: core.Pt(300)},
			Decorations: func() []Decoration {
				return []Decoration{
					{
						// Empty range at col 0: inserted BEFORE line 1.
						Range:  Range{Start: Position{Line: 1, Column: 0}, End: Position{Line: 1, Column: 0}},
						Block:  true,
						Height: 28,
						Replace: mountCounter{
							id:     "code-header",
							mounts: &headerMounts,
							press:  func() { copyClicks++ },
						},
					},
					{
						// Empty range at col > 0: inserted AFTER line 3.
						Range:   Range{Start: Position{Line: 3, Column: 3}, End: Position{Line: 3, Column: 3}},
						Block:   true,
						Height:  24,
						Replace: core.View{TestID: "code-footer", Style: core.Style{Height: core.Pt(24)}},
					},
				}
			},
			OnSelectionChange: func(s []Selection) { sels = s },
		}
	})

	app.Settle()
	e := ref.e

	// All 5 source lines remain visible alongside both inserted blocks (7 display rows).
	if got := e.st.disp.count(); got != 7 {
		t.Fatalf("disp.count() = %d, want 7 (5 lines + 2 inserted blocks)", got)
	}
	// Check exact prefix-sum Y coordinates:
	// row 0 (line 0): y=0, h=20
	// row 1 (header): y=20, h=28
	// row 2 (line 1): y=48, h=20
	// row 3 (line 2): y=68, h=20
	// row 4 (line 3): y=88, h=20
	// row 5 (footer): y=108, h=24
	// row 6 (line 4): y=132, h=20
	wantTops := []float32{0, 20, 48, 68, 88, 108, 132}
	for i, want := range wantTops {
		if got := e.rowTop(i); math.Abs(float64(got-want)) > 0.5 {
			t.Fatalf("rowTop(%d) = %v, want %v", i, got, want)
		}
	}

	app.Press("id:code-header")
	if copyClicks != 1 {
		t.Fatalf("code-header clicks = %d, want 1", copyClicks)
	}

	// Vertical navigation skips inserted-only blocks cleanly.
	ref.SetSelections([]Selection{{Anchor: Position{0, 1}, Head: Position{0, 1}}})
	ref.Run(MoveDown)
	if len(sels) != 1 || sels[0].Head.Line != 1 {
		t.Fatalf("MoveDown across inserted header -> %+v, want line 1", sels)
	}
	ref.Run(MoveUp)
	if len(sels) != 1 || sels[0].Head.Line != 0 {
		t.Fatalf("MoveUp across inserted header -> %+v, want line 0", sels)
	}
	ref.SetSelections([]Selection{{Anchor: Position{3, 1}, Head: Position{3, 1}}})
	ref.Run(MoveDown)
	if len(sels) != 1 || sels[0].Head.Line != 4 {
		t.Fatalf("MoveDown across inserted footer -> %+v, want line 4", sels)
	}
	ref.Run(MoveUp)
	if len(sels) != 1 || sels[0].Head.Line != 3 {
		t.Fatalf("MoveUp across inserted footer -> %+v, want line 3", sels)
	}

	// Editing above the inserted block shifts its row without remounting it.
	headerBefore := app.Find("id:code-header").Bounds()
	ref.Edit([]Edit{{Range: rng(0, 0, 0), Text: "top\n"}})
	app.Settle()
	if headerMounts != 1 {
		t.Fatalf("header remounted on shift: mounts=%d, want 1", headerMounts)
	}
	headerAfter := app.Find("id:code-header").Bounds()
	if dy := headerAfter.Origin.Y - headerBefore.Origin.Y; math.Abs(float64(dy-20)) > 0.5 {
		t.Fatalf("header shifted by %v, want 20", dy)
	}
}

func TestHiddenBlockCollapsesWholeLinesToZeroHeight(t *testing.T) {
	var ref Ref
	var setHideAll func(bool)
	var sels []Selection
	doc := "---\ntitle: Note\n---\n# Heading\nbody"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		hideAll := core.NewSignal(cx, false)
		setHideAll = hideAll.Set
		return Editor{
			Ref: &ref, TestID: "editor", DefaultValue: doc, LineHeight: 20,
			Style: core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Decorations: func() []Decoration {
				if hideAll.Get() {
					return []Decoration{{
						Range:  Range{Start: Position{Line: 0, Column: 0}, End: Position{Line: 4, Column: 4}},
						Hidden: true, Block: true,
					}}
				}
				return []Decoration{{
					Range:  Range{Start: Position{Line: 0, Column: 0}, End: Position{Line: 3, Column: 0}},
					Hidden: true, Block: true,
				}}
			},
			OnSelectionChange: func(s []Selection) { sels = s },
		}
	})

	app.Settle()
	e := ref.e

	// Frontmatter (lines 0..2) collapses completely: only lines 3 and 4 show.
	if got := e.st.disp.count(); got != 2 {
		t.Fatalf("disp.count() = %d, want 2", got)
	}
	if n := len(app.Query("text:title: Note")); n != 0 {
		t.Fatalf("hidden frontmatter still rendered (%d)", n)
	}
	app.Find("text:# Heading")
	headingBox := ref.RangeBounds(rng(3, 0, 1))
	if len(headingBox) != 1 || math.Abs(float64(headingBox[0].Bounds.Origin.Y)) > 0.5 {
		t.Fatalf("heading should be at y=0, got %+v", headingBox)
	}
	// Gutter line 4 sits at y=0.
	l4 := app.Find(`text:"4"`)
	if y := l4.Parent.Frame.Origin.Y; math.Abs(float64(y)) > 0.5 {
		t.Fatalf("gutter line 4 Y = %v, want 0", y)
	}

	// Pressing Up from line 3 moves to document start {0, 0} so live preview can reveal frontmatter.
	ref.SetSelections([]Selection{{Anchor: Position{3, 0}, Head: Position{3, 0}}})
	ref.Run(MoveUp)
	if len(sels) != 1 || sels[0].Head != (Position{0, 0}) {
		t.Fatalf("MoveUp from first visible line -> %+v, want {0 0}", sels)
	}

	// Edge case: hiding every line in the document must not panic on queries or navigation.
	setHideAll(true)
	app.Settle()
	if got := e.st.disp.count(); got != 0 {
		t.Fatalf("disp.count() with all lines hidden = %d, want 0", got)
	}
	_ = e.posAt(core.Point{X: 10, Y: 10})
	_, _ = e.charAt(core.Point{X: 10, Y: 10})
	_ = ref.VisibleRange()
	_, _ = ref.CaretBounds()
	ref.Run(MoveDown)
	ref.Run(MoveUp)
}

func TestPerLineFontSizeAndLineHeight(t *testing.T) {
	var ref Ref
	var setStyled func(bool)
	doc := "# Heading\nnormal line\n## Subheading\nfinal line"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		styled := core.NewSignal(cx, true)
		setStyled = styled.Set
		return Editor{
			Ref: &ref, TestID: "editor", DefaultValue: doc,
			FontSize: 13, LineHeight: 20,
			Style: core.Style{Width: core.Pt(400), Height: core.Pt(240)},
			Decorations: func() []Decoration {
				if !styled.Get() {
					return nil
				}
				return []Decoration{
					{
						Range:     Range{Start: Position{Line: 0, Column: 0}, End: Position{Line: 0, Column: 0}},
						LineStyle: core.Style{FontSize: 26, LineHeight: 40, FontWeight: core.FontWeightBold},
					},
					{
						// FontSize without explicit LineHeight scales proportionally.
						Range:     Range{Start: Position{Line: 2, Column: 0}, End: Position{Line: 2, Column: 0}},
						LineStyle: core.Style{FontSize: 19.5},
					},
					{
						// Inline replacement on the heading line uses its tall height and wider charW.
						Range:   rng(0, 0, 2),
						Replace: core.View{TestID: "h-badge", Style: core.Style{FlexGrow: 1}},
					},
				}
			},
		}
	}, nsgotest.Environment(core.Environment{Idiom: core.IdiomDesktop}))

	app.Settle()
	e := ref.e

	// Row heights: line 0 = 40, line 1 = 20, line 2 = 30 (20 * 19.5 / 13), line 3 = 20.
	if e.uniform {
		t.Fatal("expected variable geometry with per-line LineHeight/FontSize")
	}
	wantTops := []float32{0, 40, 60, 90, 110}
	for i, want := range wantTops {
		if got := e.rowTop(i); math.Abs(float64(got-want)) > 0.5 {
			t.Fatalf("rowTop(%d) = %v, want %v", i, got, want)
		}
	}

	// Heading line uses a wider character width than body lines.
	headCharW := e.lineCharW(0)
	if headCharW <= e.charW {
		t.Fatalf("heading charW (%v) should be wider than default (%v)", headCharW, e.charW)
	}

	// CaretBounds on heading line has height 40 and X scaled by headCharW.
	ref.SetSelections([]Selection{{Anchor: Position{0, 5}, Head: Position{0, 5}}})
	cb, ok := ref.CaretBounds()
	if !ok {
		t.Fatal("CaretBounds returned !ok")
	}
	if math.Abs(float64(cb.Bounds.Size.Height-40)) > 0.5 {
		t.Fatalf("heading caret height = %v, want 40", cb.Bounds.Size.Height)
	}
	wantX := e.gutterWidth() + pad + 5*headCharW
	if math.Abs(float64(cb.Bounds.Origin.X-core.Points(wantX))) > 1 {
		t.Fatalf("heading caret X = %v, want %v", cb.Bounds.Origin.X, wantX)
	}

	// Hit-testing on heading line maps X using headCharW.
	if got := e.posAt(core.Point{X: core.Points(pad + 4.1*headCharW), Y: 20}); got != (pos{0, 4}) {
		t.Fatalf("posAt on heading = %+v, want {0 4}", got)
	}
	// Hit-testing on line 3 at y=100 lands on line 3 (not line 5 as uniform 20pt would).
	if got := e.posAt(core.Point{X: core.Points(pad + 2*e.charW), Y: 100}); got != (pos{3, 2}) {
		t.Fatalf("posAt on line 3 (y=100) = %+v, want {3 2}", got)
	}

	// Selection rects on heading line have height 40 and wider width.
	ref.SetSelections([]Selection{{Anchor: Position{0, 2}, Head: Position{0, 9}}})
	rects := e.selectionRects()
	if len(rects) != 1 || math.Abs(float64(rects[0].h-40)) > 0.5 || math.Abs(float64(rects[0].w-7*headCharW)) > 1 {
		t.Fatalf("heading selection rect = %+v, want h=40 w=%v", rects, 7*headCharW)
	}

	// Gutter line numbers align with variable row tops.
	l2 := app.Find(`text:"2"`).Parent.Frame
	l4 := app.Find(`text:"4"`).Parent.Frame
	if math.Abs(float64(l2.Origin.Y-40)) > 0.5 || math.Abs(float64(l4.Origin.Y-90)) > 0.5 {
		t.Fatalf("gutter alignment mismatch: line2 Y=%v (want 40), line4 Y=%v (want 90)", l2.Origin.Y, l4.Origin.Y)
	}

	// Inline replacement on heading line takes heading height and wider width.
	badge := app.Find("id:h-badge").Bounds()
	if math.Abs(float64(badge.Size.Height-40)) > 0.5 || math.Abs(float64(badge.Size.Width-core.Points(2*headCharW))) > 1 {
		t.Fatalf("h-badge bounds = %+v, want h=40 w=%v", badge.Size, 2*headCharW)
	}

	// Clearing line style decorations restores uniform fast path.
	setStyled(false)
	app.Settle()
	if !e.uniform || e.tops != nil {
		t.Fatalf("expected uniform fast path after clearing LineStyle, uniform=%v tops=%v", e.uniform, e.tops)
	}
}

func TestVariableHeightScrollingAndPageNavigation(t *testing.T) {
	var lines []string
	for i := range 40 {
		lines = append(lines, fmt.Sprintf("line %02d", i))
	}
	var ref Ref
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return Editor{
			Ref: &ref, TestID: "editor", DefaultValue: strings.Join(lines, "\n"),
			LineHeight: 20,
			Style:      core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Decorations: func() []Decoration {
				return []Decoration{
					{
						Range:     Range{Start: Position{Line: 0}, End: Position{Line: 0}},
						LineStyle: core.Style{FontSize: 26, LineHeight: 40},
					},
					{
						Range:   Range{Start: Position{Line: 5}, End: Position{Line: 8}},
						Block:   true,
						Height:  120,
						Replace: core.View{TestID: "tall-block", Style: core.Style{Height: core.Pt(120)}},
					},
					{
						Range:         Range{Start: Position{Line: 20}, End: Position{Line: 21}},
						OverviewRuler: &OverviewRulerMark{Lane: LaneFull, Style: core.Style{Background: core.RGB(0x22c55e)}},
					},
				}
			},
		}
	})
	app.Settle()

	// Rows 0..4: 40 + 4*20 = 120; Row 5 (block for lines 5..7): 120 (ends at 240).
	// Viewport is 200pt tall -> VisibleRange starts at line 0 and ends inside block (line 7).
	vr := ref.VisibleRange()
	if vr.Start.Line != 0 || vr.End.Line != 7 {
		t.Fatalf("initial VisibleRange = %+v, want lines 0..7", vr)
	}

	// Scroll past the tall block to y=240 -> line 8 is at the top!
	ref.SetScrollOffset(core.Point{Y: 240})
	app.Settle()
	vr = ref.VisibleRange()
	if vr.Start.Line != 8 || vr.End.Line != 17 {
		t.Fatalf("VisibleRange at y=240 = %+v, want lines 8..17", vr)
	}

	// PageDown from line 0 (y=0, viewport 200pt) steps over the 120pt block rather than jumping 9 rows past it.
	ref.SetScrollOffset(core.Point{})
	ref.SetSelections([]Selection{{Anchor: Position{0, 0}, Head: Position{0, 0}}})
	ref.Run(MovePageDown)
	afterDown := ref.Selections()[0].Head.Line
	if afterDown < 5 || afterDown > 8 {
		t.Fatalf("MovePageDown from line 0 landed on line %d, want 5..8 (one viewport height)", afterDown)
	}
}

func TestEditOutsideHistoryAndClearHistory(t *testing.T) {
	var ref Ref
	var value string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return Editor{
			Ref: &ref, TestID: "editor",
			DefaultValue: "first\nsecond\nthird\nfourth",
			OnChange:     func(s string) { value = s },
			Style:        core.Style{Width: core.Pt(400), Height: core.Pt(100)},
		}
	})
	app.Settle()

	// User edit 1: append "!" on line 0.
	ref.SetSelections([]Selection{{Anchor: Position{0, 5}, Head: Position{0, 5}}})
	ref.Insert("!")
	app.Settle()

	// User edit 2: append "?" on line 2.
	ref.SetSelections([]Selection{{Anchor: Position{2, 5}, Head: Position{2, 5}}})
	ref.Insert("?")
	app.Settle()
	if value != "first!\nsecond\nthird?\nfourth" {
		t.Fatalf("after user edits: %q", value)
	}

	// Keep caret on line 2 ("third?"), and apply remote edits BEFORE line 0 and BETWEEN line 0 & 2.
	ref.SetSelections([]Selection{{Anchor: Position{2, 0}, Head: Position{2, 6}}})
	ref.EditOutsideHistory([]Edit{
		{Range: rng(0, 0, 0), Text: "SYNC\n"},
		{Range: rng(1, 0, 0), Text: "remote "},
	})
	app.Settle()
	wantMerged := "SYNC\nfirst!\nremote second\nthird?\nfourth"
	if value != wantMerged || ref.Value() != wantMerged {
		t.Fatalf("after EditOutsideHistory: %q, want %q", value, wantMerged)
	}

	// Selection mapped to line 3 ("third?").
	if got := ref.SelectedText(); got != "third?" {
		t.Fatalf("selection after remote edits = %q, want %q", got, "third?")
	}

	// Undo 1: takes back ONLY user edit 2 ("?") at its mapped line 3, keeping both remote edits.
	ref.Run(Undo)
	app.Settle()
	if want := "SYNC\nfirst!\nremote second\nthird\nfourth"; value != want {
		t.Fatalf("after Undo 1: %q, want %q", value, want)
	}

	// Undo 2: takes back ONLY user edit 1 ("!") at its mapped line 1, keeping both remote edits.
	ref.Run(Undo)
	app.Settle()
	if want := "SYNC\nfirst\nremote second\nthird\nfourth"; value != want {
		t.Fatalf("after Undo 2: %q, want %q", value, want)
	}

	// No more user edits to undo: a third Undo must NOT undo the remote sync edits.
	if ref.CanUndo() {
		t.Fatal("CanUndo should be false after undoing both user edits")
	}
	ref.Run(Undo)
	app.Settle()
	if want := "SYNC\nfirst\nremote second\nthird\nfourth"; value != want {
		t.Fatalf("extra Undo altered remote edits: %q", value)
	}

	// Redo brings both user edits back at their mapped positions.
	ref.Run(Redo)
	ref.Run(Redo)
	app.Settle()
	if value != wantMerged {
		t.Fatalf("after Redo x2: %q, want %q", value, wantMerged)
	}

	// ClearHistory drops both undo and redo stacks.
	ref.Run(Undo)
	if !ref.CanUndo() || !ref.CanRedo() {
		t.Fatalf("expected both CanUndo=%v and CanRedo=%v before ClearHistory", ref.CanUndo(), ref.CanRedo())
	}
	ref.ClearHistory()
	if ref.CanUndo() || ref.CanRedo() {
		t.Fatalf("expected CanUndo=false and CanRedo=false after ClearHistory")
	}
	beforeClear := ref.Value()
	ref.Run(Undo)
	ref.Run(Redo)
	if ref.Value() != beforeClear {
		t.Fatalf("Undo/Redo after ClearHistory changed text to %q", ref.Value())
	}
}
