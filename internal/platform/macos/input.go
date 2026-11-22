//go:build darwin && !ios

package macos

import (
	"runtime"
	"slices"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/field"
)

// Text inputs follow NSH's: a borderless NSTextField (NSSecureTextField
// when masked) for a single line, or an NSTextView in an NSScrollView
// when multiline, at the content box inside the input's view, which
// draws the decoration. A text view has no placeholder, so a label under
// it shows one while the text is empty.
//
// AppKit edits a field with the window's shared field editor, whose
// delegate is the field: the field classes hear each change before it
// happens (maxLength, OnKeyPress), the selection, and the key binding
// commands (Return, Escape). The text view is its own delegate.
// field.State turns what they hear into NSH's events, and holds a value
// the app sets while an input method composes (the editor's marked
// text) until the composition ends.

// input is an input view's editor and what it tracks.
type input struct {
	fs         field.State
	field      objc.ID // the responder: an NSTextField or an NSTextView
	host       objc.ID // what is placed: the field, or the text view's scroll view
	label      objc.ID // a text view's placeholder
	lift       float64 // placeholder's lift to match the baselines of core.Text
	editorLift float64
	multiline  bool
	masked     bool
	focused    bool
	embedded   bool // in a host's view, whose app's accent is the host's
	// applying is a change made here, clipping an insertion cut to
	// maxLength, editing an edit allowed and not reported yet, focusing a
	// field taking the focus.
	applying, clipping, editing, focusing bool
	keyMods                               uint // of the key a text view handles
	pendingSelection                      *driver.TextRange
}

var (
	selCurrentEditor       = sel("currentEditor")
	selString              = sel("string")
	selSetStringText       = sel("setString:")
	selSelectedRange       = sel("selectedRange")
	selSetSelRange         = sel("setSelectedRange:")
	selHasMarkedText       = sel("hasMarkedText")
	selMarkedRange         = sel("markedRange")
	selShouldChange        = sel("textView:shouldChangeTextInRange:replacementString:")
	selDidChangeSel        = sel("textViewDidChangeSelection:")
	selFirstResponder      = sel("firstResponder")
	selLength              = sel("length")
	selUnmarkText          = sel("unmarkText")
	selInputContext        = sel("inputContext")
	selDiscardMarkedText   = sel("discardMarkedText")
	selFirstRectForRange   = sel("firstRectForCharacterRange:actualRange:")
	selDoCommand           = sel("textView:doCommandBySelector:")
	selControlCommand      = sel("control:textView:doCommandBySelector:")
	selInsertNewline       = sel("insertNewline:")
	selInsertNLIgnore      = sel("insertNewlineIgnoringFieldEditor:")
	selInsertLineBreak     = sel("insertLineBreak:")
	selCancelOperation     = sel("cancelOperation:")
	selInsertTab           = sel("insertTab:")
	selInsertBacktab       = sel("insertBacktab:")
	selInsertTabIgnore     = sel("insertTabIgnoringFieldEditor:")
	selDeleteBackward      = sel("deleteBackward:")
	selSetDefaultParagraph = sel("setDefaultParagraphStyle:")
	selSetTypingAttributes = sel("setTypingAttributes:")
	selTextStorage         = sel("textStorage")
	selAddAttribute        = sel("addAttribute:value:range:")

	classField, classSecureField, classTextView, classSecureEditor, classSecureFieldCell objc.Class

	selResetCursorRects = sel("resetCursorRects")
)

// nsNotFound is NSNotFound: no range to replace or no marked range.
const nsNotFound = uint(1<<63 - 1)

// registerInput registers the field and text view classes.
func registerInput() {
	fieldMethods := []objc.MethodDef{
		{Cmd: sel("controlTextDidChange:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) { inputOf(self).changed() }},
		// The field editor tells its field of marked text too, which the
		// field does not pass on as controlTextDidChange:.
		{Cmd: sel("textDidChange:"), Fn: func(self objc.ID, cmd objc.SEL, note objc.ID) {
			apple.Super(self, cmd, note)
			inputOf(self).changed()
		}},
		{Cmd: sel("controlTextDidEndEditing:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			in := inputOf(self)
			if in == nil {
				return
			}
			if !in.stillEditing() {
				in.focusChanged(false)
				return
			}
			// The field editor posts this from resignFirstResponder, still
			// the window's first responder: ask again once the focus moved.
			apple.OnMain(func() {
				if v := views[in.field]; v != nil && v.input == in && !in.stillEditing() {
					in.focusChanged(false)
				}
			})
		}},
		{Cmd: sel("becomeFirstResponder"), Fn: fieldFocused},
		{Cmd: selControlCommand, Fn: func(self objc.ID, _ objc.SEL, _, tv objc.ID, cmd objc.SEL) bool {
			return inputOf(self).command(tv, cmd)
		}},
		{Cmd: selShouldChange, Fn: fieldShouldChange},
		{Cmd: selDidChangeSel, Fn: fieldSelectionChanged},
		{Cmd: selResetCursorRects, Fn: func(objc.ID, objc.SEL) {}},
	}
	registerPaste()
	delegate := []*objc.Protocol{objc.GetProtocol("NSTextFieldDelegate")}
	classField = register("NsgoField", apple.Class("NSTextField"), delegate, fieldMethods)
	classSecureField = register("NsgoSecureField", apple.Class("NSSecureTextField"), delegate, fieldMethods)
	classSecureEditor = register("NsgoSecureEditor", apple.Class("NSSecureTextView"), nil, editorCursor())
	classSecureFieldCell = register("NsgoSecureFieldCell", apple.Class("NSSecureTextFieldCell"), nil, editorAccessMethods(true))
	classTextView = register("NsgoTextView", apple.Class("NSTextView"),
		[]*objc.Protocol{objc.GetProtocol("NSTextViewDelegate")}, slices.Concat(editorCursor(), editorAccessMethods(false), []objc.MethodDef{
			{Cmd: sel("textDidChange:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) { inputOf(self).changed() }},
			{Cmd: selShouldChange, Fn: func(self objc.ID, _ objc.SEL, tv objc.ID, r apple.NSRange, repl objc.ID) bool {
				return inputOf(self).shouldChange(tv, r, repl)
			}},
			{Cmd: selDidChangeSel, Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
				inputOf(self).selectionMoved()
			}},
			{Cmd: selDoCommand, Fn: func(self objc.ID, _ objc.SEL, tv objc.ID, cmd objc.SEL) bool {
				return inputOf(self).command(tv, cmd)
			}},
			{Cmd: sel("textContainerOrigin"), Fn: func(self objc.ID, _ objc.SEL) apple.CGPoint {
				in := inputOf(self)
				if in == nil {
					return apple.CGPoint{}
				}
				p := in.fs.Config.Padding
				return apple.CGPoint{X: float64(p.Left), Y: float64(p.Top) - in.editorLift}
			}},
			{Cmd: sel("keyDown:"), Fn: func(self objc.ID, cmd objc.SEL, event objc.ID) {
				in := inputOf(self)
				if in != nil {
					if v := views[in.field]; v != nil {
						defer v.surface.measuring(false)()
					}
					in.keyMods = objc.Send[uint](event, sel("modifierFlags"))
				}
				apple.Super(self, cmd, event)
				if in != nil {
					in.keyMods = 0
				}
			}},
			{Cmd: sel("becomeFirstResponder"), Fn: func(self objc.ID, cmd objc.SEL) bool {
				ok := apple.SendSuper[bool](self, cmd)
				if ok {
					inputOf(self).focusChanged(true)
				}
				return ok
			}},
			{Cmd: sel("resignFirstResponder"), Fn: func(self objc.ID, cmd objc.SEL) bool {
				in := inputOf(self)
				in.commitMarked(self)
				ok := apple.SendSuper[bool](self, cmd)
				if ok {
					in.focusChanged(false)
				}
				return ok
			}},
			{Cmd: selUnmarkText, Fn: func(self objc.ID, cmd objc.SEL) {
				apple.Super(self, cmd)
				inputOf(self).compositionChanged()
			}},
			{Cmd: selFirstRectForRange, Fn: firstRectForCharacterRange},
			{Cmd: sel("acceptsFirstMouse:"), Fn: func(objc.ID, objc.SEL, objc.ID) bool { return true }},
			{Cmd: sel("paste:"), Fn: func(self objc.ID, cmd objc.SEL, sender objc.ID) {
				if pasteInto(self) {
					apple.Super(self, cmd, sender)
				}
			}},
			{Cmd: sel("pasteAsPlainText:"), Fn: func(self objc.ID, cmd objc.SEL, sender objc.ID) {
				if pasteInto(self) {
					apple.Super(self, cmd, sender)
				}
			}},
			{Cmd: sel("validateUserInterfaceItem:"), Fn: validatePaste},
		}))
}

// editorCursor keeps a text view from setting the cursor, which is the
// surface's (updateCursor): AppKit sets the I-beam from the view's tracking
// area, its cursor rects and the moves a first responder hears, wherever
// the mouse is over it, overlays above it or not.
func editorCursor() []objc.MethodDef {
	return []objc.MethodDef{
		{Cmd: sel("addTrackingArea:"), Fn: func(objc.ID, objc.SEL, objc.ID) {}},
		{Cmd: selResetCursorRects, Fn: func(objc.ID, objc.SEL) {}},
		{Cmd: sel("mouseMoved:"), Fn: func(objc.ID, objc.SEL, objc.ID) {}},
		{Cmd: sel("cursorUpdate:"), Fn: cursorUpdate},
	}
}

// quietEditor does for the field editor what the field's class cannot:
// AppKit makes a secure field's, which becomes NSGo's as it comes, and the
// clip view around it with a cursor rect.
func (in *input) quietEditor() {
	e := in.field.Send(selCurrentEditor)
	if e == 0 {
		return
	}
	if e.Send(sel("class")) == objc.ID(apple.Class("NSSecureTextView")) {
		loadSetClass()
		objectSetClass(e, classSecureEditor)
		areas := e.Send(sel("trackingAreas"))
		for i := objc.Send[int](areas, sel("count")) - 1; i >= 0; i-- {
			e.Send(sel("removeTrackingArea:"), areas.Send(sel("objectAtIndex:"), i))
		}
	}
	if clip := e.Send(selSuperview); objc.Send[bool](clip, sel("respondsToSelector:"), sel("setDocumentCursor:")) {
		clip.Send(sel("setDocumentCursor:"), objc.ID(0))
	}
}

// inputOf returns the input of a field or text view, or nil.
func inputOf(editor objc.ID) *input {
	if v := views[editor]; v != nil {
		return v.input
	}
	return nil
}

// showsIBeam reports whether the cursor over hit, a view of the input, is
// the I-beam: its text selects, and hit is not the text view's scroll bar.
func (in *input) showsIBeam(hit objc.ID) bool {
	if in.multiline {
		return objc.Send[bool](in.field, sel("isSelectable")) &&
			!objc.Send[bool](hit, sel("isKindOfClass:"), apple.Class("NSScroller"))
	}
	return objc.Send[bool](in.field, sel("isEnabled"))
}

// isEditor reports whether n is v's editor, which keeps its presses.
func (v *view) isEditor(n objc.ID) bool { return v.input != nil && v.input.field == n }

// newInput makes an input view's editor.
func (v *view) newInput() {
	v.input = &input{fs: field.State{ID: v.id, Dispatch: v.surface.engine.Dispatch}, embedded: v.surface.embedded}
	v.setEditor(false, false)
}

// setEditor replaces an input's editor, keeping its text.
func (v *view) setEditor(multiline, masked bool) {
	in := v.input
	text := ""
	if in.field != 0 {
		text = in.text()
		in.host.Send(selRemoveFromSuperview)
		delete(views, in.field)
		delete(views, in.host)
		if in.label != 0 {
			in.label.Send(selRemoveFromSuperview)
			in.label = 0
		}
	}
	if multiline {
		scroll, tv := newMultilineEditor()
		label := objc.ID(apple.Class("NSTextField")).Send(sel("labelWithString:"), apple.NSString(""))
		label.Send(selSetHidden, true)
		// The placeholder is the text view's AXPlaceholderValue, so its drawn label is no element; the label's cell answers for it, so both stand down (accessibilityHidden leaves it in the tree VoiceOver walks).
		label.Send(selSetAccessibilityElement, false)
		label.Send(sel("cell")).Send(selSetAccessibilityElement, false)
		label.Send(sel("cell")).Send(sel("setWraps:"), true)
		v.children().Send(selAddSubview, label)
		in.field, in.host, in.label = tv, scroll, label
	} else {
		class := classField
		if masked {
			class = classSecureField
		}
		f := objc.ID(class).Send(selNew)
		// A secure field keeps AppKit's secure field editor: its cell only takes the input's accessibility (editorAccessMethods).
		cellClass := classFieldCell
		if masked {
			cellClass = classSecureFieldCell
		}
		loadSetClass()
		objectSetClass(f.Send(sel("cell")), cellClass)
		f.Send(sel("setBezeled:"), false)
		f.Send(sel("setBordered:"), false)
		f.Send(sel("setDrawsBackground:"), false)
		f.Send(sel("setFocusRingType:"), 1)
		f.Send(sel("setAllowsEditingTextAttributes:"), false)
		f.Send(selSetDelegate, f)
		cell := f.Send(sel("cell"))
		cell.Send(sel("setScrollable:"), true)
		cell.Send(sel("setWraps:"), false)
		// Not single-line mode: it replaces pasted line breaks in a second
		// edit, which shouldChange does at once.
		cell.Send(sel("setLineBreakMode:"), 2) // clipping
		in.field, in.host = f, f
	}
	in.multiline, in.masked = multiline, masked && !multiline
	v.children().Send(selAddSubview, in.host)
	in.host.Send(selRelease)
	views[in.field], views[in.host] = v, v
	if text != "" {
		in.replaceText(text)
	}
}

// newMultilineEditor returns a scroll view and its text view, which the
// scroll view keeps. The text view starts at the scroll view's content
// size: it follows the width as it changes, keeping the difference it
// starts with, and its container follows it.
func newMultilineEditor() (scroll, tv objc.ID) {
	scroll = objc.ID(apple.Class("NSScrollView")).Send(selNew)
	scroll.Send(sel("setBorderType:"), 0)
	scroll.Send(sel("setDrawsBackground:"), false)
	scroll.Send(sel("setHasVerticalScroller:"), true)
	scroll.Send(sel("setAutohidesScrollers:"), true)
	tv = objc.ID(classTextView).Send(selAlloc).Send(sel("initWithFrame:"), apple.CGRect{Size: objc.Send[apple.CGSize](scroll, sel("contentSize"))})
	tv.Send(sel("setMinSize:"), apple.CGSize{Width: 0, Height: 0})
	tv.Send(sel("setMaxSize:"), apple.CGSize{Width: 1e7, Height: 1e7})
	tv.Send(sel("setVerticallyResizable:"), true)
	tv.Send(sel("setHorizontallyResizable:"), false)
	tv.Send(sel("setAutoresizingMask:"), uint(2)) // width sizable
	tv.Send(sel("setDrawsBackground:"), false)
	tv.Send(sel("setRichText:"), false)
	tv.Send(sel("setImportsGraphics:"), false)
	tv.Send(sel("setAllowsUndo:"), true)
	tv.Send(sel("setTextContainerInset:"), apple.CGSize{})
	container := tv.Send(sel("textContainer"))
	container.Send(sel("setLineFragmentPadding:"), 0.0)
	container.Send(sel("setWidthTracksTextView:"), true)
	tv.Send(selSetDelegate, tv)
	scroll.Send(sel("setDocumentView:"), tv)
	scroll.Send(sel("setDocumentCursor:"), objc.ID(0))
	tv.Send(selRelease)
	return scroll, tv
}

// editor is the text view that edits now: the field editor while a
// field is edited, or the text view.
func (in *input) editor() objc.ID {
	if in.multiline {
		return in.field
	}
	return in.field.Send(selCurrentEditor)
}

// stillEditing reports whether a field's editor still has the keyboard
// focus. AppKit tells a field reopened in a dialog that its editing ended
// as it lays the field out, while it goes on editing: no blur.
func (in *input) stillEditing() bool {
	e := in.editor()
	w := in.field.Send(selWindow)
	return e != 0 && w != 0 && w.Send(sel("firstResponder")) == e
}

func (in *input) text() string {
	if e := in.editor(); e != 0 {
		return apple.GoString(e.Send(selString))
	}
	return apple.GoString(in.field.Send(selStringValue))
}

// markedText is what an input method composes in the editor, if it does.
func (in *input) markedText() (string, bool) {
	e := in.editor()
	if e == 0 || !objc.Send[bool](e, selHasMarkedText) {
		return "", false
	}
	r := objc.Send[apple.NSRange](e, selMarkedRange)
	s := e.Send(selString)
	if r.Length == 0 || r.Location+r.Length > objc.Send[uint](s, selLength) {
		return "", false
	}
	return apple.GoString(s.Send(sel("substringWithRange:"), r)), true
}

func (in *input) setText(text string) {
	ns := apple.NSString(text)
	switch e := in.editor(); {
	case in.multiline:
		in.field.Send(selSetStringText, ns)
	case e != 0:
		// The field takes the editor's text when editing ends; a new
		// string loses the editor's font and color.
		e.Send(selSetStringText, ns)
		e.Send(selSetFont, in.field.Send(sel("font")))
		e.Send(selSetTextColor, in.field.Send(sel("textColor")))
	default:
		in.field.Send(selSetString, ns)
	}
}

// selection is the editor's selection, if it has one: a field has one
// only while it is edited.
func (in *input) selection() (driver.TextRange, bool) {
	e := in.editor()
	if e == 0 {
		return driver.TextRange{}, false
	}
	r := objc.Send[apple.NSRange](e, selSelectedRange)
	return driver.TextRange{Start: int(r.Location), End: int(r.Location + r.Length)}, true
}

// selectRange selects r, clamped to the text, if the editor is there.
func (in *input) selectRange(r driver.TextRange) bool {
	e := in.editor()
	if e == 0 {
		return false
	}
	n := int(objc.Send[uint](e.Send(selString), selLength))
	start := min(max(r.Start, 0), n)
	end := min(max(r.End, start), n)
	e.Send(selSetSelRange, apple.NSRange{Location: uint(start), Length: uint(end - start)})
	return true
}

// replaceText sets the text without reporting it, keeping the caret
// where it was, at the end if it was there.
func (in *input) replaceText(text string) {
	old := driver.UTF16Len(in.text())
	caret, ok := in.selection()
	in.applying = true
	in.setText(text)
	in.styleLines()
	if ok {
		n := driver.UTF16Len(text)
		at := min(caret.End, n)
		if caret.End >= old {
			at = n
		}
		in.selectRange(driver.TextRange{Start: at, End: at})
	}
	in.applying = false
	in.updatePlaceholder()
	in.selectionChanged()
}

// styleLines styles a text view's lines after its text is replaced.
func (in *input) styleLines() {
	if !in.multiline {
		return
	}
	c := &in.fs.Config
	font := apple.Font(c.Style.Font)
	defer apple.Release(font)
	align := nsAlignLeft
	switch c.Style.Align {
	case driver.TextAlignCenter:
		align = nsAlignCenter
	case driver.TextAlignEnd:
		align = nsAlignRight
	}
	in.setLineStyle(objc.ID(font), nsColor(c.Style.Color), c.Style.LineHeight, align)
}

func (in *input) updatePlaceholder() {
	if in.label != 0 {
		in.label.Send(selSetHidden, in.fs.Config.Placeholder == "" || in.text() != "")
	}
}

// changed reports the user's edit. A composition that starts or changes
// is reported before the text holding it, one that ends after the text it
// committed, as browsers order them.
func (in *input) changed() {
	if in == nil {
		return
	}
	in.updatePlaceholder()
	in.editing = false
	if in.applying {
		return
	}
	marked, composing := in.markedText()
	if composing {
		in.compose(true, marked)
	}
	in.fs.Edited(in.text())
	if !composing {
		in.compositionChanged()
	}
	in.selectionChanged()
	in.caretMoved()
}

// selectionMoved hears the editor's selection change. Marking text does
// not report the change of the text, but moves the selection: a text an
// input method marks is reported from here.
func (in *input) selectionMoved() {
	if in == nil {
		return
	}
	if _, ok := in.markedText(); ok && !in.applying && in.text() != in.fs.Known() {
		in.changed()
		return
	}
	in.compositionChanged()
	in.selectionChanged()
	in.caretMoved()
}

// compositionChanged follows the editor's marked text, once its edits are
// reported.
func (in *input) compositionChanged() {
	if in == nil || in.applying || in.editing || in.fs.Known() != in.text() {
		return
	}
	marked, composing := in.markedText()
	in.compose(composing, marked)
}

func (in *input) compose(composing bool, marked string) {
	if value, apply := in.fs.Compose(composing, marked, in.text()); apply {
		in.replaceText(value)
	}
}

// commitMarked commits any marked text the editor still composes, ends the input method's session so it does not linger or drop the text on focus loss, and reports the committed text before ending the composition.
func (in *input) commitMarked(e objc.ID) {
	if in == nil || e == 0 || !objc.Send[bool](e, selHasMarkedText) {
		return
	}
	r := objc.Send[apple.NSRange](e, selMarkedRange)
	in.applying = true
	e.Send(selUnmarkText)
	if ic := e.Send(selInputContext); ic != 0 {
		ic.Send(selDiscardMarkedText)
	}
	if r.Location != nsNotFound && r.Length > 0 {
		n := objc.Send[uint](e.Send(selString), selLength)
		end := min(r.Location+r.Length, n)
		e.Send(selSetSelRange, apple.NSRange{Location: end})
	}
	in.applying = false
	in.updatePlaceholder()
	in.editing = false
	text := apple.GoString(e.Send(selString))
	in.fs.Edited(text)
	in.compose(false, "")
}

// commitMarked commits any composition in the surface's active editor.
func (s *surface) commitMarked() {
	if s == nil || s.window == 0 {
		return
	}
	fr := s.window.Send(selFirstResponder)
	if fr == 0 || !objc.Send[bool](fr, sel("isKindOfClass:"), apple.Class("NSTextView")) {
		return
	}
	pasteInput(fr).commitMarked(fr)
}

// firstRectForCharacterRange answers AppKit's candidate-window and caret geometry query. When the input is a point-wide sink (as code, rich text and terminal editors place at the caret), or when AppKit returns an empty rectangle because the range is clipped outside the view's visible box, return the input's own screen rectangle instead so macOS opens the candidate window beside the caret rather than at the screen's corner.
func firstRectForCharacterRange(self objc.ID, cmd objc.SEL, r apple.NSRange, actual unsafe.Pointer) apple.CGRect {
	rect := apple.SendSuper[apple.CGRect](self, cmd, r, actual)
	in := pasteInput(self)
	if in == nil {
		return rect
	}
	w := self.Send(selWindow)
	if w == 0 {
		return rect
	}
	if !isPointWide(in) && rect.Size.Height > 0 {
		return rect
	}
	if fallback, ok := fallbackCaretScreenRect(self, in, w); ok {
		return fallback
	}
	return rect
}

// isPointWide reports whether in is a caret-tracking input sink whose box is too narrow to lay out glyphs inside its clip view.
func isPointWide(in *input) bool {
	if in == nil || in.host == 0 {
		return false
	}
	return objc.Send[apple.CGRect](in.host, selBounds).Size.Width <= 2
}

// fallbackCaretScreenRect returns the input's screen-coordinate caret box for firstRectForCharacterRange:actualRange: when AppKit's layout geometry cannot place a range (point-wide sinks or clipped ranges).
func fallbackCaretScreenRect(self objc.ID, in *input, w objc.ID) (apple.CGRect, bool) {
	var anchor objc.ID
	var local apple.CGRect
	if in.multiline {
		anchor = self
		local = objc.Send[apple.CGRect](self, sel("visibleRect"))
		if local.Size.Height <= 0 && in.host != 0 {
			anchor = in.host
			local = objc.Send[apple.CGRect](in.host, selBounds)
		}
		if local.Size.Height <= 0 {
			if v := views[in.field]; v != nil {
				anchor = v.native
				local = objc.Send[apple.CGRect](anchor, selBounds)
			}
		}
		if local.Size.Height <= 0 {
			return apple.CGRect{}, false
		}
		if lm, font := self.Send(sel("layoutManager")), self.Send(sel("font")); lm != 0 && font != 0 {
			if lh := objc.Send[float64](lm, sel("defaultLineHeightForFont:"), font); lh > 0 && local.Size.Height > lh {
				if !objc.Send[bool](anchor, sel("isFlipped")) {
					local.Origin.Y += local.Size.Height - lh
				}
				local.Size.Height = lh
			}
		}
	} else {
		anchor = in.field
		local = objc.Send[apple.CGRect](anchor, selBounds)
		if local.Size.Height <= 0 {
			if v := views[in.field]; v != nil {
				anchor = v.native
				local = objc.Send[apple.CGRect](anchor, selBounds)
			}
		}
		if local.Size.Height <= 0 {
			return apple.CGRect{}, false
		}
	}
	winRect := objc.Send[apple.CGRect](anchor, sel("convertRect:toView:"), local, objc.ID(0))
	screen := objc.Send[apple.CGRect](w, sel("convertRectToScreen:"), winRect)
	return screen, screen.Size.Height > 0
}

// selectionChanged reports the user's selection.
func (in *input) selectionChanged() {
	if in == nil || in.applying || in.focusing || !in.fs.Events.Has(driver.EventSelectionChange) {
		return
	}
	e := in.editor()
	// A field's editor selects its text as editing ends: no change of the
	// user's.
	if e == 0 || e.Send(selWindow).Send(selFirstResponder) != e {
		return
	}
	if r, ok := in.selection(); ok {
		in.fs.Selected(r)
	}
}

func (in *input) focusChanged(focused bool) {
	if in == nil {
		return
	}
	if focused {
		in.styleEditor()
		if r := in.pendingSelection; r != nil {
			in.pendingSelection = nil
			in.selectRange(*r)
		}
	} else {
		// Leaving the input commits any remaining text and ends its composition.
		in.commitMarked(in.editor())
		in.editing = false
		in.fs.Edited(in.text())
		in.compose(false, "")
	}
	if in.focused == focused {
		return
	}
	in.focused = focused
	v := views[in.field]
	if focused {
		// The scroll views around show the field, as on phones.
		v.reveal()
		v.dispatch(driver.EventFocus)
		in.selectionChanged()
		in.caretMoved()
	} else {
		v.dispatch(driver.EventBlur)
	}
}

// shouldChange answers whether the editor may replace r with repl: it
// reports the key, keeps line breaks out of a single line and cuts the
// insertion to maxLength, making the cut edit itself.
func (in *input) shouldChange(tv objc.ID, r apple.NSRange, repl objc.ID) bool {
	if in == nil || in.applying || in.clipping || repl == 0 {
		return true
	}
	in.editing = true
	kept, ok, whole := in.fs.Insert(apple.GoString(tv.Send(selString)), int(r.Length), apple.GoString(repl))
	if whole {
		return true
	}
	in.editing = false
	if ok && kept != "" {
		ns := apple.NSString(kept)
		in.clipping = true
		if objc.Send[bool](tv, sel("shouldChangeTextInRange:replacementString:"), r, ns) {
			tv.Send(sel("replaceCharactersInRange:withString:"), r, ns)
			tv.Send(sel("didChangeText"))
		}
		in.clipping = false
	}
	return false
}

// command handles the editor's key binding commands: Return, line
// breaks, Escape, Tab and Backspace. It returns whether it handled one.
func (in *input) command(tv objc.ID, cmd objc.SEL) bool {
	if in == nil {
		return false
	}
	const shift = 1 << 17
	switch cmd {
	case selInsertNewline:
		if objc.Send[bool](tv, selHasMarkedText) {
			// A return that commits a composition only commits it.
			in.commitMarked(tv)
			return true
		}
		insert, blur := in.fs.Return(in.text(), in.keyMods&shift != 0)
		if insert {
			return false
		}
		if blur {
			in.unfocus()
		}
		return true
	case selInsertNLIgnore, selInsertLineBreak:
		// Option and Control with Return: a line break in a multiline
		// input whatever Return does; a single line holds none.
		if objc.Send[bool](tv, selHasMarkedText) {
			in.commitMarked(tv)
		} else if in.multiline {
			tv.Send(selInsertNLIgnore, objc.ID(0))
		}
		return true
	case selCancelOperation:
		in.fs.Key("Escape")
		in.unfocus()
		return true
	case selInsertTabIgnore:
		return !in.multiline
	case selInsertTab, selInsertBacktab:
		// Tab moves the focus out of a text view too.
		if in.multiline {
			step := sel("selectNextKeyView:")
			if cmd == selInsertBacktab {
				step = sel("selectPreviousKeyView:")
			}
			tv.Send(selWindow).Send(step, objc.ID(0))
			return true
		}
	case selDeleteBackward:
		// Backspace in an empty input deletes nothing, so no change
		// reports it.
		if !in.fs.Config.Readonly && objc.Send[uint](tv.Send(selString), selLength) == 0 {
			in.fs.Key("Backspace")
		}
	}
	return false
}

func (in *input) unfocus() {
	if w := in.field.Send(selWindow); w != 0 && in.focused {
		w.Send(selMakeFirstResponer, objc.ID(0))
	}
}

// focus focuses the input with the caret at the end, where AppKit would
// select a field's whole text.
func (in *input) focus() {
	w := in.field.Send(selWindow)
	if w == 0 || in.focused {
		return
	}
	w.Send(selMakeFirstResponer, in.field)
	if e := in.editor(); e != 0 && !in.multiline {
		n := objc.Send[uint](e.Send(selString), selLength)
		e.Send(selSetSelRange, apple.NSRange{Location: n})
	}
}

// setSelection is ItemSelection: applied now if the editor is there, else
// when the field takes the focus.
func (in *input) setSelection(r driver.TextRange) {
	in.fs.SetSelection(r)
	in.applying = true
	done := in.selectRange(r)
	in.applying = false
	if !done {
		in.pendingSelection = &r
	}
}

// value is ItemInputValue.
func (in *input) value(text string, count uint32) {
	if in.fs.Value(text, count) && in.text() != text {
		in.replaceText(text)
	}
}

// styleEditor sets the caret and selection colors and corrections of the
// view that edits now: the window's field editor is shared, so a field
// sets them as it takes the focus.
func (in *input) styleEditor() {
	e := in.editor()
	if e == 0 {
		return
	}
	c := &in.fs.Config
	caret := nsColor(c.Style.Color)
	if c.Caret() != 0 {
		caret = nsColor(c.Caret())
	}
	if c.HideCaret {
		caret = objc.ID(apple.Class("NSColor")).Send(sel("clearColor"))
	}
	e.Send(sel("setInsertionPointColor:"), caret)
	highlight := objc.ID(apple.Class("NSColor")).Send(sel("selectedTextBackgroundColor"))
	if h := c.Highlight(); h != 0 {
		highlight = nsColor(h)
	}
	if key := textAttrs().background; key != 0 {
		attrs := objc.ID(apple.Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"), highlight, key)
		e.Send(sel("setSelectedTextAttributes:"), attrs)
	}
	e.Send(sel("setContinuousSpellCheckingEnabled:"), !c.NoAutoCorrect)
	e.Send(sel("setAutomaticSpellingCorrectionEnabled:"), !c.NoAutoCorrect)
	in.followAccent(e)
}

// appAccent is the accent the app was last given.
var appAccent driver.Color

// followAccent gives the app the theme's accent, which input methods draw
// their candidates and marked text in. Info.plist's NSAccentColorName
// does the same but names one color for the app's life, so this takes
// AppKit's private setter and a system without it keeps its own accent;
// either way an accent the user chose in System Settings wins, as only
// Multicolor leaves it to the app. An input method reads the accent as
// its context activates, so the context activates again, unless it is
// composing: the next focus or theme change applies the color then.
func (in *input) followAccent(editor objc.ID) {
	accent := in.fs.Config.AccentColor
	if _, composing := in.markedText(); in.embedded || composing || accent == 0 || accent == appAccent {
		return
	}
	app, set := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication")), sel("_setAccentColor:")
	if !objc.Send[bool](app, sel("respondsToSelector:"), set) {
		return
	}
	app.Send(set, nsColor(accent))
	appAccent = accent
	context := editor.Send(selInputContext)
	context.Send(sel("deactivate"))
	context.Send(sel("activate"))
}

// AppKit's text alignments, which Apple silicon numbers as iOS does.
const nsAlignLeft = 0

var nsAlignRight, nsAlignCenter = func() (int, int) {
	if runtime.GOARCH == "arm64" {
		return 2, 1
	}
	return 1, 2
}()

// configure is ItemInput.
func (v *view) configure(c *driver.InputConfig) {
	in := v.input
	if c.Multiline != in.multiline || !c.Multiline && c.Masked != in.masked {
		focused := in.focused
		if focused {
			in.unfocus()
		}
		v.setEditor(c.Multiline, c.Masked)
		in.fs.Config = *c
		if focused {
			in.focus()
		}
	}
	if c.Disabled || c.Readonly {
		// Commit any active composition before the editor stops taking edits.
		in.commitMarked(in.editor())
	}
	in.fs.Config = *c
	font := apple.Font(c.Style.Font)
	align := nsAlignLeft
	switch c.Style.Align {
	case driver.TextAlignCenter:
		align = nsAlignCenter
	case driver.TextAlignEnd:
		align = nsAlignRight
	}
	in.field.Send(selSetFont, objc.ID(font))
	in.field.Send(selSetTextColor, nsColor(c.Style.Color))
	in.field.Send(selSetAlignment, align)
	in.field.Send(selSetEditable, !c.Readonly && !c.Disabled)
	if in.multiline {
		in.field.Send(sel("setSelectable:"), !c.Disabled)
		// The inset's halves sum each axis's padding; textContainerOrigin
		// puts the text after the top and left padding.
		p := c.Padding
		in.field.Send(sel("setTextContainerInset:"), apple.CGSize{Width: float64(p.Left+p.Right) / 2, Height: float64(p.Top+p.Bottom) / 2})
		in.label.Send(selSetFont, objc.ID(font))
		in.label.Send(selSetTextColor, nsColor(placeholderColor(c)))
		in.label.Send(selSetAlignment, align)
		setPlaceholderLabel(in.label, c.Placeholder, objc.ID(font), placeholderColor(c), align, c.Style.LineHeight)
		in.setLineStyle(objc.ID(font), nsColor(c.Style.Color), c.Style.LineHeight, align)
	} else {
		in.field.Send(selSetEnabled, !c.Disabled)
		// A read-only field still selects and copies: turning editing off turns selecting off with it, and the field editor of a field being edited keeps what it had.
		in.field.Send(sel("setSelectable:"), !c.Disabled)
		setPlaceholder(in.field, c.Placeholder, objc.ID(font), placeholderColor(c), align)
		if e := in.field.Send(selCurrentEditor); e != 0 {
			e.Send(selSetEditable, !c.Readonly && !c.Disabled)
			e.Send(selSetFont, objc.ID(font))
			e.Send(selSetTextColor, nsColor(c.Style.Color))
			e.Send(selSetAlignment, align)
		}
		setContentType(in.field, c.AutoComplete, in.masked)
	}
	apple.Release(font)
	if in.focused {
		in.styleEditor()
	}
	in.updatePlaceholder()
	v.inset = c.Content
	v.native.Send(selSetNeedsLayout, true)
}

// placeholderColor is the config's, else the text color at half strength,
// so the placeholder follows the app's colors as the text does.
func placeholderColor(c *driver.InputConfig) driver.Color {
	if c.PlaceholderColor != 0 {
		return c.PlaceholderColor
	}
	return c.Style.Color.Faded(0.5)
}

// textAttrs are AppKit's attribute names, read on first use.
var textAttrs = sync.OnceValue(func() (a struct{ font, color, background, paragraph objc.ID }) {
	const appKit = "/System/Library/Frameworks/AppKit.framework/AppKit"
	a.font = apple.Global(appKit, "NSFontAttributeName")
	a.color = apple.Global(appKit, "NSForegroundColorAttributeName")
	a.background = apple.Global(appKit, "NSBackgroundColorAttributeName")
	a.paragraph = apple.Global(appKit, "NSParagraphStyleAttributeName")
	return a
})

// newParagraph returns a paragraph style aligned as align whose lines are lineHeight tall when that is set, as a core.Text's are (apple/text.go). The caller releases it.
func newParagraph(lineHeight driver.Points, align int) objc.ID {
	para := objc.ID(apple.Class("NSMutableParagraphStyle")).Send(selAlloc).Send(sel("init"))
	para.Send(selSetAlignment, align)
	if lineHeight > 0 {
		para.Send(sel("setMinimumLineHeight:"), float64(lineHeight))
		para.Send(sel("setMaximumLineHeight:"), float64(lineHeight))
	}
	return para
}

// setLineStyle gives the text view's current and future text the input's alignment and line height.
func (in *input) setLineStyle(font, color objc.ID, lineHeight driver.Points, align int) {
	tv := in.field
	a := textAttrs()
	if tv == 0 || font == 0 || color == 0 || a.font == 0 || a.color == 0 || a.paragraph == 0 {
		return
	}
	in.lift, in.editorLift = 0, 0
	height := lineHeight
	if lineHeight > 0 {
		natural := objc.Send[float64](font, sel("ascender")) - objc.Send[float64](font, sel("descender"))
		in.lift = (float64(lineHeight) - natural) / 2
		in.editorLift = in.lift
		// TextKit 2 uses the line box for editing geometry. Put surplus
		// height below that box and move the origin to keep the baselines.
		manager := sel("textLayoutManager")
		if in.lift > 0 && objc.Send[bool](tv, sel("respondsToSelector:"), manager) && tv.Send(manager) != 0 {
			height = driver.Points(natural)
			in.editorLift = -in.lift
		}
	}
	para := newParagraph(height, align)
	if height < lineHeight {
		para.Send(sel("setLineSpacing:"), float64(lineHeight-height))
	}
	tv.Send(selSetDefaultParagraph, para)
	attrs := objc.ID(apple.Class("NSMutableDictionary")).Send(sel("dictionary"))
	attrs.Send(sel("setObject:forKey:"), font, a.font)
	attrs.Send(sel("setObject:forKey:"), color, a.color)
	attrs.Send(sel("setObject:forKey:"), para, a.paragraph)
	tv.Send(selSetTypingAttributes, attrs)
	if n := objc.Send[uint](tv.Send(selString), selLength); n > 0 {
		tv.Send(selTextStorage).Send(selAddAttribute, a.paragraph, para, apple.NSRange{Length: n})
	}
	para.Send(selRelease)
	tv.Send(sel("invalidateTextContainerOrigin"))
}

// setPlaceholder gives a field its placeholder in its font, color and
// alignment: an attributed placeholder keeps to the start otherwise.
func setPlaceholder(f objc.ID, text string, font objc.ID, color driver.Color, align int) {
	a := textAttrs()
	if a.font == 0 || a.color == 0 {
		f.Send(selSetPlaceholder, apple.NSString(text))
		return
	}
	attrs := objc.ID(apple.Class("NSMutableDictionary")).Send(sel("dictionary"))
	attrs.Send(sel("setObject:forKey:"), font, a.font)
	attrs.Send(sel("setObject:forKey:"), nsColor(color), a.color)
	if a.paragraph != 0 {
		para := newParagraph(0, align)
		attrs.Send(sel("setObject:forKey:"), para, a.paragraph)
		para.Send(selRelease)
	}
	str := objc.ID(apple.Class("NSAttributedString")).Send(selAlloc).Send(
		sel("initWithString:attributes:"), apple.NSString(text), attrs)
	f.Send(sel("setPlaceholderAttributedString:"), str)
	str.Send(selRelease)
}

// setPlaceholderLabel sets a text view's placeholder label, its lines as tall as the text view's.
func setPlaceholderLabel(label objc.ID, text string, font objc.ID, color driver.Color, align int, lineHeight driver.Points) {
	a := textAttrs()
	if lineHeight <= 0 || a.font == 0 || a.color == 0 || a.paragraph == 0 {
		label.Send(selSetString, apple.NSString(text))
		return
	}
	para := newParagraph(lineHeight, align)
	attrs := objc.ID(apple.Class("NSMutableDictionary")).Send(sel("dictionary"))
	attrs.Send(sel("setObject:forKey:"), font, a.font)
	attrs.Send(sel("setObject:forKey:"), nsColor(color), a.color)
	attrs.Send(sel("setObject:forKey:"), para, a.paragraph)
	str := objc.ID(apple.Class("NSAttributedString")).Send(selAlloc).Send(
		sel("initWithString:attributes:"), apple.NSString(text), attrs)
	label.Send(sel("setAttributedStringValue:"), str)
	str.Send(selRelease)
	para.Send(selRelease)
}

// setContentType gives a field the content type AppKit's autofill knows
// (macOS 11): accounts, passwords and one-time codes. A field without one
// has AutoFill guess on each focus, and the first guess after AutoFill
// starts flashes an empty panel over the field. So a plain field with no
// autocomplete, or with autocomplete off, gets a type AutoFill does not
// know, which it leaves alone; a masked one is a password AutoFill may
// guess.
func setContentType(f objc.ID, a driver.AutoComplete, masked bool) {
	if !objc.Send[bool](f, sel("respondsToSelector:"), sel("setContentType:")) {
		return
	}
	name := ""
	switch a {
	case driver.AutoCompleteUsername, driver.AutoCompleteEmail:
		name = "NSTextContentTypeUsername"
	case driver.AutoCompletePassword, driver.AutoCompleteNewPassword:
		name = "NSTextContentTypePassword"
	case driver.AutoCompleteOneTimeCode:
		name = "NSTextContentTypeOneTimeCode"
	}
	var t objc.ID
	switch {
	case name != "":
		t = apple.Global("/System/Library/Frameworks/AppKit.framework/AppKit", name)
	case a == driver.AutoCompleteOff, a == driver.AutoCompleteDefault && !masked:
		t = apple.NSString("none")
	}
	f.Send(sel("setContentType:"), t)
}

// inputLayout places the editor: a field's line centered in the content
// box, a text view's scroll view filling the padding box, so its scroller
// runs along the edges and the text scrolls under the padding.
func inputLayout(self objc.ID, cmd objc.SEL) {
	apple.Super(self, cmd)
	v := views[self]
	// AppKit can lay out a retained native view after unmount.
	if v == nil || v.input == nil {
		return
	}
	in, box := v.input, v.inset
	x, w := float64(box.Origin.X), float64(box.Size.Width)
	if in.multiline {
		p := in.fs.Config.Padding
		in.host.Send(selSetFrame, apple.Rect(x-float64(p.Left), float64(box.Origin.Y-p.Top), w+float64(p.Left+p.Right), float64(box.Size.Height+p.Top+p.Bottom)))
		h := objc.Send[apple.CGSize](in.label, selIntrinsicContentSize).Height
		in.label.Send(selSetFrame, apple.Rect(x, float64(box.Origin.Y)-in.lift, w, max(h, float64(box.Size.Height)+in.lift)))
		return
	}
	h := objc.Send[apple.CGSize](in.field, selIntrinsicContentSize).Height
	y := float64(box.Origin.Y) + (float64(box.Size.Height)-h)/2
	in.field.Send(selSetFrame, apple.Rect(x, y, w, h))
}

// fieldFocused hears a field take the focus. AppKit selects its whole
// text as it does: a step the focus reports, if it stays.
func fieldFocused(self objc.ID, cmd objc.SEL) bool {
	in := inputOf(self)
	if in != nil {
		in.focusing = true
	}
	ok := apple.SendSuper[bool](self, cmd)
	if in != nil {
		in.focusing = false
		if ok {
			in.quietEditor()
			in.focusChanged(true)
		}
	}
	return ok
}

// fieldShouldChange asks NSTextField, then the input, whether the field
// editor may change its text.
func fieldShouldChange(self objc.ID, cmd objc.SEL, tv objc.ID, r apple.NSRange, repl objc.ID) bool {
	ok := true
	if superResponds(self, cmd) {
		ok = apple.SendSuper[bool](self, cmd, tv, r, repl)
	}
	return ok && inputOf(self).shouldChange(tv, r, repl)
}

func fieldSelectionChanged(self objc.ID, cmd objc.SEL, note objc.ID) {
	if superResponds(self, cmd) {
		apple.Super(self, cmd, note)
	}
	inputOf(self).selectionMoved()
}

// superResponds reports whether AppKit's class of the field implements
// cmd.
func superResponds(self objc.ID, cmd objc.SEL) bool {
	super := apple.Class("NSTextField")
	if self.Class() == classSecureField {
		super = apple.Class("NSSecureTextField")
	}
	return objc.Send[bool](objc.ID(super), sel("instancesRespondToSelector:"), cmd)
}
