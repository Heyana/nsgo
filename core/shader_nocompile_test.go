//go:build nsgo_noshader

package core

import (
	"errors"
	"testing"
)

func TestShadersReportErrUnsupported(t *testing.T) {
	if p, err := CompileShader(testShader); p != nil || ErrorCode(err) != CodeUnavailable || !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("CompileShader = (%v, %v) code=%q, want nil and CodeUnavailable matching errors.ErrUnsupported", p, err, ErrorCode(err))
	}
	if p, err := CompileWGSL("fn shade(uv: vec2f) -> vec4f { return vec4f(1.0); }"); p != nil || ErrorCode(err) != CodeUnavailable || !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("CompileWGSL = (%v, %v) code=%q, want nil and CodeUnavailable matching errors.ErrUnsupported", p, err, ErrorCode(err))
	}
	for _, tc := range []struct {
		name string
		fn   func()
	}{
		{"MustCompileShader", func() { MustCompileShader(testShader) }},
		{"MustCompileWGSL", func() { MustCompileWGSL("fn shade(uv: vec2f) -> vec4f { return vec4f(1.0); }") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				tc.fn()
			}()
			err, ok := recovered.(error)
			if !ok || !errors.Is(err, errors.ErrUnsupported) {
				t.Fatalf("%s panic = %#v, want error matching errors.ErrUnsupported", tc.name, recovered)
			}
		})
	}

	// Default crossfade transitions degrade cleanly without panicking when
	// shader compilation is omitted.
	var page *Signal[int]
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		page = NewSignal(cx, 0)
		return View{Children: []Node{
			ShaderTransition{Value: func() any { return page.Get() }, Children: []Node{Text{Content: "a"}}},
			ScreenTransition{Value: func() any { return page.Get() }},
		}}
	})
	h.commit()
	page.Set(1)
	h.commit()

	var nilProg *ShaderProgram
	if _, _, ok := nilProg.UniformLanes("Time"); ok || nilProg.HasUniform("Time") {
		t.Fatal("nil ShaderProgram unexpectedly reported uniforms")
	}
}
