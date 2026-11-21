//go:build ios

package ios

import (
	"sync"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/field"
)

// Text inputs follow NSH's: a UITextField for one line, or a UITextView
// when multiline, at the content box inside the input's view, which
// draws the decoration and is the editor's delegate. A text view has no
// placeholder, so a label under it shows one while the text is empty.
// The delegate answers the editor's questions natively (maxLength, what
// Return does) and field.State turns what it hears into NSH's events,
// holding a value the app sets while an input method composes (the
// editor's marked text) until the composition ends.

type input struct {
	fs    field.State
	field objc.ID // the UITextField or UITextView
	label objc.ID // a text view's placeholder
	// lift is how far a text view's lines, and its placeholder's, are drawn above where UIKit puts them: UIKit sets a line's extra height above its glyphs, where core.Text centers the font in the line.
	lift      float64
	bar       objc.ID // the InputAccessoryView's view
	multiline bool
	focused   bool
	// applying is a change made here, clipping an insertion cut to
	// maxLength, lineBreak a line break typed with a modifier.
	applying, clipping, lineBreak bool
	// hardwareReturn keeps a physical Return's unhandled UIKit edit from being matched again as an unmodified software-keyboard action.
	hardwareReturn bool
	// Multiline scrolling: scrollApplied counts ItemScrollTo commands applied; lastScroll* deduplicate scrollViewDidScroll: notifications.
	scrollApplied     uint32
	lastScroll        driver.Point
	lastScrollMax     driver.Points
	lastScrollApplied uint32
	scrollReported    bool
	// editMenu is the touch edit menu ItemEditMenu opened on this input:
	// offered actions (0 when closed), its generation, its target rect in
	// the editor, and the UIEditMenuInteraction (iOS 16+).
	editMenu        driver.EditAction
	editMenuGen     uint
	editMenuRect    apple.CGRect
	editInteraction objc.ID
}

var classField, classTextView objc.Class

// inputMethods are the delegate methods of an input's view.
func inputMethods() []objc.MethodDef {
	return []objc.MethodDef{
		{Cmd: sel("layoutSubviews"), Fn: inputLayout},
		{Cmd: selChanged, Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) { inputAt(self).changed() }},
		{Cmd: sel("textFieldShouldReturn:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) bool {
			in := inputAt(self)
			if in == nil {
				return false
			}
			in.returnPressed()
			return false
		}},
		{Cmd: sel("textFieldDidBeginEditing:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) { inputAt(self).focusChanged(true) }},
		{Cmd: sel("textViewDidBeginEditing:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) { inputAt(self).focusChanged(true) }},
		{Cmd: sel("textFieldDidEndEditing:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) { inputAt(self).focusChanged(false) }},
		{Cmd: sel("textViewDidEndEditing:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) { inputAt(self).focusChanged(false) }},
		{Cmd: sel("textFieldDidChangeSelection:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) { inputAt(self).selectionMoved() }},
		{Cmd: sel("textViewDidChangeSelection:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) { inputAt(self).selectionMoved() }},
		{Cmd: sel("textViewDidChange:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) { inputAt(self).changed() }},
		{Cmd: sel("textField:shouldChangeCharactersInRange:replacementString:"), Fn: inputShouldChange},
		{Cmd: sel("textView:shouldChangeTextInRange:replacementText:"), Fn: inputShouldChange},
		{Cmd: sel("scrollViewDidScroll:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) { inputAt(self).didScroll() }},
	}
}

// registerInput registers the editor classes.
func registerInput() {
	caretRect := objc.MethodDef{Cmd: sel("caretRectForPosition:"), Fn: func(self objc.ID, cmd objc.SEL, pos objc.ID) apple.CGRect {
		if in := inputOf(self); in != nil && (in.fs.Config.HideCaret || in.fs.Config.Readonly) {
			return apple.CGRect{}
		}
		return apple.SendSuper[apple.CGRect](self, cmd, pos)
	}}
	deleteBackward := objc.MethodDef{Cmd: sel("deleteBackward"), Fn: func(self objc.ID, cmd objc.SEL) {
		in := inputOf(self)
		empty := in != nil && in.text() == ""
		apple.Super(self, cmd)
		// Deleting in an empty field changes nothing, so no delegate call
		// reports it.
		if empty {
			in.fs.Key("Backspace")
		}
	}}
	returnPress := objc.MethodDef{Cmd: sel("pressesBegan:withEvent:"), Fn: func(self objc.ID, cmd objc.SEL, presses, event objc.ID) {
		in := inputOf(self)
		hardwareReturn := false
		apple.ForEach(presses, func(p objc.ID) {
			if key := p.Send(sel("key")); key != 0 {
				code := objc.Send[int](key, sel("keyCode"))
				hardwareReturn = hardwareReturn || code == 40 || code == 88 // Return, keypad Enter
			}
		})
		if in == nil || !hardwareReturn {
			apple.Super(self, cmd, presses, event)
			return
		}
		in.hardwareReturn = true
		defer func() { in.hardwareReturn = false }()
		if _, composing := in.markedText(); composing {
			apple.Super(self, cmd, presses, event)
			return
		}
		// UITextField/UITextView can consume a physical Return before it reaches the controller: offer its actual modifiers first.
		pressesBegan(self, cmd, presses, event)
	}}
	returnEnd := objc.MethodDef{Cmd: sel("pressesEnded:withEvent:"), Fn: pressesEnded}
	returnCancel := objc.MethodDef{Cmd: sel("pressesCancelled:withEvent:"), Fn: pressesEnded}
	classField = register("NsgoField", apple.Class("UITextField"), nil,
		append(append([]objc.MethodDef{caretRect, deleteBackward, returnPress, returnEnd, returnCancel}, pasteMethods()...), editorAccess()...))
	classTextView = register("NsgoTextView", apple.Class("UITextView"), nil, append(append(pasteMethods(), editorAccess()...), caretRect, deleteBackward, returnPress, returnEnd, returnCancel,
		// Shift and Option with Return break the line of a text view whose
		// Return submits, as chat apps do on hardware keyboards.
		objc.MethodDef{Cmd: sel("keyCommands"), Fn: func(self objc.ID, cmd objc.SEL) objc.ID {
			inherited := apple.SendSuper[objc.ID](self, cmd)
			in := inputOf(self)
			if in == nil || in.fs.Config.Submit() == driver.SubmitBehaviorNewline {
				return inherited
			}
			commands := objc.ID(apple.Class("NSMutableArray")).Send(sel("array"))
			if inherited != 0 {
				commands.Send(sel("addObjectsFromArray:"), inherited)
			}
			for _, mods := range []int{1 << 17, 1 << 19} { // Shift, Option
				c := objc.ID(apple.Class("UIKeyCommand")).Send(sel("keyCommandWithInput:modifierFlags:action:"),
					apple.NSString("\r"), mods, sel("nsgoInsertLineBreak:"))
				c.Send(sel("setWantsPriorityOverSystemBehavior:"), true)
				commands.Send(sel("addObject:"), c)
			}
			return commands
		}},
		objc.MethodDef{Cmd: sel("nsgoInsertLineBreak:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			if in := inputOf(self); in != nil {
				in.lineBreak = true
				self.Send(sel("insertText:"), apple.NSString("\n"))
				in.lineBreak = false
			}
		}},
	))
}

// inputOf returns the input an editor belongs to; inputAt the one of an
// input's view.
func inputOf(editor objc.ID) *input {
	if v := views[editor]; v != nil {
		return v.input
	}
	return nil
}

func inputAt(native objc.ID) *input { return inputOf(native) }

func (v *view) newInput() {
	v.native = objc.ID(classInput).Send(selNew)
	v.input = &input{fs: field.State{ID: v.id, Dispatch: v.surface.engine.Dispatch}}
	v.setEditor(false)
}

// setEditor replaces an input's editor, keeping its text.
func (v *view) setEditor(multiline bool) {
	in := v.input
	text := ""
	if in.field != 0 {
		in.closeEditMenu()
		in.editInteraction = 0
		text = in.text()
		in.field.Send(selRemoveFromSuperview)
		delete(views, in.field)
		if in.label != 0 {
			in.label.Send(selRemoveFromSuperview)
			in.label = 0
		}
	}
	in.scrollApplied = 0
	in.lastScroll = driver.Point{}
	in.lastScrollMax = 0
	in.lastScrollApplied = 0
	in.scrollReported = false
	if multiline {
		tv := objc.ID(classTextView).Send(selNew)
		tv.Send(sel("setBackgroundColor:"), objc.ID(apple.Class("UIColor")).Send(sel("clearColor")))
		tv.Send(sel("setTextContainerInset:"), edgeInsets{})
		tv.Send(sel("textContainer")).Send(sel("setLineFragmentPadding:"), 0.0)
		tv.Send(selSetDeleg, v.native)
		label := objc.ID(apple.Class("UILabel")).Send(selNew)
		label.Send(sel("setNumberOfLines:"), 0)
		label.Send(selSetHidden, true)
		// The placeholder is exposed on the text view itself (editorAccess), so its drawn label is not a separate VoiceOver stop.
		label.Send(selSetIsAccessibilityElement, false)
		label.Send(selSetAccessibilityElementsHidden, true)
		in.field, in.label = tv, label
	} else {
		f := objc.ID(classField).Send(selNew)
		f.Send(selSetDeleg, v.native)
		const editingChanged = 1 << 17
		f.Send(sel("addTarget:action:forControlEvents:"), v.native, selChanged, uint(editingChanged))
		in.field = f
	}
	in.multiline = multiline
	// Place the editor (and placeholder behind it) underneath any overlay children such as a Scrollbar host.
	v.children().Send(sel("insertSubview:atIndex:"), in.field, 0)
	in.field.Send(selRelease)
	if in.label != 0 {
		v.children().Send(sel("insertSubview:atIndex:"), in.label, 0)
		in.label.Send(selRelease)
	}
	views[in.field] = v
	if v.surface != nil {
		v.surface.directTree(v.native)
	}
	if in.bar != 0 {
		in.field.Send(sel("setInputAccessoryView:"), in.bar)
	}
	if text != "" {
		in.replaceText(text)
	}
}

func (in *input) text() string { return apple.GoString(in.field.Send(selText)) }

// markedText is what an input method composes in the editor, if it does.
func (in *input) markedText() (string, bool) {
	r := in.field.Send(sel("markedTextRange"))
	if r == 0 {
		return "", false
	}
	t := apple.GoString(in.field.Send(sel("textInRange:"), r))
	return t, t != ""
}

// selection is the editor's selection in UTF-16 units.
func (in *input) selection() (driver.TextRange, bool) {
	f := in.field
	r := f.Send(sel("selectedTextRange"))
	if r == 0 {
		return driver.TextRange{}, false
	}
	begin := f.Send(sel("beginningOfDocument"))
	offset := func(p objc.ID) int {
		return max(objc.Send[int](f, sel("offsetFromPosition:toPosition:"), begin, p), 0)
	}
	return driver.TextRange{Start: offset(r.Send(sel("start"))), End: offset(r.Send(sel("end")))}, true
}

// selectRange selects r, clamped to the text.
func (in *input) selectRange(r driver.TextRange) {
	f := in.field
	begin := f.Send(sel("beginningOfDocument"))
	n := objc.Send[int](f, sel("offsetFromPosition:toPosition:"), begin, f.Send(sel("endOfDocument")))
	start := min(max(r.Start, 0), n)
	end := min(max(r.End, start), n)
	from := f.Send(sel("positionFromPosition:offset:"), begin, start)
	to := f.Send(sel("positionFromPosition:offset:"), begin, end)
	if from == 0 || to == 0 {
		return
	}
	f.Send(sel("setSelectedTextRange:"), f.Send(sel("textRangeFromPosition:toPosition:"), from, to))
}

// replaceText sets the text without reporting it, keeping the caret where it was, at the end if it was there. A focused editor replaces only what changed, through UITextInput, and tells the keyboard (programmatic).
func (in *input) replaceText(text string) {
	current := in.text()
	old := driver.UTF16Len(current)
	caret, ok := in.selection()
	in.applying = true
	if in.focused {
		in.programmatic(true, func() {
			if !in.replaceRange(current, text) {
				in.field.Send(selSetText, apple.NSString(text))
			}
			if ok {
				n := driver.UTF16Len(text)
				at := min(caret.End, n)
				if caret.End >= old {
					at = n
				}
				in.selectRange(driver.TextRange{Start: at, End: at})
			}
		})
	} else {
		in.field.Send(selSetText, apple.NSString(text))
	}
	in.styleLines()
	in.applying = false
	in.updatePlaceholder()
}

func uiTextAlignment(a driver.TextAlign, rtl bool) int {
	switch a {
	case driver.TextAlignCenter:
		return uiAlignCenter
	case driver.TextAlignEnd:
		if rtl {
			return uiAlignLeft
		}
		return uiAlignRight
	default:
		if rtl {
			return uiAlignRight
		}
		return uiAlignLeft
	}
}

// styleLines styles a text view's lines after its text is replaced.
func (in *input) styleLines() {
	if !in.multiline {
		return
	}
	c := &in.fs.Config
	font := apple.Font(c.Style.Font)
	defer apple.Release(font)
	styleLines(in.field, objc.ID(font), uiColor(c.Style.Color), c.Style.LineHeight, uiTextAlignment(c.Style.Align, c.Style.RTL), c.Style.Font.Language, c.Style.RTL)
}

// replaceRange replaces the part of current that differs from text, as the keyboard's own edits do, reporting whether the editor took it.
func (in *input) replaceRange(current, text string) bool {
	start, end, insert := field.Replaced(current, text)
	f := in.field
	begin := f.Send(sel("beginningOfDocument"))
	from := f.Send(sel("positionFromPosition:offset:"), begin, start)
	to := f.Send(sel("positionFromPosition:offset:"), begin, end)
	if from == 0 || to == 0 {
		return false
	}
	r := f.Send(sel("textRangeFromPosition:toPosition:"), from, to)
	if r == 0 {
		return false
	}
	f.Send(sel("replaceRange:withText:"), r, apple.NSString(insert))
	return in.text() == text
}

// programmatic makes change, NSGo's change to a focused editor's text (when text) and selection, between the input delegate's will and did calls, as UITextInput asks of changes from outside the text input system, so the keyboard rereads the text around the caret and drops what it offered for the old text. A keyboard that counts the characters of the word it is completing may still delete that many on its next key: on an iPhone, Qwen's English mode turned "@re" accepted as "README.md" into "README.c" with a following "c", as it does after a tap that moves the caret in Apple's own text fields, and only hiding and showing the keyboard resets it.
func (in *input) programmatic(text bool, change func()) {
	d := in.field.Send(sel("inputDelegate"))
	if d != 0 {
		if text {
			d.Send(sel("textWillChange:"), in.field)
		}
		d.Send(sel("selectionWillChange:"), in.field)
	}
	change()
	if d != 0 {
		d.Send(sel("selectionDidChange:"), in.field)
		if text {
			d.Send(sel("textDidChange:"), in.field)
		}
	}
}

func (in *input) updatePlaceholder() {
	if in.label != 0 {
		in.label.Send(selSetHidden, in.fs.Config.Placeholder == "" || in.text() != "")
	}
}

// changed reports the user's edit, a composition starting or changing
// before the text holding it and one ending after the text it committed.
func (in *input) changed() {
	if in == nil {
		return
	}
	in.updatePlaceholder()
	if in.applying {
		return
	}
	marked, composing := in.markedText()
	if composing {
		in.compose(true, marked)
	}
	in.fs.Edited(in.text())
	if !composing {
		in.compose(false, "")
	}
	in.selectionMoved()
}

func (in *input) compose(composing bool, marked string) {
	if value, apply := in.fs.Compose(composing, marked, in.text()); apply {
		in.replaceText(value)
	}
}

// selectionMoved reports the user's selection, and a composition the
// input method ended by unmarking, which leaves the text as it was.
func (in *input) selectionMoved() {
	if in == nil || in.applying || !in.focused {
		return
	}
	if in.text() == in.fs.Known() {
		marked, composing := in.markedText()
		in.compose(composing, marked)
	}
	if r, ok := in.selection(); ok && in.fs.Events.Has(driver.EventSelectionChange) {
		in.fs.Selected(r)
	}
	in.caretMoved()
}

func (in *input) focusChanged(focused bool) {
	if in == nil || in.focused == focused {
		return
	}
	in.focused = focused
	v := views[in.field]
	if focused {
		v.dispatch(driver.EventFocus)
		in.selectionMoved()
		v.surface.keepFocusedClear()
	} else {
		in.closeEditMenu()
		in.compose(false, "")
		v.dispatch(driver.EventBlur)
	}
	v.surface.refreshKeyCommands()
}

// takeSoftwareReturn offers one complete software-keyboard action to the surface's current keymap, without changing input modality or starting a hardware repeat timer. Native editing and composition keep their keys.
func (in *input) takeSoftwareReturn() bool {
	if in.hardwareReturn || in.applying || in.clipping || in.lineBreak || in.fs.Config.Readonly {
		return false
	}
	v := views[in.field]
	if v == nil || v.surface.engine == nil {
		return false
	}
	id, ok := driver.FindKey(v.surface.keys, []string{"enter"}, driver.KeyUseText, false)
	if ok {
		v.surface.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: id})
	}
	return ok
}

// returnPressed is UITextFieldDelegate's Return action. A Return that commits marked text belongs to the input method, before shortcuts too.
func (in *input) returnPressed() {
	if in.fs.Config.Readonly {
		return
	}
	if _, composing := in.markedText(); composing {
		return
	}
	if in.takeSoftwareReturn() {
		return
	}
	if _, blur := in.fs.Return(in.text(), false); blur {
		in.field.Send(selResign)
	}
}

// shouldChange answers whether the editor may replace r with repl: Return
// in a text view does what SubmitBehavior says, and an insertion is cut
// to maxLength, the cut edit made here.
func (in *input) shouldChange(r apple.NSRange, repl string) bool {
	if in.applying || in.clipping {
		return true
	}
	if in.fs.Config.Readonly {
		return false
	}
	if in.multiline && repl == "\n" && !in.lineBreak {
		if _, composing := in.markedText(); !composing {
			if in.takeSoftwareReturn() {
				return false
			}
			insert, blur := in.fs.Return(in.text(), false)
			if !insert {
				if blur {
					in.field.Send(selResign)
				}
				return false
			}
		}
	}
	kept, ok, whole := in.fs.Insert(in.text(), int(r.Length), repl)
	if whole {
		return true
	}
	if ok && kept != "" {
		f := in.field
		begin := f.Send(sel("beginningOfDocument"))
		from := f.Send(sel("positionFromPosition:offset:"), begin, int(r.Location))
		to := f.Send(sel("positionFromPosition:offset:"), begin, int(r.Location+r.Length))
		if from != 0 && to != 0 {
			in.clipping = true
			f.Send(sel("replaceRange:withText:"), f.Send(sel("textRangeFromPosition:toPosition:"), from, to), apple.NSString(kept))
			in.clipping = false
			in.changed()
		}
	}
	return false
}

func inputShouldChange(self objc.ID, _ objc.SEL, _ objc.ID, r apple.NSRange, repl objc.ID) bool {
	in := inputAt(self)
	return in == nil || in.shouldChange(r, apple.GoString(repl))
}

// value is ItemInputValue.
func (in *input) value(text string, count uint32) {
	if in.fs.Value(text, count) && in.text() != text {
		in.replaceText(text)
	}
}

// setSelection is ItemSelection.
func (in *input) setSelection(r driver.TextRange) {
	in.fs.SetSelection(r)
	in.applying = true
	if in.focused {
		in.programmatic(false, func() { in.selectRange(r) })
	} else {
		in.selectRange(r)
	}
	in.applying = false
}

// setBar makes bar the view above the editor's keyboard.
func (in *input) setBar(bar objc.ID) {
	in.bar = bar
	in.field.Send(sel("setInputAccessoryView:"), bar)
	if objc.Send[bool](in.field, sel("isFirstResponder")) {
		in.field.Send(sel("reloadInputViews"))
	}
}

// UIKit's text alignments.
const uiAlignLeft, uiAlignCenter, uiAlignRight = 0, 1, 2

// UIKeyboardType of each core keyboard type.
var keyboards = map[driver.KeyboardType]int{
	driver.KeyboardDefault: 0, driver.KeyboardURL: 3, driver.KeyboardNumber: 4,
	driver.KeyboardPhone: 5, driver.KeyboardEmail: 7, driver.KeyboardDecimal: 8,
}

// UIReturnKeyType of each return key.
var returnKeys = map[driver.ReturnKeyType]int{
	driver.ReturnKeyDefault: 0, driver.ReturnKeyGo: 1, driver.ReturnKeySearch: 6, driver.ReturnKeySend: 7,
	driver.ReturnKeyDone: 9, driver.ReturnKeyNext: 4,
}

// UITextAutocapitalizationType of each capitalization.
var capitalizations = map[driver.AutoCapitalize]int{
	driver.AutoCapitalizeNone: 0, driver.AutoCapitalizeWords: 1, driver.AutoCapitalizeSentences: 2,
	driver.AutoCapitalizeCharacters: 3,
}

// contentTypes are UIKit's UITextContentType constants.
var contentTypes = map[driver.AutoComplete]string{
	driver.AutoCompleteUsername: "UITextContentTypeUsername", driver.AutoCompletePassword: "UITextContentTypePassword",
	driver.AutoCompleteNewPassword: "UITextContentTypeNewPassword", driver.AutoCompleteOneTimeCode: "UITextContentTypeOneTimeCode",
	driver.AutoCompleteEmail: "UITextContentTypeEmailAddress", driver.AutoCompleteTel: "UITextContentTypeTelephoneNumber",
	driver.AutoCompleteName: "UITextContentTypeName", driver.AutoCompleteGivenName: "UITextContentTypeGivenName",
	driver.AutoCompleteFamilyName: "UITextContentTypeFamilyName", driver.AutoCompleteMiddleName: "UITextContentTypeMiddleName",
	driver.AutoCompleteNickname: "UITextContentTypeNickname", driver.AutoCompleteOrganization: "UITextContentTypeOrganizationName",
	driver.AutoCompleteStreetAddress: "UITextContentTypeFullStreetAddress", driver.AutoCompleteAddressLine1: "UITextContentTypeStreetAddressLine1",
	driver.AutoCompleteAddressLine2: "UITextContentTypeStreetAddressLine2", driver.AutoCompleteCity: "UITextContentTypeAddressCity",
	driver.AutoCompleteRegion: "UITextContentTypeAddressState", driver.AutoCompletePostalCode: "UITextContentTypePostalCode",
	driver.AutoCompleteCountry: "UITextContentTypeCountryName", driver.AutoCompleteCreditCardNumber: "UITextContentTypeCreditCardNumber",
	driver.AutoCompleteCreditCardExpiration:   "UITextContentTypeCreditCardExpiration",
	driver.AutoCompleteCreditCardSecurityCode: "UITextContentTypeCreditCardSecurityCode",
	driver.AutoCompleteCreditCardName:         "UITextContentTypeCreditCardName", driver.AutoCompleteURL: "UITextContentTypeURL",
	driver.AutoCompleteBirthdate: "UITextContentTypeBirthdate",
}

func uiColor(c driver.Color) objc.ID {
	r, g, b, a := c.Components()
	return objc.ID(apple.Class("UIColor")).Send(sel("colorWithRed:green:blue:alpha:"), r, g, b, a)
}

// nsLineBreakStrategyHangulWordPriority is NSLineBreakStrategyHangulWordPriority (iOS 14+).
const nsLineBreakStrategyHangulWordPriority uint = 1 << 1

// textAttrs are UIKit's attributed-string keys, read once because the
// constants are process-wide. UIKit exports these Foundation constants from
// its umbrella framework on iOS.
var textAttrs = sync.OnceValue(func() (a struct{ font, color, paragraph, language objc.ID }) {
	a.font = apple.Global(uikitPath, "NSFontAttributeName")
	a.color = apple.Global(uikitPath, "NSForegroundColorAttributeName")
	a.paragraph = apple.Global(uikitPath, "NSParagraphStyleAttributeName")
	a.language = apple.LanguageAttr()
	return a
})

// newParagraph returns a paragraph style aligned as align whose lines are lineHeight tall when that is set, as a core.Text's are (apple/text.go). Korean paragraphs break lines between words as CSS's word-break: keep-all does. The caller releases it.
func newParagraph(lineHeight driver.Points, align int, language string, rtl bool) objc.ID {
	para := objc.ID(apple.Class("NSMutableParagraphStyle")).Send(selAlloc).Send(sel("init"))
	para.Send(sel("setAlignment:"), align)
	para.Send(sel("setBaseWritingDirection:"), uiWritingDirection(rtl))
	if lineHeight > 0 {
		para.Send(sel("setMinimumLineHeight:"), float64(lineHeight))
		para.Send(sel("setMaximumLineHeight:"), float64(lineHeight))
	}
	if driver.KeepAllWords(language) {
		para.Send(sel("setLineBreakStrategy:"), nsLineBreakStrategyHangulWordPriority)
	}
	return para
}

func setLanguageAttr(attrs objc.ID, language string) {
	if a := textAttrs(); language != "" && a.language != 0 {
		attrs.Send(sel("setObject:forKey:"), apple.NSString(driver.KeepAllLanguage(language)), a.language)
	}
}

// styleLines gives a text view's text, and what is typed or pasted into it later, the input's alignment, line height and language. UIKit lays a text view's lines at its font's own height otherwise, and a new text drops the paragraph style the old one had, so this goes over the whole text each time.
func styleLines(tv, font, color objc.ID, lineHeight driver.Points, align int, language string, rtl bool) {
	a := textAttrs()
	if tv == 0 || font == 0 || color == 0 || a.font == 0 || a.color == 0 || a.paragraph == 0 {
		return
	}
	para := newParagraph(lineHeight, align, language, rtl)
	attrs := objc.ID(apple.Class("NSMutableDictionary")).Send(sel("dictionary"))
	attrs.Send(sel("setObject:forKey:"), font, a.font)
	attrs.Send(sel("setObject:forKey:"), color, a.color)
	attrs.Send(sel("setObject:forKey:"), para, a.paragraph)
	setLanguageAttr(attrs, language)
	tv.Send(sel("setTypingAttributes:"), attrs)
	if n := objc.Send[uint](tv.Send(selText), sel("length")); n > 0 {
		storage := tv.Send(sel("textStorage"))
		storage.Send(sel("addAttribute:value:range:"), a.paragraph, para, apple.NSRange{Length: n})
		if a.language != 0 {
			if language != "" {
				storage.Send(sel("addAttribute:value:range:"), a.language, apple.NSString(driver.KeepAllLanguage(language)), apple.NSRange{Length: n})
			} else {
				storage.Send(sel("removeAttribute:range:"), a.language, apple.NSRange{Length: n})
			}
		}
	}
	para.Send(selRelease)
}

// setFieldAttributes sets a UITextField's defaultTextAttributes so both existing and typed text carry the field's font, color and language.
func setFieldAttributes(f, font, color objc.ID, align int, language string, rtl bool) {
	a := textAttrs()
	if f == 0 || font == 0 || color == 0 || a.font == 0 || a.color == 0 {
		return
	}
	attrs := objc.ID(apple.Class("NSMutableDictionary")).Send(sel("dictionary"))
	attrs.Send(sel("setObject:forKey:"), font, a.font)
	attrs.Send(sel("setObject:forKey:"), color, a.color)
	if a.paragraph != 0 {
		para := newParagraph(0, align, language, rtl)
		attrs.Send(sel("setObject:forKey:"), para, a.paragraph)
		para.Send(selRelease)
	}
	setLanguageAttr(attrs, language)
	f.Send(sel("setDefaultTextAttributes:"), attrs)
}

// setPlaceholderLabel sets a text view's placeholder label, its lines as tall as the text view's and in its language.
func setPlaceholderLabel(label objc.ID, text string, font objc.ID, color driver.Color, lineHeight driver.Points, align int, language string, rtl bool) {
	a := textAttrs()
	if (lineHeight <= 0 && language == "") || a.font == 0 || a.color == 0 || a.paragraph == 0 {
		label.Send(selSetText, apple.NSString(text))
		return
	}
	para := newParagraph(lineHeight, align, language, rtl)
	attrs := objc.ID(apple.Class("NSMutableDictionary")).Send(sel("dictionary"))
	attrs.Send(sel("setObject:forKey:"), font, a.font)
	attrs.Send(sel("setObject:forKey:"), uiColor(color), a.color)
	attrs.Send(sel("setObject:forKey:"), para, a.paragraph)
	setLanguageAttr(attrs, language)
	str := objc.ID(apple.Class("NSAttributedString")).Send(selAlloc).Send(
		sel("initWithString:attributes:"), apple.NSString(text), attrs)
	label.Send(sel("setAttributedText:"), str)
	str.Send(selRelease)
	para.Send(selRelease)
}

// setPlaceholder gives a field its placeholder in its font, color, language, alignment and direction.
func setPlaceholder(f objc.ID, text string, font objc.ID, color driver.Color, language string, align int, rtl bool) {
	a := textAttrs()
	if a.font == 0 || a.color == 0 {
		f.Send(sel("setPlaceholder:"), apple.NSString(text))
		return
	}
	attrs := objc.ID(apple.Class("NSMutableDictionary")).Send(sel("dictionary"))
	attrs.Send(sel("setObject:forKey:"), uiColor(color), a.color)
	attrs.Send(sel("setObject:forKey:"), font, a.font)
	if a.paragraph != 0 {
		para := newParagraph(0, align, language, rtl)
		attrs.Send(sel("setObject:forKey:"), para, a.paragraph)
		para.Send(selRelease)
	}
	setLanguageAttr(attrs, language)
	str := objc.ID(apple.Class("NSAttributedString")).Send(selAlloc).Send(
		sel("initWithString:attributes:"), apple.NSString(text), attrs)
	f.Send(sel("setAttributedPlaceholder:"), str)
	str.Send(selRelease)
}

// configure is ItemInput.
func (v *view) configure(c *driver.InputConfig) {
	in := v.input
	if c.Multiline != in.multiline {
		focused := in.focused
		if focused {
			in.field.Send(selResign)
		}
		v.setEditor(c.Multiline)
		if focused {
			in.field.Send(selBecomeFirst)
		}
	}
	in.fs.Config = *c
	f := in.field
	font := apple.Font(c.Style.Font)
	f.Send(selSetFont, objc.ID(font))
	f.Send(selSetTextColor, uiColor(c.Style.Color))
	align := uiTextAlignment(c.Style.Align, c.Style.RTL)
	lang := c.Style.Font.Language
	f.Send(selSetTextAlignment, align)
	placeholder := c.Style.Color.Faded(0.5)
	if c.PlaceholderColor != 0 {
		placeholder = c.PlaceholderColor
	}
	if in.multiline {
		f.Send(sel("setShowsVerticalScrollIndicator:"), !c.HideIndicator)
		f.Send(sel("setShowsHorizontalScrollIndicator:"), false)
		in.label.Send(selSetFont, objc.ID(font))
		in.label.Send(selSetTextColor, uiColor(placeholder))
		in.label.Send(selSetTextAlignment, align)
		setPlaceholderLabel(in.label, c.Placeholder, objc.ID(font), placeholder, c.Style.LineHeight, align, lang, c.Style.RTL)
		styleLines(f, objc.ID(font), uiColor(c.Style.Color), c.Style.LineHeight, align, lang, c.Style.RTL)
		f.Send(sel("setEditable:"), !c.Readonly && !c.Disabled)
		f.Send(sel("setSelectable:"), !c.Disabled)
		in.lift = 0
		if c.Style.LineHeight > 0 {
			natural := objc.Send[float64](objc.ID(font), sel("ascender")) - objc.Send[float64](objc.ID(font), sel("descender"))
			in.lift = (float64(c.Style.LineHeight) - natural) / 2
		}
		// The lift comes off the top padding and goes below the text, so the text scrolls as far; UIKit takes no negative inset, so inputLayout raises the text view by what the top padding lacks.
		p := c.Padding
		f.Send(sel("setTextContainerInset:"), edgeInsets{Top: max(float64(p.Top)-in.lift, 0), Left: float64(p.Left),
			Bottom: max(float64(p.Bottom)+in.lift, 0), Right: float64(p.Right)})
	} else {
		setFieldAttributes(f, objc.ID(font), uiColor(c.Style.Color), align, lang, c.Style.RTL)
		f.Send(selSetTextAlignment, align)
		setPlaceholder(f, c.Placeholder, objc.ID(font), placeholder, lang, align, c.Style.RTL)
		f.Send(selSetEnabled, !c.Disabled)
		// A read-only field still takes the focus, so the user can select and copy its text, but with no keyboard: an empty view is its input view, and shouldChange refuses every edit.
		if c.Readonly != (f.Send(sel("inputView")) != 0) {
			var none objc.ID
			if c.Readonly {
				none = objc.ID(apple.Class("UIView")).Send(selNew)
			}
			f.Send(sel("setInputView:"), none)
			if none != 0 {
				none.Send(selRelease)
			}
			if objc.Send[bool](f, sel("isFirstResponder")) {
				f.Send(sel("reloadInputViews"))
			}
		}
	}
	apple.Release(font)
	f.Send(selSetSecure, c.Masked && !c.Multiline)
	f.Send(selSetKeyboardType, keyboards[c.KeyboardType])
	f.Send(sel("setReturnKeyType:"), returnKeys[c.ReturnKey])
	f.Send(sel("setAutocapitalizationType:"), capitalizations[c.AutoCapitalize])
	correction := 0 // default
	if c.NoAutoCorrect {
		correction = 1
	}
	f.Send(sel("setAutocorrectionType:"), correction)
	f.Send(sel("setKeyboardAppearance:"), int(c.KeyboardAppearance))
	var contentType objc.ID
	if name := contentTypes[c.AutoComplete]; name != "" {
		contentType = apple.Global(uikitPath, name)
	} else if c.AutoComplete == driver.AutoCompleteOff {
		contentType = apple.NSString("")
	}
	f.Send(sel("setTextContentType:"), contentType)
	if c.Caret() != 0 {
		f.Send(sel("setTintColor:"), uiColor(c.Caret()))
	} else {
		f.Send(sel("setTintColor:"), objc.ID(0))
	}
	in.updatePlaceholder()
	v.inset = c.Content
	v.native.Send(selSetNeedsLayout)
}

func inputLayout(self objc.ID, cmd objc.SEL) {
	apple.Super(self, cmd)
	v := views[self]
	// UIKit can lay out a view an outgoing transition still holds after the engine has unmounted it.
	if v == nil || v.input == nil {
		return
	}
	in, box := v.input, v.inset
	frame := apple.Rect(float64(box.Origin.X), float64(box.Origin.Y), float64(box.Size.Width), float64(box.Size.Height))
	if in.multiline {
		// The text view fills the padding box, its text inset by the
		// padding, so its indicator runs along the edges and the text
		// scrolls under the padding.
		p := in.fs.Config.Padding
		up := max(in.lift-float64(p.Top), 0)
		in.field.Send(selSetFrame, apple.Rect(float64(box.Origin.X-p.Left), float64(box.Origin.Y-p.Top)-up,
			float64(box.Size.Width+p.Left+p.Right), float64(box.Size.Height+p.Top+p.Bottom)+up))
	} else {
		in.field.Send(selSetFrame, frame)
	}
	if in.label != 0 {
		fit := objc.Send[apple.CGSize](in.label, sel("sizeThatFits:"), apple.CGSize{Width: frame.Size.Width, Height: 1e7})
		frame.Size.Height = min(fit.Height, frame.Size.Height+in.lift)
		frame.Origin.Y -= in.lift
		in.label.Send(selSetFrame, frame)
	}
}

func (in *input) scrollMetrics() (offset driver.Point, maxY driver.Points) {
	if !in.multiline || in.field == 0 {
		return driver.Point{}, 0
	}
	start, end := scrollLimits(in.field)
	off := objc.Send[apple.CGPoint](in.field, sel("contentOffset"))
	offset = driver.Point{
		X: driver.Points(off.X - start.X),
		Y: driver.Points(off.Y - start.Y),
	}
	if in.text() == "" {
		return offset, 0
	}
	maxY = driver.Points(max(end.Y-start.Y, 0))
	return offset, maxY
}

func (in *input) didScroll() {
	if in == nil || !in.multiline {
		return
	}
	in.reportScroll(false)
}

func (in *input) reportScroll(force bool) {
	if in == nil || !in.multiline || in.field == 0 {
		return
	}
	v := views[in.field]
	if v == nil || v.surface == nil {
		return
	}
	offset, maxY := in.scrollMetrics()
	if !force && in.scrollReported && offset == in.lastScroll && maxY == in.lastScrollMax && in.scrollApplied == in.lastScrollApplied {
		return
	}
	in.scrollReported = true
	in.lastScroll = offset
	in.lastScrollMax = maxY
	in.lastScrollApplied = in.scrollApplied
	frame := objc.Send[apple.CGRect](v.native, selBounds).Size
	v.report(driver.Event{
		Kind:  driver.EventScroll,
		Point: offset,
		Size:  driver.Size{Width: driver.Points(frame.Width), Height: driver.Points(frame.Height) + maxY},
		Count: in.scrollApplied,
	})
}

func (v *view) scrollInputTo(p driver.Point, animated bool) {
	in := v.input
	if in == nil || !in.multiline || in.field == 0 {
		return
	}
	in.scrollApplied++
	v.native.Send(sel("layoutIfNeeded"))
	in.field.Send(sel("layoutIfNeeded"))
	start, end := scrollLimits(in.field)
	cur := objc.Send[apple.CGPoint](in.field, sel("contentOffset"))
	to := apple.CGPoint{
		X: min(max(start.X+float64(p.X), start.X), end.X),
		Y: min(max(start.Y+float64(p.Y), start.Y), end.Y),
	}
	in.field.Send(sel("setContentOffset:animated:"), to, animated)
	if (!animated || cur == to) && in.lastScrollApplied != in.scrollApplied {
		in.reportScroll(true)
	}
}
