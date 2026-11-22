package android

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestSelectionCopiesAttachmentsAndHiddenSpans(t *testing.T) {
	const text = "汉🚀 a\uFFFC b\uFFFC secret end"
	first, second, secret := strings.IndexRune(text, driver.AttachmentRune), strings.LastIndex(text, "\uFFFC"), strings.Index(text, "secret")
	spans := []driver.TextSpan{
		{Start: first, End: first + 3, Copy: "$x_i^2$"},
		{Start: second, End: second + 3, Copy: "$$\nE=mc^2\n$$"},
		{Start: secret, End: secret + len("secret"), Hidden: true},
	}
	tests := []struct {
		text  string
		spans []driver.TextSpan
	}{
		{text, spans},
		{text, nil}, // A later text with no metadata must omit bare placeholders.
		{"\uFFFC", []driver.TextSpan{{Start: 0, End: 3, Copy: "$z$"}}},
		{"ordinary text", nil},
	}
	var wire writer
	wire.u32(uint32(len(tests)))
	type answer struct{ want string }
	var answers []answer
	for _, test := range tests {
		wire.str(test.text)
		writeTextCopies(&wire, &driver.TextContent{Text: test.text, Spans: test.spans})
		// Every valid UTF-16 range, excluding boundaries inside surrogate pairs.
		var boundaries []int
		for offset := range test.text {
			boundaries = append(boundaries, offset)
		}
		boundaries = append(boundaries, len(test.text))
		wire.u32(uint32(len(boundaries) * (len(boundaries) + 1) / 2))
		for i, from := range boundaries {
			for _, to := range boundaries[i:] {
				wire.u32(uint32(driver.UTF16Units(test.text, from)))
				wire.u32(uint32(driver.UTF16Units(test.text, to)))
				answers = append(answers, answer{driver.CopiedText(test.text, test.spans, from, to)})
			}
		}
	}
	out := runJavaAlgorithm(t, "selection", "CopySelectionCheck", wire.buf, "Selection", "Protocol", "TextCopy")
	r := reader{out}
	for i, answer := range answers {
		if copied, report := r.str(), r.str(); copied != answer.want || report != answer.want {
			t.Errorf("range %d: copied=%q reported=%q want=%q", i, copied, report, answer.want)
		}
	}
	if len(r.buf) != 0 {
		t.Fatalf("unexpected %d trailing bytes", len(r.buf))
	}
}

func TestTextCopyFooterCompatibility(t *testing.T) {
	for _, version := range []uint64{0, 42} {
		var w writer
		writeTextCopyFooter(&w, &driver.MountBatch{GeometryVersion: version}, false)
		if version == 0 {
			if len(w.buf) != 0 {
				t.Fatal("pixel-only follow-up gained a footer")
			}
		} else if r := (&reader{w.buf}).u64(); r != version || len(w.buf) != 8 {
			t.Fatal("geometry-only footer changed")
		}
	}
	var w writer
	writeTextCopyFooter(&w, &driver.MountBatch{GeometryVersion: 42, Items: []driver.MountItem{
		{Kind: driver.ItemText, ID: 7, Text: &driver.TextContent{Text: "\uFFFC", Spans: []driver.TextSpan{{Start: 0, End: 3, Copy: "$x$"}}}},
		{Kind: driver.ItemDelete, ID: 8},
		{Kind: driver.ItemText, ID: 9, Text: &driver.TextContent{Text: "plain"}},
	}}, false)
	r := reader{w.buf}
	if r.u64() != 42 || r.u32() != 2 || r.u32() != 7 || r.u32() != 1 || r.u32() != 0 || r.u32() != 1 || r.byte() != 0 || r.str() != "$x$" || r.u32() != 9 || r.u32() != 0 || len(r.buf) != 0 {
		t.Fatal("copy footer does not retain geometry, node IDs or empty replacement")
	}
}
