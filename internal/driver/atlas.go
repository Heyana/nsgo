package driver

import "slices"

// RSXform is a scaled rotation followed by a translation. The sprite's
// local origin is its top-left corner. SCos and SSin are the cosine and
// sine of the rotation multiplied by the scale. TX and TY are in canvas
// points. RSXform{SCos: 1} is the identity.
type RSXform struct {
	SCos, SSin float32
	TX, TY     float32
}

// AtlasSprite is one source rectangle, its transform and opacity.
type AtlasSprite struct {
	Rect      Bounds
	Transform RSXform
	Alpha     float32
}

// AtlasSprites holds the immutable records of one DrawAtlas operation.
type AtlasSprites []AtlasSprite

// DrawAtlas draws tiles from img in one operation, in the order given.
// Source bounds are in the image's own points, as with DrawImageFrom. Each
// corresponding transform acts on a tile starting at (0, 0) at its source
// size. Alpha multiplies the drawing's alpha, defaulting to 1 for entries
// absent from alphas. Rectangles and transforms are paired up to the shorter
// slice. Invalid rectangles and transforms are skipped. The drawing keeps a
// copy of the sprite data in its Sprites buffer.
func (d *Drawing) DrawAtlas(img ImageSource, rects []Bounds, xforms []RSXform, alphas []float32) *Drawing {
	n := min(len(rects), len(xforms))
	if n == 0 || img.Empty() {
		return d
	}
	start := len(d.Sprites)
	d.Sprites = slices.Grow(d.Sprites, n)
	for i, r := range rects[:n] {
		t := xforms[i]
		if r.Size.Width <= 0 || r.Size.Height <= 0 || t.SCos == 0 && t.SSin == 0 ||
			!finite(float64(r.Origin.X)) || !finite(float64(r.Origin.Y)) ||
			!finite(float64(r.Size.Width)) || !finite(float64(r.Size.Height)) ||
			!finite(float64(t.SCos)) || !finite(float64(t.SSin)) || !finite(float64(t.TX)) || !finite(float64(t.TY)) {
			continue
		}
		alpha := float32(1)
		if i < len(alphas) && finite(float64(alphas[i])) {
			alpha = min(max(alphas[i], 0), 1)
		}
		d.Sprites = append(d.Sprites, AtlasSprite{Rect: r, Transform: t, Alpha: alpha})
	}
	count := len(d.Sprites) - start
	if count == 0 {
		return d
	}
	return d.op(DrawOp{Kind: DrawAtlas, ImageIndex: d.addImage(img), SpriteStart: int32(start), SpriteLen: int32(count)})
}
