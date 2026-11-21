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
