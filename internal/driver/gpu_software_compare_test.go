package driver

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// A comparison on the software device follows IEEE 754 and WGSL: with a NaN every ordered comparison is false and only != is true, and a bool compares its value (#2335).
func TestSoftwareDeviceComparesNaNAndBools(t *testing.T) {
	vm := &irVM{}
	floatVal := func(v float32) irVal {
		return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{v}}
	}
	intVal := func(v int32) irVal {
		return irVal{typ: shaderir.Type{Main: shaderir.Int}, i: [4]int32{v}, f: [16]float32{float32(v)}}
	}
	uintVal := func(v uint32) irVal {
		return irVal{typ: shaderir.Type{Main: shaderir.Uint}, u: [4]uint32{v}, f: [16]float32{float32(v)}}
	}
	vec2Val := func(x, y float32) irVal {
		return irVal{typ: shaderir.Type{Main: shaderir.Vec2}, f: [16]float32{x, y}}
	}
	boolVal := func(v bool) irVal {
		return irVal{typ: shaderir.Type{Main: shaderir.Bool}, b: v}
	}
	nan := floatVal(float32(math.NaN()))
	one := floatVal(1)
	cases := []struct {
		op   shaderir.Op
		a, b irVal
		want bool
	}{
		{shaderir.EqualOp, nan, nan, false},
		{shaderir.LessThanOp, nan, one, false},
		{shaderir.LessThanEqualOp, nan, one, false},
		{shaderir.GreaterThanOp, nan, one, false},
		{shaderir.GreaterThanEqualOp, nan, one, false},
		{shaderir.NotEqualOp, nan, nan, true},
		{shaderir.EqualOp, one, one, true},
		{shaderir.LessThanOp, one, floatVal(2), true},
		{shaderir.EqualOp, boolVal(true), boolVal(false), false},
		{shaderir.NotEqualOp, boolVal(true), boolVal(false), true},
		{shaderir.EqualOp, boolVal(true), boolVal(true), true},
		{shaderir.LessThanOp, intVal(-1), intVal(0), true},
		{shaderir.GreaterThanEqualOp, uintVal(3), uintVal(3), true},
		{shaderir.VectorEqualOp, vec2Val(1, 2), vec2Val(1, 2), true},
		{shaderir.VectorEqualOp, vec2Val(1, 2), vec2Val(1, 3), false},
		{shaderir.VectorNotEqualOp, vec2Val(1, 2), vec2Val(1, 3), true},
		{shaderir.VectorEqualOp, vec2Val(float32(math.NaN()), 2), vec2Val(float32(math.NaN()), 2), false},
		{shaderir.VectorNotEqualOp, vec2Val(float32(math.NaN()), 2), vec2Val(float32(math.NaN()), 2), true},
	}
	for _, c := range cases {
		if got := vm.evalBinary(c.op, c.a, c.b); got.b != c.want {
			t.Errorf("%v = %v, want %v", c.op, got.b, c.want)
		}
	}
}
