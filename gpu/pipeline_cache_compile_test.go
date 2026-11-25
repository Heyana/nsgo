//go:build !nsgo_noshader

package gpu

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

func TestTranslateShaderModuleAndArtifacts(t *testing.T) {
	if _, err := TranslateShaderModule(nil); err == nil {
		t.Fatal("TranslateShaderModule(nil) = nil, want error")
	}

	renderMod := MustCompileWGSL(`
struct Uniforms {
	mvp: mat4x4<f32>,
};
@group(0) @binding(0) var<uniform> u: Uniforms;

struct VSOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
};

@vertex
fn vs_main(@location(0) pos: vec3<f32>, @location(1) color: vec4<f32>) -> VSOut {
	var out: VSOut;
	out.pos = u.mvp * vec4<f32>(pos, 1.0);
	out.color = color;
	return out;
}

@fragment
fn fs_main(@location(0) color: vec4<f32>) -> @location(0) vec4<f32> {
	return color;
}
`)
	ps, err := TranslateShaderModule(renderMod)
	if err != nil {
		t.Fatalf("TranslateShaderModule(render) error: %v", err)
	}
	if ps.MSL == "" || ps.HLSLVertex == "" || ps.HLSLPixel == "" ||
		ps.GLSLVertex == "" || ps.GLSLFragment == "" ||
		ps.GLESVertex == "" || ps.GLESFragment == "" || ps.WGSL == "" {
		t.Fatalf("missing expected translated sources: %+v", ps)
	}
	if ps.MSLError != nil || ps.HLSLError != nil || ps.GLSLError != nil || ps.GLESError != nil || ps.WGSLError != nil {
		t.Fatalf("unexpected backend errors: %+v", ps)
	}

	// Storage buffer render shader succeeds on MSL, HLSL, WGSL and reports GLSL/GLES errors.
	storageMod := MustCompileWGSL(`
@group(0) @binding(0) var<storage, read> colors: array<vec4<f32>>;

struct VSOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) color: vec4<f32>,
};

@vertex
fn vs_main(@builtin(vertex_index) vid: u32) -> VSOut {
	var out: VSOut;
	out.pos = vec4<f32>(0.0, 0.0, 0.0, 1.0);
	out.color = colors[vid];
	return out;
}

@fragment
fn fs_main(@location(0) color: vec4<f32>) -> @location(0) vec4<f32> {
	return color;
}
`)
	sPS, err := TranslateShaderModule(storageMod)
	if err == nil {
		t.Fatal("TranslateShaderModule(storage) = nil, want GLSL/GLES errors")
	}
	if sPS.MSL == "" || sPS.HLSLVertex == "" || sPS.HLSLPixel == "" || sPS.WGSL == "" {
		t.Errorf("expected MSL, HLSL, WGSL sources, got %+v", sPS)
	}
	if sPS.MSLError != nil || sPS.HLSLError != nil || sPS.WGSLError != nil {
		t.Errorf("unexpected errors on MSL/HLSL/WGSL: msl=%v hlsl=%v wgsl=%v", sPS.MSLError, sPS.HLSLError, sPS.WGSLError)
	}
	if sPS.GLSLError == nil || !strings.Contains(sPS.GLSLError.Error(), "GLSL") {
		t.Errorf("expected GLSLError naming GLSL, got %v", sPS.GLSLError)
	}
	if sPS.GLESError == nil || !strings.Contains(sPS.GLESError.Error(), "GLES") {
		t.Errorf("expected GLESError naming GLES, got %v", sPS.GLESError)
	}

	// Kage fragment module translates cleanly when self-contained.
	kageMod, err := CompileKage(`package main

func Fragment(dst vec4, pos vec2) vec4 {
	return vec4(1.0, 0.0, 0.0, 1.0)
}
`)
	if err != nil {
		t.Fatalf("CompileKage: %v", err)
	}
	kPS, err := TranslateShaderModule(kageMod)
	if err != nil {
		t.Fatalf("TranslateShaderModule(kage) error: %v", err)
	}
	if kPS.MSL == "" || kPS.HLSLPixel == "" || kPS.GLSLFragment == "" || kPS.GLESFragment == "" || kPS.WGSL == "" {
		t.Errorf("expected all fragment backends for Kage module, got %+v", kPS)
	}

	// compilePipelineArtifacts still succeeds when at least one backend compiles,
	// and returns an error instead of empty artifacts when every backend fails.
	art, err := compilePipelineArtifacts(driver.GPUPipelineDesc{
		Vertex:   driver.GPUShaderStage{IR: storageMod.ir},
		Fragment: driver.GPUShaderStage{IR: storageMod.ir},
	})
	if err != nil {
		t.Fatalf("compilePipelineArtifacts(storage) unexpected error: %v", err)
	}
	if art.MSLSource == "" || art.HLSLVertexSource == "" || art.WGSLSource == "" {
		t.Errorf("expected partial artifacts for storage shader, got %+v", art)
	}

	if _, err := compilePipelineArtifacts(driver.GPUPipelineDesc{
		Vertex:   driver.GPUShaderStage{IR: &shaderir.Program{}},
		Fragment: driver.GPUShaderStage{IR: &shaderir.Program{}},
	}); err == nil {
		t.Fatal("compilePipelineArtifacts(empty IR) = nil, want error")
	}

	// Panic recovery in pipeline translation turns translator panics into errors naming every failing backend.
	panicIR := &shaderir.Program{
		Bindings: []shaderir.Binding{{
			Kind: shaderir.BindingUniformBuffer,
			Type: shaderir.Type{Main: shaderir.Array},
		}},
		VertexFunc: shaderir.VertexFunc{Block: &shaderir.Block{}},
	}
	_, pPS := translatePipelineIR(panicIR)
	for name, bErr := range map[string]error{
		"MSL":  pPS.MSLError,
		"HLSL": pPS.HLSLError,
		"GLSL": pPS.GLSLError,
		"GLES": pPS.GLESError,
		"WGSL": pPS.WGSLError,
	} {
		if bErr == nil || !strings.Contains(bErr.Error(), name) || !strings.Contains(bErr.Error(), "panic:") {
			t.Errorf("%s panic error = %v, want recovered panic naming %s", name, bErr, name)
		}
	}
}
