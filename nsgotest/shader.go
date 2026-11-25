package nsgotest

import "github.com/nsgo-dev/nsgo/gpu"

// ShaderSources holds translated GPU shader sources across backends.
type ShaderSources struct {
	MSL  string
	HLSL string
	GLSL string
	GLES string
	WGSL string
}

// TranslateShaderModule translates m into GPU shader sources for MSL, HLSL,
// GLSL 3.30, GLSL ES 3.00, and WGSL without opening a GPU device. Any translator
// error or recovered panic is returned in an error naming each failing backend.
func TranslateShaderModule(m *gpu.ShaderModule) (ShaderSources, error) {
	ps, err := gpu.TranslateShaderModule(m)
	return ShaderSources{
		MSL:  ps.MSL,
		HLSL: ps.HLSL(),
		GLSL: ps.GLSL(),
		GLES: ps.GLES(),
		WGSL: ps.WGSL,
	}, err
}
