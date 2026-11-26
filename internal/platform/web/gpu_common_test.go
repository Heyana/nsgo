package web

import (
	"bytes"
	"go/constant"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

func TestSelectGPUBackend(t *testing.T) {
	cases := []struct {
		name string
		caps gpuProbeCaps
		want gpuBackendKind
	}{
		{
			name: "webgpu primary when navigator.gpu, adapter, and device succeed",
			caps: gpuProbeCaps{HasNavigatorGPU: true, WebGPUAdapterOK: true, WebGPUDeviceOK: true, HasWebGL2: true},
			want: gpuBackendWebGPU,
		},
		{
			name: "webgpu primary even without webgl2",
			caps: gpuProbeCaps{HasNavigatorGPU: true, WebGPUAdapterOK: true, WebGPUDeviceOK: true, HasWebGL2: false},
			want: gpuBackendWebGPU,
		},
		{
			name: "fallback to webgl2 when navigator.gpu absent",
			caps: gpuProbeCaps{HasNavigatorGPU: false, HasWebGL2: true},
			want: gpuBackendWebGL2,
		},
		{
			name: "fallback to webgl2 when requestAdapter returns null",
			caps: gpuProbeCaps{HasNavigatorGPU: true, WebGPUAdapterOK: false, WebGPUDeviceOK: false, HasWebGL2: true},
			want: gpuBackendWebGL2,
		},
		{
			name: "fallback to webgl2 when requestDevice fails",
			caps: gpuProbeCaps{HasNavigatorGPU: true, WebGPUAdapterOK: true, WebGPUDeviceOK: false, HasWebGL2: true},
			want: gpuBackendWebGL2,
		},
		{
			name: "forced webgl2 skips webgpu even when available",
			caps: gpuProbeCaps{HasNavigatorGPU: true, WebGPUAdapterOK: true, WebGPUDeviceOK: true, HasWebGL2: true, ForceWebGL2: true},
			want: gpuBackendWebGL2,
		},
		{
			name: "none when neither webgpu nor webgl2 is available",
			caps: gpuProbeCaps{HasNavigatorGPU: false, HasWebGL2: false},
			want: gpuBackendNone,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := selectGPUBackend(tc.caps); got != tc.want {
				t.Fatalf("selectGPUBackend(%+v) = %s, want %s", tc.caps, got, tc.want)
			}
		})
	}

	if !gpuEnvSupported(true, false) || !gpuEnvSupported(false, true) || !gpuEnvSupported(true, true) {
		t.Fatal("gpuEnvSupported must be true whenever WebGPU or WebGL 2 is present")
	}
	if gpuEnvSupported(false, false) {
		t.Fatal("gpuEnvSupported must be false when neither WebGPU nor WebGL 2 is present")
	}
}

func TestWebGPUDescriptorMappings(t *testing.T) {
	// Formats round-trip.
	formats := []struct {
		f driver.GPUTextureFormat
		s string
	}{
		{driver.GPUFormatRGBA8Unorm, "rgba8unorm"},
		{driver.GPUFormatRGBA8UnormSRGB, "rgba8unorm-srgb"},
		{driver.GPUFormatBGRA8Unorm, "bgra8unorm"},
		{driver.GPUFormatRG8Unorm, "rg8unorm"},
		{driver.GPUFormatR8Unorm, "r8unorm"},
		{driver.GPUFormatRGBA16Float, "rgba16float"},
		{driver.GPUFormatDepth24Plus, "depth24plus"},
		{driver.GPUFormatDepth24PlusStencil8, "depth24plus-stencil8"},
		{driver.GPUFormatDepth32Float, "depth32float"},
	}
	for _, tc := range formats {
		if got := wgslTextureFormat(tc.f); got != tc.s {
			t.Errorf("wgslTextureFormat(%d) = %q, want %q", tc.f, got, tc.s)
		}
		if got := driverFormatFromWebGPU(tc.s); got != tc.f {
			t.Errorf("driverFormatFromWebGPU(%q) = %d, want %d", tc.s, got, tc.f)
		}
	}

	// Buffer usage always includes COPY_DST for WriteBuffer safety.
	if u := webgpuBufferUsage(driver.GPUBufferVertex | driver.GPUBufferUniform); u != (webgpuBufVertex | webgpuBufUniform | webgpuBufCopyDst) {
		t.Errorf("webgpuBufferUsage = 0x%x, unexpected", u)
	}
	if u := webgpuBufferUsage(driver.GPUBufferIndex | driver.GPUBufferStorage | driver.GPUBufferCopySrc); u != (webgpuBufIndex | webgpuBufStorage | webgpuBufCopySrc | webgpuBufCopyDst) {
		t.Errorf("webgpuBufferUsage storage = 0x%x, unexpected", u)
	}

	// Single-sample color texture gets COPY_SRC|COPY_DST; MSAA and depth textures do not.
	if u := webgpuTextureUsage(driver.GPUTextureSampled, 1, false); u != (webgpuTexBinding | webgpuTexCopyDst | webgpuTexCopySrc) {
		t.Errorf("single-sample color usage = 0x%x", u)
	}
	if u := webgpuTextureUsage(driver.GPUTextureRenderAttachment, 4, false); u != webgpuTexRenderAttachment {
		t.Errorf("MSAA usage = 0x%x, want 0x%x", u, webgpuTexRenderAttachment)
	}
	if u := webgpuTextureUsage(driver.GPUTextureRenderAttachment, 1, true); u != webgpuTexRenderAttachment {
		t.Errorf("depth usage = 0x%x, want 0x%x", u, webgpuTexRenderAttachment)
	}

	// Vertex formats, topology, cull, blend, compare, lost reason.
	if got := webgpuVertexFormat(driver.GPUVertexFloat32x3); got != "float32x3" {
		t.Errorf("webgpuVertexFormat = %q", got)
	}
	if got := webgpuVertexFormat(driver.GPUVertexUnorm8x4); got != "unorm8x4" {
		t.Errorf("webgpuVertexFormat unorm8x4 = %q", got)
	}
	if got := webgpuVertexStepMode(driver.GPUStepInstance); got != "instance" {
		t.Errorf("webgpuVertexStepMode = %q", got)
	}
	if got := webgpuIndexFormat(driver.GPUIndexUint32); got != "uint32" {
		t.Errorf("webgpuIndexFormat = %q", got)
	}
	if got := webgpuTopology(driver.GPUTopologyLineStrip); got != "line-strip" {
		t.Errorf("webgpuTopology = %q", got)
	}
	if got := webgpuFrontFace(driver.GPUFrontCW); got != "cw" {
		t.Errorf("webgpuFrontFace = %q", got)
	}
	if got := webgpuCullMode(driver.GPUCullBack); got != "back" {
		t.Errorf("webgpuCullMode = %q", got)
	}
	if got := webgpuBlendFactor(driver.GPUBlendOneMinusSrcAlpha); got != "one-minus-src-alpha" {
		t.Errorf("webgpuBlendFactor = %q", got)
	}
	if got := webgpuBlendOp(driver.GPUBlendReverseSubtract); got != "reverse-subtract" {
		t.Errorf("webgpuBlendOp = %q", got)
	}
	if got := webgpuCompareFunc(driver.GPUCompareLessEqual); got != "less-equal" {
		t.Errorf("webgpuCompareFunc = %q", got)
	}
	if got := webgpuLoadOp(driver.GPULoadLoad); got != "load" {
		t.Errorf("webgpuLoadOp = %q", got)
	}
	if got := webgpuStoreOp(driver.GPUStoreDiscard); got != "discard" {
		t.Errorf("webgpuStoreOp = %q", got)
	}
	if got := webgpuVisibility(0); got != (webgpuStageVertex | webgpuStageFragment) {
		t.Errorf("webgpuVisibility(0) = 0x%x", got)
	}
	if got := webgpuLostReason("destroyed"); got != driver.GPULostDestroyed {
		t.Errorf("webgpuLostReason(destroyed) = %d", got)
	}
	if got := webgpuLostReason("unknown"); got != driver.GPULostReset {
		t.Errorf("webgpuLostReason(unknown) = %d", got)
	}
	if got := webgpuReadbackBytesPerRow(10, 4); got != 256 {
		t.Errorf("webgpuReadbackBytesPerRow(10, 4) = %d, want 256", got)
	}
	if got := webgpuReadbackBytesPerRow(65, 4); got != 512 {
		t.Errorf("webgpuReadbackBytesPerRow(65, 4) = %d, want 512", got)
	}
}

func TestWebGL2DescriptorMappingsAndPixelHelpers(t *testing.T) {
	if fi := glFormatInfo(driver.GPUFormatDepth32Float); !fi.IsDepth || fi.HasStencil || fi.InternalFormat != glDepthComponent32F {
		t.Errorf("glFormatInfo(Depth32Float) = %+v", fi)
	}
	if fi := glFormatInfo(driver.GPUFormatDepth24PlusStencil8); !fi.IsDepth || !fi.HasStencil || fi.InternalFormat != glDepth24Stencil8 {
		t.Errorf("glFormatInfo(Depth24PlusStencil8) = %+v", fi)
	}
	if fi := glFormatInfo(driver.GPUFormatRGBA16Float); fi.IsDepth || fi.InternalFormat != glRGBA16F || fi.Type != glHalfFloat {
		t.Errorf("glFormatInfo(RGBA16Float) = %+v", fi)
	}
	if got := glTextureTarget(driver.GPUDimCube); got != glTextureCubeMap {
		t.Errorf("glTextureTarget(Cube) = 0x%x", got)
	}
	if got := glTextureTarget(driver.GPUDim2DArray); got != glTexture2DArray {
		t.Errorf("glTextureTarget(2DArray) = 0x%x", got)
	}
	if got := glBufferTarget(driver.GPUBufferUniform); got != glUniformBuffer {
		t.Errorf("glBufferTarget(Uniform) = 0x%x", got)
	}
	if got := glBufferTarget(driver.GPUBufferIndex); got != glElementArrayBuffer {
		t.Errorf("glBufferTarget(Index) = 0x%x", got)
	}
	if va := glVertexAttrInfo(driver.GPUVertexUint32x2); !va.Integer || va.Components != 2 || va.GLType != glUnsignedInt {
		t.Errorf("glVertexAttrInfo(Uint32x2) = %+v", va)
	}
	if va := glVertexAttrInfo(driver.GPUVertexUnorm8x4); va.Integer || !va.Normalized || va.Components != 4 {
		t.Errorf("glVertexAttrInfo(Unorm8x4) = %+v", va)
	}
	if enabled, face := glCullFace(driver.GPUCullBack); !enabled || face != glBack {
		t.Errorf("glCullFace(Back) = %v, 0x%x", enabled, face)
	}
	if enabled, _ := glCullFace(driver.GPUCullNone); enabled {
		t.Error("glCullFace(None) should disable culling")
	}
	if got := glMinFilter(driver.GPUFilterLinear, driver.GPUFilterNearest, true); got != glLinearMipmapNearest {
		t.Errorf("glMinFilter = 0x%x", got)
	}
	if got := glMinFilter(driver.GPUFilterLinear, driver.GPUFilterLinear, false); got != glLinear {
		t.Errorf("glMinFilter no mips = 0x%x", got)
	}
	if typ, sz := glIndexType(driver.GPUIndexUint16); typ != glUnsignedShort || sz != 2 {
		t.Errorf("glIndexType(Uint16) = 0x%x, %d", typ, sz)
	}

	// Top-left to bottom-left viewport/scissor Y inversion.
	if y := glViewportY(10, 30, 100); y != 60 {
		t.Errorf("glViewportY(10, 30, 100) = %d, want 60", y)
	}
	if y := glScissorY(0, 100, 100); y != 0 {
		t.Errorf("glScissorY(0, 100, 100) = %d, want 0", y)
	}

	// Row flip helper (2x2 RGBA).
	pix := []byte{
		1, 2, 3, 4, 5, 6, 7, 8, // row 0 (bottom)
		9, 10, 11, 12, 13, 14, 15, 16, // row 1 (top)
	}
	glFlipRowsRGBA(pix, 2, 2)
	wantFlipped := []byte{
		9, 10, 11, 12, 13, 14, 15, 16,
		1, 2, 3, 4, 5, 6, 7, 8,
	}
	if !bytes.Equal(pix, wantFlipped) {
		t.Errorf("glFlipRowsRGBA = %v, want %v", pix, wantFlipped)
	}

	// Strip 256-byte row padding and swizzle BGRA -> RGBA.
	padded := make([]byte, 512)
	// Row 0, pixel 0: B=10, G=20, R=30, A=255
	copy(padded[0:4], []byte{10, 20, 30, 255})
	// Row 1, pixel 0: B=40, G=50, R=60, A=128
	copy(padded[256:260], []byte{40, 50, 60, 128})
	rgba := stripRowPaddingRGBA(padded, 1, 2, 256, true)
	wantRGBA := []byte{30, 20, 10, 255, 60, 50, 40, 128}
	if !bytes.Equal(rgba, wantRGBA) {
		t.Errorf("stripRowPaddingRGBA(BGRA) = %v, want %v", rgba, wantRGBA)
	}
}

func samplePipelineIR() *shaderir.Program {
	// Struct Uniforms { mvp: mat4 }; @group(0) @binding(0) uniform; @group(0) @binding(1) texture2d; @group(0) @binding(2) sampler
	uStruct := shaderir.Type{
		Main: shaderir.Struct,
		Sub:  []shaderir.Type{{Main: shaderir.Mat4}},
	}
	return &shaderir.Program{
		VertexEntry:   "vs_main",
		FragmentEntry: "fs_main",
		Attributes: []shaderir.Type{
			{Main: shaderir.Vec3},
			{Main: shaderir.Vec2},
		},
		AttributeMeta: []shaderir.StageIO{
			{Location: 0},
			{Location: 1},
		},
		Varyings: []shaderir.Type{
			{Main: shaderir.Vec2},
		},
		VaryingMeta: []shaderir.StageIO{
			{Location: 0},
		},
		Bindings: []shaderir.Binding{
			{Group: 0, Binding: 0, Kind: shaderir.BindingUniformBuffer, Type: uStruct},
			{Group: 0, Binding: 1, Kind: shaderir.BindingTexture, Type: shaderir.Type{Main: shaderir.Texture2D}},
			{Group: 0, Binding: 2, Kind: shaderir.BindingSampler, Type: shaderir.Type{Main: shaderir.Sampler}},
		},
		VertexFunc: shaderir.VertexFunc{
			Block: &shaderir.Block{
				Stmts: []shaderir.Stmt{
					// position = B0.M0 * vec4(a0, 1.0)
					{
						Type: shaderir.Assign,
						Exprs: []shaderir.Expr{
							{Type: shaderir.LocalVariable, Index: 2},
							{
								Type: shaderir.Binary,
								Op:   shaderir.MatrixMul,
								Exprs: []shaderir.Expr{
									{
										Type: shaderir.FieldSelector,
										Exprs: []shaderir.Expr{
											{Type: shaderir.BindingVariable, Index: 0},
											{Type: shaderir.StructMember, Index: 0},
										},
									},
									{
										Type: shaderir.Call,
										Exprs: []shaderir.Expr{
											{Type: shaderir.BuiltinFuncExpr, BuiltinFunc: shaderir.Vec4F},
											{Type: shaderir.LocalVariable, Index: 0},
											{Type: shaderir.NumberExpr, Const: constant.MakeFloat64(1.0)},
										},
									},
								},
							},
						},
					},
					// v0 = a1
					{
						Type: shaderir.Assign,
						Exprs: []shaderir.Expr{
							{Type: shaderir.LocalVariable, Index: 3},
							{Type: shaderir.LocalVariable, Index: 1},
						},
					},
				},
			},
		},
		FragmentFunc: shaderir.FragmentFunc{
			Block: &shaderir.Block{
				Stmts: []shaderir.Stmt{
					{
						Type: shaderir.Return,
						Exprs: []shaderir.Expr{
							{
								Type: shaderir.Call,
								Exprs: []shaderir.Expr{
									{Type: shaderir.BuiltinFuncExpr, BuiltinFunc: shaderir.TextureSample},
									{Type: shaderir.BindingVariable, Index: 1},
									{Type: shaderir.BindingVariable, Index: 2},
									{Type: shaderir.LocalVariable, Index: 1},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestCompileWebGPUAndWebGL2Shaders(t *testing.T) {
	ir := samplePipelineIR()

	// WebGPU shared module compilation.
	wgpu, err := compileWebGPUShaders(
		driver.GPUShaderStage{IR: ir, EntryPoint: "vs_main"},
		driver.GPUShaderStage{IR: ir, EntryPoint: "fs_main"},
	)
	if err != nil {
		t.Fatalf("compileWebGPUShaders: %v", err)
	}
	if !wgpu.SharedModule || wgpu.VertexEntry != "vs_main" || wgpu.FragmentEntry != "fs_main" {
		t.Fatalf("unexpected webgpu compiled metadata: %+v", wgpu)
	}
	for _, want := range []string{"@group(0) @binding(0) var<uniform> B0", "@vertex fn vs_main", "@fragment fn fs_main", "textureSample(B1, B2"} {
		if !strings.Contains(wgpu.VertexWGSL, want) {
			t.Errorf("WGSL missing %q:\n%s", want, wgpu.VertexWGSL)
		}
	}

	// WebGL 2 GLSL ES 3.00 compilation with UBO and combined sampler pair table.
	glSrc, err := compileWebGL2Shaders(
		driver.GPUShaderStage{IR: ir, EntryPoint: "vs_main"},
		driver.GPUShaderStage{IR: ir, EntryPoint: "fs_main"},
	)
	if err != nil {
		t.Fatalf("compileWebGL2Shaders: %v", err)
	}
	if !strings.Contains(glSrc.VertexSource, "#version 300 es") {
		t.Errorf("GLSL vertex missing #version 300 es:\n%s", glSrc.VertexSource)
	}
	if !strings.Contains(glSrc.VertexSource, "gl_Position.z = gl_Position.z * 2.0 - gl_Position.w;") {
		t.Errorf("GLSL vertex missing [0,1]->[-1,1] depth remap:\n%s", glSrc.VertexSource)
	}
	if len(glSrc.UniformBlocks) != 1 || glSrc.UniformBlocks[0].BlockName != "UB0" || glSrc.UniformBlocks[0].Group != 0 || glSrc.UniformBlocks[0].Binding != 0 {
		t.Errorf("unexpected UniformBlocks: %+v", glSrc.UniformBlocks)
	}
	if len(glSrc.SamplerPairs) != 1 || glSrc.SamplerPairs[0].TextureBinding != 1 || glSrc.SamplerPairs[0].SamplerBinding != 2 {
		t.Errorf("unexpected SamplerPairs: %+v", glSrc.SamplerPairs)
	}
	if !strings.Contains(glSrc.FragmentSource, "uniform sampler2D "+glSrc.SamplerPairs[0].Name+";") {
		t.Errorf("GLSL fragment missing combined sampler declaration:\n%s", glSrc.FragmentSource)
	}

	// Depth-only pass (nil fragment stage) produces valid empty GLSL ES 3.00 fragment shader.
	depthOnly, err := compileWebGL2Shaders(driver.GPUShaderStage{IR: ir}, driver.GPUShaderStage{})
	if err != nil {
		t.Fatalf("compileWebGL2Shaders depth-only: %v", err)
	}
	if depthOnly.FragmentSource == "" || !strings.Contains(depthOnly.FragmentSource, "#version 300 es") {
		t.Errorf("expected valid empty GLSL ES 3.00 fragment shader, got %q", depthOnly.FragmentSource)
	}

	// Nil vertex IR returns error on both paths.
	if _, err := compileWebGPUShaders(driver.GPUShaderStage{}, driver.GPUShaderStage{}); err == nil {
		t.Error("expected error for nil vertex IR in WebGPU")
	}
	if _, err := compileWebGL2Shaders(driver.GPUShaderStage{}, driver.GPUShaderStage{}); err == nil {
		t.Error("expected error for nil vertex IR in WebGL 2")
	}
}

func TestGPUViewControllerLifecycle(t *testing.T) {
	c := newGPUViewController(42)
	c.configure(driver.GPUViewConfig{
		ColorFormat: driver.GPUFormatRGBA8Unorm,
		DepthFormat: driver.GPUFormatDepth32Float,
		SampleCount: 1,
	})

	// Resize before ready updates physical pixels without emitting GPUEventResize.
	w, h, changed, ev := c.resize(200, 100, 2)
	if !changed || w != 400 || h != 200 || ev != nil {
		t.Fatalf("pre-ready resize = (%d,%d,%v,%v), want (400,200,true,nil)", w, h, changed, ev)
	}

	if !c.beginInit() {
		t.Fatal("first beginInit should succeed")
	}
	if c.beginInit() {
		t.Fatal("second beginInit while initializing must return false")
	}

	// Mark Ready on WebGPU -> generation 1.
	readyEv := c.markReady(gpuBackendWebGPU, driver.GPUFormatBGRA8Unorm, driver.DefaultGPULimits(), driver.GPUFeatures{StorageBuffers: true, Depth32Float: true})
	if readyEv == nil || readyEv.Kind != driver.GPUEventDeviceReady || readyEv.Generation != 1 || readyEv.Width != 400 || readyEv.Height != 200 || readyEv.SurfaceFormat != driver.GPUFormatBGRA8Unorm {
		t.Fatalf("unexpected ready event: %+v", readyEv)
	}

	// Resize while ready emits GPUEventResize.
	w, h, changed, ev = c.resize(250, 150, 2)
	if !changed || w != 500 || h != 300 || ev == nil || ev.Kind != driver.GPUEventResize || ev.Generation != 1 {
		t.Fatalf("ready resize = (%d,%d,%v,%+v)", w, h, changed, ev)
	}
	// Identical size is a no-op.
	if _, _, changed, ev := c.resize(250, 150, 2); changed || ev != nil {
		t.Fatalf("no-op resize should not change or emit event: %v, %+v", changed, ev)
	}

	// Accept frame with matching generation and track all resource types.
	ok := c.acceptFrame(driver.GPUFrame{
		ViewID:     42,
		Generation: 1,
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreateBuffer, ID: 1},
			{Kind: driver.GPUOpCreateTexture, ID: 2},
			{Kind: driver.GPUOpCreateSampler, ID: 3},
			{Kind: driver.GPUOpCreateBindGroupLayout, ID: 4},
			{Kind: driver.GPUOpCreateBindGroup, ID: 5},
			{Kind: driver.GPUOpCreatePipeline, ID: 6},
		},
	})
	if !ok {
		t.Fatal("expected generation-1 frame to be accepted")
	}
	if cnt := c.resourceCounts(); cnt != (gpuResourceCounts{1, 1, 1, 1, 1, 1}) {
		t.Fatalf("resourceCounts = %+v, want all 1s", cnt)
	}

	// Reject stale generation frame.
	if c.acceptFrame(driver.GPUFrame{ViewID: 42, Generation: 99}) {
		t.Fatal("frame with mismatched generation must be dropped")
	}

	// Destroy individual resources.
	c.acceptFrame(driver.GPUFrame{
		ViewID:     42,
		Generation: 1,
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpDestroyBuffer, ID: 1},
			{Kind: driver.GPUOpDestroyPipeline, ID: 6},
		},
	})
	if cnt := c.resourceCounts(); cnt.Buffers != 0 || cnt.Pipelines != 0 || cnt.Textures != 1 {
		t.Fatalf("unexpected counts after partial destroy: %+v", cnt)
	}

	// Device lost clears tracked resources and emits GPUEventDeviceLost once.
	lostEv := c.markLost(driver.GPULostReset, "GPU reset")
	if lostEv == nil || lostEv.Kind != driver.GPUEventDeviceLost || lostEv.Generation != 1 || lostEv.LostReason != driver.GPULostReset {
		t.Fatalf("unexpected lost event: %+v", lostEv)
	}
	if c.markLost(driver.GPULostReset, "duplicate") != nil {
		t.Fatal("duplicate markLost must be suppressed")
	}
	if cnt := c.resourceCounts(); cnt != (gpuResourceCounts{}) {
		t.Fatalf("device loss must clear tracked resources, got %+v", cnt)
	}
	if c.acceptFrame(driver.GPUFrame{ViewID: 42, Generation: 1}) {
		t.Fatal("frames must be rejected while device is lost")
	}

	// Restore (e.g. fallback to WebGL 2) bumps generation to 2.
	restoredEv := c.markReady(gpuBackendWebGL2, driver.GPUFormatRGBA8Unorm, driver.DefaultGPULimits(), driver.GPUFeatures{Depth32Float: true})
	if restoredEv == nil || restoredEv.Generation != 2 || restoredEv.SurfaceFormat != driver.GPUFormatRGBA8Unorm {
		t.Fatalf("unexpected restored ready event: %+v", restoredEv)
	}
	if c.acceptFrame(driver.GPUFrame{ViewID: 42, Generation: 1}) {
		t.Fatal("generation-1 frame must be rejected after generation-2 restore")
	}
	if !c.acceptFrame(driver.GPUFrame{ViewID: 42, Generation: 2, Ops: []driver.GPUResourceOp{{Kind: driver.GPUOpCreateBuffer, ID: 10}}}) {
		t.Fatal("generation-2 frame should be accepted")
	}

	// Unmount / destroy releases everything and suppresses all late async events.
	c.destroy()
	if cnt := c.resourceCounts(); cnt != (gpuResourceCounts{}) {
		t.Fatalf("destroy must clear tracked resources, got %+v", cnt)
	}
	if c.markReady(gpuBackendWebGPU, driver.GPUFormatRGBA8Unorm, driver.DefaultGPULimits(), driver.GPUFeatures{}) != nil {
		t.Fatal("late markReady after destroy must return nil")
	}
	if c.markLost(driver.GPULostDestroyed, "late") != nil {
		t.Fatal("late markLost after destroy must return nil")
	}
	if c.makeError(driver.GPUErrorFailed, "late") != nil {
		t.Fatal("late makeError after destroy must return nil")
	}
	if c.acceptFrame(driver.GPUFrame{ViewID: 42, Generation: 2}) {
		t.Fatal("frames after destroy must be rejected")
	}
}

func TestResolveDynamicOffsetsAndClampLimits(t *testing.T) {
	layout := []driver.GPUBindGroupLayoutEntry{
		{Binding: 0, Type: driver.GPUBindingUniform, HasDynamicOffset: false},
		{Binding: 1, Type: driver.GPUBindingUniform, HasDynamicOffset: true},
		{Binding: 4, Type: driver.GPUBindingUniform, HasDynamicOffset: true},
	}
	got := resolveDynamicOffsets(layout, []uint32{256, 512})
	if got[1] != 256 || got[4] != 512 || got[0] != 0 {
		t.Fatalf("resolveDynamicOffsets = %v", got)
	}

	lim := clampGPULimits(driver.GPULimits{MaxTextureDimension2D: 8192})
	if lim.MaxTextureDimension2D != 8192 || lim.MaxBindGroups == 0 || lim.MinUniformBufferAlignment == 0 {
		t.Fatalf("clampGPULimits = %+v", lim)
	}
}

func TestPassClassificationMipDimScissorAndRowPadding(t *testing.T) {
	// Swapchain vs offscreen pass classification (including depth-only shadow passes).
	if !isSwapchainPass(driver.GPUPass{}) {
		t.Error("empty pass should target swapchain")
	}
	if !isSwapchainPass(driver.GPUPass{ColorAttachments: []driver.GPUColorAttachment{{Texture: 0}}}) {
		t.Error("color texture 0 should target swapchain")
	}
	if isSwapchainPass(driver.GPUPass{ColorAttachments: []driver.GPUColorAttachment{{Texture: 5}}}) {
		t.Error("non-zero color texture must be offscreen")
	}
	if isSwapchainPass(driver.GPUPass{DepthStencil: &driver.GPUDepthStencilAttachment{Texture: 7}}) {
		t.Error("depth-only pass with non-zero depth texture must be offscreen")
	}
	if !isSwapchainPass(driver.GPUPass{DepthStencil: &driver.GPUDepthStencilAttachment{Texture: 0}}) {
		t.Error("depth-only pass with depth texture 0 should target swapchain")
	}

	// Mip dimensions clamp at 1.
	if got := mipDim(128, 0); got != 128 {
		t.Errorf("mipDim(128, 0) = %d, want 128", got)
	}
	if got := mipDim(128, 2); got != 32 {
		t.Errorf("mipDim(128, 2) = %d, want 32", got)
	}
	if got := mipDim(128, 10); got != 1 {
		t.Errorf("mipDim(128, 10) = %d, want 1", got)
	}

	// Scissor clamping for negative origin/dimensions.
	if x, y, w, h := clampScissorRect(-4, -2, 10, 8); x != 0 || y != 0 || w != 6 || h != 6 {
		t.Errorf("clampScissorRect(-4,-2,10,8) = (%d,%d,%d,%d), want (0,0,6,6)", x, y, w, h)
	}
	if x, y, w, h := clampScissorRect(-20, 5, 10, -3); x != 0 || y != 5 || w != 0 || h != 0 {
		t.Errorf("clampScissorRect(-20,5,10,-3) = (%d,%d,%d,%d), want (0,5,0,0)", x, y, w, h)
	}

	// Generalized stripRowPadding for 1-bpp (R8) and 8-bpp (RGBA16Float).
	r8 := make([]byte, 512)
	r8[0], r8[1] = 11, 22
	r8[256], r8[257] = 33, 44
	if got := stripRowPadding(r8, 2, 2, 1, 256, false); !bytes.Equal(got, []byte{11, 22, 33, 44}) {
		t.Errorf("stripRowPadding R8 = %v", got)
	}
	f16 := make([]byte, 512)
	copy(f16[0:8], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	copy(f16[256:264], []byte{9, 10, 11, 12, 13, 14, 15, 16})
	wantF16 := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	if got := stripRowPadding(f16, 1, 2, 8, 256, false); !bytes.Equal(got, wantF16) {
		t.Errorf("stripRowPadding RGBA16F = %v", got)
	}
}

func TestCompileSeparateStageShaders(t *testing.T) {
	vsIR := samplePipelineIR()
	fsIR := samplePipelineIR()

	// Distinct IR pointers exercise the non-shared-module compilation path on both backends.
	wgpu, err := compileWebGPUShaders(
		driver.GPUShaderStage{IR: vsIR, EntryPoint: "vs_main"},
		driver.GPUShaderStage{IR: fsIR, EntryPoint: "fs_main"},
	)
	if err != nil {
		t.Fatalf("compileWebGPUShaders separate: %v", err)
	}
	if wgpu.SharedModule || wgpu.VertexWGSL == "" || wgpu.FragmentWGSL == "" {
		t.Fatalf("expected non-shared WGSL modules, got %+v", wgpu)
	}

	glSrc, err := compileWebGL2Shaders(
		driver.GPUShaderStage{IR: vsIR, EntryPoint: "vs_main"},
		driver.GPUShaderStage{IR: fsIR, EntryPoint: "fs_main"},
	)
	if err != nil {
		t.Fatalf("compileWebGL2Shaders separate: %v", err)
	}
	if len(glSrc.UniformBlocks) != 1 || len(glSrc.SamplerPairs) != 1 {
		t.Fatalf("expected deduplicated UB and sampler pair across stages: UBs=%+v SPs=%+v", glSrc.UniformBlocks, glSrc.SamplerPairs)
	}
}
