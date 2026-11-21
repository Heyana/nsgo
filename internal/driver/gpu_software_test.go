package driver

import (
	"encoding/binary"
	"go/constant"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

func packF32s(vals ...float32) []byte {
	out := make([]byte, len(vals)*4)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(v))
	}
	return out
}

func TestNormalizeFramesInFlight(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{-10, GPUDefaultFramesInFlight},
		{0, GPUDefaultFramesInFlight},
		{1, GPUMinFramesInFlight},
		{2, 2},
		{3, 3},
		{4, GPUMaxFramesInFlight},
		{99, GPUMaxFramesInFlight},
	}
	for _, tc := range cases {
		if got := NormalizeFramesInFlight(tc.in); got != tc.want {
			t.Errorf("NormalizeFramesInFlight(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// buildInstancedColorProgram builds a minimal shaderir.Program:
//   - attr 0 (@location(0)): vec2 pos
//   - attr 1 (@location(1)): vec4 instColor (per-instance)
//   - vertex output pos = vec4(pos.x + (f32(instance_index) - 3.5) * 0.25, pos.y, 0.5, 1.0)
//   - varying 0: vec4 color = instColor
//   - fragment output: varying 0
func buildInstancedColorProgram() *shaderir.Program {
	vec2T := shaderir.Type{Main: shaderir.Vec2}
	vec4T := shaderir.Type{Main: shaderir.Vec4}
	floatT := shaderir.Type{Main: shaderir.Float}
	uintT := shaderir.Type{Main: shaderir.Uint}

	// Vertex locals: 0=attr0(pos), 1=attr1(instColor), 2=out.position, 3=out.v0(color)
	instFloat := shaderir.Expr{
		Type: shaderir.Call,
		Exprs: []shaderir.Expr{
			{Type: shaderir.BuiltinFuncExpr, BuiltinFunc: shaderir.FloatF},
			{Type: shaderir.BuiltinInput, Index: int(shaderir.BuiltinInstanceIndex)},
		},
	}
	// colOffset = (f32(instIdx) - 3.5) * 0.25 -> places 8 columns across [-1, 1]
	colOffset := shaderir.Expr{
		Type: shaderir.Binary,
		Op:   shaderir.ComponentWiseMul,
		Exprs: []shaderir.Expr{
			{
				Type: shaderir.Binary,
				Op:   shaderir.Sub,
				Exprs: []shaderir.Expr{
					instFloat,
					{Type: shaderir.NumberExpr, Const: constant.MakeFloat64(3.5)},
				},
			},
			{Type: shaderir.NumberExpr, Const: constant.MakeFloat64(0.25)},
		},
	}
	posX := shaderir.Expr{
		Type: shaderir.Binary,
		Op:   shaderir.Add,
		Exprs: []shaderir.Expr{
			{
				Type: shaderir.FieldSelector,
				Exprs: []shaderir.Expr{
					{Type: shaderir.LocalVariable, Index: 0},
					{Type: shaderir.SwizzlingExpr, Swizzling: "x"},
				},
			},
			colOffset,
		},
	}
	posY := shaderir.Expr{
		Type: shaderir.FieldSelector,
		Exprs: []shaderir.Expr{
			{Type: shaderir.LocalVariable, Index: 0},
			{Type: shaderir.SwizzlingExpr, Swizzling: "y"},
		},
	}
	outPos := shaderir.Expr{
		Type: shaderir.Call,
		Exprs: []shaderir.Expr{
			{Type: shaderir.BuiltinFuncExpr, BuiltinFunc: shaderir.Vec4F},
			posX,
			posY,
			{Type: shaderir.NumberExpr, Const: constant.MakeFloat64(0.5)},
			{Type: shaderir.NumberExpr, Const: constant.MakeFloat64(1.0)},
		},
	}
	_ = floatT
	_ = uintT
	return &shaderir.Program{
		VertexEntry:   "vs_main",
		FragmentEntry: "fs_main",
		Attributes:    []shaderir.Type{vec2T, vec4T},
		AttributeMeta: []shaderir.StageIO{
			{Name: "pos", Location: 0},
			{Name: "color", Location: 1},
		},
		Varyings:    []shaderir.Type{vec4T},
		VaryingMeta: []shaderir.StageIO{{Name: "color", Location: 0}},
		VertexFunc: shaderir.VertexFunc{
			Block: &shaderir.Block{
				LocalVarIndexOffset: 4,
				Stmts: []shaderir.Stmt{
					{
						Type:  shaderir.Assign,
						Exprs: []shaderir.Expr{{Type: shaderir.LocalVariable, Index: 2}, outPos},
					},
					{
						Type:  shaderir.Assign,
						Exprs: []shaderir.Expr{{Type: shaderir.LocalVariable, Index: 3}, {Type: shaderir.LocalVariable, Index: 1}},
					},
					{Type: shaderir.Return},
				},
			},
		},
		FragmentFunc: shaderir.FragmentFunc{
			Block: &shaderir.Block{
				LocalVarIndexOffset: 2,
				Stmts: []shaderir.Stmt{
					{Type: shaderir.Return, Exprs: []shaderir.Expr{{Type: shaderir.LocalVariable, Index: 1}}},
				},
			},
		},
	}
}

// buildUniformColorProgram builds a minimal shaderir.Program reading a vec4
// color from @group(0) @binding(0) uniform struct { color: vec4 }.
func buildUniformColorProgram() *shaderir.Program {
	vec2T := shaderir.Type{Main: shaderir.Vec2}
	vec4T := shaderir.Type{Main: shaderir.Vec4}
	uStruct := shaderir.Type{Main: shaderir.Struct, Sub: []shaderir.Type{vec4T}}
	// Vertex locals: 0=attr0(pos), 1=out.position, 2=out.v0(color)
	outPos := shaderir.Expr{
		Type: shaderir.Call,
		Exprs: []shaderir.Expr{
			{Type: shaderir.BuiltinFuncExpr, BuiltinFunc: shaderir.Vec4F},
			{Type: shaderir.LocalVariable, Index: 0},
			{Type: shaderir.NumberExpr, Const: constant.MakeFloat64(0.5)},
			{Type: shaderir.NumberExpr, Const: constant.MakeFloat64(1.0)},
		},
	}
	uColor := shaderir.Expr{
		Type: shaderir.FieldSelector,
		Exprs: []shaderir.Expr{
			{Type: shaderir.BindingVariable, Index: 0},
			{Type: shaderir.StructMember, Index: 0},
		},
	}
	return &shaderir.Program{
		VertexEntry:   "vs_main",
		FragmentEntry: "fs_main",
		Attributes:    []shaderir.Type{vec2T},
		AttributeMeta: []shaderir.StageIO{{Name: "pos", Location: 0}},
		Varyings:      []shaderir.Type{vec4T},
		VaryingMeta:   []shaderir.StageIO{{Name: "color", Location: 0}},
		Bindings: []shaderir.Binding{{
			Group:   0,
			Binding: 0,
			Name:    "u",
			Kind:    shaderir.BindingUniformBuffer,
			Type:    uStruct,
		}},
		VertexFunc: shaderir.VertexFunc{
			Block: &shaderir.Block{
				LocalVarIndexOffset: 3,
				Stmts: []shaderir.Stmt{
					{Type: shaderir.Assign, Exprs: []shaderir.Expr{{Type: shaderir.LocalVariable, Index: 1}, outPos}},
					{Type: shaderir.Assign, Exprs: []shaderir.Expr{{Type: shaderir.LocalVariable, Index: 2}, uColor}},
					{Type: shaderir.Return},
				},
			},
		},
		FragmentFunc: shaderir.FragmentFunc{
			Block: &shaderir.Block{
				LocalVarIndexOffset: 2,
				Stmts: []shaderir.Stmt{
					{Type: shaderir.Return, Exprs: []shaderir.Expr{{Type: shaderir.LocalVariable, Index: 1}}},
				},
			},
		},
	}
}

func TestGPUSoftwareInstancingStepRateAndFirstInstance(t *testing.T) {
	dev := NewGPUSoftwareDevice(80, 16, GPUViewConfig{ColorFormat: GPUFormatRGBA8Unorm})
	prog := buildInstancedColorProgram()

	// Unit column quad centered at x=0, width=0.22 in NDC (fits cleanly inside each 0.25 column).
	quadVerts := packF32s(
		-0.11, -0.8,
		0.11, -0.8,
		0.11, 0.8,
		-0.11, -0.8,
		0.11, 0.8,
		-0.11, 0.8,
	)
	// Instance colors for indices 0..5. With firstInstance=2 and StepRate=2,
	// drawing 4 instances (inst=0..3) produces:
	//   instID = 2, 3 -> elemIdx = 2 + 0 = 2 (green)
	//   instID = 4, 5 -> elemIdx = 2 + 1 = 3 (blue)
	instColors := packF32s(
		1, 0, 0, 1, // idx 0: red
		1, 1, 0, 1, // idx 1: yellow
		0, 1, 0, 1, // idx 2: green
		0, 0, 1, 1, // idx 3: blue
		1, 0, 1, 1, // idx 4: magenta
		0, 1, 1, 1, // idx 5: cyan
	)

	_, err := dev.Submit(GPUFrame{
		Ops: []GPUResourceOp{
			{Kind: GPUOpCreateBuffer, ID: 1, Buffer: &GPUBufferDesc{Size: uint64(len(quadVerts)), Usage: GPUBufferVertex, Data: quadVerts}},
			{Kind: GPUOpCreateBuffer, ID: 2, Buffer: &GPUBufferDesc{Size: uint64(len(instColors)), Usage: GPUBufferVertex, Data: instColors}},
			{Kind: GPUOpCreatePipeline, ID: 3, Pipeline: &GPUPipelineDesc{
				Vertex:   GPUShaderStage{IR: prog, EntryPoint: "vs_main"},
				Fragment: GPUShaderStage{IR: prog, EntryPoint: "fs_main"},
				VertexBuffers: []GPUVertexBufferLayout{
					{Stride: 8, StepMode: GPUStepVertex, Attributes: []GPUVertexAttribute{{Format: GPUVertexFloat32x2, Offset: 0, Location: 0}}},
					{Stride: 16, StepMode: GPUStepInstance, StepRate: 2, Attributes: []GPUVertexAttribute{{Format: GPUVertexFloat32x4, Offset: 0, Location: 1}}},
				},
				Targets: []GPUColorTargetState{{Format: GPUFormatRGBA8Unorm}},
			}},
		},
		Passes: []GPUPass{{
			ColorAttachments: []GPUColorAttachment{{LoadOp: GPULoadClear, StoreOp: GPUStoreStore}},
			Commands: []GPUCommand{
				{Kind: GPUCmdSetPipeline, ID: 3},
				{Kind: GPUCmdSetVertexBuffer, Slot: 0, ID: 1},
				{Kind: GPUCmdSetVertexBuffer, Slot: 1, ID: 2},
				// 4 instances starting at firstInstance=2 (columns 2, 3, 4, 5)
				{Kind: GPUCmdDrawInstanced, U0: 6, U1: 4, U2: 0, U3: 2},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Submit instanced draw: %v", err)
	}

	img := dev.SurfaceImage()
	wantCols := map[int]color.RGBA{
		0: {0, 0, 0, 0},
		1: {0, 0, 0, 0},
		2: {0, 255, 0, 255}, // inst=0, instID=2, elem=2+0/2=2 -> green
		3: {0, 255, 0, 255}, // inst=1, instID=3, elem=2+1/2=2 -> green
		4: {0, 0, 255, 255}, // inst=2, instID=4, elem=2+2/2=3 -> blue
		5: {0, 0, 255, 255}, // inst=3, instID=5, elem=2+3/2=3 -> blue
		6: {0, 0, 0, 0},
		7: {0, 0, 0, 0},
	}
	for col, want := range wantCols {
		px := col*10 + 5
		got := img.RGBAAt(px, 8)
		if got != want {
			t.Errorf("col %d (x=%d): got %+v, want %+v", col, px, got, want)
		}
	}
}

func TestGPUSoftwareDynamicOffsetsValidationAndExecution(t *testing.T) {
	dev := NewGPUSoftwareDevice(16, 16, GPUViewConfig{ColorFormat: GPUFormatRGBA8Unorm})
	prog := buildUniformColorProgram()

	quad := packF32s(-1, -1, 1, -1, 1, 1, -1, -1, 1, 1, -1, 1)
	uData := make([]byte, 256*3)
	copy(uData[0:], packF32s(1, 0, 0, 1))   // offset 0: red
	copy(uData[256:], packF32s(0, 1, 0, 1)) // offset 256: green
	copy(uData[512:], packF32s(0, 0, 1, 1)) // offset 512: blue

	_, err := dev.Submit(GPUFrame{
		Ops: []GPUResourceOp{
			{Kind: GPUOpCreateBuffer, ID: 1, Buffer: &GPUBufferDesc{Size: uint64(len(quad)), Usage: GPUBufferVertex, Data: quad}},
			{Kind: GPUOpCreateBuffer, ID: 2, Buffer: &GPUBufferDesc{Size: uint64(len(uData)), Usage: GPUBufferUniform, Data: uData}},
			{Kind: GPUOpCreateBindGroupLayout, ID: 3, BindGroupLayout: &GPUBindGroupLayoutDesc{
				Entries: []GPUBindGroupLayoutEntry{{Binding: 0, Visibility: GPUStageVertex, Type: GPUBindingUniform, HasDynamicOffset: true}},
			}},
			{Kind: GPUOpCreateBindGroup, ID: 4, BindGroup: &GPUBindGroupDesc{
				Layout:  3,
				Entries: []GPUBindGroupEntry{{Binding: 0, Buffer: 2, Offset: 0, Size: 256}},
			}},
			{Kind: GPUOpCreatePipeline, ID: 5, Pipeline: &GPUPipelineDesc{
				Vertex:        GPUShaderStage{IR: prog, EntryPoint: "vs_main"},
				Fragment:      GPUShaderStage{IR: prog, EntryPoint: "fs_main"},
				Layouts:       []GPUID{3},
				VertexBuffers: []GPUVertexBufferLayout{{Stride: 8, Attributes: []GPUVertexAttribute{{Format: GPUVertexFloat32x2, Location: 0}}}},
				Targets:       []GPUColorTargetState{{Format: GPUFormatRGBA8Unorm}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Wrong dynamic offset count must fail.
	if _, err := dev.Submit(GPUFrame{
		Passes: []GPUPass{{
			ColorAttachments: []GPUColorAttachment{{LoadOp: GPULoadClear, StoreOp: GPUStoreStore}},
			Commands: []GPUCommand{
				{Kind: GPUCmdSetPipeline, ID: 5},
				{Kind: GPUCmdSetBindGroup, Slot: 0, ID: 4},
			},
		}},
	}); err == nil || !strings.Contains(err.Error(), "dynamic offsets") {
		t.Fatalf("expected dynamic offset count error, got %v", err)
	}

	// Unaligned dynamic offset must fail.
	if _, err := dev.Submit(GPUFrame{
		Passes: []GPUPass{{
			ColorAttachments: []GPUColorAttachment{{LoadOp: GPULoadClear, StoreOp: GPUStoreStore}},
			Commands: []GPUCommand{
				{Kind: GPUCmdSetPipeline, ID: 5},
				{Kind: GPUCmdSetBindGroup, Slot: 0, ID: 4, DynamicOffsets: []uint32{128}},
			},
		}},
	}); err == nil || !strings.Contains(err.Error(), "aligned") {
		t.Fatalf("expected alignment error, got %v", err)
	}

	// Valid dynamic offsets select each distinct color.
	wants := []struct {
		off  uint32
		want color.RGBA
	}{
		{0, color.RGBA{255, 0, 0, 255}},
		{256, color.RGBA{0, 255, 0, 255}},
		{512, color.RGBA{0, 0, 255, 255}},
	}
	for _, tc := range wants {
		if _, err := dev.Submit(GPUFrame{
			Passes: []GPUPass{{
				ColorAttachments: []GPUColorAttachment{{LoadOp: GPULoadClear, StoreOp: GPUStoreStore}},
				Commands: []GPUCommand{
					{Kind: GPUCmdSetPipeline, ID: 5},
					{Kind: GPUCmdSetBindGroup, Slot: 0, ID: 4, DynamicOffsets: []uint32{tc.off}},
					{Kind: GPUCmdSetVertexBuffer, Slot: 0, ID: 1},
					{Kind: GPUCmdDraw, U0: 6},
				},
			}},
		}); err != nil {
			t.Fatalf("Submit offset=%d: %v", tc.off, err)
		}
		if got := dev.SurfaceImage().RGBAAt(8, 8); got != tc.want {
			t.Errorf("offset %d pixel = %+v, want %+v", tc.off, got, tc.want)
		}
	}
}

func TestGPUSoftwarePipelinedSharedRingZeroCorruption(t *testing.T) {
	const slotSize = 256
	shared := make([]byte, slotSize*2)
	dev := NewGPUSoftwareDevice(8, 8, GPUViewConfig{ColorFormat: GPUFormatRGBA8Unorm, FramesInFlight: 2})
	defer dev.Close()
	dev.SetPipelined(true)

	prog := buildUniformColorProgram()
	quad := packF32s(-1, -1, 1, -1, 1, 1, -1, -1, 1, 1, -1, 1)

	frame0Paused := make(chan struct{}, 1)
	releaseFrame0 := make(chan struct{}, 1)
	captured := make([]*image.RGBA, 2)
	frame0Done := make(chan struct{}, 1)
	frame1Done := make(chan struct{}, 1)

	dev.SetFrameHooks(
		func(f GPUFrame) {
			if f.FrameIndex == 0 {
				frame0Paused <- struct{}{}
				<-releaseFrame0
			}
		},
		func(f GPUFrame) {
			if f.FrameIndex < 2 {
				captured[f.FrameIndex] = dev.SurfaceImage()
			}
		},
	)

	// Write slot 0 (red) and submit Frame 0.
	copy(shared[0:], packF32s(1, 0, 0, 1))
	if _, err := dev.Submit(GPUFrame{
		FrameIndex: 0,
		FrameSlot:  0,
		OnComplete: func() { frame0Done <- struct{}{} },
		Ops: []GPUResourceOp{
			{Kind: GPUOpCreateBuffer, ID: 1, Buffer: &GPUBufferDesc{Size: uint64(len(quad)), Usage: GPUBufferVertex, Data: quad}},
			{Kind: GPUOpCreateBuffer, ID: 2, Buffer: &GPUBufferDesc{Size: uint64(len(shared)), Usage: GPUBufferUniform, Shared: shared}},
			{Kind: GPUOpCreateBindGroupLayout, ID: 3, BindGroupLayout: &GPUBindGroupLayoutDesc{
				Entries: []GPUBindGroupLayoutEntry{{Binding: 0, Visibility: GPUStageVertex, Type: GPUBindingUniform, HasDynamicOffset: true}},
			}},
			{Kind: GPUOpCreateBindGroup, ID: 4, BindGroup: &GPUBindGroupDesc{
				Layout:  3,
				Entries: []GPUBindGroupEntry{{Binding: 0, Buffer: 2, Offset: 0, Size: 256}},
			}},
			{Kind: GPUOpCreatePipeline, ID: 5, Pipeline: &GPUPipelineDesc{
				Vertex:        GPUShaderStage{IR: prog, EntryPoint: "vs_main"},
				Fragment:      GPUShaderStage{IR: prog, EntryPoint: "fs_main"},
				Layouts:       []GPUID{3},
				VertexBuffers: []GPUVertexBufferLayout{{Stride: 8, Attributes: []GPUVertexAttribute{{Format: GPUVertexFloat32x2, Location: 0}}}},
				Targets:       []GPUColorTargetState{{Format: GPUFormatRGBA8Unorm}},
			}},
		},
		Passes: []GPUPass{{
			ColorAttachments: []GPUColorAttachment{{LoadOp: GPULoadClear, StoreOp: GPUStoreStore}},
			Commands: []GPUCommand{
				{Kind: GPUCmdSetPipeline, ID: 5},
				{Kind: GPUCmdSetBindGroup, Slot: 0, ID: 4, DynamicOffsets: []uint32{0}},
				{Kind: GPUCmdSetVertexBuffer, Slot: 0, ID: 1},
				{Kind: GPUCmdDraw, U0: 6},
			},
		}},
	}); err != nil {
		t.Fatalf("Submit frame 0: %v", err)
	}

	// Wait until GPU worker holds Frame 0 mid-execution on slot 0.
	select {
	case <-frame0Paused:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for frame 0 to enter GPU worker")
	}

	// Concurrently write slot 1 (green) and submit Frame 1 while Frame 0 is paused!
	copy(shared[slotSize:], packF32s(0, 1, 0, 1))
	if _, err := dev.Submit(GPUFrame{
		FrameIndex: 1,
		FrameSlot:  1,
		OnComplete: func() { frame1Done <- struct{}{} },
		Passes: []GPUPass{{
			ColorAttachments: []GPUColorAttachment{{LoadOp: GPULoadClear, StoreOp: GPUStoreStore}},
			Commands: []GPUCommand{
				{Kind: GPUCmdSetPipeline, ID: 5},
				{Kind: GPUCmdSetBindGroup, Slot: 0, ID: 4, DynamicOffsets: []uint32{slotSize}},
				{Kind: GPUCmdSetVertexBuffer, Slot: 0, ID: 1},
				{Kind: GPUCmdDraw, U0: 6},
			},
		}},
	}); err != nil {
		t.Fatalf("Submit frame 1: %v", err)
	}

	// Release GPU worker and wait for both frames to complete.
	releaseFrame0 <- struct{}{}
	<-frame0Done
	<-frame1Done
	dev.WaitIdle()
	if err := dev.LastError(); err != nil {
		t.Fatalf("LastError: %v", err)
	}
	if got := captured[0].RGBAAt(4, 4); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("frame 0 pixel = %+v, want red (zero corruption from concurrent slot 1 write)", got)
	}
	if got := captured[1].RGBAAt(4, 4); got != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("frame 1 pixel = %+v, want green", got)
	}
}
