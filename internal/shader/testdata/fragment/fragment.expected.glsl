uniform vec2 U0;
uniform float U1;
uniform uint U2;
uniform int U3;
uniform vec4 U4[4];
uniform ivec4 flipRowsOf;

ivec2 flipRows(ivec2 p, int h) {
	return h > 0 ? ivec2(p.x, h - 1 - p.y) : p;
}

uint F0(in uint l0);
vec2 F1(void);
float F2(void);
vec4 F3(in vec4 l0, in vec2 l1);

uint F0(in uint l0) {
	uint l1 = 0u;
	uint l2 = 0u;
	l1 = ((l0) * (747796405u)) + (2891336453u);
	l2 = (((l1) >> (((l1) >> (28)) + (4u))) ^ (l1)) * (277803737u);
	return ((l2) >> (22)) ^ (l2);
}

vec2 F1(void) {
	return U0;
}

float F2(void) {
	return U1;
}

vec4 F3(in vec4 l0, in vec2 l1) {
	float l2 = float(0);
	uint l5 = 0u;
	l2 = 0.0;
	for (int l3 = 0; l3 < U3; l3++) {
		l2 = (l2) + ((((U4)[l3]).z) / ((1.0) + (distance(l1, ((U4)[l3]).xy))));
	}
	for (uint l4 = 0u; l4 < U2; l4 += 2u) {
		l2 = (l2) + (1.0000000000e-01);
	}
	l5 = F0((((uvec3(uint(int((l0).x)))).z) * (2654435769u)) ^ (F0((uint(int((l0).y))) ^ (U2))));
	return (((vec4(((l1) * (F2())) / (F1()), 0.0, 1.0)) * (float((l5) % (255u)))) / (255.0)) * (l2);
}

vec4 shaderFragment(vec4 position, vec2 v0) {
	return F3(position, v0);
}
