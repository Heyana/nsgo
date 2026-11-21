//go:build !nsgodev && !nsgotest

package gpu_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
)

func TestDebugLabelsStrippedAtZeroCostInReleaseBuild(t *testing.T) {
	if gpu.DebugLabelsEnabled {
		t.Fatalf("expected DebugLabelsEnabled == false in release build (!nsgodev && !nsgotest)")
	}

	sr := gpu.NewSoftwareRenderer(16, 16, gpu.FormatDepth24Plus)
	dev := sr.Device()

	mod, err := gpu.CompileShader(gpu.ShaderDesc{
		Label:  "shader.secret",
		Source: texturedWGSL,
	})
	if err != nil {
		t.Fatalf("CompileShader: %v", err)
	}
	if mod.Label() != "" {
		t.Errorf("release ShaderModule.Label() = %q, want empty", mod.Label())
	}

	buf, err := dev.CreateBuffer(nil, gpu.BufferDesc{Label: "buf.secret", Usage: gpu.UsageVertex, Data: make([]byte, 20)})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	tex, err := dev.CreateTexture(nil, gpu.TextureDesc{
		Label:  "tex.secret",
		Width:  4,
		Height: 4,
		Format: gpu.FormatRGBA8Unorm,
		Usage:  gpu.TextureSampled | gpu.TextureCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	if tex.Desc().Label != "" {
		t.Errorf("release tex.Desc().Label = %q, want empty", tex.Desc().Label)
	}
	rt, err := dev.CreateRenderTarget(nil, gpu.RenderTargetDesc{
		Label:       "rt.secret",
		Width:       8,
		Height:      8,
		ColorFormat: gpu.FormatRGBA8Unorm,
		DepthFormat: gpu.FormatDepth24Plus,
	})
	if err != nil {
		t.Fatalf("CreateRenderTarget: %v", err)
	}
	samp, err := dev.CreateSampler(nil, gpu.SamplerDesc{Label: "samp.secret"})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}
	bgl, err := dev.CreateBindGroupLayoutDesc(nil, gpu.BindGroupLayoutDesc{
		Label: "bgl.secret",
		Entries: []gpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
			{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingSampler},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayoutDesc: %v", err)
	}
	bg, err := dev.CreateBindGroupDesc(nil, gpu.BindGroupDesc{
		Label:  "bg.secret",
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
		Label:    "pipe.secret",
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
	frame.InsertDebugMarker("MarkerA")
	pass := frame.BeginRenderPass(gpu.RenderPassDesc{Label: "PassSecret"})
	pass.ClearColor(core.RGB(0x010203))
	pass.PushDebugGroup("NestedGroup")
	pass.InsertDebugMarker("Draw0")
	pass.PopDebugGroup()
	pass.End()
	frame.PopDebugGroup()
	frame.Submit()

	for _, id := range []uint64{buf.ID(), tex.ID(), rt.Color.ID(), rt.Depth.ID(), samp.ID(), bgl.ID(), bg.ID(), pipe.ID()} {
		if got := sr.ResourceLabel(id); got != "" {
			t.Errorf("release ResourceLabel(%d) = %q, want empty", id, got)
		}
	}
	if events := sr.DebugEvents(); len(events) != 0 {
		t.Errorf("release DebugEvents = %+v, want empty", events)
	}

	// Verify zero heap allocations for debug group & marker calls in release builds.
	enc := dev.BeginEncoder()
	p := enc.BeginRenderPass(gpu.RenderPassDesc{Label: "NoAllocPass"})
	allocs := testing.AllocsPerRun(100, func() {
		enc.PushDebugGroup("Outer")
		enc.InsertDebugMarker("Mark")
		p.PushDebugGroup("Inner")
		p.InsertDebugMarker("PassMark")
		p.PopDebugGroup()
		enc.PopDebugGroup()
	})
	p.End()
	if allocs != 0 {
		t.Errorf("release debug group/marker allocs = %v, want 0", allocs)
	}
}
