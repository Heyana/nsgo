package core

import (
	"image"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestOffscreenImageLifecycleAndCaching(t *testing.T) {
	var (
		cameraX *Signal[float32]
		layer   *OffscreenImage
		draws   int
		color   = RGB(0xff0000)
	)
	h := newHost(t, Size{Width: 300, Height: 200}, func(cx *Scope) Node {
		cameraX = NewSignal(cx, float32(0))
		layer = cx.NewOffscreenImage(Size{Width: 128, Height: 64}, 2, func(d *Drawing) {
			draws++
			d.SetFillColor(color).FillRect(0, 0, 128, 64)
		})
		return Canvas{
			TestID: "host",
			Style:  Style{Width: Pt(300), Height: Pt(200)},
			Draw: func(d *Drawing, _ Size) {
				d.DrawImage(layer.Source(), -cameraX.Get(), 0)
			},
		}
	})
	h.commit()
	if draws != 1 || layer.Version() != 1 {
		t.Fatalf("initial draw: draws=%d version=%d, want 1, 1", draws, layer.Version())
	}
	hostCanvas := h.find("host").canvas.drawing
	if len(hostCanvas.Ops) != 1 || hostCanvas.Ops[0].Kind != driver.DrawImage {
		t.Fatalf("host canvas ops = %d, want 1 DrawImage", len(hostCanvas.Ops))
	}
	firstSnap := hostCanvas.Ops[0].Image.Offscreen
	if firstSnap == nil || firstSnap.ID != layer.ID() || firstSnap.Version != 1 || firstSnap.Scale != 2 {
		t.Fatalf("unexpected initial snapshot: %+v", firstSnap)
	}

	// Pan the camera across several frames without invalidating: the offscreen
	// callback must not re-run and the exact same snapshot pointer/version stays.
	for step := 1; step <= 5; step++ {
		cameraX.Set(float32(step * 10))
		h.commit()
		if draws != 1 || layer.Version() != 1 {
			t.Fatalf("step %d re-rendered offscreen image: draws=%d version=%d", step, draws, layer.Version())
		}
		ops := h.find("host").canvas.drawing.Ops
		if len(ops) != 1 || ops[0].Image.Offscreen != firstSnap {
			t.Fatalf("step %d changed snapshot without Invalidate", step)
		}
	}

	// Multiple Invalidates in one batch coalesce into one re-record.
	color = RGB(0x00ff00)
	h.root.Batch(func() {
		layer.Invalidate()
		layer.Invalidate()
	})
	h.commit()
	if draws != 2 || layer.Version() != 2 {
		t.Fatalf("after Invalidate: draws=%d version=%d, want 2, 2", draws, layer.Version())
	}
	secondSnap := h.find("host").canvas.drawing.Ops[0].Image.Offscreen
	if secondSnap == nil || secondSnap.Version != 2 || secondSnap == firstSnap {
		t.Fatalf("expected new snapshot at version 2, got %+v", secondSnap)
	}

	// Resize with unchanged size and scale is a no-op.
	layer.Resize(Size{Width: 128, Height: 64}, 2)
	h.commit()
	if draws != 2 || layer.Version() != 2 {
		t.Fatalf("no-op Resize re-rendered: draws=%d version=%d", draws, layer.Version())
	}

	// Resize with new size re-renders and bumps version.
	layer.Resize(Size{Width: 256, Height: 128}, 1.5)
	h.commit()
	if draws != 3 || layer.Version() != 3 {
		t.Fatalf("after Resize: draws=%d version=%d, want 3, 3", draws, layer.Version())
	}
	if layer.Size() != (Size{Width: 256, Height: 128}) || layer.Scale() != 1.5 {
		t.Fatalf("size/scale = %v / %v", layer.Size(), layer.Scale())
	}

	// Release emits ItemReleaseOffscreen once and clears Source.
	layer.Release()
	layer.Release() // idempotent
	batch := h.commit()
	var releases []uint64
	for _, it := range batch.Items {
		if it.Kind == driver.ItemReleaseOffscreen {
			releases = append(releases, it.OffscreenID)
		}
	}
	if len(releases) != 1 || releases[0] != layer.ID() {
		t.Fatalf("releases = %v, want [%d]", releases, layer.ID())
	}
	if !layer.Released() || !layer.Source().Empty() {
		t.Fatal("released OffscreenImage still returned a non-empty Source")
	}
}

func TestOffscreenImageDefaultScaleAndImageSize(t *testing.T) {
	var (
		scope *Scope
		layer *OffscreenImage
	)
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		scope = cx
		return View{}
	})
	h.root.Env().ratio.Set(2)
	h.commit()

	layer = scope.NewOffscreenImage(Size{Width: 80, Height: 40}, 0, func(d *Drawing) {
		d.FillRect(0, 0, 80, 40)
	})
	if layer.Scale() != 2 {
		t.Fatalf("default scale = %v, want display pixel ratio 2", layer.Scale())
	}

	var got Size
	var gotErr error
	called := false
	scope.ImageSize(layer.Source(), func(pixels Size, err error) {
		called = true
		got, gotErr = pixels, err
	})
	if !called || gotErr != nil || got != (Size{Width: 160, Height: 80}) {
		t.Fatalf("ImageSize = (%v, %v, called=%v), want (160x80, nil, true)", got, gotErr, called)
	}
}

func TestOffscreenImageScopeDisposalAndImageView(t *testing.T) {
	var (
		shown   *Signal[bool]
		layer   *OffscreenImage
		loadEvt bool
	)
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		shown = NewSignal(cx, true)
		return Show{
			When: shown.Get,
			Then: func(cx *Scope) Node {
				layer = cx.NewOffscreenImage(Size{Width: 64, Height: 32}, 1, func(d *Drawing) {
					d.FillRect(0, 0, 64, 32)
				})
				return Image{
					TestID:      "img",
					SourceFunc:  layer.Source,
					Placeholder: ImageSource{URI: "https://example.invalid/ph.png"},
					OnLoad:      func(Size) { loadEvt = true },
				}
			},
		}
	})
	batch := h.commit()
	imgNode := h.find("img")
	if imgNode == nil {
		t.Fatal("missing image node")
	}
	var sent *driver.ImageConfig
	for _, it := range batch.Items {
		if it.Kind == driver.ItemImage && it.ID == imgNode.id {
			sent = it.Image
		}
	}
	if sent == nil || sent.Source.Offscreen == nil || !sent.Placeholder.Empty() {
		t.Fatalf("image item = %+v, want Offscreen source and empty placeholder", sent)
	}
	// Load events must be dropped for offscreen sources, as for Pixels.
	h.tree.Dispatch(driver.Event{Kind: driver.EventLoad, ID: imgNode.id, Size: Size{Width: 64, Height: 32}})
	if loadEvt {
		t.Fatal("offscreen Image fired OnLoad")
	}

	// Disposing the Show scope automatically releases the OffscreenImage.
	shown.Set(false)
	batch = h.commit()
	if !layer.Released() {
		t.Fatal("OffscreenImage was not released when its Scope disposed")
	}
	foundRelease := false
	for _, it := range batch.Items {
		if it.Kind == driver.ItemReleaseOffscreen && it.OffscreenID == layer.ID() {
			foundRelease = true
		}
	}
	if !foundRelease {
		t.Fatal("scope disposal did not emit ItemReleaseOffscreen")
	}
}

func TestOffscreenImageFontsChangedAndPixelsCopy(t *testing.T) {
	var (
		layer *OffscreenImage
		draws int
	)
	buf := image.NewRGBA(image.Rect(0, 0, 4, 4))
	buf.Pix[0] = 42
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		layer = cx.NewOffscreenImage(Size{Width: 50, Height: 50}, 1, func(d *Drawing) {
			draws++
			d.DrawImage(ImageSource{Pixels: buf, Version: 1}, 0, 0)
		})
		return Canvas{
			TestID: "c",
			Style:  Style{Width: Pt(50), Height: Pt(50)},
			Draw: func(d *Drawing, _ Size) {
				d.DrawImage(layer.Source(), 0, 0)
			},
		}
	})
	h.commit()
	if draws != 1 {
		t.Fatalf("draws = %d, want 1", draws)
	}
	copied := h.find("c").canvas.drawing.Ops[0].Image.Offscreen.Drawing.Ops[0].Image.Pixels
	if copied == nil || copied == buf || copied.Pix[0] != 42 {
		t.Fatal("offscreen drawing did not copy Pixels buffer")
	}
	// Mutating caller's buffer must not affect the copied pixels.
	buf.Pix[0] = 99
	if copied.Pix[0] != 42 {
		t.Fatal("mutating caller buffer affected offscreen snapshot")
	}

	// FontsChanged invalidates live offscreen images.
	h.tree.FontsChanged()
	h.commit()
	if draws != 2 || layer.Version() != 2 {
		t.Fatalf("after FontsChanged: draws=%d version=%d, want 2, 2", draws, layer.Version())
	}
}
