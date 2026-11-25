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
