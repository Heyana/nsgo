//go:build nsgo_noshader

package nsgotest

import (
	"errors"

	"github.com/nsgo-dev/nsgo/core"
)

// TranslateShader returns an error when shader compilation is disabled.
func TranslateShader(*core.ShaderProgram) (ShaderSources, error) {
	return ShaderSources{}, errors.New("nsgotest: shader compilation is disabled in this build")
}
