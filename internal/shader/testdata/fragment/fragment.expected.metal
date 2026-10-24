#include <metal_stdlib>

using namespace metal;

constexpr sampler __texelSampler(coord::pixel, filter::nearest, address::clamp_to_zero);

template<typename T, typename U>
T mod(T x, U y) {
	return x - y * floor(x/y);
}

// Metal defines sign only for floating-point types. These overloads cover int and ivecN arguments.
int sign(int x) {
	return int(x > 0) - int(x < 0);
}

int2 sign(int2 x) {
	return int2(x > 0) - int2(x < 0);
}

int3 sign(int3 x) {
	return int3(x > 0) - int3(x < 0);
}

int4 sign(int4 x) {
	return int4(x > 0) - int4(x < 0);
}

struct Uniforms {
	float2 U0;
	float U1;
	uint U2;
	int U3;
	array<float4, 4> U4;
};

uint F0(constant Uniforms& uniforms, uint l0);
float2 F1(constant Uniforms& uniforms);
float F2(constant Uniforms& uniforms);
float4 F3(constant Uniforms& uniforms, float4 l0, float2 l1);

uint F0(constant Uniforms& uniforms, uint l0) {
	uint l1 = 0u;
	uint l2 = 0u;
	l1 = ((l0) * (747796405u)) + (2891336453u);
	l2 = (((l1) >> (((l1) >> (28)) + (4u))) ^ (l1)) * (277803737u);
	return ((l2) >> (22)) ^ (l2);
}

float2 F1(constant Uniforms& uniforms) {
	return uniforms.U0;
}

float F2(constant Uniforms& uniforms) {
	return uniforms.U1;
}

float4 F3(constant Uniforms& uniforms, float4 l0, float2 l1) {
	float l2 = float(0);
	uint l5 = 0u;
	l2 = 0.0;
	for (int l3 = 0; l3 < uniforms.U3; l3++) {
		l2 = (l2) + ((((uniforms.U4)[l3]).z) / ((1.0) + (distance(l1, ((uniforms.U4)[l3]).xy))));
	}
	for (uint l4 = 0u; l4 < uniforms.U2; l4 += 2u) {
		l2 = (l2) + (1.0000000000e-01);
	}
	l5 = F0(uniforms, (((uint3(static_cast<uint>(static_cast<int>((l0).x)))).z) * (2654435769u)) ^ (F0(uniforms, (static_cast<uint>(static_cast<int>((l0).y))) ^ (uniforms.U2))));
	return (((float4(((l1) * (F2(uniforms))) / (F1(uniforms)), 0.0, 1.0)) * (static_cast<float>((l5) % (255u)))) / (255.0)) * (l2);
}

float4 shaderFragment(constant Uniforms& uniforms, float4 position, float2 v0) {
	return F3(uniforms, position, v0);
}
