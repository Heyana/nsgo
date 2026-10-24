package main

import (
	"bytes"
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/vector"

	"github.com/oneo-me/nsgo/core"
)

// devBadgeFiles are the badge's layers, written into an .icon's Assets
// with devBadgePrefix before their names.
//
//go:embed devbadge/*.svg
var devBadgeFiles embed.FS

const devBadgePrefix = "nsgo-dev-"

// devBadgeGroup is the Icon Composer group nsgo dev puts over an .icon's
// own groups: a translucent red plate with DEV on it, at the top right.
// Flat icons get the same layers at the same place, without the glass.
const devBadgeGroup = `{
  "name" : "nsgo-dev",
  "blur-material" : 0.2,
  "layers" : [
    {
      "name" : "label",
      "image-name" : "nsgo-dev-label.svg",
      "fill" : { "automatic-gradient" : "srgb:1.00000,1.00000,1.00000,1.00000" },
      "position" : { "scale" : 1, "translation-in-points" : [ 248, -419 ] }
    },
    {
      "name" : "plate",
      "image-name" : "nsgo-dev-plate.svg",
      "fill" : { "solid" : "srgb:1.00000,0.14913,0.00000,0.37945" },
      "position" : { "scale" : 1, "translation-in-points" : [ 248, -419 ] }
    }
  ],
  "position" : { "scale" : 0.74, "translation-in-points" : [ 0, 0 ] },
  "shadow" : { "kind" : "neutral", "opacity" : 0.5 },
  "translucency" : { "enabled" : true, "value" : 0.3 }
}`

// iconCanvas is the side of Icon Composer's canvas in points; the badge's
// positions are from its center.
const iconCanvas = 1024

// badgeApple writes an Apple icon with the dev badge into dir and
// returns its path. A .icns keeps its PNG entries only, the largest of
// them badged and scaled to the others.
func badgeApple(src, dir string) (string, error) {
	out := filepath.Join(dir, filepath.Base(src))
	var err error
	if filepath.Ext(src) == ".icon" {
		err = badgeIconComposer(src, out)
	} else {
		err = badgeICNS(src, out)
	}
	if err != nil {
		return "", fmt.Errorf("dev badge on %s: %w", rel(src), err)
	}
	return out, nil
}

// badgePNG writes a PNG icon with the dev badge into dir and returns its
// path.
func badgePNG(src, dir string) (string, error) {
	img, err := readIcon(src)
	if err != nil {
		return "", err
	}
	badged := image.NewRGBA(img.Bounds())
	draw.Draw(badged, badged.Rect, img, img.Bounds().Min, draw.Src)
	if err := drawDevBadge(badged, badged.Rect); err != nil {
		return "", err
	}
	var data bytes.Buffer
	if err := png.Encode(&data, badged); err != nil {
		return "", err
	}
	out := filepath.Join(dir, filepath.Base(src))
	return out, os.WriteFile(out, data.Bytes(), 0o644)
}

// badgeIconComposer copies an Icon Composer .icon to out with the badge's
// group above its own, so actool renders the badge in the same glass. An
// icon already at actool's four visible groups takes the badge's layers
// into its top group instead.
func badgeIconComposer(src, out string) error {
	if err := os.CopyFS(out, os.DirFS(src)); err != nil {
		return err
	}
	path := filepath.Join(out, "icon.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	var group map[string]any
	if err := json.Unmarshal([]byte(devBadgeGroup), &group); err != nil {
		return err
	}
	groups, _ := doc["groups"].([]any)
	var top map[string]any
	visible := 0
	for _, g := range groups {
		if g, ok := g.(map[string]any); ok && g["hidden"] != true {
			if top == nil {
				top = g
			}
			visible++
		}
	}
	if visible < maxIconGroups {
		doc["groups"] = append([]any{group}, groups...)
	} else if err := mergeBadge(top, group); err != nil {
		return err
	}
	if data, err = json.MarshalIndent(doc, "", "  "); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	assets := filepath.Join(out, "Assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		return err
	}
	names, err := fs.Glob(devBadgeFiles, "devbadge/*.svg")
	if err != nil {
		return err
	}
	for _, name := range names {
		svg, err := devBadgeFiles.ReadFile(name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(assets, devBadgePrefix+filepath.Base(name)), svg, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// maxIconGroups is how many visible groups actool takes in an .icon.
const maxIconGroups = 4

// mergeBadge puts the badge group's layers at the top of group, placed
// where the badge group would have put them.
func mergeBadge(group, badge map[string]any) error {
	pos := func(m map[string]any) (iconPosition, error) {
		p := iconPosition{Scale: 1}
		if v, ok := m["position"]; ok {
			data, err := json.Marshal(v)
			if err != nil {
				return p, err
			}
			if err := json.Unmarshal(data, &p); err != nil {
				return p, err
			}
		}
		return p, nil
	}
	into, err := pos(group)
	if err != nil {
		return err
	}
	from, err := pos(badge)
	if err != nil {
		return err
	}
	if into.Scale == 0 {
		into.Scale = 1
	}
	layers, _ := badge["layers"].([]any)
	for _, l := range layers {
		layer := l.(map[string]any)
		at, err := pos(layer)
		if err != nil {
			return err
		}
		layer["position"] = map[string]any{
			"scale": from.Scale * at.Scale / into.Scale,
			"translation-in-points": []float32{
				(from.Translation[0] + from.Scale*at.Translation[0] - into.Translation[0]) / into.Scale,
				(from.Translation[1] + from.Scale*at.Translation[1] - into.Translation[1]) / into.Scale,
			},
		}
	}
	own, _ := group["layers"].([]any)
	group["layers"] = append(layers, own...)
	return nil
}

// badgeICNS writes the largest PNG entry of an .icns, badged, to out as a
// new .icns. Its art sits in Apple's icon grid: 824 of 1024 pixels with
// the margin around them.
func badgeICNS(src, out string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if len(data) < 8 || string(data[:4]) != "icns" {
		return fmt.Errorf("not an .icns")
	}
	var largest []byte
	size := 0
	for at := 8; at+8 <= len(data); {
		n := int(binary.BigEndian.Uint32(data[at+4:]))
		if n < 8 || at+n > len(data) {
			break
		}
		entry := data[at+8 : at+n]
		if cfg, err := png.DecodeConfig(bytes.NewReader(entry)); err == nil && cfg.Width == cfg.Height && cfg.Width > size {
			largest, size = entry, cfg.Width
		}
		at += n
	}
	if largest == nil {
		return fmt.Errorf("no square PNG entry to badge")
	}
	img, err := png.Decode(bytes.NewReader(largest))
	if err != nil {
		return err
	}
	badged := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(badged, badged.Rect, img, img.Bounds().Min, draw.Src)
	margin := size * 100 / 1024
	if err := drawDevBadge(badged, badged.Rect.Inset(margin)); err != nil {
		return err
	}
	return os.WriteFile(out, icns(badged), 0o644)
}

// drawDevBadge draws devBadgeGroup's layers into dst, with Icon Composer's
// canvas filling area. Each layer's fill replaces its SVG's colors. There
// is no glass to blur the art under a translucent layer, so the layer
// frosts it instead: its fill covers the mean color of its box.
func drawDevBadge(dst *image.RGBA, area image.Rectangle) error {
	var group struct {
		Layers []struct {
			Image string `json:"image-name"`
			Fill  struct {
				Solid    string `json:"solid"`
				Gradient string `json:"automatic-gradient"`
			} `json:"fill"`
			Position iconPosition `json:"position"`
		} `json:"layers"`
		Position iconPosition `json:"position"`
	}
	if err := json.Unmarshal([]byte(devBadgeGroup), &group); err != nil {
		return err
	}
	unit := float32(area.Dx()) / iconCanvas
	// Layers are listed from the top, so draw them from the bottom.
	for i := len(group.Layers) - 1; i >= 0; i-- {
		layer := group.Layers[i]
		data, err := devBadgeFiles.ReadFile("devbadge/" + strings.TrimPrefix(layer.Image, devBadgePrefix))
		if err != nil {
			return err
		}
		svg, err := core.ParseSVG(data)
		if err != nil {
			return err
		}
		paint, err := iconColor(layer.Fill.Solid + layer.Fill.Gradient)
		if err != nil {
			return err
		}
		w, h := svg.Size()
		var d core.Drawing
		svg.Draw(&d, core.Size{Width: w, Height: h}, 0)
		// The layer's center, in points from the canvas's top left.
		gs, ls := group.Position.Scale, layer.Position.Scale
		cx := iconCanvas/2 + group.Position.Translation[0] + gs*layer.Position.Translation[0]
		cy := iconCanvas/2 + group.Position.Translation[1] + gs*layer.Position.Translation[1]
		scale := gs * ls * unit
		origin := [2]float32{
			float32(area.Min.X) + cx*unit - float32(w)*scale/2,
			float32(area.Min.Y) + cy*unit - float32(h)*scale/2,
		}
		var fill color.Color = paint
		if paint.A < 0xff {
			box := image.Rect(int(origin[0]), int(origin[1]),
				int(origin[0]+float32(w)*scale+0.5), int(origin[1]+float32(h)*scale+0.5))
			fill = over(paint, meanColor(dst, box))
		}
		if err := fillDrawing(dst, d.Ops, origin, scale, fill); err != nil {
			return err
		}
	}
	return nil
}

// meanColor averages the pixels of img in r, premultiplied.
func meanColor(img *image.RGBA, r image.Rectangle) color.RGBA {
	r = r.Intersect(img.Rect)
	var sum [4]uint64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			sum[0], sum[1], sum[2], sum[3] = sum[0]+uint64(c.R), sum[1]+uint64(c.G), sum[2]+uint64(c.B), sum[3]+uint64(c.A)
		}
	}
	n := uint64(max(r.Dx()*r.Dy(), 1))
	return color.RGBA{uint8(sum[0] / n), uint8(sum[1] / n), uint8(sum[2] / n), uint8(sum[3] / n)}
}

// over composites c over the premultiplied color under.
func over(c color.NRGBA, under color.RGBA) color.RGBA {
	a := uint32(c.A)
	mix := func(v, u uint8) uint8 { return uint8((uint32(v)*a + uint32(u)*(255-a)) / 255) }
	return color.RGBA{mix(c.R, under.R), mix(c.G, under.G), mix(c.B, under.B), uint8(a + uint32(under.A)*(255-a)/255)}
}

// iconPosition is where Icon Composer puts a group or layer.
type iconPosition struct {
	Scale       float32    `json:"scale"`
	Translation [2]float32 `json:"translation-in-points"`
}

// iconColor reads an Icon Composer color, such as
// srgb:1.00000,0.14913,0.00000,0.37945; display-p3 reads as sRGB.
func iconColor(v string) (color.NRGBA, error) {
	_, values, ok := strings.Cut(v, ":")
	parts := strings.Split(values, ",")
	if !ok || len(parts) != 4 {
		return color.NRGBA{}, fmt.Errorf("unknown color %q", v)
	}
	var c [4]uint8
	for i, s := range parts {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return color.NRGBA{}, fmt.Errorf("unknown color %q", v)
		}
		c[i] = uint8(min(max(f, 0), 1)*255 + 0.5)
	}
	return color.NRGBA{c[0], c[1], c[2], c[3]}, nil
}

// fillDrawing fills the paths of an SVG's drawing into dst in one color,
// the drawing's points scaled by scale and placed at origin.
func fillDrawing(dst *image.RGBA, ops []core.DrawOp, origin [2]float32, scale float32, fill color.Color) error {
	type affine struct{ sx, sy, tx, ty float32 }
	m := affine{scale, scale, origin[0], origin[1]}
	var stack []affine
	at := func(x, y float32) (float32, float32) { return m.sx*x + m.tx, m.sy*y + m.ty }
	b := dst.Bounds()
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	src := image.NewUniform(fill)
	for _, op := range ops {
		switch op.Kind {
		case core.DrawSave:
			stack = append(stack, m)
		case core.DrawRestore:
			m, stack = stack[len(stack)-1], stack[:len(stack)-1]
		case core.DrawTranslate:
			m.tx, m.ty = m.tx+m.sx*op.X, m.ty+m.sy*op.Y
		case core.DrawScale:
			m.sx, m.sy = m.sx*op.X, m.sy*op.Y
		case core.DrawBeginPath:
			z.Reset(b.Dx(), b.Dy())
		case core.DrawMoveTo:
			z.MoveTo(at(op.X, op.Y))
		case core.DrawLineTo:
			z.LineTo(at(op.X, op.Y))
		case core.DrawQuadTo:
			x1, y1 := at(op.X1, op.Y1)
			x, y := at(op.X, op.Y)
			z.QuadTo(x1, y1, x, y)
		case core.DrawCubicTo:
			x1, y1 := at(op.X1, op.Y1)
			x2, y2 := at(op.X2, op.Y2)
			x, y := at(op.X, op.Y)
			z.CubeTo(x1, y1, x2, y2, x, y)
		case core.DrawClosePath:
			z.ClosePath()
		case core.DrawRect:
			roundRect(z, at, op.X, op.Y, op.W, op.H, 0)
		case core.DrawRoundRect:
			roundRect(z, at, op.X, op.Y, op.W, op.H, op.R)
		case core.DrawFill:
			z.Draw(dst, b, src, image.Point{})
		case core.DrawFillColor, core.DrawFillGradient:
		default:
			return fmt.Errorf("the dev badge draws only filled paths and rectangles, not operation %d", op.Kind)
		}
	}
	return nil
}

// roundRect adds a rectangle with circular corners of radius r to z,
// each corner a cubic curve.
func roundRect(z *vector.Rasterizer, at func(x, y float32) (float32, float32), x, y, w, h, r float32) {
	r = min(r, w/2, h/2)
	// k places a cubic's controls so it follows a quarter circle.
	k := r * (1 - 0.5523)
	z.MoveTo(at(x+r, y))
	z.LineTo(at(x+w-r, y))
	x1, y1 := at(x+w-k, y)
	x2, y2 := at(x+w, y+k)
	x3, y3 := at(x+w, y+r)
	z.CubeTo(x1, y1, x2, y2, x3, y3)
	z.LineTo(at(x+w, y+h-r))
	x1, y1 = at(x+w, y+h-k)
	x2, y2 = at(x+w-k, y+h)
	x3, y3 = at(x+w-r, y+h)
	z.CubeTo(x1, y1, x2, y2, x3, y3)
	z.LineTo(at(x+r, y+h))
	x1, y1 = at(x+k, y+h)
	x2, y2 = at(x, y+h-k)
	x3, y3 = at(x, y+h-r)
	z.CubeTo(x1, y1, x2, y2, x3, y3)
	z.LineTo(at(x, y+r))
	x1, y1 = at(x, y+k)
	x2, y2 = at(x+k, y)
	x3, y3 = at(x+r, y)
	z.CubeTo(x1, y1, x2, y2, x3, y3)
	z.ClosePath()
}
