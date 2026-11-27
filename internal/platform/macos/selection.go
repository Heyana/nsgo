//go:build darwin && !ios

package macos

import (
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/desktop/locales"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// A selection area (a view with SelectionArea, or a selectable text
// outside one) selects with the mouse as a desktop does: drag, double
// click a word, triple click a whole text, Shift-click to extend. The
// surface takes the focus while an area has a selection, for Copy and
// Select All (the Edit menu, Cmd-C and Cmd-A) and Escape, which clears
// it; a press anywhere else clears it, so the window shows one selection.
// A drag past the edge of the scroll view around the area scrolls it on
// the frame clock, held still too, as NSH's does.

// selection is the surface's selection: an area's model over its texts.
type selection struct {
	area   *view
	texts  []*view
	frames []driver.Bounds // the texts' content boxes in the area
	model  driver.SelectionModel
	// down says the button pressed in the area is down, and dragging that
	// its drag selects; a press on a press target in the area (a button,
	// a link) goes to it as well, until the drag passes selectPressSlop
	// from where it went down, from.
	down, dragging bool
	press          *view
	from           apple.CGPoint
	// at is where the drag is, in the window.
	at apple.CGPoint
	// told, toldText and toldRanges are the extent, the text and the
	// per-text ranges the area last reported.
	told       driver.Bounds
	toldText   string
	toldRanges []driver.SelectedRange
}

const (
	// selectionScroll is how far a drag past a scroll view's edge scrolls
	// it a 60 Hz frame, per point past the edge, and selectionScrollMax
	// the most.
	selectionScroll    = 0.4
	selectionScrollMax = 40
	// selectPressSlop is how far a press on a press target in an area
	// drags before it selects instead, as NSH's does.
	selectPressSlop = 10
	// pasteboardLegacyText is NSStringPboardType, which macOS Services still
	// asks for beside public.utf8-plain-text.
	pasteboardLegacyText = "NSStringPboardType"
)

var classSelectionHelper objc.Class

// registerSelection registers the hidden non-editable NSTextView class that
// builds AppKit's Look Up, Translate, Share and Speech items for a selection's
// secondary-click menu without taking focus, hit tests or VoiceOver.
func registerSelection() {
	no := func(objc.ID, objc.SEL) bool { return false }
	classSelectionHelper = register("NsgoSelectionHelper", apple.Class("NSTextView"), nil, []objc.MethodDef{
		{Cmd: sel("hitTest:"), Fn: func(objc.ID, objc.SEL, apple.CGPoint) objc.ID { return 0 }},
		{Cmd: sel("acceptsFirstResponder"), Fn: no},
		{Cmd: sel("canBecomeKeyView"), Fn: no},
		{Cmd: sel("isAccessibilityElement"), Fn: no},
	})
}

// registerServicesMenuSendTypes tells macOS Services that selections in this
// app can send plain text to other apps' services.
func registerServicesMenuSendTypes(app objc.ID) {
	if app == 0 {
		return
	}
	sendTypes := objc.ID(apple.Class("NSMutableArray")).Send(sel("array"))
	sendTypes.Send(sel("addObject:"), apple.NSString(pasteboardText))
	sendTypes.Send(sel("addObject:"), apple.NSString(pasteboardLegacyText))
	returnTypes := objc.ID(apple.Class("NSArray")).Send(sel("array"))
	app.Send(sel("registerServicesMenuSendTypes:returnTypes:"), sendTypes, returnTypes)
}

// Len implements core.SelectionTexts.
func (s *selection) Len() int { return len(s.texts) }

// Content implements core.SelectionTexts.
func (s *selection) Content(i int) string { return s.texts[i].plain }

// Spans implements driver.SelectionSpans.
func (s *selection) Spans(i int) []driver.TextSpan { return s.texts[i].spans }

// Frame implements core.SelectionTexts.
func (s *selection) Frame(i int) driver.Bounds { return s.frames[i] }

// Key implements core.SelectionTexts.
func (s *selection) Key(i int) uint64 { return uint64(s.texts[i].id) }

// OffsetAt implements core.SelectionTexts.
func (s *selection) OffsetAt(i int, p driver.Point) int {
	v, f := s.texts[i], s.frames[i]
	at := apple.CGPoint{X: float64(p.X - f.Origin.X + v.box.Origin.X), Y: float64(p.Y - f.Origin.Y + v.box.Origin.Y)}
	return v.layout.OffsetAt(at, boxRect(v.box))
}

func boxRect(b driver.Bounds) apple.CGRect {
	return apple.BoundsRect(b)
}

// list finds the area's texts again, in document order, with their frames,
// which AppKit converts once the scroll offsets are committed.
func (s *selection) list() {
	s.area.surface.commitPending()
	s.texts, s.frames = s.texts[:0], s.frames[:0]
	var walk func(n objc.ID)
	walk = func(n objc.ID) {
		v := views[n]
		if v != nil && v.native == n && v.kind == driver.KindText {
			if v.layout != nil && v.selectable != driver.SelectableOff &&
				!objc.Send[bool](n, sel("isHiddenOrHasHiddenAncestor")) {
				r := objc.Send[apple.CGRect](s.area.native, sel("convertRect:fromView:"), boxRect(v.box), n)
				s.texts = append(s.texts, v)
				s.frames = append(s.frames, driver.Bounds{
					Origin: driver.Point{X: driver.Points(r.Origin.X), Y: driver.Points(r.Origin.Y)},
					Size:   driver.Size{Width: driver.Points(r.Size.Width), Height: driver.Points(r.Size.Height)},
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
func (s *selection) point(event objc.ID) driver.Point {
	return s.pointAt(objc.Send[apple.CGPoint](event, selLocationInWindow))
}

// pointAt returns a point of the window in the area.
func (s *selection) pointAt(loc apple.CGPoint) driver.Point {
	p := objc.Send[apple.CGPoint](s.area.native, selConvertPoint, loc, objc.ID(0))
	return driver.Point{X: driver.Points(p.X), Y: driver.Points(p.Y)}
}

// drag extends the selection to where the drag is.
func (s *selection) drag() {
	old := s.texts
	s.list()
	s.model.Drag(s, s.pointAt(s.at))
	s.show(old)
}

// scroller returns the scroll view around the area, if any.
func (s *selection) scroller() *view {
	for n := s.area; n != nil; n = n.up {
		if n.surface.alive(n) {
			return n
		}
	}
	return nil
}

// show has each text draw its part of the selection.
func (s *selection) show(old []*view) {
	for _, v := range old {
		v.setSelected(0, 0)
	}
	for _, r := range s.model.Ranges(s) {
		s.texts[r.Text].setSelected(r.Start, r.End)
	}
	s.tell(s.model.Extent(s))
}

// tell reports the selection's text, per-text ranges and extent to the area when any of them changed: the extent stays while the selection moves within one text, and so can the text and extent while the same words are chosen on another text. No Size is no selection, which has no text.
func (s *selection) tell(b driver.Bounds) {
	text := ""
	var ranges []driver.SelectedRange
	if b.Size.Height > 0 {
		text = s.model.Text(s)
		ranges = s.ranges()
	}
	if b != s.told || text != s.toldText || !slices.Equal(ranges, s.toldRanges) {
		s.told, s.toldText, s.toldRanges = b, text, ranges
		s.area.report(driver.Event{Kind: driver.EventSelectionChange, Point: b.Origin, Size: b.Size, Text: text, Ranges: ranges})
	}
}

// ranges returns what each text has selected, by its node, in document order: what EventSelectionChange's Ranges carries.
func (s *selection) ranges() []driver.SelectedRange {
	model := s.model.Ranges(s)
	out := make([]driver.SelectedRange, len(model))
	for i, r := range model {
		out[i] = driver.SelectedRange{ID: driver.NodeID(s.texts[r.Text].id), Start: r.Start, End: r.End}
	}
	return out
}

// setSelected sets the UTF-16 range a text highlights.
func (v *view) setSelected(start, end int) {
	if v.surface.views[v.id] != v {
		return // deleted
	}
	if v.selStart != start || v.selEnd != end {
		v.selStart, v.selEnd = start, end
		v.redraw()
	}
}

// areaAt returns the selection area a mouse press at event goes to, the
// outermost area around the view hit or the selectable text hit, and the
// press target it hit in the area, if any: a pressable view, or a text's
// link. A field or a gesture inside the area keeps the press.
func (s *surface) areaAt(event objc.ID) (area, press *view) {
	up := s.native.Send(selSuperview)
	if up == 0 {
		return nil, nil
	}
	at := objc.Send[apple.CGPoint](event, selLocationInWindow)
	p := objc.Send[apple.CGPoint](up, selConvertPoint, at, objc.ID(0))
	rawHit = true
	hit := s.native.Send(sel("hitTest:"), p)
	rawHit = false
	taken := false
	for n := hit; n != 0 && n != s.native; n = n.Send(selSuperview) {
		v := views[n]
		if v == nil || v.native != n && !v.isEditor(n) {
			continue
		}
		if area == nil && press == nil && !taken && v.native == n &&
			v.pressableAt(objc.Send[apple.CGPoint](n, selConvertPoint, at, objc.ID(0))) {
			press = v
		}
		switch {
		case v.isEditor(n) || v.kind == driver.KindInput || v.kind == driver.KindWeb:
			taken = true
		case area == nil && v.events.Has(driver.EventPan):
			taken = true
		case v.area && (area != nil || !taken):
			area = v
		case v.kind == driver.KindText && v.selectable == driver.SelectableOn && !taken:
			area = v
		}
	}
	return area, press
}

// selectPress starts or extends a selection on a mouse press, or clears
// the selection a press elsewhere leaves. It reports whether it took the
// press: one on a press target goes on to it.
func (s *surface) selectPress(event objc.ID) bool {
	area, press := s.areaAt(event)
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
	sl.down, sl.dragging, sl.press = true, press == nil, press
	// The drag starts where the press is: the frame clock may scroll
	// before the first drag event, from where the last drag ended.
	sl.from = objc.Send[apple.CGPoint](event, selLocationInWindow)
	sl.at = sl.from
	sl.show(old)
	s.window.Send(selMakeFirstResponer, s.native)
	return press == nil
}

// selectDrag follows a drag of the press that started a selection; the
// frame clock scrolls the scroll view around the area while the drag is
// past its edge (autoscrollSelection).
func (s *surface) selectDrag(event objc.ID) bool {
	sl := s.selecting
	if sl == nil || !sl.down {
		return false
	}
	sl.at = objc.Send[apple.CGPoint](event, selLocationInWindow)
	if !sl.dragging {
		if math.Hypot(sl.at.X-sl.from.X, sl.at.Y-sl.from.Y) <= selectPressSlop {
			return false
		}
		// The drag selects: the press target's press is no press.
		sl.dragging = true
		if p := sl.press; p != nil && p.pressing.Swap(false) {
			p.longFired.Store(true)
			p.pressed(driver.EventPressUp)
		}
	}
	sl.drag()
	if sl.scroller() != nil {
		s.startTicking()
	}
	return true
}

// autoscrollSelection scrolls the scroll view around a selection's area
// for a frame of dt while the drag is past its edge, extending the
// selection, and reports whether it does.
func (s *surface) autoscrollSelection(dt time.Duration) bool {
	sl := s.selecting
	if sl == nil || !sl.dragging {
		return false
	}
	v := sl.scroller()
	if v == nil || v.sc.phase.kind != atRest {
		return false
	}
	p := objc.Send[apple.CGPoint](v.native, selConvertPoint, sl.at, objc.ID(0))
	along, length := p.Y, float64(v.frame.Size.Height)
	if v.sc.config.Horizontal {
		along, length = p.X, float64(v.frame.Size.Width)
	}
	var over float64
	switch {
	case along < 0:
		over = along
	case along > length:
		over = along - length
	default:
		return false
	}
	step := min(max(over*selectionScroll, -selectionScrollMax), selectionScrollMax) * dt.Seconds() * 60
	lo, hi := v.restLimits()
	to := min(max(v.pos()+driver.Points(step), lo), hi)
	if to == v.pos() {
		return false
	}
	v.showIndicator() // the user's drag moves the view
	v.moveTo(to)
	v.commit()
	sl.drag()
	return true
}

// selectUp ends a selection's drag; the release of a press that never
// dragged goes on to its press target.
func (s *surface) selectUp() bool {
	sl := s.selecting
	if sl == nil || !sl.down {
		return false
	}
	took := sl.dragging
	sl.down, sl.dragging, sl.press = false, false, nil
	return took
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
	s.selecting.tell(driver.Bounds{})
	s.selecting = nil
}

// surfaceCopy is the surface's copy:, which copies the selection.
func surfaceCopy(self objc.ID, _ objc.SEL, _ objc.ID) {
	s := views[self].surface
	if s.selecting == nil || s.selecting.model.IsEmpty() {
		return
	}
	s.selecting.list()
	pb := objc.ID(apple.Class("NSPasteboard")).Send(sel("generalPasteboard"))
	if item, ok := s.selecting.model.ClipboardItem(s.selecting); ok {
		if err := writeClipboardItems(pb, []driver.ClipboardItem{item}); err == nil {
			return
		}
	}
	setClipboardText(pb, s.selecting.model.Text(s.selecting))
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

// surfaceValidateMenuItem enables Copy while the selection is non-empty and
// Select All while an area is active.
func surfaceValidateMenuItem(self objc.ID, _ objc.SEL, item objc.ID) bool {
	v := views[self]
	if v == nil || v.surface == nil {
		return false
	}
	s := v.surface
	switch objc.Send[objc.SEL](item, sel("action")) {
	case sel("copy:"):
		return s.selecting != nil && !s.selecting.model.IsEmpty()
	case sel("selectAll:"):
		return s.selecting != nil
	}
	return true
}

func isTextPasteboardType(t objc.ID) bool {
	if t == 0 {
		return false
	}
	name := apple.GoString(t)
	return name == pasteboardText || name == pasteboardLegacyText
}

func hasTextPasteboardType(types objc.ID) bool {
	if types == 0 {
		return false
	}
	found := false
	apple.ForEach(types, func(t objc.ID) {
		if !found && isTextPasteboardType(t) {
			found = true
		}
	})
	return found
}

// surfaceValidRequestor implements NSServicesMenuRequestor's negotiation on
// the surface so macOS Services (in both context menus and the app's Services
// submenu) see the active read-only selection as plain text.
func surfaceValidRequestor(self objc.ID, cmd objc.SEL, sendType, returnType objc.ID) objc.ID {
	if returnType == 0 && isTextPasteboardType(sendType) {
		if v := views[self]; v != nil && v.surface != nil && v.surface.selecting != nil && !v.surface.selecting.model.IsEmpty() {
			return self
		}
	}
	return apple.SendSuper[objc.ID](self, cmd, sendType, returnType)
}

// surfaceWriteSelection implements NSServicesMenuRequestor's
// writeSelectionToPasteboard:types: for the active selection.
func surfaceWriteSelection(self objc.ID, _ objc.SEL, pb, types objc.ID) bool {
	v := views[self]
	if v == nil || v.surface == nil || v.surface.selecting == nil || v.surface.selecting.model.IsEmpty() || pb == 0 || !hasTextPasteboardType(types) {
		return false
	}
	text := v.surface.selecting.selectedText()
	if text == "" {
		return false
	}
	pb.Send(sel("clearContents"))
	return objc.Send[bool](pb, sel("setString:forType:"), apple.NSString(text), apple.NSString(pasteboardText))
}

func (s *selection) selectedText() string {
	s.list()
	return s.model.Text(s)
}

// rectAt reports whether event lands on the selection's highlight, and returns
// the union of the selection's highlight rectangles in the surface's flipped
// coordinates.
func (s *selection) rectAt(event objc.ID) (apple.CGRect, bool) {
	if s.model.IsEmpty() || s.area == nil || s.area.surface == nil {
		return apple.CGRect{}, false
	}
	surfaceView := s.area.surface.native
	loc := objc.Send[apple.CGPoint](surfaceView, selConvertPoint, objc.Send[apple.CGPoint](event, selLocationInWindow), objc.ID(0))
	hit := false
	hasRect := false
	var minX, minY, maxX, maxY float64
	for _, r := range s.model.Ranges(s) {
		v := s.texts[r.Text]
		if v.layout == nil {
			continue
		}
		for _, b := range v.layout.SelectionRects(r.Start, r.End, boxRect(v.box)) {
			sr := objc.Send[apple.CGRect](surfaceView, sel("convertRect:fromView:"), b, v.native)
			if !hasRect {
				minX, minY = sr.Origin.X, sr.Origin.Y
				maxX, maxY = sr.Origin.X+sr.Size.Width, sr.Origin.Y+sr.Size.Height
				hasRect = true
			} else {
				minX = min(minX, sr.Origin.X)
				minY = min(minY, sr.Origin.Y)
				maxX = max(maxX, sr.Origin.X+sr.Size.Width)
				maxY = max(maxY, sr.Origin.Y+sr.Size.Height)
			}
			if loc.X >= sr.Origin.X && loc.X <= sr.Origin.X+sr.Size.Width &&
				loc.Y >= sr.Origin.Y && loc.Y <= sr.Origin.Y+sr.Size.Height {
				hit = true
			}
		}
	}
	if !hit || !hasRect {
		return apple.CGRect{}, false
	}
	return apple.Rect(minX, minY, maxX-minX, maxY-minY), true
}

// selectionMenuAt reports whether a secondary click at event should open the
// selection's system menu rather than clear or drag the selection.
func (s *surface) selectionMenuAt(event objc.ID) bool {
	sl := s.selecting
	if sl == nil || sl.model.IsEmpty() || s.contextView(event) != nil {
		return false
	}
	sl.list()
	_, ok := sl.rectAt(event)
	return ok
}

// menuForEvent supplies AppKit's context menu on secondary clicks over an
// active selection when no app context menu handles the click.
func menuForEvent(self objc.ID, cmd objc.SEL, event objc.ID) objc.ID {
	if v := views[self]; v != nil && v.surface != nil {
		if menu := v.surface.selectionMenu(event); menu != 0 {
			return menu
		}
	}
	return apple.SendSuper[objc.ID](self, cmd, event)
}

// quickLookWithEvent opens macOS's dictionary panel on a three-finger tap or
// Force Touch over an active selection.
func quickLookWithEvent(self objc.ID, cmd objc.SEL, event objc.ID) {
	if v := views[self]; v != nil && v.surface != nil && v.surface.lookUpSelection(event) {
		return
	}
	apple.Super(self, cmd, event)
}

func (s *surface) lookUpSelection(event objc.ID) bool {
	sl := s.selecting
	if sl == nil || sl.model.IsEmpty() {
		return false
	}
	sl.list()
	rect, ok := sl.rectAt(event)
	if !ok {
		return false
	}
	text := sl.model.Text(sl)
	if strings.TrimSpace(text) == "" {
		return false
	}
	s.lookUp(text, driver.Bounds{
		Origin: driver.Point{X: driver.Points(rect.Origin.X), Y: driver.Points(rect.Origin.Y)},
		Size:   driver.Size{Width: driver.Points(rect.Size.Width), Height: driver.Points(rect.Size.Height)},
	})
	return true
}

func (s *surface) ensureSelectionHelper() objc.ID {
	if s.selHelper == 0 {
		h := objc.ID(classSelectionHelper).Send(selAlloc).Send(sel("initWithFrame:"), apple.Rect(0, 0, 1, 1))
		h.Send(selSetEditable, false)
		h.Send(sel("setSelectable:"), true)
		h.Send(sel("setRichText:"), false)
		h.Send(sel("setDrawsBackground:"), false)
		h.Send(selSetTextColor, objc.ID(apple.Class("NSColor")).Send(sel("clearColor")))
		h.Send(sel("setSelectedTextAttributes:"), objc.ID(apple.Class("NSDictionary")).Send(sel("dictionary")))
		h.Send(sel("setTextContainerInset:"), apple.CGSize{})
		if tc := h.Send(sel("textContainer")); tc != 0 {
			tc.Send(sel("setLineFragmentPadding:"), 0.0)
		}
		h.Send(selSetAccessibilityElement, false)
		s.selHelper = h
	}
	if s.selHelper.Send(selSuperview) != s.native && s.native != 0 {
		s.native.Send(selAddSubview, s.selHelper)
	}
	return s.selHelper
}

// selectionMenu builds AppKit's secondary-click menu for the active selection:
// Look Up, Translate, Copy, Share, Speech and (via AppKit's context menu
// plug-ins and NsgoSurface's NSServicesMenuRequestor methods) macOS Services.
func (s *surface) selectionMenu(event objc.ID) objc.ID {
	sl := s.selecting
	if sl == nil || sl.model.IsEmpty() || s.contextView(event) != nil {
		return 0
	}
	sl.list()
	rect, ok := sl.rectAt(event)
	if !ok {
		return 0
	}
	text := sl.model.Text(sl)
	if text == "" {
		return 0
	}
	helper := s.ensureSelectionHelper()
	frame := apple.Rect(rect.Origin.X, rect.Origin.Y, max(rect.Size.Width, 1), max(rect.Size.Height, 1))
	helper.Send(selSetFrame, frame)
	helper.Send(selSetStringText, apple.NSString(text))
	if lm, tc := helper.Send(sel("layoutManager")), helper.Send(sel("textContainer")); lm != 0 && tc != 0 {
		lm.Send(sel("ensureLayoutForTextContainer:"), tc)
	}
	helper.Send(selSetSelRange, apple.NSRange{Location: 0, Length: uint(len(utf16.Encode([]rune(text))))})

	menu := helper.Send(sel("menuForEvent:"), event)
	if menu == 0 {
		menu = objc.ID(apple.Class("NSMenu")).Send(selAlloc).Send(sel("initWithTitle:"), apple.NSString("")).Send(sel("autorelease"))
	}
	s.filterSelectionMenu(menu, helper)
	menu.Send(sel("setAllowsContextMenuPlugIns:"), true)
	s.directMenu(menu)
	return menu
}

// filterSelectionMenu keeps the read-only text services from NSTextView's
// menu (Look Up, Translate, Copy, Share and Speech) and strips editing items.
func (s *surface) filterSelectionMenu(menu, helper objc.ID) {
	hasCopy := false
	for i := objc.Send[int](menu, sel("numberOfItems")) - 1; i >= 0; i-- {
		item := menu.Send(sel("itemAtIndex:"), i)
		if objc.Send[bool](item, sel("isSeparatorItem")) {
			continue
		}
		action := objc.Send[objc.SEL](item, sel("action"))
		switch {
		case action == sel("_rvMenuItemAction"):
			// Look Up and Translate items constructed by AppKit's Reveal integration.
		case action == sel("copy:"):
			item.Send(selSetTarget, s.native)
			hasCopy = true
		case action == sel("_performStandardShareMenuItem:"):
			// Share…
		case objc.Send[bool](item, sel("hasSubmenu")) && isSpeechSubmenu(item.Send(sel("submenu"))):
			retargetSubmenu(item.Send(sel("submenu")), helper)
		default:
			menu.Send(sel("removeItemAtIndex:"), i)
		}
	}
	if !hasCopy {
		copyItem := objc.ID(apple.Class("NSMenuItem")).Send(selAlloc).Send(
			sel("initWithTitle:action:keyEquivalent:"),
			apple.NSString(locales.In(s.language).Copy()), sel("copy:"), apple.NSString(""))
		copyItem.Send(selSetTarget, s.native)
		menu.Send(sel("addItem:"), copyItem)
		copyItem.Send(selRelease)
	}
	// Remove leading, trailing and consecutive separators left by stripped items.
	prevSep := true
	for i := 0; i < objc.Send[int](menu, sel("numberOfItems")); {
		item := menu.Send(sel("itemAtIndex:"), i)
		if objc.Send[bool](item, sel("isSeparatorItem")) {
			if prevSep {
				menu.Send(sel("removeItemAtIndex:"), i)
				continue
			}
			prevSep = true
		} else {
			prevSep = false
		}
		i++
	}
	for n := objc.Send[int](menu, sel("numberOfItems")); n > 0; n-- {
		if !objc.Send[bool](menu.Send(sel("itemAtIndex:"), n-1), sel("isSeparatorItem")) {
			break
		}
		menu.Send(sel("removeItemAtIndex:"), n-1)
	}
}

func isSpeechSubmenu(sub objc.ID) bool {
	if sub == 0 {
		return false
	}
	n := objc.Send[int](sub, sel("numberOfItems"))
	for i := 0; i < n; i++ {
		if objc.Send[objc.SEL](sub.Send(sel("itemAtIndex:"), i), sel("action")) == sel("startSpeaking:") {
			return true
		}
	}
	return false
}

func retargetSubmenu(sub, target objc.ID) {
	if sub == 0 {
		return
	}
	n := objc.Send[int](sub, sel("numberOfItems"))
	for i := 0; i < n; i++ {
		item := sub.Send(sel("itemAtIndex:"), i)
		if !objc.Send[bool](item, sel("isSeparatorItem")) {
			item.Send(selSetTarget, target)
		}
	}
}

// selectionColor returns the CGColor text v highlights in, the text's
// SelectionColor or the system's, and whether the caller releases it.
func (v *view) selectionColor() (color uintptr, owned bool) {
	if v.selColor != 0 {
		return apple.CGColor(v.selColor), true
	}
	ns := objc.ID(apple.Class("NSColor")).Send(sel("selectedTextBackgroundColor"))
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
