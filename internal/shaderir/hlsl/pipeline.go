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

// PipelineSource holds HLSL Shader Model 5.0 vertex and pixel shader sources
// and register bindings compiled from a GPU pipeline shaderir.Program.
type PipelineSource struct {
	VertexSource string
	PixelSource  string
	VertexEntry  string
	PixelEntry   string
	Registers    []RegisterBinding
}

// CompilePipeline emits HLSL SM 5.0 vertex and pixel shaders for a 3D GPU
// pipeline program.
func CompilePipeline(p *shaderir.Program) (PipelineSource, error) {
	if p == nil {
		return PipelineSource{}, fmt.Errorf("hlsl: nil program")
	}
	w := &pipeWriter{
		p:           p,
		structNames: map[string]string{},
		bindingReg:  make([]uint32, len(p.Bindings)),
	}
	var bReg, tReg, sReg uint32
	for i, b := range p.Bindings {
		switch b.Kind {
		case shaderir.BindingUniformBuffer:
			w.bindingReg[i] = bReg
			w.regs = append(w.regs, RegisterBinding{Group: b.Group, Binding: b.Binding, Kind: b.Kind, Register: bReg})
			bReg++
		case shaderir.BindingStorageBufferRead, shaderir.BindingTexture:
			w.bindingReg[i] = tReg
			w.regs = append(w.regs, RegisterBinding{Group: b.Group, Binding: b.Binding, Kind: b.Kind, Register: tReg})
			tReg++
		case shaderir.BindingSampler:
			w.bindingReg[i] = sReg
			w.regs = append(w.regs, RegisterBinding{Group: b.Group, Binding: b.Binding, Kind: b.Kind, Register: sReg})
			sReg++
		}
	}

	vEntry := p.VertexEntry
	if vEntry == "" {
		vEntry = "vs_main"
	}
	pEntry := p.FragmentEntry
	if pEntry == "" {
		pEntry = "fs_main"
	}

	var common []string
	common = append(common, utilFuncs, "")
	// Pre-register binding struct types and helper functions.
	for _, b := range p.Bindings {
		if b.Type.Main == shaderir.Struct {
			w.structName(b.Type)
		}
	}
	var helperLines []string
	for _, f := range p.Funcs {
		helperLines = append(helperLines, w.emitHelper(f)...)
	}

	var vsBody, psBody []string
	if p.VertexFunc.Block != nil {
		vsBody = w.emitVertex(vEntry)
	}
	if p.FragmentFunc.Block != nil {
		psBody = w.emitPixel(pEntry)
	}

	var header []string
	header = append(header, common...)
	for i, st := range w.structTypes {
		header = append(header, fmt.Sprintf("struct S%d {", i))
		for j, m := range st.Sub {
			header = append(header, fmt.Sprintf("\t%s;", w.declVar(m, fmt.Sprintf("M%d", j))))
		}
		header = append(header, "};", "")
	}
	for i, b := range p.Bindings {
		reg := w.bindingReg[i]
		switch b.Kind {
		case shaderir.BindingUniformBuffer:
			header = append(header, fmt.Sprintf("cbuffer CB%d : register(b%d) {", i, reg))
			header = append(header, fmt.Sprintf("\t%s;", w.declVar(b.Type, fmt.Sprintf("B%d", i))))
			header = append(header, "};")
		case shaderir.BindingStorageBufferRead:
			header = append(header, fmt.Sprintf("StructuredBuffer<%s> B%d : register(t%d);", w.typeStr(b.Type), i, reg))
		case shaderir.BindingTexture:
			header = append(header, fmt.Sprintf("%s B%d : register(t%d);", w.typeStr(b.Type), i, reg))
		case shaderir.BindingSampler:
			samp := "SamplerState"
			if b.Type.Main == shaderir.SamplerComparison {
				samp = "SamplerComparisonState"
			}
			header = append(header, fmt.Sprintf("%s B%d : register(s%d);", samp, i, reg))
		}
	}
	header = append(header, "")
	header = append(header, w.emitVaryingsStruct()...)
	header = append(header, helperLines...)

	hdr := strings.Join(header, "\n")
	var vsSrc, psSrc string
	if len(vsBody) > 0 {
		vsSrc = strings.TrimSpace(hdr+"\n"+strings.Join(vsBody, "\n")) + "\n"
	}
	if len(psBody) > 0 {
		psSrc = strings.TrimSpace(hdr+"\n"+strings.Join(psBody, "\n")) + "\n"
	}
	return PipelineSource{
		VertexSource: vsSrc,
		PixelSource:  psSrc,
		VertexEntry:  vEntry,
		PixelEntry:   pEntry,
		Registers:    w.regs,
	}, nil
}

type pipeWriter struct {
	p           *shaderir.Program
	structNames map[string]string
	structTypes []shaderir.Type
	bindingReg  []uint32
	regs        []RegisterBinding
}

func (w *pipeWriter) structName(t shaderir.Type) string {
	key := t.String()
	if n, ok := w.structNames[key]; ok {
		return n
	}
	for _, f := range t.Sub {
		if f.Main == shaderir.Struct {
			w.structName(f)
		}
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
	case shaderir.Texture2D, shaderir.Texture, shaderir.TextureDepth2D:
		return "Texture2D"
	case shaderir.TextureCube:
		return "TextureCube"
	case shaderir.Texture2DArray:
		return "Texture2DArray"
	case shaderir.TextureMultisampled2D:
		return "Texture2DMS<float4>"
	default:
		return basicTypeString(t.Main)
	}
}

func (w *pipeWriter) declVar(t shaderir.Type, name string) string {
	if t.Main == shaderir.Array {
		return fmt.Sprintf("%s %s[%d]", w.typeStr(t.Sub[0]), name, t.Length)
	}
	return fmt.Sprintf("%s %s", w.typeStr(t), name)
}

func (w *pipeWriter) emitVaryingsStruct() []string {
	lines := []string{"struct StageOut {", "\tfloat4 position : SV_POSITION;"}
	for i, v := range w.p.Varyings {
		loc := uint32(i)
		interp := ""
		if i < len(w.p.VaryingMeta) {
			loc = w.p.VaryingMeta[i].Location
			if w.p.VaryingMeta[i].Interpolation == shaderir.InterpFlat {
				interp = "nointerpolation "
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
	lines = append(lines, fmt.Sprintf("float4 %s(StageOut __in, bool __front_facing : SV_IsFrontFace) : SV_TARGET {", entry))
	lines = append(lines, w.emitBlock(w.p.FragmentFunc.Block, w.p.FragmentFunc.Block, 1, "fragment")...)
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
		switch {
		case idx == 0:
			return "__in.position"
		case idx < nv+1:
			return fmt.Sprintf("__in.v%d", idx-1)
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

func (w *pipeWriter) emitExpr(e shaderir.Expr, stage string) string {
	switch e.Type {
	case shaderir.NumberExpr:
		return hlslConstLit(e.Const, e.Unsigned)
	case shaderir.LocalVariable:
		return w.localName(stage, e.Index)
	case shaderir.BindingVariable:
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
		}
	case shaderir.StructConstruct:
		name := w.typeStr(*e.StructType)
		if len(e.Exprs) == 0 {
			return fmt.Sprintf("(%s)0", name)
		}
		args := make([]string, len(e.Exprs))
		for i, a := range e.Exprs {
			args[i] = w.emitExpr(a, stage)
		}
		return fmt.Sprintf("(%s){%s}", name, strings.Join(args, ", "))
	case shaderir.Unary:
		return fmt.Sprintf("(%s%s)", opString(e.Op), w.emitExpr(e.Exprs[0], stage))
	case shaderir.Binary:
		if e.Op == shaderir.MatrixMul {
			// HLSL uses mul(lhs, rhs) for matrix-vector / matrix-matrix multiplication.
			return fmt.Sprintf("mul(%s, %s)", w.emitExpr(e.Exprs[0], stage), w.emitExpr(e.Exprs[1], stage))
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
		args := make([]string, len(e.Exprs)-1)
		for i, a := range e.Exprs[1:] {
			args[i] = w.emitExpr(a, stage)
		}
		if callee.Type == shaderir.FunctionExpr {
			return fmt.Sprintf("F%d(%s)", callee.Index, strings.Join(args, ", "))
		}
		switch callee.BuiltinFunc {
		case shaderir.TextureSample:
			texIdx := e.Exprs[1].Index
			suffix := ""
			if texIdx >= 0 && texIdx < len(w.p.Bindings) && w.p.Bindings[texIdx].Type.Main == shaderir.TextureDepth2D {
				suffix = ".x"
			}
			return fmt.Sprintf("%s.Sample(%s, %s)%s", args[0], args[1], strings.Join(args[2:], ", "), suffix)
		case shaderir.TextureSampleLevel:
			texIdx := e.Exprs[1].Index
			suffix := ""
			if texIdx >= 0 && texIdx < len(w.p.Bindings) && w.p.Bindings[texIdx].Type.Main == shaderir.TextureDepth2D {
				suffix = ".x"
			}
			if len(args) >= 5 {
				return fmt.Sprintf("%s.SampleLevel(%s, %s, %s, %s)%s", args[0], args[1], args[2], args[3], args[4], suffix)
			}
			return fmt.Sprintf("%s.SampleLevel(%s, %s, %s)%s", args[0], args[1], args[2], args[3], suffix)
		case shaderir.TextureSampleBias:
			return fmt.Sprintf("%s.SampleBias(%s, %s, %s)", args[0], args[1], args[2], args[3])
		case shaderir.TextureSampleGrad:
			return fmt.Sprintf("%s.SampleGrad(%s, %s, %s, %s)", args[0], args[1], args[2], args[3], args[4])
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
			texIdx := e.Exprs[1].Index
			if texIdx >= 0 && texIdx < len(w.p.Bindings) && w.p.Bindings[texIdx].Type.Main == shaderir.TextureDepth2D {
				return fmt.Sprintf("%s.Load(int3(%s, %s)).x", args[0], args[1], args[2])
			}
			return fmt.Sprintf("%s.Load(int3(%s, %s))", args[0], args[1], args[2])
		case shaderir.TextureDimensions:
			return fmt.Sprintf("uint2(0u, 0u) /* %s */", args[0])
		case shaderir.Atan2:
			return fmt.Sprintf("atan2(%s)", strings.Join(args, ", "))
		default:
			var cc compileContext
			return fmt.Sprintf("%s(%s)", cc.builtinFuncString(callee.BuiltinFunc), strings.Join(args, ", "))
		}
	}
	return "0"
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
