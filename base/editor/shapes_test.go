package editor

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func rng(line, from, to int) Range {
	return Range{Start: Position{Line: line, Column: from}, End: Position{Line: line, Column: to}}
}

// hiddenEditor is the Markdown sample with a switch for hiding its markers.
func hiddenEditor(t *testing.T) (*nsgotest.App, *Ref, func(bool), *[]Selection, *string) {
	var ref Ref
	var hide func(bool)
	var sels []Selection
	value := "a **bold** c"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		hidden := core.NewSignal(cx, false)
		hide = hidden.Set
		return Editor{
			Ref: &ref, TestID: "editor", Label: "Markdown", Language: "markdown",
			DefaultValue: value,
			Style:        core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Decorations: func() []Decoration {
				if !hidden.Get() {
					return nil
				}
				return []Decoration{{Range: rng(0, 2, 4), Hidden: true, Class: "markers"}, {Range: rng(0, 8, 10), Hidden: true, Class: "markers"}}
			},
			OnChange:          func(s string) { value = s },
			OnSelectionChange: func(s []Selection) { sels = s },
		}
	})
	return app, &ref, hide, &sels, &value
}

func TestHiddenDecorationCollapsesItsColumns(t *testing.T) {
	app, ref, hide, sels, value := hiddenEditor(t)

	app.Find("text:a **bold** c")
	before := ref.RangeBounds(rng(0, 11, 12))
	if len(before) != 1 {
		t.Fatalf("%d boxes for the last c", len(before))
	}

	hide(true)
	app.Settle()
	app.Find("text:a bold c")
	if n := len(app.Query("text:a **bold** c")); n != 0 {
		t.Fatalf("the markers still show in %d rows", n)
	}

	after := ref.RangeBounds(rng(0, 11, 12))
	if len(after) != 1 {
		t.Fatalf("%d boxes for the last c while hidden", len(after))
	}
	charW := after[0].Bounds.Size.Width
	if moved := before[0].Bounds.Origin.X - after[0].Bounds.Origin.X; math.Abs(float64(moved-4*charW)) > 0.5 {
		t.Fatalf("the c moved %v, want four columns of %v", moved, charW)
	}

	// Pressing the character puts the caret before it, as the columns say.
	b := after[0].Bounds
	app.PressAt("id:editor", driver.Point{X: driver.Points(b.Origin.X + b.Size.Width/2), Y: driver.Points(b.Origin.Y + b.Size.Height/2)})
	if len(*sels) != 1 || (*sels)[0].Head != (Position{Line: 0, Column: 11}) {
		t.Fatalf("the press left %+v, want the caret before the last c", *sels)
	}

	// The text keeps its offsets: typing at the end lands after the last c,
	// and the hidden range moves with the text.
	ref.SetSelections([]Selection{{Anchor: Position{Line: 0, Column: 12}, Head: Position{Line: 0, Column: 12}}})
	ref.Focus()
	app.Type("label:Markdown", "!")
	app.Settle()
	if *value != "a **bold** c!" {
		t.Fatalf("typed %q", *value)
	}
	app.Find("text:a bold c!")
	if n := len(app.Query("text:**")); n != 0 {
		t.Fatalf("the markers show again after the edit")
	}
}

func TestReplacedDecorationShowsItsViewInTheRange(t *testing.T) {
	var ref Ref
	var replace func(bool)
	var pressed int
	value := "- [ ] task"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		replace = on.Set
		return Editor{
			Ref: &ref, TestID: "editor", Label: "Markdown", Language: "markdown",
			DefaultValue: value,
			Style:        core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Decorations: func() []Decoration {
				if !on.Get() {
					return nil
				}
				return []Decoration{{
					Range: rng(0, 2, 5),
					Replace: core.View{TestID: "check", Style: core.Style{Width: core.Pt(12), Height: core.Pt(12)},
						OnPress: func(core.PressEvent) { pressed++ }},
				}}
			},
		}
	})

	app.Find("text:- [ ] task")
	before := ref.RangeBounds(rng(0, 6, 10))
	if len(before) != 1 {
		t.Fatalf("%d boxes for the word", len(before))
	}

	replace(true)
	app.Settle()
	check := app.Find("id:check")
	if n := len(app.Query("text:- [ ] task")); n != 0 {
		t.Fatalf("the replaced text still shows in %d rows", n)
	}
	// Placeholder spaces keep trailing text aligned with the replaced columns.
	app.Find("text:-     task")

	// The view keeps the range's columns, so the word after it stays put.
	after := ref.RangeBounds(rng(0, 6, 10))
	if len(after) != 1 {
		t.Fatalf("%d boxes for the word while replaced", len(after))
	}
	if moved := before[0].Bounds.Origin.X - after[0].Bounds.Origin.X; math.Abs(float64(moved)) > 0.5 {
		t.Fatalf("the word moved %v, want it where it was", moved)
	}
	replBox := ref.RangeBounds(rng(0, 2, 5))
	if len(replBox) != 1 || math.Abs(float64(check.Bounds().Origin.X-replBox[0].PageBounds.Origin.X)) > 1 {
		t.Fatalf("check at x=%v, want %v", check.Bounds().Origin.X, replBox[0].PageBounds.Origin.X)
	}
	// The replacement wrapper resets Cursor to CursorDefault instead of inheriting CursorText.
	if got := app.CursorAt("id:check").Cursor; got != driver.CursorDefault {
		t.Fatalf("cursor over Replace view = %v, want CursorDefault", got)
	}

	app.Press("id:check")
	if pressed != 1 {
		t.Fatalf("the view took %d presses, want one", pressed)
	}
}

func TestHiddenDecorationAcrossLinesKeepsTheBreaks(t *testing.T) {
	var ref Ref
	var hide func(bool)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		hidden := core.NewSignal(cx, false)
		hide = hidden.Set
		return Editor{
			Ref: &ref, TestID: "editor", Label: "Markdown", Language: "markdown",
			DefaultValue: "a\n**b\nc**\nd",
			Style:        core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Decorations: func() []Decoration {
				if !hidden.Get() {
					return nil
				}
				return []Decoration{{Range: Range{Start: Position{Line: 1, Column: 0}, End: Position{Line: 2, Column: 3}}, Hidden: true}}
			},
		}
	})

	hide(true)
	app.Settle()
	app.Find("text:a")
	app.Find("text:d")
	for _, row := range []string{"**b", "c**"} {
		if n := len(app.Query("text:" + row)); n != 0 {
			t.Fatalf("%q still shows", row)
		}
	}
	if rows := ref.RangeBounds(Range{Start: Position{Line: 3, Column: 0}, End: Position{Line: 3, Column: 1}}); len(rows) != 1 {
		t.Fatalf("%d boxes for the last line", len(rows))
	} else if y := rows[0].Bounds.Origin.Y; math.Abs(float64(y/rows[0].Bounds.Size.Height-3)) > 0.01 {
		t.Fatalf("the last line is at row %v, want 3", y/rows[0].Bounds.Size.Height)
	}
}

// mountCounter counts how many times its node mounts inside a Replace box.
type mountCounter struct {
	id     string
	mounts *int
	press  func()
}

func (m mountCounter) Render(*core.Scope) core.Node {
	*m.mounts++
	return core.View{
		TestID: m.id,
		Style:  core.Style{Width: core.Pt(12), Height: core.Pt(12)},
		OnPress: func(core.PressEvent) {
			if m.press != nil {
				m.press()
			}
		},
	}
}

func TestReplacedDecorationUpdatesInPlaceAndKeepsUnchangedMounted(t *testing.T) {
	var ref Ref
	var setChecked func(bool)
	var openPresses, donePresses, mounts int
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		checked := core.NewSignal(cx, false)
		setChecked = checked.Set
		return Editor{
			Ref: &ref, TestID: "editor", Label: "Tasks", Language: "markdown",
			DefaultValue: "header\n- [ ] task",
			Style:        core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Decorations: func() []Decoration {
				if checked.Get() {
					return []Decoration{{
						Range:   rng(1, 2, 5),
						Replace: mountCounter{id: "box-done", mounts: &mounts, press: func() { donePresses++ }},
					}}
				}
				return []Decoration{{
					Range:   rng(1, 2, 5),
					Replace: mountCounter{id: "box-open", mounts: &mounts, press: func() { openPresses++ }},
				}}
			},
		}
	})

	openBefore := app.Find("id:box-open").Bounds()
	if mounts != 1 {
		t.Fatalf("mounts=%d, want 1", mounts)
	}
	app.Press("id:box-open")
	if openPresses != 1 {
		t.Fatalf("openPresses=%d, want 1", openPresses)
	}

	// Updating Decoration.Replace in place on the exact same range swaps the view reactively.
	setChecked(true)
	app.Settle()
	if n := len(app.Query("id:box-open")); n != 0 {
		t.Fatalf("box-open still mounted (%d)", n)
	}
	doneBefore := app.Find("id:box-done").Bounds()
	if mounts != 2 {
		t.Fatalf("mounts=%d after swap, want 2", mounts)
	}
	app.Press("id:box-done")
	if donePresses != 1 {
		t.Fatalf("donePresses=%d, want 1", donePresses)
	}

	// Shifting the replacement to a new row via an unrelated edit updates its Y
	// without remounting the unchanged decoration's view.
	ref.Edit([]Edit{{Range: rng(0, 0, 0), Text: "top\n"}})
	app.Settle()
	doneAfter := app.Find("id:box-done").Bounds()
	if mounts != 2 {
		t.Fatalf("unchanged decoration remounted on shift: mounts=%d, want 2", mounts)
	}
	wantShift := core.Points(ref.e.lh)
	if dy := doneAfter.Origin.Y - doneBefore.Origin.Y; math.Abs(float64(dy-wantShift)) > 0.5 {
		t.Fatalf("box-done shifted by dy=%v, want %v (openBefore=%v)", dy, wantShift, openBefore.Origin.Y)
	}
}

func TestHiddenAndReplacedOnSameLine(t *testing.T) {
	var ref Ref
	var setHide func(bool)
	var setOverlap func(bool)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		hide := core.NewSignal(cx, false)
		overlap := core.NewSignal(cx, false)
		setHide = hide.Set
		setOverlap = overlap.Set
		return Editor{
			Ref: &ref, TestID: "editor", Label: "Markdown", Language: "markdown",
			DefaultValue: "**a** [ ] task",
			Style:        core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Decorations: func() []Decoration {
				var decos []Decoration
				if hide.Get() {
					decos = append(decos,
						Decoration{Range: rng(0, 0, 2), Hidden: true},
						Decoration{Range: rng(0, 3, 5), Hidden: true},
					)
				}
				if overlap.Get() {
					decos = append(decos, Decoration{Range: rng(0, 6, 9), Hidden: true})
				}
				decos = append(decos, Decoration{
					Range:   rng(0, 6, 9),
					Replace: core.View{TestID: "chk", Style: core.Style{Width: core.Pt(12), Height: core.Pt(12)}},
				})
				return decos
			},
		}
	})

	app.Find("text:**a**     task")
	before := app.Find("id:chk").Bounds()
	charW := core.Points(ref.e.charW)

	// Toggling earlier Hidden ranges moves the Replace view left immediately.
	setHide(true)
	app.Settle()
	app.Find("text:a     task")
	after := app.Find("id:chk").Bounds()
	if moved := before.Origin.X - after.Origin.X; math.Abs(float64(moved-4*charW)) > 1 {
		t.Fatalf("chk moved %v, want %v", moved, 4*charW)
	}
	wantBox := ref.RangeBounds(rng(0, 6, 9))
	if len(wantBox) != 1 || math.Abs(float64(after.Origin.X-wantBox[0].PageBounds.Origin.X)) > 1 {
		t.Fatalf("chk at x=%v, want %v", after.Origin.X, wantBox[0].PageBounds.Origin.X)
	}

	// Replace keeps its columns even when a Hidden range covers the same bytes.
	setOverlap(true)
	app.Settle()
	app.Find("text:a     task")
	app.Find("id:chk")
}

func TestReplacedDecorationUnderSoftWrap(t *testing.T) {
	var ref Ref
	var setDecos func([]Decoration)
	// Narrow soft-wrapped line so "aaaaaaaaaa " wraps before "bb [ ] cc".
	text := "aaaaaaaaaa bb [ ] cc"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		decos := core.NewSignalFunc(cx, []Decoration(nil), func(_, _ []Decoration) bool { return false })
		setDecos = decos.Set
		return Editor{
			Ref: &ref, TestID: "editor", NoLineNumbers: true, NoFolding: true, SoftWrap: true,
			DefaultValue: text,
			Style:        core.Style{Width: core.Pt(100), Height: core.Pt(200)},
			Decorations:  decos.Get,
		}
	})

	if ref.e.st.disp.count() < 2 {
		t.Fatalf("expected line to soft-wrap, got %d display rows", ref.e.st.disp.count())
	}
	row1 := ref.e.st.disp.row(1, ref.e.st.r())
	if row1.start == 0 {
		t.Fatalf("expected continuation row with start > 0, got %+v", row1)
	}

	// 1. Replace wholly on the wrapped continuation row aligns with row 1.
	replStart := row1.start + 3 // "[ ]" on row 1 (or 3 bytes into row 1)
	replEnd := replStart + 3
	setDecos([]Decoration{{
		Range:   rng(0, replStart, replEnd),
		Replace: core.View{TestID: "wrap-repl", Style: core.Style{Width: core.Pt(12), Height: core.Pt(12)}},
	}})
	app.Settle()
	view := app.Find("id:wrap-repl").Bounds()
	want := ref.RangeBounds(rng(0, replStart, replEnd))
	if len(want) != 1 {
		t.Fatalf("expected 1 box for continuation replacement, got %d", len(want))
	}
	if dx := math.Abs(float64(view.Origin.X - want[0].PageBounds.Origin.X)); dx > 1 {
		t.Fatalf("wrapped Replace X=%v, want %v", view.Origin.X, want[0].PageBounds.Origin.X)
	}
	if dy := math.Abs(float64(view.Origin.Y - want[0].PageBounds.Origin.Y)); dy > 1 {
		t.Fatalf("wrapped Replace Y=%v, want %v", view.Origin.Y, want[0].PageBounds.Origin.Y)
	}

	// 2. Replace split across the wrap boundary hides its text, keeps its columns, and shows no view.
	afterBefore := ref.RangeBounds(rng(0, len(text)-2, len(text)))
	setDecos([]Decoration{{
		Range:   rng(0, row1.start-2, row1.start+2),
		Replace: core.View{TestID: "split-repl", Style: core.Style{Width: core.Pt(12), Height: core.Pt(12)}},
	}})
	app.Settle()
	if n := len(app.Query("id:split-repl")); n != 0 {
		t.Fatalf("split Replace showed %d views, want 0", n)
	}
	afterSplit := ref.RangeBounds(rng(0, len(text)-2, len(text)))
	if len(afterBefore) != 1 || len(afterSplit) != 1 ||
		math.Abs(float64(afterBefore[0].Bounds.Origin.X-afterSplit[0].Bounds.Origin.X)) > 0.5 {
		t.Fatalf("split Replace changed trailing column: before=%+v after=%+v", afterBefore, afterSplit)
	}
	// The characters spanning the wrap boundary were replaced with spaces.
	row0Text := text[:row1.start]
	if len(app.Query("text:"+row0Text)) != 0 {
		t.Fatalf("split Replace still drew original row 0 text %q", row0Text)
	}
}

func TestMultilineReplacedDecorationHidesTextWithoutView(t *testing.T) {
	var ref Ref
	var setOn func(bool)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		setOn = on.Set
		return Editor{
			Ref: &ref, TestID: "editor",
			DefaultValue: "aa\nbb[cc\ndd]ee\nff",
			Style:        core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Decorations: func() []Decoration {
				if !on.Get() {
					return nil
				}
				return []Decoration{{
					Range:   Range{Start: Position{Line: 1, Column: 2}, End: Position{Line: 2, Column: 3}},
					Replace: core.View{TestID: "multi-repl", Style: core.Style{Width: core.Pt(12), Height: core.Pt(12)}},
				}}
			},
		}
	})

	eeBefore := ref.RangeBounds(rng(2, 3, 5))
	setOn(true)
	app.Settle()

	if n := len(app.Query("id:multi-repl")); n != 0 {
		t.Fatalf("multiline Replace showed %d views, want 0", n)
	}
	// Both lines hide their replaced characters with spaces while keeping columns.
	app.Find("text:bb   ")
	app.Find("text:   ee")
	if len(app.Query("text:bb[cc")) != 0 || len(app.Query("text:dd]ee")) != 0 {
		t.Fatalf("multiline Replace still drew original text")
	}
	eeAfter := ref.RangeBounds(rng(2, 3, 5))
	if len(eeBefore) != 1 || len(eeAfter) != 1 ||
		math.Abs(float64(eeBefore[0].Bounds.Origin.X-eeAfter[0].Bounds.Origin.X)) > 0.5 {
		t.Fatalf("multiline Replace shifted trailing text: before=%+v after=%+v", eeBefore, eeAfter)
	}
}

func TestOverlappingAndReversedShapeRanges(t *testing.T) {
	var ref Ref
	var setDecos func([]Decoration)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		decos := core.NewSignalFunc(cx, []Decoration(nil), func(_, _ []Decoration) bool { return false })
		setDecos = decos.Set
		return Editor{
			Ref: &ref, TestID: "editor",
			DefaultValue: "0123456789\nsecond\nthird",
			Style:        core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Decorations:  decos.Get,
		}
	})

	// Overlapping Replace ranges merge and keep only the first view without stacking.
	setDecos([]Decoration{
		{Range: rng(0, 2, 5), Replace: core.View{TestID: "first-repl", Style: core.Style{Width: core.Pt(10), Height: core.Pt(10)}}},
		{Range: rng(0, 4, 7), Replace: core.View{TestID: "second-repl", Style: core.Style{Width: core.Pt(10), Height: core.Pt(10)}}},
	})
	app.Settle()
	app.Find("id:first-repl")
	if n := len(app.Query("id:second-repl")); n != 0 {
		t.Fatalf("overlapping Replace stacked %d duplicate views", n)
	}
	app.Find("text:01     789")

	// Reversed ranges (single-line and multiline) are ignored cleanly.
	setDecos([]Decoration{
		{Range: rng(0, 5, 2), Replace: core.View{TestID: "rev-repl", Style: core.Style{Width: core.Pt(10), Height: core.Pt(10)}}},
		{Range: rng(0, 8, 6), Hidden: true},
		{Range: Range{Start: Position{Line: 2, Column: 1}, End: Position{Line: 0, Column: 1}}, Replace: core.View{TestID: "rev-multi"}},
	})
	app.Settle()
	if len(app.Query("id:rev-repl")) != 0 || len(app.Query("id:rev-multi")) != 0 {
		t.Fatalf("reversed Replace ranges should show no views")
	}
	app.Find("text:0123456789")
}

func TestShapesWithWideRunesAndTabs(t *testing.T) {
	var ref Ref
	var setDecos func([]Decoration)
	// "a\t[中文] b": 'a' (col 0..1), '\t' (3 cols -> 1..4), "[中文]" (6 cols -> 4..10), " b" (10..12).
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		decos := core.NewSignalFunc(cx, []Decoration(nil), func(_, _ []Decoration) bool { return false })
		setDecos = decos.Set
		return Editor{
			Ref: &ref, TestID: "editor", TabSize: 4,
			DefaultValue: "a\t[中文] b\n**\tc",
			Style:        core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Decorations:  decos.Get,
		}
	})

	charW := ref.e.charW
	// Replace "[中文]" (UTF-16 cols 2..6 = 6 display cols) and hide leading "**" before '\t' on line 1.
	setDecos([]Decoration{
		{Range: rng(0, 2, 6), Replace: core.View{TestID: "cjk-repl", Style: core.Style{FlexGrow: 1}}},
		{Range: rng(1, 0, 2), Hidden: true},
	})
	app.Settle()

	// Line 0: "a" + 3 spaces (tab) + 6 spaces ([中文]) + " b" = "a          b".
	app.Find("text:a          b")
	cjk := app.Find("id:cjk-repl").Bounds()
	box := ref.RangeBounds(rng(0, 2, 6))
	if len(box) != 1 {
		t.Fatalf("expected 1 box for CJK range, got %d", len(box))
	}
	if math.Abs(float64(box[0].Bounds.Size.Width-core.Points(6*charW))) > 1 {
		t.Fatalf("CJK replaced width=%v, want %v", box[0].Bounds.Size.Width, 6*charW)
	}
	if math.Abs(float64(cjk.Origin.X-box[0].PageBounds.Origin.X)) > 1 {
		t.Fatalf("cjk-repl X=%v, want %v", cjk.Origin.X, box[0].PageBounds.Origin.X)
	}
	// Line 1: hidden "**" collapses, so '\t' starts at shown col 0 and takes 4 spaces.
	app.Find("text:    c")

	// Replacing a tab keeps its 3-column tab stop width.
	setDecos([]Decoration{
		{Range: rng(0, 1, 2), Replace: core.View{TestID: "tab-repl", Style: core.Style{FlexGrow: 1}}},
	})
	app.Settle()
	app.Find("text:a   [中文] b")
	tabBox := ref.RangeBounds(rng(0, 1, 2))
	if len(tabBox) != 1 || math.Abs(float64(tabBox[0].Bounds.Size.Width-core.Points(3*charW))) > 0.5 {
		t.Fatalf("tab replaced box=%+v, want width %v", tabBox, 3*charW)
	}
}

func TestShowWhitespacesWithShapes(t *testing.T) {
	var ref Ref
	var setDecos func([]Decoration)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		decos := core.NewSignalFunc(cx, []Decoration(nil), func(_, _ []Decoration) bool { return false })
		setDecos = decos.Set
		return Editor{
			Ref: &ref, TestID: "editor", ShowWhitespaces: true, TabSize: 4,
			DefaultValue: "- [ ] task\na\tb\n**x  y",
			Style:        core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Decorations:  decos.Get,
		}
	})

	countExact := func(text string) int {
		n := 0
		for _, v := range app.Query("text:" + text) {
			if v.Text == text {
				n++
			}
		}
		return n
	}

	// Before shapes: 3 single-space dots on line 0, 1 tab arrow on line 1, and "··" on line 2.
	if n := countExact("·"); n != 3 {
		t.Fatalf("got %d space dots before Replace, want 3", n)
	}
	app.Find("text:→")
	dotsBefore := app.Find("text:··").Bounds()

	// Replace "[ ]" and "\t", and hide "**" on line 2:
	// whitespace marks inside replaced ranges vanish and marks after hidden ranges shift reactively.
	setDecos([]Decoration{
		{Range: rng(0, 2, 5), Replace: core.View{TestID: "ws-chk", Style: core.Style{Width: core.Pt(12), Height: core.Pt(12)}}},
		{Range: rng(1, 1, 2), Replace: core.View{TestID: "ws-tab", Style: core.Style{Width: core.Pt(12), Height: core.Pt(12)}}},
		{Range: rng(2, 0, 2), Hidden: true},
	})
	app.Settle()

	if n := countExact("·"); n != 2 {
		t.Fatalf("got %d space dots with Replace over [ ], want 2 (none inside [ ])", n)
	}
	if n := countExact("→"); n != 0 {
		t.Fatalf("got %d tab arrows with Replace over tab, want 0", n)
	}
	dotsAfter := app.Find("text:··").Bounds()
	wantShift := core.Points(2 * ref.e.charW)
	if moved := dotsBefore.Origin.X - dotsAfter.Origin.X; math.Abs(float64(moved-wantShift)) > 1 {
		t.Fatalf("whitespace marks moved %v, want %v", moved, wantShift)
	}
}

func TestLeadingHiddenClickAndTextPress(t *testing.T) {
	var ref Ref
	var presses []TextPressEvent
	var sels []Selection
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return Editor{
			Ref: &ref, TestID: "editor",
			DefaultValue: "**bold**\na **bold** c",
			Style:        core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Decorations: func() []Decoration {
				return []Decoration{
					{Range: rng(0, 0, 2), Hidden: true},
					{Range: rng(0, 6, 8), Hidden: true},
					{Range: rng(1, 2, 4), Hidden: true},
					{Range: rng(1, 8, 10), Hidden: true},
				}
			},
			OnTextPress:       func(ev TextPressEvent) { presses = append(presses, ev) },
			OnSelectionChange: func(s []Selection) { sels = s },
		}
	}, nsgotest.Environment(core.Environment{Idiom: core.IdiomDesktop}))

	e := ref.e
	y0 := core.Points(0.5 * e.lh)
	y1 := core.Points(1.5 * e.lh)

	// Clicking col 0 of "**bold**" lands AFTER the leading hidden "**" (col 2, 'b').
	ptCol0 := core.Point{X: core.Points(pad + 0.1*e.charW), Y: y0}
	if got := e.posAt(ptCol0); got != (pos{0, 2}) {
		t.Fatalf("posAt col 0 of leading hidden = %+v, want {0 2}", got)
	}
	e.pressDown(ptCol0, 1, core.Modifiers{})
	e.textPress(ptCol0, core.Point{}, core.Modifiers{}, 1)
	if len(sels) != 1 || sels[0].Head != (Position{0, 2}) {
		t.Fatalf("caret after col 0 press = %+v, want {0 2}", sels)
	}
	if len(presses) != 1 || presses[0].Position != (Position{0, 2}) {
		t.Fatalf("press at col 0 = %+v, want {0 2}", presses)
	}

	// Clicking in left padding before leading hidden "**" reports no hidden character.
	presses = nil
	e.textPress(core.Point{X: core.Points(pad - 2), Y: y0}, core.Point{}, core.Modifiers{}, 1)
	if len(presses) != 0 {
		t.Fatalf("press in left padding reported %+v, want none", presses)
	}

	// Clicking right half of 'd' before trailing hidden "**" reports 'd' (col 5), not '*'.
	presses = nil
	e.textPress(core.Point{X: core.Points(pad + 3.8*e.charW), Y: y0}, core.Point{}, core.Modifiers{}, 1)
	if len(presses) != 1 || presses[0].Position != (Position{0, 5}) {
		t.Fatalf("press on right half of d = %+v, want {0 5}", presses)
	}

	// On "a **bold** c", clicking right half of ' ' (col 1) before mid-line hidden "**" reports ' ' (col 1), not '*'.
	presses = nil
	e.textPress(core.Point{X: core.Points(pad + 1.8*e.charW), Y: y1}, core.Point{}, core.Modifiers{}, 1)
	if len(presses) != 1 || presses[0].Position != (Position{1, 1}) {
		t.Fatalf("press on right half of space = %+v, want {1 1}", presses)
	}

	// Clicking right half of 'd' (col 7) before second mid-line hidden "**" reports 'd' (col 7), not '*'.
	presses = nil
	e.textPress(core.Point{X: core.Points(pad + 5.8*e.charW), Y: y1}, core.Point{}, core.Modifiers{}, 1)
	if len(presses) != 1 || presses[0].Position != (Position{1, 7}) {
		t.Fatalf("press on right half of line 1 d = %+v, want {1 7}", presses)
	}
	_ = app
}

func TestReactiveOverlaysFollowMarksRev(t *testing.T) {
	var ref Ref
	var hide func(bool)
	var defReqID int
	var compReqID int
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		hidden := core.NewSignal(cx, false)
		hide = hidden.Set
		return Editor{
			Ref: &ref, TestID: "editor", Label: "Markdown", Language: "markdown",
			DefaultValue: "a **bold** c",
			Style:        core.Style{Width: core.Pt(400), Height: core.Pt(200)},
			Styles: Styles{
				DefinitionLink:   core.Style{Background: core.RGB(0x0000ff)},
				InlineCompletion: core.Style{Color: core.RGB(0x888888)},
				CompletionMenu:   core.Style{Background: core.RGB(0xffffff)},
			},
			Decorations: func() []Decoration {
				if !hidden.Get() {
					return nil
				}
				return []Decoration{
					{Range: rng(0, 2, 4), Hidden: true},
					{Range: rng(0, 8, 10), Hidden: true},
				}
			},
			OnDefinitionRequest: func(r DefinitionRequest) { defReqID = r.ID },
			OnCompletionRequest: func(r CompletionRequest) { compReqID = r.ID },
		}
	})

	e := ref.e
	charW := e.charW
	ref.SetSelections([]Selection{{Anchor: Position{0, 12}, Head: Position{0, 12}}})
	ref.Focus()
	app.Settle()

	// Present a definition link on "c" (col 11..12).
	e.requestDefinition(pos{0, 11}, DefinitionHover)
	ref.PresentDefinitionLink(defReqID, rng(0, 11, 12))
	// Present an inline completion at the caret (col 12).
	e.inlineID = 42
	ref.PresentInlineCompletion(42, "tail()")
	app.Settle()

	findLinkX := func() driver.Points {
		t.Helper()
		wantColor := driver.RGBA(0x0000ff, 1)
		var walk func(*nsgotest.View) (driver.Points, bool)
		walk = func(v *nsgotest.View) (driver.Points, bool) {
			if v.Style.Background == wantColor && v.Frame.Size.Height == 1 {
				return v.Frame.Origin.X, true
			}
			for _, ch := range v.Children {
				if x, ok := walk(ch); ok {
					return x, true
				}
			}
			return 0, false
		}
		x, ok := walk(app.Find("id:editor-text"))
		if !ok {
			t.Fatal("mounted definition link view not found")
		}
		return x
	}

	linkBefore := findLinkX()
	inlineBefore := app.Find("text:tail()").Frame.Origin.X
	sinkBefore := app.Find("label:Markdown").Frame.Origin.X

	// Toggle Hidden on "**" without moving selection or editing text:
	// the mounted definition link, inlineView, and sinkView all shift left reactively.
	hide(true)
	app.Settle()

	linkAfter := findLinkX()
	if moved := linkBefore - linkAfter; math.Abs(float64(moved-driver.Points(4*charW))) > 1 {
		t.Fatalf("mounted linkRects view moved %v, want %v", moved, 4*charW)
	}
	inlineAfter := app.Find("text:tail()").Frame.Origin.X
	if moved := inlineBefore - inlineAfter; math.Abs(float64(moved-driver.Points(4*charW))) > 1 {
		t.Fatalf("inline completion moved %v, want %v", moved, 4*charW)
	}
	sinkAfter := app.Find("label:Markdown").Frame.Origin.X
	if moved := sinkBefore - sinkAfter; math.Abs(float64(moved-driver.Points(4*charW))) > 1 {
		t.Fatalf("sinkView moved %v, want %v", moved, 4*charW)
	}

	// Completion overlay anchor also shifts reactively when Hidden toggles off.
	ref.ShowCompletions()
	ref.PresentCompletions(compReqID, []CompletionItem{{Label: "completeMe"}}, false)
	app.Settle()
	compHiddenX := app.Find("text:completeMe").Bounds().Origin.X

	hide(false)
	app.Settle()
	compShownX := app.Find("text:completeMe").Bounds().Origin.X
	if moved := compShownX - compHiddenX; math.Abs(float64(moved-core.Points(4*charW))) > 1 {
		t.Fatalf("completion list moved %v, want %v", moved, 4*charW)
	}

	// Touch selection handles also shift reactively when Hidden toggles.
	e.longPress(core.Point{X: core.Points(pad + 11.5*charW), Y: core.Points(0.5 * e.lh)}, core.Point{})
	app.Settle()
	handleBefore := app.Find("id:selection-handle-start").Bounds().Origin.X

	hide(true)
	app.Settle()
	handleAfter := app.Find("id:selection-handle-start").Bounds().Origin.X
	if moved := handleBefore - handleAfter; math.Abs(float64(moved-core.Points(4*charW))) > 1 {
		t.Fatalf("selection handle moved %v, want %v", moved, 4*charW)
	}
}
