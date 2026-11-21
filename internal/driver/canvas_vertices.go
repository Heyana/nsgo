package driver

import "math"

// CanvasVertex is one corner of a triangle. Position is in canvas
// points. TexCoord is in image points, as in DrawImageFrom. Color
// multiplies the sampled texture. Its zero value is opaque white;
// RGB(0xffffff) is also white and RGBA(0xffffff, 0) is transparent.
type CanvasVertex struct {
	Position Point
	TexCoord Point
	Color    Color
}

// CanvasMesh is an immutable triangle list. Every three vertices form
// a triangle. Vertices are kept behind a pointer so DrawOp is comparable.
type CanvasMesh struct {
	Vertices []CanvasVertex
}

// DrawVertices draws a textured triangle list as one operation. It
// copies vertices so the caller can reuse its slice on the next frame.
// Nonfinite and zero-area triangles are ignored, as are trailing vertices
// that do not make a triangle.
func (d *Drawing) DrawVertices(img ImageSource, vertices []CanvasVertex) *Drawing {
	if img.Empty() || len(vertices) < 3 {
		return d
	}
	list := make([]CanvasVertex, 0, len(vertices)-len(vertices)%3)
	minX, minY := Points(math.Inf(1)), Points(math.Inf(1))
	maxX, maxY := -minX, -minY
	for i := 0; i+2 < len(vertices); i += 3 {
		a, b, c := vertices[i], vertices[i+1], vertices[i+2]
		if !finiteVertex(a) || !finiteVertex(b) || !finiteVertex(c) {
			continue
		}
		ap, bp, cp := a.Position, b.Position, c.Position
		x1, y1 := float64(bp.X)-float64(ap.X), float64(bp.Y)-float64(ap.Y)
		x2, y2 := float64(cp.X)-float64(ap.X), float64(cp.Y)-float64(ap.Y)
		if x1*y2 == x2*y1 {
			continue
		}
		list = append(list, a, b, c)
		minX = min(minX, ap.X, bp.X, cp.X)
		minY = min(minY, ap.Y, bp.Y, cp.Y)
		maxX = max(maxX, ap.X, bp.X, cp.X)
		maxY = max(maxY, ap.Y, bp.Y, cp.Y)
	}
	if len(list) == 0 {
		return d
	}
	return d.op(DrawOp{Kind: DrawVertices, Image: &img, Mesh: &CanvasMesh{Vertices: list},
		X: float32(minX), Y: float32(minY), W: float32(maxX - minX), H: float32(maxY - minY)})
}

func finiteVertex(v CanvasVertex) bool {
	for _, x := range [...]Points{v.Position.X, v.Position.Y, v.TexCoord.X, v.TexCoord.Y} {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return false
		}
	}
	return true
}

// ImageRect returns the equivalent image draw for two white triangles
// that cover an axis-aligned rectangular crop exactly.
func (op *DrawOp) ImageRect() (DrawOp, bool) {
	if op.Mesh == nil || len(op.Mesh.Vertices) != 6 || op.W <= 0 || op.H <= 0 {
		return DrawOp{}, false
	}
	var uv [4]Point
	var seen uint8
	var triangles [2]uint8
	for i, v := range op.Mesh.Vertices {
		if v.Color != 0 && v.Color != RGB(0xffffff) {
			return DrawOp{}, false
		}
		var corner uint8
		switch float32(v.Position.X) {
		case op.X:
		case op.X + op.W:
			corner |= 1
		default:
			return DrawOp{}, false
		}
		switch float32(v.Position.Y) {
		case op.Y:
		case op.Y + op.H:
			corner |= 2
		default:
			return DrawOp{}, false
		}
		bit := uint8(1 << corner)
		if seen&bit != 0 && uv[corner] != v.TexCoord {
			return DrawOp{}, false
		}
		seen |= bit
		triangles[i/3] |= bit
		uv[corner] = v.TexCoord
	}
	// Two valid triangles cover the rectangle when they share a diagonal.
	common := triangles[0] & triangles[1]
	if seen != 15 || (common != 0b1001 && common != 0b0110) ||
		uv[0].X != uv[2].X || uv[1].X != uv[3].X ||
		uv[0].Y != uv[1].Y || uv[2].Y != uv[3].Y ||
		uv[0].X < 0 || uv[0].Y < 0 || uv[3].X <= uv[0].X || uv[3].Y <= uv[0].Y {
		return DrawOp{}, false
	}
	return DrawOp{Kind: DrawImage, Image: op.Image, X: op.X, Y: op.Y, W: op.W, H: op.H,
		X1: float32(uv[0].X), Y1: float32(uv[0].Y), X2: float32(uv[3].X - uv[0].X), Y2: float32(uv[3].Y - uv[0].Y)}, true
}
