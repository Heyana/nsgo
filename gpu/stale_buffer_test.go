package gpu_test

import (
	"image/color"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
)

// A rejected vertex or index buffer bind leaves its slot missing, so the next draw is skipped instead of drawing with the buffers bound before it (#2253).
func TestRejectedBufferBindsSkipTheDraw(t *testing.T) {
	sr := gpu.NewSoftwareRenderer(8, 8, gpu.FormatUndefined)
	dev := sr.Device()
	mod := gpu.MustCompileWGSL(directVertexColorWGSL)
	quad := func(r, g, b float32) []byte {
		return packFloat32s(
			-1, -1, 0.5, 1, r, g, b, 1,
			1, -1, 0.5, 1, r, g, b, 1,
			1, 1, 0.5, 1, r, g, b, 1,
			-1, -1, 0.5, 1, r, g, b, 1,
			1, 1, 0.5, 1, r, g, b, 1,
			-1, 1, 0.5, 1, r, g, b, 1,
		)
	}
	newBuffer := func(label string, usage gpu.BufferUsage, data []byte) *gpu.Buffer {
		t.Helper()
		buf, err := dev.CreateBuffer(nil, gpu.BufferDesc{Label: label, Usage: usage, Data: data})
		if err != nil {
			t.Fatalf("CreateBuffer %s: %v", label, err)
		}
		return buf
	}
	red := newBuffer("red", gpu.UsageVertex, quad(1, 0, 0))
	blue := newBuffer("blue", gpu.UsageVertex, quad(0, 0, 1))
	indices := packUint16s(0, 1, 2, 3, 4, 5)
	index := newBuffer("index", gpu.UsageIndex, indices)
	closedIndex := newBuffer("closed index", gpu.UsageIndex, indices)
	pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Label:    "quad",
		Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride: 8 * 4,
			Attributes: []gpu.VertexAttribute{
				{Location: 0, Offset: 0, Format: gpu.VertexFloat32x4},
				{Location: 1, Offset: 4 * 4, Format: gpu.VertexFloat32x4},
			},
		}},
		Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
	})
	if err != nil {
		t.Fatalf("CreatePipeline: %v", err)
	}
	render := func(bind func(p *gpu.RenderPass)) {
		t.Helper()
		enc := dev.BeginEncoder()
		p := enc.BeginRenderPass(gpu.RenderPassDesc{})
		p.ClearColor(core.RGB(0x000000))
		p.SetPipeline(pipe)
		bind(p)
		p.End()
		dev.Submit(enc.Finish())
		if err := sr.LastError(); err != nil {
			t.Fatalf("SoftwareRenderer error: %v", err)
		}
	}

	// The red quad draws, then a closed index buffer rejects its bind in the same pass and the blue draw is skipped, so the red quad stays.
	if err := closedIndex.Close(); err != nil {
		t.Fatal(err)
	}
	render(func(p *gpu.RenderPass) {
		p.SetVertexBuffer(0, red, 0)
		p.SetIndexBuffer(index, gpu.IndexUint16, 0)
		p.DrawIndexed(6, 0, 0)
		p.SetVertexBuffer(0, blue, 0)
		p.SetIndexBuffer(closedIndex, gpu.IndexUint16, 0)
		p.DrawIndexed(6, 0, 0)
	})
	if got := sr.Image().RGBAAt(4, 4); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("draw after a rejected index bind = %v, want the red quad alone", got)
	}

	// A valid bind to the slot lets drawing resume.
	render(func(p *gpu.RenderPass) {
		p.SetVertexBuffer(0, blue, 0)
		p.SetIndexBuffer(index, gpu.IndexUint16, 0)
		p.DrawIndexed(6, 0, 0)
	})
	if got := sr.Image().RGBAAt(4, 4); got != (color.RGBA{B: 255, A: 255}) {
		t.Fatalf("draw after a valid index bind = %v, want blue", got)
	}

	// A closed vertex buffer rejects its slot the same way.
	if err := blue.Close(); err != nil {
		t.Fatal(err)
	}
	render(func(p *gpu.RenderPass) {
		p.SetVertexBuffer(0, blue, 0)
		p.SetIndexBuffer(index, gpu.IndexUint16, 0)
		p.DrawIndexed(6, 0, 0)
	})
	if got := sr.Image().RGBAAt(4, 4); got != (color.RGBA{A: 255}) {
		t.Fatalf("draw after a rejected vertex bind = %v, want the clear color", got)
	}
}
