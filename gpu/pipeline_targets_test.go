package gpu_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
)

// A pipeline whose colour target format does not match the pass's attachment records no draw, as the strictest backends refuse it (#2341).
func TestPipelineTargetMismatchSkipsTheDraw(t *testing.T) {
	sr := gpu.NewSoftwareRenderer(8, 8, gpu.FormatUndefined)
	dev := sr.Device()
	mod := gpu.MustCompileWGSL(directVertexColorWGSL)
	tex, err := dev.CreateTexture(nil, gpu.TextureDesc{Label: "target", Width: 8, Height: 8, Format: gpu.FormatRGBA8Unorm, Usage: gpu.TextureRenderAttachment})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	vertices, err := dev.CreateBuffer(nil, gpu.BufferDesc{Label: "quad", Usage: gpu.UsageVertex, Data: packFloat32s(
		-1, -1, 0.5, 1, 0, 0, 1, 1,
		1, -1, 0.5, 1, 0, 0, 1, 1,
		1, 1, 0.5, 1, 0, 0, 1, 1,
		-1, -1, 0.5, 1, 0, 0, 1, 1,
		1, 1, 0.5, 1, 0, 0, 1, 1,
		-1, 1, 0.5, 1, 0, 0, 1, 1,
	)})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	pipe := func(format gpu.TextureFormat) *gpu.Pipeline {
		t.Helper()
		p, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: []gpu.VertexBufferLayout{{
				Stride: 8 * 4,
				Attributes: []gpu.VertexAttribute{
					{Location: 0, Offset: 0, Format: gpu.VertexFloat32x4},
					{Location: 1, Offset: 4 * 4, Format: gpu.VertexFloat32x4},
				},
			}},
			Targets: []gpu.ColorTargetState{{Format: format}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline(%v): %v", format, err)
		}
		return p
	}
	draw := func(p *gpu.Pipeline) {
		t.Helper()
		enc := dev.BeginEncoder()
		pass := enc.BeginRenderPass(gpu.RenderPassDesc{ColorAttachments: []gpu.ColorAttachment{{Texture: tex}}})
		pass.ClearColor(core.RGB(0xff0000))
		pass.SetPipeline(p)
		pass.SetVertexBuffer(0, vertices, 0)
		pass.Draw(6, 0)
		pass.End()
		dev.Submit(enc.Finish())
		if err := sr.LastError(); err != nil {
			t.Fatalf("SoftwareRenderer error: %v", err)
		}
	}
	center := func() []byte {
		t.Helper()
		var got []byte
		dev.ReadTexture(tex, gpu.TextureRegion{Width: 8, Height: 8}, func(rgba []byte, err error) {
			if err != nil {
				t.Fatalf("ReadTexture: %v", err)
			}
			got = rgba
		})
		return got[(4*8+4)*4 : (4*8+4)*4+4]
	}

	// A target of another format is refused: the pass keeps its clear colour.
	draw(pipe(gpu.FormatRGBA16Float))
	if got := center(); got[0] != 255 || got[1] != 0 || got[2] != 0 {
		t.Fatalf("draw with a mismatched target = %v, want the red clear colour", got)
	}

	// A pipeline for the attachment's format draws.
	draw(pipe(gpu.FormatRGBA8Unorm))
	if got := center(); got[0] != 0 || got[1] != 0 || got[2] != 255 {
		t.Fatalf("draw with a matching target = %v, want blue", got)
	}
}

// A render pass begun from a nil encoder is invalid rather than a panic, as before the target check existed (#2341).
func TestNilEncoderRenderPassIsSafe(t *testing.T) {
	var enc *gpu.Encoder
	pass := enc.BeginRenderPass(gpu.RenderPassDesc{})
	if pass == nil {
		t.Fatal("a nil encoder returned no pass")
	}
	pass.ClearColor(core.RGB(0x000000))
	pass.Draw(3, 0)
	pass.End()
}

// A pipeline whose sample count or depth format does not fit the pass is refused, with the pipeline's zero sample count read as one (#2341).
func TestPipelineSampleAndDepthMismatchesSkipTheDraw(t *testing.T) {
	mod := gpu.MustCompileWGSL(directVertexColorWGSL)
	quad := packFloat32s(
		-1, -1, 0.5, 1, 0, 0, 1, 1,
		1, -1, 0.5, 1, 0, 0, 1, 1,
		1, 1, 0.5, 1, 0, 0, 1, 1,
		-1, -1, 0.5, 1, 0, 0, 1, 1,
		1, 1, 0.5, 1, 0, 0, 1, 1,
		-1, 1, 0.5, 1, 0, 0, 1, 1,
	)
	layout := []gpu.VertexBufferLayout{{
		Stride: 8 * 4,
		Attributes: []gpu.VertexAttribute{
			{Location: 0, Offset: 0, Format: gpu.VertexFloat32x4},
			{Location: 1, Offset: 4 * 4, Format: gpu.VertexFloat32x4},
		},
	}}
	t.Run("default pipeline on a 4-sample attachment", func(t *testing.T) {
		sr := gpu.NewSoftwareRenderer(8, 8, gpu.FormatUndefined)
		dev := sr.Device()
		msaa, err := dev.CreateTexture(nil, gpu.TextureDesc{Width: 8, Height: 8, SampleCount: 4, Format: gpu.FormatRGBA8Unorm, Usage: gpu.TextureRenderAttachment})
		if err != nil {
			t.Fatalf("CreateTexture msaa: %v", err)
		}
		resolve, err := dev.CreateTexture(nil, gpu.TextureDesc{Width: 8, Height: 8, Format: gpu.FormatRGBA8Unorm, Usage: gpu.TextureRenderAttachment | gpu.TextureCopySrc})
		if err != nil {
			t.Fatalf("CreateTexture resolve: %v", err)
		}
		vertices, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: quad})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: layout,
			Targets:       []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline: %v", err)
		}
		enc := dev.BeginEncoder()
		pass := enc.BeginRenderPass(gpu.RenderPassDesc{ColorAttachments: []gpu.ColorAttachment{{Texture: msaa, ResolveTarget: resolve}}})
		pass.ClearColor(core.RGB(0x000000))
		pass.SetPipeline(pipe)
		pass.SetVertexBuffer(0, vertices, 0)
		pass.Draw(6, 0)
		pass.End()
		dev.Submit(enc.Finish())
		var got []byte
		dev.ReadTexture(resolve, gpu.TextureRegion{Width: 8, Height: 8}, func(rgba []byte, err error) {
			if err != nil {
				t.Fatalf("ReadTexture: %v", err)
			}
			got = rgba
		})
		if px := got[(4*8+4)*4 : (4*8+4)*4+4]; px[2] != 0 {
			t.Fatalf("the single-sample pipeline drew into a 4-sample attachment: %v", px)
		}
	})
	t.Run("pipeline depth on a pass without one", func(t *testing.T) {
		sr := gpu.NewSoftwareRenderer(8, 8, gpu.FormatUndefined)
		dev := sr.Device()
		target, err := dev.CreateTexture(nil, gpu.TextureDesc{Width: 8, Height: 8, Format: gpu.FormatRGBA8Unorm, Usage: gpu.TextureRenderAttachment | gpu.TextureCopySrc})
		if err != nil {
			t.Fatalf("CreateTexture: %v", err)
		}
		vertices, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: quad})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: layout,
			DepthStencil:  &gpu.DepthStencilState{Format: gpu.FormatDepth24Plus, DepthWrite: true, DepthCompare: gpu.CompareLess},
			Targets:       []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline: %v", err)
		}
		enc := dev.BeginEncoder()
		pass := enc.BeginRenderPass(gpu.RenderPassDesc{ColorAttachments: []gpu.ColorAttachment{{Texture: target}}})
		pass.ClearColor(core.RGB(0x000000))
		pass.SetPipeline(pipe)
		pass.SetVertexBuffer(0, vertices, 0)
		pass.Draw(6, 0)
		pass.End()
		dev.Submit(enc.Finish())
		var got []byte
		dev.ReadTexture(target, gpu.TextureRegion{Width: 8, Height: 8}, func(rgba []byte, err error) {
			if err != nil {
				t.Fatalf("ReadTexture: %v", err)
			}
			got = rgba
		})
		if px := got[(4*8+4)*4 : (4*8+4)*4+4]; px[2] != 0 {
			t.Fatalf("a pipeline with depth drew into a pass without it: %v", px)
		}
	})
	t.Run("single-sample pipeline on a 4-sample swapchain", func(t *testing.T) {
		sr := gpu.NewSoftwareRendererWithConfig(8, 8, gpu.FormatRGBA8Unorm, gpu.FormatUndefined, 4)
		dev := sr.Device()
		vertices, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: quad})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: layout,
			Targets:       []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline: %v", err)
		}
		enc := dev.BeginEncoder()
		pass := enc.BeginRenderPass(gpu.RenderPassDesc{})
		pass.ClearColor(core.RGB(0x000000))
		pass.SetPipeline(pipe)
		pass.SetVertexBuffer(0, vertices, 0)
		pass.Draw(6, 0)
		pass.End()
		dev.Submit(enc.Finish())
		if got := sr.Image().RGBAAt(4, 4); got.B != 0 {
			t.Fatalf("a single-sample pipeline drew into a 4-sample swapchain: %v", got)
		}
	})
}
