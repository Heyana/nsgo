//go:build darwin && !ios

package macos

import (
	"bytes"
	"reflect"
	"slices"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestNativeClipboardItems(t *testing.T) {
	onMain(func() {
		loadAppKit()
		pb := objc.ID(apple.Class("NSPasteboard")).Send(sel("pasteboardWithUniqueName"))
		if pb == 0 {
			t.Error("cannot create a named pasteboard")
			return
		}
		defer pb.Send(sel("releaseGlobally"))

		items := []driver.ClipboardItem{
			{Representations: []driver.ClipboardRepresentation{
				{Type: pasteboardText, Data: []byte("bold")},
				{Type: "public.rtf", Data: []byte("{\\rtf1 \\b bold}")},
				{Type: "public.html", Data: []byte("<b>bold</b>")},
				{Type: "org.nspasteboard.ConcealedType"},
			}},
			{Representations: []driver.ClipboardRepresentation{{Type: "public.file-url", Data: []byte("file:///tmp/clipboard-probe.txt")}}},
		}
		empty := pasteboardChangeCount(pb)
		if err := writeClipboardItems(pb, items); err != nil {
			t.Error(err)
			return
		}
		initial := pasteboardChangeCount(pb)
		saved, err := readClipboardItems(pb)
		if err != nil || len(items) != len(saved) {
			t.Errorf("saved %v items: %v", len(saved), err)
			return
		}
		// AppKit can offer additional synthesized representations.
		for i, item := range items {
			for _, rep := range item.Representations {
				if !slices.ContainsFunc(saved[i].Representations, func(r driver.ClipboardRepresentation) bool {
					return r.Type == rep.Type && bytes.Equal(r.Data, rep.Data)
				}) {
					t.Errorf("saved item %d lacks %q", i, rep.Type)
				}
			}
		}
		setClipboardText(pb, "temporary")
		if got := apple.GoString(pb.Send(sel("stringForType:"), apple.NSString(pasteboardText))); got != "temporary" {
			t.Errorf("temporary text = %q", got)
		}
		temporary := pasteboardChangeCount(pb)
		if err := writeClipboardItems(pb, saved); err != nil {
			t.Error(err)
			return
		}
		restored := pasteboardChangeCount(pb)
		if initial <= empty || temporary <= initial || restored <= temporary {
			t.Errorf("change counts = %d, %d, %d, %d", empty, initial, temporary, restored)
		}
		got, err := readClipboardItems(pb)
		if err != nil || !reflect.DeepEqual(saved, got) {
			t.Errorf("restored clipboard = %#v, %v", got, err)
		}
	})
}
