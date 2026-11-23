package terminal

import (
	"image"
	"image/draw"
	"math"
	"slices"

	"github.com/nsgo-dev/nsgo/base/terminal/internal/vt"
	"github.com/nsgo-dev/nsgo/core"
)

// graphic is a placement snapshot in canvas points, with source coordinates in image pixels. Pixels and placements advance together with the render state.
type graphic struct {
	source                     core.ImageSource
	imageID, placementID       uint32
	z                          int32
	sx, sy, sw, sh, x, y, w, h float32
}

type graphicImage struct {
	generation uint64
	source     core.ImageSource
}

// crop clips the destination and moves the source by the same fraction.
func (g graphic) crop(x, y, w, h float32) (graphic, bool) {
	if g.w <= 0 || g.h <= 0 {
		return g, false
	}
	x0, y0 := max(g.x, x), max(g.y, y)
	x1, y1 := min(g.x+g.w, x+w), min(g.y+g.h, y+h)
	if x1 <= x0 || y1 <= y0 {
		return g, false
	}
	g.sx += (x0 - g.x) * g.sw / g.w
	g.sy += (y0 - g.y) * g.sh / g.h
	g.sw *= (x1 - x0) / g.w
	g.sh *= (y1 - y0) / g.h
	g.x, g.y, g.w, g.h = x0, y0, x1-x0, y1-y0
	return g, true
}

// snapshotGraphics runs with Terminal.mu held, only when render state advances.
func (v *view) snapshotGraphics() {
	t := v.t
	if v.imageCache == nil {
		v.imageCache = map[uint32]graphicImage{}
	}
	seen := map[uint32]bool{}
	source := func(id uint32) (core.ImageSource, bool) {
		gen := t.term.ImageGeneration(id)
		if gen == 0 {
			return core.ImageSource{}, false
		}
		seen[id] = true
		cached, ok := v.imageCache[id]
		if ok && cached.generation == gen {
			return cached.source, true
		}
		img, ok := t.term.Image(id)
		if !ok {
			return core.ImageSource{}, false
		}
		var raw *image.NRGBA
		if img.Format == vt.ImageRGBA {
			raw = &image.NRGBA{Pix: img.Pixels, Stride: 4 * img.Width, Rect: image.Rect(0, 0, img.Width, img.Height)}
		} else {
			raw = decoded(img)
		}
		pixels := image.NewRGBA(raw.Rect)
		draw.Draw(pixels, pixels.Rect, raw, image.Point{}, draw.Src)
		src := core.ImageSource{Pixels: pixels, Version: gen, Size: core.Size{Width: core.Points(img.Width), Height: core.Points(img.Height)}}
		v.imageCache[id] = graphicImage{gen, src}
		return src, true
	}
	// Drop pixel references from placements that disappeared, including the unused tail of the reused backing array.
	clear(v.graphics)
	v.graphics = v.graphics[:0]
	var virtual []vt.Placement
	t.term.Placements(func(p vt.Placement) {
		if p.Virtual {
			virtual = append(virtual, p)
			return
		}
		if !p.Visible {
			return
		}
		src, ok := source(p.ImageID)
		if !ok {
			return
		}
		xoff, yoff, w, h := placementGeometry(p, v.cellW, v.cellH, v.ratio, t.size.cellW, t.size.cellH)
		v.graphics = append(v.graphics, graphic{source: src, imageID: p.ImageID, placementID: p.ID, z: p.Z,
			sx: float32(p.SrcX), sy: float32(p.SrcY), sw: float32(p.SrcW), sh: float32(p.SrcH),
			x: v.padX + float32(p.Col)*v.cellW + xoff,
			y: v.padY + float32(p.Row)*v.cellH + yoff,
			w: w, h: h})
	})
	// Sort before choosing a virtual target without an explicit placement ID.
	slices.SortFunc(virtual, func(a, b vt.Placement) int { return cmpID(a.ID, b.ID) })
	if len(virtual) > 0 {
		rs := t.rs
		rs.Rows()
		for y := 0; rs.NextRow(); y++ {
			var prev placeholder
			previous := false
			for x, c := range rs.RowCells() {
				if c.Codepoint != kittyPlaceholder {
					previous = false
					continue
				}
				p := placeholderOf(rs.CellStyle(x), rs.CellGraphemes(x, nil))
				if previous && p.follows(prev) {
					if p.row < 0 {
						p.row = prev.row
					}
					if p.col < 0 {
						p.col = prev.col + 1
					}
					if p.high < 0 {
						p.high = prev.high
					}
				}
				p.row, p.col, p.high = max(p.row, 0), max(p.col, 0), max(p.high, 0)
				prev, previous = p, true
				id := p.low | uint32(p.high)<<24
				for _, pl := range virtual {
					if pl.ImageID != id || p.placement != 0 && p.placement != pl.ID {
						continue
					}
					src, ok := source(id)
					if !ok {
						break
					}
					iw, ih := float32(src.Size.Width), float32(src.Size.Height)
					cols, rows := pl.ParamCols, pl.ParamRows
					if cols == 0 {
						cols = int(math.Ceil(float64(iw / (v.cellW * v.ratio))))
					}
					if rows == 0 {
						rows = int(math.Ceil(float64(ih / (v.cellH * v.ratio))))
					}
					if cols <= 0 || rows <= 0 || p.col >= cols || p.row >= rows {
						break
					}
					gw, gh := float32(cols)*v.cellW, float32(rows)*v.cellH
					scale := min(gw/iw, gh/ih)
					w, h := iw*scale, ih*scale
					cx, cy := v.padX+float32(x)*v.cellW, v.padY+float32(y)*v.cellH
					g := graphic{source: src, imageID: id, placementID: pl.ID, z: pl.Z, sw: iw, sh: ih,
						x: cx - float32(p.col)*v.cellW + (gw-w)/2, y: cy - float32(p.row)*v.cellH + (gh-h)/2, w: w, h: h}
					if g, ok = g.crop(cx, cy, v.cellW, v.cellH); ok {
						v.graphics = append(v.graphics, g)
					}
					break
				}
			}
		}
	}
	for id := range v.imageCache {
		if !seen[id] {
			delete(v.imageCache, id)
		}
	}
	slices.SortStableFunc(v.graphics, func(a, b graphic) int {
		if a.z < b.z {
			return -1
		}
		if a.z > b.z {
			return 1
		}
		if n := cmpID(a.imageID, b.imageID); n != 0 {
			return n
		}
		return cmpID(a.placementID, b.placementID)
	})
}

// placementGeometry keeps natural image pixels and offsets physical, but maps explicit cell counts to the actual fractional canvas grid. The VT API reports integral cell pixels; scaling its whole rectangle would stretch natural images.
func placementGeometry(p vt.Placement, cw, ch, ratio float32, cellW, cellH int) (xoff, yoff, w, h float32) {
	xoff = float32(min(p.XOffset, max(cellW-1, 0))) / ratio
	yoff = float32(min(p.YOffset, max(cellH-1, 0))) / ratio
	w, h = float32(p.Width)/ratio, float32(p.Height)/ratio
	if p.ParamCols > 0 {
		w = max(float32(p.ParamCols)*cw-xoff, 0)
	}
	if p.ParamRows > 0 {
		h = max(float32(p.ParamRows)*ch-yoff, 0)
	}
	if p.ParamCols > 0 && p.ParamRows == 0 && p.SrcW > 0 {
		h = w * float32(p.SrcH) / float32(p.SrcW)
	}
	if p.ParamRows > 0 && p.ParamCols == 0 && p.SrcH > 0 {
		w = h * float32(p.SrcW) / float32(p.SrcH)
	}
	return
}

func cmpID(a, b uint32) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

const kittyPlaceholder = '\U0010eeee'

type placeholder struct {
	low, placement uint32
	row, col, high int
}

func placeholderOf(s vt.Style, graphemes []rune) placeholder {
	p := placeholder{low: graphicID(s.FG), placement: graphicID(s.UnderlineColor), row: -1, col: -1, high: -1}
	indices := []*int{&p.row, &p.col, &p.high}
	if len(graphemes) > 0 && graphemes[0] == kittyPlaceholder {
		graphemes = graphemes[1:]
	}
	for i, r := range graphemes {
		if i >= len(indices) {
			break
		}
		if at, ok := slices.BinarySearch(kittyDiacritics[:], r); ok && (i != 2 || at < 256) {
			*indices[i] = at
		}
	}
	return p
}

func graphicID(c vt.Color) uint32 {
	switch c.Kind {
	case 1:
		return uint32(c.Index)
	case 2:
		return uint32(c.RGB.R)<<16 | uint32(c.RGB.G)<<8 | uint32(c.RGB.B)
	}
	return 0
}

func (p placeholder) follows(prev placeholder) bool {
	return p.low == prev.low && p.placement == prev.placement && (p.row < 0 || p.row == prev.row) && (p.col < 0 || p.col == prev.col+1) && (p.high < 0 || p.high == prev.high)
}

// paintGraphics draws one of the protocol's three stacking layers. Negative images above backgrounds are clipped to each row so later row backgrounds cannot cover a preceding row's image.
func (f *frame) paintGraphics(layer, row int) {
	v := f.v
	x, y, w, h := v.padX, v.padY, float32(v.cols)*v.cellW, float32(v.rows)*v.cellH
	if layer == 1 {
		y += float32(row) * v.cellH
		h = v.cellH
	}
	for _, g := range v.graphics {
		l := 2
		if g.z < -1<<30 {
			l = 0
		} else if g.z < 0 {
			l = 1
		}
		if l != layer {
			continue
		}
		if g, ok := g.crop(x, y, w, h); ok {
			f.d.DrawImageFrom(g.source, g.sx, g.sy, g.sw, g.sh, g.x, g.y, g.w, g.h)
		}
	}
}
