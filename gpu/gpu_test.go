package gpu_test

import (
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

const color3DWGSL = `
struct Uniforms {
	mvp: mat4x4<f32>,
}

@group(0) @binding(0) var<uniform> u: Uniforms;

struct VertexIn {
	@location(0) pos: vec3<f32>,
	@location(1) color: vec4<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = u.mvp * vec4<f32>(in.pos, 1.0);
	out.color = in.color;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	return in.color;
}
`

const texturedWGSL = `
@group(0) @binding(0) var tex: texture_2d<f32>;
@group(0) @binding(1) var samp: sampler;

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
	return textureSample(tex, samp, in.uv);
}
`

func packFloat32s(vals ...float32) []byte {
	out := make([]byte, len(vals)*4)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(v))
	}
	return out
}

func packUint16s(vals ...uint16) []byte {
	out := make([]byte, len(vals)*2)
	for i, v := range vals {
		binary.LittleEndian.PutUint16(out[i*2:], v)
	}
	return out
}

// perspectiveRH returns a WebGPU [0, 1] depth right-handed perspective matrix (column-major).
func perspectiveRH(fovYRad, aspect, near, far float32) [16]float32 {
	f := float32(1.0 / math.Tan(float64(fovYRad)/2.0))
	return [16]float32{
		f / aspect, 0, 0, 0,
		0, f, 0, 0,
		0, 0, far / (near - far), -1,
		0, 0, (far * near) / (near - far), 0,
	}
}

// translateMat4 returns a column-major translation matrix.
func translateMat4(tx, ty, tz float32) [16]float32 {
	return [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		tx, ty, tz, 1,
	}
}

func mulMat4(a, b [16]float32) [16]float32 {
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

func TestHeadless3DCubeDepthAndReadPixels(t *testing.T) {
	mod := gpu.MustCompileWGSL(color3DWGSL)

	var (
		readyCount int
		frameCount int
		readback   []byte
		readErr    error
	)

	// Two overlapping 3D quads inside a cube volume:
	// - Near quad at z = +0.5 (bright red, drawn FIRST)
	// - Far quad at z = -0.5 (bright blue, drawn SECOND — must be rejected by depth test!)
	// Both are wound CCW when viewed from +Z looking down -Z.
	verts := packFloat32s(
		// Near red quad (vertices 0..3) at z = +0.5
		-0.5, -0.5, 0.5, 1, 0, 0, 1,
		0.5, -0.5, 0.5, 1, 0, 0, 1,
		0.5, 0.5, 0.5, 1, 0, 0, 1,
		-0.5, 0.5, 0.5, 1, 0, 0, 1,
		// Far blue quad (vertices 4..7) at z = -0.5, larger so its outer rim is visible behind the red face!
		-1.0, -1.0, -0.5, 0, 0, 1, 1,
		1.0, -1.0, -0.5, 0, 0, 1, 1,
		1.0, 1.0, -0.5, 0, 0, 1, 1,
		-1.0, 1.0, -0.5, 0, 0, 1, 1,
	)
	indices := packUint16s(
		0, 1, 2, 0, 2, 3, // near red
		4, 5, 6, 4, 6, 7, // far blue
	)

	var (
		vbuf *gpu.Buffer
		ibuf *gpu.Buffer
		ubuf *gpu.Buffer
		bg   *gpu.BindGroup
		pipe *gpu.Pipeline
	)

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		if !cx.Env().SupportsGPU() {
			t.Fatalf("expected SupportsGPU() == true in headless nsgotest")
		}
		return gpu.View(cx, gpu.ViewProps{
			TestID:      "viewport",
			Style:       core.Style{Width: core.Pt(120), Height: core.Pt(120)},
			DepthFormat: gpu.FormatDepth24Plus,
			Children: []core.Node{
				core.Text{Content: "HUD"},
			},
			OnReady: func(dev *gpu.Device) {
				readyCount++
				var err error
				vbuf, err = dev.CreateBuffer(cx, gpu.BufferDesc{Label: "verts", Usage: gpu.UsageVertex, Data: verts})
				if err != nil {
					t.Fatalf("CreateBuffer verts: %v", err)
				}
				ibuf, err = dev.CreateBuffer(cx, gpu.BufferDesc{Label: "indices", Usage: gpu.UsageIndex, Data: indices})
				if err != nil {
					t.Fatalf("CreateBuffer indices: %v", err)
				}
				mvp := mulMat4(perspectiveRH(math.Pi/3, 1.0, 0.1, 20.0), translateMat4(0, 0, -3.0))
				ubuf, err = dev.CreateBuffer(cx, gpu.BufferDesc{Label: "uniforms", Usage: gpu.UsageUniform | gpu.UsageCopyDst, Data: packFloat32s(mvp[:]...)})
				if err != nil {
					t.Fatalf("CreateBuffer uniforms: %v", err)
				}
				bgl, err := dev.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
					{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
				})
				if err != nil {
					t.Fatalf("CreateBindGroupLayout: %v", err)
				}
				bg, err = dev.CreateBindGroup(cx, bgl, []gpu.BindGroupEntry{
					{Binding: 0, Buffer: ubuf},
				})
				if err != nil {
					t.Fatalf("CreateBindGroup: %v", err)
				}
				pipe, err = dev.CreatePipeline(cx, gpu.PipelineDesc{
					Label:    "cube",
					Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
					Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
					Layouts:  []*gpu.BindGroupLayout{bgl},
					VertexBuffers: []gpu.VertexBufferLayout{{
						Stride: 7 * 4,
						Attributes: []gpu.VertexAttribute{
							{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
							{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x4},
						},
					}},
					Primitive: gpu.PrimitiveState{
						Topology:  gpu.TopologyTriangleList,
						FrontFace: gpu.FrontCCW,
						CullMode:  gpu.CullBack,
					},
					DepthStencil: &gpu.DepthStencilState{
						Format:       gpu.FormatDepth24Plus,
						DepthWrite:   true,
						DepthCompare: gpu.CompareLess,
					},
					Targets: []gpu.ColorTargetState{{Format: dev.SurfaceFormat()}},
				})
				if err != nil {
					t.Fatalf("CreatePipeline: %v", err)
				}
			},
			OnFrame: func(frame *gpu.Frame) {
				frameCount++
				pass := frame.BeginRenderPass(gpu.RenderPassDesc{})
				pass.ClearColor(core.RGB(0x10141c))
				pass.ClearDepth(1.0)
				pass.SetPipeline(pipe)
				pass.SetBindGroup(0, bg)
				pass.SetVertexBuffer(0, vbuf, 0)
				pass.SetIndexBuffer(ibuf, gpu.IndexUint16, 0)
				pass.DrawIndexed(12, 0, 0)
				pass.End()
				frame.Submit()
				if frameCount == 1 {
					frame.Device().ReadSurface(func(rgba []byte, err error) {
						readback, readErr = rgba, err
					})
				}
			},
		})
	}, nsgotest.Size(120, 120), nsgotest.ManualFrames())

	if readyCount != 1 {
		t.Fatalf("readyCount = %d, want 1", readyCount)
	}
	// HUD child still participates in the native view tree.
	app.Find("text:HUD")

	// Step one display frame so OnFrame executes off-commit.
	app.Advance(16 * time.Millisecond)

	if frameCount < 1 {
		t.Fatalf("expected OnFrame to run, got frameCount=%d", frameCount)
	}
	frames := app.GPUFrames()
	if len(frames) == 0 {
		t.Fatalf("expected recorded GPU frames")
	}
	var totalDraws int
	for _, fr := range frames {
		totalDraws += fr.DrawsCount
	}
	if totalDraws < 1 {
		t.Fatalf("expected at least 1 recorded draw call, got %+v", frames)
	}

	img, err := app.GPUReadPixels("id:viewport")
	if err != nil {
		t.Fatalf("GPUReadPixels: %v", err)
	}
	if img.Bounds().Dx() != 120 || img.Bounds().Dy() != 120 {
		t.Fatalf("image size = %v, want 120x120", img.Bounds())
	}

	// Corner pixel (2, 2) must equal the clear color (16, 20, 28, 255).
	corner := img.RGBAAt(2, 2)
	if corner != (color.RGBA{R: 16, G: 20, B: 28, A: 255}) {
		t.Errorf("corner pixel = %+v, want clear color {16 20 28 255}", corner)
	}

	// Center pixel (60, 60) must be bright red (255, 0, 0, 255): the near red quad
	// won the depth test against the far blue quad drawn after it!
	center := img.RGBAAt(60, 60)
	if center.R < 250 || center.G > 5 || center.B > 5 || center.A != 255 {
		t.Errorf("center pixel = %+v, want red (depth test should reject far blue face)", center)
	}

	// Rim pixel (36, 60) lies outside the near red quad but inside the larger far blue quad.
	rim := img.RGBAAt(36, 60)
	if rim.B < 250 || rim.R > 5 || rim.G > 5 || rim.A != 255 {
		t.Errorf("rim pixel (36,60) = %+v, want blue from larger far quad", rim)
	}

	// Async ReadSurface callback delivered matching pixels on the engine goroutine.
	if readErr != nil {
		t.Fatalf("ReadSurface error: %v", readErr)
	}
	if len(readback) != 120*120*4 {
		t.Fatalf("ReadSurface bytes = %d, want %d", len(readback), 120*120*4)
	}
	ci := (60*120 + 60) * 4
	if readback[ci] < 250 || readback[ci+2] > 5 {
		t.Errorf("ReadSurface center pixel = (%d,%d,%d,%d), want red", readback[ci], readback[ci+1], readback[ci+2], readback[ci+3])
	}
}

func TestSoftwareTexturedBlendAndOffscreenTarget(t *testing.T) {
	texMod := gpu.MustCompileWGSL(texturedWGSL)
	colorMod := gpu.MustCompileWGSL(color3DWGSL)

	sr := gpu.NewSoftwareRenderer(32, 32, gpu.FormatDepth24Plus)
	dev := sr.Device()

	// Validate synchronous descriptor errors.
	if _, err := dev.CreateBuffer(nil, gpu.BufferDesc{Size: 16}); !errors.Is(err, gpu.ErrInvalidDescriptor) {
		t.Fatalf("CreateBuffer without Usage got %v, want ErrInvalidDescriptor", err)
	}
	if _, err := dev.CreateTexture(nil, gpu.TextureDesc{Width: 0, Height: 4, Format: gpu.FormatRGBA8Unorm, Usage: gpu.TextureSampled}); !errors.Is(err, gpu.ErrInvalidDescriptor) {
		t.Fatalf("CreateTexture width=0 got %v, want ErrInvalidDescriptor", err)
	}

	// Pass 1: Render a solid green triangle into an offscreen RenderTarget.
	rt, err := dev.CreateRenderTarget(nil, gpu.RenderTargetDesc{
		Label:       "offscreen",
		Width:       16,
		Height:      16,
		ColorFormat: gpu.FormatRGBA8Unorm,
	})
	if err != nil {
		t.Fatalf("CreateRenderTarget: %v", err)
	}

	// Pass 2: Sample a 2x2 RGBA texture (left=green, right=magenta) on a fullscreen quad
	// AND blend a semi-transparent white overlay on top.
	texImg := image.NewRGBA(image.Rect(0, 0, 2, 2))
	texImg.SetRGBA(0, 0, color.RGBA{0, 255, 0, 255})
	texImg.SetRGBA(1, 0, color.RGBA{255, 0, 255, 255})
	texImg.SetRGBA(0, 1, color.RGBA{0, 255, 0, 255})
	texImg.SetRGBA(1, 1, color.RGBA{255, 0, 255, 255})

	tex, err := dev.CreateTextureFromImage(nil, core.ImageSource{Pixels: texImg}, gpu.TextureDesc{
		Label:  "checker",
		Format: gpu.FormatRGBA8Unorm,
		Usage:  gpu.TextureSampled | gpu.TextureCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTextureFromImage: %v", err)
	}
	samp, err := dev.CreateSampler(nil, gpu.SamplerDesc{
		MinFilter: gpu.FilterNearest,
		MagFilter: gpu.FilterNearest,
		AddressU:  gpu.AddressClampToEdge,
		AddressV:  gpu.AddressClampToEdge,
	})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}
	bgl, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
		{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingSampler},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	bg, err := dev.CreateBindGroup(nil, bgl, []gpu.BindGroupEntry{
		{Binding: 0, Texture: tex},
		{Binding: 1, Sampler: samp},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	quadVerts := packFloat32s(
		-1, -1, 0.5, 0, 1,
		1, -1, 0.5, 1, 1,
		1, 1, 0.5, 1, 0,
		-1, -1, 0.5, 0, 1,
		1, 1, 0.5, 1, 0,
		-1, 1, 0.5, 0, 0,
	)
	vb, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: quadVerts})
	if err != nil {
		t.Fatalf("CreateBuffer quad: %v", err)
	}
	texPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:   gpu.ShaderStage{Module: texMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: texMod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{bgl},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride: 5 * 4,
			Attributes: []gpu.VertexAttribute{
				{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
				{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x2},
			},
		}},
		Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
	})
	if err != nil {
		t.Fatalf("CreatePipeline tex: %v", err)
	}

	// Blended overlay pipeline (50% red over the textured background).
	ident := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	uBuf, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: packFloat32s(ident[:]...)})
	cBGL, _ := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform}})
	cBG, _ := dev.CreateBindGroup(nil, cBGL, []gpu.BindGroupEntry{{Binding: 0, Buffer: uBuf}})
	blendVerts := packFloat32s(
		-1, -1, 0.2, 1, 0, 0, 0.5,
		1, -1, 0.2, 1, 0, 0, 0.5,
		1, 1, 0.2, 1, 0, 0, 0.5,
		-1, -1, 0.2, 1, 0, 0, 0.5,
		1, 1, 0.2, 1, 0, 0, 0.5,
		-1, 1, 0.2, 1, 0, 0, 0.5,
	)
	blendVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: blendVerts})
	blendPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:   gpu.ShaderStage{Module: colorMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: colorMod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{cBGL},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride: 7 * 4,
			Attributes: []gpu.VertexAttribute{
				{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
				{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x4},
			},
		}},
		Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm, Blend: gpu.BlendAlpha()}},
	})
	if err != nil {
		t.Fatalf("CreatePipeline blend: %v", err)
	}

	enc := dev.BeginEncoder()
	// Offscreen clear pass on rt.Color.
	p0 := enc.BeginRenderPass(gpu.RenderPassDesc{
		ColorAttachments: []gpu.ColorAttachment{{
			Texture:    rt.Color,
			LoadOp:     gpu.LoadClear,
			StoreOp:    gpu.StoreStore,
			ClearColor: core.RGB(0x0a141e),
		}},
	})
	p0.End()

	// Main pass: textured quad + 50% red alpha-blended overlay.
	p1 := enc.BeginRenderPass(gpu.RenderPassDesc{})
	p1.ClearColor(core.RGB(0x000000))
	p1.SetPipeline(texPipe)
	p1.SetBindGroup(0, bg)
	p1.SetVertexBuffer(0, vb, 0)
	p1.Draw(6, 0)
	p1.SetPipeline(blendPipe)
	p1.SetBindGroup(0, cBG)
	p1.SetVertexBuffer(0, blendVB, 0)
	p1.Draw(6, 0)
	p1.End()

	dev.Submit(enc.Finish())
	if sr.LastError() != nil {
		t.Fatalf("SoftwareRenderer error: %v", sr.LastError())
	}

	img := sr.Image()
	// Left half: green (0,255,0) blended with 50% red (255,0,0) -> (~128, ~128, 0, 255)
	left := img.RGBAAt(4, 16)
	if left.R < 120 || left.R > 136 || left.G < 120 || left.G > 136 || left.B > 5 {
		t.Errorf("left blended pixel = %+v, want ~{128 128 0 255}", left)
	}
	// Right half: magenta (255,0,255) blended with 50% red (255,0,0) -> (255, 0, ~128, 255)
	right := img.RGBAAt(28, 16)
	if right.R < 250 || right.G > 5 || right.B < 120 || right.B > 136 {
		t.Errorf("right blended pixel = %+v, want ~{255 0 128 255}", right)
	}

	// Verify offscreen RenderTarget readback works too.
	var rtBytes []byte
	dev.ReadTexture(rt.Color, gpu.TextureRegion{Width: 16, Height: 16}, func(rgba []byte, err error) {
		if err != nil {
			t.Errorf("ReadTexture rt: %v", err)
		}
		rtBytes = rgba
	})
	if len(rtBytes) != 16*16*4 || rtBytes[0] != 10 || rtBytes[1] != 20 || rtBytes[2] != 30 || rtBytes[3] != 255 {
		t.Errorf("offscreen ReadTexture first pixel = %v, want [10 20 30 255]", rtBytes[:min(4, len(rtBytes))])
	}
}

func TestGPUDeviceLossAndNoGPUFallback(t *testing.T) {
	var (
		gens []uint32
		lost []gpu.DeviceLostEvent
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return gpu.View(cx, gpu.ViewProps{
			TestID: "gpu",
			Style:  core.Style{Width: core.Pt(64), Height: core.Pt(64)},
			OnReady: func(dev *gpu.Device) {
				gens = append(gens, dev.Generation())
			},
			OnDeviceLost: func(ev gpu.DeviceLostEvent) {
				lost = append(lost, ev)
			},
		})
	}, nsgotest.Size(64, 64))

	if len(gens) != 1 || gens[0] != 1 {
		t.Fatalf("initial generations = %v, want [1]", gens)
	}
	app.TriggerGPUDeviceLost("id:gpu", nsgotest.GPULostReset, "driver reset")
	if len(lost) != 1 || lost[0].Reason != gpu.LostReset || lost[0].Message != "driver reset" {
		t.Fatalf("lost events = %+v", lost)
	}
	app.RestoreGPUDevice("id:gpu")
	if len(gens) != 2 || gens[1] != 2 {
		t.Fatalf("restored generations = %v, want [1 2]", gens)
	}

	// NoGPU fallback path renders Children and reports ErrUnsupported via OnError.
	var gotErr gpu.Error
	fallbackApp := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		if cx.Env().SupportsGPU() {
			t.Fatalf("expected SupportsGPU() == false under NoGPU()")
		}
		return gpu.View(cx, gpu.ViewProps{
			TestID: "fallback",
			Children: []core.Node{
				core.Text{Content: "No GPU"},
			},
			OnReady: func(*gpu.Device) {
				t.Errorf("OnReady must not run when SupportsGPU() is false")
			},
			OnError: func(err gpu.Error) {
				gotErr = err
			},
		})
	}, nsgotest.NoGPU())

	fallbackApp.Find("text:No GPU")
	if !errors.Is(gotErr, gpu.ErrUnsupported) {
		t.Errorf("NoGPU OnError = %v, want ErrUnsupported", gotErr)
	}
}

const postProcessSwizzleWGSL = `
@group(0) @binding(0) var srcTex: texture_2d<f32>;
@group(0) @binding(1) var srcSamp: sampler;

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
	let c = textureSample(srcTex, srcSamp, in.uv);
	// Rotate channels (R,G,B) -> (B,R,G) to prove pass 2 processed pass 1's rendered texture.
	return vec4<f32>(c.b, c.r, c.g, 1.0);
}
`

const shadowDepthPassWGSL = `
struct LightUniforms {
	lightVP: mat4x4<f32>,
}

@group(0) @binding(0) var<uniform> u: LightUniforms;

struct VertexIn {
	@location(0) pos: vec3<f32>,
}

@vertex
fn vs_shadow(in: VertexIn) -> @builtin(position) vec4<f32> {
	return u.lightVP * vec4<f32>(in.pos, 1.0);
}
`

const shadowLitPassWGSL = `
struct ReceiverUniforms {
	mvp: mat4x4<f32>,
	lightVP: mat4x4<f32>,
}

@group(0) @binding(0) var<uniform> u: ReceiverUniforms;
@group(0) @binding(1) var shadowMap: texture_depth_2d;
@group(0) @binding(2) var shadowSamp: sampler_comparison;

struct VertexIn {
	@location(0) pos: vec3<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) lightClip: vec4<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = u.mvp * vec4<f32>(in.pos, 1.0);
	out.lightClip = u.lightVP * vec4<f32>(in.pos, 1.0);
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	let ndc = in.lightClip.xyz / in.lightClip.w;
	let uv = vec2<f32>(ndc.x * 0.5 + 0.5, 0.5 - ndc.y * 0.5);
	let vis = textureSampleCompare(shadowMap, shadowSamp, uv, ndc.z);
	let lit = 0.2 + 0.8 * vis;
	return vec4<f32>(lit, lit, lit, 1.0);
}
`

const shadowPCFProbeWGSL = `
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

func TestRenderTargetPostProcessMultiPass(t *testing.T) {
	sceneMod := gpu.MustCompileWGSL(color3DWGSL)
	postMod := gpu.MustCompileWGSL(postProcessSwizzleWGSL)

	sr := gpu.NewSoftwareRenderer(64, 64, gpu.FormatDepth24Plus)
	dev := sr.Device()

	// Color+depth offscreen render target (both attachments sampled + render-attachable).
	rt, err := dev.CreateRenderTarget(nil, gpu.RenderTargetDesc{
		Label:       "scene-rt",
		Width:       64,
		Height:      64,
		ColorFormat: gpu.FormatRGBA8Unorm,
		DepthFormat: gpu.FormatDepth24Plus,
	})
	if err != nil {
		t.Fatalf("CreateRenderTarget: %v", err)
	}
	if rt.Color == nil || rt.Depth == nil {
		t.Fatalf("expected non-nil Color and Depth textures, got color=%v depth=%v", rt.Color, rt.Depth)
	}
	if rt.Depth.Desc().Usage&(gpu.TextureRenderAttachment|gpu.TextureSampled) != (gpu.TextureRenderAttachment | gpu.TextureSampled) {
		t.Fatalf("rt.Depth usage = %v, want TextureRenderAttachment|TextureSampled", rt.Depth.Desc().Usage)
	}

	ident := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	uBuf, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: packFloat32s(ident[:]...)})
	if err != nil {
		t.Fatalf("CreateBuffer uniform: %v", err)
	}
	sceneBGL, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout scene: %v", err)
	}
	sceneBG, err := dev.CreateBindGroup(nil, sceneBGL, []gpu.BindGroupEntry{
		{Binding: 0, Buffer: uBuf},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup scene: %v", err)
	}

	// Near red quad (z=0.2) drawn before larger far green quad (z=0.6).
	sceneVerts := packFloat32s(
		// Near red square [-0.4, 0.4]
		-0.4, -0.4, 0.2, 1, 0, 0, 1,
		0.4, -0.4, 0.2, 1, 0, 0, 1,
		0.4, 0.4, 0.2, 1, 0, 0, 1,
		-0.4, -0.4, 0.2, 1, 0, 0, 1,
		0.4, 0.4, 0.2, 1, 0, 0, 1,
		-0.4, 0.4, 0.2, 1, 0, 0, 1,
		// Far green square [-0.8, 0.8]
		-0.8, -0.8, 0.6, 0, 1, 0, 1,
		0.8, -0.8, 0.6, 0, 1, 0, 1,
		0.8, 0.8, 0.6, 0, 1, 0, 1,
		-0.8, -0.8, 0.6, 0, 1, 0, 1,
		0.8, 0.8, 0.6, 0, 1, 0, 1,
		-0.8, 0.8, 0.6, 0, 1, 0, 1,
	)
	sceneVB, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: sceneVerts})
	if err != nil {
		t.Fatalf("CreateBuffer sceneVB: %v", err)
	}

	// Small yellow accent in top-left corner for LoadLoad follow-up pass.
	accentVerts := packFloat32s(
		-1.0, 0.75, 0.1, 1, 1, 0, 1,
		-0.75, 0.75, 0.1, 1, 1, 0, 1,
		-0.75, 1.0, 0.1, 1, 1, 0, 1,
		-1.0, 0.75, 0.1, 1, 1, 0, 1,
		-0.75, 1.0, 0.1, 1, 1, 0, 1,
		-1.0, 1.0, 0.1, 1, 1, 0, 1,
	)
	accentVB, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: accentVerts})
	if err != nil {
		t.Fatalf("CreateBuffer accentVB: %v", err)
	}

	scenePipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Label:    "scene",
		Vertex:   gpu.ShaderStage{Module: sceneMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: sceneMod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{sceneBGL},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride: 7 * 4,
			Attributes: []gpu.VertexAttribute{
				{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
				{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x4},
			},
		}},
		DepthStencil: &gpu.DepthStencilState{
			Format:       gpu.FormatDepth24Plus,
			DepthWrite:   true,
			DepthCompare: gpu.CompareLess,
		},
		Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
	})
	if err != nil {
		t.Fatalf("CreatePipeline scene: %v", err)
	}

	// Pass 2: sample rt.Color in post-process shader.
	samp, err := dev.CreateSampler(nil, gpu.SamplerDesc{
		MinFilter: gpu.FilterNearest,
		MagFilter: gpu.FilterNearest,
		AddressU:  gpu.AddressClampToEdge,
		AddressV:  gpu.AddressClampToEdge,
	})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}
	postBGL, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
		{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingSampler},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout post: %v", err)
	}
	postBG, err := dev.CreateBindGroup(nil, postBGL, []gpu.BindGroupEntry{
		{Binding: 0, Texture: rt.Color},
		{Binding: 1, Sampler: samp},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup post: %v", err)
	}
	quadVerts := packFloat32s(
		-1, -1, 0.5, 0, 1,
		1, -1, 0.5, 1, 1,
		1, 1, 0.5, 1, 0,
		-1, -1, 0.5, 0, 1,
		1, 1, 0.5, 1, 0,
		-1, 1, 0.5, 0, 0,
	)
	quadVB, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: quadVerts})
	if err != nil {
		t.Fatalf("CreateBuffer quadVB: %v", err)
	}
	postPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Label:    "post",
		Vertex:   gpu.ShaderStage{Module: postMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: postMod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{postBGL},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride: 5 * 4,
			Attributes: []gpu.VertexAttribute{
				{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
				{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x2},
			},
		}},
		Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
	})
	if err != nil {
		t.Fatalf("CreatePipeline post: %v", err)
	}

	enc := dev.BeginEncoder()
	// Pass 1a: Render 3D scene into offscreen rt.Color + rt.Depth.
	p1a := enc.BeginRenderPass(gpu.RenderPassDesc{
		ColorAttachments: []gpu.ColorAttachment{{
			Texture:    rt.Color,
			LoadOp:     gpu.LoadClear,
			StoreOp:    gpu.StoreStore,
			ClearColor: core.RGB(0x0a14c8),
		}},
		DepthStencil: &gpu.DepthStencilAttachment{
			Texture:      rt.Depth,
			DepthLoadOp:  gpu.LoadClear,
			DepthStoreOp: gpu.StoreStore,
			ClearDepth:   1.0,
		},
	})
	p1a.SetPipeline(scenePipe)
	p1a.SetBindGroup(0, sceneBG)
	p1a.SetVertexBuffer(0, sceneVB, 0)
	p1a.Draw(12, 0)
	p1a.End()

	// Pass 1b: Preserve offscreen contents with LoadLoad and draw top-left yellow accent.
	p1b := enc.BeginRenderPass(gpu.RenderPassDesc{
		ColorAttachments: []gpu.ColorAttachment{{
			Texture: rt.Color,
			LoadOp:  gpu.LoadLoad,
			StoreOp: gpu.StoreStore,
		}},
		DepthStencil: &gpu.DepthStencilAttachment{
			Texture:      rt.Depth,
			DepthLoadOp:  gpu.LoadLoad,
			DepthStoreOp: gpu.StoreStore,
		},
	})
	p1b.SetPipeline(scenePipe)
	p1b.SetBindGroup(0, sceneBG)
	p1b.SetVertexBuffer(0, accentVB, 0)
	p1b.Draw(6, 0)
	p1b.End()

	// Pass 2: Sample rt.Color in post-process pass onto swapchain.
	p2 := enc.BeginRenderPass(gpu.RenderPassDesc{
		ColorAttachments: []gpu.ColorAttachment{{
			LoadOp:     gpu.LoadClear,
			StoreOp:    gpu.StoreStore,
			ClearColor: core.RGB(0x000000),
		}},
	})
	p2.SetPipeline(postPipe)
	p2.SetBindGroup(0, postBG)
	p2.SetVertexBuffer(0, quadVB, 0)
	p2.Draw(6, 0)
	p2.End()

	dev.Submit(enc.Finish())
	if sr.LastError() != nil {
		t.Fatalf("SoftwareRenderer error: %v", sr.LastError())
	}

	img := sr.Image()
	// Center (32,32): Pass 1 red (255,0,0) -> Pass 2 rotated (0,255,0).
	if got := img.RGBAAt(32, 32); got != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("center (32,32) = %+v, want {0 255 0 255}", got)
	}
	// Rim (14,32): Pass 1 green (0,255,0) -> Pass 2 rotated (0,0,255).
	if got := img.RGBAAt(14, 32); got != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("rim (14,32) = %+v, want {0 0 255 255}", got)
	}
	// Bottom-right corner (62,62): Pass 1 clear (10,20,200) -> Pass 2 rotated (200,10,20).
	if got := img.RGBAAt(62, 62); got != (color.RGBA{200, 10, 20, 255}) {
		t.Errorf("corner (62,62) = %+v, want {200 10 20 255}", got)
	}
	// Top-left corner (2,2): Pass 1b LoadLoad yellow (255,255,0) -> Pass 2 rotated (0,255,255).
	if got := img.RGBAAt(2, 2); got != (color.RGBA{0, 255, 255, 255}) {
		t.Errorf("top-left (2,2) = %+v, want {0 255 255 255}", got)
	}
}

func TestShadowMapDepthTextureComparisonSampler(t *testing.T) {
	depthMod := gpu.MustCompileWGSL(shadowDepthPassWGSL)
	litMod := gpu.MustCompileWGSL(shadowLitPassWGSL)
	pcfMod := gpu.MustCompileWGSL(shadowPCFProbeWGSL)

	sr := gpu.NewSoftwareRenderer(64, 64, gpu.FormatDepth24Plus)
	dev := sr.Device()

	// Create depth-only shadow map render target (Color == nil, Depth != nil).
	shadowRT, err := dev.CreateRenderTarget(nil, gpu.RenderTargetDesc{
		Label:       "shadow-map",
		Width:       64,
		Height:      64,
		DepthFormat: gpu.FormatDepth32Float,
	})
	if err != nil {
		t.Fatalf("CreateRenderTarget depth-only: %v", err)
	}
	if shadowRT.Color != nil {
		t.Fatalf("expected depth-only RenderTarget.Color == nil, got %v", shadowRT.Color)
	}
	if shadowRT.Depth == nil {
		t.Fatalf("expected depth-only RenderTarget.Depth != nil")
	}

	// Validate BindGroup checks for comparison samplers and TextureSampled usage.
	plainSamp, err := dev.CreateSampler(nil, gpu.SamplerDesc{MinFilter: gpu.FilterNearest, MagFilter: gpu.FilterNearest})
	if err != nil {
		t.Fatalf("CreateSampler plain: %v", err)
	}
	cmpSampNearest, err := dev.CreateSampler(nil, gpu.SamplerDesc{
		MinFilter: gpu.FilterNearest,
		MagFilter: gpu.FilterNearest,
		AddressU:  gpu.AddressClampToEdge,
		AddressV:  gpu.AddressClampToEdge,
		Compare:   gpu.CompareLessEqual,
	})
	if err != nil {
		t.Fatalf("CreateSampler cmp nearest: %v", err)
	}
	cmpSampPCF, err := dev.CreateSampler(nil, gpu.SamplerDesc{
		MinFilter: gpu.FilterLinear,
		MagFilter: gpu.FilterLinear,
		AddressU:  gpu.AddressClampToEdge,
		AddressV:  gpu.AddressClampToEdge,
		Compare:   gpu.CompareLessEqual,
	})
	if err != nil {
		t.Fatalf("CreateSampler cmp PCF: %v", err)
	}
	cmpSampGreater, err := dev.CreateSampler(nil, gpu.SamplerDesc{
		MinFilter: gpu.FilterNearest,
		MagFilter: gpu.FilterNearest,
		AddressU:  gpu.AddressClampToEdge,
		AddressV:  gpu.AddressClampToEdge,
		Compare:   gpu.CompareGreater,
	})
	if err != nil {
		t.Fatalf("CreateSampler cmp greater: %v", err)
	}

	// Reject binding non-comparison sampler into BindingComparisonSampler and vice versa.
	cmpOnlyBGL, _ := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
		{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingComparisonSampler},
	})
	if _, err := dev.CreateBindGroup(nil, cmpOnlyBGL, []gpu.BindGroupEntry{
		{Binding: 0, Texture: shadowRT.Depth},
		{Binding: 1, Sampler: plainSamp},
	}); !errors.Is(err, gpu.ErrInvalidDescriptor) {
		t.Fatalf("expected ErrInvalidDescriptor when binding non-comparison sampler to BindingComparisonSampler, got %v", err)
	}
	regOnlyBGL, _ := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingSampler},
	})
	if _, err := dev.CreateBindGroup(nil, regOnlyBGL, []gpu.BindGroupEntry{
		{Binding: 0, Sampler: cmpSampNearest},
	}); !errors.Is(err, gpu.ErrInvalidDescriptor) {
		t.Fatalf("expected ErrInvalidDescriptor when binding comparison sampler to BindingSampler, got %v", err)
	}
	unsampledTex, _ := dev.CreateTexture(nil, gpu.TextureDesc{
		Width:  4,
		Height: 4,
		Format: gpu.FormatDepth32Float,
		Usage:  gpu.TextureRenderAttachment,
	})
	if _, err := dev.CreateBindGroup(nil, cmpOnlyBGL, []gpu.BindGroupEntry{
		{Binding: 0, Texture: unsampledTex},
		{Binding: 1, Sampler: cmpSampNearest},
	}); !errors.Is(err, gpu.ErrInvalidDescriptor) {
		t.Fatalf("expected ErrInvalidDescriptor when binding texture without TextureSampled, got %v", err)
	}

	ident := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	lightUBuf, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: packFloat32s(ident[:]...)})
	recvUniforms := append(append([]float32{}, ident[:]...), ident[:]...)
	recvUBuf, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: packFloat32s(recvUniforms...)})

	// Occluder quad covering NDC [-0.5, 0.5] x [-0.5, 0.5] at light depth z = 0.25.
	occluderVerts := packFloat32s(
		-0.5, -0.5, 0.25,
		0.5, -0.5, 0.25,
		0.5, 0.5, 0.25,
		-0.5, -0.5, 0.25,
		0.5, 0.5, 0.25,
		-0.5, 0.5, 0.25,
	)
	occluderVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: occluderVerts})

	// Receiver plane covering full NDC [-1, 1] x [-1, 1] at light depth z = 0.6.
	receiverVerts := packFloat32s(
		-1, -1, 0.6,
		1, -1, 0.6,
		1, 1, 0.6,
		-1, -1, 0.6,
		1, 1, 0.6,
		-1, 1, 0.6,
	)
	receiverVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: receiverVerts})

	// Depth-only shadow pipeline (no Fragment shader, no Color Targets).
	depthBGL, _ := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
	})
	depthBG, _ := dev.CreateBindGroup(nil, depthBGL, []gpu.BindGroupEntry{
		{Binding: 0, Buffer: lightUBuf},
	})
	depthPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Label:   "shadow-depth",
		Vertex:  gpu.ShaderStage{Module: depthMod, EntryPoint: "vs_shadow"},
		Layouts: []*gpu.BindGroupLayout{depthBGL},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride:     3 * 4,
			Attributes: []gpu.VertexAttribute{{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3}},
		}},
		DepthStencil: &gpu.DepthStencilState{
			Format:       gpu.FormatDepth32Float,
			DepthWrite:   true,
			DepthCompare: gpu.CompareLess,
		},
	})
	if err != nil {
		t.Fatalf("CreatePipeline shadow-depth: %v", err)
	}

	// Receiver pipeline sampling shadowRT.Depth via sampler_comparison.
	litBGL, _ := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
		{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
		{Binding: 2, Visibility: gpu.StageFragment, Type: gpu.BindingComparisonSampler},
	})
	litBGNear, err := dev.CreateBindGroup(nil, litBGL, []gpu.BindGroupEntry{
		{Binding: 0, Buffer: recvUBuf},
		{Binding: 1, Texture: shadowRT.Depth},
		{Binding: 2, Sampler: cmpSampNearest},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup lit near: %v", err)
	}
	litBGGreater, err := dev.CreateBindGroup(nil, litBGL, []gpu.BindGroupEntry{
		{Binding: 0, Buffer: recvUBuf},
		{Binding: 1, Texture: shadowRT.Depth},
		{Binding: 2, Sampler: cmpSampGreater},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup lit greater: %v", err)
	}
	litPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Label:    "shadow-lit",
		Vertex:   gpu.ShaderStage{Module: litMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: litMod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{litBGL},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride:     3 * 4,
			Attributes: []gpu.VertexAttribute{{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3}},
		}},
		Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
	})
	if err != nil {
		t.Fatalf("CreatePipeline shadow-lit: %v", err)
	}

	// Frame 1: Pass 1 renders occluder to depth-only shadow map; Pass 2 renders lit receiver.
	enc := dev.BeginEncoder()
	pShadow := enc.BeginRenderPass(gpu.RenderPassDesc{
		DepthStencil: &gpu.DepthStencilAttachment{
			Texture:      shadowRT.Depth,
			DepthLoadOp:  gpu.LoadClear,
			DepthStoreOp: gpu.StoreStore,
			ClearDepth:   1.0,
		},
	})
	pShadow.SetPipeline(depthPipe)
	pShadow.SetBindGroup(0, depthBG)
	pShadow.SetVertexBuffer(0, occluderVB, 0)
	pShadow.Draw(6, 0)
	pShadow.End()

	pMain := enc.BeginRenderPass(gpu.RenderPassDesc{
		ColorAttachments: []gpu.ColorAttachment{{
			LoadOp:     gpu.LoadClear,
			StoreOp:    gpu.StoreStore,
			ClearColor: core.RGB(0x000000),
		}},
	})
	pMain.SetPipeline(litPipe)
	pMain.SetBindGroup(0, litBGNear)
	pMain.SetVertexBuffer(0, receiverVB, 0)
	pMain.Draw(6, 0)
	pMain.End()

	dev.Submit(enc.Finish())
	if sr.LastError() != nil {
		t.Fatalf("SoftwareRenderer shadow error: %v", sr.LastError())
	}

	// Verify raw depth texture readback (float32 per texel).
	var depthBytes []byte
	dev.ReadTexture(shadowRT.Depth, gpu.TextureRegion{Width: 64, Height: 64}, func(raw []byte, err error) {
		if err != nil {
			t.Errorf("ReadTexture shadow depth: %v", err)
		}
		depthBytes = raw
	})
	if len(depthBytes) != 64*64*4 {
		t.Fatalf("depth readback bytes = %d, want %d", len(depthBytes), 64*64*4)
	}
	readDepth := func(x, y int) float32 {
		return math.Float32frombits(binary.LittleEndian.Uint32(depthBytes[(y*64+x)*4:]))
	}
	if d := readDepth(32, 32); math.Abs(float64(d-0.25)) > 1e-4 {
		t.Errorf("shadow map center depth = %f, want 0.25", d)
	}
	if d := readDepth(4, 4); math.Abs(float64(d-1.0)) > 1e-4 {
		t.Errorf("shadow map corner depth = %f, want 1.0", d)
	}

	// Verify EVERY pixel of the 64x64 receiver against exact analytical shadow reference.
	img := sr.Image()
	shadowCol := color.RGBA{51, 51, 51, 255}
	litCol := color.RGBA{255, 255, 255, 255}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			inShadow := x >= 16 && x < 48 && y >= 16 && y < 48
			want := litCol
			if inShadow {
				want = shadowCol
			}
			if got := img.RGBAAt(x, y); got != want {
				t.Fatalf("pixel (%d,%d) = %+v, want %+v (inShadow=%v)", x, y, got, want, inShadow)
			}
		}
	}

	// Verify CompareGreater inverts the comparison result cleanly.
	encInv := dev.BeginEncoder()
	pInv := encInv.BeginRenderPass(gpu.RenderPassDesc{
		ColorAttachments: []gpu.ColorAttachment{{
			LoadOp:     gpu.LoadClear,
			StoreOp:    gpu.StoreStore,
			ClearColor: core.RGB(0x000000),
		}},
	})
	pInv.SetPipeline(litPipe)
	pInv.SetBindGroup(0, litBGGreater)
	pInv.SetVertexBuffer(0, receiverVB, 0)
	pInv.Draw(6, 0)
	pInv.End()
	dev.Submit(encInv.Finish())
	imgInv := sr.Image()
	if got := imgInv.RGBAAt(32, 32); got != litCol {
		t.Errorf("CompareGreater center (32,32) = %+v, want lit %+v", got, litCol)
	}
	if got := imgInv.RGBAAt(4, 4); got != shadowCol {
		t.Errorf("CompareGreater corner (4,4) = %+v, want shadow %+v", got, shadowCol)
	}

	// Verify hardware 2x2 PCF (FilterLinear + textureSampleCompareLevel) right on the
	// shadow boundary UV = (0.25, 0.5), where 2 texels pass (1.0) and 2 fail (0.25) -> 0.5 visibility.
	pcfBG, err := dev.CreateBindGroup(nil, cmpOnlyBGL, []gpu.BindGroupEntry{
		{Binding: 0, Texture: shadowRT.Depth},
		{Binding: 1, Sampler: cmpSampPCF},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup pcf: %v", err)
	}
	pcfPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Label:    "shadow-pcf",
		Vertex:   gpu.ShaderStage{Module: pcfMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: pcfMod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{cmpOnlyBGL},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride: 5 * 4,
			Attributes: []gpu.VertexAttribute{
				{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
				{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x2},
			},
		}},
		Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
	})
	if err != nil {
		t.Fatalf("CreatePipeline pcf: %v", err)
	}
	// Constant UV = (0.25, 0.5) across fullscreen quad to probe exact 2x2 PCF boundary.
	edgeVerts := packFloat32s(
		-1, -1, 0.5, 0.25, 0.5,
		1, -1, 0.5, 0.25, 0.5,
		1, 1, 0.5, 0.25, 0.5,
		-1, -1, 0.5, 0.25, 0.5,
		1, 1, 0.5, 0.25, 0.5,
		-1, 1, 0.5, 0.25, 0.5,
	)
	edgeVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: edgeVerts})
	encPCF := dev.BeginEncoder()
	pPCF := encPCF.BeginRenderPass(gpu.RenderPassDesc{
		ColorAttachments: []gpu.ColorAttachment{{
			LoadOp:     gpu.LoadClear,
			StoreOp:    gpu.StoreStore,
			ClearColor: core.RGB(0x000000),
		}},
	})
	pPCF.SetPipeline(pcfPipe)
	pPCF.SetBindGroup(0, pcfBG)
	pPCF.SetVertexBuffer(0, edgeVB, 0)
	pPCF.Draw(6, 0)
	pPCF.End()
	dev.Submit(encPCF.Finish())
	if sr.LastError() != nil {
		t.Fatalf("PCF error: %v", sr.LastError())
	}
	// 0.2 + 0.8 * 0.5 = 0.6 -> round(0.6 * 255) = 153.
	if got := sr.Image().RGBAAt(32, 32); got != (color.RGBA{153, 153, 153, 255}) {
		t.Errorf("PCF boundary pixel = %+v, want {153 153 153 255}", got)
	}
}

func TestHeadlessViewShadowMapEndToEnd(t *testing.T) {
	depthMod := gpu.MustCompileWGSL(shadowDepthPassWGSL)
	litMod := gpu.MustCompileWGSL(shadowLitPassWGSL)

	ident := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	recvUniforms := append(append([]float32{}, ident[:]...), ident[:]...)
	occluderVerts := packFloat32s(
		-0.5, -0.5, 0.25,
		0.5, -0.5, 0.25,
		0.5, 0.5, 0.25,
		-0.5, -0.5, 0.25,
		0.5, 0.5, 0.25,
		-0.5, 0.5, 0.25,
	)
	receiverVerts := packFloat32s(
		-1, -1, 0.6,
		1, -1, 0.6,
		1, 1, 0.6,
		-1, -1, 0.6,
		1, 1, 0.6,
		-1, 1, 0.6,
	)

	var (
		shadowRT   *gpu.RenderTarget
		occluderVB *gpu.Buffer
		receiverVB *gpu.Buffer
		depthBG    *gpu.BindGroup
		litBG      *gpu.BindGroup
		depthPipe  *gpu.Pipeline
		litPipe    *gpu.Pipeline
		gpuErr     error
	)

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return gpu.View(cx, gpu.ViewProps{
			TestID: "shadow-view",
			Style:  core.Style{Width: core.Pt(64), Height: core.Pt(64)},
			OnError: func(err gpu.Error) {
				gpuErr = err
			},
			OnReady: func(dev *gpu.Device) {
				var err error
				shadowRT, err = dev.CreateRenderTarget(cx, gpu.RenderTargetDesc{
					Label:       "shadow",
					Width:       64,
					Height:      64,
					DepthFormat: gpu.FormatDepth32Float,
				})
				if err != nil {
					t.Fatalf("CreateRenderTarget: %v", err)
				}
				occluderVB, _ = dev.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: occluderVerts})
				receiverVB, _ = dev.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: receiverVerts})
				lightUB, _ := dev.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: packFloat32s(ident[:]...)})
				recvUB, _ := dev.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: packFloat32s(recvUniforms...)})
				cmpSamp, _ := dev.CreateSampler(cx, gpu.SamplerDesc{
					MinFilter: gpu.FilterNearest,
					MagFilter: gpu.FilterNearest,
					Compare:   gpu.CompareLessEqual,
				})
				dBGL, _ := dev.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
					{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
				})
				depthBG, _ = dev.CreateBindGroup(cx, dBGL, []gpu.BindGroupEntry{{Binding: 0, Buffer: lightUB}})
				depthPipe, err = dev.CreatePipeline(cx, gpu.PipelineDesc{
					Vertex:  gpu.ShaderStage{Module: depthMod, EntryPoint: "vs_shadow"},
					Layouts: []*gpu.BindGroupLayout{dBGL},
					VertexBuffers: []gpu.VertexBufferLayout{{
						Stride:     3 * 4,
						Attributes: []gpu.VertexAttribute{{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3}},
					}},
					DepthStencil: &gpu.DepthStencilState{
						Format:       gpu.FormatDepth32Float,
						DepthWrite:   true,
						DepthCompare: gpu.CompareLess,
					},
				})
				if err != nil {
					t.Fatalf("CreatePipeline depth: %v", err)
				}

				lBGL, _ := dev.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
					{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
					{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
					{Binding: 2, Visibility: gpu.StageFragment, Type: gpu.BindingComparisonSampler},
				})
				litBG, err = dev.CreateBindGroup(cx, lBGL, []gpu.BindGroupEntry{
					{Binding: 0, Buffer: recvUB},
					{Binding: 1, Texture: shadowRT.Depth},
					{Binding: 2, Sampler: cmpSamp},
				})
				if err != nil {
					t.Fatalf("CreateBindGroup lit: %v", err)
				}
				litPipe, err = dev.CreatePipeline(cx, gpu.PipelineDesc{
					Vertex:   gpu.ShaderStage{Module: litMod, EntryPoint: "vs_main"},
					Fragment: gpu.ShaderStage{Module: litMod, EntryPoint: "fs_main"},
					Layouts:  []*gpu.BindGroupLayout{lBGL},
					VertexBuffers: []gpu.VertexBufferLayout{{
						Stride:     3 * 4,
						Attributes: []gpu.VertexAttribute{{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3}},
					}},
					Targets: []gpu.ColorTargetState{{Format: dev.SurfaceFormat()}},
				})
				if err != nil {
					t.Fatalf("CreatePipeline lit: %v", err)
				}
			},
			OnFrame: func(frame *gpu.Frame) {
				p0 := frame.BeginRenderPass(gpu.RenderPassDesc{
					DepthStencil: &gpu.DepthStencilAttachment{
						Texture:      shadowRT.Depth,
						DepthLoadOp:  gpu.LoadClear,
						DepthStoreOp: gpu.StoreStore,
						ClearDepth:   1.0,
					},
				})
				p0.SetPipeline(depthPipe)
				p0.SetBindGroup(0, depthBG)
				p0.SetVertexBuffer(0, occluderVB, 0)
				p0.Draw(6, 0)
				p0.End()

				p1 := frame.BeginRenderPass(gpu.RenderPassDesc{
					ColorAttachments: []gpu.ColorAttachment{{
						LoadOp:     gpu.LoadClear,
						StoreOp:    gpu.StoreStore,
						ClearColor: core.RGB(0x000000),
					}},
				})
				p1.SetPipeline(litPipe)
				p1.SetBindGroup(0, litBG)
				p1.SetVertexBuffer(0, receiverVB, 0)
				p1.Draw(6, 0)
				p1.End()
			},
		})
	}, nsgotest.Size(64, 64), nsgotest.ManualFrames())

	app.Advance(16 * time.Millisecond)
	if gpuErr != nil {
		t.Fatalf("gpu OnError: %v", gpuErr)
	}
	img, err := app.GPUReadPixels("id:shadow-view")
	if err != nil {
		t.Fatalf("GPUReadPixels: %v", err)
	}
	if got := img.RGBAAt(32, 32); got != (color.RGBA{51, 51, 51, 255}) {
		t.Errorf("shadow-view center (32,32) = %+v, want {51 51 51 255}", got)
	}
	if got := img.RGBAAt(4, 4); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("shadow-view corner (4,4) = %+v, want {255 255 255 255}", got)
	}
}

const depthVisualizeWGSL = `
@group(0) @binding(0) var depthTex: texture_depth_2d;
@group(0) @binding(1) var depthSamp: sampler;

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
	let sampled = textureSample(depthTex, depthSamp, in.uv);
	let loaded = textureLoad(depthTex, vec2<i32>(32, 32), 0);
	return vec4<f32>(sampled, loaded, 0.0, 1.0);
}
`

func TestDepthTextureDirectSampleLoadAndCopy(t *testing.T) {
	depthMod := gpu.MustCompileWGSL(shadowDepthPassWGSL)
	visMod := gpu.MustCompileWGSL(depthVisualizeWGSL)

	sr := gpu.NewSoftwareRenderer(64, 64, gpu.FormatDepth24Plus)
	dev := sr.Device()

	rt, err := dev.CreateRenderTarget(nil, gpu.RenderTargetDesc{
		Label:       "depth-src",
		Width:       64,
		Height:      64,
		DepthFormat: gpu.FormatDepth32Float,
	})
	if err != nil {
		t.Fatalf("CreateRenderTarget: %v", err)
	}
	depthCopy, err := dev.CreateTexture(nil, gpu.TextureDesc{
		Label:  "depth-copy",
		Width:  64,
		Height: 64,
		Format: gpu.FormatDepth32Float,
		Usage:  gpu.TextureSampled | gpu.TextureCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture depthCopy: %v", err)
	}

	ident := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	uBuf, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: packFloat32s(ident[:]...)})
	occluderVerts := packFloat32s(
		-0.5, -0.5, 0.25,
		0.5, -0.5, 0.25,
		0.5, 0.5, 0.25,
		-0.5, -0.5, 0.25,
		0.5, 0.5, 0.25,
		-0.5, 0.5, 0.25,
	)
	occluderVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: occluderVerts})

	depthBGL, _ := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
	})
	depthBG, _ := dev.CreateBindGroup(nil, depthBGL, []gpu.BindGroupEntry{{Binding: 0, Buffer: uBuf}})
	depthPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:  gpu.ShaderStage{Module: depthMod, EntryPoint: "vs_shadow"},
		Layouts: []*gpu.BindGroupLayout{depthBGL},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride:     3 * 4,
			Attributes: []gpu.VertexAttribute{{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3}},
		}},
		DepthStencil: &gpu.DepthStencilState{
			Format:       gpu.FormatDepth32Float,
			DepthWrite:   true,
			DepthCompare: gpu.CompareLess,
		},
	})
	if err != nil {
		t.Fatalf("CreatePipeline depth: %v", err)
	}

	rawSamp, _ := dev.CreateSampler(nil, gpu.SamplerDesc{
		MinFilter: gpu.FilterNearest,
		MagFilter: gpu.FilterNearest,
		AddressU:  gpu.AddressClampToEdge,
		AddressV:  gpu.AddressClampToEdge,
	})
	visBGL, _ := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
		{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingSampler},
	})
	visBG, err := dev.CreateBindGroup(nil, visBGL, []gpu.BindGroupEntry{
		{Binding: 0, Texture: depthCopy},
		{Binding: 1, Sampler: rawSamp},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup vis: %v", err)
	}
	quadVerts := packFloat32s(
		-1, -1, 0.5, 0, 1,
		1, -1, 0.5, 1, 1,
		1, 1, 0.5, 1, 0,
		-1, -1, 0.5, 0, 1,
		1, 1, 0.5, 1, 0,
		-1, 1, 0.5, 0, 0,
	)
	quadVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: quadVerts})
	visPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:   gpu.ShaderStage{Module: visMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: visMod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{visBGL},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride: 5 * 4,
			Attributes: []gpu.VertexAttribute{
				{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
				{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x2},
			},
		}},
		Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
	})
	if err != nil {
		t.Fatalf("CreatePipeline vis: %v", err)
	}

	// Pass 1 renders occluder into rt.Depth.
	enc1 := dev.BeginEncoder()
	p1 := enc1.BeginRenderPass(gpu.RenderPassDesc{
		DepthStencil: &gpu.DepthStencilAttachment{
			Texture:      rt.Depth,
			DepthLoadOp:  gpu.LoadClear,
			DepthStoreOp: gpu.StoreStore,
			ClearDepth:   1.0,
		},
	})
	p1.SetPipeline(depthPipe)
	p1.SetBindGroup(0, depthBG)
	p1.SetVertexBuffer(0, occluderVB, 0)
	p1.Draw(6, 0)
	p1.End()
	dev.Submit(enc1.Finish())

	// Copy rt.Depth -> depthCopy, then Pass 2 samples and loads depthCopy directly.
	enc2 := dev.BeginEncoder()
	enc2.CopyTextureToTexture(
		gpu.TextureCopy{Texture: rt.Depth},
		gpu.TextureCopy{Texture: depthCopy},
		64, 64, 1,
	)
	p2 := enc2.BeginRenderPass(gpu.RenderPassDesc{
		ColorAttachments: []gpu.ColorAttachment{{
			LoadOp:     gpu.LoadClear,
			StoreOp:    gpu.StoreStore,
			ClearColor: core.RGB(0x000000),
		}},
	})
	p2.SetPipeline(visPipe)
	p2.SetBindGroup(0, visBG)
	p2.SetVertexBuffer(0, quadVB, 0)
	p2.Draw(6, 0)
	p2.End()
	dev.Submit(enc2.Finish())

	if sr.LastError() != nil {
		t.Fatalf("SoftwareRenderer error: %v", sr.LastError())
	}
	img := sr.Image()
	// Center (32,32): sampled = 0.25 -> 64, loaded = 0.25 -> 64.
	if got := img.RGBAAt(32, 32); got != (color.RGBA{64, 64, 0, 255}) {
		t.Errorf("center (32,32) = %+v, want {64 64 0 255}", got)
	}
	// Corner (4,4): sampled = 1.0 -> 255, loaded(32,32) = 0.25 -> 64.
	if got := img.RGBAAt(4, 4); got != (color.RGBA{255, 64, 0, 255}) {
		t.Errorf("corner (4,4) = %+v, want {255 64 0 255}", got)
	}
}

const computeParticlesWGSL = `
struct StepParams {
	dt: f32,
	gain: f32,
}

struct Vertex {
	pos: vec4<f32>,
	color: vec4<f32>,
}

@group(0) @binding(0) var<uniform> params: StepParams;
@group(0) @binding(1) var<storage, read> vel: array<vec4<f32>>;
@group(0) @binding(2) var<storage, read_write> verts: array<Vertex>;

@compute @workgroup_size(4, 1, 1)
fn step_main(
	@builtin(global_invocation_id) gid: vec3<u32>,
	@builtin(local_invocation_id) lid: vec3<u32>,
	@builtin(workgroup_id) wid: vec3<u32>,
	@builtin(num_workgroups) nwg: vec3<u32>,
) {
	let idx = gid.x;
	let count = arrayLength(&verts);
	if (idx < count) {
		let v = verts[idx];
		var next: Vertex;
		next.pos = v.pos + vel[idx] * params.dt;
		// Encode a deterministic green channel scaled by params.gain and workgroup/num_workgroups sanity check.
		let wgCheck = f32(wid.x + nwg.x - nwg.x) * 0.0 + f32(lid.x) * 0.0;
		next.color = vec4<f32>(v.color.x, params.gain + wgCheck, v.color.z, 1.0);
		verts[idx] = next;
	}
}
`

const storageTexComputeWGSL = `
@group(0) @binding(0) var outTex: texture_storage_2d<rgba8unorm, write>;

@compute @workgroup_size(2, 2, 1)
fn fill_tex(@builtin(global_invocation_id) gid: vec3<u32>) {
	let r = f32(gid.x) / 3.0;
	let g = f32(gid.y) / 3.0;
	textureStore(outTex, vec2<i32>(i32(gid.x), i32(gid.y)), vec4<f32>(r, g, 1.0, 1.0));
}
`

const directVertexColorWGSL = `
struct VertexIn {
	@location(0) pos: vec4<f32>,
	@location(1) color: vec4<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = in.pos;
	out.color = in.color;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	return in.color;
}
`

func unpackFloat32s(b []byte) []float32 {
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4 : i*4+4]))
	}
	return out
}

func packUint32s(vals ...uint32) []byte {
	out := make([]byte, len(vals)*4)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(out[i*4:], v)
	}
	return out
}

func TestHeadlessComputeStorageBufferAndRenderPass(t *testing.T) {
	computeMod := gpu.MustCompileWGSL(computeParticlesWGSL)
	renderMod := gpu.MustCompileWGSL(directVertexColorWGSL)
	texComputeMod := gpu.MustCompileWGSL(storageTexComputeWGSL)

	sr := gpu.NewSoftwareRenderer(32, 32, gpu.FormatUndefined)
	dev := sr.Device()

	if !dev.Capabilities().Features.Compute || !dev.SupportsCompute() {
		t.Fatalf("expected SoftwareRenderer to report Compute capability true")
	}
	if !dev.Capabilities().Features.StorageBuffers || !dev.Capabilities().Features.StorageTextures {
		t.Fatalf("expected SoftwareRenderer to support storage buffers and storage textures")
	}

	// Start with 6 vertices (2 triangles) off-screen at x = [-2, 0] and red=0, green=0, blue=0. The compute shader adds velocity (+1.0, 0, 0, 0) * dt (1.0) so the quad moves onto [-1, +1] x [-1, +1] (covering the whole screen) and sets green = 1.0!
	initVerts := packFloat32s(
		// pos (vec4)                 color (vec4)
		-2.0, -1.0, 0.5, 1.0, 0.0, 0.0, 0.0, 1.0,
		0.0, -1.0, 0.5, 1.0, 0.0, 0.0, 0.0, 1.0,
		0.0, 1.0, 0.5, 1.0, 0.0, 0.0, 0.0, 1.0,
		-2.0, -1.0, 0.5, 1.0, 0.0, 0.0, 0.0, 1.0,
		0.0, 1.0, 0.5, 1.0, 0.0, 0.0, 0.0, 1.0,
		-2.0, 1.0, 0.5, 1.0, 0.0, 0.0, 0.0, 1.0,
	)
	velocities := packFloat32s(
		1.0, 0.0, 0.0, 0.0,
		1.0, 0.0, 0.0, 0.0,
		1.0, 0.0, 0.0, 0.0,
		1.0, 0.0, 0.0, 0.0,
		1.0, 0.0, 0.0, 0.0,
		1.0, 0.0, 0.0, 0.0,
	)

	paramsBuf, err := dev.CreateBuffer(nil, gpu.BufferDesc{
		Label: "params",
		Usage: gpu.UsageUniform,
		Data:  packFloat32s(1.0, 1.0), // dt=1.0, gain=1.0
	})
	if err != nil {
		t.Fatalf("CreateBuffer params: %v", err)
	}
	velBuf, err := dev.CreateBuffer(nil, gpu.BufferDesc{
		Label: "velocities",
		Usage: gpu.BufferUsageStorage,
		Data:  velocities,
	})
	if err != nil {
		t.Fatalf("CreateBuffer vel: %v", err)
	}
	// Shared storage + vertex + copy-src buffer: written by @compute, read back via ReadBuffer, AND bound as a vertex buffer in the subsequent render pass!
	vertBuf, err := dev.CreateBuffer(nil, gpu.BufferDesc{
		Label: "particles",
		Usage: gpu.BufferUsageStorage | gpu.UsageVertex | gpu.UsageCopySrc | gpu.UsageCopyDst,
		Data:  initVerts,
	})
	if err != nil {
		t.Fatalf("CreateBuffer verts: %v", err)
	}
	indirectBuf, err := dev.CreateBuffer(nil, gpu.BufferDesc{
		Label: "dispatch_indirect",
		Usage: gpu.BufferUsageIndirect,
		Data:  packUint32s(2, 1, 1), // 2 workgroups * 4 threads = 8 invocations (bounds-checked to 6 verts by arrayLength)
	})
	if err != nil {
		t.Fatalf("CreateBuffer indirect: %v", err)
	}

	cBGL, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageCompute, Type: gpu.BindingUniform},
		{Binding: 1, Visibility: gpu.StageCompute, Type: gpu.BindingTypeReadOnlyStorageBuffer},
		{Binding: 2, Visibility: gpu.StageCompute, Type: gpu.BindingTypeStorageBuffer},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout compute: %v", err)
	}
	cBG, err := dev.CreateBindGroup(nil, cBGL, []gpu.BindGroupEntry{
		{Binding: 0, Buffer: paramsBuf},
		{Binding: 1, Buffer: velBuf},
		{Binding: 2, Buffer: vertBuf},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup compute: %v", err)
	}
	cPipe, err := dev.CreateComputePipeline(nil, gpu.ComputePipelineDesc{
		Label:   "step_particles",
		Compute: gpu.ShaderStage{Module: computeMod, EntryPoint: "step_main"},
		Layouts: []*gpu.BindGroupLayout{cBGL},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}

	rPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Label:    "draw_particles",
		Vertex:   gpu.ShaderStage{Module: renderMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: renderMod, EntryPoint: "fs_main"},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride: 8 * 4,
			Attributes: []gpu.VertexAttribute{
				{Location: 0, Offset: 0, Format: gpu.VertexFloat32x4},
				{Location: 1, Offset: 4 * 4, Format: gpu.VertexFloat32x4},
			},
		}},
		Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
	})
	if err != nil {
		t.Fatalf("CreatePipeline render: %v", err)
	}

	// Storage texture compute pass setup.
	storageTex, err := dev.CreateTexture(nil, gpu.TextureDesc{
		Label:  "compute_tex",
		Width:  4,
		Height: 4,
		Format: gpu.FormatRGBA8Unorm,
		Usage:  gpu.TextureStorageBinding | gpu.TextureCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture storage: %v", err)
	}
	tBGL, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageCompute, Type: gpu.BindingTypeStorageTexture},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout storage tex: %v", err)
	}
	tBG, err := dev.CreateBindGroup(nil, tBGL, []gpu.BindGroupEntry{
		{Binding: 0, Texture: storageTex},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup storage tex: %v", err)
	}
	tPipe, err := dev.CreateComputePipeline(nil, gpu.ComputePipelineDesc{
		Label:   "fill_tex",
		Compute: gpu.ShaderStage{Module: texComputeMod, EntryPoint: "fill_tex"},
		Layouts: []*gpu.BindGroupLayout{tBGL},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline tex: %v", err)
	}

	// Single command encoder: compute pass (indirect dispatch) -> storage texture compute pass -> render pass!
	enc := dev.BeginEncoder()

	cp := enc.BeginComputePass()
	cp.SetPipeline(cPipe)
	cp.SetBindGroup(0, cBG)
	if err := cp.DispatchIndirect(indirectBuf, 0); err != nil {
		t.Fatalf("DispatchIndirect: %v", err)
	}
	cp.SetPipeline(tPipe)
	cp.SetBindGroup(0, tBG)
	if err := cp.Dispatch(2, 2, 1); err != nil {
		t.Fatalf("Dispatch storage tex: %v", err)
	}
	cp.End()

	rp := enc.BeginRenderPass(gpu.RenderPassDesc{})
	rp.ClearColor(core.RGB(0x000000))
	rp.SetPipeline(rPipe)
	rp.SetVertexBuffer(0, vertBuf, 0)
	rp.Draw(6, 0)
	rp.End()

	dev.Submit(enc.Finish())
	if sr.LastError() != nil {
		t.Fatalf("SoftwareRenderer error: %v", sr.LastError())
	}

	// 1. Read back the compute-written storage buffer and verify exact float32 results.
	var gotBytes []byte
	var gotErr error
	dev.ReadBuffer(vertBuf, 0, 0, func(data []byte, err error) {
		gotBytes, gotErr = data, err
	})
	if gotErr != nil {
		t.Fatalf("ReadBuffer: %v", gotErr)
	}
	gotFloats := unpackFloat32s(gotBytes)
	wantFloats := []float32{
		-1.0, -1.0, 0.5, 1.0, 0.0, 1.0, 0.0, 1.0,
		1.0, -1.0, 0.5, 1.0, 0.0, 1.0, 0.0, 1.0,
		1.0, 1.0, 0.5, 1.0, 0.0, 1.0, 0.0, 1.0,
		-1.0, -1.0, 0.5, 1.0, 0.0, 1.0, 0.0, 1.0,
		1.0, 1.0, 0.5, 1.0, 0.0, 1.0, 0.0, 1.0,
		-1.0, 1.0, 0.5, 1.0, 0.0, 1.0, 0.0, 1.0,
	}
	if len(gotFloats) != len(wantFloats) {
		t.Fatalf("ReadBuffer float count = %d, want %d", len(gotFloats), len(wantFloats))
	}
	for i := range wantFloats {
		if math.Abs(float64(gotFloats[i]-wantFloats[i])) > 1e-6 {
			t.Errorf("ReadBuffer float[%d] = %v, want %v", i, gotFloats[i], wantFloats[i])
		}
	}

	// 2. Verify the render pass drew the compute-transformed fullscreen green quad!
	img := sr.Image()
	center := img.RGBAAt(16, 16)
	if center != (color.RGBA{R: 0, G: 255, B: 0, A: 255}) {
		t.Errorf("render pass center pixel = %+v, want green {0 255 0 255} from compute-written vertex buffer", center)
	}
	rightEdge := img.RGBAAt(28, 16)
	if rightEdge != (color.RGBA{R: 0, G: 255, B: 0, A: 255}) {
		t.Errorf("render pass right pixel = %+v, want green (proves x shifted from [-2,0] to [-1,1])", rightEdge)
	}

	// 3. Verify the storage texture written by textureStore!
	var texBytes []byte
	dev.ReadTexture(storageTex, gpu.TextureRegion{Width: 4, Height: 4}, func(rgba []byte, err error) {
		if err != nil {
			t.Fatalf("ReadTexture storageTex: %v", err)
		}
		texBytes = rgba
	})
	if len(texBytes) != 4*4*4 {
		t.Fatalf("storageTex bytes = %d, want 64", len(texBytes))
	}
	// Pixel (3, 3) has r = 3/3 = 255, g = 3/3 = 255, b = 255, a = 255.
	p33 := (3*4 + 3) * 4
	if texBytes[p33] != 255 || texBytes[p33+1] != 255 || texBytes[p33+2] != 255 || texBytes[p33+3] != 255 {
		t.Errorf("storageTex(3,3) = %v, want [255 255 255 255]", texBytes[p33:p33+4])
	}
	// Pixel (0, 0) has r = 0, g = 0, b = 255, a = 255.
	if texBytes[0] != 0 || texBytes[1] != 0 || texBytes[2] != 255 || texBytes[3] != 255 {
		t.Errorf("storageTex(0,0) = %v, want [0 0 255 255]", texBytes[0:4])
	}
}

func TestComputeCapabilityQueryAndUnsupportedRejection(t *testing.T) {
	computeMod := gpu.MustCompileWGSL(computeParticlesWGSL)

	// 1. Standalone SoftwareRenderer simulating WebGL 2 / GLES 3.0 (Compute: false).
	sr := gpu.NewSoftwareRendererWithFeatures(16, 16, gpu.FormatUndefined, gpu.Features{Depth32Float: true})
	dev := sr.Device()

	if dev.Capabilities().Features.Compute {
		t.Fatalf("expected Capabilities().Features.Compute == false on WebGL2/GLES3-like device")
	}
	if dev.SupportsCompute() {
		t.Fatalf("expected SupportsCompute() == false on WebGL2/GLES3-like device")
	}
	if _, err := dev.CreateBuffer(nil, gpu.BufferDesc{Size: 16, Usage: gpu.BufferUsageStorage}); !errors.Is(err, gpu.ErrUnsupported) {
		t.Errorf("CreateBuffer(UsageStorage) = %v, want ErrUnsupported", err)
	}
	if _, err := dev.CreateTexture(nil, gpu.TextureDesc{Width: 4, Height: 4, Format: gpu.FormatRGBA8Unorm, Usage: gpu.TextureStorageBinding}); !errors.Is(err, gpu.ErrUnsupported) {
		t.Errorf("CreateTexture(TextureStorageBinding) = %v, want ErrUnsupported", err)
	}
	if _, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageCompute, Type: gpu.BindingTypeStorageBuffer},
	}); !errors.Is(err, gpu.ErrUnsupported) {
		t.Errorf("CreateBindGroupLayout(StorageBuffer) = %v, want ErrUnsupported", err)
	}
	if _, err := dev.CreateComputePipeline(nil, gpu.ComputePipelineDesc{
		Compute: gpu.ShaderStage{Module: computeMod, EntryPoint: "step_main"},
	}); !errors.Is(err, gpu.ErrUnsupported) {
		t.Errorf("CreateComputePipeline = %v, want ErrUnsupported", err)
	}

	enc := dev.BeginEncoder()
	cp := enc.BeginComputePass()
	if err := cp.Dispatch(1, 1, 1); !errors.Is(err, gpu.ErrUnsupported) {
		t.Errorf("ComputePass.Dispatch on unsupported device = %v, want ErrUnsupported", err)
	}
	indBuf, err := dev.CreateBuffer(nil, gpu.BufferDesc{Size: 12, Usage: gpu.BufferUsageIndirect, Data: packUint32s(1, 1, 1)})
	if err != nil {
		t.Fatalf("CreateBuffer indirect: %v", err)
	}
	if err := cp.DispatchIndirect(indBuf, 0); !errors.Is(err, gpu.ErrUnsupported) {
		t.Errorf("ComputePass.DispatchIndirect on unsupported device = %v, want ErrUnsupported", err)
	}
	cp.End()

	// 2. Mounted gpu.View under nsgotest.NoGPUCompute() reports Compute: false cleanly.
	var viewCaps gpu.DeviceCapabilities
	var viewDispatchErr, viewReadErr error
	readCalled := false
	noCompute := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return gpu.View(cx, gpu.ViewProps{
			TestID: "nocompute",
			Style:  core.Style{Width: core.Pt(32), Height: core.Pt(32)},
			OnReady: func(d *gpu.Device) {
				viewCaps = d.Capabilities()
				e := d.BeginEncoder()
				p := e.BeginComputePass()
				viewDispatchErr = p.Dispatch(1, 1, 1)
				p.End()
				// A backend without compute must never receive a compute pass, so the frame carries no passes at all.
				d.Submit(e.Finish())
				buf, err := d.CreateBuffer(cx, gpu.BufferDesc{Size: 16, Usage: gpu.UsageUniform | gpu.UsageCopySrc})
				if err != nil {
					t.Fatalf("CreateBuffer: %v", err)
				}
				d.ReadBuffer(buf, 0, 0, func(_ []byte, err error) {
					readCalled = true
					viewReadErr = err
				})
			},
		})
	}, nsgotest.Size(32, 32), nsgotest.NoGPUCompute())

	if viewCaps.Features.Compute {
		t.Errorf("NoGPUCompute view Capabilities().Features.Compute = true, want false")
	}
	if !errors.Is(viewDispatchErr, gpu.ErrUnsupported) {
		t.Errorf("NoGPUCompute Dispatch error = %v, want ErrUnsupported", viewDispatchErr)
	}
	if !readCalled || !errors.Is(viewReadErr, gpu.ErrUnsupported) {
		t.Errorf("NoGPUCompute ReadBuffer called=%v err=%v, want ErrUnsupported", readCalled, viewReadErr)
	}
	for _, fr := range noCompute.GPUFrames() {
		if fr.PassesCount != 0 || fr.DispatchesCount != 0 {
			t.Errorf("NoGPUCompute frame recorded %d passes and %d dispatches, want none", fr.PassesCount, fr.DispatchesCount)
		}
	}

	// 3. Default nsgotest gpu.View supports compute and records DispatchesCount in GPUFrames.
	var gotDispatched []float32
	simpleCompute := gpu.MustCompileWGSL(`
@group(0) @binding(0) var<storage, read_write> data: array<f32>;
@compute @workgroup_size(4)
fn double_it(@builtin(global_invocation_id) gid: vec3<u32>) {
	let i = gid.x;
	if (i < arrayLength(&data)) {
		data[i] = data[i] * 2.0 + 1.0;
	}
}
`)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return gpu.View(cx, gpu.ViewProps{
			TestID: "computeview",
			Style:  core.Style{Width: core.Pt(32), Height: core.Pt(32)},
			OnReady: func(d *gpu.Device) {
				if !d.SupportsCompute() {
					t.Fatalf("expected default nsgotest GPU device to support compute")
				}
				buf, err := d.CreateBuffer(cx, gpu.BufferDesc{
					Usage: gpu.BufferUsageStorage | gpu.UsageCopySrc,
					Data:  packFloat32s(1, 2, 3, 4),
				})
				if err != nil {
					t.Fatalf("CreateBuffer: %v", err)
				}
				bgl, err := d.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
					{Binding: 0, Visibility: gpu.StageCompute, Type: gpu.BindingTypeStorageBuffer},
				})
				if err != nil {
					t.Fatalf("CreateBindGroupLayout: %v", err)
				}
				bg, err := d.CreateBindGroup(cx, bgl, []gpu.BindGroupEntry{{Binding: 0, Buffer: buf}})
				if err != nil {
					t.Fatalf("CreateBindGroup: %v", err)
				}
				pipe, err := d.CreateComputePipeline(cx, gpu.ComputePipelineDesc{
					Compute: gpu.ShaderStage{Module: simpleCompute, EntryPoint: "double_it"},
					Layouts: []*gpu.BindGroupLayout{bgl},
				})
				if err != nil {
					t.Fatalf("CreateComputePipeline: %v", err)
				}
				e := d.BeginEncoder()
				p := e.BeginComputePass()
				p.SetPipeline(pipe)
				p.SetBindGroup(0, bg)
				if err := p.Dispatch(1, 1, 1); err != nil {
					t.Fatalf("Dispatch: %v", err)
				}
				p.End()
				d.Submit(e.Finish())
				d.ReadBuffer(buf, 0, 0, func(b []byte, err error) {
					if err != nil {
						t.Errorf("ReadBuffer: %v", err)
						return
					}
					gotDispatched = unpackFloat32s(b)
				})
			},
		})
	}, nsgotest.Size(32, 32))

	want := []float32{3, 5, 7, 9}
	if len(gotDispatched) != len(want) {
		t.Fatalf("nsgotest ReadBuffer = %v, want %v", gotDispatched, want)
	}
	for i := range want {
		if gotDispatched[i] != want[i] {
			t.Errorf("gotDispatched[%d] = %v, want %v", i, gotDispatched[i], want[i])
		}
	}
	var totalDispatches int
	for _, fr := range app.GPUFrames() {
		totalDispatches += fr.DispatchesCount
	}
	if totalDispatches < 1 {
		t.Errorf("expected recorded DispatchesCount >= 1, got %+v", app.GPUFrames())
	}
}

const flatColorWGSL = `
struct VertexIn {
	@location(0) pos: vec3<f32>,
	@location(1) color: vec4<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = vec4<f32>(in.pos, 1.0);
	out.color = in.color;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	return in.color;
}
`

func rgbaColor(r, g, b, a uint8) core.Color {
	return core.RGBA(uint32(r)<<16|uint32(g)<<8|uint32(b), float32(a)/255.0)
}

func makeQuadVerts(x0, y0, x1, y1, z, r, g, b, a float32) []byte {
	// Two CCW triangles in NDC covering [x0, x1] x [y0, y1] at depth z.
	return packFloat32s(
		x0, y0, z, r, g, b, a,
		x1, y0, z, r, g, b, a,
		x1, y1, z, r, g, b, a,
		x0, y0, z, r, g, b, a,
		x1, y1, z, r, g, b, a,
		x0, y1, z, r, g, b, a,
	)
}

func makeQuadVertsCW(x0, y0, x1, y1, z, r, g, b, a float32) []byte {
	// Two CW triangles in NDC covering [x0, x1] x [y0, y1] at depth z.
	return packFloat32s(
		x0, y0, z, r, g, b, a,
		x1, y1, z, r, g, b, a,
		x1, y0, z, r, g, b, a,
		x0, y0, z, r, g, b, a,
		x0, y1, z, r, g, b, a,
		x1, y1, z, r, g, b, a,
	)
}

func readTexSync(t *testing.T, dev *gpu.Device, tex *gpu.Texture, w, h uint32) []byte {
	t.Helper()
	var out []byte
	var gotErr error
	dev.ReadTexture(tex, gpu.TextureRegion{Width: w, Height: h}, func(data []byte, err error) {
		out, gotErr = data, err
	})
	if gotErr != nil {
		t.Fatalf("ReadTexture: %v", gotErr)
	}
	return out
}

func TestMSAA4xResolveAntiAliasing(t *testing.T) {
	mod := gpu.MustCompileWGSL(flatColorWGSL)
	sr := gpu.NewSoftwareRenderer(32, 32, gpu.FormatUndefined)
	dev := sr.Device()

	if !dev.Features().MSAA4x {
		t.Fatalf("expected SoftwareRenderer Features.MSAA4x == true")
	}

	// Invalid sample count must be rejected synchronously.
	if _, err := dev.CreateTexture(nil, gpu.TextureDesc{
		Width: 16, Height: 16, SampleCount: 3, Format: gpu.FormatRGBA8Unorm, Usage: gpu.TextureRenderAttachment,
	}); !errors.Is(err, gpu.ErrInvalidDescriptor) {
		t.Fatalf("CreateTexture SampleCount=3 got %v, want ErrInvalidDescriptor", err)
	}

	// Diagonal white triangle whose hypotenuse crosses pixels at a shallow angle so 4x rotated-grid subsamples hit 1/4, 2/4, and 3/4 coverage along the edge.
	diagVerts := packFloat32s(
		-0.8, -0.8, 0.5, 1, 1, 1, 1,
		0.8, -0.8, 0.5, 1, 1, 1, 1,
		-0.2, 0.9, 0.5, 1, 1, 1, 1,
	)
	vb, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: diagVerts})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	vbLayout := []gpu.VertexBufferLayout{{
		Stride: 7 * 4,
		Attributes: []gpu.VertexAttribute{
			{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
			{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x4},
		},
	}}

	// 1x target & pipeline.
	rt1, err := dev.CreateRenderTarget(nil, gpu.RenderTargetDesc{
		Label: "msaa1", Width: 32, Height: 32, ColorFormat: gpu.FormatRGBA8Unorm, SampleCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateRenderTarget 1x: %v", err)
	}
	pipe1, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
		Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
		VertexBuffers: vbLayout,
		Targets:       []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		SampleCount:   1,
	})
	if err != nil {
		t.Fatalf("CreatePipeline 1x: %v", err)
	}

	// 4x MSAA target (automatic ResolveTarget on pass end) + explicit resolve destination.
	rt4, err := dev.CreateRenderTarget(nil, gpu.RenderTargetDesc{
		Label: "msaa4", Width: 32, Height: 32, ColorFormat: gpu.FormatRGBA8Unorm, SampleCount: 4,
	})
	if err != nil {
		t.Fatalf("CreateRenderTarget 4x: %v", err)
	}
	if rt4.Resolve == nil {
		t.Fatalf("expected 4x CreateRenderTarget to allocate Resolve texture")
	}
	explicitDst, err := dev.CreateTexture(nil, gpu.TextureDesc{
		Label: "explicitResolve", Width: 32, Height: 32, Format: gpu.FormatRGBA8Unorm,
		Usage: gpu.TextureRenderAttachment | gpu.TextureSampled | gpu.TextureCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture explicitDst: %v", err)
	}
	pipe4, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
		Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
		VertexBuffers: vbLayout,
		Targets:       []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		SampleCount:   4,
	})
	if err != nil {
		t.Fatalf("CreatePipeline 4x: %v", err)
	}

	enc := dev.BeginEncoder()
	// Pass 1: 1x render.
	p1 := enc.BeginRenderPass(gpu.RenderPassDesc{
		ColorAttachments: []gpu.ColorAttachment{{
			Texture: rt1.Color, LoadOp: gpu.LoadClear, StoreOp: gpu.StoreStore, ClearColor: rgbaColor(0, 0, 0, 0),
		}},
	})
	p1.SetPipeline(pipe1)
	p1.SetVertexBuffer(0, vb, 0)
	p1.Draw(3, 0)
	p1.End()

	// Pass 2: 4x MSAA render with automatic pass-end resolve into rt4.Resolve.
	p4 := enc.BeginRenderPass(gpu.RenderPassDesc{
		ColorAttachments: []gpu.ColorAttachment{{
			Texture: rt4.Color, ResolveTarget: rt4.Resolve, LoadOp: gpu.LoadClear, StoreOp: gpu.StoreStore, ClearColor: rgbaColor(0, 0, 0, 0),
		}},
	})
	p4.SetPipeline(pipe4)
	p4.SetVertexBuffer(0, vb, 0)
	p4.Draw(3, 0)
	p4.End()

	// Explicit ResolveTexture from rt4.Color into explicitDst.
	enc.ResolveTexture(gpu.TextureCopy{Texture: rt4.Color}, gpu.TextureCopy{Texture: explicitDst})
	dev.Submit(enc.Finish())
	if sr.LastError() != nil {
		t.Fatalf("SoftwareRenderer error: %v", sr.LastError())
	}

	pix1 := readTexSync(t, dev, rt1.Color, 32, 32)
	pix4 := readTexSync(t, dev, rt4.Resolve, 32, 32)
	pixExp := readTexSync(t, dev, explicitDst, 32, 32)

	countIntermediate := func(buf []byte) (inter, full, empty int) {
		for i := 0; i < len(buf); i += 4 {
			r := buf[i]
			switch {
			case r == 0:
				empty++
			case r == 255:
				full++
			default:
				inter++
			}
		}
		return
	}

	inter1, full1, empty1 := countIntermediate(pix1)
	inter4, full4, empty4 := countIntermediate(pix4)
	if inter1 != 0 {
		t.Errorf("1x render had %d intermediate edge pixels, want 0 (full=%d empty=%d)", inter1, full1, empty1)
	}
	if full1 == 0 || empty1 == 0 {
		t.Fatalf("1x triangle didn't draw properly (full=%d empty=%d)", full1, empty1)
	}
	if inter4 < 8 {
		t.Errorf("4x MSAA resolve had only %d anti-aliased intermediate edge pixels, want >= 8 (full=%d empty=%d)", inter4, full4, empty4)
	}
	// Verify explicit ResolveTexture produced identical pixels to automatic ResolveTarget.
	for i := range pix4 {
		if pix4[i] != pixExp[i] {
			t.Fatalf("explicit ResolveTexture mismatch at byte %d: got %d, want %d", i, pixExp[i], pix4[i])
		}
	}

	// Also verify swapchain 4x MSAA automatic resolve via NewSoftwareRendererWithConfig.
	srSwap4 := gpu.NewSoftwareRendererWithConfig(32, 32, gpu.FormatRGBA8Unorm, gpu.FormatUndefined, 4)
	dSwap := srSwap4.Device()
	vbSwap, _ := dSwap.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: diagVerts})
	pSwap, _ := dSwap.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
		Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
		VertexBuffers: vbLayout,
		Targets:       []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		SampleCount:   4,
	})
	eSwap := dSwap.BeginEncoder()
	rp := eSwap.BeginRenderPass(gpu.RenderPassDesc{})
	rp.ClearColor(rgbaColor(0, 0, 0, 0))
	rp.SetPipeline(pSwap)
	rp.SetVertexBuffer(0, vbSwap, 0)
	rp.Draw(3, 0)
	rp.End()
	dSwap.Submit(eSwap.Finish())
	interSwap, _, _ := countIntermediate(srSwap4.Image().Pix)
	if interSwap < 8 {
		t.Errorf("swapchain 4x MSAA had only %d anti-aliased edge pixels, want >= 8", interSwap)
	}
}

func TestStencilMaskAndMaskedDraw(t *testing.T) {
	mod := gpu.MustCompileWGSL(flatColorWGSL)
	vbLayout := []gpu.VertexBufferLayout{{
		Stride: 7 * 4,
		Attributes: []gpu.VertexAttribute{
			{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
			{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x4},
		},
	}}

	for _, dsFmt := range []gpu.TextureFormat{gpu.FormatDepth24PlusStencil8, gpu.FormatStencil8} {
		sr := gpu.NewSoftwareRenderer(32, 32, dsFmt)
		dev := sr.Device()

		maskVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: makeQuadVerts(-0.5, -0.5, 0.5, 0.5, 0.5, 1, 0, 0, 1)})
		fullVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: makeQuadVerts(-1, -1, 1, 1, 0.5, 0, 1, 0, 1)})
		cwVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: makeQuadVertsCW(-0.25, -0.25, 0.25, 0.25, 0.5, 0, 0, 1, 1)})

		// Pipeline 1: write stencil ref into center square without modifying color (ColorWriteNone).
		maskPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: vbLayout,
			DepthStencil: &gpu.DepthStencilState{
				Format:       dsFmt,
				DepthWrite:   false,
				DepthCompare: gpu.CompareAlways,
				StencilFront: gpu.StencilFaceState{
					Compare: gpu.CompareAlways,
					PassOp:  gpu.StencilReplace,
				},
				StencilReadMask:  0xFF,
				StencilWriteMask: 0x0F, // test partial write mask: only low nibble written
			},
			Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm, WriteMask: gpu.ColorWriteNone}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline mask (%d): %v", dsFmt, err)
		}

		// Pipeline 2: draw green ONLY where stencil == ref (masked by StencilReadMask 0x0F).
		maskedPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: vbLayout,
			DepthStencil: &gpu.DepthStencilState{
				Format:       dsFmt,
				DepthWrite:   false,
				DepthCompare: gpu.CompareAlways,
				StencilFront: gpu.StencilFaceState{
					Compare: gpu.CompareEqual,
					PassOp:  gpu.StencilIncrementClamp, // increments stencil 5 -> 6 inside mask
				},
				StencilReadMask:  0x0F,
				StencilWriteMask: 0xFF,
			},
			Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline masked (%d): %v", dsFmt, err)
		}

		// Pipeline 3: back-facing (CW) triangle tested with StencilBack (CompareEqual ref=6) with CullNone.
		backPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: vbLayout,
			Primitive:     gpu.PrimitiveState{FrontFace: gpu.FrontFaceCCW, CullMode: gpu.CullNone},
			DepthStencil: &gpu.DepthStencilState{
				Format:       dsFmt,
				DepthWrite:   false,
				DepthCompare: gpu.CompareAlways,
				StencilFront: gpu.StencilFaceState{Compare: gpu.CompareNever},
				StencilBack:  gpu.StencilFaceState{Compare: gpu.CompareEqual, PassOp: gpu.StencilKeep},
			},
			Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline back (%d): %v", dsFmt, err)
		}

		enc := dev.BeginEncoder()
		pass := enc.BeginRenderPass(gpu.RenderPassDesc{})
		pass.ClearColor(core.RGB(0x102030))
		pass.ClearDepth(1.0)
		pass.ClearStencil(0xA0) // high nibble set; StencilWriteMask=0x0F keeps 0xA0 and writes low nibble 0x05 -> 0xA5!

		pass.SetPipeline(maskPipe)
		pass.SetStencilReference(0x05)
		pass.SetVertexBuffer(0, maskVB, 0)
		pass.Draw(6, 0)

		pass.SetPipeline(maskedPipe)
		pass.SetStencilReference(0x05) // (0x05 & 0x0F) == (0xA5 & 0x0F) -> passes inside square, increments 0xA5 -> 0xA6!
		pass.SetVertexBuffer(0, fullVB, 0)
		pass.Draw(6, 0)

		pass.SetPipeline(backPipe)
		pass.SetStencilReference(0xA6) // back-facing inner square matches 0xA6 and turns blue!
		pass.SetVertexBuffer(0, cwVB, 0)
		pass.Draw(6, 0)
		pass.End()

		dev.Submit(enc.Finish())
		if sr.LastError() != nil {
			t.Fatalf("SoftwareRenderer stencil (%d) error: %v", dsFmt, sr.LastError())
		}

		img := sr.Image()
		// Outer corner (2, 2): rejected by stencil -> keeps clear color (16, 32, 48, 255).
		outer := img.RGBAAt(2, 2)
		if outer != (color.RGBA{R: 16, G: 32, B: 48, A: 255}) {
			t.Errorf("format %d outer pixel = %+v, want clear {16 32 48 255}", dsFmt, outer)
		}
		// Mid ring (10, 16): inside mask square, outside CW center -> green (0, 255, 0, 255).
		mid := img.RGBAAt(10, 16)
		if mid.G < 250 || mid.R > 5 || mid.B > 5 {
			t.Errorf("format %d masked ring pixel = %+v, want green", dsFmt, mid)
		}
		// Center (16, 16): CW back-facing quad matched incremented stencil 0xA6 -> blue (0, 0, 255, 255).
		center := img.RGBAAt(16, 16)
		if center.B < 250 || center.R > 5 || center.G > 5 {
			t.Errorf("format %d back-face center pixel = %+v, want blue", dsFmt, center)
		}
	}
}

func TestBlendPresetsCustomFactorsWindingAndDepthBias(t *testing.T) {
	mod := gpu.MustCompileWGSL(flatColorWGSL)
	vbLayout := []gpu.VertexBufferLayout{{
		Stride: 7 * 4,
		Attributes: []gpu.VertexAttribute{
			{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
			{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x4},
		},
	}}

	// 1. Blend presets + custom factors + ColorWriteMask.
	runBlendCase := func(name string, clear core.Color, srcRGBA [4]float32, blend *gpu.BlendState, mask gpu.ColorWriteMask, constRGBA *[4]float32) color.RGBA {
		t.Helper()
		sr := gpu.NewSoftwareRenderer(8, 8, gpu.FormatUndefined)
		dev := sr.Device()
		vb, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: makeQuadVerts(-1, -1, 1, 1, 0.5, srcRGBA[0], srcRGBA[1], srcRGBA[2], srcRGBA[3])})
		pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: vbLayout,
			Targets:       []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm, Blend: blend, WriteMask: mask}},
		})
		if err != nil {
			t.Fatalf("%s CreatePipeline: %v", name, err)
		}
		enc := dev.BeginEncoder()
		p := enc.BeginRenderPass(gpu.RenderPassDesc{})
		p.ClearColor(clear)
		p.SetPipeline(pipe)
		if constRGBA != nil {
			p.SetBlendConstant(constRGBA[0], constRGBA[1], constRGBA[2], constRGBA[3])
		}
		p.SetVertexBuffer(0, vb, 0)
		p.Draw(6, 0)
		p.End()
		dev.Submit(enc.Finish())
		return sr.Image().RGBAAt(4, 4)
	}

	near := func(got, want uint8) bool { return int(got)-int(want) <= 3 && int(want)-int(got) <= 3 }

	// BlendOpaque: replaces dst completely.
	px := runBlendCase("opaque", rgbaColor(0, 200, 0, 255), [4]float32{0.5, 0, 0, 0.5}, gpu.BlendOpaque(), gpu.ColorWriteAll, nil)
	if !near(px.R, 128) || px.G > 2 || !near(px.A, 128) {
		t.Errorf("BlendOpaque got %+v, want ~{128 0 0 128}", px)
	}
	// BlendPremultipliedAlpha: src=(0.5,0,0,0.5) over dst=(0,1,0,1) -> (0.5, 0.5, 0, 1.0).
	px = runBlendCase("premul", rgbaColor(0, 255, 0, 255), [4]float32{0.5, 0, 0, 0.5}, gpu.BlendPremultipliedAlpha(), gpu.ColorWriteAll, nil)
	if !near(px.R, 128) || !near(px.G, 128) || px.B > 2 || px.A != 255 {
		t.Errorf("BlendPremultipliedAlpha got %+v, want ~{128 128 0 255}", px)
	}
	// BlendAdditive: src=(0.5,0,0,1.0) + dst=(0.25,0.5,0,1.0) -> (0.75, 0.5, 0, 1.0).
	px = runBlendCase("additive", rgbaColor(64, 128, 0, 255), [4]float32{0.5, 0, 0, 1.0}, gpu.BlendAdditive(), gpu.ColorWriteAll, nil)
	if !near(px.R, 191) || !near(px.G, 128) {
		t.Errorf("BlendAdditive got %+v, want ~{191 128 0 255}", px)
	}
	// BlendMultiply: src=(0.5,0.5,1,1) * dst=(1,0.5,0.5,1) -> (0.5, 0.25, 0.5, 1.0).
	px = runBlendCase("multiply", rgbaColor(255, 128, 128, 255), [4]float32{0.5, 0.5, 1.0, 1.0}, gpu.BlendMultiply(), gpu.ColorWriteAll, nil)
	if !near(px.R, 128) || !near(px.G, 64) || !near(px.B, 128) {
		t.Errorf("BlendMultiply got %+v, want ~{128 64 128 255}", px)
	}
	// Custom separate color/alpha BlendFactor + BlendOp with BlendConstant: Color: src*const - dst*1 -> (1*0.75 - 0.25) = 0.5 on R; Alpha: Max(src.a=0.25, dst.a=0.5) = 0.5.
	customBlend := &gpu.BlendState{
		Color: gpu.BlendComponent{SrcFactor: gpu.BlendConstant, DstFactor: gpu.BlendOne, Operation: gpu.BlendOp(gpu.BlendSubtract)},
		Alpha: gpu.BlendComponent{SrcFactor: gpu.BlendOne, DstFactor: gpu.BlendOne, Operation: gpu.BlendMax},
	}
	bc := [4]float32{0.75, 0.5, 0.25, 1.0}
	px = runBlendCase("custom", rgbaColor(64, 0, 0, 128), [4]float32{1, 1, 1, 0.25}, customBlend, gpu.ColorWriteAll, &bc)
	if !near(px.R, 128) || !near(px.G, 128) || !near(px.B, 64) || !near(px.A, 128) {
		t.Errorf("custom BlendConstant/Subtract/Max got %+v, want ~{128 128 64 128}", px)
	}
	// ColorWriteMask: write red channel only, preserving green/blue clear color.
	px = runBlendCase("writeMaskR", rgbaColor(0, 200, 100, 255), [4]float32{1, 0, 0, 0}, nil, gpu.ColorWriteRed, nil)
	if px.R != 255 || !near(px.G, 200) || !near(px.B, 100) || px.A != 255 {
		t.Errorf("ColorWriteRed got %+v, want {255 200 100 255}", px)
	}

	// 2. Face culling & winding (FrontFaceCCW vs FrontFaceCW, CullFront vs CullBack).
	checkCull := func(cwVerts bool, front gpu.FrontFace, cull gpu.CullMode, wantDrawn bool) {
		t.Helper()
		sr := gpu.NewSoftwareRenderer(8, 8, gpu.FormatUndefined)
		dev := sr.Device()
		verts := makeQuadVerts(-1, -1, 1, 1, 0.5, 1, 0, 0, 1)
		if cwVerts {
			verts = makeQuadVertsCW(-1, -1, 1, 1, 0.5, 1, 0, 0, 1)
		}
		vb, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: verts})
		pipe, _ := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: vbLayout,
			Primitive:     gpu.PrimitiveState{Topology: gpu.TopologyTriangleList, FrontFace: front, CullMode: cull},
			Targets:       []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		})
		enc := dev.BeginEncoder()
		p := enc.BeginRenderPass(gpu.RenderPassDesc{})
		p.ClearColor(rgbaColor(0, 0, 0, 255))
		p.SetPipeline(pipe)
		p.SetVertexBuffer(0, vb, 0)
		p.Draw(6, 0)
		p.End()
		dev.Submit(enc.Finish())
		got := sr.Image().RGBAAt(4, 4).R > 200
		if got != wantDrawn {
			t.Errorf("cull(cw=%v, front=%d, cull=%d) drawn=%v, want %v", cwVerts, front, cull, got, wantDrawn)
		}
	}
	checkCull(false, gpu.FrontFaceCCW, gpu.CullBack, true)
	checkCull(true, gpu.FrontFaceCCW, gpu.CullBack, false)
	checkCull(true, gpu.FrontFaceCW, gpu.CullBack, true)
	checkCull(false, gpu.FrontFaceCW, gpu.CullBack, false)
	checkCull(false, gpu.FrontFaceCCW, gpu.CullFront, false)
	checkCull(true, gpu.FrontFaceCCW, gpu.CullFront, true)
	checkCull(true, gpu.FrontFaceCCW, gpu.CullNone, true)

	// 3. Per-pipeline DepthBias, DepthBiasSlopeScale, and DepthBiasClamp.
	sr := gpu.NewSoftwareRenderer(16, 16, gpu.FormatDepth24Plus)
	dev := sr.Device()
	blueVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: makeQuadVerts(-1, -1, 1, 1, 0.5, 0, 0, 1, 1)})
	redVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: makeQuadVerts(-1, -1, 1, 1, 0.5, 1, 0, 0, 1)})
	greenVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: makeQuadVerts(-0.5, -0.5, 0.5, 0.5, 0.5, 0, 1, 0, 1)})

	makeBiasPipe := func(bias int32, slope, clamp float32) *gpu.Pipeline {
		p, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: vbLayout,
			DepthStencil: &gpu.DepthStencilState{
				Format:              gpu.FormatDepth24Plus,
				DepthWrite:          true,
				DepthCompare:        gpu.CompareLess,
				DepthBias:           bias,
				DepthBiasSlopeScale: slope,
				DepthBiasClamp:      clamp,
			},
			Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline bias: %v", err)
		}
		return p
	}
	basePipe := makeBiasPipe(0, 0, 0)
	posBiasPipe := makeBiasPipe(1000, 0, 0.01)   // pushed further away -> loses CompareLess to coplanar z=0.5
	negBiasPipe := makeBiasPipe(-1000, 0, -0.01) // pulled closer -> wins CompareLess over coplanar z=0.5

	enc := dev.BeginEncoder()
	p := enc.BeginRenderPass(gpu.RenderPassDesc{})
	p.ClearColor(rgbaColor(0, 0, 0, 255))
	p.ClearDepth(1.0)
	p.SetPipeline(basePipe)
	p.SetVertexBuffer(0, blueVB, 0)
	p.Draw(6, 0)
	p.SetPipeline(posBiasPipe)
	p.SetVertexBuffer(0, redVB, 0)
	p.Draw(6, 0)
	p.SetPipeline(negBiasPipe)
	p.SetVertexBuffer(0, greenVB, 0)
	p.Draw(6, 0)
	p.End()
	dev.Submit(enc.Finish())

	img := sr.Image()
	// Outer pixel (2, 2): blue base survived positive-biased coplanar red quad!
	if got := img.RGBAAt(2, 2); got.B < 250 || got.R > 5 {
		t.Errorf("positive depth bias should lose CompareLess: got %+v, want blue", got)
	}
	// Center pixel (8, 8): negative-biased coplanar green quad won CompareLess over blue base!
	if got := img.RGBAAt(8, 8); got.G < 250 || got.B > 5 {
		t.Errorf("negative depth bias should win CompareLess: got %+v, want green", got)
	}
}

const mrt4WGSL = `
struct VertexIn {
	@location(0) pos: vec3<f32>,
}

struct FragOut {
	@location(0) t0: vec4<f32>,
	@location(1) t1: vec4<f32>,
	@location(2) t2: vec4<f32>,
	@location(3) t3: f32,
}

@vertex
fn vs_main(in: VertexIn) -> @builtin(position) vec4<f32> {
	return vec4<f32>(in.pos, 1.0);
}

@fragment
fn fs_main() -> FragOut {
	var out: FragOut;
	out.t0 = vec4<f32>(1.0, 0.0, 0.0, 1.0);
	out.t1 = vec4<f32>(0.0, 1.0, 0.5, 1.0);
	out.t2 = vec4<f32>(3.0, 1.5, 0.25, 1.0);
	out.t3 = 6.25;
	return out;
}
`

func TestMRTFourColorTargets(t *testing.T) {
	mod := gpu.MustCompileWGSL(mrt4WGSL)
	sr := gpu.NewSoftwareRenderer(8, 8, gpu.FormatUndefined)
	dev := sr.Device()

	if dev.Limits().MaxColorAttachments < 4 {
		t.Fatalf("MaxColorAttachments = %d, want >= 4", dev.Limits().MaxColorAttachments)
	}

	// Pipeline exceeding MaxColorAttachments must fail synchronously.
	if _, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
		Targets:  make([]gpu.ColorTargetState, 5),
	}); !errors.Is(err, gpu.ErrInvalidDescriptor) {
		t.Fatalf("CreatePipeline 5 targets got %v, want ErrInvalidDescriptor", err)
	}

	mkTex := func(label string, format gpu.TextureFormat) *gpu.Texture {
		tex, err := dev.CreateTexture(nil, gpu.TextureDesc{
			Label: label, Width: 8, Height: 8, Format: format,
			Usage: gpu.TextureRenderAttachment | gpu.TextureSampled | gpu.TextureCopySrc,
		})
		if err != nil {
			t.Fatalf("CreateTexture %s: %v", label, err)
		}
		return tex
	}
	t0 := mkTex("mrt0", gpu.FormatRGBA8Unorm)
	t1 := mkTex("mrt1", gpu.FormatRGBA8Unorm)
	t2 := mkTex("mrt2", gpu.FormatRGBA16F)
	t3 := mkTex("mrt3", gpu.FormatR32F)

	quad := packFloat32s(
		-1, -1, 0.5,
		1, -1, 0.5,
		1, 1, 0.5,
		-1, -1, 0.5,
		1, 1, 0.5,
		-1, 1, 0.5,
	)
	vb, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: quad})
	pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride:     3 * 4,
			Attributes: []gpu.VertexAttribute{{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3}},
		}},
		Targets: []gpu.ColorTargetState{
			{Format: gpu.FormatRGBA8Unorm},
			{Format: gpu.FormatRGBA8Unorm, WriteMask: gpu.ColorWriteGreen | gpu.ColorWriteAlpha}, // mask out blue on target 1
			{Format: gpu.FormatRGBA16F},
			{Format: gpu.FormatR32F},
		},
	})
	if err != nil {
		t.Fatalf("CreatePipeline MRT: %v", err)
	}

	enc := dev.BeginEncoder()
	p := enc.BeginRenderPass(gpu.RenderPassDesc{
		ColorAttachments: []gpu.ColorAttachment{
			{Texture: t0, LoadOp: gpu.LoadClear, StoreOp: gpu.StoreStore, ClearColor: rgbaColor(0, 0, 0, 255)},
			{Texture: t1, LoadOp: gpu.LoadClear, StoreOp: gpu.StoreStore, ClearColor: rgbaColor(10, 20, 30, 255)},
			{Texture: t2, LoadOp: gpu.LoadClear, StoreOp: gpu.StoreStore, ClearColor: rgbaColor(0, 0, 0, 0)},
			{Texture: t3, LoadOp: gpu.LoadClear, StoreOp: gpu.StoreStore, ClearColor: rgbaColor(0, 0, 0, 0)},
		},
	})
	p.SetPipeline(pipe)
	p.SetVertexBuffer(0, vb, 0)
	p.Draw(6, 0)
	p.End()
	dev.Submit(enc.Finish())
	if sr.LastError() != nil {
		t.Fatalf("SoftwareRenderer MRT error: %v", sr.LastError())
	}

	// Target 0: red (255, 0, 0, 255).
	b0 := readTexSync(t, dev, t0, 8, 8)
	if b0[0] != 255 || b0[1] != 0 || b0[2] != 0 || b0[3] != 255 {
		t.Errorf("MRT target 0 = %v, want [255 0 0 255]", b0[:4])
	}
	// Target 1: green updated to 255, while red=10 and blue=30 preserved by per-target WriteMask!
	b1 := readTexSync(t, dev, t1, 8, 8)
	if b1[0] != 10 || b1[1] != 255 || b1[2] != 30 || b1[3] != 255 {
		t.Errorf("MRT target 1 = %v, want [10 255 30 255]", b1[:4])
	}
	// Target 2 (RGBA16F): 8 bytes/px; R=3.0 (0x4200), G=1.5 (0x3E00), B=0.25 (0x3400), A=1.0 (0x3C00).
	b2 := readTexSync(t, dev, t2, 8, 8)
	if len(b2) != 8*8*8 {
		t.Fatalf("MRT target 2 byte len = %d, want %d", len(b2), 8*8*8)
	}
	r16 := binary.LittleEndian.Uint16(b2[0:2])
	g16 := binary.LittleEndian.Uint16(b2[2:4])
	b16 := binary.LittleEndian.Uint16(b2[4:6])
	a16 := binary.LittleEndian.Uint16(b2[6:8])
	if r16 != 0x4200 || g16 != 0x3E00 || b16 != 0x3400 || a16 != 0x3C00 {
		t.Errorf("MRT target 2 RGBA16F bits = (%#x, %#x, %#x, %#x), want (0x4200, 0x3e00, 0x3400, 0x3c00)", r16, g16, b16, a16)
	}
	// Target 3 (R32F): 4 bytes/px; R=6.25.
	b3 := readTexSync(t, dev, t3, 8, 8)
	r32 := math.Float32frombits(binary.LittleEndian.Uint32(b3[0:4]))
	if math.Abs(float64(r32-6.25)) > 1e-5 {
		t.Errorf("MRT target 3 R32F = %v, want 6.25", r32)
	}
}

const hdrEmitWGSL = `
struct Params {
	val: vec4<f32>,
}
@group(0) @binding(0) var<uniform> params: Params;

@vertex
fn vs_main(@location(0) pos: vec3<f32>) -> @builtin(position) vec4<f32> {
	return vec4<f32>(pos, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
	return params.val;
}
`

const tonemapWGSL = `
@group(0) @binding(0) var hdrTex: texture_2d<f32>;
@group(0) @binding(1) var hdrSamp: sampler;

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
	let c = textureSample(hdrTex, hdrSamp, in.uv);
	let mapped = c.rgb / (vec3<f32>(1.0, 1.0, 1.0) + c.rgb);
	return vec4<f32>(mapped, 1.0);
}
`

func TestHDRFloatTargetsToneMapAndSRGBRoundtrip(t *testing.T) {
	hdrMod := gpu.MustCompileWGSL(hdrEmitWGSL)
	toneMod := gpu.MustCompileWGSL(tonemapWGSL)
	texMod := gpu.MustCompileWGSL(texturedWGSL)

	sr := gpu.NewSoftwareRenderer(8, 8, gpu.FormatUndefined)
	dev := sr.Device()

	// Verify capability queries for HDR float formats.
	feat := dev.Features()
	if !feat.ExtendedFormats || !dev.SupportsFormat(gpu.FormatRGBA32F) || !dev.SupportsFormat(gpu.FormatRG11B10F) {
		t.Fatalf("software device should support the extended HDR formats: %+v", feat)
	}

	posQuad := packFloat32s(
		-1, -1, 0.5,
		1, -1, 0.5,
		1, 1, 0.5,
		-1, -1, 0.5,
		1, 1, 0.5,
		-1, 1, 0.5,
	)
	uvQuad := packFloat32s(
		-1, -1, 0.5, 0, 1,
		1, -1, 0.5, 1, 1,
		1, 1, 0.5, 1, 0,
		-1, -1, 0.5, 0, 1,
		1, 1, 0.5, 1, 0,
		-1, 1, 0.5, 0, 0,
	)
	posVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: posQuad})
	uvVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: uvQuad})

	uBGL, _ := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingUniform},
	})
	tBGL, _ := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
		{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingSampler},
	})
	sampLin, _ := dev.CreateSampler(nil, gpu.SamplerDesc{MinFilter: gpu.FilterLinear, MagFilter: gpu.FilterLinear})

	tonePipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:   gpu.ShaderStage{Module: toneMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: toneMod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{tBGL},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride: 5 * 4,
			Attributes: []gpu.VertexAttribute{
				{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
				{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x2},
			},
		}},
		Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
	})
	if err != nil {
		t.Fatalf("CreatePipeline tonemap: %v", err)
	}

	// Render HDR (4.0, 2.0, 0.5, 1.0) into RGBA16F, RG11B10F, R32F, and RGBA32F, then sample & tone-map.
	uBuf, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: packFloat32s(4.0, 2.0, 0.5, 1.0)})
	uBG, _ := dev.CreateBindGroup(nil, uBGL, []gpu.BindGroupEntry{{Binding: 0, Buffer: uBuf}})

	type hdrCase struct {
		name                string
		format              gpu.TextureFormat
		wantR, wantG, wantB uint8 // Reinhard: 4/5=0.8->204, 2/3=0.667->170, 0.5/1.5=0.333->85
	}
	for _, tc := range []hdrCase{
		{"RGBA16F", gpu.FormatRGBA16F, 204, 170, 85},
		{"RG11B10F", gpu.FormatRG11B10F, 204, 170, 85},
		{"R32F", gpu.FormatR32F, 204, 0, 0},
		{"RGBA32F", gpu.FormatRGBA32F, 204, 170, 85},
	} {
		hdrTex, err := dev.CreateTexture(nil, gpu.TextureDesc{
			Label: tc.name, Width: 8, Height: 8, Format: tc.format,
			Usage: gpu.TextureRenderAttachment | gpu.TextureSampled | gpu.TextureCopySrc,
		})
		if err != nil {
			t.Fatalf("%s CreateTexture: %v", tc.name, err)
		}
		hdrPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:   gpu.ShaderStage{Module: hdrMod, EntryPoint: "vs_main"},
			Fragment: gpu.ShaderStage{Module: hdrMod, EntryPoint: "fs_main"},
			Layouts:  []*gpu.BindGroupLayout{uBGL},
			VertexBuffers: []gpu.VertexBufferLayout{{
				Stride:     3 * 4,
				Attributes: []gpu.VertexAttribute{{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3}},
			}},
			Targets: []gpu.ColorTargetState{{Format: tc.format}},
		})
		if err != nil {
			t.Fatalf("%s CreatePipeline: %v", tc.name, err)
		}
		sampleBG, _ := dev.CreateBindGroup(nil, tBGL, []gpu.BindGroupEntry{
			{Binding: 0, Texture: hdrTex},
			{Binding: 1, Sampler: sampLin},
		})

		enc := dev.BeginEncoder()
		p0 := enc.BeginRenderPass(gpu.RenderPassDesc{
			ColorAttachments: []gpu.ColorAttachment{{Texture: hdrTex, LoadOp: gpu.LoadClear, StoreOp: gpu.StoreStore}},
		})
		p0.SetPipeline(hdrPipe)
		p0.SetBindGroup(0, uBG)
		p0.SetVertexBuffer(0, posVB, 0)
		p0.Draw(6, 0)
		p0.End()

		p1 := enc.BeginRenderPass(gpu.RenderPassDesc{})
		p1.ClearColor(rgbaColor(0, 0, 0, 255))
		p1.SetPipeline(tonePipe)
		p1.SetBindGroup(0, sampleBG)
		p1.SetVertexBuffer(0, uvVB, 0)
		p1.Draw(6, 0)
		p1.End()
		dev.Submit(enc.Finish())

		got := sr.Image().RGBAAt(4, 4)
		diff := func(a, b uint8) int {
			d := int(a) - int(b)
			if d < 0 {
				return -d
			}
			return d
		}
		if diff(got.R, tc.wantR) > 2 || diff(got.G, tc.wantG) > 2 || diff(got.B, tc.wantB) > 2 {
			t.Errorf("%s tone-mapped pixel = %+v, want ~{%d %d %d 255} (HDR > 1.0 must not clamp!)", tc.name, got, tc.wantR, tc.wantG, tc.wantB)
		}
	}

	// Verify HDR additive blending on RGBA16F accumulates values > 1.0 (1.5 + 1.5 = 3.0 -> 3/4 = 0.75 -> ~191).
	u15Buf, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: packFloat32s(1.5, 0, 0, 1.0)})
	u15BG, _ := dev.CreateBindGroup(nil, uBGL, []gpu.BindGroupEntry{{Binding: 0, Buffer: u15Buf}})
	hdrBlendTex, _ := dev.CreateTexture(nil, gpu.TextureDesc{
		Label: "hdrBlend", Width: 8, Height: 8, Format: gpu.FormatRGBA16F,
		Usage: gpu.TextureRenderAttachment | gpu.TextureSampled | gpu.TextureCopySrc,
	})
	hdrBlendPipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Vertex:   gpu.ShaderStage{Module: hdrMod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: hdrMod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{uBGL},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride:     3 * 4,
			Attributes: []gpu.VertexAttribute{{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3}},
		}},
		Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA16F, Blend: gpu.BlendAdditive()}},
	})
	if err != nil {
		t.Fatalf("CreatePipeline HDR blend: %v", err)
	}
	blendSampleBG, _ := dev.CreateBindGroup(nil, tBGL, []gpu.BindGroupEntry{
		{Binding: 0, Texture: hdrBlendTex},
		{Binding: 1, Sampler: sampLin},
	})
	enc := dev.BeginEncoder()
	p0 := enc.BeginRenderPass(gpu.RenderPassDesc{
		ColorAttachments: []gpu.ColorAttachment{{Texture: hdrBlendTex, LoadOp: gpu.LoadClear, StoreOp: gpu.StoreStore}},
	})
	p0.SetPipeline(hdrBlendPipe)
	p0.SetBindGroup(0, u15BG)
	p0.SetVertexBuffer(0, posVB, 0)
	p0.Draw(6, 0)
	p0.Draw(6, 0) // 1.5 + 1.5 = 3.0
	p0.End()
	p1 := enc.BeginRenderPass(gpu.RenderPassDesc{})
	p1.SetPipeline(tonePipe)
	p1.SetBindGroup(0, blendSampleBG)
	p1.SetVertexBuffer(0, uvVB, 0)
	p1.Draw(6, 0)
	p1.End()
	dev.Submit(enc.Finish())
	if got := sr.Image().RGBAAt(4, 4); got.R < 189 || got.R > 193 {
		t.Errorf("RGBA16F additive blend (1.5+1.5=3.0) tone-mapped R = %d, want ~191", got.R)
	}

	// Verify sRGB <-> linear conversion for RGBA8UnormSRGB and BGRA8UnormSRGB.
	for _, srgbFmt := range []gpu.TextureFormat{gpu.FormatRGBA8UnormSRGB, gpu.FormatBGRA8UnormSRGB} {
		srcTex, err := dev.CreateTexture(nil, gpu.TextureDesc{
			Label: "srgbSrc", Width: 2, Height: 2, Format: srgbFmt,
			Usage: gpu.TextureSampled | gpu.TextureCopyDst | gpu.TextureCopySrc,
		})
		if err != nil {
			t.Fatalf("CreateTexture sRGB (%d): %v", srgbFmt, err)
		}
		// Upload sRGB-encoded pixel (R=188, G=128, B=50, A=255). Note: BGRA8UnormSRGB expects B, G, R, A byte order on upload!
		var pxBytes [4]byte
		if srgbFmt == gpu.FormatBGRA8UnormSRGB {
			pxBytes = [4]byte{50, 128, 188, 255}
		} else {
			pxBytes = [4]byte{188, 128, 50, 255}
		}
		raw := make([]byte, 2*2*4)
		for i := 0; i < len(raw); i += 4 {
			copy(raw[i:], pxBytes[:])
		}
		dev.WriteTexture(srcTex, gpu.TextureCopy{Texture: srcTex}, raw, gpu.DataLayout{BytesPerRow: 8, RowsPerImage: 2})

		sampNear, _ := dev.CreateSampler(nil, gpu.SamplerDesc{MinFilter: gpu.FilterNearest, MagFilter: gpu.FilterNearest})
		srgbBG, _ := dev.CreateBindGroup(nil, tBGL, []gpu.BindGroupEntry{
			{Binding: 0, Texture: srcTex},
			{Binding: 1, Sampler: sampNear},
		})

		// Pass A: Sample sRGB texture -> render into LINEAR RGBA8Unorm target. sRGB 188 -> linear ~0.503 (~128); sRGB 128 -> linear ~0.216 (~55); sRGB 50 -> linear ~0.0319 (~8).
		linPipe, _ := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:   gpu.ShaderStage{Module: texMod, EntryPoint: "vs_main"},
			Fragment: gpu.ShaderStage{Module: texMod, EntryPoint: "fs_main"},
			Layouts:  []*gpu.BindGroupLayout{tBGL},
			VertexBuffers: []gpu.VertexBufferLayout{{
				Stride: 5 * 4,
				Attributes: []gpu.VertexAttribute{
					{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
					{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x2},
				},
			}},
			Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		})

		// Pass B: Sample sRGB texture -> render into sRGB target (full sRGB -> linear -> sRGB roundtrip!).
		dstSRGB, _ := dev.CreateTexture(nil, gpu.TextureDesc{
			Label: "srgbDst", Width: 8, Height: 8, Format: srgbFmt,
			Usage: gpu.TextureRenderAttachment | gpu.TextureCopySrc,
		})
		srgbPipe, _ := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:   gpu.ShaderStage{Module: texMod, EntryPoint: "vs_main"},
			Fragment: gpu.ShaderStage{Module: texMod, EntryPoint: "fs_main"},
			Layouts:  []*gpu.BindGroupLayout{tBGL},
			VertexBuffers: []gpu.VertexBufferLayout{{
				Stride: 5 * 4,
				Attributes: []gpu.VertexAttribute{
					{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
					{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x2},
				},
			}},
			Targets: []gpu.ColorTargetState{{Format: srgbFmt}},
		})

		enc2 := dev.BeginEncoder()
		pa := enc2.BeginRenderPass(gpu.RenderPassDesc{})
		pa.SetPipeline(linPipe)
		pa.SetBindGroup(0, srgbBG)
		pa.SetVertexBuffer(0, uvVB, 0)
		pa.Draw(6, 0)
		pa.End()

		pb := enc2.BeginRenderPass(gpu.RenderPassDesc{
			ColorAttachments: []gpu.ColorAttachment{{Texture: dstSRGB, LoadOp: gpu.LoadClear, StoreOp: gpu.StoreStore}},
		})
		pb.SetPipeline(srgbPipe)
		pb.SetBindGroup(0, srgbBG)
		pb.SetVertexBuffer(0, uvVB, 0)
		pb.Draw(6, 0)
		pb.End()
		dev.Submit(enc2.Finish())

		linGot := sr.Image().RGBAAt(4, 4)
		if linGot.R < 126 || linGot.R > 130 || linGot.G < 53 || linGot.G > 57 || linGot.B < 6 || linGot.B > 10 {
			t.Errorf("sRGB->linear decode (%d) got %+v, want ~{128 55 8 255}", srgbFmt, linGot)
		}

		rtGot := readTexSync(t, dev, dstSRGB, 8, 8)
		if rtGot[0] < 187 || rtGot[0] > 189 || rtGot[1] < 127 || rtGot[1] > 129 || rtGot[2] < 49 || rtGot[2] > 51 || rtGot[3] != 255 {
			t.Errorf("sRGB roundtrip (%d) got %v, want ~[188 128 50 255]", srgbFmt, rtGot[:4])
		}
	}
}

// TestNativeLikeDeviceRejectsUngatedFeatures checks that a device reporting the capabilities of the native backends (no extended formats, no stencil testing) rejects those features with ErrUnsupported instead of handing them to a backend that cannot honour them.
func TestNativeLikeDeviceRejectsUngatedFeatures(t *testing.T) {
	sr := gpu.NewSoftwareRendererWithFeatures(8, 8, gpu.FormatDepth24PlusStencil8, gpu.Features{MSAA4x: true, Depth32Float: true})
	dev := sr.Device()
	if dev.Features().ExtendedFormats || dev.Features().StencilTest {
		t.Fatalf("features = %+v, want ExtendedFormats and StencilTest false", dev.Features())
	}
	for _, f := range []gpu.TextureFormat{gpu.FormatBGRA8UnormSRGB, gpu.FormatRG11B10F, gpu.FormatR32F, gpu.FormatRGBA32F, gpu.FormatStencil8} {
		if dev.SupportsFormat(f) {
			t.Errorf("SupportsFormat(%s) = true, want false", f)
		}
		if _, err := dev.CreateTexture(nil, gpu.TextureDesc{Width: 4, Height: 4, Format: f, Usage: gpu.TextureRenderAttachment | gpu.TextureSampled}); !errors.Is(err, gpu.ErrUnsupported) {
			t.Errorf("CreateTexture(%s) = %v, want ErrUnsupported", f, err)
		}
	}
	if !dev.SupportsFormat(gpu.FormatRGBA16F) || !dev.SupportsFormat(gpu.FormatRGBA8UnormSRGB) {
		t.Errorf("core formats must stay supported")
	}

	mod := gpu.MustCompileWGSL(flatColorWGSL)
	vbLayout := []gpu.VertexBufferLayout{{
		Stride: 7 * 4,
		Attributes: []gpu.VertexAttribute{
			{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
			{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x4},
		},
	}}
	base := gpu.PipelineDesc{
		Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
		Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
		VertexBuffers: vbLayout,
		Targets:       []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
	}
	hdr := base
	hdr.Targets = []gpu.ColorTargetState{{Format: gpu.FormatRGBA32F}}
	if _, err := dev.CreatePipeline(nil, hdr); !errors.Is(err, gpu.ErrUnsupported) {
		t.Errorf("CreatePipeline(RGBA32F target) = %v, want ErrUnsupported", err)
	}
	stencil := base
	stencil.DepthStencil = &gpu.DepthStencilState{
		Format:       gpu.FormatDepth24PlusStencil8,
		StencilFront: gpu.StencilFaceState{Compare: gpu.CompareAlways, PassOp: gpu.StencilReplace},
	}
	if _, err := dev.CreatePipeline(nil, stencil); !errors.Is(err, gpu.ErrUnsupported) {
		t.Errorf("CreatePipeline(stencil test) = %v, want ErrUnsupported", err)
	}
	// Depth/stencil attachments without stencil testing keep working.
	plain := base
	plain.DepthStencil = &gpu.DepthStencilState{Format: gpu.FormatDepth24PlusStencil8, DepthWrite: true}
	if _, err := dev.CreatePipeline(nil, plain); err != nil {
		t.Errorf("CreatePipeline(depth only) = %v, want nil", err)
	}
}
