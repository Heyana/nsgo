package gpu_test

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
)

const sparseRenderWGSL = `
@group(0) @binding(0) var<uniform> frameColor: vec4<f32>;
@group(3) @binding(0) var<uniform> objectTint: vec4<f32>;

struct VertexIn {
	@location(0) pos: vec4<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> @builtin(position) vec4<f32> {
	return in.pos;
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
	return frameColor * objectTint;
}
`

const sparseComputeWGSL = `
@group(0) @binding(0) var<storage, read> input: array<u32>;
@group(3) @binding(0) var<storage, read_write> out: array<u32>;

@compute @workgroup_size(1)
fn main() {
	out[0] = input[0] + 42u;
}
`

func assertInvalidDescriptorWithGroup(t *testing.T, err error, group string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected ErrInvalidDescriptor naming group %s, got nil", group)
	}
	var ge gpu.Error
	if !errors.As(err, &ge) || ge.Code != gpu.ErrInvalidDescriptor {
		t.Fatalf("expected ErrInvalidDescriptor, got %v", err)
	}
	if !strings.Contains(ge.Message, group) {
		t.Fatalf("expected error message to name group %s, got %q", group, ge.Message)
	}
}

func TestPipelineNilBindGroupLayoutSlotsRenderAndCompute(t *testing.T) {
	sr := gpu.NewSoftwareRenderer(4, 4, gpu.FormatUndefined)
	dev := sr.Device()

	frameBGL, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingUniform},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout frame: %v", err)
	}
	objectBGL, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingUniform},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout object: %v", err)
	}

	frameBuf, err := dev.CreateBuffer(nil, gpu.BufferDesc{
		Usage: gpu.UsageUniform,
		Data:  packFloat32s(1, 0.5, 0, 1),
	})
	if err != nil {
		t.Fatalf("CreateBuffer frame: %v", err)
	}
	objectBuf, err := dev.CreateBuffer(nil, gpu.BufferDesc{
		Usage: gpu.UsageUniform,
		Data:  packFloat32s(0.5, 1, 1, 1),
	})
	if err != nil {
		t.Fatalf("CreateBuffer object: %v", err)
	}
	frameBG, err := dev.CreateBindGroup(nil, frameBGL, []gpu.BindGroupEntry{{Binding: 0, Buffer: frameBuf}})
	if err != nil {
		t.Fatalf("CreateBindGroup frame: %v", err)
	}
	objectBG, err := dev.CreateBindGroup(nil, objectBGL, []gpu.BindGroupEntry{{Binding: 0, Buffer: objectBuf}})
	if err != nil {
		t.Fatalf("CreateBindGroup object: %v", err)
	}

	verts, err := dev.CreateBuffer(nil, gpu.BufferDesc{
		Usage: gpu.UsageVertex,
		Data: packFloat32s(
			-1, -1, 0, 1,
			3, -1, 0, 1,
			-1, 3, 0, 1,
		),
	})
	if err != nil {
		t.Fatalf("CreateBuffer verts: %v", err)
	}

	renderMod := gpu.MustCompileWGSL(sparseRenderWGSL)
	pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Label:    "sparse-render",
		Vertex:   gpu.ShaderStage{Module: renderMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: renderMod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{frameBGL, nil, nil, objectBGL},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride: 16,
			Attributes: []gpu.VertexAttribute{
				{Format: gpu.VertexFloat32x4, Offset: 0, Location: 0},
			},
		}},
		Targets: []gpu.ColorTargetState{{Format: dev.SurfaceFormat()}},
	})
	if err != nil {
		t.Fatalf("CreatePipeline with nil slots: %v", err)
	}

	// Render without calling SetBindGroup for nil slots 1 and 2.
	enc := dev.BeginEncoder()
	rp := enc.BeginRenderPass(gpu.RenderPassDesc{
		ColorAttachments: []gpu.ColorAttachment{{
			LoadOp:     gpu.LoadClear,
			StoreOp:    gpu.StoreStore,
			ClearColor: core.RGBA(0x000000, 1),
		}},
	})
	rp.SetPipeline(pipe)
	rp.SetVertexBuffer(0, verts, 0)
	rp.SetBindGroup(0, frameBG)
	rp.SetBindGroup(3, objectBG)
	// A rejected bind on a nil layout slot must not block the draw: it is the pass's only draw, so a skipped draw leaves the clear colour and fails the pixel check below.
	rp.SetBindGroup(1, nil)
	rp.Draw(3, 0)
	rp.End()
	dev.Submit(enc.Finish())
	if err := sr.LastError(); err != nil {
		t.Fatalf("SoftwareRenderer render error: %v", err)
	}

	gotPx := sr.Image().RGBAAt(2, 2)
	// Expected RGBA: (0.5, 0.5, 0, 1) -> ~(128, 128, 0, 255).
	if gotPx.R < 120 || gotPx.R > 135 || gotPx.G < 120 || gotPx.G > 135 || gotPx.B > 5 || gotPx.A < 250 {
		t.Fatalf("rendered pixel = %v, want ~(128, 128, 0, 255)", gotPx)
	}

	// Compute pipeline with nil layout slots 1 and 2.
	inBGL, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageCompute, Type: gpu.BindingTypeReadOnlyStorageBuffer},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout in: %v", err)
	}
	outBGL, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageCompute, Type: gpu.BindingTypeStorageBuffer},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout out: %v", err)
	}
	inBuf, err := dev.CreateBuffer(nil, gpu.BufferDesc{
		Usage: gpu.BufferUsageStorage,
		Data:  packUint32s(100),
	})
	if err != nil {
		t.Fatalf("CreateBuffer in: %v", err)
	}
	outBuf, err := dev.CreateBuffer(nil, gpu.BufferDesc{
		Usage: gpu.BufferUsageStorage | gpu.UsageCopySrc,
		Data:  packUint32s(0),
	})
	if err != nil {
		t.Fatalf("CreateBuffer out: %v", err)
	}
	inBG, err := dev.CreateBindGroup(nil, inBGL, []gpu.BindGroupEntry{{Binding: 0, Buffer: inBuf}})
	if err != nil {
		t.Fatalf("CreateBindGroup in: %v", err)
	}
	outBG, err := dev.CreateBindGroup(nil, outBGL, []gpu.BindGroupEntry{{Binding: 0, Buffer: outBuf}})
	if err != nil {
		t.Fatalf("CreateBindGroup out: %v", err)
	}

	computeMod := gpu.MustCompileWGSL(sparseComputeWGSL)
	cpipe, err := dev.CreateComputePipeline(nil, gpu.ComputePipelineDesc{
		Label:   "sparse-compute",
		Compute: gpu.ShaderStage{Module: computeMod, EntryPoint: "main"},
		Layouts: []*gpu.BindGroupLayout{inBGL, nil, nil, outBGL},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline with nil slots: %v", err)
	}

	cenc := dev.BeginEncoder()
	cp := cenc.BeginComputePass()
	cp.SetPipeline(cpipe)
	cp.SetBindGroup(0, inBG)
	cp.SetBindGroup(3, outBG)
	if err := cp.Dispatch(1, 1, 1); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	cp.End()
	dev.Submit(cenc.Finish())
	if err := sr.LastError(); err != nil {
		t.Fatalf("SoftwareRenderer compute error: %v", err)
	}

	var got uint32
	dev.ReadBuffer(outBuf, 0, 4, func(data []byte, err error) {
		if err != nil {
			t.Fatalf("ReadBuffer: %v", err)
		}
		if len(data) >= 4 {
			got = binary.LittleEndian.Uint32(data)
		}
	})
	if got != 142 {
		t.Fatalf("compute output = %d, want 142", got)
	}
}

func TestPipelineNilBindGroupLayoutValidation(t *testing.T) {
	sr := gpu.NewSoftwareRenderer(4, 4, gpu.FormatUndefined)
	dev := sr.Device()

	otherSR := gpu.NewSoftwareRenderer(4, 4, gpu.FormatUndefined)
	otherDev := otherSR.Device()

	bgl0, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingUniform},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout bgl0: %v", err)
	}
	bgl3, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingUniform},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout bgl3: %v", err)
	}

	renderMod := gpu.MustCompileWGSL(sparseRenderWGSL)

	// Shader uses @group(3), but Layouts[3] is nil -> ErrInvalidDescriptor naming group 3.
	_, err = dev.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:   gpu.ShaderStage{Module: renderMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: renderMod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{bgl0, nil, nil, nil},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride:     16,
			Attributes: []gpu.VertexAttribute{{Format: gpu.VertexFloat32x4, Offset: 0, Location: 0}},
		}},
		Targets: []gpu.ColorTargetState{{Format: dev.SurfaceFormat()}},
	})
	assertInvalidDescriptorWithGroup(t, err, "3")

	// Compute shader uses @group(3), but Layouts[3] is nil -> ErrInvalidDescriptor naming group 3.
	computeMod := gpu.MustCompileWGSL(sparseComputeWGSL)
	_, err = dev.CreateComputePipeline(nil, gpu.ComputePipelineDesc{
		Compute: gpu.ShaderStage{Module: computeMod, EntryPoint: "main"},
		Layouts: []*gpu.BindGroupLayout{bgl0, nil, nil, nil},
	})
	assertInvalidDescriptorWithGroup(t, err, "3")

	// Foreign-device layout alongside nil slots is still refused.
	foreignBGL, err := otherDev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingUniform},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout foreign: %v", err)
	}
	_, err = dev.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:   gpu.ShaderStage{Module: renderMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: renderMod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{bgl0, nil, nil, foreignBGL},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride:     16,
			Attributes: []gpu.VertexAttribute{{Format: gpu.VertexFloat32x4, Offset: 0, Location: 0}},
		}},
		Targets: []gpu.ColorTargetState{{Format: dev.SurfaceFormat()}},
	})
	var ge gpu.Error
	if !errors.As(err, &ge) || ge.Code != gpu.ErrInvalidDescriptor {
		t.Fatalf("expected ErrInvalidDescriptor for foreign layout, got %v", err)
	}

	_, err = dev.CreateComputePipeline(nil, gpu.ComputePipelineDesc{
		Compute: gpu.ShaderStage{Module: computeMod, EntryPoint: "main"},
		Layouts: []*gpu.BindGroupLayout{bgl0, nil, nil, foreignBGL},
	})
	if !errors.As(err, &ge) || ge.Code != gpu.ErrInvalidDescriptor {
		t.Fatalf("expected ErrInvalidDescriptor for foreign compute layout, got %v", err)
	}

	// Closed layout alongside nil slots is still refused.
	if err := bgl3.Close(); err != nil {
		t.Fatalf("Close bgl3: %v", err)
	}
	_, err = dev.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:   gpu.ShaderStage{Module: renderMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: renderMod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{bgl0, nil, nil, bgl3},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride:     16,
			Attributes: []gpu.VertexAttribute{{Format: gpu.VertexFloat32x4, Offset: 0, Location: 0}},
		}},
		Targets: []gpu.ColorTargetState{{Format: dev.SurfaceFormat()}},
	})
	if !errors.As(err, &ge) || ge.Code != gpu.ErrInvalidDescriptor {
		t.Fatalf("expected ErrInvalidDescriptor for closed layout, got %v", err)
	}

	_, err = dev.CreateComputePipeline(nil, gpu.ComputePipelineDesc{
		Compute: gpu.ShaderStage{Module: computeMod, EntryPoint: "main"},
		Layouts: []*gpu.BindGroupLayout{bgl0, nil, nil, bgl3},
	})
	if !errors.As(err, &ge) || ge.Code != gpu.ErrInvalidDescriptor {
		t.Fatalf("expected ErrInvalidDescriptor for closed compute layout, got %v", err)
	}
}
