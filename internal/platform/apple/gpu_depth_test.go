//go:build darwin

package apple

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
)

const gpuDepthOnlyWGSL = `
struct VertexIn {
	@location(0) pos: vec3<f32>,
}

@vertex
fn vs_depth(in: VertexIn) -> @builtin(position) vec4<f32> {
	return vec4<f32>(in.pos, 1.0);
}
`

const gpuShadowProbeWGSL = `
@group(0) @binding(0) var shadowMap: texture_depth_2d;
@group(0) @binding(1) var shadowSamp: sampler_comparison;

struct VertexIn {
	@location(0) pos: vec3<f32>,
	@location(1) uv: vec2<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) uv: vec2<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = vec4<f32>(in.pos, 1.0);
	out.uv = in.uv;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	let vis = textureSampleCompareLevel(shadowMap, shadowSamp, in.uv, 0.6);
	let lit = 0.2 + 0.8 * vis;
	return vec4<f32>(lit, lit, lit, 1.0);
}
`

// TestMetalDepthOnlyPassFeedsAComparisonSampler renders a depth-only pass, as a shadow map's, into a sampled depth texture, samples it with a comparison sampler in a second pass, and checks that the depth-only pass left the view's surface alone. It runs in a child process with Metal's API validation, which aborts on an attachment the pipeline does not match; without it Metal draws such a pass anyway.
func TestMetalDepthOnlyPassFeedsAComparisonSampler(t *testing.T) {
	if os.Getenv("MTL_DEBUG_LAYER") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "MTL_DEBUG_LAYER=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	depthProg, err := wgsl.CompilePipeline(gpuDepthOnlyWGSL)
	if err != nil {
		t.Fatal(err)
	}
	probeProg, err := wgsl.CompilePipeline(gpuShadowProbeWGSL)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan *driver.GPUEvent, 32)
	var layer *GPULayer
	withPool(func() {
		layer = NewGPULayer(driver.GPUViewConfig{SampleCount: 1}, func(ev *driver.GPUEvent) { events <- ev })
		if layer != nil {
			layer.Resize(8, 8, 1)
		}
	})
	if layer == nil {
		t.Skip("Metal unavailable")
	}
	t.Cleanup(func() { withPool(layer.Release) })
	next := func() *driver.GPUEvent {
		t.Helper()
		for {
			select {
			case ev := <-events:
				if ev.Kind == driver.GPUEventError {
					t.Fatalf("GPU error %d: %s", ev.ErrorCode, ev.Message)
				}
				return ev
			case <-time.After(15 * time.Second):
				t.Fatal("no GPU event")
			}
		}
	}
	ready := next()
	if ready.Kind != driver.GPUEventDeviceReady {
		t.Fatalf("first event = %+v, want the device", ready)
	}
	gen := ready.Generation

	const (
		occluder driver.GPUID = iota + 1
		screen
		indices
		shadow
		samp
		layout
		group
		depthPipe
		probePipe
		target
	)
	var idx []byte
	for _, i := range []uint16{0, 1, 2, 0, 2, 3} {
		idx = binary.LittleEndian.AppendUint16(idx, i)
	}
	rgba := driver.GPUFormatRGBA8Unorm
	layer.Submit(driver.GPUFrame{Generation: gen, Ops: []driver.GPUResourceOp{
		// The occluder covers the left half of the 16x8 shadow map at depth 0.25.
		{Kind: driver.GPUOpCreateBuffer, ID: occluder, Buffer: &driver.GPUBufferDesc{Usage: driver.GPUBufferVertex, Data: gpuFloats(-1, -1, .25, 0, -1, .25, 0, 1, .25, -1, 1, .25)}},
		{Kind: driver.GPUOpCreateBuffer, ID: screen, Buffer: &driver.GPUBufferDesc{Usage: driver.GPUBufferVertex, Data: gpuFloats(-1, -1, .5, 0, 1, 1, -1, .5, 1, 1, 1, 1, .5, 1, 0, -1, 1, .5, 0, 0)}},
		{Kind: driver.GPUOpCreateBuffer, ID: indices, Buffer: &driver.GPUBufferDesc{Usage: driver.GPUBufferIndex, Data: idx}},
		{Kind: driver.GPUOpCreateTexture, ID: shadow, Texture: &driver.GPUTextureDesc{Width: 16, Height: 8, Layers: 1, MipLevels: 1, Format: driver.GPUFormatDepth32Float, Usage: driver.GPUTextureRenderAttachment | driver.GPUTextureSampled}},
		{Kind: driver.GPUOpCreateSampler, ID: samp, Sampler: &driver.GPUSamplerDesc{Compare: driver.GPUCompareLessEqual}},
		{Kind: driver.GPUOpCreateBindGroupLayout, ID: layout, BindGroupLayout: &driver.GPUBindGroupLayoutDesc{Entries: []driver.GPUBindGroupLayoutEntry{
			{Binding: 0, Visibility: driver.GPUStageFragment, Type: driver.GPUBindingTexture},
			{Binding: 1, Visibility: driver.GPUStageFragment, Type: driver.GPUBindingComparisonSampler},
		}}},
		{Kind: driver.GPUOpCreateBindGroup, ID: group, BindGroup: &driver.GPUBindGroupDesc{Layout: layout, Entries: []driver.GPUBindGroupEntry{{Binding: 0, Texture: shadow}, {Binding: 1, Sampler: samp}}}},
		{Kind: driver.GPUOpCreatePipeline, ID: depthPipe, Pipeline: &driver.GPUPipelineDesc{
			Label:         "shadow depth",
			Vertex:        driver.GPUShaderStage{IR: depthProg.IR, EntryPoint: "vs_depth"},
			VertexBuffers: []driver.GPUVertexBufferLayout{{Stride: 12, Attributes: []driver.GPUVertexAttribute{{Format: driver.GPUVertexFloat32x3}}}},
			DepthStencil:  &driver.GPUDepthStencilState{Format: driver.GPUFormatDepth32Float, DepthWrite: true, DepthCompare: driver.GPUCompareLess},
			SampleCount:   1,
		}},
		{Kind: driver.GPUOpCreatePipeline, ID: probePipe, Pipeline: &driver.GPUPipelineDesc{
			Label:    "shadow probe",
			Vertex:   driver.GPUShaderStage{IR: probeProg.IR, EntryPoint: "vs_main"},
			Fragment: driver.GPUShaderStage{IR: probeProg.IR, EntryPoint: "fs_main"},
			Layouts:  []driver.GPUID{layout},
			VertexBuffers: []driver.GPUVertexBufferLayout{{Stride: 20, Attributes: []driver.GPUVertexAttribute{
				{Format: driver.GPUVertexFloat32x3}, {Format: driver.GPUVertexFloat32x2, Offset: 12, Location: 1},
			}}},
			Targets:     []driver.GPUColorTargetState{{Format: rgba, WriteMask: driver.GPUColorWriteAll}},
			SampleCount: 1,
		}},
		{Kind: driver.GPUOpCreateTexture, ID: target, Texture: &driver.GPUTextureDesc{Width: 8, Height: 8, Layers: 1, MipLevels: 1, Format: rgba, Usage: driver.GPUTextureRenderAttachment | driver.GPUTextureCopySrc}},
	}})
	// The view's surface turns blue, and the depth-only pass that follows must not draw into it.
	layer.Submit(driver.GPUFrame{Generation: gen, Passes: []driver.GPUPass{
		{ColorAttachments: []driver.GPUColorAttachment{{LoadOp: driver.GPULoadClear, StoreOp: driver.GPUStoreStore, ClearColor: driver.RGB(0x0000ff)}}},
		{
			DepthStencil: &driver.GPUDepthStencilAttachment{Texture: shadow, DepthLoadOp: driver.GPULoadClear, DepthStoreOp: driver.GPUStoreStore, ClearDepth: 1},
			Commands: []driver.GPUCommand{
				{Kind: driver.GPUCmdSetPipeline, ID: depthPipe},
				{Kind: driver.GPUCmdSetVertexBuffer, Slot: 0, ID: occluder},
				{Kind: driver.GPUCmdSetIndexBuffer, ID: indices, IndexFormat: driver.GPUIndexUint16},
				{Kind: driver.GPUCmdSetViewport, F2: 16, F3: 8, F5: 1},
				{Kind: driver.GPUCmdDrawIndexed, U0: 6},
			},
		},
		{
			ColorAttachments: []driver.GPUColorAttachment{{Texture: target, LoadOp: driver.GPULoadClear, StoreOp: driver.GPUStoreStore}},
			Commands: []driver.GPUCommand{
				{Kind: driver.GPUCmdSetBindGroup, Slot: 0, ID: group},
				{Kind: driver.GPUCmdSetPipeline, ID: probePipe},
				{Kind: driver.GPUCmdSetVertexBuffer, Slot: 0, ID: screen},
				{Kind: driver.GPUCmdSetIndexBuffer, ID: indices, IndexFormat: driver.GPUIndexUint16},
				{Kind: driver.GPUCmdSetViewport, F2: 8, F3: 8, F5: 1},
				{Kind: driver.GPUCmdDrawIndexed, U0: 6},
			},
		},
	}})
	read := func(id uint64, op driver.GPUResourceOp) []byte {
		t.Helper()
		op.Kind, op.ReadbackID = driver.GPUOpReadTexture, id
		layer.Submit(driver.GPUFrame{Generation: gen, Ops: []driver.GPUResourceOp{op}})
		ev := next()
		if ev.Kind != driver.GPUEventReadback || ev.ReadbackID != id || ev.Message != "" {
			t.Fatalf("readback %d = %+v", id, ev)
		}
		return ev.Pixels
	}
	near := func(got []byte, want ...byte) bool {
		for i := range want {
			if d := int(got[i]) - int(want[i]); d < -2 || d > 2 {
				return false
			}
		}
		return len(got) == len(want)
	}
	// Left of the probe the occluder's 0.25 fails 0.6 <= depth, so it is in shadow; right of it the cleared 1.0 passes.
	left := read(1, driver.GPUResourceOp{ID: target, ReadbackRegion: driver.GPUTextureRegion{X: 1, Y: 4, Width: 1, Height: 1}})
	right := read(2, driver.GPUResourceOp{ID: target, ReadbackRegion: driver.GPUTextureRegion{X: 6, Y: 4, Width: 1, Height: 1}})
	if !near(left, 51, 51, 51, 255) || !near(right, 255, 255, 255, 255) {
		t.Errorf("probe pixels %v and %v, want [51 51 51 255] in shadow and [255 255 255 255] lit", left, right)
	}
	if px := read(3, driver.GPUResourceOp{ReadbackSurface: true, ReadbackRegion: driver.GPUTextureRegion{X: 4, Y: 4, Width: 1, Height: 1}}); !near(px, 0, 0, 255, 255) {
		t.Errorf("surface pixel %v after the depth-only pass, want the blue it was cleared to", px)
	}
}
