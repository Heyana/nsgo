//go:build windows

package windows

import (
	"image"
	"image/color"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

func TestCanvasAtlasDrawsLikeImages(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	ctx3 := g.dc.query(&iidD2D1DeviceContext3)
	nativeBatch := ctx3 != nil
	ctx3.release()
	pixels := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := range 32 {
		for x := range 64 {
			pixels.SetRGBA(x, y, color.RGBA{R: uint8(4 * x), G: uint8(8 * y), B: 64, A: 255})
		}
	}
	source := driver.ImageSource{Pixels: pixels}
	whole := driver.Bounds{Size: driver.Size{Width: 32, Height: 32}}
	other := driver.Bounds{Origin: driver.Point{X: 32}, Size: whole.Size}
	testCases := []struct {
		name    string
		rects   []driver.Bounds
		xforms  []driver.RSXform
		alphas  []float32
		setup   func(*driver.Drawing)
		world   matrix
		batched bool
	}{
		{
			name:    "downsampled and one-to-one",
			rects:   []driver.Bounds{whole, other},
			xforms:  []driver.RSXform{{SCos: 0.25, TX: 4, TY: 3}, {SCos: 0.5, TX: 9, TY: 9}},
			alphas:  []float32{1, 0.5},
			world:   translation(3, 2),
			batched: true,
		},
		{
			name:    "nearest native batch",
			rects:   []driver.Bounds{whole, other},
			xforms:  []driver.RSXform{{SCos: 0.25, TX: 4, TY: 3}, {SCos: 0.5, TX: 9, TY: 9}},
			setup:   func(d *driver.Drawing) { d.SetImageSmoothing(false) },
			world:   translation(3, 2),
			batched: true,
		},
		{
			name:   "nearest enlarged crop",
			rects:  []driver.Bounds{whole},
			xforms: []driver.RSXform{{SCos: 1.5, TX: 2, TY: 3}},
			setup:  func(d *driver.Drawing) { d.SetImageSmoothing(false) },
			world:  identity,
		},
		{
			name:   "rotated",
			rects:  []driver.Bounds{whole},
			xforms: []driver.RSXform{{SCos: 0.5, SSin: 0.25, TX: 10, TY: 4}},
			world:  identity,
		},
		{
			name:   "fractional crop",
			rects:  []driver.Bounds{{Origin: driver.Point{X: 4.25, Y: 2}, Size: driver.Size{Width: 12, Height: 10}}},
			xforms: []driver.RSXform{{SCos: 0.5, TX: 7, TY: 6}},
			world:  identity,
		},
		{
			name:   "subpixel view translation",
			rects:  []driver.Bounds{whole},
			xforms: []driver.RSXform{{SCos: 0.5, TX: 7, TY: 6}},
			world:  translation(0.25, 0),
		},
		{
			name:   "composite and shadow",
			rects:  []driver.Bounds{whole, other},
			xforms: []driver.RSXform{{SCos: 0.5, TX: 6, TY: 7}, {SCos: 0.5, TX: 14, TY: 11}},
			setup: func(d *driver.Drawing) {
				d.SetFillColor(driver.RGB(0x786499)).FillRect(3, 4, 20, 18)
				d.SetComposite(driver.CompositeDestinationOver).SetShadow(driver.RGB(0x405060), 2, 3, 4)
			},
			world: identity,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			w := &window{scale: canvastest.Scale, animator: driver.NewAnimator(nil)}
			defer w.closeCanvasImages()
			atlas, images := &driver.Drawing{}, &driver.Drawing{}
			if tt.setup != nil {
				tt.setup(atlas)
				tt.setup(images)
			}
			atlas.DrawAtlas(source, tt.rects, tt.xforms, tt.alphas)
			for i, r := range tt.rects {
				xform := tt.xforms[i]
				alpha := float32(1)
				if i < len(tt.alphas) {
					alpha = tt.alphas[i]
				}
				images.Save().SetAlpha(alpha).
					Transform(xform.SCos, xform.SSin, -xform.SSin, xform.SCos, xform.TX, xform.TY).
					DrawImageFrom(source, float32(r.Origin.X), float32(r.Origin.Y), float32(r.Size.Width), float32(r.Size.Height),
						0, 0, float32(r.Size.Width), float32(r.Size.Height)).
					Restore()
			}
			const size = 40
			paint := func(p *painter, d *driver.Drawing, draw func(*view)) {
				p.setTransform(tt.world)
				v := &view{drawing: d}
				defer v.canvasCache.release()
				draw(v)
			}
			want := drawImage(t, g, size, size, func(p *painter) {
				paint(p, images, func(v *view) { p.playCanvas(v, images.Ops, size, size) })
			}, w)
			got := drawImage(t, g, size, size, func(p *painter) {
				paint(p, atlas, func(v *view) { p.canvas(v, size, size) })
			}, w)
			if n, at := canvastest.Differ(got, want); n > 0 {
				t.Errorf("%d pixels differ from DrawImageFrom, first at %v, got %v, want %v", n, at, got.RGBAAt(at.X, at.Y), want.RGBAAt(at.X, at.Y))
			}
			if tt.batched && nativeBatch {
				direct := drawImage(t, g, size, size, func(p *painter) {
					c := p.newCanvasPlayer(&view{}, tt.world, size, size)
					s := w.canvasImage(source)
					if !c.batchAtlas(s.bitmapOf(p.dc, 0), *atlas.Ops[0].Atlas, 1, s.pic.width, s.pic.height, atlas.Ops[0].Sampling) {
						t.Fatal("the compatible DrawAtlas did not batch")
					}
				}, w)
				if n, at := canvastest.Differ(direct, want); n > 0 {
					t.Errorf("%d directly batched pixels differ, first at %v, got %v, want %v", n, at, direct.RGBAAt(at.X, at.Y), want.RGBAAt(at.X, at.Y))
				}
			}
		})
	}
}
