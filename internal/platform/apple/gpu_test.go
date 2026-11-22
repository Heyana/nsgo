//go:build darwin

package apple

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
)

const gpuTestWGSL = `
struct Tint {
	color: vec4<f32>,
}

@group(0) @binding(0) var<uniform> tint: Tint;
@group(0) @binding(1) var tex: texture_2d<f32>;
@group(0) @binding(2) var samp: sampler;

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
	return textureSample(tex, samp, in.uv) * tint.color;
}
`

func gpuFloats(vs ...float32) []byte {
	var b []byte
	for _, v := range vs {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
	}
	return b
}

// TestMetalGPUFrames draws a textured, tinted quad over a farther one into an offscreen target and into the view's surface, reads both back, and checks that releasing the layer lets every Metal resource go.
func TestMetalGPUFrames(t *testing.T) {
	prog, err := wgsl.CompilePipeline(gpuTestWGSL)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan *driver.GPUEvent, 32)
	var layer *GPULayer
	withPool(func() {
		layer = NewGPULayer(driver.GPUViewConfig{DepthFormat: driver.GPUFormatDepth24Plus, SampleCount: 1}, func(ev *driver.GPUEvent) { events <- ev })
		if layer != nil {
			layer.Resize(8, 8, 1)
		}
	})
	if layer == nil {
		t.Skip("Metal unavailable")
	}
	t.Cleanup(func() {
		withPool(layer.Release)
		select {
		case <-layer.render.done:
			r := layer.render
			if n := len(r.buffers) + len(r.textures) + len(r.samplers) + len(r.pipelines); n != 0 || r.surface != nil {
				t.Errorf("%d GPU resources and the surface survived the layer's release", n)
			}
		case <-time.After(10 * time.Second):
			t.Error("the GPU render goroutine did not stop")
		}
	})
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
	if ready.Kind != driver.GPUEventDeviceReady || ready.Width != 8 || ready.Height != 8 || ready.SurfaceFormat != driver.GPUFormatBGRA8Unorm {
		t.Fatalf("first event = %+v, want an 8x8 BGRA8 device", ready)
	}
	gen := ready.Generation

	const (
		vertices driver.GPUID = iota + 1
		indices
		tint
		tex
		samp
		layout
		group
		pipe
		target
		depth
	)
	quads := gpuFloats(
		// Near, with UVs across the texture.
		-1, -1, .2, 0, 1, 1, -1, .2, 1, 1, 1, 1, .2, 1, 0, -1, 1, .2, 0, 0,
		// Far, sampling the texture's green texel everywhere.
		-1, -1, .8, .75, .5, 1, -1, .8, .75, .5, 1, 1, .8, .75, .5, -1, 1, .8, .75, .5,
	)
	var idx []byte
	for _, i := range []uint16{0, 1, 2, 0, 2, 3} {
		idx = binary.LittleEndian.AppendUint16(idx, i)
	}
	rgba := driver.GPUFormatRGBA8Unorm
	layer.Submit(driver.GPUFrame{Generation: gen, Ops: []driver.GPUResourceOp{
		{Kind: driver.GPUOpCreateBuffer, ID: vertices, Buffer: &driver.GPUBufferDesc{Usage: driver.GPUBufferVertex, Data: quads}},
		{Kind: driver.GPUOpCreateBuffer, ID: indices, Buffer: &driver.GPUBufferDesc{Usage: driver.GPUBufferIndex, Data: idx}},
		{Kind: driver.GPUOpCreateBuffer, ID: tint, Buffer: &driver.GPUBufferDesc{Size: 16, Usage: driver.GPUBufferUniform | driver.GPUBufferCopyDst}},
		{Kind: driver.GPUOpWriteBuffer, ID: tint, Data: gpuFloats(.5, 1, 1, 1)},
		{Kind: driver.GPUOpCreateTexture, ID: tex, Texture: &driver.GPUTextureDesc{Width: 2, Height: 1, Layers: 1, MipLevels: 1, Format: rgba, Usage: driver.GPUTextureSampled | driver.GPUTextureCopyDst}},
		{Kind: driver.GPUOpWriteTexture, ID: tex, Data: []byte{200, 0, 0, 255, 0, 200, 0, 255}, TextureDst: driver.GPUTextureCopy{Texture: tex}, DataLayout: driver.GPUDataLayout{BytesPerRow: 8, RowsPerImage: 1}},
		{Kind: driver.GPUOpCreateSampler, ID: samp, Sampler: &driver.GPUSamplerDesc{}},
		{Kind: driver.GPUOpCreateBindGroupLayout, ID: layout, BindGroupLayout: &driver.GPUBindGroupLayoutDesc{Entries: []driver.GPUBindGroupLayoutEntry{
			{Binding: 0, Visibility: driver.GPUStageFragment, Type: driver.GPUBindingUniform},
			{Binding: 1, Visibility: driver.GPUStageFragment, Type: driver.GPUBindingTexture},
			{Binding: 2, Visibility: driver.GPUStageFragment, Type: driver.GPUBindingSampler},
		}}},
		{Kind: driver.GPUOpCreateBindGroup, ID: group, BindGroup: &driver.GPUBindGroupDesc{Layout: layout, Entries: []driver.GPUBindGroupEntry{
			{Binding: 0, Buffer: tint, Size: 16}, {Binding: 1, Texture: tex}, {Binding: 2, Sampler: samp},
		}}},
		{Kind: driver.GPUOpCreatePipeline, ID: pipe, Pipeline: &driver.GPUPipelineDesc{
			Label:    "quad",
			Vertex:   driver.GPUShaderStage{IR: prog.IR, EntryPoint: "vs_main"},
			Fragment: driver.GPUShaderStage{IR: prog.IR, EntryPoint: "fs_main"},
			Layouts:  []driver.GPUID{layout},
			VertexBuffers: []driver.GPUVertexBufferLayout{{Stride: 20, Attributes: []driver.GPUVertexAttribute{
				{Format: driver.GPUVertexFloat32x3, Location: 0}, {Format: driver.GPUVertexFloat32x2, Offset: 12, Location: 1},
			}}},
			DepthStencil: &driver.GPUDepthStencilState{Format: driver.GPUFormatDepth24Plus, DepthWrite: true, DepthCompare: driver.GPUCompareLess},
			Targets:      []driver.GPUColorTargetState{{Format: rgba, WriteMask: driver.GPUColorWriteAll}},
			SampleCount:  1,
		}},
		{Kind: driver.GPUOpCreateTexture, ID: target, Texture: &driver.GPUTextureDesc{Width: 8, Height: 8, Layers: 1, MipLevels: 1, Format: rgba, Usage: driver.GPUTextureRenderAttachment | driver.GPUTextureCopySrc}},
		{Kind: driver.GPUOpCreateTexture, ID: depth, Texture: &driver.GPUTextureDesc{Width: 8, Height: 8, Layers: 1, MipLevels: 1, Format: driver.GPUFormatDepth24Plus, Usage: driver.GPUTextureRenderAttachment}},
	}})
	draw := []driver.GPUCommand{
		{Kind: driver.GPUCmdSetBindGroup, Slot: 0, ID: group},
		{Kind: driver.GPUCmdSetPipeline, ID: pipe},
		{Kind: driver.GPUCmdSetVertexBuffer, Slot: 0, ID: vertices},
		{Kind: driver.GPUCmdSetIndexBuffer, ID: indices, IndexFormat: driver.GPUIndexUint16},
		{Kind: driver.GPUCmdSetViewport, F2: 8, F3: 8, F5: 1},
		{Kind: driver.GPUCmdDrawIndexed, U0: 6},
		{Kind: driver.GPUCmdDrawIndexed, U0: 6, I0: 4},
	}
	layer.Submit(driver.GPUFrame{Generation: gen, Passes: []driver.GPUPass{{
		ColorAttachments: []driver.GPUColorAttachment{{Texture: target, LoadOp: driver.GPULoadClear, StoreOp: driver.GPUStoreStore}},
		DepthStencil:     &driver.GPUDepthStencilAttachment{Texture: depth, DepthLoadOp: driver.GPULoadClear, ClearDepth: 1},
		Commands:         draw,
	}}})
	// The view's own surface and depth buffer, which a BGRA8 pipeline draws into.
	layer.Submit(driver.GPUFrame{Generation: gen, Ops: []driver.GPUResourceOp{{Kind: driver.GPUOpCreatePipeline, ID: pipe + 100, Pipeline: &driver.GPUPipelineDesc{
		Vertex:        driver.GPUShaderStage{IR: prog.IR, EntryPoint: "vs_main"},
		Fragment:      driver.GPUShaderStage{IR: prog.IR, EntryPoint: "fs_main"},
		Layouts:       []driver.GPUID{layout},
		VertexBuffers: []driver.GPUVertexBufferLayout{{Stride: 20, Attributes: []driver.GPUVertexAttribute{{Format: driver.GPUVertexFloat32x3}, {Format: driver.GPUVertexFloat32x2, Offset: 12, Location: 1}}}},
		DepthStencil:  &driver.GPUDepthStencilState{DepthWrite: true, DepthCompare: driver.GPUCompareLess},
		Targets:       []driver.GPUColorTargetState{{Format: driver.GPUFormatBGRA8Unorm, WriteMask: driver.GPUColorWriteAll}},
		SampleCount:   1,
	}}}, Passes: []driver.GPUPass{{
		ColorAttachments: []driver.GPUColorAttachment{{LoadOp: driver.GPULoadClear}},
		DepthStencil:     &driver.GPUDepthStencilAttachment{DepthLoadOp: driver.GPULoadClear, ClearDepth: 1},
		Commands:         append([]driver.GPUCommand{draw[0], {Kind: driver.GPUCmdSetPipeline, ID: pipe + 100}}, draw[2:]...),
	}}})
	// A frame for another device draws nothing.
	layer.Submit(driver.GPUFrame{Generation: gen + 1000, Passes: []driver.GPUPass{{ColorAttachments: []driver.GPUColorAttachment{{Texture: target, LoadOp: driver.GPULoadClear, ClearColor: driver.Color(0xffffffff)}}}}})

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
			if d := int(got[i]) - int(want[i]); d < -1 || d > 1 {
				return false
			}
		}
		return len(got) == len(want)
	}
	for i, op := range []driver.GPUResourceOp{{ID: target}, {ReadbackSurface: true}} {
		left := read(uint64(2*i+1), driver.GPUResourceOp{ID: op.ID, ReadbackSurface: op.ReadbackSurface, ReadbackRegion: driver.GPUTextureRegion{X: 1, Y: 4, Width: 1, Height: 1}})
		right := read(uint64(2*i+2), driver.GPUResourceOp{ID: op.ID, ReadbackSurface: op.ReadbackSurface, ReadbackRegion: driver.GPUTextureRegion{X: 6, Y: 4, Width: 1, Height: 1}})
		// The near quad's red texel, tinted, and its green one: the far quad fails the depth test.
		if !near(left, 100, 0, 0, 255) || !near(right, 0, 200, 0, 255) {
			t.Errorf("target %d: pixels %v and %v, want [100 0 0 255] and [0 200 0 255]", i, left, right)
		}
	}
}
