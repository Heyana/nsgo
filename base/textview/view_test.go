package textview_test

import (
	"strings"
	"testing"

	"github.com/oneo-me/nsgo/base/textview"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/nsgotest"
)

const doc = "# Title\n\n[Docs](https://nsgo.dev) and **bold**.\n\n- one\n- [x] two\n\n```go\nx := 1\n```\n\n" +
	"| A | B |\n|---|---|\n| 1 | 2 |\n\n> [!TIP]\n> Use it.\n\n:::callout\nInside\n:::\n\nNote[^1].\n\n[^1]: The note."

func TestTextViewBuildsBlocks(t *testing.T) {
	var pressed []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{Markdown: doc, Selectable: true, TestID: "doc",
			Extensions:  []textview.Extension{{Name: "callout", Container: "callout"}},
			OnLinkPress: func(url string) { pressed = append(pressed, url) },
			Highlighter: func(c textview.CodeBlock) []textview.Highlight {
				return []textview.Highlight{{Start: 0, End: 1, Style: core.Style{Color: core.RGB(0xff0000)}}, {Start: 0, End: 2}}
			},
			RenderCustom: func(cx *core.Scope, node textview.Custom) core.Node {
				return core.Text{Content: "custom " + node.Name, TestID: "custom"}
			},
			CodeBlockActions: func(cx *core.Scope, code textview.CodeBlock) core.Node {
				return core.View{OnPress: func() { pressed = append(pressed, "copy "+code.Code) }, TestID: "copy"}
			},
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
	app.Press("id:copy")
	if want := []string{"https://nsgo.dev", "copy x := 1"}; strings.Join(pressed, "|") != strings.Join(want, "|") {
		t.Errorf("pressed %q, want %q", pressed, want)
	}
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
