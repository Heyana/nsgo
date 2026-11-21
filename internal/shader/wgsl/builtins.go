package wgsl

import (
	"go/constant"
	"math"
	"math/bits"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

type builtinFunc func(c *checker, args []value, pos int, out *[]shaderir.Stmt) value

// builtin returns the WGSL built-in function a fragment shader may call
// by name, of those the IR's languages share, or nil. The table is built
// on first use, so apps that compile no shader link none of it.
func builtin(name string) builtinFunc {
	builtinTable.Do(initBuiltins)
	return builtinTable.funcs[name]
}

var builtinTable struct {
	sync.Once
	funcs map[string]builtinFunc
}

func initBuiltins() {
	builtinFuncs := map[string]builtinFunc{
		"select": selectFunc, "all": allAny(true), "any": allAny(false),
		"length": floatScalar(shaderir.Length, 1), "distance": floatScalar(shaderir.Distance, 2), "dot": floatScalar(shaderir.Dot, 2),
		"cross": crossFunc, "transpose": transposeFunc, "determinant": determinantFunc,
		"min": numeric(shaderir.Min, 2, false), "max": numeric(shaderir.Max, 2, false), "clamp": numeric(shaderir.Clamp, 3, false),
		"abs": numeric(shaderir.Abs, 1, false), "sign": numeric(shaderir.Sign, 1, true),
		"atan2": floats(shaderir.Atan2, 2), "pow": floats(shaderir.Pow, 2), "step": floats(shaderir.Step, 2),
		"reflect": vectors(shaderir.Reflect, 2), "smoothstep": floats(shaderir.Smoothstep, 3),
		"faceForward": vectors(shaderir.Faceforward, 3), "normalize": vectors(shaderir.Normalize, 1),
		"mix": mixFunc, "refract": refractFunc, "fma": fmaFunc, "saturate": saturateFunc,
		"degrees": scaled(57.29577951308232), "radians": scaled(0.017453292519943295),
		"ldexp": ldexpFunc,
		"asinh": lowered("asinh"), "acosh": lowered("acosh"), "atanh": lowered("atanh"),
		"countLeadingZeros":  bitUnary("countLeadingZeros", "__wgsl_clz_u32", false),
		"countOneBits":       bitUnary("countOneBits", "__wgsl_popcnt_u32", false),
		"countTrailingZeros": bitUnary("countTrailingZeros", "__wgsl_ctz_u32", false),
		"firstLeadingBit":    bitUnary("firstLeadingBit", "__wgsl_flb_u32", true),
		"firstTrailingBit":   bitUnary("firstTrailingBit", "__wgsl_ftb_u32", false),
		"reverseBits":        bitUnary("reverseBits", "__wgsl_rev_u32", false),
		"extractBits":        extractBitsFunc,
		"insertBits":         insertBitsFunc,
		"pack4x8snorm":       packFunc("pack4x8snorm", vec(kF32, 4), "__wgsl_pack4x8snorm"),
		"pack4x8unorm":       packFunc("pack4x8unorm", vec(kF32, 4), "__wgsl_pack4x8unorm"),
		"pack2x16snorm":      packFunc("pack2x16snorm", vec(kF32, 2), "__wgsl_pack2x16snorm"),
		"pack2x16unorm":      packFunc("pack2x16unorm", vec(kF32, 2), "__wgsl_pack2x16unorm"),
		"pack2x16float":      packFunc("pack2x16float", vec(kF32, 2), "__wgsl_pack2x16float"),
		"pack4xI8":           packFunc("pack4xI8", vec(kI32, 4), "__wgsl_pack4xI8"),
		"pack4xU8":           packFunc("pack4xU8", vec(kU32, 4), "__wgsl_pack4xU8"),
		"pack4xI8Clamp":      packFunc("pack4xI8Clamp", vec(kI32, 4), "__wgsl_pack4xI8Clamp"),
		"pack4xU8Clamp":      packFunc("pack4xU8Clamp", vec(kU32, 4), "__wgsl_pack4xU8Clamp"),
		"unpack4x8snorm":     unpackFunc("unpack4x8snorm", vec(kF32, 4), "__wgsl_unpack4x8snorm"),
		"unpack4x8unorm":     unpackFunc("unpack4x8unorm", vec(kF32, 4), "__wgsl_unpack4x8unorm"),
		"unpack2x16snorm":    unpackFunc("unpack2x16snorm", vec(kF32, 2), "__wgsl_unpack2x16snorm"),
		"unpack2x16unorm":    unpackFunc("unpack2x16unorm", vec(kF32, 2), "__wgsl_unpack2x16unorm"),
		"unpack2x16float":    unpackFunc("unpack2x16float", vec(kF32, 2), "__wgsl_unpack2x16float"),
		"unpack4xI8":         unpackFunc("unpack4xI8", vec(kI32, 4), "__wgsl_unpack4xI8"),
		"unpack4xU8":         unpackFunc("unpack4xU8", vec(kU32, 4), "__wgsl_unpack4xU8"),
		"modf":               modfFunc,
		"frexp":              frexpFunc,
	}
	for name, f := range map[string]shaderir.BuiltinFunc{
		"sin": shaderir.Sin, "cos": shaderir.Cos, "tan": shaderir.Tan, "asin": shaderir.Asin, "acos": shaderir.Acos,
		"atan": shaderir.Atan, "exp": shaderir.Exp, "exp2": shaderir.Exp2, "log": shaderir.Log, "log2": shaderir.Log2,
		"sqrt": shaderir.Sqrt, "inverseSqrt": shaderir.Inversesqrt, "fract": shaderir.Fract, "ceil": shaderir.Ceil,
		"floor": shaderir.Floor, "trunc": shaderir.Trunc, "round": shaderir.RoundEven,
		"sinh": shaderir.Sinh, "cosh": shaderir.Cosh, "tanh": shaderir.Tanh,
		"dpdx": shaderir.Dfdx, "dpdy": shaderir.Dfdy, "fwidth": shaderir.Fwidth,
		"dpdxCoarse": shaderir.Dfdx, "dpdyCoarse": shaderir.Dfdy, "fwidthCoarse": shaderir.Fwidth,
		"dpdxFine": shaderir.Dfdx, "dpdyFine": shaderir.Dfdy, "fwidthFine": shaderir.Fwidth,
	} {
		builtinFuncs[name] = floats(f, 1)
	}
	builtinTable.funcs = builtinFuncs
}

func arity(args []value, n int, pos int, name string) {
	if len(args) != n {
		fail(pos, "%s takes %d arguments, not %d", name, n, len(args))
	}
}

// common gives args one type, an abstract argument taking a concrete
// one's, and, for floats, an integer abstract one a float's.
func (c *checker) common(args []value, pos int, float bool) ty {
	var t ty
	found := false
	for _, a := range args {
		if !a.t.abstract() {
			t, found = a.t, true
			break
		}
	}
	if !found {
		t = args[0].t
		for _, a := range args {
			if a.t.scalar() == kAbsFloat {
				t = a.t
			}
		}
		if float {
			t = t.withScalar(kF32)
		}
		t = t.concrete()
	}
	for i := range args {
		args[i] = c.convert(args[i], t, pos)
	}
	return t
}

func callOf(c *checker, f shaderir.BuiltinFunc, t ty, args []value) value {
	var irArgs []shaderir.Expr
	for _, a := range args {
		irArgs = append(irArgs, c.ir(a))
	}
	return value{t: t, e: builtinCall(f, t, irArgs...)}
}

// floats is a function of n arguments of one float type, scalar or
// vector, returning that type.
func floats(f shaderir.BuiltinFunc, n int) builtinFunc {
	return func(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
		arity(args, n, pos, string(f))
		t := c.common(args, pos, true)
		if t.scalar() != kF32 || (t.k != kF32 && t.k != kVec) {
			fail(pos, "%s takes f32 or vectors of it, not a %s", f, t)
		}
		return callOf(c, f, t, args)
	}
}

// vectors is floats for float vectors only.
func vectors(f shaderir.BuiltinFunc, n int) builtinFunc {
	return func(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
		arity(args, n, pos, string(f))
		t := c.common(args, pos, true)
		if t.k != kVec || t.elem != kF32 {
			fail(pos, "%s takes vectors of f32, not a %s", f, t)
		}
		return callOf(c, f, t, args)
	}
}

// floatScalar is a function of float vectors returning an f32.
func floatScalar(f shaderir.BuiltinFunc, n int) builtinFunc {
	return func(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
		arity(args, n, pos, string(f))
		t := c.common(args, pos, true)
		if t.scalar() != kF32 || (t.k != kF32 && t.k != kVec) {
			fail(pos, "%s takes f32 or vectors of it, not a %s", f, t)
		}
		if t.k == kF32 && f == shaderir.Dot {
			fail(pos, "dot takes vectors")
		}
		return callOf(c, f, tF32, args)
	}
}

// numeric is a function of numbers of one type, floats or integers;
// signed refuses u32.
func numeric(f shaderir.BuiltinFunc, n int, signed bool) builtinFunc {
	return func(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
		arity(args, n, pos, string(f))
		t := c.common(args, pos, false)
		s := t.scalar()
		if !isNumeric(s) || (t.k != kVec && !t.isScalar()) || signed && s == kU32 {
			fail(pos, "%s takes numbers, not a %s", f, t)
		}
		if f == shaderir.Abs && s == kU32 {
			return args[0]
		}
		return callOf(c, f, t, args)
	}
}

func selectFunc(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
	arity(args, 3, pos, "select")
	cond := args[2]
	vals := args[:2]
	t := c.common(vals, pos, false)
	if !t.isScalar() && t.k != kVec {
		fail(pos, "select takes scalars or vectors, not a %s", t)
	}
	switch {
	case cond.t.k == kBool:
		if cond.c != nil {
			if b := cond.c.ExactString() == "true"; b {
				return vals[1]
			}
			return vals[0]
		}
		e := shaderir.Expr{Type: shaderir.Selection, Exprs: []shaderir.Expr{c.ir(cond), c.ir(vals[1]), c.ir(vals[0])}}
		return value{t: t, e: mk(e, t)}
	case cond.t.boolVector() && t.k == kVec && t.n == cond.t.n:
		f, tv := c.reuse(vals[0], out), c.reuse(vals[1], out)
		cv := c.reuse(cond, out)
		if t.elem == kF32 {
			return value{t: t, e: builtinCall(shaderir.Mix, t, c.ir(f), c.ir(tv), c.ir(cv))}
		}
		var comps []shaderir.Expr
		for i := range t.n {
			pick := mk(shaderir.Expr{Type: shaderir.Binary, Op: shaderir.GreaterThanOp, Exprs: []shaderir.Expr{component(c.ir(cv), i, cv.t), floatNumber(0.5)}}, tBool)
			comps = append(comps, mk(shaderir.Expr{Type: shaderir.Selection, Exprs: []shaderir.Expr{pick, component(c.ir(tv), i, t), component(c.ir(f), i, t)}}, ty{k: t.elem}))
		}
		return value{t: t, e: builtinCall(vecFunc(t), t, comps...)}
	}
	fail(pos, "select takes a bool, or a vector of bools as wide as its values, not a %s", cond.t)
	return value{}
}

func allAny(all bool) builtinFunc {
	return func(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
		arity(args, 1, pos, "all")
		v := args[0]
		if v.t.k == kBool {
			return v
		}
		if !v.t.boolVector() {
			fail(pos, "all and any take bools, not a %s", v.t)
		}
		// The components are 0 and 1: all are 1 where their sum is n, any
		// where it is above 0.
		ones := builtinCall(vecFunc(vec(kF32, v.t.n)), vec(kF32, v.t.n), floatNumber(1))
		sum := builtinCall(shaderir.Dot, tF32, c.ir(v), ones)
		bound := 0.5
		if all {
			bound = float64(v.t.n) - 0.5
		}
		return value{t: tBool, e: shaderir.Expr{Type: shaderir.Binary, Op: shaderir.GreaterThanOp, Exprs: []shaderir.Expr{sum, floatNumber(bound)}}}
	}
}

func crossFunc(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
	arity(args, 2, pos, "cross")
	t := c.common(args, pos, true)
	if !t.equal(vec(kF32, 3)) {
		fail(pos, "cross takes vec3f, not a %s", t)
	}
	return callOf(c, shaderir.Cross, t, args)
}

func transposeFunc(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
	arity(args, 1, pos, "transpose")
	m := args[0].t
	if m.k != kMat {
		fail(pos, "transpose takes a matrix, not a %s", m)
	}
	res := ty{k: kMat, elem: m.elem, n: m.rows, rows: m.n}
	return callOf(c, shaderir.Transpose, res, args)
}

func determinantFunc(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
	arity(args, 1, pos, "determinant")
	m := args[0].t
	if m.k != kMat || m.n != m.rows {
		fail(pos, "determinant takes a square matrix, not a %s", m)
	}
	return callOf(c, shaderir.Determinant, tF32, args)
}

// splat widens a scalar to t, a vector, as the languages that need one
// type of their arguments take it.
func (c *checker) splat(v value, t ty, pos int) value {
	if t.k != kVec || v.t.k == kVec {
		return v
	}
	v = c.convert(v, ty{k: t.elem}, pos)
	return value{t: t, e: builtinCall(vecFunc(t), t, c.ir(v))}
}

func mixFunc(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
	arity(args, 3, pos, "mix")
	t := c.common(args[:2], pos, true)
	if t.scalar() != kF32 || t.k != kF32 && t.k != kVec {
		fail(pos, "mix takes f32 or vectors of it, not a %s", t)
	}
	f := args[2]
	if f.t.k == kVec || t.k == kF32 {
		f = c.convert(f, t, pos)
	} else {
		f = c.splat(c.convert(f, tF32, pos), t, pos)
	}
	return callOf(c, shaderir.Mix, t, []value{args[0], args[1], f})
}

func refractFunc(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
	arity(args, 3, pos, "refract")
	t := c.common(args[:2], pos, true)
	if t.k != kVec || t.elem != kF32 {
		fail(pos, "refract takes vectors of f32, not a %s", t)
	}
	return callOf(c, shaderir.Refract, t, []value{args[0], args[1], c.convert(args[2], tF32, pos)})
}

func fmaFunc(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
	arity(args, 3, pos, "fma")
	t := c.common(args, pos, true)
	if t.scalar() != kF32 || t.k != kF32 && t.k != kVec {
		fail(pos, "fma takes f32 or vectors of it, not a %s", t)
	}
	m := c.arith("*", args[0], args[1], pos, out)
	return c.arith("+", m, args[2], pos, out)
}

func saturateFunc(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
	arity(args, 1, pos, "saturate")
	t := c.common(args, pos, true)
	if t.scalar() != kF32 || t.k != kF32 && t.k != kVec {
		fail(pos, "saturate takes f32 or vectors of it, not a %s", t)
	}
	lo := c.splat(value{t: tF32, e: floatNumber(0)}, t, pos)
	hi := c.splat(value{t: tF32, e: floatNumber(1)}, t, pos)
	return callOf(c, shaderir.Clamp, t, []value{args[0], lo, hi})
}

// scaled multiplies its float argument by k, as degrees and radians do.
func scaled(k float64) builtinFunc {
	return func(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
		arity(args, 1, pos, "degrees")
		t := c.common(args, pos, true)
		if t.scalar() != kF32 || t.k != kF32 && t.k != kVec {
			fail(pos, "degrees and radians take f32 or vectors of it, not a %s", t)
		}
		return value{t: t, e: mk(shaderir.Expr{Type: shaderir.Binary, Op: shaderir.ComponentWiseMul, Exprs: []shaderir.Expr{c.ir(args[0]), floatNumber(k)}}, t)}
	}
}

func ldexpFunc(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
	arity(args, 2, pos, "ldexp")
	x := c.concrete(args[0], pos)
	if x.t.abstract() || x.t.scalar() != kAbsFloat && x.t.scalar() != kF32 {
		x = c.convert(args[0], args[0].t.withScalar(kF32), pos)
	}
	e := c.concrete(args[1], pos)
	if e.t.scalar() != kI32 {
		fail(pos, "ldexp's exponent is an i32, not a %s", e.t)
	}
	ft := e.t.withScalar(kF32)
	var fe shaderir.Expr
	if ft.k == kVec {
		fe = builtinCall(vecFunc(ft), ft, c.ir(e))
	} else {
		fe = builtinCall(shaderir.FloatF, ft, c.ir(e))
	}
	p := builtinCall(shaderir.Exp2, ft, fe)
	return value{t: x.t, e: mk(shaderir.Expr{Type: shaderir.Binary, Op: shaderir.ComponentWiseMul, Exprs: []shaderir.Expr{c.ir(x), p}}, x.t)}
}

// lowered writes the inverse hyperbolic functions with logarithms.
func lowered(name string) builtinFunc {
	return func(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
		arity(args, 1, pos, name)
		t := c.common(args, pos, true)
		if t.scalar() != kF32 || t.k != kF32 && t.k != kVec {
			fail(pos, "%s takes f32 or vectors of it, not a %s", name, t)
		}
		x := c.reuse(args[0], out)
		one := c.splat(value{t: tF32, e: floatNumber(1)}, t, pos)
		bin := func(op shaderir.Op, a, b shaderir.Expr) shaderir.Expr {
			return mk(shaderir.Expr{Type: shaderir.Binary, Op: op, Exprs: []shaderir.Expr{a, b}}, t)
		}
		xi := c.ir(x)
		switch name {
		case "asinh", "acosh":
			op := shaderir.Add
			if name == "acosh" {
				op = shaderir.Sub
			}
			r := builtinCall(shaderir.Sqrt, t, bin(op, bin(shaderir.ComponentWiseMul, xi, xi), c.ir(one)))
			return value{t: t, e: builtinCall(shaderir.Log, t, bin(shaderir.Add, xi, r))}
		}
		q := bin(shaderir.Div, bin(shaderir.Add, c.ir(one), xi), bin(shaderir.Sub, c.ir(one), xi))
		return value{t: t, e: bin(shaderir.ComponentWiseMul, builtinCall(shaderir.Log, t, q), floatNumber(0.5))}
	}
}

func (c *checker) callHelper(name string, pos int, args ...shaderir.Expr) value {
	sym := c.global.names[name]
	if sym == nil || sym.kind != symFn {
		fail(pos, "internal error: helper %s not found", name)
	}
	f := sym.fn
	if c.cur == nil {
		fail(pos, "the expression is not a constant")
	}
	c.cur.info.calls = append(c.cur.info.calls, f)
	exprs := append([]shaderir.Expr{{Type: shaderir.FunctionExpr, Index: f.index}}, args...)
	return value{t: f.ret, e: mk(shaderir.Expr{Type: shaderir.Call, Exprs: exprs}, f.ret)}
}

func constUint32(v constant.Value, k kind) uint32 {
	if k == kU32 {
		u, _ := constant.Uint64Val(constant.ToInt(v))
		return uint32(u)
	}
	i, _ := constant.Int64Val(constant.ToInt(v))
	return uint32(int32(i))
}

func makeIntConst(u uint32, k kind) constant.Value {
	if k == kU32 {
		return constant.MakeUint64(uint64(u))
	}
	return constant.MakeInt64(int64(int32(u)))
}

func evalBitUnary(name string, u uint32, signed bool) uint32 {
	switch name {
	case "countLeadingZeros":
		return uint32(bits.LeadingZeros32(u))
	case "countOneBits":
		return uint32(bits.OnesCount32(u))
	case "countTrailingZeros":
		return uint32(bits.TrailingZeros32(u))
	case "reverseBits":
		return bits.Reverse32(u)
	case "firstTrailingBit":
		if u == 0 {
			return 0xFFFFFFFF
		}
		return uint32(bits.TrailingZeros32(u))
	case "firstLeadingBit":
		if signed {
			x := int32(u)
			y := u
			if x < 0 {
				y = ^y
			}
			if y == 0 {
				return 0xFFFFFFFF
			}
			return 31 - uint32(bits.LeadingZeros32(y))
		}
		if u == 0 {
			return 0xFFFFFFFF
		}
		return 31 - uint32(bits.LeadingZeros32(u))
	}
	return 0
}

func bitUnary(name, u32Helper string, signedDiffers bool) builtinFunc {
	return func(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
		arity(args, 1, pos, name)
		v := c.concrete(args[0], pos)
		t := v.t
		s := t.scalar()
		if (s != kI32 && s != kU32) || (!t.isScalar() && t.k != kVec) {
			fail(pos, "%s takes i32, u32 or vectors of them, not a %s", name, t)
		}
		signed := s == kI32
		if v.c != nil {
			r := evalBitUnary(name, constUint32(v.c, s), signed)
			return value{t: t, c: makeIntConst(r, s)}
		}
		if v.comps != nil {
			comps := make([]constant.Value, len(v.comps))
			for i, comp := range v.comps {
				r := evalBitUnary(name, constUint32(comp, s), signed)
				comps[i] = makeIntConst(r, s)
			}
			return c.vectorOfConstants(t, comps, pos)
		}
		applyScalar := func(arg shaderir.Expr) shaderir.Expr {
			if signed && signedDiffers {
				return c.callHelper("__wgsl_flb_i32", pos, arg).e
			}
			if signed {
				arg = builtinCall(shaderir.UintF, tU32, arg)
			}
			r := c.callHelper(u32Helper, pos, arg).e
			if signed {
				r = builtinCall(shaderir.IntF, tI32, r)
			}
			return r
		}
		if t.isScalar() {
			return value{t: t, e: applyScalar(c.ir(v))}
		}
		v = c.reuse(v, out)
		comps := make([]shaderir.Expr, t.n)
		for i := range t.n {
			comps[i] = applyScalar(component(c.ir(v), i, t))
		}
		return value{t: t, e: builtinCall(vecFunc(t), t, comps...)}
	}
}

func evalExtractBits(u, offset, count uint32, signed bool) uint32 {
	o := min(offset, 32)
	cnt := min(count, 32-o)
	if cnt == 0 {
		return 0
	}
	r := (u << (32 - (o + cnt))) >> (32 - cnt)
	if signed {
		signBit := uint32(1) << (cnt - 1)
		r = (r ^ signBit) - signBit
	}
	return r
}

func extractBitsFunc(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
	arity(args, 3, pos, "extractBits")
	v := c.concrete(args[0], pos)
	t := v.t
	s := t.scalar()
	if (s != kI32 && s != kU32) || (!t.isScalar() && t.k != kVec) {
		fail(pos, "extractBits takes i32, u32 or vectors of them, not a %s", t)
	}
	off := c.convert(args[1], tU32, pos)
	cnt := c.convert(args[2], tU32, pos)
	signed := s == kI32
	if off.c != nil && cnt.c != nil {
		ou, cu := constUint32(off.c, kU32), constUint32(cnt.c, kU32)
		if v.c != nil {
			r := evalExtractBits(constUint32(v.c, s), ou, cu, signed)
			return value{t: t, c: makeIntConst(r, s)}
		}
		if v.comps != nil {
			comps := make([]constant.Value, len(v.comps))
			for i, comp := range v.comps {
				r := evalExtractBits(constUint32(comp, s), ou, cu, signed)
				comps[i] = makeIntConst(r, s)
			}
			return c.vectorOfConstants(t, comps, pos)
		}
	}
	helper := "__wgsl_extract_u32"
	if signed {
		helper = "__wgsl_extract_i32"
	}
	if t.isScalar() {
		return c.callHelper(helper, pos, c.ir(v), c.ir(off), c.ir(cnt))
	}
	v = c.reuse(v, out)
	off = c.reuse(off, out)
	cnt = c.reuse(cnt, out)
	comps := make([]shaderir.Expr, t.n)
	for i := range t.n {
		comps[i] = c.callHelper(helper, pos, component(c.ir(v), i, t), c.ir(off), c.ir(cnt)).e
	}
	return value{t: t, e: builtinCall(vecFunc(t), t, comps...)}
}

func evalInsertBits(e, nb, offset, count uint32) uint32 {
	o := min(offset, 32)
	cnt := min(count, 32-o)
	if cnt == 0 {
		return e
	}
	if cnt == 32 {
		return nb
	}
	mask := ((uint32(1) << cnt) - 1) << o
	return (e &^ mask) | ((nb << o) & mask)
}

func insertBitsFunc(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
	arity(args, 4, pos, "insertBits")
	t := c.common(args[:2], pos, false)
	s := t.scalar()
	if (s != kI32 && s != kU32) || (!t.isScalar() && t.k != kVec) {
		fail(pos, "insertBits takes i32, u32 or vectors of them, not a %s", t)
	}
	e, nb := args[0], args[1]
	off := c.convert(args[2], tU32, pos)
	cnt := c.convert(args[3], tU32, pos)
	signed := s == kI32
	if off.c != nil && cnt.c != nil {
		ou, cu := constUint32(off.c, kU32), constUint32(cnt.c, kU32)
		if e.c != nil && nb.c != nil {
			r := evalInsertBits(constUint32(e.c, s), constUint32(nb.c, s), ou, cu)
			return value{t: t, c: makeIntConst(r, s)}
		}
		if e.comps != nil && nb.comps != nil {
			comps := make([]constant.Value, len(e.comps))
			for i := range e.comps {
				r := evalInsertBits(constUint32(e.comps[i], s), constUint32(nb.comps[i], s), ou, cu)
				comps[i] = makeIntConst(r, s)
			}
			return c.vectorOfConstants(t, comps, pos)
		}
	}
	applyScalar := func(ae, anb, aoff, acnt shaderir.Expr) shaderir.Expr {
		if signed {
			ae = builtinCall(shaderir.UintF, tU32, ae)
			anb = builtinCall(shaderir.UintF, tU32, anb)
		}
		r := c.callHelper("__wgsl_insert_u32", pos, ae, anb, aoff, acnt).e
		if signed {
			r = builtinCall(shaderir.IntF, tI32, r)
		}
		return r
	}
	if t.isScalar() {
		return value{t: t, e: applyScalar(c.ir(e), c.ir(nb), c.ir(off), c.ir(cnt))}
	}
	e = c.reuse(e, out)
	nb = c.reuse(nb, out)
	off = c.reuse(off, out)
	cnt = c.reuse(cnt, out)
	comps := make([]shaderir.Expr, t.n)
	for i := range t.n {
		comps[i] = applyScalar(component(c.ir(e), i, t), component(c.ir(nb), i, t), c.ir(off), c.ir(cnt))
	}
	return value{t: t, e: builtinCall(vecFunc(t), t, comps...)}
}

func f32ToF16(x float32) uint32 {
	bits := math.Float32bits(x)
	sign := (bits >> 16) & 0x8000
	exp := (bits >> 23) & 0xFF
	mant := bits & 0x7FFFFF
	if exp == 255 {
		if mant != 0 {
			return sign | 0x7E00 | (mant >> 13)
		}
		return sign | 0x7C00
	}
	if exp > 142 {
		return sign | 0x7C00
	}
	if exp >= 113 {
		hExp := (exp - 112) << 10
		hMant := mant >> 13
		rem := mant & 0x1FFF
		h := hExp | hMant
		if rem > 0x1000 || (rem == 0x1000 && (hMant&1) != 0) {
			h++
		}
		return sign | h
	}
	if exp >= 103 {
		fullMant := 0x800000 | mant
		shift := 126 - exp
		hMant := fullMant >> shift
		half := uint32(1) << (shift - 1)
		rem := fullMant & ((uint32(1) << shift) - 1)
		if rem > half || (rem == half && (hMant&1) != 0) {
			hMant++
		}
		return sign | hMant
	}
	if exp == 102 && mant != 0 {
		return sign | 1
	}
	return sign
}

func f16ToF32(h uint32) float32 {
	sign := (h & 0x8000) << 16
	exp := (h >> 10) & 0x1F
	mant := h & 0x3FF
	if exp == 0 {
		if mant == 0 {
			return math.Float32frombits(sign)
		}
		f := float32(mant) * 5.960464477539063e-08
		if sign != 0 {
			return -f
		}
		return f
	}
	if exp == 31 {
		if mant == 0 {
			return math.Float32frombits(sign | 0x7F800000)
		}
		return math.Float32frombits(sign | 0x7FC00000 | (mant << 13))
	}
	return math.Float32frombits(sign | ((exp + 112) << 23) | (mant << 13))
}

func evalPack(name string, comps []constant.Value, elem kind) uint32 {
	fAt := func(i int) float64 {
		v, _ := constant.Float64Val(constant.ToFloat(comps[i]))
		return v
	}
	clampF := func(v, lo, hi float64) float64 {
		return math.Max(lo, math.Min(hi, v))
	}
	switch name {
	case "pack4x8snorm":
		var r uint32
		for i := range 4 {
			q := int32(math.RoundToEven(clampF(fAt(i), -1, 1) * 127))
			r |= (uint32(q) & 0xFF) << (8 * uint(i))
		}
		return r
	case "pack4x8unorm":
		var r uint32
		for i := range 4 {
			q := uint32(math.RoundToEven(clampF(fAt(i), 0, 1) * 255))
			r |= (q & 0xFF) << (8 * uint(i))
		}
		return r
	case "pack2x16snorm":
		var r uint32
		for i := range 2 {
			q := int32(math.RoundToEven(clampF(fAt(i), -1, 1) * 32767))
			r |= (uint32(q) & 0xFFFF) << (16 * uint(i))
		}
		return r
	case "pack2x16unorm":
		var r uint32
		for i := range 2 {
			q := uint32(math.RoundToEven(clampF(fAt(i), 0, 1) * 65535))
			r |= (q & 0xFFFF) << (16 * uint(i))
		}
		return r
	case "pack2x16float":
		return f32ToF16(float32(fAt(0))) | (f32ToF16(float32(fAt(1))) << 16)
	case "pack4xI8", "pack4xU8":
		var r uint32
		for i := range 4 {
			r |= (constUint32(comps[i], elem) & 0xFF) << (8 * uint(i))
		}
		return r
	case "pack4xI8Clamp":
		var r uint32
		for i := range 4 {
			v := int32(constUint32(comps[i], kI32))
			v = max(-128, min(127, v))
			r |= (uint32(v) & 0xFF) << (8 * uint(i))
		}
		return r
	case "pack4xU8Clamp":
		var r uint32
		for i := range 4 {
			v := min(uint32(255), constUint32(comps[i], kU32))
			r |= (v & 0xFF) << (8 * uint(i))
		}
		return r
	}
	return 0
}

func packFunc(name string, argTy ty, helper string) builtinFunc {
	return func(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
		arity(args, 1, pos, name)
		v := c.convert(args[0], argTy, pos)
		if v.comps != nil {
			r := evalPack(name, v.comps, argTy.elem)
			return value{t: tU32, c: constant.MakeUint64(uint64(r))}
		}
		return c.callHelper(helper, pos, c.ir(v))
	}
}

func evalUnpack(name string, e uint32) []constant.Value {
	sign8 := func(b uint32) int32 { return int32(((b & 0xFF) ^ 0x80) - 0x80) }
	sign16 := func(h uint32) int32 { return int32(((h & 0xFFFF) ^ 0x8000) - 0x8000) }
	switch name {
	case "unpack4x8snorm":
		out := make([]constant.Value, 4)
		for i := range 4 {
			f := math.Max(float64(sign8(e>>(8*uint(i))))/127.0, -1.0)
			out[i] = constant.MakeFloat64(f)
		}
		return out
	case "unpack4x8unorm":
		out := make([]constant.Value, 4)
		for i := range 4 {
			out[i] = constant.MakeFloat64(float64((e>>(8*uint(i)))&0xFF) / 255.0)
		}
		return out
	case "unpack2x16snorm":
		return []constant.Value{
			constant.MakeFloat64(math.Max(float64(sign16(e))/32767.0, -1.0)),
			constant.MakeFloat64(math.Max(float64(sign16(e>>16))/32767.0, -1.0)),
		}
	case "unpack2x16unorm":
		return []constant.Value{
			constant.MakeFloat64(float64(e&0xFFFF) / 65535.0),
			constant.MakeFloat64(float64((e>>16)&0xFFFF) / 65535.0),
		}
	case "unpack2x16float":
		return []constant.Value{
			constant.MakeFloat64(float64(f16ToF32(e & 0xFFFF))),
			constant.MakeFloat64(float64(f16ToF32((e >> 16) & 0xFFFF))),
		}
	case "unpack4xI8":
		out := make([]constant.Value, 4)
		for i := range 4 {
			out[i] = constant.MakeInt64(int64(sign8(e >> (8 * uint(i)))))
		}
		return out
	case "unpack4xU8":
		out := make([]constant.Value, 4)
		for i := range 4 {
			out[i] = constant.MakeUint64(uint64((e >> (8 * uint(i))) & 0xFF))
		}
		return out
	}
	return nil
}

func unpackFunc(name string, retTy ty, helper string) builtinFunc {
	return func(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
		arity(args, 1, pos, name)
		v := c.convert(args[0], tU32, pos)
		if v.c != nil {
			comps := evalUnpack(name, constUint32(v.c, kU32))
			return c.vectorOfConstants(retTy, comps, pos)
		}
		return c.callHelper(helper, pos, c.ir(v))
	}
}

func (c *checker) materializeConst(v value, out *[]shaderir.Stmt) value {
	if v.fields == nil {
		return v
	}
	fields := make([]value, len(v.fields))
	for i, f := range v.fields {
		fields[i] = c.materializeConst(f, out)
	}
	return c.foldStruct(v.t, fields, out)
}

func (c *checker) foldStruct(resTy ty, fields []value, out *[]shaderir.Stmt) value {
	if c.cur == nil {
		return value{t: resTy, fields: fields}
	}
	idx := c.newLocal(resTy)
	s := local(idx, resTy)
	for i, v := range fields {
		field := mk(shaderir.Expr{Type: shaderir.FieldSelector, Exprs: []shaderir.Expr{s, {Type: shaderir.StructMember, Index: i}}}, resTy.st.fields[i].t)
		*out = append(*out, assign(field, c.ir(v)))
	}
	return value{t: resTy, e: s, fields: fields}
}

func modfFunc(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
	arity(args, 1, pos, "modf")
	t := c.common(args, pos, true)
	if t.scalar() != kF32 || (t.k != kF32 && t.k != kVec) {
		fail(pos, "modf takes f32 or vectors of it, not a %s", t)
	}
	v := args[0]
	suffix := "f32"
	if t.k == kVec {
		suffix = [...]string{2: "vec2_f32", 3: "vec3_f32", 4: "vec4_f32"}[t.n]
	}
	resTy := c.global.names["__modf_result_"+suffix].t
	if v.c != nil {
		f, _ := constant.Float64Val(constant.ToFloat(v.c))
		w := math.Trunc(f)
		return c.foldStruct(resTy, []value{
			{t: tF32, c: constant.MakeFloat64(f - w)},
			{t: tF32, c: constant.MakeFloat64(w)},
		}, out)
	}
	if v.comps != nil {
		fr := make([]constant.Value, len(v.comps))
		wh := make([]constant.Value, len(v.comps))
		for i, comp := range v.comps {
			f, _ := constant.Float64Val(constant.ToFloat(comp))
			w := math.Trunc(f)
			fr[i] = constant.MakeFloat64(f - w)
			wh[i] = constant.MakeFloat64(w)
		}
		return c.foldStruct(resTy, []value{
			c.vectorOfConstants(t, fr, pos),
			c.vectorOfConstants(t, wh, pos),
		}, out)
	}
	return c.callHelper("__wgsl_modf_"+suffix, pos, c.ir(v))
}

func frexpFunc(c *checker, args []value, pos int, out *[]shaderir.Stmt) value {
	arity(args, 1, pos, "frexp")
	t := c.common(args, pos, true)
	if t.scalar() != kF32 || (t.k != kF32 && t.k != kVec) {
		fail(pos, "frexp takes f32 or vectors of it, not a %s", t)
	}
	v := args[0]
	suffix := "f32"
	expTy := tI32
	if t.k == kVec {
		suffix = [...]string{2: "vec2_f32", 3: "vec3_f32", 4: "vec4_f32"}[t.n]
		expTy = vec(kI32, t.n)
	}
	resTy := c.global.names["__frexp_result_"+suffix].t
	if v.c != nil {
		f, _ := constant.Float64Val(constant.ToFloat(v.c))
		fr, exp := math.Frexp(float64(float32(f)))
		return c.foldStruct(resTy, []value{
			{t: tF32, c: constant.MakeFloat64(fr)},
			{t: tI32, c: constant.MakeInt64(int64(exp))},
		}, out)
	}
	if v.comps != nil {
		fr := make([]constant.Value, len(v.comps))
		ex := make([]constant.Value, len(v.comps))
		for i, comp := range v.comps {
			f, _ := constant.Float64Val(constant.ToFloat(comp))
			frac, exp := math.Frexp(float64(float32(f)))
			fr[i] = constant.MakeFloat64(frac)
			ex[i] = constant.MakeInt64(int64(exp))
		}
		return c.foldStruct(resTy, []value{
			c.vectorOfConstants(t, fr, pos),
			c.vectorOfConstants(expTy, ex, pos),
		}, out)
	}
	return c.callHelper("__wgsl_frexp_"+suffix, pos, c.ir(v))
}
