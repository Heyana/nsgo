//go:build darwin

package apple

import (
	"context"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
)

const gpuInstancedWGSL = `
struct VertexIn {
	@location(0) pos: vec2<f32>,
	@location(1) offset: vec2<f32>,
	@location(2) color: vec4<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = vec4<f32>(in.pos + in.offset, 0.5, 1.0);
	out.color = in.color;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	return in.color;
}
`

// TestMetalFramesCompleteAndDrawInstanced submits more frames than gpu.MaxFramesInFlight and checks that Metal runs every frame's OnComplete, which gpu.Device's FrameFences wait on, including frames with no GPU work and frames for another device. Its frames draw four instances whose offsets advance per instance and whose colors advance every second instance (StepRate 2), starting at the second instance (firstInstance), into a target read back pixel by pixel. It runs in a child process with Metal's API validation.
func TestMetalFramesCompleteAndDrawInstanced(t *testing.T) {
	if os.Getenv("MTL_DEBUG_LAYER") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "MTL_DEBUG_LAYER=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	prog, err := wgsl.CompilePipeline(gpuInstancedWGSL)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan *driver.GPUEvent, 64)
	var layer *GPULayer
	withPool(func() {
		layer = NewGPULayer(driver.GPUViewConfig{SampleCount: 1, FramesInFlight: 3}, func(ev *driver.GPUEvent) { events <- ev })
		if layer != nil {
			layer.Resize(8, 8, 1)
		}
	})
	if layer == nil {
		t.Skip("Metal unavailable")
	}
	t.Cleanup(func() { withPool(layer.Release) })
	next := func() *driver.GPUEvent {
		t.Helper()
		for {
			select {
			case ev := <-events:
				if ev.Kind == driver.GPUEventError {
					t.Fatalf("GPU error %d: %s", ev.ErrorCode, ev.Message)
				}
				return ev
			case <-time.After(15 * time.Second):
				t.Fatal("no GPU event")
			}
		}
	}
	ready := next()
	if ready.Kind != driver.GPUEventDeviceReady {
		t.Fatalf("first event = %+v, want the device", ready)
	}
	gen := ready.Generation

	const (
		quad driver.GPUID = iota + 1
		offsets
		colors
		pipe
		target
	)
	// A quad covering a quarter of the target's width, at its left edge; each instance moves it right by a quarter.
	quadVerts := gpuFloats(-1, -1, -.5, -1, -.5, 1, -1, -1, -.5, 1, -1, 1)
	offsetData := gpuFloats(0, 0, .5, 0, 1, 0, 1.5, 0, 2, 0)
	// Colors advance every second instance: instances 1 and 2 take green, 3 and 4 blue.
	colorData := gpuFloats(1, 0, 0, 1, 0, 1, 0, 1, 0, 0, 1, 1)
	rgba := driver.GPUFormatRGBA8Unorm
	layer.Submit(driver.GPUFrame{Generation: gen, Ops: []driver.GPUResourceOp{
		{Kind: driver.GPUOpCreateBuffer, ID: quad, Buffer: &driver.GPUBufferDesc{Usage: driver.GPUBufferVertex, Data: quadVerts}},
		{Kind: driver.GPUOpCreateBuffer, ID: offsets, Buffer: &driver.GPUBufferDesc{Usage: driver.GPUBufferVertex, Data: offsetData}},
		{Kind: driver.GPUOpCreateBuffer, ID: colors, Buffer: &driver.GPUBufferDesc{Usage: driver.GPUBufferVertex, Data: colorData}},
		{Kind: driver.GPUOpCreatePipeline, ID: pipe, Pipeline: &driver.GPUPipelineDesc{
			Label:    "instanced",
			Vertex:   driver.GPUShaderStage{IR: prog.IR, EntryPoint: "vs_main"},
			Fragment: driver.GPUShaderStage{IR: prog.IR, EntryPoint: "fs_main"},
			VertexBuffers: []driver.GPUVertexBufferLayout{
				{Stride: 8, Attributes: []driver.GPUVertexAttribute{{Format: driver.GPUVertexFloat32x2}}},
				{Stride: 8, StepMode: driver.GPUStepInstance, Attributes: []driver.GPUVertexAttribute{{Format: driver.GPUVertexFloat32x2, Location: 1}}},
				{Stride: 16, StepMode: driver.GPUStepInstance, StepRate: 2, Attributes: []driver.GPUVertexAttribute{{Format: driver.GPUVertexFloat32x4, Location: 2}}},
			},
			Targets:     []driver.GPUColorTargetState{{Format: rgba, WriteMask: driver.GPUColorWriteAll}},
			SampleCount: 1,
		}},
		{Kind: driver.GPUOpCreateTexture, ID: target, Texture: &driver.GPUTextureDesc{Width: 8, Height: 8, Layers: 1, MipLevels: 1, Format: rgba, Usage: driver.GPUTextureRenderAttachment | driver.GPUTextureCopySrc}},
	}})

	var completed atomic.Int32
	done := func() { completed.Add(1) }
	draw := driver.GPUPass{
		ColorAttachments: []driver.GPUColorAttachment{{Texture: target, LoadOp: driver.GPULoadClear, StoreOp: driver.GPUStoreStore}},
		Commands: []driver.GPUCommand{
			{Kind: driver.GPUCmdSetPipeline, ID: pipe},
			{Kind: driver.GPUCmdSetVertexBuffer, Slot: 0, ID: quad},
			{Kind: driver.GPUCmdSetVertexBuffer, Slot: 1, ID: offsets},
			{Kind: driver.GPUCmdSetVertexBuffer, Slot: 2, ID: colors},
			{Kind: driver.GPUCmdSetViewport, F2: 8, F3: 8, F5: 1},
			// Six vertices, three instances from the second: the quad's slots 1, 2 and 3 of 4.
			{Kind: driver.GPUCmdDrawInstanced, U0: 6, U1: 3, U3: 1},
		},
	}
	const frames = 8 // more than gpu.MaxFramesInFlight (3)
	want := int32(0)
	for i := range frames {
		layer.Submit(driver.GPUFrame{Generation: gen, FrameIndex: uint64(i), FrameSlot: i % 3, Passes: []driver.GPUPass{draw}, OnComplete: done})
		want++
	}
	// A frame with no GPU work and one for another device complete too.
	layer.Submit(driver.GPUFrame{Generation: gen, OnComplete: done})
	layer.Submit(driver.GPUFrame{Generation: gen + 1000, Passes: []driver.GPUPass{draw}, OnComplete: done})
	want += 2
	deadline := time.Now().Add(10 * time.Second)
	for completed.Load() < want && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := completed.Load(); got != want {
		t.Fatalf("%d of %d frames completed", got, want)
	}

	read := func(id uint64, x uint32) []byte {
		t.Helper()
		layer.Submit(driver.GPUFrame{Generation: gen, Ops: []driver.GPUResourceOp{{Kind: driver.GPUOpReadTexture, ReadbackID: id, ID: target,
			ReadbackRegion: driver.GPUTextureRegion{X: x, Y: 4, Width: 1, Height: 1}}}})
		ev := next()
		if ev.Kind != driver.GPUEventReadback || ev.ReadbackID != id || ev.Message != "" {
			t.Fatalf("readback %d = %+v", id, ev)
		}
		return ev.Pixels
	}
	// Columns 0-1 are instance 0's, which firstInstance skips; 2-3 instance 1's (green); 4-5 instance 2's (green, as the color steps every second instance); 6-7 instance 3's (blue).
	for i, c := range []struct {
		x    uint32
		want [4]byte
	}{{1, [4]byte{0, 0, 0, 0}}, {3, [4]byte{0, 255, 0, 255}}, {5, [4]byte{0, 255, 0, 255}}, {7, [4]byte{0, 0, 255, 255}}} {
		if got := read(uint64(i+1), c.x); [4]byte(got) != c.want {
			t.Errorf("column %d = %v, want %v", c.x, got, c.want)
		}
	}
}
