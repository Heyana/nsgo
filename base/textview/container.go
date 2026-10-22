package textview

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// container is a :::name block of an Extension's: its blocks are parsed
// as the document's are. goldmark has no such syntax.
type container struct {
	ast.BaseBlock
	name   string
	attrs  map[string]string
	colons int
}

var kindContainer = ast.NewNodeKind("Container")

func (n *container) Kind() ast.NodeKind { return kindContainer }

func (n *container) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Name": n.name}, nil)
}

// containerParser opens containers of its names: three or more colons, the
// name, an optional [label] and {attributes}. A line of at least as many
// colons closes one.
type containerParser struct{ names map[string]string } // container → node name

func (p *containerParser) Trigger() []byte { return []byte{':'} }

func (p *containerParser) Open(_ ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 {
		return nil, parser.NoChildren
	}
	rest := strings.TrimRight(string(line[pos:]), " \t\r\n")
	colons := len(rest) - len(strings.TrimLeft(rest, ":"))
	if colons < 3 {
		return nil, parser.NoChildren
	}
	rest = rest[colons:]
	end := strings.IndexFunc(rest, func(c rune) bool {
		return !(c == '-' || c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z')
	})
	if end < 0 {
		end = len(rest)
	}
	name, ok := p.names[rest[:end]]
	if !ok {
		return nil, parser.NoChildren
	}
	n := &container{name: name, colons: colons}
	rest = rest[end:]
	if strings.HasPrefix(rest, "[") {
		if close := strings.IndexByte(rest, ']'); close > 0 {
			n.set("label", rest[1:close])
			rest = rest[close+1:]
		}
	}
	if strings.HasPrefix(rest, "{") {
		if close := strings.IndexByte(rest, '}'); close > 0 {
			n.parseAttrs(rest[1:close])
		}
	}
	reader.AdvanceToEOL()
	return n, parser.HasChildren
}

func (n *container) set(k, v string) {
	if n.attrs == nil {
		n.attrs = map[string]string{}
	}
	n.attrs[k] = v
}

// parseAttrs reads key=value, key="quoted value", .class and #id.
func (n *container) parseAttrs(s string) {
	for {
		if s = strings.TrimLeft(s, " \t"); s == "" {
			return
		}
		end := strings.IndexAny(s, " \t=")
		if end >= 0 && s[end] == '=' {
			key, value := s[:end], s[end+1:]
			if strings.HasPrefix(value, `"`) {
				value, s, _ = strings.Cut(value[1:], `"`)
			} else if i := strings.IndexAny(value, " \t"); i >= 0 {
				value, s = value[:i], value[i:]
			} else {
				s = ""
			}
			n.set(key, value)
			continue
		}
		if end < 0 {
			end = len(s)
		}
		word := s[:end]
		s = s[end:]
		switch {
		case strings.HasPrefix(word, "."):
			n.set("class", strings.TrimSpace(n.attrs["class"]+" "+word[1:]))
		case strings.HasPrefix(word, "#"):
			n.set("id", word[1:])
		default:
			n.set(word, "")
		}
	}
}

func (p *containerParser) Continue(node ast.Node, reader text.Reader, _ parser.Context) parser.State {
	line, _ := reader.PeekLine()
	trimmed := util.TrimRightSpace(util.TrimLeftSpace(line))
	if len(trimmed) >= node.(*container).colons && len(strings.Trim(string(trimmed), ":")) == 0 {
		reader.AdvanceToEOL()
		return parser.Close
	}
	return parser.Continue | parser.HasChildren
}

func (p *containerParser) Close(ast.Node, text.Reader, parser.Context) {}
func (p *containerParser) CanInterruptParagraph() bool                 { return true }
func (p *containerParser) CanAcceptIndentedLine() bool                 { return false }
