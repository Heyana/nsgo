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

// A rejected bind skips only the draws whose pipeline reads that slot: an unused vertex slot or an unbound group the pipeline does not declare leaves the draw alone, and a rejected bind group the pipeline does declare blocks it until a valid bind (#2253).
func TestRejectedBindOnlySkipsDrawsThatNeedIt(t *testing.T) {
	sr := gpu.NewSoftwareRenderer(8, 8, gpu.FormatUndefined)
	dev := sr.Device()
	mod := gpu.MustCompileWGSL(directVertexColorWGSL)
	quad := packFloat32s(
		-1, -1, 0.5, 1, 0, 0, 1, 1,
		1, -1, 0.5, 1, 0, 0, 1, 1,
		1, 1, 0.5, 1, 0, 0, 1, 1,
		-1, -1, 0.5, 1, 0, 0, 1, 1,
		1, 1, 0.5, 1, 0, 0, 1, 1,
		-1, 1, 0.5, 1, 0, 0, 1, 1,
	)
	vertices, err := dev.CreateBuffer(nil, gpu.BufferDesc{Label: "vertices", Usage: gpu.UsageVertex, Data: quad})
	if err != nil {
		t.Fatalf("CreateBuffer vertices: %v", err)
	}
	spare, err := dev.CreateBuffer(nil, gpu.BufferDesc{Label: "spare", Usage: gpu.UsageVertex, Data: quad})
	if err != nil {
		t.Fatalf("CreateBuffer spare: %v", err)
	}
	index, err := dev.CreateBuffer(nil, gpu.BufferDesc{Label: "index", Usage: gpu.UsageIndex, Data: packUint16s(0, 1, 2, 3, 4, 5)})
	if err != nil {
		t.Fatalf("CreateBuffer index: %v", err)
	}
	layout, err := dev.CreateBindGroupLayout(nil, nil)
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	group, err := dev.CreateBindGroup(nil, layout, nil)
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	spareGroup, err := dev.CreateBindGroup(nil, layout, nil)
	if err != nil {
		t.Fatalf("CreateBindGroup spare: %v", err)
	}
	if err := spareGroup.Close(); err != nil {
		t.Fatal(err)
	}
	if err := spare.Close(); err != nil {
		t.Fatal(err)
	}
	pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Label:    "quad",
		Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{layout},
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
		p.SetVertexBuffer(0, vertices, 0)
		p.SetIndexBuffer(index, gpu.IndexUint16, 0)
		p.SetBindGroup(0, group)
		bind(p)
		p.DrawIndexed(6, 0, 0)
		p.End()
		dev.Submit(enc.Finish())
		if err := sr.LastError(); err != nil {
			t.Fatalf("SoftwareRenderer error: %v", err)
		}
	}
	blue := color.RGBA{B: 255, A: 255}
	clear := color.RGBA{A: 255}

	// An unused vertex slot and an undeclared group slot change nothing.
	render(func(p *gpu.RenderPass) {
		p.SetVertexBuffer(3, spare, 0)
		p.SetBindGroup(2, spareGroup)
	})
	if got := sr.Image().RGBAAt(4, 4); got != blue {
		t.Fatalf("draw with unused slots rejected = %v, want blue", got)
	}

	// A rejected bind group the pipeline declares skips the draw, and a valid bind brings it back.
	render(func(p *gpu.RenderPass) { p.SetBindGroup(0, spareGroup) })
	if got := sr.Image().RGBAAt(4, 4); got != clear {
		t.Fatalf("draw after a rejected bind group = %v, want the clear color", got)
	}
	render(func(p *gpu.RenderPass) { p.SetBindGroup(0, group) })
	if got := sr.Image().RGBAAt(4, 4); got != blue {
		t.Fatalf("draw after a valid bind group = %v, want blue", got)
	}
}

// A pipeline that declares no layouts still reads the groups its shaders bind, so a rejected bind group skips its draws (#2253).
func TestAutoLayoutPipelineTracksItsShaderGroups(t *testing.T) {
	const shader = `
@group(0) @binding(0) var<uniform> params: vec4<f32>;

struct VertexIn {
	@location(0) pos: vec4<f32>,
	@location(1) color: vec4<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = in.pos;
	out.color = in.color;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	return in.color + params * 0.0;
}
`
	sr := gpu.NewSoftwareRenderer(8, 8, gpu.FormatUndefined)
	dev := sr.Device()
	mod := gpu.MustCompileWGSL(shader)
	vertices, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: packFloat32s(
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
	layout, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingUniform}})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	uniform, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: packFloat32s(0, 0, 0, 0)})
	if err != nil {
		t.Fatalf("CreateBuffer uniform: %v", err)
	}
	group, err := dev.CreateBindGroup(nil, layout, []gpu.BindGroupEntry{{Binding: 0, Buffer: uniform}})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	closedGroup, err := dev.CreateBindGroup(nil, layout, []gpu.BindGroupEntry{{Binding: 0, Buffer: uniform}})
	if err != nil {
		t.Fatalf("CreateBindGroup spare: %v", err)
	}
	if err := closedGroup.Close(); err != nil {
		t.Fatal(err)
	}
	pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
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
		p.SetVertexBuffer(0, vertices, 0)
		p.SetBindGroup(0, group)
		bind(p)
		p.Draw(6, 0)
		p.End()
		dev.Submit(enc.Finish())
		if err := sr.LastError(); err != nil {
			t.Fatalf("SoftwareRenderer error: %v", err)
		}
	}
	render(func(p *gpu.RenderPass) {})
	if got := sr.Image().RGBAAt(4, 4); got != (color.RGBA{B: 255, A: 255}) {
		t.Fatalf("draw with the group bound = %v, want blue", got)
	}
	render(func(p *gpu.RenderPass) { p.SetBindGroup(0, closedGroup) })
	if got := sr.Image().RGBAAt(4, 4); got != (color.RGBA{A: 255}) {
		t.Fatalf("draw after a rejected bind group on an auto layout = %v, want the clear color", got)
	}
	render(func(p *gpu.RenderPass) { p.SetBindGroup(0, group) })
	if got := sr.Image().RGBAAt(4, 4); got != (color.RGBA{B: 255, A: 255}) {
		t.Fatalf("draw after a valid bind group = %v, want blue", got)
	}
}
