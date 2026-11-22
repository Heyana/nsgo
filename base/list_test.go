package base_test

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// listRows are the rows the list tests show.
var listRows = []string{"Apple", "Apricot", "Banana", "Blueberry", "Cherry"}

// rowsOf is a List of listRows' props, sized by the test.
func rowsOf(sel *base.Selection, activate func(int)) base.List {
	return base.List{TestID: "list", Label: "Fruits", Count: func() int { return len(listRows) }, Key: func(i int) any { return listRows[i] },
		RowLabel: func(i int) string { return listRows[i] }, Selection: sel, OnActivate: activate,
		Row: func(_ *core.Scope, r core.ListRow) core.Node {
			return core.Text{ContentFunc: func() string { return listRows[r.Index()] }}
		}, Style: core.Style{Height: core.Pt(300)}, ItemStyle: core.Style{Height: core.Pt(30)}}
}

// TestListHasNoLook leaves everything but layout's needs to the app, and
// reads what the app styles a row with from its states.
func TestListHasNoLook(t *testing.T) {
	selected := core.RGB(0x123456)
	var sel *base.Selection
	app := startDesktop(t, func(cx *core.Scope) core.Node {
		sel = base.NewSelection(cx, false)
		l := rowsOf(sel, nil)
		l.ItemStyle.Selected = &core.Style{Background: selected}
		return l
	})
	row := app.Find("label:Apple")
	if row.Style.Background != 0 || row.Style.CornerRadius != 0 {
		t.Fatalf("a row has a look of its own: %+v", row.Style)
	}
	app.Press("label:Banana")
	if got := app.Find("label:Banana").Style.Background; got != selected {
		t.Fatalf("a chosen row's background is %v, want the app's", got)
	}
}

// TestListRoles says a list of choices is one to assistive technology, its
// rows options, and rows that only activate are buttons.
func TestListRoles(t *testing.T) {
	app := startDesktop(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			func() core.Node {
				l := rowsOf(base.NewSelection(cx, false), nil)
				l.TestID, l.Label = "choices", "Choices"
				return l
			}(),
			func() core.Node {
				l := rowsOf(nil, func(int) {})
				l.TestID, l.Label = "actions", "Actions"
				return l
			}(),
		}}
	})
	app.Find("id:choices >> role:option Apple")
	app.Find("id:actions >> role:button Apple")
	if !app.Exists("role:listbox Choices") {
		t.Fatal("a list of choices is no listbox")
	}
	if app.Exists("role:listbox Actions") {
		t.Fatal("a list with no selection is a listbox")
	}
}

// TestListPlainRows takes no keys and no presses without a Selection or
// OnActivate.
func TestListPlainRows(t *testing.T) {
	app := startDesktop(t, func(cx *core.Scope) core.Node { return rowsOf(nil, nil) })
	if v := app.Find("label:Apple"); v.Props.Role != 0 {
		t.Fatalf("a plain row has role %v", v.Props.Role)
	}
}

// TestListTypeahead moves the cursor by what was typed, repeats of a letter
// cycling through its rows, and starts over after a pause.
func TestListTypeahead(t *testing.T) {
	var sel *base.Selection
	app := startDesktop(t, func(cx *core.Scope) core.Node {
		sel = base.NewSelection(cx, false)
		return rowsOf(sel, nil)
	})
	cursor := func(want string) {
		t.Helper()
		if got := sel.Keys(); !slices.Equal(got, []any{want}) {
			t.Fatalf("the selection is %v, want %s", got, want)
		}
	}
	app.Press("label:Cherry")
	app.Key("b")
	cursor("Banana")
	app.Key("b")
	cursor("Blueberry")
	app.Key("l")
	cursor("Blueberry")
	app.Advance(time.Second)
	app.Key("a")
	cursor("Apple")
	app.Key("p")
	cursor("Apple")
	app.Key("r")
	cursor("Apricot")
	app.Key("z")
	cursor("Apricot")
}

// TestListSelectionChange reports the rows a press or a key chooses, once each, and none of the app's own writes to the Selection.
func TestListSelectionChange(t *testing.T) {
	var sel *base.Selection
	var changes [][]any
	app := startDesktop(t, func(cx *core.Scope) core.Node {
		sel = base.NewSelection(cx, false)
		l := rowsOf(sel, nil)
		l.OnSelectionChange = func(s *base.Selection) { changes = append(changes, s.Keys()) }
		return l
	})
	step := selectionSteps(t, &sel, &changes)
	step("a press", func() { app.Press("label:Apple") }, "Apple")
	step("down", func() { app.Key("down") }, "Apricot")
	step("space on the chosen row", func() { app.Key("space") })
	app.Advance(time.Second)
	step("typeahead", func() { app.Key("c") }, "Cherry")
	app.Advance(time.Second)
	step("a press on the chosen row", func() { app.Press("label:Cherry") })
	step("the app's Set", post(app, func() { sel.Set("Banana") }))
	step("the app's Toggle", post(app, func() { sel.Toggle("Apple") }))
	step("the app's Clear", post(app, sel.Clear))
}

// TestListMultipleSelectionChange reports each press, modified press and key that changes a multiple selection's rows once, after the selection holds them.
func TestListMultipleSelectionChange(t *testing.T) {
	var sel *base.Selection
	var changes [][]any
	app := startDesktop(t, func(cx *core.Scope) core.Node {
		sel = base.NewSelection(cx, true)
		l := rowsOf(sel, nil)
		l.OnSelectionChange = func(s *base.Selection) { changes = append(changes, s.Keys()) }
		return l
	})
	press := func(label string, mods core.Modifiers) func() {
		return func() {
			row := app.Find("label:" + label)
			if !row.Events.Has(nsgotest.EventPress) {
				t.Fatalf("row %s takes no press", label)
			}
			app.Report(nsgotest.Event{Kind: nsgotest.EventPress, ID: row.ID, Count: 1, Modifiers: mods})
		}
	}
	step := selectionSteps(t, &sel, &changes)
	step("a press", func() { app.Press("label:Apple") }, "Apple")
	step("a Mod press", press("Cherry", core.Modifiers{Platform: true}), "Apple", "Cherry")
	step("a Control press", press("Apricot", core.Modifiers{Control: true}), "Apple", "Cherry", "Apricot")
	step("a Shift press", press("Banana", core.Modifiers{Shift: true}), "Apricot", "Banana")
	step("down", func() { app.Key("down") }, "Blueberry")
	step("shift+down", func() { app.Key("shift+down") }, "Blueberry", "Cherry")
	step("mod+a", func() { app.Key("mod+a") }, "Apple", "Apricot", "Banana", "Blueberry", "Cherry")
	step("mod+a with every row chosen", func() { app.Key("mod+a") })
	step("space", func() { app.Key("space") }, "Apple", "Apricot", "Banana", "Blueberry")
	app.Advance(time.Second)
	step("typeahead", func() { app.Key("b") }, "Banana")
	step("a Mod press after typeahead", press("Apricot", core.Modifiers{Platform: true}), "Banana", "Apricot")
	step("a Shift press that only reorders the rows", press("Banana", core.Modifiers{Shift: true}))
	step("the app's SetRange", post(app, func() { sel.SetRange("Apple", "Banana", "Apple", "Apricot", "Banana") }))
	step("the app's Toggle", post(app, func() { sel.Toggle("Cherry") }))
}

// post returns a step that runs fn on the app's goroutine, as the app's own write, and settles.
func post(app *nsgotest.App, fn func()) func() {
	return func() {
		app.Post(fn)
		app.Settle()
	}
}

// selectionSteps returns a step that runs do and checks that it reported want, the rows *sel then holds, once, or with no want nothing.
func selectionSteps(t *testing.T, sel **base.Selection, changes *[][]any) func(name string, do func(), want ...any) {
	return func(name string, do func(), want ...any) {
		t.Helper()
		n := len(*changes)
		do()
		got := (*changes)[n:]
		switch {
		case want == nil && len(got) > 0:
			t.Fatalf("%s reported %v", name, got)
		case want != nil && (len(got) != 1 || !slices.Equal(got[0], want)):
			t.Fatalf("%s reported %v, want %v once", name, got, want)
		case want != nil && !slices.Equal((*sel).Keys(), want):
			t.Fatalf("after %s the selection is %v, want %v", name, (*sel).Keys(), want)
		}
	}
}

// TestListScreenReader has the screen reader's focus follow the cursor as
// the keys move it.
func TestListScreenReader(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node { return rowsOf(base.NewSelection(cx, false), nil) },
		nsgotest.Environment(core.Environment{Idiom: core.IdiomDesktop, ScreenReader: true}))
	app.Press("label:Apple")
	app.Key("down")
	if v := app.AccessibilityFocus; v == nil || v.Props.Label != "Apricot" {
		t.Fatalf("the screen reader's focus is on %+v, want Apricot", v)
	}
}

// TestListWithoutLabels binds no typeahead, whose letters stay the app's.
func TestListWithoutLabels(t *testing.T) {
	var sel *base.Selection
	heard := 0
	app := startDesktop(t, func(cx *core.Scope) core.Node {
		sel = base.NewSelection(cx, false)
		cx.OnKey("b", func() { heard++ })
		l := rowsOf(sel, nil)
		l.RowLabel = nil
		return l
	})
	app.Press("text:Apple")
	app.Key("b")
	if heard != 1 || !slices.Equal(sel.Keys(), []any{"Apple"}) {
		t.Fatalf("b reached the app %d times, the selection is %v", heard, sel.Keys())
	}
}

// TestListTaps follows a touch screen: a tap activates unless the list is
// Multiple, which toggles, or TapSelects, whose second tap activates.
func TestListTaps(t *testing.T) {
	phone := nsgotest.Environment(core.Environment{Idiom: core.IdiomPhone})
	for _, c := range []struct {
		name               string
		multiple, tapFirst bool
		activates          []string
		keys               []any
	}{
		{name: "one", activates: []string{"0"}, keys: []any{"Apple"}},
		{name: "multiple", multiple: true, keys: []any{"Apple"}},
		{name: "tapSelects", tapFirst: true, keys: []any{"Apple"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var sel *base.Selection
			var activated []string
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				sel = base.NewSelection(cx, c.multiple)
				l := rowsOf(sel, func(i int) { activated = append(activated, strconv.Itoa(i)) })
				l.TapSelects = c.tapFirst
				return l
			}, phone)
			app.Press("label:Apple")
			if !slices.Equal(activated, c.activates) || !slices.Equal(sel.Keys(), c.keys) {
				t.Fatalf("a tap activated %v and chose %v", activated, sel.Keys())
			}
		})
	}
}

// TestListEmpty shows Empty with no rows, at the app's size.
func TestListEmpty(t *testing.T) {
	app := startDesktop(t, func(cx *core.Scope) core.Node {
		return base.List{TestID: "list", Empty: core.Text{Content: "Nothing"}, Style: core.Style{Height: core.Pt(100)}}
	})
	app.Find("text:Nothing")
	if got := app.Find("id:list").Frame.Size.Height; got != 100 {
		t.Fatalf("the list is %v tall, want the app's 100", got)
	}
}

// TestListInUnplacedContentLeavesTheKeys has a list that took the keys give them up while a Holder keeps it out of the tree, and take them again once it shows.
func TestListInUnplacedContentLeavesTheKeys(t *testing.T) {
	var sel *base.Selection
	var tab *core.Signal[int]
	page := 0
	app := startDesktop(t, func(cx *core.Scope) core.Node {
		tab = core.NewSignal(cx, 0)
		sel = base.NewSelection(cx, true)
		cx.OnKey("down", func() { page++ })
		cx.OnKey("mod+a", func() { page++ })
		pane := core.NewHolder(cx, func(cx *core.Scope) core.Node { return rowsOf(sel, nil) })
		return core.Dynamic(func(cx *core.Scope) core.Node {
			if tab.Get() == 0 {
				return pane.Place()
			}
			return core.Text{Content: "other"}
		})
	})
	app.Press("label:Apple")
	app.Key("down")
	if got := sel.Cursor(); got != "Apricot" {
		t.Fatalf("placed, the list's cursor is on %v, want Apricot", got)
	}
	app.Post(func() { tab.Set(1) })
	app.Settle()
	app.Key("down")
	app.Key("mod+a")
	if got := sel.Cursor(); got != "Apricot" || sel.Len() != 1 || page != 2 {
		t.Fatalf("unplaced, the list's cursor is on %v with %d selected and the page heard %d keys, want Apricot, 1 and 2", got, sel.Len(), page)
	}
	app.Post(func() { tab.Set(0) })
	app.Settle()
	app.Key("down")
	if got := sel.Cursor(); got != "Banana" || page != 2 {
		t.Fatalf("placed again, the list's cursor is on %v and the page heard %d keys, want Banana and 2", got, page)
	}
}
