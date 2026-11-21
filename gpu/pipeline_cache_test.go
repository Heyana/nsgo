package gpu_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestPipelineAsyncPrecompilationAndDiskCacheReuse(t *testing.T) {
	cacheDir := t.TempDir()

	// --- Launch 1: cold persistent cache ---
	cache1, err := gpu.NewPipelineCache(cacheDir)
	if err != nil {
		t.Fatalf("NewPipelineCache launch 1: %v", err)
	}
	colorMod1 := gpu.MustCompileWGSL(color3DWGSL)
	texMod1 := gpu.MustCompileWGSL(texturedWGSL)

	ident := [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
	triVerts := packFloat32s(
		-0.9, -0.9, 0.5, 0, 0, 1, 1,
		0.9, -0.9, 0.5, 0, 0, 1, 1,
		0.0, 0.9, 0.5, 0, 0, 1, 1,
	)

	var (
		dev1          *gpu.Device
		vbuf1         *gpu.Buffer
		bg1           *gpu.BindGroup
		asyncPipes1   []*gpu.Pipeline
		precompileEnd bool
		precompileErr error
		readyAtReturn [2]bool
		gpuErrors1    []gpu.Error
	)

	app1 := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return gpu.View(cx, gpu.ViewProps{
			TestID:        "cache-v1",
			Style:         core.Style{Width: core.Pt(48), Height: core.Pt(48)},
			PipelineCache: cache1,
			OnReady: func(d *gpu.Device) {
				dev1 = d
				var err error
				vbuf1, err = d.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: triVerts})
				if err != nil {
					t.Fatalf("CreateBuffer verts: %v", err)
				}
				ubuf, err := d.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: packFloat32s(ident[:]...)})
				if err != nil {
					t.Fatalf("CreateBuffer ubuf: %v", err)
				}
				colorBGL, err := d.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
					{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
				})
				if err != nil {
					t.Fatalf("CreateBindGroupLayout color: %v", err)
				}
				bg1, err = d.CreateBindGroup(cx, colorBGL, []gpu.BindGroupEntry{{Binding: 0, Buffer: ubuf}})
				if err != nil {
					t.Fatalf("CreateBindGroup: %v", err)
				}
				texBGL, err := d.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
					{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
					{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingSampler},
				})
				if err != nil {
					t.Fatalf("CreateBindGroupLayout tex: %v", err)
				}

				descs := []gpu.PipelineDesc{
					{
						Label:    "color-mesh",
						Vertex:   gpu.ShaderStage{Module: colorMod1, EntryPoint: "vs_main"},
						Fragment: gpu.ShaderStage{Module: colorMod1, EntryPoint: "fs_main"},
						Layouts:  []*gpu.BindGroupLayout{colorBGL},
						VertexBuffers: []gpu.VertexBufferLayout{{
							Stride: 7 * 4,
							Attributes: []gpu.VertexAttribute{
								{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
								{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x4},
							},
						}},
						Targets: []gpu.ColorTargetState{{Format: d.SurfaceFormat()}},
					},
					{
						Label:    "textured-quad",
						Vertex:   gpu.ShaderStage{Module: texMod1, EntryPoint: "vs_main"},
						Fragment: gpu.ShaderStage{Module: texMod1, EntryPoint: "fs_main"},
						Layouts:  []*gpu.BindGroupLayout{texBGL},
						VertexBuffers: []gpu.VertexBufferLayout{{
							Stride: 5 * 4,
							Attributes: []gpu.VertexAttribute{
								{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
								{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x2},
							},
						}},
						Targets: []gpu.ColorTargetState{{Format: d.SurfaceFormat()}},
					},
				}

				asyncPipes1 = d.PrecompilePipelines(cx, descs, func(pipes []*gpu.Pipeline, err error) {
					precompileEnd = true
					precompileErr = err
				})
				readyAtReturn[0] = asyncPipes1[0].Ready()
				readyAtReturn[1] = asyncPipes1[1].Ready()
			},
			OnFrame: func(frame *gpu.Frame) {
				pass := frame.BeginRenderPass(gpu.RenderPassDesc{})
				pass.ClearColor(core.RGB(0x0a0e14))
				if len(asyncPipes1) > 0 {
					// Safe even if pipeline is still compiling asynchronously.
					pass.SetPipeline(asyncPipes1[0])
					pass.SetBindGroup(0, bg1)
					pass.SetVertexBuffer(0, vbuf1, 0)
					pass.Draw(3, 0)
				}
				pass.End()
				frame.Submit()
			},
			OnError: func(err gpu.Error) {
				gpuErrors1 = append(gpuErrors1, err)
			},
		})
	}, nsgotest.Size(48, 48), nsgotest.ManualFrames())

	// PrecompilePipelines must return immediately before background compilation finishes.
	if readyAtReturn[0] || readyAtReturn[1] {
		t.Fatalf("expected async pipelines not yet ready synchronously at return, got %v", readyAtReturn)
	}
	if asyncPipes1[0].CacheKey() == "" || asyncPipes1[1].CacheKey() == "" || asyncPipes1[0].CacheKey() == asyncPipes1[1].CacheKey() {
		t.Fatalf("expected distinct non-empty cache keys, got %q and %q", asyncPipes1[0].CacheKey(), asyncPipes1[1].CacheKey())
	}

	// Wait for background precompilation to complete on the engine goroutine.
	app1.WaitUntil(5*time.Second, func() bool { return precompileEnd })
	if precompileErr != nil {
		t.Fatalf("PrecompilePipelines launch 1 error: %v", precompileErr)
	}
	if !asyncPipes1[0].Ready() || !asyncPipes1[1].Ready() {
		t.Fatalf("expected both pipelines Ready() after onDone")
	}

	stats1 := dev1.CacheStats()
	if stats1.Hits != 0 || stats1.Misses != 2 || stats1.Stores != 2 || stats1.Entries != 2 || stats1.Compilations != 2 {
		t.Fatalf("launch 1 cache stats = %+v, want Hits=0 Misses=2 Stores=2 Entries=2 Compilations=2", stats1)
	}

	// Verify persisted files on disk are valid deterministic JSON containing target shaders (zero encoding/gob).
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("ReadDir cacheDir: %v", err)
	}
	var jsonCount int
	for _, de := range entries {
		if !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		jsonCount++
		raw, err := os.ReadFile(filepath.Join(cacheDir, de.Name()))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", de.Name(), err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("cache file %s is not valid JSON: %v", de.Name(), err)
		}
		if doc["version"] != float64(1) {
			t.Fatalf("cache file %s version = %v, want 1", de.Name(), doc["version"])
		}
		arts, ok := doc["artifacts"].(map[string]any)
		if !ok {
			t.Fatalf("cache file %s missing artifacts map", de.Name())
		}
		for _, field := range []string{"msl_source", "hlsl_vertex_source", "glsl_vertex_source", "gles_vertex_source", "wgsl_source"} {
			if s, _ := arts[field].(string); s == "" {
				t.Errorf("cache file %s missing compiled %s", de.Name(), field)
			}
		}
	}
	if jsonCount != 2 {
		t.Fatalf("expected 2 JSON pipeline cache files on disk, found %d", jsonCount)
	}

	// Verify backend binary archive blob persistence (Metal binary archive / Vulkan cache / D3D PSO library).
	blobData := []byte("MOCK_MTL_BINARY_ARCHIVE_V1")
	if err := cache1.PutBackendBlob(asyncPipes1[0].CacheKey(), "metal", blobData); err != nil {
		t.Fatalf("PutBackendBlob: %v", err)
	}

	// Draw first frame on launch 1 and verify pixels.
	app1.Advance(16 * time.Millisecond)
	img1, err := app1.GPUReadPixels("id:cache-v1")
	if err != nil {
		t.Fatalf("GPUReadPixels launch 1: %v", err)
	}
	if c := img1.RGBAAt(24, 24); c.B < 250 || c.R > 5 || c.G > 5 {
		t.Fatalf("launch 1 center pixel = %+v, want blue", c)
	}
	if len(gpuErrors1) != 0 {
		t.Fatalf("launch 1 unexpected GPU errors: %+v", gpuErrors1)
	}

	// Write a corrupt JSON file and an outdated version file to verify graceful load handling.
	if err := os.WriteFile(filepath.Join(cacheDir, "corrupt.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "stale.json"), []byte(`{"version":999,"key":"stale","artifacts":{}}`), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	// --- Launch 2: fresh PipelineCache + fresh Device/App sharing cacheDir ---
	cache2, err := gpu.NewPipelineCache(cacheDir)
	if err != nil {
		t.Fatalf("NewPipelineCache launch 2: %v", err)
	}
	if got := cache2.CacheStats(); got.Entries != 2 || got.Hits != 0 || got.Misses != 0 || got.Compilations != 0 {
		t.Fatalf("launch 2 pre-compile stats = %+v, want Entries=2 Hits=0 Misses=0 Compilations=0", got)
	}
	gotBlob, ok := cache2.GetBackendBlob(asyncPipes1[0].CacheKey(), "metal")
	if !ok || !bytes.Equal(gotBlob, blobData) {
		t.Fatalf("launch 2 GetBackendBlob = (%q, %v), want (%q, true)", gotBlob, ok, blobData)
	}

	colorMod2 := gpu.MustCompileWGSL(color3DWGSL)
	texMod2 := gpu.MustCompileWGSL(texturedWGSL)
	var (
		dev2           *gpu.Device
		vbuf2          *gpu.Buffer
		bg2            *gpu.BindGroup
		asyncPipes2    []*gpu.Pipeline
		precompileEnd2 bool
		precompileErr2 error
	)

	app2 := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return gpu.View(cx, gpu.ViewProps{
			TestID:        "cache-v2",
			Style:         core.Style{Width: core.Pt(48), Height: core.Pt(48)},
			PipelineCache: cache2,
			OnReady: func(d *gpu.Device) {
				dev2 = d
				var err error
				vbuf2, err = d.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: triVerts})
				if err != nil {
					t.Fatalf("CreateBuffer launch 2: %v", err)
				}
				ubuf, err := d.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: packFloat32s(ident[:]...)})
				if err != nil {
					t.Fatalf("CreateBuffer ubuf 2: %v", err)
				}
				colorBGL, err := d.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
					{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
				})
				if err != nil {
					t.Fatalf("CreateBindGroupLayout color 2: %v", err)
				}
				bg2, err = d.CreateBindGroup(cx, colorBGL, []gpu.BindGroupEntry{{Binding: 0, Buffer: ubuf}})
				if err != nil {
					t.Fatalf("CreateBindGroup 2: %v", err)
				}
				texBGL, err := d.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
					{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
					{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingSampler},
				})
				if err != nil {
					t.Fatalf("CreateBindGroupLayout tex 2: %v", err)
				}

				descs := []gpu.PipelineDesc{
					{
						Label:    "color-mesh",
						Vertex:   gpu.ShaderStage{Module: colorMod2, EntryPoint: "vs_main"},
						Fragment: gpu.ShaderStage{Module: colorMod2, EntryPoint: "fs_main"},
						Layouts:  []*gpu.BindGroupLayout{colorBGL},
						VertexBuffers: []gpu.VertexBufferLayout{{
							Stride: 7 * 4,
							Attributes: []gpu.VertexAttribute{
								{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
								{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x4},
							},
						}},
						Targets: []gpu.ColorTargetState{{Format: d.SurfaceFormat()}},
					},
					{
						Label:    "textured-quad",
						Vertex:   gpu.ShaderStage{Module: texMod2, EntryPoint: "vs_main"},
						Fragment: gpu.ShaderStage{Module: texMod2, EntryPoint: "fs_main"},
						Layouts:  []*gpu.BindGroupLayout{texBGL},
						VertexBuffers: []gpu.VertexBufferLayout{{
							Stride: 5 * 4,
							Attributes: []gpu.VertexAttribute{
								{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
								{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x2},
							},
						}},
						Targets: []gpu.ColorTargetState{{Format: d.SurfaceFormat()}},
					},
				}

				asyncPipes2 = d.PrecompilePipelines(cx, descs, func(_ []*gpu.Pipeline, err error) {
					precompileEnd2 = true
					precompileErr2 = err
				})
			},
			OnFrame: func(frame *gpu.Frame) {
				pass := frame.BeginRenderPass(gpu.RenderPassDesc{})
				pass.ClearColor(core.RGB(0x0a0e14))
				if len(asyncPipes2) > 0 {
					pass.SetPipeline(asyncPipes2[0])
					pass.SetBindGroup(0, bg2)
					pass.SetVertexBuffer(0, vbuf2, 0)
					pass.Draw(3, 0)
				}
				pass.End()
				frame.Submit()
			},
		})
	}, nsgotest.Size(48, 48), nsgotest.ManualFrames())

	app2.WaitUntil(5*time.Second, func() bool { return precompileEnd2 })
	if precompileErr2 != nil {
		t.Fatalf("PrecompilePipelines launch 2 error: %v", precompileErr2)
	}
	if asyncPipes2[0].CacheKey() != asyncPipes1[0].CacheKey() {
		t.Fatalf("cache key drifted across launches: %q vs %q", asyncPipes2[0].CacheKey(), asyncPipes1[0].CacheKey())
	}

	// Second launch must hit the cache with ZERO misses and ZERO shader recompilations!
	stats2 := dev2.CacheStats()
	if stats2.Hits != 2 || stats2.Misses != 0 || stats2.Compilations != 0 {
		t.Fatalf("launch 2 cache stats = %+v, want Hits=2 Misses=0 Compilations=0", stats2)
	}

	app2.Advance(16 * time.Millisecond)
	img2, err := app2.GPUReadPixels("id:cache-v2")
	if err != nil {
		t.Fatalf("GPUReadPixels launch 2: %v", err)
	}
	if !bytes.Equal(img1.Pix, img2.Pix) {
		t.Fatalf("launch 2 rendered pixels differ from launch 1")
	}

	// Mutating pipeline render state (e.g. CullMode) must produce a new cache key and miss.
	app2.Post(func() {
		bgl, _ := dev2.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
		})
		pMut, err := dev2.CreatePipeline(nil, gpu.PipelineDesc{
			Label:    "color-mesh-culled",
			Vertex:   gpu.ShaderStage{Module: colorMod2, EntryPoint: "vs_main"},
			Fragment: gpu.ShaderStage{Module: colorMod2, EntryPoint: "fs_main"},
			Layouts:  []*gpu.BindGroupLayout{bgl},
			VertexBuffers: []gpu.VertexBufferLayout{{
				Stride: 7 * 4,
				Attributes: []gpu.VertexAttribute{
					{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
					{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x4},
				},
			}},
			Primitive: gpu.PrimitiveState{CullMode: gpu.CullBack},
			Targets:   []gpu.ColorTargetState{{Format: dev2.SurfaceFormat()}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline mutated: %v", err)
		}
		if pMut.CacheKey() == asyncPipes2[0].CacheKey() {
			t.Fatalf("expected different cache key when CullMode changes")
		}
	})
	app2.Settle()
	if statsMut := cache2.CacheStats(); statsMut.Misses != 1 || statsMut.Compilations != 1 {
		t.Fatalf("mutated pipeline stats = %+v, want Misses=1 Compilations=1", statsMut)
	}
}

func TestPrecompiledPipelineFirstDrawZeroHitch(t *testing.T) {
	cacheDir := t.TempDir()
	cache1, err := gpu.NewPipelineCache(cacheDir)
	if err != nil {
		t.Fatalf("NewPipelineCache: %v", err)
	}
	mod := gpu.MustCompileWGSL(color3DWGSL)
	ident := [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
	triVerts := packFloat32s(
		-0.8, -0.8, 0.5, 0, 1, 0, 1,
		0.8, -0.8, 0.5, 0, 1, 0, 1,
		0.0, 0.8, 0.5, 0, 1, 0, 1,
	)

	// 1. Precompile asynchronously on Launch 1 without blocking the engine goroutine.
	var (
		pipe1           *gpu.Pipeline
		vbuf1           *gpu.Buffer
		bg1             *gpu.BindGroup
		asyncCallDur    time.Duration
		readyBeforeDone bool
		pipe1Ready      bool
		drawEnabled     bool
	)
	app1 := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return gpu.View(cx, gpu.ViewProps{
			TestID:        "hitch-1",
			Style:         core.Style{Width: core.Pt(64), Height: core.Pt(64)},
			PipelineCache: cache1,
			OnReady: func(d *gpu.Device) {
				vbuf1, _ = d.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: triVerts})
				ubuf, _ := d.CreateBuffer(cx, gpu.BufferDesc{Usage: gpu.UsageUniform, Data: packFloat32s(ident[:]...)})
				bgl, _ := d.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
					{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
				})
				bg1, _ = d.CreateBindGroup(cx, bgl, []gpu.BindGroupEntry{{Binding: 0, Buffer: ubuf}})

				t0 := time.Now()
				pipe1 = d.CreatePipelineAsync(cx, gpu.PipelineDesc{
					Label:    "precompiled",
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
				}, func(p *gpu.Pipeline, err error) {
					if err != nil {
						t.Errorf("CreatePipelineAsync failed: %v", err)
						return
					}
					pipe1Ready = p.Ready()
				})
				asyncCallDur = time.Since(t0)
				readyBeforeDone = pipe1.Ready()
			},
			OnFrame: func(frame *gpu.Frame) {
				if !drawEnabled {
					return
				}
				pass := frame.BeginRenderPass(gpu.RenderPassDesc{})
				pass.ClearColor(core.RGB(0x000000))
				pass.SetPipeline(pipe1)
				pass.SetBindGroup(0, bg1)
				pass.SetVertexBuffer(0, vbuf1, 0)
				pass.Draw(3, 0)
				pass.End()
				frame.Submit()
			},
		})
	}, nsgotest.Size(64, 64), nsgotest.ManualFrames())

	if readyBeforeDone {
		t.Fatalf("CreatePipelineAsync must not block or synchronously mark Ready before returning")
	}
	if asyncCallDur > 20*time.Millisecond {
		t.Fatalf("CreatePipelineAsync blocked engine goroutine for %v", asyncCallDur)
	}
	app1.WaitUntil(5*time.Second, func() bool { return pipe1Ready })

	// 2. Measure the very first draw frame of the precompiled pipeline:
	//    zero GPU resource-creation ops, zero shader compilations, zero hitch!
	compilationsBeforeDraw := cache1.CacheStats().Compilations
	framesBeforeDraw := len(app1.GPUFrames())
	drawEnabled = true

	firstDrawStart := time.Now()
	app1.Advance(16 * time.Millisecond)
	firstDrawDur := time.Since(firstDrawStart)

	if cache1.CacheStats().Compilations != compilationsBeforeDraw {
		t.Fatalf("first draw triggered unexpected shader compilation: before=%d after=%d",
			compilationsBeforeDraw, cache1.CacheStats().Compilations)
	}
	framesAfterDraw := app1.GPUFrames()
	if len(framesAfterDraw) != framesBeforeDraw+1 {
		t.Fatalf("expected exactly 1 draw GPUFrame, got %d new frames", len(framesAfterDraw)-framesBeforeDraw)
	}
	firstFrameRec := framesAfterDraw[len(framesAfterDraw)-1]
	if firstFrameRec.OpsCount != 0 {
		t.Fatalf("first draw frame carried %d pipeline/resource ops; precompiled pipeline must already be uploaded ahead of first draw", firstFrameRec.OpsCount)
	}
	if firstFrameRec.DrawsCount != 1 {
		t.Fatalf("first draw frame DrawsCount = %d, want 1", firstFrameRec.DrawsCount)
	}

	// Compare against steady-state frames: no compilation hitch on first draw.
	var steadyTotal time.Duration
	const steadyFrames = 4
	for range steadyFrames {
		s := time.Now()
		app1.Advance(16 * time.Millisecond)
		steadyTotal += time.Since(s)
	}
	steadyAvg := steadyTotal / steadyFrames
	// Guard generously against CI jitter while still catching any real compilation stall.
	maxAllowed := max(50*time.Millisecond, steadyAvg*20)
	if firstDrawDur > maxAllowed {
		t.Fatalf("first draw hitched: firstDraw=%v steadyAvg=%v limit=%v", firstDrawDur, steadyAvg, maxAllowed)
	}

	img, err := app1.GPUReadPixels("id:hitch-1")
	if err != nil {
		t.Fatalf("GPUReadPixels: %v", err)
	}
	if c := img.RGBAAt(32, 32); c.G < 250 || c.R > 5 || c.B > 5 {
		t.Fatalf("first draw pixel = %+v, want green", c)
	}

	// 3. Second standalone Device / launch using the same PipelineCache hits cache with 0 misses.
	cache2, err := gpu.NewPipelineCache(cacheDir)
	if err != nil {
		t.Fatalf("NewPipelineCache launch 2: %v", err)
	}
	sr2 := gpu.NewSoftwareRenderer(64, 64, gpu.FormatUndefined)
	dev2 := sr2.Device()
	dev2.SetPipelineCache(cache2)
	bgl2, err := dev2.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageVertex, Type: gpu.BindingUniform},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout dev2: %v", err)
	}
	pipe2 := dev2.CreatePipelineAsync(nil, gpu.PipelineDesc{
		Label:    "precompiled",
		Vertex:   gpu.ShaderStage{Module: gpu.MustCompileWGSL(color3DWGSL), EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: gpu.MustCompileWGSL(color3DWGSL), EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{bgl2},
		VertexBuffers: []gpu.VertexBufferLayout{{
			Stride: 7 * 4,
			Attributes: []gpu.VertexAttribute{
				{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
				{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x4},
			},
		}},
		Targets: []gpu.ColorTargetState{{Format: dev2.SurfaceFormat()}},
	}, nil)
	<-pipe2.Done()
	if err := pipe2.Err(); err != nil {
		t.Fatalf("second launch async pipeline error: %v", err)
	}
	if !pipe2.Ready() {
		t.Fatalf("expected second launch pipeline Ready()")
	}
	stats2 := dev2.CacheStats()
	if stats2.Hits != 1 || stats2.Misses != 0 || stats2.Compilations != 0 {
		t.Fatalf("second Device cache stats = %+v, want Hits=1 Misses=0 Compilations=0", stats2)
	}

	// Verify invalid descriptor in CreatePipelineAsync reports ErrInvalidDescriptor cleanly.
	var invalidErr error
	bad := dev2.CreatePipelineAsync(nil, gpu.PipelineDesc{Label: "missing-vs"}, func(_ *gpu.Pipeline, err error) {
		invalidErr = err
	})
	if bad != nil {
		t.Fatalf("expected nil *Pipeline on invalid descriptor")
	}
	if invalidErr == nil || !strings.Contains(invalidErr.Error(), "invalid descriptor") {
		t.Fatalf("expected ErrInvalidDescriptor callback, got %v", invalidErr)
	}

	// Verify Clear removes disk entries and resets stats.
	if err := cache2.Clear(); err != nil {
		t.Fatalf("cache2.Clear: %v", err)
	}
	if st := cache2.CacheStats(); st.Entries != 0 || st.Hits != 0 || st.Misses != 0 {
		t.Fatalf("expected zeroed stats after Clear, got %+v", st)
	}
}
