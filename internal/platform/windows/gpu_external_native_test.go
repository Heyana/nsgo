//go:build windows

package windows

import (
	"bytes"
	"syscall"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
)

var iidDXGIResource1 = parseGUID("30961379-4609-4a41-998e-54fe567ee0c1")

const (
	dxgiResource1CreateSharedHandle = 13
	dxgiSharedResourceRead          = 0x80000000
	dxgiSharedResourceWrite         = 1
)

// yuvPlanesWGSL samples an NV12 frame's luma and chroma planes into one color: (Y, U, V, 1).
const yuvPlanesWGSL = `
@group(0) @binding(0) var lumaTex: texture_2d<f32>;
@group(0) @binding(1) var chromaTex: texture_2d<f32>;
@group(0) @binding(2) var samp: sampler;

struct VOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) uv: vec2<f32>,
}

@vertex
fn vs_main(@location(0) pos: vec2<f32>, @location(1) uv: vec2<f32>) -> VOut {
	var out: VOut;
	out.pos = vec4<f32>(pos, 0.0, 1.0);
	out.uv = uv;
	return out;
}

@fragment
fn fs_main(in: VOut) -> @location(0) vec4<f32> {
	let y = textureSample(lumaTex, samp, in.uv).r;
	let c = textureSample(chromaTex, samp, in.uv).rg;
	return vec4<f32>(y, c.r, c.g, 1.0);
}
`

// TestD3D11ImportedNV12MatchesUpload writes an NV12 frame on a second Direct3D 11 device, as a decoder would, shares it with a keyed mutex, imports both planes, and checks that sampling them renders exactly what sampling the same bytes uploaded with WriteTexture does; closing the imports releases what they opened.
func TestD3D11ImportedNV12MatchesUpload(t *testing.T) {
	if !gpuAvailable() {
		t.Skip("Direct3D 11 or D3DCompile unavailable")
	}
	if _, err := devices(); err != nil {
		t.Skip(err)
	}
	const w, h = 8, 4
	// NV12: w x h luma bytes, then w/2 x h/2 interleaved U, V pairs, both w bytes a row.
	frame := make([]byte, w*h+w*h/2)
	for i := range w * h {
		frame[i] = byte(16 + i*7)
	}
	for i := range w * h / 2 {
		frame[w*h+i] = byte(240 - i*13)
	}

	decoder, err := createD3D(d3dDriverHardware)
	if err != nil {
		if decoder, err = createD3D(d3dDriverWARP); err != nil {
			t.Skip(err)
		}
	}
	defer decoder.release()
	td := d3d11Texture2DDesc{
		width: w, height: h, mipLevels: 1, arraySize: 1, format: dxgiFmtNV12, sampleCount: 1,
		usage: d3d11UsageDefault, bindFlags: d3d11BindShaderResource,
		misc: d3d11MiscSharedNTHandle | d3d11MiscSharedKeyedMutex,
	}
	init := d3d11SubresourceData{pSysMem: uintptr(unsafe.Pointer(&frame[0])), sysMemPitch: w}
	var shared *comObject
	if hr := decoder.call(d3dCreateTexture2D, uintptr(unsafe.Pointer(&td)), uintptr(unsafe.Pointer(&init)), uintptr(unsafe.Pointer(&shared))); failed(hr) {
		t.Skip(hresultError("this adapter cannot create shared NV12 textures", hr))
	}
	defer shared.release()
	res := shared.query(&iidDXGIResource1)
	if res == nil {
		t.Fatal("no IDXGIResource1")
	}
	var handle uintptr
	hr := res.call(dxgiResource1CreateSharedHandle, 0, dxgiSharedResourceRead|dxgiSharedResourceWrite, 0, uintptr(unsafe.Pointer(&handle)))
	res.release()
	if failed(hr) {
		t.Fatal(hresultError("CreateSharedHandle", hr))
	}
	defer syscall.CloseHandle(syscall.Handle(handle))
	// The decoder takes key 0, has written the frame, and hands it over as key 1.
	mutex := shared.query(&iidDXGIKeyedMutex)
	if mutex == nil {
		t.Fatal("no IDXGIKeyedMutex")
	}
	defer mutex.release()
	if hr := mutex.call(keyedMutexAcquireSync, keyedMutexArgs(0, 1000)...); failed(hr) || uint32(hr) == d3dKeyedMutexWaitTimeout {
		t.Fatalf("decoder AcquireSync(0) = %#x", hr)
	}
	if hr := mutex.call(keyedMutexReleaseSync, keyedMutexArgs(1)...); failed(hr) {
		t.Fatal(hresultError("decoder ReleaseSync(1)", hr))
	}

	prog, err := wgsl.CompilePipeline(yuvPlanesWGSL)
	if err != nil {
		t.Fatal(err)
	}
	var events []*driver.GPUEvent
	ops := &d3d11NativeOps{scaleFn: func() float32 { return 1 }}
	ctrl := newD3DGPUController(ops, func(ev *driver.GPUEvent) { events = append(events, ev) })
	defer ctrl.Release()
	ctrl.Configure(driver.GPUViewConfig{ColorFormat: driver.GPUFormatBGRA8Unorm, SampleCount: 1})
	if !ctrl.MountOrResize(w, h) {
		t.Fatalf("MountOrResize failed: %+v", events)
	}

	quad := packF32(
		-1, -1, 0, 1,
		1, -1, 1, 1,
		1, 1, 1, 0,
		-1, -1, 0, 1,
		1, 1, 1, 0,
		-1, 1, 0, 0,
	)
	sampled := driver.GPUTextureSampled | driver.GPUTextureCopyDst
	target := driver.GPUTextureRenderAttachment | driver.GPUTextureCopySrc
	pass := func(target, group driver.GPUID) driver.GPUPass {
		return driver.GPUPass{
			ColorAttachments: []driver.GPUColorAttachment{{Texture: target, LoadOp: driver.GPULoadClear, StoreOp: driver.GPUStoreStore}},
			Commands: []driver.GPUCommand{
				{Kind: driver.GPUCmdSetPipeline, ID: 20},
				{Kind: driver.GPUCmdSetBindGroup, ID: group},
				{Kind: driver.GPUCmdSetVertexBuffer, ID: 1},
				{Kind: driver.GPUCmdDraw, U0: 6},
			},
		}
	}
	ctrl.Submit(driver.GPUFrame{
		Generation: 1,
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreateBuffer, ID: 1, Buffer: &driver.GPUBufferDesc{Size: uint64(len(quad)), Usage: driver.GPUBufferVertex, Data: quad}},
			{Kind: driver.GPUOpCreateSampler, ID: 2, Sampler: &driver.GPUSamplerDesc{MinFilter: driver.GPUFilterNearest, MagFilter: driver.GPUFilterNearest, AddressU: driver.GPUAddressClampToEdge, AddressV: driver.GPUAddressClampToEdge}},
			{Kind: driver.GPUOpImportTexture, ID: 3, External: &driver.GPUExternalTextureDesc{Kind: driver.GPUExternalD3D11Shared, Width: w, Height: h, Format: driver.GPUFormatR8Unorm, Handle: handle, KeyedMutex: true, AcquireKey: 1, ReleaseKey: 1}},
			{Kind: driver.GPUOpImportTexture, ID: 4, External: &driver.GPUExternalTextureDesc{Kind: driver.GPUExternalD3D11Shared, Width: w / 2, Height: h / 2, Plane: 1, Format: driver.GPUFormatRG8Unorm, Handle: handle, KeyedMutex: true, AcquireKey: 1, ReleaseKey: 1}},
			{Kind: driver.GPUOpCreateTexture, ID: 5, Texture: &driver.GPUTextureDesc{Width: w, Height: h, Format: driver.GPUFormatR8Unorm, Usage: sampled}},
			{Kind: driver.GPUOpWriteTexture, ID: 5, Data: frame[:w*h], TextureDst: driver.GPUTextureCopy{Texture: 5}, DataLayout: driver.GPUDataLayout{BytesPerRow: w, RowsPerImage: h}},
			{Kind: driver.GPUOpCreateTexture, ID: 6, Texture: &driver.GPUTextureDesc{Width: w / 2, Height: h / 2, Format: driver.GPUFormatRG8Unorm, Usage: sampled}},
			{Kind: driver.GPUOpWriteTexture, ID: 6, Data: frame[w*h:], TextureDst: driver.GPUTextureCopy{Texture: 6}, DataLayout: driver.GPUDataLayout{BytesPerRow: w, RowsPerImage: h / 2}},
			{Kind: driver.GPUOpCreateBindGroupLayout, ID: 7, BindGroupLayout: &driver.GPUBindGroupLayoutDesc{}},
			{Kind: driver.GPUOpCreateBindGroup, ID: 8, BindGroup: &driver.GPUBindGroupDesc{Layout: 7, Entries: []driver.GPUBindGroupEntry{{Binding: 0, Texture: 3}, {Binding: 1, Texture: 4}, {Binding: 2, Sampler: 2}}}},
			{Kind: driver.GPUOpCreateBindGroup, ID: 9, BindGroup: &driver.GPUBindGroupDesc{Layout: 7, Entries: []driver.GPUBindGroupEntry{{Binding: 0, Texture: 5}, {Binding: 1, Texture: 6}, {Binding: 2, Sampler: 2}}}},
			{Kind: driver.GPUOpCreateTexture, ID: 10, Texture: &driver.GPUTextureDesc{Width: w, Height: h, Format: driver.GPUFormatRGBA8Unorm, Usage: target}},
			{Kind: driver.GPUOpCreateTexture, ID: 11, Texture: &driver.GPUTextureDesc{Width: w, Height: h, Format: driver.GPUFormatRGBA8Unorm, Usage: target}},
			{Kind: driver.GPUOpCreatePipeline, ID: 20, Pipeline: &driver.GPUPipelineDesc{
				Vertex:   driver.GPUShaderStage{IR: prog.IR, EntryPoint: "vs_main"},
				Fragment: driver.GPUShaderStage{IR: prog.IR, EntryPoint: "fs_main"},
				Layouts:  []driver.GPUID{7},
				VertexBuffers: []driver.GPUVertexBufferLayout{{Stride: 16, Attributes: []driver.GPUVertexAttribute{
					{Location: 0, Offset: 0, Format: driver.GPUVertexFloat32x2},
					{Location: 1, Offset: 8, Format: driver.GPUVertexFloat32x2},
				}}},
				Primitive: driver.GPUPrimitiveState{Topology: driver.GPUTopologyTriangleList},
				Targets:   []driver.GPUColorTargetState{{Format: driver.GPUFormatRGBA8Unorm}},
			}},
			{Kind: driver.GPUOpReadTexture, ID: 10, ReadbackID: 1, ReadbackRegion: driver.GPUTextureRegion{Width: w, Height: h}},
			{Kind: driver.GPUOpReadTexture, ID: 11, ReadbackID: 2, ReadbackRegion: driver.GPUTextureRegion{Width: w, Height: h}},
		},
		Passes: []driver.GPUPass{pass(10, 8), pass(11, 9)},
	})
	read := map[uint64][]byte{}
	for _, ev := range events {
		switch ev.Kind {
		case driver.GPUEventError:
			t.Fatalf("GPU error: %s", ev.Message)
		case driver.GPUEventReadback:
			if ev.Message != "" {
				t.Fatalf("readback %d: %s", ev.ReadbackID, ev.Message)
			}
			read[ev.ReadbackID] = ev.Pixels
		}
	}
	imported, uploaded := read[1], read[2]
	if len(imported) != w*h*4 || !bytes.Equal(imported, uploaded) {
		t.Fatalf("imported NV12 rendered\n%v\nuploaded planes rendered\n%v", imported, uploaded)
	}
	// Pixel (3, 1) samples luma byte 1*w+3 and the chroma pair at (1, 0).
	if px := imported[(1*w+3)*4:][:4]; px[0] != frame[1*w+3] || px[1] != frame[w*h+2] || px[2] != frame[w*h+3] {
		t.Fatalf("pixel (3, 1) = %v, want Y %d U %d V %d", px, frame[1*w+3], frame[w*h+2], frame[w*h+3])
	}
	// The renderer gave key 1 back, so the decoder can take it again.
	if hr := mutex.call(keyedMutexAcquireSync, keyedMutexArgs(1, 1000)...); failed(hr) || uint32(hr) == d3dKeyedMutexWaitTimeout {
		t.Fatalf("decoder AcquireSync(1) after rendering = %#x", hr)
	}
	mutex.call(keyedMutexReleaseSync, keyedMutexArgs(1)...)

	if len(ops.shared) != 1 {
		t.Fatalf("both planes should view one opened texture, have %d", len(ops.shared))
	}
	// Closing the imports releases the views they made and the texture they opened.
	var views []*comObject
	for _, id := range []driver.GPUID{3, 4} {
		srv := ctrl.textures[id].handle.(*d3d11TextureHandle).srv
		srv.addRef()
		views = append(views, srv)
	}
	ctrl.Submit(driver.GPUFrame{Generation: 1, Ops: []driver.GPUResourceOp{{Kind: driver.GPUOpDestroyTexture, ID: 3}, {Kind: driver.GPUOpDestroyTexture, ID: 4}}})
	if ctrl.textures[3] != nil || ctrl.textures[4] != nil {
		t.Fatal("imported textures outlived their destroy")
	}
	for i, srv := range views {
		if refs := srv.call(comRelease); refs != 0 {
			t.Errorf("plane %d's view has %d references left after its destroy", i, refs)
		}
	}
	if len(ops.shared) != 0 {
		t.Errorf("the opened shared texture outlived its planes")
	}
}
