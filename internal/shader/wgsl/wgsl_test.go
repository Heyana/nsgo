package wgsl_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/hlsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/msl"
)

// testdata holds shaders that use what WGSL has, which compile into
// every language; the Apple backend's tests compile them with Metal.
func programs(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "*.wgsl"))
	if err != nil || len(files) == 0 {
		t.Fatal("no shaders in testdata", err)
	}
	out := map[string]string{"transition": wgsl.Crossfade}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[strings.TrimSuffix(filepath.Base(f), ".wgsl")] = string(b)
	}
	return out
}

func TestCompile(t *testing.T) {
	for name, src := range programs(t) {
		p, err := wgsl.Compile(src)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for lang, out := range map[string]string{
			"msl":  msl.Fragment(p.IR, "f"),
			"glsl": glsl.Fragment(p.IR, "f", glsl.Options{Block: "U"}),
			"hlsl": hlsl.Fragment(p.IR, "f", 0, 1),
		} {
			if strings.Contains(out, "?(") {
				t.Errorf("%s: %s has what it cannot write:\n%s", name, lang, out)
			}
		}
	}
}

// A program reads the pictures and the time shade reaches, through the
// functions it calls.
func TestInputs(t *testing.T) {
	programs := programs(t)
	for _, c := range []struct {
		src      string
		inputs   wgsl.Inputs
		textures int
		animated bool
	}{
		{programs["fill"], 0, 0, false},
		{programs["uniforms"], 0, 0, true},
		{programs["pictures"], wgsl.Content | wgsl.Backdrop, 2, true},
		{programs["transition"], wgsl.From | wgsl.To, 4, false},
		{`fn unused() -> vec4f { return sample_to(vec2f(0.0)) * nsh.time; }
fn helper(uv: vec2f) -> vec4f { return sample_from(uv); }
fn shade(uv: vec2f) -> vec4f { return helper(uv); }`, wgsl.From, 3, false},
	} {
		p, err := wgsl.Compile(c.src)
		if err != nil {
			t.Fatal(err)
		}
		if p.Inputs != c.inputs || p.IR.TextureCount != c.textures || p.Animated != c.animated {
			t.Errorf("%q: inputs %b, textures %d, animated %v; want %b, %d, %v", c.src, p.Inputs, p.IR.TextureCount, p.Animated, c.inputs, c.textures, c.animated)
		}
	}
}

// The uniforms follow the bridge's and the built-ins' in the IR.
func TestUniforms(t *testing.T) {
	programs := programs(t)
	p, err := wgsl.Compile(programs["uniforms"])
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"__rectSize", "__scale", "__nsh", "tint", "glow", "speed", "steps", "seed", "stops", "at"}
	if strings.Join(p.IR.UniformNames, " ") != strings.Join(want, " ") {
		t.Errorf("uniforms %v, want %v", p.IR.UniformNames, want)
	}
	var names []string
	for _, u := range p.Uniforms {
		names = append(names, u.Name)
	}
	if strings.Join(names, " ") != strings.Join(want[3:], " ") || !p.Uniforms[1].Vec3 || p.Uniforms[5].Lanes != 12 {
		t.Errorf("members %+v", p.Uniforms)
	}
	pic, err := wgsl.Compile(programs["pictures"])
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(pic.IR.UniformNames, " "); got != "__rectSize __scale __imageSizes __nsh" || pic.IR.Uniforms[2].Length != 2 {
		t.Errorf("a program reading pictures has uniforms %s", got)
	}
}

// Errors give the place in the app's source.
func TestErrors(t *testing.T) {
	for _, c := range []struct {
		src, want string
	}{
		{"fn shade(uv: vec2f) -> vec4f {\n  return vec3f(1.0);\n}", "2:10: cannot use a vec3<f32> as a vec4<f32>"},
		{"fn shade(uv: vec2f) -> vec4f {\n  return vec4f(missing);\n}", "2:16: missing is not declared"},
		{"fn shade(x: f32) -> vec4f { return vec4f(x); }", "1:4: shade must be fn shade(uv: vec2f) -> vec4f"},
		{"fn other() {}", "shader: the shader defines no fn shade"},
		{"@fragment fn shade(uv: vec2f) -> vec4f { return vec4f(1.0); }", "1:1: a shader declares no entry points"},
		{"@group(0) @binding(3) var t: texture_2d<f32>;\nfn shade(uv: vec2f) -> vec4f { return vec4f(1.0); }", "1:1: a shader declares no resources"},
		{"struct Uniforms { m: mat4x4f }\nfn shade(uv: vec2f) -> vec4f { return vec4f(1.0); }", "1:19: a uniform is f32"},
		{"struct Uniforms { a: array<vec4f, 300> }\nfn shade(uv: vec2f) -> vec4f { return vec4f(1.0); }", "1:8: the uniforms take 4800 bytes, more than 4096"},
		{"fn shade(uv: vec2f) -> vec4f {\n  let x = 1;\n  x = 2;\n  return vec4f(1.0);\n}", "3:3: the value cannot be assigned"},
		{"fn shade(uv: vec2f) -> vec4f {\n  if (uv.x > 0.5) { return vec4f(1.0); }\n}", "1:4: shade does not return a value on every path"},
		{"fn a(x: f32) -> f32 { return b(x); }\nfn b(x: f32) -> f32 { return a(x); }\nfn shade(uv: vec2f) -> vec4f { return vec4f(a(uv.x)); }", "calls itself"},
		{"fn shade(uv: vec2f) -> vec4f { return vec4f(1.0) /* open", "the block comment is not closed"},
		{"fn shade(uv: vec2f) -> vec4f { let i = 3000000000i; return vec4f(1.0); }", "does not fit a i32"},
		{"fn shade(uv: vec2f) -> vec4f { return vec4f(uv.xyz, 1.0); }", "1:48: a vec2<f32> has no member xyz"},
		{"var<private> g: f32 = nsh.time;\nfn shade(uv: vec2f) -> vec4f { return vec4f(g); }", "a var<private> initializer must be a constant expression"},
		{"var<private> a: f32 = 1.0;\nvar<private> b: f32 = a;\nfn shade(uv: vec2f) -> vec4f { return vec4f(b); }", "a var<private> initializer must be a constant expression"},
		{"fn h() -> f32 { return 1.0; }\nvar<private> g: f32 = h();\nfn shade(uv: vec2f) -> vec4f { return vec4f(g); }", "a var<private> initializer must be a constant expression"},
		{"var<private> g: vec4f = sample_content(vec2f(0.5));\nfn shade(uv: vec2f) -> vec4f { return g; }", "a var<private> initializer must be a constant expression"},
		{"var<private> g;\nfn shade(uv: vec2f) -> vec4f { return vec4f(1.0); }", "has neither a type nor a value"},
		{"struct Empty {}\nfn shade(uv: vec2f) -> vec4f { return vec4f(1.0); }", "struct Empty has no members"},
		{"struct Dup { a: f32, a: f32 }\nfn shade(uv: vec2f) -> vec4f { return vec4f(1.0); }", "struct Dup has member a twice"},
		{"struct A { b: B }\nstruct B { a: A }\nfn shade(uv: vec2f) -> vec4f { return vec4f(1.0); }", "struct A contains itself"},
		{"struct A { x: f32 }\nstruct B { x: f32 }\nfn shade(uv: vec2f) -> vec4f { let a: A = B(1.0); return vec4f(a.x); }", "cannot use a B as a A"},
		{"struct A { x: f32 }\nfn shade(uv: vec2f) -> vec4f { let a = A(1.0, 2.0); return vec4f(a.x); }", "struct A takes 1 members, not 2"},
		{"struct A { x: f32 }\nfn shade(uv: vec2f) -> vec4f { let a = A(1.0); return vec4f(a.y); }", "struct A has no member y"},
		{"struct A { x: f32 }\nfn shade(uv: vec2f) -> vec4f { let a = A(1.0) + A(2.0); return vec4f(a.x); }", "+ takes numbers"},
		{"fn shade(uv: vec2f) -> vec4f { let m = mat2x3f(); return vec4f(m[2], 1.0); }", "out of the mat2x3<f32>'s range"},
		{"fn shade(uv: vec2f) -> vec4f { let m = mat2x3f(); let x = m * m; return vec4f(1.0); }", "* is not defined on a mat2x3<f32> and a mat2x3<f32>"},
		{"fn shade(uv: vec2f) -> vec4f { let m = mat2x3f(); let x = m * vec3f(1.0); return vec4f(1.0); }", "* is not defined on a mat2x3<f32> and a vec3<f32>"},
		{"fn shade(uv: vec2f) -> vec4f { let m = mat2x3f(); return vec4f(determinant(m)); }", "determinant takes a square matrix, not a mat2x3<f32>"},
		{"fn shade(uv: vec2f) -> vec4f { return vec4f(f32(countOneBits(uv.x))); }", "countOneBits takes i32, u32 or vectors of them"},
		{"fn shade(uv: vec2f) -> vec4f { return vec4f(f32(pack4x8unorm(uv))); }", "cannot use a vec2<f32> as a vec4<f32>"},
		{"fn shade(uv: vec2f) -> vec4f { return vec4f(modf(1i).fract); }", "modf takes f32 or vectors of it"},
		{"fn shade(uv: vec2f) -> vec4f { return vec4f(frexp(1u).fract); }", "frexp takes f32 or vectors of it"},
		{"fn shade(uv: vec2f) -> vec4f { let m: __modf_result_f32 = modf(uv.x); return vec4f(m.fract); }", "__modf_result_f32 is not a type"},
		{"fn shade(uv: vec2f) -> vec4f { return vec4f(f32(__wgsl_clz_u32(1u))); }", "__wgsl_clz_u32 is not declared"},
		{"fn shade(uv: vec2f) -> vec4f { return nsh.depth; }", "nsh has no member depth"},
		{"fn shade(uv: vec2f) -> vec4f { return __texel(0, uv); }", "__texel is not declared"},
		{"fn shade(uv: vec2f) -> vec4f { return vec4f(uniforms.x); }", "uniforms needs struct Uniforms"},
	} {
		_, err := wgsl.Compile(c.src)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: got %v, want %q", c.src, err, c.want)
		}
	}
	if _, err := wgsl.Compile(strings.Repeat(" ", wgsl.MaxSourceBytes+1)); err == nil {
		t.Error("a source over the limit compiled")
	}
}

// Constants fold as WGSL's constant expressions do.
func TestConstants(t *testing.T) {
	for _, src := range []string{
		"const A = 7 / 2; const_assert A == 3;",
		"const B = -7 % 3; const_assert B == -1;",
		"const C = 1u << 31u; const_assert C == 2147483648u;",
		"const D = 1.5 % 1.0; const_assert D == 0.5;",
		"const E = i32(2.9); const_assert E == 2;",
		"const F = vec3(1, 2, 3).z + 1; const_assert F == 4;",
		"const G = ~0u; const_assert G == 4294967295u;",
		// Bit-counting built-ins (scalar & vector, signed & unsigned).
		"const_assert countLeadingZeros(0u) == 32u && countLeadingZeros(1u) == 31u && countLeadingZeros(0x80000000u) == 0u;",
		"const_assert countLeadingZeros(-1i) == 0i && countLeadingZeros(0i) == 32i && countLeadingZeros(16i) == 27i;",
		"const_assert countOneBits(0u) == 0u && countOneBits(0xF0F00F0Fu) == 16u && countOneBits(-1i) == 32i;",
		"const_assert countTrailingZeros(0u) == 32u && countTrailingZeros(1u) == 0u && countTrailingZeros(0x80000000u) == 31u;",
		"const_assert countTrailingZeros(8i) == 3i && countTrailingZeros(0i) == 32i;",
		"const_assert firstLeadingBit(0u) == 0xFFFFFFFFu && firstLeadingBit(1u) == 0u && firstLeadingBit(0x80000000u) == 31u;",
		"const_assert firstLeadingBit(0i) == -1i && firstLeadingBit(-1i) == -1i && firstLeadingBit(-2i) == 0i && firstLeadingBit(12i) == 3i;",
		"const_assert firstTrailingBit(0u) == 0xFFFFFFFFu && firstTrailingBit(12u) == 2u && firstTrailingBit(0i) == -1i && firstTrailingBit(-8i) == 3i;",
		"const_assert reverseBits(0x12345678u) == 0x1E6A2C48u && reverseBits(1i) == i32(-2147483648);",
		"const_assert extractBits(0x12345678u, 4u, 8u) == 0x67u && extractBits(0x12345678u, 32u, 4u) == 0u && extractBits(0x12345678u, 0u, 0u) == 0u;",
		"const_assert extractBits(0xF0i, 4u, 4u) == -1i && extractBits(0x70i, 4u, 4u) == 7i;",
		"const_assert insertBits(0x12345678u, 0xABu, 8u, 8u) == 0x1234AB78u && insertBits(0x12345678u, 0xDEADBEEFu, 0u, 32u) == 0xDEADBEEFu && insertBits(0x12345678u, 0xFFu, 4u, 0u) == 0x12345678u;",
		"const_assert countOneBits(vec2u(3u, 7u)).y == 3u && extractBits(vec2i(-1i, 15i), 0u, 4u).x == -1i && insertBits(vec2u(0u, 0u), vec2u(5u, 9u), 4u, 4u).y == 0x90u;",
		// Packing and unpacking built-ins.
		"const_assert pack4x8unorm(vec4f(0.0, 1.0, 0.0, 1.0)) == 0xFF00FF00u;",
		"const_assert unpack4x8unorm(0xFF00FF00u).y == 1.0 && unpack4x8unorm(0xFF00FF00u).x == 0.0;",
		"const_assert pack4x8snorm(vec4f(1.0, -1.0, 0.0, 1.0)) == 0x7F00817Fu;",
		"const_assert unpack4x8snorm(0x00807F00u).y == 1.0 && unpack4x8snorm(0x00807F00u).z == -1.0;",
		"const_assert pack2x16unorm(vec2f(0.0, 1.0)) == 0xFFFF0000u && unpack2x16unorm(0xFFFF0000u).y == 1.0;",
		"const_assert pack2x16snorm(vec2f(1.0, -1.0)) == 0x80017FFFu && unpack2x16snorm(0x80007FFFu).x == 1.0 && unpack2x16snorm(0x80007FFFu).y == -1.0;",
		"const_assert pack2x16float(vec2f(1.0, -2.0)) == 0xC0003C00u && unpack2x16float(0xC0003C00u).x == 1.0 && unpack2x16float(0xC0003C00u).y == -2.0;",
		"const_assert pack4xI8(vec4i(1i, -1i, 127i, -128i)) == 0x807FFF01u && unpack4xI8(0x807FFF01u).w == -128i;",
		"const_assert pack4xU8(vec4u(1u, 2u, 3u, 4u)) == 0x04030201u && unpack4xU8(0x04030201u).z == 3u;",
		"const_assert pack4xI8Clamp(vec4i(-200i, 200i, 0i, 1i)) == 0x01007F80u && pack4xU8Clamp(vec4u(300u, 1u, 2u, 3u)) == 0x030201FFu;",
		// modf and frexp constant folding.
		"const M1 = modf(3.75); const_assert M1.whole == 3.0 && M1.fract == 0.75;",
		"const M2 = modf(-1.25); const_assert M2.whole == -1.0 && M2.fract == -0.25;",
		"const MV = modf(vec2f(2.5, -3.25)); const_assert MV.whole.x == 2.0 && MV.fract.y == -0.25;",
		"const F1 = frexp(1.5); const_assert F1.fract == 0.75 && F1.exp == 1i;",
		"const F0 = frexp(0.0); const_assert F0.fract == 0.0 && F0.exp == 0i;",
		"const FV = frexp(vec2f(-3.0, 0.5)); const_assert FV.fract.x == -0.75 && FV.exp.x == 2i && FV.fract.y == 0.5 && FV.exp.y == 0i;",
		// Const structs and nested const structs.
		"struct Inner { v: f32 }\nstruct Outer { a: Inner, b: vec2f }\nconst O = Outer(Inner(2.5), vec2f(1.0, 3.0));\nconst_assert O.a.v == 2.5 && O.b.y == 3.0;",
	} {
		if _, err := wgsl.Compile(src + "\nfn shade(uv: vec2f) -> vec4f { return vec4f(1.0); }"); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
	if _, err := wgsl.Compile("const_assert 1 == 2;\nfn shade(uv: vec2f) -> vec4f { return vec4f(1.0); }"); err == nil {
		t.Error("a false assertion compiled")
	}
}

func TestRestFeatures(t *testing.T) {
	// 1. Local const struct referencing local const scalar + out-of-order structs/privates/aliases.
	src := `
alias MyPair = Pair;
fn read_g() -> f32 { return g_val + g_pair.a; }
var<private> g_val: f32 = 0.5;
var<private> g_pair: MyPair = MyPair(0.25, vec2<bool>(true, false));
struct Pair { a: f32, flags: vec2<bool> }
fn shade(uv: vec2f) -> vec4f {
	const k = 1.5;
	const lp = Pair(k, vec2<bool>(false, true));
	let copy = lp;
	var z = MyPair();
	z.flags.x = true;
	if (z.flags.x && !copy.flags.x) {
		g_val += copy.a;
	}
	return vec4f(read_g(), uv.x, uv.y, 1.0);
}
`
	p, err := wgsl.Compile(src)
	if err != nil {
		t.Fatalf("compile local const struct & forward refs: %v", err)
	}
	for lang, out := range map[string]string{
		"glsl": glsl.Fragment(p.IR, "f", glsl.Options{Block: "U"}),
		"msl":  msl.Fragment(p.IR, "f"),
		"hlsl": hlsl.Fragment(p.IR, "f", 0, 1),
	} {
		if strings.Contains(out, "?(") {
			t.Errorf("%s has invalid output:\n%s", lang, out)
		}
	}
	if !strings.Contains(msl.Fragment(p.IR, "f"), "struct Privates {") {
		t.Errorf("MSL missing Privates struct")
	}
	if !strings.Contains(hlsl.Fragment(p.IR, "f", 0, 1), "static float P0;") {
		t.Errorf("HLSL missing static global P0")
	}
	if !strings.Contains(glsl.Fragment(p.IR, "f", glsl.Options{Block: "U"}), "float P0;") {
		t.Errorf("GLSL missing global P0")
	}

	// 2. Simple shader should not emit unused __wgsl_* helpers or Privates.
	simple, err := wgsl.Compile(`fn shade(uv: vec2f) -> vec4f { return vec4f(uv, 0.0, 1.0); }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(simple.IR.Privates) != 0 {
		t.Errorf("simple shader has %d privates, want 0", len(simple.IR.Privates))
	}
	if len(simple.IR.Funcs) > 2 {
		t.Errorf("simple shader compiled %d funcs (unused helpers leaked)", len(simple.IR.Funcs))
	}
	if strings.Contains(msl.Fragment(simple.IR, "f"), "struct Privates") {
		t.Errorf("simple shader emitted Privates struct")
	}
}
