package core

import (
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// selectionNode holds a View's or Text's semantic selection metadata.
type selectionNode struct {
	block *SelectionBlock
	fn    func() *SelectionBlock
}

func (s *selectionNode) resolve() *SelectionBlock {
	if s == nil {
		return nil
	}
	if s.fn != nil {
		return s.fn()
	}
	return s.block
}

func setSelectionBlock(n *node, block *SelectionBlock, fn func() *SelectionBlock) {
	if block == nil && fn == nil {
		n.selection = nil
		return
	}
	if n.selection == nil {
		n.selection = &selectionNode{}
	}
	n.selection.block, n.selection.fn = block, fn
}

// selNode is one node in the merged selection tree built before serializing to Markdown and HTML.
type selNode struct {
	owner    *node
	block    SelectionBlock
	inlines  []SelectionInline
	partial  bool
	sameRow  bool
	children []*selNode
}

func selNodeFromBlock(b SelectionBlock) *selNode {
	if b.Kind == SelectionBlockIgnore {
		return nil
	}
	n := &selNode{block: b, inlines: b.Inlines}
	for _, c := range b.Children {
		if cn := selNodeFromBlock(c); cn != nil {
			n.children = append(n.children, cn)
		}
	}
	return n
}

func isContainerBlock(k SelectionBlockKind) bool {
	switch k {
	case SelectionBlockQuote, SelectionBlockList, SelectionBlockListItem,
		SelectionBlockTable, SelectionBlockDetails, SelectionBlockFootnote:
		return true
	}
	return false
}

// formatTextSelection serializes the selected ranges of area into Markdown and HTML, preserving the block and inline structure of TextViews and styled texts.
func (t *Tree) formatTextSelection(area *node, plain string, ranges []driver.SelectedRange) (md, htmlOut string) {
	if plain == "" && len(ranges) == 0 {
		return "", ""
	}
	if len(ranges) == 0 {
		return plain, plainTextToHTML(plain)
	}
	byNode := make(map[*node]driver.SelectedRange, len(ranges))
	var first, last *node
	for _, r := range ranges {
		if r.End <= r.Start {
			continue
		}
		nd := t.node(r.ID)
		if nd == nil {
			continue
		}
		byNode[nd] = r
		if first == nil {
			first = nd
		}
		last = nd
	}
	if len(byNode) == 0 {
		if plain == "" {
			return "", ""
		}
		return plain, plainTextToHTML(plain)
	}

	root := &selNode{}
	var prevAutoFrame Bounds
	hasPrevAuto := false

	// Collect ancestor containers between area (exclusive) and nd (exclusive). Returns ok=false if any ancestor is marked SelectionBlockIgnore.
	ancestorsOf := func(nd *node) ([]*node, []SelectionBlock, bool) {
		var nodes []*node
		var blocks []SelectionBlock
		for p := nd.parent; p != nil && p != area; p = p.parent {
			sb := p.selection.resolve()
			if sb == nil {
				continue
			}
			if sb.Kind == SelectionBlockIgnore {
				return nil, nil, false
			}
			if isContainerBlock(sb.Kind) {
				nodes = append(nodes, p)
				blocks = append(blocks, *sb)
			}
		}
		slices.Reverse(nodes)
		slices.Reverse(blocks)
		return nodes, blocks, true
	}

	insert := func(ancNodes []*node, ancBlocks []SelectionBlock, leaf *selNode) {
		cur := root
		for i, an := range ancNodes {
			var found *selNode
			for _, ch := range cur.children {
				if ch.owner == an {
					found = ch
					break
				}
			}
			if found == nil {
				found = &selNode{owner: an, block: ancBlocks[i]}
				cur.children = append(cur.children, found)
			}
			cur = found
		}
		cur.children = append(cur.children, leaf)
	}

	// Walk area in document order between first and last selected nodes, picking up selected texts and any horizontal rules between them.
	inSpan := false
	done := false
	var walk func(nd *node)
	walk = func(nd *node) {
		if done || nd == nil || nd.deleted {
			return
		}
		if nd == first {
			inSpan = true
		}
		if r, ok := byNode[nd]; ok {
			if ancNodes, ancBlocks, keep := ancestorsOf(nd); keep {
				sb := nd.selection.resolve()
				if sb == nil || sb.Kind != SelectionBlockIgnore {
					leaf := t.buildSelectedLeaf(nd, sb, r, len(ancNodes) == 0, &prevAutoFrame, &hasPrevAuto)
					if leaf != nil {
						insert(ancNodes, ancBlocks, leaf)
					}
				}
			}
		} else if inSpan && nd != last {
			if sb := nd.selection.resolve(); sb != nil && sb.Kind == SelectionBlockRule {
				if ancNodes, ancBlocks, keep := ancestorsOf(nd); keep {
					hasPrevAuto = false
					insert(ancNodes, ancBlocks, &selNode{owner: nd, block: *sb})
				}
			}
		}
		if nd == last {
			done = true
			return
		}
		for _, c := range nd.children {
			walk(c)
			if done {
				return
			}
		}
	}
	walk(area)

	// If every selected node was decorative/ignored (e.g. only a bullet marker or alert title was selected), fall back to the reported plain text.
	if len(root.children) == 0 {
		if plain == "" {
			return "", ""
		}
		return plain, plainTextToHTML(plain)
	}
	return serializeSelection(root.children)
}

func (t *Tree) buildSelectedLeaf(nd *node, sb *SelectionBlock, r driver.SelectedRange, topLevel bool, prevAutoFrame *Bounds, hasPrevAuto *bool) *selNode {
	var blk SelectionBlock
	if sb != nil {
		blk = *sb
	}
	var src []SelectionInline
	switch {
	case len(blk.Inlines) > 0:
		src = blk.Inlines
	case blk.Kind == SelectionBlockCode:
		src = []SelectionInline{{Kind: SelectionInlineText, Text: nodePlainText(nd)}}
	case blk.Kind == SelectionBlockMath:
		text := nodePlainText(nd)
		if blk.TeX != "" && text == string(driver.AttachmentRune) {
			src = []SelectionInline{{Kind: SelectionInlineMath, Text: blk.TeX, UTF16Len: 1}}
		} else {
			src = []SelectionInline{{Kind: SelectionInlineMath, Text: text}}
		}
	default:
		src = inferredInlinesFromText(nd.text)
		if blk.Kind == SelectionBlockAuto && nd.props.Role == RoleHeader && nd.props.Level >= 1 && nd.props.Level <= 6 {
			blk.Kind = SelectionBlockHeading
			blk.Level = nd.props.Level
		}
	}
	visibleEnd := inlinesVisibleEndUTF16(src)
	sliced := sliceSelectionInlines(src, r.Start, r.End)
	if len(sliced) == 0 {
		return nil
	}
	partial := r.Start > 0 || r.End < visibleEnd
	sameRow := false
	if blk.Kind == SelectionBlockAuto && topLevel {
		frame := t.surfaceBounds(nd)
		if *hasPrevAuto {
			sameRow = prevAutoFrame.Origin.Y < frame.Origin.Y+frame.Size.Height &&
				frame.Origin.Y < prevAutoFrame.Origin.Y+prevAutoFrame.Size.Height
		}
		*prevAutoFrame = frame
		*hasPrevAuto = true
	} else {
		*hasPrevAuto = false
	}
	return &selNode{owner: nd, block: blk, inlines: sliced, partial: partial, sameRow: sameRow}
}

func nodePlainText(nd *node) string {
	if nd == nil || nd.text == nil {
		return ""
	}
	if nd.text.text.Text != "" {
		return nd.text.text.Text
	}
	return nd.text.content
}

// inferredInlinesFromText builds SelectionInlines from a plain core.Text's prepared AttributedText (respecting hidden spans, attachment copy texts, and styled spans over the text's base style).
func inferredInlinesFromText(tn *textNode) []SelectionInline {
	if tn == nil {
		return nil
	}
	text := tn.text.Text
	spans := tn.text.Spans
	base := tn.text.Style
	if text == "" {
		text = tn.content
		spans = nil
	}
	if text == "" {
		return nil
	}
	if len(spans) == 0 {
		return []SelectionInline{{Kind: SelectionInlineText, Text: text}}
	}

	var out []SelectionInline
	emitPlain := func(s string) {
		if s == "" {
			return
		}
		out = append(out, SelectionInline{Kind: SelectionInlineText, Text: s})
	}
	emitStyled := func(s string, sp driver.TextSpan) {
		if s == "" {
			return
		}
		if sp.Hidden {
			out = append(out, SelectionInline{Kind: SelectionInlineSkip, UTF16Len: selectionUTF16Len(s)})
			return
		}
		// Attachment placeholder(s).
		if strings.ContainsRune(s, driver.AttachmentRune) {
			attLen := len(string(driver.AttachmentRune))
			for s != "" {
				idx := strings.IndexRune(s, driver.AttachmentRune)
				if idx < 0 {
					out = append(out, wrapSpanStyle(SelectionInline{Kind: SelectionInlineText, Text: s}, sp.Style, base))
					return
				}
				if idx > 0 {
					out = append(out, wrapSpanStyle(SelectionInline{Kind: SelectionInlineText, Text: s[:idx]}, sp.Style, base))
				}
				if sp.Copy != "" {
					out = append(out, SelectionInline{Kind: SelectionInlineText, Text: sp.Copy, UTF16Len: 1})
				} else {
					out = append(out, SelectionInline{Kind: SelectionInlineSkip, UTF16Len: 1})
				}
				s = s[idx+attLen:]
			}
			return
		}
		out = append(out, wrapSpanStyle(SelectionInline{Kind: SelectionInlineText, Text: s}, sp.Style, base))
	}

	at := 0
	for _, sp := range spans {
		start, end := max(sp.Start, at), min(sp.End, len(text))
		if start > at {
			emitPlain(text[at:start])
		}
		if end > start {
			emitStyled(text[start:end], sp)
			at = end
		}
	}
	if at < len(text) {
		emitPlain(text[at:])
	}
	return out
}

func wrapSpanStyle(leaf SelectionInline, st, base driver.TextStyle) SelectionInline {
	if st.Font.Family == FontMonospace && base.Font.Family != FontMonospace {
		leaf.Kind = SelectionInlineCode
		return leaf
	}
	node := leaf
	if st.Decoration&driver.TextDecorationUnderline != 0 && base.Decoration&driver.TextDecorationUnderline == 0 {
		node = SelectionInline{Kind: SelectionInlineUnderline, Children: []SelectionInline{node}}
	}
	if st.Decoration&driver.TextDecorationLineThrough != 0 && base.Decoration&driver.TextDecorationLineThrough == 0 {
		node = SelectionInline{Kind: SelectionInlineStrikethrough, Children: []SelectionInline{node}}
	}
	if st.Font.Italic && !base.Font.Italic {
		node = SelectionInline{Kind: SelectionInlineEmphasis, Children: []SelectionInline{node}}
	}
	if st.Font.Weight >= driver.FontWeightBold && base.Font.Weight < driver.FontWeightBold {
		node = SelectionInline{Kind: SelectionInlineStrong, Children: []SelectionInline{node}}
	}
	return node
}

// --- UTF-16 offset helpers and inline slicing ---

func selectionUTF16Len(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

func sliceUTF16(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end <= start || s == "" {
		return ""
	}
	fromByte, toByte := -1, len(s)
	u := 0
	for i, r := range s {
		if fromByte < 0 && u >= start {
			fromByte = i
		}
		if u >= end {
			toByte = i
			break
		}
		if r > 0xFFFF {
			u += 2
		} else {
			u++
		}
	}
	if fromByte < 0 {
		return ""
	}
	if toByte < fromByte {
		return ""
	}
	return s[fromByte:toByte]
}

func inlineUTF16Len(in SelectionInline) int {
	switch in.Kind {
	case SelectionInlineStrong, SelectionInlineEmphasis, SelectionInlineStrikethrough,
		SelectionInlineUnderline, SelectionInlineMark, SelectionInlineSuperscript,
		SelectionInlineSubscript, SelectionInlineSmall, SelectionInlineLink:
		return inlinesUTF16Len(in.Children)
	case SelectionInlineImage, SelectionInlineBreak:
		if in.UTF16Len > 0 {
			return in.UTF16Len
		}
		return 1
	case SelectionInlineFootnoteRef:
		if in.UTF16Len > 0 {
			return in.UTF16Len
		}
		return selectionUTF16Len("[" + strconv.Itoa(in.Index) + "]")
	case SelectionInlineSkip:
		if in.UTF16Len > 0 {
			return in.UTF16Len
		}
		return selectionUTF16Len(in.Text)
	default:
		if in.UTF16Len > 0 {
			return in.UTF16Len
		}
		return selectionUTF16Len(in.Text)
	}
}

func inlinesUTF16Len(inlines []SelectionInline) int {
	n := 0
	for _, in := range inlines {
		n += inlineUTF16Len(in)
	}
	return n
}

// inlinesVisibleEndUTF16 returns the UTF-16 offset after the last non-skip inline, so selecting up to before a trailing streaming caret counts as a whole block selection.
func inlinesVisibleEndUTF16(inlines []SelectionInline) int {
	pos, lastVisible := 0, 0
	var walk func([]SelectionInline)
	walk = func(list []SelectionInline) {
		for _, in := range list {
			switch in.Kind {
			case SelectionInlineStrong, SelectionInlineEmphasis, SelectionInlineStrikethrough,
				SelectionInlineUnderline, SelectionInlineMark, SelectionInlineSuperscript,
				SelectionInlineSubscript, SelectionInlineSmall, SelectionInlineLink:
				walk(in.Children)
			case SelectionInlineSkip:
				pos += inlineUTF16Len(in)
			default:
				pos += inlineUTF16Len(in)
				lastVisible = pos
			}
		}
	}
	walk(inlines)
	return lastVisible
}

func sliceSelectionInlines(inlines []SelectionInline, start, end int) []SelectionInline {
	if end <= start {
		return nil
	}
	pos := 0
	return sliceInlinesAt(inlines, &pos, start, end)
}

func sliceInlinesAt(inlines []SelectionInline, pos *int, start, end int) []SelectionInline {
	var out []SelectionInline
	for _, in := range inlines {
		if *pos >= end {
			break
		}
		switch in.Kind {
		case SelectionInlineStrong, SelectionInlineEmphasis, SelectionInlineStrikethrough,
			SelectionInlineUnderline, SelectionInlineMark, SelectionInlineSuperscript,
			SelectionInlineSubscript, SelectionInlineSmall, SelectionInlineLink:
			children := sliceInlinesAt(in.Children, pos, start, end)
			if len(children) > 0 {
				cp := in
				cp.Children = children
				out = append(out, cp)
			}
		case SelectionInlineSkip:
			*pos += inlineUTF16Len(in)
		case SelectionInlineImage, SelectionInlineBreak, SelectionInlineFootnoteRef:
			n := inlineUTF16Len(in)
			p := *pos
			*pos += n
			if p < end && p+n > start {
				out = append(out, in)
			}
		default:
			n := inlineUTF16Len(in)
			p := *pos
			*pos += n
			lo, hi := max(p, start), min(p+n, end)
			if lo >= hi {
				continue
			}
			cp := in
			if in.UTF16Len == 0 {
				cp.Text = sliceUTF16(in.Text, lo-p, hi-p)
				if cp.Text == "" {
					continue
				}
			}
			out = append(out, cp)
		}
	}
	return out
}

// --- Unified Markdown & HTML serialization ---

func serializeSelection(nodes []*selNode) (md, htmlOut string) {
	if len(nodes) == 0 {
		return "", ""
	}
	var mdb, htmlb strings.Builder
	writeMarkdownNodes(&mdb, nodes)
	writeHTMLNodes(&htmlb, nodes, true)
	return mdb.String(), htmlb.String()
}

// --- Markdown ---

var mdTextEscaper = strings.NewReplacer(
	`\`, `\\`,
	`*`, `\*`,
	`_`, `\_`,
	"`", "\\`",
	`[`, `\[`,
	`~`, `\~`,
)

func escapeMarkdownText(s string) string {
	return mdTextEscaper.Replace(s)
}

func writeMarkdownNodes(b *strings.Builder, nodes []*selNode) {
	for i := 0; i < len(nodes); {
		if i > 0 {
			b.WriteString("\n\n")
		}
		if nodes[i].block.Kind == SelectionBlockAuto {
			writeMarkdownInlines(b, nodes[i].inlines)
			j := i + 1
			for j < len(nodes) && nodes[j].block.Kind == SelectionBlockAuto && nodes[j].sameRow {
				b.WriteByte('\t')
				writeMarkdownInlines(b, nodes[j].inlines)
				j++
			}
			i = j
			continue
		}
		writeMarkdownNode(b, nodes[i])
		i++
	}
}

func writeMarkdownNode(b *strings.Builder, n *selNode) {
	switch n.block.Kind {
	case SelectionBlockAuto, SelectionBlockParagraph, SelectionBlockSummary:
		writeMarkdownInlines(b, n.inlines)
	case SelectionBlockHeading:
		level := min(max(n.block.Level, 1), 6)
		b.WriteString(strings.Repeat("#", level))
		b.WriteByte(' ')
		writeMarkdownInlines(b, n.inlines)
	case SelectionBlockCode:
		code := rawInlinesText(n.inlines)
		writeMarkdownCodeBlock(b, n.block.Language, n.block.Meta, code)
	case SelectionBlockMath:
		tex := rawInlinesText(n.inlines)
		if tex == "" {
			tex = n.block.TeX
		}
		b.WriteString("$$\n")
		b.WriteString(tex)
		b.WriteString("\n$$")
	case SelectionBlockRule:
		b.WriteString("---")
	case SelectionBlockQuote:
		var inner strings.Builder
		writeMarkdownNodes(&inner, n.children)
		if n.block.Alert != "" {
			b.WriteString("> [!")
			b.WriteString(n.block.Alert)
			b.WriteByte(']')
			if inner.Len() > 0 {
				b.WriteByte('\n')
			}
		}
		if inner.Len() > 0 {
			lines := strings.Split(inner.String(), "\n")
			for i, line := range lines {
				if i > 0 {
					b.WriteByte('\n')
				}
				if line == "" {
					b.WriteByte('>')
				} else {
					b.WriteString("> ")
					b.WriteString(line)
				}
			}
		}
	case SelectionBlockList:
		sep := "\n"
		if n.block.Loose {
			sep = "\n\n"
		}
		start := max(n.block.Start, 1)
		for i, item := range n.children {
			if i > 0 {
				b.WriteString(sep)
			}
			var prefix string
			switch {
			case item.block.Task && item.block.Done:
				prefix = "- [x] "
			case item.block.Task:
				prefix = "- [ ] "
			case n.block.Ordered:
				prefix = strconv.Itoa(start+item.block.Index) + ". "
			default:
				prefix = "- "
			}
			var inner strings.Builder
			for j, c := range item.children {
				if j > 0 {
					if c.block.Kind == SelectionBlockList && !n.block.Loose {
						inner.WriteByte('\n')
					} else {
						inner.WriteString("\n\n")
					}
				}
				writeMarkdownNode(&inner, c)
			}
			pad := strings.Repeat(" ", len(prefix))
			lines := strings.Split(inner.String(), "\n")
			for j, line := range lines {
				if j > 0 {
					b.WriteByte('\n')
				}
				switch {
				case j == 0:
					b.WriteString(prefix)
					b.WriteString(line)
				case line == "":
				default:
					b.WriteString(pad)
					b.WriteString(line)
				}
			}
		}
	case SelectionBlockTable:
		writeMarkdownTable(b, n)
	case SelectionBlockDetails:
		b.WriteString("<details>\n")
		bodyStart := 0
		if len(n.children) > 0 && n.children[0].block.Kind == SelectionBlockSummary {
			b.WriteString("<summary>")
			writeMarkdownInlines(b, n.children[0].inlines)
			b.WriteString("</summary>\n")
			bodyStart = 1
		}
		if bodyStart < len(n.children) {
			if bodyStart > 0 {
				b.WriteByte('\n')
			}
			writeMarkdownNodes(b, n.children[bodyStart:])
			b.WriteByte('\n')
		}
		b.WriteString("</details>")
	case SelectionBlockFootnote:
		label := n.block.Label
		if label == "" {
			label = strconv.Itoa(n.block.Index)
		}
		var inner strings.Builder
		writeMarkdownNodes(&inner, n.children)
		lines := strings.Split(inner.String(), "\n")
		for i, line := range lines {
			if i > 0 {
				b.WriteByte('\n')
			}
			switch {
			case i == 0:
				b.WriteString("[^")
				b.WriteString(label)
				b.WriteString("]: ")
				b.WriteString(line)
			case line == "":
			default:
				b.WriteString("    ")
				b.WriteString(line)
			}
		}
	default:
		writeMarkdownInlines(b, n.inlines)
	}
}

func writeMarkdownTable(b *strings.Builder, table *selNode) {
	rows := groupTableRows(table.children)
	if len(rows) == 0 {
		return
	}
	if len(rows) == 1 && len(rows[0].cells) == 1 {
		writeMarkdownInlines(b, rows[0].cells[0].inlines)
		return
	}
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r.cells))
	}
	writeRow := func(cells []string) {
		b.WriteString("| ")
		for i := range cols {
			if i > 0 {
				b.WriteString(" | ")
			}
			if i < len(cells) {
				b.WriteString(cells[i])
			}
		}
		b.WriteString(" |")
	}
	for ri, r := range rows {
		if ri > 0 {
			b.WriteByte('\n')
		}
		vals := make([]string, len(r.cells))
		for ci, c := range r.cells {
			var cb strings.Builder
			writeMarkdownInlines(&cb, c.inlines)
			s := strings.ReplaceAll(cb.String(), "|", `\|`)
			s = strings.ReplaceAll(s, "\n", "<br>")
			vals[ci] = s
		}
		writeRow(vals)
		if ri == 0 {
			b.WriteByte('\n')
			delim := make([]string, cols)
			for ci := range cols {
				var align SelectionAlign
				if ci < len(r.cells) && r.cells[ci].block.Col < len(table.block.Align) {
					align = table.block.Align[r.cells[ci].block.Col]
				} else if ci < len(table.block.Align) {
					align = table.block.Align[ci]
				}
				switch align {
				case SelectionAlignLeft:
					delim[ci] = ":---"
				case SelectionAlignCenter:
					delim[ci] = ":---:"
				case SelectionAlignRight:
					delim[ci] = "---:"
				default:
					delim[ci] = "---"
				}
			}
			writeRow(delim)
		}
	}
}

func writeMarkdownCodeBlock(b *strings.Builder, lang, meta, code string) {
	fenceLen := max(3, maxBacktickRun(code)+1)
	fence := strings.Repeat("`", fenceLen)
	b.WriteString(fence)
	if lang != "" {
		b.WriteString(lang)
		if meta != "" {
			b.WriteByte(' ')
			b.WriteString(meta)
		}
	}
	b.WriteByte('\n')
	if code != "" {
		b.WriteString(code)
		b.WriteByte('\n')
	}
	b.WriteString(fence)
}

func maxBacktickRun(s string) int {
	best, cur := 0, 0
	for i := range len(s) {
		if s[i] == '`' {
			cur++
			best = max(best, cur)
		} else {
			cur = 0
		}
	}
	return best
}

func writeMarkdownCodeSpan(b *strings.Builder, code string) {
	fence := strings.Repeat("`", maxBacktickRun(code)+1)
	pad := code != "" && (code[0] == '`' || code[len(code)-1] == '`' ||
		(code[0] == ' ' && code[len(code)-1] == ' ' && strings.Trim(code, " ") != ""))
	b.WriteString(fence)
	if pad {
		b.WriteByte(' ')
	}
	b.WriteString(code)
	if pad {
		b.WriteByte(' ')
	}
	b.WriteString(fence)
}

func writeMarkdownInlines(b *strings.Builder, inlines []SelectionInline) {
	for _, in := range inlines {
		switch in.Kind {
		case SelectionInlineText:
			b.WriteString(escapeMarkdownText(in.Text))
		case SelectionInlineStrong:
			writeMarkdownDelimited(b, "**", "**", in.Children)
		case SelectionInlineEmphasis:
			writeMarkdownDelimited(b, "*", "*", in.Children)
		case SelectionInlineStrikethrough:
			writeMarkdownDelimited(b, "~~", "~~", in.Children)
		case SelectionInlineUnderline:
			writeMarkdownHTMLTag(b, "u", "", in.Children)
		case SelectionInlineMark:
			attrs := ""
			if in.Color != 0 {
				attrs = ` style="background-color:` + colorHexCSS(in.Color) + `"`
			}
			writeMarkdownHTMLTag(b, "mark", attrs, in.Children)
		case SelectionInlineSuperscript:
			writeMarkdownHTMLTag(b, "sup", "", in.Children)
		case SelectionInlineSubscript:
			writeMarkdownHTMLTag(b, "sub", "", in.Children)
		case SelectionInlineSmall:
			writeMarkdownHTMLTag(b, "small", "", in.Children)
		case SelectionInlineCode:
			writeMarkdownCodeSpan(b, in.Text)
		case SelectionInlineKeyboard:
			b.WriteString("<kbd>")
			b.WriteString(html.EscapeString(in.Text))
			b.WriteString("</kbd>")
		case SelectionInlineMath:
			b.WriteByte('$')
			b.WriteString(in.Text)
			b.WriteByte('$')
		case SelectionInlineLink:
			var inner strings.Builder
			writeMarkdownInlines(&inner, in.Children)
			if inner.Len() == 0 {
				continue
			}
			b.WriteByte('[')
			b.WriteString(inner.String())
			b.WriteString("](")
			writeMarkdownLinkTarget(b, in.URL, in.Title)
			b.WriteByte(')')
		case SelectionInlineImage:
			b.WriteString("![")
			b.WriteString(escapeMarkdownText(in.Text))
			b.WriteString("](")
			writeMarkdownLinkTarget(b, in.URL, in.Title)
			b.WriteByte(')')
		case SelectionInlineBreak:
			b.WriteString("  \n")
		case SelectionInlineFootnoteRef:
			label := in.Label
			if label == "" {
				label = strconv.Itoa(in.Index)
			}
			b.WriteString("[^")
			b.WriteString(label)
			b.WriteByte(']')
		case SelectionInlineSkip:
		}
	}
}

// writeMarkdownDelimited wraps Children in open/close Markdown emphasis delimiters, keeping leading and trailing spaces outside the delimiters so partial selections remain valid CommonMark flanking runs.
func writeMarkdownDelimited(b *strings.Builder, open, close string, children []SelectionInline) {
	var inner strings.Builder
	writeMarkdownInlines(&inner, children)
	s := inner.String()
	if s == "" {
		return
	}
	leadEnd := 0
	for leadEnd < len(s) && (s[leadEnd] == ' ' || s[leadEnd] == '\t' || s[leadEnd] == '\n') {
		leadEnd++
	}
	if leadEnd == len(s) {
		b.WriteString(s)
		return
	}
	trailStart := len(s)
	for trailStart > leadEnd && (s[trailStart-1] == ' ' || s[trailStart-1] == '\t' || s[trailStart-1] == '\n') {
		trailStart--
	}
	b.WriteString(s[:leadEnd])
	b.WriteString(open)
	b.WriteString(s[leadEnd:trailStart])
	b.WriteString(close)
	b.WriteString(s[trailStart:])
}

func writeMarkdownHTMLTag(b *strings.Builder, tag, attrs string, children []SelectionInline) {
	var inner strings.Builder
	writeMarkdownInlines(&inner, children)
	if inner.Len() == 0 {
		return
	}
	b.WriteByte('<')
	b.WriteString(tag)
	b.WriteString(attrs)
	b.WriteByte('>')
	b.WriteString(inner.String())
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteByte('>')
}

func writeMarkdownLinkTarget(b *strings.Builder, u, title string) {
	if needsAngleBrackets(u) {
		b.WriteByte('<')
		b.WriteString(u)
		b.WriteByte('>')
	} else {
		b.WriteString(u)
	}
	if title != "" {
		b.WriteString(` "`)
		for i := range len(title) {
			if title[i] == '"' || title[i] == '\\' {
				b.WriteByte('\\')
			}
			b.WriteByte(title[i])
		}
		b.WriteByte('"')
	}
}

func needsAngleBrackets(u string) bool {
	depth := 0
	for i := range len(u) {
		switch u[i] {
		case ' ', '\t', '\n', '\r':
			return true
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return true
			}
			depth--
		}
	}
	return depth != 0
}

func rawInlinesText(inlines []SelectionInline) string {
	var b strings.Builder
	var walk func([]SelectionInline)
	walk = func(list []SelectionInline) {
		for _, in := range list {
			switch in.Kind {
			case SelectionInlineSkip:
			case SelectionInlineBreak:
				b.WriteByte('\n')
			case SelectionInlineText, SelectionInlineCode, SelectionInlineKeyboard, SelectionInlineMath:
				b.WriteString(in.Text)
			default:
				walk(in.Children)
			}
		}
	}
	walk(inlines)
	return b.String()
}

// --- HTML ---

func plainTextToHTML(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) == 1 {
		return html.EscapeString(s)
	}
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(line))
		b.WriteString("</p>")
	}
	return b.String()
}

func writeHTMLNodes(b *strings.Builder, nodes []*selNode, topLevel bool) {
	if topLevel {
		// Single partial paragraph stays inline without a <p> wrapper.
		if len(nodes) == 1 && nodes[0].block.Kind == SelectionBlockParagraph && nodes[0].partial {
			writeHTMLInlines(b, nodes[0].inlines)
			return
		}
		// Plain core.Text(s) on a single row stay inline without a <p> wrapper.
		if allAutoOneRow(nodes) {
			for i, n := range nodes {
				if i > 0 {
					b.WriteByte('\t')
				}
				writeHTMLInlines(b, n.inlines)
			}
			return
		}
	}
	first := true
	for i := 0; i < len(nodes); {
		if !first {
			b.WriteByte('\n')
		}
		first = false
		if nodes[i].block.Kind == SelectionBlockAuto {
			b.WriteString("<p>")
			writeHTMLInlines(b, nodes[i].inlines)
			j := i + 1
			for j < len(nodes) && nodes[j].block.Kind == SelectionBlockAuto && nodes[j].sameRow {
				b.WriteByte('\t')
				writeHTMLInlines(b, nodes[j].inlines)
				j++
			}
			b.WriteString("</p>")
			i = j
			continue
		}
		writeHTMLNode(b, nodes[i])
		i++
	}
}

func allAutoOneRow(nodes []*selNode) bool {
	if len(nodes) == 0 {
		return false
	}
	for i, n := range nodes {
		if n.block.Kind != SelectionBlockAuto {
			return false
		}
		if i > 0 && !n.sameRow {
			return false
		}
	}
	return true
}

func writeHTMLNode(b *strings.Builder, n *selNode) {
	switch n.block.Kind {
	case SelectionBlockAuto, SelectionBlockParagraph:
		b.WriteString("<p>")
		writeHTMLInlines(b, n.inlines)
		b.WriteString("</p>")
	case SelectionBlockHeading:
		level := min(max(n.block.Level, 1), 6)
		tag := "h" + strconv.Itoa(level)
		b.WriteByte('<')
		b.WriteString(tag)
		b.WriteByte('>')
		writeHTMLInlines(b, n.inlines)
		b.WriteString("</")
		b.WriteString(tag)
		b.WriteByte('>')
	case SelectionBlockCode:
		code := rawInlinesText(n.inlines)
		b.WriteString("<pre><code")
		if n.block.Language != "" {
			b.WriteString(` class="language-`)
			b.WriteString(html.EscapeString(n.block.Language))
			b.WriteByte('"')
		}
		b.WriteByte('>')
		b.WriteString(html.EscapeString(code))
		b.WriteString("</code></pre>")
	case SelectionBlockMath:
		tex := rawInlinesText(n.inlines)
		if tex == "" {
			tex = n.block.TeX
		}
		b.WriteString(`<math display="block"><semantics><annotation encoding="application/x-tex">`)
		b.WriteString(html.EscapeString(tex))
		b.WriteString(`</annotation></semantics></math>`)
	case SelectionBlockRule:
		b.WriteString("<hr>")
	case SelectionBlockQuote:
		if n.block.Alert != "" {
			kind := strings.ToLower(n.block.Alert)
			title := n.block.Title
			if title == "" {
				title = defaultAlertTitle(n.block.Alert)
			}
			b.WriteString(`<div class="markdown-alert markdown-alert-`)
			b.WriteString(html.EscapeString(kind))
			b.WriteString("\">\n<p class=\"markdown-alert-title\">")
			b.WriteString(html.EscapeString(title))
			b.WriteString("</p>")
			if len(n.children) > 0 {
				b.WriteByte('\n')
				writeHTMLNodes(b, n.children, false)
			}
			b.WriteString("\n</div>")
			return
		}
		b.WriteString("<blockquote>\n")
		writeHTMLNodes(b, n.children, false)
		b.WriteString("\n</blockquote>")
	case SelectionBlockList:
		tag := "ul"
		if n.block.Ordered {
			tag = "ol"
		}
		b.WriteByte('<')
		b.WriteString(tag)
		if n.block.Ordered && len(n.children) > 0 {
			firstNum := max(n.block.Start, 1) + n.children[0].block.Index
			if firstNum != 1 {
				fmt.Fprintf(b, ` start="%d"`, firstNum)
			}
		}
		b.WriteString(">\n")
		for i, item := range n.children {
			if i > 0 {
				b.WriteByte('\n')
			}
			checkbox := ""
			if item.block.Task {
				if item.block.Done {
					checkbox = `<input type="checkbox" checked disabled> `
				} else {
					checkbox = `<input type="checkbox" disabled> `
				}
			}
			b.WriteString("<li>")
			if !n.block.Loose && len(item.children) == 1 && item.children[0].block.Kind == SelectionBlockParagraph {
				b.WriteString(checkbox)
				writeHTMLInlines(b, item.children[0].inlines)
			} else if !n.block.Loose && len(item.children) > 1 && item.children[0].block.Kind == SelectionBlockParagraph && item.children[1].block.Kind == SelectionBlockList {
				b.WriteString(checkbox)
				writeHTMLInlines(b, item.children[0].inlines)
				b.WriteByte('\n')
				writeHTMLNodes(b, item.children[1:], false)
				b.WriteByte('\n')
			} else if len(item.children) > 0 {
				b.WriteByte('\n')
				for j, c := range item.children {
					if j > 0 {
						b.WriteByte('\n')
					}
					if j == 0 && checkbox != "" && c.block.Kind == SelectionBlockParagraph {
						b.WriteString("<p>")
						b.WriteString(checkbox)
						writeHTMLInlines(b, c.inlines)
						b.WriteString("</p>")
						continue
					}
					writeHTMLNode(b, c)
				}
				b.WriteByte('\n')
			}
			b.WriteString("</li>")
		}
		b.WriteString("\n</")
		b.WriteString(tag)
		b.WriteByte('>')
	case SelectionBlockTable:
		writeHTMLTable(b, n)
	case SelectionBlockDetails:
		b.WriteString("<details>\n")
		bodyStart := 0
		if len(n.children) > 0 && n.children[0].block.Kind == SelectionBlockSummary {
			b.WriteString("<summary>")
			writeHTMLInlines(b, n.children[0].inlines)
			b.WriteString("</summary>\n")
			bodyStart = 1
		}
		if bodyStart < len(n.children) {
			writeHTMLNodes(b, n.children[bodyStart:], false)
			b.WriteByte('\n')
		}
		b.WriteString("</details>")
	case SelectionBlockFootnote:
		label := n.block.Label
		if label == "" {
			label = strconv.Itoa(n.block.Index)
		}
		b.WriteString(`<div id="footnote-`)
		b.WriteString(html.EscapeString(label))
		b.WriteString("\">\n")
		writeHTMLNodes(b, n.children, false)
		b.WriteString("\n</div>")
	default:
		writeHTMLInlines(b, n.inlines)
	}
}

func writeHTMLTable(b *strings.Builder, table *selNode) {
	rows := groupTableRows(table.children)
	if len(rows) == 0 {
		return
	}
	if len(rows) == 1 && len(rows[0].cells) == 1 {
		writeHTMLInlines(b, rows[0].cells[0].inlines)
		return
	}
	var head, body []tableRowGroup
	for _, r := range rows {
		if r.header {
			head = append(head, r)
		} else {
			body = append(body, r)
		}
	}
	writeRows := func(list []tableRowGroup) {
		for _, r := range list {
			b.WriteString("<tr>")
			for _, c := range r.cells {
				tag := "td"
				if c.block.Header {
					tag = "th"
				}
				b.WriteByte('<')
				b.WriteString(tag)
				if c.block.ColSpan > 1 {
					fmt.Fprintf(b, ` colspan="%d"`, c.block.ColSpan)
				}
				if c.block.Col >= 0 && c.block.Col < len(table.block.Align) {
					switch table.block.Align[c.block.Col] {
					case SelectionAlignLeft:
						b.WriteString(` style="text-align:left"`)
					case SelectionAlignCenter:
						b.WriteString(` style="text-align:center"`)
					case SelectionAlignRight:
						b.WriteString(` style="text-align:right"`)
					}
				}
				b.WriteByte('>')
				writeHTMLInlines(b, c.inlines)
				b.WriteString("</")
				b.WriteString(tag)
				b.WriteByte('>')
			}
			b.WriteString("</tr>\n")
		}
	}
	b.WriteString("<table>\n")
	if len(head) > 0 {
		b.WriteString("<thead>\n")
		writeRows(head)
		b.WriteString("</thead>\n")
	}
	if len(body) > 0 {
		b.WriteString("<tbody>\n")
		writeRows(body)
		b.WriteString("</tbody>\n")
	}
	b.WriteString("</table>")
}

func writeHTMLInlines(b *strings.Builder, inlines []SelectionInline) {
	for _, in := range inlines {
		switch in.Kind {
		case SelectionInlineText:
			b.WriteString(html.EscapeString(in.Text))
		case SelectionInlineStrong:
			writeHTMLInlineTag(b, "strong", "", in.Children)
		case SelectionInlineEmphasis:
			writeHTMLInlineTag(b, "em", "", in.Children)
		case SelectionInlineStrikethrough:
			writeHTMLInlineTag(b, "del", "", in.Children)
		case SelectionInlineUnderline:
			writeHTMLInlineTag(b, "u", "", in.Children)
		case SelectionInlineMark:
			attrs := ""
			if in.Color != 0 {
				attrs = ` style="background-color:` + colorHexCSS(in.Color) + `"`
			}
			writeHTMLInlineTag(b, "mark", attrs, in.Children)
		case SelectionInlineSuperscript:
			writeHTMLInlineTag(b, "sup", "", in.Children)
		case SelectionInlineSubscript:
			writeHTMLInlineTag(b, "sub", "", in.Children)
		case SelectionInlineSmall:
			writeHTMLInlineTag(b, "small", "", in.Children)
		case SelectionInlineCode:
			b.WriteString("<code>")
			b.WriteString(html.EscapeString(in.Text))
			b.WriteString("</code>")
		case SelectionInlineKeyboard:
			b.WriteString("<kbd>")
			b.WriteString(html.EscapeString(in.Text))
			b.WriteString("</kbd>")
		case SelectionInlineMath:
			b.WriteString(`<math><semantics><annotation encoding="application/x-tex">`)
			b.WriteString(html.EscapeString(in.Text))
			b.WriteString(`</annotation></semantics></math>`)
		case SelectionInlineLink:
			var inner strings.Builder
			writeHTMLInlines(&inner, in.Children)
			if inner.Len() == 0 {
				continue
			}
			b.WriteString(`<a href="`)
			b.WriteString(html.EscapeString(in.URL))
			b.WriteByte('"')
			if in.Title != "" {
				b.WriteString(` title="`)
				b.WriteString(html.EscapeString(in.Title))
				b.WriteByte('"')
			}
			b.WriteByte('>')
			b.WriteString(inner.String())
			b.WriteString("</a>")
		case SelectionInlineImage:
			b.WriteString(`<img src="`)
			b.WriteString(html.EscapeString(in.URL))
			b.WriteString(`" alt="`)
			b.WriteString(html.EscapeString(in.Text))
			b.WriteByte('"')
			if in.Title != "" {
				b.WriteString(` title="`)
				b.WriteString(html.EscapeString(in.Title))
				b.WriteByte('"')
			}
			b.WriteByte('>')
		case SelectionInlineBreak:
			b.WriteString("<br>")
		case SelectionInlineFootnoteRef:
			label := in.Label
			if label == "" {
				label = strconv.Itoa(in.Index)
			}
			b.WriteString(`<sup><a href="#footnote-`)
			b.WriteString(html.EscapeString(label))
			b.WriteString(`">`)
			b.WriteString(strconv.Itoa(in.Index))
			b.WriteString(`</a></sup>`)
		case SelectionInlineSkip:
		}
	}
}

func writeHTMLInlineTag(b *strings.Builder, tag, attrs string, children []SelectionInline) {
	var inner strings.Builder
	writeHTMLInlines(&inner, children)
	if inner.Len() == 0 {
		return
	}
	b.WriteByte('<')
	b.WriteString(tag)
	b.WriteString(attrs)
	b.WriteByte('>')
	b.WriteString(inner.String())
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteByte('>')
}

type tableRowGroup struct {
	row    int
	header bool
	cells  []*selNode
}

func groupTableRows(cells []*selNode) []tableRowGroup {
	var groups []tableRowGroup
	index := map[int]int{}
	for _, c := range cells {
		if c.block.Kind != SelectionBlockTableCell {
			continue
		}
		ri, ok := index[c.block.Row]
		if !ok {
			ri = len(groups)
			index[c.block.Row] = ri
			groups = append(groups, tableRowGroup{row: c.block.Row, header: c.block.Header})
		}
		groups[ri].cells = append(groups[ri].cells, c)
		if c.block.Header {
			groups[ri].header = true
		}
	}
	slices.SortStableFunc(groups, func(a, b tableRowGroup) int { return a.row - b.row })
	for i := range groups {
		slices.SortStableFunc(groups[i].cells, func(a, b *selNode) int { return a.block.Col - b.block.Col })
	}
	return groups
}

func colorHexCSS(c Color) string {
	if c&0xff == 0xff {
		return fmt.Sprintf("#%06x", uint32(c>>8))
	}
	return fmt.Sprintf("#%08x", uint32(c))
}

func defaultAlertTitle(kind string) string {
	switch strings.ToUpper(kind) {
	case "NOTE":
		return "Note"
	case "TIP":
		return "Tip"
	case "IMPORTANT":
		return "Important"
	case "WARNING":
		return "Warning"
	case "CAUTION":
		return "Caution"
	}
	return kind
}
