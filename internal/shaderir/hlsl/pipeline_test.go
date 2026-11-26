package hlsl_test

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/hlsl"
)

func TestCompilePipelineHLSLSM50Constructs(t *testing.T) {
	const litWGSL = `
struct Uniforms {
	mvp: mat4x4<f32>,
	lightDir: vec4<f32>,
}

@group(0) @binding(0) var<uniform> u: Uniforms;
@group(0) @binding(1) var tex: texture_2d<f32>;
@group(0) @binding(2) var samp: sampler;

struct VertexIn {
	@location(0) pos: vec3<f32>,
	@location(1) normal: vec3<f32>,
	@location(2) uv: vec2<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) normal: vec3<f32>,
	@location(1) uv: vec2<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = u.mvp * vec4<f32>(in.pos, 1.0);
	out.normal = in.normal;
	out.uv = in.uv;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	let n = normalize(in.normal);
	let l = normalize(u.lightDir.xyz);
	let ndl = max(dot(n, l), 0.0);
	let albedo = textureSample(tex, samp, in.uv);
	let shade = vec3<f32>(0.2) + vec3<f32>(0.8) * ndl;
	return vec4<f32>(albedo.rgb * shade, albedo.a);
}
`
	prog, err := wgsl.CompilePipeline(litWGSL)
	if err != nil {
		t.Fatalf("wgsl.CompilePipeline: %v", err)
	}
	out, err := hlsl.CompilePipeline(prog.IR)
	if err != nil {
		t.Fatalf("hlsl.CompilePipeline: %v", err)
	}
	if out.VertexEntry != "vs_main" || out.PixelEntry != "fs_main" {
		t.Fatalf("entries = (%q, %q), want (vs_main, fs_main)", out.VertexEntry, out.PixelEntry)
	}
	if len(out.Registers) != 3 {
		t.Fatalf("registers = %d, want 3: %+v", len(out.Registers), out.Registers)
	}
	if out.Registers[0] != (hlsl.RegisterBinding{Group: 0, Binding: 0, Kind: shaderir.BindingUniformBuffer, Register: 0}) ||
		out.Registers[1] != (hlsl.RegisterBinding{Group: 0, Binding: 1, Kind: shaderir.BindingTexture, Register: 0}) ||
		out.Registers[2] != (hlsl.RegisterBinding{Group: 0, Binding: 2, Kind: shaderir.BindingSampler, Register: 0}) {
		t.Fatalf("unexpected register bindings: %+v", out.Registers)
	}

	// Vector splats must never emit invalid single-arg float3(0.2) which fxc rejects.
	if strings.Contains(out.PixelSource, "float3(0.2)") || strings.Contains(out.PixelSource, "float3(0.8)") {
		t.Errorf("HLSL PS emitted invalid single-arg float3(...) constructor:\n%s", out.PixelSource)
	}
	if !strings.Contains(out.PixelSource, "float3(0.2, 0.2, 0.2)") {
		t.Errorf("HLSL PS missing expanded float3(0.2, 0.2, 0.2):\n%s", out.PixelSource)
	}
	for _, want := range []string{"cbuffer CB0 : register(b0)", "Texture2D B1 : register(t0)", "SamplerState B2 : register(s0)", "mul("} {
		if !strings.Contains(out.VertexSource, want) {
			t.Errorf("HLSL VS missing %q:\n%s", want, out.VertexSource)
		}
	}
}

func TestCompilePipelineHLSLStructConstructorAndDiscard(t *testing.T) {
	st := shaderir.Type{
		Main: shaderir.Struct,
		Sub:  []shaderir.Type{{Main: shaderir.Vec3}, {Main: shaderir.Float}},
	}
	p := &shaderir.Program{
		VertexEntry:   "vs_main",
		FragmentEntry: "fs_main",
		VertexFunc: shaderir.VertexFunc{
			Block: &shaderir.Block{
				Stmts: []shaderir.Stmt{{Type: shaderir.Return}},
			},
		},
		FragmentFunc: shaderir.FragmentFunc{
			Block: &shaderir.Block{
				LocalVars: []shaderir.Type{st},
				Stmts: []shaderir.Stmt{
					{
						Type: shaderir.Assign,
						Exprs: []shaderir.Expr{
							{Type: shaderir.LocalVariable, Index: 1},
							{
								Type:       shaderir.StructConstruct,
								StructType: &st,
								Exprs: []shaderir.Expr{
									{Type: shaderir.Call, Exprs: []shaderir.Expr{
										{Type: shaderir.BuiltinFuncExpr, BuiltinFunc: shaderir.Vec3F},
										{Type: shaderir.LocalVariable, Index: 2},
									}},
									{Type: shaderir.LocalVariable, Index: 2},
								},
							},
						},
					},
					{Type: shaderir.Discard},
				},
			},
		},
	}
	out, err := hlsl.CompilePipeline(p)
	if err != nil {
		t.Fatalf("CompilePipeline: %v", err)
	}
	if strings.Contains(out.PixelSource, "(S0){") {
		t.Errorf("HLSL PS emitted invalid C99 compound literal (S0){...}:\n%s", out.PixelSource)
	}
	if !strings.Contains(out.PixelSource, "S0 __make_S0(") || !strings.Contains(out.PixelSource, "__make_S0(") {
		t.Errorf("HLSL PS missing __make_S0 helper constructor:\n%s", out.PixelSource)
	}
	if !strings.Contains(out.PixelSource, "(float3)(l2)") {
		t.Errorf("HLSL PS missing single-arg vector cast (float3)(l2):\n%s", out.PixelSource)
	}
	if !strings.Contains(out.PixelSource, "discard;\n\treturn (float4)0;") {
		t.Errorf("HLSL PS discard missing trailing return:\n%s", out.PixelSource)
	}
}

func TestCompilePipelineHLSLInterpolationAndMatrixConstructor(t *testing.T) {
	p := &shaderir.Program{
		VertexEntry:   "vs_main",
		FragmentEntry: "fs_main",
		Varyings:      []shaderir.Type{{Main: shaderir.Uint}, {Main: shaderir.Vec2}},
		VaryingMeta: []shaderir.StageIO{
			{Location: 0, Interpolation: shaderir.InterpFlat},
			{Location: 1, Interpolation: shaderir.InterpLinear},
		},
		VertexFunc: shaderir.VertexFunc{
			Block: &shaderir.Block{
				Stmts: []shaderir.Stmt{
					{
						Type: shaderir.Assign,
						Exprs: []shaderir.Expr{
							{Type: shaderir.LocalVariable, Index: 0},
							{
								Type: shaderir.Binary,
								Op:   shaderir.MatrixMul,
								Exprs: []shaderir.Expr{
									{
										Type: shaderir.Call,
										Exprs: []shaderir.Expr{
											{Type: shaderir.BuiltinFuncExpr, BuiltinFunc: shaderir.Mat2F},
											{Type: shaderir.LocalVariable, Index: 2},
											{Type: shaderir.LocalVariable, Index: 2},
										},
									},
									{Type: shaderir.LocalVariable, Index: 2},
								},
							},
						},
					},
					{Type: shaderir.Return},
				},
			},
		},
		FragmentFunc: shaderir.FragmentFunc{
			Block: &shaderir.Block{
				Stmts: []shaderir.Stmt{{Type: shaderir.Return, Exprs: []shaderir.Expr{{Type: shaderir.LocalVariable, Index: 0}}}},
			},
		},
	}
	out, err := hlsl.CompilePipeline(p)
	if err != nil {
		t.Fatalf("CompilePipeline: %v", err)
	}
	for _, want := range []string{"nointerpolation uint v0 : TEXCOORD0;", "noperspective float2 v1 : TEXCOORD1;", "mul(transpose(float2x2("} {
		if !strings.Contains(out.VertexSource, want) {
			t.Errorf("HLSL VS missing %q:\n%s", want, out.VertexSource)
		}
	}
}

func TestCompilePipelineHLSLBufferStructsKeepWGSLOffsets(t *testing.T) {
	const src = `
struct P { s: f32, d: vec3<f32> }
@group(0) @binding(0) var<uniform> u: P;
@group(0) @binding(1) var<storage, read> b: array<P>;
@vertex fn vs_main() -> @builtin(position) vec4<f32> { return vec4<f32>(u.d + b[1].d, u.s + b[1].s); }
@fragment fn fs_main() -> @location(0) vec4<f32> { return vec4<f32>(u.d, u.s); }
@compute @workgroup_size(1) fn cs_main() {}
`
	prog, err := wgsl.CompilePipeline(src)
	if err != nil {
		t.Fatalf("wgsl.CompilePipeline: %v", err)
	}
	out, err := hlsl.CompilePipeline(prog.IR)
	if err != nil {
		t.Fatalf("hlsl.CompilePipeline: %v", err)
	}
	for _, want := range []string{
		"struct S0 {\n\tfloat M0;\n\tuint __pad0;\n\tuint __pad1;\n\tuint __pad2;\n\tfloat3 M1;",
		"cbuffer CB0 : register(b0)",
		"ByteAddressBuffer B1 : register(t0)",
		"asfloat(B1.Load(32u))",
		"asfloat(B1.Load3(48u))",
	} {
		if !strings.Contains(out.VertexSource, want) {
			t.Errorf("HLSL missing %q:\n%s", want, out.VertexSource)
		}
	}
}

func TestCompilePipelineHLSLRawStorageTypes(t *testing.T) {
	const src = `
struct Inner {
	signed: i32,
	basis: mat2x3f,
	coords: array<vec2u, 2>,
}
struct Data {
	prefix: u32,
	inner: Inner,
	tail: array<vec3f>,
}
@group(0) @binding(0) var<storage, read> data: Data;
@group(0) @binding(1) var<storage, read> words: array<i32>;
@group(0) @binding(2) var<storage, read> grid: array<array<vec2i, 2>, 2>;
@group(0) @binding(3) var<storage, read> strength: f32;

fn color(part: Inner) -> vec3f {
	return part.basis * vec2f(f32(part.signed), f32(part.coords[1].x));
}
@vertex fn vs_main(@builtin(vertex_index) i: u32) -> @builtin(position) vec4f {
	let n = arrayLength(&data.tail) + arrayLength(&words);
	let v = data.tail[i] + data.inner.basis[1] + color(data.inner);
	let s = f32(words[i] + grid[1][i].x) + strength;
	return vec4f(v, f32(n) + s);
}
`
	prog, err := wgsl.CompilePipeline(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := hlsl.CompilePipeline(prog.IR)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ByteAddressBuffer B0 : register(t0);",
		"ByteAddressBuffer B1 : register(t1);",
		"ByteAddressBuffer B2 : register(t2);",
		"ByteAddressBuffer B3 : register(t3);",
		"B0.GetDimensions(size); return (max(size, base) - base) / stride;",
		"__len_B0(80u, 16u)",
		"__len_B1(0u, 4u)",
		"asfloat(B0.Load3((uint(__in.__vertex_id) * 16u + 80u)))",
		"asfloat(B0.Load3(48u))",
		"asint(B0.Load(at))",
		"transpose(float2x3(asfloat(B0.Load3(at)), asfloat(B0.Load3((at + 16u)))))",
		"B0.Load2(at + i * 8u)",
		"asint(B1.Load(uint(__in.__vertex_id) * 4u))",
		"asint(B2.Load2((uint(min(__in.__vertex_id, 1u)) * 8u + 16u)))",
		"asfloat(B3.Load(0u))",
	} {
		if !strings.Contains(out.VertexSource, want) {
			t.Errorf("HLSL missing %q:\n%s", want, out.VertexSource)
		}
	}
	if strings.Contains(out.VertexSource, "StructuredBuffer") || strings.Contains(out.VertexSource, "[0];") {
		t.Errorf("HLSL materialized a typed resource or a runtime-sized array:\n%s", out.VertexSource)
	}
	binding := strings.Index(out.VertexSource, "ByteAddressBuffer B0")
	helper := strings.Index(out.VertexSource, "__load_B0_")
	entry := strings.Index(out.VertexSource, " vs_main(")
	if binding < 0 || helper <= binding || entry <= helper {
		t.Errorf("HLSL must declare the raw resource before its load helpers and entry point:\n%s", out.VertexSource)
	}
}
