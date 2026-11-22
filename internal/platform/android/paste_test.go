package android

import (
	"bytes"
	"encoding/base64"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestReadPasteHTML(t *testing.T) {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

	t.Run("text and HTML", func(t *testing.T) {
		html := "<p>hello <b>world</b>\n\t日本語 🎉</p>"
		got := readPaste("hello world", "html\t"+b64(html)+"\n")
		if got.Text != "hello world" || got.HTML != html || len(got.URIs) != 0 || len(got.Files) != 0 {
			t.Fatalf("readPaste = %+v", got)
		}
	})

	t.Run("HTML with URI and file", func(t *testing.T) {
		html := "<b>bold</b>"
		png := []byte{0x89, 'P', 'N', 'G'}
		data := "uri\tfile:///tmp/a.txt\n" +
			"file\timage/png\t" + base64.StdEncoding.EncodeToString(png) + "\n" +
			"html\t" + b64(html) + "\n"
		got := readPaste("bold", data)
		if got.Text != "bold" || got.HTML != html {
			t.Fatalf("text/html = %q/%q", got.Text, got.HTML)
		}
		if !slices.Equal(got.URIs, []string{"file:///tmp/a.txt"}) {
			t.Fatalf("URIs = %v", got.URIs)
		}
		if len(got.Files) != 1 || got.Files[0].MIME != "image/png" || !bytes.Equal(got.Files[0].Data, png) {
			t.Fatalf("Files = %+v", got.Files)
		}
	})

	t.Run("multiple HTML lines concatenate", func(t *testing.T) {
		data := "html\t" + b64("<p>one</p>") + "\nhtml\t" + b64("<p>two</p>") + "\n"
		got := readPaste("onetwo", data)
		if got.HTML != "<p>one</p><p>two</p>" {
			t.Fatalf("HTML = %q", got.HTML)
		}
	})

	t.Run("invalid base64 HTML ignored", func(t *testing.T) {
		got := readPaste("plain", "html\t!!!not-base64!!!\n")
		if got.Text != "plain" || got.HTML != "" {
			t.Fatalf("readPaste = %+v", got)
		}
	})

	t.Run("empty data", func(t *testing.T) {
		got := readPaste("only text", "")
		if got.Text != "only text" || got.HTML != "" || len(got.URIs) != 0 || len(got.Files) != 0 {
			t.Fatalf("readPaste = %+v", got)
		}
	})
}

// TestPasteJavaRoundTrip compiles Protocol.java and PasteCheck.java on the JVM, encodes paste payloads containing HTML (with newlines, tabs and Unicode), URIs and files, and decodes the wire output through Go's readPaste.
func TestPasteJavaRoundTrip(t *testing.T) {
	javac, java := androidJDK(t)
	dir := t.TempDir()
	args := []string{
		"-d", dir,
		filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"),
		filepath.Join("testdata", "PasteCheck.java"),
	}
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}

	run := func(opArgs ...string) string {
		t.Helper()
		cmdArgs := append([]string{"-cp", dir, "dev.oneo.nsgo.PasteCheck"}, opArgs...)
		out, err := exec.CommandContext(t.Context(), java, cmdArgs...).Output()
		if err != nil {
			t.Fatalf("PasteCheck %v: %v", opArgs, err)
		}
		return string(out)
	}

	multilineHTML := "<div>\n\t<b>Hello</b> <em>日本語</em> 🎉\r\n</div>"
	data := run(
		"uri", "file:///storage/emulated/0/doc.pdf",
		"file", "image/png", "PNGDATA",
		"null-html",
		"empty-html",
		"html", multilineHTML,
	)
	got := readPaste("Hello 日本語 🎉", data)
	want := driver.PasteEvent{
		HTML:  multilineHTML,
		Text:  "Hello 日本語 🎉",
		URIs:  []string{"file:///storage/emulated/0/doc.pdf"},
		Files: []driver.PastedFile{{MIME: "image/png", Data: []byte("PNGDATA")}},
	}
	if got.HTML != want.HTML || got.Text != want.Text || !slices.Equal(got.URIs, want.URIs) ||
		len(got.Files) != 1 || got.Files[0].MIME != want.Files[0].MIME || !bytes.Equal(got.Files[0].Data, want.Files[0].Data) {
		t.Fatalf("round-trip = %+v, want %+v", got, want)
	}

	// Empty/null HTML produces no data lines.
	if empty := run("null-html", "empty-html"); empty != "" {
		t.Fatalf("null/empty HTML produced %q, want empty", empty)
	}
}
