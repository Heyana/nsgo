//go:build !nsgo_noshader

package gpu

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/hlsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/msl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/wgsl"
)

// TranslateShaderModule translates m into GPU shader sources for MSL, HLSL,
// GLSL 3.30, GLSL ES 3.00, and WGSL without opening a GPU device. Any translator
// error or recovered panic is recorded on the returned PipelineSources and
// included in the returned error with the backend name.
func TranslateShaderModule(m *ShaderModule) (PipelineSources, error) {
	if m == nil || m.ir == nil {
		return PipelineSources{}, Error{Code: ErrShaderCompile, Message: "missing shader IR for pipeline translation"}
	}
	_, ps := translatePipelineIR(m.ir)
	return ps, ps.Err()
}

func mergePipelineIR(vs, fs *shaderir.Program) *shaderir.Program {
	if vs == fs && vs != nil {
		return vs
	}
	if vs == nil {
		return fs
	}
	if fs == nil {
		return vs
	}
	out := *vs
	out.FragmentFunc = fs.FragmentFunc
	if len(fs.Varyings) > len(out.Varyings) {
		out.Varyings = fs.Varyings
		out.VaryingMeta = fs.VaryingMeta
	}
	return &out
}

func compilePipelineArtifacts(dd driver.GPUPipelineDesc) (*driver.GPUPipelineArtifacts, error) {
	prog := mergePipelineIR(dd.Vertex.IR, dd.Fragment.IR)
	if prog == nil {
		return nil, Error{Code: ErrShaderCompile, Message: "missing shader IR for pipeline compilation"}
	}
	art, ps := translatePipelineIR(prog)
	if !hasPipelineSource(ps) {
		if err := ps.Err(); err != nil {
			return nil, Error{Code: ErrShaderCompile, Message: err.Error(), Cause: err}
		}
		return nil, Error{Code: ErrShaderCompile, Message: "shader pipeline produced no backend output"}
	}
	return art, nil
}

func hasPipelineSource(ps PipelineSources) bool {
	return ps.MSL != "" || ps.HLSLVertex != "" || ps.HLSLPixel != "" || ps.HLSLCompute != "" ||
		ps.GLSLVertex != "" || ps.GLSLFragment != "" || ps.GLSLCompute != "" ||
		ps.GLESVertex != "" || ps.GLESFragment != "" || ps.GLESCompute != "" || ps.WGSL != ""
}

func translatePipelineIR(prog *shaderir.Program) (*driver.GPUPipelineArtifacts, PipelineSources) {
	art := &driver.GPUPipelineArtifacts{}
	var ps PipelineSources
	if prog == nil || (prog.VertexFunc.Block == nil && prog.FragmentFunc.Block == nil && prog.ComputeFunc.Block == nil) {
		for _, entry := range []struct {
			name string
			dst  *error
		}{
			{"MSL", &ps.MSLError},
			{"HLSL", &ps.HLSLError},
			{"GLSL", &ps.GLSLError},
			{"GLES", &ps.GLESError},
			{"WGSL", &ps.WGSLError},
		} {
			*entry.dst = fmt.Errorf("gpu: %s: missing shader entry point", entry.name)
		}
		return art, ps
	}

	ps.MSLError = runPipelineBackend("MSL", func() error {
		mslSrc, err := msl.CompilePipeline(prog)
		if err != nil {
			return err
		}
		if mslSrc.Source == "" {
			return fmt.Errorf("empty shader output")
		}
		ps.MSL = mslSrc.Source
		art.MSLSource = mslSrc.Source
		art.MSLVertexEntry = mslSrc.VertexEntry
		art.MSLFragmentEntry = mslSrc.FragmentEntry
		if len(mslSrc.Slots) > 0 {
			art.MSLSlots = make([]driver.GPUPipelineBindingSlot, len(mslSrc.Slots))
			for i, s := range mslSrc.Slots {
				art.MSLSlots[i] = driver.GPUPipelineBindingSlot{
					Group:   s.Group,
					Binding: s.Binding,
					Kind:    uint8(s.Kind),
					Slot:    s.Slot,
				}
			}
		}
		return nil
	})

	ps.HLSLError = runPipelineBackend("HLSL", func() error {
		hlslSrc, err := hlsl.CompilePipeline(prog)
		if err != nil {
			return err
		}
		if hlslSrc.VertexSource == "" && hlslSrc.PixelSource == "" && hlslSrc.ComputeSource == "" {
			return fmt.Errorf("empty shader output")
		}
		ps.HLSLVertex = hlslSrc.VertexSource
		ps.HLSLPixel = hlslSrc.PixelSource
		ps.HLSLCompute = hlslSrc.ComputeSource
		art.HLSLVertexSource = hlslSrc.VertexSource
		art.HLSLPixelSource = hlslSrc.PixelSource
		art.HLSLVertexEntry = hlslSrc.VertexEntry
		art.HLSLPixelEntry = hlslSrc.PixelEntry
		if len(hlslSrc.Registers) > 0 {
			art.HLSLRegisters = make([]driver.GPUPipelineBindingSlot, len(hlslSrc.Registers))
			for i, r := range hlslSrc.Registers {
				art.HLSLRegisters[i] = driver.GPUPipelineBindingSlot{
					Group:   r.Group,
					Binding: r.Binding,
					Kind:    uint8(r.Kind),
					Slot:    r.Register,
				}
			}
		}
		return nil
	})

	ps.GLSLError = runPipelineBackend("GLSL", func() error {
		glSrc, err := glsl.CompilePipeline(prog, glsl.GLSLVersionDefault)
		if err != nil {
			return err
		}
		if glSrc.VertexSource == "" && glSrc.FragmentSource == "" && glSrc.ComputeSource == "" {
			return fmt.Errorf("empty shader output")
		}
		ps.GLSLVertex = glSrc.VertexSource
		ps.GLSLFragment = glSrc.FragmentSource
		ps.GLSLCompute = glSrc.ComputeSource
		art.GLSLVertexSource = glSrc.VertexSource
		art.GLSLFragmentSource = glSrc.FragmentSource
		art.GLSLUniformBlocks = convertGLSLUniformBlocks(glSrc.UniformBlocks)
		art.GLSLSamplerPairs = convertGLSLSamplerPairs(glSrc.SamplerPairs)
		return nil
	})

	ps.GLESError = runPipelineBackend("GLES", func() error {
		glesSrc, err := glsl.CompilePipeline(prog, glsl.GLSLVersionES300)
		if err != nil {
			return err
		}
		if glesSrc.VertexSource == "" && glesSrc.FragmentSource == "" && glesSrc.ComputeSource == "" {
			return fmt.Errorf("empty shader output")
		}
		ps.GLESVertex = glesSrc.VertexSource
		ps.GLESFragment = glesSrc.FragmentSource
		ps.GLESCompute = glesSrc.ComputeSource
		art.GLESVertexSource = glesSrc.VertexSource
		art.GLESFragmentSource = glesSrc.FragmentSource
		art.GLESUniformBlocks = convertGLSLUniformBlocks(glesSrc.UniformBlocks)
		art.GLESSamplerPairs = convertGLSLSamplerPairs(glesSrc.SamplerPairs)
		return nil
	})

	ps.WGSLError = runPipelineBackend("WGSL", func() error {
		wgslSrc, err := wgsl.CompilePipeline(prog)
		if err != nil {
			return err
		}
		if wgslSrc == "" {
			return fmt.Errorf("empty shader output")
		}
		ps.WGSL = wgslSrc
		art.WGSLSource = wgslSrc
		return nil
	})

	return art, ps
}

func runPipelineBackend(backend string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("gpu: %s: panic: %v", backend, r)
		}
	}()
	if err = fn(); err != nil {
		return fmt.Errorf("gpu: %s: %w", backend, err)
	}
	return nil
}

func convertGLSLUniformBlocks(in []glsl.UniformBlockSlot) []driver.GPUPipelineUniformBlock {
	if len(in) == 0 {
		return nil
	}
	out := make([]driver.GPUPipelineUniformBlock, len(in))
	for i, u := range in {
		out[i] = driver.GPUPipelineUniformBlock{
			Group:     u.Group,
			Binding:   u.Binding,
			BlockName: u.BlockName,
			Slot:      u.Slot,
		}
	}
	return out
}

func convertGLSLSamplerPairs(in []glsl.SamplerPair) []driver.GPUPipelineSamplerPair {
	if len(in) == 0 {
		return nil
	}
	out := make([]driver.GPUPipelineSamplerPair, len(in))
	for i, sp := range in {
		out[i] = driver.GPUPipelineSamplerPair{
			Name:           sp.Name,
			Unit:           sp.Unit,
			TextureGroup:   sp.TextureGroup,
			TextureBinding: sp.TextureBinding,
			SamplerGroup:   sp.SamplerGroup,
			SamplerBinding: sp.SamplerBinding,
		}
	}
	return out
}
