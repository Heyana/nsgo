// Package canvasmesh plays triangle meshes for canvas contexts without
// textured vertices. The native player composites the result as one image.
package canvasmesh

import (
	"encoding/binary"
	"image"
	"math"
	"math/bits"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Texture is a read-only premultiplied image, with four bytes per pixel.
type Texture struct {
	Pix                   []byte
	Width, Height, Stride int
	Points                driver.Size
	BGRA                  bool
}

// Target is the zeroed premultiplied pixels Draw draws into, with four bytes per pixel in BGRA order when BGRA is set and RGBA order otherwise. Its top left corner is canvas point X, Y, and it has ScaleX and ScaleY pixels per point.
type Target struct {
	Pix                   []byte
	Width, Height, Stride int
	X, Y, ScaleX, ScaleY  float64
	BGRA                  bool
}

const maxSide, maxPixels = 4096, 4 << 20

// RGBA uses the pixels of img without copying them.
func RGBA(img *image.RGBA, size driver.Size) Texture {
	if size.Width <= 0 || size.Height <= 0 {
		size = driver.Size{Width: driver.Points(img.Rect.Dx()), Height: driver.Points(img.Rect.Dy())}
	}
	return Texture{Pix: img.Pix, Width: img.Rect.Dx(), Height: img.Rect.Dy(), Stride: img.Stride, Points: size}
}

// Bounds returns a Target without pixels that spans op's bounds at scale pixels per point, at most 4096 pixels a side and 4M in all. Its Width is zero when op draws nothing.
func Bounds(op *driver.DrawOp, texture Texture, scale float64) Target {
	w, h := float64(op.W), float64(op.H)
	if op.Mesh == nil || w <= 0 || h <= 0 || math.IsInf(w, 0) || math.IsInf(h, 0) ||
		texture.Width <= 0 || texture.Height <= 0 || texture.Points.Width <= 0 || texture.Points.Height <= 0 {
		return Target{}
	}
	scale = min(scale, maxSide/max(w, h), math.Sqrt(maxPixels/(w*h)))
	if scale <= 0 || math.IsNaN(scale) {
		return Target{}
	}
	pw, ph := int(math.Ceil(w*scale)), int(math.Ceil(h*scale))
	return Target{Width: pw, Height: ph, X: float64(op.X), Y: float64(op.Y), ScaleX: float64(pw) / w, ScaleY: float64(ph) / h}
}

// Device returns a Target without pixels whose pixels are the device's, for a context that maps canvas point x, y to device pixel x*sx+tx, y*sy+ty, so compositing it copies pixels rather than resampling them. When that takes more pixels than Bounds allows, it returns Bounds at the larger scale and false.
func Device(op *driver.DrawOp, texture Texture, sx, sy, tx, ty float64) (Target, bool) {
	bounds := Bounds(op, texture, max(math.Abs(sx), math.Abs(sy)))
	if bounds.Width == 0 || sx == 0 || sy == 0 {
		return bounds, false
	}
	// The device pixels the bounds touch, and the canvas point at their top left.
	span := func(lo, size, s, t float64) (float64, int) {
		a, b := lo*s+t, (lo+size)*s+t
		first, last := math.Floor(min(a, b)), math.Ceil(max(a, b))
		corner := first
		if s < 0 {
			corner = last
		}
		return (corner - t) / s, int(last - first)
	}
	x, w := span(float64(op.X), float64(op.W), sx, tx)
	y, h := span(float64(op.Y), float64(op.H), sy, ty)
	if w <= 0 || h <= 0 || w > maxSide || h > maxSide || w*h > maxPixels {
		return bounds, false
	}
	return Target{Width: w, Height: h, X: x, Y: y, ScaleX: math.Abs(sx), ScaleY: math.Abs(sy)}, true
}

// Raster draws op's triangles in its bounds at scale pixels per point.
// Shared edges pass the same half-open coverage rule as GPU triangles,
// so no pixel along the join is drawn twice or left empty.
func Raster(op *driver.DrawOp, texture Texture, scale float64) *image.RGBA {
	dst := Bounds(op, texture, scale)
	if dst.Width == 0 {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, dst.Width, dst.Height))
	dst.Pix, dst.Stride = out.Pix, out.Stride
	Draw(dst, op, texture)
	return out
}

// Draw draws op's triangles into dst, which spans op's bounds, as Raster does.
func Draw(dst Target, op *driver.DrawOp, texture Texture) {
	if op.Mesh == nil || dst.Width <= 0 || dst.Height <= 0 || !(dst.ScaleX > 0) || !(dst.ScaleY > 0) ||
		texture.Width <= 0 || texture.Height <= 0 || texture.Points.Width <= 0 || texture.Points.Height <= 0 {
		return
	}
	pw, ph := dst.Width, dst.Height
	s := sampler{Texture: texture, nearest: op.Sampling == driver.ImageSamplingNearest, swap: texture.BGRA != dst.BGRA}
	sx, sy := dst.ScaleX, dst.ScaleY
	vertices := op.Mesh.Vertices
	triangles := make([]triangle, 0, len(vertices)/3)
	for i := 0; i+2 < len(vertices); i += 3 {
		var tri [3]vertex
		for j, v := range vertices[i : i+3] {
			r, g, b, a := v.Color.Components()
			if v.Color == 0 {
				r, g, b, a = 1, 1, 1, 1
			}
			if dst.BGRA {
				r, b = b, r
			}
			tri[j] = vertex{
				x:     (float64(v.Position.X) - dst.X) * sx,
				y:     (float64(v.Position.Y) - dst.Y) * sy,
				u:     float64(v.TexCoord.X)*float64(texture.Width)/float64(texture.Points.Width) - .5,
				v:     float64(v.TexCoord.Y)*float64(texture.Height)/float64(texture.Points.Height) - .5,
				color: [4]float64{r * a, g * a, b * a, a},
			}
		}
		if t, ok := setup(tri, pw, ph); ok {
			triangles = append(triangles, t)
		}
	}
	// Rows are independent, including when triangles overlap, so workers
	// take bands of rows in turn and each band keeps triangle order.
	// Small bands let fast and slow cores finish together.
	const band = 32
	bands := (ph + band - 1) / band
	workers := min(runtime.GOMAXPROCS(0), bands, max(1, pw*ph/(64<<10)))
	if workers == 1 {
		drawRows(dst, &s, triangles, 0, ph)
		return
	}
	var next atomic.Int32
	work := func() {
		for {
			i := int(next.Add(1)) - 1
			if i >= bands {
				return
			}
			drawRows(dst, &s, triangles, i*band, min(ph, (i+1)*band))
		}
	}
	var wg sync.WaitGroup
	for range workers - 1 {
		wg.Go(work)
	}
	work()
	wg.Wait()
}

func drawRows(dst Target, s *sampler, triangles []triangle, ymin, ymax int) {
	for i := range triangles {
		if t := &triangles[i]; t.y0 < ymax && t.y1 > ymin {
			t.draw(dst, s, max(t.y0, ymin), min(t.y1, ymax))
		}
	}
}

type vertex struct {
	x, y, u, v float64
	color      [4]float64
}

type edge struct {
	x, y, stepX, stepY float64
	includeTie         bool
}

func edgeOf(a, b vertex) edge {
	dx, dy := b.x-a.x, b.y-a.y
	return edge{a.x, a.y, -dy, dx, dy < 0 || dy == 0 && dx > 0}
}

func (e edge) at(x, y float64) float64 {
	return e.stepX*(x-e.x) + e.stepY*(y-e.y)
}

func (e edge) covers(value float64) bool {
	return value > 0 || value == 0 && e.includeTie
}

// clipRow intersects [start, end) with this edge at pixel centers.
func (e edge) clipRow(y float64, start, end int) (int, int) {
	if e.stepX == 0 {
		if !e.covers(e.at(0, y)) {
			return start, start
		}
		return start, end
	}
	boundary := e.x - .5 - e.stepY*(y-e.y)/e.stepX
	// Clamp before converting to int: a nearly horizontal edge can
	// intersect the row far beyond the allocation's bounds.
	x := int(math.Floor(min(max(boundary, float64(start-1)), float64(end))))
	if e.stepX > 0 {
		start = max(start, x)
		for start < end && !e.covers(e.at(float64(start)+.5, y)) {
			start++
		}
		return start, end
	}
	end = min(end, x+2)
	for start < end && !e.covers(e.at(float64(end)-.5, y)) {
		end--
	}
	return start, end
}

// triangle is a triangle ready to draw: wound clockwise in pixels, with
// the rows and columns it may cover and its attributes' steps along x.
type triangle struct {
	a, b, c    vertex
	e0, e1, e2 edge
	invArea    float64
	y0, y1     int
	x0, x1     int
	du, dv     float64
	dc         [4]float64
	tinted     bool
}

func setup(tri [3]vertex, pw, ph int) (triangle, bool) {
	a, b, c := tri[0], tri[1], tri[2]
	y0 := max(int(math.Floor(min(a.y, b.y, c.y))), 0)
	y1 := min(int(math.Ceil(max(a.y, b.y, c.y))), ph)
	if y0 >= y1 {
		return triangle{}, false
	}
	area := edgeOf(a, b).at(c.x, c.y)
	if area == 0 || math.IsNaN(area) {
		return triangle{}, false
	}
	if area < 0 {
		b, c = c, b
		area = -area
	}
	t := triangle{a: a, b: b, c: c, e0: edgeOf(b, c), e1: edgeOf(c, a), e2: edgeOf(a, b), invArea: 1 / area, y0: y0, y1: y1,
		x0: max(int(math.Floor(min(a.x, b.x, c.x))), 0), x1: min(int(math.Ceil(max(a.x, b.x, c.x))), pw)}
	alongX := func(a, b, c float64) float64 {
		return (t.e0.stepX*a + t.e1.stepX*b + t.e2.stepX*c) * t.invArea
	}
	t.du, t.dv = alongX(a.u, b.u, c.u), alongX(a.v, b.v, c.v)
	white := [4]float64{1, 1, 1, 1}
	t.tinted = a.color != white || b.color != white || c.color != white
	if t.tinted {
		for i := range t.dc {
			t.dc[i] = alongX(a.color[i], b.color[i], c.color[i])
		}
	}
	return t, true
}

// fixed is the scale of texel coordinates stepped as integers along a row:
// 32 fractional bits keep a row of 4096 steps within a millionth of a texel.
const fixed = 1 << 32

func (t *triangle) draw(dst Target, s *sampler, y0, y1 int) {
	a, b, c := t.a, t.b, t.c
	maxU, maxV := int64(s.Width-1)*fixed, int64(s.Height-1)*fixed
	du, dv := int64(math.Round(t.du*fixed)), int64(math.Round(t.dv*fixed))
	for y := y0; y < y1; y++ {
		yf := float64(y) + .5
		start, end := t.e0.clipRow(yf, t.x0, t.x1)
		start, end = t.e1.clipRow(yf, start, end)
		start, end = t.e2.clipRow(yf, start, end)
		if start == end {
			continue
		}
		xf := float64(start) + .5
		p, q, r := t.e0.at(xf, yf)*t.invArea, t.e1.at(xf, yf)*t.invArea, t.e2.at(xf, yf)*t.invArea
		u := int64(math.Round((p*a.u + q*b.u + r*c.u) * fixed))
		v := int64(math.Round((p*a.v + q*b.v + r*c.v) * fixed))
		row := dst.Pix[y*dst.Stride+start*4 : y*dst.Stride+end*4]
		if !t.tinted {
			for i := 0; i < len(row); i += 4 {
				blendPixel(row[i:i+4:i+4], s.sample(min(max(u, 0), maxU), min(max(v, 0), maxV)))
				u, v = u+du, v+dv
			}
			continue
		}
		var tint [4]float64
		for k := range tint {
			tint[k] = p*a.color[k] + q*b.color[k] + r*c.color[k]
		}
		for i := 0; i < len(row); i += 4 {
			px := tintPixel(s.sample(min(max(u, 0), maxU), min(max(v, 0), maxV)), tint)
			blendPixel(row[i:i+4:i+4], px)
			u, v = u+du, v+dv
			for k := range tint {
				tint[k] += t.dc[k]
			}
		}
	}
}

func tintPixel(pixel uint32, tint [4]float64) uint32 {
	var out uint32
	for k := range tint {
		channel := pixel >> (8 * k) & 255
		tinted := uint32(float64(channel)*min(max(tint[k], 0), 1) + .5)
		out |= tinted << (8 * k)
	}
	return out
}

func blendPixel(dst []byte, pixel uint32) {
	alpha := pixel >> 24
	if alpha == 255 || dst[3] == 0 {
		binary.LittleEndian.PutUint32(dst, pixel)
	} else if alpha > 0 {
		remain := 255 - alpha
		for k := range 4 {
			dst[k] = byte(pixel>>(8*k)) + byte((uint32(dst[k])*remain+127)/255)
		}
	}
}

// sampler reads a texture for one Draw: nearest samples the closest texel
// rather than blending four, as the op's ImageSamplingNearest asks, and
// swap exchanges red and blue for a target in the other byte order.
type sampler struct {
	Texture
	nearest, swap bool
}

// sample reads the texture at u, v, texel coordinates scaled by fixed and
// clamped to its pixels.
func (s *sampler) sample(u, v int64) uint32 {
	var color uint32
	if s.nearest {
		// u and v are at texel centers, so the closest texel is the rounded one.
		i := int((v+fixed/2)>>32)*s.Stride + int((u+fixed/2)>>32)*4
		color = binary.LittleEndian.Uint32(s.Pix[i : i+4])
	} else {
		x, y := int(u>>32), int(v>>32)
		fx, fy := uint32(u>>24)&255, uint32(v>>24)&255
		i := y*s.Stride + x*4
		right, down := 4, s.Stride
		if x == s.Width-1 {
			right = 0
		}
		if y == s.Height-1 {
			down = 0
		}
		top, bottom := s.Pix[i:i+right+4], s.Pix[i+down:i+down+right+4]
		color = mixPixels(
			mixPixels(binary.LittleEndian.Uint32(top), binary.LittleEndian.Uint32(top[right:]), fx),
			mixPixels(binary.LittleEndian.Uint32(bottom), binary.LittleEndian.Uint32(bottom[right:]), fx), fy)
	}
	if s.swap {
		color = color&0xff00ff00 | bits.RotateLeft32(color&0x000000ff, 16) | color>>16&0xff
	}
	return color
}

// mixPixels interpolates two premultiplied RGBA values. Two color
// channels fit side by side in 16-bit lanes while calculating each mix.
func mixPixels(a, b, f uint32) uint32 {
	const lane = 0x00ff00ff
	lo := ((a&lane)*(256-f) + (b&lane)*f + 0x00800080) >> 8 & lane
	hi := (((a>>8)&lane)*(256-f) + ((b>>8)&lane)*f + 0x00800080) >> 8 & lane
	return lo | hi<<8
}
