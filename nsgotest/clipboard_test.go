package nsgotest

import (
	"encoding/binary"
	"reflect"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/nsgo-dev/nsgo/core"
)

func TestClipboardReadsRestoredWindowsText(t *testing.T) {
	const text = "copy 東京 😀"
	var data []byte
	for _, unit := range utf16.Encode([]rune(text + "\x00padding")) {
		data = binary.LittleEndian.AppendUint16(data, unit)
	}
	app := Start(t, func(cx *core.Scope) core.Node {
		got := core.NewSignal(cx, "")
		cx.WriteClipboardItems([]core.ClipboardItem{{Representations: []core.ClipboardRepresentation{
			{Type: "CF_UNICODETEXT", Data: data},
		}}}, func(err error) {
			if err != nil {
				t.Error(err)
			}
			cx.ReadClipboard(got.Set)
		})
		return core.Text{ContentFunc: got.Get}
	})
	app.Find("text:" + text)
}

func TestClipboardSaveOverwriteRestore(t *testing.T) {
	start := []core.ClipboardItem{
		{Representations: []core.ClipboardRepresentation{
			{Type: "public.utf8-plain-text", Data: []byte("bold")},
			{Type: "public.rtf", Data: []byte("{\\rtf1 \\b bold}")},
			{Type: "public.html", Data: []byte("<b>bold</b>")},
			{Type: "org.nspasteboard.ConcealedType"},
		}},
		{Representations: []core.ClipboardRepresentation{{Type: "public.file-url", Data: []byte("file:///tmp/sample.txt")}}},
	}
	var cx *core.Scope
	app := Start(t, func(scope *core.Scope) core.Node {
		cx = scope
		return core.View{}
	})
	var saved, restored []core.ClipboardItem
	var before, afterText, afterRestore int64
	app.Post(func() {
		cx.WriteClipboardItems(start, func(err error) {
			if err != nil {
				t.Error(err)
			}
			cx.ClipboardChangeCount(func(n int64) { before = n })
			cx.ReadClipboardItems(func(items []core.ClipboardItem, err error) {
				if err != nil {
					t.Error(err)
				}
				saved = items
			})
		})
	})
	app.Settle()
	if !reflect.DeepEqual(start, saved) {
		t.Fatalf("saved clipboard = %#v", saved)
	}

	app.Post(func() {
		cx.SetClipboard("temporary")
		cx.ClipboardChangeCount(func(n int64) { afterText = n })
	})
	app.Settle()
	if app.Clipboard != "temporary" || afterText <= before {
		t.Fatalf("overwrite gave %q, change count %d to %d", app.Clipboard, before, afterText)
	}

	app.Post(func() {
		cx.WriteClipboardItems(saved)
		cx.ClipboardChangeCount(func(n int64) { afterRestore = n })
		cx.ReadClipboardItems(func(items []core.ClipboardItem, err error) {
			if err != nil {
				t.Error(err)
			}
			restored = items
		})
	})
	app.WaitUntil(time.Second, func() bool { return len(restored) > 0 })
	if !reflect.DeepEqual(start, restored) || afterRestore <= afterText {
		t.Fatalf("restore gave %#v, change count %d to %d", restored, afterText, afterRestore)
	}
	if app.Clipboard != "bold" || len(app.ClipboardFiles) != 1 || app.ClipboardFiles[0] != "file:///tmp/sample.txt" {
		t.Errorf("restored clipboard has text %q and files %v", app.Clipboard, app.ClipboardFiles)
	}

	var failure error
	app.Post(func() {
		cx.WriteClipboardItems([]core.ClipboardItem{{Representations: []core.ClipboardRepresentation{
			{Type: "public.text", Data: make([]byte, core.MaxClipboardImage+1)},
		}}}, func(err error) { failure = err })
	})
	app.Settle()
	if core.ErrorCode(failure) != core.CodeQuotaExceeded || app.ClipboardChangeCount != afterRestore {
		t.Errorf("oversize write returned %v and changed count to %d", failure, app.ClipboardChangeCount)
	}
	app.Post(func() { cx.WriteClipboardItems(nil) })
	app.Settle()
	if len(app.ClipboardItems) != 0 || app.Clipboard != "" || app.ClipboardChangeCount <= afterRestore {
		t.Error("empty items did not clear the clipboard")
	}
}
