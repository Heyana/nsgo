package windows

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/hlsl"
)

func TestD3D11StructSizesMatchABI(t *testing.T) {
	// Verify every D3D11 descriptor struct matches the exact Windows SDK byte size.
	checks := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"D3D11_BUFFER_DESC", unsafe.Sizeof(d3d11BufferDesc{}), 24},
		{"D3D11_TEXTURE2D_DESC", unsafe.Sizeof(d3d11Texture2DDesc{}), 44},
		{"D3D11_SHADER_RESOURCE_VIEW_DESC", unsafe.Sizeof(d3d11SRVDesc{}), 24},
		{"D3D11_RENDER_TARGET_VIEW_DESC", unsafe.Sizeof(d3d11RTVDesc{}), 20},
		{"D3D11_DEPTH_STENCIL_VIEW_DESC", unsafe.Sizeof(d3d11DSVDesc{}), 24},
		{"D3D11_SAMPLER_DESC", unsafe.Sizeof(d3d11SamplerDesc{}), 52},
		{"D3D11_RASTERIZER_DESC", unsafe.Sizeof(d3d11RasterizerDesc{}), 40},
		{"D3D11_DEPTH_STENCILOP_DESC", unsafe.Sizeof(d3d11DepthStencilOpDesc{}), 16},
		{"D3D11_DEPTH_STENCIL_DESC", unsafe.Sizeof(d3d11DepthStencilDesc{}), 52},
		{"D3D11_RENDER_TARGET_BLEND_DESC", unsafe.Sizeof(d3d11RenderTargetBlendDesc{}), 32},
		{"D3D11_BLEND_DESC", unsafe.Sizeof(d3d11BlendDesc{}), 264},
		{"D3D11_BOX", unsafe.Sizeof(d3d11Box{}), 24},
		{"D3D11_VIEWPORT", unsafe.Sizeof(d3d11Viewport{}), 24},
		{"D3D11_RECT", unsafe.Sizeof(d3d11Rect{}), 16},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s sizeof = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestD3D11ConstantBufferAlignmentAndDescriptors(t *testing.T) {
	for _, tc := range []struct {
		in   uint64
		want uint32
	}{
		{0, 16},
		{1, 16},
		{12, 16},
		{16, 16},
		{17, 32},
		{64, 64},
		{80, 80},
		{84, 96},
		{65536, 65536},
		{100000, 65536},
	} {
		if got := d3dAlignConstantBufferSize(tc.in); got != tc.want {
			t.Errorf("d3dAlignConstantBufferSize(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}

	// Uniform-only buffer: 16-byte aligned, D3D11_BIND_CONSTANT_BUFFER alone.
	main, hasCB, cb := d3dBufferDescriptor(driver.GPUBufferDesc{
		Size:  20,
		Usage: driver.GPUBufferUniform | driver.GPUBufferCopyDst,
	})
	if !hasCB || main != cb || cb.byteWidth != 32 || cb.bindFlags != d3d11BindConstantBuffer {
		t.Fatalf("uniform buffer desc = main:%+v hasCB:%v cb:%+v", main, hasCB, cb)
	}

	// Combined Uniform + Vertex buffer: D3D11 forbids mixing CONSTANT_BUFFER with VERTEX_BUFFER, so mainDesc must hold VERTEX_BUFFER and cbDesc must be a separate 16-byte aligned CB.
	main, hasCB, cb = d3dBufferDescriptor(driver.GPUBufferDesc{
		Size:  28,
		Usage: driver.GPUBufferUniform | driver.GPUBufferVertex,
	})
	if !hasCB || main.bindFlags != d3d11BindVertexBuffer || main.byteWidth != 28 {
		t.Fatalf("combined mainDesc = %+v, hasCB=%v", main, hasCB)
	}
	if cb.bindFlags != d3d11BindConstantBuffer || cb.byteWidth != 32 {
		t.Fatalf("combined cbDesc = %+v", cb)
	}

	// Vertex + Index buffer.
	main, hasCB, _ = d3dBufferDescriptor(driver.GPUBufferDesc{
		Size:  128,
		Usage: driver.GPUBufferVertex | driver.GPUBufferIndex,
	})
	if hasCB || main.bindFlags != (d3d11BindVertexBuffer|d3d11BindIndexBuffer) || main.byteWidth != 128 {
		t.Fatalf("vertex+index desc = %+v, hasCB=%v", main, hasCB)
	}

	// Structured storage buffer rounds ByteWidth up to a multiple of StructureByteStride (4).
	main, hasCB, _ = d3dBufferDescriptor(driver.GPUBufferDesc{
		Size:  10,
		Usage: driver.GPUBufferStorage,
	})
	if hasCB || main.bindFlags != d3d11BindShaderResource || main.miscFlags != d3d11MiscBufferStructured || main.structureByteStride != 4 || main.byteWidth != 12 {
		t.Fatalf("storage buffer desc = %+v, hasCB=%v", main, hasCB)
	}
}

func TestD3D11TextureAndDepthDescriptors(t *testing.T) {
	// Standard color texture with render target + sampled.
	td, srv, rtv, dsv := d3dTexture2DDescriptor(driver.GPUTextureDesc{
		Dimension: driver.GPUDim2D,
		Width:     256,
		Height:    128,
		MipLevels: 4,
		Format:    driver.GPUFormatRGBA8Unorm,
		Usage:     driver.GPUTextureSampled | driver.GPUTextureRenderAttachment,
	})
	if td.format != dxgiFmtR8G8B8A8Unorm || td.bindFlags != (d3d11BindShaderResource|d3d11BindRenderTarget) {
		t.Fatalf("color td = %+v", td)
	}
	if srv.viewDimension != d3d11SRVDimTexture2D || srv.u1 != 4 || rtv.viewDimension != d3d11RTVDimTexture2D || dsv.format != 0 {
		t.Fatalf("color views srv=%+v rtv=%+v dsv=%+v", srv, rtv, dsv)
	}

	// Depth32Float render-attachment only -> DXGI_FORMAT_D32_FLOAT.
	td, srv, rtv, dsv = d3dTexture2DDescriptor(driver.GPUTextureDesc{
		Width:  64,
		Height: 64,
		Format: driver.GPUFormatDepth32Float,
		Usage:  driver.GPUTextureRenderAttachment,
	})
	if td.format != dxgiFmtD32Float || dsv.format != dxgiFmtD32Float || srv.format != 0 || rtv.format != 0 {
		t.Fatalf("depth32 td=%+v srv=%+v dsv=%+v", td, srv, dsv)
	}

	// Sampled depth buffer (shadow map) -> R32_TYPELESS resource, R32_FLOAT SRV, D32_FLOAT DSV.
	td, srv, _, dsv = d3dTexture2DDescriptor(driver.GPUTextureDesc{
		Width:  512,
		Height: 512,
		Format: driver.GPUFormatDepth24Plus,
		Usage:  driver.GPUTextureRenderAttachment | driver.GPUTextureSampled,
	})
	if td.format != dxgiFmtR32Typeless || srv.format != dxgiFmtR32Float || dsv.format != dxgiFmtD32Float {
		t.Fatalf("sampled depth td=%+v srv=%+v dsv=%+v", td, srv, dsv)
	}
	if td.bindFlags != (d3d11BindShaderResource | d3d11BindDepthStencil) {
		t.Fatalf("sampled depth bindFlags = %#x", td.bindFlags)
	}

	// Cube texture -> 6 slices + TEXTURECUBE misc flag + TEXTURECUBE SRV.
	td, srv, _, _ = d3dTexture2DDescriptor(driver.GPUTextureDesc{
		Dimension: driver.GPUDimCube,
		Width:     32,
		Height:    32,
		Format:    driver.GPUFormatRGBA16Float,
		Usage:     driver.GPUTextureSampled,
	})
	if td.arraySize != 6 || td.misc&d3d11MiscTextureCube == 0 || srv.viewDimension != d3d11SRVDimTextureCube {
		t.Fatalf("cube td=%+v srv=%+v", td, srv)
	}
}

func TestD3D11InputLayoutSamplerAndPipelineStates(t *testing.T) {
	// Per-vertex + per-instance vertex buffer layout.
	elems, strides := d3dBuildInputElements([]driver.GPUVertexBufferLayout{
		{
			Stride:   32,
			StepMode: driver.GPUStepVertex,
			Attributes: []driver.GPUVertexAttribute{
				{Location: 0, Offset: 0, Format: driver.GPUVertexFloat32x3},
				{Location: 1, Offset: 12, Format: driver.GPUVertexFloat32x3},
				{Location: 2, Offset: 24, Format: driver.GPUVertexFloat32x2},
			},
		},
		{
			Stride:   16,
			StepMode: driver.GPUStepInstance,
			Attributes: []driver.GPUVertexAttribute{
				{Location: 3, Offset: 0, Format: driver.GPUVertexFloat32x4},
			},
		},
	})
	if len(strides) != 2 || strides[0] != 32 || strides[1] != 16 {
		t.Fatalf("strides = %v, want [32 16]", strides)
	}
	if len(elems) != 4 {
		t.Fatalf("elements = %d, want 4", len(elems))
	}
	if elems[0].semanticName != "TEXCOORD" || elems[0].semanticIndex != 0 || elems[0].format != dxgiFmtR32G32B32Float || elems[0].inputSlotClass != d3d11InputPerVertexData || elems[0].instanceDataStepRate != 0 {
		t.Errorf("elem[0] = %+v", elems[0])
	}
	if elems[3].semanticIndex != 3 || elems[3].inputSlot != 1 || elems[3].format != dxgiFmtR32G32B32A32Float || elems[3].inputSlotClass != d3d11InputPerInstanceData || elems[3].instanceDataStepRate != 1 {
		t.Errorf("elem[3] = %+v", elems[3])
	}

	// Sampler: linear + repeat + anisotropy + shadow comparison.
	sLinear := d3dSamplerDescriptor(driver.GPUSamplerDesc{
		MinFilter:    driver.GPUFilterLinear,
		MagFilter:    driver.GPUFilterLinear,
		MipmapFilter: driver.GPUFilterLinear,
		AddressU:     driver.GPUAddressRepeat,
		AddressV:     driver.GPUAddressMirrorRepeat,
		AddressW:     driver.GPUAddressClampToEdge,
	})
	if sLinear.filter != 0x15 || sLinear.addressU != d3d11AddressWrap || sLinear.addressV != d3d11AddressMirror || sLinear.addressW != d3d11AddressClamp || sLinear.maxAnisotropy != 1 || sLinear.maxLOD != math.MaxFloat32 {
		t.Errorf("sLinear = %+v", sLinear)
	}
	sShadow := d3dSamplerDescriptor(driver.GPUSamplerDesc{
		MinFilter:     driver.GPUFilterLinear,
		MagFilter:     driver.GPUFilterLinear,
		Compare:       driver.GPUCompareLessEqual,
		MaxAnisotropy: 32,
	})
	if sShadow.filter != (0x55|0x80) || sShadow.maxAnisotropy != 16 || sShadow.comparisonFunc != d3d11CompareLessEqual {
		t.Errorf("sShadow = %+v", sShadow)
	}

	// Rasterizer, DepthStencil, and Blend states.
	rs := d3dRasterizerDescriptor(driver.GPUPrimitiveState{
		FrontFace: driver.GPUFrontCCW,
		CullMode:  driver.GPUCullBack,
	}, &driver.GPUDepthStencilState{DepthBias: 2, DepthBiasSlopeScale: 1.5}, 4)
	if rs.cullMode != d3d11CullBack || rs.frontCounterClockwise != 1 || rs.scissorEnable != 1 || rs.multisampleEnable != 1 || rs.depthBias != 2 || rs.slopeScaledDepthBias != 1.5 {
		t.Errorf("rs = %+v", rs)
	}

	dss := d3dDepthStencilDescriptor(&driver.GPUDepthStencilState{
		Format:       driver.GPUFormatDepth32Float,
		DepthWrite:   true,
		DepthCompare: driver.GPUCompareLessEqual,
	})
	if dss.depthEnable != 1 || dss.depthWriteMask != d3d11DepthWriteMaskAll || dss.depthFunc != d3d11CompareLessEqual {
		t.Errorf("dss = %+v", dss)
	}

	bs := d3dBlendDescriptor([]driver.GPUColorTargetState{{
		Format: driver.GPUFormatBGRA8Unorm,
		Blend: &driver.GPUBlendState{
			Color: driver.GPUBlendComponent{SrcFactor: driver.GPUBlendSrcAlpha, DstFactor: driver.GPUBlendOneMinusSrcAlpha, Operation: driver.GPUBlendAdd},
			Alpha: driver.GPUBlendComponent{SrcFactor: driver.GPUBlendSrc, DstFactor: driver.GPUBlendOneMinusDst, Operation: driver.GPUBlendAdd},
		},
	}})
	rt0 := bs.renderTarget[0]
	if rt0.blendEnable != 1 || rt0.srcBlend != d3d11BlendSrcAlpha || rt0.destBlend != d3d11BlendInvSrcAlpha {
		t.Errorf("blend color rt0 = %+v", rt0)
	}
	// Color-only factors in alpha channel must be remapped to alpha equivalents for D3D11.
	if rt0.srcBlendAlpha != d3d11BlendSrcAlpha || rt0.destBlendAlpha != d3d11BlendInvDestAlpha {
		t.Errorf("blend alpha remapping rt0 = %+v", rt0)
	}
}

func TestD3D11BGRASwizzleAndDeviceLostHR(t *testing.T) {
	// 2x2 BGRA with 12-byte row pitch (4 bytes padding per row).
	src := []byte{
		10, 20, 30, 255, 40, 50, 60, 128, 0, 0, 0, 0,
		70, 80, 90, 200, 100, 110, 120, 64, 0, 0, 0, 0,
	}
	dst := make([]byte, 2*2*4)
	d3dSwizzleBGRAtoRGBA(dst, src, 2, 2, 12)
	want := []byte{
		30, 20, 10, 255, 60, 50, 40, 128,
		90, 80, 70, 200, 120, 110, 100, 64,
	}
	if !bytes.Equal(dst, want) {
		t.Fatalf("swizzled = %v, want %v", dst, want)
	}
	if !d3dIsDeviceLostHR(dxgiHRErrDeviceRemoved) || !d3dIsDeviceLostHR(dxgiHRErrDeviceReset) || d3dIsDeviceLostHR(0) {
		t.Errorf("unexpected d3dIsDeviceLostHR results")
	}
	if d3dLostReasonFromHR(dxgiHRErrDeviceRemoved) != driver.GPULostReset {
		t.Errorf("DXGI_ERROR_DEVICE_REMOVED reason mismatch")
	}
	if d3dLostReasonFromHR(winHRErrOutOfMemory) != driver.GPULostOutOfMemory {
		t.Errorf("E_OUTOFMEMORY reason mismatch")
	}
}

// --- Fake native backend for full controller lifecycle, device-loss & release tests ---

type fakeD3DHandle struct {
	kind     string
	sharedID int
	released *int
	alive    *int
	closed   bool
}

func (h *fakeD3DHandle) Release() {
	if h.closed {
		panic("double Release() on " + h.kind)
	}
	h.closed = true
	*h.released++
	*h.alive--
}

type fakeD3D11Ops struct {
	created           int
	released          int
	alive             int
	deviceRemovedHR   uint32
	failNextCreateHR  uint32
	lastCBShadow      []byte
	lastCompiledHLSL  d3dCompiledHLSL
	executedPasses    int
	executedCopies    int
	readbackPixelsOut []byte
	imports           int
	sharedOpens       int
	shared            map[uintptr]int
	keyedMutexBusy    bool
	keyedMutexLog     []string
}

func (f *fakeD3D11Ops) alloc(kind string) *fakeD3DHandle {
	f.created++
	f.alive++
	return &fakeD3DHandle{kind: kind, released: &f.released, alive: &f.alive}
}

func (f *fakeD3D11Ops) CreateSwapTargets(w, h uint32, cfg driver.GPUViewConfig) (d3dReleaser, uint32, error) {
	if f.failNextCreateHR != 0 {
		hr := f.failNextCreateHR
		f.failNextCreateHR = 0
		return nil, hr, errors.New("simulated swap creation failure")
	}
	return f.alloc("swap"), 0, nil
}

func (f *fakeD3D11Ops) CreateBuffer(mainDesc d3d11BufferDesc, hasCB bool, cbDesc d3d11BufferDesc, initData, cbShadow []byte) (d3dReleaser, uint32, error) {
	if f.failNextCreateHR != 0 {
		hr := f.failNextCreateHR
		f.failNextCreateHR = 0
		return nil, hr, errors.New("simulated buffer creation failure")
	}
	if hasCB {
		f.lastCBShadow = append([]byte(nil), cbShadow...)
	}
	return f.alloc("buffer"), 0, nil
}

func (f *fakeD3D11Ops) WriteBuffer(buf *d3dTrackedBuffer, offset uint64, data []byte) (uint32, error) {
	if buf.hasCB {
		f.lastCBShadow = append([]byte(nil), buf.cbShadow...)
	}
	return 0, nil
}

func (f *fakeD3D11Ops) CreateTexture(desc driver.GPUTextureDesc, td d3d11Texture2DDesc, srv d3d11SRVDesc, rtv d3d11RTVDesc, dsv d3d11DSVDesc) (d3dReleaser, uint32, error) {
	return f.alloc("texture"), 0, nil
}

func (f *fakeD3D11Ops) WriteTexture(tex *d3dTrackedTexture, dst driver.GPUTextureCopy, data []byte, layout driver.GPUDataLayout) (uint32, error) {
	return 0, nil
}

func (f *fakeD3D11Ops) CreateSampler(desc d3d11SamplerDesc) (d3dReleaser, uint32, error) {
	return f.alloc("sampler"), 0, nil
}

func (f *fakeD3D11Ops) CreatePipeline(desc *driver.GPUPipelineDesc, compiled d3dCompiledHLSL, elements []d3dInputElementSpec, rs d3d11RasterizerDesc, dss d3d11DepthStencilDesc, bs d3d11BlendDesc) (d3dReleaser, driver.GPUErrorCode, uint32, error) {
	f.lastCompiledHLSL = compiled
	return f.alloc("pipeline"), driver.GPUErrorNone, 0, nil
}

func (f *fakeD3D11Ops) ExecuteCopy(cp driver.GPUCopyOp, lookup *d3dPassLookup) (uint32, error) {
	f.executedCopies++
	return 0, nil
}

func (f *fakeD3D11Ops) ExecutePass(pass driver.GPUPass, swap d3dReleaser, swapW, swapH uint32, lookup *d3dPassLookup) (bool, uint32, error) {
	f.executedPasses++
	drewSwap := false
	for _, ca := range pass.ColorAttachments {
		if ca.Texture == 0 {
			drewSwap = true
		}
	}
	return drewSwap, 0, nil
}

func (f *fakeD3D11Ops) ReadTexture(swap d3dReleaser, swapW, swapH uint32, tex *d3dTrackedTexture, region driver.GPUTextureRegion, surface bool) ([]byte, uint32, error) {
	return append([]byte(nil), f.readbackPixelsOut...), 0, nil
}

func (f *fakeD3D11Ops) ImportTexture(desc driver.GPUExternalTextureDesc) (d3dReleaser, d3d11Texture2DDesc, uint32, error) {
	f.imports++
	if f.shared == nil {
		f.shared = map[uintptr]int{}
	}
	sid, ok := f.shared[desc.Handle]
	if !ok {
		f.sharedOpens++
		sid = f.sharedOpens
		f.shared[desc.Handle] = sid
	}
	h := f.alloc("imported texture")
	h.sharedID = sid
	return h, d3d11Texture2DDesc{width: desc.Width, height: desc.Height, mipLevels: 1, arraySize: 1}, 0, nil
}

func (f *fakeD3D11Ops) EndImportBatch() {
	clear(f.shared)
}

func (f *fakeD3D11Ops) AcquireKeyedMutex(tex *d3dTrackedTexture, key uint64, timeoutMS uint32) (uint32, error) {
	if f.keyedMutexBusy {
		return d3dKeyedMutexWaitTimeout, nil
	}
	f.keyedMutexLog = append(f.keyedMutexLog, fmt.Sprintf("acquire %d:%d", tex.id, key))
	return 0, nil
}

func (f *fakeD3D11Ops) ReleaseKeyedMutex(tex *d3dTrackedTexture, key uint64) {
	f.keyedMutexLog = append(f.keyedMutexLog, fmt.Sprintf("release %d:%d", tex.id, key))
}

func (f *fakeD3D11Ops) DeviceRemovedReason() uint32 {
	return f.deviceRemovedHR
}

func TestD3D11ControllerLifecycleDeviceRemovedAndRelease(t *testing.T) {
	const meshWGSL = `
struct Camera { mvp: mat4x4<f32>, }
@group(0) @binding(0) var<uniform> cam: Camera;
@group(0) @binding(1) var tex: texture_2d<f32>;
@group(0) @binding(2) var samp: sampler;

struct VIn { @location(0) pos: vec3<f32>, @location(1) uv: vec2<f32>, }
struct VOut { @builtin(position) pos: vec4<f32>, @location(0) uv: vec2<f32>, }

@vertex fn vs_main(in: VIn) -> VOut {
	var out: VOut;
	out.pos = cam.mvp * vec4<f32>(in.pos, 1.0);
	out.uv = in.uv;
	return out;
}
@fragment fn fs_main(in: VOut) -> @location(0) vec4<f32> {
	return textureSample(tex, samp, in.uv);
}
`
	compiled, err := wgsl.CompilePipeline(meshWGSL)
	if err != nil {
		t.Fatalf("wgsl.CompilePipeline: %v", err)
	}

	ops := &fakeD3D11Ops{readbackPixelsOut: []byte{255, 0, 0, 255}}
	var events []*driver.GPUEvent
	ctrl := newD3DGPUController(ops, func(ev *driver.GPUEvent) {
		events = append(events, ev)
	})

	// 1. Mount & configure -> GPUEventDeviceReady (gen 1, BGRA8Unorm).
	ctrl.Configure(driver.GPUViewConfig{DepthFormat: driver.GPUFormatDepth32Float, SampleCount: 1})
	if !ctrl.MountOrResize(100, 80) {
		t.Fatal("initial MountOrResize failed")
	}
	if len(events) != 1 || events[0].Kind != driver.GPUEventDeviceReady || events[0].Generation != 1 ||
		events[0].Width != 100 || events[0].Height != 80 || events[0].SurfaceFormat != driver.GPUFormatBGRA8Unorm {
		t.Fatalf("mount events = %+v", events)
	}
	if ops.alive != 1 {
		t.Fatalf("alive after mount = %d, want 1", ops.alive)
	}

	// 2. Resize -> old swap released, new swap created, GPUEventResize emitted.
	if !ctrl.MountOrResize(200, 160) {
		t.Fatal("resize MountOrResize failed")
	}
	if len(events) != 2 || events[1].Kind != driver.GPUEventResize || events[1].Width != 200 || events[1].Height != 160 {
		t.Fatalf("resize event = %+v", events[1])
	}
	if ops.alive != 1 || ops.released != 1 {
		t.Fatalf("after resize alive=%d released=%d, want 1, 1", ops.alive, ops.released)
	}

	// 3. Off-commit frame creating buffer (20B uniform -> 32B shadow CB), texture, sampler, pipeline, pass & readback.
	drew := ctrl.Submit(driver.GPUFrame{
		Generation: 1,
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreateBuffer, ID: 1, Buffer: &driver.GPUBufferDesc{Size: 20, Usage: driver.GPUBufferUniform | driver.GPUBufferCopyDst, Data: []byte{1, 2, 3, 4}}},
			{Kind: driver.GPUOpWriteBuffer, ID: 1, Offset: 4, Data: []byte{5, 6, 7, 8}},
			{Kind: driver.GPUOpCreateTexture, ID: 2, Texture: &driver.GPUTextureDesc{Width: 4, Height: 4, Format: driver.GPUFormatRGBA8Unorm, Usage: driver.GPUTextureSampled | driver.GPUTextureCopyDst}},
			{Kind: driver.GPUOpCreateSampler, ID: 3, Sampler: &driver.GPUSamplerDesc{MinFilter: driver.GPUFilterLinear, MagFilter: driver.GPUFilterLinear}},
			{Kind: driver.GPUOpCreateBindGroupLayout, ID: 4, BindGroupLayout: &driver.GPUBindGroupLayoutDesc{}},
			{Kind: driver.GPUOpCreateBindGroup, ID: 5, BindGroup: &driver.GPUBindGroupDesc{Layout: 4, Entries: []driver.GPUBindGroupEntry{
				{Binding: 0, Buffer: 1}, {Binding: 1, Texture: 2}, {Binding: 2, Sampler: 3},
			}}},
			{Kind: driver.GPUOpCreatePipeline, ID: 6, Pipeline: &driver.GPUPipelineDesc{
				Vertex:   driver.GPUShaderStage{IR: compiled.IR, EntryPoint: "vs_main"},
				Fragment: driver.GPUShaderStage{IR: compiled.IR, EntryPoint: "fs_main"},
				VertexBuffers: []driver.GPUVertexBufferLayout{{
					Stride: 20,
					Attributes: []driver.GPUVertexAttribute{
						{Location: 0, Offset: 0, Format: driver.GPUVertexFloat32x3},
						{Location: 1, Offset: 12, Format: driver.GPUVertexFloat32x2},
					},
				}},
			}},
			{Kind: driver.GPUOpReadTexture, ReadbackID: 99, ReadbackSurface: true},
		},
		Passes: []driver.GPUPass{{
			ColorAttachments: []driver.GPUColorAttachment{{LoadOp: driver.GPULoadClear, StoreOp: driver.GPUStoreStore}},
		}},
		Present: true,
	})
	if !drew || ops.executedPasses != 1 {
		t.Fatalf("drew=%v executedPasses=%d", drew, ops.executedPasses)
	}
	if len(ops.lastCBShadow) != 32 || !bytes.Equal(ops.lastCBShadow[:8], []byte{1, 2, 3, 4, 5, 6, 7, 8}) {
		t.Fatalf("constant buffer 16-byte aligned shadow = %v", ops.lastCBShadow)
	}
	if !strings.Contains(ops.lastCompiledHLSL.VertexSource, "register(b0)") || !strings.Contains(ops.lastCompiledHLSL.PixelSource, ".Sample(") {
		t.Fatalf("HLSL compilation mismatch: VS=%s PS=%s", ops.lastCompiledHLSL.VertexSource, ops.lastCompiledHLSL.PixelSource)
	}
	// Check readback event delivered.
	if len(events) != 3 || events[2].Kind != driver.GPUEventReadback || events[2].ReadbackID != 99 || !bytes.Equal(events[2].Pixels, []byte{255, 0, 0, 255}) {
		t.Fatalf("readback event = %+v", events[2])
	}
	// 5 alive handles: swap, buffer, texture, sampler, pipeline.
	if ops.alive != 5 {
		t.Fatalf("alive resources = %d, want 5", ops.alive)
	}

	// 4. Simulate DXGI_ERROR_DEVICE_REMOVED during next frame submission.
	ops.deviceRemovedHR = dxgiHRErrDeviceRemoved
	if ctrl.Submit(driver.GPUFrame{Generation: 1, Present: true}) {
		t.Fatal("Submit after device removal must not report drewSwap=true")
	}
	if ops.alive != 0 {
		t.Fatalf("alive resources after DXGI_ERROR_DEVICE_REMOVED = %d, want 0 (all COM objects must be released)", ops.alive)
	}
	last := events[len(events)-1]
	if last.Kind != driver.GPUEventDeviceLost || last.Generation != 1 || last.LostReason != driver.GPULostReset {
		t.Fatalf("device lost event = %+v", last)
	}

	// 5. Stale-generation frame while lost must be rejected and fail any pending readback cleanly.
	ctrl.Submit(driver.GPUFrame{
		Generation: 1,
		Ops:        []driver.GPUResourceOp{{Kind: driver.GPUOpReadTexture, ReadbackID: 100, ReadbackSurface: true}},
	})
	last = events[len(events)-1]
	if last.Kind != driver.GPUEventReadback || last.ReadbackID != 100 || last.Message == "" {
		t.Fatalf("stale readback error event = %+v", last)
	}

	// 6. Device recovery -> generation bumps to 2, GPUEventDeviceReady emitted.
	ops.deviceRemovedHR = 0
	if !ctrl.Recover() {
		t.Fatal("Recover failed")
	}
	last = events[len(events)-1]
	if last.Kind != driver.GPUEventDeviceReady || last.Generation != 2 || last.Width != 200 || last.Height != 160 {
		t.Fatalf("recovery event = %+v", last)
	}
	if ops.alive != 1 {
		t.Fatalf("alive after recovery = %d, want 1", ops.alive)
	}

	// 7. Create a buffer on generation 2 and unmount (Release) -> every single handle released, zero leaks.
	ctrl.Submit(driver.GPUFrame{
		Generation: 2,
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreateBuffer, ID: 10, Buffer: &driver.GPUBufferDesc{Size: 64, Usage: driver.GPUBufferVertex}},
		},
	})
	if ops.alive != 2 {
		t.Fatalf("alive before unmount = %d, want 2", ops.alive)
	}
	ctrl.Release()
	ctrl.Release() // idempotent
	if ops.alive != 0 || ops.created != ops.released {
		t.Fatalf("leak on unmount: created=%d released=%d alive=%d", ops.created, ops.released, ops.alive)
	}
}

func TestD3D11ResolveBindingHelper(t *testing.T) {
	var bound [8]*d3dTrackedBindGroup
	bound[1] = &d3dTrackedBindGroup{
		id: 50,
		entries: map[uint32]driver.GPUBindGroupEntry{
			2: {Binding: 2, Texture: 77},
		},
	}
	e, ok := d3dResolveBinding(hlsl.RegisterBinding{Group: 1, Binding: 2, Kind: shaderir.BindingTexture, Register: 0}, bound)
	if !ok || e.Texture != 77 {
		t.Fatalf("d3dResolveBinding = %+v, %v", e, ok)
	}
	if _, ok := d3dResolveBinding(hlsl.RegisterBinding{Group: 0, Binding: 0, Kind: shaderir.BindingUniformBuffer, Register: 0}, bound); ok {
		t.Fatal("expected unbound group 0 to return false")
	}
	if !d3dIsSRVBinding(shaderir.BindingTexture) || !d3dIsSRVBinding(shaderir.BindingStorageBufferRead) || d3dIsSRVBinding(shaderir.BindingSampler) {
		t.Fatal("unexpected d3dIsSRVBinding results")
	}
}

func TestD3D11ControllerReconfigureAndMultiReadbackDeviceLoss(t *testing.T) {
	ops := &fakeD3D11Ops{}
	var events []*driver.GPUEvent
	ctrl := newD3DGPUController(ops, func(ev *driver.GPUEvent) {
		events = append(events, ev)
	})
	ctrl.Configure(driver.GPUViewConfig{DepthFormat: driver.GPUFormatUndefined, SampleCount: 1})
	if !ctrl.MountOrResize(64, 64) {
		t.Fatal("initial MountOrResize failed")
	}
	if ops.created != 1 || ops.released != 0 || ops.alive != 1 {
		t.Fatalf("after mount: created=%d released=%d alive=%d", ops.created, ops.released, ops.alive)
	}
	// Reconfiguring DepthFormat at the same pixel size must recreate swap targets without emitting a spurious resize event.
	ctrl.Configure(driver.GPUViewConfig{DepthFormat: driver.GPUFormatDepth32Float, SampleCount: 4})
	if !ctrl.MountOrResize(64, 64) {
		t.Fatal("reconfigure MountOrResize failed")
	}
	if ops.created != 2 || ops.released != 1 || ops.alive != 1 {
		t.Fatalf("after reconfigure: created=%d released=%d alive=%d, want 2/1/1", ops.created, ops.released, ops.alive)
	}
	if len(events) != 1 || events[0].Kind != driver.GPUEventDeviceReady {
		t.Fatalf("unexpected events on same-size reconfigure: %+v", events)
	}

	// Device loss mid-readback must fail ALL pending readbacks in the frame, not just the first.
	lostOps := &fakeReadbackLossOps{fakeD3D11Ops: fakeD3D11Ops{}}
	events = nil
	ctrl2 := newD3DGPUController(lostOps, func(ev *driver.GPUEvent) {
		events = append(events, ev)
	})
	if !ctrl2.MountOrResize(16, 16) {
		t.Fatal("ctrl2 mount failed")
	}
	ctrl2.Submit(driver.GPUFrame{
		Generation: 1,
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpReadTexture, ReadbackID: 11, ReadbackSurface: true},
			{Kind: driver.GPUOpReadTexture, ReadbackID: 12, ReadbackSurface: true},
		},
	})
	var failedIDs []uint64
	for _, ev := range events {
		if ev.Kind == driver.GPUEventReadback && ev.Message != "" {
			failedIDs = append(failedIDs, ev.ReadbackID)
		}
	}
	if len(failedIDs) != 2 || failedIDs[0] != 11 || failedIDs[1] != 12 {
		t.Fatalf("failed readback IDs = %v, want [11 12] (events=%+v)", failedIDs, events)
	}
	ctrl.Release()
	ctrl2.Release()
}

type fakeReadbackLossOps struct {
	fakeD3D11Ops
}

func (f *fakeReadbackLossOps) ReadTexture(swap d3dReleaser, swapW, swapH uint32, tex *d3dTrackedTexture, region driver.GPUTextureRegion, surface bool) ([]byte, uint32, error) {
	return nil, dxgiHRErrDeviceRemoved, errors.New("simulated readback device removed")
}

func TestD3D11ExternalSRVDescriptor(t *testing.T) {
	nv12 := d3d11Texture2DDesc{width: 64, height: 35, mipLevels: 1, arraySize: 1, format: dxgiFmtNV12, sampleCount: 1, bindFlags: d3d11BindShaderResource, misc: d3d11MiscSharedNTHandle | d3d11MiscSharedKeyedMutex}
	for _, tc := range []struct {
		name  string
		td    d3d11Texture2DDesc
		ext   driver.GPUExternalTextureDesc
		want  uint32
		fails bool
	}{
		{"NV12 luma", nv12, driver.GPUExternalTextureDesc{Width: 64, Height: 35, Format: driver.GPUFormatR8Unorm}, dxgiFmtR8Unorm, false},
		{"NV12 chroma rounds up", nv12, driver.GPUExternalTextureDesc{Width: 32, Height: 18, Plane: 1, Format: driver.GPUFormatRG8Unorm, KeyedMutex: true}, dxgiFmtR8G8Unorm, false},
		{"P010 chroma", d3d11Texture2DDesc{width: 8, height: 8, arraySize: 1, format: dxgiFmtP010, bindFlags: d3d11BindShaderResource}, driver.GPUExternalTextureDesc{Width: 4, Height: 4, Plane: 1, Format: driver.GPUFormatRG16Unorm}, dxgiFmtR16G16Unorm, false},
		{"BGRA", d3d11Texture2DDesc{width: 8, height: 8, arraySize: 1, format: dxgiFmtB8G8R8A8Unorm, bindFlags: d3d11BindShaderResource}, driver.GPUExternalTextureDesc{Width: 8, Height: 8, Format: driver.GPUFormatBGRA8Unorm}, dxgiFmtB8G8R8A8Unorm, false},
		{"wrong plane format", nv12, driver.GPUExternalTextureDesc{Width: 32, Height: 18, Plane: 1, Format: driver.GPUFormatR8Unorm}, 0, true},
		{"wrong size", nv12, driver.GPUExternalTextureDesc{Width: 64, Height: 36, Format: driver.GPUFormatR8Unorm}, 0, true},
		{"plane of a single-plane texture", d3d11Texture2DDesc{width: 8, height: 8, arraySize: 1, format: dxgiFmtR8Unorm, bindFlags: d3d11BindShaderResource}, driver.GPUExternalTextureDesc{Width: 8, Height: 8, Plane: 1, Format: driver.GPUFormatR8Unorm}, 0, true},
		{"array", d3d11Texture2DDesc{width: 8, height: 8, arraySize: 4, format: dxgiFmtNV12, bindFlags: d3d11BindShaderResource}, driver.GPUExternalTextureDesc{Width: 8, Height: 8, Format: driver.GPUFormatR8Unorm}, 0, true},
		{"not a shader resource", d3d11Texture2DDesc{width: 8, height: 8, arraySize: 1, format: dxgiFmtNV12}, driver.GPUExternalTextureDesc{Width: 8, Height: 8, Format: driver.GPUFormatR8Unorm}, 0, true},
		{"no keyed mutex", d3d11Texture2DDesc{width: 8, height: 8, arraySize: 1, format: dxgiFmtNV12, bindFlags: d3d11BindShaderResource}, driver.GPUExternalTextureDesc{Width: 8, Height: 8, Format: driver.GPUFormatR8Unorm, KeyedMutex: true}, 0, true},
	} {
		srv, err := d3dExternalSRVDescriptor(tc.td, tc.ext)
		if tc.fails {
			if err == nil {
				t.Errorf("%s: no error, srv %+v", tc.name, srv)
			}
			continue
		}
		if err != nil || srv.format != tc.want || srv.viewDimension != d3d11SRVDimTexture2D || srv.u1 != 1 {
			t.Errorf("%s: srv %+v, err %v", tc.name, srv, err)
		}
	}
}

func TestD3D11ControllerImportsTexturesAndHoldsKeyedMutexWhileDrawing(t *testing.T) {
	ops := &fakeD3D11Ops{}
	var errs []string
	ctrl := newD3DGPUController(ops, func(ev *driver.GPUEvent) {
		if ev.Kind == driver.GPUEventError {
			errs = append(errs, ev.Message)
		}
	})
	if !ctrl.MountOrResize(16, 16) {
		t.Fatal("mount failed")
	}
	luma := &driver.GPUExternalTextureDesc{Kind: driver.GPUExternalD3D11Shared, Width: 16, Height: 16, Format: driver.GPUFormatR8Unorm, Handle: 0x40, KeyedMutex: true, AcquireKey: 1, ReleaseKey: 0}
	chroma := &driver.GPUExternalTextureDesc{Kind: driver.GPUExternalD3D11Shared, Width: 8, Height: 8, Plane: 1, Format: driver.GPUFormatRG8Unorm, Handle: 0x40, KeyedMutex: true, AcquireKey: 1, ReleaseKey: 0}
	pass := driver.GPUPass{
		ColorAttachments: []driver.GPUColorAttachment{{LoadOp: driver.GPULoadClear, StoreOp: driver.GPUStoreStore}},
		Commands:         []driver.GPUCommand{{Kind: driver.GPUCmdSetBindGroup, ID: 3}, {Kind: driver.GPUCmdDraw, U0: 3}},
	}
	ctrl.Submit(driver.GPUFrame{
		Generation: 1,
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpImportTexture, ID: 1, External: luma},
			{Kind: driver.GPUOpImportTexture, ID: 2, External: chroma},
			{Kind: driver.GPUOpCreateBindGroup, ID: 3, BindGroup: &driver.GPUBindGroupDesc{Entries: []driver.GPUBindGroupEntry{{Binding: 0, Texture: 1}, {Binding: 1, Texture: 2}}}},
			// Writes to an imported texture are dropped.
			{Kind: driver.GPUOpWriteTexture, ID: 1, Data: []byte{1}},
		},
		Passes:  []driver.GPUPass{pass},
		Present: true,
	})
	if ops.imports != 2 || ctrl.textures[1].external == nil || ctrl.textures[2].desc.Format != driver.GPUFormatRG8Unorm {
		t.Fatalf("imports = %d, textures = %+v", ops.imports, ctrl.textures)
	}
	// Both planes share one handle, so its mutex is taken once, around the pass.
	if len(ops.keyedMutexLog) != 2 || !strings.HasPrefix(ops.keyedMutexLog[0], "acquire ") || !strings.HasSuffix(ops.keyedMutexLog[0], ":1") || !strings.HasSuffix(ops.keyedMutexLog[1], ":0") {
		t.Fatalf("keyed mutex log = %v", ops.keyedMutexLog)
	}
	// A frame that does not bind them leaves the mutex alone.
	ops.keyedMutexLog = nil
	ctrl.Submit(driver.GPUFrame{Generation: 1, Passes: []driver.GPUPass{{ColorAttachments: pass.ColorAttachments}}, Present: true})
	if len(ops.keyedMutexLog) != 0 {
		t.Fatalf("keyed mutex log without bindings = %v", ops.keyedMutexLog)
	}
	// A mutex the decoder keeps is reported, and not released.
	ops.keyedMutexBusy = true
	ctrl.Submit(driver.GPUFrame{Generation: 1, Passes: []driver.GPUPass{pass}, Present: true})
	if len(errs) != 1 || !strings.Contains(errs[0], "keyed mutex") || len(ops.keyedMutexLog) != 0 {
		t.Fatalf("busy mutex errors = %v, log = %v", errs, ops.keyedMutexLog)
	}
	// Destroying both releases what the imports opened.
	ctrl.Submit(driver.GPUFrame{Generation: 1, Ops: []driver.GPUResourceOp{{Kind: driver.GPUOpDestroyTexture, ID: 1}, {Kind: driver.GPUOpDestroyTexture, ID: 2}}})
	if ops.alive != 1 || len(ctrl.textures) != 0 {
		t.Fatalf("alive after destroying imports = %d (want the swap chain only), textures = %v", ops.alive, ctrl.textures)
	}
	ctrl.Release()
	if ops.alive != 0 {
		t.Fatalf("leak: alive = %d", ops.alive)
	}
}

// TestD3D11ImportedTextureRecycledHandleOpensFreshResource verifies that shared-handle deduplication is scoped to one Submit: when a frame destroys the previous imports and imports a new shared texture whose Windows NT handle value was recycled, phase 1 opens a fresh resource rather than reusing the old texture's cached entry (which is only destroyed in phase 5), and keyed-mutex acquisition does not alias distinct import batches.
func TestD3D11ImportedTextureRecycledHandleOpensFreshResource(t *testing.T) {
	ops := &fakeD3D11Ops{}
	ctrl := newD3DGPUController(ops, nil)
	if !ctrl.MountOrResize(16, 16) {
		t.Fatal("mount failed")
	}
	const recycledHandle = uintptr(0x40)
	luma := &driver.GPUExternalTextureDesc{Kind: driver.GPUExternalD3D11Shared, Width: 16, Height: 16, Format: driver.GPUFormatR8Unorm, Handle: recycledHandle, KeyedMutex: true, AcquireKey: 1, ReleaseKey: 1}
	chroma := &driver.GPUExternalTextureDesc{Kind: driver.GPUExternalD3D11Shared, Width: 8, Height: 8, Plane: 1, Format: driver.GPUFormatRG8Unorm, Handle: recycledHandle, KeyedMutex: true, AcquireKey: 1, ReleaseKey: 1}

	// Frame 1: both planes imported in the same Submit share one opened resource.
	ctrl.Submit(driver.GPUFrame{
		Generation: 1,
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpImportTexture, ID: 1, External: luma},
			{Kind: driver.GPUOpImportTexture, ID: 2, External: chroma},
		},
	})
	h1 := ctrl.textures[1].handle.(*fakeD3DHandle)
	h2 := ctrl.textures[2].handle.(*fakeD3DHandle)
	if ops.sharedOpens != 1 || h1.sharedID != 1 || h2.sharedID != 1 {
		t.Fatalf("frame 1 shared opens=%d, ids=(%d,%d), want 1 and (1,1)", ops.sharedOpens, h1.sharedID, h2.sharedID)
	}
	if len(ops.shared) != 0 {
		t.Fatalf("per-submit shared map was not cleared after frame 1: %v", ops.shared)
	}

	// Frame 2: destroys frame 1's textures (phase 5) while importing frame 2's planes under the recycled handle value (phase 1).
	// Also bind texture 1 (still alive during phases 2-3) alongside texture 3 to confirm keyed mutexes are not aliased across batches.
	pass := driver.GPUPass{
		ColorAttachments: []driver.GPUColorAttachment{{LoadOp: driver.GPULoadClear, StoreOp: driver.GPUStoreStore}},
		Commands:         []driver.GPUCommand{{Kind: driver.GPUCmdSetBindGroup, ID: 10}, {Kind: driver.GPUCmdDraw, U0: 3}},
	}
	ctrl.Submit(driver.GPUFrame{
		Generation: 1,
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpDestroyTexture, ID: 1},
			{Kind: driver.GPUOpDestroyTexture, ID: 2},
			{Kind: driver.GPUOpImportTexture, ID: 3, External: luma},
			{Kind: driver.GPUOpImportTexture, ID: 4, External: chroma},
			{Kind: driver.GPUOpCreateBindGroup, ID: 10, BindGroup: &driver.GPUBindGroupDesc{Entries: []driver.GPUBindGroupEntry{{Binding: 0, Texture: 1}, {Binding: 1, Texture: 3}, {Binding: 2, Texture: 4}}}},
		},
		Passes: []driver.GPUPass{pass},
	})
	h3 := ctrl.textures[3].handle.(*fakeD3DHandle)
	h4 := ctrl.textures[4].handle.(*fakeD3DHandle)
	if ops.sharedOpens != 2 || h3.sharedID != 2 || h4.sharedID != 2 {
		t.Fatalf("frame 2 shared opens=%d, ids=(%d,%d), want 2 and (2,2)", ops.sharedOpens, h3.sharedID, h4.sharedID)
	}
	if ctrl.textures[1] != nil || ctrl.textures[2] != nil {
		t.Fatalf("frame 1 textures survived deferred destroy")
	}
	// Textures 1 (batch 1) and 3/4 (batch 2) must each acquire and release their own keyed mutex (2 acquires + 2 releases).
	if len(ops.keyedMutexLog) != 4 {
		t.Fatalf("keyed mutex log = %v, want 4 entries (two distinct batches)", ops.keyedMutexLog)
	}
	ctrl.Release()
	if ops.alive != 0 {
		t.Fatalf("leak: alive = %d", ops.alive)
	}
}
