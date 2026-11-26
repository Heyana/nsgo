package hlsl

import (
	"fmt"
	"go/constant"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// RegisterBinding records the HLSL register (bN, tN, or sN) assigned to a
// WGSL @group(G) @binding(B) resource.
type RegisterBinding struct {
	Group    uint32
	Binding  uint32
	Kind     shaderir.BindingKind
	Register uint32
}

// PipelineSource holds HLSL Shader Model 5.0 vertex, pixel, and/or compute shader sources and register bindings compiled from a GPU pipeline shaderir.Program.
type PipelineSource struct {
	VertexSource  string
	PixelSource   string
	ComputeSource string
	VertexEntry   string
	PixelEntry    string
	ComputeEntry  string
	WorkgroupSize [3]uint32
	Registers     []RegisterBinding
}

// CompilePipeline emits HLSL SM 5.0 vertex, pixel, and/or compute shaders for a 3D GPU pipeline program.
func CompilePipeline(p *shaderir.Program) (PipelineSource, error) {
	if p == nil {
		return PipelineSource{}, fmt.Errorf("hlsl: nil program")
	}
	w := &pipeWriter{
		p:              p,
		valueNames:     map[string]string{},
		needStructCtor: map[string]bool{},
		bindingReg:     make([]uint32, len(p.Bindings)),
		lenHelpers:     map[int]bool{},
		storageHelpers: map[storageHelperKey]string{},
		rawStorage:     p.ComputeFunc.Block == nil || p.VertexFunc.Block != nil || p.FragmentFunc.Block != nil,
	}
	var bReg, tReg, uReg, sReg uint32
	for i, b := range p.Bindings {
		switch b.Kind {
		case shaderir.BindingUniformBuffer:
			w.typeStr(b.Type)
			w.bindingReg[i] = bReg
			w.regs = append(w.regs, RegisterBinding{Group: b.Group, Binding: b.Binding, Kind: b.Kind, Register: bReg})
			bReg++
		case shaderir.BindingStorageBufferRead, shaderir.BindingTexture:
			if b.Kind == shaderir.BindingStorageBufferRead && !w.rawStorage {
				w.typeStr(storageElement(b.Type))
			}
			w.bindingReg[i] = tReg
			w.regs = append(w.regs, RegisterBinding{Group: b.Group, Binding: b.Binding, Kind: b.Kind, Register: tReg})
			tReg++
		case shaderir.BindingStorageBufferReadWrite, shaderir.BindingStorageTexture:
			if b.Kind == shaderir.BindingStorageBufferReadWrite && b.Type.Main != shaderir.None {
				w.typeStr(storageElement(b.Type))
			}
			w.bindingReg[i] = uReg
			w.regs = append(w.regs, RegisterBinding{Group: b.Group, Binding: b.Binding, Kind: b.Kind, Register: uReg})
			uReg++
		case shaderir.BindingSampler:
			w.bindingReg[i] = sReg
			w.regs = append(w.regs, RegisterBinding{Group: b.Group, Binding: b.Binding, Kind: b.Kind, Register: sReg})
			sReg++
		}
	}

	vEntry := p.VertexEntry
	if vEntry == "" && p.VertexFunc.Block != nil {
		vEntry = "vs_main"
	}
	pEntry := p.FragmentEntry
	if pEntry == "" && p.FragmentFunc.Block != nil {
		pEntry = "fs_main"
	}
	cEntry := p.ComputeEntry
	if cEntry == "" && p.ComputeFunc.Block != nil {
		cEntry = "cs_main"
	}

	var common []string
	common = append(common, utilFuncs, "")
	var helperLines []string
	for _, f := range p.Funcs {
		helperLines = append(helperLines, w.emitHelper(f)...)
	}

	var vsBody, psBody, csBody []string
	if p.VertexFunc.Block != nil {
		vsBody = w.emitVertex(vEntry)
	}
	if p.FragmentFunc.Block != nil {
		psBody = w.emitPixel(pEntry)
	}
	if p.ComputeFunc.Block != nil {
		csBody = w.emitCompute(cEntry)
	}
	if w.storageErr != nil {
		return PipelineSource{}, w.storageErr
	}
	var varyings []string
	if p.VertexFunc.Block != nil || p.FragmentFunc.Block != nil {
		varyings = w.emitVaryingsStruct()
	}

	var header []string
	header = append(header, common...)
	for i, st := range w.valueTypes {
		if st.Main == shaderir.Array {
			header = append(header, fmt.Sprintf("typedef %s A%d[%d];", w.typeStr(st.Sub[0]), i, st.Length))
			continue
		}
		name := fmt.Sprintf("S%d", i)
		header = append(header, fmt.Sprintf("struct %s {", name))
		header = append(header, w.emitPaddedStructFields(st)...)
		header = append(header, "};", "")
		if w.needStructCtor[name] {
			params := make([]string, len(st.Sub))
			for j, m := range st.Sub {
				params[j] = w.declVar(m, fmt.Sprintf("a%d", j))
			}
			header = append(header, fmt.Sprintf("%s __make_%s(%s) {", name, name, strings.Join(params, ", ")))
			header = append(header, fmt.Sprintf("\t%s r = (%s)0;", name, name))
			for j := range st.Sub {
				header = append(header, fmt.Sprintf("\tr.M%d = a%d;", j, j))
			}
			header = append(header, "\treturn r;", "}", "")
		}
	}
	for i, b := range p.Bindings {
		reg := w.bindingReg[i]
		switch b.Kind {
		case shaderir.BindingUniformBuffer:
			header = append(header, fmt.Sprintf("cbuffer CB%d : register(b%d) {", i, reg))
			header = append(header, fmt.Sprintf("\t%s;", w.declVar(b.Type, fmt.Sprintf("B%d", i))))
			header = append(header, "};")
		case shaderir.BindingStorageBufferRead:
			if w.rawStorage {
				header = append(header, fmt.Sprintf("ByteAddressBuffer B%d : register(t%d);", i, reg))
			} else {
				header = append(header, fmt.Sprintf("StructuredBuffer<%s> B%d : register(t%d);", w.typeStr(storageElement(b.Type)), i, reg))
			}
		case shaderir.BindingStorageBufferReadWrite:
			if b.Type.Main == shaderir.None {
				header = append(header, fmt.Sprintf("RWByteAddressBuffer B%d : register(u%d);", i, reg))
			} else {
				header = append(header, fmt.Sprintf("RWStructuredBuffer<%s> B%d : register(u%d);", w.typeStr(storageElement(b.Type)), i, reg))
			}
		case shaderir.BindingTexture:
			header = append(header, fmt.Sprintf("%s B%d : register(t%d);", w.typeStr(b.Type), i, reg))
		case shaderir.BindingStorageTexture:
			header = append(header, fmt.Sprintf("RWTexture2D<%s> B%d : register(u%d);", hlslStorageTexelType(b.StorageFormat), i, reg))
		case shaderir.BindingSampler:
			samp := "SamplerState"
			if b.Type.Main == shaderir.SamplerComparison {
				samp = "SamplerComparisonState"
			}
			header = append(header, fmt.Sprintf("%s B%d : register(s%d);", samp, i, reg))
		}
	}
	if w.usesNumWorkgroups {
		header = append(header, "cbuffer __NSGoComputeMeta : register(b15) {", "\tuint3 __num_workgroups;", "};")
	}
	for i, b := range p.Bindings {
		if !w.lenHelpers[i] {
			continue
		}
		if w.rawStorage && b.Kind == shaderir.BindingStorageBufferRead {
			header = append(header, fmt.Sprintf("uint __len_B%d(uint base, uint stride) { uint size; B%d.GetDimensions(size); return (max(size, base) - base) / stride; }", i, i))
		} else {
			header = append(header, fmt.Sprintf("uint __len_B%d() { uint __n = 0u, __s = 0u; B%d.GetDimensions(__n, __s); return __n; }", i, i))
		}
	}
	header = append(header, w.storageFuncs...)
	header = append(header, "")
	header = append(header, varyings...)
	header = append(header, helperLines...)

	hdr := strings.Join(header, "\n")
	var vsSrc, psSrc, csSrc string
	if len(vsBody) > 0 {
		vsSrc = strings.TrimSpace(hdr+"\n"+strings.Join(vsBody, "\n")) + "\n"
	}
	if len(psBody) > 0 {
		psSrc = strings.TrimSpace(hdr+"\n"+strings.Join(psBody, "\n")) + "\n"
	}
	if len(csBody) > 0 {
		csSrc = strings.TrimSpace(hdr+"\n"+strings.Join(csBody, "\n")) + "\n"
	}
	return PipelineSource{
		VertexSource:  vsSrc,
		PixelSource:   psSrc,
		ComputeSource: csSrc,
		VertexEntry:   vEntry,
		PixelEntry:    pEntry,
		ComputeEntry:  cEntry,
		WorkgroupSize: p.ComputeFunc.WorkgroupSize,
		Registers:     w.regs,
	}, nil
}

// emitPaddedStructFields gives HLSL structs the offsets and stride used by
// WGSL storage and uniform buffers. HLSL otherwise packs float3 at the next
// four-byte boundary, while WGSL aligns it to sixteen bytes.
func (w *pipeWriter) emitPaddedStructFields(t shaderir.Type) []string {
	var lines []string
	cursor := 0
	pad := 0
	for i, field := range t.Sub {
		offset := t.StorageStructFieldOffset(i)
		for cursor < offset {
			lines = append(lines, fmt.Sprintf("\tuint __pad%d;", pad))
			pad++
			cursor += 4
		}
		lines = append(lines, fmt.Sprintf("\t%s;", w.declVar(field, fmt.Sprintf("M%d", i))))
		cursor = offset + field.StorageSizeBytes()
	}
	for cursor < t.StorageSizeBytes() {
		lines = append(lines, fmt.Sprintf("\tuint __pad%d;", pad))
		pad++
		cursor += 4
	}
	return lines
}

type pipeWriter struct {
	p                 *shaderir.Program
	valueNames        map[string]string
	valueTypes        []shaderir.Type
	needStructCtor    map[string]bool
	bindingReg        []uint32
	regs              []RegisterBinding
	usesNumWorkgroups bool
	lenHelpers        map[int]bool
	rawStorage        bool
	storageHelpers    map[storageHelperKey]string
	storageFuncs      []string
	storageErr        error
}

func storageElement(t shaderir.Type) shaderir.Type {
	if t.Main == shaderir.Array && len(t.Sub) > 0 {
		return t.Sub[0]
	}
	return t
}

func (w *pipeWriter) valueTypeName(t shaderir.Type) string {
	key := t.String()
	if n, ok := w.valueNames[key]; ok {
		return n
	}
	for _, f := range t.Sub {
		w.typeStr(f)
	}
	prefix := "S"
	if t.Main == shaderir.Array {
		prefix = "A"
	}
	n := fmt.Sprintf("%s%d", prefix, len(w.valueTypes))
	w.valueNames[key] = n
	w.valueTypes = append(w.valueTypes, t)
	return n
}

func hlslStorageTexelType(f shaderir.StorageTextureFormat) string {
	switch f {
	case shaderir.StorageFormatR32Uint:
		return "uint"
	case shaderir.StorageFormatR32Sint:
		return "int"
	case shaderir.StorageFormatR32Float:
		return "float"
	default:
		return "float4"
	}
}

func (w *pipeWriter) typeStr(t shaderir.Type) string {
	switch t.Main {
	case shaderir.Struct, shaderir.Array:
		return w.valueTypeName(t)
	case shaderir.Texture2D, shaderir.Texture, shaderir.TextureDepth2D:
		return "Texture2D"
	case shaderir.TextureCube:
		return "TextureCube"
	case shaderir.Texture2DArray:
		return "Texture2DArray"
	case shaderir.TextureMultisampled2D:
		return "Texture2DMS<float4>"
	case shaderir.Mat2x3:
		return "float3x2"
	case shaderir.Mat2x4:
		return "float4x2"
	case shaderir.Mat3x2:
		return "float2x3"
	case shaderir.Mat3x4:
		return "float4x3"
	case shaderir.Mat4x2:
		return "float2x4"
	case shaderir.Mat4x3:
		return "float3x4"
	default:
		return basicTypeString(t.Main)
	}
}

func (w *pipeWriter) declVar(t shaderir.Type, name string) string {
	return fmt.Sprintf("%s %s", w.typeStr(t), name)
}

func (w *pipeWriter) emitVaryingsStruct() []string {
	lines := []string{"struct StageOut {", "\tfloat4 position : SV_POSITION;"}
	for i, v := range w.p.Varyings {
		loc := uint32(i)
		interp := ""
		if i < len(w.p.VaryingMeta) {
			loc = w.p.VaryingMeta[i].Location
			switch w.p.VaryingMeta[i].Interpolation {
			case shaderir.InterpFlat:
				interp = "nointerpolation "
			case shaderir.InterpLinear:
				interp = "noperspective "
			}
		}
		lines = append(lines, fmt.Sprintf("\t%s%s : TEXCOORD%d;", interp, w.declVar(v, fmt.Sprintf("v%d", i)), loc))
	}
	lines = append(lines, "};", "")
	return lines
}

func (w *pipeWriter) emitHelper(f shaderir.Func) []string {
	var params []string
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
	lines = append(lines, "struct VertexIn {")
	for i, a := range w.p.Attributes {
		loc := uint32(i)
		if i < len(w.p.AttributeMeta) {
			loc = w.p.AttributeMeta[i].Location
		}
		lines = append(lines, fmt.Sprintf("\t%s : TEXCOORD%d;", w.declVar(a, fmt.Sprintf("a%d", i)), loc))
	}
	lines = append(lines, "\tuint __vertex_id : SV_VertexID;", "\tuint __instance_id : SV_InstanceID;", "};", "")
	lines = append(lines, fmt.Sprintf("StageOut %s(VertexIn __in) {", entry))
	lines = append(lines, "\tStageOut __out = (StageOut)0;")
	lines = append(lines, w.emitBlock(w.p.VertexFunc.Block, w.p.VertexFunc.Block, 1, "vertex")...)
	lines = append(lines, "\treturn __out;", "}", "")
	return lines
}

func (w *pipeWriter) emitPixel(entry string) []string {
	var lines []string
	if len(w.p.FragmentOutputs) > 0 {
		lines = append(lines, "struct FragOut {")
		for i, ot := range w.p.FragmentOutputs {
			loc := uint32(i)
			if i < len(w.p.FragmentOutputMeta) {
				loc = w.p.FragmentOutputMeta[i].Location
			}
			lines = append(lines, fmt.Sprintf("\t%s : SV_TARGET%d;", w.declVar(ot, fmt.Sprintf("c%d", i)), loc))
		}
		lines = append(lines, "};", "")
		lines = append(lines, fmt.Sprintf("FragOut %s(StageOut __in, bool __front_facing : SV_IsFrontFace) {", entry))
		lines = append(lines, "\tFragOut __fout = (FragOut)0;")
		lines = append(lines, w.emitBlock(w.p.FragmentFunc.Block, w.p.FragmentFunc.Block, 1, "fragment")...)
		lines = append(lines, "\treturn __fout;", "}", "")
		return lines
	}
	lines = append(lines, fmt.Sprintf("float4 %s(StageOut __in, bool __front_facing : SV_IsFrontFace) : SV_TARGET {", entry))
	lines = append(lines, w.emitBlock(w.p.FragmentFunc.Block, w.p.FragmentFunc.Block, 1, "fragment")...)
	lines = append(lines, "}", "")
	return lines
}

func (w *pipeWriter) emitCompute(entry string) []string {
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
	var lines []string
	lines = append(lines, fmt.Sprintf("[numthreads(%d, %d, %d)]", ws[0], ws[1], ws[2]))
	lines = append(lines, fmt.Sprintf("void %s(uint3 __global_invocation_id : SV_DispatchThreadID, uint3 __local_invocation_id : SV_GroupThreadID, uint3 __workgroup_id : SV_GroupID, uint __local_invocation_index : SV_GroupIndex) {", entry))
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
			lines = append(lines, fmt.Sprintf("%s%s = (%s)0;", indent, w.declVar(lt, fmt.Sprintf("l%d", b.LocalVarIndexOffset+i)), w.typeStr(lt)))
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
			lines = append(lines, fmt.Sprintf("%s// init %s", indent, w.localName(stage, s.InitIndex)))
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
				initStr = hlslConstLit(s.ForInit, s.ForVarType.Main == shaderir.Uint)
				endStr = hlslConstLit(s.ForEnd, s.ForVarType.Main == shaderir.Uint)
			}
			deltaStr := hlslConstLit(s.ForDelta, s.ForVarType.Main == shaderir.Uint)
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
			lines = append(lines, indent+"discard;")
			if stage == "fragment" {
				lines = append(lines, indent+"return (float4)0;")
			}
		}
	}
	return lines
}

func (w *pipeWriter) emitExpr(e shaderir.Expr, stage string) string {
	if w.rawStorage {
		if ref, ok := w.storageRef(e, stage); ok {
			return w.storageLoad(ref)
		}
	}
	switch e.Type {
	case shaderir.NumberExpr:
		return hlslConstLit(e.Const, e.Unsigned)
	case shaderir.LocalVariable:
		return w.localName(stage, e.Index)
	case shaderir.BindingVariable:
		if e.Index >= 0 && e.Index < len(w.p.Bindings) {
			b := w.p.Bindings[e.Index]
			if (b.Kind == shaderir.BindingStorageBufferRead || b.Kind == shaderir.BindingStorageBufferReadWrite) && b.Type.Main != shaderir.Array && b.Type.Main != shaderir.None {
				return fmt.Sprintf("B%d[0]", e.Index)
			}
		}
		return fmt.Sprintf("B%d", e.Index)
	case shaderir.BuiltinInput:
		switch shaderir.StageBuiltin(e.Index) {
		case shaderir.BuiltinVertexIndex:
			return "__in.__vertex_id"
		case shaderir.BuiltinInstanceIndex:
			return "__in.__instance_id"
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
			w.usesNumWorkgroups = true
			return "__num_workgroups"
		case shaderir.BuiltinLocalInvocationIndex:
			return "__local_invocation_index"
		}
	case shaderir.StructConstruct:
		name := w.typeStr(*e.StructType)
		if len(e.Exprs) == 0 {
			return fmt.Sprintf("(%s)0", name)
		}
		w.needStructCtor[name] = true
		args := make([]string, len(e.Exprs))
		for i, a := range e.Exprs {
			args[i] = w.emitExpr(a, stage)
		}
		return fmt.Sprintf("__make_%s(%s)", name, strings.Join(args, ", "))
	case shaderir.Unary:
		return fmt.Sprintf("(%s%s)", opString(e.Op), w.emitExpr(e.Exprs[0], stage))
	case shaderir.Binary:
		switch e.Op {
		case shaderir.MatrixMul:
			// HLSL uses mul(lhs, rhs) for column-major cbuffer matrix * vector / matrix multiplication.
			return fmt.Sprintf("mul(%s, %s)", w.emitExpr(e.Exprs[0], stage), w.emitExpr(e.Exprs[1], stage))
		case shaderir.VectorEqualOp:
			return fmt.Sprintf("all((%s) == (%s))", w.emitExpr(e.Exprs[0], stage), w.emitExpr(e.Exprs[1], stage))
		case shaderir.VectorNotEqualOp:
			return fmt.Sprintf("!all((%s) == (%s))", w.emitExpr(e.Exprs[0], stage), w.emitExpr(e.Exprs[1], stage))
		case shaderir.AndNot:
			return fmt.Sprintf("((%s) & ~(%s))", w.emitExpr(e.Exprs[0], stage), w.emitExpr(e.Exprs[1], stage))
		}
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
		if callee.BuiltinFunc == shaderir.ArrayLength && len(e.Exprs) > 1 {
			array := e.Exprs[1]
			if w.rawStorage {
				if ref, ok := w.storageRef(array, stage); ok {
					w.lenHelpers[ref.binding] = true
					return fmt.Sprintf("__len_B%d(%s, %du)", ref.binding, ref.address(), ref.typ.Sub[0].StorageStride())
				}
			}
			if array.Type == shaderir.BindingVariable {
				w.lenHelpers[array.Index] = true
				return fmt.Sprintf("__len_B%d()", array.Index)
			}
			return "0u"
		}
		args := make([]string, len(e.Exprs)-1)
		for i, a := range e.Exprs[1:] {
			args[i] = w.emitExpr(a, stage)
		}
		if callee.Type == shaderir.FunctionExpr {
			return fmt.Sprintf("F%d(%s)", callee.Index, strings.Join(args, ", "))
		}
		var texKind shaderir.BasicType
		if len(e.Exprs) > 1 {
			texKind = w.texType(e.Exprs[1])
		}
		switch callee.BuiltinFunc {
		case shaderir.TextureSample:
			texIdx := e.Exprs[1].Index
			suffix := ""
			if texIdx >= 0 && texIdx < len(w.p.Bindings) && w.p.Bindings[texIdx].Type.Main == shaderir.TextureDepth2D {
				suffix = ".x"
			}
			if texKind == shaderir.Texture2DArray && len(args) >= 4 {
				return fmt.Sprintf("%s.Sample(%s, float3(%s, float(%s)))", args[0], args[1], args[2], args[3])
			}
			return fmt.Sprintf("%s.Sample(%s, %s)%s", args[0], args[1], strings.Join(args[2:], ", "), suffix)
		case shaderir.TextureSampleLevel:
			texIdx := e.Exprs[1].Index
			suffix := ""
			if texIdx >= 0 && texIdx < len(w.p.Bindings) && w.p.Bindings[texIdx].Type.Main == shaderir.TextureDepth2D {
				suffix = ".x"
			}
			if texKind == shaderir.Texture2DArray && len(args) >= 5 {
				return fmt.Sprintf("%s.SampleLevel(%s, float3(%s, float(%s)), %s)", args[0], args[1], args[2], args[3], args[4])
			}
			if len(args) >= 5 {
				return fmt.Sprintf("%s.SampleLevel(%s, %s, %s, %s)%s", args[0], args[1], args[2], args[3], args[4], suffix)
			}
			return fmt.Sprintf("%s.SampleLevel(%s, %s, %s)%s", args[0], args[1], args[2], args[3], suffix)
		case shaderir.TextureSampleBias:
			if texKind == shaderir.Texture2DArray && len(args) >= 5 {
				return fmt.Sprintf("%s.SampleBias(%s, float3(%s, float(%s)), %s)", args[0], args[1], args[2], args[3], args[4])
			}
			return fmt.Sprintf("%s.SampleBias(%s, %s)", args[0], args[1], strings.Join(args[2:], ", "))
		case shaderir.TextureSampleGrad:
			if texKind == shaderir.Texture2DArray && len(args) >= 6 {
				return fmt.Sprintf("%s.SampleGrad(%s, float3(%s, float(%s)), %s, %s)", args[0], args[1], args[2], args[3], args[4], args[5])
			}
			return fmt.Sprintf("%s.SampleGrad(%s, %s)", args[0], args[1], strings.Join(args[2:], ", "))
		case shaderir.TextureSampleCompare:
			if len(args) >= 5 {
				return fmt.Sprintf("%s.SampleCmp(%s, %s, %s, %s)", args[0], args[1], args[2], args[3], args[4])
			}
			return fmt.Sprintf("%s.SampleCmp(%s, %s, %s)", args[0], args[1], args[2], args[3])
		case shaderir.TextureSampleCompareLevel:
			if len(args) >= 5 {
				return fmt.Sprintf("%s.SampleCmpLevelZero(%s, %s, %s, %s)", args[0], args[1], args[2], args[3], args[4])
			}
			return fmt.Sprintf("%s.SampleCmpLevelZero(%s, %s, %s)", args[0], args[1], args[2], args[3])
		case shaderir.TextureLoad:
			if texKind == shaderir.Texture2DArray && len(args) >= 4 {
				return fmt.Sprintf("%s.Load(int4(%s, int(%s), int(%s)))", args[0], args[1], args[2], args[3])
			}
			texIdx := e.Exprs[1].Index
			if texIdx >= 0 && texIdx < len(w.p.Bindings) && w.p.Bindings[texIdx].Type.Main == shaderir.TextureDepth2D {
				return fmt.Sprintf("%s.Load(int3(%s, %s)).x", args[0], args[1], args[2])
			}
			return fmt.Sprintf("%s.Load(int3(%s, %s))", args[0], args[1], args[2])
		case shaderir.TextureDimensions:
			return fmt.Sprintf("uint2(0u, 0u) /* %s */", args[0])
		case shaderir.TextureStore:
			return fmt.Sprintf("%s[%s] = %s", args[0], args[1], args[2])
		case shaderir.WorkgroupBarrier:
			return "GroupMemoryBarrierWithGroupSync()"
		case shaderir.StorageBarrier:
			return "DeviceMemoryBarrierWithGroupSync()"
		case shaderir.Atan2:
			return fmt.Sprintf("atan2(%s)", strings.Join(args, ", "))
		case shaderir.Vec2F, shaderir.Vec3F, shaderir.Vec4F,
			shaderir.IVec2F, shaderir.IVec3F, shaderir.IVec4F,
			shaderir.UVec2F, shaderir.UVec3F, shaderir.UVec4F:
			var cc compileContext
			fn := cc.builtinFuncString(callee.BuiltinFunc)
			if len(args) == 1 {
				// HLSL SM 5.0 rejects single-argument vector constructors like float3(x); use a cast.
				return fmt.Sprintf("(%s)(%s)", fn, args[0])
			}
			return fmt.Sprintf("%s(%s)", fn, strings.Join(args, ", "))
		case shaderir.Mat2F:
			if len(args) == 1 {
				return fmt.Sprintf("float2x2FromScalar(%s)", args[0])
			}
			// WGSL constructs matrices column-by-column whereas HLSL floatNxN(...) takes rows.
			return fmt.Sprintf("transpose(float2x2(%s))", strings.Join(args, ", "))
		case shaderir.Mat3F:
			if len(args) == 1 {
				return fmt.Sprintf("float3x3FromScalar(%s)", args[0])
			}
			return fmt.Sprintf("transpose(float3x3(%s))", strings.Join(args, ", "))
		case shaderir.Mat4F:
			if len(args) == 1 {
				return fmt.Sprintf("float4x4FromScalar(%s)", args[0])
			}
			return fmt.Sprintf("transpose(float4x4(%s))", strings.Join(args, ", "))
		case shaderir.Min, shaderir.Max:
			var cc compileContext
			fn := cc.builtinFuncString(callee.BuiltinFunc)
			res := args[0]
			for i := 1; i < len(args); i++ {
				res = fmt.Sprintf("%s(%s, %s)", fn, res, args[i])
			}
			return res
		default:
			var cc compileContext
			return fmt.Sprintf("%s(%s)", cc.builtinFuncString(callee.BuiltinFunc), strings.Join(args, ", "))
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

func hlslConstLit(c constant.Value, unsigned bool) string {
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
		return s
	}
	return "0"
}
