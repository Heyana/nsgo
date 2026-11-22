package tui

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func fixture() (tr *tree, events *[]driver.Event) {
	events = &[]driver.Event{}
	tr = newTree(func(e driver.Event) { *events = append(*events, e) })
	tr.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView}, {Kind: driver.ItemFrame, ID: 1, Frame: driver.Bounds{Size: driver.Size{Width: 320, Height: 160}}},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindInput}, {Kind: driver.ItemInsert, ID: 2, Parent: 1}, {Kind: driver.ItemFrame, ID: 2, Frame: driver.Bounds{Size: driver.Size{Width: 160, Height: 32}}},
		{Kind: driver.ItemInput, ID: 2, Input: &driver.InputConfig{Content: driver.Bounds{Size: driver.Size{Width: 160, Height: 32}}}},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView}, {Kind: driver.ItemInsert, ID: 3, Parent: 1}, {Kind: driver.ItemFrame, ID: 3, Frame: driver.Bounds{Origin: driver.Point{Y: 48}, Size: driver.Size{Width: 80, Height: 32}}}, {Kind: driver.ItemEvents, ID: 3, Events: 1 << driver.EventPress},
	}})
	return
}

func TestCellInputUnicodeSelectionAndStaleValues(t *testing.T) {
	tr, events := fixture()
	v := tr.views[2]
	tr.setFocus(v)
	tr.key("a", "A界é👩‍💻", driver.Modifiers{}, false)
	tr.key("backspace", "", driver.Modifiers{}, false)
	if v.value != "A界é" || v.selection.End != 4 {
		t.Fatalf("emoji deletion: %q %+v", v.value, v.selection)
	}
	tr.key("backspace", "", driver.Modifiers{}, false)
	if v.value != "A界" {
		t.Fatalf("combining cluster deletion: %q", v.value)
	}
	tr.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{{Kind: driver.ItemInputValue, ID: 2, Value: "stale", Count: 0}}})
	if v.value != "A界" {
		t.Fatalf("stale value replaced newer edit: %q", v.value)
	}
	tr.key("a", "", driver.Modifiers{Control: true}, false)
	tr.key("x", "x", driver.Modifiers{}, false)
	if v.value != "x" {
		t.Fatalf("replace selection: %q", v.value)
	}
	v.input.MaxLength = 3
	tr.replace(v, "👩‍💻")
	if v.value != "x👩" {
		t.Fatalf("maxLength must preserve runes within UTF16 limit: %q", v.value)
	}
	changes := 0
	for _, e := range *events {
		if e.Kind == driver.EventChangeText {
			changes++
			if e.Count != uint32(changes) {
				t.Fatalf("edit counter: %+v", e)
			}
		}
	}
}

func TestCellFocusKeymapAndDisabledControls(t *testing.T) {
	tr, events := fixture()
	tr.tab(false)
	if tr.focus != tr.views[2] {
		t.Fatal("first Tab did not focus editor")
	}
	tr.keys = []driver.KeyBinding{{Key: "enter", ID: 7, Yields: driver.KeyUsePress}, {Key: "mod+k", ID: 9, InText: true}}
	tr.key("k", "k", driver.Modifiers{Control: true}, false)
	if last := (*events)[len(*events)-1]; last.Kind != driver.EventKey || last.Count != 9 {
		t.Fatalf("Control mod binding: %+v", last)
	}
	tr.key("tab", "", driver.Modifiers{}, false)
	if tr.focus != tr.views[3] {
		t.Fatal("Tab did not focus button")
	}
	tr.key("enter", "", driver.Modifiers{}, false)
	if last := (*events)[len(*events)-1]; last.Kind != driver.EventPress || last.ID != 3 {
		t.Fatalf("KeyAuto did not yield to button: %+v", last)
	}
	tr.keys = append(tr.keys, driver.KeyBinding{Key: "enter", ID: 10})
	tr.key("enter", "", driver.Modifiers{}, false)
	if last := (*events)[len(*events)-1]; last.Kind != driver.EventKey || last.Count != 10 {
		t.Fatalf("KeyAlways did not win: %+v", last)
	}
	tr.views[3].props.Disabled = true
	tr.tab(false)
	if tr.focus != tr.views[2] {
		t.Fatal("disabled button remained in Tab order")
	}
	tr.views[2].input.Readonly = true
	tr.replace(tr.views[2], "bad")
	if tr.views[2].value != "" {
		t.Fatal("readonly field changed")
	}
}

func TestCellPointerClipsTransformsAndScrolls(t *testing.T) {
	tr, events := fixture()
	tr.mouseDown(driver.Point{X: 8, Y: 56}, driver.Modifiers{})
	tr.mouseUp(driver.Point{X: 8, Y: 56}, driver.Modifiers{})
	if last := (*events)[len(*events)-1]; last.Kind != driver.EventPress || last.ID != 3 {
		t.Fatalf("mouse press: %+v", last)
	}
	tr.views[3].style.Transform.TranslateX = 80
	if got := tr.hit(driver.Point{X: 8, Y: 56}, func(v *view) bool { return v.id == 3 }); got != nil {
		t.Fatal("old transformed position still hits")
	}
	if got := tr.hit(driver.Point{X: 88, Y: 56}, func(v *view) bool { return v.id == 3 }); got == nil {
		t.Fatal("drawn transformed position misses")
	}
	root := tr.views[1]
	root.kind = driver.KindScroll
	root.content = driver.Size{Width: 320, Height: 320}
	root.frame.Size.Height = 80
	tr.wheel(driver.Point{X: 88, Y: 56}, driver.Point{Y: 48})
	if root.offset.Y != 48 {
		t.Fatalf("wheel offset: %+v", root.offset)
	}
	if got := tr.hit(driver.Point{X: 88, Y: 8}, func(v *view) bool { return v.id == 3 }); got == nil {
		t.Fatal("scrolled hit missed")
	}
	tr.scrollTo(root, driver.Point{Y: 9999})
	if root.offset.Y != 240 {
		t.Fatal("scroll escaped content bounds")
	}
}

func TestCellPointerDoesNotStealEditorFocus(t *testing.T) {
	tr, _ := fixture()
	tr.setFocus(tr.views[2])
	p := driver.Point{X: 8, Y: 56}
	tr.mouseDown(p, driver.Modifiers{})
	tr.mouseUp(p, driver.Modifiers{})
	if tr.focus != tr.views[2] {
		t.Fatal("button click stole the editor's focus")
	}
	tr.setFocus(tr.views[3])
	tr.mouseDown(p, driver.Modifiers{})
	tr.mouseUp(p, driver.Modifiers{})
	if tr.focus != nil {
		t.Fatal("pointer press retained keyboard control focus")
	}
}

func TestCellMaskedEditorUsesOriginalUTF16AtScrolledCaret(t *testing.T) {
	tr, _ := fixture()
	v := tr.views[2]
	v.input.Masked = true
	v.input.Content.Size = driver.Size{Width: 16, Height: 16}
	v.value = "界👩‍💻x"
	v.selection = driver.TextRange{Start: 7, End: 7}
	tr.setFocus(v)
	l := v.editLayout(v.value, v.input.Style)
	if len(l.glyphs) != 3 || l.glyphs[1].text != "•" || l.CaretLayout[0].Clusters[1].End != 6 {
		t.Fatalf("masked grapheme geometry: %+v", l)
	}
	g := tr.render(20, 2)
	if v.inputShift != 16 || g.cursorX != 1 || g.at(0, 0).text != "•" {
		t.Fatalf("scrolled password: shift%v cursor%d cells%+v", v.inputShift, g.cursorX, g.cells[:3])
	}
	tr.mouseDown(driver.Point{X: 4, Y: 8}, driver.Modifiers{})
	if v.selection.End != 6 {
		t.Fatalf("password click selected UTF16 offset%d, want6", v.selection.End)
	}
}

func TestCellPasteReportsBeforeEditingAndHonorsReportOnly(t *testing.T) {
	tr, events := fixture()
	v := tr.views[2]
	v.events |= 1 << driver.EventPaste
	v.input.PasteBehavior = driver.PasteReport
	tr.pasteText(v, "literal pasted shortcut")
	if v.value != "" || len(*events) != 1 || (*events)[0].Kind != driver.EventPaste {
		t.Fatalf("report-only paste: value%q events%+v", v.value, *events)
	}
	v.input.PasteBehavior = driver.PasteInsert
	*events = nil
	tr.pasteText(v, "hello")
	if v.value != "hello" || len(*events) < 2 || (*events)[0].Kind != driver.EventPaste || (*events)[1].Kind != driver.EventChangeText {
		t.Fatalf("paste order: value%q events%+v", v.value, *events)
	}
}

func TestCellRenderWideClippingAndReordering(t *testing.T) {
	tr, _ := fixture()
	root := tr.views[1]
	root.frame.Size = driver.Size{Width: 24, Height: 16}
	root.style.Clip = true
	text := (textEngine{}).Prepare(&driver.AttributedText{Text: "A界B", Style: driver.TextStyle{Color: 0xffffffff}}).Layout(driver.Size{Width: 80})
	tr.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{{Kind: driver.ItemCreate, ID: 4, View: driver.KindText}, {Kind: driver.ItemInsert, ID: 4, Parent: 1}, {Kind: driver.ItemFrame, ID: 4, Frame: driver.Bounds{Size: driver.Size{Width: 80, Height: 16}}}, {Kind: driver.ItemText, ID: 4, Text: &driver.TextContent{Layout: text}}}})
	g := tr.render(5, 1)
	if g.at(0, 0).text != "A" || g.at(1, 0).text != "界" || g.at(2, 0).width != 0 || g.at(3, 0).text != " " {
		t.Fatalf("clipped wide text: %+v", g.cells)
	}
	g.put(2, 0, "x", 1, driver.TextStyle{})
	if g.at(1, 0).text != " " || g.at(2, 0).text != "x" {
		t.Fatalf("overwriting wide continuation left a glyph: %+v", g.cells)
	}
	tr.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{{Kind: driver.ItemInsert, ID: 4, Parent: 1, Before: 2}}})
	if root.children[0].id != 4 {
		t.Fatal("insert-before did not reorder mounted children")
	}
	tr.apply(&driver.MountBatch{Root: 1, Items: []driver.MountItem{{Kind: driver.ItemDelete, ID: 4}, {Kind: driver.ItemCreate, ID: 4, View: driver.KindView}}})
	if tr.views[4].text != nil || tr.views[4].parent != nil {
		t.Fatal("recycled node retained text or parent")
	}
}
