package main

import (
	"regexp"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/base/textview"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/ui"
	"github.com/oneo-me/nsgo/ui/highlight"
)

const markdown = "# Text View\n\n" +
	"TextView shows **Markdown** with *emphasis*, ~~strikethrough~~, `inline code`, " +
	"[links](https://github.com/oneo-me/nsgo), autolinks such as https://go.dev and mentions like @gopher.\n\n" +
	"## Lists\n\n" +
	"- Bullets nest\n  - by depth\n    - three deep\n- [x] Tasks are done\n- [ ] or open\n\n" +
	"1. Numbered\n2. lists\n\n" +
	"## Code\n\n" +
	"```go\n// Render runs once; signals keep the view up to date.\nfunc (c Counter) Render(cx *core.Scope) core.Node {\n" +
	"\tcount := core.NewSignal(cx, 0)\n\treturn core.Text{ContentFunc: func() string { return fmt.Sprint(count.Get()) }}\n}\n```\n\n" +
	"## Tables\n\n" +
	"| Target | Reaches the system through | Status |\n|:--|:--|--:|\n" +
	"| macOS | purego and the Objective-C runtime | Runs |\n| Web | `syscall/js` in WebAssembly | Runs |\n" +
	"| Windows | `syscall` and GDI | Runs |\n\n" +
	"## Quotes and alerts\n\n" +
	"> Port the design, not the code line by line.\n\n" +
	"> [!NOTE]\n> Alerts are GitHub's `> [!NOTE]` syntax.\n\n> [!WARNING]\n> Warnings, tips, important notes and cautions too.\n\n" +
	":::callout[A container]\nContainers are an extension the app declares.\n:::\n\n" +
	"## Formulas and inline images\n\n" +
	"Formulas parse as `$…$` in text, such as $E = mc^2$ or \\(a^2 + b^2 = c^2\\), and as blocks between `$$` lines; " +
	"the app renders them with RenderMath. Prices such as $5 and $10 stay text. " +
	"An image in a line of text ![a river](https://picsum.photos/id/1015/120/60) flows with the words, three quarters of a line tall.\n\n" +
	"$$\n\\int_0^1 x^2 \\, dx = \\frac{1}{3}\n$$\n\n" +
	"## HTML\n\n" +
	"HTML in Markdown reads as text: <kbd>⌘</kbd> <kbd>C</kbd>, <mark>marks</mark>, <u>underlines</u>, " +
	"H<sub>2</sub>O and E=mc<sup>2</sup>, and <b>Markdown *inside* tags</b> stays Markdown.\n\n" +
	"<details>\n<summary>A disclosure</summary>\n\nIts blocks show while it is **open**.\n\n</details>\n\n" +
	"---\n\n" +
	"Footnotes follow the blocks.[^1]\n\n[^1]: Like this one."

// page is an HTML document, read as a reader mode reads a page.
const page = `<!DOCTYPE html><html><head><title>Ignored</title><script>track()</script></head><body>
<nav hidden>Menu</nav>
<article>
  <h2>An HTML page</h2>
  <p>TextView also reads <a href="https://html.spec.whatwg.org">HTML</a>: whitespace
     collapses as a browser collapses it, and scripts, styles and hidden elements are left out.</p>
  <ul><li><input type="checkbox" checked> Task lists</li><li><input type="checkbox"> as GitHub writes them</li></ul>
  <pre><code class="language-go">fmt.Println("code keeps its    spacing")</code></pre>
  <table><thead><tr><th>Element</th><th align="right">Reads as</th></tr></thead>
    <tbody><tr><td><code>&lt;dl&gt;</code></td><td align="right">paragraphs</td></tr>
    <tr><td><code>&lt;details&gt;</code></td><td align="right">a disclosure</td></tr></tbody></table>
</article></body></html>`

// reply is the text the streaming demo writes, a few words at a time.
const reply = "Sure. Here is how a **streamed reply** grows:\n\n" +
	"1. Each chunk is appended to the Markdown.\n2. Only the block being written is built again.\n" +
	"3. Syntax still open, such as **bold** or a [link](https://nsgo.dev), shows finished.\n\n" +
	"```go\nfor chunk := range chunks {\n\ttext.Update(func(s string) string { return s + chunk })\n}\n```\n\n" +
	"| Blocks | Built again |\n|---|---|\n| Finished | Never |\n| Last | Each chunk |\n\nDone."

func textViewPage(cx *core.Scope, t *theme) core.Node {
	link := core.NewSignal(cx, "no link pressed yet")
	streamed := core.NewSignal(cx, "")
	var stop func() bool
	stream := func() {
		if stop != nil {
			stop()
		}
		words := strings.SplitAfter(reply, " ")
		streamed.Set("")
		var next func(int)
		next = func(i int) {
			if i == len(words) {
				return
			}
			streamed.Update(func(s string) string { return s + words[i] })
			stop = cx.AfterFunc(40*time.Millisecond, func() { next(i + 1) })
		}
		next(0)
	}
	extensions := []textview.Extension{
		{Name: "mention", Pattern: regexp.MustCompile(`@(\w+)`)},
		{Name: "callout", Container: "callout"},
	}
	theme := ui.UseTheme(cx)
	return ui.ThemeView{Style: core.Style{Padding: core.All(20), Gap: 16, CornerRadius: cardRadius}, Children: []core.Node{
		ui.TextView{Markdown: markdown, Extensions: extensions, Selectable: true, Highlighter: highlight.New(cx),
			OnLinkPress: link.Set, TestID: "markdown", RenderMath: renderMath(theme),
			CustomSpan: func(node textview.Custom) core.Span {
				return core.Span{Content: node.Text, Style: core.Style{Color: theme().Primary, FontWeight: core.FontWeightMedium},
					OnPress: func(core.PressEvent) { link.Set("mention " + node.Attributes["1"]) }}
			},
			RenderCustom: func(cx *core.Scope, node textview.Custom) core.Node {
				return core.View{StyleFunc: func() core.Style {
					th := theme()
					return core.Style{Padding: core.All(12), Gap: 4, CornerRadius: th.Radius, Background: th.Accent,
						Color: th.AccentForeground}
				}, Children: []core.Node{
					core.Text{Content: node.Attributes["label"], Style: core.Style{FontWeight: core.FontWeightSemibold}},
					ui.TextView{Document: textview.Document{Blocks: node.Children}, Style: core.Style{Color: theme().AccentForeground}},
				}}
			},
		},
		ui.TextView{HTML: page, Selectable: true, Highlighter: highlight.New(cx), OnLinkPress: link.Set, TestID: "html"},
		t.live(link.Get),
		ui.Button{Label: "Stream a reply", Variant: ui.Secondary, OnPress: func(core.PressEvent) { stream() }, TestID: "stream"},
		ui.TextView{MarkdownFunc: streamed.Get, Streaming: true, Highlighter: highlight.New(cx), TestID: "streamed"},
	}}
}

// renderMath stands in for a TeX renderer, which the app brings: it shows
// a formula's TeX in a serif italic, inline on a tint, display centered.
func renderMath(theme func() ui.Tokens) func(cx *core.Scope, m textview.Math, display bool) core.Node {
	return func(cx *core.Scope, m textview.Math, display bool) core.Node {
		text := core.Text{Content: m.TeX, Style: core.Style{FontFamily: "Times New Roman", FontStyle: core.FontStyleItalic}}
		if display {
			text.Style.FontSize = 18
			return core.View{Style: core.Style{AlignItems: core.AlignCenter, Padding: core.Symmetric(8, 0)},
				Children: []core.Node{text}}
		}
		text.Style.LineHeight = 20
		return core.View{StyleFunc: func() core.Style {
			return core.Style{Background: theme().Muted, CornerRadius: 4, Padding: core.Symmetric(0, 4),
				VerticalAlign: core.VerticalAlignMiddle}
		}, Children: []core.Node{text}}
	}
}
