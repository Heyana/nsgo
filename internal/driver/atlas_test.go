package driver

import (
	"slices"
	"testing"
)

func TestDrawAtlasCopiesTheSpriteList(t *testing.T) {
	crop := Bounds{Origin: Point{4, 8}, Size: Size{12, 16}}
	rects := []Bounds{crop, {}, crop}
	placement := RSXform{SCos: 2, TX: 11, TY: 23}
	xforms := []RSXform{placement, placement, placement}
	alphas := []float32{.5}
	src := ImageSource{URI: "asset:atlas.png"}
	d := NewDrawing(nil).DrawAtlas(src, rects, xforms, alphas)
	rects[0], xforms[0], alphas[0] = Bounds{}, RSXform{}, 0
	if len(d.Ops) != 1 || d.Ops[0].Kind != DrawAtlas || d.Image(&d.Ops[0]) == nil || d.Image(&d.Ops[0]).URI != src.URI {
		t.Fatalf("got %+v, want one atlas op", d.Ops)
	}
	got := d.AtlasSprites(&d.Ops[0])
	if len(got) != 2 || got[0] != (AtlasSprite{crop, placement, .5}) || got[1] != (AtlasSprite{crop, placement, 1}) {
		t.Fatalf("recorded %+v", got)
	}
}

func TestCanvasDamageIncludesAnAtlasUnderChangingTransform(t *testing.T) {
	at := Bounds{Origin: Point{5, 4}, Size: Size{2, 3}}
	d := NewDrawing(nil).Translate(3, 4).DrawAtlas(ImageSource{URI: "asset:atlas.png"}, []Bounds{at},
		[]RSXform{{SSin: 2, TX: 11, TY: 7}}, nil)
	after := append([]DrawOp(nil), d.Ops...)
	after[0].X = 6
	want := Bounds{Origin: Point{8, 11}, Size: Size{9, 4}}
	if got, ok := CanvasDamage(d, d.Ops, after); !ok || got != want {
		t.Fatalf("got %v, %v, want %v", got, ok, want)
	}
}

func TestDrawAtlasKeepsSavedSampling(t *testing.T) {
	d := NewDrawing(nil)
	src := ImageSource{URI: "atlas.png"}
	rects := []Bounds{{Size: Size{Width: 16, Height: 16}}}
	xforms := []RSXform{{SCos: 8}}
	d.DrawAtlas(src, rects, xforms, nil)
	d.Save().SetImageSmoothing(false).DrawAtlas(src, rects, xforms, nil)
	d.Save().SetImageSmoothing(true).Restore().DrawAtlas(src, rects, xforms, nil)
	d.Restore().DrawAtlas(src, rects, xforms, nil)
	want := []ImageSampling{ImageSamplingSmooth, ImageSamplingNearest, ImageSamplingNearest, ImageSamplingSmooth}
	var got []ImageSampling
	for _, op := range d.Ops {
		if op.Kind == DrawAtlas {
			got = append(got, op.Sampling)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("atlas sampling = %v, want %v", got, want)
	}
}

func TestConsecutiveImageOpsShareImageEntry(t *testing.T) {
	a := ImageSource{URI: "asset:atlas-a.png"}
	b := ImageSource{URI: "asset:atlas-b.png"}
	d := NewDrawing(nil)
	for i := range 50 {
		d.DrawImageFrom(a, 0, 0, 16, 16, float32(i*16), 0, 16, 16)
	}
	d.DrawImage(b, 0, 16)
	d.DrawTintedImageIn(b, RGB(0xff0000), 16, 16, 16, 16)
	if len(d.Images) != 2 {
		t.Fatalf("stored %d images, want 2 shared entries", len(d.Images))
	}
	if d.Image(&d.Ops[0]).URI != a.URI || d.Image(&d.Ops[49]).URI != a.URI || d.Image(&d.Ops[50]).URI != b.URI || d.Image(&d.Ops[51]).URI != b.URI {
		t.Fatalf("image lookup mismatch across shared entries")
	}
}

func drawSpriteFrame(d *Drawing, atlas, hud ImageSource, rects []Bounds, xforms []RSXform, alphas []float32, frame int) {
	d.Reset()
	shift := float32(frame % 64)
	for range 4 {
		d.DrawAtlas(atlas, rects, xforms, alphas)
	}
	for i := range 256 {
		x := float32(i%32)*16 + shift
		y := float32(i/32) * 16
		d.DrawImageFrom(atlas, 0, 0, 32, 32, x, y, 16, 16)
	}
	for i := range 16 {
		d.DrawTintedImageFrom(hud, RGBA(0xffcc66, 0.75), 0, 0, 16, 16, float32(i*16), 0, 16, 16)
	}
}

func spriteFrameInputs() (atlas, hud ImageSource, rects []Bounds, xforms []RSXform, alphas []float32) {
	atlas = ImageSource{URI: "asset:sprites.png"}
	hud = ImageSource{URI: "asset:hud.png"}
	rects = make([]Bounds, 100)
	xforms = make([]RSXform, 100)
	alphas = make([]float32, 100)
	for i := range 100 {
		rects[i] = Bounds{Origin: Point{X: Points((i % 8) * 32), Y: Points((i / 8) * 32)}, Size: Size{Width: 32, Height: 32}}
		xforms[i] = RSXform{SCos: 1, TX: float32((i % 10) * 20), TY: float32((i / 10) * 20)}
		alphas[i] = 0.9
	}
	return
}

func TestDrawImageAndAtlasZeroAllocInReusedDrawing(t *testing.T) {
	atlas, hud, rects, xforms, alphas := spriteFrameInputs()
	d := NewDrawing(nil)
	// Warm up buffers to steady-state capacity.
	drawSpriteFrame(d, atlas, hud, rects, xforms, alphas, 0)
	if len(d.Ops) != 4+256+16 || len(d.Sprites) != 400 || len(d.Images) != 2 {
		t.Fatalf("unexpected frame counts: ops=%d sprites=%d images=%d", len(d.Ops), len(d.Sprites), len(d.Images))
	}
	frame := 1
	allocs := testing.AllocsPerRun(20, func() {
		drawSpriteFrame(d, atlas, hud, rects, xforms, alphas, frame)
		frame++
	})
	if allocs != 0 {
		t.Fatalf("steady-state reused Drawing allocated %.2f times per run, want 0", allocs)
	}
}

func BenchmarkDrawImageAndAtlasReused(b *testing.B) {
	atlas, hud, rects, xforms, alphas := spriteFrameInputs()
	d := NewDrawing(nil)
	drawSpriteFrame(d, atlas, hud, rects, xforms, alphas, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		drawSpriteFrame(d, atlas, hud, rects, xforms, alphas, i+1)
	}
}
