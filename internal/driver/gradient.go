package driver

import "math"

// Gradient is a gradient image, as CSS's linear-gradient(),
// radial-gradient() and conic-gradient() and their repeating forms. A
// view draws its Gradients over its Background, the first on top.
type Gradient struct {
	Kind GradientKind
	// Angle points a linear gradient, in degrees clockwise from up, as
	// CSS's angles: 90 runs left to right, 180 top to bottom. ToCorner,
	// when set, points it at a corner instead, along the angle that
	// depends on the box's shape. A conic gradient starts at Angle.
	Angle    float32
	ToCorner GradientCorner
	// Circle makes a radial gradient a circle rather than an ellipse of
	// the box's proportions; Extent is how far it reaches.
	Circle bool
	Extent GradientExtent
	// Center is where radial and conic gradients start; its zero value is
	// the box's center, and its percentages are of the box's size.
	Center ObjectPosition
	// Stops are the colors, at least one, in order.
	Stops []ColorStop
	// Repeating repeats the stops past the last, as repeating-*-gradient().
	Repeating bool
	// Space is where colors mix between stops.
	Space ColorSpace
}

// GradientKind is a gradient's geometry.
type GradientKind uint8

const (
	// GradientLinear runs along a line through the box's center.
	GradientLinear GradientKind = iota
	// GradientRadial spreads out from Center to its Extent.
	GradientRadial
	// GradientConic turns around Center, the stops at fractions of a turn.
	GradientConic
)

// GradientCorner points a linear gradient at a corner: CSS's "to top
// right".
type GradientCorner uint8

const (
	ToTopLeft GradientCorner = iota + 1
	ToTopRight
	ToBottomRight
	ToBottomLeft
)

// GradientExtent is how far a radial gradient's last stop reaches.
type GradientExtent uint8

const (
	// ExtentFarthestCorner, the zero value, reaches the farthest corner,
	// as CSS's default.
	ExtentFarthestCorner GradientExtent = iota
	ExtentClosestSide
	ExtentClosestCorner
	ExtentFarthestSide
)

// ColorSpace is where a gradient's colors mix between stops.
type ColorSpace uint8

const (
	// ColorSpaceOklab, the zero value, mixes perceptually, without the
	// grey middle sRGB gives complementary colors; Tailwind's gradients
	// mix in it.
	ColorSpaceOklab ColorSpace = iota
	// ColorSpaceSRGB mixes the channels, as CSS's gradients of plain
	// colors do.
	ColorSpaceSRGB
	ColorSpaceLinearSRGB
	// ColorSpaceOklch mixes lightness, chroma and hue, the hue the
	// shorter way round.
	ColorSpaceOklch
)

// ColorStop is a color at a place along a gradient: a percentage of its
// line (of a turn for a conic gradient) or a length from its start. An
// auto At, the zero value, spreads the stop evenly between its neighbors.
type ColorStop struct {
	Color Color
	At    Length
}

// GradientPaint is a gradient resolved against a box, what platforms
// draw. Its geometry is in fractions of the box's width and height, so it
// stretches with the box, and its stops run from 0 to 1, sorted, repeats
// laid out and mixed so that platforms can blend each pair of stops
// channel by channel in sRGB, unpremultiplied, and pad past the ends.
type GradientPaint struct {
	Kind GradientKind
	// X0, Y0 to X1, Y1 is a linear gradient's line, from its first stop to
	// its last.
	X0, Y0, X1, Y1 float32
	// CX, CY is the center of radial and conic gradients; RX and RY are a
	// radial gradient's radii, where its last stop is.
	CX, CY, RX, RY float32
	// Angle is where a conic gradient's first stop is, in degrees
	// clockwise from up.
	Angle float32
	Stops []GradientStop
}

// blendChannels mixes colors channel by channel, unpremultiplied, as
// platforms blend gradient stops.
const blendChannels ColorSpace = 255

// maxStops bounds the stops a repeating gradient lays out.
const maxStops = 512

// Paint resolves g against a box of size: what platforms draw. It
// reports false when g has no stops.
func (g *Gradient) Paint(size Size) (GradientPaint, bool) {
	if len(g.Stops) == 0 {
		return GradientPaint{}, false
	}
	w, h := float64(size.Width), float64(size.Height)
	p := GradientPaint{Kind: g.Kind}
	var line float64 // what lengths of stops are fractions of
	// reach is how far, in units of the gradient line, the box goes.
	reach := 1.0
	switch g.Kind {
	case GradientLinear:
		angle := float64(g.Angle) * math.Pi / 180
		if g.ToCorner != 0 {
			angle = cornerAngle(g.ToCorner, w, h)
		}
		s, c := math.Sincos(angle)
		line = math.Abs(w*s) + math.Abs(h*c)
		dx, dy := s*line/2, -c*line/2
		p.X0, p.Y0 = fraction(w/2-dx, w), fraction(h/2-dy, h)
		p.X1, p.Y1 = fraction(w/2+dx, w), fraction(h/2+dy, h)
	case GradientRadial:
		at := g.Center.Place(size, Size{})
		cx, cy := float64(at.X), float64(at.Y)
		rx, ry := radialRadii(g.Circle, g.Extent, cx, cy, w, h)
		line = rx
		p.CX, p.CY = fraction(cx, w), fraction(cy, h)
		p.RX, p.RY = fraction(rx, w), fraction(ry, h)
		if rx > 0 && ry > 0 {
			for _, corner := range [4][2]float64{{0, 0}, {w, 0}, {w, h}, {0, h}} {
				reach = max(reach, math.Hypot((corner[0]-cx)/rx, (corner[1]-cy)/ry))
			}
		}
	case GradientConic:
		at := g.Center.Place(size, Size{})
		p.CX, p.CY = fraction(float64(at.X), w), fraction(float64(at.Y), h)
		p.Angle = g.Angle
	}
	stops := resolveStops(g.Stops, line)
	first, last := stops[0].Offset, stops[len(stops)-1].Offset
	if period := last - first; g.Repeating && period > 0 {
		stops = repeatStops(stops, period, float32(reach))
	}
	if g.Kind == GradientRadial {
		// Stops past the ending shape reach further out: the radii grow
		// to the last one, inside the box.
		end := min(max(stops[len(stops)-1].Offset, 1), float32(reach))
		p.RX, p.RY = p.RX*end, p.RY*end
		for i := range stops {
			stops[i].Offset /= end
		}
	}
	p.Stops = mixStops(clipStops(stops), g.Space)
	return p, true
}

// fraction is v as a fraction of side, 0 for an empty side.
func fraction(v, side float64) float32 {
	if side <= 0 {
		return 0
	}
	return float32(v / side)
}

// cornerAngle is the angle of a gradient to a corner of a w by h box:
// into the corner's quadrant, perpendicular to the line through the two
// corners beside it, so that both lie halfway along it.
func cornerAngle(c GradientCorner, w, h float64) float64 {
	a := math.Atan(max(h, 1e-6) / max(w, 1e-6))
	switch c {
	case ToTopRight:
		return a
	case ToBottomRight:
		return math.Pi - a
	case ToBottomLeft:
		return math.Pi + a
	}
	return 2*math.Pi - a
}

// radialRadii returns the radii of a radial gradient's ending shape
// centered at cx, cy in a w by h box.
func radialRadii(circle bool, extent GradientExtent, cx, cy, w, h float64) (float64, float64) {
	nearX, nearY := math.Abs(min(cx, w-cx)), math.Abs(min(cy, h-cy))
	farX, farY := math.Abs(max(cx, w-cx)), math.Abs(max(cy, h-cy))
	if circle {
		var r float64
		switch extent {
		case ExtentClosestSide:
			r = min(nearX, nearY)
		case ExtentFarthestSide:
			r = max(farX, farY)
		case ExtentClosestCorner:
			r = math.Hypot(nearX, nearY)
		default:
			r = math.Hypot(farX, farY)
		}
		return r, r
	}
	// An ellipse to a corner keeps the proportions of the one to its sides.
	switch extent {
	case ExtentClosestSide:
		return nearX, nearY
	case ExtentFarthestSide:
		return farX, farY
	case ExtentClosestCorner:
		return nearX * math.Sqrt2, nearY * math.Sqrt2
	}
	return farX * math.Sqrt2, farY * math.Sqrt2
}

// resolveStops places stops as CSS does: lengths against line, the first
// and last at the ends when auto, each at least the one before it, the
// rest spread evenly between their neighbors.
func resolveStops(in []ColorStop, line float64) []GradientStop {
	out := make([]GradientStop, len(in), len(in)+1)
	nan := float32(math.NaN())
	for i, s := range in {
		out[i] = GradientStop{Color: s.Color, Offset: nan}
		if pct, ok := s.At.Percent(); ok {
			out[i].Offset = pct / 100
		} else if pt, ok := s.At.Points(); ok {
			switch {
			case line > 0:
				out[i].Offset = float32(float64(pt) / line)
			case pt > 0:
				out[i].Offset = 1
			default:
				out[i].Offset = 0
			}
		}
	}
	n := len(out)
	if out[0].Offset != out[0].Offset {
		out[0].Offset = 0
	}
	if out[n-1].Offset != out[n-1].Offset {
		out[n-1].Offset = 1
	}
	most := float32(math.Inf(-1))
	for i := range out {
		if out[i].Offset == out[i].Offset {
			most = max(most, out[i].Offset)
			out[i].Offset = most
		}
	}
	for i := 1; i < n; {
		if out[i].Offset == out[i].Offset {
			i++
			continue
		}
		start, end := i-1, i
		for out[end].Offset != out[end].Offset {
			end++
		}
		a, b := out[start].Offset, out[end].Offset
		for k := i; k < end; k++ {
			out[k].Offset = a + (b-a)*float32(k-start)/float32(end-start)
		}
		i = end
	}
	if n == 1 {
		out = append(out, GradientStop{Color: out[0].Color, Offset: 1})
		out[0].Offset = 0
	}
	return out
}

// repeatStops lays out copies of stops, period apart, from at most 0 to
// at least reach.
func repeatStops(stops []GradientStop, period, reach float32) []GradientStop {
	first := stops[0].Offset
	k := float32(math.Floor(float64(first / period)))
	start := first - k*period
	if start > 0 {
		start -= period
	}
	var out []GradientStop
	for offset := start - first; offset+first < reach && len(out) < maxStops; offset += period {
		for _, s := range stops {
			out = append(out, GradientStop{Color: s.Color, Offset: s.Offset + offset})
		}
	}
	return out
}

// clipStops cuts stops to the gradient from 0 to 1, with the colors
// there in place of what lies outside.
func clipStops(stops []GradientStop) []GradientStop {
	out := stops[:0:0]
	for i, s := range stops {
		if i > 0 {
			prev := stops[i-1]
			for _, edge := range [2]float32{0, 1} {
				if prev.Offset < edge && s.Offset > edge {
					t := (edge - prev.Offset) / (s.Offset - prev.Offset)
					out = append(out, GradientStop{Color: MixColors(prev.Color, s.Color, t, ColorSpaceSRGB), Offset: edge})
				}
			}
		}
		if s.Offset >= 0 && s.Offset <= 1 {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		// Every stop is on one side: the gradient is that side's color.
		c := stops[len(stops)-1].Color
		if stops[0].Offset > 1 {
			c = stops[0].Color
		}
		return []GradientStop{{0, c}, {1, c}}
	}
	if out[0].Offset > 0 {
		out = append([]GradientStop{{0, out[0].Color}}, out...)
	}
	if out[len(out)-1].Offset < 1 {
		out = append(out, GradientStop{1, out[len(out)-1].Color})
	}
	return out
}

// mixStops adds stops between each pair, so that blending them channel
// by channel follows space within about one 8-bit step: bisected where
// the curve strays, at most 32 per pair.
func mixStops(stops []GradientStop, space ColorSpace) []GradientStop {
	out := make([]GradientStop, 0, len(stops))
	for i, s := range stops {
		if i > 0 {
			prev := stops[i-1]
			if prev.Color != s.Color && s.Offset > prev.Offset {
				budget := 32
				out = bisect(out, prev, s, 0, 1, space, &budget)
			}
		}
		out = append(out, s)
	}
	return out
}

// bisect appends the stops strictly between t0 and t1 of the pair a, b
// that sampling in space needs.
func bisect(out []GradientStop, a, b GradientStop, t0, t1 float32, space ColorSpace, budget *int) []GradientStop {
	c0, c1 := MixColors(a.Color, b.Color, t0, space), MixColors(a.Color, b.Color, t1, space)
	mid := (t0 + t1) / 2
	want := MixColors(a.Color, b.Color, mid, space)
	if *budget <= 0 || colorDistance(want, MixColors(c0, c1, 0.5, blendChannels)) <= 1 {
		return out
	}
	*budget--
	out = bisect(out, a, b, t0, mid, space, budget)
	out = append(out, GradientStop{Color: want, Offset: a.Offset + (b.Offset-a.Offset)*mid})
	return bisect(out, a, b, mid, t1, space, budget)
}

// colorDistance is the largest difference between two colors' channels,
// in 8-bit steps.
func colorDistance(a, b Color) int {
	d := 0
	for shift := 0; shift < 32; shift += 8 {
		x, y := int(a>>shift&0xff), int(b>>shift&0xff)
		d = max(d, x-y, y-x)
	}
	return d
}

// MixColors returns the color t of the way from a to b, mixed in space
// with premultiplied alpha, as CSS's gradients and color-mix() mix.
func MixColors(a, b Color, t float32, space ColorSpace) Color {
	ar, ag, ab, aa := a.Components()
	br, bg, bb, ba := b.Components()
	f := float64(t)
	lerp := func(x, y float64) float64 { return x + (y-x)*f }
	if space == blendChannels {
		return packColor([4]float64{lerp(ar, br), lerp(ag, bg), lerp(ab, bb), lerp(aa, ba)})
	}
	alpha := lerp(aa, ba)
	if alpha <= 0 {
		return 0
	}
	x := toSpace(ar, ag, ab, space)
	y := toSpace(br, bg, bb, space)
	// A transparent end takes the other's color, so no grey creeps in.
	if aa == 0 {
		x = y
	}
	if ba == 0 {
		y = x
	}
	var m [3]float64
	for i := range m {
		m[i] = (x[i]*aa + (y[i]*ba-x[i]*aa)*f) / alpha
	}
	if space == ColorSpaceOklch {
		// Hues take the shorter way, without premultiplying.
		hx, hy := x[2], y[2]
		if d := hy - hx; d > 180 {
			hx += 360
		} else if d < -180 {
			hy += 360
		}
		m[2] = math.Mod(lerp(hx, hy)+360, 360)
		if x[1] < 1e-4 {
			m[2] = y[2]
		} else if y[1] < 1e-4 {
			m[2] = x[2]
		}
	}
	r, g, bl := fromSpace(m, space)
	return packColor([4]float64{r, g, bl, alpha})
}

func toLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func fromLinear(c float64) float64 {
	if c <= 0.0031308 {
		return c * 12.92
	}
	return 1.055*math.Pow(c, 1/2.4) - 0.055
}

// toSpace converts sRGB channels to space's coordinates.
func toSpace(r, g, b float64, space ColorSpace) [3]float64 {
	switch space {
	case ColorSpaceSRGB:
		return [3]float64{r, g, b}
	case ColorSpaceLinearSRGB:
		return [3]float64{toLinear(r), toLinear(g), toLinear(b)}
	}
	r, g, b = toLinear(r), toLinear(g), toLinear(b)
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	lab := [3]float64{
		0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
	if space == ColorSpaceOklch {
		h := math.Atan2(lab[2], lab[1]) * 180 / math.Pi
		return [3]float64{lab[0], math.Hypot(lab[1], lab[2]), math.Mod(h+360, 360)}
	}
	return lab
}

// fromSpace converts space's coordinates to sRGB channels.
func fromSpace(c [3]float64, space ColorSpace) (r, g, b float64) {
	switch space {
	case ColorSpaceSRGB:
		return c[0], c[1], c[2]
	case ColorSpaceLinearSRGB:
		return fromLinear(c[0]), fromLinear(c[1]), fromLinear(c[2])
	case ColorSpaceOklch:
		s, co := math.Sincos(c[2] * math.Pi / 180)
		c = [3]float64{c[0], c[1] * co, c[1] * s}
	}
	l := c[0] + 0.3963377774*c[1] + 0.2158037573*c[2]
	m := c[0] - 0.1055613458*c[1] - 0.0638541728*c[2]
	s := c[0] - 0.0894841775*c[1] - 1.2914855480*c[2]
	l, m, s = l*l*l, m*m*m, s*s*s
	return fromLinear(4.0767416621*l - 3.3077115913*m + 0.2309699292*s),
		fromLinear(-1.2684380046*l + 2.6097574011*m - 0.3413193965*s),
		fromLinear(-0.0041960863*l - 0.7034186147*m + 1.7076147010*s)
}

// ColorAt returns the gradient's color at fraction t of its line, as
// platforms blend it: for those that draw a gradient pixel by pixel.
func (p *GradientPaint) ColorAt(t float32) Color {
	s := p.Stops
	if len(s) == 0 {
		return 0
	}
	if t <= s[0].Offset {
		return s[0].Color
	}
	for i := 1; i < len(s); i++ {
		if t <= s[i].Offset {
			a, b := s[i-1], s[i]
			if b.Offset <= a.Offset {
				return b.Color
			}
			return MixColors(a.Color, b.Color, (t-a.Offset)/(b.Offset-a.Offset), blendChannels)
		}
	}
	return s[len(s)-1].Color
}

// Equal reports whether two paints draw the same.
func (p *GradientPaint) Equal(o *GradientPaint) bool {
	if p == o {
		return true
	}
	if p == nil || o == nil || len(p.Stops) != len(o.Stops) {
		return false
	}
	a, b := *p, *o
	a.Stops, b.Stops = nil, nil
	if a.Kind != b.Kind || a.X0 != b.X0 || a.Y0 != b.Y0 || a.X1 != b.X1 || a.Y1 != b.Y1 ||
		a.CX != b.CX || a.CY != b.CY || a.RX != b.RX || a.RY != b.RY || a.Angle != b.Angle {
		return false
	}
	for i := range p.Stops {
		if p.Stops[i] != o.Stops[i] {
			return false
		}
	}
	return true
}
