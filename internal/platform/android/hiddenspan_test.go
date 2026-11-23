package android

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestHiddenSpansSkippedJava(t *testing.T) {
	black := driver.RGB(0x000000)
	red := driver.RGB(0xff0000)
	base := driver.TextStyle{Font: driver.Font{Size: 16, Weight: 400}, Color: black}
	deco := base
	deco.Decoration = driver.TextDecorationUnderline | driver.TextDecorationLineThrough
	deco.DecorationColor = red
	shadow := driver.TextShadow{Color: black, Offset: driver.Point{X: 2, Y: 2}, Radius: 2}

	type testCase struct {
		name      string
		width     float32
		at        driver.AttributedText
		hasHidden bool
		wantOps   []string
	}
	cases := []testCase{
		{
			name:  "mixed visible and hidden spans with decorations background and shadow",
			width: 200,
			at: driver.AttributedText{
				Text:   "AAA BBB CCC",
				Style:  base,
				Shadow: shadow,
				Spans: []driver.TextSpan{
					{Start: 0, End: 4, Style: deco, Background: driver.TextBackground{Color: red}},
					{Start: 4, End: 8, Style: deco, Hidden: true, Background: driver.TextBackground{Color: red}},
					{Start: 8, End: 11, Style: base},
				},
			},
			hasHidden: true,
			wantOps: []string{
				"bg:0..40",
				"glyphs:AAA @0,15+shadow",
				"glyphs:CCC@80,15+shadow",
				"deco-rect:0..40",
				"deco-rect:0..40",
			},
		},
		{
			name:  "all hidden with emoji decorations background and shadow draws nothing",
			width: 200,
			at: driver.AttributedText{
				Text:   "😀 Hide",
				Style:  base,
				Shadow: shadow,
				Spans: []driver.TextSpan{{
					Start: 0, End: len("😀 Hide"), Style: deco, Hidden: true,
					Background: driver.TextBackground{Color: red},
				}},
			},
			hasHidden: true,
			wantOps:   nil,
		},
		{
			name:  "multi-line with one hidden line and one mixed line",
			width: 200,
			at: driver.AttributedText{
				Text:  "hidden\nShow Hide",
				Style: base,
				Spans: []driver.TextSpan{
					{Start: 0, End: 7, Style: base, Hidden: true},
					{Start: 7, End: 12, Style: base},
					{Start: 12, End: 16, Style: base, Hidden: true},
				},
			},
			hasHidden: true,
			wantOps: []string{
				"glyphs:Show @0,35",
			},
		},
	}

	var input writer
	input.u32(uint32(len(cases)))
	for _, tc := range cases {
		input.float(tc.width)
		writeTextPrepare(&input, &tc.at, 0)
	}

	out := runJavaAlgorithm(t, "hiddenspan", "HiddenSpanCheck", input.buf, "Text", "TextNode", "Protocol", "FontFallback", "RasterScale", "TextCopy")
	r := reader{out}
	for _, tc := range cases {
		w, h := r.float(), r.float()
		hasHidden := r.byte() != 0
		n := int(r.u32())
		var ops []string
		for range n {
			ops = append(ops, r.str())
		}
		if w <= 0 || h <= 0 {
			t.Errorf("%s: measured size = %gx%g, want positive", tc.name, w, h)
		}
		if hasHidden != tc.hasHidden {
			t.Errorf("%s: hasHidden = %v, want %v", tc.name, hasHidden, tc.hasHidden)
		}
		if !slices.Equal(ops, tc.wantOps) {
			t.Errorf("%s:\n got ops = %v\nwant ops = %v", tc.name, ops, tc.wantOps)
		}
	}
	if len(r.buf) != 0 {
		t.Errorf("Java wrote %d unexpected bytes", len(r.buf))
	}
}

// TestTextPrepareAppendsHiddenFlags checks that TEXT_PREPARE keeps the layout a runtime that predates hidden spans reads, with one hidden byte per span appended after everything it reads.
func TestTextPrepareAppendsHiddenFlags(t *testing.T) {
	base := driver.TextStyle{Font: driver.Font{Size: 16, Weight: 400}}
	shown := driver.AttributedText{Text: "one two six", Style: base, Spans: []driver.TextSpan{
		{Start: 0, End: 4, Style: base},
		{Start: 4, End: 8, Style: base},
		{Start: 8, End: 11, Style: base},
	}}
	hidden := shown
	hidden.Spans = slices.Clone(shown.Spans)
	hidden.Spans[1].Hidden = true
	var old, now writer
	writeTextPrepare(&old, &shown, 7)
	writeTextPrepare(&now, &hidden, 7)
	n := len(shown.Spans)
	const surface = 4 // the surface ID follows the hidden flags
	if len(now.buf) != len(old.buf) || !slices.Equal(now.buf[:len(now.buf)-n-surface], old.buf[:len(old.buf)-n-surface]) {
		t.Fatalf("a hidden span changed the payload before its trailing flags")
	}
	if got, want := now.buf[len(now.buf)-n-surface:len(now.buf)-surface], []byte{0, 1, 0}; !slices.Equal(got, want) {
		t.Errorf("trailing hidden flags = %v, want %v", got, want)
	}
	if got, want := now.buf[len(now.buf)-surface:], []byte{0, 0, 0, 7}; !slices.Equal(got, want) {
		t.Errorf("trailing surface ID = %v, want %v", got, want)
	}
}
