package msl_test

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

func TestCompilePipelineVec3HostShareableLayouts(t *testing.T) {
	t.Run("vec3_followed_by_scalar_in_uniform_and_storage", func(t *testing.T) {
		const src = `
struct Packet {
	d: vec3<f32>,
	s: f32,
}

@group(0) @binding(0) var<uniform> u: Packet;
@group(0) @binding(1) var<storage, read> arr: array<Packet>;

@fragment
fn fs_main(@builtin(position) pos: vec4<f32>) -> @location(0) vec4<f32> {
	let p = arr[1u];
	return vec4<f32>(u.d + p.d, u.s + p.s);
}
`
		prog, err := wgsl.CompilePipeline(src)
		if err != nil {
			t.Fatalf("wgsl.CompilePipeline: %v", err)
		}
		out, err := msl.CompilePipeline(prog.IR)
		if err != nil {
			t.Fatalf("msl.CompilePipeline: %v", err)
		}
		for _, want := range []string{
			"struct alignas(16) S0 {",
			"\tpacked_float3 M0;",
			"\tfloat M1;",
			"float3((B0).M0)",
			"float3((l1).M0)",
		} {
			if !strings.Contains(out.Source, want) {
				t.Errorf("MSL missing %q:\n%s", want, out.Source)
			}
		}
		if strings.Contains(out.Source, "__pad") {
			t.Errorf("MSL emitted unexpected padding in tightly packed {vec3, f32}:\n%s", out.Source)
		}
	})

	t.Run("leading_and_trailing_padding_for_vec3_and_ivec3_uvec3", func(t *testing.T) {
		const src = `
struct Mixed {
	lead: f32,
	fv: vec3<f32>,
	afterF: f32,
	iv: vec3<i32>,
	afterI: i32,
	uv: vec3<u32>,
	tail: vec3<f32>,
}

@group(0) @binding(0) var<uniform> m: Mixed;

@fragment
fn fs_main() -> @location(0) vec4<f32> {
	let fsum = m.fv + m.tail + vec3<f32>(m.lead + m.afterF);
	let isum = f32(m.iv.x + m.afterI);
	let usum = f32(m.uv.z);
	return vec4<f32>(fsum.x + isum, fsum.y + usum, fsum.z, 1.0);
}
`
		prog, err := wgsl.CompilePipeline(src)
		if err != nil {
			t.Fatalf("wgsl.CompilePipeline: %v", err)
		}
		st := prog.IR.Bindings[0].Type
		if st.StructFieldOffset(0) != 0 || st.StructFieldOffset(1) != 16 || st.StructFieldOffset(2) != 28 ||
			st.StructFieldOffset(3) != 32 || st.StructFieldOffset(4) != 44 || st.StructFieldOffset(5) != 48 ||
			st.StructFieldOffset(6) != 64 || st.SizeBytes() != 80 {
			t.Fatalf("unexpected WGSL layout: size=%d offsets=[%d %d %d %d %d %d %d]",
				st.SizeBytes(),
				st.StructFieldOffset(0), st.StructFieldOffset(1), st.StructFieldOffset(2),
				st.StructFieldOffset(3), st.StructFieldOffset(4), st.StructFieldOffset(5), st.StructFieldOffset(6))
		}
		out, err := msl.CompilePipeline(prog.IR)
		if err != nil {
			t.Fatalf("msl.CompilePipeline: %v", err)
		}
		for _, want := range []string{
			"struct alignas(16) S0 {",
			"\tfloat M0;\n\tuchar __pad0[12];\n\tpacked_float3 M1;\n\tfloat M2;\n\tpacked_int3 M3;\n\tint M4;\n\tpacked_uint3 M5;\n\tuchar __pad1[4];\n\tpacked_float3 M6;\n\tuchar __pad2[4];",
			"float3((B0).M1)",
			"int3((B0).M3)",
			"uint3((B0).M5)",
			"float3((B0).M6)",
		} {
			if !strings.Contains(out.Source, want) {
				t.Errorf("MSL missing %q:\n%s", want, out.Source)
			}
		}
	})

	t.Run("vec3_followed_by_vec2_and_nested_structs", func(t *testing.T) {
		const src = `
struct Inner {
	v: vec3<f32>,
}

struct Outer {
	tag: f32,
	inner: Inner,
	pair: vec2<f32>,
	v3: vec3<f32>,
	v2: vec2<f32>,
}

@group(0) @binding(0) var<uniform> o: Outer;

@fragment
fn fs_main() -> @location(0) vec4<f32> {
	return vec4<f32>(o.inner.v + o.v3, o.tag + o.pair.x + o.v2.y);
}
`
		prog, err := wgsl.CompilePipeline(src)
		if err != nil {
			t.Fatalf("wgsl.CompilePipeline: %v", err)
		}
		out, err := msl.CompilePipeline(prog.IR)
		if err != nil {
			t.Fatalf("msl.CompilePipeline: %v", err)
		}
		for _, want := range []string{
			"struct alignas(16) S0 {\n\tpacked_float3 M0;\n\tuchar __pad0[4];\n};",
			"struct S1 {\n\tfloat M0;\n\tS0 M1;\n\tfloat2 M2;\n\tuchar __pad0[8];\n\tpacked_float3 M3;\n\tfloat2 M4;\n};",
			"float3(((B0).M1).M0)",
			"float3((B0).M3)",
		} {
			if !strings.Contains(out.Source, want) {
				t.Errorf("MSL missing %q:\n%s", want, out.Source)
			}
		}
	})

	t.Run("arrays_of_vec3_keep_unpacked_float3_stride_16", func(t *testing.T) {
		const src = `
struct Uniforms {
	pts: array<vec3<f32>, 3>,
	scale: f32,
}

struct RuntimeBuf {
	count: u32,
	dirs: array<vec3<f32>>,
}

@group(0) @binding(0) var<uniform> u: Uniforms;
@group(0) @binding(1) var<storage, read> rb: RuntimeBuf;

@fragment
fn fs_main() -> @location(0) vec4<f32> {
	return vec4<f32>(u.pts[2] + rb.dirs[1], u.scale + f32(rb.count));
}
`
		prog, err := wgsl.CompilePipeline(src)
		if err != nil {
			t.Fatalf("wgsl.CompilePipeline: %v", err)
		}
		out, err := msl.CompilePipeline(prog.IR)
		if err != nil {
			t.Fatalf("msl.CompilePipeline: %v", err)
		}
		for _, want := range []string{
			"struct S0 {\n\tarray<float3, 3> M0;\n\tfloat M1;\n};",
			"struct S1 {\n\tuint M0;\n\tfloat3 M1[1];\n};",
		} {
			if !strings.Contains(out.Source, want) {
				t.Errorf("MSL missing %q:\n%s", want, out.Source)
			}
		}
		if strings.Contains(out.Source, "packed_float3") {
			t.Errorf("array<vec3<f32>> must keep unpacked float3 (stride 16), got:\n%s", out.Source)
		}
	})

	t.Run("matrices_mat2x3_mat3x3_mat4x3_followed_by_scalar", func(t *testing.T) {
		const src = `
struct Mats {
	m23: mat2x3<f32>,
	s0: f32,
	m33: mat3x3<f32>,
	s1: f32,
	m43: mat4x3<f32>,
	s2: f32,
}

@group(0) @binding(0) var<uniform> u: Mats;

@fragment
fn fs_main() -> @location(0) vec4<f32> {
	let a = u.m23 * vec2<f32>(1.0, 2.0);
	let b = u.m33 * a;
	let c = u.m43 * vec4<f32>(b, u.s0 + u.s1 + u.s2);
	return vec4<f32>(c, 1.0);
}
`
		prog, err := wgsl.CompilePipeline(src)
		if err != nil {
			t.Fatalf("wgsl.CompilePipeline: %v", err)
		}
		out, err := msl.CompilePipeline(prog.IR)
		if err != nil {
			t.Fatalf("msl.CompilePipeline: %v", err)
		}
		for _, want := range []string{
			"struct S0 {\n\tfloat2x3 M0;\n\tfloat M1;\n\tfloat3x3 M2;\n\tfloat M3;\n\tfloat4x3 M4;\n\tfloat M5;\n};",
		} {
			if !strings.Contains(out.Source, want) {
				t.Errorf("MSL missing %q:\n%s", want, out.Source)
			}
		}
	})

	t.Run("local_only_structs_keep_float3_while_shared_structs_convert_cleanly", func(t *testing.T) {
		const src = `
struct Shared {
	lead: f32,
	v: vec3<f32>,
	s: f32,
}

struct LocalOnly {
	v: vec3<f32>,
	s: f32,
}

@group(0) @binding(0) var<storage, read> src: array<Shared>;
@group(0) @binding(1) var<storage, read_write> dst: array<Shared>;

fn bump(p: Shared) -> Shared {
	var localOnly = LocalOnly(p.v, p.s);
	return Shared(p.lead, localOnly.v + vec3<f32>(1.0, 2.0, 3.0), localOnly.s + 0.5);
}

@compute @workgroup_size(1)
fn cs_main(@builtin(global_invocation_id) gid: vec3<u32>) {
	let i = gid.x;
	var cur = bump(src[i]);
	cur.v.x += 2.0;
	cur.v = cur.v * 2.0;
	dst[i] = cur;
	dst[i].v = vec3<f32>(4.0, 5.0, 6.0);
	dst[i].v.y += 1.0;
}
`
		prog, err := wgsl.CompilePipeline(src)
		if err != nil {
			t.Fatalf("wgsl.CompilePipeline: %v", err)
		}
		out, err := msl.CompilePipeline(prog.IR)
		if err != nil {
			t.Fatalf("msl.CompilePipeline: %v", err)
		}
		for _, want := range []string{
			"struct alignas(16) S0 {\n\tfloat M0;\n\tuchar __pad0[12];\n\tpacked_float3 M1;\n\tfloat M2;\n};",
			"S0 __make_S0(float a0, float3 a1, float a2) {",
			"r.M1 = packed_float3(a1);",
			"struct S1 {\n\tfloat3 M0;\n\tfloat M1;\n};",
			"S1{float3((l0).M1), (l0).M2}",
			"__make_S0((l0).M0, ((l1).M0 + float3(1.0f, 2.0f, 3.0f)), ((l1).M1 + 0.5f))",
			"((l1).M1).x = ((float3((l1).M1)).x + 2.0f);",
			"(l1).M1 = packed_float3((float3((l1).M1) * 2.0f));",
			"((B1)[l0]).M1 = packed_float3(float3(4.0f, 5.0f, 6.0f));",
			"(((B1)[l0]).M1).y = ((float3(((B1)[l0]).M1)).y + 1.0f);",
		} {
			if !strings.Contains(out.Source, want) {
				t.Errorf("MSL missing %q:\n%s", want, out.Source)
			}
		}
	})
}

// TestStructOffsetsMatchWGSLInIRAndMSL checks the IR's WGSL offsets and the MSL struct that pads to them; the GLSL, HLSL and WGSL writers are only checked to accept the struct. HLSL lays it out by cbuffer and StructuredBuffer packing, a gap the roadmap records.
func TestStructOffsetsMatchWGSLInIRAndMSL(t *testing.T) {
	const src = `
struct Packet {
	d: vec3<f32>,
	s: f32,
	lead: f32,
	tail: vec3<f32>,
}

@group(0) @binding(0) var<uniform> u: Packet;
@group(0) @binding(1) var<storage, read> buf: array<Packet>;

@vertex
fn vs_main(@location(0) pos: vec2<f32>) -> @builtin(position) vec4<f32> {
	return vec4<f32>(pos, 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
	let p = buf[0];
	return vec4<f32>(u.d + p.tail, u.s + p.lead);
}
`
	prog, err := wgsl.CompilePipeline(src)
	if err != nil {
		t.Fatalf("wgsl.CompilePipeline: %v", err)
	}
	st := prog.IR.Bindings[0].Type
	if got := [4]int{st.StructFieldOffset(0), st.StructFieldOffset(1), st.StructFieldOffset(2), st.StructFieldOffset(3)}; got != [4]int{0, 12, 16, 32} {
		t.Fatalf("uniform offsets = %v, want [0 12 16 32]", got)
	}
	if st.SizeBytes() != 48 || st.StorageSizeBytes() != 48 || st.StorageStride() != 48 {
		t.Fatalf("struct sizes = uniform:%d storage:%d stride:%d, want 48", st.SizeBytes(), st.StorageSizeBytes(), st.StorageStride())
	}
	for i := range st.Sub {
		if st.StructFieldOffset(i) != st.StorageStructFieldOffset(i) {
			t.Errorf("field %d offset mismatch uniform=%d storage=%d", i, st.StructFieldOffset(i), st.StorageStructFieldOffset(i))
		}
	}

	mslOut, err := msl.CompilePipeline(prog.IR)
	if err != nil {
		t.Fatalf("msl.CompilePipeline: %v", err)
	}
	if !strings.Contains(mslOut.Source, "struct alignas(16) S0 {\n\tpacked_float3 M0;\n\tfloat M1;\n\tfloat M2;\n\tuchar __pad0[12];\n\tpacked_float3 M3;\n\tuchar __pad1[4];\n};") {
		t.Errorf("unexpected MSL struct layout:\n%s", mslOut.Source)
	}
	if _, err := glsl.CompilePipeline(prog.IR, glsl.GLSLVersion430); err != nil {
		t.Errorf("glsl.CompilePipeline: %v", err)
	}
	if _, err := hlsl.CompilePipeline(prog.IR); err != nil {
		t.Errorf("hlsl.CompilePipeline: %v", err)
	}
	wgslSrc, err := irwgsl.CompilePipeline(prog.IR)
	if err != nil {
		t.Errorf("irwgsl.CompilePipeline: %v", err)
	}
	if _, err := wgsl.CompilePipeline(wgslSrc); err != nil {
		t.Errorf("WGSL round-trip: %v", err)
	}
}

// TestCompilePipelineHelperPackedVec3 checks a helper that reads a buffer struct copied into its own local.
func TestCompilePipelineHelperPackedVec3(t *testing.T) {
	st := shaderir.Type{
		Main: shaderir.Struct,
		Sub:  []shaderir.Type{{Main: shaderir.Vec3}, {Main: shaderir.Float}},
	}
	p := &shaderir.Program{
		Bindings: []shaderir.Binding{
			{Group: 0, Binding: 0, Kind: shaderir.BindingUniformBuffer, Type: st},
		},
		Funcs: []shaderir.Func{
			{
				Index:  0,
				Return: shaderir.Type{Main: shaderir.Vec4},
				Block: &shaderir.Block{
					LocalVars:           []shaderir.Type{st},
					LocalVarIndexOffset: 0,
					Stmts: []shaderir.Stmt{
						{
							Type: shaderir.Assign,
							Exprs: []shaderir.Expr{
								{Type: shaderir.LocalVariable, Index: 0},
								{Type: shaderir.BindingVariable, Index: 0},
							},
						},
						{
							Type: shaderir.Return,
							Exprs: []shaderir.Expr{
								{
									Type: shaderir.Call,
									Exprs: []shaderir.Expr{
										{Type: shaderir.BuiltinFuncExpr, BuiltinFunc: shaderir.Vec4F},
										{
											Type: shaderir.FieldSelector,
											Exprs: []shaderir.Expr{
												{Type: shaderir.LocalVariable, Index: 0},
												{Type: shaderir.StructMember, Index: 0},
											},
										},
										{
											Type: shaderir.FieldSelector,
											Exprs: []shaderir.Expr{
												{Type: shaderir.LocalVariable, Index: 0},
												{Type: shaderir.StructMember, Index: 1},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		FragmentFunc: shaderir.FragmentFunc{
			Block: &shaderir.Block{
				Stmts: []shaderir.Stmt{
					{
						Type: shaderir.Return,
						Exprs: []shaderir.Expr{
							{Type: shaderir.Call, Exprs: []shaderir.Expr{{Type: shaderir.FunctionExpr, Index: 0}}},
						},
					},
				},
			},
		},
	}
	out, err := msl.CompilePipeline(p)
	if err != nil {
		t.Fatalf("CompilePipeline: %v", err)
	}
	if !strings.Contains(out.Source, "float4(float3((l0).M0), (l0).M1)") {
		t.Errorf("expected helper packed_float3 read to unwrap to float3:\n%s", out.Source)
	}
}

// TestCompilePipelineNestedBlockPackedVec3 checks that a buffer struct local declared in a function's outer block keeps its packed vec3 reads and writes inside if and for bodies, in a helper and in an entry point.
func TestCompilePipelineNestedBlockPackedVec3(t *testing.T) {
	const src = `
struct Packet {
	d: vec3<f32>,
	s: f32,
}

@group(0) @binding(0) var<uniform> u: Packet;

fn shade(k: f32) -> vec4<f32> {
	var p = u;
	if (k > 0.5) {
		p.d = p.d * k;
	}
	for (var i = 0; i < 2; i++) {
		p.s = p.s + p.d.x;
	}
	return vec4<f32>(p.d, p.s);
}

@fragment
fn fs_main(@builtin(position) pos: vec4<f32>) -> @location(0) vec4<f32> {
	var q = u;
	if (pos.x > 1.0) {
		q.d = vec3<f32>(pos.y);
	}
	for (var i = 0; i < 2; i++) {
		q.s = q.s + q.d.z;
	}
	return shade(q.s) + vec4<f32>(q.d, 0.0);
}
`
	prog, err := wgsl.CompilePipeline(src)
	if err != nil {
		t.Fatalf("wgsl.CompilePipeline: %v", err)
	}
	out, err := msl.CompilePipeline(prog.IR)
	if err != nil {
		t.Fatalf("msl.CompilePipeline: %v", err)
	}
	for _, want := range []string{
		" = packed_float3((float3((l1).M0) * ",
		"(float3((l1).M0)).x",
		" = packed_float3(float3(",
		"(float3((l1).M0)).z",
		"float4(float3((l1).M0), (l1).M1)",
	} {
		if !strings.Contains(out.Source, want) {
			t.Errorf("MSL missing %q:\n%s", want, out.Source)
		}
	}
}
