//go:build windows

package windows

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestRichEditGUIDs verifies the windowless RichEdit interface GUIDs against Windows' CLSIDFromString parser.
func TestRichEditGUIDs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		id         *guid
		documented string
	}{
		{"IID_ITextServices", &iidTextServices, "{8D33F740-CF58-11CE-A89D-00AA006CADC5}"},
		{"IID_ITextServices2", &iidTextServices2, "{8D33F741-CF58-11CE-A89D-00AA006CADC5}"},
		{"IID_ITextHost", &iidTextHost, "{13E670F4-1A5A-11CF-ABEB-00AA00B65EA1}"},
		{"IID_ITextHost2", &iidTextHost2, "{13E670F5-1A5A-11CF-ABEB-00AA00B65EA1}"},
	} {
		var want guid
		if hr, _, _ := clsidFromString.Call(uintptr(unsafe.Pointer(wide(tc.documented))), uintptr(unsafe.Pointer(&want))); failed(hr) {
			t.Fatalf("CLSIDFromString(%s): HRESULT %#x", tc.documented, uint32(hr))
		}
		if *tc.id != want {
			t.Errorf("%s = %s, want %s", tc.name, formatGUID(*tc.id), tc.documented)
		}
	}
}

// TestRichEditNativeEditingUndoMaskingScrollAndDraw exercises windowless RichEdit (ITextServices2 + ITextHost2) end to end on a real Windows surface: editing, selection, native undo/redo, password masking, spell-checking options, multiline vertical scrolling, and Direct2D TxDrawD2D rendering.
func TestRichEditNativeEditingUndoMaskingScrollAndDraw(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if !richEditAvailable() {
		t.Skip("msftedit.dll windowless RichEdit unavailable on this architecture")
	}

	title := "nsgo-richedit"
	app := func(cx *core.Scope) core.Node {
		plain := core.NewSignal(cx, "")
		secret := core.NewSignal(cx, "")
		multi := core.NewSignal(cx, strings.Repeat("line of text for scrolling\n", 14))
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|%s|%s|%d", title, plain.Get(), secret.Get(), len(multi.Get())))
		})
		return core.View{
			Style: core.Style{Width: driver.Pt(340), Height: driver.Pt(260), Padding: driver.All(10), Gap: 8,
				Background: driver.RGB(0xffffff)},
			Children: []core.Node{
				core.Input{
					TestID: "plain", Value: plain.Get, OnChange: plain.Set, AutoFocus: true,
					Style: core.Style{Width: driver.Pt(300), Height: driver.Pt(32), FontSize: 16},
				},
				core.Input{
					TestID: "secret", Value: secret.Get, OnChange: secret.Set, Masked: true,
					Style: core.Style{Width: driver.Pt(300), Height: driver.Pt(32), FontSize: 16},
				},
				core.Input{
					TestID: "multi", Value: multi.Get, OnChange: multi.Set, Multiline: true, NoAutoCorrect: true,
					Style: core.Style{Width: driver.Pt(300), Height: driver.Pt(80), FontSize: 16},
				},
			},
		}
	}

	hwnd := startApp(t, title, app, 340, 260)

	findField := func(w *window, testID string) *textInput {
		for _, v := range w.views {
			if v != nil && v.props.TestID == testID && v.field != nil {
				return v.field
			}
		}
		return nil
	}

	// 1. Plain input: verify ITextServices2 is active, spell checking enabled,    editing, selection, and native undo/redo work.
	typeText(hwnd, "hello")
	waitForTitle(t, hwnd, title+"|hello||", 5*time.Second)

	onMain(t, hwnd, func(w *window) {
		in := findField(w, "plain")
		if in == nil || in.re == nil || in.re.ts2 == nil {
			t.Fatalf("plain field did not initialize windowless RichEdit ITextServices2")
		}
		if got := in.re.getText(); got != "hello" {
			t.Fatalf("RichEdit text = %q, want %q", got, "hello")
		}
		// Spell checking option enabled on editable unmasked field.
		if opts, ok := in.re.sendMessage(emGetLangOptions, 0, 0); !ok || uint32(opts)&imfSpellChecking == 0 {
			t.Errorf("plain field EM_GETLANGOPTIONS = %#x (ok=%v), want IMF_SPELLCHECKING", opts, ok)
		}
		// Replace selection "ell" -> "i", then undo and redo via RichEdit.
		in.selectRange(driver.TextRange{Start: 1, End: 4})
		if s, e := in.re.getSel(); s != 1 || e != 4 {
			t.Fatalf("RichEdit selection = %d..%d, want 1..4", s, e)
		}
		in.insert("i")
		if got := in.re.getText(); got != "hio" {
			t.Fatalf("after replacement text = %q, want %q", got, "hio")
		}
		if !in.canUndo() {
			t.Fatal("expected RichEdit EM_CANUNDO to be true after edit")
		}
		w.editKey('Z', true, false, false)
		if got := in.re.getText(); got != "hello" {
			t.Fatalf("after undo text = %q, want %q", got, "hello")
		}
		w.editKey('Y', true, false, false)
		if got := in.re.getText(); got != "hio" {
			t.Fatalf("after redo text = %q, want %q", got, "hio")
		}
	})

	// 2. Password input: verify TXTBIT_USEPASSWORD, bullet password char,    spell checking disabled, and copy blocked.
	onMain(t, hwnd, func(w *window) {
		in := findField(w, "secret")
		if in == nil {
			t.Fatal("missing secret field")
		}
		w.setFocus(in)
		re := in.ensureRichEdit()
		if re == nil {
			t.Fatal("secret field missing RichEdit host")
		}
		if ch, ok := re.sendMessage(emGetPasswordChar, 0, 0); !ok || uint16(ch) != richEditBullet {
			t.Errorf("secret EM_GETPASSWORDCHAR = %#x (ok=%v), want %#x", ch, ok, richEditBullet)
		}
		if opts, ok := re.sendMessage(emGetLangOptions, 0, 0); !ok || uint32(opts)&imfSpellChecking != 0 {
			t.Errorf("secret EM_GETLANGOPTIONS = %#x, must not enable spell checking", opts)
		}
		in.insert("s3cr3t")
		in.selectAll()
		w.setClipboard("keep-me")
		w.copySelection(in)
		if got := w.clipboard(); got != "keep-me" {
			t.Errorf("password copy overwrote clipboard with %q", got)
		}
	})

	// 3. Multiline input: NoAutoCorrect disables spell checking, scrolling works,    and TxDrawD2D renders cleanly during paint.
	onMain(t, hwnd, func(w *window) {
		in := findField(w, "multi")
		if in == nil {
			t.Fatal("missing multi field")
		}
		re := in.ensureRichEdit()
		if re == nil {
			t.Fatal("multi field missing RichEdit host")
		}
		if opts, ok := re.sendMessage(emGetLangOptions, 0, 0); !ok || uint32(opts)&imfSpellChecking != 0 {
			t.Errorf("NoAutoCorrect EM_GETLANGOPTIONS = %#x, must not enable spell checking", opts)
		}
		// Ensure a full Direct2D frame paints every RichEdit field via TxDrawD2D.
		w.redrawAll()
		w.paint()
		if !in.wheelScroll(-2) || in.scroll <= 0 {
			t.Errorf("multiline RichEdit wheelScroll(-2) did not advance scroll offset (scroll=%v, contentHeight=%v)",
				in.scroll, in.contentHeight)
		}
	})
}
