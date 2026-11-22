package richeditor

import (
	"time"
	"unicode"

	"github.com/nsgo-dev/nsgo/core"
)

const (
	// doubleTapWindow is how soon a second tap makes a double tap.
	doubleTapWindow = 300 * time.Millisecond
	// doubleTapSlop is how far from the first tap, in points, a second one still counts.
	doubleTapSlop = 24.0
	// wordSlop is how far beside a word's box a long press still lands on the word.
	wordSlop = 4.0
	// handleWidth is the width of a handle's touch hit area, centered on its end.
	handleWidth = 32.0
	// knobRoom is the room a handle's knob hangs in, above the start's line and below the end's.
	knobRoom = 16.0
	// knobSize is the knob's default diameter.
	knobSize = 10.0
)

type handleEdge uint8

const (
	edgeStart handleEdge = iota
	edgeEnd
)

type handleDrag struct {
	// fixed is the other end of the selection, which stays put.
	fixed int
	// anchor is the middle of the dragged end's line at grant, in the content box.
	anchor core.Point
	// origin is the finger at grant, in the surface.
	origin core.Point
	// scroll is scrollY at grant (text moves under the finger as it scrolls).
	scroll core.Points
}

type menuKind uint8

const (
	menuClosed menuKind = iota
	menuSelection
	menuCaret
)

type editMenuState struct {
	kind     menuKind
	from, to int
	editable bool
}

// touchState holds a View's touch selection, handles, long-press sweep and edit menu state.
type touchState struct {
	hasRange   bool
	from, to   int
	drag       *handleDrag
	hasSweep   bool
	sweepFrom  int
	sweepTo    int
	hasLastTap bool
	lastTap    core.Point
	lastTapAt  time.Time
	tapCount   int
	menu       editMenuState
	menuOnLift bool
	sweeping   bool
	rev        *core.Signal[int]
}

func nearPoint(a, b core.Point, slop float64) bool {
	dx, dy := float64(a.X-b.X), float64(a.Y-b.Y)
	return dx*dx+dy*dy <= slop*slop
}

func rectContains(r Rect2, pt core.Point, slop float64) bool {
	x, y := float64(pt.X), float64(pt.Y)
	return x >= r.X-slop && x <= r.X+r.Width+slop && y >= r.Y-slop && y <= r.Y+r.Height+slop
}

// sweepSelection is the selection a long-press sweep makes from the pressed word to the word under the finger: the pressed word and every word up to the finger's.
func sweepSelection(wordFrom, wordTo, atFrom, atTo int) (anchor, head int) {
	switch {
	case atTo > wordTo:
		return wordFrom, atTo
	case atFrom < wordFrom:
		return wordTo, atFrom
	default:
		return wordFrom, wordTo
	}
}

// handleSelection is the selection dragging a handle to pos makes from the fixed end; ok is false where it would collapse to empty (a handle stops at the other end until the finger passes it).
func handleSelection(fixed, pos int) (anchor, head int, ok bool) {
	if pos == fixed {
		return 0, 0, false
	}
	return fixed, pos, true
}

// handleBox is the hit area of edge's handle over caret (the end's caret box in the content box): the line's height plus knobRoom above the start or below the end.
func handleBox(edge handleEdge, caret Rect2) Rect2 {
	top := caret.Y
	if edge == edgeStart {
		top -= knobRoom
	}
	return Rect2{
		X:      caret.X - handleWidth/2,
		Y:      top,
		Width:  handleWidth,
		Height: caret.Height + knobRoom,
	}
}

// menuBoundsAroundHandles grows r vertically by knobRoom above and below so the system edit menu leaves both handles' knobs free to touch.
func menuBoundsAroundHandles(r Rect2) Rect2 {
	return Rect2{
		X:      r.X,
		Y:      r.Y - knobRoom,
		Width:  r.Width,
		Height: r.Height + knobRoom*2,
	}
}

// menuActions returns the system edit menu actions applicable to the current editor state.
func menuActions(hasSelection, editable, allSelected bool) core.EditAction {
	var actions core.EditAction
	if hasSelection && editable {
		actions |= core.EditCut
	}
	if hasSelection {
		actions |= core.EditCopy
	}
	if editable {
		actions |= core.EditPaste
	}
	if !allSelected {
		actions |= core.EditSelectAll
	}
	return actions
}

func (v *rv) bumpTouch() {
	v.touch.rev.Update(func(n int) int { return n + 1 })
}

// touchRange returns the touch selection's range while it matches the session's selection.
func (v *rv) touchRange() (from, to int, ok bool) {
	if !v.touch.hasRange {
		return 0, 0, false
	}
	sel := v.s.State().Selection
	if sel.From() == v.touch.from && sel.To() == v.touch.to && sel.From() < sel.To() {
		return v.touch.from, v.touch.to, true
	}
	return 0, 0, false
}

// hasTextHandles reports whether draggable start/end selection handles should show.
func (v *rv) hasTextHandles() bool {
	if _, _, ok := v.touchRange(); !ok {
		return false
	}
	switch v.s.State().Selection.(type) {
	case *TextSelection, *AllSelection:
		return true
	}
	return false
}

// touchFollow reconciles touch state when the session state changes: any change to a different selection drops the touch selection, its handles and a stale menu.
func (v *rv) touchFollow() {
	sel := v.s.State().Selection
	if v.touch.hasRange && (v.touch.from != sel.From() || v.touch.to != sel.To()) {
		v.touch.hasRange = false
		v.touch.drag = nil
		v.touch.hasSweep = false
		v.bumpTouch()
	}
	stale := false
	switch v.touch.menu.kind {
	case menuSelection:
		stale = v.touch.menu.from != sel.From() || v.touch.menu.to != sel.To()
	case menuCaret:
		stale = !sel.Empty() || v.touch.menu.from != sel.Head()
	}
	if stale {
		v.closeEditMenu()
	} else if v.touch.menu.kind != menuClosed && v.touch.menu.editable != v.s.Editable() {
		v.openEditMenu()
	}
}

func (v *rv) forgetTap() {
	v.touch.hasLastTap = false
	v.touch.tapCount = 0
}

// tap handles a tap on a touch screen: single tap places the caret (or reopens the edit menu on the current touch selection / toggles it on the caret), double tap selects a word and triple tap selects the textblock.
func (v *rv) tap(pt, page core.Point, mods core.Modifiers, clicks int) {
	now := v.cx.Now()
	var n int
	if v.touch.hasLastTap {
		if now.Sub(v.touch.lastTapAt) <= doubleTapWindow && nearPoint(v.touch.lastTap, page, doubleTapSlop) {
			v.touch.tapCount = min(v.touch.tapCount+1, 3)
			n = max(clicks, v.touch.tapCount)
		} else {
			v.touch.tapCount = 1
			n = 1
		}
	} else {
		v.touch.tapCount = max(min(clicks, 3), 1)
		n = v.touch.tapCount
	}
	v.touch.hasLastTap, v.touch.lastTap, v.touch.lastTapAt = true, page, now
	if n >= 3 {
		v.forgetTap()
	}
	if n >= 2 {
		v.pressDown(pt, n, mods)
		v.takeTouchSelection()
		return
	}
	// A tap on the active touch selection or on one of its handles reopens the edit menu.
	if from, to, ok := v.touchRange(); ok {
		onHandle := false
		for _, edge := range []handleEdge{edgeStart, edgeEnd} {
			if b, ok := v.handleAt(edge); ok && rectContains(b, pt, 0) {
				onHandle = true
				break
			}
		}
		inside := false
		for _, r := range v.rangeRects(from, to) {
			if rectContains(r, pt, 0) {
				inside = true
				break
			}
		}
		if !inside {
			if pos, ok := v.PosAtCoords(float64(pt.X), float64(pt.Y)); ok && pos > from && pos < to {
				inside = true
			}
		}
		if onHandle || inside {
			v.openEditMenu()
			return
		}
	}
	stBefore := v.s.State()
	selBefore := stBefore.Selection
	_, wasText := selBefore.(*TextSelection)
	hadCaret := v.focused.GetUntracked() && selBefore.Empty() && wasText
	beforeHead := selBefore.Head()
	wasOpen := v.touch.menu.kind != menuClosed

	v.closeEditMenu()
	v.pressDown(pt, 1, mods)

	// A tap on the caret toggles its edit menu, matching native inputs.
	selNow := v.s.State().Selection
	if !wasOpen && hadCaret && selNow.Empty() && selNow.Head() == beforeHead {
		v.openEditMenu()
	}
}

// longPress selects the word (or selectable leaf) under the finger, or places the caret on blank space, arms a word-by-word sweep, and opens the edit menu on lift.
func (v *rv) longPress(pt, _ core.Point) {
	v.forgetTap()
	v.sinkRef.Focus()
	x, y := float64(pt.X), float64(pt.Y)
	st := v.s.State()
	doc := st.Doc

	var sel Selection
	var hasWord bool
	var wordFrom, wordTo int
	if l := v.leafAt(x, y); l != nil && isNodeSelectable(l.nowNode) {
		sel = NodeSelectionAt(doc, l.pos)
	} else {
		pos, ok := v.PosAtCoords(x, y)
		if !ok {
			return
		}
		if a, z, ok := v.wordUnder(pt, pos); ok {
			sel = TextSelectionAt(doc, a, z)
			hasWord, wordFrom, wordTo = true, a, z
		} else {
			sel = NewTextSelection(doc.Resolve(pos), nil)
		}
	}
	v.clearSink()
	v.closeEditMenu()
	if !sel.Empty() {
		v.touch.hasRange, v.touch.from, v.touch.to = true, sel.From(), sel.To()
	} else {
		v.touch.hasRange = false
	}
	v.touch.drag = nil
	v.touch.hasSweep, v.touch.sweepFrom, v.touch.sweepTo = hasWord, wordFrom, wordTo
	v.touch.menuOnLift = true
	v.s.Dispatch(st.Tr().SetSelection(sel))
	// Re-sync range in case a plugin adjusted the selection.
	after := v.s.State().Selection
	if !after.Empty() {
		v.touch.hasRange, v.touch.from, v.touch.to = true, after.From(), after.To()
	} else {
		v.touch.hasRange = false
	}
	v.bumpTouch()
}

// wordUnder returns the word (or inline leaf) at pos when pt lands on it, rejecting whitespace and blank space past a line's end.
func (v *rv) wordUnder(pt core.Point, pos int) (int, int, bool) {
	doc := v.s.State().Doc
	r := doc.Resolve(pos)
	if !r.Parent().InlineContent() {
		return 0, 0, false
	}
	a, z := wordAround(r)
	if a >= z {
		return 0, 0, false
	}
	text := doc.TextBetween(a, z, "", "￼")
	allSpace := true
	for _, ch := range text {
		if !unicode.IsSpace(ch) {
			allSpace = false
			break
		}
	}
	if allSpace {
		return 0, 0, false
	}
	for _, box := range v.rangeRects(a, z) {
		if rectContains(box, pt, wordSlop) {
			return a, z, true
		}
	}
	return 0, 0, false
}

// takeTouchSelection records the current selection as a touch selection and opens the edit menu.
func (v *rv) takeTouchSelection() {
	sel := v.s.State().Selection
	if !sel.Empty() {
		v.touch.hasRange, v.touch.from, v.touch.to = true, sel.From(), sel.To()
	} else {
		v.touch.hasRange = false
	}
	v.bumpTouch()
	v.openEditMenu()
}

func (v *rv) sweepArmed() bool {
	return v.touch.hasSweep
}

func (v *rv) touchDown() {
	v.touch.hasSweep = false
	v.touch.sweeping = false
	v.touch.menuOnLift = false
}

func (v *rv) touchUp() {
	open := v.touch.menuOnLift && !v.touch.sweeping
	v.touch.menuOnLift = false
	if open {
		v.openEditMenu()
	}
}

func (v *rv) beginSweep() {
	v.touch.sweeping = true
	v.touch.menuOnLift = false
	v.closeEditMenu()
}

func (v *rv) sweepTo(page core.Point) {
	if !v.touch.hasSweep {
		return
	}
	origin, ok := v.content.Bounds()
	if !ok {
		return
	}
	at := core.Point{X: page.X - origin.Origin.X, Y: page.Y - origin.Origin.Y}
	pos, ok := v.PosAtCoords(float64(at.X), float64(at.Y))
	if !ok {
		return
	}
	st := v.s.State()
	doc := st.Doc
	r := doc.Resolve(pos)
	a, z := pos, pos
	if r.Parent().InlineContent() {
		wa, wz := wordAround(r)
		if wa < wz {
			a, z = wa, wz
		}
	}
	anchor, head := sweepSelection(v.touch.sweepFrom, v.touch.sweepTo, a, z)
	sel := TextSelectionBetween(doc.Resolve(anchor), doc.Resolve(head), 0)
	if sel.From() == st.Selection.From() && sel.To() == st.Selection.To() {
		return
	}
	v.touch.hasRange, v.touch.from, v.touch.to = true, sel.From(), sel.To()
	v.s.Dispatch(st.Tr().SetSelection(sel).ScrollIntoView())
	v.bumpTouch()
}

func (v *rv) endSweep() {
	swept := v.touch.hasSweep
	v.touch.sweeping = false
	v.touch.hasSweep = false
	if swept {
		v.takeTouchSelection()
	}
}

// endCaret returns the caret box at edge of the active touch selection in content coordinates.
func (v *rv) endCaret(edge handleEdge) (Rect2, bool) {
	from, to, ok := v.touchRange()
	if !ok {
		return Rect2{}, false
	}
	doc := v.s.State().Doc
	var pos int
	if edge == edgeStart {
		pos = SelectionNear(doc.Resolve(from), 1).From()
	} else {
		pos = SelectionNear(doc.Resolve(to), -1).To()
	}
	return v.CoordsAtPos(pos)
}

func (v *rv) handleAt(edge handleEdge) (Rect2, bool) {
	if !v.focused.GetUntracked() || !v.hasTextHandles() {
		return Rect2{}, false
	}
	caret, ok := v.endCaret(edge)
	if !ok {
		return Rect2{}, false
	}
	return handleBox(edge, caret), true
}

func (v *rv) beginHandleDrag(edge handleEdge, page core.Point) {
	from, to, ok := v.touchRange()
	if !ok {
		return
	}
	caret, ok := v.endCaret(edge)
	if !ok {
		return
	}
	doc := v.s.State().Doc
	var fixed int
	if edge == edgeStart {
		fixed = SelectionNear(doc.Resolve(to), -1).Head()
	} else {
		fixed = SelectionNear(doc.Resolve(from), 1).Head()
	}
	v.closeEditMenu()
	v.touch.drag = &handleDrag{
		fixed:  fixed,
		anchor: core.Point{X: core.Points(caret.X), Y: core.Points(caret.Y + caret.Height/2)},
		origin: page,
		scroll: v.scrollY.GetUntracked(),
	}
	v.bumpTouch()
}

func (v *rv) handleDragTo(page core.Point) {
	if v.touch.drag == nil {
		return
	}
	drag := *v.touch.drag
	scrolled := v.scrollY.GetUntracked() - drag.scroll
	at := core.Point{
		X: drag.anchor.X + (page.X - drag.origin.X),
		Y: drag.anchor.Y + (page.Y - drag.origin.Y) + scrolled,
	}
	pos, ok := v.PosAtCoords(float64(at.X), float64(at.Y))
	if !ok {
		return
	}
	anchor, head, ok := handleSelection(drag.fixed, pos)
	if !ok {
		return
	}
	st := v.s.State()
	sel := TextSelectionBetween(st.Doc.Resolve(anchor), st.Doc.Resolve(head), 0)
	from, to := sel.From(), sel.To()
	if from == to {
		return
	}
	if curFrom, curTo, has := v.touchRange(); has && curFrom == from && curTo == to {
		return
	}
	v.touch.hasRange, v.touch.from, v.touch.to = true, from, to
	v.s.Dispatch(st.Tr().SetSelection(sel).ScrollIntoView())
	v.touch.drag = &drag
	v.bumpTouch()
}

func (v *rv) endHandleDrag() {
	if v.touch.drag == nil {
		return
	}
	v.touch.drag = nil
	v.bumpTouch()
	v.openEditMenu()
}

// visibleSelectionBox returns the selection's box in content coordinates clamped to the viewport.
func (v *rv) visibleSelectionBox() (Rect2, bool) {
	b, ok := v.SelectionRect()
	if !ok {
		return Rect2{}, false
	}
	top := float64(v.scrollY.GetUntracked())
	height := float64(v.viewport.GetUntracked().Height)
	if height <= 0 {
		return b, true
	}
	y0 := max(b.Y, top)
	y1 := min(b.Y+b.Height, top+height)
	if y1 <= y0 {
		return b, true
	}
	return Rect2{X: b.X, Y: y0, Width: b.Width, Height: y1 - y0}, true
}

// openEditMenu opens the platform's touch edit menu on the sink clear of the selection (including handle knobs when handles show) or at the caret.
func (v *rv) openEditMenu() {
	st := v.s.State()
	doc := st.Doc
	sel := st.Selection
	all := NewAllSelection(doc)
	emptyDoc := doc.ChildCount() == 1 && doc.FirstChild().IsTextblock() && doc.FirstChild().Content.Size == 0
	allSelected := emptyDoc ||
		(sel.From() == all.From() && sel.To() == all.To()) ||
		(sel.From() <= SelectionAtStart(doc).From() && sel.To() >= SelectionAtEnd(doc).To())

	hasSelection := !sel.Empty()
	var menu editMenuState
	var bounds Rect2
	var ok bool
	if hasSelection {
		bounds, ok = v.visibleSelectionBox()
		if ok && v.hasTextHandles() {
			bounds = menuBoundsAroundHandles(bounds)
		}
		menu = editMenuState{kind: menuSelection, from: sel.From(), to: sel.To()}
	} else {
		bounds, ok = v.CoordsAtPos(sel.Head())
		menu = editMenuState{kind: menuCaret, from: sel.Head()}
	}
	origin, have := v.content.Bounds()
	if !ok || !have {
		return
	}
	actions := menuActions(hasSelection, v.s.Editable(), allSelected)
	if actions == 0 {
		v.closeEditMenu()
		return
	}
	page := core.Bounds{
		Origin: core.Point{X: origin.Origin.X + core.Points(bounds.X), Y: origin.Origin.Y + core.Points(bounds.Y)},
		Size:   core.Size{Width: core.Points(bounds.Width), Height: core.Points(bounds.Height)},
	}
	menu.editable = v.s.Editable()
	v.touch.menu = menu
	v.sinkRef.OpenEditMenu(page, actions)
}

func (v *rv) closeEditMenu() {
	if v.touch.menu.kind == menuClosed {
		return
	}
	v.touch.menu = editMenuState{kind: menuClosed}
	v.sinkRef.CloseEditMenu()
}

// editAction runs an action chosen from the system's touch edit menu.
func (v *rv) editAction(act core.EditAction) {
	v.touch.menu = editMenuState{kind: menuClosed}
	switch act {
	case core.EditCopy:
		v.copy()
	case core.EditCut:
		v.cut()
	case core.EditSelectAll:
		v.clearSink()
		v.s.Run(SelectAll)
		v.takeTouchSelection()
	}
}

// handles renders the draggable start and end knobs while a touch text selection is active.
func (v *rv) handles() core.Node {
	shown := func() bool {
		v.rev.Get()
		v.touch.rev.Get()
		return v.focused.Get() && v.hasTextHandles()
	}
	return core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
		return core.Group{v.handleView(cx, edgeStart), v.handleView(cx, edgeEnd)}
	}}
}

func (v *rv) handleView(cx *core.Scope, edge handleEdge) core.Node {
	id := "rich-editor-selection-handle-end"
	if edge == edgeStart {
		id = "rich-editor-selection-handle-start"
	}
	type boxState struct {
		r  Rect2
		ok bool
	}
	at := core.NewMemo(cx, func() boxState {
		v.rev.Get()
		v.geo.Get()
		v.touch.rev.Get()
		r, ok := v.handleAt(edge)
		return boxState{r, ok}
	})
	knob := core.View{
		Class:         v.p.Classes.SelectionHandle,
		PointerEvents: core.PointerEventsNone,
		StyleFunc: func() core.Style {
			return core.Style{Width: core.Pt(knobSize), Height: core.Pt(knobSize), CornerRadius: knobSize / 2}.Merge(v.looks.Get().SelectionHandle)
		},
	}
	slot := core.View{
		PointerEvents: core.PointerEventsNone,
		Style:         core.Style{Height: core.Pt(knobRoom), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		Children:      []core.Node{knob},
	}
	line := core.View{
		PointerEvents: core.PointerEventsNone,
		StyleFunc: func() core.Style {
			h := max(at.Get().r.Height-knobRoom, 0)
			return core.Style{Height: core.Pt(core.Points(h))}
		},
	}
	kids := []core.Node{line, slot}
	if edge == edgeStart {
		kids = []core.Node{slot, line}
	}
	return core.View{
		TestID: id,
		StyleFunc: func() core.Style {
			s := at.Get()
			st := core.Style{
				Position: core.Absolute,
				Left:     core.Pt(core.Points(s.r.X)),
				Top:      core.Pt(core.Points(s.r.Y)),
				Width:    core.Pt(handleWidth),
				Height:   core.Pt(core.Points(max(s.r.Height, knobRoom))),
			}
			if !s.ok {
				st.Opacity = core.Alpha(0)
			}
			return st
		},
		Responder: &core.Responder{
			Claims:      []core.TouchClaim{{Ask: func(core.ResponderEvent) bool { return at.GetUntracked().ok }}},
			Keep:        true,
			OnGrant:     func(e core.ResponderEvent) { v.beginHandleDrag(edge, e.PagePosition) },
			OnMove:      func(e core.ResponderEvent) { v.handleDragTo(e.PagePosition) },
			OnRelease:   func(core.ResponderEvent) { v.endHandleDrag() },
			OnTerminate: func(core.ResponderEvent) { v.endHandleDrag() },
		},
		Children: kids,
	}
}
