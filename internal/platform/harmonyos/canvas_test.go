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
	if err := json.Unmarshal([]byte(canvasJSON(d.Ops)), &ops); err != nil {
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
	ops := decodeCanvas(t, d.Ops)
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
	if got := decodeCanvas(t, ops); len(got) != 0 {
		t.Fatalf("got %+v, want nothing", got)
	}
}

func decodeCanvas(t *testing.T, ops []driver.DrawOp) []canvasOp {
	t.Helper()
	var out []canvasOp
	if err := json.Unmarshal([]byte(canvasJSON(ops)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
