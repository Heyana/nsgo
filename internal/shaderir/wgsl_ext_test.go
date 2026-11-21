package shaderir_test

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/hlsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/msl"
)

func TestPrivatesAndStructsCodegen(t *testing.T) {
	inner := shaderir.Type{
		Main: shaderir.Struct,
		Sub: []shaderir.Type{
			{Main: shaderir.Float},
			{Main: shaderir.Array, Sub: []shaderir.Type{{Main: shaderir.Float}}, Length: 2},
		},
	}
	outer := shaderir.Type{
		Main: shaderir.Struct,
		Sub: []shaderir.Type{
			{Main: shaderir.Vec2},
			inner,
		},
	}
	if got := inner.DwordCount(); got != 3 {
		t.Fatalf("inner.DwordCount() = %d, want 3", got)
	}
	if got := outer.DwordCount(); got != 5 {
		t.Fatalf("outer.DwordCount() = %d, want 5", got)
	}

	p := &shaderir.Program{
		Uniforms: []shaderir.Type{{Main: shaderir.Vec2}},
		Privates: []shaderir.Type{
			{Main: shaderir.Float},
			outer,
			{Main: shaderir.Array, Sub: []shaderir.Type{inner}, Length: 2},
		},
		Varyings: []shaderir.Type{{Main: shaderir.Vec2}},
		Funcs: []shaderir.Func{
			{
				Index:    0,
				InParams: []shaderir.Type{outer},
				Return:   shaderir.Type{Main: shaderir.Float},
				Block: &shaderir.Block{
					LocalVars:           []shaderir.Type{inner},
					LocalVarIndexOffset: 1,
					Stmts: []shaderir.Stmt{
						assignStmt(
							shaderir.Expr{Type: shaderir.PrivateVariable, Index: 0},
							fieldSelectorExpr(
								fieldSelectorExpr(localVariableExpr(0), shaderir.Expr{Type: shaderir.StructMember, Index: 1}),
								shaderir.Expr{Type: shaderir.StructMember, Index: 0},
							),
						),
						returnStmt(shaderir.Expr{Type: shaderir.PrivateVariable, Index: 0}),
					},
				},
			},
		},
		FragmentFunc: shaderir.FragmentFunc{
			Block: &shaderir.Block{
				LocalVarIndexOffset: 2,
				Stmts: []shaderir.Stmt{
					returnStmt(callExpr(
						builtinFuncExpr(shaderir.Vec4F),
						callExpr(functionExpr(0), shaderir.Expr{Type: shaderir.PrivateVariable, Index: 1}),
						floatExpr(0),
						floatExpr(0),
						floatExpr(1),
					)),
				},
			},
		},
	}

	// GLSL Fragment and Compile: Inner struct S0 must precede Outer struct S1,
	// and P0/P1/P2 must be module-scope variables.
	glslFrag := glsl.Fragment(p, "nsgoShader", glsl.Options{Block: "U"})
	_, glslFS := glsl.Compile(p, glsl.GLSLVersionES300, -1)
	for name, out := range map[string]string{"glsl.Fragment": glslFrag, "glsl.Compile": glslFS} {
		if strings.Contains(out, "?(") {
			t.Errorf("%s contains invalid output:\n%s", name, out)
		}
		iS0 := strings.Index(out, "struct S0 {")
		iS1 := strings.Index(out, "struct S1 {")
		if iS0 < 0 || iS1 < 0 || iS0 >= iS1 {
			t.Errorf("%s: dependency struct S0 must be declared before S1:\n%s", name, out)
		}
		for _, want := range []string{"float P0;", "S1 P1;", "S0 P2[2];", "S0 l1 = S0(float(0), float[2](float(0), float(0)));", "P0 = ((l0).M1).M0;"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s missing %q:\n%s", name, want, out)
			}
		}
	}

	// MSL Fragment and Compile: struct Privates passed by thread reference.
	mslFrag := msl.Fragment(p, "nsgoShader")
	mslAll := msl.Compile(p)
	for name, out := range map[string]string{"msl.Fragment": mslFrag, "msl.Compile": mslAll} {
		if strings.Contains(out, "?(") {
			t.Errorf("%s contains invalid output:\n%s", name, out)
		}
		iS0 := strings.Index(out, "struct S0 {")
		iS1 := strings.Index(out, "struct S1 {")
		iPriv := strings.Index(out, "struct Privates {")
		if iS0 < 0 || iS1 < 0 || iPriv < 0 || !(iS0 < iS1 && iS1 < iPriv) {
			t.Errorf("%s: expected S0 < S1 < Privates:\n%s", name, out)
		}
		for _, want := range []string{"thread Privates& privates", "Privates privates = {};", "privates.P0 = ((l0).M1).M0;", "array<S0, 2> P2;"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s missing %q:\n%s", name, want, out)
			}
		}
	}

	// HLSL Fragment and Compile: static globals, (S0)0 zero-init, S0 before S1.
	hlslFrag := hlsl.Fragment(p, "nsgoShader", 0, 1)
	_, hlslPS, _, _ := hlsl.Compile(p)
	for name, out := range map[string]string{"hlsl.Fragment": hlslFrag, "hlsl.Compile": hlslPS} {
		if strings.Contains(out, "?(") {
			t.Errorf("%s contains invalid output:\n%s", name, out)
		}
		iS0 := strings.Index(out, "struct S0 {")
		iS1 := strings.Index(out, "struct S1 {")
		if iS0 < 0 || iS1 < 0 || iS0 >= iS1 {
			t.Errorf("%s: dependency struct S0 must be declared before S1:\n%s", name, out)
		}
		for _, want := range []string{"static float P0;", "static S1 P1;", "static S0 P2[2];", "S0 l1 = (S0)0;", "P0 = ((l0).M1).M0;"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s missing %q:\n%s", name, want, out)
			}
		}
	}
}

func TestNonSquareMatricesIR(t *testing.T) {
	mats := []struct {
		bt         shaderir.BasicType
		cols, rows int
		name       string
		glslType   string
		mslType    string
		hlslType   string
	}{
		{shaderir.Mat2x3, 2, 3, "mat2x3", "mat2x3", "float2x3", "float2x3"},
		{shaderir.Mat2x4, 2, 4, "mat2x4", "mat2x4", "float2x4", "float2x4"},
		{shaderir.Mat3x2, 3, 2, "mat3x2", "mat3x2", "float3x2", "float3x2"},
		{shaderir.Mat3x4, 3, 4, "mat3x4", "mat3x4", "float3x4", "float3x4"},
		{shaderir.Mat4x2, 4, 2, "mat4x2", "mat4x2", "float4x2", "float4x2"},
		{shaderir.Mat4x3, 4, 3, "mat4x3", "mat4x3", "float4x3", "float4x3"},
	}

	for _, m := range mats {
		ty := shaderir.Type{Main: m.bt}
		if !ty.IsMatrix() {
			t.Errorf("%s: IsMatrix() = false", m.name)
		}
		if ty.MatrixCols() != m.cols || ty.MatrixRows() != m.rows {
			t.Errorf("%s: dims = %dx%d, want %dx%d", m.name, ty.MatrixCols(), ty.MatrixRows(), m.cols, m.rows)
		}
		if ty.DwordCount() != m.cols*m.rows {
			t.Errorf("%s: DwordCount() = %d, want %d", m.name, ty.DwordCount(), m.cols*m.rows)
		}
		if ty.String() != m.name {
			t.Errorf("%s: String() = %q", m.name, ty.String())
		}
		if shaderir.MatrixOf(m.cols, m.rows) != m.bt {
			t.Errorf("MatrixOf(%d,%d) != %s", m.cols, m.rows, m.name)
		}

		// Uniform layout sanity checks across GLSL, MSL, HLSL.
		p := &shaderir.Program{Uniforms: []shaderir.Type{ty}}
		if lanes := p.LaneTypes(); len(lanes) != m.cols*m.rows {
			t.Errorf("%s: LaneTypes len = %d, want %d", m.name, len(lanes), m.cols*m.rows)
		}
		if l, _ := glsl.UniformLanes(p); len(l) != m.cols*m.rows {
			t.Errorf("%s: glsl.UniformLanes len = %d", m.name, len(l))
		}
		if l, _ := msl.UniformLanes(p); len(l) != m.cols*m.rows {
			t.Errorf("%s: msl.UniformLanes len = %d", m.name, len(l))
		}
		if l, _ := hlsl.UniformLanes(p); len(l) != m.cols*m.rows {
			t.Errorf("%s: hlsl.UniformLanes len = %d", m.name, len(l))
		}
	}

	// Binary MatrixMul type checking.
	if got, ok := shaderir.TypeFromBinaryOp(shaderir.MatrixMul, shaderir.Type{Main: shaderir.Mat3x2}, shaderir.Type{Main: shaderir.Mat2x3}, nil, nil); !ok || got.Main != shaderir.Mat2 {
		t.Errorf("mat3x2 * mat2x3 = (%v, %v), want (mat2, true)", got, ok)
	}
	if got, ok := shaderir.TypeFromBinaryOp(shaderir.MatrixMul, shaderir.Type{Main: shaderir.Mat2x3}, shaderir.Type{Main: shaderir.Mat3x4}, nil, nil); ok {
		t.Errorf("mat2x3 * mat3x4 should fail, got %v", got)
	}
	if got, ok := shaderir.TypeFromBinaryOp(shaderir.MatrixMul, shaderir.Type{Main: shaderir.Mat2x3}, shaderir.Type{Main: shaderir.Mat2x3}, nil, nil); ok {
		t.Errorf("mat2x3 * mat2x3 should fail, got %v", got)
	}
	if got, ok := shaderir.TypeFromBinaryOp(shaderir.MatrixMul, shaderir.Type{Main: shaderir.Mat2x3}, shaderir.Type{Main: shaderir.Vec2}, nil, nil); !ok || got.Main != shaderir.Vec3 {
		t.Errorf("mat2x3 * vec2 = (%v, %v), want (vec3, true)", got, ok)
	}
	if got, ok := shaderir.TypeFromBinaryOp(shaderir.MatrixMul, shaderir.Type{Main: shaderir.Vec3}, shaderir.Type{Main: shaderir.Mat2x3}, nil, nil); !ok || got.Main != shaderir.Vec2 {
		t.Errorf("vec3 * mat2x3 = (%v, %v), want (vec2, true)", got, ok)
	}

	// Codegen for non-square matrix construction, multiplication, and transpose.
	p := &shaderir.Program{
		Funcs: []shaderir.Func{
			{
				Index:    0,
				InParams: []shaderir.Type{{Main: shaderir.Mat2x3}, {Main: shaderir.Vec2}},
				Return:   shaderir.Type{Main: shaderir.Vec2},
				Block: &shaderir.Block{
					LocalVars:           []shaderir.Type{{Main: shaderir.Mat3x2}},
					LocalVarIndexOffset: 2,
					Stmts: []shaderir.Stmt{
						assignStmt(localVariableExpr(2), callExpr(builtinFuncExpr(shaderir.Transpose), localVariableExpr(0))),
						returnStmt(binaryExpr(shaderir.MatrixMul, localVariableExpr(2), binaryExpr(shaderir.MatrixMul, localVariableExpr(0), localVariableExpr(1)))),
					},
				},
			},
		},
		FragmentFunc: shaderir.FragmentFunc{
			Block: &shaderir.Block{
				LocalVarIndexOffset: 1,
				Stmts: []shaderir.Stmt{
					returnStmt(callExpr(
						builtinFuncExpr(shaderir.Vec4F),
						callExpr(
							functionExpr(0),
							callExpr(builtinFuncExpr(shaderir.Mat2x3F), floatExpr(1), floatExpr(0), floatExpr(0), floatExpr(0), floatExpr(1), floatExpr(0)),
							callExpr(builtinFuncExpr(shaderir.Vec2F), floatExpr(1), floatExpr(2)),
						),
						floatExpr(0),
						floatExpr(1),
					)),
				},
			},
		},
	}

	glslOut := glsl.Fragment(p, "nsgoShader", glsl.Options{Block: "U"})
	mslOut := msl.Fragment(p, "nsgoShader")
	hlslOut := hlsl.Fragment(p, "nsgoShader", 0, 1)
	for lang, pair := range map[string]struct {
		out  string
		want []string
	}{
		"glsl": {glslOut, []string{"mat2x3", "mat3x2", "transpose(l0)", "mat3x2(0)"}},
		"msl":  {mslOut, []string{"float2x3", "float3x2", "transpose(l0)", "float3x2(0)"}},
		"hlsl": {hlslOut, []string{"float2x3", "float3x2", "transpose(l0)", "mul(mul(l1, l0), l2)"}},
	} {
		if strings.Contains(pair.out, "?(") {
			t.Errorf("%s contains invalid output:\n%s", lang, pair.out)
		}
		for _, w := range pair.want {
			if !strings.Contains(pair.out, w) {
				t.Errorf("%s missing %q:\n%s", lang, w, pair.out)
			}
		}
	}
}
