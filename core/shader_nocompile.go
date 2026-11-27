//go:build nsgo_noshader

package core

// Builds tagged nsgo_noshader leave the shader compilers out, for targets
// that draw no shader and count their size, as mini programs do: compiling
// a shader fails with an Unavailable error matching errors.ErrUnsupported.

// builtinLanes is wgsl.BuiltinLanes, which shader_lanes_test.go checks.
const builtinLanes = 3

const crossfadeSource = ""

var errShadersUnsupported = Unavailable("core: shaders are unavailable in this build")

func compileFragment(string) (*ShaderProgram, error) { return nil, errShadersUnsupported }

func compileWGSL(string) (*ShaderProgram, error) { return nil, errShadersUnsupported }
