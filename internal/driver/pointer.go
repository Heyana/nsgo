package driver

import "math"

// Unproject returns the point of the plane z=0 that m draws at p. It inverts the plane's homography, rather than applying the 4D inverse to a screen point whose depth is unknown. A collapsed plane, a point at infinity or behind the viewer has no hit.
func (m Matrix3D) Unproject(p Point) (Point, bool) {
	if m == Identity3D {
		return p, true
	}
	x, y := float64(p.X), float64(p.Y)
	a, b, c := m[0]-x*m[3], m[4]-x*m[7], x*m[15]-m[12]
	d, e, f := m[1]-y*m[3], m[5]-y*m[7], y*m[15]-m[13]
	det := a*e - b*d
	if math.Abs(det) < 1e-12 || math.IsNaN(det) || math.IsInf(det, 0) {
		return Point{}, false
	}
	u, v := (c*e-b*f)/det, (a*f-c*d)/det
	w := m[3]*u + m[7]*v + m[15]
	if w <= 1e-12 || math.IsNaN(u) || math.IsNaN(v) || math.IsInf(u, 0) || math.IsInf(v, 0) {
		return Point{}, false
	}
	return Point{X: Points(u), Y: Points(v)}, true
}

// IntoView maps p in the parent's content into a view's viewport. m acts about the view's top left, after its frame places it.
func IntoView(frame Bounds, m Matrix3D, p Point) (Point, bool) {
	return m.Unproject(Point{X: p.X - frame.Origin.X, Y: p.Y - frame.Origin.Y})
}

// ImagePixelAt maps p to the bitmap pixel in the fitted image, whose
// first row is at the top. Points outside the fitted image miss.
func ImagePixelAt(fit Bounds, p Point, width, height int) (x, y int, ok bool) {
	if width <= 0 || height <= 0 || !fit.Contains(p) {
		return 0, 0, false
	}
	x = int(float64(p.X-fit.Origin.X) * float64(width) / float64(fit.Size.Width))
	y = int(float64(p.Y-fit.Origin.Y) * float64(height) / float64(fit.Size.Height))
	return min(x, width-1), min(y, height-1), true
}

// ReachView is Reach for p in a view's viewport, its transform already undone, with the area and transformed flag ItemOverflow gave the view. A view whose children may be transformed has its children searched at any point, since a child or its animation can draw anywhere; the others keep the search to their bounds and layout area, so a hit test skips untransformed subtrees the point cannot reach. Hit slop extends only the view itself, in its own coordinates.
func ReachView(size Size, props *ViewProps, area Bounds, transformed bool, p Point) (self, children bool) {
	self, children = Reach(Bounds{Size: size}, props, area, p)
	if transformed && !children && (props == nil || props.PointerEvents == PointerEventsAuto || props.PointerEvents == PointerEventsBoxNone) {
		children = true
	}
	return self, children
}
