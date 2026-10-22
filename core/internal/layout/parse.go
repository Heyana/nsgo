package layout

import (
	"fmt"
	"math"
	"strings"
)

// This file parses CSS grid values (track lists and placements) the way
// Taffy's FromStr implementations do through cssparser, for callers that
// keep grid styles as strings and for the fixture tests.

type cssTokenKind uint8

const (
	cssIdent cssTokenKind = iota
	cssNumber
	cssPercentage
	cssDimension
	cssFunction
	cssSquareBlock
	cssComma
	cssDelim
)

// cssToken is a CSS token, as cssparser makes them.
type cssToken struct {
	kind cssTokenKind
	// ident is an ident's name, a dimension's unit or a function's name.
	ident string
	// value is a number's or dimension's value, or a percentage's unit
	// value (the percentage divided by 100).
	value float32
	// isInt says whether a numeric token was written as an integer, intValue
	// being its value saturated to int32.
	isInt    bool
	intValue int32
	// block holds the tokens inside a function or a [] block.
	block []cssToken
	delim byte
}

// tokenizeCSS splits a value into tokens, dropping whitespace.
func tokenizeCSS(s string) ([]cssToken, error) {
	toks, rest, err := tokenizeUntil(s, 0)
	if err != nil {
		return nil, err
	}
	if rest != "" {
		return nil, fmt.Errorf("unbalanced %q", s)
	}
	return toks, nil
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isNameChar(c byte) bool { return isNameStart(c) || c == '-' || (c >= '0' && c <= '9') }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func startsIdent(s string) bool {
	if s == "" {
		return false
	}
	if isNameStart(s[0]) {
		return true
	}
	return s[0] == '-' && len(s) > 1 && (isNameStart(s[1]) || s[1] == '-')
}

func startsNumber(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	if c == '+' || c == '-' {
		s = s[1:]
		if s == "" {
			return false
		}
		c = s[0]
	}
	return isDigit(c) || (c == '.' && len(s) > 1 && isDigit(s[1]))
}

func tokenizeUntil(s string, closer byte) ([]cssToken, string, error) {
	var toks []cssToken
	for {
		s = strings.TrimLeft(s, " \t\n\r\f")
		if s == "" {
			if closer != 0 {
				return nil, "", fmt.Errorf("missing %q", closer)
			}
			return toks, "", nil
		}
		c := s[0]
		switch {
		case c == closer:
			return toks, s[1:], nil
		case c == ',':
			toks = append(toks, cssToken{kind: cssComma})
			s = s[1:]
		case c == '[':
			inner, rest, err := tokenizeUntil(s[1:], ']')
			if err != nil {
				return nil, "", err
			}
			toks = append(toks, cssToken{kind: cssSquareBlock, block: inner})
			s = rest
		case startsNumber(s):
			tok, rest := consumeNumeric(s)
			toks = append(toks, tok)
			s = rest
		case startsIdent(s):
			i := 1
			for i < len(s) && isNameChar(s[i]) {
				i++
			}
			name := s[:i]
			s = s[i:]
			if s != "" && s[0] == '(' {
				inner, rest, err := tokenizeUntil(s[1:], ')')
				if err != nil {
					return nil, "", err
				}
				toks = append(toks, cssToken{kind: cssFunction, ident: name, block: inner})
				s = rest
			} else {
				toks = append(toks, cssToken{kind: cssIdent, ident: name})
			}
		default:
			toks = append(toks, cssToken{kind: cssDelim, delim: c})
			s = s[1:]
		}
	}
}

// consumeNumeric follows cssparser's consume_numeric, which accumulates the
// digits in float64.
func consumeNumeric(s string) (cssToken, string) {
	sign := 1.0
	if s[0] == '+' || s[0] == '-' {
		if s[0] == '-' {
			sign = -1
		}
		s = s[1:]
	}
	integral := 0.0
	for s != "" && isDigit(s[0]) {
		integral = integral*10 + float64(s[0]-'0')
		s = s[1:]
	}
	isInt := true
	fractional := 0.0
	if len(s) > 1 && s[0] == '.' && isDigit(s[1]) {
		isInt = false
		s = s[1:]
		factor := 0.1
		for s != "" && isDigit(s[0]) {
			fractional += float64(s[0]-'0') * factor
			factor *= 0.1
			s = s[1:]
		}
	}
	value := sign * (integral + fractional)
	if len(s) > 1 && (s[0] == 'e' || s[0] == 'E') &&
		(isDigit(s[1]) || ((s[1] == '+' || s[1] == '-') && len(s) > 2 && isDigit(s[2]))) {
		isInt = false
		s = s[1:]
		expSign := 1.0
		if s[0] == '+' || s[0] == '-' {
			if s[0] == '-' {
				expSign = -1
			}
			s = s[1:]
		}
		exp := 0.0
		for s != "" && isDigit(s[0]) {
			exp = exp*10 + float64(s[0]-'0')
			s = s[1:]
		}
		value *= math.Pow(10, expSign*exp)
	}
	tok := cssToken{isInt: isInt}
	if isInt {
		switch {
		case value >= math.MaxInt32:
			tok.intValue = math.MaxInt32
		case value <= math.MinInt32:
			tok.intValue = math.MinInt32
		default:
			tok.intValue = int32(value)
		}
	}
	if s != "" && s[0] == '%' {
		tok.kind = cssPercentage
		tok.value = float32(value / 100)
		return tok, s[1:]
	}
	tok.value = float32(value)
	if startsIdent(s) {
		i := 1
		for i < len(s) && isNameChar(s[i]) {
			i++
		}
		tok.kind = cssDimension
		tok.ident = s[:i]
		return tok, s[i:]
	}
	tok.kind = cssNumber
	return tok, s
}

// cssSingle tokenizes s and requires it to be a single token.
func cssSingle(s string) (cssToken, error) {
	toks, err := tokenizeCSS(s)
	if err != nil {
		return cssToken{}, err
	}
	if len(toks) != 1 {
		return cssToken{}, fmt.Errorf("want one token in %q", s)
	}
	return toks[0], nil
}

// lengthFrom parses a px dimension or a percentage token. A unitless 0 is
// 0px, as CSS allows.
func lengthFrom(tok cssToken) (Length, bool) {
	switch {
	case tok.kind == cssPercentage:
		return Percent(tok.value), true
	case tok.kind == cssDimension && tok.ident == "px":
		return Px(tok.value), true
	case tok.kind == cssNumber && tok.value == 0:
		return Px(0), true
	}
	return Length{}, false
}

// fitContentFrom parses fit-content(<length-percentage>).
func fitContentFrom(tok cssToken) (Length, bool) {
	if tok.kind != cssFunction || tok.ident != "fit-content" || len(tok.block) != 1 {
		return Length{}, false
	}
	arg := tok.block[0]
	switch {
	case arg.kind == cssPercentage:
		return FitContentPercent(arg.value), true
	case arg.kind == cssDimension && arg.ident == "px":
		return FitContentPx(arg.value), true
	}
	return Length{}, false
}

// maxTrackFrom parses a max track sizing function.
func maxTrackFrom(tok cssToken) (Length, bool) {
	if l, ok := lengthFrom(tok); ok {
		return l, true
	}
	switch tok.kind {
	case cssDimension:
		if tok.ident == "fr" && !math.Signbit(float64(tok.value)) {
			return Fr(tok.value), true
		}
	case cssIdent:
		switch tok.ident {
		case "auto":
			return Auto, true
		case "min-content":
			return MinContent, true
		case "max-content":
			return MaxContent, true
		}
	case cssFunction:
		return fitContentFrom(tok)
	}
	return Length{}, false
}

// minTrackFrom parses a min track sizing function.
func minTrackFrom(tok cssToken) (Length, bool) {
	if l, ok := lengthFrom(tok); ok {
		return l, true
	}
	if tok.kind == cssIdent {
		switch tok.ident {
		case "auto":
			return Auto, true
		case "min-content":
			return MinContent, true
		case "max-content":
			return MaxContent, true
		}
	}
	return Length{}, false
}

// trackFrom parses a track sizing function: minmax() or a single size.
func trackFrom(tok cssToken) (TrackSizing, bool) {
	if tok.kind == cssFunction && tok.ident == "minmax" {
		b := tok.block
		if len(b) != 3 || b[1].kind != cssComma {
			return TrackSizing{}, false
		}
		min, ok1 := minTrackFrom(b[0])
		max, ok2 := maxTrackFrom(b[2])
		return MinMax(min, max), ok1 && ok2
	}
	max, ok := maxTrackFrom(tok)
	if !ok {
		return TrackSizing{}, false
	}
	return Track(max), true
}

// trackListFrom parses tracks with line name groups, as Taffy's
// GridTemplateTracks: one name group per line, empty when unnamed. With
// repeat, entries may be repeat() functions.
func trackListFrom(toks []cssToken, repeat bool) ([]GridTemplateComponent, [][]string, error) {
	var comps []GridTemplateComponent
	var names [][]string
	lineNames := func() []string {
		if len(toks) > 0 && toks[0].kind == cssSquareBlock {
			var group []string
			for _, t := range toks[0].block {
				if t.kind != cssIdent {
					return nil
				}
				group = append(group, t.ident)
			}
			toks = toks[1:]
			return group
		}
		return nil
	}
	names = append(names, lineNames())
	for len(toks) > 0 {
		tok := toks[0]
		toks = toks[1:]
		if repeat && tok.kind == cssFunction && tok.ident == "repeat" {
			r, err := repetitionFrom(tok.block)
			if err != nil {
				return nil, nil, err
			}
			comps = append(comps, GridTemplateComponent{Repeat: r})
		} else {
			t, ok := trackFrom(tok)
			if !ok {
				return nil, nil, fmt.Errorf("bad track %+v", tok)
			}
			comps = append(comps, GridTemplateComponent{Track: t})
		}
		names = append(names, lineNames())
	}
	if len(comps) == 0 {
		return nil, nil, fmt.Errorf("empty track list")
	}
	return comps, names, nil
}

// repetitionFrom parses the arguments of repeat().
func repetitionFrom(toks []cssToken) (*Repetition, error) {
	if len(toks) < 3 || toks[1].kind != cssComma {
		return nil, fmt.Errorf("bad repeat()")
	}
	var count RepetitionCount
	switch c := toks[0]; {
	case c.kind == cssNumber && c.isInt && c.intValue > 0:
		count = RepetitionCount(min(c.intValue, math.MaxUint16))
	case c.kind == cssIdent && c.ident == "auto-fit":
		count = AutoFit
	case c.kind == cssIdent && c.ident == "auto-fill":
		count = AutoFill
	default:
		return nil, fmt.Errorf("bad repeat() count")
	}
	comps, names, err := trackListFrom(toks[2:], false)
	if err != nil {
		return nil, err
	}
	r := &Repetition{Count: count, LineNames: names}
	for _, c := range comps {
		r.Tracks = append(r.Tracks, c.Track)
	}
	return r, nil
}

// ParseGridTemplate parses grid-template-rows or grid-template-columns into
// tracks and line names.
func ParseGridTemplate(s string) ([]GridTemplateComponent, [][]string, error) {
	toks, err := tokenizeCSS(s)
	if err != nil {
		return nil, nil, err
	}
	return trackListFrom(toks, true)
}

// ParseGridAutoTracks parses grid-auto-rows or grid-auto-columns.
func ParseGridAutoTracks(s string) ([]TrackSizing, error) {
	toks, err := tokenizeCSS(s)
	if err != nil {
		return nil, err
	}
	var tracks []TrackSizing
	for _, tok := range toks {
		t, ok := trackFrom(tok)
		if !ok {
			return nil, fmt.Errorf("bad track %q", s)
		}
		tracks = append(tracks, t)
	}
	if len(tracks) == 0 {
		return nil, fmt.Errorf("empty track list")
	}
	return tracks, nil
}

// ParseGridPlacement parses grid-row-start or one of its siblings.
func ParseGridPlacement(s string) (GridPlacement, error) {
	toks, err := tokenizeCSS(s)
	if err != nil {
		return GridPlacement{}, err
	}
	span := false
	var number *int32
	var ident *string
	for i, tok := range toks {
		switch {
		case tok.kind == cssIdent && tok.ident == "auto":
			if span || number != nil || ident != nil || i != len(toks)-1 {
				return GridPlacement{}, fmt.Errorf("bad placement %q", s)
			}
			return GridPlacement{}, nil
		case tok.kind == cssIdent && tok.ident == "span":
			if span {
				return GridPlacement{}, fmt.Errorf("bad placement %q", s)
			}
			span = true
		case tok.kind == cssIdent:
			if ident != nil {
				return GridPlacement{}, fmt.Errorf("bad placement %q", s)
			}
			name := tok.ident
			ident = &name
		case tok.kind == cssNumber && tok.isInt && tok.intValue != 0:
			if number != nil {
				return GridPlacement{}, fmt.Errorf("bad placement %q", s)
			}
			v := tok.intValue
			number = &v
		default:
			return GridPlacement{}, fmt.Errorf("bad placement %q", s)
		}
	}
	satU16 := func(v int32) uint16 { return uint16(max(0, min(v, math.MaxUint16))) }
	satI16 := func(v int32) int16 { return int16(max(math.MinInt16, min(v, math.MaxInt16))) }
	switch {
	case span && number == nil && ident == nil:
		return GridPlacement{Kind: PlacementSpan}, nil
	case span && ident == nil:
		return GridPlacement{Kind: PlacementSpan, Span: satU16(*number)}, nil
	case span && number == nil:
		return GridPlacement{Kind: PlacementSpan, Name: *ident}, nil
	case span:
		return GridPlacement{Kind: PlacementSpan, Span: satU16(*number), Name: *ident}, nil
	case number != nil && ident == nil:
		return GridPlacement{Kind: PlacementLine, Line: satI16(*number)}, nil
	case number != nil:
		return GridPlacement{Kind: PlacementLine, Line: satI16(*number), Name: *ident}, nil
	case ident != nil:
		return GridPlacement{Kind: PlacementLine, Name: *ident}, nil
	}
	return GridPlacement{}, fmt.Errorf("empty placement")
}

// FromStr implementations do (through cssparser), so that styles built from
// a fixture match the ones Taffy's harness builds.
