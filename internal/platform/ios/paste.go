//go:build ios

package ios

import (
	"net/url"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// The caret and pastes of text inputs: the field and text view classes
// hear their own paste:, and the caret is the editor's own caret rect,
// even with HideCaret.

// pasteMethods are the editors' paste and edit-menu methods.
func pasteMethods() []objc.MethodDef {
	paste := func(self objc.ID, cmd objc.SEL, sender objc.ID) {
		in := inputOf(self)
		if in != nil && in.editMenu != 0 {
			in.editMenu = 0
		}
		if in == nil {
			apple.Super(self, cmd, sender)
			return
		}
		if in.fs.Config.Readonly {
			return
		}
		pb := generalPasteboard()
		text, hasText, insert := in.pasteEvent(pb)
		if !insert {
			return
		}
		if shouldPasteSelf(in.fs.Config) {
			in.pasteText(pb, text, hasText)
			return
		}
		apple.Super(self, cmd, sender)
	}
	editAction := func(action driver.EditAction) func(objc.ID, objc.SEL, objc.ID) {
		return func(self objc.ID, cmd objc.SEL, sender objc.ID) {
			if editMenuChose(self, action) {
				return
			}
			apple.Super(self, cmd, sender)
		}
	}
	return []objc.MethodDef{
		{Cmd: sel("paste:"), Fn: paste},
		{Cmd: sel("cut:"), Fn: editAction(driver.EditCut)},
		{Cmd: sel("copy:"), Fn: editAction(driver.EditCopy)},
		{Cmd: sel("selectAll:"), Fn: editAction(driver.EditSelectAll)},
		{Cmd: sel("canPerformAction:withSender:"), Fn: func(self objc.ID, cmd objc.SEL, action objc.SEL, sender objc.ID) bool {
			if in := inputOf(self); in != nil && in.editMenu != 0 {
				return editMenuOffers(in.editMenu, action)
			}
			if in := inputOf(self); in != nil && in.fs.Config.Readonly && readonlyRefuses[action] {
				return false
			}
			if in := inputOf(self); action == sel("paste:") && in != nil && in.fs.Events.Has(driver.EventPaste) &&
				!in.fs.Config.Readonly {
				pb := objc.ID(apple.Class("UIPasteboard")).Send(sel("generalPasteboard"))
				if objc.Send[bool](pb, sel("hasImages")) || objc.Send[bool](pb, sel("hasURLs")) {
					return true // images and files too
				}
			}
			return apple.SendSuper[bool](self, cmd, action, sender)
		}},
		// The caret the editor draws, which caretRectForPosition: hides
		// with HideCaret.
		{Cmd: sel("nsgoCaretRectForPosition:"), Fn: func(self objc.ID, _ objc.SEL, pos objc.ID) apple.CGRect {
			return objc.Send[apple.CGRect](self, sel("caretRectForPosition:"), pos)
		}},
	}
}

// editMenuOffers answers canPerformAction:withSender: while a component's
// touch edit menu (ItemEditMenu) is open on the input: only the actions it
// requested (Paste when the pasteboard holds something, checked without
// reading it so iOS shows no paste banner).
func editMenuOffers(actions driver.EditAction, action objc.SEL) bool {
	switch action {
	case sel("cut:"):
		return actions&driver.EditCut != 0
	case sel("copy:"):
		return actions&driver.EditCopy != 0
	case sel("paste:"):
		if actions&driver.EditPaste == 0 {
			return false
		}
		pb := objc.ID(apple.Class("UIPasteboard")).Send(sel("generalPasteboard"))
		return objc.Send[bool](pb, sel("hasStrings")) || objc.Send[bool](pb, sel("hasImages")) || objc.Send[bool](pb, sel("hasURLs"))
	case sel("selectAll:"):
		return actions&driver.EditSelectAll != 0
	default:
		return false
	}
}

// editMenuChose reports action chosen from an edit menu open on editor.
func editMenuChose(editor objc.ID, action driver.EditAction) bool {
	in := inputOf(editor)
	if in == nil || in.editMenu == 0 {
		return false
	}
	in.editMenu = 0
	in.fs.Dispatch(driver.Event{Kind: driver.EventEditAction, ID: in.fs.ID, Count: uint32(action)})
	return true
}

// openEditMenu is ItemEditMenu: opens the system edit menu on the input's
// editor clear of bounds (in the surface's points), offering actions.
func (v *view) openEditMenu(bounds driver.Bounds, actions driver.EditAction) {
	in := v.input
	if in == nil || in.field == 0 {
		return
	}
	if !objc.Send[bool](in.field, sel("isFirstResponder")) && !objc.Send[bool](in.field, sel("becomeFirstResponder")) {
		return
	}
	surfRect := apple.Rect(float64(bounds.Origin.X), float64(bounds.Origin.Y), float64(bounds.Size.Width), float64(bounds.Size.Height))
	rect := objc.Send[apple.CGRect](in.field, sel("convertRect:fromView:"), surfRect, v.surface.native)
	in.editMenuGen++
	in.editMenu = actions
	in.editMenuRect = rect
	if class := apple.Class("UIEditMenuInteraction"); class != 0 {
		if in.editInteraction == 0 {
			in.editInteraction = objc.ID(class).Send(selAlloc).Send(sel("initWithDelegate:"), selector)
			in.field.Send(sel("addInteraction:"), in.editInteraction)
			in.editInteraction.Send(selRelease)
		}
		source := apple.CGPoint{X: rect.Origin.X + rect.Size.Width/2, Y: rect.Origin.Y}
		ident := objc.ID(apple.Class("NSNumber")).Send(sel("numberWithUnsignedInteger:"), in.editMenuGen)
		config := objc.ID(apple.Class("UIEditMenuConfiguration")).Send(
			sel("configurationWithIdentifier:sourcePoint:"), ident, source)
		// UIEditMenuArrowDirectionDown (2): above the selection where it fits.
		config.Send(sel("setPreferredArrowDirection:"), int(2))
		in.editInteraction.Send(sel("presentEditMenuWithConfiguration:"), config)
		return
	}
	menu := objc.ID(apple.Class("UIMenuController")).Send(sel("sharedMenuController"))
	menu.Send(sel("showMenuFromView:rect:"), in.field, rect)
}

// closeEditMenu closes the edit menu openEditMenu opened on in.
func (in *input) closeEditMenu() {
	if in == nil || in.editMenu == 0 {
		return
	}
	in.editMenu = 0
	if in.editInteraction != 0 {
		in.editInteraction.Send(sel("dismissMenu"))
	} else if in.field != 0 && apple.Class("UIEditMenuInteraction") == 0 {
		objc.ID(apple.Class("UIMenuController")).Send(sel("sharedMenuController")).Send(sel("hideMenuFromView:"), in.field)
	}
}

// readonlyRefuses are the edit menu's actions a read-only field leaves out, keeping those that copy, select and look up.
var readonlyRefuses = map[objc.SEL]bool{sel("cut:"): true, sel("paste:"): true, sel("pasteAndMatchStyle:"): true,
	sel("delete:"): true, sel("replace:"): true, sel("_promptForReplace:"): true, sel("captureTextFromCamera:"): true}

// pasteEvent reports a paste from pb into in when listened, returning any already-read text and whether the editor should insert text.
func (in *input) pasteEvent(pb objc.ID) (text string, hasText, insert bool) {
	if !in.fs.Events.Has(driver.EventPaste) {
		return "", false, in.fs.Config.PasteBehavior == driver.PasteInsert
	}
	p := &driver.PasteEvent{Text: apple.GoString(pb.Send(sel("string")))}
	if d := pb.Send(sel("dataForPasteboardType:"), apple.NSString("public.html")); d != 0 {
		p.HTML = string(apple.Bytes(d))
	}
	apple.ForEach(pb.Send(sel("URLs")), func(u objc.ID) {
		if objc.Send[bool](u, sel("isFileURL")) {
			p.URIs = append(p.URIs, (&url.URL{Scheme: "file", Path: apple.GoString(u.Send(sel("path")))}).String())
		}
	})
	for _, t := range []struct{ uti, mime string }{{"public.png", "image/png"}, {"public.jpeg", "image/jpeg"}} {
		if d := pb.Send(sel("dataForPasteboardType:"), apple.NSString(t.uti)); d != 0 {
			n := objc.Send[uint](d, sel("length"))
			b := make([]byte, n)
			if n > 0 {
				copy(b, unsafe.Slice(objc.Send[*byte](d, sel("bytes")), n))
			}
			p.Files = append(p.Files, driver.PastedFile{MIME: t.mime, Data: b})
			break
		}
	}
	in.fs.Dispatch(driver.Event{Kind: driver.EventPaste, ID: in.fs.ID, Data: p})
	return p.Text, true, p.Text != "" && len(p.URIs) == 0 && in.fs.Config.PasteBehavior == driver.PasteInsert
}

// pasteText inserts text from pb (or the text already read by pasteEvent) into in, clipped to MaxLength on a composed character sequence boundary, asking the delegate and calling insertText: so UIKit registers an undo step matching the inserted text rather than the unclipped pasteboard string.
func (in *input) pasteText(pb objc.ID, text string, hasText bool) {
	if !hasText {
		ns := pb.Send(sel("string"))
		if ns == 0 {
			return
		}
		text = apple.GoString(ns)
	}
	selRange, ok := in.selection()
	if !ok {
		return
	}
	out, replaced, ok := preparePasteText(text, in.multiline, in.text(), selRange, in.fs.Config.MaxLength, clipComposedUTF16)
	if !ok {
		return
	}
	nsRange := apple.NSRange{Location: uint(selRange.Start), Length: uint(replaced)}
	if !in.shouldChange(nsRange, out) {
		return
	}
	in.field.Send(sel("insertText:"), apple.NSString(out))
}

// clipComposedUTF16 trims s to at most maxUnits UTF-16 code units without splitting a composed character sequence (surrogate pairs, combining marks, ZWJ emoji or CRLF).
func clipComposedUTF16(s string, maxUnits int) string {
	if maxUnits <= 0 {
		return ""
	}
	ns := apple.NSString(s)
	n := int(objc.Send[uint](ns, sel("length")))
	if maxUnits >= n {
		return s
	}
	seq := objc.Send[apple.NSRange](ns, sel("rangeOfComposedCharacterSequenceAtIndex:"), uint(maxUnits))
	return apple.GoString(ns.Send(sel("substringToIndex:"), seq.Location))
}

// caretMoved reports where in's caret is, while it has the focus.
func (in *input) caretMoved() {
	if in == nil || !in.focused || !in.fs.Events.Has(driver.EventCaretMove) {
		return
	}
	v := views[in.field]
	r := in.field.Send(sel("selectedTextRange"))
	if v == nil || r == 0 {
		return
	}
	at := objc.Send[apple.CGRect](in.field, sel("nsgoCaretRectForPosition:"), r.Send(sel("end")))
	if at.Size.Height > 0 {
		at = objc.Send[apple.CGRect](in.field, sel("convertRect:toView:"), at, v.native)
	}
	in.fs.Dispatch(driver.Event{Kind: driver.EventCaretMove, ID: in.fs.ID,
		Point: driver.Point{X: driver.Points(at.Origin.X), Y: driver.Points(at.Origin.Y)},
		Size:  driver.Size{Width: driver.Points(at.Size.Width), Height: driver.Points(at.Size.Height)}})
}
