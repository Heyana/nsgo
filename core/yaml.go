package core

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// parseYAML reads a YAML 1.2 (or JSON) document into plain Go values:
// map[string]any, []any, string, int, float64, bool, or nil.
// It avoids go.yaml.in/yaml/v3 (whose init pulls regexp into every app)
// and has no package initializer, so apps that never call ParseTheme or
// ParseTailwind link none of it.
func parseYAML(data []byte) (any, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("invalid UTF-8")
	}
	s := string(data)
	s = strings.TrimPrefix(s, "\uFEFF")
	p := &yamlParser{src: s}
	p.skipBlockTrivia()
	if p.eof() {
		return nil, nil
	}
	if p.consumeDocStart() {
		p.skipBlockTrivia()
		if p.eof() || p.atDocEnd() {
			return nil, nil
		}
	}
	val, err := p.parseValue(-1)
	if err != nil {
		return nil, err
	}
	p.skipBlockTrivia()
	if p.atDocEnd() {
		p.skipLine()
		p.skipBlockTrivia()
	}
	if !p.eof() {
		return nil, p.errorf("unexpected trailing content")
	}
	return val, nil
}

type yamlParser struct {
	src string
	pos int
}

func (p *yamlParser) eof() bool { return p.pos >= len(p.src) }

func (p *yamlParser) peek() byte {
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

func (p *yamlParser) errorf(format string, args ...any) error {
	line := 1 + strings.Count(p.src[:p.pos], "\n")
	return fmt.Errorf("line %d: %s", line, fmt.Sprintf(format, args...))
}

func (p *yamlParser) column() int {
	nl := strings.LastIndexByte(p.src[:p.pos], '\n')
	if nl < 0 {
		return p.pos
	}
	return p.pos - nl - 1
}

func (p *yamlParser) skipSpaces() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

func (p *yamlParser) skipLine() {
	for p.pos < len(p.src) && p.src[p.pos] != '\n' {
		p.pos++
	}
	if p.pos < len(p.src) && p.src[p.pos] == '\n' {
		p.pos++
	}
}

// skipBlockTrivia skips blank lines, leading indentation on blank/comment lines, and comments.
func (p *yamlParser) skipBlockTrivia() {
	for !p.eof() {
		save := p.pos
		for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\r') {
			p.pos++
		}
		if p.eof() {
			return
		}
		switch p.src[p.pos] {
		case '\n':
			p.pos++
		case '#':
			p.skipLine()
		default:
			// Restore indentation if we started at the beginning of a line.
			if save == 0 || p.src[save-1] == '\n' {
				p.pos = save
			}
			return
		}
	}
}

// skipFlowTrivia skips spaces, tabs, newlines, and # comments inside flow collections.
func (p *yamlParser) skipFlowTrivia() {
	for !p.eof() {
		switch p.src[p.pos] {
		case ' ', '\t', '\r', '\n':
			p.pos++
		case '#':
			p.skipLine()
		default:
			return
		}
	}
}

func (p *yamlParser) consumeDocStart() bool {
	if p.column() != 0 || !strings.HasPrefix(p.src[p.pos:], "---") {
		return false
	}
	end := p.pos + 3
	if end < len(p.src) && p.src[end] != ' ' && p.src[end] != '\t' && p.src[end] != '\r' && p.src[end] != '\n' {
		return false
	}
	p.pos = end
	p.skipInlineCommentAndNewline()
	return true
}

func (p *yamlParser) atDocEnd() bool {
	if p.column() != 0 || !strings.HasPrefix(p.src[p.pos:], "...") {
		return false
	}
	end := p.pos + 3
	return end == len(p.src) || p.src[end] == ' ' || p.src[end] == '\t' || p.src[end] == '\r' || p.src[end] == '\n'
}

func (p *yamlParser) skipInlineCommentAndNewline() {
	p.skipSpaces()
	if !p.eof() && p.src[p.pos] == '#' {
		p.skipLine()
		return
	}
	if !p.eof() && p.src[p.pos] == '\r' {
		p.pos++
	}
	if !p.eof() && p.src[p.pos] == '\n' {
		p.pos++
	}
}

// currentLineIndent skips leading spaces on the current line and returns the indent column, or -1 at EOF / doc end.
func (p *yamlParser) currentLineIndent() int {
	p.skipBlockTrivia()
	if p.eof() || p.atDocEnd() {
		return -1
	}
	col := 0
	for p.pos+col < len(p.src) && p.src[p.pos+col] == ' ' {
		col++
	}
	if p.pos+col < len(p.src) && p.src[p.pos+col] == '\t' {
		// Allow tab-indented JSON flow structures.
		for p.pos+col < len(p.src) && (p.src[p.pos+col] == ' ' || p.src[p.pos+col] == '\t') {
			col++
		}
	}
	return col
}

func (p *yamlParser) parseValue(minIndent int) (any, error) {
	p.skipBlockTrivia()
	if p.eof() || p.atDocEnd() {
		return nil, nil
	}
	indent := p.currentLineIndent()
	if indent < 0 || indent <= minIndent {
		return nil, nil
	}
	p.pos += indent
	c := p.peek()
	switch c {
	case '{':
		v, err := p.parseFlowMap()
		if err != nil {
			return nil, err
		}
		p.skipInlineCommentAndNewline()
		return v, nil
	case '[':
		v, err := p.parseFlowSeq()
		if err != nil {
			return nil, err
		}
		p.skipInlineCommentAndNewline()
		return v, nil
	}
	if p.atBlockSeqItem() {
		p.pos -= indent
		return p.parseBlockSeq(indent)
	}
	if p.looksLikeBlockMappingLine() {
		p.pos -= indent
		return p.parseBlockMap(indent)
	}
	v, err := p.parseInlineValue(indent)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (p *yamlParser) atBlockSeqItem() bool {
	if p.peek() != '-' {
		return false
	}
	if p.pos+1 >= len(p.src) {
		return true
	}
	next := p.src[p.pos+1]
	return next == ' ' || next == '\t' || next == '\r' || next == '\n'
}

// looksLikeBlockMappingLine reports whether the current line (starting at p.pos) has a key: separator.
func (p *yamlParser) looksLikeBlockMappingLine() bool {
	i := p.pos
	if i >= len(p.src) {
		return false
	}
	switch p.src[i] {
	case '"':
		i++
		for i < len(p.src) && p.src[i] != '\n' {
			if p.src[i] == '\\' {
				i += 2
				continue
			}
			if p.src[i] == '"' {
				i++
				break
			}
			i++
		}
		for i < len(p.src) && (p.src[i] == ' ' || p.src[i] == '\t') {
			i++
		}
		return i < len(p.src) && p.src[i] == ':' && isMappingColonAfter(p.src, i)
	case '\'':
		i++
		for i < len(p.src) && p.src[i] != '\n' {
			if p.src[i] == '\'' {
				if i+1 < len(p.src) && p.src[i+1] == '\'' {
					i += 2
					continue
				}
				i++
				break
			}
			i++
		}
		for i < len(p.src) && (p.src[i] == ' ' || p.src[i] == '\t') {
			i++
		}
		return i < len(p.src) && p.src[i] == ':' && isMappingColonAfter(p.src, i)
	}
	for i < len(p.src) && p.src[i] != '\n' && p.src[i] != '\r' {
		if p.src[i] == '#' && (i == p.pos || p.src[i-1] == ' ' || p.src[i-1] == '\t') {
			return false
		}
		if p.src[i] == ':' && isMappingColonAfter(p.src, i) {
			return true
		}
		i++
	}
	return false
}

func isMappingColonAfter(s string, colon int) bool {
	if colon+1 >= len(s) {
		return true
	}
	c := s[colon+1]
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func (p *yamlParser) parseBlockMap(indent int) (map[string]any, error) {
	out := map[string]any{}
	for {
		ind := p.currentLineIndent()
		if ind < 0 || ind < indent {
			break
		}
		if ind > indent {
			return nil, p.errorf("unexpected indentation")
		}
		p.pos += ind
		if p.atBlockSeqItem() {
			p.pos -= ind
			break
		}
		key, err := p.parseMappingKey(false)
		if err != nil {
			return nil, err
		}
		p.skipSpaces()
		if p.eof() || p.src[p.pos] != ':' {
			return nil, p.errorf("expected ':' after key %q", key)
		}
		p.pos++ // skip ':'
		p.skipSpaces()
		if p.eof() || p.src[p.pos] == '\n' || p.src[p.pos] == '\r' || p.src[p.pos] == '#' {
			p.skipInlineCommentAndNewline()
			childIndent := p.currentLineIndent()
			if childIndent > indent {
				val, err := p.parseValue(indent)
				if err != nil {
					return nil, err
				}
				out[key] = val
			} else if childIndent == indent {
				// Block sequences in YAML may align '-' with the parent mapping key.
				save := p.pos
				p.pos += childIndent
				isSeq := p.atBlockSeqItem()
				p.pos = save
				if isSeq {
					val, err := p.parseBlockSeq(childIndent)
					if err != nil {
						return nil, err
					}
					out[key] = val
				} else {
					out[key] = nil
				}
			} else {
				out[key] = nil
			}
			continue
		}
		val, err := p.parseInlineValue(indent)
		if err != nil {
			return nil, err
		}
		out[key] = val
	}
	return out, nil
}

func (p *yamlParser) parseBlockSeq(indent int) ([]any, error) {
	var out []any
	for {
		ind := p.currentLineIndent()
		if ind < 0 || ind < indent {
			break
		}
		if ind > indent {
			return nil, p.errorf("unexpected indentation in sequence")
		}
		p.pos += ind
		if !p.atBlockSeqItem() {
			p.pos -= ind
			break
		}
		p.pos++ // skip '-'
		p.skipSpaces()
		if p.eof() || p.src[p.pos] == '\n' || p.src[p.pos] == '\r' || p.src[p.pos] == '#' {
			p.skipInlineCommentAndNewline()
			val, err := p.parseValue(indent)
			if err != nil {
				return nil, err
			}
			out = append(out, val)
			continue
		}
		val, err := p.parseInlineValue(indent)
		if err != nil {
			return nil, err
		}
		out = append(out, val)
	}
	return out, nil
}

func (p *yamlParser) parseInlineValue(parentIndent int) (any, error) {
	switch p.peek() {
	case '{':
		v, err := p.parseFlowMap()
		if err != nil {
			return nil, err
		}
		p.skipInlineCommentAndNewline()
		return v, nil
	case '[':
		v, err := p.parseFlowSeq()
		if err != nil {
			return nil, err
		}
		p.skipInlineCommentAndNewline()
		return v, nil
	case '"':
		s, err := p.parseDoubleQuoted()
		if err != nil {
			return nil, err
		}
		p.skipSpaces()
		if !p.eof() && p.src[p.pos] != '\n' && p.src[p.pos] != '\r' && p.src[p.pos] != '#' {
			return nil, p.errorf("unexpected characters after quoted string")
		}
		p.skipInlineCommentAndNewline()
		return s, nil
	case '\'':
		s, err := p.parseSingleQuoted()
		if err != nil {
			return nil, err
		}
		p.skipSpaces()
		if !p.eof() && p.src[p.pos] != '\n' && p.src[p.pos] != '\r' && p.src[p.pos] != '#' {
			return nil, p.errorf("unexpected characters after quoted string")
		}
		p.skipInlineCommentAndNewline()
		return s, nil
	default:
		_ = parentIndent
		start := p.pos
		for p.pos < len(p.src) && p.src[p.pos] != '\n' && p.src[p.pos] != '\r' {
			if p.src[p.pos] == '#' && p.pos > start && (p.src[p.pos-1] == ' ' || p.src[p.pos-1] == '\t') {
				break
			}
			p.pos++
		}
		raw := strings.TrimRight(p.src[start:p.pos], " \t")
		p.skipInlineCommentAndNewline()
		return parsePlainScalar(raw), nil
	}
}

func (p *yamlParser) parseFlowMap() (map[string]any, error) {
	p.pos++ // skip '{'
	out := map[string]any{}
	p.skipFlowTrivia()
	if !p.eof() && p.src[p.pos] == '}' {
		p.pos++
		return out, nil
	}
	for {
		p.skipFlowTrivia()
		if p.eof() {
			return nil, p.errorf("unterminated mapping")
		}
		if p.src[p.pos] == '}' {
			p.pos++
			return out, nil
		}
		key, err := p.parseMappingKey(true)
		if err != nil {
			return nil, err
		}
		p.skipFlowTrivia()
		if p.eof() || p.src[p.pos] != ':' {
			return nil, p.errorf("expected ':' after key %q", key)
		}
		p.pos++ // skip ':'
		p.skipFlowTrivia()
		val, err := p.parseFlowValue()
		if err != nil {
			return nil, err
		}
		out[key] = val
		p.skipFlowTrivia()
		if p.eof() {
			return nil, p.errorf("unterminated mapping")
		}
		switch p.src[p.pos] {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return out, nil
		default:
			return nil, p.errorf("expected ',' or '}' in mapping")
		}
	}
}

func (p *yamlParser) parseFlowSeq() ([]any, error) {
	p.pos++ // skip '['
	var out []any
	p.skipFlowTrivia()
	if !p.eof() && p.src[p.pos] == ']' {
		p.pos++
		return out, nil
	}
	for {
		p.skipFlowTrivia()
		if p.eof() {
			return nil, p.errorf("unterminated sequence")
		}
		if p.src[p.pos] == ']' {
			p.pos++
			return out, nil
		}
		val, err := p.parseFlowValue()
		if err != nil {
			return nil, err
		}
		out = append(out, val)
		p.skipFlowTrivia()
		if p.eof() {
			return nil, p.errorf("unterminated sequence")
		}
		switch p.src[p.pos] {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return out, nil
		default:
			return nil, p.errorf("expected ',' or ']' in sequence")
		}
	}
}

func (p *yamlParser) parseFlowValue() (any, error) {
	p.skipFlowTrivia()
	if p.eof() {
		return nil, p.errorf("unexpected end of input")
	}
	switch p.src[p.pos] {
	case '{':
		return p.parseFlowMap()
	case '[':
		return p.parseFlowSeq()
	case '"':
		return p.parseDoubleQuoted()
	case '\'':
		return p.parseSingleQuoted()
	case ',', '}', ']':
		return nil, nil
	default:
		start := p.pos
		for p.pos < len(p.src) {
			c := p.src[p.pos]
			if c == ',' || c == '}' || c == ']' || c == '\n' || c == '\r' {
				break
			}
			if c == '#' && p.pos > start && (p.src[p.pos-1] == ' ' || p.src[p.pos-1] == '\t') {
				break
			}
			p.pos++
		}
		raw := strings.TrimRight(p.src[start:p.pos], " \t")
		return parsePlainScalar(raw), nil
	}
}

func (p *yamlParser) parseMappingKey(flow bool) (string, error) {
	if p.eof() {
		return "", p.errorf("expected key")
	}
	switch p.src[p.pos] {
	case '"':
		return p.parseDoubleQuoted()
	case '\'':
		return p.parseSingleQuoted()
	}
	start := p.pos
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '\n' || c == '\r' {
			break
		}
		if flow && (c == ',' || c == '}' || c == ']' || c == '{' || c == '[') {
			break
		}
		if c == ':' && (flow || isMappingColonAfter(p.src, p.pos)) {
			break
		}
		p.pos++
	}
	key := strings.TrimRight(p.src[start:p.pos], " \t")
	if key == "" {
		return "", p.errorf("empty mapping key")
	}
	return key, nil
}

func (p *yamlParser) parseDoubleQuoted() (string, error) {
	p.pos++ // skip opening '"'
	var b strings.Builder
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		p.pos++
		if c == '"' {
			return b.String(), nil
		}
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		if p.eof() {
			return "", p.errorf("unterminated escape sequence")
		}
		esc := p.src[p.pos]
		p.pos++
		switch esc {
		case '"', '\\', '/':
			b.WriteByte(esc)
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case '0':
			b.WriteByte(0)
		case 'x':
			r, err := p.parseHexRune(2)
			if err != nil {
				return "", err
			}
			b.WriteRune(r)
		case 'u':
			r, err := p.parseHexRune(4)
			if err != nil {
				return "", err
			}
			if utf16.IsSurrogate(r) && strings.HasPrefix(p.src[p.pos:], `\u`) {
				p.pos += 2
				r2, err := p.parseHexRune(4)
				if err != nil {
					return "", err
				}
				r = utf16.DecodeRune(r, r2)
			}
			b.WriteRune(r)
		case 'U':
			r, err := p.parseHexRune(8)
			if err != nil {
				return "", err
			}
			b.WriteRune(r)
		default:
			return "", p.errorf("unknown escape \\%c", esc)
		}
	}
	return "", p.errorf("unterminated double-quoted string")
}

func (p *yamlParser) parseHexRune(digits int) (rune, error) {
	if p.pos+digits > len(p.src) {
		return 0, p.errorf("truncated hex escape")
	}
	v, err := strconv.ParseUint(p.src[p.pos:p.pos+digits], 16, 32)
	if err != nil {
		return 0, p.errorf("invalid hex escape")
	}
	p.pos += digits
	return rune(v), nil
}

func (p *yamlParser) parseSingleQuoted() (string, error) {
	p.pos++ // skip opening '\''
	var b strings.Builder
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		p.pos++
		if c == '\'' {
			if p.pos < len(p.src) && p.src[p.pos] == '\'' {
				b.WriteByte('\'')
				p.pos++
				continue
			}
			return b.String(), nil
		}
		b.WriteByte(c)
	}
	return "", p.errorf("unterminated single-quoted string")
}

func parsePlainScalar(s string) any {
	switch s {
	case "", "null", "Null", "NULL", "~":
		return nil
	case "true", "True", "TRUE":
		return true
	case "false", "False", "FALSE":
		return false
	}
	if i, err := strconv.ParseInt(s, 0, 64); err == nil {
		return int(i)
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		return f
	}
	return s
}

func yamlStringMap(v any, what string) (map[string]string, error) {
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a map", what)
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		if val == nil {
			out[k] = ""
			continue
		}
		switch s := val.(type) {
		case string:
			out[k] = s
		case int, float64, bool:
			out[k] = fmt.Sprint(s)
		default:
			return nil, fmt.Errorf("%s.%s must be a string", what, k)
		}
	}
	return out, nil
}
