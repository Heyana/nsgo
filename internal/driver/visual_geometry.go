package driver

import (
	"maps"
	"math"
)

// ViewGeometry places a view in its native parent. Matrix acts in its viewport before Frame places it; Offset scrolls its children.
type ViewGeometry struct {
	Parent NodeID
	Frame  Bounds
	Matrix Matrix3D
	Offset Point
}

// GeometrySnapshot is the surface's displayed bounds after Version's commit was applied; zero predates the first diagnostic barrier. Bounds are immutable, unclipped, in surface points.
type GeometrySnapshot struct {
	Version  uint64
	Bounds   map[NodeID]Bounds
	Matrices map[NodeID]Matrix3D
	Sizes    map[NodeID]Size
}

// GeometrySource reports presentation geometry for DevTools. ObserveGeometry must not block the engine; it reports on the native thread after mounts and animation/scroll frames. A nil report ends observation.
type GeometrySource interface {
	ObserveGeometry(report func(GeometrySnapshot))
}

// GeometryObserver coalesces native-thread snapshots. A surface only builds them while Active; a new applied version is reported even if no box moved.
type GeometryObserver struct {
	Version  uint64
	report   func(GeometrySnapshot)
	last     GeometrySnapshot
	applying int
}

func (g *GeometryObserver) Observe(report func(GeometrySnapshot)) {
	g.report, g.last = report, GeometrySnapshot{}
}

func (g *GeometryObserver) Active() bool { return g.report != nil && g.applying == 0 }

func (g *GeometryObserver) Begin(version uint64) {
	g.Version = version
	g.applying++
}

func (g *GeometryObserver) End() { g.applying-- }

func (g *GeometryObserver) Notify(snapshot GeometrySnapshot) {
	snapshot.Version = g.Version
	if !g.Active() || g.last.Bounds != nil && g.last.Version == g.Version && maps.Equal(g.last.Bounds, snapshot.Bounds) && maps.Equal(g.last.Matrices, snapshot.Matrices) && maps.Equal(g.last.Sizes, snapshot.Sizes) {
		return
	}
	g.last = snapshot
	g.report(g.last)
}

// PointBounds encloses a projected border box's four corners. A box with a point at infinity has no finite outline.
func PointBounds(points [4]Point) (Bounds, bool) {
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range points {
		x, y := float64(p.X), float64(p.Y)
		if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
			return Bounds{}, false
		}
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	box := Bounds{Origin: Point{X: Points(x0), Y: Points(y0)}, Size: Size{Width: Points(x1 - x0), Height: Points(y1 - y0)}}
	for _, value := range []Points{box.Origin.X, box.Origin.Y, box.Size.Width, box.Size.Height} {
		if math.IsInf(float64(value), 0) {
			return Bounds{}, false
		}
	}
	return box, true
}

// GeometryBounds projects every view's border box through its native ancestors, flattening each viewport as IntoView does in reverse. It includes transforms, perspective, native frame motion, sticky shifts and scrolling supplied in views. Invalid or undisplayed planes have no finite box.
func GeometryBounds(views map[NodeID]ViewGeometry) map[NodeID]Bounds {
	return ProjectGeometry(views).Bounds
}

// ProjectGeometry projects native viewports and retains their local-to-surface matrices and sizes for transformed audit outlines and picking.
func ProjectGeometry(views map[NodeID]ViewGeometry) GeometrySnapshot {
	matrices := make(map[NodeID]Matrix3D, len(views))
	visiting := make(map[NodeID]bool)
	var matrix func(NodeID) (Matrix3D, bool)
	matrix = func(id NodeID) (Matrix3D, bool) {
		if m, ok := matrices[id]; ok {
			return m, true
		}
		v, ok := views[id]
		if !ok || visiting[id] {
			return Matrix3D{}, false
		}
		visiting[id] = true
		defer delete(visiting, id)
		m := v.Matrix
		if m == (Matrix3D{}) {
			m = Identity3D
		}
		// Native hierarchies flatten a child's plane at each viewport.
		m[2], m[6], m[8], m[9], m[10], m[11], m[14] = 0, 0, 0, 0, 1, 0, 0
		at := v.Frame.Origin
		if v.Parent != 0 {
			parent, ok := views[v.Parent]
			if !ok {
				return Matrix3D{}, false
			}
			at.X, at.Y = at.X-parent.Offset.X, at.Y-parent.Offset.Y
		}
		m = Translate3D(float64(at.X), float64(at.Y), 0).Mul(m)
		if v.Parent != 0 {
			up, ok := matrix(v.Parent)
			if !ok {
				return Matrix3D{}, false
			}
			m = up.Mul(m)
		}
		matrices[id] = m
		return m, true
	}
	out := GeometrySnapshot{Bounds: make(map[NodeID]Bounds, len(views)), Matrices: make(map[NodeID]Matrix3D, len(views)), Sizes: make(map[NodeID]Size, len(views))}
	for id, v := range views {
		m, ok := matrix(id)
		if !ok {
			continue
		}
		var points [4]Point
		for i, p := range [4]Point{{}, {X: v.Frame.Size.Width}, {X: v.Frame.Size.Width, Y: v.Frame.Size.Height}, {Y: v.Frame.Size.Height}} {
			x, y := float64(p.X), float64(p.Y)
			w := m[3]*x + m[7]*y + m[15]
			if w <= 1e-12 {
				ok = false
				break
			}
			x, y = m.Apply(x, y)
			if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
				ok = false
				break
			}
			points[i] = Point{X: Points(x), Y: Points(y)}
		}
		if box, finite := PointBounds(points); ok && finite {
			out.Bounds[id], out.Matrices[id], out.Sizes[id] = box, m, v.Frame.Size
		}
	}
	return out
}

// MatrixFromQuad maps a viewport to its four projected border corners. It recovers a planar homography, including perspective, without guessing the native animation's parameters or its start time.
func MatrixFromQuad(quad [4]Point, size Size) (Matrix3D, bool) {
	if size.Width <= 0 || size.Height <= 0 {
		return Matrix3D{}, false
	}
	if _, ok := PointBounds(quad); !ok {
		return Matrix3D{}, false
	}
	x0, y0, x1, y1, x2, y2, x3, y3 := float64(quad[0].X), float64(quad[0].Y), float64(quad[1].X), float64(quad[1].Y), float64(quad[2].X), float64(quad[2].Y), float64(quad[3].X), float64(quad[3].Y)
	dx1, dy1, dx2, dy2 := x1-x2, y1-y2, x3-x2, y3-y2
	sx, sy := x0-x1+x2-x3, y0-y1+y2-y3
	var g, h float64
	if math.Abs(sx)+math.Abs(sy) > 1e-9 {
		det := dx1*dy2 - dx2*dy1
		if math.Abs(det) < 1e-12 {
			return Matrix3D{}, false
		}
		g, h = (sx*dy2-dx2*sy)/det, (dx1*sy-sx*dy1)/det
	}
	w, height := float64(size.Width), float64(size.Height)
	m := Matrix3D{(x1 - x0 + g*x1) / w, (y1 - y0 + g*y1) / w, 0, g / w, (x3 - x0 + h*x3) / height, (y3 - y0 + h*y3) / height, 0, h / height, 0, 0, 1, 0, x0, y0, 0, 1}
	_, ok := m.Invert()
	return m, ok
}
