package textview_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

const doc = "# Title\n\n[Docs](https://nsgo.dev) and **bold**.\n\n- one\n- [x] two\n\n```go\nx := 1\n```\n\n" +
	"| A | B |\n|---|---|\n| 1 | 2 |\n\n> [!TIP]\n> Use it.\n\n:::callout\nInside\n:::\n\nNote[^1].\n\n[^1]: The note."

func TestTextViewBuildsBlocks(t *testing.T) {
	var pressed []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{Markdown: doc, Selectable: true, TestID: "doc",
			Extensions:  []textview.Extension{{Name: "callout", Container: "callout"}},
			OnLinkPress: func(e textview.LinkPressEvent) { pressed = append(pressed, e.URL) },
			Highlighter: func(c textview.CodeBlock) []textview.Highlight {
				return []textview.Highlight{{Start: 0, End: 1, Style: core.Style{Color: core.RGB(0xff0000)}}, {Start: 0, End: 2}}
			},
			RenderCustom: func(cx *core.Scope, node textview.Custom) core.Node {
				return core.Text{Content: "custom " + node.Name, TestID: "custom"}
			},
			CodeBlockActions: []textview.CodeAction{{ID: textview.CopyAction}},
			RenderCodeBlockAction: func(cx *core.Scope, a textview.CodeBlockAction) core.Node {
				return core.Text{ContentFunc: func() string {
					if a.Done() {
						return "copied"
					}
					return a.ID
				}}
			},
			OnCodeBlockAction: func(e textview.CodeActionEvent) { pressed = append(pressed, e.ID+" "+e.Code.Code) },
		}
	})
	if !app.Find("id:doc").Props.SelectionArea {
		t.Error("a selectable text view is no selection area")
	}
	app.Find("role:header Title")
	for _, text := range []string{"Docs and bold.", "•", "one", "two", "x := 1", "A", "2", "Tip", "Use it.", "custom callout", "Note[1].", "1.", "The note."} {
		app.Find("text:" + text)
	}
	if !app.Find("role:checkbox").Props.Checked {
		t.Error("the done task is not checked")
	}
	app.Press("text:Docs and bold.")
	app.Press("label:copy")
	if want := []string{"https://nsgo.dev", "copy x := 1"}; strings.Join(pressed, "|") != strings.Join(want, "|") {
		t.Errorf("pressed %q, want %q", pressed, want)
	}
	if app.Clipboard != "x := 1" || !app.Exists("text:copied") {
		t.Errorf("clipboard %q, copied shown %v", app.Clipboard, app.Exists("text:copied"))
	}
	app.Advance(2 * time.Second)
	if app.Exists("text:copied") {
		t.Error("the copy action stays done")
	}
}

func TestTextViewStreamsWithACaret(t *testing.T) {
	var md *core.Signal[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		md = core.NewSignal(cx, "")
		return textview.TextView{MarkdownFunc: md.Get, Options: textview.Options{Streaming: true},
			StreamingCaret: core.View{TestID: "caret", Style: core.Style{Width: core.Pt(6), Height: core.Pt(12)}}}
	})
	app.Find("id:caret")
	md.Set("Some **bold")
	app.Settle()
	if p := app.Find("id:caret").Parent.Parent; p.Kind != nsgotest.KindText {
		t.Errorf("the caret is not in the paragraph's text: %v", p.Kind)
	}
	app.Find("text:Some bold")
	md.Set("Some **bold**\n\n- one\n- two")
	app.Settle()
	if n := len(app.Query("id:caret")); n != 1 {
		t.Fatalf("%d carets", n)
	}
	if !strings.Contains(app.Find("id:caret").Parent.Parent.Text, "two") {
		t.Error("the caret is not at the end of the last item")
	}
	md.Set("Done\n\n---\n")
	app.Settle()
	if p := app.Find("id:caret").Parent.Parent; p != nil && p.Kind == nsgotest.KindText {
		t.Error("after a rule the caret is not on a line of its own")
	}
}

// A block growing as a reply streams in follows its text in place: its inline images, formulas and the caret keep their views as chunks arrive, those that arrive among them included, in a search too.
func TestTextViewStreamKeepsAttachments(t *testing.T) {
	for _, c := range []struct {
		name, start string
		chunks      []string
		attachments bool
	}{
		{"paragraph", "First.\n\nSee ![a](https://x.y/a.png) and $x$", []string{" then", " **mo", "re", "** words", "."}, true},
		{"paragraph gaining a formula", "First.\n\nSee ![a](https://x.y/a.png) and", []string{" $x$", " then", " more."}, true},
		{"list item", "> - one\n> - see ![a](https://x.y/a.png) and $x$", []string{" then", " more", " words."}, true},
		{"code", "```go\nx := 1", []string{"\ny", " := 2", "\nz := 3"}, false},
	} {
		for _, searching := range []bool{false, true} {
			t.Run(fmt.Sprint(c.name, " searching ", searching), func(t *testing.T) {
				var md *core.Signal[string]
				var images, formulas, carets int
				app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
					md = core.NewSignal(cx, c.start)
					var s *textview.Search
					if searching {
						s = textview.NewSearch(cx)
						s.SetQuery("e")
						s.Open()
					}
					return textview.TextView{MarkdownFunc: md.Get, Options: textview.Options{Streaming: true}, Search: s,
						RenderImage: func(cx *core.Scope, im textview.Image) core.Node {
							images++
							return core.View{TestID: "image", Style: core.Style{Width: core.Pt(10), Height: core.Pt(10)}}
						},
						RenderMath: func(cx *core.Scope, m textview.Math) core.Node {
							formulas++
							return core.View{TestID: "formula", Style: core.Style{Width: core.Pt(10), Height: core.Pt(10)}}
						},
						StreamingCaret: core.Func(func(cx *core.Scope) core.Node {
							carets++
							return core.View{TestID: "caret", Style: core.Style{Width: core.Pt(6), Height: core.Pt(12)}}
						})}
				})
				text := c.start
				for _, chunk := range c.chunks {
					text += chunk
					md.Set(text)
					app.Settle()
				}
				want := 0
				if c.attachments {
					want = 1
				}
				if images != want || formulas != want || carets != 1 {
					t.Errorf("after %d chunks the image was made %d times, the formula %d and the caret %d, want %d, %d and 1", len(c.chunks), images, formulas, carets, want, want)
				}
				if p := app.Find("id:caret").Parent.Parent; p.Kind != nsgotest.KindText || !strings.HasSuffix(p.Text, c.chunks[len(c.chunks)-1][1:]+"\ufffc") {
					t.Errorf("the caret is not at the end of the text: %q", p.Text)
				}
			})
		}
	}
}

// A code block growing in place copies all the code that has arrived.
func TestTextViewStreamCopiesGrownCode(t *testing.T) {
	var md *core.Signal[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		md = core.NewSignal(cx, "```go\nx := 1")
		return textview.TextView{MarkdownFunc: md.Get, Options: textview.Options{Streaming: true}, CodeBlockActions: []textview.CodeAction{{ID: textview.CopyAction}},
			StreamingCaret: core.View{TestID: "caret", Style: core.Style{Width: core.Pt(6), Height: core.Pt(12)}}}
	})
	code := app.Find("text:/x := 1/").ID
	md.Set("```go\nx := 1\ny := 2")
	app.Settle()
	if app.Find("text:/y := 2/").ID != code {
		t.Error("the code's text was made again")
	}
	app.Press("label:copy")
	if app.Clipboard != "x := 1\ny := 2" {
		t.Errorf("clipboard %q", app.Clipboard)
	}
}

func TestTextViewOptionsAndPlugins(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		textview.Provide(cx, textview.Plugin{
			Extension: textview.Extension{Name: "mention", Pattern: regexp.MustCompile(`@\w+`)},
			Render:    func(cx *core.Scope, n textview.Custom) core.Node { return core.Text{Content: "user " + n.Text} },
		})
		return core.View{Children: []core.Node{
			textview.TextView{Markdown: "---\ntitle: x\n---\nHi @ada, $5 is <b>cheap</b>.\n\n> [!NOTE]\n> Plain.",
				Options:    textview.Options{Frontmatter: true, NoHTML: true, NoAlerts: true, NoMath: true},
				CustomSpan: func(n textview.Custom) core.Span { return core.Span{Content: "user " + n.Text} },
				RenderCustom: func(cx *core.Scope, n textview.Custom) core.Node {
					return core.Text{Content: n.Name + ": " + n.Text, TestID: n.Name}
				}},
			textview.TextView{Markdown: "<Chart kind=\"bar\" />\n\nIt is {1 + 2}.", Options: textview.Options{MDX: true},
				CustomSpan: func(n textview.Custom) core.Span { return core.Span{Content: "[" + n.Name + ":" + n.Text + "]"} },
				RenderCustom: func(cx *core.Scope, n textview.Custom) core.Node {
					return core.Text{Content: n.Name + " " + n.Attributes["kind"], TestID: "mdx-" + n.Name}
				}},
		}}
	})
	if got := app.Find("id:frontmatter").Text; got != "frontmatter: title: x" {
		t.Errorf("front matter %q", got)
	}
	app.Find("text:Hi user @ada, $5 is <b>cheap</b>.")
	app.Find("text:[!NOTE]")
	app.Find("text:chart bar")
	app.Find("text:It is [expression:1 + 2].")
}

func TestTextViewRebuildsOnlyChangedBlocks(t *testing.T) {
	var md *core.Signal[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		md = core.NewSignal(cx, "First paragraph.\n\nSecond")
		return textview.TextView{MarkdownFunc: md.Get}
	})
	first := app.Find("text:First paragraph.").ID
	md.Set("First paragraph.\n\nSecond, longer")
	app.Settle()
	if app.Find("text:First paragraph.").ID != first {
		t.Error("an unchanged block was built again")
	}
	app.Find("text:Second, longer")
	md.Set("First paragraph.\n\n| a |\n|---|\n| b |")
	app.Settle()
	if app.Exists("text:Second") || !app.Exists("text:b") {
		t.Error("a block that changed kind was not replaced")
	}
}

// What a document's function reads changing costs no parse while the
// document stays, as a conversation's finished replies do.
func TestTextViewParsesOnlyWhenItsDocumentChanges(t *testing.T) {
	doc := strings.Repeat("A paragraph with **bold** text.\n\n", 200)
	var tick *core.Signal[int]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		tick = core.NewSignal(cx, 0)
		return textview.TextView{MarkdownFunc: func() string { tick.Get(); return doc }}
	})
	allocs := testing.AllocsPerRun(5, func() {
		tick.Update(func(n int) int { return n + 1 })
		app.Settle()
	})
	if allocs > 100 {
		t.Fatalf("%v allocations", allocs)
	}
}

func TestTextViewShowsHTML(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{HTML: "<h2>Title</h2><p>Press <kbd>Ctrl</kbd> and <mark>see</mark> x<sup>2</sup>.</p>" +
			"<details><summary>More</summary><p>Hidden</p></details>"}
	})
	app.Find("role:header Title")
	app.Find("text:Press Ctrl and see x2.")
	app.Find("text:▸")
	if app.Exists("text:Hidden") {
		t.Error("a closed disclosure shows its blocks")
	}
	app.Press("role:button")
	app.Settle()
	app.Find("text:▾")
	app.Find("text:Hidden")
}

func TestTextViewRendersFormulasAndInlineNodes(t *testing.T) {
	var shown []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{
			Markdown:   "Energy $E=mc^2$ and ![dot](dot.png) and @ada.\n\n$$\nx^2\n$$",
			Extensions: []textview.Extension{{Name: "mention", Pattern: regexp.MustCompile(`@(\w+)`)}},
			RenderMath: func(cx *core.Scope, m textview.Math) core.Node {
				shown = append(shown, fmt.Sprint(m.TeX, " ", m.Display))
				return core.View{TestID: "math " + m.TeX, Style: core.Style{Width: core.Pt(10), Height: core.Pt(10)}}
			},
			RenderCustom: func(cx *core.Scope, node textview.Custom) core.Node {
				return core.View{TestID: "custom " + node.Text, Style: core.Style{Width: core.Pt(10), Height: core.Pt(10)}}
			},
		}
	})
	app.Find("id:math E=mc^2")
	app.Find("id:math x^2")
	app.Find("id:custom @ada")
	app.Find("role:image")
	if got := strings.Join(shown, "|"); got != "E=mc^2 false|x^2 true" {
		t.Errorf("rendered %q", got)
	}
	v := app.Find("text:Energy ￼ and ￼ and ￼.")
	// A copy takes a formula as its TeX, and leaves the other attachments out.
	var copies []string
	for _, sp := range v.Spans {
		if v.Text[sp.Start:sp.End] == "￼" {
			copies = append(copies, sp.Copy)
		}
	}
	if got := strings.Join(copies, "|"); got != "$E=mc^2$||" {
		t.Errorf("the attachments copy as %q", got)
	}
}

// A display formula centers in a sideways scroll that fills its block and takes PartMathBlockScroll's padding, so its bar runs along the block's edge as a code block's does, and copies as its TeX between $$ fences.
func TestTextViewScrollsDisplayFormulasAtTheBlocksEdge(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{Markdown: "$$\nx^2\n$$",
			Styles: textview.Styles{textview.PartMathBlock: {Background: core.RGB(0xeeeeee)}, textview.PartMathBlockScroll: {Padding: core.All(12)}},
			RenderMath: func(cx *core.Scope, m textview.Math) core.Node {
				return core.View{TestID: "formula", Style: core.Style{Width: core.Pt(10), Height: core.Pt(10)}}
			}}
	}, nsgotest.Size(300, 200))
	text := app.Find("id:formula")
	for text.Kind != nsgotest.KindText {
		text = text.Parent
	}
	if text.Spans[0].Copy != "$$\nx^2\n$$" {
		t.Errorf("the display formula copies as %q", text.Spans[0].Copy)
	}
	scroll := text.Parent
	if scroll.Kind != nsgotest.KindScroll || !scroll.Scroll.Horizontal {
		t.Fatalf("the formula's text is in a %v, not a sideways scroll", scroll.Kind)
	}
	if scroll.Scroll.HideIndicator {
		t.Errorf("the display formula's scroll bar is hidden without NoMathBlockScrollbar")
	}
	block := scroll.Parent
	if block.Style.Background == 0 || scroll.Style.Background != 0 {
		t.Errorf("the block's background is %v and its scroll's %v: PartMathBlock is the block", block.Style.Background, scroll.Style.Background)
	}
	if scroll.Frame.Size.Width != block.Frame.Size.Width || scroll.Frame.Size.Width != 300 {
		t.Errorf("the scroll is %v wide in a block %v wide: it fills the block", scroll.Frame.Size.Width, block.Frame.Size.Width)
	}
	// Headless text lays lines out at the start: the text is centered across the scroll inside its padding.
	if text.TextStyle.Align != core.TextAlignCenter || text.Frame.Origin.X != 12 || text.Frame.Size.Width != 300-24 {
		t.Errorf("the formula's text aligns %v, at %v and %v wide: it centers across the scroll's padded width", text.TextStyle.Align, text.Frame.Origin.X, text.Frame.Size.Width)
	}
}

func TestTextViewHidesDisplayFormulasScrollBarsWithNoMathBlockScrollbar(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{Markdown: "$$\nx^2\n$$\n\n```\ncode\n```", NoMathBlockScrollbar: true,
			RenderMath: func(cx *core.Scope, m textview.Math) core.Node {
				return core.View{TestID: "formula", Style: core.Style{Width: core.Pt(10), Height: core.Pt(10)}}
			}}
	})
	scroll := app.Find("id:formula")
	for scroll.Kind != nsgotest.KindScroll {
		scroll = scroll.Parent
	}
	if !scroll.Scroll.HideIndicator {
		t.Errorf("NoMathBlockScrollbar leaves the display formula's scroll bar showing")
	}
	code := app.Find("text:code")
	for code.Kind != nsgotest.KindScroll {
		code = code.Parent
	}
	if code.Scroll.HideIndicator {
		t.Errorf("NoMathBlockScrollbar hides a code block's scroll bar: that is NoCodeBlockScrollbar's")
	}
}

func TestTextViewReportsImagePressesAndOpensLinksThatLeave(t *testing.T) {
	var links []string
	var images []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{
			Markdown: "![Alone](a.png)\n\n[![Linked](b.png)](https://b.example)\n\nSee [the site](https://nsgo.dev), [mail](mailto:x@nsgo.dev), " +
				"[a section](#title) and [a page](docs/page.md) ![inline](c.png).",
			OpenLinks:    true,
			Classes:      textview.Classes{textview.PartImage: "size-10"},
			OnLinkPress:  func(e textview.LinkPressEvent) { links = append(links, e.URL) },
			OnImagePress: func(im textview.Image) { images = append(images, im.Alt) },
		}
	})
	app.Press("label:Alone")
	app.Press("label:Linked")
	app.Press("label:inline")
	for _, l := range []string{"the site", "mail", "a section", "a page"} {
		app.Press("text:" + l)
	}
	if want := "Alone|inline"; strings.Join(images, "|") != want {
		t.Errorf("images pressed %q, want %q", images, want)
	}
	if want := "https://b.example|https://nsgo.dev|mailto:x@nsgo.dev|#title|docs/page.md"; strings.Join(links, "|") != want {
		t.Errorf("links pressed %q, want %q", links, want)
	}
	if want := "https://b.example|https://nsgo.dev|mailto:x@nsgo.dev"; strings.Join(app.OpenedURLs, "|") != want {
		t.Errorf("opened %q, want the links that leave the document", app.OpenedURLs)
	}
}

func TestTextViewAlertTitleFollowsLanguage(t *testing.T) {
	i18n.SetApp(i18n.NewCatalog("en", "zh-Hans", "de"))
	defer i18n.SetApp(nil)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{Markdown: "> [!WARNING]\n> Hot."}
	}, nsgotest.Environment(core.Environment{Locale: "zh-CN"}))
	app.Find("text:警告")
	app.SetEnvironment(core.Environment{Locale: "en-US"})
	app.Find("text:Warning")
	// German is the app's, which the text view lacks: its source shows.
	app.SetEnvironment(core.Environment{Locale: "de-DE"})
	app.Find("text:Warning")
}
