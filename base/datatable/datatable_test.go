package datatable_test

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/base/datatable"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func tableColumns(n int) []datatable.Column {
	cols := make([]datatable.Column, n)
	for i := range cols {
		cols[i] = datatable.Column{Key: fmt.Sprint("c", i), Title: fmt.Sprint("Col ", i), Width: 100}
	}
	cols[0].Fixed = true
	cols[1].Sortable = true
	return cols
}

// The grid is the table's only Tab stop; its pointer hit layer is not one.
func TestDataTableIsOneTabStop(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			datatable.DataTable{
				TestID:   "table",
				Label:    "Quotes",
				Columns:  func() []datatable.Column { return tableColumns(3) },
				RowCount: func() int { return 10 },
				Text:     func(row, col int) string { return fmt.Sprintf("R%dC%d", row, col) },
				Style:    core.Style{Width: core.Pt(300), Height: core.Pt(200)},
			},
			core.View{TestID: "after", Role: core.RoleButton, Label: "After", OnPress: func(core.PressEvent) {}, Style: core.Style{Width: core.Pt(80), Height: core.Pt(30)}},
		}}
	}, nsgotest.Size(400, 400), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.Key("tab")
	if !app.Find("id:table").Focused {
		t.Fatal("Tab did not focus the table's grid")
	}
	// The table takes Tab itself to move between cells, so reach it backwards from the next control: Shift+Tab lands on the grid, not on the hit layer after it in tree order.
	app.Focus("id:after")
	app.Key("shift+tab")
	if !app.Find("id:table").Focused {
		t.Fatal("Shift+Tab from the next control did not reach the table's grid")
	}
}

func TestDataTableMakesOnlyTheCellsInView(t *testing.T) {
	var (
		sel     []datatable.Selection
		sorts   []datatable.Sort
		rows    [2]int
		resized []string
		ref     datatable.Ref
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return datatable.DataTable{
			TestID:              "table",
			Label:               "Quotes",
			Ref:                 &ref,
			Columns:             func() []datatable.Column { return tableColumns(60) },
			RowCount:            func() int { return 1_000_000 },
			Text:                func(row, col int) string { return fmt.Sprintf("R%dC%d", row, col) },
			Style:               core.Style{Width: core.Pt(400), Height: core.Pt(332)},
			Styles:              datatable.Styles{RowSelected: core.Style{Background: core.RGB(0x0000ff)}},
			OnSelectionChange:   func(s datatable.Selection) { sel = append(sel, s) },
			OnSortChange:        func(s datatable.Sort) { sorts = append(sorts, s) },
			OnVisibleRowsChange: func(s, e int) { rows = [2]int{s, e} },
			OnColumnResize:      func(key string, w core.Points) { resized = append(resized, fmt.Sprint(key, " ", w)) },
		}
	}, nsgotest.Size(400, 400), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	if v := app.Find("id:table"); v.Props.Role != core.RoleGrid || v.Props.Label != "Quotes" {
		t.Fatalf("table role %v label %q, want a grid", v.Props.Role, v.Props.Label)
	}
	app.Press("text:R3C2")
	focused := app.Find("id:table")
	if !focused.Focused || focused.Props.Role != core.RoleGrid || focused.Props.Label != "Quotes" {
		t.Fatalf("focused table = focused %v, role %v, label %q; want the named grid", focused.Focused, focused.Props.Role, focused.Props.Label)
	}
	cells := len(app.Query("role:cell"))
	if cells == 0 || cells > 400 {
		t.Fatalf("%d cells made, want the window's only", cells)
	}
	// 300 points under a 32-point header: rows 0 to 9 in view.
	if rows != [2]int{0, 10} {
		t.Errorf("visible rows %v, want 0 to 10", rows)
	}
	app.Find("text:R0C0")
	app.Press("text:R3C2")
	if len(sel) != 1 || sel[0] != (datatable.Selection{Row: 3, Column: -1}) {
		t.Fatalf("selection %v, want row 3", sel)
	}
	app.Key("down")
	if sel[len(sel)-1].Row != 4 {
		t.Errorf("down selected %v, want row 4", sel[len(sel)-1])
	}
	// Far down a million rows: the rows there show, scaled.
	ref.ScrollToRow(777_777, false)
	app.Settle()
	app.Find("text:R777777C0")
	if rows[0] != 777_777 {
		t.Errorf("visible rows %v after scrolling to 777777", rows)
	}
	app.Press("id:sort:c1")
	app.Press("id:sort:c1")
	if !slices.Equal(sorts, []datatable.Sort{{Key: "c1"}, {Key: "c1"}}) {
		t.Errorf("sorts %v: the app keeps the sort, so each press asks for ascending", sorts)
	}
	app.Drag("id:resize:c1", core.Point{X: 30}, core.Point{X: 60})
	if len(resized) == 0 || resized[len(resized)-1] != "c1 160" {
		t.Errorf("resized %v, want c1 at 160", resized)
	}
	if d := ref.Dump(0, 2); len(d.Rows) != 2 || d.Headers[0] != "Col 0" || d.Rows[1][1] != "R1C1" {
		t.Errorf("dump %+v", d)
	}
}

func TestDataTableCellTooltip(t *testing.T) {
	desktop := nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop})
	texts := []string{"Shared", "Private"}
	tips := []string{"Shared with 3 people", ""}
	table := func(render bool, selected *datatable.Selection) func(cx *core.Scope) core.Node {
		return func(cx *core.Scope) core.Node {
			p := datatable.DataTable{
				Columns:           func() []datatable.Column { return []datatable.Column{{Key: "name", Width: 200}} },
				RowCount:          func() int { return len(texts) },
				Text:              func(row, _ int) string { return texts[row] },
				CellTooltip:       func(row, _ int) string { return tips[row] },
				OnSelectionChange: func(s datatable.Selection) { *selected = s },
				Styles:            datatable.Styles{CellTooltip: core.Style{Padding: core.All(4)}},
				Style:             core.Style{Width: core.Pt(300), Height: core.Pt(200)},
			}
			if render {
				// What RenderCell draws takes no pointer: the table shows the tooltip it is given.
				p.RenderCell = func(_ *core.Scope, cell func() datatable.Cell) core.Node {
					return core.Text{ContentFunc: func() string { return cell().Text + "|" + cell().Tooltip }}
				}
			}
			return p
		}
	}
	for _, render := range []bool{false, true} {
		var selected datatable.Selection
		app := nsgotest.Start(t, table(render, &selected), desktop, nsgotest.Size(400, 300))
		shared, private := "Shared", "Private"
		if render {
			shared, private = "Shared|Shared with 3 people", "Private|"
		}
		// The tooltip waits for the pointer to rest, as a View's does.
		app.Hover(shared)
		app.Advance(300 * time.Millisecond)
		if app.Exists("Shared with 3 people") {
			t.Fatalf("render=%v: the tooltip showed before the pointer rested", render)
		}
		app.Advance(300 * time.Millisecond)
		app.Find("Shared with 3 people")
		// It is the cell's description, and the other cell has none.
		var described []string
		for _, c := range app.QueryMounted("role:cell") {
			if c.Props.Description != "" {
				described = append(described, c.Props.Description)
			}
		}
		if !slices.Equal(described, []string{"Shared with 3 people"}) {
			t.Fatalf("render=%v: cell descriptions = %q", render, described)
		}
		// A press hides it and still selects the cell's row.
		app.Press(shared)
		if app.Exists("Shared with 3 people") {
			t.Fatalf("render=%v: the tooltip stayed after a press", render)
		}
		if selected.Row != 0 {
			t.Fatalf("render=%v: selected = %+v, want row 0", render, selected)
		}
		// A cell without a tooltip shows none, and leaving the table hides it.
		app.Hover(private)
		app.Advance(time.Second)
		if app.Exists("Shared with 3 people") {
			t.Fatalf("render=%v: another cell showed the tooltip", render)
		}
		app.Hover(shared)
		app.Advance(time.Second)
		app.Find("Shared with 3 people")
		app.HoverOut()
		app.Advance(time.Second)
		if app.Exists("Shared with 3 people") {
			t.Fatalf("render=%v: the tooltip stayed after the pointer left", render)
		}
	}
}

func TestDataTableSelectsCellsMovesColumnsAndCopies(t *testing.T) {
	var (
		moves []datatable.ColumnMove
		menus []datatable.ContextMenu
		sel   datatable.Selection
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return datatable.DataTable{
			TestID:     "table",
			CellSelect: func() bool { return true },
			Columns: func() []datatable.Column {
				return []datatable.Column{{Key: "a", Title: "A"}, {Key: "b", Title: "B"}, {Key: "c", Title: "C"}}
			},
			Groups: func() [][]datatable.ColumnGroup {
				return [][]datatable.ColumnGroup{{{Label: "Both", Span: 2}, {Label: "One"}}}
			},
			RowCount:          func() int { return 5 },
			Text:              func(row, col int) string { return fmt.Sprintf("%c%d", 'a'+col, row) },
			Style:             core.Style{Width: core.Pt(500), Height: core.Pt(300)},
			OnSelectionChange: func(s datatable.Selection) { sel = s },
			OnColumnMove:      func(m datatable.ColumnMove) { moves = append(moves, m) },
			OnContextMenu:     func(m datatable.ContextMenu) { menus = append(menus, m) },
		}
	}, nsgotest.Size(500, 300))
	app.Find("text:Both")
	// Row numbers show with cell selection.
	app.Find("text:5")
	app.Press("text:b2")
	if sel != (datatable.Selection{Row: 2, Column: 1}) {
		t.Fatalf("selection %v, want the cell b2", sel)
	}
	app.Key("right")
	app.Key("mod+c")
	if app.Clipboard != "c2" {
		t.Errorf("copied %q, want c2", app.Clipboard)
	}
	app.Drag("text:A", core.Point{X: 40}, core.Point{X: 160})
	if len(moves) != 1 || moves[0].Key != "a" || !slices.Equal(moves[0].Order, []string{"b", "a", "c"}) {
		t.Errorf("moves %+v, want a after b", moves)
	}
	app.RightClick("text:c4")
	if len(menus) != 1 || menus[0].Row != 4 || menus[0].Column != 2 {
		t.Errorf("menus %+v, want c4's", menus)
	}
}

func TestDataTableSelectsMultipleRowsAndExtendsWithShift(t *testing.T) {
	var (
		ref     datatable.Ref
		other   core.Ref
		changes [][]int
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			datatable.DataTable{
				TestID:               "table",
				Ref:                  &ref,
				MultiSelect:          true,
				Columns:              func() []datatable.Column { return []datatable.Column{{Key: "name"}} },
				RowCount:             func() int { return 6 },
				Text:                 func(row, _ int) string { return fmt.Sprintf("R%d", row) },
				Style:                core.Style{Width: core.Pt(300), Height: core.Pt(240)},
				OnSelectedRowsChange: func(rows []int) { changes = append(changes, append([]int(nil), rows...)) },
			},
			base.Button{Ref: &other, TestID: "other", Label: "Other", Style: core.Style{Height: core.Pt(40)}},
		}}
	}, nsgotest.Size(300, 300), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.Press("text:R1")
	app.PressWith("text:R3", driver.Modifiers{Shift: true})
	if got := ref.SelectedRows(); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("selected rows %v, want 1 through 3", got)
	}
	if len(changes) != 2 || !slices.Equal(changes[1], []int{1, 2, 3}) {
		t.Fatalf("selection changes %v", changes)
	}
	for _, row := range []int{1, 2, 3} {
		v := app.Find(fmt.Sprintf("text:R%d", row))
		for v != nil && v.Props.Role != core.RoleRow {
			v = v.Parent
		}
		if v == nil || !v.Props.Selected {
			t.Errorf("row %d is not selected to accessibility", row)
		}
	}
	app.Key("shift+down")
	if got := ref.SelectedRows(); !slices.Equal(got, []int{1, 2, 3, 4}) {
		t.Fatalf("shift down selected rows %v, want 1 through 4", got)
	}
	app.PressWith("text:R2", driver.Modifiers{Platform: true})
	if got := ref.SelectedRows(); !slices.Equal(got, []int{1, 3, 4}) {
		t.Fatalf("command click selected rows %v, want row 2 toggled off", got)
	}
	app.Press("id:other")
	other.Focus()
	app.Settle()
	if !app.Find("id:other").Focused {
		t.Fatal("other control did not take focus")
	}
	app.Key("shift+down")
	if got := ref.SelectedRows(); !slices.Equal(got, []int{1, 3, 4}) {
		t.Fatalf("other control changed selected rows to %v", got)
	}
}

// TestDataTableMultiSelectKeepsToRowsAndFollowsTheApp holds a multi-row table off Apple's keys with cell selection asked for: Control toggles, cells and titles do not select, the app's rows and Ref.SelectRows show and anchor the selection, the copy has every selected row, and Escape clears the rows before it gives up the keyboard.
func TestDataTableMultiSelectKeepsToRowsAndFollowsTheApp(t *testing.T) {
	var (
		ref     datatable.Ref
		version *core.Signal[int]
		given   = []int{4, 0, 9}
		changes [][]int
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		version = core.NewSignal(cx, 0)
		return datatable.DataTable{
			TestID:      "table",
			Ref:         &ref,
			MultiSelect: true,
			CellSelect:  func() bool { return true },
			Columns: func() []datatable.Column {
				return []datatable.Column{{Key: "a", Title: "A"}, {Key: "b", Title: "B"}}
			},
			RowCount:             func() int { return 6 },
			Text:                 func(row, col int) string { return fmt.Sprintf("%c%d", 'a'+col, row) },
			Style:                core.Style{Width: core.Pt(300), Height: core.Pt(300)},
			SelectedRows:         func() []int { version.Get(); return given },
			OnSelectedRowsChange: func(rows []int) { changes = append(changes, append([]int(nil), rows...)) },
		}
	}, nsgotest.Size(300, 300), nsgotest.Environment(core.Environment{OS: core.OSWindows, Idiom: core.IdiomDesktop}))
	rowSelected := func(text string) bool {
		v := app.Find("text:" + text)
		for v != nil && v.Props.Role != core.RoleRow {
			v = v.Parent
		}
		return v != nil && v.Props.Selected
	}
	if got := ref.SelectedRows(); !slices.Equal(got, []int{0, 4}) {
		t.Fatalf("app's rows %v, want 0 and 4 (9 is past the end)", got)
	}
	if !rowSelected("a0") || !rowSelected("a4") || rowSelected("a2") {
		t.Error("the app's rows are not the ones selected to accessibility")
	}
	if len(app.Query("text:6")) != 0 {
		t.Error("row numbers show though cells do not select in MultiSelect mode")
	}
	given = []int{1}
	version.Set(1)
	app.Settle()
	if got := ref.SelectedRows(); !slices.Equal(got, []int{1}) {
		t.Fatalf("app's new rows %v, want 1", got)
	}
	app.Press("text:b2")
	app.PressWith("text:a3", driver.Modifiers{Control: true})
	if got := ref.SelectedRows(); !slices.Equal(got, []int{2, 3}) {
		t.Fatalf("rows %v after a press and a Control press, want 2 and 3", got)
	}
	if s := ref.Selection(); s != (datatable.Selection{Row: 3, Column: -1}) {
		t.Errorf("selection %v, want the keyboard on row 3, not a cell", s)
	}
	app.Press("text:A")
	if s := ref.Selection(); s.Column >= 0 {
		t.Errorf("a title press selected %v, want no column in MultiSelect mode", s)
	}
	ref.SelectRows([]int{3, 1})
	app.Key("shift+down")
	if got := ref.SelectedRows(); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("shift down after SelectRows(3, 1) selected %v, want 1 and 2 from the anchor 1", got)
	}
	app.Key("mod+c")
	if app.Clipboard != "a1\tb1\na2\tb2" {
		t.Errorf("copied %q, want rows 1 and 2", app.Clipboard)
	}
	n := len(changes)
	app.Key("escape")
	if got := ref.SelectedRows(); len(got) != 0 || len(changes) != n+1 || len(changes[n]) != 0 {
		t.Fatalf("escape left rows %v, changes %v", got, changes[n:])
	}
	app.Key("escape")
	app.Key("shift+down")
	if got := ref.SelectedRows(); len(got) != 0 {
		t.Errorf("shift down selected %v after Escape gave up the keyboard", got)
	}
}

// TestDataTableReadsTheCellsNearTheView scrolls and ticks a table whose
// texts change: the rows made are those in view and half a viewport
// around, so a scroll reads the rows coming near the view and a tick the
// cells near it.
func TestDataTableReadsTheCellsNearTheView(t *testing.T) {
	var (
		tick  *core.Signal[int]
		reads int
		rows  [2]int
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		tick = core.NewSignal(cx, 0)
		return datatable.DataTable{
			TestID:   "table",
			Columns:  func() []datatable.Column { return tableColumns(60) },
			RowCount: func() int { return 1_000_000 },
			Text: func(row, col int) string {
				reads++
				return fmt.Sprintf("R%dC%d·%d", row, col, tick.Get())
			},
			Style:               core.Style{Width: core.Pt(400), Height: core.Pt(332)},
			OnVisibleRowsChange: func(s, e int) { rows = [2]int{s, e} },
		}
	}, nsgotest.Size(400, 400))
	// 10 rows of 4 or 5 columns in view, one more each way, and 5 rows
	// made each way. A million rows scale the scroll view's offsets: 6
	// points scroll a row or two.
	app.Scroll("id:data-table-scroll", 60)
	from := rows[0]
	reads = 0
	app.Scroll("id:data-table-scroll", 6)
	if reads == 0 || reads > 3*7 {
		t.Errorf("a scroll of a row or two read %d texts, want the rows coming near the view's", reads)
	}
	reads = 0
	tick.Set(1)
	app.Settle()
	if reads == 0 || reads > 22*7 {
		t.Errorf("a tick read %d texts, want those near the view", reads)
	}
	if n := len(app.Query(fmt.Sprintf("text:R%dC1·1", from+40))); n != 0 {
		t.Errorf("row %d, past the rows made, shows %d texts", from+40, n)
	}
	app.Find(fmt.Sprintf("text:R%dC1·1", rows[0]+1))
	app.Scroll("id:data-table-scroll", 3000)
	if rows[0] < 10 {
		t.Fatalf("visible rows %v after scrolling", rows)
	}
	app.Find(fmt.Sprintf("text:R%dC1·1", rows[0]+1))
}

// TestDataTableScrollsByDrag scrolls the table's own scroll view, over
// the body, which takes no pointer.
func TestDataTableScrollsByDrag(t *testing.T) {
	var ref datatable.Ref
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return datatable.DataTable{
			TestID:   "table",
			Ref:      &ref,
			Columns:  func() []datatable.Column { return tableColumns(20) },
			RowCount: func() int { return 10_000 },
			Text:     func(row, col int) string { return fmt.Sprintf("R%dC%d", row, col) },
			Style:    core.Style{Width: core.Pt(400), Height: core.Pt(332)},
		}
	}, nsgotest.Size(400, 400), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	from, _ := ref.VisibleRows()
	app.Scroll("id:data-table-scroll", 120)
	app.Settle()
	if to, _ := ref.VisibleRows(); to <= from {
		t.Fatalf("visible rows %v after scrolling, want past %v", to, from)
	}
}

// TestDataTableHitLayerLiesOverTheBody keeps the scroll views above the
// rows' backgrounds, so a selected row cannot cover the bar.
func TestDataTableHitLayerLiesOverTheBody(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return datatable.DataTable{TestID: "table",
			Columns: func() []datatable.Column { return tableColumns(3) }, RowCount: func() int { return 100 },
			Text:  func(row, col int) string { return fmt.Sprintf("R%dC%d", row, col) },
			Style: core.Style{Width: core.Pt(400), Height: core.Pt(332)}}
	}, nsgotest.Size(400, 400))
	scroll := app.Find("id:data-table-scroll")
	over := scroll.Parent.Children[slices.Index(scroll.Parent.Children, scroll)+1:]
	cell := app.Find(`text:"R0C0"`)
	for cell.Parent != scroll.Parent {
		cell = cell.Parent
	}
	if slices.Contains(over, cell) || cell == scroll {
		t.Fatal("cells lie over the hit layer")
	}
}

// TestDataTableReadsRowsAColumnAtATime shows typed columns by key from
// DataStart, gaps as nothing, and rises and falls from numbers.
func TestDataTableReadsRowsAColumnAtATime(t *testing.T) {
	rise, fall := core.RGB(0x00ff00), core.RGB(0xff0000)
	rows := datatable.NewRows(3).Texts("name", []string{"Ann", "Bob", "Cy"}).
		Numbers("change", []float64{1.5, -2, math.NaN()}).Bools("open", []bool{true, false})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return datatable.DataTable{
			Columns: func() []datatable.Column {
				return []datatable.Column{{Key: "name"}, {Key: "open"}, {Key: "missing"},
					{Key: "change", PositiveStyle: core.Style{Color: rise}, NegativeStyle: core.Style{Color: fall}}}
			},
			Data: func() *datatable.Rows { return rows }, DataStart: func() int { return 2 },
			Style: core.Style{Width: core.Pt(600), Height: core.Pt(332)}}
	}, nsgotest.Size(600, 400))
	for _, want := range []string{`text:"Ann"`, `text:"true"`, `text:"false"`, `text:"1.5"`, `text:"-2"`} {
		app.Find(want)
	}
	if rows.Len() != 3 || rows.Text(1, "change") != "-2" || rows.Text(2, "change") != "" || rows.Text(2, "open") != "" || rows.Text(9, "name") != "" {
		t.Fatalf("cells read %q %q %q", rows.Text(1, "change"), rows.Text(2, "change"), rows.Text(2, "open"))
	}
	if got := app.Find(`text:"-2"`).TextStyle.Color; got != fall {
		t.Fatalf("a fall's color is %v", got)
	}
}

// pageX is where v starts across the surface, the scroll offsets around
// it applied.
func pageX(v *nsgotest.View) float32 {
	x := float32(0)
	for p := v; p != nil; p = p.Parent {
		x += float32(p.Frame.Origin.X)
		if p.Parent != nil {
			x -= float32(p.Parent.Offset.X)
		}
	}
	return x
}

// A column fixed at the right stays against the table's right edge while
// the others scroll under it, after them in the order.
func TestDataTableColumnFixedAtTheRight(t *testing.T) {
	var ref datatable.Ref
	var pressed []datatable.Selection
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return datatable.DataTable{
			TestID: "table", Ref: &ref, CellSelect: func() bool { return true }, NoRowHeader: true,
			Columns: func() []datatable.Column {
				cols := tableColumns(20)
				cols[19].FixedRight = true
				return cols
			},
			RowCount:          func() int { return 100 },
			Text:              func(row, col int) string { return fmt.Sprintf("R%dC%d", row, col) },
			Style:             core.Style{Width: core.Pt(400), Height: core.Pt(332)},
			OnSelectionChange: func(s datatable.Selection) { pressed = append(pressed, s) },
		}
	}, nsgotest.Size(400, 400))
	right := pageX(app.Find("text:R0C19").Parent)
	if right != 300 {
		t.Fatalf("the right column starts at %v, want against the right edge (300)", right)
	}
	ref.ScrollToColumn("c15", false)
	app.Settle()
	if x := pageX(app.Find("text:R0C19").Parent); x != right {
		t.Errorf("the right column moved to %v as the others scrolled", x)
	}
	shown := false
	for _, v := range app.Query("text:R0C15") {
		if x := pageX(v.Parent); x >= 100 && x+100 <= 300 {
			shown = true
		}
	}
	if !shown {
		t.Error("column 15 is not between the fixed columns")
	}
	app.Press("text:R2C19")
	if len(pressed) == 0 || pressed[len(pressed)-1] != (datatable.Selection{Row: 2, Column: 19}) {
		t.Errorf("a press on the right column selected %v", pressed)
	}
}

// A sortable column's sort control takes a tap at its center on a phone,
// where the adjacent resize handle's touch slop extends left from the
// trailing edge; the handle itself still resizes from its band, and on a
// non-sortable column from its slop too.
func TestDataTableSortControlTakesATapOnPhones(t *testing.T) {
	var (
		ref     datatable.Ref
		sorts   []datatable.Sort
		sel     []datatable.Selection
		resized []string
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		sort := core.NewSignal(cx, datatable.Sort{Key: "symbol"})
		return datatable.DataTable{
			TestID: "table", Ref: &ref, NoRowHeader: true,
			Columns: func() []datatable.Column {
				return []datatable.Column{
					{Key: "symbol", Title: "Symbol", Width: 96, Sortable: true, Fixed: true},
					{Key: "price", Title: "Price", Width: 96, Sortable: true, Align: datatable.AlignEndColumn},
					{Key: "plain", Title: "Plain", Width: 96},
					{Key: "volume", Title: "Volume", Width: 110, Sortable: true, Align: datatable.AlignEndColumn},
					{Key: "total", Title: "Total", Width: 96, Sortable: true, FixedRight: true},
				}
			},
			RowCount:          func() int { return 20 },
			Text:              func(row, col int) string { return fmt.Sprintf("R%dC%d", row, col) },
			Sort:              sort.Get,
			OnSortChange:      func(s datatable.Sort) { sort.Set(s); sorts = append(sorts, s) },
			OnSelectionChange: func(s datatable.Selection) { sel = append(sel, s) },
			OnColumnResize:    func(key string, w core.Points) { resized = append(resized, fmt.Sprint(key, " ", w)) },
			Style:             core.Style{Width: core.Pt(356), Height: core.Pt(300)},
		}
	}, nsgotest.Size(390, 844), nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}))

	tapCenter := func(selector string) {
		v := app.Find(selector)
		app.PressAt(selector, core.Point{X: v.Frame.Size.Width / 2, Y: v.Frame.Size.Height / 2})
	}

	// Pinned left column: center and trailing edge of the 20pt sort button
	// must sort, not hit the resize handle's touch slop.
	tapCenter("id:sort:symbol")
	if len(sorts) != 1 || sorts[0] != (datatable.Sort{Key: "symbol", Descending: true}) {
		t.Fatalf("after tapping pinned sort button: sorts = %v, want [{symbol true}]", sorts)
	}
	symSort := app.Find("id:sort:symbol")
	app.PressAt("id:sort:symbol", core.Point{X: symSort.Frame.Size.Width - 1, Y: symSort.Frame.Size.Height / 2})
	if len(sorts) != 2 || sorts[1] != (datatable.Sort{}) {
		t.Fatalf("after tapping trailing edge of pinned sort button: sorts = %v, want unsorted", sorts)
	}

	// Scrolling column: center of its sort button must sort too.
	tapCenter("id:sort:price")
	if len(sorts) != 3 || sorts[2] != (datatable.Sort{Key: "price", Descending: false}) {
		t.Fatalf("after tapping scrolling sort button: sorts = %v, want price ascending", sorts)
	}

	// Right-pinned column: center of its sort button must sort.
	tapCenter("id:sort:total")
	if len(sorts) != 4 || sorts[3] != (datatable.Sort{Key: "total", Descending: false}) {
		t.Fatalf("after tapping right-pinned sort button: sorts = %v, want total ascending", sorts)
	}

	// After horizontal scroll, the newly revealed scrolling column's sort button must sort.
	ref.ScrollToColumn("volume", false)
	app.Settle()
	tapCenter("id:sort:volume")
	if len(sorts) != 5 || sorts[4] != (datatable.Sort{Key: "volume", Descending: false}) {
		t.Fatalf("after scrolling and tapping volume sort button: sorts = %v, want volume ascending", sorts)
	}

	// Tapping the title text still hits the header cell and selects the column.
	app.PressAt("text:Symbol", core.Point{X: 10, Y: 8})
	if len(sel) == 0 || sel[len(sel)-1] != (datatable.Selection{Row: -1, Column: 0}) {
		t.Fatalf("tapping header label selected %v, want column 0", sel)
	}

	// On a non-sortable column, a tap inside the resize handle's 10pt touch
	// slop passes through the label to the handle rather than selecting the column.
	selBefore := len(sel)
	plainHeader := app.Find("text:Plain").Parent.Parent
	app.PressAt("id:resize:plain", core.Point{X: -5, Y: plainHeader.Frame.Size.Height / 2})
	if len(sel) != selBefore {
		t.Fatalf("tapping non-sortable resize handle touch slop selected %v", sel[selBefore:])
	}

	// The resize handle itself still accepts drags on sortable columns.
	app.Drag("id:resize:symbol", core.Point{X: 20})
	if len(resized) == 0 || resized[len(resized)-1] != "symbol 116" {
		t.Fatalf("dragging sortable column resize handle: resized = %v, want symbol 116", resized)
	}
}
