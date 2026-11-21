//go:build !nsgo_noshader

package gpu

import (
	"slices"

	"github.com/nsgo-dev/nsgo/internal/shader"
	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// ShaderModule is a portable compiled WGSL or Kage shader module ready to be
// attached to one or more Pipelines.
type ShaderModule struct {
	label           string
	ir              *shaderir.Program
	vertexEntries   []string
	fragmentEntries []string
	source          string
}

// Label returns the shader module's debug label (always empty in release builds).
func (m *ShaderModule) Label() string {
	if m == nil {
		return ""
	}
	return m.label
}

// HasVertexEntry reports whether m defines a @vertex entry point named name
// (or any vertex entry point when name is empty).
func (m *ShaderModule) HasVertexEntry(name string) bool {
	if m == nil || m.ir == nil || m.ir.VertexFunc.Block == nil {
		return false
	}
	if name == "" {
		return true
	}
	return slices.Contains(m.vertexEntries, name) || m.ir.VertexEntry == name
}

// HasFragmentEntry reports whether m defines a @fragment entry point named name
// (or any fragment entry point when name is empty).
func (m *ShaderModule) HasFragmentEntry(name string) bool {
	if m == nil || m.ir == nil || m.ir.FragmentFunc.Block == nil {
		return false
	}
	if name == "" {
		return true
	}
	return slices.Contains(m.fragmentEntries, name) || m.ir.FragmentEntry == name
}

// CompileShader parses, type-checks, and lowers desc.Source (WGSL) into a
// ShaderModule, recording desc.Label in nsgodev/nsgotest builds only.
func CompileShader(desc ShaderDesc) (*ShaderModule, error) {
	m, err := CompileWGSL(desc.Source)
	if err != nil {
		return nil, err
	}
	m.label = debugLabel(desc.Label)
	return m, nil
}

// CompileWGSL parses, type-checks, and lowers a WGSL 3D pipeline module into
// portable shaderir.
func CompileWGSL(src string) (*ShaderModule, error) {
	p, err := wgsl.CompilePipeline(src)
	if err != nil {
		return nil, Error{Code: ErrShaderCompile, Message: err.Error(), Cause: err}
	}
	return &ShaderModule{
		ir:              p.IR,
		vertexEntries:   p.VertexEntries,
		fragmentEntries: p.FragmentEntries,
		source:          src,
	}, nil
}

// MustCompileWGSL compiles src and panics if compilation fails.
func MustCompileWGSL(src string) *ShaderModule {
	m, err := CompileWGSL(src)
	if err != nil {
		panic(err)
	}
	return m
}

// CompileKage compiles a Kage fragment shader into a ShaderModule.
func CompileKage(src string) (*ShaderModule, error) {
	ir, err := shader.CompileFragment([]byte(src))
	if err != nil {
		return nil, Error{Code: ErrShaderCompile, Message: err.Error(), Cause: err}
	}
	return &ShaderModule{
		ir:              ir,
		fragmentEntries: []string{"Fragment"},
		source:          src,
	}, nil
}
