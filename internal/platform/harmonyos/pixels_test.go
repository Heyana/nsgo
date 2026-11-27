//go:build js && wasm

package harmonyos

import (
	"encoding/base64"
	"encoding/json"
	"image"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestImageSendsEachPixelCopyOnce checks that an image view gets its pixel copy as an ip op of width, height and base64 RGBA when the copy changes, nothing when the same copy is sent again, and an empty ip when it stops showing pixels.
func TestImageSendsEachPixelCopyOnce(t *testing.T) {
	m := &mounter{views: map[driver.NodeID]*view{4: {id: 4}}}
	first := image.NewRGBA(image.Rect(0, 0, 2, 1))
	copy(first.Pix, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	second := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixelOps := func(src driver.ImageSource) [][]any {
		t.Helper()
		m.o.reset()
		m.item(&driver.MountBatch{}, &driver.MountItem{Kind: driver.ItemImage, ID: 4, Image: &driver.ImageConfig{Source: src}})
		var got [][]any
		if err := json.Unmarshal(m.o.bytes(), &got); err != nil {
			t.Fatal(err)
		}
		var ip [][]any
		for _, o := range got {
			if o[0] == "ip" {
				ip = append(ip, o)
			}
		}
		return ip
	}
	want := base64.StdEncoding.EncodeToString(first.Pix)
	if got := pixelOps(driver.ImageSource{Pixels: first, Version: 1}); len(got) != 1 || got[0][2] != 2.0 || got[0][3] != 1.0 || got[0][4] != want {
		t.Fatalf("first copy sent %v, want one ip of 2, 1 and %q", got, want)
	}
	if got := pixelOps(driver.ImageSource{Pixels: first, Version: 1}); len(got) != 0 {
		t.Fatalf("the same copy sent again as %v", got)
	}
	if got := pixelOps(driver.ImageSource{Pixels: second, Version: 2}); len(got) != 1 || got[0][2] != 1.0 || got[0][3] != 1.0 {
		t.Fatalf("second copy sent %v", got)
	}
	if got := pixelOps(driver.ImageSource{URI: "a.png"}); len(got) != 1 || got[0][2] != 0.0 || got[0][4] != "" {
		t.Fatalf("leaving pixels sent %v, want an empty ip", got)
	}
}

// TestCanvasSendsPixelCopiesOnceAndByKey checks that a drawing's pixel copy travels with the first op that draws it, the rest naming it by the same key, that the key holds across drawings, and that another copy gets another key.
func TestCanvasSendsPixelCopiesOnceAndByKey(t *testing.T) {
	first := image.NewRGBA(image.Rect(0, 0, 3, 2))
	second := image.NewRGBA(image.Rect(0, 0, 3, 2))
	draw := func(px *image.RGBA) []canvasOp {
		t.Helper()
		src := driver.ImageSource{Pixels: px, Version: 1}
		d := driver.NewDrawing(nil).DrawImage(src, 0, 0).DrawImageIn(src, 4, 4, 6, 4)
		var images []canvasOp
		for _, o := range decodeCanvas(t, d, d.Ops, true) {
			if o.K == int(driver.DrawImage) {
				images = append(images, o)
			}
		}
		if len(images) != 2 {
			t.Fatalf("%d image ops, want 2", len(images))
		}
		return images
	}
	a := draw(first)
	if a[0].ImgKey == "" || a[0].Pixels != base64.StdEncoding.EncodeToString(first.Pix) || a[0].PW != 3 || a[0].PH != 2 {
		t.Fatalf("first op = %+v, want the copy's key, bytes and size", a[0])
	}
	if a[1].ImgKey != a[0].ImgKey || a[1].Pixels != "" || a[1].PW != 0 {
		t.Fatalf("second op = %+v, want only the key %q", a[1], a[0].ImgKey)
	}
	if b := draw(first); b[0].ImgKey != a[0].ImgKey || b[0].Pixels == "" {
		t.Fatalf("the copy drawn again is %+v, want key %q with its bytes", b[0], a[0].ImgKey)
	}
	if c := draw(second); c[0].ImgKey == a[0].ImgKey {
		t.Fatalf("another copy kept key %q", c[0].ImgKey)
	}
}

// TestCanvasExpandedAtlasSendsPixelsOnce checks that an atlas of a pixel copy expanded for an older host carries the bytes with its first sprite only.
func TestCanvasExpandedAtlasSendsPixelsOnce(t *testing.T) {
	px := image.NewRGBA(image.Rect(0, 0, 4, 4))
	rects := []driver.Bounds{{Size: driver.Size{Width: 2, Height: 2}}, {Origin: driver.Point{X: 2}, Size: driver.Size{Width: 2, Height: 2}}}
	xforms := []driver.RSXform{{SCos: 1}, {SCos: 1, TX: 8}}
	d := driver.NewDrawing(nil).DrawAtlas(driver.ImageSource{Pixels: px, Version: 1}, rects, xforms, nil)
	var withBytes, named int
	for _, o := range decodeCanvas(t, d, d.Ops, false) {
		if o.K != int(driver.DrawImage) {
			continue
		}
		named++
		if o.Pixels != "" {
			withBytes++
		}
	}
	if named != 2 || withBytes != 1 {
		t.Fatalf("%d sprite images, %d with bytes; want 2 and 1", named, withBytes)
	}
}

// TestPixelKeysForgetOldCopies checks that the key registry holds 16 copies and names a forgotten copy anew.
func TestPixelKeysForgetOldCopies(t *testing.T) {
	var k pixelKeys
	first := image.NewRGBA(image.Rect(0, 0, 1, 1))
	key := k.key(first)
	for range 16 {
		k.key(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	}
	if len(k.named) != 16 {
		t.Fatalf("%d copies named, want 16", len(k.named))
	}
	if again := k.key(first); again == key {
		t.Fatalf("a forgotten copy kept its key %q", key)
	}
}
