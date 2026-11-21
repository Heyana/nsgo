//go:build windows

package windows

import (
	"encoding/binary"
	"math"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
)

func packF32(vals ...float32) []byte {
	out := make([]byte, len(vals)*4)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(v))
	}
	return out
}

func packU16(vals ...uint16) []byte {
	out := make([]byte, len(vals)*2)
	for i, v := range vals {
		binary.LittleEndian.PutUint16(out[i*2:], v)
	}
	return out
}

func testPerspectiveRH(fovYRad, aspect, near, far float32) [16]float32 {
	f := float32(1.0 / math.Tan(float64(fovYRad)/2.0))
	return [16]float32{
		f / aspect, 0, 0, 0,
		0, f, 0, 0,
		0, 0, far / (near - far), -1,
		0, 0, (far * near) / (near - far), 0,
	}
}

func testTranslateMat4(tx, ty, tz float32) [16]float32 {
	return [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		tx, ty, tz, 1,
	}
}

func testMulMat4(a, b [16]float32) [16]float32 {
	var out [16]float32
	for c := 0; c < 4; c++ {
		for r := 0; r < 4; r++ {
			var sum float32
			for k := 0; k < 4; k++ {
				sum += a[k*4+r] * b[c*4+k]
			}
			out[c*4+r] = sum
		}
	}
	return out
}

// TestD3D11GPULitTexturedDepthMeshAndD2DComposite renders a textured, directional-lit, depth-tested 3D scene on Direct3D 11 (hardware or WARP), verifies CPU readback pixels (BGRA->RGBA swizzle, depth rejection, lighting, texture sampling), verifies Direct2D compositing via painter.drawGPU, and verifies clean COM release on unmount.
func TestD3D11GPULitTexturedDepthMeshAndD2DComposite(t *testing.T) {
	if !gpuAvailable() {
		t.Skip("Direct3D 11 or D3DCompile unavailable")
	}
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}

	const litTexturedWGSL = `
struct Uniforms {
	mvp: mat4x4<f32>,
	lightDir: vec4<f32>,
}

@group(0) @binding(0) var<uniform> u: Uniforms;
@group(0) @binding(1) var albedoTex: texture_2d<f32>;
@group(0) @binding(2) var albedoSamp: sampler;

struct VertexIn {
	@location(0) pos: vec3<f32>,
	@location(1) normal: vec3<f32>,
	@location(2) uv: vec2<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) normal: vec3<f32>,
	@location(1) uv: vec2<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = u.mvp * vec4<f32>(in.pos, 1.0);
	out.normal = in.normal;
	out.uv = in.uv;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	let n = normalize(in.normal);
	let l = normalize(u.lightDir.xyz);
	let ndl = max(dot(n, l), 0.0);
	let albedo = textureSample(albedoTex, albedoSamp, in.uv);
	let lit = vec3<f32>(0.2) + vec3<f32>(0.8) * ndl;
	return vec4<f32>(albedo.rgb * lit, albedo.a);
}
`
	prog, err := wgsl.CompilePipeline(litTexturedWGSL)
	if err != nil {
		t.Fatalf("wgsl.CompilePipeline: %v", err)
	}

	const pw, ph = 64, 64
	var events []*driver.GPUEvent
	ops := &d3d11NativeOps{scaleFn: func() float32 { return 1 }}
	ctrl := newD3DGPUController(ops, func(ev *driver.GPUEvent) {
		events = append(events, ev)
	})
	defer ctrl.Release()

	ctrl.Configure(driver.GPUViewConfig{
		ColorFormat: driver.GPUFormatBGRA8Unorm,
		DepthFormat: driver.GPUFormatDepth32Float,
		SampleCount: 1,
	})
	if !ctrl.MountOrResize(pw, ph) {
		t.Fatalf("MountOrResize failed; events=%+v", events)
	}

	// Near quad at z = +0.5 (CCW, normal +Z -> full light = 1.0, textured red/green). Far quad at z = -0.5 (CCW, normal +Z, larger rim, textured blue).
	nearVerts := packF32(
		-0.5, -0.5, 0.5, 0, 0, 1, 0, 1,
		0.5, -0.5, 0.5, 0, 0, 1, 1, 1,
		0.5, 0.5, 0.5, 0, 0, 1, 1, 0,
		-0.5, 0.5, 0.5, 0, 0, 1, 0, 0,
	)
	farVerts := packF32(
		-1.0, -1.0, -0.5, 0, 0, 1, 0, 1,
		1.0, -1.0, -0.5, 0, 0, 1, 1, 1,
		1.0, 1.0, -0.5, 0, 0, 1, 1, 0,
		-1.0, 1.0, -0.5, 0, 0, 1, 0, 0,
	)
	indices := packU16(0, 1, 2, 0, 2, 3)

	mvp := testMulMat4(testPerspectiveRH(math.Pi/3, 1.0, 0.1, 20.0), testTranslateMat4(0, 0, -3.0))
	uniformData := append(packF32(mvp[:]...), packF32(0, 0, 1, 0)...)

	// 2x2 near texture: left=red (255,0,0,255), right=green (0,255,0,255).
	nearTexPix := []byte{
		255, 0, 0, 255, 0, 255, 0, 255,
		255, 0, 0, 255, 0, 255, 0, 255,
	}
	// 2x2 far texture: solid blue (0,0,255,255).
	farTexPix := []byte{
		0, 0, 255, 255, 0, 0, 255, 255,
		0, 0, 255, 255, 0, 0, 255, 255,
	}

	frame := driver.GPUFrame{
		Generation: 1,
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreateBuffer, ID: 1, Buffer: &driver.GPUBufferDesc{Size: uint64(len(nearVerts)), Usage: driver.GPUBufferVertex, Data: nearVerts}},
			{Kind: driver.GPUOpCreateBuffer, ID: 2, Buffer: &driver.GPUBufferDesc{Size: uint64(len(farVerts)), Usage: driver.GPUBufferVertex, Data: farVerts}},
			{Kind: driver.GPUOpCreateBuffer, ID: 3, Buffer: &driver.GPUBufferDesc{Size: uint64(len(indices)), Usage: driver.GPUBufferIndex, Data: indices}},
			{Kind: driver.GPUOpCreateBuffer, ID: 4, Buffer: &driver.GPUBufferDesc{Size: uint64(len(uniformData)), Usage: driver.GPUBufferUniform | driver.GPUBufferCopyDst, Data: uniformData}},
			{Kind: driver.GPUOpCreateTexture, ID: 5, Texture: &driver.GPUTextureDesc{Width: 2, Height: 2, Format: driver.GPUFormatRGBA8Unorm, Usage: driver.GPUTextureSampled | driver.GPUTextureCopyDst}},
			{Kind: driver.GPUOpWriteTexture, ID: 5, Data: nearTexPix, TextureDst: driver.GPUTextureCopy{Texture: 5}, DataLayout: driver.GPUDataLayout{BytesPerRow: 8, RowsPerImage: 2}},
			{Kind: driver.GPUOpCreateTexture, ID: 6, Texture: &driver.GPUTextureDesc{Width: 2, Height: 2, Format: driver.GPUFormatRGBA8Unorm, Usage: driver.GPUTextureSampled | driver.GPUTextureCopyDst}},
			{Kind: driver.GPUOpWriteTexture, ID: 6, Data: farTexPix, TextureDst: driver.GPUTextureCopy{Texture: 6}, DataLayout: driver.GPUDataLayout{BytesPerRow: 8, RowsPerImage: 2}},
			{Kind: driver.GPUOpCreateSampler, ID: 7, Sampler: &driver.GPUSamplerDesc{MinFilter: driver.GPUFilterNearest, MagFilter: driver.GPUFilterNearest, AddressU: driver.GPUAddressClampToEdge, AddressV: driver.GPUAddressClampToEdge}},
			{Kind: driver.GPUOpCreateBindGroupLayout, ID: 8, BindGroupLayout: &driver.GPUBindGroupLayoutDesc{}},
			{Kind: driver.GPUOpCreateBindGroup, ID: 9, BindGroup: &driver.GPUBindGroupDesc{Layout: 8, Entries: []driver.GPUBindGroupEntry{
				{Binding: 0, Buffer: 4}, {Binding: 1, Texture: 5}, {Binding: 2, Sampler: 7},
			}}},
			{Kind: driver.GPUOpCreateBindGroup, ID: 10, BindGroup: &driver.GPUBindGroupDesc{Layout: 8, Entries: []driver.GPUBindGroupEntry{
				{Binding: 0, Buffer: 4}, {Binding: 1, Texture: 6}, {Binding: 2, Sampler: 7},
			}}},
			{Kind: driver.GPUOpCreatePipeline, ID: 11, Pipeline: &driver.GPUPipelineDesc{
				Vertex:   driver.GPUShaderStage{IR: prog.IR, EntryPoint: "vs_main"},
				Fragment: driver.GPUShaderStage{IR: prog.IR, EntryPoint: "fs_main"},
				Layouts:  []driver.GPUID{8},
				VertexBuffers: []driver.GPUVertexBufferLayout{{
					Stride: 8 * 4,
					Attributes: []driver.GPUVertexAttribute{
						{Location: 0, Offset: 0, Format: driver.GPUVertexFloat32x3},
						{Location: 1, Offset: 3 * 4, Format: driver.GPUVertexFloat32x3},
						{Location: 2, Offset: 6 * 4, Format: driver.GPUVertexFloat32x2},
					},
				}},
				Primitive: driver.GPUPrimitiveState{
					Topology:  driver.GPUTopologyTriangleList,
					FrontFace: driver.GPUFrontCCW,
					CullMode:  driver.GPUCullBack,
				},
				DepthStencil: &driver.GPUDepthStencilState{
					Format:       driver.GPUFormatDepth32Float,
					DepthWrite:   true,
					DepthCompare: driver.GPUCompareLess,
				},
				Targets: []driver.GPUColorTargetState{{Format: driver.GPUFormatBGRA8Unorm}},
			}},
			{Kind: driver.GPUOpReadTexture, ReadbackID: 42, ReadbackSurface: true},
		},
		Passes: []driver.GPUPass{{
			ColorAttachments: []driver.GPUColorAttachment{{
				LoadOp:     driver.GPULoadClear,
				StoreOp:    driver.GPUStoreStore,
				ClearColor: driver.RGB(0x10141c),
			}},
			DepthStencil: &driver.GPUDepthStencilAttachment{
				DepthLoadOp:  driver.GPULoadClear,
				DepthStoreOp: driver.GPUStoreDiscard,
				ClearDepth:   1.0,
			},
			Commands: []driver.GPUCommand{
				{Kind: driver.GPUCmdSetPipeline, ID: 11},
				{Kind: driver.GPUCmdSetIndexBuffer, ID: 3, IndexFormat: driver.GPUIndexUint16},
				// Draw near textured quad first.
				{Kind: driver.GPUCmdSetBindGroup, Slot: 0, ID: 9},
				{Kind: driver.GPUCmdSetVertexBuffer, Slot: 0, ID: 1},
				{Kind: driver.GPUCmdDrawIndexed, U0: 6},
				// Draw far larger blue quad second: depth test must reject pixels behind near quad.
				{Kind: driver.GPUCmdSetBindGroup, Slot: 0, ID: 10},
				{Kind: driver.GPUCmdSetVertexBuffer, Slot: 0, ID: 2},
				{Kind: driver.GPUCmdDrawIndexed, U0: 6},
			},
		}},
		Present: true,
	}

	if !ctrl.Submit(frame) {
		t.Fatalf("Submit did not draw swapchain; events=%+v", events)
	}

	var pixels []byte
	for _, ev := range events {
		if ev.Kind == driver.GPUEventError {
			t.Fatalf("unexpected GPUEventError: code=%d msg=%s", ev.ErrorCode, ev.Message)
		}
		if ev.Kind == driver.GPUEventReadback && ev.ReadbackID == 42 {
			if ev.Message != "" {
				t.Fatalf("ReadSurface failed: %s", ev.Message)
			}
			pixels = ev.Pixels
		}
	}
	if len(pixels) != pw*ph*4 {
		t.Fatalf("readback len = %d, want %d", len(pixels), pw*ph*4)
	}
	pxAt := func(x, y int) [4]byte {
		i := (y*pw + x) * 4
		return [4]byte{pixels[i], pixels[i+1], pixels[i+2], pixels[i+3]}
	}

	// Corner (2, 2): clear color (16, 20, 28, 255).
	if c := pxAt(2, 2); c[0] < 14 || c[0] > 18 || c[1] < 18 || c[1] > 22 || c[2] < 26 || c[2] > 30 || c[3] != 255 {
		t.Errorf("corner pixel = %v, want clear color ~[16 20 28 255]", c)
	}
	// Near quad left half (26, 32): lit red (~255, 0, 0, 255).
	if c := pxAt(26, 32); c[0] < 245 || c[1] > 10 || c[2] > 10 || c[3] != 255 {
		t.Errorf("near-left pixel (26,32) = %v, want lit red (depth test should keep near quad over far blue)", c)
	}
	// Near quad right half (38, 32): lit green (~0, 255, 0, 255).
	if c := pxAt(38, 32); c[1] < 245 || c[0] > 10 || c[2] > 10 || c[3] != 255 {
		t.Errorf("near-right pixel (38,32) = %v, want lit green", c)
	}
	// Far quad outer rim (19, 32): lit blue (~0, 0, 255, 255).
	if c := pxAt(19, 32); c[2] < 245 || c[0] > 10 || c[1] > 10 || c[3] != 255 {
		t.Errorf("far-rim pixel (19,32) = %v, want lit blue", c)
	}

	// Verify Direct2D compositing via painter.drawGPU into an offscreen D2D bitmap.
	w := &window{scale: 1, animator: driver.NewAnimator(nil)}
	v := &view{id: 1, kind: driver.KindGPU, gpuView: &gpuView{w: w, v: nil, ctrl: ctrl}}
	v.gpuView.v = v

	props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied, dpiX: 96, dpiY: 96, options: d2dBitmapTarget}
	var target, readable *comObject
	if failed(g.dc.callSize(dcCreateBitmap, pw, ph, 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&target)))) {
		t.Fatal("failed to create D2D target bitmap")
	}
	defer target.release()
	props.options = d2dBitmapCPURead | d2dBitmapCannotDraw
	if failed(g.dc.callSize(dcCreateBitmap, pw, ph, 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&readable)))) {
		t.Fatal("failed to create D2D readable bitmap")
	}
	defer readable.release()

	g.dc.call(dcSetTarget, target.ptr())
	g.dc.callF(rtSetDpi, 0b11, f32(96), f32(96))
	g.dc.call(rtBeginDraw)
	clearCol := colorF{}
	g.dc.call(rtClear, uintptr(unsafe.Pointer(&clearCol)))
	p := newPainter(w, g)
	p.setTransform(identity)
	p.drawGPU(v, pw, ph)
	p.release()
	if hr := g.dc.call(rtEndDraw, 0, 0); failed(hr) {
		t.Fatal(hresultError("D2D EndDraw", hr))
	}
	g.dc.call(dcSetTarget, 0)

	readable.call(bitmapCopyFromBitmap, 0, target.ptr(), 0)
	var mapped struct {
		pitch uint32
		bits  *byte
	}
	if failed(readable.call(bitmapMap, d2dMapRead, uintptr(unsafe.Pointer(&mapped)))) {
		t.Fatal("cannot map D2D readable bitmap")
	}
	pix := unsafe.Slice(mapped.bits, int(mapped.pitch)*ph)
	// BGRA order in D2D bitmap: (26, 32) is red -> B<10, G<10, R>245.
	off := 32*int(mapped.pitch) + 26*4
	b0, g0, r0, a0 := pix[off], pix[off+1], pix[off+2], pix[off+3]
	readable.call(bitmapUnmap)
	if r0 < 245 || g0 > 10 || b0 > 10 || a0 != 255 {
		t.Errorf("D2D composited pixel (26,32) BGRA = (%d,%d,%d,%d), want red", b0, g0, r0, a0)
	}

	// Verify full COM Release on unmount clears all retained resource maps.
	v.gpuView.release()
	if len(ctrl.buffers) != 0 || len(ctrl.textures) != 0 || len(ctrl.samplers) != 0 || len(ctrl.pipelines) != 0 || ctrl.swap != nil {
		t.Errorf("unmount release left live resources")
	}
}

// TestD3D11GPUOffscreenTargetBlendInstancedAndScissor verifies offscreen render-to-texture sampling, alpha blending, per-instance vertex buffers with DrawIndexedInstanced, and SetScissor clipping on Direct3D 11.
func TestD3D11GPUOffscreenTargetBlendInstancedAndScissor(t *testing.T) {
	if !gpuAvailable() {
		t.Skip("Direct3D 11 or D3DCompile unavailable")
	}
	const texWGSL = `
@group(0) @binding(0) var tex: texture_2d<f32>;
@group(0) @binding(1) var samp: sampler;
struct VIn { @location(0) pos: vec2<f32>, @location(1) uv: vec2<f32>, }
struct VOut { @builtin(position) pos: vec4<f32>, @location(0) uv: vec2<f32>, }
@vertex fn vs_main(in: VIn) -> VOut {
	var out: VOut;
	out.pos = vec4<f32>(in.pos, 0.0, 1.0);
	out.uv = in.uv;
	return out;
}
@fragment fn fs_main(in: VOut) -> @location(0) vec4<f32> {
	return textureSample(tex, samp, in.uv);
}
`
	const instWGSL = `
struct VIn {
	@location(0) pos: vec2<f32>,
	@location(1) offset: vec2<f32>,
	@location(2) color: vec4<f32>,
}
struct VOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
}
@vertex fn vs_main(in: VIn) -> VOut {
	var out: VOut;
	out.pos = vec4<f32>(in.pos + in.offset, 0.0, 1.0);
	out.color = in.color;
	return out;
}
@fragment fn fs_main(in: VOut) -> @location(0) vec4<f32> {
	return in.color;
}
`
	texProg, err := wgsl.CompilePipeline(texWGSL)
	if err != nil {
		t.Fatalf("compile texWGSL: %v", err)
	}
	instProg, err := wgsl.CompilePipeline(instWGSL)
	if err != nil {
		t.Fatalf("compile instWGSL: %v", err)
	}

	const pw, ph = 32, 32
	var events []*driver.GPUEvent
	ctrl := newD3DGPUController(&d3d11NativeOps{scaleFn: func() float32 { return 1 }}, func(ev *driver.GPUEvent) {
		events = append(events, ev)
	})
	defer ctrl.Release()
	ctrl.Configure(driver.GPUViewConfig{ColorFormat: driver.GPUFormatBGRA8Unorm, SampleCount: 1})
	if !ctrl.MountOrResize(pw, ph) {
		t.Fatal("MountOrResize failed")
	}

	quadVerts := packF32(
		-1, -1, 0, 1,
		1, -1, 1, 1,
		1, 1, 1, 0,
		-1, 1, 0, 0,
	)
	unitQuad := packF32(
		-1, -1,
		1, -1,
		1, 1,
		-1, 1,
	)
	// Two instances: first shifted left (off-screen/clipped), second centered at (0,0) with 50% red.
	instData := packF32(
		-4, 0, 0, 1, 0, 1,
		0, 0, 1, 0, 0, 0.5,
	)
	indices := packU16(0, 1, 2, 0, 2, 3)

	ctrl.Submit(driver.GPUFrame{
		Generation: 1,
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreateTexture, ID: 1, Texture: &driver.GPUTextureDesc{
				Width: 16, Height: 16, Format: driver.GPUFormatRGBA8Unorm,
				Usage: driver.GPUTextureRenderAttachment | driver.GPUTextureSampled | driver.GPUTextureCopySrc,
			}},
			{Kind: driver.GPUOpCreateSampler, ID: 2, Sampler: &driver.GPUSamplerDesc{MinFilter: driver.GPUFilterNearest, MagFilter: driver.GPUFilterNearest}},
			{Kind: driver.GPUOpCreateBindGroupLayout, ID: 3, BindGroupLayout: &driver.GPUBindGroupLayoutDesc{}},
			{Kind: driver.GPUOpCreateBindGroup, ID: 4, BindGroup: &driver.GPUBindGroupDesc{Layout: 3, Entries: []driver.GPUBindGroupEntry{
				{Binding: 0, Texture: 1}, {Binding: 1, Sampler: 2},
			}}},
			{Kind: driver.GPUOpCreateBuffer, ID: 5, Buffer: &driver.GPUBufferDesc{Size: uint64(len(quadVerts)), Usage: driver.GPUBufferVertex, Data: quadVerts}},
			{Kind: driver.GPUOpCreateBuffer, ID: 6, Buffer: &driver.GPUBufferDesc{Size: uint64(len(unitQuad)), Usage: driver.GPUBufferVertex, Data: unitQuad}},
			{Kind: driver.GPUOpCreateBuffer, ID: 7, Buffer: &driver.GPUBufferDesc{Size: uint64(len(instData)), Usage: driver.GPUBufferVertex, Data: instData}},
			{Kind: driver.GPUOpCreateBuffer, ID: 8, Buffer: &driver.GPUBufferDesc{Size: uint64(len(indices)), Usage: driver.GPUBufferIndex, Data: indices}},
			{Kind: driver.GPUOpCreatePipeline, ID: 9, Pipeline: &driver.GPUPipelineDesc{
				Vertex:   driver.GPUShaderStage{IR: texProg.IR, EntryPoint: "vs_main"},
				Fragment: driver.GPUShaderStage{IR: texProg.IR, EntryPoint: "fs_main"},
				Layouts:  []driver.GPUID{3},
				VertexBuffers: []driver.GPUVertexBufferLayout{{
					Stride: 16,
					Attributes: []driver.GPUVertexAttribute{
						{Location: 0, Offset: 0, Format: driver.GPUVertexFloat32x2},
						{Location: 1, Offset: 8, Format: driver.GPUVertexFloat32x2},
					},
				}},
				Targets: []driver.GPUColorTargetState{{Format: driver.GPUFormatBGRA8Unorm}},
			}},
			{Kind: driver.GPUOpCreatePipeline, ID: 10, Pipeline: &driver.GPUPipelineDesc{
				Vertex:   driver.GPUShaderStage{IR: instProg.IR, EntryPoint: "vs_main"},
				Fragment: driver.GPUShaderStage{IR: instProg.IR, EntryPoint: "fs_main"},
				VertexBuffers: []driver.GPUVertexBufferLayout{
					{
						Stride:   8,
						StepMode: driver.GPUStepVertex,
						Attributes: []driver.GPUVertexAttribute{
							{Location: 0, Offset: 0, Format: driver.GPUVertexFloat32x2},
						},
					},
					{
						Stride:   24,
						StepMode: driver.GPUStepInstance,
						Attributes: []driver.GPUVertexAttribute{
							{Location: 1, Offset: 0, Format: driver.GPUVertexFloat32x2},
							{Location: 2, Offset: 8, Format: driver.GPUVertexFloat32x4},
						},
					},
				},
				Targets: []driver.GPUColorTargetState{{
					Format: driver.GPUFormatBGRA8Unorm,
					Blend: &driver.GPUBlendState{
						Color: driver.GPUBlendComponent{SrcFactor: driver.GPUBlendSrcAlpha, DstFactor: driver.GPUBlendOneMinusSrcAlpha, Operation: driver.GPUBlendAdd},
						Alpha: driver.GPUBlendComponent{SrcFactor: driver.GPUBlendOne, DstFactor: driver.GPUBlendOneMinusSrcAlpha, Operation: driver.GPUBlendAdd},
					},
				}},
			}},
			{Kind: driver.GPUOpReadTexture, ID: 1, ReadbackID: 1, ReadbackRegion: driver.GPUTextureRegion{Width: 16, Height: 16}},
			{Kind: driver.GPUOpReadTexture, ReadbackID: 2, ReadbackSurface: true},
		},
		Passes: []driver.GPUPass{
			// Pass 1: Clear offscreen render target to green (0, 200, 0, 255).
			{
				ColorAttachments: []driver.GPUColorAttachment{{
					Texture:    1,
					LoadOp:     driver.GPULoadClear,
					StoreOp:    driver.GPUStoreStore,
					ClearColor: driver.RGB(0x00c800),
				}},
			},
			// Pass 2: Sample offscreen RT onto swapchain, then blend 50% red ONLY inside right half via SetScissor.
			{
				ColorAttachments: []driver.GPUColorAttachment{{
					LoadOp:     driver.GPULoadClear,
					StoreOp:    driver.GPUStoreStore,
					ClearColor: driver.RGB(0x000000),
				}},
				Commands: []driver.GPUCommand{
					{Kind: driver.GPUCmdSetIndexBuffer, ID: 8, IndexFormat: driver.GPUIndexUint16},
					{Kind: driver.GPUCmdSetPipeline, ID: 9},
					{Kind: driver.GPUCmdSetBindGroup, Slot: 0, ID: 4},
					{Kind: driver.GPUCmdSetVertexBuffer, Slot: 0, ID: 5},
					{Kind: driver.GPUCmdDrawIndexed, U0: 6},
					{Kind: driver.GPUCmdSetScissor, I0: 16, I1: 0, I2: 16, I3: 32},
					{Kind: driver.GPUCmdSetPipeline, ID: 10},
					{Kind: driver.GPUCmdSetVertexBuffer, Slot: 0, ID: 6},
					{Kind: driver.GPUCmdSetVertexBuffer, Slot: 1, ID: 7},
					{Kind: driver.GPUCmdDrawIndexedInstanced, U0: 6, U1: 2},
				},
			},
		},
		Present: true,
	})

	var rtPix, swapPix []byte
	for _, ev := range events {
		if ev.Kind == driver.GPUEventError {
			t.Fatalf("unexpected GPUEventError: %+v", ev)
		}
		if ev.Kind == driver.GPUEventReadback {
			if ev.Message != "" {
				t.Fatalf("readback %d error: %s", ev.ReadbackID, ev.Message)
			}
			if ev.ReadbackID == 1 {
				rtPix = ev.Pixels
			} else if ev.ReadbackID == 2 {
				swapPix = ev.Pixels
			}
		}
	}
	if len(rtPix) != 16*16*4 || rtPix[0] > 4 || rtPix[1] < 196 || rtPix[1] > 204 || rtPix[2] > 4 {
		t.Fatalf("offscreen RT first pixel = %v, want ~[0 200 0 255]", rtPix[:min(4, len(rtPix))])
	}
	if len(swapPix) != pw*ph*4 {
		t.Fatalf("swapPix len = %d, want %d", len(swapPix), pw*ph*4)
	}
	// Left half (8, 16) outside scissor: pure offscreen green (~0, 200, 0).
	li := (16*pw + 8) * 4
	if swapPix[li] > 6 || swapPix[li+1] < 194 || swapPix[li+1] > 206 {
		t.Errorf("left unblended pixel = %v, want ~[0 200 0 255]", swapPix[li:li+4])
	}
	// Right half (24, 16) inside scissor: 50% red blended over (0, 200, 0) -> (~128, ~100, 0).
	ri := (16*pw + 24) * 4
	if swapPix[ri] < 120 || swapPix[ri] > 136 || swapPix[ri+1] < 92 || swapPix[ri+1] > 108 {
		t.Errorf("right blended pixel = %v, want ~[128 100 0 255]", swapPix[ri:ri+4])
	}
}

// TestD3D11GPUDeviceRemovedAndRecovery verifies device-removed reporting, resource release, and clean generation-2 recovery on Direct3D 11.
func TestD3D11GPUDeviceRemovedAndRecovery(t *testing.T) {
	if !gpuAvailable() {
		t.Skip("Direct3D 11 or D3DCompile unavailable")
	}
	var events []*driver.GPUEvent
	ctrl := newD3DGPUController(&d3d11NativeOps{scaleFn: func() float32 { return 1 }}, func(ev *driver.GPUEvent) {
		events = append(events, ev)
	})
	defer ctrl.Release()

	ctrl.Configure(driver.GPUViewConfig{DepthFormat: driver.GPUFormatDepth32Float, SampleCount: 1})
	if !ctrl.MountOrResize(16, 16) {
		t.Fatal("initial mount failed")
	}
	ctrl.NotifyDeviceLost(dxgiHRErrDeviceRemoved, "simulated DXGI_ERROR_DEVICE_REMOVED")
	if ctrl.swap != nil || !ctrl.lost {
		t.Fatalf("expected swap released and lost=true after NotifyDeviceLost")
	}
	if !ctrl.Recover() {
		t.Fatal("Recover failed")
	}
	if ctrl.generation != 2 || !ctrl.ready || ctrl.lost {
		t.Fatalf("post-recovery state: gen=%d ready=%v lost=%v", ctrl.generation, ctrl.ready, ctrl.lost)
	}
	// Clear + readback on generation 2 must succeed cleanly.
	ctrl.Submit(driver.GPUFrame{
		Generation: 2,
		Ops:        []driver.GPUResourceOp{{Kind: driver.GPUOpReadTexture, ReadbackID: 7, ReadbackSurface: true}},
		Passes: []driver.GPUPass{{
			ColorAttachments: []driver.GPUColorAttachment{{LoadOp: driver.GPULoadClear, StoreOp: driver.GPUStoreStore, ClearColor: driver.RGB(0x112233)}},
		}},
		Present: true,
	})
	var got []byte
	for _, ev := range events {
		if ev.Kind == driver.GPUEventReadback && ev.ReadbackID == 7 {
			got = ev.Pixels
		}
	}
	if len(got) != 16*16*4 || got[0] != 0x11 || got[1] != 0x22 || got[2] != 0x33 || got[3] != 0xff {
		t.Fatalf("post-recovery pixel = %v, want [17 34 51 255]", got[:min(4, len(got))])
	}
}
