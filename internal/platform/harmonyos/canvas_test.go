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
	if err := json.Unmarshal([]byte(canvasJSON(d.Ops, true)), &ops); err != nil {
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
	ops := decodeCanvas(t, d.Ops, true)
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
	sprites := driver.AtlasSprites{{Rect: driver.Bounds{Size: driver.Size{Width: 1, Height: 1}}, Transform: driver.RSXform{SCos: 1}, Alpha: 1}}
	ops := []driver.DrawOp{{Kind: driver.DrawAtlas, Atlas: &sprites}, {Kind: driver.DrawAtlas, Image: &driver.ImageSource{URI: "a.png"}}}
	for _, packed := range []bool{true, false} {
		if got := decodeCanvas(t, ops, packed); len(got) != 0 {
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
	ops := decodeCanvas(t, d.Ops, false)
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

func decodeCanvas(t *testing.T, ops []driver.DrawOp, packed bool) []canvasOp {
	t.Helper()
	var out []canvasOp
	if err := json.Unmarshal([]byte(canvasJSON(ops, packed)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
