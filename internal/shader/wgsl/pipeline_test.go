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

const shadowReceiverWGSL = `
struct ShadowUniforms {
	mvp: mat4x4<f32>,
	lightVP: mat4x4<f32>,
}

@group(0) @binding(0) var<uniform> u: ShadowUniforms;
@group(0) @binding(1) var shadowMap: texture_depth_2d;
@group(0) @binding(2) var shadowSamp: sampler_comparison;

struct VertexIn {
	@location(0) pos: vec3<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) lightClip: vec4<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = u.mvp * vec4<f32>(in.pos, 1.0);
	out.lightClip = u.lightVP * vec4<f32>(in.pos, 1.0);
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	let ndc = in.lightClip.xyz / in.lightClip.w;
	let uv = vec2<f32>(ndc.x * 0.5 + 0.5, 0.5 - ndc.y * 0.5);
	let s0 = textureSampleCompare(shadowMap, shadowSamp, uv, ndc.z);
	let s1 = textureSampleCompareLevel(shadowMap, shadowSamp, uv, ndc.z);
	let vis = (s0 + s1) * 0.5;
	return vec4<f32>(vis, vis, vis, 1.0);
}
`

const depthOnlyShadowWGSL = `
struct LightUniforms {
	lightVP: mat4x4<f32>,
}

@group(0) @binding(0) var<uniform> u: LightUniforms;

struct VertexIn {
	@location(0) pos: vec3<f32>,
}

@vertex
fn vs_shadow(in: VertexIn) -> @builtin(position) vec4<f32> {
	return u.lightVP * vec4<f32>(in.pos, 1.0);
}
`

func TestDepthTextureComparisonSamplersAndEmitters(t *testing.T) {
	prog, err := wgsl.CompilePipeline(shadowReceiverWGSL)
	if err != nil {
		t.Fatalf("CompilePipeline shadowReceiverWGSL: %v", err)
	}
	ir := prog.IR
	if len(ir.Bindings) != 3 {
		t.Fatalf("bindings = %d, want 3", len(ir.Bindings))
	}
	if ir.Bindings[1].Kind != shaderir.BindingTexture || ir.Bindings[1].Type.Main != shaderir.TextureDepth2D {
		t.Fatalf("binding 1 = %+v, want TextureDepth2D", ir.Bindings[1])
	}
	if ir.Bindings[2].Kind != shaderir.BindingSampler || ir.Bindings[2].Type.Main != shaderir.SamplerComparison {
		t.Fatalf("binding 2 = %+v, want SamplerComparison", ir.Bindings[2])
	}

	// MSL emitter: depth2d<float>, sampler, .sample_compare(..., level(0))
	mslOut, err := msl.CompilePipeline(ir)
	if err != nil {
		t.Fatalf("msl.CompilePipeline: %v", err)
	}
	for _, want := range []string{"depth2d<float>", "sampler B2 [[sampler(0)]]", ".sample_compare(B2,", "level(0)"} {
		if !strings.Contains(mslOut.Source, want) {
			t.Errorf("MSL missing %q:\n%s", want, mslOut.Source)
		}
	}

	// HLSL emitter: Texture2D, SamplerComparisonState, SampleCmp + SampleCmpLevelZero
	hlslOut, err := hlsl.CompilePipeline(ir)
	if err != nil {
		t.Fatalf("hlsl.CompilePipeline: %v", err)
	}
	for _, want := range []string{"Texture2D B1 : register(t0);", "SamplerComparisonState B2 : register(s0);", ".SampleCmp(B2,", ".SampleCmpLevelZero(B2,"} {
		if !strings.Contains(hlslOut.PixelSource, want) {
			t.Errorf("HLSL PS missing %q:\n%s", want, hlslOut.PixelSource)
		}
	}

	// GLSL ES 3.00 emitter: precision highp sampler2DShadow, uniform sampler2DShadow, texture(s, vec3(uv, ref)), textureGrad(s, vec3(uv, ref), ...)
	glslES, err := glsl.CompilePipeline(ir, glsl.GLSLVersionES300)
	if err != nil {
		t.Fatalf("glsl.CompilePipeline ES300: %v", err)
	}
	for _, want := range []string{
		"precision highp sampler2DShadow;",
		"uniform sampler2DShadow S_t1_s2;",
		"texture(S_t1_s2, vec3(",
		"textureGrad(S_t1_s2, vec3(",
	} {
		if !strings.Contains(glslES.FragmentSource, want) {
			t.Errorf("GLSL ES 3.00 FS missing %q:\n%s", want, glslES.FragmentSource)
		}
	}
	if len(glslES.SamplerPairs) != 1 || !glslES.SamplerPairs[0].Comparison || !glslES.SamplerPairs[0].HasSampler || glslES.SamplerPairs[0].TextureType != shaderir.TextureDepth2D {
		t.Errorf("GLSL ES 3.00 SamplerPairs = %+v, want 1 comparison TextureDepth2D pair", glslES.SamplerPairs)
	}

	// GLSL 3.30 Core emitter
	glsl330, err := glsl.CompilePipeline(ir, glsl.GLSLVersionDefault)
	if err != nil {
		t.Fatalf("glsl.CompilePipeline 330: %v", err)
	}
	for _, want := range []string{
		"#version 330 core",
		"uniform sampler2DShadow S_t1_s2;",
		"texture(S_t1_s2, vec3(",
		"textureGrad(S_t1_s2, vec3(",
	} {
		if !strings.Contains(glsl330.FragmentSource, want) {
			t.Errorf("GLSL 3.30 FS missing %q:\n%s", want, glsl330.FragmentSource)
		}
	}

	// WGSL emitter + round-trip re-compilation
	wgslOut, err := irwgsl.CompilePipeline(ir)
	if err != nil {
		t.Fatalf("irwgsl.CompilePipeline: %v", err)
	}
	for _, want := range []string{
		"var B1: texture_depth_2d;",
		"var B2: sampler_comparison;",
		"textureSampleCompare(B1, B2,",
		"textureSampleCompareLevel(B1, B2,",
	} {
		if !strings.Contains(wgslOut, want) {
			t.Errorf("WGSL missing %q:\n%s", want, wgslOut)
		}
	}
	if _, err := wgsl.CompilePipeline(wgslOut); err != nil {
		t.Errorf("WGSL round-trip CompilePipeline failed: %v\n%s", err, wgslOut)
	}
}

func TestDepthTextureRawSampleLoadAndOffsets(t *testing.T) {
	const src = `
@group(0) @binding(0) var depthTex: texture_depth_2d;
@group(0) @binding(1) var rawSamp: sampler;
@group(0) @binding(2) var cmpSamp: sampler_comparison;

@fragment
fn fs_main(@location(0) uv: vec2<f32>) -> @location(0) vec4<f32> {
	let d0 = textureSample(depthTex, rawSamp, uv);
	let d1 = textureSampleLevel(depthTex, rawSamp, uv, 0);
	let d2 = textureLoad(depthTex, vec2<i32>(1, 2), 0);
	let c0 = textureSampleCompare(depthTex, cmpSamp, uv, d0, vec2<i32>(1, -1));
	let c1 = textureSampleCompareLevel(depthTex, cmpSamp, uv, d1, vec2<i32>(-1, 1));
	return vec4<f32>(d0 + d1 + d2, c0, c1, 1.0);
}
`
	prog, err := wgsl.CompilePipeline(src)
	if err != nil {
		t.Fatalf("CompilePipeline depth raw/offset: %v", err)
	}

	mslOut, err := msl.CompilePipeline(prog.IR)
	if err != nil {
		t.Fatalf("msl.CompilePipeline: %v", err)
	}
	for _, want := range []string{"depth2d<float>", ".sample(B1,", ".read(uint2(", ".sample_compare(B2,", "level(0),"} {
		if !strings.Contains(mslOut.Source, want) {
			t.Errorf("MSL missing %q:\n%s", want, mslOut.Source)
		}
	}

	hlslOut, err := hlsl.CompilePipeline(prog.IR)
	if err != nil {
		t.Fatalf("hlsl.CompilePipeline: %v", err)
	}
	for _, want := range []string{".Sample(B1,", ").x", ".SampleLevel(B1,", ".Load(int3(", ".SampleCmp(B2,", ".SampleCmpLevelZero(B2,"} {
		if !strings.Contains(hlslOut.PixelSource, want) {
			t.Errorf("HLSL PS missing %q:\n%s", want, hlslOut.PixelSource)
		}
	}

	glslES, err := glsl.CompilePipeline(prog.IR, glsl.GLSLVersionES300)
	if err != nil {
		t.Fatalf("glsl.CompilePipeline ES300: %v", err)
	}
	for _, want := range []string{
		"precision highp sampler2DShadow;",
		"uniform sampler2D S_t0_s1;",
		"uniform sampler2D S_t0_raw;",
		"uniform sampler2DShadow S_t0_s2;",
		"texture(S_t0_s1,",
		").r",
		"textureLod(S_t0_s1,",
		"texelFetch(S_t0_raw,",
		"textureOffset(S_t0_s2, vec3(",
		"textureGradOffset(S_t0_s2, vec3(",
	} {
		if !strings.Contains(glslES.FragmentSource, want) {
			t.Errorf("GLSL ES 3.00 FS missing %q:\n%s", want, glslES.FragmentSource)
		}
	}
	if len(glslES.SamplerPairs) != 3 {
		t.Fatalf("SamplerPairs = %d, want 3: %+v", len(glslES.SamplerPairs), glslES.SamplerPairs)
	}
	if glslES.SamplerPairs[1].HasSampler {
		t.Errorf("expected raw texelFetch pair HasSampler=false, got %+v", glslES.SamplerPairs[1])
	}
	if !glslES.SamplerPairs[2].Comparison || !glslES.SamplerPairs[2].HasSampler {
		t.Errorf("expected comparison pair Comparison=true HasSampler=true, got %+v", glslES.SamplerPairs[2])
	}

	wgslOut, err := irwgsl.CompilePipeline(prog.IR)
	if err != nil {
		t.Fatalf("irwgsl.CompilePipeline: %v", err)
	}
	if _, err := wgsl.CompilePipeline(wgslOut); err != nil {
		t.Errorf("WGSL round-trip failed: %v\n%s", err, wgslOut)
	}
}

func TestDepthOnlyVertexPipelineAndComparisonTypeErrors(t *testing.T) {
	prog, err := wgsl.CompilePipeline(depthOnlyShadowWGSL)
	if err != nil {
		t.Fatalf("CompilePipeline depthOnlyShadowWGSL: %v", err)
	}
	if prog.IR.VertexFunc.Block == nil || prog.IR.FragmentFunc.Block != nil {
		t.Fatalf("expected vertex-only IR, got vertex=%v fragment=%v", prog.IR.VertexFunc.Block != nil, prog.IR.FragmentFunc.Block != nil)
	}
	if len(prog.VertexEntries) != 1 || prog.VertexEntries[0] != "vs_shadow" || len(prog.FragmentEntries) != 0 {
		t.Fatalf("entries = vs:%v fs:%v, want [vs_shadow] / []", prog.VertexEntries, prog.FragmentEntries)
	}

	mslOut, err := msl.CompilePipeline(prog.IR)
	if err != nil || !strings.Contains(mslOut.Source, "vertex StageOut vs_shadow") || strings.Contains(mslOut.Source, "fragment ") {
		t.Errorf("unexpected depth-only MSL (err=%v):\n%s", err, mslOut.Source)
	}
	hlslOut, err := hlsl.CompilePipeline(prog.IR)
	if err != nil || hlslOut.VertexSource == "" || hlslOut.PixelSource != "" {
		t.Errorf("unexpected depth-only HLSL (err=%v): vs=%d ps=%d", err, len(hlslOut.VertexSource), len(hlslOut.PixelSource))
	}
	glslOut, err := glsl.CompilePipeline(prog.IR, glsl.GLSLVersionES300)
	if err != nil || glslOut.VertexSource == "" || glslOut.FragmentSource != "" {
		t.Errorf("unexpected depth-only GLSL (err=%v): vs=%d fs=%d", err, len(glslOut.VertexSource), len(glslOut.FragmentSource))
	}
	wgslOut, err := irwgsl.CompilePipeline(prog.IR)
	if err != nil || !strings.Contains(wgslOut, "@vertex fn vs_shadow") || strings.Contains(wgslOut, "@fragment") {
		t.Errorf("unexpected depth-only WGSL (err=%v):\n%s", err, wgslOut)
	}

	// Reject textureSampleCompare when given a non-depth texture or non-comparison sampler.
	badTex := `
@group(0) @binding(0) var tex: texture_2d<f32>;
@group(0) @binding(1) var samp: sampler_comparison;
@fragment fn fs_main(@location(0) uv: vec2<f32>) -> @location(0) vec4<f32> {
	let s = textureSampleCompare(tex, samp, uv, 0.5);
	return vec4<f32>(s, s, s, 1.0);
}
`
	if _, err := wgsl.CompilePipeline(badTex); err == nil {
		t.Errorf("expected error for textureSampleCompare with texture_2d<f32>")
	}

	badSamp := `
@group(0) @binding(0) var tex: texture_depth_2d;
@group(0) @binding(1) var samp: sampler;
@fragment fn fs_main(@location(0) uv: vec2<f32>) -> @location(0) vec4<f32> {
	let s = textureSampleCompare(tex, samp, uv, 0.5);
	return vec4<f32>(s, s, s, 1.0);
}
`
	if _, err := wgsl.CompilePipeline(badSamp); err == nil {
		t.Errorf("expected error for textureSampleCompare with non-comparison sampler")
	}
}

const instancedWGSL = `
struct VertexIn {
	@location(0) pos: vec2<f32>,
	@location(1) instOffset: vec2<f32>,
	@location(2) instColor: vec4<f32>,
	@builtin(instance_index) instIdx: u32,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
	@location(1) @interpolate(flat) id: u32,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	let boost = f32(in.instIdx % 4u) * 0.01;
	out.pos = vec4<f32>(in.pos + in.instOffset, 0.5, 1.0);
	out.color = in.instColor + vec4<f32>(boost, 0.0, 0.0, 0.0);
	out.id = in.instIdx;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	return in.color;
}
`

const instancedParamWGSL = `
struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
}

@vertex
fn vs_main(
	@location(0) pos: vec2<f32>,
	@builtin(instance_index) idx: u32,
) -> VertexOut {
	var out: VertexOut;
	out.pos = vec4<f32>(pos, 0.5, 1.0);
	out.color = vec4<f32>(f32(idx) / 100.0, 0.0, 1.0, 1.0);
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	return in.color;
}
`

func TestCompileInstancedPipelineEmitters(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{"struct_builtin_instance_index", instancedWGSL, 3},
		{"param_builtin_instance_index", instancedParamWGSL, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := wgsl.CompilePipeline(tc.src)
			if err != nil {
				t.Fatalf("CompilePipeline: %v", err)
			}
			ir := prog.IR
			if len(ir.Attributes) != tc.want {
				t.Fatalf("attributes = %d, want %d (@builtin(instance_index) must not occupy an attribute slot)", len(ir.Attributes), tc.want)
			}

			mslOut, err := msl.CompilePipeline(ir)
			if err != nil {
				t.Fatalf("msl.CompilePipeline: %v", err)
			}
			for _, want := range []string{"[[instance_id]]", "__instance_id"} {
				if !strings.Contains(mslOut.Source, want) {
					t.Errorf("MSL missing %q:\n%s", want, mslOut.Source)
				}
			}

			hlslOut, err := hlsl.CompilePipeline(ir)
			if err != nil {
				t.Fatalf("hlsl.CompilePipeline: %v", err)
			}
			for _, want := range []string{"SV_InstanceID", "__in.__instance_id"} {
				if !strings.Contains(hlslOut.VertexSource, want) {
					t.Errorf("HLSL VS missing %q:\n%s", want, hlslOut.VertexSource)
				}
			}

			for _, ver := range []glsl.GLSLVersion{glsl.GLSLVersionDefault, glsl.GLSLVersionES300} {
				glslOut, err := glsl.CompilePipeline(ir, ver)
				if err != nil {
					t.Fatalf("glsl.CompilePipeline(%v): %v", ver, err)
				}
				if !strings.Contains(glslOut.VertexSource, "uint(gl_InstanceID)") {
					t.Errorf("GLSL(%v) VS missing uint(gl_InstanceID):\n%s", ver, glslOut.VertexSource)
				}
			}

			wgslOut, err := irwgsl.CompilePipeline(ir)
			if err != nil {
				t.Fatalf("irwgsl.CompilePipeline: %v", err)
			}
			for _, want := range []string{"@builtin(instance_index) nsgo_instance_id: u32", "nsgo_instance_id"} {
				if !strings.Contains(wgslOut, want) {
					t.Errorf("WGSL missing %q:\n%s", want, wgslOut)
				}
			}
		})
	}

	// Reject non-u32 @builtin(instance_index).
	badSrc := `
@vertex
fn vs_main(@builtin(instance_index) idx: f32) -> @builtin(position) vec4<f32> {
	return vec4<f32>(idx, 0.0, 0.0, 1.0);
}
`
	if _, err := wgsl.CompilePipeline(badSrc); err == nil {
		t.Fatalf("expected error for f32 @builtin(instance_index), got nil")
	}
}

const particleComputeWGSL = `
struct SimParams {
	dt: f32,
	scale: f32,
}

struct Particle {
	pos: vec4<f32>,
	vel: vec4<f32>,
}

@group(0) @binding(0) var<uniform> params: SimParams;
@group(0) @binding(1) var<storage, read> src: array<Particle>;
@group(0) @binding(2) var<storage, read_write> dst: array<Particle>;
@group(0) @binding(3) var outImg: texture_storage_2d<rgba8unorm, write>;

@compute @workgroup_size(4, 2, 1)
fn step_particles(
	@builtin(global_invocation_id) gid: vec3<u32>,
	@builtin(local_invocation_id) lid: vec3<u32>,
	@builtin(workgroup_id) wid: vec3<u32>,
	@builtin(num_workgroups) nwg: vec3<u32>,
	@builtin(local_invocation_index) lidx: u32,
) {
	let idx = gid.x + gid.y * (nwg.x * 4u);
	let total = arrayLength(&dst);
	if (idx < total) {
		let p = src[idx];
		var next: Particle;
		next.pos = p.pos + p.vel * params.dt;
		next.vel = p.vel * params.scale;
		dst[idx] = next;
		textureStore(outImg, vec2<i32>(i32(gid.x), i32(gid.y)), vec4<f32>(f32(lid.x), f32(wid.x), f32(lidx), 1.0));
	}
	workgroupBarrier();
	storageBarrier();
}
`

func TestCompileComputeShaderAndEmitters(t *testing.T) {
	prog, err := wgsl.CompilePipeline(particleComputeWGSL)
	if err != nil {
		t.Fatalf("CompilePipeline compute: %v", err)
	}
	ir := prog.IR
	if !ir.IsPipeline() {
		t.Fatalf("expected IsPipeline() == true for compute module")
	}
	if ir.ComputeEntry != "step_particles" {
		t.Errorf("ComputeEntry = %q, want step_particles", ir.ComputeEntry)
	}
	if ir.ComputeFunc.WorkgroupSize != [3]uint32{4, 2, 1} {
		t.Errorf("WorkgroupSize = %v, want [4 2 1]", ir.ComputeFunc.WorkgroupSize)
	}
	if len(ir.Bindings) != 4 {
		t.Fatalf("bindings = %d, want 4", len(ir.Bindings))
	}
	if ir.Bindings[1].Kind != shaderir.BindingStorageBufferRead {
		t.Errorf("binding 1 kind = %v, want BindingStorageBufferRead", ir.Bindings[1].Kind)
	}
	if ir.Bindings[2].Kind != shaderir.BindingStorageBufferReadWrite {
		t.Errorf("binding 2 kind = %v, want BindingStorageBufferReadWrite", ir.Bindings[2].Kind)
	}
	if ir.Bindings[3].Kind != shaderir.BindingStorageTexture {
		t.Errorf("binding 3 kind = %v, want BindingStorageTexture", ir.Bindings[3].Kind)
	}

	// MSL emitter
	mslOut, err := msl.CompilePipeline(ir)
	if err != nil {
		t.Fatalf("msl.CompilePipeline compute: %v", err)
	}
	if mslOut.ComputeEntry != "step_particles" || mslOut.WorkgroupSize != [3]uint32{4, 2, 1} {
		t.Errorf("MSL compute meta = (%q, %v)", mslOut.ComputeEntry, mslOut.WorkgroupSize)
	}
	for _, want := range []string{
		"kernel void step_particles",
		"[[thread_position_in_grid]]",
		"[[thread_position_in_threadgroup]]",
		"[[threadgroup_position_in_grid]]",
		"[[threadgroups_per_grid]]",
		"[[thread_index_in_threadgroup]]",
		"device const S1* B1",
		"device S1* B2",
		"texture2d<float, access::write> B3",
		".write(",
		"threadgroup_barrier(mem_flags::mem_threadgroup)",
		"threadgroup_barrier(mem_flags::mem_device)",
	} {
		if !strings.Contains(mslOut.Source, want) {
			t.Errorf("MSL compute missing %q:\n%s", want, mslOut.Source)
		}
	}

	// HLSL emitter
	hlslOut, err := hlsl.CompilePipeline(ir)
	if err != nil {
		t.Fatalf("hlsl.CompilePipeline compute: %v", err)
	}
	if hlslOut.ComputeEntry != "step_particles" || hlslOut.WorkgroupSize != [3]uint32{4, 2, 1} {
		t.Errorf("HLSL compute meta = (%q, %v)", hlslOut.ComputeEntry, hlslOut.WorkgroupSize)
	}
	for _, want := range []string{
		"[numthreads(4, 2, 1)]",
		"void step_particles(",
		"SV_DispatchThreadID",
		"SV_GroupThreadID",
		"SV_GroupID",
		"SV_GroupIndex",
		"StructuredBuffer<S1> B1 : register(t0);",
		"RWStructuredBuffer<S1> B2 : register(u0);",
		"RWTexture2D<float4> B3 : register(u1);",
		"uint3 __num_workgroups;",
		"B2.GetDimensions(",
		"GroupMemoryBarrierWithGroupSync()",
		"DeviceMemoryBarrierWithGroupSync()",
	} {
		if !strings.Contains(hlslOut.ComputeSource, want) {
			t.Errorf("HLSL compute missing %q:\n%s", want, hlslOut.ComputeSource)
		}
	}

	// GLSL 4.30 Core emitter
	glsl430, err := glsl.CompilePipeline(ir, glsl.GLSLVersion430)
	if err != nil {
		t.Fatalf("glsl.CompilePipeline 430: %v", err)
	}
	for _, want := range []string{
		"#version 430 core",
		"layout(local_size_x = 4, local_size_y = 2, local_size_z = 1) in;",
		"layout(std430, binding = 0) readonly buffer SB1",
		"layout(std430, binding = 1) buffer SB2",
		"layout(rgba8, binding = 0) writeonly uniform image2D B3;",
		"gl_GlobalInvocationID",
		"gl_LocalInvocationID",
		"gl_WorkGroupID",
		"gl_NumWorkGroups",
		"gl_LocalInvocationIndex",
		"imageStore(",
		".length()",
		"barrier()",
		"memoryBarrierBuffer()",
	} {
		if !strings.Contains(glsl430.ComputeSource, want) {
			t.Errorf("GLSL 4.30 compute missing %q:\n%s", want, glsl430.ComputeSource)
		}
	}
	if len(glsl430.StorageBuffers) != 2 || !glsl430.StorageBuffers[0].ReadOnly || glsl430.StorageBuffers[1].ReadOnly {
		t.Errorf("GLSL 4.30 StorageBuffers = %+v", glsl430.StorageBuffers)
	}
	if len(glsl430.StorageImages) != 1 {
		t.Errorf("GLSL 4.30 StorageImages = %+v", glsl430.StorageImages)
	}

	// GLSL ES 3.10 emitter
	glslES310, err := glsl.CompilePipeline(ir, glsl.GLSLVersionES310)
	if err != nil {
		t.Fatalf("glsl.CompilePipeline ES 3.10: %v", err)
	}
	for _, want := range []string{
		"#version 310 es",
		"layout(local_size_x = 4, local_size_y = 2, local_size_z = 1) in;",
		"layout(std430, binding = 1) buffer SB2",
	} {
		if !strings.Contains(glslES310.ComputeSource, want) {
			t.Errorf("GLSL ES 3.10 compute missing %q:\n%s", want, glslES310.ComputeSource)
		}
	}

	// GLSL ES 3.00 (WebGL 2 / GLES 3.0) and GLSL 3.30 must reject compute shaders.
	if _, err := glsl.CompilePipeline(ir, glsl.GLSLVersionES300); err == nil {
		t.Errorf("expected glsl.CompilePipeline(ES300) to reject compute shader")
	}
	if _, err := glsl.CompilePipeline(ir, glsl.GLSLVersionDefault); err == nil {
		t.Errorf("expected glsl.CompilePipeline(330) to reject compute shader")
	}

	// WGSL emitter
	wgslOut, err := irwgsl.CompilePipeline(ir)
	if err != nil {
		t.Fatalf("irwgsl.CompilePipeline compute: %v", err)
	}
	for _, want := range []string{
		"@group(0) @binding(1) var<storage, read> B1: array<S1>;",
		"@group(0) @binding(2) var<storage, read_write> B2: array<S1>;",
		"@group(0) @binding(3) var B3: texture_storage_2d<rgba8unorm, write>;",
		"@compute @workgroup_size(4, 2, 1)",
		"fn step_particles(",
		"@builtin(global_invocation_id)",
		"@builtin(local_invocation_id)",
		"@builtin(workgroup_id)",
		"@builtin(num_workgroups)",
		"arrayLength(&B2)",
		"textureStore(",
		"workgroupBarrier()",
		"storageBarrier()",
	} {
		if !strings.Contains(wgslOut, want) {
			t.Errorf("WGSL compute missing %q:\n%s", want, wgslOut)
		}
	}
}

func TestComputeShaderValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "missing workgroup_size",
			src: `
@group(0) @binding(0) var<storage, read_write> buf: array<f32>;
@compute fn main(@builtin(global_invocation_id) id: vec3<u32>) {
	buf[id.x] = 1.0;
}`,
			want: "@workgroup_size",
		},
		{
			name: "write to read-only storage",
			src: `
@group(0) @binding(0) var<storage, read> buf: array<f32>;
@compute @workgroup_size(1) fn main(@builtin(global_invocation_id) id: vec3<u32>) {
	buf[id.x] = 1.0;
}`,
			want: "cannot be assigned",
		},
		{
			name: "wrong builtin type",
			src: `
@compute @workgroup_size(1) fn main(@builtin(global_invocation_id) id: u32) {}`,
			want: "vec3<u32>",
		},
		{
			name: "compute return value",
			src: `
@compute @workgroup_size(1) fn main() -> f32 { return 1.0; }`,
			want: "must not return",
		},
		{
			name: "discard in compute",
			src: `
@compute @workgroup_size(1) fn main() { discard; }`,
			want: "discard is not allowed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := wgsl.CompilePipeline(tc.src)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}
