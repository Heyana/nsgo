package gpu_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

const vec3UniformAndStorageWGSL = `
struct Packet {
	d: vec3<f32>,
	s: f32,
}

struct Padded {
	lead: f32,
	v: vec3<f32>,
	after: f32,
}

@group(0) @binding(0) var<uniform> u: Packet;
@group(0) @binding(1) var<uniform> pad: Padded;
@group(0) @binding(2) var<storage, read> items: array<Packet>;

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
}

@vertex
fn vs_main(@location(0) pos: vec2<f32>) -> VertexOut {
	var out: VertexOut;
	out.pos = vec4<f32>(pos, 0.5, 1.0);
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	// Left half reads { u.d.x, u.d.y, u.s, 1.0 }: u.s sits at byte offset 12, right after vec3 d!
	if (in.pos.x < 16.0) {
		return vec4<f32>(u.d.x, u.d.y, u.s, 1.0);
	}
	// Right half reads storage array element 1 (stride 16) + Padded.v (offset 16) and Padded.after (offset 28).
	let item = items[1u];
	return vec4<f32>(item.d.z + pad.lead, item.s, pad.v.y + pad.after, 1.0);
}
`

// TestVec3FollowedByScalarInUniformAndStorageBuffers verifies that WGSL structs with vec3<f32> followed by f32 (and leading/trailing padding, plus storage buffer arrays of {vec3<f32>, f32}) pack identically across the headless software GPU and native Metal.
func TestVec3FollowedByScalarInUniformAndStorageBuffers(t *testing.T) {
	mod := gpu.MustCompileWGSL(vec3UniformAndStorageWGSL)
	quad := packFloat32s(
		-1, -1,
		1, -1,
		1, 1,
		-1, -1,
		1, 1,
		-1, 1,
	)
	// Packet{ d: (0.2, 0.4, 0.0), s: 0.8 } -> 16 bytes, s at offset 12.
	uData := packFloat32s(0.2, 0.4, 0.0, 0.8)
	// Padded{ lead: 0.2, [12 pad bytes], v: (0.0, 0.6, 0.0), after: 0.2 } -> 32 bytes.
	padData := packFloat32s(
		0.2, 0.0, 0.0, 0.0,
		0.0, 0.6, 0.0, 0.2,
	)
	// Storage array<Packet> of 2 elements (stride 16 = 32 bytes):
	// [0]: unused sentinel
	// [1]: d=(0.0, 0.0, 0.4), s=0.6
	storageData := packFloat32s(
		0.9, 0.9, 0.9, 0.9,
		0.0, 0.0, 0.4, 0.6,
	)

	var (
		vb   *gpu.Buffer
		pipe *gpu.Pipeline
		bg   *gpu.BindGroup
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return gpu.View(cx, gpu.ViewProps{
			TestID: "vec3-layout",
			Style:  core.Style{Width: core.Pt(32), Height: core.Pt(16)},
			OnReady: func(dev *gpu.Device) {
				if !dev.Capabilities().Features.StorageBuffers {
					t.Skip("device does not support read-only storage buffers")
				}
				var err error
				vb, err = dev.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: quad})
				if err != nil {
					t.Fatalf("CreateBuffer quad: %v", err)
				}
				ub, err := dev.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: uData})
				if err != nil {
					t.Fatalf("CreateBuffer u: %v", err)
				}
				pb, err := dev.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: padData})
				if err != nil {
					t.Fatalf("CreateBuffer pad: %v", err)
				}
				sb, err := dev.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.BufferUsageStorage, Data: storageData})
				if err != nil {
					t.Fatalf("CreateBuffer storage: %v", err)
				}
				bgl, err := dev.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
					{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingUniform},
					{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingUniform},
					{Binding: 2, Visibility: gpu.StageFragment, Type: gpu.BindingStorageRead},
				})
				if err != nil {
					t.Fatalf("CreateBindGroupLayout: %v", err)
				}
				bg, err = dev.CreateBindGroup(cx, bgl, []gpu.BindGroupEntry{
					{Binding: 0, Buffer: ub},
					{Binding: 1, Buffer: pb},
					{Binding: 2, Buffer: sb},
				})
				if err != nil {
					t.Fatalf("CreateBindGroup: %v", err)
				}
				pipe, err = dev.CreatePipeline(cx, gpu.PipelineDesc{
					Label:    "vec3-layout",
					Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
					Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
					Layouts:  []*gpu.BindGroupLayout{bgl},
					VertexBuffers: []gpu.VertexBufferLayout{{
						Stride:     2 * 4,
						Attributes: []gpu.VertexAttribute{{Location: 0, Offset: 0, Format: gpu.VertexFloat32x2}},
					}},
					Targets: []gpu.ColorTargetState{{Format: dev.SurfaceFormat()}},
				})
				if err != nil {
					t.Fatalf("CreatePipeline: %v", err)
				}
			},
			OnFrame: func(frame *gpu.Frame) {
				pass := frame.BeginRenderPass(gpu.RenderPassDesc{})
				pass.ClearColor(core.RGB(0x000000))
				pass.SetPipeline(pipe)
				pass.SetBindGroup(0, bg)
				pass.SetVertexBuffer(0, vb, 0)
				pass.Draw(6, 0)
				pass.End()
				frame.Submit()
			},
		})
	}, nsgotest.Size(32, 16), nsgotest.ManualFrames())

	app.Advance(16 * time.Millisecond)

	img, err := app.GPUReadPixels("id:vec3-layout")
	if err != nil {
		t.Fatalf("GPUReadPixels: %v", err)
	}
	left := img.RGBAAt(8, 8)
	// (0.2, 0.4, 0.8, 1.0) -> ~(51, 102, 204, 255)
	if !closeByte(left.R, 51) || !closeByte(left.G, 102) || !closeByte(left.B, 204) || left.A != 255 {
		t.Errorf("left pixel = %+v, want ~(51, 102, 204, 255)", left)
	}
	right := img.RGBAAt(24, 8)
	// (0.4+0.2=0.6, 0.6, 0.6+0.2=0.8, 1.0) -> ~(153, 153, 204, 255)
	if !closeByte(right.R, 153) || !closeByte(right.G, 153) || !closeByte(right.B, 204) || right.A != 255 {
		t.Errorf("right pixel = %+v, want ~(153, 153, 204, 255)", right)
	}
}
