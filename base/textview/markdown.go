package textview

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Extension is syntax of the app's that becomes Custom nodes named Name,
// which the TextView's RenderCustom (blocks) and CustomSpan (inline)
// show. Set one of Pattern, Language, Container or Tag.
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
	// Tag makes HTML elements of this name, in any case (<Callout>), custom
	// nodes: their attributes the node's, their text its Text, and what
	// they hold its blocks when they stand alone, else its inline content.
	// An element may close itself as JSX writes it (<Callout />).
	Tag string
}

// Parse reads Markdown into a document: CommonMark with GitHub's tables,
// task lists, strikethrough, autolinks, footnotes and alerts, formulas,
// and the syntax of extensions. HTML inside reads as ParseHTML reads it;
// the Markdown between an element's tags, on lines of their own or in a
// paragraph (<b>x *y*</b>), stays Markdown.
//
// Formulas are TeX between dollars: $x$ and $$x$$ in text, a block
// between $$ fences on lines of their own, or fenced code of language
// math, as GitHub shows it. Single dollars around a space or before a
// digit are prose ("costs $5 and $10"). TeX's own \(x\) and \[x\] are
// formulas too.
func Parse(markdown string, extensions ...Extension) Document {
	return ParseWith(markdown, Options{}, extensions...)
}

// Options say what a Markdown document reads beyond CommonMark and
// GitHub's syntax, as NSH's MarkdownOptions; the zero value is NSH's
// default.
type Options struct {
	// NoMath reads dollars and \( \[ as text and math fences as code.
	NoMath bool
	// NoHTML shows HTML in the Markdown as text.
	NoHTML bool
	// NoAlerts reads > [!NOTE] and its kin as plain quotations.
	NoAlerts bool
	// Frontmatter reads YAML front matter (--- … --- first) as a Custom
	// block "frontmatter" whose Text is the YAML, rather than a rule and
	// text.
	Frontmatter bool
	// MDX reads JSX elements as Custom nodes named by their tag, with
	// their attributes and content, and {expressions} as Custom nodes
	// "expression" whose Text is the expression; HTML is then read as
	// elements too.
	MDX bool
	// Streaming repairs the syntax the last block leaves open before
	// parsing (RepairStreaming), for a document still arriving.
	Streaming bool
}

// mdxExtensions are the extensions MDX adds: every element, and braces
// in text (mdxExpression). Built lazily so importing textview executes no
// regexp compilation at init time.
var (
	mdxOnce          sync.Once
	mdxExpressionVal *regexp.Regexp
	mdxExtensionsVal []Extension
)

func getMDX() (*regexp.Regexp, []Extension) {
	mdxOnce.Do(func() {
		mdxExpressionVal = regexp.MustCompile(`\{([^{}\n]*)\}`)
		mdxExtensionsVal = []Extension{{Tag: anyTag}, {Name: "expression", Pattern: mdxExpressionVal}}
	})
	return mdxExpressionVal, mdxExtensionsVal
}

// anyTag is the Tag of an extension that takes every element, named by
// its tag.
const anyTag = "*"

// ParseWith is Parse with options.
func ParseWith(markdown string, o Options, extensions ...Extension) Document {
	if o.Streaming {
		markdown = RepairStreaming(markdown)
	}
	var front *Custom
	if o.Frontmatter {
		if yaml, rest, ok := frontmatter(markdown); ok {
			front, markdown = &Custom{Name: "frontmatter", Text: yaml}, rest
		}
	}
	if o.MDX {
		_, mdxExt := getMDX()
		extensions = append(slices.Clip(extensions), mdxExt...)
	}
	if !o.NoMath {
		markdown = rewriteTeX(markdown)
	}
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	markdown = strings.ReplaceAll(markdown, "\r", "\n")

	containers := map[string]string{}
	for _, e := range extensions {
		if e.Container != "" {
			containers[e.Container] = e.Name
		}
	}

	p := &mdParser{
		ext:        extensions,
		opts:       o,
		containers: containers,
		linkDefs:   map[string]mdLinkDef{},
		fnDefs:     map[string][]string{},
		fnIndex:    map[string]int{},
		slugs:      slugs{},
	}
	lines := splitLines(markdown)
	rawBlocks := p.parseBlockLines(lines, true)

	var doc Document
	doc.Blocks = p.renderBlocks(rawBlocks)
	// Render referenced footnotes in order of first reference; footnotes may
	// themselves reference additional footnotes.
	for i := 0; i < len(p.fnOrder); i++ {
		label := p.fnOrder[i]
		fnLines := p.fnDefs[label]
		fnBlocks := p.parseBlockLines(fnLines, false)
		doc.Footnotes = append(doc.Footnotes, Footnote{
			Label:    label,
			Index:    i + 1,
			Children: p.renderBlocks(fnBlocks),
		})
	}
	if front != nil {
		doc.Blocks = append([]Block{*front}, doc.Blocks...)
	}
	return doc
}

// frontmatter splits YAML front matter off the start of markdown: a ---
// line, the YAML, and a --- or ... line.
func frontmatter(markdown string) (yaml, rest string, ok bool) {
	first, body, found := strings.Cut(markdown, "\n")
	if !found || strings.TrimRight(first, " \t\r") != "---" {
		return "", markdown, false
	}
	at := 0
	for at <= len(body) {
		line, next, more := strings.Cut(body[at:], "\n")
		if l := strings.TrimRight(line, " \t\r"); l == "---" || l == "..." {
			yaml = strings.TrimSuffix(body[:at], "\n")
			if !more {
				return yaml, "", true
			}
			return yaml, next, true
		}
		if !more {
			break
		}
		at += len(line) + 1
	}
	return "", markdown, false
}

// --- Internal Markdown AST ---

type mdBlockKind uint8

const (
	mdKindParagraph mdBlockKind = iota
	mdKindHeading
	mdKindRule
	mdKindFencedCode
	mdKindIndentedCode
	mdKindBlockquote
	mdKindList
	mdKindTable
	mdKindHTML
	mdKindMath
	mdKindContainer
)

type mdBlock struct {
	kind     mdBlockKind
	level    int
	info     string
	text     string
	lines    []string
	children []mdBlock
	ordered  bool
	start    int
	loose    bool
	items    []mdListItem
	align    []ColumnAlign
	head     []string
	rows     [][]string
	name     string
	attrs    map[string]string
}

type mdListItem struct {
	children []mdBlock
}

type mdLinkDef struct {
	url   string
	title string
}

type mdParser struct {
	ext        []Extension
	opts       Options
	containers map[string]string
	linkDefs   map[string]mdLinkDef
	fnDefs     map[string][]string
	fnIndex    map[string]int
	fnOrder    []string
	slugs      slugs
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

func leadingSpaces(line string) int {
	n := 0
	for n < len(line) && line[n] == ' ' {
		n++
	}
	return n
}

// leadingColumns returns the visual column width of leading spaces/tabs and
// the byte offset where non-whitespace begins.
func leadingColumns(line string) (cols, offset int) {
	for offset < len(line) {
		switch line[offset] {
		case ' ':
			cols++
			offset++
		case '\t':
			cols += 4 - (cols % 4)
			offset++
		default:
			return cols, offset
		}
	}
	return cols, offset
}

// stripColumns removes up to n visual columns of leading spaces/tabs.
func stripColumns(line string, n int) string {
	cols := 0
	i := 0
	for i < len(line) && cols < n {
		switch line[i] {
		case ' ':
			cols++
			i++
		case '\t':
			adv := 4 - (cols % 4)
			if cols+adv > n {
				// Partial tab expansion.
				rem := (cols + adv) - n
				return strings.Repeat(" ", rem) + line[i+1:]
			}
			cols += adv
			i++
		default:
			return line[i:]
		}
	}
	return line[i:]
}

func isBlankLine(line string) bool {
	return strings.Trim(line, " \t\r") == ""
}

// --- Block parser ---

func (p *mdParser) parseBlockLines(lines []string, topLevel bool) []mdBlock {
	var out []mdBlock
	i := 0
	for i < len(lines) {
		line := lines[i]
		if isBlankLine(line) {
			i++
			continue
		}
		cols, off := leadingColumns(line)

		// Indented code block (4+ columns).
		if cols >= 4 {
			var code []string
			for i < len(lines) {
				if isBlankLine(lines[i]) {
					code = append(code, "")
					i++
					continue
				}
				c, _ := leadingColumns(lines[i])
				if c < 4 {
					break
				}
				code = append(code, stripColumns(lines[i], 4))
				i++
			}
			for len(code) > 0 && code[len(code)-1] == "" {
				code = code[:len(code)-1]
			}
			out = append(out, mdBlock{kind: mdKindIndentedCode, text: strings.Join(code, "\n")})
			continue
		}

		// Custom container (:::name).
		if name, attrs, colons, ok := parseContainerOpen(line, p.containers); ok {
			i++
			var inner []string
			depth := 0
			for i < len(lines) {
				if _, _, subColons, subOK := parseContainerOpen(lines[i], p.containers); subOK {
					depth++
					_ = subColons
					inner = append(inner, lines[i])
					i++
					continue
				}
				if isContainerClose(lines[i], colons) {
					if depth == 0 {
						i++
						break
					}
					depth--
				} else if depth > 0 && isContainerClose(lines[i], 3) {
					depth--
				}
				inner = append(inner, lines[i])
				i++
			}
			out = append(out, mdBlock{
				kind:     mdKindContainer,
				name:     name,
				attrs:    attrs,
				children: p.parseBlockLines(inner, false),
			})
			continue
		}

		// Display math block ($$+ fence).
		if !p.opts.NoMath {
			if indent, fence, ok := parseMathBlockOpen(line); ok {
				i++
				var body []string
				for i < len(lines) {
					if isMathBlockClose(lines[i], fence) {
						i++
						break
					}
					body = append(body, stripColumns(lines[i], indent))
					i++
				}
				out = append(out, mdBlock{kind: mdKindMath, text: strings.Join(body, "\n")})
				continue
			}
		}

		// Footnote definition ([^label]: ...).
		if label, first, ok := parseFootnoteDef(line[off:]); ok {
			var fnLines []string
			if first != "" {
				fnLines = append(fnLines, first)
			}
			i++
			for i < len(lines) {
				if isBlankLine(lines[i]) {
					// Keep blank line only if followed by indented continuation.
					j := i
					for j < len(lines) && isBlankLine(lines[j]) {
						j++
					}
					if j < len(lines) {
						c, _ := leadingColumns(lines[j])
						if c >= 4 {
							for ; i < j; i++ {
								fnLines = append(fnLines, "")
							}
							continue
						}
					}
					break
				}
				c, _ := leadingColumns(lines[i])
				if c >= 4 {
					fnLines = append(fnLines, stripColumns(lines[i], 4))
					i++
					continue
				}
				// Lazy continuation of a single-line footnote if not starting another footnote/block.
				if len(fnLines) > 0 && fnLines[len(fnLines)-1] != "" && !p.startsBlock(lines, i, true) {
					fnLines = append(fnLines, strings.TrimLeft(lines[i], " \t"))
					i++
					continue
				}
				break
			}
			p.fnDefs[label] = fnLines
			continue
		}

		// Link reference definition ([label]: url "title").
		if label, def, consumed, ok := parseLinkRefDef(lines[i:]); ok {
			norm := normalizeLabel(label)
			if _, exists := p.linkDefs[norm]; !exists {
				p.linkDefs[norm] = def
			}
			i += consumed
			continue
		}

		// ATX heading (# .. ######).
		if level, text, ok := parseATXHeading(line[off:]); ok {
			out = append(out, mdBlock{kind: mdKindHeading, level: level, text: text})
			i++
			continue
		}

		// Fenced code block (``` or ~~~).
		if ch, fenceLen, info, ok := parseFencedCodeOpen(line[off:]); ok {
			i++
			var body []string
			for i < len(lines) {
				if isFencedCodeClose(lines[i], ch, fenceLen) {
					i++
					break
				}
				body = append(body, stripColumns(lines[i], cols))
				i++
			}
			out = append(out, mdBlock{kind: mdKindFencedCode, info: info, text: strings.Join(body, "\n")})
			continue
		}

		// Thematic break (---, ***, ___).
		if isThematicBreak(line[off:]) {
			out = append(out, mdBlock{kind: mdKindRule})
			i++
			continue
		}

		// Blockquote (>).
		if strings.HasPrefix(line[off:], ">") {
			var qLines []string
			for i < len(lines) {
				if isBlankLine(lines[i]) {
					break
				}
				c, o := leadingColumns(lines[i])
				if c < 4 && strings.HasPrefix(lines[i][o:], ">") {
					rest := lines[i][o+1:]
					if strings.HasPrefix(rest, " ") {
						rest = rest[1:]
					} else if strings.HasPrefix(rest, "\t") {
						rest = "   " + rest[1:]
					}
					qLines = append(qLines, rest)
					i++
					continue
				}
				// Lazy continuation for open paragraph inside blockquote.
				if len(qLines) > 0 && !isBlankLine(qLines[len(qLines)-1]) && !p.startsBlock(lines, i, true) {
					qLines = append(qLines, lines[i])
					i++
					continue
				}
				break
			}
			out = append(out, mdBlock{kind: mdKindBlockquote, children: p.parseBlockLines(qLines, false)})
			continue
		}

		// List (unordered or ordered).
		if marker, ok := parseListMarker(line, false); ok {
			blk, next := p.parseList(lines, i, marker)
			out = append(out, blk)
			i = next
			continue
		}

		// HTML block (types 1–7).
		if htmlType := htmlBlockStart(line[off:], true); htmlType > 0 {
			var hLines []string
			for i < len(lines) {
				hLines = append(hLines, lines[i])
				if htmlBlockEnds(lines[i], htmlType) {
					i++
					break
				}
				i++
				if (htmlType == 6 || htmlType == 7) && i < len(lines) && isBlankLine(lines[i]) {
					break
				}
			}
			out = append(out, mdBlock{kind: mdKindHTML, text: strings.Join(hLines, "\n") + "\n"})
			continue
		}

		// GFM table (header row followed by delimiter row).
		if i+1 < len(lines) {
			if align, ok := parseTableDelimiter(lines[i+1]); ok {
				head := splitTableRow(line)
				if len(head) == len(align) {
					i += 2
					var rows [][]string
					for i < len(lines) && !isBlankLine(lines[i]) {
						if p.startsBlock(lines, i, true) {
							break
						}
						if !strings.Contains(lines[i], "|") {
							break
						}
						cells := splitTableRow(lines[i])
						for len(cells) < len(align) {
							cells = append(cells, "")
						}
						if len(cells) > len(align) {
							cells = cells[:len(align)]
						}
						rows = append(rows, cells)
						i++
					}
					out = append(out, mdBlock{kind: mdKindTable, align: align, head: head, rows: rows})
					continue
				}
			}
		}

		// Paragraph (or Setext heading if underlined by === or ---).
		var para []string
		para = append(para, strings.TrimLeft(line, " \t"))
		i++
		setextLevel := 0
		for i < len(lines) {
			if isBlankLine(lines[i]) {
				break
			}
			c, o := leadingColumns(lines[i])
			if c < 4 {
				if lvl := parseSetextUnderline(lines[i][o:]); lvl > 0 {
					setextLevel = lvl
					i++
					break
				}
				if p.startsBlock(lines, i, true) {
					break
				}
				// Check if current line starts a GFM table (unlikely mid-para, but handle).
				if i+1 < len(lines) {
					if align, ok := parseTableDelimiter(lines[i+1]); ok && len(splitTableRow(lines[i])) == len(align) {
						break
					}
				}
			}
			para = append(para, strings.TrimLeft(lines[i], " \t"))
			i++
		}
		if setextLevel > 0 {
			out = append(out, mdBlock{kind: mdKindHeading, level: setextLevel, text: strings.TrimSpace(strings.Join(para, "\n"))})
		} else {
			out = append(out, mdBlock{kind: mdKindParagraph, lines: para})
		}
	}
	_ = topLevel
	return out
}

// startsBlock reports whether lines[i] begins a block construct that interrupts
// a paragraph (when interrupting is true).
func (p *mdParser) startsBlock(lines []string, i int, interrupting bool) bool {
	line := lines[i]
	if isBlankLine(line) {
		return true
	}
	cols, off := leadingColumns(line)
	if cols >= 4 {
		return false
	}
	rest := line[off:]
	if _, _, _, ok := parseContainerOpen(line, p.containers); ok {
		return true
	}
	if !p.opts.NoMath {
		if _, _, ok := parseMathBlockOpen(line); ok {
			return true
		}
	}
	if _, _, ok := parseATXHeading(rest); ok {
		return true
	}
	if _, _, _, ok := parseFencedCodeOpen(rest); ok {
		return true
	}
	if isThematicBreak(rest) {
		return true
	}
	if strings.HasPrefix(rest, ">") {
		return true
	}
	if _, ok := parseListMarker(line, interrupting); ok {
		return true
	}
	if htmlBlockStart(rest, !interrupting) > 0 {
		return true
	}
	if _, _, ok := parseFootnoteDef(rest); ok {
		return true
	}
	return false
}

// --- Block syntax helpers ---

func parseATXHeading(s string) (level int, text string, ok bool) {
	for level < len(s) && s[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0, "", false
	}
	if level < len(s) && s[level] != ' ' && s[level] != '\t' {
		return 0, "", false
	}
	body := strings.Trim(s[level:], " \t")
	// Strip optional closing sequence of # preceded by space/tab (or whole line of #).
	trimmed := strings.TrimRight(body, "#")
	if len(trimmed) < len(body) {
		if trimmed == "" || trimmed[len(trimmed)-1] == ' ' || trimmed[len(trimmed)-1] == '\t' {
			body = strings.TrimRight(trimmed, " \t")
		}
	}
	return level, body, true
}

func parseSetextUnderline(s string) int {
	s = strings.TrimRight(s, " \t")
	if len(s) == 0 {
		return 0
	}
	ch := s[0]
	if ch != '=' && ch != '-' {
		return 0
	}
	for i := 1; i < len(s); i++ {
		if s[i] != ch {
			return 0
		}
	}
	if ch == '=' {
		return 1
	}
	return 2
}

func isThematicBreak(s string) bool {
	var ch byte
	count := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' {
			continue
		}
		if c != '-' && c != '*' && c != '_' {
			return false
		}
		if ch == 0 {
			ch = c
		} else if c != ch {
			return false
		}
		count++
	}
	return count >= 3
}

func parseFencedCodeOpen(s string) (ch byte, fenceLen int, info string, ok bool) {
	if len(s) < 3 || (s[0] != '`' && s[0] != '~') {
		return 0, 0, "", false
	}
	ch = s[0]
	for fenceLen < len(s) && s[fenceLen] == ch {
		fenceLen++
	}
	if fenceLen < 3 {
		return 0, 0, "", false
	}
	info = strings.TrimSpace(s[fenceLen:])
	if ch == '`' && strings.ContainsRune(info, '`') {
		return 0, 0, "", false
	}
	return ch, fenceLen, info, true
}

func isFencedCodeClose(line string, ch byte, minLen int) bool {
	cols, off := leadingColumns(line)
	if cols >= 4 {
		return false
	}
	rest := strings.TrimRight(line[off:], " \t")
	if len(rest) < minLen {
		return false
	}
	for i := 0; i < len(rest); i++ {
		if rest[i] != ch {
			return false
		}
	}
	return true
}

type listMarker struct {
	ordered bool
	bullet  byte
	start   int
	indent  int // visual columns of content after marker
	after   int // byte offset in line where item content begins
}

func parseListMarker(line string, interrupting bool) (listMarker, bool) {
	cols, off := leadingColumns(line)
	if cols >= 4 || off >= len(line) {
		return listMarker{}, false
	}
	rest := line[off:]
	if isThematicBreak(rest) {
		return listMarker{}, false
	}
	// Bullet list: -, *, +
	if rest[0] == '-' || rest[0] == '*' || rest[0] == '+' {
		if len(rest) == 1 {
			if interrupting {
				return listMarker{}, false
			}
			return listMarker{bullet: rest[0], indent: cols + 2, after: len(line)}, true
		}
		if rest[1] != ' ' && rest[1] != '\t' {
			return listMarker{}, false
		}
		spCols, spOff := leadingColumns(rest[1:])
		if spOff == len(rest[1:]) {
			if interrupting {
				return listMarker{}, false
			}
			return listMarker{bullet: rest[0], indent: cols + 2, after: len(line)}, true
		}
		pad := spCols
		after := off + 1 + spOff
		if pad > 4 {
			pad = 1
			after = off + 2
		}
		return listMarker{bullet: rest[0], indent: cols + 1 + pad, after: after}, true
	}
	// Ordered list: 1-9 digits followed by '.' or ')'
	d := 0
	for d < len(rest) && d < 9 && rest[d] >= '0' && rest[d] <= '9' {
		d++
	}
	if d == 0 || d >= len(rest) || (rest[d] != '.' && rest[d] != ')') {
		return listMarker{}, false
	}
	delim := rest[d]
	num, err := strconv.Atoi(rest[:d])
	if err != nil {
		return listMarker{}, false
	}
	if interrupting && num != 1 {
		return listMarker{}, false
	}
	afterDelim := rest[d+1:]
	if len(afterDelim) == 0 {
		if interrupting {
			return listMarker{}, false
		}
		return listMarker{ordered: true, bullet: delim, start: num, indent: cols + d + 2, after: len(line)}, true
	}
	if afterDelim[0] != ' ' && afterDelim[0] != '\t' {
		return listMarker{}, false
	}
	spCols, spOff := leadingColumns(afterDelim)
	if spOff == len(afterDelim) {
		if interrupting {
			return listMarker{}, false
		}
		return listMarker{ordered: true, bullet: delim, start: num, indent: cols + d + 2, after: len(line)}, true
	}
	pad := spCols
	after := off + d + 1 + spOff
	if pad > 4 {
		pad = 1
		after = off + d + 2
	}
	return listMarker{ordered: true, bullet: delim, start: num, indent: cols + d + 1 + pad, after: after}, true
}

func (p *mdParser) parseList(lines []string, i int, first listMarker) (mdBlock, int) {
	blk := mdBlock{kind: mdKindList, ordered: first.ordered, start: first.start}
	sawBlankBetweenItems := false
	for i < len(lines) {
		m, ok := parseListMarker(lines[i], false)
		if !ok || m.ordered != first.ordered || m.bullet != first.bullet {
			break
		}
		if sawBlankBetweenItems {
			blk.loose = true
			sawBlankBetweenItems = false
		}
		var itemLines []string
		itemLines = append(itemLines, lines[i][m.after:])
		i++
		itemHadBlank := false
		for i < len(lines) {
			if isBlankLine(lines[i]) {
				// Look ahead past blank lines.
				j := i
				for j < len(lines) && isBlankLine(lines[j]) {
					j++
				}
				if j >= len(lines) {
					i = j
					break
				}
				c, _ := leadingColumns(lines[j])
				if c >= m.indent {
					itemHadBlank = true
					for ; i < j; i++ {
						itemLines = append(itemLines, "")
					}
					continue
				}
				// Next non-blank line is another sibling list item?
				if nm, nok := parseListMarker(lines[j], false); nok && nm.ordered == first.ordered && nm.bullet == first.bullet {
					sawBlankBetweenItems = true
					i = j
					break
				}
				i = j
				break
			}
			c, _ := leadingColumns(lines[i])
			if c >= m.indent {
				itemLines = append(itemLines, stripColumns(lines[i], m.indent))
				i++
				continue
			}
			if nm, nok := parseListMarker(lines[i], false); nok && nm.ordered == first.ordered && nm.bullet == first.bullet {
				break
			}
			if p.startsBlock(lines, i, true) {
				break
			}
			// Lazy continuation of an open paragraph inside the list item.
			if len(itemLines) > 0 && !isBlankLine(itemLines[len(itemLines)-1]) {
				itemLines = append(itemLines, lines[i])
				i++
				continue
			}
			break
		}
		children := p.parseBlockLines(itemLines, false)
		if itemHadBlank && len(children) > 1 {
			blk.loose = true
		}
		blk.items = append(blk.items, mdListItem{children: children})
	}
	return blk, i
}

// --- GFM Table helpers ---

func parseTableDelimiter(line string) ([]ColumnAlign, bool) {
	if !strings.Contains(line, "|") && !strings.Contains(line, "-") {
		return nil, false
	}
	cols, off := leadingColumns(line)
	if cols >= 4 {
		return nil, false
	}
	trimmed := strings.TrimSpace(line[off:])
	if !strings.Contains(trimmed, "-") {
		return nil, false
	}
	if !strings.Contains(trimmed, "|") {
		return nil, false
	}
	cells := splitTableRow(trimmed)
	if len(cells) == 0 {
		return nil, false
	}
	align := make([]ColumnAlign, len(cells))
	for i, c := range cells {
		c = strings.TrimSpace(c)
		if len(c) == 0 {
			return nil, false
		}
		left := c[0] == ':'
		if left {
			c = c[1:]
		}
		right := len(c) > 0 && c[len(c)-1] == ':'
		if right {
			c = c[:len(c)-1]
		}
		if len(c) == 0 || strings.Trim(c, "-") != "" {
			return nil, false
		}
		switch {
		case left && right:
			align[i] = AlignCenter
		case left:
			align[i] = AlignLeft
		case right:
			align[i] = AlignRight
		default:
			align[i] = AlignNone
		}
	}
	return align, true
}

func splitTableRow(line string) []string {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "|")
	// Trim unescaped trailing pipe.
	if strings.HasSuffix(s, "|") && !isEscapedAt(s, len(s)-1) {
		s = s[:len(s)-1]
	}
	var cells []string
	var cur strings.Builder
	inCode := 0
	for i := 0; i < len(s); {
		if s[i] == '`' {
			n := 0
			for i+n < len(s) && s[i+n] == '`' {
				n++
			}
			cur.WriteString(s[i : i+n])
			if inCode == 0 {
				inCode = n
			} else if inCode == n {
				inCode = 0
			}
			i += n
			continue
		}
		if inCode == 0 && s[i] == '\\' && i+1 < len(s) && s[i+1] == '|' {
			cur.WriteByte('|')
			i += 2
			continue
		}
		if inCode == 0 && s[i] == '|' {
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
			i++
			continue
		}
		cur.WriteByte(s[i])
		i++
	}
	cells = append(cells, strings.TrimSpace(cur.String()))
	return cells
}

func isEscapedAt(s string, pos int) bool {
	bs := 0
	for i := pos - 1; i >= 0 && s[i] == '\\'; i-- {
		bs++
	}
	return bs%2 == 1
}

// --- HTML block helpers (CommonMark §4.6 types 1–7) ---

func htmlBlockStart(s string, allowType7 bool) int {
	if !strings.HasPrefix(s, "<") {
		return 0
	}
	low := strings.ToLower(s)
	// Type 1: <pre, <script, <style, <textarea
	for _, tag := range []string{"<pre", "<script", "<style", "<textarea"} {
		if strings.HasPrefix(low, tag) {
			rest := low[len(tag):]
			if rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '>' || rest[0] == '\r' || rest[0] == '\n' {
				return 1
			}
		}
	}
	// Type 2: <!--
	if strings.HasPrefix(s, "<!--") {
		return 2
	}
	// Type 3: <?
	if strings.HasPrefix(s, "<?") {
		return 3
	}
	// Type 4: <!LETTER
	if len(s) >= 3 && s[1] == '!' && ((s[2] >= 'A' && s[2] <= 'Z') || (s[2] >= 'a' && s[2] <= 'z')) {
		return 4
	}
	// Type 5: <![CDATA[
	if strings.HasPrefix(s, "<![CDATA[") {
		return 5
	}
	// Type 6: standard block tag (<tag or </tag).
	inner := low[1:]
	inner = strings.TrimPrefix(inner, "/")
	end := 0
	for end < len(inner) && (inner[end] >= 'a' && inner[end] <= 'z' || inner[end] >= '0' && inner[end] <= '9' || inner[end] == '-') {
		end++
	}
	if end > 0 && isHTMLBlockTag(inner[:end]) {
		after := inner[end:]
		if after == "" || after[0] == ' ' || after[0] == '\t' || after[0] == '>' || strings.HasPrefix(after, "/>") {
			return 6
		}
	}
	// Type 7: complete open or close tag alone on a line (for custom/MDX tags).
	if allowType7 {
		if endTag, ok := scanInlineHTMLTag(s, 0); ok && strings.TrimSpace(s[endTag:]) == "" {
			return 7
		}
	}
	return 0
}

func htmlBlockEnds(line string, typ int) bool {
	low := strings.ToLower(line)
	switch typ {
	case 1:
		return strings.Contains(low, "</pre>") || strings.Contains(low, "</script>") ||
			strings.Contains(low, "</style>") || strings.Contains(low, "</textarea>")
	case 2:
		return strings.Contains(line, "-->")
	case 3:
		return strings.Contains(line, "?>")
	case 4:
		return strings.Contains(line, ">")
	case 5:
		return strings.Contains(line, "]]>")
	}
	return false
}

func isHTMLBlockTag(name string) bool {
	switch name {
	case "address", "article", "aside", "base", "basefont", "blockquote", "body", "caption", "center",
		"col", "colgroup", "dd", "details", "dialog", "dir", "div", "dl", "dt", "fieldset", "figcaption",
		"figure", "footer", "form", "frame", "frameset", "h1", "h2", "h3", "h4", "h5", "h6", "head",
		"header", "hr", "html", "iframe", "legend", "li", "link", "main", "menu", "menuitem", "nav",
		"noframes", "ol", "optgroup", "option", "p", "param", "search", "section", "summary", "table",
		"tbody", "td", "tfoot", "th", "thead", "title", "tr", "track", "ul":
		return true
	}
	return false
}

// --- Footnote and link reference definition helpers ---

func parseFootnoteDef(s string) (label, first string, ok bool) {
	if !strings.HasPrefix(s, "[^") {
		return "", "", false
	}
	close := strings.Index(s[2:], "]:")
	if close <= 0 {
		return "", "", false
	}
	label = s[2 : 2+close]
	if strings.ContainsAny(label, " \t\n[]") {
		return "", "", false
	}
	rest := s[2+close+2:]
	if len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
		rest = strings.TrimLeft(rest, " \t")
	}
	return label, rest, true
}

func parseLinkRefDef(lines []string) (label string, def mdLinkDef, consumed int, ok bool) {
	_, off := leadingColumns(lines[0])
	s := lines[0][off:]
	if !strings.HasPrefix(s, "[") || strings.HasPrefix(s, "[^") {
		return "", mdLinkDef{}, 0, false
	}
	close := findUnescapedByte(s, ']', 1)
	if close <= 1 || close+1 >= len(s) || s[close+1] != ':' {
		return "", mdLinkDef{}, 0, false
	}
	label = s[1:close]
	if strings.TrimSpace(label) == "" {
		return "", mdLinkDef{}, 0, false
	}
	rest := strings.TrimSpace(s[close+2:])
	consumed = 1
	if rest == "" && len(lines) > 1 && !isBlankLine(lines[1]) {
		rest = strings.TrimSpace(lines[1])
		consumed = 2
	}
	if rest == "" {
		return "", mdLinkDef{}, 0, false
	}
	url, after, ok := parseLinkDestination(rest)
	if !ok {
		return "", mdLinkDef{}, 0, false
	}
	after = strings.TrimLeft(after, " \t")
	var title string
	if after != "" {
		var tok bool
		title, after, tok = parseLinkTitle(after)
		if !tok || strings.TrimSpace(after) != "" {
			return "", mdLinkDef{}, 0, false
		}
	}
	return label, mdLinkDef{url: unescapeMarkdown(url), title: unescapeMarkdown(title)}, consumed, true
}

func normalizeLabel(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// --- Block-to-Document conversion ---

func (p *mdParser) renderBlocks(blocks []mdBlock) []Block {
	var out []Block
	for i := 0; i < len(blocks); i++ {
		b := blocks[i]
		if b.kind == mdKindHTML {
			if p.opts.NoHTML {
				out = append(out, Paragraph{Content: []Inline{Text{strings.TrimRight(b.text, "\n")}}})
				continue
			}
			if closeIdx := p.renderHTMLElement(blocks, i, &out); closeIdx >= 0 {
				i = closeIdx
				continue
			}
			out = append(out, htmlBlocks(b.text, p.ext)...)
			continue
		}
		if rendered := p.renderBlock(b); rendered != nil {
			out = append(out, rendered)
		}
	}
	return out
}

func (p *mdParser) renderHTMLElement(blocks []mdBlock, openIdx int, out *[]Block) int {
	source := blocks[openIdx].text
	name, ok := unclosedElement(source)
	if !ok {
		return -1
	}
	depth := 0
	closeIdx := -1
	for j := openIdx + 1; j < len(blocks); j++ {
		if blocks[j].kind != mdKindHTML {
			continue
		}
		s := blocks[j].text
		if inner, ok := unclosedElement(s); ok && strings.EqualFold(inner, name) {
			depth++
		} else if isClosingTag(s, name) {
			if depth == 0 {
				closeIdx = j
				break
			}
			depth--
		}
	}
	if closeIdx < 0 {
		return -1
	}
	element := htmlBlocks(source+"\n"+strings.TrimSpace(blocks[closeIdx].text), p.ext)
	if len(element) == 0 {
		return -1
	}
	inner := p.renderBlocks(blocks[openIdx+1 : closeIdx])
	switch b := element[len(element)-1].(type) {
	case Details:
		b.Children = append(b.Children, inner...)
		element[len(element)-1] = b
	case Blockquote:
		b.Children = append(b.Children, inner...)
		element[len(element)-1] = b
	case Custom:
		b.Children = append(b.Children, inner...)
		if len(b.Content) == 0 {
			b.Text = Document{Blocks: b.Children}.PlainText()
		}
		element[len(element)-1] = b
	default:
		return -1
	}
	*out = append(*out, element...)
	return closeIdx
}

func (p *mdParser) renderBlock(b mdBlock) Block {
	switch b.kind {
	case mdKindParagraph:
		return Paragraph{Content: p.parseInlineText(strings.Join(b.lines, "\n"))}
	case mdKindHeading:
		content := p.parseInlineText(b.text)
		return Heading{Level: b.level, Content: content, Anchor: p.slugs.slug(PlainText(content))}
	case mdKindRule:
		return Rule{}
	case mdKindIndentedCode:
		return CodeBlock{Code: b.text}
	case mdKindFencedCode:
		info := unescapeMarkdown(strings.TrimSpace(b.info))
		lang, meta, _ := strings.Cut(info, " ")
		meta = strings.TrimSpace(meta)
		for _, e := range p.ext {
			if e.Language != "" && strings.EqualFold(e.Language, lang) {
				c := Custom{Name: e.Name, Text: b.text}
				if meta != "" {
					c.Attributes = map[string]string{"meta": meta}
				}
				return c
			}
		}
		if strings.EqualFold(lang, "math") && !p.opts.NoMath {
			return Math{TeX: b.text}
		}
		return CodeBlock{Language: lang, Meta: meta, Code: b.text}
	case mdKindBlockquote:
		return p.renderBlockquote(b)
	case mdKindList:
		l := List{Ordered: b.ordered, Loose: b.loose}
		if l.Ordered {
			l.Start = b.start
		}
		for _, it := range b.items {
			item := ListItem{}
			children := it.children
			if len(children) > 0 && children[0].kind == mdKindParagraph && len(children[0].lines) > 0 {
				firstLine := children[0].lines[0]
				if len(firstLine) >= 4 && firstLine[0] == '[' && firstLine[2] == ']' && (firstLine[3] == ' ' || firstLine[3] == '\t') &&
					(firstLine[1] == ' ' || firstLine[1] == 'x' || firstLine[1] == 'X') {
					item.Task = true
					item.Done = firstLine[1] != ' '
					cp := make([]mdBlock, len(children))
					copy(cp, children)
					cpLines := slices.Clone(children[0].lines)
					cpLines[0] = strings.TrimLeft(firstLine[3:], " \t")
					cp[0].lines = cpLines
					children = cp
				}
			}
			item.Children = p.renderBlocks(children)
			l.Items = append(l.Items, item)
		}
		return l
	case mdKindTable:
		t := Table{Align: b.align}
		for _, c := range b.head {
			t.Head = append(t.Head, TableCell{Content: p.parseInlineText(c)})
		}
		for _, row := range b.rows {
			var cells []TableCell
			for _, c := range row {
				cells = append(cells, TableCell{Content: p.parseInlineText(c)})
			}
			t.Rows = append(t.Rows, cells)
		}
		return t
	case mdKindMath:
		return Math{TeX: b.text}
	case mdKindContainer:
		return Custom{Name: b.name, Attributes: b.attrs, Children: p.renderBlocks(b.children)}
	}
	return nil
}

func (p *mdParser) renderBlockquote(b mdBlock) Block {
	if len(b.children) == 0 || b.children[0].kind != mdKindParagraph || len(b.children[0].lines) == 0 {
		return Blockquote{Children: p.renderBlocks(b.children)}
	}
	alert := alertKind(strings.TrimSpace(b.children[0].lines[0]))
	if alert == AlertNone || p.opts.NoAlerts {
		return Blockquote{Children: p.renderBlocks(b.children)}
	}
	q := Blockquote{Alert: alert}
	if rest := b.children[0].lines[1:]; len(rest) > 0 {
		if content := p.parseInlineText(strings.Join(rest, "\n")); len(content) > 0 {
			q.Children = append(q.Children, Paragraph{Content: content})
		}
	}
	q.Children = append(q.Children, p.renderBlocks(b.children[1:])...)
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

// --- Inline parser ---

type inlineTokenKind uint8

const (
	tokText inlineTokenKind = iota
	tokInline
	tokRawHTML
	tokDelim
)

type inlineToken struct {
	kind      inlineTokenKind
	text      string
	inline    Inline
	start     int
	end       int
	ch        byte
	count     int
	origCount int
	canOpen   bool
	canClose  bool
}

func (p *mdParser) parseInlineText(src string) []Inline {
	toks := p.tokenizeInlines(src)
	p.resolveDelimiters(toks)
	out := p.tokensToInlines(toks, 0, len(toks), src)
	return p.finish(out)
}

func (p *mdParser) tokenizeInlines(src string) []inlineToken {
	var toks []inlineToken
	flushText := func(s string, start, end int) {
		if s == "" {
			return
		}
		toks = append(toks, inlineToken{kind: tokText, text: s, start: start, end: end})
	}
	var buf strings.Builder
	bufStart := 0
	flush := func(pos int) {
		if buf.Len() > 0 {
			flushText(buf.String(), bufStart, pos)
			buf.Reset()
		}
		bufStart = pos
	}

	i := 0
	for i < len(src) {
		c := src[i]

		// Backslash escape or hard line break.
		if c == '\\' && i+1 < len(src) {
			next := src[i+1]
			if next == '\n' {
				flush(i)
				toks = append(toks, inlineToken{kind: tokInline, inline: Break{}, start: i, end: i + 2})
				i += 2
				for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
					i++
				}
				bufStart = i
				continue
			}
			if isASCIIPunct(next) {
				buf.WriteByte(next)
				i += 2
				continue
			}
		}

		// Line break: 2+ trailing spaces -> hard break, else soft break (" ").
		if c == '\n' {
			cur := buf.String()
			trimmed := strings.TrimRight(cur, " ")
			hard := len(cur)-len(trimmed) >= 2
			buf.Reset()
			buf.WriteString(trimmed)
			flush(i)
			if hard {
				toks = append(toks, inlineToken{kind: tokInline, inline: Break{}, start: i, end: i + 1})
			} else {
				toks = append(toks, inlineToken{kind: tokText, text: " ", start: i, end: i + 1})
			}
			i++
			for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
				i++
			}
			bufStart = i
			continue
		}

		// Backtick code span.
		if c == '`' {
			if code, end, ok := parseCodeSpan(src, i); ok {
				flush(i)
				toks = append(toks, inlineToken{kind: tokInline, inline: Code{code}, start: i, end: end})
				i = end
				bufStart = i
				continue
			}
			// Unmatched backtick run: emit literally so inner backticks aren't re-matched.
			n := 0
			for i+n < len(src) && src[i+n] == '`' {
				n++
			}
			buf.WriteString(src[i : i+n])
			i += n
			continue
		}

		// Inline math ($...$ or $$...$$).
		if c == '$' && !p.opts.NoMath {
			if tex, end, ok := parseInlineMath(src, i); ok {
				flush(i)
				toks = append(toks, inlineToken{kind: tokInline, inline: Math{TeX: tex}, start: i, end: end})
				i = end
				bufStart = i
				continue
			}
			// Emit the dollar run literally.
			n := dollarsStr(src, i)
			buf.WriteString(src[i : i+n])
			i += n
			continue
		}

		// Angle-bracket autolink or raw HTML tag.
		if c == '<' {
			if url, label, end, ok := parseAngleAutolink(src, i); ok {
				flush(i)
				toks = append(toks, inlineToken{kind: tokInline, inline: Link{URL: url, Content: []Inline{Text{label}}}, start: i, end: end})
				i = end
				bufStart = i
				continue
			}
			if end, ok := scanInlineHTMLTag(src, i); ok {
				flush(i)
				toks = append(toks, inlineToken{kind: tokRawHTML, text: src[i:end], start: i, end: end})
				i = end
				bufStart = i
				continue
			}
		}

		// Image (![alt](...)) or link ([text](...)) or footnote reference ([^label]).
		if c == '!' && i+1 < len(src) && src[i+1] == '[' {
			if img, end, ok := p.parseImage(src, i); ok {
				flush(i)
				toks = append(toks, inlineToken{kind: tokInline, inline: img, start: i, end: end})
				i = end
				bufStart = i
				continue
			}
		}
		if c == '[' {
			if i+1 < len(src) && src[i+1] == '^' {
				if ref, end, ok := p.parseFootnoteRef(src, i); ok {
					flush(i)
					toks = append(toks, inlineToken{kind: tokInline, inline: ref, start: i, end: end})
					i = end
					bufStart = i
					continue
				}
			}
			if link, end, ok := p.parseLink(src, i); ok {
				flush(i)
				toks = append(toks, inlineToken{kind: tokInline, inline: link, start: i, end: end})
				i = end
				bufStart = i
				continue
			}
		}

		// GFM bare autolink (http://, https://, www.).
		if (c == 'h' || c == 'w') && (i == 0 || isAutolinkBoundary(src, i)) {
			if url, label, end, ok := parseBareAutolink(src, i); ok {
				flush(i)
				toks = append(toks, inlineToken{kind: tokInline, inline: Link{URL: url, Content: []Inline{Text{label}}}, start: i, end: end})
				i = end
				bufStart = i
				continue
			}
		}

		// Emphasis / strikethrough delimiter run (*, _, ~).
		if c == '*' || c == '_' || c == '~' {
			n := 0
			for i+n < len(src) && src[i+n] == c {
				n++
			}
			if c == '~' && n > 2 {
				buf.WriteString(src[i : i+n])
				i += n
				continue
			}
			canOpen, canClose := delimiterFlanking(src, i, i+n, c)
			if canOpen || canClose {
				flush(i)
				toks = append(toks, inlineToken{
					kind:      tokDelim,
					text:      src[i : i+n],
					start:     i,
					end:       i + n,
					ch:        c,
					count:     n,
					origCount: n,
					canOpen:   canOpen,
					canClose:  canClose,
				})
				i += n
				bufStart = i
				continue
			}
			buf.WriteString(src[i : i+n])
			i += n
			continue
		}

		// HTML entity (&amp;, &#123;, &#x1f600;).
		if c == '&' {
			if decoded, end, ok := parseHTMLEntity(src, i); ok {
				buf.WriteString(decoded)
				i = end
				continue
			}
		}

		buf.WriteByte(c)
		i++
	}
	flush(i)
	return toks
}

func (p *mdParser) resolveDelimiters(toks []inlineToken) {
	// Track matched pairs on the token slice using inline nodes written back in-place.
	// Use a simple stack pass over tokDelim tokens matching CommonMark's process_emphasis.
	type delimEntry struct{ idx int }
	var stack []delimEntry
	for i := 0; i < len(toks); i++ {
		if toks[i].kind != tokDelim {
			continue
		}
		if toks[i].canClose {
			for toks[i].count > 0 {
				matched := -1
				for s := len(stack) - 1; s >= 0; s-- {
					oi := stack[s].idx
					o := &toks[oi]
					if o.count == 0 || !o.canOpen || o.ch != toks[i].ch {
						continue
					}
					if o.ch == '~' && o.count != toks[i].count {
						continue
					}
					// CommonMark rule of 3 for * and _.
					if (o.ch == '*' || o.ch == '_') && (o.canClose || toks[i].canOpen) &&
						(o.origCount+toks[i].origCount)%3 == 0 && (o.origCount%3 != 0 || toks[i].origCount%3 != 0) {
						continue
					}
					matched = s
					break
				}
				if matched < 0 {
					break
				}
				oi := stack[matched].idx
				use := 1
				if toks[oi].ch == '~' {
					use = toks[oi].count
				} else if toks[oi].count >= 2 && toks[i].count >= 2 {
					use = 2
				}
				toks[oi].count -= use
				toks[i].count -= use
				// Discard any unmatched delimiters strictly between opener and closer.
				stack = stack[:matched+1]
				if toks[oi].count == 0 {
					stack = stack[:matched]
				}
				// Wrap tokens (oi+1 .. i-1) into Strong / Emphasis / Strikethrough and
				// collapse them into slot oi+1.
				inner := p.tokensToInlines(toks, oi+1, i, "")
				var wrapped Inline
				switch toks[oi].ch {
				case '~':
					wrapped = Strikethrough{inner}
				default:
					if use == 2 {
						wrapped = Strong{inner}
					} else {
						wrapped = Emphasis{inner}
					}
				}
				for k := oi + 1; k < i; k++ {
					toks[k] = inlineToken{kind: tokText, text: ""}
				}
				// Place wrapped node at oi+1 if room, else keep in a new token?
				// Always keep an empty slot or merge: if oi+1 < i, reuse oi+1.
				if oi+1 < i {
					toks[oi+1] = inlineToken{kind: tokInline, inline: wrapped, start: toks[oi].start, end: toks[i].end}
				} else {
					// Empty emphasis (shouldn't happen for valid flanking, skip).
					toks[oi].count += use
					toks[i].count += use
					break
				}
			}
		}
		if toks[i].canOpen && toks[i].count > 0 {
			stack = append(stack, delimEntry{idx: i})
		}
	}
	// Convert remaining unmatched delimiters to literal text.
	for i := range toks {
		if toks[i].kind == tokDelim {
			if toks[i].count > 0 {
				toks[i].kind = tokText
				toks[i].text = strings.Repeat(string(toks[i].ch), toks[i].count)
			} else {
				toks[i].kind = tokText
				toks[i].text = ""
			}
		}
	}
}

func (p *mdParser) tokensToInlines(toks []inlineToken, start, end int, src string) []Inline {
	var out []Inline
	for i := start; i < end; i++ {
		t := toks[i]
		switch t.kind {
		case tokText:
			if t.text != "" {
				out = append(out, Text{t.text})
			}
		case tokInline:
			if t.inline != nil {
				out = append(out, t.inline)
			}
		case tokDelim:
			if t.count > 0 {
				out = append(out, Text{strings.Repeat(string(t.ch), t.count)})
			}
		case tokRawHTML:
			if p.opts.NoHTML {
				out = append(out, Text{t.text})
				continue
			}
			tag := t.text
			name, ok := unclosedElement(tag)
			closeIdx := -1
			if ok {
				depth := 0
				for j := i + 1; j < end; j++ {
					if toks[j].kind != tokRawHTML {
						continue
					}
					if inner, ok := unclosedElement(toks[j].text); ok && strings.EqualFold(inner, name) {
						depth++
					} else if isClosingTag(toks[j].text, name) {
						if depth == 0 {
							closeIdx = j
							break
						}
						depth--
					}
				}
			}
			if closeIdx < 0 {
				out = append(out, htmlInlines(tag, p.ext)...)
				continue
			}
			if in, ok := p.formatting(tag, strings.ToLower(name), toks, i+1, closeIdx, src); ok {
				out = append(out, in)
			} else if src != "" && t.start < toks[closeIdx].end {
				out = append(out, htmlInlines(src[t.start:toks[closeIdx].end], p.ext)...)
			} else {
				var raw strings.Builder
				for k := i; k <= closeIdx; k++ {
					raw.WriteString(toks[k].text)
				}
				out = append(out, htmlInlines(raw.String(), p.ext)...)
			}
			i = closeIdx
		}
	}
	return join(out)
}

// formatting reads a formatting element whose content is Markdown
// (<strong>b *c*</strong>: c is emphasized): the element as HTML reads its
// opening tag (a <mark>'s color, a link's URL), holding the Markdown
// between its tags.
func (p *mdParser) formatting(openTag, name string, toks []inlineToken, innerStart, innerEnd int, src string) (Inline, bool) {
	const slot = "\ue000"
	if !isFormattingElement(name) {
		return nil, false
	}
	read := htmlInlines(openTag+slot+"</"+name+">", p.ext)
	if len(read) != 1 {
		return nil, false
	}
	content := p.tokensToInlines(toks, innerStart, innerEnd, src)
	isSlot := func(c []Inline) bool { return len(c) == 1 && c[0] == Inline(Text{slot}) }
	switch in := read[0].(type) {
	case Strong:
		return Strong{content}, isSlot(in.Content)
	case Emphasis:
		return Emphasis{content}, isSlot(in.Content)
	case Strikethrough:
		return Strikethrough{content}, isSlot(in.Content)
	case Underline:
		return Underline{content}, isSlot(in.Content)
	case Mark:
		return Mark{content, in.Color}, isSlot(in.Content)
	case Superscript:
		return Superscript{content}, isSlot(in.Content)
	case Subscript:
		return Subscript{content}, isSlot(in.Content)
	case Small:
		return Small{content}, isSlot(in.Content)
	case Link:
		return Link{URL: in.URL, Title: in.Title, Content: content}, isSlot(in.Content)
	}
	return nil, false
}

// --- Inline construct parsers ---

func parseCodeSpan(src string, pos int) (code string, end int, ok bool) {
	n := 0
	for pos+n < len(src) && src[pos+n] == '`' {
		n++
	}
	for i := pos + n; i < len(src); {
		if src[i] != '`' {
			i++
			continue
		}
		m := 0
		for i+m < len(src) && src[i+m] == '`' {
			m++
		}
		if m == n {
			body := strings.ReplaceAll(src[pos+n:i], "\n", " ")
			if len(body) >= 2 && body[0] == ' ' && body[len(body)-1] == ' ' && strings.Trim(body, " ") != "" {
				body = body[1 : len(body)-1]
			}
			return body, i + m, true
		}
		i += m
	}
	return "", 0, false
}

func (p *mdParser) parseFootnoteRef(src string, pos int) (FootnoteReference, int, bool) {
	close := strings.IndexByte(src[pos+2:], ']')
	if close <= 0 {
		return FootnoteReference{}, 0, false
	}
	label := src[pos+2 : pos+2+close]
	if strings.ContainsAny(label, " \t\n[") {
		return FootnoteReference{}, 0, false
	}
	if _, defined := p.fnDefs[label]; !defined {
		return FootnoteReference{}, 0, false
	}
	idx, ok := p.fnIndex[label]
	if !ok {
		p.fnOrder = append(p.fnOrder, label)
		idx = len(p.fnOrder)
		p.fnIndex[label] = idx
	}
	return FootnoteReference{Label: label, Index: idx}, pos + 2 + close + 1, true
}

func (p *mdParser) parseImage(src string, pos int) (Image, int, bool) {
	labelText, closeIdx, ok := matchBracketLabel(src, pos+1)
	if !ok {
		return Image{}, 0, false
	}
	url, title, end, ok := p.parseLinkTail(src, closeIdx+1, labelText)
	if !ok {
		return Image{}, 0, false
	}
	alt := PlainText(p.parseInlineText(labelText))
	return Image{Src: url, Alt: alt, Title: title}, end, true
}

func (p *mdParser) parseLink(src string, pos int) (Link, int, bool) {
	labelText, closeIdx, ok := matchBracketLabel(src, pos)
	if !ok {
		return Link{}, 0, false
	}
	url, title, end, ok := p.parseLinkTail(src, closeIdx+1, labelText)
	if !ok {
		return Link{}, 0, false
	}
	toks := p.tokenizeInlines(labelText)
	p.resolveDelimiters(toks)
	content := p.tokensToInlines(toks, 0, len(toks), labelText)
	return Link{URL: url, Title: title, Content: content}, end, true
}

func (p *mdParser) parseLinkTail(src string, pos int, labelText string) (url, title string, end int, ok bool) {
	// Inline link: (url "title")
	if pos < len(src) && src[pos] == '(' {
		i := pos + 1
		for i < len(src) && isSpaceOrNewline(src[i]) {
			i++
		}
		if i < len(src) && src[i] == ')' {
			return "", "", i + 1, true
		}
		dest, rest, ok := parseLinkDestination(src[i:])
		if !ok {
			return "", "", 0, false
		}
		consumed := len(src[i:]) - len(rest)
		j := i + consumed
		ws := j
		for j < len(src) && isSpaceOrNewline(src[j]) {
			j++
		}
		var t string
		if j < len(src) && src[j] != ')' {
			if j == ws {
				return "", "", 0, false
			}
			var tok bool
			t, rest, tok = parseLinkTitle(src[j:])
			if !tok {
				return "", "", 0, false
			}
			j += len(src[j:]) - len(rest)
			for j < len(src) && isSpaceOrNewline(src[j]) {
				j++
			}
		}
		if j >= len(src) || src[j] != ')' {
			return "", "", 0, false
		}
		return unescapeMarkdown(dest), unescapeMarkdown(t), j + 1, true
	}
	// Full or collapsed reference link: [text][ref] or [ref][]
	if pos < len(src) && src[pos] == '[' {
		refLabel, closeRef, ok := matchBracketLabel(src, pos)
		if ok {
			key := refLabel
			if key == "" {
				key = labelText
			}
			if def, exists := p.linkDefs[normalizeLabel(key)]; exists {
				return def.url, def.title, closeRef + 1, true
			}
		}
	}
	// Shortcut reference link: [ref]
	if def, exists := p.linkDefs[normalizeLabel(labelText)]; exists {
		return def.url, def.title, pos, true
	}
	return "", "", 0, false
}

func matchBracketLabel(src string, openPos int) (string, int, bool) {
	depth := 0
	for i := openPos + 1; i < len(src); {
		switch src[i] {
		case '\\':
			i += 2
			continue
		case '`':
			if _, end, ok := parseCodeSpan(src, i); ok {
				i = end
				continue
			}
			i++
		case '[':
			depth++
			i++
		case ']':
			if depth == 0 {
				return src[openPos+1 : i], i, true
			}
			depth--
			i++
		default:
			i++
		}
	}
	return "", 0, false
}

func parseLinkDestination(s string) (url, rest string, ok bool) {
	if len(s) == 0 {
		return "", "", false
	}
	if s[0] == '<' {
		for i := 1; i < len(s); i++ {
			if s[i] == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if s[i] == '\n' || s[i] == '<' {
				return "", "", false
			}
			if s[i] == '>' {
				return s[1:i], s[i+1:], true
			}
		}
		return "", "", false
	}
	parens := 0
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]) {
			i += 2
			continue
		}
		if c <= ' ' || c == 0x7f {
			break
		}
		if c == '(' {
			parens++
		} else if c == ')' {
			if parens == 0 {
				break
			}
			parens--
		}
		i++
	}
	if i == 0 || parens != 0 {
		return "", "", false
	}
	return s[:i], s[i:], true
}

func parseLinkTitle(s string) (title, rest string, ok bool) {
	if len(s) == 0 {
		return "", "", false
	}
	var close byte
	switch s[0] {
	case '"':
		close = '"'
	case '\'':
		close = '\''
	case '(':
		close = ')'
	default:
		return "", "", false
	}
	for i := 1; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			continue
		}
		if s[i] == close {
			return s[1:i], s[i+1:], true
		}
	}
	return "", "", false
}

func parseAngleAutolink(src string, pos int) (url, label string, end int, ok bool) {
	close := strings.IndexAny(src[pos+1:], "> \t\n<")
	if close <= 0 || src[pos+1+close] != '>' {
		return "", "", 0, false
	}
	inner := src[pos+1 : pos+1+close]
	if colon := strings.IndexByte(inner, ':'); colon >= 2 && isValidScheme(inner[:colon]) {
		return inner, inner, pos + 1 + close + 1, true
	}
	if at := strings.IndexByte(inner, '@'); at > 0 && at < len(inner)-1 && strings.ContainsRune(inner[at+1:], '.') {
		return "mailto:" + inner, inner, pos + 1 + close + 1, true
	}
	return "", "", 0, false
}

func isValidScheme(s string) bool {
	if len(s) < 2 || len(s) > 32 || !isASCIILetter(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !isASCIILetter(c) && !isDigit(c) && c != '+' && c != '.' && c != '-' {
			return false
		}
	}
	return true
}

func isAutolinkBoundary(src string, pos int) bool {
	prev := src[pos-1]
	return prev == ' ' || prev == '\t' || prev == '\n' || prev == '(' || prev == '*' || prev == '_' || prev == '~'
}

func parseBareAutolink(src string, pos int) (url, label string, end int, ok bool) {
	rest := src[pos:]
	prefix := ""
	switch {
	case strings.HasPrefix(rest, "https://"):
		prefix = "https://"
	case strings.HasPrefix(rest, "http://"):
		prefix = "http://"
	case strings.HasPrefix(rest, "www."):
		prefix = "www."
	default:
		return "", "", 0, false
	}
	i := len(prefix)
	for i < len(rest) {
		c := rest[i]
		if c <= ' ' || c == '<' || c == '>' {
			break
		}
		i++
	}
	raw := rest[:i]
	// Trim GFM trailing punctuation.
	for len(raw) > len(prefix) {
		last := raw[len(raw)-1]
		if last == '?' || last == '!' || last == '.' || last == ',' || last == ':' || last == '*' || last == '_' || last == '~' {
			raw = raw[:len(raw)-1]
			continue
		}
		if last == ')' && strings.Count(raw, ")") > strings.Count(raw, "(") {
			raw = raw[:len(raw)-1]
			continue
		}
		break
	}
	host := raw[len(prefix):]
	if host == "" || !strings.ContainsRune(host, '.') {
		return "", "", 0, false
	}
	u := raw
	if prefix == "www." {
		u = "http://" + raw
	}
	return u, raw, pos + len(raw), true
}

func scanInlineHTMLTag(src string, pos int) (int, bool) {
	if pos+2 >= len(src) || src[pos] != '<' {
		return 0, false
	}
	s := src[pos:]
	// Comment <!-- ... -->
	if strings.HasPrefix(s, "<!--") {
		if end := strings.Index(s[4:], "-->"); end >= 0 {
			return pos + 4 + end + 3, true
		}
		return 0, false
	}
	// Processing instruction <? ... ?>
	if strings.HasPrefix(s, "<?") {
		if end := strings.Index(s[2:], "?>"); end >= 0 {
			return pos + 2 + end + 2, true
		}
		return 0, false
	}
	// CDATA <![CDATA[ ... ]]>
	if strings.HasPrefix(s, "<![CDATA[") {
		if end := strings.Index(s[9:], "]]>"); end >= 0 {
			return pos + 9 + end + 3, true
		}
		return 0, false
	}
	// Declaration <!NAME ...>
	if len(s) >= 3 && s[1] == '!' && isASCIILetter(s[2]) {
		if end := strings.IndexByte(s[3:], '>'); end >= 0 {
			return pos + 3 + end + 1, true
		}
		return 0, false
	}
	// Closing tag </name ...>
	if s[1] == '/' {
		j := 2
		if j >= len(s) || !isASCIILetter(s[j]) {
			return 0, false
		}
		for j < len(s) && (isASCIILetter(s[j]) || isDigit(s[j]) || s[j] == '-' || s[j] == ':' || s[j] == '.') {
			j++
		}
		for j < len(s) && isSpaceOrNewline(s[j]) {
			j++
		}
		if j < len(s) && s[j] == '>' {
			return pos + j + 1, true
		}
		return 0, false
	}
	// Opening or self-closing tag <name ...>
	if !isASCIILetter(s[1]) {
		return 0, false
	}
	j := 2
	for j < len(s) && (isASCIILetter(s[j]) || isDigit(s[j]) || s[j] == '-' || s[j] == ':' || s[j] == '.') {
		j++
	}
	inSingle, inDouble := false, false
	for j < len(s) {
		c := s[j]
		switch {
		case inDouble:
			if c == '"' {
				inDouble = false
			}
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case c == '"':
			inDouble = true
		case c == '\'':
			inSingle = true
		case c == '<':
			return 0, false
		case c == '>':
			return pos + j + 1, true
		}
		j++
	}
	return 0, false
}

func delimiterFlanking(src string, start, end int, ch byte) (canOpen, canClose bool) {
	before := rune(' ')
	if start > 0 {
		before, _ = utf8.DecodeLastRuneInString(src[:start])
	}
	after := rune(' ')
	if end < len(src) {
		after, _ = utf8.DecodeRuneInString(src[end:])
	}
	beforeSpace := unicode.IsSpace(before)
	afterSpace := unicode.IsSpace(after)
	beforePunct := isUnicodePunct(before)
	afterPunct := isUnicodePunct(after)

	leftFlanking := !afterSpace && (!afterPunct || beforeSpace || beforePunct)
	rightFlanking := !beforeSpace && (!beforePunct || afterSpace || afterPunct)

	if ch == '_' {
		canOpen = leftFlanking && (!rightFlanking || beforePunct)
		canClose = rightFlanking && (!leftFlanking || afterPunct)
		return
	}
	return leftFlanking, rightFlanking
}

func parseHTMLEntity(src string, pos int) (string, int, bool) {
	semi := strings.IndexByte(src[pos+1:min(pos+36, len(src))], ';')
	if semi <= 0 {
		return "", 0, false
	}
	raw := src[pos : pos+1+semi+1]
	decoded := html.UnescapeString(raw)
	if decoded == raw {
		return "", 0, false
	}
	return decoded, pos + 1 + semi + 1, true
}

func unescapeMarkdown(s string) string {
	if !strings.ContainsAny(s, `\&`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]) {
			b.WriteByte(s[i+1])
			i += 2
			continue
		}
		if s[i] == '&' {
			if decoded, end, ok := parseHTMLEntity(s, i); ok {
				b.WriteString(decoded)
				i = end
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func isASCIIPunct(c byte) bool {
	return (c >= '!' && c <= '/') || (c >= ':' && c <= '@') || (c >= '[' && c <= '`') || (c >= '{' && c <= '~')
}

func isUnicodePunct(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isSpaceOrNewline(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func findUnescapedByte(s string, target byte, from int) int {
	for i := from; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == target {
			return i
		}
	}
	return -1
}

// unclosedElement returns the name of the element tag opens and does not
// close (<details>, <b class="x">), but not of void elements.
func unclosedElement(tag string) (string, bool) {
	inner, ok := strings.CutPrefix(strings.TrimLeftFunc(tag, unicode.IsSpace), "<")
	if !ok {
		return "", false
	}
	n := 0
	for n < len(inner) && (inner[n] == '-' || inner[n] < utf8.RuneSelf && (unicode.IsLetter(rune(inner[n])) || unicode.IsDigit(rune(inner[n])))) {
		n++
	}
	if n == 0 || !unicode.IsLetter(rune(inner[0])) {
		return "", false
	}
	name := inner[:n]
	if isVoidElement(strings.ToLower(name)) {
		return "", false
	}
	end := strings.IndexByte(inner, '>')
	if end < 0 || strings.HasSuffix(inner[:end], "/") {
		return "", false
	}
	if strings.Contains(strings.ToLower(inner), "</"+strings.ToLower(name)) {
		return "", false
	}
	return name, true
}

// isClosingTag reports whether tag closes the element name (</details>).
func isClosingTag(tag, name string) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(tag), "</")
	if !ok {
		return false
	}
	rest, ok = strings.CutSuffix(rest, ">")
	return ok && strings.EqualFold(strings.TrimSpace(rest), name)
}

// finish joins neighboring texts, trims the end and splits text where
// patterns match.
func (p *mdParser) finish(in []Inline) []Inline {
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
	return applyPatterns(out, p.ext)
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

// slugs makes headings' anchors unique in a document.
type slugs map[string]int

// slug returns the next heading's slug as GitHub makes it: lower case,
// spaces as hyphens, punctuation left out, a count after repeats.
func (r slugs) slug(text string) string {
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
		if _, seen := r[slug]; !seen {
			break
		}
		r[base]++
		slug = base + "-" + strconv.Itoa(r[base])
	}
	r[slug] = 0
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
			case Underline:
				out = append(out, Underline{walk(it.Content)})
			case Mark:
				out = append(out, Mark{walk(it.Content), it.Color})
			case Superscript:
				out = append(out, Superscript{walk(it.Content)})
			case Subscript:
				out = append(out, Subscript{walk(it.Content)})
			case Small:
				out = append(out, Small{walk(it.Content)})
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
		if mdxExpr, _ := getMDX(); p.Pattern == mdxExpr {
			c.Text = s[groups[2]:groups[3]]
		}
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
