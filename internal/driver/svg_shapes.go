package driver

import (
	"fmt"
	"math"
)

// rect adds a rect element's shape.
func (p *svgParser) rect(a map[string]string) error {
	x, err := p.length(a, "x")
	if err != nil {
		return err
	}
	y, err := p.length(a, "y")
	if err != nil {
		return err
	}
	w, err := p.length(a, "width")
	if err != nil {
		return err
	}
	h, err := p.length(a, "height")
	if err != nil {
		return err
	}
	if w <= 0 || h <= 0 {
		return nil
	}
	rx, err := p.length(a, "rx")
	if err != nil {
		return err
	}
	ry, err := p.length(a, "ry")
	if err != nil {
		return err
	}
	if rx > 0 && ry == 0 {
		ry = rx
	} else if ry > 0 && rx == 0 {
		rx = ry
	}
	rx, ry = min(rx, w/2), min(ry, h/2)
	p.begin()
	switch {
	case rx <= 0 || ry <= 0:
		p.lop(DrawOp{Kind: DrawRect, X: x, Y: y, W: w, H: h})
	case rx == ry:
		p.lop(DrawOp{Kind: DrawRoundRect, X: x, Y: y, W: w, H: h, R: rx})
	default:
		p.local = roundRectPath(p.local, x, y, w, h, rx, ry)
	}
	return nil
}

// roundRectPath appends a rectangle with elliptical corners of radii rx
// and ry, as an elliptical arc each, to ops.
func roundRectPath(ops []DrawOp, x, y, w, h, rx, ry float32) []DrawOp {
	ops = append(ops, DrawOp{Kind: DrawMoveTo, X: x + rx, Y: y},
		DrawOp{Kind: DrawLineTo, X: x + w - rx, Y: y})
	ops = appendCorner(ops, x+w-rx, y+ry, rx, ry, -90, 0)
	ops = append(ops, DrawOp{Kind: DrawLineTo, X: x + w, Y: y + h - ry})
	ops = appendCorner(ops, x+w-rx, y+h-ry, rx, ry, 0, 90)
	ops = append(ops, DrawOp{Kind: DrawLineTo, X: x + rx, Y: y + h})
	ops = appendCorner(ops, x+rx, y+h-ry, rx, ry, 90, 180)
	ops = append(ops, DrawOp{Kind: DrawLineTo, X: x, Y: y + ry})
	ops = appendCorner(ops, x+rx, y+ry, rx, ry, 180, 270)
	return append(ops, DrawOp{Kind: DrawClosePath})
}

// appendCorner appends an axis-aligned elliptical arc around (cx, cy)
// between two angles in degrees to ops, as a cubic curve per quarter turn.
func appendCorner(ops []DrawOp, cx, cy, rx, ry float32, from, to float64) []DrawOp {
	a0 := from * math.Pi / 180
	a1 := to * math.Pi / 180
	n := int(math.Ceil(math.Abs(a1-a0) / (math.Pi / 2)))
	step := (a1 - a0) / float64(n)
	for i := range n {
		t0 := a0 + float64(i)*step
		t1 := t0 + step
		alpha := 4.0 / 3.0 * math.Tan((t1-t0)/4)
		x0, y0 := float64(cx)+float64(rx)*math.Cos(t0), float64(cy)+float64(ry)*math.Sin(t0)
		x1, y1 := float64(cx)+float64(rx)*math.Cos(t1), float64(cy)+float64(ry)*math.Sin(t1)
		d0x, d0y := -float64(rx)*math.Sin(t0), float64(ry)*math.Cos(t0)
		d1x, d1y := -float64(rx)*math.Sin(t1), float64(ry)*math.Cos(t1)
		ops = append(ops, DrawOp{Kind: DrawCubicTo, X1: float32(x0 + alpha*d0x), Y1: float32(y0 + alpha*d0y),
			X2: float32(x1 - alpha*d1x), Y2: float32(y1 - alpha*d1y), X: float32(x1), Y: float32(y1)})
	}
	return ops
}

func (p *svgParser) circle(a map[string]string) error {
	cx, err := p.length(a, "cx")
	if err != nil {
		return err
	}
	cy, err := p.length(a, "cy")
	if err != nil {
		return err
	}
	r, err := p.length(a, "r")
	if err != nil {
		return err
	}
	if r > 0 {
		p.begin()
		p.lop(DrawOp{Kind: DrawEllipse, X: cx, Y: cy, R: r, RY: r})
	}
	return nil
}

func (p *svgParser) ellipse(a map[string]string) error {
	cx, err := p.length(a, "cx")
	if err != nil {
		return err
	}
	cy, err := p.length(a, "cy")
	if err != nil {
		return err
	}
	rx, err := p.length(a, "rx")
	if err != nil {
		return err
	}
	ry, err := p.length(a, "ry")
	if err != nil {
		return err
	}
	if rx > 0 && ry > 0 {
		p.begin()
		p.lop(DrawOp{Kind: DrawEllipse, X: cx, Y: cy, R: rx, RY: ry})
	}
	return nil
}

func (p *svgParser) line(a map[string]string) error {
	x1, err := p.length(a, "x1")
	if err != nil {
		return err
	}
	y1, err := p.length(a, "y1")
	if err != nil {
		return err
	}
	x2, err := p.length(a, "x2")
	if err != nil {
		return err
	}
	y2, err := p.length(a, "y2")
	if err != nil {
		return err
	}
	p.begin()
	p.lop(DrawOp{Kind: DrawMoveTo, X: x1, Y: y1})
	p.lop(DrawOp{Kind: DrawLineTo, X: x2, Y: y2})
	return nil
}

func (p *svgParser) polygon(a map[string]string, closed bool) error {
	values, err := svgNumbers(a["points"], -1)
	if err != nil {
		return fmt.Errorf("svg: points: %w", err)
	}
	if len(values) < 4 || len(values)%2 != 0 {
		return nil
	}
	p.begin()
	p.lop(DrawOp{Kind: DrawMoveTo, X: values[0], Y: values[1]})
	for i := 2; i < len(values); i += 2 {
		p.lop(DrawOp{Kind: DrawLineTo, X: values[i], Y: values[i+1]})
	}
	if closed {
		p.lop(DrawOp{Kind: DrawClosePath})
	}
	return nil
}

// path reads path data and adds its shape. It understands every command of
// SVG 1.1's path grammar; arcs become cubic curves.
func (p *svgParser) path(d string) error {
	p.begin()
	sc := svgScanner{s: d}
	var (
		x, y   float32 // the current point
		sx, sy float32 // the subpath's start
		ctrlX  float32 // the last cubic control point
		ctrlY  float32
		quadX  float32 // the last quadratic control point
		quadY  float32
		last   byte // the command repeated by bare numbers
		prev   byte // the command of the segment before
	)
	num := func() (float32, error) {
		n, ok := sc.number()
		if !ok {
			return 0, fmt.Errorf("svg: path data %q: expected a number", d)
		}
		return n, nil
	}
	rel := func(cmd byte) bool { return cmd >= 'a' && cmd <= 'z' }
	for {
		sc.skip()
		if sc.eof() {
			return nil
		}
		var cmd byte
		if b := sc.peek(); isPathCommand(b) {
			cmd = sc.next()
		} else if last != 0 {
			cmd = last
		} else {
			return fmt.Errorf("svg: path data %q: expected a command", d)
		}
		lower := rel(cmd)
		toX := func(v float32) float32 {
			if lower {
				return x + v
			}
			return v
		}
		toY := func(v float32) float32 {
			if lower {
				return y + v
			}
			return v
		}
		switch cmd {
		case 'M', 'm':
			nx, err := num()
			if err != nil {
				return err
			}
			ny, err := num()
			if err != nil {
				return err
			}
			x, y = toX(nx), toY(ny)
			sx, sy = x, y
			p.lop(DrawOp{Kind: DrawMoveTo, X: x, Y: y})
			prev = 'M'
			last = 'L'
			if lower {
				last = 'l'
			}
			for sc.numberStart() {
				nx, err := num()
				if err != nil {
					return err
				}
				ny, err := num()
				if err != nil {
					return err
				}
				x, y = toX(nx), toY(ny)
				p.lop(DrawOp{Kind: DrawLineTo, X: x, Y: y})
				prev = 'L'
			}
		case 'L', 'l':
			if !sc.numberStart() {
				return fmt.Errorf("svg: path data %q: L needs a point", d)
			}
			for sc.numberStart() {
				nx, err := num()
				if err != nil {
					return err
				}
				ny, err := num()
				if err != nil {
					return err
				}
				x, y = toX(nx), toY(ny)
				p.lop(DrawOp{Kind: DrawLineTo, X: x, Y: y})
				prev = 'L'
			}
			last = cmd
		case 'H', 'h':
			if !sc.numberStart() {
				return fmt.Errorf("svg: path data %q: H needs a number", d)
			}
			for sc.numberStart() {
				nx, err := num()
				if err != nil {
					return err
				}
				x = toX(nx)
				p.lop(DrawOp{Kind: DrawLineTo, X: x, Y: y})
				prev = 'L'
			}
			last = cmd
		case 'V', 'v':
			if !sc.numberStart() {
				return fmt.Errorf("svg: path data %q: V needs a number", d)
			}
			for sc.numberStart() {
				ny, err := num()
				if err != nil {
					return err
				}
				y = toY(ny)
				p.lop(DrawOp{Kind: DrawLineTo, X: x, Y: y})
				prev = 'L'
			}
			last = cmd
		case 'C', 'c':
			for {
				if !sc.numberStart() {
					break
				}
				x1, err := num()
				if err != nil {
					return err
				}
				y1, err := num()
				if err != nil {
					return err
				}
				x2, err := num()
				if err != nil {
					return err
				}
				y2, err := num()
				if err != nil {
					return err
				}
				nx, err := num()
				if err != nil {
					return err
				}
				ny, err := num()
				if err != nil {
					return err
				}
				x1, y1, x2, y2 = toX(x1), toY(y1), toX(x2), toY(y2)
				x, y = toX(nx), toY(ny)
				p.lop(DrawOp{Kind: DrawCubicTo, X1: x1, Y1: y1, X2: x2, Y2: y2, X: x, Y: y})
				ctrlX, ctrlY, prev = x2, y2, 'C'
			}
			last = cmd
		case 'S', 's':
			for {
				if !sc.numberStart() {
					break
				}
				x2, err := num()
				if err != nil {
					return err
				}
				y2, err := num()
				if err != nil {
					return err
				}
				nx, err := num()
				if err != nil {
					return err
				}
				ny, err := num()
				if err != nil {
					return err
				}
				x1, y1 := x, y
				if prev == 'C' || prev == 'S' {
					x1, y1 = 2*x-ctrlX, 2*y-ctrlY
				}
				x2, y2 = toX(x2), toY(y2)
				x, y = toX(nx), toY(ny)
				p.lop(DrawOp{Kind: DrawCubicTo, X1: x1, Y1: y1, X2: x2, Y2: y2, X: x, Y: y})
				ctrlX, ctrlY, prev = x2, y2, 'S'
			}
			last = cmd
		case 'Q', 'q':
			for {
				if !sc.numberStart() {
					break
				}
				qx, err := num()
				if err != nil {
					return err
				}
				qy, err := num()
				if err != nil {
					return err
				}
				nx, err := num()
				if err != nil {
					return err
				}
				ny, err := num()
				if err != nil {
					return err
				}
				qx, qy = toX(qx), toY(qy)
				x, y = toX(nx), toY(ny)
				p.lop(DrawOp{Kind: DrawQuadTo, X1: qx, Y1: qy, X: x, Y: y})
				quadX, quadY, prev = qx, qy, 'Q'
			}
			last = cmd
		case 'T', 't':
			for {
				if !sc.numberStart() {
					break
				}
				nx, err := num()
				if err != nil {
					return err
				}
				ny, err := num()
				if err != nil {
					return err
				}
				qx, qy := x, y
				if prev == 'Q' || prev == 'T' {
					qx, qy = 2*x-quadX, 2*y-quadY
				}
				x, y = toX(nx), toY(ny)
				p.lop(DrawOp{Kind: DrawQuadTo, X1: qx, Y1: qy, X: x, Y: y})
				quadX, quadY, prev = qx, qy, 'T'
			}
			last = cmd
		case 'A', 'a':
			for {
				if !sc.numberStart() {
					break
				}
				rx, err := num()
				if err != nil {
					return err
				}
				ry, err := num()
				if err != nil {
					return err
				}
				rotation, err := num()
				if err != nil {
					return err
				}
				large, ok := sc.flag()
				if !ok {
					return fmt.Errorf("svg: path data %q: expected an arc flag", d)
				}
				sweep, ok := sc.flag()
				if !ok {
					return fmt.Errorf("svg: path data %q: expected an arc flag", d)
				}
				nx, err := num()
				if err != nil {
					return err
				}
				ny, err := num()
				if err != nil {
					return err
				}
				nx, ny = toX(nx), toY(ny)
				p.arc(x, y, rx, ry, rotation, large, sweep, nx, ny)
				x, y = nx, ny
				prev = 'A'
			}
			last = cmd
		case 'Z', 'z':
			p.lop(DrawOp{Kind: DrawClosePath})
			x, y = sx, sy
			prev = 'Z'
			last = 0
		default:
			return fmt.Errorf("svg: path data %q: unknown command %q", d, cmd)
		}
	}
}

// arc adds an SVG elliptical arc from (x0, y0) to (x1, y1) as cubic curves,
// following SVG 1.1's endpoint to center conversion (F.6.5).
func (p *svgParser) arc(x0, y0, rx, ry, rotation float32, large, sweep bool, x1, y1 float32) {
	if rx == 0 || ry == 0 {
		p.lop(DrawOp{Kind: DrawLineTo, X: x1, Y: y1})
		return
	}
	if x0 == x1 && y0 == y1 {
		return
	}
	rx, ry = float32(math.Abs(float64(rx))), float32(math.Abs(float64(ry)))
	phi := float64(rotation) * math.Pi / 180
	cos, sin := math.Cos(phi), math.Sin(phi)
	frx, fry := float64(rx), float64(ry)
	dx, dy := (float64(x0)-float64(x1))/2, (float64(y0)-float64(y1))/2
	xp := cos*dx + sin*dy
	yp := -sin*dx + cos*dy
	lambda := xp*xp/(frx*frx) + yp*yp/(fry*fry)
	if lambda > 1 {
		s := math.Sqrt(lambda)
		frx, fry = frx*s, fry*s
	}
	num := frx*frx*fry*fry - frx*frx*yp*yp - fry*fry*xp*xp
	den := frx*frx*yp*yp + fry*fry*xp*xp
	coef := 0.0
	if num > 0 && den > 0 {
		coef = math.Sqrt(num / den)
	}
	if large == sweep {
		coef = -coef
	}
	cxp := coef * frx * yp / fry
	cyp := -coef * fry * xp / frx
	cx := cos*cxp - sin*cyp + (float64(x0)+float64(x1))/2
	cy := sin*cxp + cos*cyp + (float64(y0)+float64(y1))/2
	ux, uy := (xp-cxp)/frx, (yp-cyp)/fry
	vx, vy := (-xp-cxp)/frx, (-yp-cyp)/fry
	a0 := math.Atan2(uy, ux)
	da := math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	if !sweep && da > 0 {
		da -= 2 * math.Pi
	}
	if sweep && da < 0 {
		da += 2 * math.Pi
	}
	segments := int(math.Ceil(math.Abs(da) / (math.Pi / 2)))
	if segments < 1 {
		segments = 1
	}
	step := da / float64(segments)
	at := func(t float64) (x, y, dx, dy float64) {
		x = cx + frx*math.Cos(t)*cos - fry*math.Sin(t)*sin
		y = cy + frx*math.Cos(t)*sin + fry*math.Sin(t)*cos
		dx = -frx*math.Sin(t)*cos - fry*math.Cos(t)*sin
		dy = -frx*math.Sin(t)*sin + fry*math.Cos(t)*cos
		return
	}
	for i := range segments {
		t0 := a0 + float64(i)*step
		t1 := t0 + step
		alpha := 4.0 / 3.0 * math.Tan((t1-t0)/4)
		sx, sy, d0x, d0y := at(t0)
		ex, ey, d1x, d1y := at(t1)
		p.lop(DrawOp{Kind: DrawCubicTo,
			X1: float32(sx + alpha*d0x), Y1: float32(sy + alpha*d0y),
			X2: float32(ex - alpha*d1x), Y2: float32(ey - alpha*d1y),
			X: float32(ex), Y: float32(ey)})
	}
}
