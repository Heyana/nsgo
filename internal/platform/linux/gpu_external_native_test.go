//go:build linux && !android

package linux

import (
	"bytes"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// gbmAPI is the part of libgbm the test makes DMA-BUFs with, standing in for a decoder's VAAPI surfaces.
type gbmAPI struct {
	createDevice  func(fd int32) uintptr
	destroyDevice func(dev uintptr)
	boCreate      func(dev uintptr, w, h, format, flags uint32) uintptr
	boDestroy     func(bo uintptr)
	boMap         func(bo uintptr, x, y, w, h, flags uint32, stride *uint32, data *uintptr) uintptr
	boUnmap       func(bo, data uintptr)
	boGetFD       func(bo uintptr) int32
	boGetStride   func(bo uintptr) uint32
	boGetOffset   func(bo uintptr, plane int32) uint32
}

const (
	gbmBOUseRendering = 1 << 2
	gbmBOUseLinear    = 1 << 4
	gbmBOTransferW    = 2
)

func openGBM(t *testing.T) (*gbmAPI, uintptr) {
	lib, err := purego.Dlopen("libgbm.so.1", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Skipf("no libgbm: %v", err)
	}
	g := &gbmAPI{}
	purego.RegisterLibFunc(&g.createDevice, lib, "gbm_create_device")
	purego.RegisterLibFunc(&g.destroyDevice, lib, "gbm_device_destroy")
	purego.RegisterLibFunc(&g.boCreate, lib, "gbm_bo_create")
	purego.RegisterLibFunc(&g.boDestroy, lib, "gbm_bo_destroy")
	purego.RegisterLibFunc(&g.boMap, lib, "gbm_bo_map")
	purego.RegisterLibFunc(&g.boUnmap, lib, "gbm_bo_unmap")
	purego.RegisterLibFunc(&g.boGetFD, lib, "gbm_bo_get_fd")
	purego.RegisterLibFunc(&g.boGetStride, lib, "gbm_bo_get_stride")
	purego.RegisterLibFunc(&g.boGetOffset, lib, "gbm_bo_get_offset")
	node, err := os.OpenFile("/dev/dri/renderD128", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no DRM render node: %v", err)
	}
	dev := g.createDevice(int32(node.Fd()))
	if dev == 0 {
		node.Close()
		t.Skip("gbm_create_device failed")
	}
	t.Cleanup(func() {
		g.destroyDevice(dev)
		node.Close()
	})
	return g, dev
}

// plane makes a linear w x h DMA-BUF in fourcc holding texels, rows of w*bpp bytes, and returns it as an import's plane.
func (g *gbmAPI) plane(t *testing.T, dev uintptr, w, h, bpp, fourcc uint32, texels []byte) gpu.DMABufPlane {
	bo := g.boCreate(dev, w, h, fourcc, gbmBOUseLinear|gbmBOUseRendering)
	if bo == 0 {
		t.Skipf("GBM cannot make linear %#08x buffers", fourcc)
	}
	t.Cleanup(func() { g.boDestroy(bo) })
	var stride uint32
	var mapData uintptr
	ptr := g.boMap(bo, 0, 0, w, h, gbmBOTransferW, &stride, &mapData)
	if ptr == 0 {
		t.Skip("gbm_bo_map failed")
	}
	dst := unsafe.Slice((*byte)(cPointer(ptr)), int(stride*h))
	for y := range h {
		copy(dst[y*stride:], texels[y*w*bpp:(y+1)*w*bpp])
	}
	g.boUnmap(bo, mapData)
	fd := g.boGetFD(bo)
	if fd < 0 {
		t.Fatal("gbm_bo_get_fd failed")
	}
	t.Cleanup(func() { syscall.Close(int(fd)) })
	return gpu.DMABufPlane{FD: int(fd), Offset: g.boGetOffset(bo, 0), Pitch: g.boGetStride(bo)}
}

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

// TestLinuxGPUViewImportedDMABufMatchesUpload imports an NV12 frame's planes from DMA-BUFs into a gpu.View and checks that sampling them renders exactly what sampling the same bytes uploaded with WriteTexture does, and that closing the imports releases their EGLImages.
func TestLinuxGPUViewImportedDMABufMatchesUpload(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	const w, h = 8, 4
	luma := make([]byte, w*h)
	for i := range luma {
		luma[i] = byte(16 + i*7)
	}
	chroma := make([]byte, w*h/2)
	for i := range chroma {
		chroma[i] = byte(240 - i*13)
	}
	g, gbmDev := openGBM(t)
	lumaPlane := g.plane(t, gbmDev, w, h, 1, drmFormatR8, luma)
	chromaPlane := g.plane(t, gbmDev, w/2, h/2, 2, drmFormatGR88, chroma)

	mod := gpu.MustCompileWGSL(yuvPlanesWGSL)
	quad := packF32(
		-1, -1, 0, 1,
		1, -1, 1, 1,
		1, 1, 1, 0,
		-1, -1, 0, 1,
		1, 1, 1, 0,
		-1, 1, 0, 0,
	)
	type result struct {
		imported, uploaded []byte
		unsupported        bool
		err                error
	}
	done := make(chan result, 1)
	finish := func(r result) {
		select {
		case done <- r:
		default:
		}
	}
	var imports []*gpu.Texture
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		cx.SetTitle(title)
		if !cx.Env().SupportsGPU() {
			finish(result{unsupported: true})
			return core.View{}
		}
		var (
			pipe        *gpu.Pipeline
			vb          *gpu.Buffer
			groups      [2]*gpu.BindGroup
			targets     [2]*gpu.RenderTarget
			sent        bool
			got         [2][]byte
			pendingRead int
		)
		return gpu.View(cx, gpu.ViewProps{
			Style:   core.Style{Width: driver.Pt(64), Height: driver.Pt(64)},
			OnError: func(err gpu.Error) { finish(result{err: err}) },
			OnReady: func(dev *gpu.Device) {
				if !dev.SupportsExternalTexture(gpu.ExternalDMABuf) {
					finish(result{unsupported: true})
					return
				}
				fail := func(err error) bool {
					if err != nil {
						finish(result{err: err})
					}
					return err != nil
				}
				y, err := dev.ImportTexture(cx, gpu.ExternalTextureDesc{Kind: gpu.ExternalDMABuf, Width: w, Height: h, Format: gpu.FormatR8Unorm, FourCC: drmFormatR8, Planes: []gpu.DMABufPlane{lumaPlane}})
				if fail(err) {
					return
				}
				uv, err := dev.ImportTexture(cx, gpu.ExternalTextureDesc{Kind: gpu.ExternalDMABuf, Width: w / 2, Height: h / 2, Format: gpu.FormatRG8Unorm, FourCC: drmFormatGR88, Planes: []gpu.DMABufPlane{chromaPlane}})
				if fail(err) {
					return
				}
				imports = []*gpu.Texture{y, uv}
				upY, err := dev.CreateTexture(cx, gpu.TextureDesc{Width: w, Height: h, Format: gpu.FormatR8Unorm, Usage: gpu.TextureSampled | gpu.TextureCopyDst})
				if fail(err) {
					return
				}
				dev.WriteTexture(upY, gpu.TextureCopy{Texture: upY}, luma, gpu.DataLayout{BytesPerRow: w, RowsPerImage: h})
				upUV, err := dev.CreateTexture(cx, gpu.TextureDesc{Width: w / 2, Height: h / 2, Format: gpu.FormatRG8Unorm, Usage: gpu.TextureSampled | gpu.TextureCopyDst})
				if fail(err) {
					return
				}
				dev.WriteTexture(upUV, gpu.TextureCopy{Texture: upUV}, chroma, gpu.DataLayout{BytesPerRow: w, RowsPerImage: h / 2})
				samp, err := dev.CreateSampler(cx, gpu.SamplerDesc{MinFilter: gpu.FilterNearest, MagFilter: gpu.FilterNearest, AddressU: gpu.AddressClampToEdge, AddressV: gpu.AddressClampToEdge})
				if fail(err) {
					return
				}
				layout, err := dev.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
					{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
					{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
					{Binding: 2, Visibility: gpu.StageFragment, Type: gpu.BindingSampler},
				})
				if fail(err) {
					return
				}
				for i, planes := range [2][2]*gpu.Texture{{y, uv}, {upY, upUV}} {
					if groups[i], err = dev.CreateBindGroup(cx, layout, []gpu.BindGroupEntry{{Binding: 0, Texture: planes[0]}, {Binding: 1, Texture: planes[1]}, {Binding: 2, Sampler: samp}}); fail(err) {
						return
					}
					if targets[i], err = dev.CreateRenderTarget(cx, gpu.RenderTargetDesc{Width: w, Height: h, ColorFormat: gpu.FormatRGBA8Unorm}); fail(err) {
						return
					}
				}
				if vb, err = dev.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: quad}); fail(err) {
					return
				}
				pipe, err = dev.CreatePipeline(cx, gpu.PipelineDesc{
					Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
					Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
					Layouts:  []*gpu.BindGroupLayout{layout},
					VertexBuffers: []gpu.VertexBufferLayout{{Stride: 16, Attributes: []gpu.VertexAttribute{
						{Location: 0, Offset: 0, Format: gpu.VertexFloat32x2},
						{Location: 1, Offset: 8, Format: gpu.VertexFloat32x2},
					}}},
					Primitive: gpu.PrimitiveState{Topology: gpu.TopologyTriangleList},
					Targets:   []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
				})
				fail(err)
			},
			OnFrame: func(frame *gpu.Frame) {
				if pipe == nil || sent {
					return
				}
				sent = true
				for i := range 2 {
					pass := frame.BeginRenderPass(gpu.RenderPassDesc{ColorAttachments: []gpu.ColorAttachment{{Texture: targets[i].Color, LoadOp: gpu.LoadClear, StoreOp: gpu.StoreStore}}})
					pass.SetPipeline(pipe)
					pass.SetBindGroup(0, groups[i])
					pass.SetVertexBuffer(0, vb, 0)
					pass.Draw(6, 0)
					pass.End()
				}
				frame.Submit()
				pendingRead = 2
				for i := range 2 {
					frame.Device().ReadTexture(targets[i].Color, gpu.TextureRegion{Width: w, Height: h}, func(rgba []byte, err error) {
						if err != nil {
							finish(result{err: err})
							return
						}
						got[i] = rgba
						if pendingRead--; pendingRead == 0 {
							finish(result{imported: got[0], uploaded: got[1]})
						}
					})
				}
			},
		})
	}
	win := startTestWindow(t, app, Options{Title: title, Width: 64, Height: 64})
	waitForTitle(t, win, title)
	var r result
	select {
	case r = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the imported frame was not rendered")
	}
	switch {
	case r.unsupported:
		t.Skip("GTK's GL context is not EGL, or lacks DMA-BUF import")
	case r.err != nil:
		t.Fatal(r.err)
	}
	if len(r.imported) != w*h*4 || !bytes.Equal(r.imported, r.uploaded) {
		t.Fatalf("imported DMA-BUFs rendered\n%v\nuploaded planes rendered\n%v", r.imported, r.uploaded)
	}
	// Pixel (3, 1) samples luma byte 1*w+3 and the chroma pair at (1, 0).
	if px := r.imported[(1*w+3)*4:][:4]; px[0] != luma[1*w+3] || px[1] != chroma[2] || px[2] != chroma[3] {
		t.Fatalf("pixel (3, 1) = %v, want Y %d U %d V %d", px, luma[1*w+3], chroma[2], chroma[3])
	}

	// Closing the imports destroys their EGLImages.
	countImages := func() (n int) {
		win.onLoop(t, func() {
			for _, v := range win.views {
				if v != nil && v.gpu != nil && v.gpu.ctx != nil {
					for _, tex := range v.gpu.ctx.textures {
						if tex.eglImage != 0 {
							n++
						}
					}
				}
			}
		})
		return n
	}
	if n := countImages(); n != 2 {
		t.Fatalf("%d EGLImages before closing the imports, want 2", n)
	}
	win.onLoop(t, func() {
		for _, tex := range imports {
			_ = tex.Close()
		}
	})
	deadline := time.Now().Add(3 * time.Second)
	for countImages() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := countImages(); n != 0 {
		t.Fatalf("%d EGLImages left after closing the imports", n)
	}
}
