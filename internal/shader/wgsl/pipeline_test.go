package wgsl_test

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/hlsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/msl"
	irwgsl "github.com/nsgo-dev/nsgo/internal/shaderir/wgsl"
)

const litCubeWGSL = `
struct Camera {
	mvp: mat4x4<f32>,
	tint: vec4<f32>,
}

@group(0) @binding(0) var<uniform> cam: Camera;
@group(0) @binding(1) var albedoTex: texture_2d<f32>;
@group(0) @binding(2) var albedoSamp: sampler;

struct VertexIn {
	@location(0) pos: vec3<f32>,
	@location(1) color: vec4<f32>,
	@location(2) uv: vec2<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
	@location(1) uv: vec2<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = cam.mvp * vec4<f32>(in.pos, 1.0);
	out.color = in.color * cam.tint;
	out.uv = in.uv;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	let texel = textureSample(albedoTex, albedoSamp, in.uv);
	return in.color * texel;
}
`

func TestCompilePipelineAndEmitters(t *testing.T) {
	prog, err := wgsl.CompilePipeline(litCubeWGSL)
	if err != nil {
		t.Fatalf("CompilePipeline: %v", err)
	}
	ir := prog.IR
	if len(ir.Attributes) != 3 {
		t.Fatalf("attributes = %d, want 3", len(ir.Attributes))
	}
	if len(ir.Varyings) != 2 {
		t.Fatalf("varyings = %d, want 2", len(ir.Varyings))
	}
	if len(ir.Bindings) != 3 {
		t.Fatalf("bindings = %d, want 3", len(ir.Bindings))
	}
	if ir.Bindings[0].Kind != shaderir.BindingUniformBuffer || ir.Bindings[0].Group != 0 || ir.Bindings[0].Binding != 0 {
		t.Errorf("unexpected binding 0: %+v", ir.Bindings[0])
	}
	if ir.Bindings[0].Type.SizeBytes() != 80 {
		t.Errorf("Camera struct SizeBytes = %d, want 80", ir.Bindings[0].Type.SizeBytes())
	}

	// MSL emitter
	mslOut, err := msl.CompilePipeline(ir)
	if err != nil {
		t.Fatalf("msl.CompilePipeline: %v", err)
	}
	for _, want := range []string{"vertex StageOut vs_main", "fragment float4 fs_main", "[[attribute(0)]]", "[[position]]", "[[buffer(8)]]", "[[texture(0)]]", "[[sampler(0)]]", ".sample("} {
		if !strings.Contains(mslOut.Source, want) {
			t.Errorf("MSL missing %q:\n%s", want, mslOut.Source)
		}
	}

	// HLSL emitter
	hlslOut, err := hlsl.CompilePipeline(ir)
	if err != nil {
		t.Fatalf("hlsl.CompilePipeline: %v", err)
	}
	for _, want := range []string{"SV_POSITION", "TEXCOORD0", "register(b0)", "register(t0)", "register(s0)", "mul("} {
		if !strings.Contains(hlslOut.VertexSource, want) {
			t.Errorf("HLSL VS missing %q:\n%s", want, hlslOut.VertexSource)
		}
	}
	if !strings.Contains(hlslOut.PixelSource, ".Sample(") {
		t.Errorf("HLSL PS missing .Sample(:\n%s", hlslOut.PixelSource)
	}

	// GLSL ES 3.00 emitter (including WebGPU [0,1] -> OpenGL [-1,1] clip-space Z remap and combined sampler pair table)
	glslOut, err := glsl.CompilePipeline(ir, glsl.GLSLVersionES300)
	if err != nil {
		t.Fatalf("glsl.CompilePipeline: %v", err)
	}
	if !strings.Contains(glslOut.VertexSource, "gl_Position.z = gl_Position.z * 2.0 - gl_Position.w;") {
		t.Errorf("GLSL VS missing depth remap:\n%s", glslOut.VertexSource)
	}
	if !strings.Contains(glslOut.VertexSource, "layout(location = 0) in vec3 a0;") {
		t.Errorf("GLSL VS missing attribute layout:\n%s", glslOut.VertexSource)
	}
	if len(glslOut.SamplerPairs) != 1 || glslOut.SamplerPairs[0].TextureBinding != 1 || glslOut.SamplerPairs[0].SamplerBinding != 2 {
		t.Errorf("GLSL sampler pairs = %+v", glslOut.SamplerPairs)
	}

	// WGSL emitter
	wgslOut, err := irwgsl.CompilePipeline(ir)
	if err != nil {
		t.Fatalf("irwgsl.CompilePipeline: %v", err)
	}
	for _, want := range []string{"@group(0) @binding(0) var<uniform>", "@vertex fn vs_main", "@fragment fn fs_main", "textureSample("} {
		if !strings.Contains(wgslOut, want) {
			t.Errorf("WGSL missing %q:\n%s", want, wgslOut)
		}
	}
}
