//go:build darwin

package apple

import "math"

// strutSpec holds the primary font's strut metrics and paragraph line-height constraints so a line whose runs all come from fallback faces keeps at least the first family's ascent, descent and leading, matching CSS's strut.
type strutSpec struct {
	ascent, descent, leading float64
	minLineHeight            float64
	maxLineHeight            float64 // 0 when attachments allow lines to grow
}

// lineMetrics holds a line's typographic vertical bounds and whether a paragraph line-height constraint clamped them.
type lineMetrics struct {
	ascent, descent, leading float64
	clamped                  bool
}

func (m lineMetrics) aboveStep() float64 {
	if m.clamped {
		return m.ascent
	}
	return math.Round(m.ascent)
}

func (m lineMetrics) belowStep() float64 {
	if m.clamped {
		return m.descent + math.Round(m.leading)
	}
	return math.Round(m.descent + m.leading)
}

const strutEps = 1e-4

// applyLineHeight applies CoreText's minimum/maximum line height adjustment to a line's raw ascent and descent, matching CTFramesetter's half-leading distribution.
func applyLineHeight(rawA, rawD, minLH, maxLH float64) (a, d float64, clamped bool) {
	rA, rD := math.Round(rawA), math.Round(rawD)
	h := rA + rD
	var target float64
	switch {
	case minLH > 0 && h < minLH:
		target = minLH
	case maxLH > 0 && h > maxLH:
		target = maxLH
	default:
		return rawA, rawD, false
	}
	a = rA + math.Ceil((target-h)/2-0.5)
	return a, target - a, true
}

// rawLineBounds returns the maximum unadjusted ascent, descent and leading across line's glyph runs (including CTRunDelegate attachments), before CTParagraphStyle's minimum/maximum line height is applied.
func rawLineBounds(line uintptr) (ascent, descent, leading float64, ok bool) {
	runs := ctLineGetGlyphRuns(line)
	n := cfArrayGetCount(runs)
	if n == 0 {
		return 0, 0, 0, false
	}
	for i := range n {
		var ra, rd, rl float64
		ctRunGetTypographicBounds(cfArrayGetValueAtIndex(runs, i), 0, 0, &ra, &rd, &rl)
		ascent = max(ascent, ra)
		descent = max(descent, rd)
		leading = max(leading, rl)
	}
	return ascent, descent, leading, true
}

// lineMetricsFor returns line's effective vertical metrics with s's strut enforced, its typographic width, and whether the strut changed its metrics.
func lineMetricsFor(line uintptr, s strutSpec) (eff lineMetrics, width float64, adjusted bool) {
	var a, d, l float64
	width = ctLineGetTypographicBounds(line, &a, &d, &l)
	if s.minLineHeight == 0 && s.maxLineHeight == 0 {
		effA := max(a, s.ascent)
		effD := max(d, s.descent)
		effL := max(l, s.leading)
		adjusted = effA > a+strutEps || effD > d+strutEps || effL > l+strutEps
		if !adjusted {
			return lineMetrics{ascent: a, descent: d, leading: l}, width, false
		}
		return lineMetrics{ascent: effA, descent: effD, leading: effL}, width, true
	}
	rawA, rawD, rawL, ok := rawLineBounds(line)
	if !ok {
		rawA, rawD, rawL = a, d, l
	}
	effRawA := max(rawA, s.ascent)
	effRawD := max(rawD, s.descent)
	effRawL := max(rawL, s.leading)
	if effRawA <= rawA+strutEps && effRawD <= rawD+strutEps && effRawL <= rawL+strutEps {
		_, _, clamped := applyLineHeight(rawA, rawD, s.minLineHeight, s.maxLineHeight)
		return lineMetrics{ascent: a, descent: d, leading: l, clamped: clamped}, width, false
	}
	effA, effD, clamped := applyLineHeight(effRawA, effRawD, s.minLineHeight, s.maxLineHeight)
	adjusted = math.Abs(effA-a) > strutEps || math.Abs(effD-d) > strutEps || math.Abs(effRawL-l) > strutEps
	if !adjusted {
		return lineMetrics{ascent: a, descent: d, leading: l, clamped: clamped}, width, false
	}
	return lineMetrics{ascent: effA, descent: effD, leading: effRawL, clamped: clamped}, width, true
}

// stackLines computes strut-adjusted top-down baseline y-coordinates and total height from CoreText's top-down baselines ctY, effective metrics eff, and per-line adjusted flags.
func stackLines(ctY []float64, eff []lineMetrics, adj []bool) (y []float64, anyAdjusted bool, height float64) {
	n := len(ctY)
	if n == 0 {
		return nil, false, 0
	}
	for _, a := range adj {
		if a {
			anyAdjusted = true
			break
		}
	}
	if !anyAdjusted {
		return ctY, false, framedHeight(ctY[n-1] + eff[n-1].descent)
	}
	y = make([]float64, n)
	if !adj[0] {
		y[0] = ctY[0]
	} else if eff[0].clamped {
		y[0] = eff[0].aboveStep()
	} else {
		y[0] = max(ctY[0], eff[0].aboveStep())
	}
	for i := 1; i < n; i++ {
		ctGap := ctY[i] - ctY[i-1]
		var gap float64
		if !adj[i-1] && !adj[i] {
			gap = ctGap
		} else {
			wantGap := eff[i-1].belowStep() + eff[i].aboveStep()
			if eff[i-1].clamped || eff[i].clamped {
				gap = wantGap
			} else {
				gap = max(ctGap, wantGap)
			}
		}
		y[i] = y[i-1] + gap
	}
	return y, true, framedHeight(y[n-1] + eff[n-1].descent)
}

// stackFrameLines reads the first n lines of frame (a CTFrame of height frameHeight) and returns their strut-adjusted top-down baseline origins, effective line metrics, whether any line was adjusted by the strut, the total framed height, and the maximum non-trailing-whitespace line width.
func stackFrameLines(frame, arr uintptr, n int, frameHeight float64, s strutSpec) (origins []CGPoint, metrics []lineMetrics, adjusted bool, height, width float64) {
	if n == 0 {
		return nil, nil, false, 0, 0
	}
	rawOrigins := make([]CGPoint, n)
	ctFrameGetLineOrigins(frame, 0, n, &rawOrigins[0])
	ctY := make([]float64, n)
	metrics = make([]lineMetrics, n)
	adj := make([]bool, n)
	for i := range n {
		line := cfArrayGetValueAtIndex(arr, i)
		ctY[i] = frameHeight - rawOrigins[i].Y
		var lw float64
		metrics[i], lw, adj[i] = lineMetricsFor(line, s)
		width = max(width, lw-ctLineTrailingWhitespace(line))
	}
	y, adjusted, height := stackLines(ctY, metrics, adj)
	origins = make([]CGPoint, n)
	for i := range n {
		origins[i] = CGPoint{X: rawOrigins[i].X, Y: y[i]}
	}
	return origins, metrics, adjusted, height, width
}
