package gpu_test

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

const instancedGridWGSL = `
struct VertexIn {
	@location(0) pos: vec2<f32>,
	@location(1) instOffset: vec2<f32>,
	@location(2) instColor: vec4<f32>,
	@builtin(instance_index) instIdx: u32,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = vec4<f32>(in.pos + in.instOffset, 0.5, 1.0);
	// Encode @builtin(instance_index) into the blue channel so tests verify
	// both per-instance vertex attributes (R, G) and @builtin(instance_index) (B).
	let b = f32(in.instIdx % 251u) / 255.0;
	out.color = vec4<f32>(in.instColor.x, in.instColor.y, b, 1.0);
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	return in.color;
}
`

const instancedStepRateWGSL = `
struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
}

@vertex
fn vs_main(
	@location(0) pos: vec2<f32>,
	@location(1) instColor: vec4<f32>,
	@builtin(instance_index) idx: u32,
) -> VertexOut {
	var out: VertexOut;
	// Place 16 columns evenly across NDC [-1, 1] using @builtin(instance_index).
	let cellW = 2.0 / 16.0;
	let cx = -1.0 + (f32(idx) + 0.5) * cellW;
	out.pos = vec4<f32>(pos.x + cx, pos.y, 0.5, 1.0);
	out.color = instColor;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	return in.color;
}
`

const dynamicUniformObjWGSL = `
struct ObjectUniform {
	offset: vec4<f32>,
	color: vec4<f32>,
}

@group(0) @binding(0) var<uniform> obj: ObjectUniform;

struct VertexIn {
	@location(0) pos: vec2<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = vec4<f32>(in.pos + obj.offset.xy, 0.5, 1.0);
	out.color = obj.color;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	return in.color;
}
`

func writeF32sInto(dst []byte, vals ...float32) {
	for i, v := range vals {
		binary.LittleEndian.PutUint32(dst[i*4:], math.Float32bits(v))
	}
}

func make100x100InstanceData() []byte {
	const grid = 100
	const count = grid * grid // 10,000 instances
	const stride = 6 * 4      // vec2 offset + vec4 color = 24 bytes
	buf := make([]byte, count*stride)
	cell := float32(2.0 / float64(grid))
	for i := 0; i < count; i++ {
		col := i % grid
		row := i / grid
		// NDC center of pixel (col, row) where screen y=0 is top (NDC y=+1).
		cx := -1.0 + (float32(col)+0.5)*cell
		cy := 1.0 - (float32(row)+0.5)*cell
		r := float32(col) / float32(grid-1)
		g := float32(row) / float32(grid-1)
		writeF32sInto(buf[i*stride:], cx, cy, r, g, 0, 1)
	}
	return buf
}

func expected100x100Color(col, row int) color.RGBA {
	idx := row*100 + col
	r := byte(math.Round(float64(col) / 99.0 * 255.0))
	g := byte(math.Round(float64(row) / 99.0 * 255.0))
	b := byte(idx % 251)
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func closeByte(a, b byte) bool {
	if a >= b {
		return a-b <= 1
	}
	return b-a <= 1
}

func TestInstancedDrawing10000Instances(t *testing.T) {
	mod := gpu.MustCompileWGSL(instancedGridWGSL)
	instData := make100x100InstanceData()

	// Unit 1x1-pixel quad centered at origin (half-extent = 0.01 in NDC for 100x100 canvas).
	const half = float32(0.01)
	quadVerts6 := packFloat32s(
		-half, -half,
		half, -half,
		half, half,
		-half, -half,
		half, half,
		-half, half,
	)
	quadVerts4 := packFloat32s(
		-half, -half,
		half, -half,
		half, half,
		-half, half,
	)
	quadIdx := packUint16s(0, 1, 2, 0, 2, 3)

	t.Run("DrawInstanced_10000_SingleCall_HeadlessView", func(t *testing.T) {
		var (
			quadVB *gpu.Buffer
			instVB *gpu.Buffer
			pipe   *gpu.Pipeline
		)
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return gpu.View(cx, gpu.ViewProps{
				TestID: "instanced",
				Style:  core.Style{Width: core.Pt(100), Height: core.Pt(100)},
				OnReady: func(dev *gpu.Device) {
					var err error
					quadVB, err = dev.CreateBuffer(cx, gpu.BufferDesc{Label: "quad", Usage: gpu.UsageVertex, Data: quadVerts6})
					if err != nil {
						t.Fatalf("CreateBuffer quad: %v", err)
					}
					instVB, err = dev.CreateBuffer(cx, gpu.BufferDesc{Label: "instances", Usage: gpu.UsageVertex, Data: instData})
					if err != nil {
						t.Fatalf("CreateBuffer instances: %v", err)
					}
					pipe, err = dev.CreatePipeline(cx, gpu.PipelineDesc{
						Label:    "instanced10k",
						Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
						Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
						VertexBuffers: []gpu.VertexBufferLayout{
							{
								Stride:   2 * 4,
								StepMode: gpu.VertexStepVertex,
								Attributes: []gpu.VertexAttribute{
									{Location: 0, Offset: 0, Format: gpu.VertexFloat32x2},
								},
							},
							{
								Stride:   6 * 4,
								StepMode: gpu.VertexStepInstance,
								Attributes: []gpu.VertexAttribute{
									{Location: 1, Offset: 0, Format: gpu.VertexFloat32x2},
									{Location: 2, Offset: 2 * 4, Format: gpu.VertexFloat32x4},
								},
							},
						},
						Targets: []gpu.ColorTargetState{{Format: dev.SurfaceFormat()}},
					})
					if err != nil {
						t.Fatalf("CreatePipeline: %v", err)
					}
				},
				OnFrame: func(frame *gpu.Frame) {
					pass := frame.BeginRenderPass(gpu.RenderPassDesc{})
					pass.ClearColor(core.RGB(0x000000))
					pass.SetPipeline(pipe)
					pass.SetVertexBuffer(0, quadVB, 0)
					pass.SetVertexBuffer(1, instVB, 0)
					// ONE single instanced draw call for all 10,000 instances!
					pass.DrawInstanced(6, 10000, 0, 0)
					pass.End()
					frame.Submit()
				},
			})
		}, nsgotest.Size(100, 100), nsgotest.ManualFrames())

		app.Advance(16 * time.Millisecond)

		var drawFrame *nsgotest.GPUFrameRecord
		for i := range app.GPUFrames() {
			fr := app.GPUFrames()[i]
			if fr.DrawsCount > 0 {
				drawFrame = &fr
				break
			}
		}
		if drawFrame == nil {
			t.Fatalf("expected a submitted draw frame, got %+v", app.GPUFrames())
		}
		if drawFrame.DrawsCount != 1 {
			t.Fatalf("DrawsCount = %d, want 1 single draw call", drawFrame.DrawsCount)
		}
		if drawFrame.InstancesCount != 10000 {
			t.Fatalf("InstancesCount = %d, want 10000", drawFrame.InstancesCount)
		}

		img, err := app.GPUReadPixels("id:instanced")
		if err != nil {
			t.Fatalf("GPUReadPixels: %v", err)
		}
		for row := 0; row < 100; row++ {
			for col := 0; col < 100; col++ {
				got := img.RGBAAt(col, row)
				want := expected100x100Color(col, row)
				if !closeByte(got.R, want.R) || !closeByte(got.G, want.G) || !closeByte(got.B, want.B) || got.A != 255 {
					t.Fatalf("pixel (%d,%d) = %+v, want %+v", col, row, got, want)
				}
			}
		}
	})

	t.Run("DrawIndexedInstanced_10000_SingleCall_And_FirstInstance", func(t *testing.T) {
		sr := gpu.NewSoftwareRenderer(100, 100, gpu.FormatUndefined)
		dev := sr.Device()

		quadVB, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: quadVerts4})
		if err != nil {
			t.Fatalf("CreateBuffer quad: %v", err)
		}
		ib, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageIndex, Data: quadIdx})
		if err != nil {
			t.Fatalf("CreateBuffer indices: %v", err)
		}
		instVB, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: instData})
		if err != nil {
			t.Fatalf("CreateBuffer instances: %v", err)
		}
		pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
			Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
			VertexBuffers: []gpu.VertexBufferLayout{
				{
					Stride:     2 * 4,
					StepMode:   gpu.VertexStepVertex,
					Attributes: []gpu.VertexAttribute{{Location: 0, Offset: 0, Format: gpu.VertexFloat32x2}},
				},
				{
					Stride:   6 * 4,
					StepMode: gpu.VertexStepInstance,
					Attributes: []gpu.VertexAttribute{
						{Location: 1, Offset: 0, Format: gpu.VertexFloat32x2},
						{Location: 2, Offset: 2 * 4, Format: gpu.VertexFloat32x4},
					},
				},
			},
			Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline: %v", err)
		}

		// 1. All 10,000 instances in ONE DrawIndexedInstanced call.
		f0 := dev.BeginFrame(time.Now())
		p0 := f0.BeginRenderPass(gpu.RenderPassDesc{})
		p0.ClearColor(core.RGB(0x000000))
		p0.SetPipeline(pipe)
		p0.SetVertexBuffer(0, quadVB, 0)
		p0.SetVertexBuffer(1, instVB, 0)
		p0.SetIndexBuffer(ib, gpu.IndexUint16, 0)
		p0.DrawIndexedInstanced(6, 10000, 0, 0, 0)
		p0.End()
		f0.Submit()
		if err := sr.LastError(); err != nil {
			t.Fatalf("LastError: %v", err)
		}
		img := sr.Image()
		for row := 0; row < 100; row++ {
			for col := 0; col < 100; col++ {
				got := img.RGBAAt(col, row)
				want := expected100x100Color(col, row)
				if !closeByte(got.R, want.R) || !closeByte(got.G, want.G) || !closeByte(got.B, want.B) || got.A != 255 {
					t.Fatalf("indexed 10k pixel (%d,%d) = %+v, want %+v", col, row, got, want)
				}
			}
		}

		// 2. Subset draw with firstInstance = 5000, instanceCount = 2500 (rows 50..74).
		f1 := dev.BeginFrame(time.Now())
		p1 := f1.BeginRenderPass(gpu.RenderPassDesc{})
		p1.ClearColor(core.RGB(0x050607))
		p1.SetPipeline(pipe)
		p1.SetVertexBuffer(0, quadVB, 0)
		p1.SetVertexBuffer(1, instVB, 0)
		p1.SetIndexBuffer(ib, gpu.IndexUint16, 0)
		p1.DrawIndexedInstanced(6, 2500, 0, 0, 5000)
		p1.End()
		f1.Submit()
		if err := sr.LastError(); err != nil {
			t.Fatalf("LastError firstInstance: %v", err)
		}
		img2 := sr.Image()
		bg := color.RGBA{R: 5, G: 6, B: 7, A: 255}
		if got := img2.RGBAAt(10, 10); got != bg {
			t.Errorf("untouched pixel (10,10) = %+v, want clear bg %+v", got, bg)
		}
		if got := img2.RGBAAt(50, 85); got != bg {
			t.Errorf("untouched pixel (50,85) = %+v, want clear bg %+v", got, bg)
		}
		for row := 50; row < 75; row++ {
			for col := 0; col < 100; col++ {
				got := img2.RGBAAt(col, row)
				want := expected100x100Color(col, row)
				if !closeByte(got.R, want.R) || !closeByte(got.G, want.G) || !closeByte(got.B, want.B) || got.A != 255 {
					t.Fatalf("firstInstance pixel (%d,%d) = %+v, want %+v", col, row, got, want)
				}
			}
		}
	})

	t.Run("StepRate_And_PipelineValidation", func(t *testing.T) {
		sr := gpu.NewSoftwareRenderer(160, 16, gpu.FormatUndefined)
		dev := sr.Device()
		srMod := gpu.MustCompileWGSL(instancedStepRateWGSL)

		// StepRate > 1 on VertexStepVertex must be rejected synchronously.
		if _, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:   gpu.ShaderStage{Module: srMod, EntryPoint: "vs_main"},
			Fragment: gpu.ShaderStage{Module: srMod, EntryPoint: "fs_main"},
			VertexBuffers: []gpu.VertexBufferLayout{
				{Stride: 8, StepMode: gpu.VertexStepVertex, StepRate: 2, Attributes: []gpu.VertexAttribute{{Location: 0, Format: gpu.VertexFloat32x2}}},
			},
			Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		}); !errors.Is(err, gpu.ErrInvalidDescriptor) {
			t.Fatalf("expected ErrInvalidDescriptor for StepRate>1 on VertexStepVertex, got %v", err)
		}

		// Valid StepRate = 2: 16 instances advance through 8 per-instance colors (2 columns per color).
		colHalf := float32(0.055)
		colQuad := packFloat32s(
			-colHalf, -0.8,
			colHalf, -0.8,
			colHalf, 0.8,
			-colHalf, -0.8,
			colHalf, 0.8,
			-colHalf, 0.8,
		)
		palette := [8][4]float32{
			{1, 0, 0, 1},
			{0, 1, 0, 1},
			{0, 0, 1, 1},
			{1, 1, 0, 1},
			{1, 0, 1, 1},
			{0, 1, 1, 1},
			{1, 0.5, 0, 1},
			{0.5, 0, 1, 1},
		}
		instBytes := make([]byte, len(palette)*16)
		for i, c := range palette {
			writeF32sInto(instBytes[i*16:], c[0], c[1], c[2], c[3])
		}
		vb0, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: colQuad})
		vb1, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: instBytes})
		pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:   gpu.ShaderStage{Module: srMod, EntryPoint: "vs_main"},
			Fragment: gpu.ShaderStage{Module: srMod, EntryPoint: "fs_main"},
			VertexBuffers: []gpu.VertexBufferLayout{
				{Stride: 8, StepMode: gpu.VertexStepVertex, Attributes: []gpu.VertexAttribute{{Location: 0, Format: gpu.VertexFloat32x2}}},
				{Stride: 16, StepMode: gpu.VertexStepInstance, StepRate: 2, Attributes: []gpu.VertexAttribute{{Location: 1, Format: gpu.VertexFloat32x4}}},
			},
			Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline StepRate=2: %v", err)
		}

		f := dev.BeginFrame(time.Now())
		p := f.BeginRenderPass(gpu.RenderPassDesc{})
		p.ClearColor(core.RGB(0x000000))
		p.SetPipeline(pipe)
		p.SetVertexBuffer(0, vb0, 0)
		p.SetVertexBuffer(1, vb1, 0)
		p.DrawInstanced(6, 16, 0, 0)
		p.End()
		f.Submit()

		img := sr.Image()
		for col := 0; col < 16; col++ {
			c := palette[col/2]
			want := color.RGBA{
				R: byte(math.Round(float64(c[0] * 255))),
				G: byte(math.Round(float64(c[1] * 255))),
				B: byte(math.Round(float64(c[2] * 255))),
				A: 255,
			}
			got := img.RGBAAt(col*10+5, 8)
			if got != want {
				t.Errorf("StepRate=2 col %d: got %+v, want %+v", col, got, want)
			}
		}
	})
}

func TestFramesInFlightClamping(t *testing.T) {
	cases := []struct {
		req, want int
	}{
		{-2, 2},
		{0, 2},
		{1, 2},
		{2, 2},
		{3, 3},
		{4, 3},
		{16, 3},
	}
	for _, tc := range cases {
		sr := gpu.NewSoftwareRendererWithFrames(16, 16, gpu.FormatUndefined, tc.req)
		if got := sr.Device().FramesInFlight(); got != tc.want {
			t.Errorf("NewSoftwareRendererWithFrames(%d).FramesInFlight() = %d, want %d", tc.req, got, tc.want)
		}

		var viewFIF int
		nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return gpu.View(cx, gpu.ViewProps{
				TestID:         "gpu",
				Style:          core.Style{Width: core.Pt(16), Height: core.Pt(16)},
				FramesInFlight: tc.req,
				OnReady: func(dev *gpu.Device) {
					viewFIF = dev.FramesInFlight()
				},
			})
		}, nsgotest.Size(16, 16))
		if viewFIF != tc.want {
			t.Errorf("ViewProps{FramesInFlight: %d} -> Device.FramesInFlight() = %d, want %d", tc.req, viewFIF, tc.want)
		}
	}
}

func TestRingBuffer10000ObjectsAndDynamicOffsets(t *testing.T) {
	mod := gpu.MustCompileWGSL(dynamicUniformObjWGSL)

	for _, framesInFlight := range []int{2, 3} {
		t.Run(fmt.Sprintf("FramesInFlight_%d", framesInFlight), func(t *testing.T) {
			sr := gpu.NewSoftwareRendererWithFrames(64, 64, gpu.FormatUndefined, framesInFlight)
			dev := sr.Device()
			if dev.FramesInFlight() != framesInFlight {
				t.Fatalf("FramesInFlight = %d, want %d", dev.FramesInFlight(), framesInFlight)
			}

			const objCount = 10000
			const align = uint64(256)
			const slotSize = objCount * align

			uRing, err := dev.CreateRingBuffer(nil, gpu.RingBufferDesc{
				Label:    "object_uniforms",
				SlotSize: slotSize,
				Usage:    gpu.UsageUniform,
			})
			if err != nil {
				t.Fatalf("CreateRingBuffer uniform: %v", err)
			}
			if uRing.Alignment() != align {
				t.Fatalf("uRing.Alignment() = %d, want %d", uRing.Alignment(), align)
			}
			if uRing.SlotSize() != slotSize {
				t.Fatalf("uRing.SlotSize() = %d, want %d", uRing.SlotSize(), slotSize)
			}
			if uRing.TotalSize() != slotSize*uint64(framesInFlight) {
				t.Fatalf("uRing.TotalSize() = %d, want %d", uRing.TotalSize(), slotSize*uint64(framesInFlight))
			}

			// Streaming vertex ring buffer as well.
			vRing, err := dev.CreateRingBuffer(nil, gpu.RingBufferDesc{
				Label:    "quad_vertices",
				SlotSize: 1024,
				Usage:    gpu.UsageVertex,
			})
			if err != nil {
				t.Fatalf("CreateRingBuffer vertex: %v", err)
			}
			if vRing.Alignment() != 16 {
				t.Fatalf("vRing.Alignment() = %d, want 16", vRing.Alignment())
			}

			bgl, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform, HasDynamicOffset: true},
			})
			if err != nil {
				t.Fatalf("CreateBindGroupLayout: %v", err)
			}
			bg, err := dev.CreateBindGroup(nil, bgl, []gpu.BindGroupEntry{
				{Binding: 0, Buffer: uRing.Buffer(), Offset: 0, Size: 32},
			})
			if err != nil {
				t.Fatalf("CreateBindGroup: %v", err)
			}
			pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
				Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
				Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
				Layouts:  []*gpu.BindGroupLayout{bgl},
				VertexBuffers: []gpu.VertexBufferLayout{
					{Stride: 8, StepMode: gpu.VertexStepVertex, Attributes: []gpu.VertexAttribute{{Location: 0, Format: gpu.VertexFloat32x2}}},
				},
				Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
			})
			if err != nil {
				t.Fatalf("CreatePipeline: %v", err)
			}

			// Sample 4 quadrant objects across the 10,000-object ring: 0, 3333, 6666, 9999.
			sampleIndices := [4]int{0, 3333, 6666, 9999}
			sampleCenters := [4][2]float32{
				{-0.5, 0.5},  // top-left  (px 16, 16)
				{0.5, 0.5},   // top-right (px 48, 16)
				{-0.5, -0.5}, // bot-left  (px 16, 48)
				{0.5, -0.5},  // bot-right (px 48, 48)
			}
			samplePixels := [4][2]int{{16, 16}, {48, 16}, {16, 48}, {48, 48}}
			quadVerts := packFloat32s(-0.4, -0.4, 0.4, -0.4, 0.4, 0.4, -0.4, -0.4, 0.4, 0.4, -0.4, 0.4)

			totalFrames := framesInFlight + 2
			offsets := make([]uint32, objCount)
			for frameNum := 0; frameNum < totalFrames; frameNum++ {
				wantSlot := frameNum % framesInFlight
				f := dev.BeginFrame(time.Unix(0, int64(frameNum+1)*16_000_000))
				if f.FrameIndex() != uint64(frameNum) || f.FrameSlot() != wantSlot {
					t.Fatalf("frame %d: got (idx=%d, slot=%d), want (%d, %d)", frameNum, f.FrameIndex(), f.FrameSlot(), frameNum, wantSlot)
				}
				if uRing.Used() != 0 || uRing.Remaining() != slotSize {
					t.Fatalf("frame %d: ring cursor did not reset: used=%d rem=%d", frameNum, uRing.Used(), uRing.Remaining())
				}

				vSlice, _, err := vRing.Write(quadVerts)
				if err != nil {
					t.Fatalf("vRing.Write frame %d: %v", frameNum, err)
				}
				if vSlice.Slot != wantSlot {
					t.Fatalf("vSlice.Slot = %d, want %d", vSlice.Slot, wantSlot)
				}

				// Update ALL 10,000 objects in this frame's ring slot.
				slotBase := uint64(wantSlot) * slotSize
				for i := 0; i < objCount; i++ {
					sl, off, err := uRing.Alloc(32)
					if err != nil {
						t.Fatalf("frame %d obj %d Alloc: %v", frameNum, i, err)
					}
					if off%256 != 0 {
						t.Fatalf("frame %d obj %d dynamic offset %d not 256-byte aligned", frameNum, i, off)
					}
					if sl.Slot != wantSlot {
						t.Fatalf("frame %d obj %d slot = %d, want %d", frameNum, i, sl.Slot, wantSlot)
					}
					if sl.Offset < slotBase || sl.Offset+32 > slotBase+slotSize {
						t.Fatalf("frame %d obj %d offset %d outside slot range [%d, %d)", frameNum, i, sl.Offset, slotBase, slotBase+slotSize)
					}
					// Default off-screen unless one of our 4 sample quadrants.
					ox, oy := float32(10), float32(10)
					r := float32((frameNum*40+20)%255) / 255.0
					g := float32((i%200)+20) / 255.0
					b := float32(((frameNum+1)*50)%255) / 255.0
					for q, si := range sampleIndices {
						if i == si {
							ox, oy = sampleCenters[q][0], sampleCenters[q][1]
							r = float32((frameNum*50+q*40+20)%255) / 255.0
							g = float32((q*60+30)%255) / 255.0
							b = float32(((frameNum+1)*45+q*20)%255) / 255.0
							break
						}
					}
					writeF32sInto(sl.Data, ox, oy, 0, 0, r, g, b, 1)
					offsets[i] = off
				}
				if uRing.Remaining() != 0 {
					t.Fatalf("frame %d: Remaining = %d, want 0", frameNum, uRing.Remaining())
				}
				if _, _, err := uRing.Alloc(16); !errors.Is(err, gpu.ErrOutOfMemory) {
					t.Fatalf("frame %d: expected ErrOutOfMemory on overflow, got %v", frameNum, err)
				}

				pass := f.BeginRenderPass(gpu.RenderPassDesc{})
				pass.ClearColor(core.RGB(0x000000))
				pass.SetPipeline(pipe)
				pass.SetVertexBuffer(0, vSlice.Buffer, vSlice.Offset)
				for _, si := range sampleIndices {
					pass.SetBindGroup(0, bg, offsets[si])
					pass.Draw(6, 0)
				}
				pass.End()
				dev.EndFrame(f)
				if err := sr.LastError(); err != nil {
					t.Fatalf("frame %d LastError: %v", frameNum, err)
				}

				img := sr.Image()
				for q := range sampleIndices {
					px := samplePixels[q]
					got := img.RGBAAt(px[0], px[1])
					wantR := byte((frameNum*50 + q*40 + 20) % 255)
					wantG := byte((q*60 + 30) % 255)
					wantB := byte(((frameNum+1)*45 + q*20) % 255)
					if !closeByte(got.R, wantR) || !closeByte(got.G, wantG) || !closeByte(got.B, wantB) || got.A != 255 {
						t.Errorf("frame %d quadrant %d pixel (%d,%d) = %+v, want (%d,%d,%d,255)", frameNum, q, px[0], px[1], got, wantR, wantG, wantB)
					}
				}
			}
		})
	}
}

func TestPipelinedRingBufferZeroCorruption(t *testing.T) {
	mod := gpu.MustCompileWGSL(dynamicUniformObjWGSL)

	for _, framesInFlight := range []int{2, 3} {
		t.Run(fmt.Sprintf("FramesInFlight_%d", framesInFlight), func(t *testing.T) {
			sr := gpu.NewSoftwareRendererWithFrames(64, 64, gpu.FormatUndefined, framesInFlight)
			defer sr.Close()
			sr.SetPipelined(true)
			dev := sr.Device()

			const objCount = 10000
			const align = uint64(256)
			const slotSize = objCount * align

			uRing, err := dev.CreateRingBuffer(nil, gpu.RingBufferDesc{
				Label:    "pipelined_uniforms",
				SlotSize: slotSize,
				Usage:    gpu.UsageUniform,
			})
			if err != nil {
				t.Fatalf("CreateRingBuffer: %v", err)
			}
			quadVerts := packFloat32s(-0.4, -0.4, 0.4, -0.4, 0.4, 0.4, -0.4, -0.4, 0.4, 0.4, -0.4, 0.4)
			vbuf, err := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: quadVerts})
			if err != nil {
				t.Fatalf("CreateBuffer quad: %v", err)
			}
			bgl, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform, HasDynamicOffset: true},
			})
			if err != nil {
				t.Fatalf("CreateBindGroupLayout: %v", err)
			}
			bg, err := dev.CreateBindGroup(nil, bgl, []gpu.BindGroupEntry{
				{Binding: 0, Buffer: uRing.Buffer(), Offset: 0, Size: 32},
			})
			if err != nil {
				t.Fatalf("CreateBindGroup: %v", err)
			}
			pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
				Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
				Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
				Layouts:  []*gpu.BindGroupLayout{bgl},
				VertexBuffers: []gpu.VertexBufferLayout{
					{Stride: 8, StepMode: gpu.VertexStepVertex, Attributes: []gpu.VertexAttribute{{Location: 0, Format: gpu.VertexFloat32x2}}},
				},
				Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
			})
			if err != nil {
				t.Fatalf("CreatePipeline: %v", err)
			}

			// Distinct frame signature colors across frames 0..framesInFlight.
			frameColors := [4][3]byte{
				{230, 20, 20},  // Frame 0 (slot 0): red
				{20, 230, 20},  // Frame 1 (slot 1): green
				{20, 20, 230},  // Frame 2 (slot 2 if 3-in-flight, or recycled slot 0 if 2-in-flight): blue
				{230, 220, 20}, // Frame 3 (recycled slot 0 for 3-in-flight): yellow
			}
			sampleIndices := [4]int{0, 3333, 6666, 9999}
			sampleCenters := [4][2]float32{
				{-0.5, 0.5},
				{0.5, 0.5},
				{-0.5, -0.5},
				{0.5, -0.5},
			}
			samplePixels := [4][2]int{{16, 16}, {48, 16}, {16, 48}, {48, 48}}

			frame0InPass := make(chan struct{})
			releaseFrame0 := make(chan struct{})
			var (
				capMu    sync.Mutex
				captured = map[uint64]*image.RGBA{}
			)
			sr.SetFrameHooks(
				func(frameIdx uint64, frameSlot int) {
					if frameIdx == 0 {
						close(frame0InPass)
						<-releaseFrame0
					}
				},
				func(frameIdx uint64, frameSlot int) {
					img := sr.Image()
					capMu.Lock()
					captured[frameIdx] = img
					capMu.Unlock()
				},
			)

			recordFrame := func(f *gpu.Frame, frameNum int) {
				sig := frameColors[frameNum]
				rf, gf, bf := float32(sig[0])/255.0, float32(sig[1])/255.0, float32(sig[2])/255.0
				var sampleOffs [4]uint32
				for i := 0; i < objCount; i++ {
					sl, off, err := uRing.Alloc(32)
					if err != nil {
						t.Fatalf("frame %d obj %d Alloc: %v", frameNum, i, err)
					}
					ox, oy := float32(10), float32(10)
					for q, si := range sampleIndices {
						if i == si {
							ox, oy = sampleCenters[q][0], sampleCenters[q][1]
							sampleOffs[q] = off
							break
						}
					}
					writeF32sInto(sl.Data, ox, oy, 0, 0, rf, gf, bf, 1)
				}
				pass := f.BeginRenderPass(gpu.RenderPassDesc{})
				pass.ClearColor(core.RGB(0x000000))
				pass.SetPipeline(pipe)
				pass.SetVertexBuffer(0, vbuf, 0)
				for _, off := range sampleOffs {
					pass.SetBindGroup(0, bg, off)
					pass.Draw(6, 0)
				}
				pass.End()
				f.Submit()
			}

			// 1. Submit Frame 0 (slot 0) and wait until GPU worker holds Frame 0 mid-pass.
			f0 := dev.BeginFrame(time.Unix(0, 1))
			if f0.FrameSlot() != 0 {
				t.Fatalf("f0.FrameSlot() = %d, want 0", f0.FrameSlot())
			}
			recordFrame(f0, 0)

			select {
			case <-frame0InPass:
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for GPU worker to enter Frame 0 pass")
			}

			// 2. While GPU worker is STILL paused mid-Frame-0 (holding slot 0),
			//    CPU writes ALL remaining in-flight slots (slot 1, and slot 2 for triple-buffering)
			//    with 10,000 objects each — ZERO stalls and ZERO corruption of slot 0!
			fences := []*gpu.FrameFence{f0.Fence()}
			for k := 1; k < framesInFlight; k++ {
				fk := dev.BeginFrame(time.Unix(0, int64(k+1)))
				if fk.FrameSlot() != k {
					t.Fatalf("frame %d slot = %d, want %d", k, fk.FrameSlot(), k)
				}
				recordFrame(fk, k)
				fences = append(fences, fk.Fence())
			}
			if f0.Fence().Signaled() {
				t.Fatal("frame 0 fence signaled before releaseFrame0 was closed")
			}

			// 3. Starting Frame `framesInFlight` wraps back to slot 0 and MUST block
			//    on Frame 0's FrameFence until Frame 0 completes!
			wrapReady := make(chan *gpu.Frame, 1)
			go func() {
				wrapReady <- dev.BeginFrame(time.Unix(0, int64(framesInFlight+1)))
			}()
			select {
			case <-wrapReady:
				t.Fatalf("BeginFrame for recycled slot 0 returned while Frame 0 was still in flight!")
			case <-time.After(30 * time.Millisecond):
				// Expected: blocked safely on Frame 0's FrameFence!
			}

			// 4. Release GPU worker -> Frame 0 finishes reading slot 0 -> fence 0 signals
			//    -> recycled slot 0 unblocks and records Frame `framesInFlight`!
			close(releaseFrame0)
			var fWrap *gpu.Frame
			select {
			case fWrap = <-wrapReady:
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for recycled slot 0 BeginFrame to unblock after Frame 0 completed")
			}
			if fWrap.FrameSlot() != 0 {
				t.Fatalf("wrapped frame slot = %d, want 0", fWrap.FrameSlot())
			}
			recordFrame(fWrap, framesInFlight)
			fences = append(fences, fWrap.Fence())

			// 5. Wait for all frames to complete and assert ZERO corruption in every frame!
			dev.WaitIdle()
			sr.WaitIdle()
			if err := sr.LastError(); err != nil {
				t.Fatalf("LastError: %v", err)
			}
			for i, f := range fences {
				if !f.Signaled() {
					t.Errorf("fence %d not signaled after WaitIdle", i)
				}
			}

			capMu.Lock()
			defer capMu.Unlock()
			for k := 0; k <= framesInFlight; k++ {
				img := captured[uint64(k)]
				if img == nil {
					t.Fatalf("missing captured image for frame %d", k)
				}
				want := frameColors[k]
				for q, px := range samplePixels {
					got := img.RGBAAt(px[0], px[1])
					if !closeByte(got.R, want[0]) || !closeByte(got.G, want[1]) || !closeByte(got.B, want[2]) || got.A != 255 {
						t.Errorf("frame %d (slot %d) quadrant %d pixel (%d,%d) = %+v, want (%d,%d,%d,255) — write-while-read corruption detected!",
							k, k%framesInFlight, q, px[0], px[1], got, want[0], want[1], want[2])
					}
				}
			}
		})
	}
}
