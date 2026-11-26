//go:build !nsgo_noshader

package nsgotest

import (
	"errors"
	"fmt"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/hlsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/msl"
)

// TranslateShader translates p into the GPU shader source each fragment backend
// compiles (MSL, HLSL, GLSL 3.30, and GLSL ES 3.00) using the same options as
// the platform backends, recovering any translator panic into an error naming
// the backend.
func TranslateShader(p *core.ShaderProgram) (ShaderSources, error) {
	dp := core.DriverShaderProgram(p)
	if dp == nil || dp.IR == nil {
		return ShaderSources{}, errors.New("nsgotest: shader program has no IR")
	}
	return translateFragmentIR(dp.IR)
}

func translateFragmentIR(ir *shaderir.Program) (ShaderSources, error) {
	if ir == nil {
		return ShaderSources{}, errors.New("nsgotest: shader program has no IR")
	}
	var src ShaderSources
	var errs []error

	mslSrc, err := runFragmentBackend("MSL", func() string {
		_, _ = msl.UniformLanes(ir)
		return msl.Fragment(ir, "nsgoShader")
	})
	src.MSL = mslSrc
	if err != nil {
		errs = append(errs, err)
	}

	hlslSrc, err := runFragmentBackend("HLSL", func() string {
		_, _ = hlsl.UniformLanes(ir)
		return hlsl.Fragment(ir, "nsgoShader", 0, 0)
	})
	src.HLSL = hlslSrc
	if err != nil {
		errs = append(errs, err)
	}

	glslOpts := glsl.Options{FlipY: "true", Block: "NsgoUniforms"}
	glslSrc, err := runFragmentBackend("GLSL", func() string {
		_, _ = glsl.UniformLanes(ir)
		body := glsl.Fragment(ir, "nsgoShader", glslOpts)
		if body == "" {
			return ""
		}
		return "#version 330 core\n\n" + body
	})
	src.GLSL = glslSrc
	if err != nil {
		errs = append(errs, err)
	}

	glesSrc, err := runFragmentBackend("GLES", func() string {
		_, _ = glsl.UniformLanes(ir)
		body := glsl.Fragment(ir, "nsgoShader", glslOpts)
		if body == "" {
			return ""
		}
		return "#version 300 es\nprecision highp float;\nprecision highp int;\n\n" + body
	})
	src.GLES = glesSrc
	if err != nil {
		errs = append(errs, err)
	}

	return src, errors.Join(errs...)
}

func runFragmentBackend(backend string, fn func() string) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			out = ""
			err = fmt.Errorf("nsgotest: %s: panic: %v", backend, r)
		}
	}()
	out = fn()
	if out == "" {
		return "", fmt.Errorf("nsgotest: %s: empty shader output", backend)
	}
	return out, nil
}
