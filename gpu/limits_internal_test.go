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
