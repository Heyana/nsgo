//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

// TestCanvasImagesCarryTheirSampling checks that each image op of the sampling drawing tells the host whether to smooth it, through Save, Restore and a smoothing set and restored in between: the left draws smooth and the right nearest.
func TestCanvasImagesCarryTheirSampling(t *testing.T) {
	d := driver.NewDrawing(nil)
	canvastest.DrawSampling(d, driver.ImageSource{URI: "asset:checker.png"}, 64)
	var ops []canvasOp
	if err := json.Unmarshal([]byte(canvasJSON(d, d.Ops, true)), &ops); err != nil {
		t.Fatal(err)
	}
	var nearest []bool
	for _, o := range ops {
		if o.K == int(driver.DrawImage) {
			nearest = append(nearest, o.Nearest)
		}
	}
	want := []bool{false, true, true, false}
	if len(nearest) != len(want) {
		t.Fatalf("%d image ops, want %d", len(nearest), len(want))
	}
	for i := range want {
		if nearest[i] != want[i] {
			t.Fatalf("image ops' nearest sampling = %v, want %v", nearest, want)
		}
	}
}

// TestImageSendsItsSampling checks that an image view's operation ends with whether it samples nearest, which Index.ets reads as o[5].
func TestImageSendsItsSampling(t *testing.T) {
	for _, sampling := range []driver.ImageSampling{driver.ImageSamplingSmooth, driver.ImageSamplingNearest} {
		m := &mounter{views: map[driver.NodeID]*view{}}
		m.item(&driver.MountBatch{}, &driver.MountItem{Kind: driver.ItemImage, ID: 4, Image: &driver.ImageConfig{Source: driver.ImageSource{URI: "a.png"}, Sampling: sampling}})
		var got [][]any
		if err := json.Unmarshal(m.o.bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || len(got[0]) != 6 || got[0][0] != "im" {
			t.Fatalf("image operations %v, want one im with 5 arguments", got)
		}
		if want := sampling == driver.ImageSamplingNearest; got[0][5] != want {
			t.Fatalf("sampling %d sent nearest %v, want %v", sampling, got[0][5], want)
		}
	}
}

// TestCanvasSendsAtlasPacked checks that an atlas reaches the host as one op holding its image, whether it samples nearest and its sprites packed nine numbers each, in the order Index.ets's drawAtlas reads them, rather than as an image op per sprite.
func TestCanvasSendsAtlasPacked(t *testing.T) {
	src := driver.ImageSource{URI: "asset:atlas.png", Data: []byte{1, 2, 3}}
	rects := []driver.Bounds{
		{Origin: driver.Point{X: 4, Y: 8}, Size: driver.Size{Width: 12, Height: 16}},
		{Origin: driver.Point{X: 0, Y: 0}, Size: driver.Size{Width: 2, Height: 3}},
	}
	xforms := []driver.RSXform{{SCos: 2, TX: 11, TY: 23}, {SCos: 0.5, SSin: -1, TX: -3, TY: 7}}
	d := driver.NewDrawing(nil).SetAlpha(0.5).SetImageSmoothing(false).DrawAtlas(src, rects, xforms, []float32{0.25})
	ops := decodeCanvas(t, d, d.Ops, true)
	if len(ops) != 2 {
		t.Fatalf("got %d ops, want the alpha and one atlas: %+v", len(ops), ops)
	}
	if ops[0].K != int(driver.DrawAlpha) || ops[0].A == nil || *ops[0].A != 0.5 {
		t.Errorf("alpha op = %+v", ops[0])
	}
	atlas := ops[1]
	if atlas.K != int(driver.DrawAtlas) || atlas.Img != src.URI || atlas.ImgData != "AQID" || !atlas.Nearest {
		t.Errorf("atlas op = %+v", atlas)
	}
	want := []float32{
		4, 8, 12, 16, 2, 0, 11, 23, 0.25,
		0, 0, 2, 3, 0.5, -1, -3, 7, 1,
	}
	if !slices.Equal(atlas.Sprites, want) {
		t.Errorf("sprites = %v, want %v", atlas.Sprites, want)
	}
}

// TestCanvasDropsAnAtlasWithoutAnImage checks that an atlas op the host could not draw is not sent.
func TestCanvasDropsAnAtlasWithoutAnImage(t *testing.T) {
	d := &driver.Drawing{
		Images:  []driver.ImageSource{{URI: "a.png"}},
		Sprites: []driver.AtlasSprite{{Rect: driver.Bounds{Size: driver.Size{Width: 1, Height: 1}}, Transform: driver.RSXform{SCos: 1}, Alpha: 1}},
		Ops:     []driver.DrawOp{{Kind: driver.DrawAtlas, SpriteStart: 0, SpriteLen: 1}, {Kind: driver.DrawAtlas, ImageIndex: 1}},
	}
	for _, packed := range []bool{true, false} {
		if got := decodeCanvas(t, d, d.Ops, packed); len(got) != 0 {
			t.Fatalf("packed %v: got %+v, want nothing", packed, got)
		}
	}
}

// TestCanvasExpandsAtlasForAnOlderHost checks that a host which does not report CanvasAtlas, such as a committed prebuilt host built before the packed op, gets each sprite as a save, its transform, its alpha times the saved alpha, the image with its sampler and a restore.
func TestCanvasExpandsAtlasForAnOlderHost(t *testing.T) {
	src := driver.ImageSource{URI: "asset:atlas.png"}
	rects := []driver.Bounds{
		{Origin: driver.Point{X: 4, Y: 8}, Size: driver.Size{Width: 12, Height: 16}},
		{Origin: driver.Point{X: 0, Y: 0}, Size: driver.Size{Width: 2, Height: 3}},
	}
	xforms := []driver.RSXform{{SCos: 2, TX: 11, TY: 23}, {SCos: 0.5, SSin: -1, TX: -3, TY: 7}}
	d := driver.NewDrawing(nil).SetAlpha(0.5).SetImageSmoothing(false).DrawAtlas(src, rects, xforms, []float32{0.25})
	ops := decodeCanvas(t, d, d.Ops, false)
	var kinds []int
	for _, o := range ops {
		kinds = append(kinds, o.K)
		if o.K == int(driver.DrawAtlas) {
			t.Fatalf("an older host got an atlas op: %+v", o)
		}
	}
	sprite := []int{int(driver.DrawSave), int(driver.DrawTransform), int(driver.DrawAlpha), int(driver.DrawImage), int(driver.DrawRestore)}
	want := append(append([]int{int(driver.DrawAlpha)}, sprite...), sprite...)
	if !slices.Equal(kinds, want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	for i, w := range []struct {
		a              float32
		sc, ss, tx, ty float32
		sx, sy, sw, sh float32
	}{{0.125, 2, 0, 11, 23, 4, 8, 12, 16}, {0.5, 0.5, -1, -3, 7, 0, 0, 2, 3}} {
		tr, al, img := ops[1+5*i+1], ops[1+5*i+2], ops[1+5*i+3]
		if *tr.X1 != w.sc || tr.Y1 != w.ss || tr.X2 != -w.ss || *tr.Y2 != w.sc || tr.X != w.tx || tr.Y != w.ty {
			t.Errorf("sprite %d transform = %+v", i, tr)
		}
		if *al.A != w.a {
			t.Errorf("sprite %d alpha = %v, want %v", i, *al.A, w.a)
		}
		if *img.X1 != w.sx || img.Y1 != w.sy || img.W != w.sw || img.H != w.sh || img.X2 != w.sw || *img.Y2 != w.sh || img.Img != src.URI || !img.Nearest {
			t.Errorf("sprite %d image = %+v", i, img)
		}
	}
}

func decodeCanvas(t *testing.T, d *driver.Drawing, ops []driver.DrawOp, packed bool) []canvasOp {
	t.Helper()
	var out []canvasOp
	if err := json.Unmarshal([]byte(canvasJSON(d, ops, packed)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func meshVertex(x, y, u, v driver.Points, c driver.Color) driver.CanvasVertex {
	return driver.CanvasVertex{Position: driver.Point{X: x, Y: y}, TexCoord: driver.Point{X: u, Y: v}, Color: c}
}

// encode returns a drawing's image ops and mesh ops as the host reads them.
func encode(t *testing.T, d *driver.Drawing) (images, meshes []canvasOp) {
	t.Helper()
	var ops []canvasOp
	if err := json.Unmarshal([]byte(canvasJSON(d, d.Ops, true)), &ops); err != nil {
		t.Fatal(err)
	}
	for _, o := range ops {
		switch o.K {
		case int(driver.DrawImage):
			images = append(images, o)
		case int(driver.DrawVertices):
			meshes = append(meshes, o)
		}
	}
	return images, meshes
}

// TestCanvasEncodesMeshes checks that a mesh reaches the host as its vertices and mean alphas, and that two white triangles covering a crop reach it as the image op.
func TestCanvasEncodesMeshes(t *testing.T) {
	src := driver.ImageSource{URI: "asset:atlas.png"}
	at := meshVertex
	a, b, c, e := at(10, 10, 2, 2, 0), at(30, 10, 6, 2, 0), at(30, 30, 6, 6, 0), at(10, 30, 2, 6, 0)
	d := driver.NewDrawing(nil).
		DrawVertices(src, []driver.CanvasVertex{a, b, c, a, c, e}).
		SetImageSmoothing(false).
		DrawVertices(src, []driver.CanvasVertex{a, b, c, a, c, at(10, 30, 2, 6, driver.RGBA(0xffffff, 0))})
	images, meshes := encode(t, d)
	if len(images) != 1 || len(meshes) != 1 {
		t.Fatalf("%d images and %d meshes, want one each", len(images), len(meshes))
	}
	if im := images[0]; im.Img != src.URI || im.X != 10 || im.W != 20 || im.X1 == nil || *im.X1 != 2 || im.X2 != 4 || im.Y2 == nil || *im.Y2 != 4 || im.Nearest {
		t.Errorf("the quad's image op is %+v", im)
	}
	m := meshes[0]
	if m.Img != src.URI || len(m.V) != 24 || m.V[4] != 30 || m.V[6] != 6 || !m.Nearest {
		t.Errorf("the mesh op is %+v", m)
	}
	if len(m.VA) != 2 || m.VA[0] != 1 || m.VA[1] != float32(2.0/3) {
		t.Errorf("the mesh's alphas are %v, want [1 2/3]", m.VA)
	}
	if m.E != nil {
		t.Errorf("the mesh's seams are %v, want none beside a translucent triangle", m.E)
	}
}

// TestCanvasEncodesNearestSampling checks that SetImageSmoothing(false) reaches the host on a plain image and on a mesh drawn as an image crop, as it does on other meshes.
func TestCanvasEncodesNearestSampling(t *testing.T) {
	src := driver.ImageSource{URI: "asset:atlas.png"}
	a, b, c, e := meshVertex(10, 10, 2, 2, 0), meshVertex(30, 10, 6, 2, 0), meshVertex(30, 30, 6, 6, 0), meshVertex(10, 30, 2, 6, 0)
	d := driver.NewDrawing(nil).
		DrawImage(src, 0, 0).
		SetImageSmoothing(false).
		DrawVertices(src, []driver.CanvasVertex{a, b, c, a, c, e}).
		DrawImageIn(src, 0, 0, 8, 8)
	images, meshes := encode(t, d)
	if len(images) != 3 || len(meshes) != 0 {
		t.Fatalf("%d images and %d meshes, want three images", len(images), len(meshes))
	}
	if images[0].Nearest || !images[1].Nearest || !images[2].Nearest {
		t.Errorf("nearest sampling is %v, %v, %v, want false, true, true", images[0].Nearest, images[1].Nearest, images[2].Nearest)
	}
}

// TestCanvasEncodesMeshSeams checks that the host learns which edges two opaque triangles share, and that a triangle whose texture coordinates meet at one texel still reaches it.
func TestCanvasEncodesMeshSeams(t *testing.T) {
	src := driver.ImageSource{URI: "asset:atlas.png"}
	at := meshVertex
	// A fan of three triangles around (0, 0), the last translucent, and one apart with a single texel's coordinates.
	o, p, q, r, s := at(0, 0, 1, 1, 0), at(10, 0, 5, 1, 0), at(10, 10, 5, 5, 0), at(0, 10, 1, 5, 0), at(-10, 5, 0, 3, driver.RGBA(0xffffff, 0.5))
	white := func(x, y driver.Points) driver.CanvasVertex { return at(x, y, 7, 7, 0) }
	d := driver.NewDrawing(nil).DrawVertices(src, []driver.CanvasVertex{
		o, p, q,
		q, r, o,
		r, s, o,
		white(40, 40), white(50, 40), white(40, 50),
	})
	_, meshes := encode(t, d)
	if len(meshes) != 1 {
		t.Fatalf("%d meshes, want one", len(meshes))
	}
	m := meshes[0]
	// Triangle 0 shares its edge 2 (q to o) with triangle 1's edge 2 (o to q); triangle 1's edge 1 (r to o) borders only the translucent triangle.
	if want := []int{0b100, 0b100, 0, 0}; len(m.E) != len(want) || m.E[0] != want[0] || m.E[1] != want[1] || m.E[2] != want[2] || m.E[3] != want[3] {
		t.Errorf("the seams are %v, want %v", m.E, want)
	}
	if len(m.V) != 48 || m.V[38] != 7 || m.V[47] != 7 {
		t.Errorf("the single-texel triangle is not sent whole: %v", m.V)
	}
}
