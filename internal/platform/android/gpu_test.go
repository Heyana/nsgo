package android

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

const testTexturedLitWGSL = `
struct Camera {
	mvp: mat4x4<f32>,
}

@group(0) @binding(0) var<uniform> cam: Camera;
@group(1) @binding(0) var albedoTex: texture_2d<f32>;
@group(1) @binding(1) var albedoSamp: sampler;

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
	out.pos = cam.mvp * vec4<f32>(in.pos, 1.0);
	out.uv = in.uv;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	return textureSample(albedoTex, albedoSamp, in.uv);
}
`

func TestGPUPipelineCompilesGLSLES300AndSamplerTable(t *testing.T) {
	prog, err := wgsl.CompilePipeline(testTexturedLitWGSL)
	if err != nil {
		t.Fatalf("wgsl.CompilePipeline: %v", err)
	}
	src, err := compileGPUPipeline(&driver.GPUPipelineDesc{
		Vertex:   driver.GPUShaderStage{IR: prog.IR, EntryPoint: "vs_main"},
		Fragment: driver.GPUShaderStage{IR: prog.IR, EntryPoint: "fs_main"},
	})
	if err != nil {
		t.Fatalf("compileGPUPipeline: %v", err)
	}
	if !strings.HasPrefix(src.VertexSource, "#version 300 es\n") {
		t.Errorf("vertex source missing #version 300 es header:\n%s", src.VertexSource)
	}
	if !strings.Contains(src.VertexSource, "gl_Position.z = gl_Position.z * 2.0 - gl_Position.w;") {
		t.Errorf("vertex source missing OpenGL clip-space Z remap:\n%s", src.VertexSource)
	}
	if !strings.HasPrefix(src.FragmentSource, "#version 300 es\n") {
		t.Errorf("fragment source missing #version 300 es header:\n%s", src.FragmentSource)
	}
	if len(src.UniformBlocks) != 1 {
		t.Fatalf("expected 1 uniform block, got %+v", src.UniformBlocks)
	}
	ub := src.UniformBlocks[0]
	if ub.Group != 0 || ub.Binding != 0 || ub.Slot != 0 || ub.BlockName == "" {
		t.Errorf("unexpected uniform block slot: %+v", ub)
	}
	if len(src.SamplerPairs) != 1 {
		t.Fatalf("expected 1 sampler pair, got %+v", src.SamplerPairs)
	}
	sp := src.SamplerPairs[0]
	if sp.Unit != 0 || sp.TextureGroup != 1 || sp.TextureBinding != 0 || sp.SamplerGroup != 1 || sp.SamplerBinding != 1 || sp.Name == "" {
		t.Errorf("unexpected sampler pair: %+v", sp)
	}
	if !strings.Contains(src.FragmentSource, "uniform sampler2D "+sp.Name+";") {
		t.Errorf("fragment source does not declare combined sampler %s:\n%s", sp.Name, src.FragmentSource)
	}

	// Separate vertex and fragment IR modules should compile and merge binding tables cleanly.
	prog2, err := wgsl.CompilePipeline(testTexturedLitWGSL)
	if err != nil {
		t.Fatalf("wgsl.CompilePipeline second module: %v", err)
	}
	merged, err := compileGPUPipeline(&driver.GPUPipelineDesc{
		Vertex:   driver.GPUShaderStage{IR: prog.IR, EntryPoint: "vs_main"},
		Fragment: driver.GPUShaderStage{IR: prog2.IR, EntryPoint: "fs_main"},
	})
	if err != nil {
		t.Fatalf("compileGPUPipeline separate stages: %v", err)
	}
	if len(merged.UniformBlocks) != 1 || len(merged.SamplerPairs) != 1 {
		t.Errorf("merged bindings deduplication failed: ubs=%+v sps=%+v", merged.UniformBlocks, merged.SamplerPairs)
	}

	// Depth-only pipeline (nil Fragment IR) clears FragmentSource cleanly.
	depthOnly, err := compileGPUPipeline(&driver.GPUPipelineDesc{
		Vertex: driver.GPUShaderStage{IR: prog.IR, EntryPoint: "vs_main"},
	})
	if err != nil {
		t.Fatalf("compileGPUPipeline depth-only: %v", err)
	}
	if depthOnly.FragmentSource != "" || depthOnly.VertexSource == "" {
		t.Errorf("depth-only pipeline: vs=%d bytes fs=%d bytes", len(depthOnly.VertexSource), len(depthOnly.FragmentSource))
	}

	// Invalid descriptors return errors.
	if _, err := compileGPUPipeline(nil); err == nil {
		t.Error("expected error for nil pipeline descriptor")
	}
	if _, err := compileGPUPipeline(&driver.GPUPipelineDesc{}); err == nil {
		t.Error("expected error for pipeline with nil vertex IR")
	}
}

func TestGPUFrameBinaryEncodingRoundTrip(t *testing.T) {
	prog, err := wgsl.CompilePipeline(testTexturedLitWGSL)
	if err != nil {
		t.Fatalf("wgsl.CompilePipeline: %v", err)
	}

	frame := driver.GPUFrame{
		SurfaceID:  2,
		ViewID:     42,
		Generation: 7,
		Present:    true,
		Ops: []driver.GPUResourceOp{
			{
				Kind: driver.GPUOpCreateBuffer,
				ID:   10,
				Buffer: &driver.GPUBufferDesc{
					Size:  64,
					Usage: driver.GPUBufferVertex | driver.GPUBufferCopyDst,
					Data:  []byte{1, 2, 3, 4},
				},
			},
			{
				Kind:   driver.GPUOpWriteBuffer,
				ID:     10,
				Offset: 4,
				Data:   []byte{5, 6, 7, 8},
			},
			{
				Kind: driver.GPUOpCreateTexture,
				ID:   20,
				Texture: &driver.GPUTextureDesc{
					Dimension:   driver.GPUDim2D,
					Width:       128,
					Height:      64,
					Layers:      1,
					MipLevels:   4,
					SampleCount: 1,
					Format:      driver.GPUFormatRGBA8Unorm,
					Usage:       driver.GPUTextureSampled | driver.GPUTextureCopyDst | driver.GPUTextureRenderAttachment,
				},
			},
			{
				Kind: driver.GPUOpWriteTexture,
				ID:   20,
				TextureDst: driver.GPUTextureCopy{
					Texture:    20,
					MipLevel:   1,
					ArrayLayer: 0,
					OriginX:    2,
					OriginY:    3,
				},
				DataLayout: driver.GPUDataLayout{
					Offset:       8,
					BytesPerRow:  16,
					RowsPerImage: 2,
				},
				Data: bytes.Repeat([]byte{0xab}, 40),
			},
			{
				Kind: driver.GPUOpCreateSampler,
				ID:   30,
				Sampler: &driver.GPUSamplerDesc{
					MinFilter:     driver.GPUFilterLinear,
					MagFilter:     driver.GPUFilterLinear,
					MipmapFilter:  driver.GPUFilterNearest,
					AddressU:      driver.GPUAddressRepeat,
					AddressV:      driver.GPUAddressMirrorRepeat,
					AddressW:      driver.GPUAddressClampToEdge,
					Compare:       driver.GPUCompareLessEqual,
					MaxAnisotropy: 8,
				},
			},
			{
				Kind: driver.GPUOpCreateBindGroupLayout,
				ID:   40,
				BindGroupLayout: &driver.GPUBindGroupLayoutDesc{
					Entries: []driver.GPUBindGroupLayoutEntry{
						{Binding: 0, Visibility: driver.GPUStageVertex, Type: driver.GPUBindingUniform, HasDynamicOffset: true},
						{Binding: 1, Visibility: driver.GPUStageFragment, Type: driver.GPUBindingTexture},
					},
				},
			},
			{
				Kind: driver.GPUOpCreateBindGroup,
				ID:   50,
				BindGroup: &driver.GPUBindGroupDesc{
					Layout: 40,
					Entries: []driver.GPUBindGroupEntry{
						{Binding: 0, Buffer: 10, Offset: 0, Size: 64},
						{Binding: 1, Texture: 20, Sampler: 30},
					},
				},
			},
			{
				Kind: driver.GPUOpCreatePipeline,
				ID:   60,
				Pipeline: &driver.GPUPipelineDesc{
					Vertex:   driver.GPUShaderStage{IR: prog.IR, EntryPoint: "vs_main"},
					Fragment: driver.GPUShaderStage{IR: prog.IR, EntryPoint: "fs_main"},
					VertexBuffers: []driver.GPUVertexBufferLayout{{
						Stride:   20,
						StepMode: driver.GPUStepVertex,
						Attributes: []driver.GPUVertexAttribute{
							{Format: driver.GPUVertexFloat32x3, Offset: 0, Location: 0},
							{Format: driver.GPUVertexFloat32x2, Offset: 12, Location: 1},
						},
					}},
					Primitive: driver.GPUPrimitiveState{
						Topology:  driver.GPUTopologyTriangleList,
						FrontFace: driver.GPUFrontCCW,
						CullMode:  driver.GPUCullBack,
					},
					DepthStencil: &driver.GPUDepthStencilState{
						Format:              driver.GPUFormatDepth24Plus,
						DepthWrite:          true,
						DepthCompare:        driver.GPUCompareLess,
						DepthBias:           -2,
						DepthBiasSlopeScale: 1.5,
					},
					Targets: []driver.GPUColorTargetState{{
						Format:    driver.GPUFormatRGBA8Unorm,
						WriteMask: driver.GPUColorWriteAll,
						Blend: &driver.GPUBlendState{
							Color: driver.GPUBlendComponent{
								SrcFactor: driver.GPUBlendSrcAlpha,
								DstFactor: driver.GPUBlendOneMinusSrcAlpha,
								Operation: driver.GPUBlendAdd,
							},
							Alpha: driver.GPUBlendComponent{
								SrcFactor: driver.GPUBlendOne,
								DstFactor: driver.GPUBlendZero,
								Operation: driver.GPUBlendAdd,
							},
						},
					}},
					SampleCount: 1,
				},
			},
			{
				Kind:            driver.GPUOpReadTexture,
				ID:              20,
				ReadbackID:      99,
				ReadbackSurface: false,
				ReadbackRegion:  driver.GPUTextureRegion{MipLevel: 0, ArrayLayer: 0, X: 1, Y: 2, Width: 32, Height: 16},
			},
			{Kind: driver.GPUOpDestroyPipeline, ID: 60},
			{Kind: driver.GPUOpDestroyBindGroup, ID: 50},
			{Kind: driver.GPUOpDestroyBindGroupLayout, ID: 40},
			{Kind: driver.GPUOpDestroySampler, ID: 30},
			{Kind: driver.GPUOpDestroyTexture, ID: 20},
			{Kind: driver.GPUOpDestroyBuffer, ID: 10},
		},
		Copies: []driver.GPUCopyOp{
			{Kind: driver.GPUCopyBufferToBuffer, SrcID: 10, DstID: 11, SrcOffset: 4, DstOffset: 8, Size: 32},
			{
				Kind: driver.GPUCopyBufferToTexture, SrcID: 10, DstID: 20, Width: 8, Height: 4, Layers: 1,
				SrcLayout: driver.GPUDataLayout{Offset: 16, BytesPerRow: 32, RowsPerImage: 4},
				DstTex:    driver.GPUTextureCopy{MipLevel: 0, ArrayLayer: 0, OriginX: 1, OriginY: 2},
			},
			{
				Kind: driver.GPUCopyTextureToTexture, SrcID: 20, DstID: 21, Width: 16, Height: 16, Layers: 1,
				SrcTex: driver.GPUTextureCopy{MipLevel: 0, ArrayLayer: 0, OriginX: 0, OriginY: 0},
				DstTex: driver.GPUTextureCopy{MipLevel: 1, ArrayLayer: 0, OriginX: 4, OriginY: 4},
			},
		},
		Passes: []driver.GPUPass{{
			ColorAttachments: []driver.GPUColorAttachment{{
				Texture:    0,
				LoadOp:     driver.GPULoadClear,
				StoreOp:    driver.GPUStoreStore,
				ClearColor: driver.RGB(0x112233),
			}},
			DepthStencil: &driver.GPUDepthStencilAttachment{
				Texture:        0,
				DepthLoadOp:    driver.GPULoadClear,
				DepthStoreOp:   driver.GPUStoreDiscard,
				ClearDepth:     1.0,
				StencilLoadOp:  driver.GPULoadLoad,
				StencilStoreOp: driver.GPUStoreDiscard,
				ClearStencil:   0,
			},
			Commands: []driver.GPUCommand{
				{Kind: driver.GPUCmdSetViewport, F0: 0, F1: 0, F2: 320, F3: 240, F4: 0, F5: 1},
				{Kind: driver.GPUCmdSetScissor, I0: 10, I1: 20, I2: 100, I3: 80},
				{Kind: driver.GPUCmdSetPipeline, ID: 60},
				{Kind: driver.GPUCmdSetBindGroup, Slot: 0, ID: 50, DynamicOffsets: []uint32{256}},
				{Kind: driver.GPUCmdSetVertexBuffer, Slot: 0, ID: 10, Offset: 16},
				{Kind: driver.GPUCmdSetIndexBuffer, ID: 11, IndexFormat: driver.GPUIndexUint16, Offset: 8},
				{Kind: driver.GPUCmdSetStencilReference, U0: 3},
				{Kind: driver.GPUCmdSetBlendConstant, F0: 0.25, F1: 0.5, F2: 0.75, F3: 1.0},
				{Kind: driver.GPUCmdDraw, U0: 6, U1: 0},
				{Kind: driver.GPUCmdDrawIndexed, U0: 12, U1: 0, I0: -1},
				{Kind: driver.GPUCmdDrawInstanced, U0: 3, U1: 4, U2: 0, U3: 2},
				{Kind: driver.GPUCmdDrawIndexedInstanced, U0: 36, U1: 8, U2: 0, U3: 1, I0: 4},
			},
		}},
	}

	var w writer
	if err := encodeGPUFrame(&w, frame); err != nil {
		t.Fatalf("encodeGPUFrame: %v", err)
	}
	if err := checkMessage(22, len(w.buf)); err != nil {
		t.Fatalf("frame exceeds maxMessage: %v", err)
	}

	// Decode every field in the exact order GpuView.java reads them.
	r := reader{buf: w.buf}
	if surf, view, gen, flags := r.u32(), r.u32(), r.u32(), r.byte(); surf != 2 || view != 42 || gen != 7 || flags != 1 {
		t.Fatalf("header: surf=%d view=%d gen=%d flags=%b", surf, view, gen, flags)
	}

	if opCount := r.u32(); opCount != uint32(len(frame.Ops)) {
		t.Fatalf("opCount=%d want %d", opCount, len(frame.Ops))
	}
	// Op 0: CreateBuffer
	if k, id, sz, usage, data := r.byte(), r.u64(), r.u64(), r.u32(), r.bytes(); k != byte(driver.GPUOpCreateBuffer) || id != 10 || sz != 64 || usage != uint32(driver.GPUBufferVertex|driver.GPUBufferCopyDst) || !bytes.Equal(data, []byte{1, 2, 3, 4}) {
		t.Fatalf("op 0 mismatch: k=%d id=%d sz=%d usage=%d data=%v", k, id, sz, usage, data)
	}
	// Op 1: WriteBuffer
	if k, id, off, data := r.byte(), r.u64(), r.u64(), r.bytes(); k != byte(driver.GPUOpWriteBuffer) || id != 10 || off != 4 || !bytes.Equal(data, []byte{5, 6, 7, 8}) {
		t.Fatalf("op 1 mismatch: k=%d id=%d off=%d data=%v", k, id, off, data)
	}
	// Op 2: CreateTexture
	if k, id, dim, tw, th, layers, mips, samples, fmtVal, usage := r.byte(), r.u64(), r.byte(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(), r.byte(), r.u32(); k != byte(driver.GPUOpCreateTexture) || id != 20 || dim != byte(driver.GPUDim2D) || tw != 128 || th != 64 || layers != 1 || mips != 4 || samples != 1 || fmtVal != byte(driver.GPUFormatRGBA8Unorm) || usage != uint32(driver.GPUTextureSampled|driver.GPUTextureCopyDst|driver.GPUTextureRenderAttachment) {
		t.Fatalf("op 2 mismatch")
	}
	// Op 3: WriteTexture
	if k, id, mip, layer, ox, oy, off, bpr, rpi, data := r.byte(), r.u64(), r.u32(), r.u32(), r.u32(), r.u32(), r.u64(), r.u32(), r.u32(), r.bytes(); k != byte(driver.GPUOpWriteTexture) || id != 20 || mip != 1 || layer != 0 || ox != 2 || oy != 3 || off != 8 || bpr != 16 || rpi != 2 || len(data) != 40 {
		t.Fatalf("op 3 mismatch")
	}
	// Op 4: CreateSampler
	if k, id := r.byte(), r.u64(); k != byte(driver.GPUOpCreateSampler) || id != 30 {
		t.Fatalf("op 4 header mismatch")
	}
	if minF, magF, mipF, au, av, aw, cmp := r.byte(), r.byte(), r.byte(), r.byte(), r.byte(), r.byte(), r.byte(); minF != byte(driver.GPUFilterLinear) || magF != byte(driver.GPUFilterLinear) || mipF != byte(driver.GPUFilterNearest) || au != byte(driver.GPUAddressRepeat) || av != byte(driver.GPUAddressMirrorRepeat) || aw != byte(driver.GPUAddressClampToEdge) || cmp != byte(driver.GPUCompareLessEqual) {
		t.Fatalf("op 4 fields mismatch")
	}
	if aniso := uint16(r.byte())<<8 | uint16(r.byte()); aniso != 8 {
		t.Fatalf("op 4 anisotropy=%d want 8", aniso)
	}
	// Op 5: CreateBindGroupLayout
	if k, id, n := r.byte(), r.u64(), r.u32(); k != byte(driver.GPUOpCreateBindGroupLayout) || id != 40 || n != 2 {
		t.Fatalf("op 5 header mismatch")
	}
	if b0, v0, t0, f0 := r.u32(), r.byte(), r.byte(), r.byte(); b0 != 0 || v0 != byte(driver.GPUStageVertex) || t0 != byte(driver.GPUBindingUniform) || f0 != 1 {
		t.Fatalf("op 5 entry 0 mismatch")
	}
	if b1, v1, t1, f1 := r.u32(), r.byte(), r.byte(), r.byte(); b1 != 1 || v1 != byte(driver.GPUStageFragment) || t1 != byte(driver.GPUBindingTexture) || f1 != 0 {
		t.Fatalf("op 5 entry 1 mismatch")
	}
	// Op 6: CreateBindGroup
	if k, id, layout, n := r.byte(), r.u64(), r.u64(), r.u32(); k != byte(driver.GPUOpCreateBindGroup) || id != 50 || layout != 40 || n != 2 {
		t.Fatalf("op 6 header mismatch")
	}
	if b0, buf0, off0, sz0, tex0, samp0 := r.u32(), r.u64(), r.u64(), r.u64(), r.u64(), r.u64(); b0 != 0 || buf0 != 10 || off0 != 0 || sz0 != 64 || tex0 != 0 || samp0 != 0 {
		t.Fatalf("op 6 entry 0 mismatch")
	}
	if b1, buf1, off1, sz1, tex1, samp1 := r.u32(), r.u64(), r.u64(), r.u64(), r.u64(), r.u64(); b1 != 1 || buf1 != 0 || off1 != 0 || sz1 != 0 || tex1 != 20 || samp1 != 30 {
		t.Fatalf("op 6 entry 1 mismatch")
	}
	// Op 7: CreatePipeline
	if k, id := r.byte(), r.u64(); k != byte(driver.GPUOpCreatePipeline) || id != 60 {
		t.Fatalf("op 7 header mismatch")
	}
	vsSrc, fsSrc := r.str(), r.str()
	if !strings.Contains(vsSrc, "#version 300 es") || !strings.Contains(fsSrc, "#version 300 es") {
		t.Fatalf("op 7 shader sources not GLSL ES 3.00")
	}
	if ubCount := r.u32(); ubCount != 1 {
		t.Fatalf("op 7 ubCount=%d want 1", ubCount)
	}
	if g, b, name, slot := r.u32(), r.u32(), r.str(), r.u32(); g != 0 || b != 0 || name == "" || slot != 0 {
		t.Fatalf("op 7 ub0 mismatch: g=%d b=%d name=%q slot=%d", g, b, name, slot)
	}
	if spCount := r.u32(); spCount != 1 {
		t.Fatalf("op 7 spCount=%d want 1", spCount)
	}
	if name, unit, tg, tb, sg, sb := r.str(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(); name == "" || unit != 0 || tg != 1 || tb != 0 || sg != 1 || sb != 1 {
		t.Fatalf("op 7 sp0 mismatch: name=%q unit=%d tex=(%d,%d) samp=(%d,%d)", name, unit, tg, tb, sg, sb)
	}
	if vbCount := r.u32(); vbCount != 1 {
		t.Fatalf("op 7 vbCount=%d want 1", vbCount)
	}
	if stride, step, attrCount := r.u64(), r.byte(), r.u32(); stride != 20 || step != byte(driver.GPUStepVertex) || attrCount != 2 {
		t.Fatalf("op 7 vb0 mismatch")
	}
	if f0, o0, l0 := r.byte(), r.u64(), r.u32(); f0 != byte(driver.GPUVertexFloat32x3) || o0 != 0 || l0 != 0 {
		t.Fatalf("op 7 attr0 mismatch")
	}
	if f1, o1, l1 := r.byte(), r.u64(), r.u32(); f1 != byte(driver.GPUVertexFloat32x2) || o1 != 12 || l1 != 1 {
		t.Fatalf("op 7 attr1 mismatch")
	}
	if topo, ff, cull := r.byte(), r.byte(), r.byte(); topo != byte(driver.GPUTopologyTriangleList) || ff != byte(driver.GPUFrontCCW) || cull != byte(driver.GPUCullBack) {
		t.Fatalf("op 7 primitive state mismatch")
	}
	if dsFlags := r.byte(); dsFlags != 3 {
		t.Fatalf("op 7 dsFlags=%b want 11", dsFlags)
	}
	if dFmt, dCmp, dBias, dSlope := r.byte(), r.byte(), int32(r.u32()), r.float(); dFmt != byte(driver.GPUFormatDepth24Plus) || dCmp != byte(driver.GPUCompareLess) || dBias != -2 || dSlope != 1.5 {
		t.Fatalf("op 7 depth state mismatch")
	}
	if tCount := r.u32(); tCount != 1 {
		t.Fatalf("op 7 target count=%d", tCount)
	}
	if tFmt, mask, bFlag := r.byte(), r.byte(), r.byte(); tFmt != byte(driver.GPUFormatRGBA8Unorm) || mask != byte(driver.GPUColorWriteAll) || bFlag != 1 {
		t.Fatalf("op 7 target 0 mismatch")
	}
	if sc, dc, oc, sa, da, oa := r.byte(), r.byte(), r.byte(), r.byte(), r.byte(), r.byte(); sc != byte(driver.GPUBlendSrcAlpha) || dc != byte(driver.GPUBlendOneMinusSrcAlpha) || oc != byte(driver.GPUBlendAdd) || sa != byte(driver.GPUBlendOne) || da != byte(driver.GPUBlendZero) || oa != byte(driver.GPUBlendAdd) {
		t.Fatalf("op 7 blend mismatch")
	}
	if samples := r.u32(); samples != 1 {
		t.Fatalf("op 7 sampleCount=%d", samples)
	}
	// Op 8: ReadTexture
	if k, id, rid, surf, mip, layer, x, y, rw, rh := r.byte(), r.u64(), r.u64(), r.byte(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(); k != byte(driver.GPUOpReadTexture) || id != 20 || rid != 99 || surf != 0 || mip != 0 || layer != 0 || x != 1 || y != 2 || rw != 32 || rh != 16 {
		t.Fatalf("op 8 readback mismatch")
	}
	// Ops 9..14: Destroys
	wantDestroys := [][2]uint64{
		{uint64(driver.GPUOpDestroyPipeline), 60},
		{uint64(driver.GPUOpDestroyBindGroup), 50},
		{uint64(driver.GPUOpDestroyBindGroupLayout), 40},
		{uint64(driver.GPUOpDestroySampler), 30},
		{uint64(driver.GPUOpDestroyTexture), 20},
		{uint64(driver.GPUOpDestroyBuffer), 10},
	}
	for i, wd := range wantDestroys {
		if k, id := r.byte(), r.u64(); uint64(k) != wd[0] || id != wd[1] {
			t.Fatalf("destroy %d: k=%d id=%d want %v", i, k, id, wd)
		}
	}

	// Copies
	if copyCount := r.u32(); copyCount != 3 {
		t.Fatalf("copyCount=%d want 3", copyCount)
	}
	if k, s, d, so, do, sz := r.byte(), r.u64(), r.u64(), r.u64(), r.u64(), r.u64(); k != byte(driver.GPUCopyBufferToBuffer) || s != 10 || d != 11 || so != 4 || do != 8 || sz != 32 {
		t.Fatalf("copy 0 mismatch")
	}
	if k, s, d, off, bpr, rpi, mip, layer, ox, oy, cw, ch, cl := r.byte(), r.u64(), r.u64(), r.u64(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(); k != byte(driver.GPUCopyBufferToTexture) || s != 10 || d != 20 || off != 16 || bpr != 32 || rpi != 4 || mip != 0 || layer != 0 || ox != 1 || oy != 2 || cw != 8 || ch != 4 || cl != 1 {
		t.Fatalf("copy 1 mismatch")
	}
	if k, s, d, sm, sl, sx, sy, dm, dl, dx, dy, cw, ch, cl := r.byte(), r.u64(), r.u64(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(), r.u32(); k != byte(driver.GPUCopyTextureToTexture) || s != 20 || d != 21 || sm != 0 || sl != 0 || sx != 0 || sy != 0 || dm != 1 || dl != 0 || dx != 4 || dy != 4 || cw != 16 || ch != 16 || cl != 1 {
		t.Fatalf("copy 2 mismatch")
	}

	// Render pass
	if passCount := r.u32(); passCount != 1 {
		t.Fatalf("passCount=%d want 1", passCount)
	}
	if caCount := r.u32(); caCount != 1 {
		t.Fatalf("caCount=%d want 1", caCount)
	}
	if tex, res, mip, layer, load, store, clr := r.u64(), r.u64(), r.u32(), r.u32(), r.byte(), r.byte(), r.u32(); tex != 0 || res != 0 || mip != 0 || layer != 0 || load != byte(driver.GPULoadClear) || store != byte(driver.GPUStoreStore) || clr != uint32(driver.RGB(0x112233)) {
		t.Fatalf("color attachment mismatch")
	}
	if hasDS := r.byte(); hasDS != 1 {
		t.Fatalf("hasDS=%d want 1", hasDS)
	}
	if tex, dl, ds, cd, sl, ss, cs := r.u64(), r.byte(), r.byte(), r.float(), r.byte(), r.byte(), r.u32(); tex != 0 || dl != byte(driver.GPULoadClear) || ds != byte(driver.GPUStoreDiscard) || cd != 1.0 || sl != byte(driver.GPULoadLoad) || ss != byte(driver.GPUStoreDiscard) || cs != 0 {
		t.Fatalf("depth attachment mismatch")
	}
	if cmdCount := r.u32(); cmdCount != 12 {
		t.Fatalf("cmdCount=%d want 12", cmdCount)
	}
	// Verify all 12 commands consume exact bytes.
	if k, f0, f1, f2, f3, f4, f5 := r.byte(), r.float(), r.float(), r.float(), r.float(), r.float(), r.float(); k != byte(driver.GPUCmdSetViewport) || f0 != 0 || f1 != 0 || f2 != 320 || f3 != 240 || f4 != 0 || f5 != 1 {
		t.Fatalf("cmd 0 viewport mismatch")
	}
	if k, i0, i1, i2, i3 := r.byte(), int32(r.u32()), int32(r.u32()), int32(r.u32()), int32(r.u32()); k != byte(driver.GPUCmdSetScissor) || i0 != 10 || i1 != 20 || i2 != 100 || i3 != 80 {
		t.Fatalf("cmd 1 scissor mismatch")
	}
	if k, id := r.byte(), r.u64(); k != byte(driver.GPUCmdSetPipeline) || id != 60 {
		t.Fatalf("cmd 2 pipeline mismatch")
	}
	if k, slot, id, dn, d0 := r.byte(), r.u32(), r.u64(), r.u32(), r.u32(); k != byte(driver.GPUCmdSetBindGroup) || slot != 0 || id != 50 || dn != 1 || d0 != 256 {
		t.Fatalf("cmd 3 bind group mismatch")
	}
	if k, slot, id, off := r.byte(), r.u32(), r.u64(), r.u64(); k != byte(driver.GPUCmdSetVertexBuffer) || slot != 0 || id != 10 || off != 16 {
		t.Fatalf("cmd 4 vertex buffer mismatch")
	}
	if k, id, ifmt, off := r.byte(), r.u64(), r.byte(), r.u64(); k != byte(driver.GPUCmdSetIndexBuffer) || id != 11 || ifmt != byte(driver.GPUIndexUint16) || off != 8 {
		t.Fatalf("cmd 5 index buffer mismatch")
	}
	if k, ref := r.byte(), r.u32(); k != byte(driver.GPUCmdSetStencilReference) || ref != 3 {
		t.Fatalf("cmd 6 stencil ref mismatch")
	}
	if k, f0, f1, f2, f3 := r.byte(), r.float(), r.float(), r.float(), r.float(); k != byte(driver.GPUCmdSetBlendConstant) || f0 != 0.25 || f1 != 0.5 || f2 != 0.75 || f3 != 1.0 {
		t.Fatalf("cmd 7 blend constant mismatch")
	}
	if k, u0, u1 := r.byte(), r.u32(), r.u32(); k != byte(driver.GPUCmdDraw) || u0 != 6 || u1 != 0 {
		t.Fatalf("cmd 8 draw mismatch")
	}
	if k, u0, u1, i0 := r.byte(), r.u32(), r.u32(), int32(r.u32()); k != byte(driver.GPUCmdDrawIndexed) || u0 != 12 || u1 != 0 || i0 != -1 {
		t.Fatalf("cmd 9 draw indexed mismatch")
	}
	if k, u0, u1, u2, u3 := r.byte(), r.u32(), r.u32(), r.u32(), r.u32(); k != byte(driver.GPUCmdDrawInstanced) || u0 != 3 || u1 != 4 || u2 != 0 || u3 != 2 {
		t.Fatalf("cmd 10 draw instanced mismatch")
	}
	if k, u0, u1, u2, u3, i0 := r.byte(), r.u32(), r.u32(), r.u32(), r.u32(), int32(r.u32()); k != byte(driver.GPUCmdDrawIndexedInstanced) || u0 != 36 || u1 != 8 || u2 != 0 || u3 != 1 || i0 != 4 {
		t.Fatalf("cmd 11 draw indexed instanced mismatch")
	}
	if len(r.buf) != 0 {
		t.Fatalf("%d trailing bytes left in GPUFrame payload", len(r.buf))
	}
}

func TestGPUFrameSplitsLargeUploadsUnderMaxMessage(t *testing.T) {
	big := make([]byte, maxGPUChunk*2+1024) // 32 MiB + 1 KiB
	big[0] = 0x11
	big[maxGPUChunk] = 0x22
	big[len(big)-1] = 0x33

	frame := driver.GPUFrame{
		SurfaceID:  0,
		ViewID:     5,
		Generation: 1,
		Present:    true,
		Ops: []driver.GPUResourceOp{
			{
				Kind: driver.GPUOpCreateBuffer,
				ID:   1,
				Buffer: &driver.GPUBufferDesc{
					Size:  uint64(len(big)),
					Usage: driver.GPUBufferVertex | driver.GPUBufferCopyDst,
					Data:  big,
				},
			},
		},
		Passes: []driver.GPUPass{{
			ColorAttachments: []driver.GPUColorAttachment{{LoadOp: driver.GPULoadClear}},
		}},
	}

	chunks := splitGPUFrames(frame)
	if len(chunks) < 2 {
		t.Fatalf("expected large buffer upload to split into >= 2 frames, got %d", len(chunks))
	}
	reassembled := make([]byte, len(big))
	sawCreate := false
	for i, ch := range chunks {
		var w writer
		if err := encodeGPUFrame(&w, ch); err != nil {
			t.Fatalf("chunk %d encodeGPUFrame: %v", i, err)
		}
		if err := checkMessage(22, len(w.buf)); err != nil {
			t.Fatalf("chunk %d exceeds maxMessage: %v", i, err)
		}
		if i < len(chunks)-1 {
			if ch.Present || len(ch.Passes) != 0 {
				t.Errorf("intermediate chunk %d must not present or execute passes", i)
			}
		} else {
			if !ch.Present || len(ch.Passes) != 1 {
				t.Errorf("final chunk must keep Present=true and Passes")
			}
		}
		for _, op := range ch.Ops {
			switch op.Kind {
			case driver.GPUOpCreateBuffer:
				sawCreate = true
				if len(op.Buffer.Data) != 0 || op.Buffer.Size != uint64(len(big)) {
					t.Errorf("expanded CreateBuffer should allocate Size without inline Data")
				}
			case driver.GPUOpWriteBuffer:
				copy(reassembled[op.Offset:], op.Data)
			}
		}
	}
	if !sawCreate {
		t.Fatal("missing CreateBuffer op")
	}
	if !bytes.Equal(reassembled, big) {
		t.Fatal("chunked buffer uploads did not reassemble identically")
	}
}

func TestGPUFrameSplitsLargeTextureUploads(t *testing.T) {
	const (
		width  = 4096
		height = 2048
		bpr    = width * 4 // 16 KiB per row -> 32 MiB total
	)
	pixels := make([]byte, bpr*height)
	pixels[0] = 0xaa
	pixels[bpr*1024+7] = 0xbb
	pixels[len(pixels)-1] = 0xcc

	frame := driver.GPUFrame{
		SurfaceID:  0,
		ViewID:     6,
		Generation: 1,
		Present:    true,
		Ops: []driver.GPUResourceOp{{
			Kind: driver.GPUOpWriteTexture,
			ID:   77,
			TextureDst: driver.GPUTextureCopy{
				Texture:    77,
				MipLevel:   0,
				ArrayLayer: 0,
				OriginX:    0,
				OriginY:    0,
			},
			DataLayout: driver.GPUDataLayout{
				Offset:       0,
				BytesPerRow:  bpr,
				RowsPerImage: height,
			},
			Data: pixels,
		}},
	}

	chunks := splitGPUFrames(frame)
	if len(chunks) < 2 {
		t.Fatalf("expected large texture upload to split into >= 2 chunks, got %d", len(chunks))
	}
	reassembled := make([]byte, len(pixels))
	var rowsCovered uint32
	for i, ch := range chunks {
		var w writer
		if err := encodeGPUFrame(&w, ch); err != nil {
			t.Fatalf("chunk %d encodeGPUFrame: %v", i, err)
		}
		if err := checkMessage(22, len(w.buf)); err != nil {
			t.Fatalf("chunk %d exceeds maxMessage: %v", i, err)
		}
		for _, op := range ch.Ops {
			if op.Kind != driver.GPUOpWriteTexture {
				t.Fatalf("unexpected op kind %d", op.Kind)
			}
			if op.TextureDst.OriginY != rowsCovered {
				t.Fatalf("OriginY=%d want %d", op.TextureDst.OriginY, rowsCovered)
			}
			copy(reassembled[int(op.TextureDst.OriginY)*bpr:], op.Data)
			rowsCovered += op.DataLayout.RowsPerImage
		}
	}
	if rowsCovered != height {
		t.Fatalf("rowsCovered=%d want %d", rowsCovered, height)
	}
	if !bytes.Equal(reassembled, pixels) {
		t.Fatal("chunked texture uploads did not reassemble identically")
	}
}

func TestGPUEventDecoding(t *testing.T) {
	// 1. DeviceReady
	{
		var w writer
		w.u32(1)                                      // surface
		w.u32(7)                                      // viewID
		w.byte(byte(driver.GPUEventDeviceReady))      // kind
		w.u32(2)                                      // generation
		w.u32(1080)                                   // width
		w.u32(2400)                                   // height
		w.byte(byte(driver.GPUFormatRGBA8Unorm))      // surface format
		w.u32(8192)                                   // MaxTextureDimension2D
		w.u64(256 << 20)                              // MaxBufferSize
		w.u64(65536)                                  // MaxUniformBufferSize
		w.u32(4)                                      // MaxBindGroups
		w.u32(8)                                      // MaxVertexBuffers
		w.u32(16)                                     // MaxVertexAttributes
		w.u32(256)                                    // MinUniformBufferAlignment
		w.flags(false, false, true, true, true, true) // MSAA4x, Depth32Float, Anisotropy, Texture3D
		w.u32(2048)                                   // MaxTextureDimension3D
		surf, ev := readGPUEvent(&reader{buf: w.buf})
		ge, ok := ev.Data.(*driver.GPUEvent)
		if !ok || surf != 1 || ev.Kind != driver.EventGPU || ev.ID != 7 {
			t.Fatalf("DeviceReady header: surf=%d ev=%+v", surf, ev)
		}
		if ge.Kind != driver.GPUEventDeviceReady || ge.Generation != 2 || ge.Width != 1080 || ge.Height != 2400 ||
			ge.SurfaceFormat != driver.GPUFormatRGBA8Unorm || ge.Limits.MaxTextureDimension2D != 8192 ||
			ge.Limits.MaxTextureDimension3D != 2048 ||
			ge.Limits.MaxUniformBufferSize != 65536 || ge.Limits.MinUniformBufferAlignment != 256 ||
			ge.Features.StorageBuffers || ge.Features.Compute || !ge.Features.MSAA4x || !ge.Features.Depth32Float || !ge.Features.Anisotropy || !ge.Features.Texture3D {
			t.Fatalf("DeviceReady payload mismatch: %+v", ge)
		}
	}

	// 2. Resize
	{
		var w writer
		w.u32(0)
		w.u32(9)
		w.byte(byte(driver.GPUEventResize))
		w.u32(3)
		w.u32(640)
		w.u32(480)
		surf, ev := readGPUEvent(&reader{buf: w.buf})
		ge := ev.Data.(*driver.GPUEvent)
		if surf != 0 || ev.ID != 9 || ge.Kind != driver.GPUEventResize || ge.Generation != 3 || ge.Width != 640 || ge.Height != 480 {
			t.Fatalf("Resize mismatch: surf=%d ge=%+v", surf, ge)
		}
	}

	// 3. DeviceLost
	{
		var w writer
		w.u32(0)
		w.u32(9)
		w.byte(byte(driver.GPUEventDeviceLost))
		w.u32(3)
		w.byte(byte(driver.GPULostReset))
		w.str("EGL_CONTEXT_LOST")
		_, ev := readGPUEvent(&reader{buf: w.buf})
		ge := ev.Data.(*driver.GPUEvent)
		if ge.Kind != driver.GPUEventDeviceLost || ge.LostReason != driver.GPULostReset || ge.Message != "EGL_CONTEXT_LOST" {
			t.Fatalf("DeviceLost mismatch: %+v", ge)
		}
	}

	// 4. Error
	{
		var w writer
		w.u32(0)
		w.u32(9)
		w.byte(byte(driver.GPUEventError))
		w.u32(3)
		w.byte(byte(driver.GPUErrorShaderCompile))
		w.str("vertex: syntax error")
		_, ev := readGPUEvent(&reader{buf: w.buf})
		ge := ev.Data.(*driver.GPUEvent)
		if ge.Kind != driver.GPUEventError || ge.ErrorCode != driver.GPUErrorShaderCompile || ge.Message != "vertex: syntax error" {
			t.Fatalf("Error mismatch: %+v", ge)
		}
	}

	// 5. Readback
	{
		var w writer
		w.u32(0)
		w.u32(9)
		w.byte(byte(driver.GPUEventReadback))
		w.u32(3)
		w.u64(42)
		w.str("")
		pixels := []byte{255, 0, 0, 255, 0, 255, 0, 255}
		w.u32(uint32(len(pixels)))
		w.buf = append(w.buf, pixels...)
		_, ev := readGPUEvent(&reader{buf: w.buf})
		ge := ev.Data.(*driver.GPUEvent)
		if ge.Kind != driver.GPUEventReadback || ge.ReadbackID != 42 || ge.Message != "" || !bytes.Equal(ge.Pixels, pixels) {
			t.Fatalf("Readback mismatch: %+v", ge)
		}
	}
}

func TestReadEnvironmentGPUFlag(t *testing.T) {
	// GPU support is the third capability byte's capGPU bit, after the locale settings; a runtime built before that byte reports no GPU view.
	buildEnv := func(third []byte) driver.Environment {
		var w writer
		writeBaseEnvironment(&w, "en-US")
		w.byte(1)
		w.str("h23")
		w.str("metric")
		w.str("celsius")
		w.str("US")
		w.buf = append(w.buf, third...)
		return readEnvironment(&reader{buf: w.buf})
	}
	for _, c := range []struct {
		third []byte
		want  bool
	}{{nil, false}, {[]byte{0}, false}, {[]byte{capExtendedMessages}, false}, {[]byte{capGPU}, true}, {[]byte{capExtendedMessages | capGPU}, true}} {
		if env := buildEnv(c.third); env.GPU != c.want {
			t.Errorf("third byte %v: GPU = %v, want %v", c.third, env.GPU, c.want)
		}
	}
}

func TestWriteGPUViewConfig(t *testing.T) {
	var w writer
	writeGPUViewConfig(&w, &driver.GPUViewConfig{
		ColorFormat: driver.GPUFormatRGBA8Unorm,
		DepthFormat: driver.GPUFormatDepth24Plus,
		SampleCount: 4,
	})
	r := reader{buf: w.buf}
	if cf, df, sc := r.byte(), r.byte(), r.u32(); cf != byte(driver.GPUFormatRGBA8Unorm) || df != byte(driver.GPUFormatDepth24Plus) || sc != 4 || len(r.buf) != 0 {
		t.Fatalf("config: cf=%d df=%d sc=%d left=%d", cf, df, sc, len(r.buf))
	}

	w = writer{}
	writeGPUViewConfig(&w, nil)
	r = reader{buf: w.buf}
	if cf, df, sc := r.byte(), r.byte(), r.u32(); cf != 0 || df != 0 || sc != 1 || len(r.buf) != 0 {
		t.Fatalf("nil config: cf=%d df=%d sc=%d left=%d", cf, df, sc, len(r.buf))
	}
}

func TestJavaGPUConstantsMatchDriver(t *testing.T) {
	testhost.RequireSource(t)
	java, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "GpuView.java"))
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]int{}
	for _, m := range regexp.MustCompile(`static final int (\w+) = (\d+);`).FindAllStringSubmatch(string(java), -1) {
		values[m[1]], _ = strconv.Atoi(m[2])
	}
	for _, m := range regexp.MustCompile(`static final int (\w+) = 1 << (\d+);`).FindAllStringSubmatch(string(java), -1) {
		shift, _ := strconv.Atoi(m[2])
		values[m[1]] = 1 << shift
	}

	want := map[string]int{
		"FORMAT_UNDEFINED": int(driver.GPUFormatUndefined), "FORMAT_RGBA8_UNORM": int(driver.GPUFormatRGBA8Unorm),
		"FORMAT_RGBA8_UNORM_SRGB": int(driver.GPUFormatRGBA8UnormSRGB), "FORMAT_BGRA8_UNORM": int(driver.GPUFormatBGRA8Unorm),
		"FORMAT_RG8_UNORM": int(driver.GPUFormatRG8Unorm), "FORMAT_R8_UNORM": int(driver.GPUFormatR8Unorm),
		"FORMAT_RGBA16_FLOAT": int(driver.GPUFormatRGBA16Float), "FORMAT_DEPTH24_PLUS": int(driver.GPUFormatDepth24Plus),
		"FORMAT_DEPTH24_PLUS_STENCIL8": int(driver.GPUFormatDepth24PlusStencil8), "FORMAT_DEPTH32_FLOAT": int(driver.GPUFormatDepth32Float),
		"DIM_2D": int(driver.GPUDim2D), "DIM_CUBE": int(driver.GPUDimCube), "DIM_2D_ARRAY": int(driver.GPUDim2DArray), "DIM_3D": int(driver.GPUDim3D),
		"BUF_VERTEX": int(driver.GPUBufferVertex), "BUF_INDEX": int(driver.GPUBufferIndex),
		"BUF_UNIFORM": int(driver.GPUBufferUniform), "BUF_COPY_DST": int(driver.GPUBufferCopyDst),
		"FILTER_NEAREST": int(driver.GPUFilterNearest), "FILTER_LINEAR": int(driver.GPUFilterLinear),
		"ADDR_CLAMP_TO_EDGE": int(driver.GPUAddressClampToEdge), "ADDR_REPEAT": int(driver.GPUAddressRepeat),
		"ADDR_MIRROR_REPEAT": int(driver.GPUAddressMirrorRepeat),
		"COMPARE_UNDEFINED":  int(driver.GPUCompareUndefined), "COMPARE_NEVER": int(driver.GPUCompareNever),
		"COMPARE_LESS": int(driver.GPUCompareLess), "COMPARE_EQUAL": int(driver.GPUCompareEqual),
		"COMPARE_LESS_EQUAL": int(driver.GPUCompareLessEqual), "COMPARE_GREATER": int(driver.GPUCompareGreater),
		"COMPARE_NOT_EQUAL": int(driver.GPUCompareNotEqual), "COMPARE_GREATER_EQUAL": int(driver.GPUCompareGreaterEqual),
		"COMPARE_ALWAYS": int(driver.GPUCompareAlways),
		"VTX_FLOAT32":    int(driver.GPUVertexFloat32), "VTX_FLOAT32X2": int(driver.GPUVertexFloat32x2),
		"VTX_FLOAT32X3": int(driver.GPUVertexFloat32x3), "VTX_FLOAT32X4": int(driver.GPUVertexFloat32x4),
		"VTX_SINT32": int(driver.GPUVertexSint32), "VTX_SINT32X2": int(driver.GPUVertexSint32x2),
		"VTX_SINT32X3": int(driver.GPUVertexSint32x3), "VTX_SINT32X4": int(driver.GPUVertexSint32x4),
		"VTX_UINT32": int(driver.GPUVertexUint32), "VTX_UINT32X2": int(driver.GPUVertexUint32x2),
		"VTX_UINT32X3": int(driver.GPUVertexUint32x3), "VTX_UINT32X4": int(driver.GPUVertexUint32x4),
		"VTX_UNORM8X4": int(driver.GPUVertexUnorm8x4),
		"STEP_VERTEX":  int(driver.GPUStepVertex), "STEP_INSTANCE": int(driver.GPUStepInstance),
		"INDEX_UINT16": int(driver.GPUIndexUint16), "INDEX_UINT32": int(driver.GPUIndexUint32),
		"TOPO_TRIANGLE_LIST": int(driver.GPUTopologyTriangleList), "TOPO_TRIANGLE_STRIP": int(driver.GPUTopologyTriangleStrip),
		"TOPO_LINE_LIST": int(driver.GPUTopologyLineList), "TOPO_LINE_STRIP": int(driver.GPUTopologyLineStrip),
		"TOPO_POINT_LIST": int(driver.GPUTopologyPointList),
		"FRONT_CCW":       int(driver.GPUFrontCCW), "FRONT_CW": int(driver.GPUFrontCW),
		"CULL_NONE": int(driver.GPUCullNone), "CULL_FRONT": int(driver.GPUCullFront), "CULL_BACK": int(driver.GPUCullBack),
		"BLEND_ZERO": int(driver.GPUBlendZero), "BLEND_ONE": int(driver.GPUBlendOne),
		"BLEND_SRC": int(driver.GPUBlendSrc), "BLEND_ONE_MINUS_SRC": int(driver.GPUBlendOneMinusSrc),
		"BLEND_SRC_ALPHA": int(driver.GPUBlendSrcAlpha), "BLEND_ONE_MINUS_SRC_ALPHA": int(driver.GPUBlendOneMinusSrcAlpha),
		"BLEND_DST": int(driver.GPUBlendDst), "BLEND_ONE_MINUS_DST": int(driver.GPUBlendOneMinusDst),
		"BLEND_DST_ALPHA": int(driver.GPUBlendDstAlpha), "BLEND_ONE_MINUS_DST_ALPHA": int(driver.GPUBlendOneMinusDstAlpha),
		"BLEND_CONSTANT": int(driver.GPUBlendConstant), "BLEND_ONE_MINUS_CONSTANT": int(driver.GPUBlendOneMinusConstant),
		"BLEND_ADD": int(driver.GPUBlendAdd), "BLEND_SUBTRACT": int(driver.GPUBlendSubtract),
		"BLEND_REVERSE_SUBTRACT": int(driver.GPUBlendReverseSubtract), "BLEND_MIN": int(driver.GPUBlendMin),
		"BLEND_MAX":        int(driver.GPUBlendMax),
		"LOAD_CLEAR":       int(driver.GPULoadClear),
		"OP_CREATE_BUFFER": int(driver.GPUOpCreateBuffer), "OP_WRITE_BUFFER": int(driver.GPUOpWriteBuffer),
		"OP_DESTROY_BUFFER": int(driver.GPUOpDestroyBuffer), "OP_CREATE_TEXTURE": int(driver.GPUOpCreateTexture),
		"OP_WRITE_TEXTURE": int(driver.GPUOpWriteTexture), "OP_DESTROY_TEXTURE": int(driver.GPUOpDestroyTexture),
		"OP_CREATE_SAMPLER": int(driver.GPUOpCreateSampler), "OP_DESTROY_SAMPLER": int(driver.GPUOpDestroySampler),
		"OP_CREATE_BIND_GROUP_LAYOUT":  int(driver.GPUOpCreateBindGroupLayout),
		"OP_DESTROY_BIND_GROUP_LAYOUT": int(driver.GPUOpDestroyBindGroupLayout),
		"OP_CREATE_BIND_GROUP":         int(driver.GPUOpCreateBindGroup), "OP_DESTROY_BIND_GROUP": int(driver.GPUOpDestroyBindGroup),
		"OP_CREATE_PIPELINE": int(driver.GPUOpCreatePipeline), "OP_DESTROY_PIPELINE": int(driver.GPUOpDestroyPipeline),
		"OP_READ_TEXTURE":       int(driver.GPUOpReadTexture),
		"COPY_BUFFER_TO_BUFFER": int(driver.GPUCopyBufferToBuffer), "COPY_BUFFER_TO_TEXTURE": int(driver.GPUCopyBufferToTexture),
		"COPY_TEXTURE_TO_TEXTURE": int(driver.GPUCopyTextureToTexture),
		"CMD_SET_VIEWPORT":        int(driver.GPUCmdSetViewport), "CMD_SET_SCISSOR": int(driver.GPUCmdSetScissor),
		"CMD_SET_PIPELINE": int(driver.GPUCmdSetPipeline), "CMD_SET_BIND_GROUP": int(driver.GPUCmdSetBindGroup),
		"CMD_SET_VERTEX_BUFFER": int(driver.GPUCmdSetVertexBuffer), "CMD_SET_INDEX_BUFFER": int(driver.GPUCmdSetIndexBuffer),
		"CMD_SET_STENCIL_REFERENCE": int(driver.GPUCmdSetStencilReference), "CMD_SET_BLEND_CONSTANT": int(driver.GPUCmdSetBlendConstant),
		"CMD_DRAW": int(driver.GPUCmdDraw), "CMD_DRAW_INDEXED": int(driver.GPUCmdDrawIndexed),
		"CMD_DRAW_INSTANCED": int(driver.GPUCmdDrawInstanced), "CMD_DRAW_INDEXED_INSTANCED": int(driver.GPUCmdDrawIndexedInstanced),
		"EVENT_DEVICE_READY": int(driver.GPUEventDeviceReady), "EVENT_DEVICE_LOST": int(driver.GPUEventDeviceLost),
		"EVENT_RESIZE": int(driver.GPUEventResize), "EVENT_ERROR": int(driver.GPUEventError),
		"EVENT_READBACK": int(driver.GPUEventReadback),
		"LOST_UNKNOWN":   int(driver.GPULostUnknown), "LOST_RESET": int(driver.GPULostReset),
		"LOST_DESTROYED": int(driver.GPULostDestroyed), "LOST_OUT_OF_MEMORY": int(driver.GPULostOutOfMemory),
		"ERR_UNSUPPORTED": int(driver.GPUErrorUnsupported), "ERR_INVALID_DESCRIPTOR": int(driver.GPUErrorInvalidDescriptor),
		"ERR_SHADER_COMPILE": int(driver.GPUErrorShaderCompile), "ERR_OUT_OF_MEMORY": int(driver.GPUErrorOutOfMemory),
		"ERR_DEVICE_LOST": int(driver.GPUErrorDeviceLost), "ERR_FAILED": int(driver.GPUErrorFailed),
	}
	for name, expected := range want {
		got, ok := values[name]
		if !ok || got != expected {
			t.Errorf("GpuView.java %s = %d (found %v), driver = %d", name, got, ok, expected)
		}
	}
}
