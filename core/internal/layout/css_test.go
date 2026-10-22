package layout

import (
	"fmt"
	"strconv"
	"strings"
)

// This file parses the other CSS values of Taffy's XML fixtures as Taffy's
// FromStr implementations do.

// tokenizeCSS splits a value into tokens, dropping whitespace.
// consumeNumeric follows cssparser's consume_numeric, which accumulates the
// digits in float64.
// cssValue tokenizes s and requires it to be a single token.
func cssKeyword[T any](s string, keywords map[string]T) (T, error) {
	var zero T
	tok, err := cssSingle(s)
	if err != nil {
		return zero, err
	}
	if tok.kind == cssIdent {
		if v, ok := keywords[strings.ToLower(tok.ident)]; ok {
			return v, nil
		}
	}
	return zero, fmt.Errorf("unknown keyword %q", s)
}

func parseDisplay(s string) (Display, error) {
	return cssKeyword(s, map[string]Display{
		"none": DisplayNone, "flex": DisplayFlex, "grid": DisplayGrid, "block": DisplayBlock,
		"flow-root": DisplayFlowRoot,
	})
}

func parsePosition(s string) (Position, error) {
	return cssKeyword(s, map[string]Position{
		"static": PositionStatic, "relative": PositionRelative, "absolute": PositionAbsolute,
		"fixed": PositionFixed, "sticky": PositionSticky,
	})
}

func parseBoxSizing(s string) (BoxSizing, error) {
	return cssKeyword(s, map[string]BoxSizing{"border-box": BorderBox, "content-box": ContentBox})
}

func parseDirection(s string) (Direction, error) {
	return cssKeyword(s, map[string]Direction{"ltr": LTR, "rtl": RTL})
}

func parseOverflow(s string) (Overflow, error) {
	return cssKeyword(s, map[string]Overflow{
		"visible": OverflowVisible, "hidden": OverflowHidden, "clip": OverflowClip, "scroll": OverflowScroll,
	})
}

func parseTextAlign(s string) (TextAlign, error) {
	return cssKeyword(s, map[string]TextAlign{
		"auto": TextAlignAuto, "-webkit-left": TextAlignLegacyLeft, "-webkit-right": TextAlignLegacyRight,
		"-webkit-center": TextAlignLegacyCenter,
	})
}

func parseFlexDirection(s string) (FlexDirection, error) {
	return cssKeyword(s, map[string]FlexDirection{
		"row": Row, "column": Column, "row-reverse": RowReverse, "column-reverse": ColumnReverse,
	})
}

func parseFlexWrap(s string) (FlexWrap, error) {
	return cssKeyword(s, map[string]FlexWrap{"nowrap": NoWrap, "wrap": Wrap, "wrap-reverse": WrapReverse})
}

func parseContain(s string) (Contain, error) {
	toks, err := tokenizeCSS(s)
	if err != nil || len(toks) == 0 {
		return 0, fmt.Errorf("bad contain %q", s)
	}
	var c Contain
	seen := map[string]bool{}
	for _, tok := range toks {
		if tok.kind != cssIdent {
			return 0, fmt.Errorf("bad contain %q", s)
		}
		name := strings.ToLower(tok.ident)
		switch name {
		case "none", "content":
			if len(toks) != 1 {
				return 0, fmt.Errorf("bad contain %q", s)
			}
			if name == "content" {
				return ContainContent, nil
			}
			return 0, nil
		case "layout":
			c |= ContainLayout
		case "paint":
			c |= ContainPaint
		case "style":
		default:
			return 0, fmt.Errorf("bad contain %q", s)
		}
		if seen[name] {
			return 0, fmt.Errorf("bad contain %q", s)
		}
		seen[name] = true
	}
	return c, nil
}

func parseGridAutoFlow(s string) (GridAutoFlow, error) {
	toks, err := tokenizeCSS(s)
	if err != nil || len(toks) == 0 || len(toks) > 2 {
		return 0, fmt.Errorf("bad grid-auto-flow %q", s)
	}
	axis, dense := "", false
	for _, tok := range toks {
		if tok.kind != cssIdent {
			return 0, fmt.Errorf("bad grid-auto-flow %q", s)
		}
		switch tok.ident {
		case "row", "column":
			axis = tok.ident
		case "dense":
			dense = true
		default:
			return 0, fmt.Errorf("bad grid-auto-flow %q", s)
		}
	}
	switch {
	case axis == "column" && dense:
		return FlowColumnDense, nil
	case axis == "column":
		return FlowColumn, nil
	case dense:
		return FlowRowDense, nil
	}
	return FlowRow, nil
}

var alignKeywords = map[string]AlignKeyword{
	"start": AlignStart, "end": AlignEnd, "flex-start": AlignFlexStart, "flex-end": AlignFlexEnd,
	"self-start": AlignSelfStart, "self-end": AlignSelfEnd, "center": AlignCenter,
}

// parseAlign parses align-items, justify-items, align-self and
// justify-self values.
func parseAlign(s string) (Align, error) {
	toks, err := tokenizeCSS(s)
	if err != nil || len(toks) == 0 || len(toks) > 2 {
		return Align{}, fmt.Errorf("bad alignment %q", s)
	}
	for _, tok := range toks {
		if tok.kind != cssIdent {
			return Align{}, fmt.Errorf("bad alignment %q", s)
		}
	}
	first := strings.ToLower(toks[0].ident)
	if first == "safe" || first == "unsafe" {
		if len(toks) != 2 {
			return Align{}, fmt.Errorf("bad alignment %q", s)
		}
		k, ok := alignKeywords[strings.ToLower(toks[1].ident)]
		if !ok {
			return Align{}, fmt.Errorf("bad alignment %q", s)
		}
		if first == "safe" {
			return Align{k, Safe}, nil
		}
		return Align{k, Unsafe}, nil
	}
	if len(toks) != 1 {
		return Align{}, fmt.Errorf("bad alignment %q", s)
	}
	switch first {
	case "normal":
		return alignOf(AlignNormal), nil
	case "baseline":
		return alignOf(AlignBaseline), nil
	case "stretch":
		return alignOf(AlignStretch), nil
	}
	if k, ok := alignKeywords[first]; ok {
		return alignOf(k), nil
	}
	return Align{}, fmt.Errorf("bad alignment %q", s)
}

// parseSelfAlign parses align-self and justify-self, where auto is unset.
func parseSelfAlign(s string) (Align, error) {
	if strings.EqualFold(strings.TrimSpace(s), "auto") {
		return Align{}, nil
	}
	return parseAlign(s)
}

var contentKeywords = map[string]ContentKeyword{
	"start": ContentStart, "end": ContentEnd, "flex-start": ContentFlexStart, "flex-end": ContentFlexEnd,
	"center": ContentCenter,
}

func parseAlignContent(s string) (AlignContent, error) {
	toks, err := tokenizeCSS(s)
	if err != nil || len(toks) == 0 || len(toks) > 2 {
		return AlignContent{}, fmt.Errorf("bad content alignment %q", s)
	}
	for _, tok := range toks {
		if tok.kind != cssIdent {
			return AlignContent{}, fmt.Errorf("bad content alignment %q", s)
		}
	}
	first := strings.ToLower(toks[0].ident)
	if first == "safe" || first == "unsafe" {
		if len(toks) != 2 {
			return AlignContent{}, fmt.Errorf("bad content alignment %q", s)
		}
		k, ok := contentKeywords[strings.ToLower(toks[1].ident)]
		if !ok {
			return AlignContent{}, fmt.Errorf("bad content alignment %q", s)
		}
		if first == "safe" {
			return AlignContent{k, Safe}, nil
		}
		return AlignContent{k, Unsafe}, nil
	}
	if len(toks) != 1 {
		return AlignContent{}, fmt.Errorf("bad content alignment %q", s)
	}
	switch first {
	case "normal":
		return contentOf(ContentNormal), nil
	case "stretch":
		return contentOf(ContentStretch), nil
	case "space-between":
		return contentOf(ContentSpaceBetween), nil
	case "space-evenly":
		return contentOf(ContentSpaceEvenly), nil
	case "space-around":
		return contentOf(ContentSpaceAround), nil
	}
	if k, ok := contentKeywords[first]; ok {
		return contentOf(k), nil
	}
	return AlignContent{}, fmt.Errorf("bad content alignment %q", s)
}

// lengthFrom parses a px dimension or a percentage token.
// parseLengthPercentage parses padding, border and gap values.
func parseLengthPercentage(s string) (Length, error) {
	tok, err := cssSingle(s)
	if err != nil {
		return Length{}, err
	}
	if l, ok := lengthFrom(tok); ok {
		return l, nil
	}
	return Length{}, fmt.Errorf("bad length %q", s)
}

// parseLengthPercentageAuto parses margins, insets and min and max sizes.
func parseLengthPercentageAuto(s string) (Length, error) {
	tok, err := cssSingle(s)
	if err != nil {
		return Length{}, err
	}
	if l, ok := lengthFrom(tok); ok {
		return l, nil
	}
	if tok.kind == cssIdent && tok.ident == "auto" {
		return Auto, nil
	}
	return Length{}, fmt.Errorf("bad length %q", s)
}

// parseDimension parses sizes and flex-basis.
func parseDimension(s string) (Length, error) {
	tok, err := cssSingle(s)
	if err != nil {
		return Length{}, err
	}
	if l, ok := lengthFrom(tok); ok {
		return l, nil
	}
	switch tok.kind {
	case cssIdent:
		switch tok.ident {
		case "auto":
			return Auto, nil
		case "min-content":
			return MinContent, nil
		case "max-content":
			return MaxContent, nil
		case "fit-content":
			return FitContent, nil
		case "stretch":
			return Stretch, nil
		case "content":
			return Content, nil
		}
	case cssFunction:
		if l, ok := fitContentFrom(tok); ok {
			return l, nil
		}
	}
	return Length{}, fmt.Errorf("bad dimension %q", s)
}

// maxTrackFrom parses a max track sizing function.
// minTrackFrom parses a min track sizing function.
// trackFrom parses a track sizing function: minmax() or a single size.
// trackListFrom parses tracks with line name groups, as Taffy's
// GridTemplateTracks: one name group per line, empty when unnamed. With
// repeat, entries may be repeat() functions.
// ParseGridTemplate parses grid-template-rows and grid-template-columns
// into tracks and line names.
// ParseGridAutoTracks parses grid-auto-rows and grid-auto-columns.
// ParseGridPlacement parses grid-row-start and its siblings.
// parseAvailableSpace parses a fixture's viewport size.
func parseAvailableSpace(s string) (AvailableSpace, error) {
	tok, err := cssSingle(s)
	if err != nil {
		return AvailableSpace{}, err
	}
	switch {
	case (tok.kind == cssNumber || tok.kind == cssDimension) && tok.value >= 0:
		return Definite(tok.value), nil
	case tok.kind == cssIdent && tok.ident == "max-content":
		return MaxContentSpace, nil
	case tok.kind == cssIdent && tok.ident == "min-content":
		return MinContentSpace, nil
	}
	return AvailableSpace{}, fmt.Errorf("bad available space %q", s)
}

// parseFloat parses a number as Rust's f32::from_str does.
func parseFloat(s string) (float32, error) {
	v, err := strconv.ParseFloat(s, 32)
	return float32(v), err
}
