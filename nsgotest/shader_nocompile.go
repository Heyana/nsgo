//go:build nsgo_noshader

package nsgotest

import (
	"errors"
	"fmt"

	"github.com/nsgo-dev/nsgo/core"
)

// TranslateShader returns an error wrapping errors.ErrUnsupported when shader compilation is disabled.
func TranslateShader(*core.ShaderProgram) (ShaderSources, error) {
	return ShaderSources{}, fmt.Errorf("nsgotest: shader compilation is disabled in this build: %w", errors.ErrUnsupported)
}
