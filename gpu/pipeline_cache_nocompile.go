//go:build nsgo_noshader

package gpu

import "github.com/nsgo-dev/nsgo/internal/driver"

func compilePipelineArtifacts(driver.GPUPipelineDesc) (*driver.GPUPipelineArtifacts, error) {
	return nil, Error{Code: ErrUnsupported, Message: "shader compilation is disabled in this build"}
}
