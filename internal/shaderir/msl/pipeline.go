package msl

import (
	"fmt"
	"go/constant"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// BindingSlot records the Metal buffer, texture, or sampler index assigned
// to a WGSL @group(G) @binding(B) resource.
type BindingSlot struct {
	Group   uint32
	Binding uint32
	Kind    shaderir.BindingKind
	Slot    uint32
}

// PipelineSource holds Metal shader source and resource slot mappings
// compiled from a GPU pipeline shaderir.Program.
type PipelineSource struct {
	Source        string
	VertexEntry   string
	FragmentEntry string
	ComputeEntry  string
	WorkgroupSize [3]uint32
	Slots         []BindingSlot
}

// CompilePipeline emits Metal Shading Language for a 3D GPU pipeline program with @vertex, @fragment, and/or @compute entry points and explicit @group/@binding resources.
func CompilePipeline(p *shaderir.Program) (PipelineSource, error) {
	if p == nil {
		return PipelineSource{}, fmt.Errorf("msl: nil program")
	}
	w := &pipeWriter{
		p:           p,
		structNames: map[string]string{},
		bindingSlot: make([]uint32, len(p.Bindings)),
	}
	var bufSlot, texSlot, sampSlot uint32
	// Reserve low buffer slots for vertex buffers (0..7) in render pipelines; compute-only pipelines start buffer slots at 0.
	if p.VertexFunc.Block != nil || p.FragmentFunc.Block != nil {
		bufSlot = 8
	}
	for i, b := range p.Bindings {
		w.registerStructs(b.Type)
		switch b.Kind {
		case shaderir.BindingUniformBuffer, shaderir.BindingStorageBufferRead, shaderir.BindingStorageBufferReadWrite:
			w.bindingSlot[i] = bufSlot
			w.slots = append(w.slots, BindingSlot{Group: b.Group, Binding: b.Binding, Kind: b.Kind, Slot: bufSlot})
			bufSlot++
		case shaderir.BindingTexture, shaderir.BindingStorageTexture:
			w.bindingSlot[i] = texSlot
			w.slots = append(w.slots, BindingSlot{Group: b.Group, Binding: b.Binding, Kind: b.Kind, Slot: texSlot})
			texSlot++
		case shaderir.BindingSampler:
			w.bindingSlot[i] = sampSlot
			w.slots = append(w.slots, BindingSlot{Group: b.Group, Binding: b.Binding, Kind: b.Kind, Slot: sampSlot})
			sampSlot++
		}
	}

	var body []string
	// Helper functions
	for _, f := range p.Funcs {
		body = append(body, w.emitHelper(f)...)
	}

	vEntry := p.VertexEntry
	if vEntry == "" && p.VertexFunc.Block != nil {
		vEntry = "vs_main"
	}
	fEntry := p.FragmentEntry
	if fEntry == "" && p.FragmentFunc.Block != nil {
		fEntry = "fs_main"
	}
	cEntry := p.ComputeEntry
	if cEntry == "" && p.ComputeFunc.Block != nil {
		cEntry = "cs_main"
	}

	if p.VertexFunc.Block != nil {
		body = append(body, w.emitVertex(vEntry)...)
	}
	if p.FragmentFunc.Block != nil {
		body = append(body, w.emitFragment(fEntry)...)
	}
	if p.ComputeFunc.Block != nil {
		body = append(body, w.emitCompute(cEntry)...)
	}

	var out []string
	out = append(out, Prelude(), "")
	for i, st := range w.structTypes {
		out = append(out, fmt.Sprintf("struct S%d {", i))
		for j, m := range st.Sub {
			out = append(out, fmt.Sprintf("\t%s;", w.declVar(m, fmt.Sprintf("M%d", j))))
		}
		out = append(out, "};", "")
	}
	out = append(out, body...)

	return PipelineSource{
		Source:        strings.TrimSpace(strings.Join(out, "\n")) + "\n",
		VertexEntry:   vEntry,
		FragmentEntry: fEntry,
		ComputeEntry:  cEntry,
		WorkgroupSize: p.ComputeFunc.WorkgroupSize,
		Slots:         w.slots,
	}, nil
}

type pipeWriter struct {
	p           *shaderir.Program
	structNames map[string]string
	structTypes []shaderir.Type
	bindingSlot []uint32
	slots       []BindingSlot
}

func (w *pipeWriter) registerStructs(t shaderir.Type) {
	switch t.Main {
	case shaderir.Struct:
		w.structName(t)
	case shaderir.Array:
		if len(t.Sub) > 0 {
			w.registerStructs(t.Sub[0])
		}
	}
}

func (w *pipeWriter) structName(t shaderir.Type) string {
	key := t.String()
	if n, ok := w.structNames[key]; ok {
		return n
	}
	// Ensure nested structs are registered first.
	for _, f := range t.Sub {
		w.registerStructs(f)
	}
	n := fmt.Sprintf("S%d", len(w.structTypes))
	w.structNames[key] = n
	w.structTypes = append(w.structTypes, t)
	return n
}

func (w *pipeWriter) typeStr(t shaderir.Type) string {
	switch t.Main {
	case shaderir.Struct:
		return w.structName(t)
	case shaderir.Array:
		if t.Length == 0 {
			return fmt.Sprintf("%s*", w.typeStr(t.Sub[0]))
		}
		return fmt.Sprintf("array<%s, %d>", w.typeStr(t.Sub[0]), t.Length)
	case shaderir.Texture2D, shaderir.Texture:
		return "texture2d<float>"
	case shaderir.TextureCube:
		return "texturecube<float>"
	case shaderir.Texture2DArray:
		return "texture2d_array<float>"
	case shaderir.TextureDepth2D:
		return "depth2d<float>"
	case shaderir.TextureMultisampled2D:
		return "texture2d_ms<float>"
	case shaderir.TextureStorage2D:
		return "texture2d<float, access::write>"
	case shaderir.Sampler, shaderir.SamplerComparison:
		return "sampler"
	default:
		return basicTypeString(t.Main)
	}
}

func (w *pipeWriter) declVar(t shaderir.Type, name string) string {
	if t.Main == shaderir.Array && t.Length == 0 {
		return fmt.Sprintf("%s %s[1]", w.typeStr(t.Sub[0]), name)
	}
	return fmt.Sprintf("%s %s", w.typeStr(t), name)
}

func mslStorageAccess(a shaderir.StorageAccess) string {
	switch a {
	case shaderir.StorageAccessRead:
		return "access::read"
	case shaderir.StorageAccessReadWrite:
		return "access::read_write"
	default:
		return "access::write"
	}
}

func (w *pipeWriter) bindingParam(i int, b shaderir.Binding) string {
	slot := w.bindingSlot[i]
	switch b.Kind {
	case shaderir.BindingUniformBuffer:
		return fmt.Sprintf("constant %s& B%d [[buffer(%d)]]", w.typeStr(b.Type), i, slot)
	case shaderir.BindingStorageBufferRead:
		if b.Type.Main == shaderir.Array && b.Type.Length == 0 {
			return fmt.Sprintf("device const %s* B%d [[buffer(%d)]]", w.typeStr(b.Type.Sub[0]), i, slot)
		}
		return fmt.Sprintf("device const %s& B%d [[buffer(%d)]]", w.typeStr(b.Type), i, slot)
	case shaderir.BindingStorageBufferReadWrite:
		if b.Type.Main == shaderir.Array && b.Type.Length == 0 {
			return fmt.Sprintf("device %s* B%d [[buffer(%d)]]", w.typeStr(b.Type.Sub[0]), i, slot)
		}
		return fmt.Sprintf("device %s& B%d [[buffer(%d)]]", w.typeStr(b.Type), i, slot)
	case shaderir.BindingTexture:
		return fmt.Sprintf("%s B%d [[texture(%d)]]", w.typeStr(b.Type), i, slot)
	case shaderir.BindingStorageTexture:
		return fmt.Sprintf("texture2d<float, %s> B%d [[texture(%d)]]", mslStorageAccess(b.StorageAccess), i, slot)
	case shaderir.BindingSampler:
		return fmt.Sprintf("sampler B%d [[sampler(%d)]]", i, slot)
	default:
		return ""
	}
}

func (w *pipeWriter) helperBindingParams() ([]string, []string) {
	var params, args []string
	for i, b := range w.p.Bindings {
		switch b.Kind {
		case shaderir.BindingUniformBuffer:
			params = append(params, fmt.Sprintf("constant %s& B%d", w.typeStr(b.Type), i))
		case shaderir.BindingStorageBufferRead:
			if b.Type.Main == shaderir.Array && b.Type.Length == 0 {
				params = append(params, fmt.Sprintf("device const %s* B%d", w.typeStr(b.Type.Sub[0]), i))
			} else {
				params = append(params, fmt.Sprintf("device const %s& B%d", w.typeStr(b.Type), i))
			}
		case shaderir.BindingStorageBufferReadWrite:
			if b.Type.Main == shaderir.Array && b.Type.Length == 0 {
				params = append(params, fmt.Sprintf("device %s* B%d", w.typeStr(b.Type.Sub[0]), i))
			} else {
				params = append(params, fmt.Sprintf("device %s& B%d", w.typeStr(b.Type), i))
			}
		case shaderir.BindingTexture:
			params = append(params, fmt.Sprintf("%s B%d", w.typeStr(b.Type), i))
		case shaderir.BindingStorageTexture:
			params = append(params, fmt.Sprintf("texture2d<float, %s> B%d", mslStorageAccess(b.StorageAccess), i))
		case shaderir.BindingSampler:
			params = append(params, fmt.Sprintf("sampler B%d", i))
		}
		args = append(args, fmt.Sprintf("B%d", i))
	}
	return params, args
}

func (w *pipeWriter) emitHelper(f shaderir.Func) []string {
	bParams, _ := w.helperBindingParams()
	params := append([]string{}, bParams...)
	for i, pt := range f.InParams {
		params = append(params, w.declVar(pt, fmt.Sprintf("l%d", i)))
	}
	ret := "void"
	if f.Return.Main != shaderir.None {
		ret = w.typeStr(f.Return)
	}
	lines := []string{fmt.Sprintf("%s F%d(%s) {", ret, f.Index, strings.Join(params, ", "))}
	lines = append(lines, w.emitBlock(f.Block, f.Block, 1, "helper")...)
	lines = append(lines, "}", "")
	return lines
}

func (w *pipeWriter) emitVertex(entry string) []string {
	var lines []string
	if len(w.p.Attributes) > 0 {
		lines = append(lines, "struct VertexIn {")
		for i, a := range w.p.Attributes {
			loc := uint32(i)
			if i < len(w.p.AttributeMeta) {
				loc = w.p.AttributeMeta[i].Location
			}
			lines = append(lines, fmt.Sprintf("\t%s [[attribute(%d)]];", w.declVar(a, fmt.Sprintf("a%d", i)), loc))
		}
		lines = append(lines, "};", "")
	}

	lines = append(lines, "struct StageOut {")
	lines = append(lines, "\tfloat4 position [[position]];")
	for i, v := range w.p.Varyings {
		loc := uint32(i)
		interp := ""
		if i < len(w.p.VaryingMeta) {
			loc = w.p.VaryingMeta[i].Location
			if w.p.VaryingMeta[i].Interpolation == shaderir.InterpFlat {
				interp = " [[flat]]"
			}
		}
		lines = append(lines, fmt.Sprintf("\t%s [[user(locn%d)]]%s;", w.declVar(v, fmt.Sprintf("v%d", i)), loc, interp))
	}
	lines = append(lines, "};", "")

	var params []string
	if len(w.p.Attributes) > 0 {
		params = append(params, "VertexIn __in [[stage_in]]")
	}
	params = append(params, "uint __vertex_id [[vertex_id]]", "uint __instance_id [[instance_id]]")
	for i, b := range w.p.Bindings {
		params = append(params, w.bindingParam(i, b))
	}
	lines = append(lines, fmt.Sprintf("vertex StageOut %s(%s) {", entry, strings.Join(params, ", ")))
	lines = append(lines, "\tStageOut __out = {};")
	lines = append(lines, w.emitBlock(w.p.VertexFunc.Block, w.p.VertexFunc.Block, 1, "vertex")...)
	lines = append(lines, "\treturn __out;", "}", "")
	return lines
}

func (w *pipeWriter) emitFragment(entry string) []string {
	var lines []string
	if w.p.VertexFunc.Block == nil {
		lines = append(lines, "struct StageOut {")
		lines = append(lines, "\tfloat4 position [[position]];")
		for i, v := range w.p.Varyings {
			loc := uint32(i)
			interp := ""
			if i < len(w.p.VaryingMeta) {
				loc = w.p.VaryingMeta[i].Location
				if w.p.VaryingMeta[i].Interpolation == shaderir.InterpFlat {
					interp = " [[flat]]"
				}
			}
			lines = append(lines, fmt.Sprintf("\t%s [[user(locn%d)]]%s;", w.declVar(v, fmt.Sprintf("v%d", i)), loc, interp))
		}
		lines = append(lines, "};", "")
	}
	params := []string{"StageOut __in [[stage_in]]", "bool __front_facing [[front_facing]]"}
	for i, b := range w.p.Bindings {
		params = append(params, w.bindingParam(i, b))
	}
	if len(w.p.FragmentOutputs) > 0 {
		lines = append(lines, "struct FragOut {")
		for i, ot := range w.p.FragmentOutputs {
			loc := uint32(i)
			if i < len(w.p.FragmentOutputMeta) {
				loc = w.p.FragmentOutputMeta[i].Location
			}
			lines = append(lines, fmt.Sprintf("\t%s [[color(%d)]];", w.declVar(ot, fmt.Sprintf("c%d", i)), loc))
		}
		lines = append(lines, "};", "")
		lines = append(lines, fmt.Sprintf("fragment FragOut %s(%s) {", entry, strings.Join(params, ", ")))
		lines = append(lines, "\tFragOut __fout = {};")
		lines = append(lines, w.emitBlock(w.p.FragmentFunc.Block, w.p.FragmentFunc.Block, 1, "fragment")...)
		lines = append(lines, "\treturn __fout;", "}", "")
		return lines
	}
	lines = append(lines, fmt.Sprintf("fragment float4 %s(%s) {", entry, strings.Join(params, ", ")))
	lines = append(lines, w.emitBlock(w.p.FragmentFunc.Block, w.p.FragmentFunc.Block, 1, "fragment")...)
	lines = append(lines, "}", "")
	return lines
}

func (w *pipeWriter) emitCompute(entry string) []string {
	params := []string{
		"uint3 __global_invocation_id [[thread_position_in_grid]]",
		"uint3 __local_invocation_id [[thread_position_in_threadgroup]]",
		"uint3 __workgroup_id [[threadgroup_position_in_grid]]",
		"uint3 __num_workgroups [[threadgroups_per_grid]]",
		"uint __local_invocation_index [[thread_index_in_threadgroup]]",
	}
	for i, b := range w.p.Bindings {
		params = append(params, w.bindingParam(i, b))
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("kernel void %s(%s) {", entry, strings.Join(params, ", ")))
	lines = append(lines, w.emitBlock(w.p.ComputeFunc.Block, w.p.ComputeFunc.Block, 1, "compute")...)
	lines = append(lines, "}", "")
	return lines
}

func (w *pipeWriter) localName(stage string, idx int) string {
	switch stage {
	case "vertex":
		na := len(w.p.Attributes)
		nv := len(w.p.Varyings)
		switch {
		case idx < na:
			return fmt.Sprintf("__in.a%d", idx)
		case idx == na:
			return "__out.position"
		case idx < na+nv+1:
			return fmt.Sprintf("__out.v%d", idx-na-1)
		}
	case "fragment":
		nv := len(w.p.Varyings)
		no := len(w.p.FragmentOutputs)
		switch {
		case idx == 0:
			return "__in.position"
		case idx < nv+1:
			return fmt.Sprintf("__in.v%d", idx-1)
		case idx < nv+1+no:
			return fmt.Sprintf("__fout.c%d", idx-nv-1)
		}
	}
	return fmt.Sprintf("l%d", idx)
}

func (w *pipeWriter) emitBlock(top, b *shaderir.Block, level int, stage string) []string {
	if b == nil {
		return nil
	}
	indent := strings.Repeat("\t", level)
	var lines []string
	for i, lt := range b.LocalVars {
		if lt.Main != shaderir.None {
			lines = append(lines, fmt.Sprintf("%s%s = {};", indent, w.declVar(lt, fmt.Sprintf("l%d", b.LocalVarIndexOffset+i))))
		}
	}
	for _, s := range b.Stmts {
		switch s.Type {
		case shaderir.ExprStmt:
			lines = append(lines, fmt.Sprintf("%s%s;", indent, w.emitExpr(s.Exprs[0], stage)))
		case shaderir.BlockStmt:
			lines = append(lines, indent+"{")
			lines = append(lines, w.emitBlock(top, s.Blocks[0], level+1, stage)...)
			lines = append(lines, indent+"}")
		case shaderir.Assign:
			lines = append(lines, fmt.Sprintf("%s%s = %s;", indent, w.emitExpr(s.Exprs[0], stage), w.emitExpr(s.Exprs[1], stage)))
		case shaderir.Init:
			lines = append(lines, fmt.Sprintf("%s%s = {};", indent, w.localName(stage, s.InitIndex)))
		case shaderir.If:
			lines = append(lines, fmt.Sprintf("%sif (%s) {", indent, w.emitExpr(s.Exprs[0], stage)))
			lines = append(lines, w.emitBlock(top, s.Blocks[0], level+1, stage)...)
			if len(s.Blocks) > 1 {
				lines = append(lines, indent+"} else {")
				lines = append(lines, w.emitBlock(top, s.Blocks[1], level+1, stage)...)
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
			lines = append(lines, fmt.Sprintf("%sfor (%s = %s; %s %s %s; %s += %s) {",
				indent, w.declVar(s.ForVarType, v), initStr, v, opString(s.ForOp), endStr, v, deltaStr))
			lines = append(lines, w.emitBlock(top, s.Blocks[0], level+1, stage)...)
			lines = append(lines, indent+"}")
		case shaderir.Continue:
			lines = append(lines, indent+"continue;")
		case shaderir.Break:
			lines = append(lines, indent+"break;")
		case shaderir.Return:
			if stage == "vertex" {
				lines = append(lines, indent+"return __out;")
			} else if stage == "fragment" && len(w.p.FragmentOutputs) > 0 {
				lines = append(lines, indent+"return __fout;")
			} else if len(s.Exprs) == 0 {
				lines = append(lines, indent+"return;")
			} else {
				lines = append(lines, fmt.Sprintf("%sreturn %s;", indent, w.emitExpr(s.Exprs[0], stage)))
			}
		case shaderir.Discard:
			lines = append(lines, indent+"discard_fragment();")
		}
	}
	return lines
}

func (w *pipeWriter) emitExpr(e shaderir.Expr, stage string) string {
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
			return "__vertex_id"
		case shaderir.BuiltinInstanceIndex:
			return "__instance_id"
		case shaderir.BuiltinFrontFacing:
			return "__front_facing"
		case shaderir.BuiltinPosition:
			return "__in.position"
		case shaderir.BuiltinGlobalInvocationID:
			return "__global_invocation_id"
		case shaderir.BuiltinLocalInvocationID:
			return "__local_invocation_id"
		case shaderir.BuiltinWorkgroupID:
			return "__workgroup_id"
		case shaderir.BuiltinNumWorkgroups:
			return "__num_workgroups"
		case shaderir.BuiltinLocalInvocationIndex:
			return "__local_invocation_index"
		}
	case shaderir.StructConstruct:
		name := w.typeStr(*e.StructType)
		args := make([]string, len(e.Exprs))
		for i, a := range e.Exprs {
			args[i] = w.emitExpr(a, stage)
		}
		return fmt.Sprintf("%s{%s}", name, strings.Join(args, ", "))
	case shaderir.Unary:
		return fmt.Sprintf("(%s%s)", opString(e.Op), w.emitExpr(e.Exprs[0], stage))
	case shaderir.Binary:
		return fmt.Sprintf("(%s %s %s)", w.emitExpr(e.Exprs[0], stage), opString(e.Op), w.emitExpr(e.Exprs[1], stage))
	case shaderir.Selection:
		return fmt.Sprintf("((%s) ? (%s) : (%s))", w.emitExpr(e.Exprs[0], stage), w.emitExpr(e.Exprs[1], stage), w.emitExpr(e.Exprs[2], stage))
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
			_, bArgs := w.helperBindingParams()
			all := append(bArgs, args...)
			return fmt.Sprintf("F%d(%s)", callee.Index, strings.Join(all, ", "))
		}
		var texKind shaderir.BasicType
		if len(e.Exprs) > 1 {
			texKind = w.texType(e.Exprs[1])
		}
		switch callee.BuiltinFunc {
		case shaderir.TextureSample:
			if texKind == shaderir.Texture2DArray && len(args) >= 4 {
				return fmt.Sprintf("%s.sample(%s, %s, uint(%s))", args[0], args[1], args[2], args[3])
			}
			return fmt.Sprintf("%s.sample(%s, %s)", args[0], args[1], strings.Join(args[2:], ", "))
		case shaderir.TextureSampleLevel:
			if texKind == shaderir.Texture2DArray && len(args) >= 5 {
				return fmt.Sprintf("%s.sample(%s, %s, uint(%s), level(%s))", args[0], args[1], args[2], args[3], args[4])
			}
			if len(args) >= 5 {
				return fmt.Sprintf("%s.sample(%s, %s, level(%s), %s)", args[0], args[1], args[2], args[3], args[4])
			}
			return fmt.Sprintf("%s.sample(%s, %s, level(%s))", args[0], args[1], args[2], args[3])
		case shaderir.TextureSampleBias:
			if texKind == shaderir.Texture2DArray && len(args) >= 5 {
				return fmt.Sprintf("%s.sample(%s, %s, uint(%s), bias(%s))", args[0], args[1], args[2], args[3], args[4])
			}
			if len(args) >= 5 {
				return fmt.Sprintf("%s.sample(%s, %s, bias(%s), %s)", args[0], args[1], args[2], args[3], args[4])
			}
			return fmt.Sprintf("%s.sample(%s, %s, bias(%s))", args[0], args[1], args[2], args[3])
		case shaderir.TextureSampleGrad:
			if texKind == shaderir.Texture2DArray && len(args) >= 6 {
				return fmt.Sprintf("%s.sample(%s, %s, uint(%s), gradient2d(%s, %s))", args[0], args[1], args[2], args[3], args[4], args[5])
			}
			if texKind == shaderir.TextureCube && len(args) >= 5 {
				return fmt.Sprintf("%s.sample(%s, %s, gradientcube(%s, %s))", args[0], args[1], args[2], args[3], args[4])
			}
			if len(args) >= 6 {
				return fmt.Sprintf("%s.sample(%s, %s, gradient2d(%s, %s), %s)", args[0], args[1], args[2], args[3], args[4], args[5])
			}
			return fmt.Sprintf("%s.sample(%s, %s, gradient2d(%s, %s))", args[0], args[1], args[2], args[3], args[4])
		case shaderir.TextureSampleCompare:
			if len(args) >= 5 {
				return fmt.Sprintf("%s.sample_compare(%s, %s, %s, %s)", args[0], args[1], args[2], args[3], args[4])
			}
			return fmt.Sprintf("%s.sample_compare(%s, %s, %s)", args[0], args[1], args[2], args[3])
		case shaderir.TextureSampleCompareLevel:
			if len(args) >= 5 {
				return fmt.Sprintf("%s.sample_compare(%s, %s, %s, level(0), %s)", args[0], args[1], args[2], args[3], args[4])
			}
			return fmt.Sprintf("%s.sample_compare(%s, %s, %s, level(0))", args[0], args[1], args[2], args[3])
		case shaderir.TextureLoad:
			if texKind == shaderir.Texture2DArray && len(args) >= 4 {
				return fmt.Sprintf("%s.read(uint2(%s), uint(%s), uint(%s))", args[0], args[1], args[2], args[3])
			}
			return fmt.Sprintf("%s.read(uint2(%s), uint(%s))", args[0], args[1], args[2])
		case shaderir.TextureDimensions:
			if len(args) >= 2 {
				return fmt.Sprintf("uint2(%s.get_width(uint(%s)), %s.get_height(uint(%s)))", args[0], args[1], args[0], args[1])
			}
			return fmt.Sprintf("uint2(%s.get_width(), %s.get_height())", args[0], args[0])
		case shaderir.TextureStore:
			return fmt.Sprintf("%s.write(%s, uint2(%s))", args[0], args[2], args[1])
		case shaderir.ArrayLength:
			return fmt.Sprintf("uint(sizeof(%s))", args[0])
		case shaderir.WorkgroupBarrier:
			return "threadgroup_barrier(mem_flags::mem_threadgroup)"
		case shaderir.StorageBarrier:
			return "threadgroup_barrier(mem_flags::mem_device)"
		case shaderir.Atan2:
			return fmt.Sprintf("atan2(%s)", strings.Join(args, ", "))
		default:
			return fmt.Sprintf("%s(%s)", builtinFuncString(callee.BuiltinFunc), strings.Join(args, ", "))
		}
	}
	return "0"
}

func (w *pipeWriter) texType(e shaderir.Expr) shaderir.BasicType {
	if e.Type == shaderir.BindingVariable && e.Index >= 0 && e.Index < len(w.p.Bindings) {
		return w.p.Bindings[e.Index].Type.Main
	}
	return shaderir.Texture2D
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
		return fmt.Sprintf("%d", v)
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
