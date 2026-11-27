package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type spyFeatures struct {
	ApproxTextEngine
	shaders, canvas, video, web, native, gpu, gpuSubmit int
}

func (s *spyFeatures) EnableShaders()            { s.shaders++ }
func (s *spyFeatures) EnableCanvas()             { s.canvas++ }
func (s *spyFeatures) EnableVideo()              { s.video++ }
func (s *spyFeatures) EnableWebView()            { s.web++ }
func (s *spyFeatures) EnableNative()             { s.native++ }
func (s *spyFeatures) EnableGPU()                { s.gpu++ }
func (s *spyFeatures) SubmitGPU(driver.GPUFrame) { s.gpuSubmit++ }

type wrappedFeatures struct {
	driver.TextEngine
}

func (w wrappedFeatures) UnwrapPlatform() any { return w.TextEngine }

func TestFeatureEnablersCalledOnDemand(t *testing.T) {
	run := func(t *testing.T, wrap bool, build func(*Scope) Node) *spyFeatures {
		t.Helper()
		spy := &spyFeatures{}
		var te driver.TextEngine = spy
		if wrap {
			te = wrappedFeatures{spy}
		}
		root := testRoot(RootOptions{})
		defer root.Dispose()
		tree := NewTree(root, te)
		tree.Resize(Size{Width: 320, Height: 240}, Geometry{})
		tree.Mount(build)
		tree.Commit()
		return spy
	}

	t.Run("minimal enables none", func(t *testing.T) {
		spy := run(t, true, func(*Scope) Node { return Text{Content: "hi"} })
		if spy.shaders+spy.canvas+spy.video+spy.web+spy.native+spy.gpu+spy.gpuSubmit != 0 {
			t.Fatalf("enabled = %+v, want all zero", spy)
		}
	})

	t.Run("shader", func(t *testing.T) {
		prog := MustCompileShader("package shader\nfunc Fragment(pos vec4) vec4 { return vec4(1, 0, 0, 1) }\n")
		spy := run(t, true, func(*Scope) Node { return Shader{Program: prog} })
		if spy.shaders == 0 || spy.canvas != 0 || spy.video != 0 || spy.web != 0 || spy.native != 0 {
			t.Fatalf("enabled = %+v, want shaders only", spy)
		}
	})

	t.Run("canvas", func(t *testing.T) {
		spy := run(t, true, func(*Scope) Node {
			return Canvas{Draw: func(d *Drawing, _ Size) { d.FillRect(0, 0, 10, 10) }}
		})
		if spy.canvas == 0 || spy.shaders != 0 || spy.video != 0 || spy.web != 0 || spy.native != 0 {
			t.Fatalf("enabled = %+v, want canvas only", spy)
		}
	})

	t.Run("offscreen image enables canvas", func(t *testing.T) {
		spy := run(t, true, func(cx *Scope) Node {
			img := cx.NewOffscreenImage(Size{Width: 10, Height: 10}, 1, func(d *Drawing) { d.FillRect(0, 0, 10, 10) })
			return Image{Source: img.Source()}
		})
		if spy.canvas == 0 {
			t.Fatalf("enabled = %+v, want canvas enabled for offscreen image", spy)
		}
	})

	t.Run("video", func(t *testing.T) {
		spy := run(t, true, func(*Scope) Node { return Video{Source: "https://example.test/v.mp4"} })
		if spy.video == 0 || spy.shaders != 0 || spy.canvas != 0 || spy.web != 0 || spy.native != 0 {
			t.Fatalf("enabled = %+v, want video only", spy)
		}
	})

	t.Run("webview", func(t *testing.T) {
		spy := run(t, true, func(*Scope) Node { return WebView{Source: WebSource{HTML: "<p>x</p>"}} })
		if spy.web == 0 || spy.shaders != 0 || spy.canvas != 0 || spy.video != 0 || spy.native != 0 {
			t.Fatalf("enabled = %+v, want web only", spy)
		}
	})

	t.Run("gpu submitter connects on mount", func(t *testing.T) {
		spy := &spyFeatures{}
		var text driver.TextEngine = spy
		root := testRoot(RootOptions{})
		defer root.Dispose()
		tree := NewTree(root, text)
		tree.Resize(Size{Width: 100, Height: 100}, Geometry{})
		tree.Mount(func(*Scope) Node { return GPUView{} })
		tree.Commit()
		if tree.gpuSubmit == nil {
			t.Fatal("the GPU submitter did not connect when a GPU view mounted")
		}
		tree.gpuSubmit(driver.GPUFrame{})
		if spy.gpuSubmit != 1 {
			t.Fatalf("gpuSubmit calls = %d, want 1", spy.gpuSubmit)
		}
	})

	t.Run("gpu submitter preserves existing hook", func(t *testing.T) {
		spy := &spyFeatures{}
		root := testRoot(RootOptions{})
		defer root.Dispose()
		tree := NewTree(root, spy)
		custom := 0
		tree.SetGPUSubmitter(func(driver.GPUFrame) { custom++ })
		tree.Resize(Size{Width: 100, Height: 100}, Geometry{})
		tree.Mount(func(*Scope) Node { return GPUView{} })
		tree.Commit()
		tree.gpuSubmit(driver.GPUFrame{})
		if custom != 1 || spy.gpuSubmit != 0 {
			t.Fatalf("custom=%d spy=%d, want custom=1 spy=0", custom, spy.gpuSubmit)
		}
	})

	t.Run("gpu view", func(t *testing.T) {
		spy := run(t, true, func(*Scope) Node { return GPUView{} })
		if spy.gpu == 0 || spy.shaders != 0 || spy.canvas != 0 || spy.video != 0 || spy.web != 0 || spy.native != 0 {
			t.Fatalf("enabled = %+v, want the GPU enabler and submitter only", spy)
		}
	})

	t.Run("native", func(t *testing.T) {
		spy := run(t, false, func(*Scope) Node { return Native{Component: "custom"} })
		if spy.native == 0 || spy.shaders != 0 || spy.canvas != 0 || spy.video != 0 || spy.web != 0 {
			t.Fatalf("enabled = %+v, want native only", spy)
		}
	})
}
