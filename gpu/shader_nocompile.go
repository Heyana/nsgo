//go:build nsgo_noshader

package gpu

import "github.com/nsgo-dev/nsgo/internal/shaderir"

// ShaderModule is a placeholder in nsgo_noshader builds (such as mini programs),
// which host no GPU views.
type ShaderModule struct {
	label           string
	ir              *shaderir.Program
	vertexEntries   []string
	fragmentEntries []string
	source          string
}

func (m *ShaderModule) Label() string                { return "" }
func (m *ShaderModule) HasVertexEntry(string) bool   { return false }
func (m *ShaderModule) HasFragmentEntry(string) bool { return false }

func CompileShader(ShaderDesc) (*ShaderModule, error) {
	return nil, Error{Code: ErrUnsupported, Message: "shader compilation is disabled in this build"}
}

func CompileWGSL(string) (*ShaderModule, error) {
	return nil, Error{Code: ErrUnsupported, Message: "shader compilation is disabled in this build"}
}

func MustCompileWGSL(src string) *ShaderModule {
	m, err := CompileWGSL(src)
	if err != nil {
		panic(err)
	}
	return m
}

func CompileKage(string) (*ShaderModule, error) {
	return nil, Error{Code: ErrUnsupported, Message: "shader compilation is disabled in this build"}
}
