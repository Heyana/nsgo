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

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Texture is a read-only premultiplied image, with four bytes per pixel.
type Texture struct {
	Pix                   []byte
	Width, Height, Stride int
	Points                driver.Size
	BGRA                  bool
	// nearest samples the closest texel rather than blending four, as the op's ImageSamplingNearest asks.
	nearest bool
}

// RGBA uses the pixels of img without copying them.
func RGBA(img *image.RGBA, size driver.Size) Texture {
	if size.Width <= 0 || size.Height <= 0 {
		size = driver.Size{Width: driver.Points(img.Rect.Dx()), Height: driver.Points(img.Rect.Dy())}
	}
	return Texture{Pix: img.Pix, Width: img.Rect.Dx(), Height: img.Rect.Dy(), Stride: img.Stride, Points: size}
}

// Raster draws op's triangles in its bounds at scale pixels per point.
// Shared edges pass the same half-open coverage rule as GPU triangles,
// so no pixel along the join is drawn twice or left empty.
func Raster(op *driver.DrawOp, texture Texture, scale float64) *image.RGBA {
	w, h := float64(op.W), float64(op.H)
	if op.Mesh == nil || w <= 0 || h <= 0 || math.IsInf(w, 0) || math.IsInf(h, 0) ||
		texture.Width <= 0 || texture.Height <= 0 || texture.Points.Width <= 0 || texture.Points.Height <= 0 {
		return nil
	}
	const maxSide, maxPixels = 4096, 4 << 20
	scale = min(scale, maxSide/max(w, h), math.Sqrt(maxPixels/(w*h)))
	if scale <= 0 || math.IsNaN(scale) {
		return nil
	}
	texture.nearest = op.Sampling == driver.ImageSamplingNearest
	pw, ph := int(math.Ceil(w*scale)), int(math.Ceil(h*scale))
	out := image.NewRGBA(image.Rect(0, 0, pw, ph))
	sx, sy := float64(pw)/w, float64(ph)/h
	vertices := op.Mesh.Vertices
	triangles := make([][3]vertex, 0, len(vertices)/3)
	for i := 0; i+2 < len(vertices); i += 3 {
		var tri [3]vertex
		for j, v := range vertices[i : i+3] {
			r, g, b, a := v.Color.Components()
			if v.Color == 0 {
				r, g, b, a = 1, 1, 1, 1
			}
			tri[j] = vertex{
				x:     (float64(v.Position.X) - float64(op.X)) * sx,
				y:     (float64(v.Position.Y) - float64(op.Y)) * sy,
				u:     float64(v.TexCoord.X)*float64(texture.Width)/float64(texture.Points.Width) - .5,
				v:     float64(v.TexCoord.Y)*float64(texture.Height)/float64(texture.Points.Height) - .5,
				color: [4]float64{r * a, g * a, b * a, a},
			}
		}
		triangles = append(triangles, tri)
	}
	// Rows are independent, including when triangles overlap. Larger
	// meshes share the work while each band preserves triangle order.
	bands := min(4, runtime.GOMAXPROCS(0), max(1, pw*ph/(128<<10)))
	if bands == 1 {
		drawBand(out, texture, triangles, 0, ph)
		return out
	}
	var wg sync.WaitGroup
	for band := 1; band < bands; band++ {
		wg.Go(func() { drawBand(out, texture, triangles, ph*band/bands, ph*(band+1)/bands) })
	}
	drawBand(out, texture, triangles, 0, ph/bands)
	wg.Wait()
	return out
}

func drawBand(out *image.RGBA, texture Texture, triangles [][3]vertex, ymin, ymax int) {
	for _, tri := range triangles {
		drawTriangle(out, texture, tri, ymin, ymax)
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

func drawTriangle(out *image.RGBA, texture Texture, tri [3]vertex, ymin, ymax int) {
	a, b, c := tri[0], tri[1], tri[2]
	y0 := max(int(math.Floor(min(a.y, b.y, c.y))), ymin)
	y1 := min(int(math.Ceil(max(a.y, b.y, c.y))), ymax)
	if y0 >= y1 {
		return
	}
	area := edgeOf(a, b).at(c.x, c.y)
	if area == 0 {
		return
	}
	if area < 0 {
		b, c = c, b
		area = -area
	}
	e0, e1, e2 := edgeOf(b, c), edgeOf(c, a), edgeOf(a, b)
	x0 := max(int(math.Floor(min(a.x, b.x, c.x))), 0)
	x1 := min(int(math.Ceil(max(a.x, b.x, c.x))), out.Rect.Max.X)
	white := [4]float64{1, 1, 1, 1}
	tinted := a.color != white || b.color != white || c.color != white
	invArea := 1 / area
	alongX := func(a, b, c float64) float64 {
		return (e0.stepX*a + e1.stepX*b + e2.stepX*c) * invArea
	}
	du, dv := alongX(a.u, b.u, c.u), alongX(a.v, b.v, c.v)
	var dc [4]float64
	if tinted {
		for i := range dc {
			dc[i] = alongX(a.color[i], b.color[i], c.color[i])
		}
	}
	for y := y0; y < y1; y++ {
		yf := float64(y) + .5
		start, end := e0.clipRow(yf, x0, x1)
		start, end = e1.clipRow(yf, start, end)
		start, end = e2.clipRow(yf, start, end)
		if start == end {
			continue
		}
		xf := float64(start) + .5
		w0, w1, w2 := e0.at(xf, yf), e1.at(xf, yf), e2.at(xf, yf)
		p, q, r := w0*invArea, w1*invArea, w2*invArea
		u, v := p*a.u+q*b.u+r*c.u, p*a.v+q*b.v+r*c.v
		var tint [4]float64
		if tinted {
			for k := range tint {
				tint[k] = p*a.color[k] + q*b.color[k] + r*c.color[k]
			}
		}
		row := out.Pix[y*out.Stride:]
		for x := start; x < end; x++ {
			px := texture.sample(u, v)
			if tinted {
				px = tintPixel(px, tint)
			}
			blendPixel(row[x*4:x*4+4], px)
			u, v = u+du, v+dv
			if tinted {
				for k := range tint {
					tint[k] += dc[k]
				}
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

func (t Texture) sample(u, v float64) uint32 {
	u = min(max(u, 0), float64(t.Width-1))
	v = min(max(v, 0), float64(t.Height-1))
	x, y := int(u), int(v)
	x1, y1 := min(x+1, t.Width-1), min(y+1, t.Height-1)
	at := func(x, y int) uint32 {
		i := y*t.Stride + x*4
		return binary.LittleEndian.Uint32(t.Pix[i:])
	}
	var color uint32
	if t.nearest {
		// u and v are at texel centers, so the closest texel is the rounded one.
		color = at(int(u+.5), int(v+.5))
	} else {
		a, b, c, d := at(x, y), at(x1, y), at(x, y1), at(x1, y1)
		fx, fy := uint32((u-float64(x))*256), uint32((v-float64(y))*256)
		color = mixPixels(mixPixels(a, b, fx), mixPixels(c, d, fx), fy)
	}
	if t.BGRA {
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
