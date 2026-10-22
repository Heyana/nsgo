//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/platform/internal/apple"
)

// A selection area (a view with SelectionArea, or a selectable text
// outside one) selects with the mouse as a desktop does: drag, double
// click a word, triple click a whole text, Shift-click to extend. The
// surface takes the focus while an area has a selection, for Copy and
// Select All (the Edit menu, Cmd-C and Cmd-A) and Escape, which clears
// it; a press anywhere else clears it, so the window shows one selection.

// selection is the surface's selection: an area's model over its texts.
type selection struct {
	area     *view
	texts    []*view
	frames   []core.Bounds // the texts' content boxes in the area
	model    core.SelectionModel
	dragging bool
}

// Len implements core.SelectionTexts.
func (s *selection) Len() int { return len(s.texts) }

// Content implements core.SelectionTexts.
func (s *selection) Content(i int) string { return s.texts[i].plain }

// Frame implements core.SelectionTexts.
func (s *selection) Frame(i int) core.Bounds { return s.frames[i] }

// Key implements core.SelectionTexts.
func (s *selection) Key(i int) uint64 { return uint64(s.texts[i].id) }

// OffsetAt implements core.SelectionTexts.
func (s *selection) OffsetAt(i int, p core.Point) int {
	v, f := s.texts[i], s.frames[i]
	at := apple.CGPoint{X: float64(p.X - f.Origin.X + v.box.Origin.X), Y: float64(p.Y - f.Origin.Y + v.box.Origin.Y)}
	return v.layout.OffsetAt(at, boxRect(v.box))
}

func boxRect(b core.Bounds) apple.CGRect {
	return apple.Rect(float64(b.Origin.X), float64(b.Origin.Y), float64(b.Size.Width), float64(b.Size.Height))
}

// list finds the area's texts again, in document order, with their frames.
func (s *selection) list() {
	s.texts, s.frames = s.texts[:0], s.frames[:0]
	var walk func(n objc.ID)
	walk = func(n objc.ID) {
		v := views[n]
		if v != nil && v.native == n && v.kind == core.KindText {
			if v.layout != nil && v.selectable != core.SelectableOff &&
				!objc.Send[bool](n, sel("isHiddenOrHasHiddenAncestor")) {
				r := objc.Send[apple.CGRect](s.area.native, sel("convertRect:fromView:"), boxRect(v.box), n)
				s.texts = append(s.texts, v)
				s.frames = append(s.frames, core.Bounds{
					Origin: core.Point{X: core.Points(r.Origin.X), Y: core.Points(r.Origin.Y)},
					Size:   core.Size{Width: core.Points(r.Size.Width), Height: core.Points(r.Size.Height)},
				})
			}
			return
		}
		for _, c := range apple.Subviews(n) {
			walk(c)
		}
	}
	walk(s.area.native)
}

// point returns event's location in the area.
func (s *selection) point(event objc.ID) core.Point {
	p := objc.Send[apple.CGPoint](s.area.native, selConvertPoint, objc.Send[apple.CGPoint](event, selLocationInWindow), objc.ID(0))
	return core.Point{X: core.Points(p.X), Y: core.Points(p.Y)}
}

// show has each text draw its part of the selection.
func (s *selection) show(old []*view) {
	for _, v := range old {
		v.setSelected(0, 0)
	}
	for _, r := range s.model.Ranges(s) {
		s.texts[r.Text].setSelected(r.Start, r.End)
	}
}

// setSelected sets the UTF-16 range a text highlights.
func (v *view) setSelected(start, end int) {
	if v.surface.views[v.id] != v {
		return // deleted
	}
	if v.selStart != start || v.selEnd != end {
		v.selStart, v.selEnd = start, end
		v.native.Send(selSetNeedsDisplay, true)
	}
}

// areaAt returns the selection area a mouse press at event goes to: the
// outermost area around the view hit, or the selectable text hit, unless
// a field, a pressable view or a gesture inside it keeps the press.
func (s *surface) areaAt(event objc.ID) *view {
	up := s.native.Send(selSuperview)
	if up == 0 {
		return nil
	}
	p := objc.Send[apple.CGPoint](up, selConvertPoint, objc.Send[apple.CGPoint](event, selLocationInWindow), objc.ID(0))
	rawHit = true
	hit := s.native.Send(sel("hitTest:"), p)
	rawHit = false
	var area *view
	taken := false
	for n := hit; n != 0 && n != s.native; n = n.Send(selSuperview) {
		v := views[n]
		if v == nil || v.native != n && v.field != n {
			continue
		}
		switch {
		case v.field == n || v.kind == core.KindInput:
			taken = true
		case area == nil && (v.pressable() || v.events.Has(core.EventPan)):
			taken = true
		case v.area && (area != nil || !taken):
			area = v
		case v.kind == core.KindText && v.selectable == core.SelectableOn && !taken:
			area = v
		}
	}
	return area
}

// selectPress starts or extends a selection on a mouse press, or clears
// the selection a press elsewhere leaves. It reports whether it took the
// press.
func (s *surface) selectPress(event objc.ID) bool {
	area := s.areaAt(event)
	if area == nil {
		s.clearSelection()
		return false
	}
	if s.selecting == nil || s.selecting.area != area {
		s.clearSelection()
		s.selecting = &selection{area: area}
	}
	sl := s.selecting
	old := sl.texts
	sl.list()
	const shift = 1 << 17
	extend := objc.Send[uint](event, sel("modifierFlags"))&shift != 0
	sl.model.Press(sl, sl.point(event), objc.Send[int](event, sel("clickCount")), extend)
	sl.dragging = true
	sl.show(old)
	s.window.Send(selMakeFirstResponer, s.native)
	return true
}

// selectDrag follows a drag of the press that started a selection,
// scrolling a scroll view around the area when it leaves its edge.
func (s *surface) selectDrag(event objc.ID) bool {
	sl := s.selecting
	if sl == nil || !sl.dragging {
		return false
	}
	sl.area.native.Send(sel("autoscroll:"), event)
	old := sl.texts
	sl.list()
	sl.model.Drag(sl, sl.point(event))
	sl.show(old)
	return true
}

// selectUp ends a selection's drag.
func (s *surface) selectUp() bool {
	if s.selecting == nil || !s.selecting.dragging {
		return false
	}
	s.selecting.dragging = false
	return true
}

// selectKey answers the keys of a selection while the surface holds it:
// Cmd-C copies, Cmd-A selects the area and Escape clears. It reports
// whether it took the key. The Edit menu's Copy and Select All reach the
// same methods; keys go here first, so menus without them work too.
func (s *surface) selectKey(event objc.ID) bool {
	if s.selecting == nil || s.window.Send(sel("firstResponder")) != s.native {
		return false
	}
	const shift, control, option, command = 1 << 17, 1 << 18, 1 << 19, 1 << 20
	const escape = 53
	mods := objc.Send[uint](event, sel("modifierFlags")) & (shift | control | option | command)
	switch key := apple.GoString(event.Send(sel("charactersIgnoringModifiers"))); {
	case mods == command && key == "c":
		surfaceCopy(s.native, 0, 0)
	case mods == command && key == "a":
		surfaceSelectAll(s.native, 0, 0)
	case mods == 0 && objc.Send[uint16](event, sel("keyCode")) == escape && !s.selecting.model.IsEmpty():
		s.clearSelection()
	default:
		return false
	}
	return true
}

// refreshSelection follows the area's texts after a batch: the selection
// keeps to its texts, and goes with its area.
func (s *surface) refreshSelection() {
	sl := s.selecting
	if sl == nil {
		return
	}
	if s.views[sl.area.id] != sl.area || sl.area.native.Send(selWindow) == 0 {
		s.clearSelection()
		return
	}
	old := sl.texts
	sl.list()
	sl.model.TextsChanged(sl)
	sl.show(old)
}

func (s *surface) clearSelection() {
	if s.selecting == nil {
		return
	}
	for _, v := range s.selecting.texts {
		v.setSelected(0, 0)
	}
	s.selecting = nil
}

// surfaceCopy is the surface's copy:, which copies the selection.
func surfaceCopy(self objc.ID, _ objc.SEL, _ objc.ID) {
	s := views[self].surface
	if s.selecting == nil || s.selecting.model.IsEmpty() {
		return
	}
	s.selecting.list()
	pb := objc.ID(objc.GetClass("NSPasteboard")).Send(sel("generalPasteboard"))
	pb.Send(sel("clearContents"))
	pb.Send(sel("setString:forType:"), apple.NSString(s.selecting.model.Text(s.selecting)), apple.NSString(pasteboardText))
}

// surfaceSelectAll is the surface's selectAll:, which selects the area.
func surfaceSelectAll(self objc.ID, _ objc.SEL, _ objc.ID) {
	s := views[self].surface
	if s.selecting == nil {
		return
	}
	old := s.selecting.texts
	s.selecting.list()
	s.selecting.model.SelectAll(s.selecting)
	s.selecting.show(old)
}

// surfaceResign clears the selection as another view takes the focus.
func surfaceResign(self objc.ID, cmd objc.SEL) bool {
	views[self].surface.clearSelection()
	return true
}

// selectionColor returns the CGColor text v highlights in, the text's
// SelectionColor or the system's, and whether the caller releases it.
func (v *view) selectionColor() (color uintptr, owned bool) {
	if v.selColor != 0 {
		return apple.CGColor(v.selColor), true
	}
	ns := objc.ID(objc.GetClass("NSColor")).Send(sel("selectedTextBackgroundColor"))
	return objc.Send[uintptr](ns, sel("CGColor")), false
}

// drawSelection fills the highlight behind a text's glyphs.
func (v *view) drawSelection(ctx uintptr) {
	if v.selEnd <= v.selStart {
		return
	}
	color, owned := v.selectionColor()
	apple.FillRects(ctx, v.layout.SelectionRects(v.selStart, v.selEnd, boxRect(v.box)), color)
	if owned {
		apple.Release(color)
	}
}
