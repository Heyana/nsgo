package linux

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func encodeUTF16WithBOM(s string, order binary.ByteOrder, trailingNUL bool) []byte {
	units := utf16.Encode([]rune(s))
	if trailingNUL {
		units = append(units, 0)
	}
	out := make([]byte, 2+len(units)*2)
	if order == binary.LittleEndian {
		out[0], out[1] = 0xFF, 0xFE
	} else {
		out[0], out[1] = 0xFE, 0xFF
	}
	for i, u := range units {
		order.PutUint16(out[2+i*2:], u)
	}
	return out
}

func TestDecodeClipboardHTML(t *testing.T) {
	t.Run("UTF-8 plain and multibyte", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			in   []byte
			want string
		}{
			{"empty", nil, ""},
			{"plain", []byte("<b>hello</b>"), "<b>hello</b>"},
			{"multibyte", []byte("<p>こんにちは 🎉 café</p>"), "<p>こんにちは 🎉 café</p>"},
			{"multiline", []byte("<ul>\n\t<li>one</li>\n\t<li>two</li>\n</ul>"), "<ul>\n\t<li>one</li>\n\t<li>two</li>\n</ul>"},
			{"trailing NULs", []byte("<em>hi</em>\x00\x00"), "<em>hi</em>"},
			{"UTF-8 BOM", append([]byte{0xEF, 0xBB, 0xBF}, "<b>bom</b>\x00"...), "<b>bom</b>"},
			{"UTF-8 BOM only", []byte{0xEF, 0xBB, 0xBF}, ""},
			{"NUL only", []byte{0, 0}, ""},
		} {
			if got := decodeClipboardHTML(tc.in); got != tc.want {
				t.Errorf("%s: decodeClipboardHTML = %q, want %q", tc.name, got, tc.want)
			}
		}
	})

	t.Run("UTF-16LE with BOM", func(t *testing.T) {
		want := "<p>Firefox 日本語 🎉</p>"
		in := encodeUTF16WithBOM(want, binary.LittleEndian, true)
		if got := decodeClipboardHTML(in); got != want {
			t.Fatalf("UTF-16LE = %q, want %q", got, want)
		}
		// Odd trailing byte is ignored cleanly.
		inOdd := append(encodeUTF16WithBOM("<b>hi</b>", binary.LittleEndian, false), 0x42)
		if got := decodeClipboardHTML(inOdd); got != "<b>hi</b>" {
			t.Fatalf("UTF-16LE odd = %q", got)
		}
		// BOM + NUL only.
		if got := decodeClipboardHTML([]byte{0xFF, 0xFE, 0, 0}); got != "" {
			t.Fatalf("UTF-16LE BOM+NUL = %q, want empty", got)
		}
		// BOM only.
		if got := decodeClipboardHTML([]byte{0xFF, 0xFE}); got != "" {
			t.Fatalf("UTF-16LE BOM only = %q, want empty", got)
		}
	})

	t.Run("UTF-16BE with BOM", func(t *testing.T) {
		want := "<p>Big Endian 🚀</p>"
		in := encodeUTF16WithBOM(want, binary.BigEndian, true)
		if got := decodeClipboardHTML(in); got != want {
			t.Fatalf("UTF-16BE = %q, want %q", got, want)
		}
		if got := decodeClipboardHTML([]byte{0xFE, 0xFF}); got != "" {
			t.Fatalf("UTF-16BE BOM only = %q, want empty", got)
		}
	})
}

func TestHasPasteHTMLFormat(t *testing.T) {
	if !hasPasteHTMLFormat(func(m string) bool { return m == "text/html" }) {
		t.Error("expected text/html to match")
	}
	if !hasPasteHTMLFormat(func(m string) bool { return m == "text/html;charset=utf-8" }) {
		t.Error("expected text/html;charset=utf-8 to match")
	}
	if hasPasteHTMLFormat(func(m string) bool { return m == "text/plain" }) {
		t.Error("text/plain must not match HTML format")
	}
}

func TestAppendPasteHTMLCap(t *testing.T) {
	var buf []byte
	var capped bool
	buf, capped = appendPasteHTML(buf, []byte("<b>"))
	if capped || string(buf) != "<b>" {
		t.Fatalf("first chunk: %q, capped=%v", buf, capped)
	}
	buf, capped = appendPasteHTML(buf, []byte("hi</b>"))
	if capped || string(buf) != "<b>hi</b>" {
		t.Fatalf("second chunk: %q, capped=%v", buf, capped)
	}

	// Fill up to near the cap and verify truncation without exceeding pasteHTMLMax.
	nearCap := make([]byte, pasteHTMLMax-4)
	out, capped := appendPasteHTML(nearCap, []byte("12345678"))
	if !capped {
		t.Fatal("expected cap to trigger")
	}
	if len(out) != pasteHTMLMax {
		t.Fatalf("len = %d, want %d", len(out), pasteHTMLMax)
	}
	if !bytes.Equal(out[pasteHTMLMax-4:], []byte("1234")) {
		t.Fatalf("tail = %q, want 1234", out[pasteHTMLMax-4:])
	}
}
