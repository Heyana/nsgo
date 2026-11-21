package driver

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// svgTextOp is a parsed <text> element positioned in painter's order at index
// at in SVG.ops. Its spans are laid out against the Drawing's platform
// TextEngine when drawn, so alignment, baselines and <tspan> advances use real
// glyph metrics.
type svgTextOp struct {
	at   int
	ctm  svgMatrix
	runs []svgTextRun
}

// svgTextRun is one styled text span inside an svgTextOp.
type svgTextRun struct {
	text          string
	font          Font
	align         TextAlign
	baseline      TextBaseline
	hasX, hasY    bool
	x, y          float32
	dx, dy        float32
	hidden        bool
	preserveSpace bool
	fill          svgResolvedPaint
	stroke        svgResolvedPaint
	lineWidth     float32
	lineCap       LineCap
	lineJoin      LineJoin
	dash          LineDash
	dashOff       float32
}

// svgResolvedPaint is a fill or stroke paint resolved at parse time so drawing
// needs no document element lookup and never mutates the parsed SVG.
type svgResolvedPaint struct {
	kind     svgPaintKind
	color    Color
	alpha    float32
	gradient *CanvasGradient
	gradBBox bool
}

// pendingTextPos tracks explicit positioning attributes waiting for the next
// text run inside a <text> or <tspan> subtree.
type pendingTextPos struct {
	hasX, hasY bool
	x, y       float32
	dx, dy     float32
}

// text parses a <text> element and its <tspan> descendants into p.texts.
func (p *svgParser) text(el *svgNode, style svgStyle) error {
	var pos pendingTextPos
	pos.hasX, pos.hasY = true, true
	if x, ok, err := p.textCoord(el.attrs, "x", style.fontSize); err != nil {
		return err
	} else if ok {
		pos.x = x
	}
	if y, ok, err := p.textCoord(el.attrs, "y", style.fontSize); err != nil {
		return err
	} else if ok {
		pos.y = y
	}
	if dx, ok, err := p.textCoord(el.attrs, "dx", style.fontSize); err != nil {
		return err
	} else if ok {
		pos.dx = dx
	}
	if dy, ok, err := p.textCoord(el.attrs, "dy", style.fontSize); err != nil {
		return err
	} else if ok {
		pos.dy = dy
	}
	var raw []svgTextRun
	if err := p.collectTextRuns(el, style, &pos, &raw); err != nil {
		return err
	}
	runs := normalizeTextRuns(raw)
	if len(runs) == 0 {
		return nil
	}
	p.texts = append(p.texts, svgTextOp{
		at:   len(p.ops),
		ctm:  style.ctm,
		runs: runs,
	})
	return nil
}

// collectTextRuns walks el's children (character data and <tspan> elements) in
// document order, accumulating runs with their inherited styles and positions.
func (p *svgParser) collectTextRuns(el *svgNode, style svgStyle, pos *pendingTextPos, out *[]svgTextRun) error {
	for _, c := range el.children {
		if c.isText {
			if c.text == "" {
				continue
			}
			if p.visits++; p.visits > maxSVGVisits || len(*out) >= maxSVGOps {
				return errSVGTooComplex
			}
			run := p.makeTextRun(c.text, style, *pos)
			*out = append(*out, run)
			*pos = pendingTextPos{}
			continue
		}
		if c.name != "tspan" {
			continue
		}
		if p.visits++; p.visits > maxSVGVisits {
			return errSVGTooComplex
		}
		a := c.attrs
		if strings.TrimSpace(a["display"]) == "none" {
			continue
		}
		childStyle := style
		if err := childStyle.apply(a); err != nil {
			return err
		}
		if v, ok := a["opacity"]; ok {
			childStyle.opacity *= svgOpacity(v)
		}
		if x, ok, err := p.textCoord(a, "x", childStyle.fontSize); err != nil {
			return err
		} else if ok {
			pos.hasX, pos.x, pos.dx = true, x, 0
		}
		if y, ok, err := p.textCoord(a, "y", childStyle.fontSize); err != nil {
			return err
		} else if ok {
			pos.hasY, pos.y, pos.dy = true, y, 0
		}
		if dx, ok, err := p.textCoord(a, "dx", childStyle.fontSize); err != nil {
			return err
		} else if ok {
			pos.dx += dx
		}
		if dy, ok, err := p.textCoord(a, "dy", childStyle.fontSize); err != nil {
			return err
		} else if ok {
			pos.dy += dy
		}
		if err := p.collectTextRuns(c, childStyle, pos, out); err != nil {
			return err
		}
	}
	return nil
}

func (p *svgParser) makeTextRun(text string, style svgStyle, pos pendingTextPos) svgTextRun {
	font := Font{
		Family:  style.fontFamily,
		Size:    Points(style.fontSize),
		Weight:  style.fontWeight,
		Italic:  style.fontItalic,
		Variant: style.fontVariant,
	}
	if font.Weight == 0 {
		font.Weight = FontWeightNormal
	}
	var dash LineDash
	if len(style.dash) > 0 {
		dash = LineDash(slices.Clone(style.dash))
	}
	strokePaint := style.stroke
	if style.width <= 0 {
		strokePaint = svgPaint{}
	}
	return svgTextRun{
		text:          text,
		font:          font,
		align:         style.textAnchor,
		baseline:      style.textBaseline,
		hasX:          pos.hasX,
		hasY:          pos.hasY,
		x:             pos.x,
		y:             pos.y,
		dx:            pos.dx,
		dy:            pos.dy,
		hidden:        style.hidden || style.opacity <= 0 || style.fontSize <= 0,
		preserveSpace: style.preserveSpace,
		fill:          p.resolveTextPaint(style.fill, style, style.fillOpacity),
		stroke:        p.resolveTextPaint(strokePaint, style, style.strokeOpacity),
		lineWidth:     style.width,
		lineCap:       style.cap,
		lineJoin:      style.join,
		dash:          dash,
		dashOff:       style.dashOffset,
	}
}

// resolveTextPaint resolves a text run's fill or stroke paint against p.ids at
// parse time.
func (p *svgParser) resolveTextPaint(paint svgPaint, style svgStyle, alpha float32) svgResolvedPaint {
	alpha *= style.opacity
	if alpha <= 0 {
		return svgResolvedPaint{}
	}
	switch paint.kind {
	case paintContextFill:
		paint = style.contextFill
	case paintContextStroke:
		paint = style.contextStroke
	}
	if paint.kind == paintURL {
		var g *CanvasGradient
		var isBBox bool
		if paint.gradient != nil {
			c := *paint.gradient
			c.Stops = slices.Clone(c.Stops)
			g = &c
		} else if el := p.ids[paint.ref]; el != nil && (el.name == "linearGradient" || el.name == "radialGradient") {
			var solid Color
			var ok bool
			g, solid, isBBox, ok = p.gradientWith(el, svgIdentity, true)
			if !ok {
				return svgResolvedPaint{}
			}
			if g == nil {
				paint = fixedPaint(solid)
			}
		} else {
			paint = svgPaint{kind: paint.fall, color: paint.color}
		}
		if g != nil {
			for i := range g.Stops {
				g.Stops[i].Color = svgAlpha(g.Stops[i].Color, alpha)
			}
			fallback := g.average()
			if fallback == 0 {
				fallback = 1
			}
			return svgResolvedPaint{
				kind:     paintURL,
				color:    fallback,
				gradient: g,
				gradBBox: isBBox,
			}
		}
	}
	if paint.kind == paintCurrent && style.color.kind == paintFixed {
		paint = style.color
	}
	switch paint.kind {
	case paintCurrent:
		return svgResolvedPaint{kind: paintCurrent, alpha: alpha}
	case paintFixed:
		c := svgAlpha(paint.color, alpha)
		if c&0xff == 0 {
			return svgResolvedPaint{}
		}
		return svgResolvedPaint{kind: paintFixed, color: c}
	}
	return svgResolvedPaint{}
}

// normalizeTextRuns applies SVG whitespace rules across the runs of a <text>
// element, trimming collapsed whitespace at the boundaries of each text chunk
// and carrying any positioning from dropped whitespace-only runs onto the next
// run.
func normalizeTextRuns(raw []svgTextRun) []svgTextRun {
	if len(raw) == 0 {
		return nil
	}
	trimTrailing := func(out []svgTextRun) []svgTextRun {
		for len(out) > 0 && !out[len(out)-1].preserveSpace {
			last := &out[len(out)-1]
			last.text = strings.TrimRight(last.text, " ")
			if last.text != "" {
				break
			}
			out = out[:len(out)-1]
		}
		return out
	}
	out := make([]svgTextRun, 0, len(raw))
	prevSpace := true
	var carry pendingTextPos
	for _, r := range raw {
		if (r.hasX || r.hasY) && len(out) > 0 {
			out = trimTrailing(out)
			prevSpace = true
		}
		var b strings.Builder
		b.Grow(len(r.text))
		if r.preserveSpace {
			s := strings.ReplaceAll(r.text, "\r\n", " ")
			for _, ch := range s {
				switch ch {
				case '\r', '\n', '\t':
					b.WriteByte(' ')
				default:
					b.WriteRune(ch)
				}
			}
			t := b.String()
			if t != "" {
				prevSpace = t[len(t)-1] == ' '
			}
			r.text = t
		} else {
			for _, ch := range r.text {
				switch ch {
				case ' ', '\t', '\n', '\r':
					if !prevSpace {
						b.WriteByte(' ')
						prevSpace = true
					}
				default:
					b.WriteRune(ch)
					prevSpace = false
				}
			}
			r.text = b.String()
		}
		if r.text == "" {
			if r.hasX {
				carry.hasX, carry.x, carry.dx = true, r.x, r.dx
			} else {
				carry.dx += r.dx
			}
			if r.hasY {
				carry.hasY, carry.y, carry.dy = true, r.y, r.dy
			} else {
				carry.dy += r.dy
			}
			continue
		}
		if carry.hasX && !r.hasX {
			r.hasX, r.x = true, carry.x
			r.dx += carry.dx
		} else if !r.hasX {
			r.dx += carry.dx
		}
		if carry.hasY && !r.hasY {
			r.hasY, r.y = true, carry.y
			r.dy += carry.dy
		} else if !r.hasY {
			r.dy += carry.dy
		}
		carry = pendingTextPos{}
		out = append(out, r)
	}
	return trimTrailing(out)
}

// drawTextOp lays out and emits one <text> element into d, measuring its spans
// with d's platform TextEngine.
func (s *SVG) drawTextOp(d *Drawing, t *svgTextOp, color Color) {
	if len(t.runs) == 0 {
		return
	}
	if t.ctm != svgIdentity {
		m := t.ctm
		d.Ops = append(d.Ops, DrawOp{Kind: DrawSave}, DrawOp{
			Kind: DrawTransform,
			X1:   float32(m.a), Y1: float32(m.b),
			X2: float32(m.c), Y2: float32(m.d),
			X: float32(m.e), Y: float32(m.f),
		})
		defer func() { d.Ops = append(d.Ops, DrawOp{Kind: DrawRestore}) }()
	}
	var curX, curY float32
	for i := 0; i < len(t.runs); {
		end := i + 1
		for end < len(t.runs) && !t.runs[end].hasX && !t.runs[end].hasY {
			end++
		}
		chunk := t.runs[i:end]
		i = end

		baseX, baseY := curX, curY
		if chunk[0].hasX {
			baseX = chunk[0].x
		}
		if chunk[0].hasY {
			baseY = chunk[0].y
		}
		baseX += chunk[0].dx
		baseY += chunk[0].dy

		metrics := make([]TextMetrics, len(chunk))
		offX := make([]float32, len(chunk))
		offY := make([]float32, len(chunk))
		var relX, relY float32
		for j := range chunk {
			if j > 0 {
				relX += chunk[j].dx
				relY += chunk[j].dy
			}
			offX[j], offY[j] = relX, relY
			if chunk[j].font.Size > 0 {
				metrics[j] = d.TextMetrics(chunk[j].text, chunk[j].font)
				relX += metrics[j].Width
			}
		}
		totalW := relX
		align := chunk[0].align
		var ax float32
		switch align {
		case TextAlignCenter:
			ax = -totalW / 2
		case TextAlignEnd:
			ax = -totalW
		}

		for j := range chunk {
			r := &chunk[j]
			if r.hidden || r.font.Size <= 0 {
				continue
			}
			m := metrics[j]
			var by float32
			switch r.baseline {
			case BaselineTop:
				by = m.Ascent
			case BaselineHanging:
				by = m.Ascent * 0.8
			case BaselineMiddle:
				by = (m.Ascent - m.Descent) / 2
			case BaselineIdeographic, BaselineBottom:
				by = -m.Descent
			}
			px, py := baseX+offX[j], baseY+offY[j]
			box := svgBox{x: px + ax, y: py + by - m.Ascent, w: m.Width, h: m.Ascent + m.Descent}
			font := drawFont(r.font)

			if ok, restore := emitTextPaint(d, r.fill, box, color, true); ok {
				d.Ops = append(d.Ops, DrawOp{
					Kind:         DrawFillText,
					Text:         r.text,
					Font:         font,
					X:            px,
					Y:            py,
					X1:           ax,
					Y1:           by,
					TextAlign:    align,
					TextBaseline: r.baseline,
				})
				if restore {
					d.Ops = append(d.Ops, DrawOp{Kind: DrawRestore})
				}
			}
			if r.lineWidth > 0 {
				if ok, restore := emitTextPaint(d, r.stroke, box, color, false); ok {
					d.Ops = append(d.Ops,
						DrawOp{Kind: DrawLineWidth, W: r.lineWidth},
						DrawOp{Kind: DrawLineCap, Cap: r.lineCap},
						DrawOp{Kind: DrawLineJoin, Join: r.lineJoin},
					)
					dashed := len(r.dash) > 0
					if dashed {
						dashCopy := LineDash(slices.Clone(r.dash))
						d.Ops = append(d.Ops,
							DrawOp{Kind: DrawLineDash, Dash: &dashCopy},
							DrawOp{Kind: DrawLineDashOffset, W: r.dashOff},
						)
					}
					d.Ops = append(d.Ops, DrawOp{
						Kind:         DrawStrokeText,
						Text:         r.text,
						Font:         font,
						X:            px,
						Y:            py,
						X1:           ax,
						Y1:           by,
						TextAlign:    align,
						TextBaseline: r.baseline,
					})
					if dashed {
						d.Ops = append(d.Ops, DrawOp{Kind: DrawLineDash}, DrawOp{Kind: DrawLineDashOffset})
					}
					if restore {
						d.Ops = append(d.Ops, DrawOp{Kind: DrawRestore})
					}
				}
			}
		}
		curX = baseX + ax + totalW
		curY = baseY + offY[len(chunk)-1]
	}
}

// emitTextPaint records the fill or stroke paint for a text run, substituting
// color for currentColor and mapping objectBoundingBox gradients to box.
func emitTextPaint(d *Drawing, paint svgResolvedPaint, box svgBox, color Color, fill bool) (ok, restore bool) {
	colorKind, gradientKind := DrawStrokeColor, DrawStrokeGradient
	if fill {
		colorKind, gradientKind = DrawFillColor, DrawFillGradient
	}
	switch paint.kind {
	case paintNone:
		return false, false
	case paintFixed:
		d.Ops = append(d.Ops, DrawOp{Kind: colorKind, Color: paint.color})
		return true, false
	case paintCurrent:
		if paint.alpha < 1 {
			d.Ops = append(d.Ops, DrawOp{Kind: DrawSave}, DrawOp{Kind: DrawAlpha, A: paint.alpha})
			restore = true
		}
		d.Ops = append(d.Ops, DrawOp{Kind: colorKind, Color: color})
		return true, restore
	case paintURL:
		g := paint.gradient
		if g == nil {
			return false, false
		}
		if paint.gradBBox {
			if box.w <= 0 || box.h <= 0 {
				return false, false
			}
			c := *g
			gm := svgMatrix{
				a: float64(c.Transform[0]), b: float64(c.Transform[1]),
				c: float64(c.Transform[2]), d: float64(c.Transform[3]),
				e: float64(c.Transform[4]), f: float64(c.Transform[5]),
			}
			bm := svgMatrix{a: float64(box.w), d: float64(box.h), e: float64(box.x), f: float64(box.y)}
			m := bm.mul(gm)
			c.Transform = [6]float32{float32(m.a), float32(m.b), float32(m.c), float32(m.d), float32(m.e), float32(m.f)}
			g = &c
		}
		d.Ops = append(d.Ops, DrawOp{Kind: colorKind, Color: paint.color}, DrawOp{Kind: gradientKind, Gradient: g})
		return true, false
	}
	return false, false
}

// textCoord reads the first coordinate in a text element's x, y, dx or dy
// attribute, resolving percentages against the viewBox and em/ex against em.
func (p *svgParser) textCoord(a map[string]string, name string, em float32) (float32, bool, error) {
	v, ok := a[name]
	if !ok {
		return 0, false, nil
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false, nil
	}
	// Coordinate attributes accept a space/comma-separated list; take the first.
	if i := strings.IndexAny(v, " \t\n\r,"); i >= 0 {
		v = v[:i]
	}
	ref := p.vw
	if name == "y" || name == "dy" {
		ref = p.vh
	}
	n, err := svgLengthOfEm(v, ref, em)
	if err != nil {
		return 0, false, fmt.Errorf("svg: %s: %w", name, err)
	}
	return n, true, nil
}

// svgLengthOfEm reads a length with percentages of ref and em/ex of em.
func svgLengthOfEm(v string, ref, em float32) (float32, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	sc := svgScanner{s: v}
	n, ok := sc.number()
	if !ok {
		return 0, fmt.Errorf("bad number %q", v)
	}
	if em <= 0 {
		em = 16
	}
	switch strings.TrimSpace(v[sc.i:]) {
	case "%":
		n *= ref / 100
	case "pt":
		n *= 96.0 / 72
	case "pc":
		n *= 16
	case "in":
		n *= 96
	case "cm":
		n *= 96 / 2.54
	case "mm":
		n *= 96 / 25.4
	case "em":
		n *= em
	case "rem":
		n *= 16
	case "ex":
		n *= em * 0.5
	}
	return n, nil
}

// parseSVGFontSize resolves a CSS/SVG font-size value relative to inherited.
func parseSVGFontSize(v string, inherited float32) (float32, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if inherited <= 0 {
		inherited = 16
	}
	switch strings.ToLower(v) {
	case "xx-small":
		return 9, true
	case "x-small":
		return 10, true
	case "small":
		return 13, true
	case "medium":
		return 16, true
	case "large":
		return 18, true
	case "x-large":
		return 24, true
	case "xx-large":
		return 32, true
	case "smaller":
		return inherited / 1.2, true
	case "larger":
		return inherited * 1.2, true
	}
	n, err := svgLengthOfEm(v, inherited, inherited)
	if err != nil || n < 0 || n != n || math.IsInf(float64(n), 0) {
		return 0, false
	}
	return n, true
}

// parseSVGFontWeight parses a CSS/SVG font-weight value.
func parseSVGFontWeight(v string, inherited FontWeight) (FontWeight, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if inherited == 0 {
		inherited = FontWeightNormal
	}
	switch v {
	case "normal":
		return FontWeightNormal, true
	case "bold":
		return FontWeightBold, true
	case "bolder":
		switch {
		case inherited < 400:
			return FontWeightNormal, true
		case inherited < 600:
			return FontWeightBold, true
		default:
			return 900, true
		}
	case "lighter":
		switch {
		case inherited > 700:
			return FontWeightBold, true
		case inherited > 500:
			return FontWeightNormal, true
		default:
			return 100, true
		}
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n < 1 || n > 1000 || n != n {
		return 0, false
	}
	w := int(math.Round(n/100)) * 100
	w = min(max(w, 100), 900)
	return FontWeight(w), true
}

// parseSVGFontFamily cleans a CSS/SVG font-family value, keeping comma-separated
// fallback lists intact for the platform text engine.
func parseSVGFontFamily(v string) string {
	v = strings.TrimSpace(v)
	if !strings.Contains(v, ",") {
		v = strings.Trim(v, `"'`)
	}
	return strings.TrimSpace(v)
}

// applyFontShorthand applies CSS's font shorthand:
// [style] [variant] [weight] size[/line-height] family.
func (s *svgStyle) applyFontShorthand(v string) {
	words := splitFontWords(v)
	for i, word := range words {
		low := strings.ToLower(word)
		switch {
		case low == "italic" || low == "oblique":
			s.fontItalic = true
		case low == "small-caps":
			s.fontVariant = FontSmallCaps
		case low == "bold":
			s.fontWeight = FontWeightBold
		case low == "normal":
		default:
			sizePart, _, hasSlash := strings.Cut(word, "/")
			if !hasSlash {
				if w, ok := parseSVGFontWeight(low, s.fontWeight); ok && i+1 < len(words) {
					// A bare number followed by a size is font-weight.
					nextSize, _, _ := strings.Cut(words[i+1], "/")
					if _, ok2 := parseSVGFontSize(nextSize, s.fontSize); ok2 {
						s.fontWeight = w
						continue
					}
				}
			}
			if sz, ok := parseSVGFontSize(sizePart, s.fontSize); ok {
				s.fontSize = sz
				if i+1 < len(words) {
					s.fontFamily = parseSVGFontFamily(strings.Join(words[i+1:], " "))
				}
				return
			}
		}
	}
}

// splitFontWords splits a CSS font shorthand on whitespace outside quotes.
func splitFontWords(s string) []string {
	var out []string
	var cur strings.Builder
	var quote byte
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for i := range len(s) {
		b := s[i]
		if quote != 0 {
			cur.WriteByte(b)
			if b == quote {
				quote = 0
			}
			continue
		}
		if b == '\'' || b == '"' {
			quote = b
			cur.WriteByte(b)
			continue
		}
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			flush()
			continue
		}
		cur.WriteByte(b)
	}
	flush()
	return out
}
