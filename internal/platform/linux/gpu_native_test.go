//go:build linux && !android

package linux

import (
	"encoding/binary"
	"image"
	"image/color"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

const linuxGPUTestLitTexturedWGSL = `
struct Uniforms {
	mvp: mat4x4<f32>,
	lightDir: vec4<f32>,
}

@group(0) @binding(0) var<uniform> u: Uniforms;
@group(0) @binding(1) var tex: texture_2d<f32>;
@group(0) @binding(2) var samp: sampler;

struct VertexIn {
	@location(0) pos: vec3<f32>,
	@location(1) normal: vec3<f32>,
	@location(2) uv: vec2<f32>,
	@location(3) tint: vec4<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) normal: vec3<f32>,
	@location(1) uv: vec2<f32>,
	@location(2) tint: vec4<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = u.mvp * vec4<f32>(in.pos, 1.0);
	out.normal = in.normal;
	out.uv = in.uv;
	out.tint = in.tint;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	let ndotl = max(dot(normalize(in.normal), normalize(u.lightDir.xyz)), 0.25);
	let base = textureSample(tex, samp, in.uv) * in.tint;
	return vec4<f32>(base.rgb * ndotl, base.a);
}
`

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

// TestLinuxGPUViewDepthTestedTexturedLitAndHUD opens a real GTK 4 window with a GtkGLArea-backed gpu.View, renders a depth-tested, textured, lit 3D scene with a HUD child overlaid on top, and verifies pixels via ReadSurface and window capture, plus clean GL resource release on unmount.
func TestLinuxGPUViewDepthTestedTexturedLitAndHUD(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	mod := gpu.MustCompileWGSL(linuxGPUTestLitTexturedWGSL)

	var (
		ready        atomic.Bool
		readOK       atomic.Bool
		readFailed   atomic.Bool
		gpuSupported = make(chan bool, 1)
		unmount      func()
	)

	// 2x2 texture: pure white so vertex tint + lighting produce crisp RGB checks.
	texImg := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		for x := range 2 {
			texImg.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}

	// Near red quad at z = 0.2 (drawn FIRST) and larger far blue quad at z = 0.8 (drawn SECOND — depth test must keep the near red quad in the center!). Layout per vertex: pos(3) + normal(3) + uv(2) + tint(4) = 12 floats = 48 bytes.
	verts := packF32(
		// Near red quad (0..3) at z = 0.2, normal = +Z
		-0.45, -0.45, 0.2, 0, 0, 1, 0, 1, 1, 0, 0, 1,
		0.45, -0.45, 0.2, 0, 0, 1, 1, 1, 1, 0, 0, 1,
		0.45, 0.45, 0.2, 0, 0, 1, 1, 0, 1, 0, 0, 1,
		-0.45, 0.45, 0.2, 0, 0, 1, 0, 0, 1, 0, 0, 1,
		// Far blue quad (4..7) at z = 0.8, normal = +Z, covers [-0.85, 0.85]
		-0.85, -0.85, 0.8, 0, 0, 1, 0, 1, 0, 0, 1, 1,
		0.85, -0.85, 0.8, 0, 0, 1, 1, 1, 0, 0, 1, 1,
		0.85, 0.85, 0.8, 0, 0, 1, 1, 0, 0, 0, 1, 1,
		-0.85, 0.85, 0.8, 0, 0, 1, 0, 0, 0, 0, 1, 1,
	)
	indices := packU16(
		0, 1, 2, 0, 2, 3,
		4, 5, 6, 4, 6, 7,
	)
	// Identity MVP (NDC depth in [0, 1] directly) + lightDir (0, 0, 1, 0).
	uniforms := packF32(
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
		0, 0, 1, 0,
	)

	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		cx.SetTitle(title)
		mountedSig := core.NewSignal(cx, true)
		unmount = func() { cx.Post(func() { mountedSig.Set(false) }) }
		supported := cx.Env().SupportsGPU()
		select {
		case gpuSupported <- supported:
		default:
		}
		if !supported {
			return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(200)}}
		}
		var (
			vb, ib, ub *gpu.Buffer
			bg         *gpu.BindGroup
			pipe       *gpu.Pipeline
			rt         *gpu.RenderTarget
			readSent   bool
		)
		return core.View{
			Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(200), Background: driver.RGB(0xffffff)},
			Children: []core.Node{
				core.Show{When: mountedSig.Get, Then: func(scx *core.Scope) core.Node {
					return gpu.View(scx, gpu.ViewProps{
						Style:       core.Style{Width: driver.Pt(200), Height: driver.Pt(200)},
						DepthFormat: gpu.FormatDepth24Plus,
						Children: []core.Node{
							core.View{Style: core.Style{Width: driver.Pt(24), Height: driver.Pt(24), Background: driver.RGB(0x00ff00)}},
						},
						OnError: func(err gpu.Error) {
							t.Errorf("gpu.View OnError: %v", err)
						},
						OnReady: func(dev *gpu.Device) {
							var err error
							vb, err = dev.CreateBuffer(scx, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: verts})
							if err != nil {
								t.Errorf("CreateBuffer vb: %v", err)
								return
							}
							ib, err = dev.CreateBuffer(scx, gpu.BufferDesc{Usage: gpu.UsageIndex, Data: indices})
							if err != nil {
								t.Errorf("CreateBuffer ib: %v", err)
								return
							}
							ub, err = dev.CreateBuffer(scx, gpu.BufferDesc{Usage: gpu.UsageUniform | gpu.UsageCopyDst, Data: uniforms})
							if err != nil {
								t.Errorf("CreateBuffer ub: %v", err)
								return
							}
							tex, err := dev.CreateTextureFromImage(scx, core.ImageSource{Pixels: texImg}, gpu.TextureDesc{
								Format: gpu.FormatRGBA8Unorm,
								Usage:  gpu.TextureSampled | gpu.TextureCopyDst,
							})
							if err != nil {
								t.Errorf("CreateTextureFromImage: %v", err)
								return
							}
							samp, err := dev.CreateSampler(scx, gpu.SamplerDesc{
								MinFilter: gpu.FilterNearest,
								MagFilter: gpu.FilterNearest,
								AddressU:  gpu.AddressClampToEdge,
								AddressV:  gpu.AddressClampToEdge,
							})
							if err != nil {
								t.Errorf("CreateSampler: %v", err)
								return
							}
							bgl, err := dev.CreateBindGroupLayout(scx, []gpu.BindGroupLayoutEntry{
								{Binding: 0, Visibility: gpu.StageAll, Type: gpu.BindingUniform},
								{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
								{Binding: 2, Visibility: gpu.StageFragment, Type: gpu.BindingSampler},
							})
							if err != nil {
								t.Errorf("CreateBindGroupLayout: %v", err)
								return
							}
							bg, err = dev.CreateBindGroup(scx, bgl, []gpu.BindGroupEntry{
								{Binding: 0, Buffer: ub},
								{Binding: 1, Texture: tex},
								{Binding: 2, Sampler: samp},
							})
							if err != nil {
								t.Errorf("CreateBindGroup: %v", err)
								return
							}
							pipe, err = dev.CreatePipeline(scx, gpu.PipelineDesc{
								Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
								Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
								Layouts:  []*gpu.BindGroupLayout{bgl},
								VertexBuffers: []gpu.VertexBufferLayout{{
									Stride: 12 * 4,
									Attributes: []gpu.VertexAttribute{
										{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
										{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x3},
										{Location: 2, Offset: 6 * 4, Format: gpu.VertexFloat32x2},
										{Location: 3, Offset: 8 * 4, Format: gpu.VertexFloat32x4},
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
								t.Errorf("CreatePipeline: %v", err)
								return
							}
							rt, err = dev.CreateRenderTarget(scx, gpu.RenderTargetDesc{
								Width:       16,
								Height:      16,
								ColorFormat: gpu.FormatRGBA8Unorm,
								DepthFormat: gpu.FormatDepth24Plus,
							})
							if err != nil {
								t.Errorf("CreateRenderTarget: %v", err)
								return
							}
							ready.Store(true)
						},
						OnFrame: func(frame *gpu.Frame) {
							if pipe == nil {
								return
							}
							// Offscreen clear pass.
							off := frame.BeginRenderPass(gpu.RenderPassDesc{
								ColorAttachments: []gpu.ColorAttachment{{
									Texture:    rt.Color,
									LoadOp:     gpu.LoadClear,
									StoreOp:    gpu.StoreStore,
									ClearColor: core.RGB(0x112233),
								}},
							})
							off.End()

							// Main swapchain pass with depth test.
							pass := frame.BeginRenderPass(gpu.RenderPassDesc{})
							pass.ClearColor(core.RGB(0x10141c))
							pass.ClearDepth(1.0)
							pass.SetPipeline(pipe)
							pass.SetBindGroup(0, bg)
							pass.SetVertexBuffer(0, vb, 0)
							pass.SetIndexBuffer(ib, gpu.IndexUint16, 0)
							pass.DrawIndexed(12, 0, 0)
							pass.End()
							frame.Submit()

							if !readSent {
								readSent = true
								frame.Device().ReadTexture(rt.Color, gpu.TextureRegion{Width: 16, Height: 16}, func(rgba []byte, err error) {
									if err != nil || len(rgba) != 16*16*4 || rgba[0] != 0x11 || rgba[1] != 0x22 || rgba[2] != 0x33 {
										readFailed.Store(true)
										return
									}
									readOK.Store(true)
								})
							}
						},
					})
				}},
			},
		}
	}

	w := startTestWindow(t, app, Options{Title: title, Width: 200, Height: 200})
	waitForTitle(t, w, title)
	if !<-gpuSupported {
		t.Skip("OpenGL 3.3 / ES 3.0 unavailable in this Linux environment")
	}

	// Wait for the depth-tested 3D scene and HUD overlay to render. Note Cairo/X11 pixels in waitPixels are BGRA ([0]=B, [1]=G, [2]=R).
	waitPixels(t, w, "green HUD top-left, red near quad center, blue far quad rim", func(p pixels) bool {
		hud := p.at(12, 12)      // green HUD child painted OVER the GPU surface
		center := p.at(100, 100) // near red quad wins depth test
		rim := p.at(35, 100)     // far blue quad visible around rim
		return hud[1] > 0xe0 && hud[2] < 0x20 &&
			center[2] > 0xe0 && center[0] < 0x20 &&
			rim[0] > 0xe0 && rim[2] < 0x20
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !readOK.Load() && !readFailed.Load() {
		time.Sleep(20 * time.Millisecond)
	}
	if readFailed.Load() || !readOK.Load() {
		t.Errorf("offscreen RenderTarget ReadTexture did not return expected clear color (ok=%v failed=%v)", readOK.Load(), readFailed.Load())
	}

	if !ready.Load() {
		t.Errorf("gpu.View OnReady was not invoked")
	}

	// Unmount the gpu.View and verify all GL resources are released cleanly.
	unmount()
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var liveGPU int
		w.onLoop(t, func() {
			for _, v := range w.views {
				if v != nil && v.gpu != nil {
					liveGPU++
				}
			}
		})
		if liveGPU == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	var remaining int
	w.onLoop(t, func() {
		for _, v := range w.views {
			if v != nil && v.gpu != nil {
				remaining++
			}
		}
	})
	if remaining != 0 {
		t.Errorf("expected 0 live glGPUView instances after unmount, got %d", remaining)
	}
}
