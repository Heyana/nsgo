//go:build nsgo_noshader

package gpu

import "github.com/nsgo-dev/nsgo/internal/driver"

// TranslateShaderModule returns ErrUnsupported when shader compilation is disabled.
func TranslateShaderModule(*ShaderModule) (PipelineSources, error) {
	return PipelineSources{}, Error{Code: ErrUnsupported, Message: "shader compilation is disabled in this build"}
}

func compilePipelineArtifacts(driver.GPUPipelineDesc) (*driver.GPUPipelineArtifacts, error) {
	return nil, Error{Code: ErrUnsupported, Message: "shader compilation is disabled in this build"}
}
