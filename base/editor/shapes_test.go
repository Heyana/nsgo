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
	app.Settle()
	app.Find("id:check")
	if n := len(app.Query("text:- [ ] task")); n != 0 {
		t.Fatalf("the replaced text still shows in %d rows", n)
	}

	// The view keeps the range's columns, so the word after it stays put.
	after := ref.RangeBounds(rng(0, 6, 10))
	if len(after) != 1 {
		t.Fatalf("%d boxes for the word while replaced", len(after))
	}
	if moved := before[0].Bounds.Origin.X - after[0].Bounds.Origin.X; math.Abs(float64(moved)) > 0.5 {
		t.Fatalf("the word moved %v, want it where it was", moved)
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
