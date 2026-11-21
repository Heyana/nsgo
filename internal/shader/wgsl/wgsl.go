// Package wgsl compiles the shaders TSX apps write in WGSL, as NSH's are,
// into package shaderir's programs, which every backend draws as it draws
// a Kage program (package shader).
//
// An app writes fn shade(uv: vec2f) -> vec4f, the straight sRGB color of
// the point uv of the view's box (0 to 1 from its top left corner), with
// helpers and consts of its own. It reads nsh.size (the box in points),
// nsh.time (seconds since the shader appeared), nsh.scale (pixels a
// point), nsh.progress (an effect's progress or a transition's, 0 to 1)
// and nsh.overflow (the points the shader draws beyond each side of its
// box), its own values through uniforms when it declares struct
// Uniforms, and pictures of other views with sample_content,
// sample_backdrop, sample_from and sample_to.
//
// The program's uniforms are the bridge's (package shader: the surface's
// size in points, its scale, and the pictures' sizes when it reads any),
// then __nsh, a vec3 of the time, the progress and the overflow
// (BuiltinLanes), then the members of struct Uniforms in turn. Its
// textures are the pictures, content, backdrop, from and to, up to the
// last it reads. The surface a platform draws covers the box and the
// overflow on each side.
package wgsl

import (
	"fmt"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// Limits on what a shader may be, as NSH's.
const (
	MaxSourceBytes  = 64 << 10
	MaxUniformBytes = 4096
)

// BuiltinLanes is how many lanes __nsh takes after the bridge's: the
// time, the progress and the overflow.
const BuiltinLanes = 3

// The pictures a program reads, a bit each and in the order of its
// textures.
const (
	Content Inputs = 1 << iota
	Backdrop
	From
	To
)

const maxPictures = 4

// Inputs is a set of pictures.
type Inputs uint8

// Program is a compiled shader.
type Program struct {
	IR *shaderir.Program
	// Uniforms are struct Uniforms' members, in their order.
	Uniforms []Uniform
	// Animated is whether shade reads nsh.time, so the shader draws every
	// frame.
	Animated bool
	// Inputs are the pictures shade reads, through the functions it
	// calls.
	Inputs Inputs
}

// Uniform is a member of struct Uniforms.
type Uniform struct {
	Name string
	// Type is its WGSL type (vec4<f32>).
	Type string
	// Lanes is how many lanes it takes; Vec3 is whether it is a vec3f,
	// which a color fills without its alpha.
	Lanes int
	Vec3  bool
}

// Error is why a shader did not compile, at a line and a column (in
// bytes) from 1 of the app's source, or 0 where it has no place there.
type Error struct {
	Line, Column int
	Message      string
	offset       int
}

func (e *Error) Error() string {
	if e.Line == 0 {
		return "shader: " + e.Message
	}
	return fmt.Sprintf("shader: %d:%d: %s", e.Line, e.Column, e.Message)
}

func errorAt(offset int, format string, args ...any) *Error {
	return &Error{offset: offset, Message: fmt.Sprintf(format, args...)}
}

// Compile checks src and writes it as a program.
func Compile(src string) (*Program, error) {
	if len(src) > MaxSourceBytes {
		return nil, &Error{Message: fmt.Sprintf("the source takes %d bytes, more than %d", len(src), MaxSourceBytes)}
	}
	full := src + "\n" + builtinSource()
	var p *Program
	err := catch(func() {
		m, err := parse(full)
		if err != nil {
			panic(bail{err.(*Error)})
		}
		p = compile(m, len(src))
	})
	if err != nil {
		e := err.(*Error)
		if e.offset < len(src) {
			e.Line, e.Column = position(src, e.offset)
		}
		return nil, e
	}
	p.IR.SourceID = shaderir.CalcSourceID([]byte(src))
	return p, nil
}

// position is the line and the column of offset in src, from 1.
func position(src string, offset int) (line, col int) {
	before := src[:offset]
	line = strings.Count(before, "\n") + 1
	return line, offset - strings.LastIndexByte(before, '\n')
}

func compile(m *module, user int) *Program {
	c := &checker{m: m, user: user, prog: &shaderir.Program{}}
	c.check()
	shade := c.global.names["shade"].fn
	time, textures := reach(shade)
	ir := c.prog
	ir.TextureCount = 0
	for i := range maxPictures {
		if textures&(1<<i) != 0 {
			ir.TextureCount = i + 1
		}
	}
	if ir.TextureCount == 0 {
		// No picture is read: the image sizes leave the bridge, and the
		// uniforms after them move down.
		ir.UniformNames = append(ir.UniformNames[:2], ir.UniformNames[3:]...)
		ir.Uniforms = append(ir.Uniforms[:2], ir.Uniforms[3:]...)
		renumber(ir, 2)
	} else {
		ir.Uniforms[2].Length = ir.TextureCount
	}
	return &Program{IR: ir, Uniforms: c.members, Animated: time, Inputs: Inputs(textures)}
}

// renumber moves every uniform after removed down one.
func renumber(p *shaderir.Program, removed int) {
	var walkExpr func(e *shaderir.Expr)
	walkExpr = func(e *shaderir.Expr) {
		if e.Type == shaderir.UniformVariable && e.Index > removed {
			e.Index--
		}
		for i := range e.Exprs {
			walkExpr(&e.Exprs[i])
		}
	}
	var walk func(b *shaderir.Block)
	walk = func(b *shaderir.Block) {
		if b == nil {
			return
		}
		for i := range b.Stmts {
			s := &b.Stmts[i]
			for j := range s.Exprs {
				walkExpr(&s.Exprs[j])
			}
			for _, sb := range s.Blocks {
				walk(sb)
			}
		}
	}
	for _, f := range p.Funcs {
		walk(f.Block)
	}
	walk(p.FragmentFunc.Block)
}

// builtinSource is the source after the app's: the pictures' samplers
// and the entry point, which clamps shade's color and premultiplies it. A
// picture covers the surface, the box and the overflow beyond each side,
// and is read between its pixels, transparent outside it.
func builtinSource() string {
	var b strings.Builder
	b.WriteString(`
fn __nsh_picture(uv: vec2f) -> vec2f {
	return (uv * nsh.size + nsh.overflow) / max(nsh.size + 2.0 * nsh.overflow, vec2f(1e-6));
}

fn __nsh_straight(c: vec4f) -> vec4f {
	if (c.a <= 0.0) {
		return vec4f(0.0);
	}
	return vec4f(c.rgb / c.a, c.a);
}
`)
	for i, name := range []string{"content", "backdrop", "from", "to"} {
		fmt.Fprintf(&b, `
fn sample_%[2]s(uv: vec2f) -> vec4f {
	let p = __nsh_picture(uv);
	if (any(p < vec2f(0.0)) || any(p > vec2f(1.0))) {
		return vec4f(0.0);
	}
	let size = __picture_size(%[1]d);
	let hi = max(size - 1.0, vec2f(0.0));
	let q = p * size - 0.5;
	let i = floor(q);
	let f = q - i;
	let a = clamp(i, vec2f(0.0), hi);
	let b = clamp(i + 1.0, vec2f(0.0), hi);
	let top = mix(__texel(%[1]d, a), __texel(%[1]d, vec2f(b.x, a.y)), f.x);
	let bottom = mix(__texel(%[1]d, vec2f(a.x, b.y)), __texel(%[1]d, b), f.x);
	return __nsh_straight(mix(top, bottom, f.y));
}
`, i, name)
	}
	b.WriteString(`
struct __modf_result_f32 { fract: f32, whole: f32 }
struct __modf_result_vec2_f32 { fract: vec2f, whole: vec2f }
struct __modf_result_vec3_f32 { fract: vec3f, whole: vec3f }
struct __modf_result_vec4_f32 { fract: vec4f, whole: vec4f }

struct __frexp_result_f32 { fract: f32, exp: i32 }
struct __frexp_result_vec2_f32 { fract: vec2f, exp: vec2i }
struct __frexp_result_vec3_f32 { fract: vec3f, exp: vec3i }
struct __frexp_result_vec4_f32 { fract: vec4f, exp: vec4i }

fn __wgsl_clz_u32(v: u32) -> u32 {
	var x = v;
	if (x == 0u) {
		return 32u;
	}
	var n = 0u;
	if ((x & 0xFFFF0000u) == 0u) { n += 16u; x <<= 16u; }
	if ((x & 0xFF000000u) == 0u) { n += 8u; x <<= 8u; }
	if ((x & 0xF0000000u) == 0u) { n += 4u; x <<= 4u; }
	if ((x & 0xC0000000u) == 0u) { n += 2u; x <<= 2u; }
	if ((x & 0x80000000u) == 0u) { n += 1u; }
	return n;
}

fn __wgsl_ctz_u32(v: u32) -> u32 {
	var x = v;
	if (x == 0u) {
		return 32u;
	}
	var n = 0u;
	if ((x & 0x0000FFFFu) == 0u) { n += 16u; x >>= 16u; }
	if ((x & 0x000000FFu) == 0u) { n += 8u; x >>= 8u; }
	if ((x & 0x0000000Fu) == 0u) { n += 4u; x >>= 4u; }
	if ((x & 0x00000003u) == 0u) { n += 2u; x >>= 2u; }
	if ((x & 0x00000001u) == 0u) { n += 1u; }
	return n;
}

fn __wgsl_popcnt_u32(v: u32) -> u32 {
	var x = v;
	x = x - ((x >> 1u) & 0x55555555u);
	x = (x & 0x33333333u) + ((x >> 2u) & 0x33333333u);
	x = (x + (x >> 4u)) & 0x0F0F0F0Fu;
	x = x + (x >> 8u);
	x = x + (x >> 16u);
	return x & 0x3Fu;
}

fn __wgsl_rev_u32(v: u32) -> u32 {
	var x = v;
	x = ((x & 0x55555555u) << 1u) | ((x >> 1u) & 0x55555555u);
	x = ((x & 0x33333333u) << 2u) | ((x >> 2u) & 0x33333333u);
	x = ((x & 0x0F0F0F0Fu) << 4u) | ((x >> 4u) & 0x0F0F0F0Fu);
	x = ((x & 0x00FF00FFu) << 8u) | ((x >> 8u) & 0x00FF00FFu);
	return (x << 16u) | (x >> 16u);
}

fn __wgsl_flb_u32(v: u32) -> u32 {
	if (v == 0u) {
		return 0xFFFFFFFFu;
	}
	return 31u - __wgsl_clz_u32(v);
}

fn __wgsl_flb_i32(v: i32) -> i32 {
	let u = select(u32(v), ~u32(v), v < 0i);
	if (u == 0u) {
		return -1i;
	}
	return i32(31u - __wgsl_clz_u32(u));
}

fn __wgsl_ftb_u32(v: u32) -> u32 {
	if (v == 0u) {
		return 0xFFFFFFFFu;
	}
	return __wgsl_ctz_u32(v);
}

fn __wgsl_extract_u32(e: u32, offset: u32, count: u32) -> u32 {
	let o = min(offset, 32u);
	let c = min(count, 32u - o);
	if (c == 0u) {
		return 0u;
	}
	return (e << (32u - (o + c))) >> (32u - c);
}

fn __wgsl_extract_i32(e: i32, offset: u32, count: u32) -> i32 {
	let o = min(offset, 32u);
	let c = min(count, 32u - o);
	if (c == 0u) {
		return 0i;
	}
	let u = (u32(e) << (32u - (o + c))) >> (32u - c);
	let sign_bit = 1u << (c - 1u);
	return i32((u ^ sign_bit) - sign_bit);
}

fn __wgsl_insert_u32(e: u32, nb: u32, offset: u32, count: u32) -> u32 {
	let o = min(offset, 32u);
	let c = min(count, 32u - o);
	if (c == 0u) {
		return e;
	}
	if (c == 32u) {
		return nb;
	}
	let mask = ((1u << c) - 1u) << o;
	return (e & ~mask) | ((nb << o) & mask);
}

fn __wgsl_f32_to_f16(x: f32) -> u32 {
	let bits = bitcast<u32>(x);
	let sign = (bits >> 16u) & 0x8000u;
	let exp = (bits >> 23u) & 0xFFu;
	let mant = bits & 0x7FFFFFu;
	if (exp == 255u) {
		if (mant != 0u) {
			return sign | 0x7E00u | (mant >> 13u);
		}
		return sign | 0x7C00u;
	}
	if (exp > 142u) {
		return sign | 0x7C00u;
	}
	if (exp >= 113u) {
		let h_exp = (exp - 112u) << 10u;
		let h_mant = mant >> 13u;
		let rem = mant & 0x1FFFu;
		var h = h_exp | h_mant;
		if (rem > 0x1000u || (rem == 0x1000u && (h_mant & 1u) != 0u)) {
			h += 1u;
		}
		return sign | h;
	}
	if (exp >= 103u) {
		let full_mant = 0x800000u | mant;
		let shift = 126u - exp;
		var h_mant = full_mant >> shift;
		let half = 1u << (shift - 1u);
		let rem = full_mant & ((1u << shift) - 1u);
		if (rem > half || (rem == half && (h_mant & 1u) != 0u)) {
			h_mant += 1u;
		}
		return sign | h_mant;
	}
	if (exp == 102u && mant != 0u) {
		return sign | 1u;
	}
	return sign;
}

fn __wgsl_f16_to_f32(h: u32) -> f32 {
	let sign = (h & 0x8000u) << 16u;
	let exp = (h >> 10u) & 0x1Fu;
	let mant = h & 0x3FFu;
	if (exp == 0u) {
		if (mant == 0u) {
			return bitcast<f32>(sign);
		}
		let f = f32(mant) * 5.960464477539063e-08;
		return select(f, -f, sign != 0u);
	}
	if (exp == 31u) {
		if (mant == 0u) {
			return bitcast<f32>(sign | 0x7F800000u);
		}
		return bitcast<f32>(sign | 0x7FC00000u | (mant << 13u));
	}
	return bitcast<f32>(sign | ((exp + 112u) << 23u) | (mant << 13u));
}

fn __wgsl_pack4x8snorm(e: vec4f) -> u32 {
	let v = vec4i(round(clamp(e, vec4f(-1.0), vec4f(1.0)) * 127.0));
	let u = vec4u(v) & vec4u(0xFFu);
	return u.x | (u.y << 8u) | (u.z << 16u) | (u.w << 24u);
}

fn __wgsl_pack4x8unorm(e: vec4f) -> u32 {
	let v = vec4u(round(clamp(e, vec4f(0.0), vec4f(1.0)) * 255.0));
	return (v.x & 0xFFu) | ((v.y & 0xFFu) << 8u) | ((v.z & 0xFFu) << 16u) | ((v.w & 0xFFu) << 24u);
}

fn __wgsl_pack2x16snorm(e: vec2f) -> u32 {
	let v = vec2i(round(clamp(e, vec2f(-1.0), vec2f(1.0)) * 32767.0));
	let u = vec2u(v) & vec2u(0xFFFFu);
	return u.x | (u.y << 16u);
}

fn __wgsl_pack2x16unorm(e: vec2f) -> u32 {
	let v = vec2u(round(clamp(e, vec2f(0.0), vec2f(1.0)) * 65535.0));
	return (v.x & 0xFFFFu) | ((v.y & 0xFFFFu) << 16u);
}

fn __wgsl_pack2x16float(e: vec2f) -> u32 {
	return __wgsl_f32_to_f16(e.x) | (__wgsl_f32_to_f16(e.y) << 16u);
}

fn __wgsl_pack4xI8(e: vec4i) -> u32 {
	let u = vec4u(e) & vec4u(0xFFu);
	return u.x | (u.y << 8u) | (u.z << 16u) | (u.w << 24u);
}

fn __wgsl_pack4xU8(e: vec4u) -> u32 {
	let u = e & vec4u(0xFFu);
	return u.x | (u.y << 8u) | (u.z << 16u) | (u.w << 24u);
}

fn __wgsl_pack4xI8Clamp(e: vec4i) -> u32 {
	return __wgsl_pack4xI8(clamp(e, vec4i(-128i), vec4i(127i)));
}

fn __wgsl_pack4xU8Clamp(e: vec4u) -> u32 {
	return __wgsl_pack4xU8(min(e, vec4u(255u)));
}

fn __wgsl_unpack4x8snorm(e: u32) -> vec4f {
	let x = i32(((e & 0xFFu) ^ 0x80u) - 0x80u);
	let y = i32((((e >> 8u) & 0xFFu) ^ 0x80u) - 0x80u);
	let z = i32((((e >> 16u) & 0xFFu) ^ 0x80u) - 0x80u);
	let w = i32((((e >> 24u) & 0xFFu) ^ 0x80u) - 0x80u);
	return max(vec4f(f32(x), f32(y), f32(z), f32(w)) / 127.0, vec4f(-1.0));
}

fn __wgsl_unpack4x8unorm(e: u32) -> vec4f {
	return vec4f(f32(e & 0xFFu), f32((e >> 8u) & 0xFFu), f32((e >> 16u) & 0xFFu), f32((e >> 24u) & 0xFFu)) / 255.0;
}

fn __wgsl_unpack2x16snorm(e: u32) -> vec2f {
	let x = i32(((e & 0xFFFFu) ^ 0x8000u) - 0x8000u);
	let y = i32((((e >> 16u) & 0xFFFFu) ^ 0x8000u) - 0x8000u);
	return max(vec2f(f32(x), f32(y)) / 32767.0, vec2f(-1.0));
}

fn __wgsl_unpack2x16unorm(e: u32) -> vec2f {
	return vec2f(f32(e & 0xFFFFu), f32((e >> 16u) & 0xFFFFu)) / 65535.0;
}

fn __wgsl_unpack2x16float(e: u32) -> vec2f {
	return vec2f(__wgsl_f16_to_f32(e & 0xFFFFu), __wgsl_f16_to_f32((e >> 16u) & 0xFFFFu));
}

fn __wgsl_unpack4xI8(e: u32) -> vec4i {
	let x = i32(((e & 0xFFu) ^ 0x80u) - 0x80u);
	let y = i32((((e >> 8u) & 0xFFu) ^ 0x80u) - 0x80u);
	let z = i32((((e >> 16u) & 0xFFu) ^ 0x80u) - 0x80u);
	let w = i32((((e >> 24u) & 0xFFu) ^ 0x80u) - 0x80u);
	return vec4i(x, y, z, w);
}

fn __wgsl_unpack4xU8(e: u32) -> vec4u {
	return vec4u(e & 0xFFu, (e >> 8u) & 0xFFu, (e >> 16u) & 0xFFu, (e >> 24u) & 0xFFu);
}

fn __wgsl_modf_f32(x: f32) -> __modf_result_f32 {
	let w = trunc(x);
	return __modf_result_f32(x - w, w);
}

fn __wgsl_modf_vec2_f32(x: vec2f) -> __modf_result_vec2_f32 {
	let w = trunc(x);
	return __modf_result_vec2_f32(x - w, w);
}

fn __wgsl_modf_vec3_f32(x: vec3f) -> __modf_result_vec3_f32 {
	let w = trunc(x);
	return __modf_result_vec3_f32(x - w, w);
}

fn __wgsl_modf_vec4_f32(x: vec4f) -> __modf_result_vec4_f32 {
	let w = trunc(x);
	return __modf_result_vec4_f32(x - w, w);
}

fn __wgsl_frexp_f32(x: f32) -> __frexp_result_f32 {
	let bits = bitcast<u32>(x);
	let abs_bits = bits & 0x7FFFFFFFu;
	if (abs_bits == 0u || abs_bits >= 0x7F800000u) {
		return __frexp_result_f32(x, 0i);
	}
	let sign = bits & 0x80000000u;
	var u = abs_bits;
	var e_adj = 0i;
	if (u < 0x00800000u) {
		u = bitcast<u32>(bitcast<f32>(u) * 16777216.0);
		e_adj = -24i;
	}
	let exp = i32((u >> 23u) & 0xFFu) - 126i + e_adj;
	let fract = bitcast<f32>(sign | 0x3F000000u | (u & 0x007FFFFFu));
	return __frexp_result_f32(fract, exp);
}

fn __wgsl_frexp_vec2_f32(x: vec2f) -> __frexp_result_vec2_f32 {
	let a = __wgsl_frexp_f32(x.x);
	let b = __wgsl_frexp_f32(x.y);
	return __frexp_result_vec2_f32(vec2f(a.fract, b.fract), vec2i(a.exp, b.exp));
}

fn __wgsl_frexp_vec3_f32(x: vec3f) -> __frexp_result_vec3_f32 {
	let a = __wgsl_frexp_f32(x.x);
	let b = __wgsl_frexp_f32(x.y);
	let c = __wgsl_frexp_f32(x.z);
	return __frexp_result_vec3_f32(vec3f(a.fract, b.fract, c.fract), vec3i(a.exp, b.exp, c.exp));
}

fn __wgsl_frexp_vec4_f32(x: vec4f) -> __frexp_result_vec4_f32 {
	let a = __wgsl_frexp_f32(x.x);
	let b = __wgsl_frexp_f32(x.y);
	let c = __wgsl_frexp_f32(x.z);
	let d = __wgsl_frexp_f32(x.w);
	return __frexp_result_vec4_f32(vec4f(a.fract, b.fract, c.fract, d.fract), vec4i(a.exp, b.exp, c.exp, d.exp));
}

fn __nsh_fragment(pos: vec2f) -> vec4f {
	let uv = (pos - nsh.overflow) / max(nsh.size, vec2f(1e-6));
	let color = clamp(shade(uv), vec4f(0.0), vec4f(1.0));
	return vec4f(color.rgb * color.a, color.a);
}
`)
	return b.String()
}

// Crossfade is the transitions' default program: from sample_from to
// sample_to as nsh.progress goes from 0 to 1.
const Crossfade = `fn shade(uv: vec2f) -> vec4f {
	return mix(sample_from(uv), sample_to(uv), nsh.progress);
}
`
