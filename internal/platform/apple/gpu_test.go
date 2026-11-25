//go:build darwin

package apple

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
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

const gpuVec3LayoutWGSL = `
struct Packet {
	d: vec3<f32>,
	s: f32,
}

struct IntPacket {
	iv: vec3<i32>,
	is: i32,
	uv: vec3<u32>,
	us: u32,
}

struct Outer {
	lead: f32,
	p: Packet,
	arr: array<vec3<f32>, 2>,
	tail: vec3<f32>,
}

@group(0) @binding(0) var<uniform> u: Outer;
@group(0) @binding(1) var<storage, read> items: array<Packet>;
@group(0) @binding(2) var<uniform> ip: IntPacket;

fn combine(a: Packet, b: Packet) -> Packet {
	var local = a;
	local.d.x += b.d.x;
	return Packet(local.d, a.s + b.s);
}

@vertex
fn vs_main(@location(0) pos: vec2<f32>) -> @builtin(position) vec4<f32> {
	return vec4<f32>(pos, 0.5, 1.0);
}

@fragment
fn fs_main(@builtin(position) pos: vec4<f32>) -> @location(0) vec4<f32> {
	let c = combine(u.p, items[1u]);
	if (pos.x < 4.0) {
		return vec4<f32>(c.d.x, c.d.y, c.s, 1.0);
	}
	let iScale = f32(ip.iv.z + ip.is) / 10.0;
	let uScale = f32(ip.uv.x + ip.us) / 10.0;
	return vec4<f32>(u.lead + u.arr[1].x, iScale + u.arr[1].y * 0.0, uScale + u.tail.z * 0.5, 1.0);
}
`

// TestMetalVec3PackedStructLayout verifies that Metal reads {vec3<f32>, f32}, {vec3<i32>, i32}, and {vec3<u32>, u32} uniform and storage buffer members, nested structs, array<vec3<f32>, 2>, trailing vec3 padding, and helper struct construction at exact WGSL offsets under Metal's API validation.
func TestMetalVec3PackedStructLayout(t *testing.T) {
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
	prog, err := wgsl.CompilePipeline(gpuVec3LayoutWGSL)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan *driver.GPUEvent, 16)
	var layer *GPULayer
	withPool(func() {
		layer = NewGPULayer(driver.GPUViewConfig{SampleCount: 1}, func(ev *driver.GPUEvent) { events <- ev })
		if layer != nil {
			layer.Resize(8, 4, 1)
		}
	})
	if layer == nil {
		t.Skip("Metal unavailable")
	}
	t.Cleanup(func() { withPool(layer.Release) })
	next := func() *driver.GPUEvent {
		t.Helper()
		select {
		case ev := <-events:
			if ev.Kind == driver.GPUEventError {
				t.Fatalf("GPU error %d: %s", ev.ErrorCode, ev.Message)
			}
			return ev
		case <-time.After(15 * time.Second):
			t.Fatal("no GPU event")
			return nil
		}
	}
	ready := next()
	gen := ready.Generation

	const (
		vb driver.GPUID = iota + 1
		ub
		sb
		ib
		layout
		group
		pipe
		target
	)
	quad := gpuFloats(-1, -1, 1, -1, 1, 1, -1, -1, 1, 1, -1, 1)
	// Outer WGSL layout (80 bytes):
	//   0..4:   lead = 0.2
	//   4..16:  pad
	//   16..32: p = Packet{ d: (0.2, 0.4, 0.0), s: 0.4 }
	//   32..64: arr = [vec3(0,0,0), vec3(0.4, 0.6, 0)]
	//   64..80: tail = vec3(0, 0, 0.8) + 4 pad bytes
	uData := gpuFloats(
		0.2, 0, 0, 0,
		0.2, 0.4, 0.0, 0.4,
		0.0, 0.0, 0.0, 0.0,
		0.4, 0.6, 0.0, 0.0,
		0.0, 0.0, 0.8, 0.0,
	)
	sData := gpuFloats(
		0.9, 0.9, 0.9, 0.9,
		0.2, 0.0, 0.0, 0.4,
	)
	// IntPacket WGSL layout (32 bytes):
	//   0..12:  iv = (1, 2, 2)
	//   12..16: is = 4       -> (2 + 4) / 10 = 0.6
	//   16..28: uv = (1, 2, 3)
	//   28..32: us = 3       -> (1 + 3) / 10 = 0.4; + 0.8*0.5 = 0.8
	var iData []byte
	for _, v := range []uint32{1, 2, 2, 4, 1, 2, 3, 3} {
		iData = binary.LittleEndian.AppendUint32(iData, v)
	}
	rgba := driver.GPUFormatRGBA8Unorm
	layer.Submit(driver.GPUFrame{Generation: gen, Ops: []driver.GPUResourceOp{
		{Kind: driver.GPUOpCreateBuffer, ID: vb, Buffer: &driver.GPUBufferDesc{Usage: driver.GPUBufferVertex, Data: quad}},
		{Kind: driver.GPUOpCreateBuffer, ID: ub, Buffer: &driver.GPUBufferDesc{Usage: driver.GPUBufferUniform, Data: uData}},
		{Kind: driver.GPUOpCreateBuffer, ID: sb, Buffer: &driver.GPUBufferDesc{Usage: driver.GPUBufferStorage, Data: sData}},
		{Kind: driver.GPUOpCreateBuffer, ID: ib, Buffer: &driver.GPUBufferDesc{Usage: driver.GPUBufferUniform, Data: iData}},
		{Kind: driver.GPUOpCreateBindGroupLayout, ID: layout, BindGroupLayout: &driver.GPUBindGroupLayoutDesc{Entries: []driver.GPUBindGroupLayoutEntry{
			{Binding: 0, Visibility: driver.GPUStageFragment, Type: driver.GPUBindingUniform},
			{Binding: 1, Visibility: driver.GPUStageFragment, Type: driver.GPUBindingStorageRead},
			{Binding: 2, Visibility: driver.GPUStageFragment, Type: driver.GPUBindingUniform},
		}}},
		{Kind: driver.GPUOpCreateBindGroup, ID: group, BindGroup: &driver.GPUBindGroupDesc{Layout: layout, Entries: []driver.GPUBindGroupEntry{
			{Binding: 0, Buffer: ub}, {Binding: 1, Buffer: sb}, {Binding: 2, Buffer: ib},
		}}},
		{Kind: driver.GPUOpCreatePipeline, ID: pipe, Pipeline: &driver.GPUPipelineDesc{
			Label:    "vec3-layout",
			Vertex:   driver.GPUShaderStage{IR: prog.IR, EntryPoint: "vs_main"},
			Fragment: driver.GPUShaderStage{IR: prog.IR, EntryPoint: "fs_main"},
			Layouts:  []driver.GPUID{layout},
			VertexBuffers: []driver.GPUVertexBufferLayout{{
				Stride: 8, Attributes: []driver.GPUVertexAttribute{{Format: driver.GPUVertexFloat32x2}},
			}},
			Targets:     []driver.GPUColorTargetState{{Format: rgba, WriteMask: driver.GPUColorWriteAll}},
			SampleCount: 1,
		}},
		{Kind: driver.GPUOpCreateTexture, ID: target, Texture: &driver.GPUTextureDesc{
			Width: 8, Height: 4, Layers: 1, MipLevels: 1, Format: rgba, Usage: driver.GPUTextureRenderAttachment | driver.GPUTextureCopySrc,
		}},
	}, Passes: []driver.GPUPass{{
		ColorAttachments: []driver.GPUColorAttachment{{Texture: target, LoadOp: driver.GPULoadClear, StoreOp: driver.GPUStoreStore}},
		Commands: []driver.GPUCommand{
			{Kind: driver.GPUCmdSetBindGroup, Slot: 0, ID: group},
			{Kind: driver.GPUCmdSetPipeline, ID: pipe},
			{Kind: driver.GPUCmdSetVertexBuffer, Slot: 0, ID: vb},
			{Kind: driver.GPUCmdSetViewport, F2: 8, F3: 4, F5: 1},
			{Kind: driver.GPUCmdDraw, U0: 6},
		},
	}}})
	read := func(id uint64, x uint32) []byte {
		t.Helper()
		layer.Submit(driver.GPUFrame{Generation: gen, Ops: []driver.GPUResourceOp{{
			Kind: driver.GPUOpReadTexture, ReadbackID: id, ID: target,
			ReadbackRegion: driver.GPUTextureRegion{X: x, Y: 2, Width: 1, Height: 1},
		}}})
		ev := next()
		if ev.Kind != driver.GPUEventReadback || ev.ReadbackID != id || ev.Message != "" {
			t.Fatalf("readback %d = %+v", id, ev)
		}
		return ev.Pixels
	}
	near := func(got []byte, want ...byte) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range want {
			if d := int(got[i]) - int(want[i]); d < -1 || d > 1 {
				return false
			}
		}
		return true
	}
	// Left pixel: c = (0.2+0.2=0.4, 0.4, 0.4+0.4=0.8, 1.0) -> ~(102, 102, 204, 255)
	if left := read(1, 1); !near(left, 102, 102, 204, 255) {
		t.Errorf("left pixel = %v, want ~[102 102 204 255]", left)
	}
	// Right pixel: (0.2+0.4=0.6, 0.6, 0.4+0.4=0.8, 1.0) -> ~(153, 153, 204, 255)
	if right := read(2, 6); !near(right, 153, 153, 204, 255) {
		t.Errorf("right pixel = %v, want ~[153 153 204 255]", right)
	}
}
