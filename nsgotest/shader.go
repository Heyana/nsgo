package nsgotest

// ShaderSources holds translated fragment-shader sources across backends.
type ShaderSources struct {
	MSL  string
	HLSL string
	GLSL string
	GLES string
}
