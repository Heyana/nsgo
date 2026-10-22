package textview

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseMath(t *testing.T) {
	m := func(tex string) Math { return Math{TeX: tex} }
	p := func(content ...Inline) Paragraph { return Paragraph{Content: content} }
	for _, c := range []struct {
		in   string
		want []Block
	}{
		{"Euler: $e^{i\\pi}+1=0$ and $$x$$.\n\n$$\n\\int_0^1 x\\,dx\n$$\n\n```math\na^2\n```", []Block{
			p(Text{"Euler: "}, m(`e^{i\pi}+1=0`), Text{" and "}, m("x"), Text{"."}),
			m(`\int_0^1 x\,dx`),
			m("a^2"),
		}},
		// Prices are prose.
		{"It costs $5 and $10, or $x$.", []Block{p(Text{"It costs $5 and $10, or "}, m("x"), Text{"."})}},
		{"Between $1 and *$2*", []Block{p(Text{"Between $1 and "}, Emphasis{[]Inline{Text{"$2"}}})}},
		{"$a $b$", []Block{p(Text{"$a "}, m("b"))}},
		// As markdown-rs, one space inside each end is dropped before the
		// prose check.
		{"from $ x $ to", []Block{p(Text{"from "}, m("x"), Text{" to"})}},
		{"from $ x$ to", []Block{p(Text{"from $ x$ to"})}},
		{"$x$2", []Block{p(Text{"$x$2"})}},
		// Doubled dollars are never prose; one space inside each end goes.
		{"$$ a + b $$ and $$5 + 5$$", []Block{p(m("a + b"), Text{" and "}, m("5 + 5"))}},
		// Over lines of a paragraph, a line ending is a space.
		{"so $a +\nb$ here", []Block{p(Text{"so "}, m("a + b"), Text{" here"})}},
		// Code keeps its dollars; an escaped dollar is one.
		{"`$x$` and \\$y and $z\\$ w", []Block{p(Code{"$x$"}, Text{" and $y and "}, m(`z\`), Text{" w"})}},
		{"```\n$x$\n```", []Block{CodeBlock{Code: "$x$"}}},
		{"    $x$", []Block{CodeBlock{Code: "$x$"}}},
		// Unclosed: text.
		{"a $$ b", []Block{p(Text{"a $$ b"})}},
		{"a $b", []Block{p(Text{"a $b"})}},
		// A fence runs to the end of its container, interrupts a
		// paragraph and closes with at least as many dollars.
		{"Formula\n$$$\nx\n$$\ny\n$$$$\nafter", []Block{p(Text{"Formula"}), m("x\n$$\ny"), p(Text{"after"})}},
		{"$$\nx^2", []Block{m("x^2")}},
		// What follows a fence is its meta, which a formula has no use for.
		{"$$ a", []Block{Math{}}},
		{"> $$\n> x\n> $$", []Block{Blockquote{Children: []Block{m("x")}}}},
		{"- $$\n  x\n  $$", []Block{List{Items: []ListItem{{Children: []Block{m("x")}}}}}},
		{"$$x$$", []Block{p(m("x"))}},
		{"| a | b |\n|---|---|\n| $x$ | 1 |", []Block{Table{Align: []ColumnAlign{AlignNone, AlignNone},
			Head: []TableCell{{Content: []Inline{Text{"a"}}}, {Content: []Inline{Text{"b"}}}},
			Rows: [][]TableCell{{{Content: []Inline{m("x")}}, {Content: []Inline{Text{"1"}}}}}}}},
	} {
		if got := Parse(c.in).Blocks; !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%q)\n got %#v\nwant %#v", c.in, got, c.want)
		}
	}
	// An extension's language comes first.
	if got := Parse("```math\nx\n```", Extension{Name: "m", Language: "math"}).Blocks; !reflect.DeepEqual(got, []Block{Custom{Name: "m", Text: "x"}}) {
		t.Errorf("math extension = %#v", got)
	}
}

func TestParseTeXDelimiters(t *testing.T) {
	m := func(tex string) Math { return Math{TeX: tex} }
	for _, c := range []struct {
		in   string
		want []Block
	}{
		{"Euler \\(e^{i\\pi} + 1 = 0\\), inline \\[x\\].\n\n\\[\n\\int_0^1 x\\,dx\n\\]\n\nAfter `\\(code\\)`", []Block{
			Paragraph{[]Inline{Text{"Euler "}, m(`e^{i\pi} + 1 = 0`), Text{", inline "}, m("x"), Text{"."}}},
			m(`\int_0^1 x\,dx`),
			Paragraph{[]Inline{Text{"After "}, Code{`\(code\)`}}},
		}},
		{"> \\[ a^2 \\]", []Block{Blockquote{Children: []Block{m("a^2")}}}},
		{"```\n\\(x\\)\n```", []Block{CodeBlock{Code: `\(x\)`}}},
		{"So \\(x^2 + y", []Block{Paragraph{[]Inline{Text{"So (x^2 + y"}}}}},
	} {
		if got := Parse(c.in).Blocks; !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%q)\n got %#v\nwant %#v", c.in, got, c.want)
		}
	}
	// Streamed: closed while open.
	got := Parse(RepairStreaming("So \\(x^2 + y")).Blocks
	if want := []Block{Paragraph{[]Inline{Text{"So "}, m("x^2 + y")}}}; !reflect.DeepEqual(got, want) {
		t.Errorf("streamed = %#v", got)
	}
}

func TestRewriteTeX(t *testing.T) {
	for _, c := range [][2]string{
		{`Euler \(e^{i\pi}\) here`, `Euler $$e^{i\pi}$$ here`},
		{`Inline \[x^2\] too`, `Inline $$x^2$$ too`},
		{`\[x\] starts a line`, `$$x$$ starts a line`},
		{"over\n\\(a +\nb\\) lines", "over\n$$a +\nb$$ lines"},
		{"`\\(code\\)` and \\(x\\)", "`\\(code\\)` and $$x$$"},
		{`escaped \\(x\\)`, `escaped \\(x\\)`},
		{`costs \($5\)`, `costs \($5\)`},
		{"plain (x) and [y]", "plain (x) and [y]"},
		{"Text\n\n\\[\nx = 1\n\\]\n\nAfter", "Text\n\n$$\nx = 1\n$$\n\nAfter"},
		{`\[ x = 1 \]`, "$$\nx = 1\n$$"},
		{"\\[ a\nb \\]\nnext", "$$\na\nb\n$$\nnext"},
		{"> \\[\n> x\n> \\]", "> $$\n> x\n> $$"},
		{"- \\[\n  x\n  \\]", "- $$\n  x\n  $$"},
		{"```\n\\[\nx\n\\]\n```", "```\n\\[\nx\n\\]\n```"},
		// Not closed: text.
		{"\\[\nx\n\nmore", "\\[\nx\n\nmore"},
	} {
		if got := rewriteTeX(c[0]); got != c[1] {
			t.Errorf("rewriteTeX(%q) = %q, want %q", c[0], got, c[1])
		}
	}
}

func FuzzParseMath(f *testing.F) {
	for _, s := range []string{"$x$", "$$\nx\n$$", `\(x\)`, "> \\[\n> x", "$", "$$", `\[`, "$ $5$"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if strings.Count(s, "\n") < 64 {
			Parse(s)
			Parse(RepairStreaming(s))
		}
	})
}
