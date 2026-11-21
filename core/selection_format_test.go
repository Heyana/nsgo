package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestSelectionFormatUTF16SlicingAndPartialSpans(t *testing.T) {
	// "Hi 👋 bold and italic!" where 👋 is 2 UTF-16 code units. Offsets:
	// "Hi " -> [0, 3) "👋"  -> [3, 5) " "   -> [5, 6) "bold " (Strong) -> [6, 11) "and italic" (Strong > Emphasis) -> [11, 21) "!" -> [21, 22)
	inlines := []SelectionInline{
		{Kind: SelectionInlineText, Text: "Hi 👋 "},
		{Kind: SelectionInlineStrong, Children: []SelectionInline{
			{Kind: SelectionInlineText, Text: "bold "},
			{Kind: SelectionInlineEmphasis, Children: []SelectionInline{
				{Kind: SelectionInlineText, Text: "and italic"},
			}},
		}},
		{Kind: SelectionInlineText, Text: "!"},
	}

	cases := []struct {
		name     string
		start    int
		end      int
		partial  bool
		wantMD   string
		wantHTML string
	}{
		{
			name:     "across emoji into bold",
			start:    3,
			end:      10,
			partial:  true,
			wantMD:   "👋 **bold**",
			wantHTML: "👋 <strong>bold</strong>",
		},
		{
			name:     "partial bold with trailing space moves space outside markdown delimiter",
			start:    8,
			end:      11,
			partial:  true,
			wantMD:   "**ld** ",
			wantHTML: "<strong>ld </strong>",
		},
		{
			name:     "partial bold into nested italic",
			start:    8,
			end:      17,
			partial:  true,
			wantMD:   "**ld *and it***",
			wantHTML: "<strong>ld <em>and it</em></strong>",
		},
		{
			name:     "only nested italic inside bold",
			start:    15,
			end:      21,
			partial:  true,
			wantMD:   "***italic***",
			wantHTML: "<strong><em>italic</em></strong>",
		},
		{
			name:     "whole paragraph wraps html in p",
			start:    0,
			end:      22,
			partial:  false,
			wantMD:   "Hi 👋 **bold *and italic***!",
			wantHTML: "<p>Hi 👋 <strong>bold <em>and italic</em></strong>!</p>",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sliced := sliceSelectionInlines(inlines, tc.start, tc.end)
			md, htmlOut := serializeSelection([]*selNode{{
				block:   SelectionBlock{Kind: SelectionBlockParagraph},
				inlines: sliced,
				partial: tc.partial,
			}})
			if md != tc.wantMD {
				t.Errorf("Markdown = %q, want %q", md, tc.wantMD)
			}
			if htmlOut != tc.wantHTML {
				t.Errorf("HTML = %q, want %q", htmlOut, tc.wantHTML)
			}
		})
	}
}

func TestSelectionFormatCodeSpansAndBlocks(t *testing.T) {
	md, htmlOut := FormatSelection([]SelectionBlock{
		{
			Kind: SelectionBlockParagraph,
			Inlines: []SelectionInline{
				{Kind: SelectionInlineText, Text: "Use "},
				{Kind: SelectionInlineCode, Text: "a`b"},
				{Kind: SelectionInlineText, Text: " and "},
				{Kind: SelectionInlineCode, Text: "`x`"},
			},
		},
		{
			Kind:     SelectionBlockCode,
			Language: "markdown",
			Inlines:  []SelectionInline{{Kind: SelectionInlineText, Text: "```go\nx := 1\n```"}},
		},
	})
	wantMD := "Use ``a`b`` and `` `x` ``\n\n````markdown\n```go\nx := 1\n```\n````"
	wantHTML := "<p>Use <code>a`b</code> and <code>`x`</code></p>\n<pre><code class=\"language-markdown\">```go\nx := 1\n```</code></pre>"
	if md != wantMD {
		t.Errorf("Markdown =\n%s\nwant:\n%s", md, wantMD)
	}
	if htmlOut != wantHTML {
		t.Errorf("HTML =\n%s\nwant:\n%s", htmlOut, wantHTML)
	}
}

func TestSelectionFormatPlainStyledTextsAndFallback(t *testing.T) {
	var got []TextSelection
	h := newHost(t, Size{Width: 400, Height: 200}, func(*Scope) Node {
		return View{
			TestID:          "area",
			Selectable:      true,
			OnTextSelection: func(s TextSelection) { got = append(got, s) },
			Children: []Node{
				Text{
					TestID: "styled",
					Spans: []Span{
						{Content: "a < b "},
						{Content: "bold", Style: Style{FontWeight: FontWeightBold}},
						{Content: " secret", Style: Style{Visibility: VisibilityHidden}},
						{Content: " "},
						{Content: "code", Style: Style{FontFamily: FontMonospace}},
					},
				},
			},
		}
	})
	h.commit()
	area := h.find("area")
	txt := h.find("styled")

	// Partial selection across escaped text, bold, hidden span (skipped), and monospace.
	h.tree.Dispatch(driver.Event{
		Kind:   driver.EventSelectionChange,
		ID:     area.id,
		Point:  Point{X: 0, Y: 0},
		Size:   Size{Width: 100, Height: 20},
		Text:   "a < b bold code",
		Ranges: []driver.SelectedRange{{ID: txt.id, Start: 0, End: selectionUTF16Len("a < b bold secret code")}},
	})
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if got[0].Markdown != "a < b **bold** `code`" {
		t.Errorf("Markdown = %q, want %q", got[0].Markdown, "a < b **bold** `code`")
	}
	if got[0].HTML != "a &lt; b <strong>bold</strong> <code>code</code>" {
		t.Errorf("HTML = %q, want %q", got[0].HTML, "a &lt; b <strong>bold</strong> <code>code</code>")
	}

	// Fallback when platform reports no Ranges (e.g. Android).
	h.tree.Dispatch(driver.Event{
		Kind:  driver.EventSelectionChange,
		ID:    area.id,
		Point: Point{X: 0, Y: 0},
		Size:  Size{Width: 100, Height: 40},
		Text:  "Line 1 <tag>\nLine 2 & more",
	})
	last := got[len(got)-1]
	if last.Markdown != "Line 1 <tag>\nLine 2 & more" {
		t.Errorf("fallback Markdown = %q", last.Markdown)
	}
	if last.HTML != "<p>Line 1 &lt;tag&gt;</p>\n<p>Line 2 &amp; more</p>" {
		t.Errorf("fallback HTML = %q", last.HTML)
	}

	// Clearing selection clears Text, Markdown, and HTML.
	h.tree.Dispatch(driver.Event{Kind: driver.EventSelectionChange, ID: area.id})
	cleared := got[len(got)-1]
	if cleared.Text != "" || cleared.Markdown != "" || cleared.HTML != "" || cleared.Ranges != nil {
		t.Errorf("cleared selection = %+v, want empty", cleared)
	}
}
