package textview

import (
	"reflect"
	"regexp"
	"testing"
)

func TestParseBlocks(t *testing.T) {
	doc := Parse("# Getting started!\n\nSome **bold** and *soft*\ntext with `code`, ~~gone~~ and [a link](https://x.y \"T\").\n\n" +
		"- [x] done\n- [ ] open\n\n3. three\n4. four\n\n```go title=x\nfunc main() {}\n```\n\n---\n\n" +
		"| A | B |\n|:--|--:|\n| 1 | 2 |\n\n> quoted\n\nSee https://nsgo.dev and line<br>break.")
	want := []Block{
		Heading{Level: 1, Content: []Inline{Text{"Getting started!"}}, Anchor: "getting-started"},
		Paragraph{Content: []Inline{
			Text{"Some "}, Strong{[]Inline{Text{"bold"}}}, Text{" and "}, Emphasis{[]Inline{Text{"soft"}}},
			Text{" text with "}, Code{"code"}, Text{", "}, Strikethrough{[]Inline{Text{"gone"}}}, Text{" and "},
			Link{URL: "https://x.y", Title: "T", Content: []Inline{Text{"a link"}}}, Text{"."},
		}},
		List{Items: []ListItem{
			{Task: true, Done: true, Children: []Block{Paragraph{[]Inline{Text{"done"}}}}},
			{Task: true, Children: []Block{Paragraph{[]Inline{Text{"open"}}}}},
		}},
		List{Ordered: true, Start: 3, Items: []ListItem{
			{Children: []Block{Paragraph{[]Inline{Text{"three"}}}}},
			{Children: []Block{Paragraph{[]Inline{Text{"four"}}}}},
		}},
		CodeBlock{Language: "go", Meta: "title=x", Code: "func main() {}"},
		Rule{},
		Table{Align: []ColumnAlign{AlignLeft, AlignRight},
			Head: []TableCell{{Content: []Inline{Text{"A"}}}, {Content: []Inline{Text{"B"}}}},
			Rows: [][]TableCell{{{Content: []Inline{Text{"1"}}}, {Content: []Inline{Text{"2"}}}}}},
		Blockquote{Children: []Block{Paragraph{[]Inline{Text{"quoted"}}}}},
		Paragraph{Content: []Inline{Text{"See "}, Link{URL: "https://nsgo.dev", Content: []Inline{Text{"https://nsgo.dev"}}},
			Text{" and line"}, Break{}, Text{"break."}}},
	}
	for i := range max(len(want), len(doc.Blocks)) {
		if i >= len(want) || i >= len(doc.Blocks) || !reflect.DeepEqual(doc.Blocks[i], want[i]) {
			var got, w any
			if i < len(doc.Blocks) {
				got = doc.Blocks[i]
			}
			if i < len(want) {
				w = want[i]
			}
			t.Errorf("block %d:\n got %#v\nwant %#v", i, got, w)
		}
	}
}

func TestParseAlertsAndFootnotes(t *testing.T) {
	doc := Parse("> [!WARNING]\n> Mind the *gap*.\n\nA claim[^src] and another[^2].\n\n[^src]: The source.\n[^2]: Second.")
	alert := Blockquote{Alert: AlertWarning, Children: []Block{
		Paragraph{[]Inline{Text{"Mind the "}, Emphasis{[]Inline{Text{"gap"}}}, Text{"."}}}}}
	if !reflect.DeepEqual(doc.Blocks[0], alert) {
		t.Errorf("alert = %#v", doc.Blocks[0])
	}
	para := Paragraph{[]Inline{Text{"A claim"}, FootnoteReference{"src", 1}, Text{" and another"}, FootnoteReference{"2", 2}, Text{"."}}}
	if !reflect.DeepEqual(doc.Blocks[1], para) {
		t.Errorf("references = %#v", doc.Blocks[1])
	}
	notes := []Footnote{
		{Label: "src", Index: 1, Children: []Block{Paragraph{[]Inline{Text{"The source."}}}}},
		{Label: "2", Index: 2, Children: []Block{Paragraph{[]Inline{Text{"Second."}}}}},
	}
	if !reflect.DeepEqual(doc.Footnotes, notes) {
		t.Errorf("footnotes = %#v", doc.Footnotes)
	}
}

func TestParseExtensions(t *testing.T) {
	ext := []Extension{
		{Name: "mention", Pattern: regexp.MustCompile(`@(?P<user>\w+)`)},
		{Name: "chart", Language: "chart"},
		{Name: "callout", Container: "note"},
	}
	doc := Parse("Hi @ann, see `@not` and **@bob**.\n\n```chart bar\n1,2\n```\n\n"+
		"::::note[Heads up]{kind=tip .wide #n1}\nInside *note*.\n\n:::note\nNested\n:::\n::::\n\n:::other\ntext", ext...)
	mention := func(u string) Custom {
		return Custom{Name: "mention", Text: "@" + u, Attributes: map[string]string{"1": u, "user": u}}
	}
	want := []Block{
		Paragraph{[]Inline{Text{"Hi "}, mention("ann"), Text{", see "}, Code{"@not"}, Text{" and "},
			Strong{[]Inline{mention("bob")}}, Text{"."}}},
		Custom{Name: "chart", Text: "1,2", Attributes: map[string]string{"meta": "bar"}},
		Custom{Name: "callout", Attributes: map[string]string{"label": "Heads up", "kind": "tip", "class": "wide", "id": "n1"},
			Children: []Block{
				Paragraph{[]Inline{Text{"Inside "}, Emphasis{[]Inline{Text{"note"}}}, Text{"."}}},
				Custom{Name: "callout", Children: []Block{Paragraph{[]Inline{Text{"Nested"}}}}},
			}},
		Paragraph{[]Inline{Text{":::other text"}}},
	}
	if !reflect.DeepEqual(doc.Blocks, want) {
		for i, b := range doc.Blocks {
			t.Logf("block %d: %#v", i, b)
		}
		t.Error("extensions were not read as custom nodes")
	}
}

func TestAnchorsAreUnique(t *testing.T) {
	doc := Parse("# Intro\n## Intro\n## API: `fetch()` & more\n# Intro")
	var got []string
	for _, b := range doc.Blocks {
		got = append(got, b.(Heading).Anchor)
	}
	if want := []string{"intro", "intro-1", "api-fetch--more", "intro-2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("anchors = %q, want %q", got, want)
	}
}

func TestPlainText(t *testing.T) {
	doc := Parse("a **b**\n\n- x\n- y\n\n| h | i |\n|---|---|\n| 1 | 2 |\n\nnote[^1]\n\n[^1]: n")
	if got, want := doc.PlainText(), "a b\n\nx\ny\n\nh\ti\n1\t2\n\nnote[1]\n\nn"; got != want {
		t.Errorf("PlainText = %q, want %q", got, want)
	}
}
