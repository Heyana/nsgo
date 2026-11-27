//go:build js && wasm

package harmonyos

import (
	"encoding/base64"
	"encoding/json"
	"image"
	"strconv"
	"sync/atomic"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

var canvasEncoder atomic.Pointer[func(*driver.Drawing, []driver.DrawOp, bool) string]

func (*surface) EnableCanvas() {
	if canvasEncoder.Load() != nil {
		return
	}
	fn := canvasJSON
	canvasEncoder.Store(&fn)
}

func encodeCanvas(d *driver.Drawing, ops []driver.DrawOp, packed bool) string {
	if fn := canvasEncoder.Load(); fn != nil {
		return (*fn)(d, ops, packed)
	}
	return "[]"
}

// canvasOp is one drawing operation as the host replays it into a
// CanvasRenderingContext2D. Short keys and omitted zero fields keep a
// drawing small.
type canvasOp struct {
	K    int       `json:"k"`
	X    float32   `json:"x,omitempty"`
	Y    float32   `json:"y,omitempty"`
	X1   *float32  `json:"x1,omitempty"`
	Y1   float32   `json:"y1,omitempty"`
	X2   float32   `json:"x2,omitempty"`
	Y2   *float32  `json:"y2,omitempty"`
	W    float32   `json:"w,omitempty"`
	H    float32   `json:"h,omitempty"`
	R    float32   `json:"r,omitempty"`
	A    *float32  `json:"a,omitempty"`
	A0   float32   `json:"a0,omitempty"`
	A1   float32   `json:"a1,omitempty"`
	RY   float32   `json:"ry,omitempty"`
	CCW  bool      `json:"ccw,omitempty"`
	C    string    `json:"c,omitempty"`
	Cap  int       `json:"cap,omitempty"`
	Join int       `json:"join,omitempty"`
	EO   bool      `json:"eo,omitempty"`
	T    string    `json:"t,omitempty"`
	FF   string    `json:"ff,omitempty"`
	FS   float32   `json:"fs,omitempty"`
	FW   int       `json:"fw,omitempty"`
	FI   bool      `json:"fi,omitempty"`
	G    *gradient `json:"g,omitempty"`
	CO   int       `json:"co,omitempty"`
	D    []float32 `json:"d,omitempty"`
	Img  string    `json:"img,omitempty"`
	// ImgKey is ImageSource.Key when the drawing embeds bytes, so KeepImage pins match inline byte sources as well as URIs.
	ImgKey string `json:"imgk,omitempty"`
	// ImgData is the image's bytes, base64, when the drawing embeds them.
	ImgData string `json:"imgd,omitempty"`
	// Tint is the color a tinted DrawImage multiplies its source channels by, as driver.Color.
	Tint *uint32 `json:"tint,omitempty"`
	// Nearest is whether the image is drawn with smoothing off, each source pixel a hard-edged block (driver.ImageSamplingNearest).
	Nearest bool `json:"n,omitempty"`
	// Sprites packs an atlas's sprites, nine numbers each: the source rectangle's x, y, width and height in the image's points, the transform's SCos, SSin, TX and TY, and the opacity factor.
	Sprites []float32 `json:"sp,omitempty"`
	// V is a mesh's vertices, x, y, u, v each, in canvas and image points; VA its triangles' mean alpha, when a vertex color is not opaque white; and E each triangle's edges shared with another opaque triangle, as meshSeams makes them. A mesh samples nearest by Nearest.
	V  []float32 `json:"v,omitempty"`
	VA []float32 `json:"va,omitempty"`
	E  []int     `json:"e,omitempty"`
	// Pixels is a pixel buffer's premultiplied RGBA, base64, PW by PH pixels, sent with the first op of a drawing that draws its copy; ImgKey names the copy, so later ops and drawings find it in the host's canvas image cache.
	Pixels string `json:"pxd,omitempty"`
	PW     int    `json:"pw,omitempty"`
	PH     int    `json:"ph,omitempty"`
}

// image names the op's image and whether it is sampled nearest. sent holds the pixel copies the drawing's earlier ops carried, which this op only names.
func (c *canvasOp) image(src *driver.ImageSource, sampling driver.ImageSampling, sent map[*image.RGBA]bool) {
	c.Img = src.URI
	if px := src.Pixels; px != nil {
		c.ImgKey = canvasPixelKeys.key(px)
		if !sent[px] {
			sent[px] = true
			c.Pixels = base64.StdEncoding.EncodeToString(px.Pix)
			c.PW, c.PH = px.Rect.Dx(), px.Rect.Dy()
		}
	} else if len(src.Data) > 0 {
		c.ImgKey = src.Key()
		c.ImgData = base64.StdEncoding.EncodeToString(src.Data)
	}
	c.Nearest = sampling == driver.ImageSamplingNearest
}

// canvasPixelKeys names the pixel copies canvases draw. Only the engine goroutine, which mounts, uses it.
var canvasPixelKeys pixelKeys

// pixelKeys names pixel copies for the host's canvas image cache, the same copy by the same key in every drawing. The core keeps a copy for as long as its version is drawn (driver.CopyPixels), so a copy is its version. It remembers the last 16 copies it named, which keeps their memory from being reused by another copy; one that drops out is named anew if it is drawn again, and the host then decodes it again, which costs time but never draws another copy.
type pixelKeys struct {
	named []namedPixels
	next  uint64
}

type namedPixels struct {
	pixels *image.RGBA
	key    string
}

func (k *pixelKeys) key(px *image.RGBA) string {
	for i, n := range k.named {
		if n.pixels == px {
			copy(k.named[i:], k.named[i+1:])
			k.named[len(k.named)-1] = n
			return n.key
		}
	}
	k.next++
	n := namedPixels{px, "px:" + strconv.FormatUint(k.next, 10)}
	if len(k.named) == 16 {
		k.named = append(k.named[:0], k.named[1:]...)
	}
	k.named = append(k.named, n)
	return n.key
}

// atlasSprites packs an atlas for the host's player, which draws every sprite from one decoded image in one op, as canvas2d's player does.
func atlasSprites(sprites driver.AtlasSprites) []float32 {
	out := make([]float32, 0, len(sprites)*9)
	for _, s := range sprites {
		b, t := s.Rect, s.Transform
		out = append(out, float32(b.Origin.X), float32(b.Origin.Y), float32(b.Size.Width), float32(b.Size.Height),
			t.SCos, t.SSin, t.TX, t.TY, s.Alpha)
	}
	return out
}

// gradient is a canvas gradient as the host builds one: radial or linear,
// its circles or endpoints, its stops.
type gradient struct {
	Radial bool    `json:"radial,omitempty"`
	X1     float32 `json:"x1,omitempty"`
	Y1     float32 `json:"y1,omitempty"`
	R1     float32 `json:"r1,omitempty"`
	X2     float32 `json:"x2,omitempty"`
	Y2     float32 `json:"y2,omitempty"`
	R2     float32 `json:"r2,omitempty"`
	Stops  []stop  `json:"s,omitempty"`
}

type stop struct {
	O float32 `json:"o"`
	C string  `json:"c"`
}

// canvasJSON serializes a drawing for the host. packed says the host plays an atlas from one op (deviceData.CanvasAtlas); a host built before that op, such as an older committed prebuilt host, gets each sprite expanded into the image ops it has always played.
func canvasJSON(d *driver.Drawing, ops []driver.DrawOp, packed bool) string {
	out := make([]canvasOp, 0, len(ops))
	sent := map[*image.RGBA]bool{}
	alpha := float32(1)
	var saved []float32
	for i := range ops {
		o := &ops[i]
		if o.Kind == driver.DrawShader {
			// The host has no GPU path for canvases: a shaded image draws plain.
			continue
		}
		c := canvasOp{
			K: int(o.Kind), X: o.X, Y: o.Y, Y1: o.Y1, X2: o.X2,
			W: o.W, H: o.H, R: o.R, A0: o.A0, A1: o.A1, RY: o.RY, CCW: o.CCW, EO: o.EvenOdd,
			T: o.Text, FF: o.Font.Family, FS: float32(o.Font.Size), FW: int(o.Font.Weight), FI: o.Font.Italic,
		}
		if o.X1 != 0 || o.Kind == driver.DrawTransform || o.Kind == driver.DrawSetTransform {
			c.X1 = &o.X1
		}
		if o.Y2 != 0 || o.Kind == driver.DrawTransform || o.Kind == driver.DrawSetTransform {
			c.Y2 = &o.Y2
		}
		if o.A != 0 || o.Kind == driver.DrawAlpha {
			c.A = &o.A
		}
		switch o.Kind {
		case driver.DrawSave:
			saved = append(saved, alpha)
		case driver.DrawRestore:
			if n := len(saved); n != 0 {
				alpha, saved = saved[n-1], saved[:n-1]
			}
		case driver.DrawAlpha:
			if o.A >= 0 && o.A <= 1 {
				alpha = o.A
			}
		case driver.DrawFillColor, driver.DrawStrokeColor, driver.DrawShadow:
			c.C = webstyle.Color(o.Color)
		case driver.DrawLineCap:
			c.Cap = int(o.Cap) + 1
		case driver.DrawLineJoin:
			c.Join = int(o.Join) + 1
		case driver.DrawComposite:
			c.CO = int(o.Composite) + 1
		case driver.DrawFillGradient, driver.DrawStrokeGradient:
			c.G = gradientOf(o.Gradient)
		case driver.DrawLineDash:
			if o.Dash != nil {
				c.D = append([]float32(nil), (*o.Dash)...)
			}
		case driver.DrawVertices:
			if draw, ok := o.ImageRect(); ok {
				c = canvasOp{K: int(driver.DrawImage), X: draw.X, Y: draw.Y, W: draw.W, H: draw.H, X1: &draw.X1, Y1: draw.Y1, X2: draw.X2, Y2: &draw.Y2}
			} else if o.Mesh != nil {
				c = canvasOp{K: c.K, V: meshVertices(o.Mesh.Vertices), VA: meshAlphas(o.Mesh.Vertices), E: meshSeams(o.Mesh.Vertices)}
			}
			fallthrough
		case driver.DrawImage:
			if o.Tinted {
				tint := uint32(o.Color)
				c.Tint = &tint
			}
			imgSrc := d.Image(o)
			if imgSrc == nil {
				continue
			}
			c.image(imgSrc, o.Sampling, sent)
		case driver.DrawAtlas:
			imgSrc := d.Image(o)
			sprites := d.AtlasSprites(o)
			if imgSrc == nil || len(sprites) == 0 {
				continue
			}
			if !packed {
				out = appendAtlas(out, imgSrc, o.Sampling, sprites, alpha, sent)
				continue
			}
			c.image(imgSrc, o.Sampling, sent)
			c.Sprites = atlasSprites(sprites)
		}
		out = append(out, c)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// appendAtlas replays an atlas through the image ops every host plays: a save, the sprite's transform, its alpha and the image, then a restore.
func appendAtlas(out []canvasOp, imgSrc *driver.ImageSource, sampling driver.ImageSampling, sprites driver.AtlasSprites, alpha float32, sent map[*image.RGBA]bool) []canvasOp {
	img := canvasOp{K: int(driver.DrawImage)}
	img.image(imgSrc, sampling, sent)
	for _, sprite := range sprites {
		t, b := sprite.Transform, sprite.Rect
		x, h := float32(b.Origin.X), float32(b.Size.Height)
		img.X1, img.Y1 = &x, float32(b.Origin.Y)
		img.W, img.H = float32(b.Size.Width), h
		img.X2, img.Y2 = img.W, &h
		a := alpha * sprite.Alpha
		out = append(out,
			canvasOp{K: int(driver.DrawSave)},
			canvasOp{K: int(driver.DrawTransform), X1: &t.SCos, Y1: t.SSin, X2: -t.SSin, Y2: &t.SCos, X: t.TX, Y: t.TY},
			canvasOp{K: int(driver.DrawAlpha), A: &a},
			img,
			canvasOp{K: int(driver.DrawRestore)},
		)
		// The first sprite carries a pixel copy's bytes; the rest name it.
		img.Pixels, img.PW, img.PH = "", 0, 0
	}
	return out
}

// meshVertices flattens a mesh's positions and texture coordinates.
func meshVertices(vertices []driver.CanvasVertex) []float32 {
	out := make([]float32, 0, len(vertices)*4)
	for _, v := range vertices {
		out = append(out, float32(v.Position.X), float32(v.Position.Y), float32(v.TexCoord.X), float32(v.TexCoord.Y))
	}
	return out
}

// meshAlphas is each triangle's mean vertex alpha, or nil when every vertex is opaque, which the host plays as the triangle's alpha: an ArkUI canvas has no way to multiply an image by interpolated colors.
func meshAlphas(vertices []driver.CanvasVertex) []float32 {
	var out []float32
	for i := 0; i+2 < len(vertices); i += 3 {
		a := triangleAlpha(vertices[i : i+3])
		if out == nil && a < 1 {
			out = make([]float32, i/3, len(vertices)/3)
			for j := range out {
				out[j] = 1
			}
		}
		if out != nil {
			out = append(out, a)
		}
	}
	return out
}

// meshSeams is, for each triangle, bit k set when the edge from its corner k to the next is shared with another triangle, both with opaque vertices; nil when no edge is. The host grows only those edges to hide the seam, so the mesh's outline keeps to its texture coordinates and translucent triangles are never drawn twice over a strip.
func meshSeams(vertices []driver.CanvasVertex) []int {
	type edge struct{ a, b driver.Point }
	edgeOf := func(t, k int) edge {
		p, q := vertices[3*t+k].Position, vertices[3*t+(k+1)%3].Position
		if q.X < p.X || (q.X == p.X && q.Y < p.Y) {
			p, q = q, p
		}
		return edge{p, q}
	}
	n := len(vertices) / 3
	shared := map[edge]int{}
	for t := 0; t < n; t++ {
		if triangleAlpha(vertices[3*t:3*t+3]) < 1 {
			continue
		}
		for k := 0; k < 3; k++ {
			shared[edgeOf(t, k)]++
		}
	}
	var out []int
	for t := 0; t < n; t++ {
		if triangleAlpha(vertices[3*t:3*t+3]) < 1 {
			continue
		}
		mask := 0
		for k := 0; k < 3; k++ {
			if shared[edgeOf(t, k)] > 1 {
				mask |= 1 << k
			}
		}
		if mask != 0 && out == nil {
			out = make([]int, n)
		}
		if out != nil {
			out[t] = mask
		}
	}
	return out
}

// triangleAlpha is the mean alpha of a triangle's vertex colors, the zero color counting as opaque.
func triangleAlpha(corners []driver.CanvasVertex) float32 {
	var sum float64
	for _, v := range corners {
		a := 1.0
		if v.Color != 0 {
			_, _, _, a = v.Color.Components()
		}
		sum += a
	}
	return float32(sum / 3)
}

// gradientOf maps a drawing gradient, leaving out what the host cannot play.
func gradientOf(g *driver.CanvasGradient) *gradient {
	if g == nil || g.Conic {
		return nil
	}
	out := &gradient{Radial: g.Radial, X1: g.X1, Y1: g.Y1, R1: g.R1, X2: g.X2, Y2: g.Y2, R2: g.R2}
	for _, s := range g.Stops {
		out.Stops = append(out.Stops, stop{O: s.Offset, C: webstyle.Color(s.Color)})
	}
	return out
}
