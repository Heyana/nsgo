struct Material {
	albedo: vec3f,
	roughness: f32,
	weights: array<f32, 2>,
}

struct Hit {
	pos: vec2f,
	mat: Material,
}

alias HitAlias = Hit;

var<private> g_accum: f32 = 0.25;
var<private> g_zero: vec2f;
var<private> g_hit: Hit = Hit(vec2f(0.1, 0.2), Material(vec3f(0.5, 0.25, 0.75), 0.5, array<f32, 2>(0.2, 0.8)));
var<private> g_arr: array<f32, 2> = array<f32, 2>(0.3, 0.7);

fn add_global(delta: f32) {
	g_accum += delta;
	g_zero = vec2f(g_accum, g_arr[0] + g_arr[1]);
}

fn make_hit(uv: vec2f) -> HitAlias {
	var m = Material(vec3f(uv.x, uv.y, 0.5), 0.25, array<f32, 2>(0.4, 0.6));
	m.roughness += 0.1;
	return Hit(uv, m);
}

fn shade_hit(h: Hit) -> vec3f {
	return h.mat.albedo * (h.mat.roughness + h.mat.weights[0] + h.mat.weights[1]);
}

fn test_matrices(uv: vec2f) -> vec4f {
	let m23: mat2x3<f32> = mat2x3f(vec3f(1.0, 0.0, 0.5), vec3f(0.0, 1.0, 0.5));
	let m32 = mat3x2<f32>(vec2f(1.0, 0.0), vec2f(0.0, 1.0), vec2f(0.5, 0.5));
	let m24 = mat2x4f(vec4f(1.0, 0.0, 0.0, 0.0), vec4f(0.0, 1.0, 0.0, 1.0));
	let m42 = transpose(m24);
	let m34 = mat3x4f(
		1.0, 0.0, 0.0, 0.0,
		0.0, 1.0, 0.0, 0.0,
		0.0, 0.0, 1.0, 1.0,
	);
	let m43 = transpose(m34);
	let v3 = m23 * uv;
	let v2 = v3 * m23;
	let sq = m32 * m23;
	let v4 = m34 * (m43 * (m24 * (m42 * (m24 * v2))));
	let d = determinant(sq);
	return vec4f(v2 + m32[2], v3.z + d * 0.01, v4.w);
}

fn test_bits(uv: vec2f) -> f32 {
	let u = u32(uv.x * 255.0) + 1u;
	let i = i32(uv.y * 127.0) - 64i;
	let uv2 = vec2u(u, u + 3u);
	let iv2 = vec2i(i, -i - 1i);
	let a = countLeadingZeros(u) + countOneBits(u) + countTrailingZeros(u);
	let b = firstLeadingBit(u) + firstTrailingBit(u) + reverseBits(u);
	let c = countLeadingZeros(i) + countOneBits(i) + countTrailingZeros(i);
	let d = firstLeadingBit(i) + firstTrailingBit(i) + reverseBits(i);
	let exu = extractBits(u, 2u, 4u);
	let exi = extractBits(i, 1u, 5u);
	let inu = insertBits(u, 0xAu, 4u, 4u);
	let ini = insertBits(i, -3i, 2u, 4u);
	let va = countOneBits(uv2) + countLeadingZeros(uv2) + countTrailingZeros(uv2);
	let vb = firstLeadingBit(iv2) + firstTrailingBit(iv2) + reverseBits(iv2);
	let vex = extractBits(iv2, 0u, 4u) + insertBits(iv2, vec2i(1i, 2i), 2u, 3u);
	return f32((a + b + exu + inu + va.x + va.y) & 0xFFu) / 255.0 + f32((c + d + exi + ini + vb.x + vex.y) & 0xFFi) / 255.0;
}

fn test_pack(uv: vec2f) -> vec4f {
	let p1 = pack4x8snorm(vec4f(uv * 2.0 - 1.0, 0.5, -0.5));
	let u1 = unpack4x8snorm(p1);
	let p2 = pack4x8unorm(vec4f(uv, 0.25, 0.75));
	let u2 = unpack4x8unorm(p2);
	let p3 = pack2x16snorm(uv * 2.0 - 1.0);
	let u3 = unpack2x16snorm(p3);
	let p4 = pack2x16unorm(uv);
	let u4 = unpack2x16unorm(p4);
	let p5 = pack2x16float(uv);
	let u5 = unpack2x16float(p5);
	let p6 = pack4xI8(vec4i(-10i, 20i, -30i, 40i));
	let u6 = unpack4xI8(p6);
	let p7 = pack4xU8(vec4u(10u, 20u, 30u, 40u));
	let u7 = unpack4xU8(p7);
	let p8 = pack4xI8Clamp(vec4i(-200i, 200i, 0i, 1i));
	let p9 = pack4xU8Clamp(vec4u(300u, 10u, 20u, 30u));
	_ = p8 ^ p9;
	return (u1 + u2) * 0.25 + vec4f(u3 + u4 + u5, f32(u6.x + i32(u7.y)) * 0.001, 1.0);
}

fn test_modf_frexp(uv: vec2f) -> vec4f {
	let m1 = modf(uv.x * 3.5 - 1.25);
	let m2 = modf(uv * 2.5);
	let m3 = modf(vec3f(uv, 1.75));
	let m4 = modf(vec4f(uv, -1.25, 2.5));
	let f1 = frexp(uv.y * 4.0 + 0.25);
	let f2 = frexp(uv + vec2f(0.5));
	let f3 = frexp(vec3f(uv + 0.1, 2.0));
	let f4 = frexp(vec4f(uv + 0.1, -3.0, 0.0));
	return vec4f(
		m1.fract + m2.whole.x + m3.fract.z + m4.whole.w,
		f1.fract + f2.fract.y + f3.fract.z + f4.fract.w,
		f32(f1.exp + f2.exp.x + f3.exp.z + f4.exp.w) * 0.1,
		1.0,
	);
}

fn shade(uv: vec2f) -> vec4f {
	add_global(0.5);
	var hits = array<Hit, 2>(make_hit(uv), g_hit);
	hits[0].pos = g_zero;
	let c = shade_hit(hits[0]) + shade_hit(hits[1]);
	let m = test_matrices(uv);
	let b = test_bits(uv);
	let p = test_pack(uv);
	let mf = test_modf_frexp(uv);
	return clamp(vec4f(c, 1.0) * 0.25 + m * 0.25 + p * 0.25 + mf * 0.25 + vec4f(b * 0.01), vec4f(0.0), vec4f(1.0));
}
