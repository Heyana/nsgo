package richeditor_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/richeditor"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func phone(os core.OS) nsgotest.Option {
	return nsgotest.Environment(core.Environment{OS: os, Idiom: core.IdiomPhone})
}

func contentPoint(t *testing.T, app *nsgotest.App, s *richeditor.Session, pos int) core.Point {
	t.Helper()
	r, ok := s.PageCoordsAtPos(pos)
	if !ok {
		t.Fatalf("no page coords for pos %d", pos)
	}
	content := app.Find("id:rich-editor-content")
	origin := pageOrigin(content)
	return core.Point{
		X: core.Points(r.X) - origin.X,
		Y: core.Points(r.Y+r.Height/2) - origin.Y,
	}
}

func pageOrigin(v *nsgotest.View) core.Point {
	var p core.Point
	for cur := v; cur != nil; cur = cur.Parent {
		p.X += cur.Frame.Origin.X
		p.Y += cur.Frame.Origin.Y
		if cur.Parent != nil {
			p.X -= cur.Parent.Offset.X
			p.Y -= cur.Parent.Offset.Y
		}
	}
	return p
}

func TestTouchTapCaretAndEditMenuToggle(t *testing.T) {
	s := session(t, "hello world\n")
	app := nsgotest.Start(t, view(s), phone(core.OSIOS))
	app.Settle()

	// First tap places the caret without handles or edit menu.
	pt := contentPoint(t, app, s, 3)
	app.PressAt("id:rich-editor-content", pt)
	if !s.State().Selection.Empty() || s.State().Selection.Head() != 3 {
		t.Fatalf("selection %+v, want caret at 3", s.State().Selection)
	}
	if app.Exists("id:rich-editor-selection-handle-start") || app.Exists("id:rich-editor-selection-handle-end") {
		t.Fatal("handles showed on caret tap")
	}
	if app.EditMenu != nil {
		t.Fatalf("edit menu opened on first tap: %+v", app.EditMenu)
	}

	// Tap again at the same caret past the double-tap window: opens caret edit menu.
	app.Advance(400 * time.Millisecond)
	app.PressAt("id:rich-editor-content", pt)
	if app.EditMenu == nil {
		t.Fatal("tapping the caret did not open the edit menu")
	}
	// Clipboard is empty so Paste is omitted on iOS; Cut/Copy omitted without selection.
	if got := app.EditMenuItems(); !slices.Equal(got, []string{"Select All"}) {
		t.Fatalf("caret edit menu items = %v, want [Select All]", got)
	}
	app.Clipboard = "inserted"
	if got := app.EditMenuItems(); !slices.Equal(got, []string{"Paste", "Select All"}) {
		t.Fatalf("caret edit menu items with clipboard = %v, want [Paste Select All]", got)
	}

	// Tapping the caret again closes the edit menu.
	app.Advance(400 * time.Millisecond)
	app.PressAt("id:rich-editor-content", pt)
	if app.EditMenu != nil {
		t.Fatalf("second caret tap did not close edit menu: %+v", app.EditMenu)
	}

	// Tapping > 24pt away within 300ms is NOT a double tap: it moves the caret.
	far := contentPoint(t, app, s, 10)
	app.PressAt("id:rich-editor-content", far)
	if !s.State().Selection.Empty() {
		t.Fatalf("far tap within 300ms selected %q instead of moving caret", selected(s))
	}
}

func TestTouchDoubleAndTripleTapSelectWithHandlesAndMenu(t *testing.T) {
	for _, tc := range []struct {
		name      string
		os        core.OS
		selectAll string
	}{
		{"iOS", core.OSIOS, "Select All"},
		{"Android", core.OSAndroid, "Select all"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := session(t, "quick brown fox\n\nsecond paragraph\n")
			app := nsgotest.Start(t, view(s), phone(tc.os))
			app.Clipboard = "clip"
			app.Settle()

			// Double-tap "brown" (positions 7..12).
			brownPt := contentPoint(t, app, s, 9)
			app.PressAt("id:rich-editor-content", brownPt)
			app.PressAt("id:rich-editor-content", brownPt)

			if got := selected(s); got != "brown" {
				t.Fatalf("double tap selected %q, want brown", got)
			}
			startHandle := app.Find("id:rich-editor-selection-handle-start")
			endHandle := app.Find("id:rich-editor-selection-handle-end")
			if startHandle.Frame.Origin.X >= endHandle.Frame.Origin.X {
				t.Fatalf("start handle %+v not left of end handle %+v", startHandle.Frame, endHandle.Frame)
			}
			if app.EditMenu == nil {
				t.Fatal("double tap did not open edit menu")
			}
			wantItems := []string{"Cut", "Copy", "Paste", tc.selectAll}
			if got := app.EditMenuItems(); !slices.Equal(got, wantItems) {
				t.Fatalf("edit menu items = %v, want %v", got, wantItems)
			}
			// Edit menu bounds must clear the handles' knobs above and below the selection.
			selRect, ok := s.PageCoordsAtPos(7)
			if !ok {
				t.Fatal("no page coords")
			}
			if float64(app.EditMenu.Bounds.Origin.Y) > selRect.Y-15 {
				t.Fatalf("edit menu top %v does not clear start handle above line %v", app.EditMenu.Bounds.Origin.Y, selRect.Y)
			}

			// Triple-tap selects the entire first paragraph.
			app.PressAt("id:rich-editor-content", brownPt)
			if got := selected(s); got != "quick brown fox" {
				t.Fatalf("triple tap selected %q, want %q", got, "quick brown fox")
			}
			if app.EditMenu == nil {
				t.Fatal("triple tap did not open edit menu")
			}
		})
	}
}

func TestTouchLongPressWordBlankAndSweep(t *testing.T) {
	s := session(t, "quick brown fox\n")
	app := nsgotest.Start(t, view(s), phone(core.OSIOS))
	app.Settle()

	// Long press on "brown" selects it, shows handles, and opens menu on lift.
	brownPt := contentPoint(t, app, s, 9)
	app.LongPressAt("id:rich-editor-content", brownPt)
	if got := selected(s); got != "brown" {
		t.Fatalf("long press selected %q, want brown", got)
	}
	app.Find("id:rich-editor-selection-handle-start")
	app.Find("id:rich-editor-selection-handle-end")
	if app.EditMenu == nil {
		t.Fatal("long press did not open edit menu on lift")
	}

	// Long press on blank space far right of the line places caret with no handles.
	blankPt := core.Point{X: 450, Y: brownPt.Y}
	app.LongPressAt("id:rich-editor-content", blankPt)
	if !s.State().Selection.Empty() {
		t.Fatalf("long press on blank space selected %q, want caret", selected(s))
	}
	if app.Exists("id:rich-editor-selection-handle-start") {
		t.Fatal("handles showed on blank-space long press")
	}
	if app.EditMenu == nil {
		t.Fatal("blank-space long press did not open caret edit menu")
	}

	// Long-press and sweep forward from "brown" into "fox" selects whole words "brown fox".
	foxPt := contentPoint(t, app, s, 14)
	f := app.Touch("id:rich-editor-content", brownPt)
	f.Hold(500 * time.Millisecond)
	if got := selected(s); got != "brown" {
		t.Fatalf("hold selected %q, want brown", got)
	}
	// Menu waits until finger lifts.
	if app.EditMenu != nil {
		t.Fatal("edit menu opened before finger lifted")
	}
	f.Move(foxPt)
	if got := selected(s); got != "brown fox" {
		t.Fatalf("forward sweep selected %q, want %q", got, "brown fox")
	}
	// Sweep backward past "brown" into "quick" selects "quick brown".
	quickPt := contentPoint(t, app, s, 3)
	f.Move(quickPt)
	if got := selected(s); got != "quick brown" {
		t.Fatalf("backward sweep selected %q, want %q", got, "quick brown")
	}
	if app.EditMenu != nil {
		t.Fatal("edit menu showed during sweep")
	}
	f.Lift()
	if app.EditMenu == nil {
		t.Fatal("edit menu did not open after sweep lifted")
	}
}

func TestTouchDragHandlesAcrossBlocksAndCrossing(t *testing.T) {
	s := session(t, "alpha beta gamma\n\ndelta epsilon\n")
	app := nsgotest.Start(t, view(s), phone(core.OSAndroid))
	app.Settle()

	// Select "beta" via long press.
	betaPt := contentPoint(t, app, s, 9)
	app.LongPressAt("id:rich-editor-content", betaPt)
	if got := selected(s); got != "beta" {
		t.Fatalf("initial selection %q, want beta", got)
	}

	// Drag end handle right into "gamma".
	endPos, _ := s.PageCoordsAtPos(s.State().Selection.To())
	gammaPos, _ := s.PageCoordsAtPos(17) // end of "gamma"
	dx := core.Points(gammaPos.X - endPos.X)
	app.Drag("id:rich-editor-selection-handle-end", core.Point{X: dx, Y: 0})
	if got := selected(s); got != "beta gamma" {
		t.Fatalf("after dragging end handle right: %q, want %q", got, "beta gamma")
	}
	if app.EditMenu == nil {
		t.Fatal("edit menu did not reopen after handle drag")
	}

	// Drag start handle left to the start of "alpha".
	startPos, _ := s.PageCoordsAtPos(s.State().Selection.From())
	alphaPos, _ := s.PageCoordsAtPos(1)
	app.Drag("id:rich-editor-selection-handle-start", core.Point{X: core.Points(alphaPos.X - startPos.X), Y: 0})
	if got := selected(s); got != "alpha beta gamma" {
		t.Fatalf("after dragging start handle left: %q, want %q", got, "alpha beta gamma")
	}

	// Drag end handle down into the second paragraph ("delta epsilon").
	endPos, _ = s.PageCoordsAtPos(s.State().Selection.To())
	deltaEnd, _ := s.PageCoordsAtPos(24) // inside second paragraph after "delta"
	app.Drag("id:rich-editor-selection-handle-end", core.Point{X: core.Points(deltaEnd.X - endPos.X), Y: core.Points(deltaEnd.Y - endPos.Y)})
	got := selected(s)
	if !strings.HasPrefix(got, "alpha beta gamma\n") || !strings.Contains(got, "delta") {
		t.Fatalf("cross-block handle drag selected %q", got)
	}

	// Cross handles: drag start handle past the end handle into "epsilon".
	startPos, _ = s.PageCoordsAtPos(s.State().Selection.From())
	endDoc := s.State().Doc.Content.Size - 1
	epsEnd, _ := s.PageCoordsAtPos(endDoc)
	app.Drag("id:rich-editor-selection-handle-start", core.Point{X: core.Points(epsEnd.X - startPos.X), Y: core.Points(epsEnd.Y - startPos.Y)})
	if s.State().Selection.Empty() {
		t.Fatal("crossing handles collapsed selection to empty")
	}
	if !strings.Contains(selected(s), "epsilon") {
		t.Fatalf("crossed handle drag selected %q, want range reaching epsilon", selected(s))
	}
}

func TestTouchDragHandleAcrossInlineLeaf(t *testing.T) {
	s := session(t, "hi there\n")
	if !s.RunNamed("insertMention", richeditor.Attrs{"id": "1", "label": "Alice"}) {
		t.Fatal("insertMention")
	}
	// Document is now: [mention(@Alice)] " hi there"
	app := nsgotest.Start(t, view(s, func(v *richeditor.View) {
		v.NodeViews = map[string]richeditor.NodeView{
			"mention": func(cx *core.Scope, p richeditor.NodeProps) core.Node {
				return core.View{TestID: "mention", Style: core.Style{Width: core.Pt(40), Height: core.Pt(16)},
					Children: []core.Node{core.Text{ContentFunc: func() string { return p.Node().AttrStr("label") }}}}
			},
		}
	}), phone(core.OSIOS))
	app.Settle()

	// Long press "hi" (after the mention + space: pos 3..5).
	hiPt := contentPoint(t, app, s, 4)
	app.LongPressAt("id:rich-editor-content", hiPt)
	if got := selected(s); got != "hi" {
		t.Fatalf("long press selected %q, want hi", got)
	}

	// Drag start handle left across the mention inline leaf to pos 1 (start of paragraph).
	startPos, _ := s.PageCoordsAtPos(s.State().Selection.From())
	paraStart, _ := s.PageCoordsAtPos(1)
	app.Drag("id:rich-editor-selection-handle-start", core.Point{X: core.Points(paraStart.X - startPos.X), Y: 0})
	if s.State().Selection.From() != 1 {
		t.Fatalf("selection from = %d, want 1 (covering inline leaf)", s.State().Selection.From())
	}
	if copyText, _ := s.Copy(); !strings.Contains(copyText, "@Alice") || !strings.Contains(copyText, "hi") {
		t.Fatalf("copied text across inline leaf = %q, want @Alice and hi", copyText)
	}

	// Long press directly on the inline leaf view selects it as NodeSelection and opens edit menu.
	app.LongPress("id:mention")
	ns, ok := s.State().Selection.(*richeditor.NodeSelection)
	if !ok || ns.Node.Type.Name != "mention" {
		t.Fatalf("long press on inline leaf selected %T (%+v), want NodeSelection(mention)", s.State().Selection, s.State().Selection)
	}
	if app.EditMenu == nil {
		t.Fatal("long press on inline leaf did not open edit menu")
	}
}

func TestTouchEditMenuActionsAndReadOnly(t *testing.T) {
	s := session(t, "one two three\n")
	app := nsgotest.Start(t, view(s), phone(core.OSIOS))
	app.Settle()

	// Select "two" via double tap.
	twoPt := contentPoint(t, app, s, 6)
	app.PressAt("id:rich-editor-content", twoPt)
	app.PressAt("id:rich-editor-content", twoPt)
	if got := selected(s); got != "two" {
		t.Fatalf("selected %q, want two", got)
	}

	// Copy puts "two" in clipboard without changing document.
	app.ChooseEditAction(core.EditCopy)
	if app.Clipboard != "two" || s.Text() != "one two three" {
		t.Fatalf("after Copy: clipboard=%q text=%q", app.Clipboard, s.Text())
	}

	// Tapping inside the touch selection reopens the edit menu.
	app.Advance(400 * time.Millisecond)
	app.PressAt("id:rich-editor-content", twoPt)
	if app.EditMenu == nil {
		t.Fatal("tapping inside touch selection did not reopen edit menu")
	}
	if got := app.EditMenuItems(); !slices.Equal(got, []string{"Cut", "Copy", "Paste", "Select All"}) {
		t.Fatalf("menu items = %v", got)
	}

	// Cut removes "two", puts it in clipboard, drops handles and is one undo step.
	app.ChooseEditAction(core.EditCut)
	if app.Clipboard != "two" || s.Text() != "one  three" {
		t.Fatalf("after Cut: clipboard=%q text=%q", app.Clipboard, s.Text())
	}
	if app.Exists("id:rich-editor-selection-handle-start") {
		t.Fatal("handles stayed after Cut")
	}
	if !s.Run(richeditor.Undo) || s.Text() != "one two three" {
		t.Fatalf("undo after Cut left %q", s.Text())
	}

	// Select All via edit menu selects whole document, keeps handles, and omits Select All.
	app.LongPressAt("id:rich-editor-content", twoPt)
	app.ChooseEditAction(core.EditSelectAll)
	if got := selected(s); got != "one two three" {
		t.Fatalf("after Select All: selected %q", got)
	}
	app.Find("id:rich-editor-selection-handle-start")
	app.Find("id:rich-editor-selection-handle-end")
	if app.EditMenu == nil {
		t.Fatal("edit menu did not reopen after Select All")
	}
	if got := app.EditMenuItems(); !slices.Equal(got, []string{"Cut", "Copy", "Paste"}) {
		t.Fatalf("items after Select All = %v, want [Cut Copy Paste]", got)
	}

	// Paste via edit menu replaces the whole selection and is undoable.
	app.Clipboard = "replaced"
	app.ChooseEditAction(core.EditPaste)
	if got := s.Text(); got != "replaced" {
		t.Fatalf("after Paste: text=%q, want replaced", got)
	}
	if app.Exists("id:rich-editor-selection-handle-start") {
		t.Fatal("handles stayed after Paste")
	}

	// Read-only editor: offers Copy and Select All, never Cut or Paste.
	s.SetEditable(false)
	app.Settle()
	app.Clipboard = "something"
	s.Dispatch(s.State().Tr().SetSelection(richeditor.TextSelectionAt(s.State().Doc, 1, 1)))
	app.Settle()
	app.LongPressAt("id:rich-editor-content", contentPoint(t, app, s, 3))
	if app.EditMenu == nil {
		t.Fatal("read-only long press did not open edit menu")
	}
	if app.EditMenu.Actions&(core.EditCut|core.EditPaste) != 0 {
		t.Fatalf("read-only edit menu offered Cut/Paste: %d", app.EditMenu.Actions)
	}
	if app.EditMenu.Actions&core.EditCopy == 0 {
		t.Fatalf("read-only edit menu omitted Copy: %d", app.EditMenu.Actions)
	}
}

func TestTouchEditMenuFollowsEditableChanges(t *testing.T) {
	s := session(t, "one two three\n")
	app := nsgotest.Start(t, view(s), phone(core.OSIOS))
	app.Settle()
	app.LongPressAt("id:rich-editor-content", contentPoint(t, app, s, 6))
	for _, editable := range []bool{false, true, false} {
		app.Post(func() { s.SetEditable(editable) })
		app.Settle()
		if app.EditMenu == nil {
			t.Fatal("changing editability dismissed the selection menu")
		}
		want := core.EditCopy | core.EditSelectAll
		if editable {
			want |= core.EditCut | core.EditPaste
		}
		if got := app.EditMenu.Actions; got != want {
			t.Fatalf("editable %t: menu offers %d, want %d", editable, got, want)
		}
		if got := selected(s); got != "two" {
			t.Fatalf("editability change lost selection: %q", got)
		}
	}
}

func TestTouchSelectionDropsOnEditProgrammaticMoveAndBlur(t *testing.T) {
	s := session(t, "hello world\n")
	var ref richeditor.Ref
	app := nsgotest.Start(t, view(s, func(v *richeditor.View) { v.Ref = &ref }), phone(core.OSIOS))
	app.Settle()

	pt := contentPoint(t, app, s, 3)
	app.LongPressAt("id:rich-editor-content", pt)
	app.Find("id:rich-editor-selection-handle-start")
	if app.EditMenu == nil {
		t.Fatal("expected edit menu")
	}

	// Programmatic selection move drops touch handles and closes menu.
	app.Post(func() {
		s.Dispatch(s.State().Tr().SetSelection(richeditor.TextSelectionAt(s.State().Doc, 8, 8)))
	})
	app.Settle()
	if app.Exists("id:rich-editor-selection-handle-start") {
		t.Fatal("handles stayed after programmatic selection move")
	}
	if app.EditMenu != nil {
		t.Fatal("edit menu stayed after programmatic selection move")
	}

	// Re-select and blur: handles hide and menu closes.
	app.LongPressAt("id:rich-editor-content", pt)
	app.Find("id:rich-editor-selection-handle-start")
	app.Post(func() { ref.Blur() })
	app.Settle()
	if app.Exists("id:rich-editor-selection-handle-start") {
		t.Fatal("handles stayed after blur")
	}
	if app.EditMenu != nil {
		t.Fatal("edit menu stayed after blur")
	}
}

func TestDesktopDoesNotOpenTouchHandlesOrEditMenu(t *testing.T) {
	s := session(t, "one two three\n")
	app := nsgotest.Start(t, view(s), desktop())
	app.Settle()
	app.Press("text:one two three")
	app.Press("text:one two three")
	if app.Exists("id:rich-editor-selection-handle-start") || app.EditMenu != nil {
		t.Fatalf("desktop double-click showed touch handles or edit menu: %+v", app.EditMenu)
	}
}
