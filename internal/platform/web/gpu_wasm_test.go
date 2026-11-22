//go:build js && wasm

package web

import (
	"encoding/binary"
	"math"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func packF32(vals ...float32) []byte {
	out := make([]byte, len(vals)*4)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(v))
	}
	return out
}

// TestWebGPUViewWebGL2FallbackAndTeardown mounts a KindGPU view on a real <canvas>, forces the WebGL 2 fallback path, clears and draws an off-commit frame, verifies readback and resize, exercises webglcontextlost/restored, and confirms complete resource and js.Func cleanup on unmount. Run in a browser with: nsgo test -target web ./internal/platform/web
func TestWebGPUViewWebGL2FallbackAndTeardown(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	if !probeWebGL2(js.Global()) {
		t.Skip("WebGL 2 is not available in this browser environment")
	}

	prevForce := gpuForceWebGL2
	gpuForceWebGL2 = true
	defer func() { gpuForceWebGL2 = prevForce }()

	s := &surface{doc: doc}
	s.env.PixelRatio = 1
	v := s.create(1, driver.KindGPU)
	v.size = driver.Size{Width: 64, Height: 64}
	doc.Get("body").Call("appendChild", v.el)
	defer func() {
		if v.gpu != nil {
			v.gpu.release()
		}
		v.el.Call("remove")
	}()

	s.configureGPU(v, driver.GPUViewConfig{
		ColorFormat: driver.GPUFormatRGBA8Unorm,
		DepthFormat: driver.GPUFormatDepth24Plus,
		SampleCount: 1,
	})
	s.initPendingGPUs()

	g := v.gpu
	var gotEvents []*driver.GPUEvent
	g.onEvent = func(ev *driver.GPUEvent) {
		gotEvents = append(gotEvents, ev)
	}
	if g == nil || g.ctrl.state != gpuStateReady || g.ctrl.backend != gpuBackendWebGL2 {
		t.Fatalf("expected WebGL 2 backend ready, got state=%v backend=%v", g.ctrl.state, g.ctrl.backend)
	}
	if g.canvas.Get("width").Int() != 64 || g.canvas.Get("height").Int() != 64 {
		t.Fatalf("canvas size = %dx%d, want 64x64", g.canvas.Get("width").Int(), g.canvas.Get("height").Int())
	}

	// Submit an off-commit frame that uploads a textured fullscreen triangle + identity MVP uniform buffer, renders it to an offscreen render target, clears the swapchain to a known color, and reads back both surfaces.
	gen := g.ctrl.generation
	vbufData := packF32(
		-1, -1, 0, 0, 0,
		3, -1, 0, 2, 0,
		-1, 3, 0, 0, 2,
	)
	identityMat4 := packF32(
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	)
	redTex2x2 := []byte{
		0xee, 0x22, 0x11, 0xff, 0xee, 0x22, 0x11, 0xff,
		0xee, 0x22, 0x11, 0xff, 0xee, 0x22, 0x11, 0xff,
	}
	s.SubmitGPU(driver.GPUFrame{
		ViewID:     v.id,
		Generation: gen,
		Ops: []driver.GPUResourceOp{
			{
				Kind: driver.GPUOpCreateBuffer,
				ID:   1,
				Buffer: &driver.GPUBufferDesc{
					Size:  uint64(len(vbufData)),
					Usage: driver.GPUBufferVertex,
					Data:  vbufData,
				},
			},
			{
				Kind: driver.GPUOpCreateBuffer,
				ID:   5,
				Buffer: &driver.GPUBufferDesc{
					Size:  uint64(len(identityMat4)),
					Usage: driver.GPUBufferUniform | driver.GPUBufferCopyDst,
					Data:  identityMat4,
				},
			},
			{
				Kind: driver.GPUOpCreateTexture,
				ID:   2,
				Texture: &driver.GPUTextureDesc{
					Dimension: driver.GPUDim2D,
					Width:     2,
					Height:    2,
					Format:    driver.GPUFormatRGBA8Unorm,
					Usage:     driver.GPUTextureSampled | driver.GPUTextureCopyDst,
				},
			},
			{
				Kind: driver.GPUOpWriteTexture,
				ID:   2,
				Data: redTex2x2,
				DataLayout: driver.GPUDataLayout{
					BytesPerRow:  8,
					RowsPerImage: 2,
				},
			},
			{
				Kind: driver.GPUOpCreateTexture,
				ID:   6,
				Texture: &driver.GPUTextureDesc{
					Dimension: driver.GPUDim2D,
					Width:     16,
					Height:    16,
					Format:    driver.GPUFormatRGBA8Unorm,
					Usage:     driver.GPUTextureRenderAttachment | driver.GPUTextureCopySrc,
				},
			},
			{
				Kind: driver.GPUOpCreateSampler,
				ID:   3,
				Sampler: &driver.GPUSamplerDesc{
					MinFilter: driver.GPUFilterNearest,
					MagFilter: driver.GPUFilterNearest,
				},
			},
			{
				Kind: driver.GPUOpCreateBindGroupLayout,
				ID:   7,
				BindGroupLayout: &driver.GPUBindGroupLayoutDesc{
					Entries: []driver.GPUBindGroupLayoutEntry{
						{Binding: 0, Visibility: driver.GPUStageVertex, Type: driver.GPUBindingUniform},
						{Binding: 1, Visibility: driver.GPUStageFragment, Type: driver.GPUBindingTexture},
						{Binding: 2, Visibility: driver.GPUStageFragment, Type: driver.GPUBindingSampler},
					},
				},
			},
			{
				Kind: driver.GPUOpCreateBindGroup,
				ID:   8,
				BindGroup: &driver.GPUBindGroupDesc{
					Layout: 7,
					Entries: []driver.GPUBindGroupEntry{
						{Binding: 0, Buffer: 5, Offset: 0, Size: uint64(len(identityMat4))},
						{Binding: 1, Texture: 2},
						{Binding: 2, Sampler: 3},
					},
				},
			},
			{
				Kind: driver.GPUOpCreatePipeline,
				ID:   4,
				Pipeline: &driver.GPUPipelineDesc{
					Vertex:   driver.GPUShaderStage{IR: samplePipelineIR(), EntryPoint: "vs_main"},
					Fragment: driver.GPUShaderStage{IR: samplePipelineIR(), EntryPoint: "fs_main"},
					Layouts:  []driver.GPUID{7},
					VertexBuffers: []driver.GPUVertexBufferLayout{{
						Stride: 5 * 4,
						Attributes: []driver.GPUVertexAttribute{
							{Location: 0, Offset: 0, Format: driver.GPUVertexFloat32x3},
							{Location: 1, Offset: 3 * 4, Format: driver.GPUVertexFloat32x2},
						},
					}},
					Targets: []driver.GPUColorTargetState{{Format: driver.GPUFormatRGBA8Unorm}},
				},
			},
			{
				Kind:            driver.GPUOpReadTexture,
				ReadbackID:      101,
				ReadbackSurface: true,
			},
			{
				Kind:       driver.GPUOpReadTexture,
				ID:         6,
				ReadbackID: 102,
			},
		},
		Passes: []driver.GPUPass{
			{
				ColorAttachments: []driver.GPUColorAttachment{{
					Texture:    6,
					LoadOp:     driver.GPULoadClear,
					StoreOp:    driver.GPUStoreStore,
					ClearColor: driver.RGBA(0x0000ff, 1),
				}},
				Commands: []driver.GPUCommand{
					{Kind: driver.GPUCmdSetPipeline, ID: 4},
					{Kind: driver.GPUCmdSetBindGroup, Slot: 0, ID: 8},
					{Kind: driver.GPUCmdSetVertexBuffer, Slot: 0, ID: 1},
					{Kind: driver.GPUCmdDraw, U0: 3, U1: 0},
				},
			},
			{
				ColorAttachments: []driver.GPUColorAttachment{{
					LoadOp:     driver.GPULoadClear,
					StoreOp:    driver.GPUStoreStore,
					ClearColor: driver.RGBA(0x204080, 1),
				}},
			},
		},
		Present: true,
	})

	gl2, ok := g.backend.(*webgl2Backend)
	if !ok {
		t.Fatalf("expected *webgl2Backend, got %T", g.backend)
	}
	if len(gl2.buffers) != 2 || len(gl2.textures) != 2 || len(gl2.samplers) != 1 || len(gl2.pipelines) != 1 {
		t.Fatalf("unexpected GL2 resource counts: buf=%d tex=%d samp=%d pipe=%d",
			len(gl2.buffers), len(gl2.textures), len(gl2.samplers), len(gl2.pipelines))
	}

	var swapPix, rtPix []byte
	for _, ev := range gotEvents {
		if ev.Kind == driver.GPUEventError {
			t.Fatalf("unexpected GPUEventError: code=%d msg=%q", ev.ErrorCode, ev.Message)
		}
		if ev.Kind == driver.GPUEventReadback {
			if ev.Message != "" {
				t.Fatalf("readback %d failed: %s", ev.ReadbackID, ev.Message)
			}
			switch ev.ReadbackID {
			case 101:
				swapPix = ev.Pixels
			case 102:
				rtPix = ev.Pixels
			}
		}
	}
	if len(swapPix) != 64*64*4 {
		t.Fatalf("swapchain readback len = %d, want %d", len(swapPix), 64*64*4)
	}
	centerSwap := (32*64 + 32) * 4
	if swapPix[centerSwap] != 0x20 || swapPix[centerSwap+1] != 0x40 || swapPix[centerSwap+2] != 0x80 || swapPix[centerSwap+3] != 0xff {
		t.Fatalf("swapchain center pixel = %v, want [32 64 128 255]", swapPix[centerSwap:centerSwap+4])
	}
	if len(rtPix) != 16*16*4 {
		t.Fatalf("offscreen RT readback len = %d, want %d", len(rtPix), 16*16*4)
	}
	centerRT := (8*16 + 8) * 4
	if rtPix[centerRT] != 0xee || rtPix[centerRT+1] != 0x22 || rtPix[centerRT+2] != 0x11 || rtPix[centerRT+3] != 0xff {
		t.Fatalf("offscreen textured triangle center pixel = %v, want [238 34 17 255]", rtPix[centerRT:centerRT+4])
	}

	// Verify resize updates canvas backing dimensions.
	v.size = driver.Size{Width: 96, Height: 48}
	s.resizeGPU(v)
	if g.canvas.Get("width").Int() != 96 || g.canvas.Get("height").Int() != 48 {
		t.Fatalf("resized canvas = %dx%d, want 96x48", g.canvas.Get("width").Int(), g.canvas.Get("height").Int())
	}

	// Verify synthetic context lost / restored bumps generation.
	evCtor := js.Global().Get("Event")
	if evCtor.Truthy() {
		g.canvas.Call("dispatchEvent", evCtor.New("webglcontextlost", map[string]any{"cancelable": true}))
		if g.ctrl.state != gpuStateLost {
			t.Fatalf("expected gpuStateLost after webglcontextlost, got %v", g.ctrl.state)
		}
		g.canvas.Call("dispatchEvent", evCtor.New("webglcontextrestored"))
		if g.ctrl.state != gpuStateReady || g.ctrl.generation != gen+1 {
			t.Fatalf("expected restored ready gen=%d, got state=%v gen=%d", gen+1, g.ctrl.state, g.ctrl.generation)
		}
	}

	// Unmount and verify full resource + js.Func teardown.
	g.release()
	if g.ctrl.state != gpuStateDestroyed {
		t.Fatalf("expected gpuStateDestroyed after release, got %v", g.ctrl.state)
	}
	if len(g.pendingFuncs) != 0 || len(g.listeners) != 0 || g.backend != nil {
		t.Fatalf("leaked resources after release: pendingFuncs=%d listeners=%d backend=%v",
			len(g.pendingFuncs), len(g.listeners), g.backend)
	}
}

// TestWebGPUPrimaryOrAutoFallbackReadback exercises the unforced default path: WebGPU primary when navigator.gpu yields an adapter+device, or automatic WebGL 2 fallback otherwise, verifying offscreen render + readback and clean js.Func release on unmount.
func TestWebGPUPrimaryOrAutoFallbackReadback(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	if !webGPUAvailable() {
		t.Skip("neither WebGPU nor WebGL 2 is available in this browser environment")
	}

	prevForce := gpuForceWebGL2
	gpuForceWebGL2 = false
	defer func() { gpuForceWebGL2 = prevForce }()

	s := &surface{doc: doc}
	s.env.PixelRatio = 1
	v := s.create(2, driver.KindGPU)
	v.size = driver.Size{Width: 32, Height: 32}
	doc.Get("body").Call("appendChild", v.el)
	defer func() {
		if v.gpu != nil {
			v.gpu.release()
		}
		v.el.Call("remove")
	}()

	readyCh := make(chan *driver.GPUEvent, 1)
	readbackCh := make(chan *driver.GPUEvent, 1)
	errCh := make(chan *driver.GPUEvent, 4)

	g := v.gpu
	g.onEvent = func(ev *driver.GPUEvent) {
		switch ev.Kind {
		case driver.GPUEventDeviceReady:
			select {
			case readyCh <- ev:
			default:
			}
		case driver.GPUEventReadback:
			select {
			case readbackCh <- ev:
			default:
			}
		case driver.GPUEventError:
			select {
			case errCh <- ev:
			default:
			}
		}
	}

	s.configureGPU(v, driver.GPUViewConfig{
		ColorFormat: driver.GPUFormatRGBA8Unorm,
		DepthFormat: driver.GPUFormatDepth24Plus,
		SampleCount: 1,
	})
	s.initPendingGPUs()

	var readyEv *driver.GPUEvent
	select {
	case readyEv = <-readyCh:
	case errEv := <-errCh:
		t.Fatalf("GPU init failed: code=%d msg=%q", errEv.ErrorCode, errEv.Message)
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for GPUEventDeviceReady (state=%v)", g.ctrl.state)
	}
	t.Logf("initialized web GPU backend=%s format=%d limits=%+v", g.ctrl.backend, readyEv.SurfaceFormat, readyEv.Limits)

	s.SubmitGPU(driver.GPUFrame{
		ViewID:     v.id,
		Generation: readyEv.Generation,
		Ops: []driver.GPUResourceOp{
			{
				Kind: driver.GPUOpCreateTexture,
				ID:   1,
				Texture: &driver.GPUTextureDesc{
					Dimension: driver.GPUDim2D,
					Width:     8,
					Height:    8,
					Format:    driver.GPUFormatRGBA8Unorm,
					Usage:     driver.GPUTextureRenderAttachment | driver.GPUTextureCopySrc,
				},
			},
			{
				Kind:       driver.GPUOpReadTexture,
				ID:         1,
				ReadbackID: 201,
			},
		},
		Passes: []driver.GPUPass{{
			ColorAttachments: []driver.GPUColorAttachment{{
				Texture:    1,
				LoadOp:     driver.GPULoadClear,
				StoreOp:    driver.GPUStoreStore,
				ClearColor: driver.RGBA(0x10c040, 1),
			}},
		}},
		Present: false,
	})

	var rbEv *driver.GPUEvent
	select {
	case rbEv = <-readbackCh:
	case errEv := <-errCh:
		t.Fatalf("GPU error during render/readback: code=%d msg=%q", errEv.ErrorCode, errEv.Message)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for GPUEventReadback")
	}
	if rbEv.Message != "" {
		t.Fatalf("readback failed: %s", rbEv.Message)
	}
	if len(rbEv.Pixels) != 8*8*4 {
		t.Fatalf("readback pixels len = %d, want %d", len(rbEv.Pixels), 8*8*4)
	}
	off := (4*8 + 4) * 4
	if rbEv.Pixels[off] != 0x10 || rbEv.Pixels[off+1] != 0xc0 || rbEv.Pixels[off+2] != 0x40 || rbEv.Pixels[off+3] != 0xff {
		t.Fatalf("offscreen clear center pixel = %v, want [16 192 64 255]", rbEv.Pixels[off:off+4])
	}

	g.release()
	// Allow WebGPU device.lost microtask to settle and release its promise js.Funcs.
	deadline := time.Now().Add(2 * time.Second)
	for len(g.pendingFuncs) > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(g.pendingFuncs) != 0 || len(g.listeners) != 0 || g.backend != nil {
		t.Fatalf("leaked resources after release: pendingFuncs=%d listeners=%d backend=%v",
			len(g.pendingFuncs), len(g.listeners), g.backend)
	}
}
