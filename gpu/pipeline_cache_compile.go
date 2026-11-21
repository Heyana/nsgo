//go:build !nsgo_noshader

package gpu

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/hlsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/msl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/wgsl"
)

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
	art := &driver.GPUPipelineArtifacts{}

	if mslSrc, err := msl.CompilePipeline(prog); err == nil {
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
	}

	if hlslSrc, err := hlsl.CompilePipeline(prog); err == nil {
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
	}

	if glSrc, err := glsl.CompilePipeline(prog, glsl.GLSLVersionDefault); err == nil {
		art.GLSLVertexSource = glSrc.VertexSource
		art.GLSLFragmentSource = glSrc.FragmentSource
		art.GLSLUniformBlocks = convertGLSLUniformBlocks(glSrc.UniformBlocks)
		art.GLSLSamplerPairs = convertGLSLSamplerPairs(glSrc.SamplerPairs)
	}

	if glesSrc, err := glsl.CompilePipeline(prog, glsl.GLSLVersionES300); err == nil {
		art.GLESVertexSource = glesSrc.VertexSource
		art.GLESFragmentSource = glesSrc.FragmentSource
		art.GLESUniformBlocks = convertGLSLUniformBlocks(glesSrc.UniformBlocks)
		art.GLESSamplerPairs = convertGLSLSamplerPairs(glesSrc.SamplerPairs)
	}

	if wgslSrc, err := wgsl.CompilePipeline(prog); err == nil {
		art.WGSLSource = wgslSrc
	}

	return art, nil
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
