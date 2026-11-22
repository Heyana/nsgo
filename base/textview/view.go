package textview

import (
	"cmp"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/base/textview/locales"
	"github.com/nsgo-dev/nsgo/core"
)

// TextView shows a document: Markdown or HTML, parsed once per change, else the app's own Document. Its blocks are rows keyed by position and kind, so a document that grows at its end (a reply streaming in) changes only its last block, whose texts follow their content in place while its frame stays: paragraphs, headings and code, in quotes, lists and custom containers, keep their views, the inline images, formulas and caret in them included.
type TextView struct {
	// Markdown is the document, read by Parse with Extensions.
	Markdown     string
	MarkdownFunc func() string
	// HTML is the document as HTML, read by ParseHTML with Extensions,
	// when no Markdown is given.
	HTML     string
	HTMLFunc func() string
	// Document is the document as blocks the app made, when neither
	// Markdown nor HTML is given.
	Document     Document
	DocumentFunc func() Document
	// Extensions go after those Provide gives the views below it.
	Extensions []Extension
	// Options say what the Markdown reads, Streaming among them.
	Options Options
	// StreamingCaret, as a document streams in, follows its end: inline
	// after the last text of the last block (into quotes, lists and
	// custom nodes' content, after a code block's code, in a table's last
	// cell), else on a line of its own after it.
	StreamingCaret core.Node

	// Styles style the parts; StyleOf's result for a part goes under
	// them, again when what it reads changes, and Classes under both.
	Classes Classes
	Styles  Styles
	StyleOf func(Part) core.Style
	// Selectable makes the view a selectable view: one selection runs
	// across its blocks, and on into the texts around inside an outer
	// selectable view.
	Selectable bool
	// OnTextSelection, with Selectable, receives the selection as the user moves it: plain text, Markdown, HTML, bounds and per-text ranges, as core.View.OnTextSelection does.
	OnTextSelection func(core.TextSelection)
	// Ref refers to the root view, as core.View.Ref does.
	Ref *core.Ref
	// Highlighter colors code blocks; without one their code is plain.
	Highlighter Highlighter
	// Search is the search the view joins, else the one ProvideSearch gives the views below it: the view finds its query in the texts it shows, which PartSearch and PartSearchSelected style, and reveals the selected match.
	Search *Search

	// OnLinkPress receives a pressed link's URL: a heading's anchor is
	// "#<anchor>", a footnote reference "#footnote-<label>". Nothing opens
	// a URL unless OpenLinks; the app decides.
	OnLinkPress func(LinkPressEvent)
	// OpenLinks also opens a pressed link to another site or app (a URL
	// with a scheme: https:, mailto:) outside the app with Scope.OpenURL,
	// after OnLinkPress; anchors are only reported.
	OpenLinks bool
	// OnImagePress receives an image pressed that is in no link, for the
	// app to show it larger.
	OnImagePress func(image Image)
	// RenderCustom shows a custom node: a block, or an inline node as an
	// attachment in its text unless CustomSpan shows it. Without either, a
	// node shows its content.
	RenderCustom func(cx *core.Scope, node Custom) core.Node
	// CustomSpan shows a custom node in text as a span.
	CustomSpan func(node Custom) core.Span
	// RenderMath shows a formula, which TextView places as an attachment that a copy takes as its TeX between dollars: in its text, or, when its Display is set ($$…$$ on lines of its own), centered in a block of its own, PartMathBlock around a sideways PartMathBlockScroll. Without it a formula shows its TeX in PartMath or PartMathBlock. formula.RenderMath (base/formula) typesets them, which links Untie and its font into the app.
	RenderMath func(cx *core.Scope, math Math) core.Node
	// RenderCodeBlock shows a code block in place of the default: the code
	// in a horizontal scroll view.
	RenderCodeBlock func(cx *core.Scope, code CodeBlock) core.Node
	// RenderDiagram shows a mermaid code block's diagram, which TextView places as an attachment that a copy takes as the block's source between ```mermaid fences, centered in PartDiagramBlock around a sideways PartDiagramBlockScroll. A block still streaming behind the caret shows as code until it is complete. Without it, and for other code blocks, RenderCodeBlock or the default shows them. diagram.RenderDiagram (base/diagram) draws them, which links limu into the app.
	RenderDiagram func(cx *core.Scope, code CodeBlock) core.Node
	// CodeBlockActions are buttons over each code block's top right
	// corner, in PartCodeBlockHeader, each a PartCodeBlockAction showing
	// RenderCodeBlockAction (its label, else its ID, as text without it).
	// CopyAction copies the code, and reports itself done for 1.6
	// seconds; a press on any reports OnCodeBlockAction.
	CodeBlockActions      []CodeAction
	RenderCodeBlockAction func(cx *core.Scope, action CodeBlockAction) core.Node
	OnCodeBlockAction     func(CodeActionEvent)
	// RenderTableActions shows content above each table, in
	// PartTableActions.
	RenderTableActions func(cx *core.Scope, table Table) core.Node
	// NoTableScroll lets a table shrink past its columns' floors rather than scroll; NoTableScrollbar, NoCodeBlockScrollbar, NoMathBlockScrollbar and NoDiagramBlockScrollbar hide the scroll bars of tables, code blocks, display formulas and diagrams.
	NoTableScroll, NoTableScrollbar, NoCodeBlockScrollbar, NoMathBlockScrollbar, NoDiagramBlockScrollbar bool
	// RenderImage shows an image in place of the default: alone in a
	// paragraph, a block; in a line of text, an attachment.
	RenderImage func(cx *core.Scope, image Image) core.Node
	TestID      string
	// OnLayout and Tooltip are its view's, as a core.View's.
	OnLayout func(core.Bounds)
	Tooltip  core.Node
}

// Highlighter colors code: byte ranges of the code with their styles, in order and apart. Ranges out of order, overlapping or off a character boundary are skipped. It runs as the code's text is made, and again, recoloring it in place, as the code or the signals it reads (the appearance) change.
type Highlighter func(code CodeBlock) []Highlight

// Highlight styles the bytes from Start to End of a code block's code,
// Class under Style.
type Highlight struct {
	Start, End int
	Class      string
	Style      core.Style
}

// row is a block with its position, the key of its row.
type row struct {
	index int
	block Block
}

type rowKey struct {
	index int
	kind  string
}

// Plugin is an extension a view gives every TextView below it, with
// what shows its custom nodes: Render comes before a view's
// RenderCustom, which shows the nodes Render leaves (returning nil).
type Plugin struct {
	Extension Extension
	Render    func(cx *core.Scope, node Custom) core.Node
}

var pluginsKey = core.NewKey[[]Plugin]("textview plugins", nil)

// Provide gives every TextView below cx's owner plugins, after those of
// the views around it.
func Provide(cx *core.Scope, plugins ...Plugin) {
	pluginsKey.Provide(cx, append(slices.Clip(pluginsKey.Use(cx)), plugins...))
}

// Render implements core.Node.
func (tv TextView) Render(cx *core.Scope) core.Node {
	plugins := pluginsKey.Use(cx)
	if len(plugins) > 0 {
		var ext []Extension
		for _, p := range plugins {
			ext = append(ext, p.Extension)
		}
		tv.Extensions = append(ext, tv.Extensions...)
	}
	r := &renderer{tv: tv, plugins: plugins, cx: cx}
	if tv.StreamingCaret != nil {
		r.caret = &pinnedNode{tv.StreamingCaret}
	}
	search := tv.Search
	if search == nil {
		search = searchKey.Use(cx)
	}
	root := tv.Ref
	if search != nil {
		r.search = search.join(cx, root)
		root = r.search.root
	}
	// What the sources read may change more often than they do.
	if tv.MarkdownFunc != nil {
		tv.MarkdownFunc = core.NewMemo(cx, tv.MarkdownFunc).Get
	}
	if tv.HTMLFunc != nil {
		tv.HTMLFunc = core.NewMemo(cx, tv.HTMLFunc).Get
	}
	doc := core.NewMemoFunc(cx, tv.document, nil)
	rows := func() []row {
		blocks := doc.Get().Blocks
		out := make([]row, len(blocks))
		for i, b := range blocks {
			out[i] = row{i, b}
		}
		return out
	}
	count := core.NewMemo(cx, func() int { return len(doc.Get().Blocks) })
	notes := core.NewMemoFunc(cx, func() []Footnote { return doc.Get().Footnotes }, equal[[]Footnote])
	return core.View{
		Class:           r.class(PartRoot),
		StyleFunc:       func() core.Style { return r.style(PartRoot) },
		Selectable:      tv.Selectable,
		OnTextSelection: tv.OnTextSelection,
		Ref:             root,
		TestID:          tv.TestID, OnLayout: tv.OnLayout, Tooltip: tv.Tooltip,
		Children: []core.Node{
			core.Each(rows, func(r row) rowKey { return rowKey{r.index, r.block.Kind()} },
				func(cx *core.Scope, item func() row, index func() int) core.Node {
					block := core.NewMemoFunc(cx, func() Block { return item().block }, equal[Block])
					// The row is made again as its frame changes; while it stays, its texts follow the block.
					frame := core.NewMemoFunc(cx, func() Block { return r.frame(block.Get()) }, equal[Block])
					// The last row holds the caret, so a row made last
					// once is made again when another follows it.
					last := core.NewMemo(cx, func() bool { return tv.StreamingCaret != nil && index() == count.Get()-1 })
					return core.Dynamic(func(cx *core.Scope) core.Node {
						frame.Get()
						return r.blockAt(cx, block.GetUntracked(), block.Get, index() == 0, 0, last.Get(), TextRef{Footnote: -1, Path: []int{index()}})
					})
				}),
			core.Show{When: func() bool { return tv.StreamingCaret != nil && count.Get() == 0 },
				Then: func(*core.Scope) core.Node { return r.caret }},
			core.Dynamic(func(cx *core.Scope) core.Node { return r.footnotes(cx, notes.Get()) }),
		},
	}
}

func (tv TextView) document() Document {
	switch {
	case tv.MarkdownFunc != nil:
		return ParseWith(tv.MarkdownFunc(), tv.Options, tv.Extensions...)
	case tv.Markdown != "":
		return ParseWith(tv.Markdown, tv.Options, tv.Extensions...)
	case tv.HTMLFunc != nil:
		return ParseHTML(tv.HTMLFunc(), tv.Extensions...)
	case tv.HTML != "":
		return ParseHTML(tv.HTML, tv.Extensions...)
	case tv.DocumentFunc != nil:
		return tv.DocumentFunc()
	}
	return tv.Document
}

// CopyAction is the ID of the code block action that copies the code.
const CopyAction = "copy"

// CodeAction is an action of the code blocks: its ID, CopyAction or the
// app's, and what screen readers say for it, its ID when empty.
type CodeAction struct{ ID, Label string }

// CodeActionEvent is a press on a code block's action.
type CodeActionEvent struct {
	ID   string
	Code CodeBlock
}

// LinkPressEvent is a press on a link: its URL, "#<anchor>" for a
// heading of the document, "#footnote-<label>" for a footnote.
type LinkPressEvent struct{ URL string }

// copiedFor is how long the copy action reports itself done.
const copiedFor = 1600 * time.Millisecond

// CodeBlockAction is what RenderCodeBlockAction gets about an action.
type CodeBlockAction struct {
	ID, Label string
	Code      CodeBlock
	// Done reports, tracked, that the action just ran: the copy action's
	// check, for 1.6 seconds.
	Done func() bool
}

func equal[T any](a, b T) bool { return reflect.DeepEqual(a, b) }

// noStyle is the style of text outside any inline part.
func noStyle() core.Style { return core.Style{} }

// renderer builds a TextView's blocks with the primitives.
type renderer struct {
	tv      TextView
	plugins []Plugin
	// cx opens the links that leave the document.
	cx *core.Scope
	// search is the view in its search, if any.
	search *searchView
	// caret is the StreamingCaret behind a pointer, made once, so the texts that follow their content keep its view.
	caret core.Node
}

// style merges the parts' styles: StyleOf's for each, then Styles'.
func (r *renderer) style(parts ...Part) core.Style {
	var s core.Style
	if r.tv.StyleOf != nil {
		for _, p := range parts {
			s = s.Merge(r.tv.StyleOf(p))
		}
	}
	for _, p := range parts {
		s = s.Merge(r.tv.Styles[p])
	}
	return s
}

// class joins the parts' classes, later parts winning.
func (r *renderer) class(parts ...Part) string {
	var out string
	for _, p := range parts {
		if c := r.tv.Classes[p]; c != "" {
			out = joinClasses(out, c)
		}
	}
	return out
}

func joinClasses(a, b string) string {
	if a == "" {
		return b
	}
	return a + " " + b
}

func (r *renderer) blocks(cx *core.Scope, blocks []Block, depth int, in TextRef) []core.Node {
	return r.blocksAt(cx, blocks, nil, depth, false, in)
}

// blocksAt builds blocks, the caret in the last when caret, each placed in in; live, when set, gives them as they grow.
func (r *renderer) blocksAt(cx *core.Scope, blocks []Block, live func() []Block, depth int, caret bool, in TextRef) []core.Node {
	out := make([]core.Node, len(blocks))
	for i, b := range blocks {
		var child func() Block
		if live != nil {
			child = func() Block {
				if bs := live(); i < len(bs) {
					return bs[i]
				}
				return b
			}
		}
		out[i] = r.blockAt(cx, b, child, i == 0, depth, caret && i == len(blocks)-1, in.child(i))
	}
	if caret && len(blocks) == 0 {
		out = append(out, r.caret)
	}
	return out
}

func (r *renderer) block(cx *core.Scope, b Block, first bool, depth int, at TextRef) core.Node {
	return r.blockAt(cx, b, nil, first, depth, false, at)
}

// caretSpans appends the streaming caret to spans, inline, when caret.
func (r *renderer) caretSpans(spans []core.Span, caret bool) []core.Span {
	if !caret {
		return spans
	}
	return append(spans, core.Span{Attachment: r.caret})
}

// frame is b without what its texts follow in place: the content of paragraphs' and headings' texts and code blocks' code, also in quotes, lists and custom containers. A row keeps its views while its block's frame stays.
func (r *renderer) frame(b Block) Block {
	switch b := b.(type) {
	case Paragraph:
		if _, ok := onlyImages(b.Content); !ok {
			return Paragraph{}
		}
	case Heading:
		return Heading{Level: b.Level}
	case Blockquote:
		return Blockquote{Alert: b.Alert, Children: r.frames(b.Children)}
	case List:
		items := make([]ListItem, len(b.Items))
		for i, it := range b.Items {
			items[i] = ListItem{Task: it.Task, Done: it.Done, Children: r.frames(it.Children)}
		}
		b.Items = items
		return b
	case CodeBlock:
		if r.tv.RenderCodeBlock == nil && (r.tv.RenderDiagram == nil || !IsDiagram(b)) {
			b.Code = ""
		}
		return b
	case Custom:
		if r.tv.RenderCustom == nil && !r.renders(b.Name) && len(b.Children) > 0 {
			b.Children = r.frames(b.Children)
		}
		return b
	}
	return b
}

func (r *renderer) frames(blocks []Block) []Block {
	out := make([]Block, len(blocks))
	for i, b := range blocks {
		out[i] = r.frame(b)
	}
	return out
}

// grown is what live gives in b's place: a block of b's kind, else b, while its row waits to be made again.
func grown[T Block](live func() Block, b T) T {
	if g, ok := live().(T); ok {
		return g
	}
	return b
}

// inlineText is t at at showing content, the caret after it when caret. With live, which gives the content as it grows, t follows it in place, and its attachments keep their views while they stay. blk describes the text's semantic block for Markdown/HTML selection serialization.
func (r *renderer) inlineText(cx *core.Scope, at TextRef, content []Inline, live func() []Inline, caret bool, blk core.SelectionBlock, t core.Text) core.Text {
	if live == nil {
		t.Spans = r.caretSpans(r.spans(nil, content, inlineContext{style: noStyle}, nil), caret)
		b := blk
		b.Inlines = r.selectionInlines(content, caret)
		t.SelectionBlock = &b
		return r.searched(cx, at, t)
	}
	keep := new(keeper)
	t.SpansFunc = func() []core.Span {
		keep.run()
		return r.caretSpans(r.spans(nil, live(), inlineContext{style: noStyle}, keep), caret)
	}
	t.SelectionBlockFunc = func() *core.SelectionBlock {
		b := blk
		b.Inlines = r.selectionInlines(live(), caret)
		return &b
	}
	return r.searched(cx, at, t)
}

// blockAt builds b. live, when set, gives b as it grows, which the texts whose content its frame leaves out follow; first drops a heading's top margin; depth is how many lists b is in; caret puts the streaming caret at its end; at is its place, which a search names its texts by.
func (r *renderer) blockAt(cx *core.Scope, b Block, live func() Block, first bool, depth int, caret bool, at TextRef) core.Node {
	// Blocks with no text of their own to end take the caret on a line
	// after them.
	after := func(n core.Node) core.Node {
		if !caret {
			return n
		}
		return core.Group{n, r.caret}
	}
	switch b := b.(type) {
	case Paragraph:
		if images, ok := onlyImages(b.Content); ok {
			nodes := make([]core.Node, len(images))
			for i, im := range images {
				nodes[i] = r.image(cx, im.image, im.url)
			}
			return after(core.Group(nodes))
		}
		var content func() []Inline
		if live != nil {
			content = func() []Inline { return grown(live, b).Content }
		}
		return r.inlineText(cx, at, b.Content, content, caret, core.SelectionBlock{Kind: core.SelectionBlockParagraph},
			core.Text{Class: r.class(PartParagraph), StyleFunc: func() core.Style { return r.style(PartParagraph) }})
	case Heading:
		level := min(max(b.Level, 1), 6)
		heading := func() core.Style {
			s := r.style(PartHeading, PartHeading1+Part(level-1))
			if first {
				s.Margin.Top = 0
			}
			return s
		}
		var content func() []Inline
		if live != nil {
			content = func() []Inline { return grown(live, b).Content }
		}
		return r.inlineText(cx, at, b.Content, content, caret, core.SelectionBlock{Kind: core.SelectionBlockHeading, Level: level},
			core.Text{Class: r.class(PartHeading, PartHeading1+Part(level-1)), StyleFunc: heading, Role: core.RoleHeader, AccessibilityLevel: func() int { return level }})
	case Blockquote:
		var quoted func() []Block
		if live != nil {
			quoted = func() []Block { return grown(live, b).Children }
		}
		children := r.blocksAt(cx, b.Children, quoted, depth, caret, at)
		if b.Alert == AlertNone {
			return core.View{Class: r.class(PartBlockquote), StyleFunc: func() core.Style { return r.style(PartBlockquote) },
				SelectionBlock: &core.SelectionBlock{Kind: core.SelectionBlockQuote}, Children: children}
		}
		alert := []Part{PartBlockquote, PartAlert, PartAlertNote + Part(b.Alert-AlertNote)}
		kind := selectionAlertName(b.Alert)
		a := b.Alert
		return core.View{Class: r.class(alert...), StyleFunc: func() core.Style { return r.style(alert...) },
			SelectionBlockFunc: func() *core.SelectionBlock {
				return &core.SelectionBlock{Kind: core.SelectionBlockQuote, Alert: kind, Title: alertTitle(cx, a)}
			},
			Children: []core.Node{
				r.selectable(core.Text{ContentFunc: func() string { return alertTitle(cx, b.Alert) }, Class: r.class(PartAlertTitle), StyleFunc: func() core.Style { return r.style(PartAlertTitle) }, SelectionBlock: selectionIgnore}),
				core.View{Class: r.class(PartAlertContent), StyleFunc: func() core.Style { return r.style(PartAlertContent) }, Children: children},
			}}
	case List:
		var items func() []ListItem
		if live != nil {
			items = func() []ListItem { return grown(live, b).Items }
		}
		return r.list(cx, b, items, depth, caret, at)
	case CodeBlock:
		if r.tv.RenderDiagram != nil && !caret && IsDiagram(b) {
			return after(r.diagramBlock(b))
		}
		if r.tv.RenderCodeBlock != nil {
			return after(r.tv.RenderCodeBlock(cx, b))
		}
		var code func() CodeBlock
		if live != nil {
			code = func() CodeBlock { return grown(live, b) }
		}
		return r.code(cx, b, code, caret, at)
	case Table:
		return r.table(cx, b, caret, at)
	case Rule:
		return after(core.View{Class: r.class(PartRule), StyleFunc: func() core.Style { return r.style(PartRule) },
			SelectionBlock: &core.SelectionBlock{Kind: core.SelectionBlockRule}})
	case Details:
		return after(r.details(cx, b, depth, at))
	case Math:
		if r.tv.RenderMath != nil {
			b.Display = true
			return after(r.mathBlock(b))
		}
		mathInlines := []core.SelectionInline{{Kind: core.SelectionInlineMath, Text: b.TeX}}
		if caret {
			mathInlines = append(mathInlines, core.SelectionInline{Kind: core.SelectionInlineSkip, UTF16Len: 1})
			return r.searched(cx, at, core.Text{Spans: r.caretSpans([]core.Span{{Content: b.TeX}}, true), Class: r.class(PartMathBlock), StyleFunc: func() core.Style { return r.style(PartMathBlock) },
				SelectionBlock: &core.SelectionBlock{Kind: core.SelectionBlockMath, TeX: b.TeX, Inlines: mathInlines}})
		}
		return r.searched(cx, at, core.Text{Content: b.TeX, Class: r.class(PartMathBlock), StyleFunc: func() core.Style { return r.style(PartMathBlock) },
			SelectionBlock: &core.SelectionBlock{Kind: core.SelectionBlockMath, TeX: b.TeX, Inlines: mathInlines}})
	case Custom:
		if n := r.custom(cx, b); n != nil {
			return after(n)
		}
		switch {
		case len(b.Children) > 0:
			var children func() []Block
			if live != nil {
				children = func() []Block { return grown(live, b).Children }
			}
			return core.View{Class: r.class(PartCustom), StyleFunc: func() core.Style { return r.style(PartCustom) }, Children: r.blocksAt(cx, b.Children, children, depth, caret, at)}
		case len(b.Content) > 0:
			return r.inlineText(cx, at, b.Content, nil, caret, core.SelectionBlock{Kind: core.SelectionBlockParagraph},
				core.Text{Class: r.class(PartCustom), StyleFunc: func() core.Style { return r.style(PartCustom) }})
		}
		customInlines := []core.SelectionInline{{Kind: core.SelectionInlineText, Text: b.Text}}
		if caret {
			customInlines = append(customInlines, core.SelectionInline{Kind: core.SelectionInlineSkip, UTF16Len: 1})
			return r.searched(cx, at, core.Text{Spans: r.caretSpans([]core.Span{{Content: b.Text}}, true), Class: r.class(PartCustom), StyleFunc: func() core.Style { return r.style(PartCustom) },
				SelectionBlock: &core.SelectionBlock{Kind: core.SelectionBlockParagraph, Inlines: customInlines}})
		}
		return r.searched(cx, at, core.Text{Content: b.Text, Class: r.class(PartCustom), StyleFunc: func() core.Style { return r.style(PartCustom) },
			SelectionBlock: &core.SelectionBlock{Kind: core.SelectionBlockParagraph, Inlines: customInlines}})
	}
	return nil
}

// renders reports whether a plugin renders custom nodes named name.
func (r *renderer) renders(name string) bool {
	for _, p := range r.plugins {
		if p.Render != nil && p.Extension.Name == name {
			return true
		}
	}
	return false
}

// custom shows a custom node through the plugins' renderers, then
// RenderCustom; nil when neither shows it.
func (r *renderer) custom(cx *core.Scope, b Custom) core.Node {
	for _, p := range r.plugins {
		if p.Render != nil && p.Extension.Name == b.Name {
			if n := p.Render(cx, b); n != nil {
				return n
			}
		}
	}
	if r.tv.RenderCustom != nil {
		return r.tv.RenderCustom(cx, b)
	}
	return nil
}

// details shows a disclosure: its summary, which a press opens and closes, and its blocks, made each time it opens, so a search finds them only while it is open.
func (r *renderer) details(cx *core.Scope, d Details, depth int, at TextRef) core.Node {
	open := core.NewSignal(cx, d.Open)
	return core.View{Class: r.class(PartDetails), StyleFunc: func() core.Style { return r.style(PartDetails) },
		SelectionBlock: &core.SelectionBlock{Kind: core.SelectionBlockDetails},
		Children: []core.Node{
			core.View{Class: r.class(PartSummary), StyleFunc: func() core.Style { return r.style(PartSummary) }, Role: core.RoleButton, OnPress: func(core.PressEvent) { open.Set(!open.Get()) },
				Children: []core.Node{
					r.selectable(core.Text{ContentFunc: func() string {
						if open.Get() {
							return "▾"
						}
						return "▸"
					}, Class: r.class(PartSummaryMarker), StyleFunc: func() core.Style { return r.style(PartSummaryMarker) }, SelectionBlock: selectionIgnore}),
					r.inlineText(cx, at.child(-1), d.Summary, nil, false, core.SelectionBlock{Kind: core.SelectionBlockSummary}, core.Text{Style: core.Style{FlexGrow: 1, FlexShrink: 1}}),
				}},
			core.Show{When: open.Get, Then: func(cx *core.Scope) core.Node {
				return core.Group(r.blocks(cx, d.Children, depth, at))
			}},
		}}
}

var bullets = [...]string{"•", "◦", "▪"}

// list builds l; live, when set, gives its items as they grow.
func (r *renderer) list(cx *core.Scope, l List, live func() []ListItem, depth int, caret bool, at TextRef) core.Node {
	kind := PartUnorderedList
	if l.Ordered {
		kind = PartOrderedList
	}
	items := make([]core.Node, len(l.Items))
	for i, it := range l.Items {
		var marker core.Node
		switch {
		case it.Task:
			parts := []Part{PartTaskMarker}
			check := ""
			if it.Done {
				parts = append(parts, PartTaskMarkerChecked)
				check = "✓"
			}
			done := it.Done
			marker = core.View{Class: r.class(parts...), StyleFunc: func() core.Style { return r.style(parts...) }, Role: core.RoleCheckbox, Checked: func() bool { return done },
				SelectionBlock: selectionIgnore,
				Children:       []core.Node{r.selectable(core.Text{Content: check, SelectionBlock: selectionIgnore})}}
		case l.Ordered:
			marker = r.selectable(core.Text{Content: strconv.Itoa(l.Start+i) + ".", Class: r.class(PartListMarker), StyleFunc: func() core.Style { return r.style(PartListMarker) }, SelectionBlock: selectionIgnore})
		default:
			marker = r.selectable(core.Text{Content: bullets[depth%len(bullets)], Class: r.class(PartListMarker), StyleFunc: func() core.Style { return r.style(PartListMarker) }, SelectionBlock: selectionIgnore})
		}
		content := []Part{PartListItemContent}
		if it.Task && it.Done {
			content = append(content, PartTaskDone)
		}
		var children func() []Block
		if live != nil {
			children = func() []Block {
				if items := live(); i < len(items) {
					return items[i].Children
				}
				return it.Children
			}
		}
		items[i] = core.View{Class: r.class(PartListItem), StyleFunc: func() core.Style { return r.style(PartListItem) },
			SelectionBlock: &core.SelectionBlock{Kind: core.SelectionBlockListItem, Index: i, Task: it.Task, Done: it.Done},
			Children: []core.Node{
				marker,
				core.View{Class: r.class(content...), StyleFunc: func() core.Style { return r.style(content...) }, Children: r.blocksAt(cx, it.Children, children, depth+1, caret && i == len(l.Items)-1, at.child(i))},
			}}
	}
	return core.View{Class: r.class(PartList, kind), StyleFunc: func() core.Style { return r.style(PartList, kind) },
		SelectionBlock: &core.SelectionBlock{Kind: core.SelectionBlockList, Ordered: l.Ordered, Start: l.Start, Loose: l.Loose},
		Children:       items}
}

// code builds c; live, when set, gives it as it grows.
func (r *renderer) code(cx *core.Scope, c CodeBlock, live func() CodeBlock, caret bool, at TextRef) core.Node {
	current := func() CodeBlock { return c }
	if live != nil {
		current = live
	}
	// The code recolors in place as the highlighter's colors change (the appearance, a lexer done) and grows in place as it streams in, the caret, made once, kept at its end. A search's parts go over the highlighter's colors.
	spans := func() []core.Span {
		c := current()
		var highlights []Highlight
		if r.tv.Highlighter != nil {
			highlights = r.tv.Highlighter(c)
		}
		return r.caretSpans(codeSpans(c.Code, highlights), caret)
	}
	code := r.searched(cx, at, core.Text{SpansFunc: spans, Class: r.class(PartCodeBlockCode), StyleFunc: func() core.Style { return r.style(PartCodeBlockCode) },
		SelectionBlockFunc: func() *core.SelectionBlock {
			cur := current()
			var inlines []core.SelectionInline
			if cur.Code != "" {
				inlines = append(inlines, core.SelectionInline{Kind: core.SelectionInlineText, Text: cur.Code})
			}
			if caret {
				inlines = append(inlines, core.SelectionInline{Kind: core.SelectionInlineSkip, UTF16Len: 1})
			}
			return &core.SelectionBlock{Kind: core.SelectionBlockCode, Language: cur.Language, Meta: cur.Meta, Inlines: inlines}
		}})
	children := []core.Node{core.Scroll{Horizontal: true, HideIndicator: r.tv.NoCodeBlockScrollbar,
		Class: r.class(PartCodeBlockScroll), StyleFunc: func() core.Style { return r.style(PartCodeBlockScroll) }, Children: []core.Node{code}}}
	if len(r.tv.CodeBlockActions) > 0 {
		// The actions take the code they act on, so they are made again as it grows.
		header := func(cx *core.Scope) core.Node {
			c := current()
			actions := make([]core.Node, len(r.tv.CodeBlockActions))
			for i, a := range r.tv.CodeBlockActions {
				actions[i] = r.codeAction(cx, a, c)
			}
			return core.View{
				Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0), Direction: core.Row},
				Class: r.class(PartCodeBlockHeader), StyleFunc: func() core.Style { return r.style(PartCodeBlockHeader) },
				SelectionBlock: selectionIgnore,
				Children:       actions,
			}
		}
		if live != nil {
			children = append(children, core.Dynamic(header))
		} else {
			children = append(children, header(cx))
		}
	}
	return core.View{Class: r.class(PartCodeBlock), StyleFunc: func() core.Style { return r.style(PartCodeBlock) }, Children: children}
}

// codeAction is a code block's action button.
func (r *renderer) codeAction(cx *core.Scope, action CodeAction, c CodeBlock) core.Node {
	done := core.NewSignal(cx, false)
	var stop func() bool
	name := action.ID
	label := cmp.Or(action.Label, name)
	a := CodeBlockAction{ID: name, Label: action.Label, Code: c, Done: done.Get}
	var content core.Node = r.selectable(core.Text{Content: label})
	if r.tv.RenderCodeBlockAction != nil {
		content = r.tv.RenderCodeBlockAction(cx, a)
	}
	return core.View{Class: r.class(PartCodeBlockAction), StyleFunc: func() core.Style { return r.style(PartCodeBlockAction) }, Role: core.RoleButton, Label: label,
		OnPress: func(core.PressEvent) {
			if name == CopyAction {
				cx.SetClipboard(c.Code)
				if stop != nil {
					stop()
				}
				done.Set(true)
				stop = cx.AfterFunc(copiedFor, func() { done.Set(false) })
			}
			if r.tv.OnCodeBlockAction != nil {
				r.tv.OnCodeBlockAction(CodeActionEvent{ID: name, Code: c})
			}
		},
		Children: []core.Node{content}}
}

// codeSpans splits code into spans at the highlights that are in order,
// apart and on character boundaries.
func codeSpans(code string, highlights []Highlight) []core.Span {
	var out []core.Span
	at := 0
	boundary := func(i int) bool { return i == len(code) || utf8.RuneStart(code[i]) }
	for _, h := range highlights {
		if h.Start < at || h.End <= h.Start || h.End > len(code) || !boundary(h.Start) || !boundary(h.End) {
			continue
		}
		if h.Start > at {
			out = append(out, core.Span{Content: code[at:h.Start]})
		}
		out = append(out, core.Span{Content: code[h.Start:h.End], Class: h.Class, Style: h.Style})
		at = h.End
	}
	if at < len(code) || len(out) == 0 {
		out = append(out, core.Span{Content: code[at:]})
	}
	return out
}

// table lays the cells out as one grid, so columns line up across rows:
// each column fits its content when there is room and shrinks, its cells
// wrapping, down to a floor estimated from its text; below the floors the
// grid scrolls.
func (r *renderer) table(cx *core.Scope, t Table, caret bool, at TextRef) core.Node {
	cols := t.Columns()
	// The floors are estimates, made with the font size at hand.
	em := core.Untracked(cx, func() core.Points { return r.style(PartTableCell).FontSize })
	if em == 0 {
		em = core.DefaultFontSize
	}
	floors := make([]float64, cols)
	rows := append([][]TableCell{t.Head}, t.Rows...)
	for _, row := range rows {
		col := 0
		for _, c := range row {
			if c.span() == 1 && col < cols {
				floors[col] = max(floors[col], textWidth(PlainText(c.Content)))
			}
			col += c.span()
		}
	}
	var tracks strings.Builder
	var total float64
	for i, f := range floors {
		f = min(f+2, min(max(f/2, 10), 30)) * float64(em)
		total += f
		if i > 0 {
			tracks.WriteByte(' ')
		}
		fmt.Fprintf(&tracks, "minmax(%gpx, auto)", f)
	}
	var children []core.Node
	for i, row := range rows {
		line := core.GridLines{Start: core.GridLine(i + 1)}
		rowParts, cellParts := []Part{PartTableRow}, []Part{PartTableCell}
		if i == 0 {
			rowParts, cellParts = append(rowParts, PartTableHead), append(cellParts, PartTableHeaderCell)
		}
		children = append(children, core.View{Class: r.class(rowParts...), StyleFunc: func() core.Style {
			return r.style(rowParts...).Merge(core.Style{GridRow: line, GridColumn: core.GridLines{Start: core.GridLine(1), End: core.GridLine(-1)}})
		}})
		col := 0
		for k, c := range row {
			place := at.child(i, k)
			at, span := col, c.span()
			s := func() core.Style {
				s := r.style(cellParts...)
				if at < len(t.Align) {
					s.TextAlign = [...]core.TextAlign{AlignNone: s.TextAlign, AlignLeft: core.TextAlignStart,
						AlignCenter: core.TextAlignCenter, AlignRight: core.TextAlignEnd}[t.Align[at]]
				}
				s.GridRow = line
				s.GridColumn = core.GridLines{Start: core.GridLine(at + 1), End: core.GridSpan(span)}
				return s
			}
			lastCell := caret && i == len(rows)-1 && col+c.span() >= cols
			children = append(children, r.inlineText(cx, place, c.Content, nil, lastCell,
				core.SelectionBlock{Kind: core.SelectionBlockTableCell, Header: i == 0, Row: i, Col: at, ColSpan: span},
				core.Text{Class: r.class(cellParts...), StyleFunc: s}))
			col += c.span()
		}
	}
	grid := core.Style{Display: core.DisplayGrid, GridTemplateColumns: core.GridTracks(tracks.String()), Width: core.Pct(100)}
	var body core.Node
	if r.tv.NoTableScroll {
		body = core.View{Style: grid, Children: children}
	} else {
		grid.MinWidth = core.Pt(core.Points(total))
		body = core.Scroll{Horizontal: true, HideIndicator: r.tv.NoTableScrollbar, Class: r.class(PartTableScroll), StyleFunc: func() core.Style { return r.style(PartTableScroll) },
			Children: []core.Node{core.View{Style: grid, Children: children}}}
	}
	nodes := []core.Node{body}
	if r.tv.RenderTableActions != nil {
		nodes = []core.Node{core.View{Class: r.class(PartTableActions), StyleFunc: func() core.Style { return r.style(PartTableActions) }, SelectionBlock: selectionIgnore, Children: []core.Node{r.tv.RenderTableActions(cx, t)}}, body}
	}
	return core.View{Class: r.class(PartTable), StyleFunc: func() core.Style { return r.style(PartTable) },
		SelectionBlock: &core.SelectionBlock{Kind: core.SelectionBlockTable, Align: selectionAligns(t.Align)},
		Children:       nodes}
}

// textWidth estimates the width of text's widest line in ems: wide (East
// Asian) characters an em, spaces a third, the rest a little over half.
func textWidth(text string) float64 {
	var widest float64
	for line := range strings.SplitSeq(text, "\n") {
		var w float64
		for _, c := range line {
			switch {
			case c == ' ':
				w += 0.3
			case c >= 0x1100 && c <= 0x115f, c >= 0x2e80 && c <= 0xa4cf, c >= 0xac00 && c <= 0xd7a3,
				c >= 0xf900 && c <= 0xfaff, c >= 0xfe30 && c <= 0xfe4f, c >= 0xff00 && c <= 0xff60,
				c >= 0xffe0 && c <= 0xffe6, c >= 0x1f300 && c <= 0x1faff, c >= 0x20000 && c <= 0x3fffd:
				w++
			default:
				w += 0.6
			}
		}
		widest = max(widest, w)
	}
	return widest
}

func (r *renderer) footnotes(cx *core.Scope, notes []Footnote) core.Node {
	if len(notes) == 0 {
		return nil
	}
	rows := make([]core.Node, len(notes))
	for i, f := range notes {
		in := TextRef{Footnote: i}
		rows[i] = core.View{Class: r.class(PartFootnote), StyleFunc: func() core.Style { return r.style(PartFootnote) },
			SelectionBlock: &core.SelectionBlock{Kind: core.SelectionBlockFootnote, Label: f.Label, Index: f.Index},
			Children: []core.Node{
				r.selectable(core.Text{Content: strconv.Itoa(f.Index) + ".", Class: r.class(PartFootnoteLabel), StyleFunc: func() core.Style { return r.style(PartFootnoteLabel) }, SelectionBlock: selectionIgnore}),
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: r.blocks(cx, f.Children, 0, in)},
			}}
	}
	return core.View{Class: r.class(PartFootnotes), StyleFunc: func() core.Style { return r.style(PartFootnotes) }, Children: rows}
}

// inlineContext is what inline content takes from the inline parts around it: their style and classes, the press of the link around it, if any, and trail, which names the parts and the link for a keeper.
type inlineContext struct {
	style func() core.Style
	class string
	link  func(core.PressEvent)
	trail string
}

// in is c inside part p, its style over c's.
func (c inlineContext) in(r *renderer, p Part) inlineContext {
	style := c.style
	return inlineContext{style: func() core.Style { return style().Merge(r.style(p)) }, class: joinClasses(c.class, r.class(p)), link: c.link,
		trail: c.trail + "/" + strconv.Itoa(int(p))}
}

// keeper keeps the attachments of a text that follows its content in place: each run of its spans gives an attachment the node the run before gave the one in its place, when it was made of the same inline in the same parts, so core keeps its view.
type keeper struct{ last, next []kept }

type kept struct {
	key  keptKey
	node core.Node
}

type keptKey struct {
	trail  string
	inline Inline
}

// run starts a run of the spans.
func (k *keeper) run() { k.last, k.next = k.next, nil }

// node is the attachment showing inline in parts trail, which build makes; a nil keeper builds it each time.
func (k *keeper) node(trail string, inline Inline, build func() core.Node) core.Node {
	if k == nil {
		return build()
	}
	key := keptKey{trail, inline}
	i := len(k.next)
	var n core.Node
	if i < len(k.last) && reflect.DeepEqual(k.last[i].key, key) {
		n = k.last[i].node
	} else {
		n = &pinnedNode{build()}
	}
	k.next = append(k.next, kept{key, n})
	return n
}

// spans appends content's spans to out, each styled over c's style and classes, the inline parts around it, and pressing c's link, if any; keep, if set, keeps their attachments.
func (r *renderer) spans(out []core.Span, content []Inline, c inlineContext, keep *keeper) []core.Span {
	style, class, link := c.style, c.class, c.link
	for _, in := range content {
		switch in := in.(type) {
		case Text:
			out = append(out, core.Span{Content: in.Text, Class: class, StyleFunc: style, OnPress: link})
		case Strong:
			out = r.spans(out, in.Content, c.in(r, PartStrong), keep)
		case Emphasis:
			out = r.spans(out, in.Content, c.in(r, PartEmphasis), keep)
		case Strikethrough:
			out = r.spans(out, in.Content, c.in(r, PartStrikethrough), keep)
		case Underline:
			out = r.spans(out, in.Content, c.in(r, PartUnderline), keep)
		case Mark:
			mark := c.in(r, PartMark)
			if in.Color != 0 {
				under := mark.style
				mark.style = func() core.Style {
					s := under()
					s.Background = in.Color
					return s
				}
				mark.trail += "#" + strconv.FormatUint(uint64(in.Color), 16)
			}
			out = r.spans(out, in.Content, mark, keep)
		case Superscript:
			out = r.spans(out, in.Content, c.in(r, PartSuperscript), keep)
		case Subscript:
			out = r.spans(out, in.Content, c.in(r, PartSubscript), keep)
		case Small:
			out = r.spans(out, in.Content, c.in(r, PartSmall), keep)
		case Code:
			code := c.in(r, PartCode)
			out = append(out, core.Span{Content: in.Code, Class: code.class, StyleFunc: code.style, OnPress: link})
		case Keyboard:
			kbd := c.in(r, PartKeyboard)
			out = append(out, core.Span{Content: in.Text, Class: kbd.class, StyleFunc: kbd.style, OnPress: link})
		case Math:
			if render := r.tv.RenderMath; render != nil {
				out = append(out, core.Span{Content: "$" + in.TeX + "$", Attachment: keep.node(c.trail, in, func() core.Node {
					return core.Func(func(cx *core.Scope) core.Node { return render(cx, in) })
				})})
			} else {
				math := c.in(r, PartMath)
				out = append(out, core.Span{Content: in.TeX, Class: math.class, StyleFunc: math.style, OnPress: link})
			}
		case Link:
			linked := c.in(r, PartLink)
			linked.link = r.press(in.URL)
			linked.trail += "\x00" + in.URL + "\x00"
			out = r.spans(out, in.Content, linked, keep)
		case Image:
			out = append(out, core.Span{Attachment: keep.node(c.trail, in, func() core.Node { return r.inlineImage(in, style, link) })})
		case Break:
			out = append(out, core.Span{Content: "\n", Class: class, StyleFunc: style})
		case FootnoteReference:
			ref := c.in(r, PartFootnoteReference)
			out = append(out, core.Span{Content: "[" + strconv.Itoa(in.Index) + "]", Class: ref.class,
				StyleFunc: ref.style, OnPress: r.press("#footnote-" + in.Label)})
		case Custom:
			if r.tv.CustomSpan != nil {
				s := r.tv.CustomSpan(in)
				own, ownFunc := s.Style, s.StyleFunc
				s.Class = joinClasses(class, s.Class)
				s.Style, s.StyleFunc = core.Style{}, func() core.Style {
					st := style().Merge(own)
					if ownFunc != nil {
						st = st.Merge(ownFunc())
					}
					return st
				}
				if s.Attachment != nil {
					attachment := s.Attachment
					s.Attachment = keep.node(c.trail, in, func() core.Node { return attachment })
				}
				out = append(out, s)
			} else if r.tv.RenderCustom != nil || r.renders(in.Name) {
				out = append(out, core.Span{Attachment: keep.node(c.trail, in, func() core.Node {
					return core.Func(func(cx *core.Scope) core.Node { return r.custom(cx, in) })
				})})
			} else {
				var b strings.Builder
				in.write(&b)
				custom := c.in(r, PartCustom)
				out = append(out, core.Span{Content: b.String(), Class: custom.class, StyleFunc: custom.style, OnPress: link})
			}
		}
	}
	return out
}

// press returns what a press on a link to url does, if anything: report
// it, then with OpenLinks open a URL that leaves the document.
func (r *renderer) press(link string) func(core.PressEvent) {
	open := r.tv.OpenLinks && leaves(link)
	if r.tv.OnLinkPress == nil && !open {
		return nil
	}
	return func(core.PressEvent) {
		if r.tv.OnLinkPress != nil {
			r.tv.OnLinkPress(LinkPressEvent{URL: link})
		}
		if open {
			r.cx.OpenURL(link, nil)
		}
	}
}

// leaves reports whether link names another site or app: it has a
// scheme.
func leaves(link string) bool {
	u, err := url.Parse(link)
	return err == nil && u.Scheme != ""
}

// imagePress returns what a press on an image in no link does: report it
// with OnImagePress.
func (r *renderer) imagePress(im Image) func(core.PressEvent) {
	if r.tv.OnImagePress == nil {
		return nil
	}
	return func(core.PressEvent) { r.tv.OnImagePress(im) }
}

type linkedImage struct {
	image Image
	url   string
}

// onlyImages returns a paragraph's images when it holds nothing else
// (links around them aside): they show as blocks.
func onlyImages(content []Inline) ([]linkedImage, bool) {
	var out []linkedImage
	for _, in := range content {
		switch in := in.(type) {
		case Image:
			out = append(out, linkedImage{image: in})
		case Link:
			if len(in.Content) != 1 {
				return nil, false
			}
			im, ok := in.Content[0].(Image)
			if !ok {
				return nil, false
			}
			out = append(out, linkedImage{image: im, url: in.URL})
		default:
			return nil, false
		}
	}
	return out, len(out) > 0
}

// inlineImage shows an image in a line of text styled style: the size it
// asks for, else three quarters of a line tall at its natural aspect.
func (r *renderer) inlineImage(im Image, style func() core.Style, link func(core.PressEvent)) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		press := link
		if press == nil {
			press = r.imagePress(im)
		}
		if r.tv.RenderImage != nil {
			return r.renderedImage(cx, im, press)
		}
		natural := core.NewSignal(cx, core.Size{})
		return core.Image{
			Source:  core.Src(im.Src),
			Label:   im.Alt,
			OnLoad:  natural.Set,
			OnPress: press,
			StyleFunc: func() core.Style {
				text := r.style(PartRoot, PartParagraph).Merge(style())
				line := text.LineHeight
				if size := text.FontSize; line == 0 && size > 0 {
					line = size * 1.2
				} else if line == 0 {
					line = core.DefaultFontSize * 1.2
				}
				s := core.Style{Width: im.Width, Height: im.Height}
				if s.Width.IsAuto() && s.Height.IsAuto() {
					s.Height = core.Pt(line * 0.75)
				}
				if n := natural.Get(); n.Width > 0 && n.Height > 0 {
					s.AspectRatio = float32(n.Width / n.Height)
				} else if s.Width.IsAuto() {
					s.Width = s.Height
				}
				return s
			},
		}
	})
}

// image shows an image as a block: the size it asks for, else its
// natural size once loaded, at most the text's width.
func (r *renderer) image(cx *core.Scope, im Image, url string) core.Node {
	press := r.imagePress(im)
	if url != "" {
		press = r.press(url)
	}
	if r.tv.RenderImage != nil {
		return r.renderedImage(cx, im, press)
	}
	natural := core.NewSignal(cx, core.Size{})
	return core.Image{
		Source: core.Src(im.Src),
		Label:  im.Alt,
		OnLoad: natural.Set,
		StyleFunc: func() core.Style {
			s := core.Style{MaxWidth: core.Pct(100), AlignSelf: core.AlignStart, Width: im.Width, Height: im.Height}
			if n := natural.Get(); n.Width > 0 && n.Height > 0 {
				s.AspectRatio = float32(n.Width / n.Height)
				if im.Width.IsAuto() && im.Height.IsAuto() {
					s.Width = core.Pt(n.Width)
				}
			}
			return s.Merge(r.style(PartImage))
		},
		Class:   r.class(PartImage),
		OnPress: press,
	}
}

// alertTitle says a's title in the language the app shows under cx.
func alertTitle(cx *core.Scope, a Alert) string {
	m := locales.For(cx)
	switch a {
	case AlertNote:
		return m.AlertNote()
	case AlertTip:
		return m.AlertTip()
	case AlertImportant:
		return m.AlertImportant()
	case AlertWarning:
		return m.AlertWarning()
	case AlertCaution:
		return m.AlertCaution()
	}
	return ""
}

// selectable returns t selectable when the view is, as NSH marks the
// texts of a selectable TextView.
func (r *renderer) selectable(t core.Text) core.Text {
	if r.tv.Selectable {
		t.Selectable = core.SelectableOn
	}
	return t
}

// renderedImage is the app's rendering of an image, which still presses
// as the image and is named by its text, as NSH wraps it.
func (r *renderer) renderedImage(cx *core.Scope, im Image, press func(core.PressEvent)) core.Node {
	return core.View{Label: im.Alt, Role: core.RoleImage, OnPress: press, Children: []core.Node{r.tv.RenderImage(cx, im)}}
}
