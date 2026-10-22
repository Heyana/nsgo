package textview

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Extension is syntax of the app's that becomes Custom nodes named Name,
// which the TextView's RenderCustom (blocks) and CustomSpan (inline)
// show. Set one of Pattern, Language or Container.
type Extension struct {
	Name string
	// Pattern is inline syntax matched in text, outside code and links
	// (`@(\w+)` for mentions). Each match is an inline node: its Text the
	// match, its groups the attributes "1", "2", ... and by name.
	Pattern *regexp.Regexp
	// Language makes fenced code of this language (```chart) a block
	// node: its Text the code, the rest of the info string the attribute
	// "meta".
	Language string
	// Container makes containers of this name (:::note … :::, as
	// remark-directive writes them) block nodes of the Markdown inside:
	// the label after the name (:::note[Heads up]) is the attribute
	// "label", {key=value .class #id} the other attributes. A container
	// inside another closes with fewer colons than the outer one.
	Container string
}

// Parse reads Markdown into a document: CommonMark with GitHub's tables,
// task lists, strikethrough, autolinks, footnotes and alerts, and the
// syntax of extensions. Raw HTML is left out, except <br> as a line break.
func Parse(markdown string, extensions ...Extension) Document {
	src := []byte(markdown)
	root := markdownFor(extensions).Parser().Parse(text.NewReader(src))
	r := reader{src: src, ext: extensions, labels: map[int]string{}, slugs: map[string]int{}}
	var doc Document
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		if list, ok := n.(*east.FootnoteList); ok {
			for f := list.FirstChild(); f != nil; f = f.NextSibling() {
				if f, ok := f.(*east.Footnote); ok {
					r.labels[f.Index] = string(f.Ref)
				}
			}
		}
	}
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		if list, ok := n.(*east.FootnoteList); ok {
			for f := list.FirstChild(); f != nil; f = f.NextSibling() {
				if f, ok := f.(*east.Footnote); ok {
					doc.Footnotes = append(doc.Footnotes,
						Footnote{Label: string(f.Ref), Index: f.Index, Children: r.blocks(f)})
				}
			}
			continue
		}
		if b := r.block(n); b != nil {
			doc.Blocks = append(doc.Blocks, b)
		}
	}
	return doc
}

var (
	plainMarkdown     goldmark.Markdown
	plainMarkdownOnce sync.Once
)

// markdownFor returns the parser for extensions' containers, shared when
// there are none.
func markdownFor(extensions []Extension) goldmark.Markdown {
	names := map[string]string{}
	for _, e := range extensions {
		if e.Container != "" {
			names[e.Container] = e.Name
		}
	}
	if len(names) == 0 {
		plainMarkdownOnce.Do(func() { plainMarkdown = newMarkdown(nil) })
		return plainMarkdown
	}
	return newMarkdown(names)
}

func newMarkdown(containers map[string]string) goldmark.Markdown {
	opts := []goldmark.Option{goldmark.WithExtensions(extension.GFM, extension.Footnote)}
	if len(containers) > 0 {
		opts = append(opts, goldmark.WithParserOptions(parser.WithBlockParsers(
			util.Prioritized(&containerParser{names: containers}, 50))))
	}
	return goldmark.New(opts...)
}

// reader turns goldmark's syntax tree into a Document.
type reader struct {
	src    []byte
	ext    []Extension
	labels map[int]string // footnote labels by number
	slugs  map[string]int
}

func (r *reader) blocks(parent ast.Node) []Block {
	var out []Block
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		if b := r.block(n); b != nil {
			out = append(out, b)
		}
	}
	return out
}

func (r *reader) block(n ast.Node) Block {
	switch n := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		return Paragraph{Content: r.content(n)}
	case *ast.Heading:
		content := r.content(n)
		return Heading{Level: n.Level, Content: content, Anchor: r.anchor(PlainText(content))}
	case *ast.ThematicBreak:
		return Rule{}
	case *ast.CodeBlock:
		return CodeBlock{Code: r.lines(n)}
	case *ast.FencedCodeBlock:
		var lang, meta string
		if n.Info != nil {
			info := strings.TrimSpace(string(n.Info.Segment.Value(r.src)))
			lang, meta, _ = strings.Cut(info, " ")
			meta = strings.TrimSpace(meta)
		}
		code := r.lines(n)
		for _, e := range r.ext {
			if e.Language != "" && strings.EqualFold(e.Language, lang) {
				c := Custom{Name: e.Name, Text: code}
				if meta != "" {
					c.Attributes = map[string]string{"meta": meta}
				}
				return c
			}
		}
		return CodeBlock{Language: lang, Meta: meta, Code: code}
	case *ast.Blockquote:
		return r.quote(n)
	case *ast.List:
		l := List{Ordered: n.IsOrdered(), Loose: !n.IsTight}
		if l.Ordered {
			l.Start = n.Start
		}
		for it := n.FirstChild(); it != nil; it = it.NextSibling() {
			item := ListItem{Children: r.blocks(it)}
			if first := it.FirstChild(); first != nil {
				if box, ok := first.FirstChild().(*east.TaskCheckBox); ok {
					item.Task, item.Done = true, box.IsChecked
				}
			}
			l.Items = append(l.Items, item)
		}
		return l
	case *east.Table:
		t := Table{}
		for _, a := range n.Alignments {
			t.Align = append(t.Align, [...]ColumnAlign{east.AlignLeft: AlignLeft, east.AlignRight: AlignRight,
				east.AlignCenter: AlignCenter, east.AlignNone: AlignNone}[a])
		}
		for row := n.FirstChild(); row != nil; row = row.NextSibling() {
			var cells []TableCell
			for c := row.FirstChild(); c != nil; c = c.NextSibling() {
				cells = append(cells, TableCell{Content: r.content(c)})
			}
			if _, head := row.(*east.TableHeader); head {
				t.Head = cells
			} else {
				t.Rows = append(t.Rows, cells)
			}
		}
		return t
	case *container:
		return Custom{Name: n.name, Attributes: n.attrs, Children: r.blocks(n)}
	}
	return nil
}

// quote reads a blockquote, or an alert when its first line is [!KIND].
func (r *reader) quote(n *ast.Blockquote) Block {
	first, ok := n.FirstChild().(*ast.Paragraph)
	if !ok || first.Lines().Len() == 0 {
		return Blockquote{Children: r.blocks(n)}
	}
	line := first.Lines().At(0)
	alert := alertKind(strings.TrimSpace(string(line.Value(r.src))))
	if alert == AlertNone {
		return Blockquote{Children: r.blocks(n)}
	}
	q := Blockquote{Alert: alert}
	var rest []Inline
	for c := first.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok && t.Segment.Stop <= line.Stop {
			continue
		}
		rest = r.inline(rest, c)
	}
	if rest = r.finish(rest); len(rest) > 0 {
		q.Children = append(q.Children, Paragraph{Content: rest})
	}
	for c := first.NextSibling(); c != nil; c = c.NextSibling() {
		if b := r.block(c); b != nil {
			q.Children = append(q.Children, b)
		}
	}
	return q
}

func alertKind(line string) Alert {
	switch strings.ToUpper(line) {
	case "[!NOTE]":
		return AlertNote
	case "[!TIP]":
		return AlertTip
	case "[!IMPORTANT]":
		return AlertImportant
	case "[!WARNING]":
		return AlertWarning
	case "[!CAUTION]":
		return AlertCaution
	}
	return AlertNone
}

// lines returns a code block's code without its final line break.
func (r *reader) lines(n ast.Node) string {
	var b bytes.Buffer
	for i := 0; i < n.Lines().Len(); i++ {
		seg := n.Lines().At(i)
		b.Write(seg.Value(r.src))
	}
	return strings.TrimRight(b.String(), "\n")
}

// content reads a block's inline content.
func (r *reader) content(n ast.Node) []Inline {
	var out []Inline
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		out = r.inline(out, c)
	}
	return r.finish(out)
}

// finish joins neighboring texts, trims the end and splits text where
// patterns match.
func (r *reader) finish(in []Inline) []Inline {
	out := join(in)
	if len(out) > 0 {
		if t, ok := out[len(out)-1].(Text); ok {
			if t.Text = strings.TrimRightFunc(t.Text, unicode.IsSpace); t.Text == "" {
				out = out[:len(out)-1]
			} else {
				out[len(out)-1] = t
			}
		}
	}
	return applyPatterns(out, r.ext)
}

// join joins neighboring texts.
func join(in []Inline) []Inline {
	var out []Inline
	for _, it := range in {
		if t, ok := it.(Text); ok && len(out) > 0 {
			if last, ok := out[len(out)-1].(Text); ok {
				out[len(out)-1] = Text{last.Text + t.Text}
				continue
			}
		}
		out = append(out, it)
	}
	return out
}

func (r *reader) inline(out []Inline, n ast.Node) []Inline {
	switch n := n.(type) {
	case *ast.Text:
		out = append(out, Text{string(n.Segment.Value(r.src))})
		switch {
		case n.HardLineBreak():
			out = append(out, Break{})
		case n.SoftLineBreak():
			out = append(out, Text{" "})
		}
	case *ast.String:
		out = append(out, Text{string(n.Value)})
	case *ast.CodeSpan:
		out = append(out, Code{r.text(n)})
	case *ast.Emphasis:
		if n.Level >= 2 {
			out = append(out, Strong{r.children(n)})
		} else {
			out = append(out, Emphasis{r.children(n)})
		}
	case *east.Strikethrough:
		out = append(out, Strikethrough{r.children(n)})
	case *ast.Link:
		out = append(out, Link{URL: string(n.Destination), Title: string(n.Title), Content: r.children(n)})
	case *ast.AutoLink:
		url := string(n.URL(r.src))
		if n.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(strings.ToLower(url), "mailto:") {
			url = "mailto:" + url
		}
		out = append(out, Link{URL: url, Content: []Inline{Text{string(n.Label(r.src))}}})
	case *ast.Image:
		out = append(out, Image{Src: string(n.Destination), Alt: r.text(n), Title: string(n.Title)})
	case *ast.RawHTML:
		var tag strings.Builder
		for i := 0; i < n.Segments.Len(); i++ {
			s := n.Segments.At(i)
			tag.Write(s.Value(r.src))
		}
		if isBreak(tag.String()) {
			out = append(out, Break{})
		}
	case *east.FootnoteLink:
		out = append(out, FootnoteReference{Label: r.labels[n.Index], Index: n.Index})
	case *east.TaskCheckBox, *east.FootnoteBacklink:
	default:
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			out = r.inline(out, c)
		}
	}
	return out
}

// children reads n's inline content; patterns run once over the whole
// block, in finish.
func (r *reader) children(n ast.Node) []Inline {
	var out []Inline
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		out = r.inline(out, c)
	}
	return join(out)
}

// text returns the text of n's descendants.
func (r *reader) text(n ast.Node) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch c := c.(type) {
			case *ast.Text:
				b.Write(c.Segment.Value(r.src))
				if c.SoftLineBreak() {
					b.WriteByte(' ')
				}
			case *ast.String:
				b.Write(c.Value)
			}
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

func isBreak(tag string) bool {
	tag = strings.ToLower(strings.Join(strings.Fields(tag), ""))
	return tag == "<br>" || tag == "<br/>"
}

// anchor returns the next heading's slug as GitHub makes it: lower case,
// spaces as hyphens, punctuation left out, a count after repeats.
func (r *reader) anchor(text string) string {
	var b strings.Builder
	for _, c := range strings.TrimSpace(text) {
		switch {
		case c == ' ':
			b.WriteByte('-')
		case unicode.IsLetter(c) || unicode.IsDigit(c) || c == '-' || c == '_':
			b.WriteRune(unicode.ToLower(c))
		}
	}
	base := b.String()
	slug := base
	for {
		if _, seen := r.slugs[slug]; !seen {
			break
		}
		r.slugs[base]++
		slug = base + "-" + strconv.Itoa(r.slugs[base])
	}
	r.slugs[slug] = 0
	return slug
}

// applyPatterns splits text outside code and links where an extension's
// pattern matches, the earliest match first.
func applyPatterns(in []Inline, ext []Extension) []Inline {
	var patterns []Extension
	for _, e := range ext {
		if e.Pattern != nil {
			patterns = append(patterns, e)
		}
	}
	if len(patterns) == 0 {
		return in
	}
	var walk func([]Inline) []Inline
	walk = func(in []Inline) []Inline {
		var out []Inline
		for _, it := range in {
			switch it := it.(type) {
			case Text:
				out = append(out, splitText(it.Text, patterns)...)
			case Strong:
				out = append(out, Strong{walk(it.Content)})
			case Emphasis:
				out = append(out, Emphasis{walk(it.Content)})
			case Strikethrough:
				out = append(out, Strikethrough{walk(it.Content)})
			default:
				out = append(out, it)
			}
		}
		return out
	}
	return walk(in)
}

func splitText(s string, patterns []Extension) []Inline {
	var out []Inline
	for s != "" {
		at, end, which := -1, 0, -1
		var groups []int
		for i, p := range patterns {
			m := p.Pattern.FindStringSubmatchIndex(s)
			if m == nil || m[1] == m[0] {
				continue
			}
			if at < 0 || m[0] < at {
				at, end, which, groups = m[0], m[1], i, m
			}
		}
		if which < 0 {
			break
		}
		if at > 0 {
			out = append(out, Text{s[:at]})
		}
		p := patterns[which]
		c := Custom{Name: p.Name, Text: s[at:end]}
		names := p.Pattern.SubexpNames()
		for g := 1; g*2 < len(groups); g++ {
			if groups[g*2] < 0 {
				continue
			}
			if c.Attributes == nil {
				c.Attributes = map[string]string{}
			}
			v := s[groups[g*2]:groups[g*2+1]]
			c.Attributes[strconv.Itoa(g)] = v
			if names[g] != "" {
				c.Attributes[names[g]] = v
			}
		}
		out = append(out, c)
		s = s[end:]
	}
	if s != "" {
		out = append(out, Text{s})
	}
	return out
}
