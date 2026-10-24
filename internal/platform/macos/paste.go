//go:build darwin && !ios

package macos

import (
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/apple"
)

// The caret and pastes of text inputs. A field edits in the window's
// field editor, which is NSGo's own (NsgoFieldEditor, from the window's
// delegate) so that it hears pastes, as NSH's FieldEditor does; a text
// view hears its own. Secure fields keep AppKit's.

var classFieldEditor, classWindowDelegate objc.Class

func registerPaste() {
	paste := func(self objc.ID, cmd objc.SEL, sender objc.ID) {
		if pasteInto(self) {
			apple.Super(self, cmd, sender)
		}
	}
	validate := func(self objc.ID, cmd objc.SEL, item objc.ID) bool {
		if a := objc.Send[objc.SEL](item, sel("action")); a == sel("paste:") || a == sel("pasteAsPlainText:") {
			if in := pasteInput(self); in != nil && in.fs.Events.Has(driver.EventPaste) {
				return true // images and files too
			}
		}
		return apple.SendSuper[bool](self, cmd, item)
	}
	classFieldEditor = register("NsgoFieldEditor", objc.GetClass("NSTextView"), nil, []objc.MethodDef{
		{Cmd: sel("paste:"), Fn: paste},
		{Cmd: sel("pasteAsPlainText:"), Fn: paste},
		{Cmd: sel("validateUserInterfaceItem:"), Fn: validate},
	})
	classWindowDelegate = register("NsgoWindowDelegate", objc.GetClass("NSObject"),
		[]*objc.Protocol{objc.GetProtocol("NSWindowDelegate")}, []objc.MethodDef{{
			Cmd: sel("windowWillReturnFieldEditor:toObject:"),
			Fn: func(self objc.ID, _ objc.SEL, window, client objc.ID) objc.ID {
				if !objc.Send[bool](client, sel("isKindOfClass:"), objc.GetClass("NsgoField")) {
					return 0
				}
				s := delegates[self]
				if s == nil {
					return 0
				}
				if s.fieldEditor == 0 {
					s.fieldEditor = objc.ID(classFieldEditor).Send(selAlloc).Send(sel("init"))
					s.fieldEditor.Send(sel("setFieldEditor:"), true)
				}
				return s.fieldEditor
			},
		}})
}

// delegates are the surfaces of the window delegates.
var delegates = map[objc.ID]*surface{}

// setWindowDelegate gives s's window the delegate that hands its fields
// NSGo's field editor.
func (s *surface) setWindowDelegate() {
	d := objc.ID(classWindowDelegate).Send(selAlloc).Send(sel("init"))
	delegates[d] = s
	s.window.Send(selSetDelegate, d)
}

// pasteInput returns the input that editor, a text view or the field
// editor editing a field, pastes into.
func pasteInput(editor objc.ID) *input {
	if in := inputOf(editor); in != nil {
		return in
	}
	return inputOf(editor.Send(sel("delegate")))
}

// pasteInto reports a paste into editor's input, and whether the editor
// inserts its text: unless the input only reports pastes, or there is
// none.
func pasteInto(editor objc.ID) bool {
	in := pasteInput(editor)
	if in == nil || !in.fs.Events.Has(driver.EventPaste) {
		return true
	}
	p := pasted()
	in.fs.Dispatch(driver.Event{Kind: driver.EventPaste, ID: in.fs.ID, Data: p})
	return p.Text != "" && in.fs.Config.PasteBehavior == driver.PasteInsert
}

// pasted reads the general pasteboard as a paste: its text, its local
// files, or else its image.
func pasted() *driver.PasteEvent {
	pb := objc.ID(objc.GetClass("NSPasteboard")).Send(sel("generalPasteboard"))
	p := &driver.PasteEvent{Text: apple.GoString(pb.Send(sel("stringForType:"), apple.NSString(pasteboardText))),
		URIs: fileURLs(pb)}
	if len(p.URIs) > 0 {
		return p // a file's icon is no image of the user's
	}
	for _, t := range []struct{ uti, mime string }{
		{"public.png", "image/png"}, {"public.jpeg", "image/jpeg"}, {"public.tiff", "image/tiff"},
	} {
		if d := pb.Send(sel("dataForType:"), apple.NSString(t.uti)); d != 0 {
			n := objc.Send[uint](d, sel("length"))
			b := make([]byte, n)
			if n > 0 {
				copy(b, unsafe.Slice(objc.Send[*byte](d, sel("bytes")), n))
			}
			p.Files = append(p.Files, driver.PastedFile{MIME: t.mime, Data: b})
			break
		}
	}
	return p
}

// caretMoved reports where in's caret is, while it has the focus.
func (in *input) caretMoved() {
	if in == nil || !in.focused || !in.fs.Events.Has(driver.EventCaretMove) {
		return
	}
	v := views[in.field]
	e := in.editor()
	if v == nil || e == 0 || v.native.Send(selWindow) == 0 {
		return
	}
	r := objc.Send[apple.NSRange](e, selSelectedRange)
	screen := objc.Send[apple.CGRect](e, sel("firstRectForCharacterRange:actualRange:"),
		apple.NSRange{Location: r.Location + r.Length}, unsafe.Pointer(nil))
	var at apple.CGRect
	if screen.Size.Height > 0 {
		w := objc.Send[apple.CGRect](v.native.Send(selWindow), sel("convertRectFromScreen:"), screen)
		at = objc.Send[apple.CGRect](v.native, sel("convertRect:fromView:"), w, objc.ID(0))
	}
	in.fs.Dispatch(driver.Event{Kind: driver.EventCaretMove, ID: in.fs.ID,
		Point: driver.Point{X: driver.Points(at.Origin.X), Y: driver.Points(at.Origin.Y)},
		Size:  driver.Size{Width: driver.Points(max(at.Size.Width, 1)), Height: driver.Points(at.Size.Height)}})
}
