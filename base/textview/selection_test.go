package textview_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestSelectionPartialSpansAndLinks(t *testing.T) {
	var sel core.TextSelection
	const md = "Hello **bold *and italic***, ~~old~~, [NSGo docs](https://nsgo.dev \"Home\") and `fmt.Println()`! 👋"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{
			TestID:          "doc",
			Markdown:        md,
			Selectable:      true,
			OnTextSelection: func(s core.TextSelection) { sel = s },
		}
	})

	// Rendered text: "Hello bold and italic, old, NSGo docs and fmt.Println()! 👋" Offsets:
	// "Hello "            [0, 6) "bold "             [6, 11) "and italic"        [11, 21) ", "                [21, 23) "old"               [23, 26) ", "                [26, 28) "NSGo docs"         [28, 37) " and "             [37, 42) "fmt.Println()"     [42, 55) "! "                [55, 57) "👋"                [57, 59)

	// 1. Partial selection across the middle of bold into nested italic.
	app.SelectTextRange("id:doc", "text:/Hello bold/", 8, "text:/Hello bold/", 17)
	if sel.Text != "ld and it" {
		t.Fatalf("Text = %q, want %q", sel.Text, "ld and it")
	}
	if sel.Markdown != "**ld *and it***" {
		t.Errorf("Markdown = %q, want %q", sel.Markdown, "**ld *and it***")
	}
	if sel.HTML != "<strong>ld <em>and it</em></strong>" {
		t.Errorf("HTML = %q, want %q", sel.HTML, "<strong>ld <em>and it</em></strong>")
	}

	// 2. Partial selection inside a link preserves its URL and title.
	app.SelectTextRange("id:doc", "text:/Hello bold/", 30, "text:/Hello bold/", 35)
	if sel.Text != "Go do" {
		t.Fatalf("Text = %q, want %q", sel.Text, "Go do")
	}
	if sel.Markdown != `[Go do](https://nsgo.dev "Home")` {
		t.Errorf("Markdown = %q, want %q", sel.Markdown, `[Go do](https://nsgo.dev "Home")`)
	}
	if sel.HTML != `<a href="https://nsgo.dev" title="Home">Go do</a>` {
		t.Errorf("HTML = %q, want %q", sel.HTML, `<a href="https://nsgo.dev" title="Home">Go do</a>`)
	}

	// 3. Partial selection inside inline code + trailing text + emoji (UTF-16 surrogate pair).
	app.SelectTextRange("id:doc", "text:/Hello bold/", 46, "text:/Hello bold/", 59)
	if sel.Text != "Println()! 👋" {
		t.Fatalf("Text = %q, want %q", sel.Text, "Println()! 👋")
	}
	if sel.Markdown != "`Println()`! 👋" {
		t.Errorf("Markdown = %q, want %q", sel.Markdown, "`Println()`! 👋")
	}
	if sel.HTML != "<code>Println()</code>! 👋" {
		t.Errorf("HTML = %q, want %q", sel.HTML, "<code>Println()</code>! 👋")
	}

	// 4. Whole paragraph selection wraps HTML in <p>.
	app.SelectTextRange("id:doc", "text:/Hello bold/", 0, "text:/Hello bold/", 59)
	if sel.Markdown != md {
		t.Errorf("whole Markdown = %q, want %q", sel.Markdown, md)
	}
	wantHTML := `<p>Hello <strong>bold <em>and italic</em></strong>, <del>old</del>, <a href="https://nsgo.dev" title="Home">NSGo docs</a> and <code>fmt.Println()</code>! 👋</p>`
	if sel.HTML != wantHTML {
		t.Errorf("whole HTML = %q, want %q", sel.HTML, wantHTML)
	}
}

func TestSelectionMultiBlockHeadingRuleQuoteListCodeAndTable(t *testing.T) {
	var sel core.TextSelection
	const md = "# Guide\n\nIntro **text**.\n\n---\n\n> Quoted *line*.\n\n> [!TIP]\n> Helpful note.\n\n1. First\n2. Second\n3. Third\n\n- [ ] Todo\n- [x] Done\n\n```go\nx := 1\ny := 2\n```\n\n| Name | Value |\n| :--- | ---: |\n| **A** | `1` |\n| B | 2 |"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{
			TestID:           "doc",
			Markdown:         md,
			Selectable:       true,
			CodeBlockActions: []textview.CodeAction{{ID: textview.CopyAction}},
			OnTextSelection:  func(s core.TextSelection) { sel = s },
		}
	})

	// 1. Heading across horizontal rule into partial blockquote.
	app.SelectTextRange("id:doc", "text:Guide", 0, "text:Quoted line.", 11)
	wantMD := "# Guide\n\nIntro **text**.\n\n---\n\n> Quoted *line*"
	wantHTML := "<h1>Guide</h1>\n<p>Intro <strong>text</strong>.</p>\n<hr>\n<blockquote>\n<p>Quoted <em>line</em></p>\n</blockquote>"
	if sel.Markdown != wantMD {
		t.Errorf("multi-block Markdown =\n%s\nwant:\n%s", sel.Markdown, wantMD)
	}
	if sel.HTML != wantHTML {
		t.Errorf("multi-block HTML =\n%s\nwant:\n%s", sel.HTML, wantHTML)
	}

	// 2. GitHub alert (omits duplicate alert title).
	app.SelectTextRange("id:doc", "text:Tip", 0, "text:Helpful note.", 13)
	if sel.Markdown != "> [!TIP]\n> Helpful note." {
		t.Errorf("alert Markdown = %q", sel.Markdown)
	}
	wantAlertHTML := "<div class=\"markdown-alert markdown-alert-tip\">\n<p class=\"markdown-alert-title\">Tip</p>\n<p>Helpful note.</p>\n</div>"
	if sel.HTML != wantAlertHTML {
		t.Errorf("alert HTML = %q, want %q", sel.HTML, wantAlertHTML)
	}

	// 3. Partial ordered list starting at item 2 (preserves start="2" and omits bullet/number marker duplication).
	app.SelectTextRange("id:doc", "text:Second", 0, "text:Third", 5)
	if sel.Markdown != "2. Second\n3. Third" {
		t.Errorf("ordered list Markdown = %q", sel.Markdown)
	}
	if sel.HTML != "<ol start=\"2\">\n<li>Second</li>\n<li>Third</li>\n</ol>" {
		t.Errorf("ordered list HTML = %q", sel.HTML)
	}

	// 4. Task list items.
	app.SelectTextRange("id:doc", "text:Todo", 0, "text:Done", 4)
	if sel.Markdown != "- [ ] Todo\n- [x] Done" {
		t.Errorf("task list Markdown = %q", sel.Markdown)
	}
	wantTaskHTML := "<ul>\n<li><input type=\"checkbox\" disabled> Todo</li>\n<li><input type=\"checkbox\" checked disabled> Done</li>\n</ul>"
	if sel.HTML != wantTaskHTML {
		t.Errorf("task list HTML = %q, want %q", sel.HTML, wantTaskHTML)
	}

	// 5. Partial code block selection preserves language and fences, ignores action button.
	app.SelectTextRange("id:doc", "text:/x := 1/", 0, "text:/x := 1/", 6)
	if sel.Markdown != "```go\nx := 1\n```" {
		t.Errorf("code Markdown = %q", sel.Markdown)
	}
	if sel.HTML != `<pre><code class="language-go">x := 1</code></pre>` {
		t.Errorf("code HTML = %q", sel.HTML)
	}

	// 6. Single table cell stays inline; multi-cell table serializes as GFM / HTML table.
	app.SelectTextRange("id:doc", "text:A", 0, "text:A", 1)
	if sel.Markdown != "**A**" || sel.HTML != "<strong>A</strong>" {
		t.Errorf("single cell: md=%q html=%q", sel.Markdown, sel.HTML)
	}
	app.SelectTextRange("id:doc", "text:Name", 0, "text:/^2$/", 1)
	wantTableMD := "| Name | Value |\n| :--- | ---: |\n| **A** | `1` |\n| B | 2 |"
	if sel.Markdown != wantTableMD {
		t.Errorf("table Markdown =\n%s\nwant:\n%s", sel.Markdown, wantTableMD)
	}
	wantTableHTML := "<table>\n<thead>\n<tr><th style=\"text-align:left\">Name</th><th style=\"text-align:right\">Value</th></tr>\n</thead>\n<tbody>\n<tr><td style=\"text-align:left\"><strong>A</strong></td><td style=\"text-align:right\"><code>1</code></td></tr>\n<tr><td style=\"text-align:left\">B</td><td style=\"text-align:right\">2</td></tr>\n</tbody>\n</table>"
	if sel.HTML != wantTableHTML {
		t.Errorf("table HTML =\n%s\nwant:\n%s", sel.HTML, wantTableHTML)
	}
}

func TestSelectionHTMLDocumentDetailsMathAndFootnotes(t *testing.T) {
	var sel core.TextSelection
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{
			TestID:     "doc",
			HTML:       `<h2>Notes</h2><p>Press <kbd>Ctrl</kbd> and <mark>see</mark> <u>this</u> x<sup>2</sup><sub>i</sub> <small>fine</small>.</p><details open><summary>More</summary><p>Inside <em>box</em>.</p></details>`,
			Selectable: true,
			OnTextSelection: func(s core.TextSelection) {
				sel = s
			},
		}
	})

	app.SelectTextRange("id:doc", "text:Notes", 0, "text:Inside box.", 11)
	wantMD := "## Notes\n\nPress <kbd>Ctrl</kbd> and <mark>see</mark> <u>this</u> x<sup>2</sup><sub>i</sub> <small>fine</small>.\n\n<details>\n<summary>More</summary>\n\nInside *box*.\n</details>"
	wantHTML := "<h2>Notes</h2>\n<p>Press <kbd>Ctrl</kbd> and <mark>see</mark> <u>this</u> x<sup>2</sup><sub>i</sub> <small>fine</small>.</p>\n<details>\n<summary>More</summary>\n<p>Inside <em>box</em>.</p>\n</details>"
	if sel.Markdown != wantMD {
		t.Errorf("HTML doc Markdown =\n%s\nwant:\n%s", sel.Markdown, wantMD)
	}
	if sel.HTML != wantHTML {
		t.Errorf("HTML doc HTML =\n%s\nwant:\n%s", sel.HTML, wantHTML)
	}
}

func TestSelectionAcrossMultipleTextViewsAndPlainTextWithClipboard(t *testing.T) {
	var sel core.TextSelection
	var copyMD, copyHTML func()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		copyMD = func() { cx.SetClipboard(sel.Markdown) }
		copyHTML = func() { cx.SetClipboard(sel.HTML) }
		return core.View{
			TestID:          "area",
			Selectable:      true,
			OnTextSelection: func(s core.TextSelection) { sel = s },
			Children: []core.Node{
				textview.TextView{Markdown: "Which **platforms** run NSGo?"},
				textview.TextView{Markdown: "- macOS\n- Linux"},
				core.Text{Content: "Ignored label", Selectable: core.SelectableOff},
				core.Text{Content: "Plain trailing text."},
			},
		}
	})

	// Select from middle of TextView #1 across TextView #2 into trailing plain core.Text.
	app.SelectTextRange("id:area", "text:Which platforms run NSGo?", 6, "text:Plain trailing text.", 14)
	wantMD := "**platforms** run NSGo?\n\n- macOS\n- Linux\n\nPlain trailing"
	wantHTML := "<p><strong>platforms</strong> run NSGo?</p>\n<ul>\n<li>macOS</li>\n<li>Linux</li>\n</ul>\n<p>Plain trailing</p>"
	if sel.Markdown != wantMD {
		t.Errorf("multi-child Markdown =\n%s\nwant:\n%s", sel.Markdown, wantMD)
	}
	if sel.HTML != wantHTML {
		t.Errorf("multi-child HTML =\n%s\nwant:\n%s", sel.HTML, wantHTML)
	}

	// mod+c copies plain text to clipboard; app can also copy Markdown or HTML.
	app.Key("mod+c")
	if !strings.Contains(app.Clipboard, "platforms run NSGo?") || !strings.Contains(app.Clipboard, "Plain trailing") {
		t.Errorf("mod+c clipboard = %q", app.Clipboard)
	}
	copyMD()
	app.Settle()
	if app.Clipboard != wantMD {
		t.Errorf("copied Markdown = %q, want %q", app.Clipboard, wantMD)
	}
	copyHTML()
	app.Settle()
	if app.Clipboard != wantHTML {
		t.Errorf("copied HTML = %q, want %q", app.Clipboard, wantHTML)
	}

	// Escape clears the active selection.
	app.Key("Escape")
	if sel.Text != "" || sel.Markdown != "" || sel.HTML != "" {
		t.Errorf("after Escape sel = %+v, want cleared", sel)
	}
}

func TestSelectionWithStreamingCaretSearchMathAndDiagrams(t *testing.T) {
	var md *core.Signal[string]
	var sel core.TextSelection
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		md = core.NewSignal(cx, "Formula $E=mc^2$ and **live")
		s := textview.NewSearch(cx)
		s.SetQuery("live")
		s.Open()
		return textview.TextView{
			TestID:       "doc",
			MarkdownFunc: md.Get,
			Options:      textview.Options{Streaming: true},
			Selectable:   true,
			Search:       s,
			RenderMath: func(cx *core.Scope, m textview.Math) core.Node {
				return core.View{Style: core.Style{Width: core.Pt(10), Height: core.Pt(10)}}
			},
			RenderDiagram: func(cx *core.Scope, c textview.CodeBlock) core.Node {
				return core.View{Style: core.Style{Width: core.Pt(20), Height: core.Pt(20)}}
			},
			StreamingCaret:  core.View{TestID: "caret", Style: core.Style{Width: core.Pt(4), Height: core.Pt(12)}},
			OnTextSelection: func(s core.TextSelection) { sel = s },
		}
	})

	// Select across the rendered math attachment and streamed bold text (including caret at end).
	p := app.Find("text:/Formula/")
	app.SelectTextRange("id:doc", "text:/Formula/", 0, "text:/Formula/", len([]rune(p.Text)))
	if sel.Markdown != "Formula $E=mc^2$ and **live**" {
		t.Errorf("streaming Markdown = %q", sel.Markdown)
	}
	wantHTML := `<p>Formula <math><semantics><annotation encoding="application/x-tex">E=mc^2</annotation></semantics></math> and <strong>live</strong></p>`
	if sel.HTML != wantHTML {
		t.Errorf("streaming HTML = %q, want %q", sel.HTML, wantHTML)
	}

	// Grow the stream and verify selection reflects the updated content.
	md.Set("Formula $E=mc^2$ and **live text**!\n\n```mermaid\ngraph LR\nA-->B\n```\n\nDone.")
	app.Settle()
	app.SelectTextRange("id:doc", "text:/Formula/", 0, "text:Done.", 5)
	if !strings.Contains(sel.Markdown, "```mermaid\ngraph LR\nA-->B\n```") {
		t.Errorf("diagram missing in Markdown:\n%s", sel.Markdown)
	}
	if !strings.Contains(sel.HTML, `<pre><code class="language-mermaid">graph LR`+"\n"+`A--&gt;B</code></pre>`) {
		t.Errorf("diagram missing in HTML:\n%s", sel.HTML)
	}
}

func TestDocumentMarkdownAndHTMLRoundTrip(t *testing.T) {
	const src = "# Heading\n\nA **bold**, *italic*, ~~struck~~, `code`, [link](https://nsgo.dev) and $a+b$.\n\n> Quoted\n\n- one\n- two\n\n```go\nfmt.Println(1)\n```"
	doc := textview.Parse(src)
	md := doc.Markdown()
	htmlOut := doc.HTML()
	if md != src {
		t.Errorf("Document.Markdown() =\n%s\nwant:\n%s", md, src)
	}
	reparsedMD := textview.Parse(md)
	if !reflect.DeepEqual(reparsedMD, doc) {
		t.Errorf("re-parsed Markdown differs:\ngot:  %#v\nwant: %#v", reparsedMD, doc)
	}
	reparsedHTML := textview.ParseHTML(htmlOut)
	if !reflect.DeepEqual(reparsedHTML, doc) {
		t.Errorf("re-parsed HTML differs:\ngot:  %#v\nwant: %#v", reparsedHTML, doc)
	}
}

func TestSelectionNestedListsBlockquotesAndFootnotes(t *testing.T) {
	var sel core.TextSelection
	const md = "- parent\n  - nested *child*\n- sibling\n\n> outer\n>\n> > inner\n\nRef[^1].\n\n[^1]: The **note**."
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{
			TestID:          "doc",
			Markdown:        md,
			Selectable:      true,
			OnTextSelection: func(s core.TextSelection) { sel = s },
		}
	})

	// Nested list preserves indentation and nesting without duplicating bullets.
	app.SelectTextRange("id:doc", "text:parent", 0, "text:sibling", 7)
	wantListMD := "- parent\n  - nested *child*\n- sibling"
	wantListHTML := "<ul>\n<li>parent\n<ul>\n<li>nested <em>child</em></li>\n</ul>\n</li>\n<li>sibling</li>\n</ul>"
	if sel.Markdown != wantListMD {
		t.Errorf("nested list Markdown =\n%s\nwant:\n%s", sel.Markdown, wantListMD)
	}
	if sel.HTML != wantListHTML {
		t.Errorf("nested list HTML =\n%s\nwant:\n%s", sel.HTML, wantListHTML)
	}

	// Nested blockquote.
	app.SelectTextRange("id:doc", "text:outer", 0, "text:inner", 5)
	wantQuoteMD := "> outer\n>\n> > inner"
	wantQuoteHTML := "<blockquote>\n<p>outer</p>\n<blockquote>\n<p>inner</p>\n</blockquote>\n</blockquote>"
	if sel.Markdown != wantQuoteMD {
		t.Errorf("nested quote Markdown =\n%s\nwant:\n%s", sel.Markdown, wantQuoteMD)
	}
	if sel.HTML != wantQuoteHTML {
		t.Errorf("nested quote HTML =\n%s\nwant:\n%s", sel.HTML, wantQuoteHTML)
	}

	// Footnote reference + definition (label "1." omitted from duplication).
	app.SelectTextRange("id:doc", "text:Ref[1].", 0, "text:The note.", 9)
	wantFootMD := "Ref[^1].\n\n[^1]: The **note**."
	wantFootHTML := "<p>Ref<sup><a href=\"#footnote-1\">1</a></sup>.</p>\n<div id=\"footnote-1\">\n<p>The <strong>note</strong>.</p>\n</div>"
	if sel.Markdown != wantFootMD {
		t.Errorf("footnote Markdown =\n%s\nwant:\n%s", sel.Markdown, wantFootMD)
	}
	if sel.HTML != wantFootHTML {
		t.Errorf("footnote HTML =\n%s\nwant:\n%s", sel.HTML, wantFootHTML)
	}
}
