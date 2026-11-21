package nsgotest_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestOffscreenImageCachedTileLayerOneOpPerFrame(t *testing.T) {
	const (
		cols     = 16
		rows     = 16
		tileSize = 16
		layerW   = cols * tileSize // 256
		layerH   = rows * tileSize // 256
	)
	var (
		cameraX   *core.Signal[float32]
		tileLayer *core.OffscreenImage
		tileDraws int
		tint      = core.RGB(0x2563eb)
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cameraX = core.NewSignal(cx, float32(0))
		tileLayer = cx.NewOffscreenImage(core.Size{Width: layerW, Height: layerH}, 2, func(d *core.Drawing) {
			tileDraws++
			for y := range rows {
				for x := range cols {
					if (x+y)%2 == 0 {
						d.SetFillColor(tint)
					} else {
						d.SetFillColor(core.RGB(0x94a3b8))
					}
					d.FillRect(float32(x*tileSize), float32(y*tileSize), tileSize, tileSize)
				}
			}
		})
		return core.Canvas{
			TestID: "viewport",
			Style:  core.Style{Width: core.Pt(256), Height: core.Pt(256)},
			Draw: func(d *core.Drawing, _ core.Size) {
				d.DrawImageFrom(tileLayer.Source(), cameraX.Get(), 0, 128, 128, 0, 0, 256, 256)
			},
		}
	}, nsgotest.Size(256, 256))

	vp := app.Find("viewport")
	if vp == nil {
		t.Fatal("viewport canvas not found")
	}
	if tileDraws != 1 {
		t.Fatalf("tile layer draw count = %d, want 1", tileDraws)
	}
	surf := app.Offscreen(tileLayer)
	if surf == nil || surf.RenderCount != 1 || surf.Version != 1 {
		t.Fatalf("offscreen surface = %+v, want RenderCount=1 Version=1", surf)
	}
	if len(surf.Ops) < cols*rows {
		t.Fatalf("offscreen tile ops = %d, want >= %d", len(surf.Ops), cols*rows)
	}
	if len(vp.Ops) != 1 || vp.Ops[0].Kind != driver.DrawImage {
		t.Fatalf("viewport ops = %d (%v), want strictly 1 DrawImage op", len(vp.Ops), vp.Ops)
	}

	// Pan the camera for 10 frames: each frame draws the cached layer in
	// strictly 1 DrawImage op with zero offscreen re-renders.
	for frame := 1; frame <= 10; frame++ {
		offset := float32(frame * 8)
		app.Post(func() { cameraX.Set(offset) })
		app.Settle()
		vp = app.Find("viewport")
		if len(vp.Ops) != 1 || vp.Ops[0].Kind != driver.DrawImage {
			t.Fatalf("frame %d: got %d ops, want strictly 1 DrawImage op", frame, len(vp.Ops))
		}
		if vp.Ops[0].X1 != offset {
			t.Fatalf("frame %d: DrawImageFrom sx = %v, want %v", frame, vp.Ops[0].X1, offset)
		}
		if tileDraws != 1 || app.Offscreen(tileLayer).RenderCount != 1 {
			t.Fatalf("frame %d: offscreen re-rendered without Invalidate (draws=%d renders=%d)",
				frame, tileDraws, app.Offscreen(tileLayer).RenderCount)
		}
	}

	// Explicit Invalidate triggers exactly one re-render.
	app.Post(func() {
		tint = core.RGB(0x16a34a)
		tileLayer.Invalidate()
	})
	app.Settle()
	if tileDraws != 2 || app.Offscreen(tileLayer).RenderCount != 2 || app.Offscreen(tileLayer).Version != 2 {
		t.Fatalf("after Invalidate: draws=%d surface=%+v, want 2", tileDraws, app.Offscreen(tileLayer))
	}

	// Further camera frames stay at RenderCount == 2.
	app.Post(func() { cameraX.Set(96) })
	app.Settle()
	if tileDraws != 2 || app.Offscreen(tileLayer).RenderCount != 2 {
		t.Fatalf("after subsequent frame: draws=%d renders=%d, want 2, 2", tileDraws, app.Offscreen(tileLayer).RenderCount)
	}

	// Resize triggers exactly one re-render with updated dimensions.
	app.Post(func() { tileLayer.Resize(core.Size{Width: 512, Height: 512}, 1) })
	app.Settle()
	surf = app.Offscreen(tileLayer)
	if tileDraws != 3 || surf.RenderCount != 3 || surf.Size != (core.Size{Width: 512, Height: 512}) || surf.Scale != 1 {
		t.Fatalf("after Resize: draws=%d surface=%+v", tileDraws, surf)
	}
}

func TestOffscreenImageInImageViewAndScopeCleanup(t *testing.T) {
	var (
		shown *core.Signal[bool]
		badge *core.OffscreenImage
		draws int
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		shown = core.NewSignal(cx, true)
		return core.Show{
			When: shown.Get,
			Then: func(cx *core.Scope) core.Node {
				badge = cx.NewOffscreenImage(core.Size{Width: 48, Height: 24}, 2, func(d *core.Drawing) {
					draws++
					d.SetFillColor(core.RGB(0xf59e0b)).FillRect(0, 0, 48, 24)
				})
				return core.Image{
					TestID:     "badge",
					SourceFunc: badge.Source,
					Style:      core.Style{Width: core.Pt(48), Height: core.Pt(24)},
				}
			},
		}
	}, nsgotest.Size(200, 100))

	img := app.Find("badge")
	if img == nil || img.Image.Source.Offscreen == nil {
		t.Fatal("image view does not hold offscreen source")
	}
	surf := app.Offscreen(badge)
	if surf == nil || surf.RenderCount != 1 || surf.Released || draws != 1 {
		t.Fatalf("initial badge surface = %+v (draws=%d)", surf, draws)
	}

	// Disposing the Show branch releases the offscreen image automatically.
	app.Post(func() { shown.Set(false) })
	app.Settle()
	surf = app.Offscreen(badge)
	if surf == nil || !surf.Released || !badge.Released() {
		t.Fatalf("after unmount: surface=%+v released=%v, want Released=true", surf, badge.Released())
	}
}

func BenchmarkOffscreenTileLayerFrame(b *testing.B) {
	const (
		cols     = 16
		rows     = 16
		tileSize = 16
	)
	var (
		cameraX   *core.Signal[float32]
		tileLayer *core.OffscreenImage
	)
	app := nsgotest.Start(b, func(cx *core.Scope) core.Node {
		cameraX = core.NewSignal(cx, float32(0))
		tileLayer = cx.NewOffscreenImage(core.Size{Width: cols * tileSize, Height: rows * tileSize}, 1, func(d *core.Drawing) {
			for y := range rows {
				for x := range cols {
					d.SetFillColor(core.RGB(0x3b82f6)).FillRect(float32(x*tileSize), float32(y*tileSize), tileSize, tileSize)
				}
			}
		})
		return core.Canvas{
			TestID: "viewport",
			Style:  core.Style{Width: core.Pt(256), Height: core.Pt(256)},
			Draw: func(d *core.Drawing, _ core.Size) {
				d.DrawImage(tileLayer.Source(), -cameraX.Get(), 0)
			},
		}
	}, nsgotest.Size(256, 256))

	b.ResetTimer()
	for i := range b.N {
		app.Post(func() { cameraX.Set(float32(i % 128)) })
		app.Settle()
	}
	if got := app.Offscreen(tileLayer).RenderCount; got != 1 {
		b.Fatalf("RenderCount = %d, want 1", got)
	}
}
