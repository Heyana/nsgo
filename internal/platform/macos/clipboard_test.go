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

func TestNativeFormulaClipboardMIME(t *testing.T) {
	onMain(func() {
		loadAppKit()
		pb := objc.ID(apple.Class("NSPasteboard")).Send(sel("pasteboardWithUniqueName"))
		if pb == 0 {
			t.Error("cannot create a named pasteboard")
			return
		}
		defer pb.Send(sel("releaseGlobally"))

		const (
			plain  = "$E=mc^2$"
			tex    = "E=mc^2"
			mathML = `<math xmlns="http://www.w3.org/1998/Math/MathML" display="inline"><mi>E</mi></math>`
		)
		items := []driver.ClipboardItem{{Representations: []driver.ClipboardRepresentation{
			{Type: "text/plain", Data: []byte(plain)},
			{Type: "application/mathml+xml", Data: []byte(mathML)},
			{Type: "text/x-tex", Data: []byte(tex)},
		}}}
		if err := writeClipboardItems(pb, items); err != nil {
			t.Errorf("writeClipboardItems(MIME): %v", err)
			return
		}
		if got := apple.GoString(pb.Send(sel("stringForType:"), apple.NSString(pasteboardText))); got != plain {
			t.Errorf("plain text = %q, want %q", got, plain)
		}
		saved, err := readClipboardItems(pb)
		if err != nil || len(saved) != 1 {
			t.Errorf("readClipboardItems: %d items, %v", len(saved), err)
			return
		}
		wantTypes := map[string]string{
			pasteboardText:             plain,
			"public.mathml":            mathML,
			"public.mathml.presentation": mathML,
			"org.tug.tex":              tex,
		}
		for _, mime := range []struct{ mime, data string }{
			{"application/mathml+xml", mathML},
			{"text/x-tex", tex},
		} {
			for _, uti := range apple.PasteboardTypes(mime.mime) {
				wantTypes[uti] = mime.data
			}
		}
		for uti, want := range wantTypes {
			if !slices.ContainsFunc(saved[0].Representations, func(r driver.ClipboardRepresentation) bool {
				return r.Type == uti && string(r.Data) == want
			}) {
				t.Errorf("pasteboard lacks %q = %q (got %#v)", uti, want, saved[0].Representations)
			}
		}
		// Snapshot round-trip keeps the written representations intact.
		if err := writeClipboardItems(pb, saved); err != nil {
			t.Errorf("restore snapshot: %v", err)
		}
	})
}
