package gpu

import (
	"errors"
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

// Err returns an error combining all non-nil per-backend translation errors,
// or nil if every backend succeeded.
func (s PipelineSources) Err() error {
	return errors.Join(s.MSLError, s.HLSLError, s.GLSLError, s.GLESError, s.WGSLError)
}
