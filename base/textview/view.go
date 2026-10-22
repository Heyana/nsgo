package textview

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/oneo-me/nsgo/core"
)

// TextView shows a document: Markdown or HTML, parsed once per change,
// else the app's own Document. Its blocks are rows keyed by position and kind, so
// a document that grows at its end (a reply streaming in) builds only its
// last block again.
type TextView struct {
	// Markdown is the document, read by Parse with Extensions;
	// MarkdownFunc replaces it, again when what it reads changes.
	Markdown     string
	MarkdownFunc func() string
	// HTML is the document as HTML, read by ParseHTML with Extensions,
	// when no Markdown is given; HTMLFunc replaces it.
	HTML     string
	HTMLFunc func() string
	// Document is the document as blocks the app made, when neither
	// Markdown nor HTML is given; DocumentFunc replaces it.
	Document     Document
	DocumentFunc func() Document
	Extensions   []Extension
	// Streaming repairs the syntax the Markdown's last block leaves open
	// before parsing (RepairStreaming), for a document still arriving.
	Streaming bool

	// Styles style the parts; StyleFunc's result for a part goes under
	// them, again when what it reads changes. ui.TextView passes its
	// typography as StyleFunc.
	Styles    Styles
	StyleFunc func(Part) core.Style
	// Selectable makes the view a selection area: one selection runs
	// across its blocks, and on into the texts around inside an outer
	// area.
	Selectable bool
	// Highlighter colors code blocks; without one their code is plain.
	Highlighter Highlighter

	// OnLinkPress receives a pressed link's URL: a heading's anchor is
	// "#<anchor>", a footnote reference "#footnote-<label>". Nothing opens
	// a URL; the app decides.
	OnLinkPress func(url string)
	// RenderCustom shows a custom node: a block, or an inline node as an
	// attachment in its text unless CustomSpan shows it. Without either, a
	// node shows its content.
	RenderCustom func(cx *core.Scope, node Custom) core.Node
	// CustomSpan shows a custom node in text as a span.
	CustomSpan func(node Custom) core.Span
	// RenderMath shows a formula, display when it is a block of its own
	// ($$…$$ on lines of its own), else as an attachment in its text.
	// Without it a formula shows its TeX in PartMath or PartMathBlock.
	RenderMath func(cx *core.Scope, math Math, display bool) core.Node
	// RenderCodeBlock shows a code block in place of the default: the code
	// in a horizontal scroll view.
	RenderCodeBlock func(cx *core.Scope, code CodeBlock) core.Node
	// CodeBlockActions is content over each code block's top right corner,
	// such as a copy button, in PartCodeBlockHeader.
	CodeBlockActions func(cx *core.Scope, code CodeBlock) core.Node
	// RenderImage shows an image in place of the default: alone in a
	// paragraph, a block; in a line of text, an attachment.
	RenderImage func(cx *core.Scope, image Image) core.Node
	TestID      string
}

// Highlighter colors code: byte ranges of the code with their styles, in
// order and apart. Ranges out of order, overlapping or off a character
// boundary are skipped. It runs while the block is built, so the signals
// it reads (the appearance) build the block again.
type Highlighter func(code CodeBlock) []Highlight

// Highlight styles the bytes from Start to End of a code block's code.
type Highlight struct {
	Start, End int
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

// Render implements core.Node.
func (tv TextView) Render(cx *core.Scope) core.Node {
	r := &renderer{tv: tv}
	doc := core.NewMemoFunc(cx, tv.document, nil)
	rows := func() []row {
		blocks := doc.Get().Blocks
		out := make([]row, len(blocks))
		for i, b := range blocks {
			out[i] = row{i, b}
		}
		return out
	}
	notes := core.NewMemoFunc(cx, func() []Footnote { return doc.Get().Footnotes }, equal[[]Footnote])
	return core.View{
		StyleFunc:     func() core.Style { return r.style(PartRoot) },
		SelectionArea: tv.Selectable,
		TestID:        tv.TestID,
		Children: []core.Node{
			core.Each(rows, func(r row) rowKey { return rowKey{r.index, r.block.Kind()} },
				func(cx *core.Scope, item func() row, index func() int) core.Node {
					block := core.NewMemoFunc(cx, func() Block { return item().block }, equal[Block])
					return core.Dynamic(func(cx *core.Scope) core.Node {
						return r.block(cx, block.Get(), index() == 0, 0)
					})
				}),
			core.Dynamic(func(cx *core.Scope) core.Node { return r.footnotes(cx, notes.Get()) }),
		},
	}
}

func (tv TextView) document() Document {
	switch {
	case tv.MarkdownFunc != nil:
		return tv.parse(tv.MarkdownFunc())
	case tv.Markdown != "":
		return tv.parse(tv.Markdown)
	case tv.HTMLFunc != nil:
		return ParseHTML(tv.HTMLFunc(), tv.Extensions...)
	case tv.HTML != "":
		return ParseHTML(tv.HTML, tv.Extensions...)
	case tv.DocumentFunc != nil:
		return tv.DocumentFunc()
	}
	return tv.Document
}

func (tv TextView) parse(markdown string) Document {
	if tv.Streaming {
		markdown = RepairStreaming(markdown)
	}
	return Parse(markdown, tv.Extensions...)
}

func equal[T any](a, b T) bool { return reflect.DeepEqual(a, b) }

// renderer builds a TextView's blocks with the primitives.
type renderer struct{ tv TextView }

// style merges the parts' styles: StyleFunc's for each, then Styles'.
func (r *renderer) style(parts ...Part) core.Style {
	var s core.Style
	if r.tv.StyleFunc != nil {
		for _, p := range parts {
			s = s.Merge(r.tv.StyleFunc(p))
		}
	}
	for _, p := range parts {
		s = s.Merge(r.tv.Styles[p])
	}
	return s
}

func (r *renderer) blocks(cx *core.Scope, blocks []Block, depth int) []core.Node {
	out := make([]core.Node, len(blocks))
	for i, b := range blocks {
		out[i] = r.block(cx, b, i == 0, depth)
	}
	return out
}

// block builds b. first drops a heading's top margin; depth is how many
// lists b is in.
func (r *renderer) block(cx *core.Scope, b Block, first bool, depth int) core.Node {
	switch b := b.(type) {
	case Paragraph:
		if images, ok := onlyImages(b.Content); ok {
			nodes := make([]core.Node, len(images))
			for i, im := range images {
				nodes[i] = r.image(cx, im.image, im.url)
			}
			return core.Group(nodes)
		}
		return core.Text{Spans: r.spans(nil, b.Content, core.Style{}, nil), Style: r.style(PartParagraph)}
	case Heading:
		level := min(max(b.Level, 1), 6)
		s := r.style(PartHeading, PartHeading1+Part(level-1))
		if first {
			s.Margin.Top = 0
		}
		return core.Text{Spans: r.spans(nil, b.Content, core.Style{}, nil), Style: s, Role: core.RoleHeader}
	case Blockquote:
		children := r.blocks(cx, b.Children, depth)
		if b.Alert == AlertNone {
			return core.View{Style: r.style(PartBlockquote), Children: children}
		}
		return core.View{Style: r.style(PartBlockquote, PartAlert, PartAlertNote+Part(b.Alert-AlertNote)),
			Children: []core.Node{
				core.Text{Content: b.Alert.String(), Style: r.style(PartAlertTitle)},
				core.View{Style: r.style(PartAlertContent), Children: children},
			}}
	case List:
		return r.list(cx, b, depth)
	case CodeBlock:
		return r.code(cx, b)
	case Table:
		return r.table(b)
	case Rule:
		return core.View{Style: r.style(PartRule)}
	case Details:
		return r.details(cx, b, depth)
	case Math:
		if r.tv.RenderMath != nil {
			return r.tv.RenderMath(cx, b, true)
		}
		return core.Text{Content: b.TeX, Style: r.style(PartMathBlock)}
	case Custom:
		if r.tv.RenderCustom != nil {
			return r.tv.RenderCustom(cx, b)
		}
		switch {
		case len(b.Children) > 0:
			return core.View{Style: r.style(PartCustom), Children: r.blocks(cx, b.Children, depth)}
		case len(b.Content) > 0:
			return core.Text{Spans: r.spans(nil, b.Content, core.Style{}, nil), Style: r.style(PartCustom)}
		}
		return core.Text{Content: b.Text, Style: r.style(PartCustom)}
	}
	return nil
}

// details shows a disclosure: its summary, which a press opens and
// closes, and its blocks, made each time it opens.
func (r *renderer) details(cx *core.Scope, d Details, depth int) core.Node {
	open := core.NewSignal(cx, d.Open)
	return core.View{Style: r.style(PartDetails), Children: []core.Node{
		core.View{Style: r.style(PartSummary), Role: core.RoleButton, OnPress: func() { open.Set(!open.Get()) },
			Children: []core.Node{
				core.Text{ContentFunc: func() string {
					if open.Get() {
						return "▾"
					}
					return "▸"
				}, Style: r.style(PartSummaryMarker)},
				core.Text{Spans: r.spans(nil, d.Summary, core.Style{}, nil), Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
			}},
		core.Show{When: open.Get, Then: func(cx *core.Scope) core.Node {
			return core.Group(r.blocks(cx, d.Children, depth))
		}},
	}}
}

var bullets = [...]string{"•", "◦", "▪"}

func (r *renderer) list(cx *core.Scope, l List, depth int) core.Node {
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
			marker = core.View{Style: r.style(parts...), Role: core.RoleCheckbox, Checked: func() bool { return done },
				Children: []core.Node{core.Text{Content: check}}}
		case l.Ordered:
			marker = core.Text{Content: strconv.Itoa(l.Start+i) + ".", Style: r.style(PartListMarker)}
		default:
			marker = core.Text{Content: bullets[depth%len(bullets)], Style: r.style(PartListMarker)}
		}
		content := []Part{PartListItemContent}
		if it.Task && it.Done {
			content = append(content, PartTaskDone)
		}
		items[i] = core.View{Style: r.style(PartListItem), Children: []core.Node{
			marker,
			core.View{Style: r.style(content...), Children: r.blocks(cx, it.Children, depth+1)},
		}}
	}
	return core.View{Style: r.style(PartList, kind), Children: items}
}

func (r *renderer) code(cx *core.Scope, c CodeBlock) core.Node {
	if r.tv.RenderCodeBlock != nil {
		return r.tv.RenderCodeBlock(cx, c)
	}
	var highlights []Highlight
	if r.tv.Highlighter != nil {
		highlights = r.tv.Highlighter(c)
	}
	children := []core.Node{core.Scroll{Horizontal: true, Style: r.style(PartCodeBlockScroll), Children: []core.Node{
		core.Text{Spans: codeSpans(c.Code, highlights), Style: r.style(PartCodeBlockCode)},
	}}}
	if r.tv.CodeBlockActions != nil {
		children = append(children, core.View{
			Style:    core.Style{Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0)}.Merge(r.style(PartCodeBlockHeader)),
			Children: []core.Node{r.tv.CodeBlockActions(cx, c)},
		})
	}
	return core.View{Style: r.style(PartCodeBlock), Children: children}
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
		out = append(out, core.Span{Content: code[h.Start:h.End], Style: h.Style})
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
func (r *renderer) table(t Table) core.Node {
	cols := t.Columns()
	cell := r.style(PartTableCell)
	em := cell.FontSize
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
		children = append(children, core.View{Style: r.style(rowParts...).Merge(core.Style{GridRow: line,
			GridColumn: core.GridLines{Start: core.GridLine(1), End: core.GridLine(-1)}})})
		cs := r.style(cellParts...)
		col := 0
		for _, c := range row {
			s := cs
			if col < len(t.Align) {
				s.TextAlign = [...]core.TextAlign{AlignNone: s.TextAlign, AlignLeft: core.TextAlignStart,
					AlignCenter: core.TextAlignCenter, AlignRight: core.TextAlignEnd}[t.Align[col]]
			}
			s.GridRow = line
			s.GridColumn = core.GridLines{Start: core.GridLine(col + 1), End: core.GridSpan(c.span())}
			children = append(children, core.Text{Spans: r.spans(nil, c.Content, core.Style{}, nil), Style: s})
			col += c.span()
		}
	}
	return core.View{Style: r.style(PartTable), Children: []core.Node{
		core.Scroll{Horizontal: true, Style: r.style(PartTableScroll), Children: []core.Node{
			core.View{Style: core.Style{Display: core.DisplayGrid, GridTemplateColumns: core.GridTracks(tracks.String()),
				Width: core.Pct(100), MinWidth: core.Pt(core.Points(total))}, Children: children},
		}},
	}}
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
		rows[i] = core.View{Style: r.style(PartFootnote), Children: []core.Node{
			core.Text{Content: strconv.Itoa(f.Index) + ".", Style: r.style(PartFootnoteLabel)},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: r.blocks(cx, f.Children, 0)},
		}}
	}
	return core.View{Style: r.style(PartFootnotes), Children: rows}
}

// spans appends content's spans to out, each styled over style and
// pressing link, the link around it, if any.
func (r *renderer) spans(out []core.Span, content []Inline, style core.Style, link func()) []core.Span {
	for _, in := range content {
		switch in := in.(type) {
		case Text:
			out = append(out, core.Span{Content: in.Text, Style: style, OnPress: link})
		case Strong:
			out = r.spans(out, in.Content, style.Merge(r.style(PartStrong)), link)
		case Emphasis:
			out = r.spans(out, in.Content, style.Merge(r.style(PartEmphasis)), link)
		case Strikethrough:
			out = r.spans(out, in.Content, style.Merge(r.style(PartStrikethrough)), link)
		case Underline:
			out = r.spans(out, in.Content, style.Merge(r.style(PartUnderline)), link)
		case Mark:
			s := style.Merge(r.style(PartMark))
			if in.Color != 0 {
				s.Background = in.Color
			}
			out = r.spans(out, in.Content, s, link)
		case Superscript:
			out = r.spans(out, in.Content, style.Merge(r.style(PartSuperscript)), link)
		case Subscript:
			out = r.spans(out, in.Content, style.Merge(r.style(PartSubscript)), link)
		case Small:
			out = r.spans(out, in.Content, style.Merge(r.style(PartSmall)), link)
		case Code:
			out = append(out, core.Span{Content: in.Code, Style: style.Merge(r.style(PartCode)), OnPress: link})
		case Keyboard:
			out = append(out, core.Span{Content: in.Text, Style: style.Merge(r.style(PartKeyboard)), OnPress: link})
		case Math:
			if render := r.tv.RenderMath; render != nil {
				out = append(out, core.Span{Attachment: core.Func(func(cx *core.Scope) core.Node { return render(cx, in, false) })})
			} else {
				out = append(out, core.Span{Content: in.TeX, Style: style.Merge(r.style(PartMath)), OnPress: link})
			}
		case Link:
			out = r.spans(out, in.Content, style.Merge(r.style(PartLink)), r.press(in.URL))
		case Image:
			out = append(out, core.Span{Attachment: r.inlineImage(in, style, link)})
		case Break:
			out = append(out, core.Span{Content: "\n", Style: style})
		case FootnoteReference:
			out = append(out, core.Span{Content: "[" + strconv.Itoa(in.Index) + "]",
				Style: style.Merge(r.style(PartFootnoteReference)), OnPress: r.press("#footnote-" + in.Label)})
		case Custom:
			if r.tv.CustomSpan != nil {
				s := r.tv.CustomSpan(in)
				s.Style = style.Merge(s.Style)
				out = append(out, s)
			} else if render := r.tv.RenderCustom; render != nil {
				out = append(out, core.Span{Attachment: core.Func(func(cx *core.Scope) core.Node { return render(cx, in) })})
			} else {
				var b strings.Builder
				in.write(&b)
				out = append(out, core.Span{Content: b.String(), Style: style.Merge(r.style(PartCustom)), OnPress: link})
			}
		}
	}
	return out
}

// press returns what reports a press on a link to url, if anything
// listens.
func (r *renderer) press(url string) func() {
	if r.tv.OnLinkPress == nil {
		return nil
	}
	return func() { r.tv.OnLinkPress(url) }
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
func (r *renderer) inlineImage(im Image, style core.Style, link func()) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		if r.tv.RenderImage != nil {
			return r.tv.RenderImage(cx, im)
		}
		text := r.style(PartRoot, PartParagraph).Merge(style)
		line := text.LineHeight
		if size := text.FontSize; line == 0 && size > 0 {
			line = size * 1.2
		} else if line == 0 {
			line = core.DefaultFontSize * 1.2
		}
		natural := core.NewSignal(cx, core.Size{})
		return core.Image{
			Source:  core.Src(im.Src),
			Label:   im.Alt,
			OnLoad:  natural.Set,
			OnPress: link,
			StyleFunc: func() core.Style {
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
	if r.tv.RenderImage != nil {
		return r.tv.RenderImage(cx, im)
	}
	natural := core.NewSignal(cx, core.Size{})
	style := r.style(PartImage)
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
			return s.Merge(style)
		},
		OnPress: r.press(url),
	}
}
