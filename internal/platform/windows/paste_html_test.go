package windows

import (
	"fmt"
	"strings"
	"testing"
)

func buildCFHTML(sourceURL, prefix, fragment, suffix string) []byte {
	const headerFmt = "Version:0.9\r\n" +
		"StartHTML:%010d\r\n" +
		"EndHTML:%010d\r\n" +
		"StartFragment:%010d\r\n" +
		"EndFragment:%010d\r\n"
	extra := ""
	if sourceURL != "" {
		extra = "SourceURL:" + sourceURL + "\r\n"
	}
	headerLen := len(fmt.Sprintf(headerFmt, 0, 0, 0, 0)) + len(extra)
	startHTML := headerLen
	startFrag := startHTML + len(prefix)
	endFrag := startFrag + len(fragment)
	endHTML := endFrag + len(suffix)
	return []byte(fmt.Sprintf(headerFmt, startHTML, endHTML, startFrag, endFrag) + extra + prefix + fragment + suffix)
}

func TestParseCFHTML(t *testing.T) {
	t.Run("standard fragment", func(t *testing.T) {
		raw := buildCFHTML(
			"https://example.com/page",
			"<html><body>\r\n<!--StartFragment-->",
			"<b>hello</b> <i>world</i>",
			"<!--EndFragment-->\r\n</body></html>",
		)
		if got := parseCFHTML(raw); got != "<b>hello</b> <i>world</i>" {
			t.Fatalf("parseCFHTML = %q", got)
		}
	})

	t.Run("UTF-8 multibyte offsets", func(t *testing.T) {
		raw := buildCFHTML(
			"https://example.com/日本語/🚀",
			"<html><head><title>見出し</title></head><body><!--StartFragment-->",
			"<p>こんにちは 🎉 café</p>",
			"<!--EndFragment--></body></html>",
		)
		if got := parseCFHTML(raw); got != "<p>こんにちは 🎉 café</p>" {
			t.Fatalf("parseCFHTML = %q", got)
		}
	})

	t.Run("trailing NUL padding from GlobalSize", func(t *testing.T) {
		raw := buildCFHTML("", "<html><body><!--StartFragment-->", "<em>hi</em>", "<!--EndFragment--></body></html>")
		raw = append(raw, 0, 0, 0, 'x', 'y', 'z')
		if got := parseCFHTML(raw); got != "<em>hi</em>" {
			t.Fatalf("parseCFHTML = %q", got)
		}
	})

	t.Run("no HTML context StartHTML -1", func(t *testing.T) {
		frag := "<span>only fragment</span>"
		hdrFmt := "Version:0.9\r\nStartHTML:-1\r\nEndHTML:-1\r\nStartFragment:%010d\r\nEndFragment:%010d\r\n"
		hdrLen := len(fmt.Sprintf(hdrFmt, 0, 0))
		raw := []byte(fmt.Sprintf(hdrFmt, hdrLen, hdrLen+len(frag)) + frag)
		if got := parseCFHTML(raw); got != frag {
			t.Fatalf("parseCFHTML = %q, want %q", got, frag)
		}
	})

	t.Run("explicit empty fragment", func(t *testing.T) {
		raw := buildCFHTML("", "<html><body><!--StartFragment-->", "", "<!--EndFragment--></body></html>")
		if got := parseCFHTML(raw); got != "" {
			t.Fatalf("parseCFHTML = %q, want empty", got)
		}
	})

	t.Run("EndFragment slightly past end clamped", func(t *testing.T) {
		frag := "<b>ok</b>"
		hdrFmt := "Version:0.9\r\nStartFragment:%010d\r\nEndFragment:%010d\r\n"
		hdrLen := len(fmt.Sprintf(hdrFmt, 0, 0))
		raw := []byte(fmt.Sprintf(hdrFmt, hdrLen, hdrLen+len(frag)+2) + frag)
		if got := parseCFHTML(raw); got != frag {
			t.Fatalf("parseCFHTML = %q, want %q", got, frag)
		}
	})

	t.Run("LF-only headers and mixed case keys", func(t *testing.T) {
		body := "<html><body><!--StartFragment--><b>lf</b><!--EndFragment--></body></html>"
		prefix := "<html><body><!--StartFragment-->"
		frag := "<b>lf</b>"
		hdrFmt := "version:0.9\nstarthtml:%010d\nendhtml:%010d\nstartfragment:%010d\nendfragment:%010d\n"
		hdrLen := len(fmt.Sprintf(hdrFmt, 0, 0, 0, 0))
		raw := []byte(fmt.Sprintf(hdrFmt, hdrLen, hdrLen+len(body), hdrLen+len(prefix), hdrLen+len(prefix)+len(frag)) + body)
		if got := parseCFHTML(raw); got != frag {
			t.Fatalf("parseCFHTML = %q, want %q", got, frag)
		}
	})

	t.Run("missing fragment offsets fall back to markers", func(t *testing.T) {
		body := "<html><body><!--StartFragment--><u>marked</u><!--EndFragment--></body></html>"
		hdrFmt := "Version:0.9\r\nStartHTML:%010d\r\nEndHTML:%010d\r\n"
		hdrLen := len(fmt.Sprintf(hdrFmt, 0, 0))
		raw := []byte(fmt.Sprintf(hdrFmt, hdrLen, hdrLen+len(body)) + body)
		if got := parseCFHTML(raw); got != "<u>marked</u>" {
			t.Fatalf("parseCFHTML = %q", got)
		}
	})

	t.Run("mid-UTF-8 fragment offset falls back safely to markers", func(t *testing.T) {
		// Place StartFragment in the middle of the 3-byte UTF-8 rune 'あ'.
		prefix := "<html><body>あ<!--StartFragment-->"
		frag := "<b>safe</b>"
		suffix := "<!--EndFragment--></body></html>"
		body := prefix + frag + suffix
		hdrFmt := "Version:0.9\r\nStartHTML:%010d\r\nEndHTML:%010d\r\nStartFragment:%010d\r\nEndFragment:%010d\r\n"
		hdrLen := len(fmt.Sprintf(hdrFmt, 0, 0, 0, 0))
		midRune := hdrLen + len("<html><body>") + 1 // continuation byte of 'あ'
		raw := []byte(fmt.Sprintf(hdrFmt, hdrLen, hdrLen+len(body), midRune, hdrLen+len(prefix)+len(frag)) + body)
		if got := parseCFHTML(raw); got != frag {
			t.Fatalf("parseCFHTML = %q, want %q", got, frag)
		}
	})

	t.Run("missing fragment offsets and markers fall back to StartHTML..EndHTML", func(t *testing.T) {
		body := "<html><body><p>whole doc</p></body></html>"
		hdrFmt := "Version:0.9\r\nStartHTML:%010d\r\nEndHTML:%010d\r\n"
		hdrLen := len(fmt.Sprintf(hdrFmt, 0, 0))
		raw := []byte(fmt.Sprintf(hdrFmt, hdrLen, hdrLen+len(body)) + body)
		if got := parseCFHTML(raw); got != body {
			t.Fatalf("parseCFHTML = %q, want %q", got, body)
		}
	})

	t.Run("malformed header falls back to body without leaking header", func(t *testing.T) {
		raw := []byte("Version:0.9\r\nStartHTML:bad\r\nEndHTML:nope\r\n<p>recovered</p>")
		if got := parseCFHTML(raw); got != "<p>recovered</p>" {
			t.Fatalf("parseCFHTML = %q", got)
		}
	})

	t.Run("header only with no body returns empty", func(t *testing.T) {
		raw := []byte("Version:0.9\r\nStartHTML:9999\r\nEndHTML:9999\r\n")
		if got := parseCFHTML(raw); got != "" {
			t.Fatalf("parseCFHTML = %q, want empty", got)
		}
	})

	t.Run("raw HTML without CF_HTML header", func(t *testing.T) {
		for _, tc := range []struct {
			in, want string
		}{
			{"<p>raw: html</p>", "<p>raw: html</p>"},
			{"\uFEFF<p>with bom</p>", "<p>with bom</p>"},
			{"", ""},
			{"\x00\x00", ""},
		} {
			if got := parseCFHTML([]byte(tc.in)); got != tc.want {
				t.Errorf("parseCFHTML(%q) = %q, want %q", tc.in, got, tc.want)
			}
		}
	})

	t.Run("UTF-8 BOM before Version header and inside fragment", func(t *testing.T) {
		raw := buildCFHTML("", "<html><body><!--StartFragment-->", "\uFEFF<b>bom</b>", "<!--EndFragment--></body></html>")
		if got := parseCFHTML(raw); got != "<b>bom</b>" {
			t.Fatalf("fragment BOM: parseCFHTML = %q", got)
		}
		// BOM before header shifts offsets by 3 if built from offset 0: test malformed fallback with leading BOM.
		malformed := []byte("\uFEFFVersion:0.9\r\nStartHTML:bad\r\n<div>after bom</div>")
		if got := parseCFHTML(malformed); got != "<div>after bom</div>" {
			t.Fatalf("header BOM: parseCFHTML = %q", got)
		}
	})

	t.Run("StartFragment past EndFragment ignored", func(t *testing.T) {
		body := "<html><body><!--StartFragment--><b>fixed</b><!--EndFragment--></body></html>"
		hdrFmt := "Version:0.9\r\nStartHTML:%010d\r\nEndHTML:%010d\r\nStartFragment:0000000500\r\nEndFragment:0000000100\r\n"
		hdrLen := len(fmt.Sprintf(hdrFmt, 0, 0))
		raw := []byte(fmt.Sprintf(hdrFmt, hdrLen, hdrLen+len(body)) + body)
		if got := parseCFHTML(raw); got != "<b>fixed</b>" {
			t.Fatalf("parseCFHTML = %q", got)
		}
	})

	t.Run("multiline HTML fragment preserved", func(t *testing.T) {
		frag := strings.Join([]string{"<ul>", "\t<li>one</li>", "\t<li>two</li>", "</ul>"}, "\r\n")
		raw := buildCFHTML("", "<html><body><!--StartFragment-->", frag, "<!--EndFragment--></body></html>")
		if got := parseCFHTML(raw); got != frag {
			t.Fatalf("parseCFHTML = %q, want %q", got, frag)
		}
	})
}
