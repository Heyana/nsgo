//go:build windows

package windows

import (
	"bytes"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestExpandWindowsClipboardReps(t *testing.T) {
	const (
		plain  = "$E=mc^2$"
		mathML = `<math xmlns="http://www.w3.org/1998/Math/MathML"><mi>E</mi></math>`
		tex    = "E=mc^2"
	)
	items := []driver.ClipboardItem{{Representations: []driver.ClipboardRepresentation{
		{Type: "text/plain", Data: []byte(plain)},
		{Type: "application/mathml+xml", Data: []byte(mathML)},
		{Type: "text/x-tex", Data: []byte(tex)},
	}}}
	got := expandWindowsClipboardReps(items)
	byName := map[string][]byte{}
	for _, rep := range got {
		if _, dup := byName[rep.Type]; dup {
			t.Fatalf("duplicate format %q", rep.Type)
		}
		byName[rep.Type] = rep.Data
	}
	wantText := encodeWindowsClipboardText(plain)
	if !bytes.Equal(byName["CF_UNICODETEXT"], wantText) {
		t.Errorf("CF_UNICODETEXT = %v, want %v", byName["CF_UNICODETEXT"], wantText)
	}
	for _, name := range []string{"MathML", "MathML Presentation", "application/mathml+xml"} {
		if string(byName[name]) != mathML {
			t.Errorf("%q = %q, want %q", name, string(byName[name]), mathML)
		}
	}
	if string(byName["text/x-tex"]) != tex {
		t.Errorf("text/x-tex = %q, want %q", string(byName["text/x-tex"]), tex)
	}

	// Restoring a snapshot that already contains the expanded native formats must not duplicate them.
	restored := expandWindowsClipboardReps([]driver.ClipboardItem{{Representations: got}})
	if len(restored) != len(got) {
		t.Errorf("snapshot restore expanded %d reps to %d", len(got), len(restored))
	}
}
