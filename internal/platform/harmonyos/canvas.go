//go:build js && wasm

package harmonyos

import (
	"encoding/base64"
	"encoding/json"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

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
	// Nearest is whether the image is drawn with smoothing off, each source pixel a hard-edged block (driver.ImageSamplingNearest).
	Nearest bool `json:"n,omitempty"`
	// Sprites packs an atlas's sprites, nine numbers each: the source rectangle's x, y, width and height in the image's points, the transform's SCos, SSin, TX and TY, and the opacity factor.
	Sprites []float32 `json:"sp,omitempty"`
}

// image names the op's image and whether it is sampled nearest.
func (c *canvasOp) image(o *driver.DrawOp) {
	c.Img = o.Image.URI
	if len(o.Image.Data) > 0 {
		c.ImgKey = o.Image.Key()
		c.ImgData = base64.StdEncoding.EncodeToString(o.Image.Data)
	}
	c.Nearest = o.Sampling == driver.ImageSamplingNearest
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

// canvasJSON serializes a drawing for the host.
func canvasJSON(ops []driver.DrawOp) string {
	out := make([]canvasOp, 0, len(ops))
	for i := range ops {
		o := &ops[i]
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
		case driver.DrawImage:
			if o.Image == nil {
				continue
			}
			c.image(o)
		case driver.DrawAtlas:
			if o.Image == nil || o.Atlas == nil || len(*o.Atlas) == 0 {
				continue
			}
			c.image(o)
			c.Sprites = atlasSprites(*o.Atlas)
		}
		out = append(out, c)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
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
