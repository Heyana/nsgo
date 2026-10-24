// mod and modInt are macros rather than functions. Some HLSL compilers cannot
// resolve overloads on the vector size, as a scalar argument is implicitly convertible
// to any vector type (#3356).

// mod is a floored modulo like GLSL's mod. HLSL's fmod is a truncated modulo.
#define mod(x, y) ((x) - (y) * floor((x) / (y)))

// modInt is a truncated modulo like the '%' operator. fxc emits warning X3556 for signed
// integer % and /, and suggests uints, so the magnitude is computed with unsigned modulo
// and the sign of the dividend is reapplied. This matches '%' except for the minimum
// integer, where abs overflows.
#define modInt(x, y) (sign(x) * asint(asuint(abs(x)) % asuint(abs(y))))

float2x2 float2x2FromScalar(float x) {
	return float2x2(x, 0, 0, x);
}

float3x3 float3x3FromScalar(float x) {
	return float3x3(x, 0, 0, 0, x, 0, 0, 0, x);
}

float4x4 float4x4FromScalar(float x) {
	return float4x4(x, 0, 0, 0, 0, x, 0, 0, 0, 0, x, 0, 0, 0, 0, x);
}

cbuffer Uniforms : register(b2) {
	float2 U0 : packoffset(c0);
	float U1 : packoffset(c0.z);
	uint U2 : packoffset(c0.w);
	int U3 : packoffset(c1);
	float4 U4[4] : packoffset(c2);
}

uint F0(in uint l0);
float2 F1(void);
float F2(void);
float4 F3(in float4 l0, in float2 l1);

uint F0(in uint l0) {
	uint l1 = 0u;
	uint l2 = 0u;
	l1 = ((l0) * (747796405u)) + (2891336453u);
	l2 = (((l1) >> (((l1) >> (28)) + (4u))) ^ (l1)) * (277803737u);
	return ((l2) >> (22)) ^ (l2);
}

float2 F1(void) {
	return U0;
}

float F2(void) {
	return U1;
}

float4 F3(in float4 l0, in float2 l1) {
	float l2 = 0.0;
	uint l5 = 0u;
	l2 = 0.0;
	for (int l3 = 0; l3 < U3; l3++) {
		l2 = (l2) + ((((U4)[l3]).z) / ((1.0) + (distance(l1, ((U4)[l3]).xy))));
	}
	for (uint l4 = 0u; l4 < U2; l4 += 2u) {
		l2 = (l2) + (1.0000000000e-01);
	}
	l5 = F0(((((uint3)(uint(int((l0).x)))).z) * (2654435769u)) ^ (F0((uint(int((l0).y))) ^ (U2))));
	return (((float4(((l1) * (F2())) / (F1()), 0.0, 1.0)) * (float((l5) % (255u)))) / (255.0)) * (l2);
}

float4 shaderFragment(float4 position, float2 v0) {
	return F3(position, v0);
}
