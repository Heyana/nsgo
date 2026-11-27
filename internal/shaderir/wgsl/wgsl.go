// Package wgsl writes validated WGSL source from a shaderir.Program for the
// WebGPU backend (docs/architecture/gpu.md).
package wgsl

import (
	"fmt"
	"go/constant"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// CompilePipeline emits canonical WGSL source for a 3D GPU pipeline program.
func CompilePipeline(p *shaderir.Program) (string, error) {
	if p == nil {
		return "", fmt.Errorf("shaderir/wgsl: nil program")
	}
	w := &writer{
		p:           p,
		structNames: map[string]string{},
	}
	for _, b := range p.Bindings {
		w.registerStructs(b.Type)
	}

	var body []string
	for _, f := range p.Funcs {
		body = append(body, w.emitHelper(f)...)
	}
	if p.VertexFunc.Block != nil {
		body = append(body, w.emitVertex()...)
	}
	if p.FragmentFunc.Block != nil {
		body = append(body, w.emitFragment()...)
	}
	if p.ComputeFunc.Block != nil {
		body = append(body, w.emitCompute()...)
	}

	var out []string
	for i, st := range w.structTypes {
		out = append(out, fmt.Sprintf("struct S%d {", i))
		for j, m := range st.Sub {
			out = append(out, fmt.Sprintf("\tM%d: %s,", j, w.typeStr(m)))
		}
		out = append(out, "};", "")
	}
	for i, b := range p.Bindings {
		switch b.Kind {
		case shaderir.BindingUniformBuffer:
			out = append(out, fmt.Sprintf("@group(%d) @binding(%d) var<uniform> B%d: %s;", b.Group, b.Binding, i, w.typeStr(b.Type)))
		case shaderir.BindingStorageBufferRead:
			out = append(out, fmt.Sprintf("@group(%d) @binding(%d) var<storage, read> B%d: %s;", b.Group, b.Binding, i, w.typeStr(b.Type)))
		case shaderir.BindingStorageBufferReadWrite:
			out = append(out, fmt.Sprintf("@group(%d) @binding(%d) var<storage, read_write> B%d: %s;", b.Group, b.Binding, i, w.typeStr(b.Type)))
		case shaderir.BindingStorageTexture:
			out = append(out, fmt.Sprintf("@group(%d) @binding(%d) var B%d: texture_storage_2d<%s, %s>;", b.Group, b.Binding, i, b.StorageFormat.WGSLName(), b.StorageAccess.WGSLName()))
		case shaderir.BindingTexture, shaderir.BindingSampler:
			out = append(out, fmt.Sprintf("@group(%d) @binding(%d) var B%d: %s;", b.Group, b.Binding, i, w.typeStr(b.Type)))
		}
	}
	out = append(out, "")
	out = append(out, body...)
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n", nil
}

type writer struct {
	p           *shaderir.Program
	structNames map[string]string
	structTypes []shaderir.Type
}

func (w *writer) registerStructs(t shaderir.Type) {
	switch t.Main {
	case shaderir.Struct:
		w.structName(t)
	case shaderir.Array:
		if len(t.Sub) > 0 {
			w.registerStructs(t.Sub[0])
		}
	}
}

func (w *writer) structName(t shaderir.Type) string {
	key := t.String()
	if n, ok := w.structNames[key]; ok {
		return n
	}
	for _, f := range t.Sub {
		w.registerStructs(f)
	}
	n := fmt.Sprintf("S%d", len(w.structTypes))
	w.structNames[key] = n
	w.structTypes = append(w.structTypes, t)
	return n
}

func (w *writer) typeStr(t shaderir.Type) string {
	switch t.Main {
	case shaderir.Bool:
		return "bool"
	case shaderir.Int:
		return "i32"
	case shaderir.Uint:
		return "u32"
	case shaderir.Float:
		return "f32"
	case shaderir.Vec2:
		return "vec2<f32>"
	case shaderir.Vec3:
		return "vec3<f32>"
	case shaderir.Vec4:
		return "vec4<f32>"
	case shaderir.IVec2:
		return "vec2<i32>"
	case shaderir.IVec3:
		return "vec3<i32>"
	case shaderir.IVec4:
		return "vec4<i32>"
	case shaderir.UVec2:
		return "vec2<u32>"
	case shaderir.UVec3:
		return "vec3<u32>"
	case shaderir.UVec4:
		return "vec4<u32>"
	case shaderir.Mat2:
		return "mat2x2<f32>"
	case shaderir.Mat2x3:
		return "mat2x3<f32>"
	case shaderir.Mat2x4:
		return "mat2x4<f32>"
	case shaderir.Mat3x2:
		return "mat3x2<f32>"
	case shaderir.Mat3:
		return "mat3x3<f32>"
	case shaderir.Mat3x4:
		return "mat3x4<f32>"
	case shaderir.Mat4x2:
		return "mat4x2<f32>"
	case shaderir.Mat4x3:
		return "mat4x3<f32>"
	case shaderir.Mat4:
		return "mat4x4<f32>"
	case shaderir.Array:
		if t.Length == 0 {
			return fmt.Sprintf("array<%s>", w.typeStr(t.Sub[0]))
		}
		return fmt.Sprintf("array<%s, %d>", w.typeStr(t.Sub[0]), t.Length)
	case shaderir.Struct:
		return w.structName(t)
	case shaderir.Texture2D, shaderir.Texture:
		return "texture_2d<f32>"
	case shaderir.Texture3D:
		return "texture_3d<f32>"
	case shaderir.TextureCube:
		return "texture_cube<f32>"
	case shaderir.Texture2DArray:
		return "texture_2d_array<f32>"
	case shaderir.TextureDepth2D:
		return "texture_depth_2d"
	case shaderir.TextureMultisampled2D:
		return "texture_multisampled_2d<f32>"
	case shaderir.Sampler:
		return "sampler"
	case shaderir.SamplerComparison:
		return "sampler_comparison"
	default:
		return "f32"
	}
}

func (w *writer) emitHelper(f shaderir.Func) []string {
	var params []string
	for i, pt := range f.InParams {
		params = append(params, fmt.Sprintf("l%d: %s", i, w.typeStr(pt)))
	}
	ret := ""
	if f.Return.Main != shaderir.None {
		ret = " -> " + w.typeStr(f.Return)
	}
	lines := []string{fmt.Sprintf("fn F%d(%s)%s {", f.Index, strings.Join(params, ", "), ret)}
	lines = append(lines, w.emitBlock(f.Block, 1, "helper")...)
	lines = append(lines, "}", "")
	return lines
}

func (w *writer) emitVertex() []string {
	entry := w.p.VertexEntry
	if entry == "" {
		entry = "vs_main"
	}
	lines := []string{"struct StageOut {", "\t@builtin(position) position: vec4<f32>,"}
	for i, v := range w.p.Varyings {
		loc := uint32(i)
		interp := ""
		if i < len(w.p.VaryingMeta) {
			loc = w.p.VaryingMeta[i].Location
			if w.p.VaryingMeta[i].Interpolation == shaderir.InterpFlat {
				interp = " @interpolate(flat)"
			}
		}
		lines = append(lines, fmt.Sprintf("\t@location(%d)%s v%d: %s,", loc, interp, i, w.typeStr(v)))
	}
	lines = append(lines, "};", "")

	var params []string
	for i, a := range w.p.Attributes {
		loc := uint32(i)
		if i < len(w.p.AttributeMeta) {
			loc = w.p.AttributeMeta[i].Location
		}
		params = append(params, fmt.Sprintf("@location(%d) a%d: %s", loc, i, w.typeStr(a)))
	}
	params = append(params, "@builtin(vertex_index) nsgo_vertex_id: u32", "@builtin(instance_index) nsgo_instance_id: u32")
	lines = append(lines, fmt.Sprintf("@vertex fn %s(%s) -> StageOut {", entry, strings.Join(params, ", ")))
	lines = append(lines, "\tvar nsgo_out: StageOut;")
	lines = append(lines, w.emitBlock(w.p.VertexFunc.Block, 1, "vertex")...)
	lines = append(lines, "\treturn nsgo_out;", "}", "")
	return lines
}

func (w *writer) emitFragment() []string {
	entry := w.p.FragmentEntry
	if entry == "" {
		entry = "fs_main"
	}
	var lines []string
	if w.p.VertexFunc.Block == nil {
		lines = append(lines, "struct StageOut {", "\t@builtin(position) position: vec4<f32>,")
		for i, v := range w.p.Varyings {
			loc := uint32(i)
			interp := ""
			if i < len(w.p.VaryingMeta) {
				loc = w.p.VaryingMeta[i].Location
				if w.p.VaryingMeta[i].Interpolation == shaderir.InterpFlat {
					interp = " @interpolate(flat)"
				}
			}
			lines = append(lines, fmt.Sprintf("\t@location(%d)%s v%d: %s,", loc, interp, i, w.typeStr(v)))
		}
		lines = append(lines, "};", "")
	}
	if len(w.p.FragmentOutputs) > 0 {
		lines = append(lines, "struct FragOut {")
		for i, ot := range w.p.FragmentOutputs {
			loc := uint32(i)
			if i < len(w.p.FragmentOutputMeta) {
				loc = w.p.FragmentOutputMeta[i].Location
			}
			lines = append(lines, fmt.Sprintf("\t@location(%d) c%d: %s,", loc, i, w.typeStr(ot)))
		}
		lines = append(lines, "};", "")
		lines = append(lines, fmt.Sprintf("@fragment fn %s(nsgo_in: StageOut, @builtin(front_facing) nsgo_front_facing: bool) -> FragOut {", entry))
		lines = append(lines, "\tvar nsgo_fout: FragOut;")
		lines = append(lines, w.emitBlock(w.p.FragmentFunc.Block, 1, "fragment")...)
		lines = append(lines, "\treturn nsgo_fout;", "}", "")
		return lines
	}
	lines = append(lines, fmt.Sprintf("@fragment fn %s(nsgo_in: StageOut, @builtin(front_facing) nsgo_front_facing: bool) -> @location(0) vec4<f32> {", entry))
	lines = append(lines, w.emitBlock(w.p.FragmentFunc.Block, 1, "fragment")...)
	lines = append(lines, "}", "")
	return lines
}

func (w *writer) emitCompute() []string {
	entry := w.p.ComputeEntry
	if entry == "" {
		entry = "cs_main"
	}
	ws := w.p.ComputeFunc.WorkgroupSize
	if ws[0] == 0 {
		ws[0] = 1
	}
	if ws[1] == 0 {
		ws[1] = 1
	}
	if ws[2] == 0 {
		ws[2] = 1
	}
	params := []string{
		"@builtin(global_invocation_id) nsgo_global_invocation_id: vec3<u32>",
		"@builtin(local_invocation_id) nsgo_local_invocation_id: vec3<u32>",
		"@builtin(workgroup_id) nsgo_workgroup_id: vec3<u32>",
		"@builtin(num_workgroups) nsgo_num_workgroups: vec3<u32>",
		"@builtin(local_invocation_index) nsgo_local_invocation_index: u32",
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("@compute @workgroup_size(%d, %d, %d)", ws[0], ws[1], ws[2]))
	lines = append(lines, fmt.Sprintf("fn %s(%s) {", entry, strings.Join(params, ", ")))
	lines = append(lines, w.emitBlock(w.p.ComputeFunc.Block, 1, "compute")...)
	lines = append(lines, "}", "")
	return lines
}

func (w *writer) localName(stage string, idx int) string {
	switch stage {
	case "vertex":
		na := len(w.p.Attributes)
		nv := len(w.p.Varyings)
		switch {
		case idx < na:
			return fmt.Sprintf("a%d", idx)
		case idx == na:
			return "nsgo_out.position"
		case idx < na+nv+1:
			return fmt.Sprintf("nsgo_out.v%d", idx-na-1)
		}
	case "fragment":
		nv := len(w.p.Varyings)
		no := len(w.p.FragmentOutputs)
		switch {
		case idx == 0:
			return "nsgo_in.position"
		case idx < nv+1:
			return fmt.Sprintf("nsgo_in.v%d", idx-1)
		case idx < nv+1+no:
			return fmt.Sprintf("nsgo_fout.c%d", idx-nv-1)
		}
	}
	return fmt.Sprintf("l%d", idx)
}

func (w *writer) emitBlock(b *shaderir.Block, level int, stage string) []string {
	if b == nil {
		return nil
	}
	indent := strings.Repeat("\t", level)
	var lines []string
	for i, lt := range b.LocalVars {
		if lt.Main != shaderir.None {
			lines = append(lines, fmt.Sprintf("%svar l%d: %s;", indent, b.LocalVarIndexOffset+i, w.typeStr(lt)))
		}
	}
	for _, s := range b.Stmts {
		switch s.Type {
		case shaderir.ExprStmt:
			lines = append(lines, fmt.Sprintf("%s%s;", indent, w.emitExpr(s.Exprs[0], stage)))
		case shaderir.BlockStmt:
			lines = append(lines, indent+"{")
			lines = append(lines, w.emitBlock(s.Blocks[0], level+1, stage)...)
			lines = append(lines, indent+"}")
		case shaderir.Assign:
			lines = append(lines, fmt.Sprintf("%s%s = %s;", indent, w.emitExpr(s.Exprs[0], stage), w.emitExpr(s.Exprs[1], stage)))
		case shaderir.Init:
			lines = append(lines, fmt.Sprintf("%s// init %s", indent, w.localName(stage, s.InitIndex)))
		case shaderir.If:
			lines = append(lines, fmt.Sprintf("%sif (%s) {", indent, w.emitExpr(s.Exprs[0], stage)))
			lines = append(lines, w.emitBlock(s.Blocks[0], level+1, stage)...)
			if len(s.Blocks) > 1 {
				lines = append(lines, indent+"} else {")
				lines = append(lines, w.emitBlock(s.Blocks[1], level+1, stage)...)
			}
			lines = append(lines, indent+"}")
		case shaderir.For:
			v := w.localName(stage, s.ForVarIndex)
			var initStr, endStr string
			if len(s.Exprs) >= 2 {
				initStr = w.emitExpr(s.Exprs[0], stage)
				endStr = w.emitExpr(s.Exprs[1], stage)
			} else {
				initStr = constLit(s.ForInit, s.ForVarType.Main == shaderir.Uint)
				endStr = constLit(s.ForEnd, s.ForVarType.Main == shaderir.Uint)
			}
			deltaStr := constLit(s.ForDelta, s.ForVarType.Main == shaderir.Uint)
			lines = append(lines, fmt.Sprintf("%sfor (var %s: %s = %s; %s %s %s; %s = %s + %s) {",
				indent, v, w.typeStr(s.ForVarType), initStr, v, opStr(s.ForOp), endStr, v, v, deltaStr))
			lines = append(lines, w.emitBlock(s.Blocks[0], level+1, stage)...)
			lines = append(lines, indent+"}")
		case shaderir.Continue:
			lines = append(lines, indent+"continue;")
		case shaderir.Break:
			lines = append(lines, indent+"break;")
		case shaderir.Return:
			if stage == "vertex" {
				lines = append(lines, indent+"return nsgo_out;")
			} else if stage == "fragment" && len(w.p.FragmentOutputs) > 0 {
				lines = append(lines, indent+"return nsgo_fout;")
			} else if len(s.Exprs) == 0 {
				lines = append(lines, indent+"return;")
			} else {
				lines = append(lines, fmt.Sprintf("%sreturn %s;", indent, w.emitExpr(s.Exprs[0], stage)))
			}
		case shaderir.Discard:
			lines = append(lines, indent+"discard;")
		}
	}
	return lines
}

func (w *writer) emitExpr(e shaderir.Expr, stage string) string {
	switch e.Type {
	case shaderir.NumberExpr:
		return constLit(e.Const, e.Unsigned)
	case shaderir.LocalVariable:
		return w.localName(stage, e.Index)
	case shaderir.BindingVariable:
		return fmt.Sprintf("B%d", e.Index)
	case shaderir.BuiltinInput:
		switch shaderir.StageBuiltin(e.Index) {
		case shaderir.BuiltinVertexIndex:
			return "nsgo_vertex_id"
		case shaderir.BuiltinInstanceIndex:
			return "nsgo_instance_id"
		case shaderir.BuiltinFrontFacing:
			return "nsgo_front_facing"
		case shaderir.BuiltinPosition:
			return "nsgo_in.position"
		case shaderir.BuiltinGlobalInvocationID:
			return "nsgo_global_invocation_id"
		case shaderir.BuiltinLocalInvocationID:
			return "nsgo_local_invocation_id"
		case shaderir.BuiltinWorkgroupID:
			return "nsgo_workgroup_id"
		case shaderir.BuiltinNumWorkgroups:
			return "nsgo_num_workgroups"
		case shaderir.BuiltinLocalInvocationIndex:
			return "nsgo_local_invocation_index"
		}
	case shaderir.StructConstruct:
		name := w.typeStr(*e.StructType)
		args := make([]string, len(e.Exprs))
		for i, a := range e.Exprs {
			args[i] = w.emitExpr(a, stage)
		}
		return fmt.Sprintf("%s(%s)", name, strings.Join(args, ", "))
	case shaderir.Unary:
		return fmt.Sprintf("(%s%s)", opStr(e.Op), w.emitExpr(e.Exprs[0], stage))
	case shaderir.Binary:
		return fmt.Sprintf("(%s %s %s)", w.emitExpr(e.Exprs[0], stage), opStr(e.Op), w.emitExpr(e.Exprs[1], stage))
	case shaderir.Selection:
		return fmt.Sprintf("select(%s, %s, %s)", w.emitExpr(e.Exprs[2], stage), w.emitExpr(e.Exprs[1], stage), w.emitExpr(e.Exprs[0], stage))
	case shaderir.FieldSelector:
		base := w.emitExpr(e.Exprs[0], stage)
		switch m := e.Exprs[1]; m.Type {
		case shaderir.SwizzlingExpr:
			return fmt.Sprintf("(%s).%s", base, m.Swizzling)
		case shaderir.StructMember:
			return fmt.Sprintf("(%s).M%d", base, m.Index)
		}
	case shaderir.Index:
		return fmt.Sprintf("(%s)[%s]", w.emitExpr(e.Exprs[0], stage), w.emitExpr(e.Exprs[1], stage))
	case shaderir.Call:
		callee := e.Exprs[0]
		args := make([]string, len(e.Exprs)-1)
		for i, a := range e.Exprs[1:] {
			args[i] = w.emitExpr(a, stage)
		}
		if callee.Type == shaderir.FunctionExpr {
			return fmt.Sprintf("F%d(%s)", callee.Index, strings.Join(args, ", "))
		}
		if callee.BuiltinFunc == shaderir.ArrayLength && len(args) == 1 {
			return fmt.Sprintf("arrayLength(&%s)", args[0])
		}
		return fmt.Sprintf("%s(%s)", builtinName(callee.BuiltinFunc), strings.Join(args, ", "))
	}
	return "0"
}

func opStr(op shaderir.Op) string {
	switch op {
	case shaderir.Add:
		return "+"
	case shaderir.Sub:
		return "-"
	case shaderir.NotOp:
		return "!"
	case shaderir.ComplementOp:
		return "~"
	case shaderir.ComponentWiseMul, shaderir.MatrixMul:
		return "*"
	case shaderir.Div:
		return "/"
	case shaderir.ModOp:
		return "%"
	case shaderir.LeftShift:
		return "<<"
	case shaderir.RightShift:
		return ">>"
	case shaderir.LessThanOp:
		return "<"
	case shaderir.LessThanEqualOp:
		return "<="
	case shaderir.GreaterThanOp:
		return ">"
	case shaderir.GreaterThanEqualOp:
		return ">="
	case shaderir.EqualOp, shaderir.VectorEqualOp:
		return "=="
	case shaderir.NotEqualOp, shaderir.VectorNotEqualOp:
		return "!="
	case shaderir.And:
		return "&"
	case shaderir.Xor:
		return "^"
	case shaderir.Or:
		return "|"
	case shaderir.AndAnd:
		return "&&"
	case shaderir.OrOr:
		return "||"
	}
	return "+"
}

func builtinName(f shaderir.BuiltinFunc) string {
	switch f {
	case shaderir.BoolF:
		return "bool"
	case shaderir.IntF:
		return "i32"
	case shaderir.UintF:
		return "u32"
	case shaderir.FloatF:
		return "f32"
	case shaderir.Vec2F:
		return "vec2<f32>"
	case shaderir.Vec3F:
		return "vec3<f32>"
	case shaderir.Vec4F:
		return "vec4<f32>"
	case shaderir.IVec2F:
		return "vec2<i32>"
	case shaderir.IVec3F:
		return "vec3<i32>"
	case shaderir.IVec4F:
		return "vec4<i32>"
	case shaderir.UVec2F:
		return "vec2<u32>"
	case shaderir.UVec3F:
		return "vec3<u32>"
	case shaderir.UVec4F:
		return "vec4<u32>"
	case shaderir.Mat2F:
		return "mat2x2<f32>"
	case shaderir.Mat3F:
		return "mat3x3<f32>"
	case shaderir.Mat4F:
		return "mat4x4<f32>"
	case shaderir.Inversesqrt:
		return "inverseSqrt"
	case shaderir.Dfdx:
		return "dpdx"
	case shaderir.Dfdy:
		return "dpdy"
	case shaderir.RoundEven:
		return "round"
	default:
		return string(f)
	}
}

func constLit(c constant.Value, unsigned bool) string {
	switch c.Kind() {
	case constant.Bool:
		if constant.BoolVal(c) {
			return "true"
		}
		return "false"
	case constant.Int:
		if unsigned {
			v, _ := constant.Uint64Val(c)
			return fmt.Sprintf("%du", v)
		}
		v, _ := constant.Int64Val(c)
		return fmt.Sprintf("%di", v)
	case constant.Float:
		f, _ := constant.Float64Val(c)
		s := fmt.Sprintf("%g", f)
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		return s + "f"
	}
	return "0"
}
