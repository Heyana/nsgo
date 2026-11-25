package driver

import (
	"go/constant"
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// callExpr is a call as the front ends emit it, for the interpreter to evaluate.
func callExpr(f shaderir.BuiltinFunc, args ...shaderir.Expr) shaderir.Expr {
	return shaderir.Expr{Type: shaderir.Call, Exprs: append([]shaderir.Expr{{Type: shaderir.BuiltinFuncExpr, BuiltinFunc: f}}, args...)}
}

func numberExpr(v float64) shaderir.Expr {
	return shaderir.Expr{Type: shaderir.NumberExpr, Const: constant.MakeFloat64(v)}
}

func closeEnough(got, want float32) bool {
	if math.IsNaN(float64(want)) {
		return math.IsNaN(float64(got))
	}
	return math.Abs(float64(got-want)) <= 1e-5*(1+math.Abs(float64(want)))
}

// The software device evaluates the builtins the shader front ends emit against Go's math package, so headless tests and offscreen rendering see the same numbers as a native device (#2334).
func TestSoftwareDeviceEvaluatesTheMathBuiltins(t *testing.T) {
	vm := &irVM{}
	cases := []struct {
		name string
		expr shaderir.Expr
		want float32
	}{
		{"asin", callExpr(shaderir.Asin, numberExpr(0.5)), float32(math.Asin(0.5))},
		{"acos", callExpr(shaderir.Acos, numberExpr(0.5)), float32(math.Acos(0.5))},
		{"atan", callExpr(shaderir.Atan, numberExpr(0.5)), float32(math.Atan(0.5))},
		{"atan2", callExpr(shaderir.Atan2, numberExpr(1), numberExpr(2)), float32(math.Atan2(1, 2))},
		{"exp", callExpr(shaderir.Exp, numberExpr(1.5)), float32(math.Exp(1.5))},
		{"exp2", callExpr(shaderir.Exp2, numberExpr(1.5)), float32(math.Exp2(1.5))},
		{"log", callExpr(shaderir.Log, numberExpr(2.5)), float32(math.Log(2.5))},
		{"log2", callExpr(shaderir.Log2, numberExpr(2.5)), float32(math.Log2(2.5))},
		{"sinh", callExpr(shaderir.Sinh, numberExpr(1.5)), float32(math.Sinh(1.5))},
		{"cosh", callExpr(shaderir.Cosh, numberExpr(1.5)), float32(math.Cosh(1.5))},
		{"tanh", callExpr(shaderir.Tanh, numberExpr(1.5)), float32(math.Tanh(1.5))},
		{"trunc", callExpr(shaderir.Trunc, numberExpr(-1.7)), float32(math.Trunc(-1.7))},
		{"round", callExpr(shaderir.RoundEven, numberExpr(2.5)), float32(math.RoundToEven(2.5))},
		{"sign", callExpr(shaderir.Sign, numberExpr(-3)), -1},
		{"sign zero", callExpr(shaderir.Sign, numberExpr(0)), 0},
		{"mod", callExpr(shaderir.Mod, numberExpr(-1.5), numberExpr(1)), -1.5 - 1*float32(math.Floor(-1.5/1))},
		{"radians", callExpr(shaderir.Radians, numberExpr(180)), float32(math.Pi)},
		{"degrees", callExpr(shaderir.Degrees, numberExpr(math.Pi)), 180},
		{"distance", callExpr(shaderir.Distance,
			callExpr(shaderir.Vec3F, numberExpr(1), numberExpr(2), numberExpr(3)),
			callExpr(shaderir.Vec3F, numberExpr(4), numberExpr(6), numberExpr(3))), 5},
		{"faceforward along", callExpr(shaderir.Faceforward,
			callExpr(shaderir.Vec3F, numberExpr(0), numberExpr(0), numberExpr(1)),
			callExpr(shaderir.Vec3F, numberExpr(0), numberExpr(0), numberExpr(-1)),
			callExpr(shaderir.Vec3F, numberExpr(0), numberExpr(0), numberExpr(1))), 1},
		{"refract", callExpr(shaderir.Refract,
			callExpr(shaderir.Vec3F, numberExpr(0), numberExpr(-1), numberExpr(0)),
			callExpr(shaderir.Vec3F, numberExpr(0), numberExpr(1), numberExpr(0)),
			numberExpr(0.5)), -1},
		{"determinant", callExpr(shaderir.Determinant,
			callExpr(shaderir.Mat3F,
				numberExpr(1), numberExpr(2), numberExpr(3),
				numberExpr(0), numberExpr(1), numberExpr(4),
				numberExpr(5), numberExpr(6), numberExpr(0))), 1},
		{"transpose", callExpr(shaderir.Transpose,
			callExpr(shaderir.Mat3x2F,
				numberExpr(1), numberExpr(2),
				numberExpr(3), numberExpr(4),
				numberExpr(5), numberExpr(6))), 3},
		{"intBitsToFloat", callExpr(shaderir.IntBitsToFloat, callExpr(shaderir.FloatBitsToInt, numberExpr(2.5))), 2.5},
		{"floatBitsToUint round trip", callExpr(shaderir.UintBitsToFloat, callExpr(shaderir.FloatBitsToUint, numberExpr(-3.25))), -3.25},
		{"derivative is zero", callExpr(shaderir.Dfdx, numberExpr(7)), 0},
	}
	// A vector result is checked at the lane its value lands in.
	lanes := map[string]int{"faceforward along": 2, "refract": 1, "transpose": 1}
	for _, c := range cases {
		got := vm.evalExpr(c.expr)
		lane := lanes[c.name]
		if !closeEnough(got.f[lane], c.want) {
			t.Errorf("%s lane %d = %v, want %v", c.name, lane, got.f[lane], c.want)
		}
	}
	if got := vm.evalExpr(callExpr(shaderir.FloatBitsToInt, numberExpr(1))); got.i[0] != int32(math.Float32bits(1)) {
		t.Errorf("floatBitsToInt = %d, want %d", got.i[0], int32(math.Float32bits(1)))
	}
	if got := vm.evalExpr(callExpr(shaderir.FloatBitsToUint, numberExpr(-2))); got.u[0] != math.Float32bits(-2) {
		t.Errorf("floatBitsToUint = %d, want %d", got.u[0], math.Float32bits(-2))
	}
}

// A builtin the interpreter has no case for is reported when the pipeline is created, instead of drawing a zero where the call is.
func TestUnsupportedBuiltinIsReported(t *testing.T) {
	missing := shaderir.BuiltinFunc("__missing")
	prog := &shaderir.Program{ComputeFunc: shaderir.ComputeFunc{Block: &shaderir.Block{
		Stmts: []shaderir.Stmt{{Type: shaderir.ExprStmt, Exprs: []shaderir.Expr{callExpr(missing, numberExpr(1))}}},
	}}}
	if got := unsupportedSoftBuiltin(prog); got != missing {
		t.Fatalf("unsupportedSoftBuiltin = %q, want %q", got, missing)
	}
	if got := unsupportedSoftBuiltin(&shaderir.Program{ComputeFunc: shaderir.ComputeFunc{Block: &shaderir.Block{
		Stmts: []shaderir.Stmt{{Type: shaderir.ExprStmt, Exprs: []shaderir.Expr{callExpr(shaderir.Exp, numberExpr(1))}}},
	}}}); got != "" {
		t.Fatalf("unsupportedSoftBuiltin = %q for exp, want none", got)
	}
}
