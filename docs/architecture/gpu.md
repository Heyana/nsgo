# GPU view

This is the architecture design for issues [#1104](https://github.com/nsgo-dev/nsgo/issues/1104) and [#1059](https://github.com/nsgo-dev/nsgo/issues/1059). It is not an implemented API yet; it settles the four design choices that gate NSGo's Stage 1 3D GPU view so the public API, shader IR extensions, and target backends can be built in parallel. Read this alongside [shaders](shaders.md), [package layers](layers.md), [frames](animation.md#frames), and the [games roadmap](../roadmap.md#games) before implementing any part of the GPU view.

`core.Shader` draws fragment-only shaders, view effects, and transitions using uniform lanes and up to four sampled images. A 3D game or data visualization needs persistent GPU resources (vertex, index, and uniform buffers, 2D/cube/depth/array textures, samplers, offscreen render targets, and immutable render pipelines with vertex and fragment stages) and per-frame command recording that does not trigger a UI view-tree commit on every frame.

## Decisions

The four decisions from [#1104](https://github.com/nsgo-dev/nsgo/issues/1104) are settled as follows:

| Decision | Choice | Rather than | Why |
| --- | --- | --- | --- |
| 1. API level | A small, portable WebGPU-shaped subset in a new `gpu` package, backed by `core`'s surface hook (`driver.KindGPU`) and `core.Env.SupportsGPU()` | Raw OpenGL/Vulkan bindings, a high-level scene graph, or placing all 3D types directly in `core` | Immutable pipelines, bind groups, and render passes map cleanly onto Metal, Direct3D 11, WebGPU, OpenGL 3.3, GLES 3.0, and WebGL 2; scene graphs belong in `nsgo-dev/game`; a dedicated `gpu` package keeps `core` small and preserves `TestMinimalAppLinks` |
| 2. Web backend | WebGPU where `navigator.gpu` is available, with automatic WebGL 2 fallback on `<canvas>` | WebGL 2 only, or WebGPU only | Reuses `internal/platform/web/shader.go`'s WebGL 2 path so older browsers, mobile WebViews, and headless CI work everywhere, while modern Chrome, Firefox, and Safari get native WebGPU pipelines, storage buffers, and future compute |
| 3. Android backend | OpenGL ES 3.0 on the Java runtime's existing shared GL thread for Stage 1; Vulkan scoped as Stage 2 via `purego` | Vulkan in Stage 1, or cgo/NDK bindings | Requires zero NDK/JDK/Android SDK on build machines (`classes.dex` rebuilt once), extends `ShaderNode.java`'s existing EGL/GLES 3.0 thread, and reaches ~99% of Android devices (`minSdkVersion` 28); arm64 Android `purego` lacks a bionic `dlopen` loader and cross-process `ANativeWindow` sharing today |
| 4. Target scope | Stage 1: macOS, iOS, Windows, Linux, Web, Android, and headless `nsgotest`/CI; Stage 2: HarmonyOS (and Android Vulkan); Out of scope: watchOS and mini programs | Shipping all targets at once, or emulating 3D on watchOS/mini programs | Matches the six targets in [#1047](https://github.com/nsgo-dev/nsgo/issues/1047); watchOS has no public Metal view host in SwiftUI and mini programs cap total packages at 2 MB with `nsgo_noshader` |

## API level and layering (Decision 1)

### Package layering

NSGo's dependency rules ([package layers](layers.md), enforced by `internal/moduletest/layering_test.go`) keep `core` platform-independent and keep heavy optional subsystems out of minimal app binaries (`TestMinimalAppLinks` in `internal/moduletest/linked_test.go`):

- **`gpu` (Components layer, portable)**: The public home of the 3D GPU API (`gpu.View`, `gpu.Device`, `gpu.Buffer`, `gpu.Texture`, `gpu.Sampler`, `gpu.BindGroupLayout`, `gpu.BindGroup`, `gpu.Pipeline`, `gpu.ShaderModule`, `gpu.Encoder`, `gpu.RenderPass`, `gpu.CommandBuffer`, and descriptor/error types). Like `audio` ([audio playback](audio.md#the-api)), `gpu` imports only `core` and is listed in `portable` in `layering_test.go` — no `purego`, `syscall`, or `unsafe`.
- **`core` (Core layer, portable)**: Owns the engine integration and names `gpu` in its comments, matching `core`'s part for `audio` and `mobile`:
  - `Env.SupportsGPU() bool` reports whether the active target and device can host a `gpu.View` (checked before mounting or creating resources without importing `gpu`).
  - The GPU view node (`driver.KindGPU`) mounts through its own `mountIn` method so apps that never mount a `gpu.View` link neither the GPU protocol encoder nor the vertex/fragment shader compilers.
  - Monotonic handle allocation (`driver.GPUID`), `*core.Scope` cleanup hooks, and routing of `EventGPUDeviceReady`, `EventGPUDeviceLost`, `EventGPUError`, and `EventGPUReadback` back onto the engine goroutine.
- **`internal/driver` (Protocol layer, portable)**: Defines serializable GPU resource descriptors, compact binary or typed command streams (`driver.GPUCommand`), `driver.KindGPU`, and GPU events, referencing `internal/shaderir` for compiled shader modules.
- **`internal/engine` (Engine layer, portable)**: Exposes a sideband GPU submission method on `engine.Platform` so per-frame GPU command buffers bypass shadow-tree commits while staying ordered with surface creation, resize, and deletion.
- **`internal/platform/<target>` (Backends layer)**: Translates `driver.GPUCommand` streams and `shaderir.Program` pipelines onto Metal, Direct3D 11, OpenGL 3.3/ES 3.0, WebGPU/WebGL 2, or Android GLES 3.0. Backends import only `internal/shaderir/{msl,hlsl,glsl,wgsl}` writers, never the front-end compilers.

Keeping the API in `gpu` rather than widening `core` avoids adding ~30 3D descriptor and enum types to `core`, keeps `core.Shader` dedicated to fragment-only UI shaders and effects, and ensures an ordinary UI app links zero GPU surface code.

### Zero view-tree commits per frame

NSGo's [architecture boundaries](../../ARCHITECTURE.md#boundaries) require that the main thread never waits for the engine, the engine never touches main-thread-only UI objects, and per-frame rendering never drives a view-tree commit per frame:

1. **Mount once**: Mounting `gpu.View` emits one `ItemCreate` (`driver.KindGPU`) in the initial `MountBatch`, with `ItemFrame` only when layout resizes the view and `ItemDelete` when it unmounts. Native NSGo views (HUD, pause menus, overlays) sit as siblings or `Children` of `gpu.View` and continue to use normal layout, accessibility, and input hit testing.
2. **Record on the engine goroutine**: Inside `cx.RequestAnimationFrame(func(at time.Time) { ... })` ([frames](animation.md#frames)), the game steps simulation, updates uniform/instance buffers with `dev.WriteBuffer`, records one or more `RenderPass`es into an `Encoder`, and calls `dev.Submit(enc.Finish())`. No signal write or `MountBatch` commit occurs unless the HUD itself changed.
3. **Submit off-commit to the render thread**: `dev.Submit` hands the immutable command slice to `engine.Platform.SubmitGPU(surfaceID, viewID, generation, cmds)` without blocking the engine goroutine. Each backend queues the batch for its render thread (Apple's Metal render goroutine, Android's shared GL `HandlerThread`, Windows' D3D11 render context, Linux's `GtkGLArea` render callback, or Web's `requestAnimationFrame` pass) and coalesces or paces presentation to the display.

### Go API sketch

The following sketch shows the planned Stage 1 surface in package `gpu` (plus `core.Env.SupportsGPU`). Names and signatures are the implementation target for [#1059](https://github.com/nsgo-dev/nsgo/issues/1059).

```go
package gpu

import (
	"image"
	"github.com/nsgo-dev/nsgo/core"
)

// View mounts a native GPU-backed surface in the view tree.
// Children render over the GPU surface as ordinary NSGo views (HUD, menus).
type View struct {
	// ColorFormat defaults to the platform's preferred swapchain format
	// (BGRA8Unorm on Apple/Windows, RGBA8Unorm on GL/Web).
	ColorFormat TextureFormat
	// DepthFormat enables an automatic swapchain depth/stencil attachment
	// resized with the view (e.g. Depth24Plus or Depth24PlusStencil8).
	// Zero disables the default depth buffer.
	DepthFormat TextureFormat
	// SampleCount is 1 (default) or 4 for swapchain MSAA.
	SampleCount int

	// OnReady runs on the engine goroutine when the GPU device and surface
	// are ready, and again after recovery from device loss.
	OnReady func(dev *Device)
	// OnResize runs when the drawable size in physical pixels changes.
	OnResize func(width, height int)
	// OnDeviceLost runs if the underlying GPU context or device is lost;
	// all handles created from the previous Device are invalidated.
	OnDeviceLost func(DeviceLostEvent)
	// OnError reports asynchronous GPU validation or out-of-memory errors.
	OnError func(Error)

	Class     string
	Style     core.Style
	StyleFunc func() core.Style
	Children  []core.Node
	OnPress   func(core.PressEvent)
	OnLayout  func(core.Bounds)
	Ref       *core.Ref
	Label     string
	TestID    string
}

// Shader compilation (portable, validated on the engine into shaderir.Program).
func CompileWGSL(src string) (*ShaderModule, error)
func MustCompileWGSL(src string) *ShaderModule
func CompileKage(src string) (*ShaderModule, error)

// Device owns persistent GPU resources and accepts recorded command buffers.
// All methods are called on the engine goroutine.
type Device struct { /* opaque handle + owning surface */ }

func (d *Device) Limits() Limits
func (d *Device) Features() Features
func (d *Device) SurfaceFormat() TextureFormat
func (d *Device) SurfaceSize() (width, height int)

func (d *Device) CreateBuffer(cx *core.Scope, desc BufferDesc) (*Buffer, error)
func (d *Device) CreateTexture(cx *core.Scope, desc TextureDesc) (*Texture, error)
func (d *Device) CreateTextureFromImage(cx *core.Scope, src core.ImageSource, desc TextureDesc) (*Texture, error)
func (d *Device) CreateSampler(cx *core.Scope, desc SamplerDesc) (*Sampler, error)
func (d *Device) CreateBindGroupLayout(cx *core.Scope, entries []BindGroupLayoutEntry) (*BindGroupLayout, error)
func (d *Device) CreateBindGroup(cx *core.Scope, layout *BindGroupLayout, entries []BindGroupEntry) (*BindGroup, error)
func (d *Device) CreatePipeline(cx *core.Scope, desc PipelineDesc) (*Pipeline, error)

// WriteBuffer and WriteTexture copy data before returning so callers may reuse
// Go slices immediately on the engine goroutine.
func (d *Device) WriteBuffer(buf *Buffer, offset uint64, data []byte)
func (d *Device) WriteTexture(tex *Texture, dst TextureCopy, data []byte, layout DataLayout)
func (d *Device) WriteTextureRGBA(tex *Texture, mipLevel, arrayLayer uint32, img *image.RGBA)

// Frame recording and submission (zero UI view-tree commits).
func (d *Device) BeginEncoder() *Encoder
func (d *Device) Submit(cmds ...*CommandBuffer)

// ReadTexture copies a texture region back to CPU memory asynchronously and
// invokes fn on the engine goroutine (used for screenshots, picking, and CI).
func (d *Device) ReadTexture(tex *Texture, region TextureRegion, fn func(rgba []byte, err error))
```

Resource descriptors and render pass recording follow a minimal WebGPU subset:

```go
type BufferUsage uint32

const (
	UsageVertex  BufferUsage = 1 << iota
	UsageIndex
	UsageUniform
	UsageStorage // read-only in Stage 1 where supported (Features.StorageBuffers)
	UsageCopySrc
	UsageCopyDst
)

type BufferDesc struct {
	Label string
	Size  uint64
	Usage BufferUsage
	Data  []byte // optional initial contents
}

type TextureDimension uint8

const (
	Dim2D TextureDimension = iota
	DimCube
	Dim2DArray
)

type TextureFormat uint8

const (
	FormatUndefined TextureFormat = iota
	FormatRGBA8Unorm
	FormatRGBA8UnormSRGB
	FormatBGRA8Unorm
	FormatRG8Unorm
	FormatR8Unorm
	FormatRGBA16Float
	FormatDepth24Plus
	FormatDepth24PlusStencil8
	FormatDepth32Float
)

type TextureUsage uint32

const (
	TextureSampled TextureUsage = 1 << iota
	TextureRenderAttachment
	TextureCopySrc
	TextureCopyDst
)

type TextureDesc struct {
	Label       string
	Dimension   TextureDimension
	Width       uint32
	Height      uint32
	Layers      uint32 // 1 for 2D, 6 for Cube, N for 2DArray
	MipLevels   uint32 // 0 defaults to 1
	SampleCount int    // 0 or 1; 4 for MSAA render attachments
	Format      TextureFormat
	Usage       TextureUsage
}

type SamplerDesc struct {
	MinFilter     FilterMode  // FilterNearest, FilterLinear
	MagFilter     FilterMode
	MipmapFilter  FilterMode
	AddressU      AddressMode // AddressClampToEdge, AddressRepeat, AddressMirrorRepeat
	AddressV      AddressMode
	AddressW      AddressMode
	Compare       CompareFunc // non-zero for shadow-map comparison samplers
	MaxAnisotropy uint16      // 0 or 1 disables anisotropic filtering
}

type PipelineDesc struct {
	Label         string
	Vertex        ShaderStage // Module + EntryPoint (e.g. "vs_main")
	Fragment      ShaderStage // Module + EntryPoint (e.g. "fs_main")
	Layouts       []*BindGroupLayout
	VertexBuffers []VertexBufferLayout // Stride, StepMode (Vertex/Instance), Attributes (Format, Offset, Location)
	Primitive     PrimitiveState       // Topology, FrontFace (CCW/CW), CullMode (None/Front/Back)
	DepthStencil  *DepthStencilState   // Format, DepthWrite, DepthCompare, Stencil, DepthBias
	Targets       []ColorTargetState   // Format, Blend (*BlendState), WriteMask
	SampleCount   int                  // 0 or 1, or 4
}

// Encoder records render passes and copies for one Submit call.
type Encoder struct { /* ... */ }

func (e *Encoder) Device() *Device
func (e *Encoder) BeginRenderPass(desc RenderPassDesc) *RenderPass
func (e *Encoder) CopyBufferToBuffer(src *Buffer, srcOffset uint64, dst *Buffer, dstOffset, size uint64)
func (e *Encoder) CopyBufferToTexture(src BufferCopy, dst TextureCopy, width, height, layers uint32)
func (e *Encoder) CopyTextureToTexture(src, dst TextureCopy, width, height, layers uint32)
func (e *Encoder) Finish() *CommandBuffer

// RenderPassDesc targets either the view's current swapchain drawable
// (ColorAttachments[0].Texture == nil) or offscreen TextureRenderAttachment textures.
type RenderPassDesc struct {
	ColorAttachments []ColorAttachment // Texture (nil = surface), ResolveTarget, LoadOp, StoreOp, ClearColor
	DepthStencil     *DepthStencilAttachment
}

type RenderPass struct { /* ... */ }

func (p *RenderPass) Device() *Device
func (p *RenderPass) SetViewport(x, y, width, height, minDepth, maxDepth float32)
func (p *RenderPass) SetScissor(x, y, width, height int32)
func (p *RenderPass) SetPipeline(pipe *Pipeline)
func (p *RenderPass) SetBindGroup(slot uint32, group *BindGroup, dynamicOffsets ...uint32)
func (p *RenderPass) SetVertexBuffer(slot uint32, buf *Buffer, offset uint64)
func (p *RenderPass) SetIndexBuffer(buf *Buffer, format IndexFormat, offset uint64)
func (p *RenderPass) SetStencilReference(ref uint32)
func (p *RenderPass) SetBlendConstant(r, g, b, a float32)
func (p *RenderPass) Draw(vertexCount, firstVertex uint32)
func (p *RenderPass) DrawIndexed(indexCount, firstIndex uint32, baseVertex int32)
func (p *RenderPass) DrawInstanced(vertexCount, instanceCount, firstVertex, firstInstance uint32)
func (p *RenderPass) DrawIndexedInstanced(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32)
func (p *RenderPass) End()
```

A draw is skipped while the pass holds no usable pipeline, or while a resource the pipeline declares is missing: a vertex buffer, bind group or index buffer whose bind was rejected — nil, closed, from another device or from an earlier device epoch — marks its slot, and a later valid bind to that slot lets drawing resume. A rejected bind for a slot the pipeline does not declare leaves the draws alone. Nothing draws against the resources bound before a rejected bind.

A pipeline's colour targets must fit the pass's attachments: the same number of colour targets, the same formats, a sample count the attachment has, and a depth format the pass's depth attachment has. `SetPipeline` with one that does not fit sets no pipeline, so the pass's draws are skipped rather than recorded for a backend that would refuse them, and the encoder leaves a debug marker naming the mismatch. Typical per-frame usage on the engine goroutine:

Typical per-frame usage on the engine goroutine:

```go
if !cx.Env().SupportsGPU() {
	return fallbackNotice()
}
return gpu.View{
	Class:       "flex-1",
	DepthFormat: gpu.FormatDepth24Plus,
	OnReady: func(dev *gpu.Device) {
		// Create persistent vertex/index/uniform buffers, textures, samplers, and pipelines
		// scoped to cx; released automatically when cx disposes or the view unmounts.
		mesh := uploadMesh(cx, dev)
		var frame func(time.Time)
		frame = func(at time.Time) {
			dev.WriteBuffer(mesh.uniforms, 0, packCamera(at))
			enc := dev.BeginEncoder()
			pass := enc.BeginRenderPass(gpu.RenderPassDesc{
				ColorAttachments: []gpu.ColorAttachment{{
					LoadOp:     gpu.LoadClear,
					StoreOp:    gpu.StoreStore,
					ClearColor: core.RGB(0x10141c),
				}},
				DepthStencil: &gpu.DepthStencilAttachment{
					DepthLoadOp:  gpu.LoadClear,
					DepthStoreOp: gpu.StoreDiscard,
					ClearDepth:   1.0,
				},
			})
			pass.SetPipeline(mesh.pipeline)
			pass.SetBindGroup(0, mesh.bindings)
			pass.SetVertexBuffer(0, mesh.vertices, 0)
			pass.SetIndexBuffer(mesh.indices, gpu.IndexUint16, 0)
			pass.DrawIndexed(mesh.indexCount, 0, 0)
			pass.End()
			dev.Submit(enc.Finish())
			cx.RequestAnimationFrame(frame)
		}
		cx.RequestAnimationFrame(frame)
	},
}
```

### Resource lifetime, errors, and device loss

- **Disposal via `*core.Scope`**: Every resource created through `Device` (`Buffer`, `Texture`, `RenderTarget`, `Sampler`, `BindGroupLayout`, `BindGroup`, `Pipeline`, `ComputePipeline`, `TimestampQuery`, `RingBuffer`) implements `Close() error` for immediate release and also registers a cleanup on the `*core.Scope` passed to its constructor, on its running owner as `Scope.Cleanup` does, so a resource created while an effect runs closes when the effect runs again. `Close()` takes that cleanup off and removes the resource from the device's retained lists, so a closed resource keeps nothing on its scope or device and recreating resources under a long-lived scope (such as resize-dependent render targets) does not grow either. Unmounting `gpu.View`, disposing the owning scope, or closing the engine closes all remaining live handles, last created first, enqueues release commands, and cancels any pending `ReadTexture` callbacks. Late backend events targeting released IDs are dropped.
- **Validation and error reporting**: Descriptor errors (misaligned uniform offsets, missing usage flags, vertex layout mismatches, invalid shader entry points, or unavailable features) are validated synchronously in Go on the engine goroutine and returned as `error` values from `Create*` and `Compile*` before reaching the backend. Runtime backend errors (driver compilation rejection, GPU out-of-memory) arrive asynchronously on `View.OnError` as `gpu.Error` (`ErrUnsupported`, `ErrInvalidDescriptor`, `ErrShaderCompile`, `ErrOutOfMemory`, `ErrDeviceLost`, `ErrFailed`).
- **Device-loss recovery**: Desktop driver resets (`DXGI_ERROR_DEVICE_REMOVED` on Windows), browser tab/GPU resets (`webglcontextlost`, `GPUDevice.lost`), and Android EGL context destruction (`EGL_CONTEXT_LOST` on Activity pause) all map to one contract:
  1. The backend emits `EventGPUDeviceLost` with a reason (`LostReset`, `LostDestroyed`, `LostOutOfMemory`).
  2. `core` bumps the surface's device generation, marks all handles from the lost `Device` invalid (subsequent writes/submits become no-ops rather than panicking), and calls `View.OnDeviceLost`.
  3. When the platform restores or recreates the device/context, it emits `EventGPUDeviceReady` with the new generation and `core` invokes `View.OnReady` with a fresh `*gpu.Device` so the app rebuilds its resources cleanly. `BufferDesc`, `TextureDesc`, `RenderTargetDesc`, `SamplerDesc`, `BindGroupLayoutDesc` and `PipelineDesc` each carry `Retained bool`, which recreates that resource in a restore on its own, as `ViewProps.RetainResources` does for all of them.

### Imported textures

`Device.ImportTexture` ([#2296](https://github.com/nsgo-dev/nsgo/issues/2296)) opens GPU memory another device owns, such as a hardware decoder's frame, as a sampled 2D texture without copying it; `Device.SupportsExternalTexture(kind)` (`Features.ImportD3D11Shared`, `Features.ImportDMABuf`) says whether a target can, and elsewhere it returns `ErrUnsupported`, so callers fall back to `WriteTexture`.

- **One plane per import**: an NV12 frame is two textures, luma `FormatR8Unorm` and chroma `FormatRG8Unorm` at half size, and P010's are the import-only `FormatR16Unorm` and `FormatRG16Unorm`, which `CreateTexture` rejects. Shaders keep a plain `texture_2d` and convert YUV themselves, so no backend needs `samplerExternalOES` or a YCbCr sampler.
- **Windows**: the backend opens the NT handle with `ID3D11Device1::OpenSharedResource1` once per handle within a `Submit`, so planes imported together share that resource (opening a handle twice can yield two resources, and the keyed mutex of one does not make the other's writes visible) while handle values closed and recycled by Windows in a later `Submit` open the new resource, and views the plane with `CreateShaderResourceView1`'s `PlaneSlice`. With `KeyedMutex`, each frame whose passes bind the texture acquires `AcquireKey` (100 ms at most, reporting a timeout on `OnError`) before the passes and releases `ReleaseKey` after them.
- **Linux**: the backend makes an EGLImage with `EGL_EXT_image_dma_buf_import` (naming the modifier where `_modifiers` is present) on the display `eglGetCurrentDisplay` gives inside the `GtkGLArea`'s context, and binds it with `glEGLImageTargetTexture2DOES`; a GLX context reports no support.
- **Lifetime**: the caller keeps the handle or file descriptors, open until the first frame submitted after the import completes. Imported textures are only sampled: writes, copies, readbacks and render passes on them are dropped or rejected, and they are not retained across device loss. Closing one releases what the backend opened after the frames already submitted, so a decoder's frame goes back to its pool once the `FrameFence` of the last frame that sampled it completes.

### HDR swapchains

`gpu.ViewProps.ColorSpace` (`ColorSpaceDefault`, `ColorSpaceSRGB`, `ColorSpaceExtendedLinearSRGB`, `ColorSpaceHDR10PQ`, `ColorSpaceHDR10HLG`, `ColorSpaceDisplayP3`) and `gpu.FormatRGB10A2Unorm` let a view request an HDR swapchain alongside `core.DisplayInfo`'s HDR reporting (`HDR`, `HDRSupported`, `HDRHeadroom`, `SDRWhiteLevel`, `MaxLuminance`) ([#2358](https://github.com/nsgo-dev/nsgo/issues/2358)). Backends negotiate what the platform supports and report the active format and color space through `Device.SurfaceFormat()` and `Device.SurfaceColorSpace()`, falling back to SDR sRGB when HDR output is unavailable:

- **macOS & iOS (`internal/platform/apple`)**: `CAMetalLayer` configures `kCGColorSpaceExtendedLinearSRGB`, `kCGColorSpaceITUR_2100_PQ`, `kCGColorSpaceITUR_2100_HLG` or `kCGColorSpaceDisplayP3` with `RGBA16Float` or `RGB10A2Unorm` and enables `wantsExtendedDynamicRangeContent` on macOS.
- **Windows (`internal/platform/windows`)**: HDR views create a DirectComposition flip-model swap chain (`IDXGIFactory2::CreateSwapChainForComposition`) configured via `IDXGISwapChain3::SetColorSpace1` (`DXGI_COLOR_SPACE_RGB_FULL_G10_NONE_P709` for `RGBA16Float` scRGB, `DXGI_COLOR_SPACE_RGB_FULL_G2084_NONE_P2020` for `RGB10A2Unorm`/`RGBA16Float` HDR10 PQ) and compose it as a DirectComposition portal so HUD children render above it.
- **Linux (`internal/platform/linux`)**: on Wayland compositors advertising `wp_color_management_v1`, `gpu.View` attaches a `wp_color_management_surface_v1` image description to its `wl_surface` and renders into an internal `RGBA16F` or `RGB10A2` framebuffer when HDR is negotiated.

## Web backend: WebGPU with WebGL 2 fallback (Decision 2)

On the web (`internal/platform/web`, [#1108](https://github.com/nsgo-dev/nsgo/issues/1108)), a `gpu.View` mounts a `<canvas>` sized to physical pixels (`width * pixelRatio`, `height * pixelRatio`), reusing the canvas sizing and animation-frame lifecycle already established in `internal/platform/web/shader.go`:

1. **WebGPU primary path**: When `navigator.gpu` is present, the backend asynchronously requests a `GPUAdapter` and `GPUDevice` and configures `canvas.getContext("webgpu")` with `navigator.gpu.getPreferredCanvasFormat()`. Once ready, it emits `EventGPUDeviceReady`. `shaderir` emits validated WGSL directly for `createShaderModule`; `BindGroup`, `RenderPipeline`, `RenderPassEncoder`, instancing, and read-only storage buffers map 1:1 onto WebGPU objects.
2. **WebGL 2 automatic fallback**: When `navigator.gpu` is absent, `requestAdapter()` resolves to `null`, or device creation fails (older Safari/Firefox, embedded mobile WebViews, software headless Chrome without WebGPU flags), the backend transparently falls back to `canvas.getContext("webgl2", {alpha: true, premultipliedAlpha: true, depth: ..., stencil: ..., antialias: ...})` on the same `<canvas>`:
   - `shaderir` compiles the same shaders to GLSL ES 3.00 (`internal/shaderir/glsl`).
   - `Buffer` maps to `WebGLBuffer`, `Texture` to `WebGLTexture` (`TEXTURE_2D`, `TEXTURE_CUBE_MAP`, `TEXTURE_2D_ARRAY`), `Sampler` to `WebGLSampler`, offscreen render targets to `WebGLFramebuffer`, and `Pipeline` to a linked `WebGLProgram` plus cached state and VAO layout descriptors.
   - `BindGroup` maps uniform buffers via `bindBufferBase(UNIFORM_BUFFER, ...)` / `bindBufferRange` and pairs textures with samplers on active texture units.
   - `DrawInstanced` and `DrawIndexedInstanced` map directly to `drawArraysInstanced` and `drawElementsInstanced`.
   - `Device.Features()` reports `StorageBuffers: false` and `Compute: false` so apps stay within the portable Stage 1 baseline (uniform buffers + textures + instancing) unless they check `Features()`.
3. **Device loss and context budget**: Both `GPUDevice.lost` and `webglcontextlost`/`webglcontextrestored` flow into `OnDeviceLost` and `OnReady`. Because browsers cap active WebGL contexts (~8–16 per tab, as noted in `internal/platform/web/shader.go`), games should host one `gpu.View` per scene rather than one per widget.

## Android backend: OpenGL ES 3.0 first, Vulkan in Stage 2 (Decision 3)

NSGo runs Android apps as a Go executable started by a prebuilt Java runtime (`classes.dex` and `classes.jar`) communicating over pipes, with no NDK, JDK, or Android SDK required on the user's machine ([principles](../../AGENTS.md#principles), [platforms](../../ARCHITECTURE.md#platforms)).

### Stage 1: OpenGL ES 3.0 on the shared GL thread

`internal/platform/android/java/dev/oneo/nsgo/ShaderNode.java` already creates one shared `EGL14` display/context and a dedicated `HandlerThread` (`gl`) that renders OpenGL ES 3.0 into `TextureView` surfaces without blocking the Android UI thread. Stage 1 ([#1109](https://github.com/nsgo-dev/nsgo/issues/1109)) builds on that proven infrastructure:

- **Java runtime host (`GpuNode.java`)**: A sibling to `ShaderNode.java` backed by `TextureView` (so it participates cleanly in Android view clipping, z-ordering, and native HUD overlays) executing on the shared GL `HandlerThread`, compiled once into the committed `classes.dex` and `classes.jar` via `internal/platform/android/runtime/build.sh`.
- **Coverage and toolchain**: NSGo's Android `minSdkVersion` is 28 (Android 9), where OpenGL ES 3.0 is mandatory across ~99% of active devices. App authors build Android APKs/AABs with Go alone — zero NDK or C++ toolchain.
- **Shader reuse**: Go translates each `shaderir.Program` once to GLSL ES 3.00 (`internal/shaderir/glsl`) and sends the vertex and fragment sources to Java with the `CreatePipeline` command, matching `internal/platform/android/shader.go`.
- **Feature completeness**: GLES 3.0 supports every Stage 1 primitive natively: VBO/IBO/VAO, `std140` UBOs, 2D/cube/2D-array/depth textures, `glGenSamplers`, FBO render targets and MRTs, depth/stencil state, face culling, blending, MSAA renderbuffers (`glRenderbufferStorageMultisample` + `glBlitFramebuffer`), and instanced drawing (`glDrawArraysInstanced`/`glDrawElementsInstanced`).
- **Pipe transport**: Static meshes and textures upload once at load time and stay resident as GL objects on the Java GL thread. Per-frame `Submit` streams contain only compact binary draw/uniform commands (typically a few kilobytes per frame) sent over the existing pipe. For large initial buffer/texture uploads that exceed a single pipe chunk budget, the backend streams chunked payloads (or hands off an unlinked temporary file/shared memory buffer when sizes warrant it) so UI commits are never starved.
- **Lifecycle**: `SurfaceTexture` destruction or `EGL_CONTEXT_LOST` on Activity pause triggers `EventGPUDeviceLost`; recreation on resume restores the EGL surface and triggers `EventGPUDeviceReady`.

### Stage 2: Vulkan via `purego`

Vulkan on Android is explicitly deferred to Stage 2 once workloads need compute shaders, read/write storage buffers, or lower CPU/pipe overhead than the Java GL thread provides. Two architectural prerequisites in NSGo must land first:

1. **Cross-process surface sharing**: the Android backend still needs a safe Vulkan surface handoff between the Java runtime and Go before Vulkan can replace the OpenGL path. The no-cgo bionic loader is shared with `base/terminal` now.
2. **Cross-process surface sharing**: Because the Go app runs in a child process while Android Activities own `Surface`/`ANativeWindow`, presenting Vulkan swapchains directly from Go requires passing an `ANativeWindow` / `Surface` over Binder or adopting the process-lifetime changes tracked in [#12](https://github.com/nsgo-dev/nsgo/issues/12).

## Target scope (Decision 4)

Stage 1 delivers the six game targets from [#1047](https://github.com/nsgo-dev/nsgo/issues/1047) plus headless `nsgotest` / CI. Secondary targets have explicit Stage 2 or out-of-scope rules so apps can rely on `core.Env.SupportsGPU()`:

| Target | Stage | Backend API | Surface host | Shader target | `Env.SupportsGPU()` |
| --- | --- | --- | --- | --- | --- |
| macOS (`internal/platform/macos` + `apple`) | Stage 1 ([#1105](https://github.com/nsgo-dev/nsgo/issues/1105)) | Metal via `purego` / Obj-C runtime (`CGO_ENABLED=0`) | `CAMetalLayer` on a dedicated render goroutine | MSL | `true` (when `MTLCreateSystemDefaultDevice` succeeds) |
| iOS (`internal/platform/ios` + `apple`) | Stage 1 ([#1105](https://github.com/nsgo-dev/nsgo/issues/1105)) | Metal via `purego` / Obj-C runtime | `CAMetalLayer` on a dedicated render goroutine | MSL | `true` |
| Windows (`internal/platform/windows`) | Stage 1 ([#1106](https://github.com/nsgo-dev/nsgo/issues/1106)) | Direct3D 11 via `syscall` / COM vtables (`D3DCompile`) | Shared DXGI/Direct3D 11 surface composited with Direct2D | HLSL SM 5.0 | `true` (hardware or WARP D3D11 device) |
| Linux (`internal/platform/linux`) | Stage 1 ([#1107](https://github.com/nsgo-dev/nsgo/issues/1107)) | OpenGL 3.3 Core or OpenGL ES 3.0 via `libepoxy` / `purego` | `GtkGLArea` | GLSL 3.30 / GLSL ES 3.00 | `true` (when GTK creates a GL 3.3 / ES 3.0 context) |
| Web (`internal/platform/web`) | Stage 1 ([#1108](https://github.com/nsgo-dev/nsgo/issues/1108)) | WebGPU (`navigator.gpu`) primary; WebGL 2 fallback via `syscall/js` | `<canvas>` | WGSL / GLSL ES 3.00 | `true` (when WebGPU or WebGL 2 is available) |
| Android (`internal/platform/android`) | Stage 1 ([#1109](https://github.com/nsgo-dev/nsgo/issues/1109)) | OpenGL ES 3.0 on Java shared GL thread (`EGL14`/`GLES30`); Vulkan in Stage 2 | `TextureView` (`GpuNode.java`) | GLSL ES 3.00 | `true` (API 28+ GLES 3.0) |
| Headless `nsgotest` | Stage 1 ([#1110](https://github.com/nsgo-dev/nsgo/issues/1110)) | Command validation & recording backend + target offscreen CI readback | Headless model / offscreen target | Validated `shaderir` / native target | `true` by default (configurable in `nsgotest.Options`) |
| tvOS (`internal/platform/ios`) | Stage 2 (shared Apple Metal) | Metal via `internal/platform/apple` | `CAMetalLayer` | MSL | Deferred until verified on tvOS simulator/device |
| HarmonyOS (`internal/platform/harmonyos`) | Stage 2 | `XComponent` GLES 3.0/Vulkan or ArkWeb WebGL 2 | ArkUI `XComponent` / `<canvas>` | GLSL ES 3.00 | `false` in Stage 1 |
| watchOS (`internal/platform/watchos`) | Out of scope | None | SwiftUI host has no Metal view | None | `false` |
| Mini programs (`internal/platform/miniprogram`) | Out of scope | None (`nsgo_noshader`, 2 MB package cap) | Logic-layer WASM has no GPU surface | None | `false` |

On any target where `Env.SupportsGPU()` is `false`, mounting a `gpu.View` renders its `Children` (or an empty box) without touching native GPU APIs and invokes `OnError(gpu.ErrUnsupported)` rather than `OnReady`.

## Shader IR extensions (`internal/shaderir`)

`internal/shaderir/program.go` already carries `Attributes []Type`, `Varyings []Type`, `VertexFunc`, and `FragmentFunc` from Ebitengine's IR, while `internal/shader/wgsl` and `internal/shaderir/{msl,hlsl,glsl}/fragment.go` currently expose only fragment-only shaders (`fn shade(uv: vec2f) -> vec4f`, one flat uniform block, and up to four untyped `texture2d` reads via `TexelAt`).

Extending `internal/shaderir` and `internal/shader/wgsl` for full 3D pipelines keeps one front end and zero C/Rust dependencies (no naga, no shaderc/SPIRV-Cross) while leaving `core.Shader`'s existing `FragmentLibrary()` path untouched:

### Required IR and front-end additions

1. **Vertex and fragment entry points**:
   - Accept `@vertex` and `@fragment` entry points in `internal/shader/wgsl` (and optional `Vertex` entry point in Kage `internal/shader`), compiling into `Program.VertexFunc` and `Program.FragmentFunc`. Reserve `@compute` for Stage 2.
   - Support multiple entry points in one WGSL module so a vertex shader and fragment shader can share structs, constants, and helper functions in a single source string.
2. **Vertex attribute locations and inter-stage varyings**:
   - Add explicit `Location uint32` metadata on `Attributes` and `Varyings` (`@location(N)`), plus interpolation qualifiers (`Perspective`, `Flat`, `Linear`) on varyings so integer attributes (bone indices, material IDs) and flat-shaded normals generate valid `flat` qualifiers in GLSL/MSL/HLSL.
3. **Stage built-ins**:
   - Add IR expressions/inputs/outputs for `@builtin(position)` (vertex output and fragment input), `@builtin(vertex_index)`, `@builtin(instance_index)`, `@builtin(front_facing)`, and `@builtin(frag_depth)`. Reserve `@builtin(sample_index)` and `@builtin(sample_mask)` for MSAA follow-ups.
4. **Explicit resource bindings (`@group(G) @binding(B)`)**:
   - Add a `Bindings []Binding` table on `shaderir.Program` alongside the legacy flat `Uniforms`/`TextureCount` fields so GPU pipelines declare multiple bind groups (for example: group 0 per-frame camera/lights, group 1 per-material uniforms/textures, group 2 per-object transforms):
     - `BindingUniformBuffer`: typed `Struct` block with std140/WGSL alignment rules (no 4096-byte single-block cap).
     - `BindingStorageBufferRead`: read-only typed array/struct buffer (`var<storage, read>`) for instancing, skinning palettes, and light arrays (enabled when `Features.StorageBuffers` is true).
     - Typed textures extending `shaderir.BasicType`: `Texture2D`, `TextureCube`, `Texture2DArray`, `TextureDepth2D`, `TextureMultisampled2D`.
     - Explicit samplers: `Sampler` (`sampler`) and `SamplerComparison` (`sampler_comparison`).
5. **User structs, non-square matrices, and sampling built-ins**:
   - Complete `Struct` member layout, alignment, and byte-size computation in `internal/shaderir/type.go` (resolving its existing `TODO`) so WGSL vertex inputs, vertex outputs, and uniform structs check and pack deterministically.
   - Add non-square matrix types (`mat2x3`, `mat3x2`, `mat2x4`, `mat4x2`, `mat3x4`, `mat4x3`) and sampling built-ins: `textureSample`, `textureSampleLevel`, `textureSampleBias`, `textureSampleGrad`, `textureSampleCompare`, `textureLoad`, and `textureDimensions`.
6. **Canonical coordinate and clip-space normalization**:
   - All shaders author against WebGPU conventions: right-handed or left-handed chosen by the app's projection matrix, NDC depth in `[0, 1]`, clip-space `+Y` up, texture UV `(0, 0)` at top-left, and `FrontFace` defaulting to `CCW`.
   - Metal, Direct3D 11, and WebGPU share `[0, 1]` NDC depth and top-left UV origin natively. For OpenGL 3.3 / GLSL ES 3.00 / WebGL 2, the GLSL vertex writer emits `gl_Position.z = gl_Position.z * 2.0 - gl_Position.w;` before exit, and the OpenGL backends upload top-row-first texture data consistently with UV orientation so one shader draws identically on every target.

### Codegen matrix

| Target writer | Used by | Entry points | Vertex inputs | Uniform & storage bindings | Textures & samplers | Clip-space Z |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/shaderir/msl` | macOS, iOS, (tvOS) | `vertex` / `fragment` functions | `[[stage_in]]` struct with `[[attribute(N)]]` | `constant T& [[buffer(slot)]]` / `device const T* [[buffer(slot)]]` (slot table mapped from `(group, binding)`) | `texture2d`/`texturecube`/`texture2d_array`/`depth2d` `[[texture(N)]]`, `sampler [[sampler(N)]]` | Native `[0, 1]` |
| `internal/shaderir/hlsl` | Windows (D3D11 SM 5.0) | `vs_5_0` / `ps_5_0` functions | `TEXCOORDn` input struct matching `D3D11_INPUT_ELEMENT_DESC` | `cbuffer : register(bN)` / `StructuredBuffer<T> : register(tN)` | `Texture2D`/`TextureCube`/`Texture2DArray` `: register(tN)`, `SamplerState`/`SamplerComparisonState : register(sN)` | Native `[0, 1]` |
| `internal/shaderir/glsl` | Linux (3.30 / ES 3.00), WebGL 2 (ES 3.00), Android (ES 3.00) | `void main()` VS + FS | `layout(location = N) in` | `layout(std140) uniform` block bound per flattened slot (`glUniformBlockBinding`) | Synthesized combined `sampler2D`/`samplerCube`/`sampler2DArray`/`sampler2DShadow` per `(texture, sampler)` pair used in the shader, paired with `glBindSampler` | Remapped `[0, 1]` → `[-1, 1]` at VS exit |
| `internal/shaderir/wgsl` | WebGPU | `@vertex fn` / `@fragment fn` | `@location(N)` struct/params | `@group(G) @binding(B) var<uniform>` / `var<storage, read>` | `texture_2d`/`texture_cube`/`texture_2d_array`/`texture_depth_2d` + `sampler`/`sampler_comparison` | Native `[0, 1]` |

For OpenGL's combined texture+sampler model, the GLSL writer emits one `uniform sampler*` per static `(textureBinding, samplerBinding)` pair referenced by sampling calls in the shader and returns the pair table alongside the GLSL source so the Linux, WebGL 2, and Android backends bind each texture unit and `glBindSampler` object deterministically.

## Headless testing and CI contract (`nsgotest`)

Testing follows NSGo's two-tier model ([testing](testing.md)):

1. **Headless command recorder (`go tool nsgo test`)**:
   - `nsgotest` implements the `driver` GPU surface purely in Go with zero native GPU dependencies.
   - `nsgotest.Options` lets tests set `SupportsGPU: false` to verify graceful fallback UI, and `GPULimits` to test conservative limits.
   - Every `Create*`, `Write*`, `BeginRenderPass`, `Set*`, `Draw*`, `Copy*`, and `Submit` call runs the exact descriptor and binding validation rules (vertex stride/format bounds, bind group layout compatibility, attachment format match against pipeline `Targets`/`DepthStencil`, index buffer bounds) and appends structured records to the view's log.
   - Tests inspect recorded frames via `app.GPU(testID)` (`Frames()`, `Passes()`, `Draws()`, `Buffers()`, `Textures()`, `Pipelines()`, `ReadBuffer(id)`), inject simulated resets via `app.TriggerGPUDeviceLost(testID, reason)` and `app.RestoreGPUDevice(testID)`, and satisfy `ReadTexture` with either deterministic mock buffers or a tiny optional software clear/triangle path so readback wiring is testable without hardware.
2. **Offscreen target pixel tests (`nsgo test -target ...`, [#1110](https://github.com/nsgo-dev/nsgo/issues/1110))**:
   - A shared `internal/platform/gputest` suite renders canonical scenes into offscreen `TextureRenderAttachment` targets and reads pixels back through `Device.ReadTexture` (or native window screenshots):
     - Clear color and viewport/scissor clipping;
     - Depth-tested intersecting triangles and back-face culling;
     - Textured quad with nearest/linear filtering, wrap modes, and mipmaps;
     - Lit 3D cube with vertex normals, uniform buffer updates, and alpha blending;
     - Instanced draw (`DrawIndexedInstanced`);
     - Offscreen render-to-texture pass sampled by a second pass;
     - Resize and device-loss recovery.
   - Runs in CI across Apple Silicon (Metal), Windows (D3D11 / WARP), Linux (Xvfb + Mesa `llvmpipe` / GL 3.3), Web (headless Chrome with WebGPU and forced WebGL 2 fallback), and Android (emulator GLES 3.0).

## Implementation plan

Work is staged in reviewable issues matching the roadmap:

### Stage 1 — Foundation and six target backends ([#1059](https://github.com/nsgo-dev/nsgo/issues/1059))

1. **Foundation (`gpu`, `core`, `internal/driver`, `internal/engine`, `internal/shaderir`)**:
   - Add the `gpu` package and its rule in `internal/moduletest/layering_test.go` (`portable`, imports `core` only; `examples/` allowed to import `gpu`).
   - Add `core.Env.SupportsGPU()`, `driver.KindGPU`, GPU handle/lifecycle plumbing, and sideband `engine.Platform.SubmitGPU`.
   - Extend `internal/shaderir`, `internal/shader/wgsl`, and the four writers (`msl`, `hlsl`, `glsl`, `wgsl`) for vertex+fragment stages, `@location`, `@builtin`, `@group`/`@binding`, structs, samplers, and GLSL clip-space Z remap.
2. **[#1105](https://github.com/nsgo-dev/nsgo/issues/1105) — macOS and iOS (`internal/platform/apple`)**:
   - Metal `GPUView` on `CAMetalLayer` via `purego` and Objective-C runtime, extending `internal/platform/apple/shader.go`'s dedicated render goroutine so `nextDrawable` never blocks the main thread.
   - `MTLBuffer`, `MTLTexture` (2D/cube/2D-array/depth/MSAA), `MTLSamplerState`, `MTLRenderPipelineState`, `MTLDepthStencilState`, `MTLCommandBuffer` / `MTLRenderCommandEncoder`, and async blit readback.
   - Implemented in `internal/platform/apple/gpu.go`: passes that target the view draw into a persistent surface texture, which a presenting frame copies into the layer's drawable, so a pass that loads the surface and `Device.ReadSurface` both see the last frame. Uploads and readbacks blit through buffers in submission order, `Depth24Plus` maps to `Depth32Float` (Apple GPUs have no 24-bit depth), and a pipeline's Metal state is made per depth attachment format the first time a pass uses it. tvOS reports `SupportsGPU() == false` until it is verified.
3. **[#1106](https://github.com/nsgo-dev/nsgo/issues/1106) — Windows (`internal/platform/windows`)**:
   - Direct3D 11 backend via `syscall` and COM vtables on the Direct2D/DXGI device (`ID3D11Device`, `ID3D11DeviceContext`, `D3DCompile` HLSL SM 5.0), compositing the GPU view cleanly beneath native views.
   - Full `DXGI_ERROR_DEVICE_REMOVED` / `DXGI_ERROR_DEVICE_RESET` detection and recovery into `OnDeviceLost` / `OnReady`.
4. **[#1107](https://github.com/nsgo-dev/nsgo/issues/1107) — Linux (`internal/platform/linux`)**:
   - OpenGL 3.3 Core / OpenGL ES 3.0 backend on `GtkGLArea` loaded through `libepoxy` via `purego`, extending `internal/platform/linux/shader.go`.
   - Per-context VAO cache, UBO/texture/sampler binding tables, offscreen FBOs, and headless Xvfb + Mesa `llvmpipe` pixel tests.
5. **[#1108](https://github.com/nsgo-dev/nsgo/issues/1108) — Web (`internal/platform/web`)**:
   - WebGPU (`navigator.gpu`) primary backend with automatic WebGL 2 fallback on `<canvas>` via `syscall/js`, covering both paths in headless Chrome tests (with a flag/override to exercise the WebGL 2 fallback explicitly).
6. **[#1109](https://github.com/nsgo-dev/nsgo/issues/1109) — Android (`internal/platform/android`)**:
   - OpenGL ES 3.0 `GpuNode.java` on the Java runtime's shared GL `HandlerThread`, binary pipe command/upload encoding, and `classes.dex`/`classes.jar` rebuild via `internal/platform/android/runtime/build.sh`.
7. **[#1110](https://github.com/nsgo-dev/nsgo/issues/1110) — Headless `nsgotest` and CI**:
   - Complete `nsgotest` GPU command validator/recorder, device-loss test hooks, and the shared `internal/platform/gputest` offscreen pixel-readback suite wired into CI.

### Follow-up GPU capabilities

Tracked as staged follow-up issues once Stage 1 (`#1105`–`#1110`) is green on all six targets:

- **Pipeline, format, and rendering extensions ([#1189](https://github.com/nsgo-dev/nsgo/issues/1189)–[#1196](https://github.com/nsgo-dev/nsgo/issues/1196))**:
  - Compressed GPU texture formats (BCn on desktops, ASTC/ETC2 on mobile) with capability probing;
  - HDR / floating-point render targets, linear/sRGB swapchain tone-mapping, and MSAA depth/color resolves;
  - Multiple render targets (MRT) and shadow-map comparison sampling (`textureSampleCompare`);
  - 4x MSAA render targets with pass-end and explicit `Encoder.ResolveTexture` resolves (a resolve render pass, so every backend with MSAA resolve targets runs it), pipeline stencil testing, up to four color targets, and the extended `FormatRG11B10Float` / `FormatR32Float` / `FormatRGBA32Float` / `FormatBGRA8UnormSRGB` / `FormatStencil8` formats ([#1192](https://github.com/nsgo-dev/nsgo/issues/1192), [#1235](https://github.com/nsgo-dev/nsgo/issues/1235), [#1236](https://github.com/nsgo-dev/nsgo/issues/1236)). They run on the software device today; the native backends report `Features.StencilTest` and `Features.ExtendedFormats` false and `Limits.MaxColorAttachments` 1 until each wires them, and the `gpu` package rejects those features there with `ErrUnsupported` / `ErrInvalidDescriptor`;
  - Dynamic uniform buffer offsets, ring-buffered streaming updates, mipmap generation, and GPU timing/profiling queries.
- **Compute, interop, and Stage 2 backends ([#1234](https://github.com/nsgo-dev/nsgo/issues/1234)–[#1240](https://github.com/nsgo-dev/nsgo/issues/1240))**:
  - Compute pipelines (`@compute @workgroup_size(X, Y, Z)`, `var<storage, read>`, `var<storage, read_write>`, `texture_storage_2d`, `Device.CreateComputePipeline`, `Encoder.BeginComputePass`, `Dispatch` / `DispatchIndirect`, `Device.ReadBuffer`) with `Device.Capabilities().Features.Compute` / `Device.SupportsCompute()` capability queries ([#1234](https://github.com/nsgo-dev/nsgo/issues/1234)). Compute shaders compile to MSL (`kernel void`), HLSL SM 5.0 (`[numthreads]` + `StructuredBuffer`/`RWStructuredBuffer`), WGSL, and GLSL 4.30 / ES 3.10 (`layout(local_size_*) in;` + `std430` SSBOs), and dispatch runs today on the headless software device (`gpu.NewSoftwareRenderer`, `nsgotest`). The Metal, Direct3D 11, WebGPU, WebGL 2, OpenGL and Android GLES backends still report `Features.Compute` false until each wires compute dispatch. Where `SupportsCompute()` is false, creating compute pipelines, read/write storage or storage texture bindings, dispatching, or `ReadBuffer` returns `gpu.ErrUnsupported`, and a compute pass records nothing; applications should fall back to CPU updates (`Device.WriteBuffer`) or ping-pong render-to-texture passes. The software device evaluates the shader builtins it runs — trigonometry, exponentials and logarithms, `sign`, `trunc`, `round`, `mod`, `distance`, `faceForward`, `refract`, `transpose`, `determinant`, the bit casts and the matrix constructors — against Go's `math` package, returns zero for derivatives because the rasterizer draws one fragment at a time, and refuses a pipeline that calls any other builtin rather than drawing a zero where the call is ([#2334](https://github.com/nsgo-dev/nsgo/issues/2334)).
  - Zero-copy interop with `core.Image`, `core.Video`, and pixel buffers, and importing `CVPixelBuffer` / IOSurface frames on Metal ([#1105](https://github.com/nsgo-dev/nsgo/issues/1105)); [imported textures](#imported-textures) cover Direct3D 11 and DMA-BUF;
  - Stage 2 **Android Vulkan** (`purego` once bionic `dlopen` and cross-process surface sharing land) and Stage 2 **HarmonyOS** GPU surface support.

### Completion criteria

Issue [#1059](https://github.com/nsgo-dev/nsgo/issues/1059) closes when a depth-tested, textured, lit 3D mesh (with instancing and an offscreen render-target pass) renders inside a `gpu.View` with native NSGo HUD controls layered over it, submits frames from `Scope.RequestAnimationFrame` with zero UI view-tree commits per frame, passes `internal/platform/gputest` pixel checks on macOS, iOS, Windows, Linux, Web (both WebGPU and WebGL 2 fallback), and Android, passes headless `nsgotest` command validation and device-loss recovery tests, and reports `Env.SupportsGPU() == false` cleanly on watchOS, mini programs, and Stage 1 HarmonyOS.
