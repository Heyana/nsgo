//go:build !nsgo_noshader

package nsgotest

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

func TestTranslateShaderKage(t *testing.T) {
	prog := core.MustCompileShader(`package main

var Time float
var Tint vec4

func Fragment(dst vec4, pos vec2) vec4 {
	tex := image0At(pos)
	v := 0.5 + 0.5*sin(pos.x/20.0+Time)
	return tex * Tint * vec4(v, v, 1.0, 1.0)
}
`)
	src, err := TranslateShader(prog)
	if err != nil {
		t.Fatalf("TranslateShader(Kage) error: %v", err)
	}
	if !strings.Contains(src.MSL, "#include <metal_stdlib>") || !strings.Contains(src.MSL, "float4 nsgoShader(") {
		t.Errorf("unexpected MSL output:\n%s", src.MSL)
	}
	if !strings.Contains(src.HLSL, "cbuffer Uniforms : register(b0)") || !strings.Contains(src.HLSL, "float4 nsgoShader(") {
		t.Errorf("unexpected HLSL output:\n%s", src.HLSL)
	}
	if !strings.Contains(src.GLSL, "layout(std140) uniform NsgoUniforms") || !strings.Contains(src.GLSL, "vec4 nsgoShader(") {
		t.Errorf("unexpected GLSL output:\n%s", src.GLSL)
	}
	if !strings.Contains(src.GLES, "layout(std140) uniform NsgoUniforms") || !strings.Contains(src.GLES, "vec4 nsgoShader(") {
		t.Errorf("unexpected GLES output:\n%s", src.GLES)
	}
	if src.GLSL == src.GLES || !strings.HasPrefix(src.GLSL, "#version 330 core") || !strings.HasPrefix(src.GLES, "#version 300 es") {
		t.Errorf("GLSL and GLES outputs must carry their target headers:\nGLSL=%s\nGLES=%s", src.GLSL, src.GLES)
	}
}

func TestTranslateShaderWGSL(t *testing.T) {
	prog := core.MustCompileWGSL(`
struct Uniforms {
	strength: f32,
	tint: vec4f,
};

fn shade(uv: vec2f) -> vec4f {
	let c = sample_content(uv);
	let wave = 0.5 + 0.5 * sin(uv.x * 6.28318 + nsh.time);
	return mix(c, uniforms.tint * wave, uniforms.strength);
}
`)
	src, err := TranslateShader(prog)
	if err != nil {
		t.Fatalf("TranslateShader(WGSL) error: %v", err)
	}
	for name, s := range map[string]string{
		"MSL":  src.MSL,
		"HLSL": src.HLSL,
		"GLSL": src.GLSL,
		"GLES": src.GLES,
	} {
		if s == "" || !strings.Contains(s, "nsgoShader(") {
			t.Errorf("%s output missing nsgoShader:\n%s", name, s)
		}
	}
}

func TestTranslateShaderErrorsAndPanicRecovery(t *testing.T) {
	if _, err := TranslateShader(nil); err == nil {
		t.Fatal("TranslateShader(nil) = nil, want error")
	}

	// Struct uniform triggers a panic in hlsl.UniformVariableOffsetsInDwords while MSL and GLSL succeed.
	structUniformIR := &shaderir.Program{
		UniformNames: []string{"bad"},
		Uniforms: []shaderir.Type{{
			Main: shaderir.Struct,
			Sub:  []shaderir.Type{{Main: shaderir.Float}},
		}},
		FragmentFunc: shaderir.FragmentFunc{
			Block: &shaderir.Block{
				Stmts: []shaderir.Stmt{{
					Type: shaderir.Return,
					Exprs: []shaderir.Expr{{
						Type: shaderir.Call,
						Exprs: []shaderir.Expr{
							{Type: shaderir.BuiltinFuncExpr, BuiltinFunc: shaderir.Vec4F},
							{Type: shaderir.NumberExpr},
						},
					}},
				}},
			},
		},
	}
	src, err := translateFragmentIR(structUniformIR)
	if err == nil {
		t.Fatal("translateFragmentIR(struct uniform) = nil, want HLSL panic error")
	}
	if !strings.Contains(err.Error(), "HLSL") {
		t.Fatalf("expected error naming HLSL, got: %v", err)
	}
	if src.HLSL != "" {
		t.Errorf("expected empty HLSL after panic, got %q", src.HLSL)
	}

	// Unsupported uniform type panics in all fragment translators.
	badTypeIR := &shaderir.Program{
		UniformNames: []string{"bad"},
		Uniforms:     []shaderir.Type{{Main: shaderir.None}},
		FragmentFunc: shaderir.FragmentFunc{Block: &shaderir.Block{}},
	}
	src, err = translateFragmentIR(badTypeIR)
	if err == nil {
		t.Fatal("translateFragmentIR(bad type) = nil, want error")
	}
	for _, backend := range []string{"MSL", "HLSL", "GLSL", "GLES"} {
		if !strings.Contains(err.Error(), backend) {
			t.Errorf("error %q missing backend %s", err.Error(), backend)
		}
	}
	if src != (ShaderSources{}) {
		t.Errorf("expected all sources empty after panics, got %+v", src)
	}

	// Missing fragment entry returns errors naming every backend.
	if _, err := translateFragmentIR(&shaderir.Program{}); err == nil {
		t.Fatal("translateFragmentIR(empty) = nil, want error")
	} else {
		for _, backend := range []string{"MSL", "HLSL", "GLSL", "GLES"} {
			if !strings.Contains(err.Error(), backend) {
				t.Errorf("empty program error %q missing backend %s", err.Error(), backend)
			}
		}
	}
}
