package ui

import (
	"testing"

	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestTextView draws Markdown in the theme, under the app's own styles.
func TestTextView(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		app, p := start(t, e, func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 12}, Children: []core.Node{
				TextView{TestID: "doc", Markdown: "# Title\n\nSome `code` and a [link](https://example.com).\n\n```go\nfmt.Println()\n```\n"},
				TextView{TestID: "custom", Markdown: "# Custom", Styles: textview.Styles{textview.PartHeading1: {Color: core.RGB(0x3366cc)}}},
			}}
		})
		title := app.Find("doc >> Title")
		if title.TextStyle.Font.Size != TypeHeading.FontSize+4 || title.TextStyle.Color != p.Text {
			t.Fatalf("heading = %v %x, want the heading's size and the text color", title.TextStyle.Font.Size, title.TextStyle.Color)
		}
		if got := app.Find("doc >> fmt.Println()").TextStyle.Font.Family; got != core.FontMonospace {
			t.Fatalf("code = %q, want monospace", got)
		}
		if got := app.Find("custom >> Custom").TextStyle.Color; got != core.RGB(0x3366cc) {
			t.Fatalf("the app's heading = %x, want its own color", got)
		}
	})
}

// A typeset display formula is as large as the text around it, in a scroll that fills its block and pads it, while its TeX alone shows in the code type.
func TestTextViewMathBlock(t *testing.T) {
	app, _ := start(t, core.Environment{}, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Gap: 12}, Children: []core.Node{
			TextView{TestID: "typeset", Markdown: "$$\nx\n$$\n", RenderMath: func(cx *core.Scope, m textview.Math) core.Node {
				return core.Canvas{TestID: "formula", Em: core.EmBox{Width: 1, Ascent: 1}, Style: core.Style{AlignSelf: core.AlignStart}}
			}},
			TextView{TestID: "source", Markdown: "$$\nx\n$$\n"},
		}}
	})
	if got := app.Find("id:formula").Frame.Size.Height; got != TypeBody.FontSize {
		t.Errorf("a display formula is %v high, want the body's %v", got, TypeBody.FontSize)
	}
	if got := app.Find("source >> x").TextStyle.Font.Size; got != TypeCode.FontSize {
		t.Errorf("a formula's TeX is %v, want the code's %v", got, TypeCode.FontSize)
	}
	text := app.Find("id:formula")
	for text.Kind != nsgotest.KindText {
		text = text.Parent
	}
	scroll, block := text.Parent, text.Parent.Parent
	if scroll.Kind != nsgotest.KindScroll || scroll.Frame.Size.Width != block.Frame.Size.Width || block.Style.Background == 0 {
		t.Errorf("a %v %v wide in a block %v wide with background %x: the scroll fills the block that has the background", scroll.Kind, scroll.Frame.Size.Width, block.Frame.Size.Width, block.Style.Background)
	}
	if got := text.Frame.Origin; got.X != 12 || got.Y != 12 {
		t.Errorf("the formula's text is at %v in its scroll, want PartMathBlockScroll's padding", got)
	}
}

func TestTextViewSelectionFormats(t *testing.T) {
	var sel core.TextSelection
	app, _ := start(t, core.Environment{}, func(cx *core.Scope) core.Node {
		return TextView{
			TestID:          "doc",
			Markdown:        "## Hello\n\nRead [the **guide**](https://nsgo.dev).",
			Selectable:      true,
			OnTextSelection: func(s core.TextSelection) { sel = s },
		}
	})
	app.SelectTextRange("id:doc", "text:Hello", 0, "text:/Read the guide/", 14)
	if sel.Markdown != "## Hello\n\nRead [the **guide**](https://nsgo.dev)" {
		t.Errorf("Markdown = %q", sel.Markdown)
	}
	if sel.HTML != "<h2>Hello</h2>\n<p>Read <a href=\"https://nsgo.dev\">the <strong>guide</strong></a></p>" {
		t.Errorf("HTML = %q", sel.HTML)
	}
}
