//go:build nsgodev || nsgotest

package gpu_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
)

func TestDebugLabelsAndGroupsRecordedInDevOrTestBuild(t *testing.T) {
	if !gpu.DebugLabelsEnabled {
		t.Fatalf("expected DebugLabelsEnabled == true under nsgodev/nsgotest")
	}

	sr := gpu.NewSoftwareRenderer(16, 16, gpu.FormatDepth24Plus)
	dev := sr.Device()

	mod, err := gpu.CompileShader(gpu.ShaderDesc{
		Label:  "shader.textured",
		Source: texturedWGSL,
	})
	if err != nil {
		t.Fatalf("CompileShader: %v", err)
	}
	if mod.Label() != "shader.textured" {
		t.Errorf("ShaderModule.Label() = %q, want shader.textured", mod.Label())
	}

	buf, err := dev.CreateBuffer(nil, gpu.BufferDesc{
		Label: "buf.quad",
		Usage: gpu.UsageVertex,
		Data:  make([]byte, 20),
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	tex, err := dev.CreateTexture(nil, gpu.TextureDesc{
		Label:  "tex.albedo",
		Width:  4,
		Height: 4,
		Format: gpu.FormatRGBA8Unorm,
		Usage:  gpu.TextureSampled | gpu.TextureCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	if tex.Desc().Label != "tex.albedo" {
		t.Errorf("tex.Desc().Label = %q, want tex.albedo", tex.Desc().Label)
	}
	rt, err := dev.CreateRenderTarget(nil, gpu.RenderTargetDesc{
		Label:       "rt.shadow",
		Width:       8,
		Height:      8,
		ColorFormat: gpu.FormatRGBA8Unorm,
		DepthFormat: gpu.FormatDepth24Plus,
	})
	if err != nil {
		t.Fatalf("CreateRenderTarget: %v", err)
	}
	samp, err := dev.CreateSampler(nil, gpu.SamplerDesc{
		Label:     "samp.linear",
		MinFilter: gpu.FilterLinear,
	})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}
	bgl, err := dev.CreateBindGroupLayoutDesc(nil, gpu.BindGroupLayoutDesc{
		Label: "bgl.material",
		Entries: []gpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
			{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingSampler},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayoutDesc: %v", err)
	}
	bg, err := dev.CreateBindGroupDesc(nil, gpu.BindGroupDesc{
		Label:  "bg.material0",
		Layout: bgl,
		Entries: []gpu.BindGroupEntry{
			{Binding: 0, Texture: tex},
			{Binding: 1, Sampler: samp},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupDesc: %v", err)
	}
	pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
		Label:    "pipe.textured",
		Vertex:   gpu.ShaderStage{Module: mod, EntryPoint: "vs_main"},
		Fragment: gpu.ShaderStage{Module: mod, EntryPoint: "fs_main"},
		Layouts:  []*gpu.BindGroupLayout{bgl},
		Targets:  []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm}},
	})
	if err != nil {
		t.Fatalf("CreatePipeline: %v", err)
	}

	frame := dev.BeginFrame(time.Now())
	frame.PushDebugGroup("FrameRoot")
	frame.InsertDebugMarker("StartMain")
	pass := frame.BeginRenderPass(gpu.RenderPassDesc{Label: "PassMain"})
	pass.ClearColor(core.RGB(0x010203))
	pass.PushDebugGroup("OpaqueObjects")
	pass.InsertDebugMarker("Mesh0")
	pass.PopDebugGroup()
	pass.End()
	frame.PopDebugGroup()
	frame.Submit()

	wantLabels := map[uint64]string{
		buf.ID():      "buf.quad",
		tex.ID():      "tex.albedo",
		rt.Color.ID(): "rt.shadow.color",
		rt.Depth.ID(): "rt.shadow.depth",
		samp.ID():     "samp.linear",
		bgl.ID():      "bgl.material",
		bg.ID():       "bg.material0",
		pipe.ID():     "pipe.textured",
	}
	for id, want := range wantLabels {
		if got := sr.ResourceLabel(id); got != want {
			t.Errorf("ResourceLabel(%d) = %q, want %q", id, got, want)
		}
	}

	events := sr.DebugEvents()
	wantSequence := []gpu.DebugEvent{
		{Kind: "buffer", Label: "buf.quad", Depth: 0},
		{Kind: "texture", Label: "tex.albedo", Depth: 0},
		{Kind: "texture", Label: "rt.shadow.color", Depth: 0},
		{Kind: "texture", Label: "rt.shadow.depth", Depth: 0},
		{Kind: "sampler", Label: "samp.linear", Depth: 0},
		{Kind: "bind_group_layout", Label: "bgl.material", Depth: 0},
		{Kind: "bind_group", Label: "bg.material0", Depth: 0},
		{Kind: "pipeline", Label: "pipe.textured", Depth: 0},
		{Kind: "shader", Label: "shader.textured", Depth: 0},
		{Kind: "push_group", Label: "FrameRoot", Depth: 0},
		{Kind: "marker", Label: "StartMain", Depth: 1},
		{Kind: "pass", Label: "PassMain", Depth: 1},
		{Kind: "pass_push_group", Label: "OpaqueObjects", Depth: 1},
		{Kind: "pass_marker", Label: "Mesh0", Depth: 2},
		{Kind: "pass_pop_group", Label: "", Depth: 1},
		{Kind: "pop_group", Label: "", Depth: 0},
	}
	if len(events) != len(wantSequence) {
		t.Fatalf("DebugEvents len = %d\ngot:  %+v\nwant: %+v", len(events), events, wantSequence)
	}
	for i, w := range wantSequence {
		if events[i] != w {
			t.Errorf("DebugEvents[%d] = %+v, want %+v", i, events[i], w)
		}
	}
}
