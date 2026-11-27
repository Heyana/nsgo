// Package canvasshade lays out the GPU pass of a canvas op that shades
// with a program (driver.DrawShader, or an image op under SetShader), for
// the backends whose canvases draw on the CPU: each renders the pass's
// triangles into a transparent texture of its own, source over, and
// draws the texture into the canvas at the pass's bounds, in the target's
// pixels, under the canvas's clip, alpha and operator.
//
// The program's dst and pos are what the vertices carry rather than the
// pixel's place in the texture: a DrawShader's rectangle in its own
// points, or the point of the image an image op draws, so a rotated or
// scaled op shades its own space. The pass's color is the program's,
// premultiplied, times the vertex's color: a sprite's alpha, a mesh
// vertex's color or a tint.
package canvasshade

import (
	"image"
	"math"
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Affine maps canvas points to the target's pixels, as the web's
// transform(a, b, c, d, e, f): (x, y) to (A*x + C*y + E, B*x + D*y + F).
type Affine struct{ A, B, C, D, E, F float64 }

// Apply maps the point x, y.
func (m Affine) Apply(x, y float64) (float64, float64) {
	return m.A*x + m.C*y + m.E, m.B*x + m.D*y + m.F
}

// Then returns m after n: n maps first.
func (m Affine) Then(n Affine) Affine {
	return Affine{
		A: m.A*n.A + m.C*n.B, B: m.B*n.A + m.D*n.B,
		C: m.A*n.C + m.C*n.D, D: m.B*n.C + m.D*n.D,
		E: m.A*n.E + m.C*n.F + m.E, F: m.B*n.E + m.D*n.F + m.F,
	}
}

// Invert returns the inverse of m, the identity when m has none.
func (m Affine) Invert() Affine {
	det := m.A*m.D - m.B*m.C
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return Affine{A: 1, D: 1}
	}
	return Affine{A: m.D / det, B: -m.B / det, C: -m.C / det, D: m.A / det,
		E: (m.C*m.F - m.D*m.E) / det, F: (m.B*m.E - m.A*m.F) / det}
}

// scale is how many pixels a point covers under m, the square root of its
// area's stretch.
func (m Affine) scale() float64 { return math.Sqrt(math.Abs(m.A*m.D - m.B*m.C)) }

// Image is the size of the image an image op draws, in its points and in
// its pixels.
type Image struct{ W, H, PixelsW, PixelsH float64 }

// Vertex is a corner of a triangle of a pass: X, Y in the pass's pixels
// from its top left corner, what the program's dst and pos interpolate,
// and the premultiplied color its color is multiplied by.
type Vertex struct {
	X, Y       float32
	DstX, DstY float32
	PosX, PosY float32
	R, G, B, A float32
}

// Floats is how many floats Pack writes for a vertex: its position in
// clip space, dst, pos and color.
const Floats = 10

// Pass is what a backend renders for one op.
type Pass struct {
	// Bounds is where the pass's texture goes, in the target's pixels.
	Bounds image.Rectangle
	// Vertices are triangles, three vertices each.
	Vertices []Vertex
	// Size and Scale are what the program's rectSize and pixelScale
	// return.
	Size  [2]float64
	Scale float64
	// Images are what the program reads as image0 on: an image op's
	// image first, then the shader's own.
	Images []driver.ImageSource
	// Shader is the op's.
	Shader *driver.Shader
}

// Shades reports whether op shades with a program a backend can run: a
// DrawShader, or an image op with a shader.
func Shades(d *driver.Drawing, op *driver.DrawOp) bool {
	s := driver.PaintShader(op.Shader)
	if s == nil || s.Program == nil || s.Program.IR == nil {
		return false
	}
	switch op.Kind {
	case driver.DrawShader:
		return true
	case driver.DrawImage, driver.DrawAtlas, driver.DrawVertices:
		return d.Image(op) != nil
	}
	return false
}

// maxSide is the longest side of a pass's texture: what every backend's
// GPU takes.
const maxSide = 8192

// Plan lays out op under m, within clip, the target's pixels the canvas
// may change; img is the size of an image op's image. It reports false
// when op shades nothing there.
func Plan(d *driver.Drawing, op *driver.DrawOp, m Affine, clip image.Rectangle, img Image) (Pass, bool) {
	if !Shades(d, op) {
		return Pass{}, false
	}
	p := Pass{Shader: driver.PaintShader(op.Shader)}
	n := 6
	switch {
	case op.Kind == driver.DrawAtlas:
		n = 6 * len(d.AtlasSprites(op))
	case op.Kind == driver.DrawVertices && op.Mesh != nil:
		n = len(op.Mesh.Vertices)
	}
	pts := make([]Vertex, 0, n) // X, Y in canvas points until mapped
	quad := func(x0, y0, x1, y1 float64, at func(u, v float64) (x, y float64), dst, pos func(u, v float64) (float64, float64), c [4]float32) {
		corners := [6][2]float64{{0, 0}, {1, 0}, {0, 1}, {1, 0}, {1, 1}, {0, 1}}
		for _, k := range corners {
			u, v := x0+(x1-x0)*k[0], y0+(y1-y0)*k[1]
			x, y := at(u, v)
			dx, dy := dst(u, v)
			px, py := pos(u, v)
			pts = append(pts, Vertex{X: float32(x), Y: float32(y), DstX: float32(dx), DstY: float32(dy),
				PosX: float32(px), PosY: float32(py), R: c[0], G: c[1], B: c[2], A: c[3]})
		}
	}
	white := [4]float32{1, 1, 1, 1}
	f := func(v float32) float64 { return float64(v) }
	if op.Kind == driver.DrawShader {
		p.Images = p.Shader.Images
		p.Size, p.Scale = [2]float64{f(op.W), f(op.H)}, m.scale()
		x, y := f(op.X), f(op.Y)
		at := func(u, v float64) (float64, float64) { return x + u, y + v }
		pos := func(u, v float64) (float64, float64) { return u, v }
		dst := func(u, v float64) (float64, float64) { return u * p.Scale, v * p.Scale }
		quad(0, 0, f(op.W), f(op.H), at, dst, pos, white)
	} else {
		src := d.Image(op)
		if src == nil || img.W <= 0 || img.H <= 0 || img.PixelsW <= 0 || img.PixelsH <= 0 {
			return Pass{}, false
		}
		p.Images = append([]driver.ImageSource{*src}, p.Shader.Images...)
		p.Size, p.Scale = [2]float64{img.W, img.H}, img.PixelsW/img.W
		kx, ky := img.PixelsW/img.W, img.PixelsH/img.H
		pos := func(u, v float64) (float64, float64) { return u, v }
		dst := func(u, v float64) (float64, float64) { return u * kx, v * ky }
		switch op.Kind {
		case driver.DrawImage:
			sx, sy, sw, sh := f(op.X1), f(op.Y1), f(op.X2), f(op.Y2)
			if sw == 0 && sh == 0 {
				sx, sy, sw, sh = 0, 0, img.W, img.H
			}
			w, h := f(op.W), f(op.H)
			if w == 0 && h == 0 {
				w, h = img.W, img.H
			}
			if sw == 0 || sh == 0 {
				return Pass{}, false
			}
			x, y := f(op.X), f(op.Y)
			at := func(u, v float64) (float64, float64) { return x + (u-sx)/sw*w, y + (v-sy)/sh*h }
			c := white
			if op.Tinted {
				c = premultiplied(op.Color)
			}
			quad(sx, sy, sx+sw, sy+sh, at, dst, pos, c)
		case driver.DrawAtlas:
			sprites := d.AtlasSprites(op)
			if len(sprites) == 0 {
				return Pass{}, false
			}
			for _, s := range sprites {
				r, t := s.Rect, s.Transform
				ox, oy := float64(r.Origin.X), float64(r.Origin.Y)
				cs, sn, tx, ty := f(t.SCos), f(t.SSin), f(t.TX), f(t.TY)
				at := func(u, v float64) (float64, float64) {
					u, v = u-ox, v-oy
					return cs*u - sn*v + tx, sn*u + cs*v + ty
				}
				a := min(max(s.Alpha, 0), 1)
				quad(ox, oy, ox+float64(r.Size.Width), oy+float64(r.Size.Height), at, dst, pos, [4]float32{a, a, a, a})
			}
		case driver.DrawVertices:
			if op.Mesh == nil {
				return Pass{}, false
			}
			for _, v := range op.Mesh.Vertices {
				u, w := float64(v.TexCoord.X), float64(v.TexCoord.Y)
				dx, dy := dst(u, w)
				c := white
				if v.Color != 0 {
					c = premultiplied(v.Color)
				}
				pts = append(pts, Vertex{X: float32(v.Position.X), Y: float32(v.Position.Y), DstX: float32(dx), DstY: float32(dy),
					PosX: float32(u), PosY: float32(w), R: c[0], G: c[1], B: c[2], A: c[3]})
			}
		}
	}
	if len(pts) < 3 {
		return Pass{}, false
	}
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for i := range pts {
		x, y := m.Apply(float64(pts[i].X), float64(pts[i].Y))
		pts[i].X, pts[i].Y = float32(x), float32(y)
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	if math.IsNaN(x0+y0+x1+y1) || math.IsInf(x0+y0+x1+y1, 0) {
		return Pass{}, false
	}
	b := image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1))).Intersect(clip)
	if b.Dx() > maxSide {
		b.Max.X = b.Min.X + maxSide
	}
	if b.Dy() > maxSide {
		b.Max.Y = b.Min.Y + maxSide
	}
	if b.Empty() {
		return Pass{}, false
	}
	for i := range pts {
		pts[i].X -= float32(b.Min.X)
		pts[i].Y -= float32(b.Min.Y)
	}
	p.Bounds, p.Vertices = b, pts
	return p, true
}

// premultiplied returns c's components from 0 to 1, premultiplied.
func premultiplied(c driver.Color) [4]float32 {
	r, g, b, a := c.Components()
	return [4]float32{float32(r * a), float32(g * a), float32(b * a), float32(a)}
}

// Pack appends the pass's vertices to buf as Floats floats each: the
// position in clip space, y up as Metal, Direct3D and OpenGL take it, or
// down when flipY is set, for a texture OpenGL reads bottom row first;
// then dst, pos and the color.
func (p *Pass) Pack(buf []float32, flipY bool) []float32 {
	buf = slices.Grow(buf, len(p.Vertices)*Floats)
	w, h := float32(p.Bounds.Dx()), float32(p.Bounds.Dy())
	for _, v := range p.Vertices {
		y := 1 - v.Y/h*2
		if flipY {
			y = -y
		}
		buf = append(buf, v.X/w*2-1, y, v.DstX, v.DstY, v.PosX, v.PosY, v.R, v.G, v.B, v.A)
	}
	return buf
}

// Lanes returns the lanes of the program's uniforms: the bridge's, which
// are rectSize, pixelScale and the size in pixels of each of the textures
// images the program reads (size), then the shader's own.
func (p *Pass) Lanes(textures int, size func(i int) (w, h float64)) []float64 {
	lanes := make([]float64, 3+2*textures, 3+2*textures+len(p.Shader.Lanes))
	lanes[0], lanes[1], lanes[2] = p.Size[0], p.Size[1], p.Scale
	for i := range textures {
		lanes[3+2*i], lanes[4+2*i] = size(i)
	}
	return append(lanes, p.Shader.Lanes...)
}
