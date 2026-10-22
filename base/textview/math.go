package textview

import (
	"bytes"
	"strings"
	"unicode"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Formulas are read as markdown-rs reads them (math_text and math_flow):
// dollars around TeX in text ($x$, $$x$$), and a block between fences of
// two or more dollars on lines of their own. goldmark has no such syntax.

// mathInline is a formula in text.
type mathInline struct {
	ast.BaseInline
	tex string
}

var kindMathInline = ast.NewNodeKind("MathInline")

func (n *mathInline) Kind() ast.NodeKind { return kindMathInline }

func (n *mathInline) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"TeX": n.tex}, nil)
}

// mathInlineParser reads a run of dollars, the TeX, then a run of as many
// dollars. As in code spans, a line ending is a space, one space inside
// each end is dropped, and backslashes are TeX's. A pair of single
// dollars is prose ("costs $5 and $10") unless, as NSH decides, its TeX
// neither starts nor ends with a space once padded, and the second is not
// before a digit.
type mathInlineParser struct{}

func (p *mathInlineParser) Trigger() []byte { return []byte{'$'} }

func (p *mathInlineParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, seg := block.PeekLine()
	src := block.Source()
	// A run starts at its first dollar, not inside a run that opened
	// nothing.
	if at := seg.Start; at > 0 && src[at-1] == '$' && (at < 2 || src[at-2] != '\\') {
		return nil
	}
	n := dollars(line, 0)
	block.Advance(n)
	var tex []byte
	for {
		line, _ := block.PeekLine()
		if line == nil {
			return nil
		}
		for i := 0; i < len(line); {
			if line[i] != '$' {
				i++
				continue
			}
			m := dollars(line, i)
			if m != n {
				i += m
				continue
			}
			tex = append(tex, line[:i]...)
			value := padding(string(tex))
			if n == 1 && (value == "" || isSpaceByte(value[0]) || isSpaceByte(value[len(value)-1]) ||
				i+m < len(line) && isDigit(line[i+m])) {
				return nil
			}
			block.Advance(i + m)
			return &mathInline{tex: value}
		}
		tex = append(tex, line...)
		block.AdvanceLine()
	}
}

// padding returns a formula's TeX with its line endings as spaces, and
// one space dropped from each end when both have one.
func padding(tex string) string {
	tex = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(tex)
	if len(tex) >= 2 && tex[0] == ' ' && tex[len(tex)-1] == ' ' && strings.Trim(tex, " ") != "" {
		tex = tex[1 : len(tex)-1]
	}
	return tex
}

// dollars counts the dollars from at on.
func dollars(line []byte, at int) int {
	n := 0
	for at+n < len(line) && line[at+n] == '$' {
		n++
	}
	return n
}

func isSpaceByte(c byte) bool { return c < 0x80 && unicode.IsSpace(rune(c)) }

// mathBlock is a formula on lines of its own, between fences of dollars.
type mathBlock struct {
	ast.BaseBlock
	indent, fence int
}

var kindMathBlock = ast.NewNodeKind("MathBlock")

func (n *mathBlock) Kind() ast.NodeKind { return kindMathBlock }
func (n *mathBlock) IsRaw() bool        { return true }

func (n *mathBlock) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

// mathBlockParser opens a formula at two or more dollars; what follows
// them on the line may hold no dollar, else they are text ($$x$$). A
// line of at least as many dollars closes it; else it runs to the end of
// its container, as a fenced code block does.
type mathBlockParser struct{}

func (p *mathBlockParser) Trigger() []byte { return []byte{'$'} }

func (p *mathBlockParser) Open(_ ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || line[pos] != '$' {
		return nil, parser.NoChildren
	}
	n := dollars(line, pos)
	if n < 2 || bytes.IndexByte(line[pos+n:], '$') >= 0 {
		return nil, parser.NoChildren
	}
	reader.AdvanceToEOL()
	return &mathBlock{indent: pos, fence: n}, parser.NoChildren
}

func (p *mathBlockParser) Continue(node ast.Node, reader text.Reader, _ parser.Context) parser.State {
	line, segment := reader.PeekLine()
	m := node.(*mathBlock)
	if w, pos := util.IndentWidth(line, reader.LineOffset()); w < 4 {
		n := dollars(line, pos)
		if n >= m.fence && util.IsBlank(line[pos+n:]) {
			reader.AdvanceToEOL()
			return parser.Close
		}
	}
	// The fence's indentation is dropped from the formula's lines.
	pos, padding := util.IndentPositionPadding(line, reader.LineOffset(), segment.Padding, m.indent)
	if pos < 0 {
		pos = max(0, util.FirstNonSpacePosition(line)) - segment.Padding
		padding = 0
	}
	seg := text.NewSegmentPadding(segment.Start+pos, segment.Stop, padding)
	seg.ForceNewline = true
	node.Lines().Append(seg)
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}

func (p *mathBlockParser) Close(ast.Node, text.Reader, parser.Context) {}
func (p *mathBlockParser) CanInterruptParagraph() bool                 { return true }
func (p *mathBlockParser) CanAcceptIndentedLine() bool                 { return false }

// rewriteTeX rewrites TeX's own delimiters, which AI models often write
// formulas with, as dollars: \(…\) and \[…\] in text as $$…$$ (a pair
// of dollars, never prose), and \[…\] on lines of its own as a block
// between $$ fences. Markdown would read \( as an escaped parenthesis.
// Code, fenced or in text, is left as written.
func rewriteTeX(src string) string {
	if !strings.Contains(src, `\(`) && !strings.Contains(src, `\[`) {
		return src
	}
	lines := sourceLines(src)
	var out strings.Builder
	out.Grow(len(src) + 16)
	// Where the lines of text not written out yet start, or -1: a formula
	// in text may run over a paragraph's lines.
	text := -1
	var fence byte
	var fenceLen int
	for i := 0; i < len(lines); {
		l := lines[i]
		content := stripContainers(src[l.start:l.end])
		if fence != 0 {
			out.WriteString(src[l.start:l.next])
			if fenceCloses(content, fence, fenceLen) {
				fence = 0
			}
			i++
			continue
		}
		ch, n, opens := fenceOpens(content)
		blank := strings.TrimSpace(content) == ""
		display := strings.HasPrefix(content, `\[`)
		if (opens || blank || display) && text >= 0 {
			inlineTeX(&out, src[text:l.start])
			text = -1
		}
		if opens {
			fence, fenceLen = ch, n
		} else {
			if display {
				if last, ok := displayEnd(src, lines, i); ok {
					displayFormula(&out, src, lines, i, last)
					i = last + 1
					continue
				}
			}
			if !blank {
				if text < 0 {
					text = l.start
				}
				i++
				continue
			}
		}
		out.WriteString(src[l.start:l.next])
		i++
	}
	if text >= 0 {
		inlineTeX(&out, src[text:])
	}
	return out.String()
}

// sourceLine is a line of a source: its start, its end before the line
// ending, and where the next starts.
type sourceLine struct{ start, end, next int }

func sourceLines(src string) []sourceLine {
	var lines []sourceLine
	for start := 0; start < len(src); {
		end := strings.IndexByte(src[start:], '\n')
		if end < 0 {
			end = len(src)
		} else {
			end += start
		}
		lines = append(lines, sourceLine{start, end, min(end+1, len(src))})
		start = end + 1
	}
	return lines
}

// displayEnd returns the line that ends the display formula line first
// opens: the first ending with \], before a blank line.
func displayEnd(src string, lines []sourceLine, first int) (int, bool) {
	for i := first; i < len(lines); i++ {
		content := strings.TrimRightFunc(stripContainers(src[lines[i].start:lines[i].end]), unicode.IsSpace)
		if i == first {
			if len(content) < 2 {
				return 0, false
			}
			content = content[2:]
			// \[x\] and more: a formula in text.
			if strings.Contains(content, `\]`) && !strings.HasSuffix(content, `\]`) {
				return 0, false
			}
		} else if content == "" {
			return 0, false
		}
		if strings.HasSuffix(content, `\]`) && !strings.HasSuffix(content, `\\]`) {
			return i, true
		}
	}
	return 0, false
}

// displayFormula writes lines first to last, a display formula, as a
// block between $$ fences.
func displayFormula(out *strings.Builder, src string, lines []sourceLine, first, last int) {
	line := src[lines[first].start:lines[first].end]
	opener := max(strings.Index(line, `\[`), 0)
	// The fences go where the formula's lines are: in the quote or list
	// item it is in, its marker as spaces, else it would start an item.
	prefix := []rune(line[:opener])
	for k, c := range prefix {
		if c != '>' && !unicode.IsSpace(c) {
			prefix[k] = ' '
		}
	}
	out.WriteString(line[:opener])
	out.WriteString("$$\n")
	for i := first; i <= last; i++ {
		body := src[lines[i].start:lines[i].end]
		if i == first {
			body = body[opener+2:]
		}
		if i == last {
			body = strings.TrimRightFunc(body, unicode.IsSpace)
			body = body[:len(body)-2]
		}
		if i == first {
			if body = strings.TrimSpace(body); body != "" {
				out.WriteString(string(prefix))
				out.WriteString(body)
				out.WriteByte('\n')
			}
		} else if strings.TrimSpace(stripContainers(body)) != "" {
			out.WriteString(strings.TrimRightFunc(body, unicode.IsSpace))
			out.WriteByte('\n')
		}
	}
	out.WriteString(string(prefix))
	out.WriteString("$$")
	if lines[last].end < len(src) {
		out.WriteByte('\n')
	}
}

// inlineTeX writes text, lines of a block, with its formulas \(…\) and
// \[…\] as $$…$$.
func inlineTeX(out *strings.Builder, text string) {
	spans, open, _ := inlineCode(text, false)
	copied := 0
	for i := 0; i < len(text); {
		if s := spanAt(spans, i); s >= 0 {
			i = spans[s][1]
			continue
		}
		if open >= 0 && i >= open {
			break
		}
		if text[i] != '\\' {
			i++
			continue
		}
		if end, ok := texEnd(text, i); ok {
			body := text[i+2 : end-2]
			// Dollars inside would end the formula early; a blank line
			// ends the paragraph.
			if strings.TrimSpace(body) != "" && !strings.Contains(body, "$") && !strings.Contains(body, "\n\n") {
				out.WriteString(text[copied:i])
				out.WriteString("$$" + body + "$$")
				copied, i = end, end
				continue
			}
		}
		// An escape: the escaped character is no delimiter.
		i += 2
	}
	out.WriteString(text[copied:])
}

// texEnd returns the end, after its closer, of the TeX formula that \(
// or \[ opens at at, and false when nothing closes it.
func texEnd(text string, at int) (int, bool) {
	if at+1 >= len(text) {
		return 0, false
	}
	var closer string
	switch text[at+1] {
	case '(':
		closer = `\)`
	case '[':
		closer = `\]`
	default:
		return 0, false
	}
	i := strings.Index(text[at+2:], closer)
	if i < 0 {
		return 0, false
	}
	return at + 2 + i + 2, true
}

// spanAt returns the index of the span starting at i, or -1.
func spanAt(spans [][2]int, i int) int {
	for k, s := range spans {
		if s[0] == i {
			return k
		}
	}
	return -1
}
