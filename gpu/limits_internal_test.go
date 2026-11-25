package gpu

import (
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const mrtLimitWGSL = `
struct FragOut {
	@location(0) a: vec4<f32>,
	@location(1) b: vec4<f32>,
}

@vertex
fn vs_main(@location(0) pos: vec3<f32>) -> @builtin(position) vec4<f32> {
	return vec4<f32>(pos, 1.0);
}

@fragment
fn fs_main() -> FragOut {
	var out: FragOut;
	out.a = vec4<f32>(1.0, 0.0, 0.0, 1.0);
	out.b = vec4<f32>(0.0, 1.0, 0.0, 1.0);
	return out;
}
`

// TestSingleColorTargetLimitRejectsMRT checks the limit native backends report until they wire multiple render targets.
func TestSingleColorTargetLimitRejectsMRT(t *testing.T) {
	sr := NewSoftwareRenderer(8, 8, FormatUndefined)
	dev := sr.Device()
	dev.limits.MaxColorAttachments = 1
	mod := MustCompileWGSL(mrtLimitWGSL)
	desc := PipelineDesc{
		Vertex:        ShaderStage{Module: mod, EntryPoint: "vs_main"},
		Fragment:      ShaderStage{Module: mod, EntryPoint: "fs_main"},
		VertexBuffers: []VertexBufferLayout{{Stride: 12, Attributes: []VertexAttribute{{Location: 0, Format: VertexFloat32x3}}}},
		Targets:       []ColorTargetState{{Format: FormatRGBA8Unorm}, {Format: FormatRGBA8Unorm}},
	}
	if _, err := dev.CreatePipeline(nil, desc); !errors.Is(err, ErrInvalidDescriptor) {
		t.Fatalf("CreatePipeline with 2 targets on a 1-target device = %v, want ErrInvalidDescriptor", err)
	}
	unreported := driver.DefaultGPULimits()
	unreported.MaxColorAttachments = 0
	if got := limitsFromDriver(unreported).MaxColorAttachments; got != 1 {
		t.Fatalf("unreported MaxColorAttachments = %d, want 1", got)
	}
}

// A bind group layout opts into restore by itself, as the other resources do (#2338).
func TestRetainedBindGroupLayoutIsRestored(t *testing.T) {
	d := newDevice(nil, nil, 1, 64, 64, FormatRGBA8Unorm, ColorSpaceDefault, 2, Limits{}, Features{}, nil)
	kept, err := d.CreateBindGroupLayoutDesc(nil, BindGroupLayoutDesc{Retained: true})
	if err != nil {
		t.Fatalf("CreateBindGroupLayoutDesc retained: %v", err)
	}
	plain, err := d.CreateBindGroupLayoutDesc(nil, BindGroupLayoutDesc{})
	if err != nil {
		t.Fatalf("CreateBindGroupLayoutDesc: %v", err)
	}
	next := newDevice(nil, nil, 2, 64, 64, FormatRGBA8Unorm, ColorSpaceDefault, 2, Limits{}, Features{}, nil)
	next.restoreRetainedFrom(d)
	if len(next.retainedLayouts) != 1 || next.retainedLayouts[0] != kept {
		t.Fatalf("restored layouts = %v, want the retained one alone", next.retainedLayouts)
	}
	if !kept.Valid() {
		t.Fatal("the retained layout is not valid on the new device")
	}
	if plain.dev != d {
		t.Fatal("a layout that asked for nothing was restored")
	}
}

// A retained bind group layout keeps its debug label through a restore (#2338).
func TestRetainedBindGroupLayoutKeepsItsLabel(t *testing.T) {
	d := newDevice(nil, nil, 1, 64, 64, FormatRGBA8Unorm, ColorSpaceDefault, 2, Limits{}, Features{}, nil)
	kept, err := d.CreateBindGroupLayoutDesc(nil, BindGroupLayoutDesc{Label: "frame layout", Retained: true})
	if err != nil {
		t.Fatalf("CreateBindGroupLayoutDesc: %v", err)
	}
	next := newDevice(nil, nil, 2, 64, 64, FormatRGBA8Unorm, ColorSpaceDefault, 2, Limits{}, Features{}, nil)
	next.restoreRetainedFrom(d)
	if len(next.retainedLayouts) != 1 || next.retainedLayouts[0] != kept {
		t.Fatalf("restored layouts = %v, want the retained one", next.retainedLayouts)
	}
	if kept.label != "frame layout" {
		t.Fatalf("the restored layout's label = %q, want frame layout", kept.label)
	}
	for _, op := range next.pendingOps {
		if op.Kind == driver.GPUOpCreateBindGroupLayout && op.BindGroupLayout != nil && op.BindGroupLayout.Label != debugLabel("frame layout") {
			t.Fatalf("the restored descriptor's label = %q", op.BindGroupLayout.Label)
		}
	}
}
