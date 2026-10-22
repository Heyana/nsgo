package core

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// SVG is a parsed SVG document that an Icon draws: vector shapes that stay
// sharp at any size, painted in the color an icon inherits.
//
// ParseSVG reads the parts icons use, and no more: path, rect, circle,
// ellipse, line, polyline and polygon inside svg and g elements, with fill,
// stroke, stroke widths, line caps and joins. A paint may be none,
// currentColor or #rgb, #rgba, #rrggbb and #rrggbbaa. Groups pass these attributes
// to their children.
//
// Transforms, gradients, patterns, text, images, clip paths, masks, filters,
// markers, animations and style sheets are not read, and neither are the
// alpha, fill-rule and dash attributes: an icon is a drawing, not a page.
type SVG struct {
	// box is the viewBox, or the document's width and height when it has
	// none. It is the user space the operations are in.
	box svgBox
	// width and height are the document's intrinsic size.
	width, height Points
	// ops draw the document in its viewBox; a Color of 0 is currentColor.
	ops []DrawOp
}

// svgBox is a viewBox in user space.
type svgBox struct{ x, y, w, h float32 }

// ParseSVG parses an SVG document.
func ParseSVG(data []byte) (*SVG, error) {
	p := &svgParser{dec: xml.NewDecoder(bytes.NewReader(data))}
	s := &SVG{}
	for {
		tok, err := p.dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("svg: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "svg" {
			return nil, fmt.Errorf("svg: root element is %q, not svg", start.Name.Local)
		}
		if err := p.root(start, s); err != nil {
			return nil, err
		}
		break
	}
	if len(s.ops) == 0 && s.width == 0 && s.height == 0 {
		return nil, errors.New("svg: no svg element")
	}
	if s.width <= 0 || s.height <= 0 {
		if s.box.w > 0 && s.box.h > 0 {
			s.width, s.height = Points(s.box.w), Points(s.box.h)
		} else {
			return nil, errors.New("svg: no size")
		}
	}
	return s, nil
}

// MustParseSVG is ParseSVG for documents that are part of the program, such
// as embedded icons: it panics when one is in error.
func MustParseSVG(data []byte) *SVG {
	s, err := ParseSVG(data)
	if err != nil {
		panic(err)
	}
	return s
}

// Size returns the document's intrinsic size: its width and height, or its
// viewBox's.
func (s *SVG) Size() (w, h Points) {
	if s == nil {
		return 0, 0
	}
	return s.width, s.height
}

// Draw records the document's shapes into d, fitted and centered in box and
// painted in color: every currentColor becomes color, and a zero color is
// black. A non-uniform box keeps the document's aspect ratio.
func (s *SVG) Draw(d *Drawing, box Size, color Color) {
	if s == nil || d == nil {
		return
	}
	if color == 0 {
		color = RGB(0x000000)
	}
	vw, vh := s.box.w, s.box.h
	if vw <= 0 || vh <= 0 {
		return
	}
	scale := min(float32(box.Width)/vw, float32(box.Height)/vh)
	if scale <= 0 {
		return
	}
	ox := (float32(box.Width) - vw*scale) / 2
	oy := (float32(box.Height) - vh*scale) / 2
	d.Save().Translate(ox, oy).Scale(scale, scale).Translate(-s.box.x, -s.box.y)
	for _, op := range s.ops {
		switch op.Kind {
		case DrawFillColor, DrawStrokeColor:
			if op.Color == 0 {
				op.Color = color
			}
		}
		d.Ops = append(d.Ops, op)
	}
	d.Restore()
}

// svgPaint is a fill or a stroke: none, the inherited color, or a fixed one.
type svgPaint struct {
	kind  svgPaintKind
	color Color
}

type svgPaintKind uint8

const (
	paintNone svgPaintKind = iota
	paintCurrent
	paintFixed
)

func fixedPaint(c Color) svgPaint { return svgPaint{paintFixed, c} }

// svgStyle is what shapes inherit from the elements around them.
type svgStyle struct {
	fill, stroke svgPaint
	width        float32
	cap          LineCap
	join         LineJoin
}

func defaultSVGStyle() svgStyle {
	return svgStyle{fill: fixedPaint(RGB(0x000000)), width: 1}
}

type svgParser struct {
	dec *xml.Decoder
	ops []DrawOp
}

// root reads the svg element's attributes and children into s.
func (p *svgParser) root(el xml.StartElement, s *SVG) error {
	style := defaultSVGStyle()
	attrs := svgAttrs(el)
	if err := style.apply(attrs); err != nil {
		return err
	}
	if v, ok := attrs["viewBox"]; ok {
		f, err := svgNumbers(v, 4)
		if err != nil {
			return fmt.Errorf("svg: viewBox: %w", err)
		}
		s.box = svgBox{f[0], f[1], f[2], f[3]}
		if s.box.w < 0 || s.box.h < 0 {
			return errors.New("svg: negative viewBox size")
		}
	}
	if v, ok := attrs["width"]; ok {
		n, err := svgLength(v)
		if err != nil {
			return fmt.Errorf("svg: width: %w", err)
		}
		s.width = Points(n)
	}
	if v, ok := attrs["height"]; ok {
		n, err := svgLength(v)
		if err != nil {
			return fmt.Errorf("svg: height: %w", err)
		}
		s.height = Points(n)
	}
	if s.box == (svgBox{}) {
		s.box = svgBox{w: float32(s.width), h: float32(s.height)}
	}
	p.ops = s.ops
	if err := p.children(style); err != nil {
		return err
	}
	s.ops = p.ops
	return nil
}

// children reads elements until the current one ends.
func (p *svgParser) children(style svgStyle) error {
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return fmt.Errorf("svg: %w", err)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			if err := p.element(tok, style); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// element reads one child element, drawing it or passing its attributes on
// to its children.
func (p *svgParser) element(el xml.StartElement, style svgStyle) error {
	attrs := svgAttrs(el)
	switch el.Name.Local {
	case "svg", "g":
		if err := style.apply(attrs); err != nil {
			return err
		}
		return p.children(style)
	case "title", "desc", "metadata", "defs":
		return p.skip()
	}
	if err := style.apply(attrs); err != nil {
		return err
	}
	switch el.Name.Local {
	case "path":
		if d, ok := attrs["d"]; ok {
			if err := p.path(d); err != nil {
				return fmt.Errorf("svg: path: %w", err)
			}
		}
	case "rect":
		if err := p.rect(attrs); err != nil {
			return err
		}
	case "circle":
		if err := p.circle(attrs); err != nil {
			return err
		}
	case "ellipse":
		if err := p.ellipse(attrs); err != nil {
			return err
		}
	case "line":
		if err := p.line(attrs); err != nil {
			return err
		}
	case "polyline", "polygon":
		if err := p.polygon(attrs, el.Name.Local == "polygon"); err != nil {
			return err
		}
	default:
		return p.skip()
	}
	p.paint(style)
	return p.skip()
}

// paint ends the current path with the fill and the stroke style asks for.
func (p *svgParser) paint(style svgStyle) {
	if style.fill.kind != paintNone {
		p.op(DrawOp{Kind: DrawFillColor, Color: style.fill.color})
		p.op(DrawOp{Kind: DrawFill})
	}
	if style.stroke.kind != paintNone && style.width > 0 {
		p.op(DrawOp{Kind: DrawStrokeColor, Color: style.stroke.color})
		p.op(DrawOp{Kind: DrawLineWidth, W: style.width})
		p.op(DrawOp{Kind: DrawLineCap, Cap: style.cap})
		p.op(DrawOp{Kind: DrawLineJoin, Join: style.join})
		p.op(DrawOp{Kind: DrawStroke})
	}
}

func (p *svgParser) op(o DrawOp) { p.ops = append(p.ops, o) }

// begin starts a shape's path.
func (p *svgParser) begin() { p.op(DrawOp{Kind: DrawBeginPath}) }

// skip reads the rest of the current element, children included.
func (p *svgParser) skip() error {
	depth := 0
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return fmt.Errorf("svg: %w", err)
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			if depth == 0 {
				return nil
			}
			depth--
		}
	}
}

// apply reads the presentation attributes on an element.
func (s *svgStyle) apply(a map[string]string) error {
	for name, value := range a {
		switch name {
		case "fill":
			v, ok, err := parseSVGPaint(value)
			if err != nil {
				return fmt.Errorf("svg: fill: %w", err)
			}
			if ok {
				s.fill = v
			}
		case "stroke":
			v, ok, err := parseSVGPaint(value)
			if err != nil {
				return fmt.Errorf("svg: stroke: %w", err)
			}
			if ok {
				s.stroke = v
			}
		case "stroke-width":
			n, err := svgLength(value)
			if err != nil {
				return fmt.Errorf("svg: stroke-width: %w", err)
			}
			s.width = n
		case "stroke-linecap":
			switch strings.TrimSpace(value) {
			case "butt":
				s.cap = CapButt
			case "round":
				s.cap = CapRound
			case "square":
				s.cap = CapSquare
			}
		case "stroke-linejoin":
			switch strings.TrimSpace(value) {
			case "miter":
				s.join = JoinMiter
			case "round":
				s.join = JoinRound
			case "bevel":
				s.join = JoinBevel
			}
		}
	}
	return nil
}

// parseSVGPaint reads a paint. It reports ok false for a paint it does not
// read, such as a gradient reference, so the inherited one stays.
func parseSVGPaint(v string) (p svgPaint, ok bool, err error) {
	v = strings.TrimSpace(v)
	switch v {
	case "none", "transparent":
		return svgPaint{kind: paintNone}, true, nil
	case "currentColor":
		return svgPaint{kind: paintCurrent}, true, nil
	}
	if strings.HasPrefix(v, "#") {
		c, err := parseSVGHex(v[1:])
		if err != nil {
			return p, false, err
		}
		return fixedPaint(c), true, nil
	}
	return p, false, nil
}

func parseSVGHex(h string) (Color, error) {
	var r, g, b, a uint32
	a = 0xff
	switch len(h) {
	case 3, 4:
		v, err := strconv.ParseUint(h, 16, 16)
		if err != nil {
			return 0, err
		}
		n := func(shift uint) uint32 { return uint32(v>>shift&0xf) * 0x11 }
		r, g, b = n(8), n(4), n(0)
		if len(h) == 4 {
			r, g, b = n(12), n(8), n(4)
			a = n(0)
		}
	case 6, 8:
		v, err := strconv.ParseUint(h, 16, 64)
		if err != nil {
			return 0, err
		}
		if len(h) == 8 {
			r, g, b, a = uint32(v>>24&0xff), uint32(v>>16&0xff), uint32(v>>8&0xff), uint32(v&0xff)
		} else {
			v <<= 8
			r, g, b = uint32(v>>24&0xff), uint32(v>>16&0xff), uint32(v>>8&0xff)
		}
	default:
		return 0, fmt.Errorf("bad color %q", h)
	}
	return Color(r<<24 | g<<16 | b<<8 | a), nil
}

// svgAttrs reads an element's attributes, keeping their case.
func svgAttrs(el xml.StartElement) map[string]string {
	a := make(map[string]string, len(el.Attr))
	for _, attr := range el.Attr {
		if attr.Name.Space != "" {
			continue
		}
		a[attr.Name.Local] = attr.Value
	}
	return a
}

// svgLength reads a number with an optional pixel unit. Percentages, em and
// the other units a viewBox does not use are read as numbers, dropping the
// unit.
func svgLength(v string) (float32, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	n, err := svgNumber(v)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// svgNumber reads the number at the start of v.
func svgNumber(v string) (float32, error) {
	i := 0
	if i < len(v) && (v[i] == '+' || v[i] == '-') {
		i++
	}
	digits := false
	for i < len(v) && v[i] >= '0' && v[i] <= '9' {
		i++
		digits = true
	}
	if i < len(v) && v[i] == '.' {
		i++
		for i < len(v) && v[i] >= '0' && v[i] <= '9' {
			i++
			digits = true
		}
	}
	if !digits {
		return 0, fmt.Errorf("bad number %q", v)
	}
	if i < len(v) && (v[i] == 'e' || v[i] == 'E') {
		j := i + 1
		if j < len(v) && (v[j] == '+' || v[j] == '-') {
			j++
		}
		if j < len(v) && v[j] >= '0' && v[j] <= '9' {
			for j < len(v) && v[j] >= '0' && v[j] <= '9' {
				j++
			}
			i = j
		}
	}
	n, err := strconv.ParseFloat(v[:i], 32)
	if err != nil {
		return 0, err
	}
	return float32(n), nil
}

// svgNumbers reads n numbers separated by space or commas, or every number
// when n is negative.
func svgNumbers(v string, n int) ([]float32, error) {
	sc := svgScanner{s: v}
	out := make([]float32, 0, max(n, 0))
	for n < 0 || len(out) < n {
		sc.skip()
		if sc.eof() {
			break
		}
		f, ok := sc.number()
		if !ok {
			return nil, fmt.Errorf("want %d numbers in %q", n, v)
		}
		out = append(out, f)
	}
	if n >= 0 && len(out) != n {
		return nil, fmt.Errorf("want %d numbers in %q", n, v)
	}
	return out, nil
}

// svgScanner reads numbers, flags and commands from an attribute value.
type svgScanner struct {
	s string
	i int
}

func (s *svgScanner) eof() bool { return s.i >= len(s.s) }

func (s *svgScanner) peek() byte {
	if s.eof() {
		return 0
	}
	return s.s[s.i]
}

func (s *svgScanner) next() byte {
	b := s.s[s.i]
	s.i++
	return b
}

func (s *svgScanner) skip() {
	for !s.eof() {
		switch s.s[s.i] {
		case ' ', '\t', '\n', '\r', '\f', ',':
			s.i++
		default:
			return
		}
	}
}

// numberStart reports whether a number starts here, after separators.
func (s *svgScanner) numberStart() bool {
	s.skip()
	if s.eof() {
		return false
	}
	b := s.s[s.i]
	return b >= '0' && b <= '9' || b == '.' || b == '+' || b == '-'
}

func (s *svgScanner) number() (float32, bool) {
	s.skip()
	start := s.i
	if s.numberStart() {
		if b := s.s[s.i]; b == '+' || b == '-' {
			s.i++
		}
	}
	digits := false
	for !s.eof() && s.s[s.i] >= '0' && s.s[s.i] <= '9' {
		s.i++
		digits = true
	}
	if !s.eof() && s.s[s.i] == '.' {
		s.i++
		for !s.eof() && s.s[s.i] >= '0' && s.s[s.i] <= '9' {
			s.i++
			digits = true
		}
	}
	if !digits {
		s.i = start
		return 0, false
	}
	if !s.eof() && (s.s[s.i] == 'e' || s.s[s.i] == 'E') {
		j := s.i + 1
		if j < len(s.s) && (s.s[j] == '+' || s.s[j] == '-') {
			j++
		}
		if j < len(s.s) && s.s[j] >= '0' && s.s[j] <= '9' {
			for j < len(s.s) && s.s[j] >= '0' && s.s[j] <= '9' {
				j++
			}
			s.i = j
		}
	}
	f, err := strconv.ParseFloat(s.s[start:s.i], 32)
	if err != nil {
		s.i = start
		return 0, false
	}
	return float32(f), true
}

// flag reads a single 0 or 1, which may run into the next number.
func (s *svgScanner) flag() (bool, bool) {
	s.skip()
	if s.eof() {
		return false, false
	}
	switch s.s[s.i] {
	case '0':
		s.i++
		return false, true
	case '1':
		s.i++
		return true, true
	}
	return false, false
}

func isPathCommand(b byte) bool {
	switch b {
	case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v', 'C', 'c', 'S', 's',
		'Q', 'q', 'T', 't', 'A', 'a', 'Z', 'z':
		return true
	}
	return false
}
