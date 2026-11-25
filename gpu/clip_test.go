package gpu_test

import (
	"image/color"
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/gpu"
)

const clipSpaceWGSL = `
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

func packFloat32Slice(vals ...float32) []byte {
	return makeClipVertsBytes(vals...)
}

func makeClipVertsBytes(vals ...float32) []byte {
	out := make([]byte, len(vals)*4)
	for i, v := range vals {
		b := math.Float32bits(v)
		out[i*4+0] = byte(b)
		out[i*4+1] = byte(b >> 8)
		out[i*4+2] = byte(b >> 16)
		out[i*4+3] = byte(b >> 24)
	}
	return out
}

func clipVertexLayout() []gpu.VertexBufferLayout {
	return []gpu.VertexBufferLayout{{
		Stride: 32,
		Attributes: []gpu.VertexAttribute{
			{Format: gpu.VertexFloat32x4, Offset: 0, Location: 0},
			{Format: gpu.VertexFloat32x4, Offset: 16, Location: 1},
		},
	}}
}

func TestSoftwareFarPlaneSkyboxCompareLessEqual(t *testing.T) {
	mod := gpu.MustCompileWGSL(clipSpaceWGSL)
	w0, w1, w2, w3 := float32(0.3), float32(7.0), float32(49.0), float32(13.0)
	quadData := packFloat32Slice(
		-w0, -w0, w0, w0, 0, 1, 0, 1,
		w1, -w1, w1, w1, 0, 1, 0, 1,
		w2, w2, w2, w2, 0, 1, 0, 1,
		-w0, -w0, w0, w0, 0, 1, 0, 1,
		w2, w2, w2, w2, 0, 1, 0, 1,
		-w3, w3, w3, w3, 0, 1, 0, 1,
	)

	for _, sz := range [][2]int{{13, 17}, {32, 32}, {64, 64}} {
		sr := gpu.NewSoftwareRenderer(sz[0], sz[1], gpu.FormatDepth24Plus)
		dev := sr.Device()
		vb, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: quadData})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: clipVertexLayout(),
			DepthStencil: &gpu.DepthStencilState{
				Format:       gpu.FormatDepth24Plus,
				DepthWrite:   true,
				DepthCompare: gpu.CompareLessEqual,
			},
			Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline: %v", err)
		}

		enc := dev.BeginEncoder()
		p := enc.BeginRenderPass(gpu.RenderPassDesc{})
		p.ClearColor(rgbaColor(0, 0, 0, 255))
		p.ClearDepth(1.0)
		p.SetPipeline(pipe)
		p.SetVertexBuffer(0, vb, 0)
		p.Draw(6, 0)
		p.End()
		dev.Submit(enc.Finish())

		img := sr.Image()
		want := color.RGBA{0, 255, 0, 255}
		for y := 0; y < sz[1]; y++ {
			for x := 0; x < sz[0]; x++ {
				if got := img.RGBAAt(x, y); got != want {
					t.Fatalf("%dx%d pixel(%d,%d) = %+v, want %+v", sz[0], sz[1], x, y, got, want)
				}
			}
		}
	}
}

func TestSoftwareNearPlaneClippingMatchesPreClippedReference(t *testing.T) {
	mod := gpu.MustCompileWGSL(clipSpaceWGSL)
	v0p, v0c := [4]float32{0.0, -1.5, -1.0, -1.0}, [4]float32{1, 0, 0, 1}
	v1p, v1c := [4]float32{1.0, 1.0, 1.0, 2.0}, [4]float32{0, 1, 0, 1}
	v2p, v2c := [4]float32{-1.0, 1.0, 1.0, 2.0}, [4]float32{0, 0, 1, 1}

	clipZ0 := func(inP, inC, outP, outC [4]float32) ([4]float32, [4]float32) {
		t := inP[2] / (inP[2] - outP[2])
		var p, c [4]float32
		for i := range 4 {
			p[i] = inP[i] + t*(outP[i]-inP[i])
			c[i] = inC[i] + t*(outC[i]-inC[i])
		}
		p[2] = 0
		return p, c
	}
	c01p, c01c := clipZ0(v1p, v1c, v0p, v0c)
	c02p, c02c := clipZ0(v2p, v2c, v0p, v0c)

	render := func(verts []float32) *gpu.SoftwareRenderer {
		sr := gpu.NewSoftwareRenderer(32, 32, gpu.FormatDepth24Plus)
		dev := sr.Device()
		vb, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: packFloat32Slice(verts...)})
		pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: clipVertexLayout(),
			DepthStencil: &gpu.DepthStencilState{
				Format:       gpu.FormatDepth24Plus,
				DepthWrite:   true,
				DepthCompare: gpu.CompareLessEqual,
			},
			Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline: %v", err)
		}
		enc := dev.BeginEncoder()
		p := enc.BeginRenderPass(gpu.RenderPassDesc{})
		p.ClearColor(rgbaColor(0, 0, 0, 255))
		p.ClearDepth(1.0)
		p.SetPipeline(pipe)
		p.SetVertexBuffer(0, vb, 0)
		p.Draw(uint32(len(verts)/8), 0)
		p.End()
		dev.Submit(enc.Finish())
		return sr
	}

	got := render([]float32{
		v0p[0], v0p[1], v0p[2], v0p[3], v0c[0], v0c[1], v0c[2], v0c[3],
		v1p[0], v1p[1], v1p[2], v1p[3], v1c[0], v1c[1], v1c[2], v1c[3],
		v2p[0], v2p[1], v2p[2], v2p[3], v2c[0], v2c[1], v2c[2], v2c[3],
	}).Image()

	ref := render([]float32{
		c01p[0], c01p[1], c01p[2], c01p[3], c01c[0], c01c[1], c01c[2], c01c[3],
		v1p[0], v1p[1], v1p[2], v1p[3], v1c[0], v1c[1], v1c[2], v1c[3],
		v2p[0], v2p[1], v2p[2], v2p[3], v2c[0], v2c[1], v2c[2], v2c[3],

		c01p[0], c01p[1], c01p[2], c01p[3], c01c[0], c01c[1], c01c[2], c01c[3],
		v2p[0], v2p[1], v2p[2], v2p[3], v2c[0], v2c[1], v2c[2], v2c[3],
		c02p[0], c02p[1], c02p[2], c02p[3], c02c[0], c02c[1], c02c[2], c02c[3],
	}).Image()

	drawn := 0
	bg := color.RGBA{0, 0, 0, 255}
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			g, r := got.RGBAAt(x, y), ref.RGBAAt(x, y)
			if r != bg {
				drawn++
			}
			if math.Abs(float64(int(g.R)-int(r.R))) > 2 ||
				math.Abs(float64(int(g.G)-int(r.G))) > 2 ||
				math.Abs(float64(int(g.B)-int(r.B))) > 2 {
				t.Fatalf("pixel(%d,%d) = %+v, ref = %+v", x, y, g, r)
			}
		}
	}
	if drawn < 50 {
		t.Fatalf("expected >= 50 clipped pixels, got %d", drawn)
	}
}

func TestSoftwareViewportDepthRangeClipsFarPlaneAndClamps(t *testing.T) {
	mod := gpu.MustCompileWGSL(clipSpaceWGSL)
	const w, h = 32, 32
	sr := gpu.NewSoftwareRenderer(w, h, gpu.FormatDepth24Plus)
	dev := sr.Device()

	// Quad whose depth ramps from z=0 at x=-1 to z=2.0 (> w=1) at x=+1.
	rampQuad := packFloat32Slice(
		-1, -1, 0.0, 1, 0, 1, 0, 1,
		1, -1, 2.0, 1, 0, 1, 0, 1,
		1, 1, 2.0, 1, 0, 1, 0, 1,
		-1, -1, 0.0, 1, 0, 1, 0, 1,
		1, 1, 2.0, 1, 0, 1, 0, 1,
		-1, 1, 0.0, 1, 0, 1, 0, 1,
	)
	// Reference quad at constant NDC z=0.5 under default [0, 1] viewport (depth = 0.5).
	// Draws in blue if stored depth > 0.5 (should never happen when vp=[0, 0.5]!).
	probeQuad := packFloat32Slice(
		-1, -1, 0.5, 1, 0, 0, 1, 1,
		1, -1, 0.5, 1, 0, 0, 1, 1,
		1, 1, 0.5, 1, 0, 0, 1, 1,
		-1, -1, 0.5, 1, 0, 0, 1, 1,
		1, 1, 0.5, 1, 0, 0, 1, 1,
		-1, 1, 0.5, 1, 0, 0, 1, 1,
	)

	rampVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: rampQuad})
	probeVB, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: probeQuad})

	makePipe := func(cmp gpu.CompareFunc) *gpu.Pipeline {
		p, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:        gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment:      gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: clipVertexLayout(),
			DepthStencil: &gpu.DepthStencilState{
				Format:       gpu.FormatDepth24Plus,
				DepthWrite:   true,
				DepthCompare: cmp,
			},
			Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline: %v", err)
		}
		return p
	}
	lePipe := makePipe(gpu.CompareLessEqual)
	gtPipe := makePipe(gpu.CompareGreater)

	enc := dev.BeginEncoder()
	p := enc.BeginRenderPass(gpu.RenderPassDesc{})
	p.ClearColor(rgbaColor(0, 0, 0, 255))
	p.ClearDepth(1.0)
	p.SetViewport(0, 0, w, h, 0.0, 0.5)
	p.SetPipeline(lePipe)
	p.SetVertexBuffer(0, rampVB, 0)
	p.Draw(6, 0)
	// With default [0, 1] viewport, probeQuad has depth=0.5; CompareGreater passes ONLY where existing depth < 0.5.
	// On the far-clipped right half (existing depth == 1.0), CompareGreater fails, leaving black.
	p.SetViewport(0, 0, w, h, 0.0, 1.0)
	p.SetPipeline(gtPipe)
	p.SetVertexBuffer(0, probeVB, 0)
	p.Draw(6, 0)
	p.End()
	dev.Submit(enc.Finish())

	img := sr.Image()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			got := img.RGBAAt(x, y)
			if x < w/2-1 {
				// Visible left half wrote depth < 0.5, so blue probeQuad with depth=0.5 & CompareGreater overwrote it!
				if got != (color.RGBA{0, 0, 255, 255}) {
					t.Fatalf("left half pixel(%d,%d) = %+v, want blue", x, y, got)
				}
			} else if x >= w/2+1 {
				// Far-clipped right half left depth == 1.0 and color == black; probeQuad (0.5 > 1.0) also fails.
				if got != (color.RGBA{0, 0, 0, 255}) {
					t.Fatalf("far-clipped right half pixel(%d,%d) = %+v, want black", x, y, got)
				}
			}
		}
	}
}
