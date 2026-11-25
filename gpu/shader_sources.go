package gpu

import (
	"errors"
	"strings"
)

// PipelineSources holds translated GPU shader sources and per-backend errors
// produced for a ShaderModule or render pipeline across MSL, HLSL, GLSL 3.30,
// GLSL ES 3.00, and WGSL.
type PipelineSources struct {
	MSL          string
	HLSLVertex   string
	HLSLPixel    string
	HLSLCompute  string
	GLSLVertex   string
	GLSLFragment string
	GLSLCompute  string
	GLESVertex   string
	GLESFragment string
	GLESCompute  string
	WGSL         string

	MSLError  error
	HLSLError error
	GLSLError error
	GLESError error
	WGSLError error
}

// HLSL returns the combined non-empty HLSL vertex, pixel, and compute sources.
func (s PipelineSources) HLSL() string {
	return joinNonEmpty(s.HLSLVertex, s.HLSLPixel, s.HLSLCompute)
}

// GLSL returns the combined non-empty GLSL 3.30 vertex, fragment, and compute sources.
func (s PipelineSources) GLSL() string {
	return joinNonEmpty(s.GLSLVertex, s.GLSLFragment, s.GLSLCompute)
}

// GLES returns the combined non-empty GLSL ES 3.00 vertex, fragment, and compute sources.
func (s PipelineSources) GLES() string {
	return joinNonEmpty(s.GLESVertex, s.GLESFragment, s.GLESCompute)
}

// Err returns an error combining all non-nil per-backend translation errors,
// or nil if every backend succeeded.
func (s PipelineSources) Err() error {
	return errors.Join(s.MSLError, s.HLSLError, s.GLSLError, s.GLESError, s.WGSLError)
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n")
}
