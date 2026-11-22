package gpu_test

import (
	"errors"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestGPUBackgroundSuspensionNeverSubmits(t *testing.T) {
	mod := gpu.MustCompileWGSL(color3DWGSL)
	ident := [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
	triVerts := packFloat32s(
		-0.8, -0.8, 0.5, 1, 0, 0, 1,
		0.8, -0.8, 0.5, 1, 0, 0, 1,
		0.0, 0.8, 0.5, 1, 0, 0, 1,
	)

	var (
		dev          *gpu.Device
		vbuf         *gpu.Buffer
		ubuf         *gpu.Buffer
		bg           *gpu.BindGroup
		pipe         *gpu.Pipeline
		frameCount   int
		suspendCount int
		resumeCount  int
		gpuErrors    []gpu.Error
		readbackGot  int
	)

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return gpu.View(cx, gpu.ViewProps{
			TestID: "ios-gpu",
			Style:  core.Style{Width: core.Pt(64), Height: core.Pt(64)},
			OnReady: func(d *gpu.Device) {
				dev = d
				var err error
				vbuf, err = d.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: triVerts})
				if err != nil {
					t.Fatalf("CreateBuffer verts: %v", err)
				}
				ubuf, err = d.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageUniform | gpu.UsageCopyDst, Data: packFloat32s(ident[:]...)})
				if err != nil {
					t.Fatalf("CreateBuffer uniforms: %v", err)
				}
				bgl, err := d.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
					{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
				})
				if err != nil {
					t.Fatalf("CreateBindGroupLayout: %v", err)
				}
				bg, err = d.CreateBindGroup(cx, bgl, []gpu.BindGroupEntry{{Binding: 0, Buffer: ubuf}})
				if err != nil {
					t.Fatalf("CreateBindGroup: %v", err)
				}
				pipe, err = d.CreatePipeline(cx, gpu.PipelineDesc{
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
					Targets: []gpu.ColorTargetState{{Format: d.SurfaceFormat()}},
				})
				if err != nil {
					t.Fatalf("CreatePipeline: %v", err)
				}
			},
			OnFrame: func(frame *gpu.Frame) {
				frameCount++
				pass := frame.BeginRenderPass(gpu.RenderPassDesc{})
				pass.ClearColor(core.RGB(0x080c14))
				pass.SetPipeline(pipe)
				pass.SetBindGroup(0, bg)
				pass.SetVertexBuffer(0, vbuf, 0)
				pass.Draw(3, 0)
				pass.End()
				frame.Submit()
			},
			OnSuspend: func() { suspendCount++ },
			OnResume:  func() { resumeCount++ },
			OnError:   func(err gpu.Error) { gpuErrors = append(gpuErrors, err) },
		})
	}, nsgotest.Size(64, 64), nsgotest.ManualFrames(), nsgotest.Environment(core.Environment{
		OS:        core.OSIOS,
		GPU:       true,
		Lifecycle: core.LifecycleActive,
	}))

	// Render two active frames first.
	app.Advance(16 * time.Millisecond)
	app.Advance(16 * time.Millisecond)
	if frameCount < 2 {
		t.Fatalf("expected active frames to run, got frameCount=%d", frameCount)
	}
	if dev == nil || dev.IsSuspended() {
		t.Fatalf("expected active non-suspended device")
	}
	if suspendCount != 0 || resumeCount != 0 {
		t.Fatalf("unexpected initial suspend=%d resume=%d", suspendCount, resumeCount)
	}

	// Background the iOS app: GPU submission must stop immediately.
	app.SetEnvironment(core.Environment{
		OS:        core.OSIOS,
		GPU:       true,
		Lifecycle: core.LifecycleBackground,
	})
	if suspendCount != 1 {
		t.Fatalf("suspendCount = %d, want 1", suspendCount)
	}
	if !dev.IsSuspended() {
		t.Fatalf("expected dev.IsSuspended() == true in LifecycleBackground")
	}

	framesBefore := len(app.GPUFrames())
	onFrameBefore := frameCount

	// Advance multiple display ticks while backgrounded.
	for range 5 {
		app.Advance(16 * time.Millisecond)
	}
	if frameCount != onFrameBefore {
		t.Fatalf("OnFrame ran while backgrounded: before=%d after=%d", onFrameBefore, frameCount)
	}
	if got := len(app.GPUFrames()); got != framesBefore {
		t.Fatalf("SubmitGPU called while backgrounded (ticks): before=%d after=%d", framesBefore, got)
	}

	// Even direct manual submissions and readbacks on Device must NEVER reach SubmitGPU while backgrounded.
	app.Post(func() {
		dev.WriteBuffer(ubuf, 0, packFloat32s(ident[:]...))
		enc := dev.BeginEncoder()
		pass := enc.BeginRenderPass(gpu.RenderPassDesc{})
		pass.ClearColor(core.RGB(0xff00ff))
		pass.SetPipeline(pipe)
		pass.SetBindGroup(0, bg)
		pass.SetVertexBuffer(0, vbuf, 0)
		pass.Draw(3, 0)
		pass.End()
		dev.Submit(enc.Finish())
		dev.ReadSurface(func(rgba []byte, err error) {
			if err == nil && len(rgba) > 0 {
				readbackGot++
			}
		})
	})
	app.Settle()
	if got := len(app.GPUFrames()); got != framesBefore {
		t.Fatalf("direct Submit/ReadSurface called SubmitGPU while backgrounded: before=%d after=%d", framesBefore, got)
	}
	if readbackGot != 0 {
		t.Fatalf("readback completed while backgrounded; must defer until resume")
	}

	// Transient LifecycleInactive (iOS willEnterForeground before didBecomeActive) stays suspended.
	app.SetEnvironment(core.Environment{
		OS:        core.OSIOS,
		GPU:       true,
		Lifecycle: core.LifecycleInactive,
	})
	app.Advance(16 * time.Millisecond)
	if !dev.IsSuspended() || resumeCount != 0 || len(app.GPUFrames()) != framesBefore {
		t.Fatalf("expected view to stay suspended through Background->Inactive (suspended=%v resume=%d frames=%d/%d)",
			dev.IsSuspended(), resumeCount, len(app.GPUFrames()), framesBefore)
	}

	// Return to LifecycleActive: queued ops flush, OnResume fires, OnFrame resumes.
	app.SetEnvironment(core.Environment{
		OS:        core.OSIOS,
		GPU:       true,
		Lifecycle: core.LifecycleActive,
	})
	if resumeCount != 1 {
		t.Fatalf("resumeCount = %d, want 1", resumeCount)
	}
	if dev.IsSuspended() {
		t.Fatalf("expected dev.IsSuspended() == false after LifecycleActive")
	}
	if readbackGot != 1 {
		t.Fatalf("expected deferred ReadSurface to complete on resume, got %d", readbackGot)
	}

	framesAfterResume := len(app.GPUFrames())
	app.Advance(16 * time.Millisecond)
	if frameCount <= onFrameBefore {
		t.Fatalf("expected OnFrame to resume after LifecycleActive")
	}
	if len(app.GPUFrames()) <= framesAfterResume {
		t.Fatalf("expected new GPU frames to submit after resume")
	}
	if len(gpuErrors) != 0 {
		t.Fatalf("unexpected GPU errors: %+v", gpuErrors)
	}

	img, err := app.GPUReadPixels("id:ios-gpu")
	if err != nil {
		t.Fatalf("GPUReadPixels: %v", err)
	}
	center := img.RGBAAt(32, 32)
	if center.R < 250 || center.G > 5 || center.B > 5 {
		t.Errorf("center pixel after resume = %+v, want red", center)
	}
}

func TestGPUDeviceLostReasonsAndStaleHandleSafety(t *testing.T) {
	mod := gpu.MustCompileWGSL(color3DWGSL)
	ident := [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
	verts := packFloat32s(
		-0.5, -0.5, 0.5, 0, 1, 0, 1,
		0.5, -0.5, 0.5, 0, 1, 0, 1,
		0.0, 0.5, 0.5, 0, 1, 0, 1,
	)

	var (
		dev1          *gpu.Device
		curDev        *gpu.Device
		oldBuf        *gpu.Buffer
		oldTex        *gpu.Texture
		oldSamp       *gpu.Sampler
		oldBGL        *gpu.BindGroupLayout
		oldBG         *gpu.BindGroup
		oldPipe       *gpu.Pipeline
		lostEvents    []gpu.DeviceLostEvent
		restoredEpoch []uint32
		gpuErrors     []gpu.Error
	)

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return gpu.View(cx, gpu.ViewProps{
			TestID: "gpu-loss",
			Style:  core.Style{Width: core.Pt(48), Height: core.Pt(48)},
			OnReady: func(d *gpu.Device) {
				curDev = d
				if d.Epoch() == 1 {
					dev1 = d
					var err error
					oldBuf, err = d.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageVertex | gpu.UsageUniform | gpu.UsageCopyDst, Data: verts})
					if err != nil {
						t.Fatalf("CreateBuffer: %v", err)
					}
					oldTex, err = d.CreateTexture(cx, gpu.TextureDesc{
						Width: 4, Height: 4, Format: gpu.FormatRGBA8Unorm, Usage: gpu.TextureSampled | gpu.TextureCopyDst,
					})
					if err != nil {
						t.Fatalf("CreateTexture: %v", err)
					}
					oldSamp, err = d.CreateSampler(cx, gpu.SamplerDesc{MinFilter: gpu.FilterNearest, MagFilter: gpu.FilterNearest})
					if err != nil {
						t.Fatalf("CreateSampler: %v", err)
					}
					oldBGL, err = d.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
						{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
					})
					if err != nil {
						t.Fatalf("CreateBindGroupLayout: %v", err)
					}
					oldBG, err = d.CreateBindGroup(cx, oldBGL, []gpu.BindGroupEntry{{Binding: 0, Buffer: oldBuf}})
					if err != nil {
						t.Fatalf("CreateBindGroup: %v", err)
					}
					oldPipe, err = d.CreatePipeline(cx, gpu.PipelineDesc{
						Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
						Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
						Layouts:  []*gpu.BindGroupLayout{oldBGL},
						VertexBuffers: []gpu.VertexBufferLayout{{
							Stride: 7 * 4,
							Attributes: []gpu.VertexAttribute{
								{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
								{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x4},
							},
						}},
						Targets: []gpu.ColorTargetState{{Format: d.SurfaceFormat()}},
					})
					if err != nil {
						t.Fatalf("CreatePipeline: %v", err)
					}
				}
			},
			OnDeviceLost: func(ev gpu.DeviceLostEvent) {
				lostEvents = append(lostEvents, ev)
			},
			OnDeviceRestored: func(d *gpu.Device) {
				restoredEpoch = append(restoredEpoch, d.Epoch())
			},
			OnError: func(err gpu.Error) {
				gpuErrors = append(gpuErrors, err)
			},
		})
	}, nsgotest.Size(48, 48), nsgotest.ManualFrames())

	if dev1 == nil || dev1.Epoch() != 1 || dev1.IsLost() {
		t.Fatalf("unexpected initial dev1 state: %v", dev1)
	}
	if !oldBuf.Valid() || !oldTex.Valid() || !oldSamp.Valid() || !oldBGL.Valid() || !oldBG.Valid() || !oldPipe.Valid() || !oldPipe.Ready() {
		t.Fatalf("expected all epoch-1 resources valid before device loss")
	}

	// Simulate WebGL/WebGPU context loss.
	app.TriggerGPUDeviceLostWithReason("id:gpu-loss", nsgotest.GPULostContextLost, "webglcontextlost")
	if len(lostEvents) != 1 || lostEvents[0].Reason != gpu.ReasonContextLost || lostEvents[0].Message != "webglcontextlost" {
		t.Fatalf("lostEvents = %+v, want ReasonContextLost", lostEvents)
	}
	if !dev1.IsLost() {
		t.Fatalf("expected dev1.IsLost() == true after context loss")
	}
	if oldBuf.Valid() || oldTex.Valid() || oldSamp.Valid() || oldBGL.Valid() || oldBG.Valid() || oldPipe.Valid() || oldPipe.Ready() {
		t.Fatalf("expected all non-retained handles invalidated after context loss")
	}
	if _, err := dev1.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Size: 16}); !errors.Is(err, gpu.ErrDeviceLost) {
		t.Fatalf("CreateBuffer on lost device = %v, want ErrDeviceLost", err)
	}

	// Restore device -> epoch 2, OnDeviceRestored fires.
	app.RestoreGPUDevice("id:gpu-loss")
	if len(restoredEpoch) != 1 || restoredEpoch[0] != 2 {
		t.Fatalf("restoredEpoch = %v, want [2]", restoredEpoch)
	}
	if curDev == nil || curDev == dev1 || curDev.Epoch() != 2 || curDev.IsLost() {
		t.Fatalf("expected fresh live epoch-2 device after restore")
	}

	// Stale handles from epoch 1 used against epoch 2 must NEVER crash or trigger backend errors.
	var staleReadErr error
	app.Post(func() {
		curDev.WriteBuffer(oldBuf, 0, packFloat32s(ident[:]...))
		curDev.WriteTexture(oldTex, gpu.TextureCopy{Texture: oldTex}, make([]byte, 4*4*4), gpu.DataLayout{BytesPerRow: 16, RowsPerImage: 4})
		enc := curDev.BeginEncoder()
		enc.CopyBufferToBuffer(oldBuf, 0, oldBuf, 0, 16)
		pass := enc.BeginRenderPass(gpu.RenderPassDesc{})
		pass.ClearColor(core.RGB(0x112233))
		pass.SetPipeline(oldPipe)
		pass.SetBindGroup(0, oldBG)
		pass.SetVertexBuffer(0, oldBuf, 0)
		pass.Draw(3, 0)
		pass.End()
		curDev.Submit(enc.Finish())
		curDev.ReadTexture(oldTex, gpu.TextureRegion{Width: 4, Height: 4}, func(_ []byte, err error) {
			staleReadErr = err
		})
	})
	app.Settle()
	if !errors.Is(staleReadErr, gpu.ErrInvalidDescriptor) {
		t.Fatalf("ReadTexture with stale texture = %v, want ErrInvalidDescriptor", staleReadErr)
	}
	if len(gpuErrors) != 0 {
		t.Fatalf("stale handles caused unexpected backend GPU errors: %+v", gpuErrors)
	}

	// Simulate Windows DXGI_ERROR_DEVICE_REMOVED and restore -> epoch 3.
	app.TriggerGPUDeviceLostWithReason("id:gpu-loss", nsgotest.GPULostDeviceRemoved, "DXGI_ERROR_DEVICE_REMOVED")
	if len(lostEvents) != 2 || lostEvents[1].Reason != gpu.ReasonDeviceRemoved {
		t.Fatalf("lostEvents[1] = %+v, want ReasonDeviceRemoved", lostEvents)
	}
	app.RestoreGPUDevice("id:gpu-loss")
	if len(restoredEpoch) != 2 || restoredEpoch[1] != 3 {
		t.Fatalf("restoredEpoch = %v, want [2 3]", restoredEpoch)
	}
	if curDev.Epoch() != 3 {
		t.Fatalf("curDev.Epoch() = %d, want 3", curDev.Epoch())
	}
}

func TestGPUAndroidSurfaceRotationAndRetainedAutoRestore(t *testing.T) {
	texMod := gpu.MustCompileWGSL(texturedWGSL)

	// Fullscreen quad with UVs covering a 2x2 texture.
	quadVerts := packFloat32s(
		-1, -1, 0.5, 0, 1,
		1, -1, 0.5, 1, 1,
		1, 1, 0.5, 1, 0,
		-1, -1, 0.5, 0, 1,
		1, 1, 0.5, 1, 0,
		-1, 1, 0.5, 0, 0,
	)
	texImg := image.NewRGBA(image.Rect(0, 0, 2, 2))
	texImg.SetRGBA(0, 0, color.RGBA{0, 255, 0, 255})
	texImg.SetRGBA(1, 0, color.RGBA{0, 0, 255, 255})
	texImg.SetRGBA(0, 1, color.RGBA{0, 255, 0, 255})
	texImg.SetRGBA(1, 1, color.RGBA{0, 0, 255, 255})

	var (
		dev           *gpu.Device
		vbuf          *gpu.Buffer
		tex           *gpu.Texture
		samp          *gpu.Sampler
		bgl           *gpu.BindGroupLayout
		bg            *gpu.BindGroup
		pipe          *gpu.Pipeline
		lostEvents    []gpu.DeviceLostEvent
		restoredSizes [][2]int
		gpuErrors     []gpu.Error
	)

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return gpu.View(cx, gpu.ViewProps{
			TestID:          "android-gpu",
			Style:           core.Style{Width: core.Pt(40), Height: core.Pt(80)},
			RetainResources: true,
			OnReady: func(d *gpu.Device) {
				dev = d
				// Only create resources once on epoch 1; subsequent rotations rely exclusively on RetainResources automatic recreation!
				if d.Epoch() > 1 {
					return
				}
				var err error
				vbuf, err = d.CreateBuffer(cx, gpu.BufferDesc{
					Label: "quad",
					Usage: gpu.UsageVertex | gpu.UsageCopyDst,
					Size:  uint64(len(quadVerts)),
				})
				if err != nil {
					t.Fatalf("CreateBuffer: %v", err)
				}
				// Dynamic WriteBuffer after creation must also be captured in shadow state!
				d.WriteBuffer(vbuf, 0, quadVerts)

				tex, err = d.CreateTextureFromImage(cx, core.ImageSource{Pixels: texImg}, gpu.TextureDesc{
					Label:  "split",
					Format: gpu.FormatRGBA8Unorm,
					Usage:  gpu.TextureSampled | gpu.TextureCopyDst,
				})
				if err != nil {
					t.Fatalf("CreateTextureFromImage: %v", err)
				}
				samp, err = d.CreateSampler(cx, gpu.SamplerDesc{
					MinFilter: gpu.FilterNearest,
					MagFilter: gpu.FilterNearest,
					AddressU:  gpu.AddressClampToEdge,
					AddressV:  gpu.AddressClampToEdge,
				})
				if err != nil {
					t.Fatalf("CreateSampler: %v", err)
				}
				bgl, err = d.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
					{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
					{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingSampler},
				})
				if err != nil {
					t.Fatalf("CreateBindGroupLayout: %v", err)
				}
				bg, err = d.CreateBindGroup(cx, bgl, []gpu.BindGroupEntry{
					{Binding: 0, Texture: tex},
					{Binding: 1, Sampler: samp},
				})
				if err != nil {
					t.Fatalf("CreateBindGroup: %v", err)
				}
				pipe, err = d.CreatePipeline(cx, gpu.PipelineDesc{
					Label:    "textured-quad",
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
					Targets: []gpu.ColorTargetState{{Format: d.SurfaceFormat()}},
				})
				if err != nil {
					t.Fatalf("CreatePipeline: %v", err)
				}
			},
			OnFrame: func(frame *gpu.Frame) {
				pass := frame.BeginRenderPass(gpu.RenderPassDesc{})
				pass.ClearColor(core.RGB(0x000000))
				pass.SetPipeline(pipe)
				pass.SetBindGroup(0, bg)
				pass.SetVertexBuffer(0, vbuf, 0)
				pass.Draw(6, 0)
				pass.End()
				frame.Submit()
			},
			OnDeviceLost: func(ev gpu.DeviceLostEvent) {
				lostEvents = append(lostEvents, ev)
			},
			OnDeviceRestored: func(d *gpu.Device) {
				w, h := d.SurfaceSize()
				restoredSizes = append(restoredSizes, [2]int{w, h})
			},
			OnError: func(err gpu.Error) {
				gpuErrors = append(gpuErrors, err)
			},
		})
	}, nsgotest.Size(40, 80), nsgotest.ManualFrames(), nsgotest.Environment(core.Environment{
		OS:        core.OSAndroid,
		GPU:       true,
		Lifecycle: core.LifecycleActive,
	}))

	// Render portrait frame (40x80).
	app.Advance(16 * time.Millisecond)
	portrait, err := app.GPUReadPixels("id:android-gpu")
	if err != nil {
		t.Fatalf("GPUReadPixels portrait: %v", err)
	}
	if portrait.Bounds().Dx() != 40 || portrait.Bounds().Dy() != 80 {
		t.Fatalf("portrait size = %v, want 40x80", portrait.Bounds())
	}
	if p := portrait.RGBAAt(8, 40); p.G < 250 || p.B > 5 {
		t.Fatalf("portrait left pixel = %+v, want green", p)
	}
	if p := portrait.RGBAAt(32, 40); p.B < 250 || p.G > 5 {
		t.Fatalf("portrait right pixel = %+v, want blue", p)
	}

	// Rotate to landscape (80x40): Android destroys and recreates the EGL surface.
	app.TriggerGPUSurfaceRotation("id:android-gpu", 80, 40)
	if len(lostEvents) != 1 || lostEvents[0].Reason != gpu.ReasonSurfaceDestroyed {
		t.Fatalf("lostEvents = %+v, want ReasonSurfaceDestroyed", lostEvents)
	}
	if len(restoredSizes) != 1 || restoredSizes[0] != [2]int{80, 40} {
		t.Fatalf("restoredSizes = %v, want [[80 40]]", restoredSizes)
	}
	if dev.Epoch() != 2 {
		t.Fatalf("dev.Epoch() = %d, want 2", dev.Epoch())
	}
	// All retained handles must now point to epoch 2 and be valid without app recreation!
	if !vbuf.Valid() || vbuf.Epoch() != 2 || !tex.Valid() || tex.Epoch() != 2 ||
		!samp.Valid() || !bgl.Valid() || !bg.Valid() || !pipe.Valid() || !pipe.Ready() {
		t.Fatalf("expected all retained resources automatically restored to epoch 2")
	}

	// Render landscape frame using the exact same Go handles!
	app.Advance(16 * time.Millisecond)
	landscape, err := app.GPUReadPixels("id:android-gpu")
	if err != nil {
		t.Fatalf("GPUReadPixels landscape: %v", err)
	}
	if landscape.Bounds().Dx() != 80 || landscape.Bounds().Dy() != 40 {
		t.Fatalf("landscape size = %v, want 80x40", landscape.Bounds())
	}
	if p := landscape.RGBAAt(16, 20); p.G < 250 || p.B > 5 {
		t.Fatalf("landscape left pixel = %+v, want green", p)
	}
	if p := landscape.RGBAAt(64, 20); p.B < 250 || p.G > 5 {
		t.Fatalf("landscape right pixel = %+v, want blue", p)
	}

	// Background -> surface lost while backgrounded -> foreground -> restore -> keeps rendering!
	app.SetEnvironment(core.Environment{OS: core.OSAndroid, GPU: true, Lifecycle: core.LifecycleBackground})
	app.TriggerGPUDeviceLostWithReason("id:android-gpu", nsgotest.GPULostSurfaceDestroyed, "Activity paused")
	framesWhileBg := len(app.GPUFrames())
	app.RestoreGPUDevice("id:android-gpu")
	app.Advance(16 * time.Millisecond)
	if len(app.GPUFrames()) != framesWhileBg {
		t.Fatalf("expected zero SubmitGPU while still in LifecycleBackground even after RestoreGPUDevice")
	}
	app.SetEnvironment(core.Environment{OS: core.OSAndroid, GPU: true, Lifecycle: core.LifecycleActive})
	app.Advance(16 * time.Millisecond)
	if len(app.GPUFrames()) <= framesWhileBg {
		t.Fatalf("expected rendering to resume cleanly after Android foreground + restore")
	}
	if dev.Epoch() != 3 || !pipe.Ready() {
		t.Fatalf("expected epoch 3 and ready retained pipeline after second restore, got epoch=%d ready=%v", dev.Epoch(), pipe.Ready())
	}
	if len(gpuErrors) != 0 {
		t.Fatalf("unexpected GPU errors: %+v", gpuErrors)
	}
}

func TestGPUPerDescriptorRetainedOptIn(t *testing.T) {
	var (
		retainedBuf  *gpu.Buffer
		ephemeralBuf *gpu.Buffer
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return gpu.View(cx, gpu.ViewProps{
			TestID: "optin-gpu",
			Style:  core.Style{Width: core.Pt(32), Height: core.Pt(32)},
			OnReady: func(d *gpu.Device) {
				if d.Epoch() > 1 {
					return
				}
				var err error
				retainedBuf, err = d.CreateBuffer(cx, gpu.BufferDesc{
					Label:    "kept",
					Usage:    gpu.UsageVertex,
					Data:     packFloat32s(1, 2, 3),
					Retained: true,
				})
				if err != nil {
					t.Fatalf("CreateBuffer retained: %v", err)
				}
				ephemeralBuf, err = d.CreateBuffer(cx, gpu.BufferDesc{
					Label: "ephemeral",
					Usage: gpu.UsageVertex,
					Data:  packFloat32s(4, 5, 6),
				})
				if err != nil {
					t.Fatalf("CreateBuffer ephemeral: %v", err)
				}
			},
		})
	}, nsgotest.Size(32, 32))

	app.TriggerGPUDeviceLostWithReason("id:optin-gpu", nsgotest.GPULostReset, "reset")
	if retainedBuf.Valid() || ephemeralBuf.Valid() {
		t.Fatalf("expected both buffers invalid while device is lost")
	}
	app.RestoreGPUDevice("id:optin-gpu")
	if !retainedBuf.Valid() || retainedBuf.Epoch() != 2 {
		t.Fatalf("expected Retained buffer restored to epoch 2, got valid=%v epoch=%d", retainedBuf.Valid(), retainedBuf.Epoch())
	}
	if ephemeralBuf.Valid() || ephemeralBuf.Epoch() != 1 {
		t.Fatalf("expected non-retained buffer to remain invalidated on epoch 1, got valid=%v epoch=%d", ephemeralBuf.Valid(), ephemeralBuf.Epoch())
	}
}
