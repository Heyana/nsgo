//go:build ios

package ios

import (
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/platform/internal/apple"
)

// A selection area (a view with SelectionArea, or a selectable text
// outside one) selects with a long press: it selects the word under the
// finger, and the finger's drag goes on by words. Lifting the finger shows
// the edit menu: Copy and Select All, which the view controller answers
// as the first responder, hardware keyboards' Cmd-C and Cmd-A included.
// A touch outside the area clears it, so the surface shows one selection.
// Touch handles are not drawn yet.

// selection is the surface's selection: an area's model over its texts.
type selection struct {
	area   *view
	texts  []*view
	frames []core.Bounds // the texts' content boxes in the area
	model  core.SelectionModel
	menu   objc.ID // the area's UIEditMenuInteraction, iOS 16 on
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
		if objc.Send[bool](n, sel("isHidden")) {
			return
		}
		v := views[n]
		if v != nil && v.native == n && v.kind == core.KindText {
			if v.layout != nil && v.selectable != core.SelectableOff {
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
		v.native.Send(selSetNeedsDisplay)
	}
}

var (
	selSelectAction = sel("nsgoSelect:")
	classSelector   objc.Class
	selector        objc.ID // the long presses' target and delegate
)

func registerSelection() {
	classSelector = register("NsgoSelector", objc.GetClass("NSObject"), nil, []objc.MethodDef{
		{Cmd: selSelectAction, Fn: selectLongPress},
		{Cmd: sel("gestureRecognizer:shouldReceiveTouch:"), Fn: selectReceives},
	})
	selector = objc.ID(classSelector).Send(selNew)
}

// setSelectable gives v a long press recognizer while it is an area or a
// selectable text, and takes it away after.
func (v *view) setSelectable() {
	want := v.area || v.kind == core.KindText && v.selectable == core.SelectableOn
	switch {
	case want && v.longPress == 0:
		v.longPress = objc.ID(objc.GetClass("UILongPressGestureRecognizer")).Send(selAlloc).Send(
			sel("initWithTarget:action:"), selector, selSelectAction)
		v.longPress.Send(selSetDeleg, selector)
		v.native.Send(sel("addGestureRecognizer:"), v.longPress)
		v.longPress.Send(selRelease)
	case !want && v.longPress != 0:
		v.native.Send(sel("removeGestureRecognizer:"), v.longPress)
		v.longPress = 0
	}
}

// outermost returns the area a long press on v selects in: the outermost
// area around it, or v.
func outermost(v *view) *view {
	area := v
	for n := v.native.Send(selSuperview); n != 0; n = n.Send(selSuperview) {
		if a := views[n]; a != nil && a.native == n && a.area {
			area = a
		}
	}
	return area
}

// selectReceives keeps a long press from touches a field, a pressable
// view or a gesture inside the area takes.
func selectReceives(_ objc.ID, _ objc.SEL, r, touch objc.ID) bool {
	stop := r.Send(selView)
	for n := touch.Send(selView); n != 0 && n != stop; n = n.Send(selSuperview) {
		if v := views[n]; v != nil && v.native == n && (v.kind == core.KindInput || v.pressable() || v.pan != 0) {
			return false
		}
	}
	return true
}

// selectLongPress selects the word under a long press and follows its
// drag by words, then shows the edit menu.
func selectLongPress(_ objc.ID, _ objc.SEL, r objc.ID) {
	v := views[r.Send(selView)]
	if v == nil {
		return
	}
	s := v.surface
	switch objc.Send[int](r, selState) {
	case stateBegan:
		area := outermost(v)
		if s.selecting == nil || s.selecting.area != area {
			s.clearSelection()
			s.selecting = &selection{area: area}
		}
		sl := s.selecting
		old := sl.texts
		sl.list()
		sl.model.SelectWordAt(sl, sl.point(r))
		sl.show(old)
	case stateChanged:
		if sl := s.selecting; sl != nil {
			old := sl.texts
			sl.list()
			sl.model.Drag(sl, sl.point(r))
			sl.show(old)
		}
	case stateEnded:
		if sl := s.selecting; sl != nil && !sl.model.IsEmpty() {
			sl.showMenu(sl.point(r))
		}
	}
}

// point returns where recognizer r is in the area.
func (s *selection) point(r objc.ID) core.Point {
	p := objc.Send[apple.CGPoint](r, selLocationInView, s.area.native)
	return core.Point{X: core.Points(p.X), Y: core.Points(p.Y)}
}

// showMenu shows the edit menu at p: UIEditMenuInteraction from iOS 16,
// UIMenuController before.
func (s *selection) showMenu(p core.Point) {
	at := apple.CGPoint{X: float64(p.X), Y: float64(p.Y)}
	if class := objc.GetClass("UIEditMenuInteraction"); class != 0 {
		if s.menu == 0 {
			s.menu = objc.ID(class).Send(selAlloc).Send(sel("initWithDelegate:"), objc.ID(0))
			s.area.native.Send(sel("addInteraction:"), s.menu)
			s.menu.Send(selRelease)
		}
		config := objc.ID(objc.GetClass("UIEditMenuConfiguration")).Send(
			sel("configurationWithIdentifier:sourcePoint:"), objc.ID(0), at)
		s.menu.Send(sel("presentEditMenuWithConfiguration:"), config)
		return
	}
	menu := objc.ID(objc.GetClass("UIMenuController")).Send(sel("sharedMenuController"))
	menu.Send(sel("showMenuFromView:rect:"), s.area.native, apple.Rect(at.X, at.Y, 1, 1))
}

// clearTouched clears the selection when a touch goes down outside its
// area.
func (s *surface) clearTouched(touch objc.ID) {
	sl := s.selecting
	if sl == nil {
		return
	}
	hit := touch.Send(selView)
	if hit != 0 && (hit == sl.area.native || objc.Send[bool](hit, sel("isDescendantOfView:"), sl.area.native)) {
		return
	}
	s.clearSelection()
}

// refreshSelection follows the area's texts after a batch: the selection
// keeps to its texts, and goes with its area.
func (s *surface) refreshSelection() {
	sl := s.selecting
	if sl == nil {
		return
	}
	if s.views[sl.area.id] != sl.area || sl.area.native.Send(sel("window")) == 0 {
		s.clearSelection()
		return
	}
	old := sl.texts
	sl.list()
	sl.model.TextsChanged(sl)
	sl.show(old)
}

func (s *surface) clearSelection() {
	sl := s.selecting
	if sl == nil {
		return
	}
	for _, v := range sl.texts {
		v.setSelected(0, 0)
	}
	if sl.menu != 0 && s.views[sl.area.id] == sl.area {
		sl.menu.Send(sel("dismissMenu"))
		sl.area.native.Send(sel("removeInteraction:"), sl.menu)
	}
	s.selecting = nil
}

// controllerCopy is the view controller's copy:, which copies the
// selection.
func controllerCopy(_ objc.ID, _ objc.SEL, _ objc.ID) {
	sl := app.selecting
	if sl == nil || sl.model.IsEmpty() {
		return
	}
	sl.list()
	pb := objc.ID(objc.GetClass("UIPasteboard")).Send(sel("generalPasteboard"))
	pb.Send(sel("setString:"), apple.NSString(sl.model.Text(sl)))
}

// controllerSelectAll is the view controller's selectAll:, which selects
// the area and shows the menu again.
func controllerSelectAll(_ objc.ID, _ objc.SEL, _ objc.ID) {
	sl := app.selecting
	if sl == nil {
		return
	}
	old := sl.texts
	sl.list()
	sl.model.SelectAll(sl)
	sl.show(old)
	if len(sl.frames) > 0 {
		sl.showMenu(sl.frames[0].Origin)
	}
}

// controllerCanPerform offers Copy and Select All while there is a
// selection.
func controllerCanPerform(self objc.ID, cmd objc.SEL, action objc.SEL, sender objc.ID) bool {
	if action == sel("copy:") || action == sel("selectAll:") {
		return app.selecting != nil && !app.selecting.model.IsEmpty()
	}
	return apple.SendSuper[bool](self, cmd, action, sender)
}

// drawSelection fills the highlight behind a text's glyphs: its
// SelectionColor, else the tint at 20%.
func (v *view) drawSelection(ctx uintptr) {
	if v.selEnd <= v.selStart {
		return
	}
	var color uintptr
	if v.selColor != 0 {
		color = apple.CGColor(v.selColor)
		defer apple.Release(color)
	} else {
		tint := v.native.Send(sel("tintColor")).Send(sel("colorWithAlphaComponent:"), 0.2)
		color = objc.Send[uintptr](tint, sel("CGColor"))
	}
	apple.FillRects(ctx, v.layout.SelectionRects(v.selStart, v.selEnd, boxRect(v.box)), color)
}
