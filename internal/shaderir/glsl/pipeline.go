package glsl

import (
	"fmt"
	"go/constant"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// UniformBlockSlot records the GLSL uniform block name and flattened binding
// slot for a @group(G) @binding(B) uniform buffer.
type UniformBlockSlot struct {
	Group     uint32
	Binding   uint32
	BlockName string
	Slot      uint32
}

// SamplerPair records one synthesized GLSL combined sampler uniform for a
// static (texture, sampler) binding pair referenced by sampling calls.
type SamplerPair struct {
	Name           string
	Unit           uint32
	TextureGroup   uint32
	TextureBinding uint32
	SamplerGroup   uint32
	SamplerBinding uint32
	HasSampler     bool
	TextureType    shaderir.BasicType
	Comparison     bool
}

// PipelineSource holds GLSL 3.30 or GLSL ES 3.00 vertex and fragment shader
// sources plus uniform block and combined-sampler binding tables.
type PipelineSource struct {
	VertexSource   string
	FragmentSource string
	UniformBlocks  []UniformBlockSlot
	SamplerPairs   []SamplerPair
}

// CompilePipeline emits GLSL 3.30 Core or GLSL ES 3.00 vertex and fragment
// shaders for a 3D GPU pipeline program. Vertex shaders automatically remap
// canonical WebGPU [0, 1] clip-space depth to OpenGL [-1, 1] before exit.
func CompilePipeline(p *shaderir.Program, version GLSLVersion) (PipelineSource, error) {
	if p == nil {
		return PipelineSource{}, fmt.Errorf("glsl: nil program")
	}
	w := &pipeWriter{
		p:           p,
		version:     version,
		structNames: map[string]string{},
		pairByKey:   map[[2]int]string{},
	}
	var slot uint32
	for i, b := range p.Bindings {
		if b.Kind == shaderir.BindingUniformBuffer {
			w.uniformBlocks = append(w.uniformBlocks, UniformBlockSlot{
				Group:     b.Group,
				Binding:   b.Binding,
				BlockName: fmt.Sprintf("UB%d", i),
				Slot:      slot,
			})
			slot++
		}
		if b.Type.Main == shaderir.Struct {
			w.structName(b.Type)
		}
	}

	var helperLines []string
	for _, f := range p.Funcs {
		helperLines = append(helperLines, w.emitHelper(f)...)
	}

	var vsBody, fsBody []string
	if p.VertexFunc.Block != nil {
		vsBody = w.emitVertex()
	}
	if p.FragmentFunc.Block != nil {
		fsBody = w.emitFragment()
	}

	buildStage := func(isVert bool, body []string) string {
		if len(body) == 0 {
			return ""
		}
		var lines []string
		hasShadow := false
		hasArray := false
		for _, sp := range w.samplerPairs {
			if sp.Comparison {
				hasShadow = true
			}
			if sp.TextureType == shaderir.Texture2DArray {
				hasArray = true
			}
		}
		if version == GLSLVersionES300 {
			lines = append(lines, "#version 300 es", "precision highp float;", "precision highp int;")
			if hasShadow {
				lines = append(lines, "precision highp sampler2DShadow;")
			}
			if hasArray {
				lines = append(lines, "precision highp sampler2DArray;")
			}
			lines = append(lines, "")
		} else {
			lines = append(lines, "#version 330 core", "")
		}
		for i, st := range w.structTypes {
			lines = append(lines, fmt.Sprintf("struct S%d {", i))
			for j, m := range st.Sub {
				lines = append(lines, fmt.Sprintf("\t%s;", w.declVar(m, fmt.Sprintf("M%d", j))))
			}
			lines = append(lines, "};", "")
		}
		for i, b := range p.Bindings {
			if b.Kind == shaderir.BindingUniformBuffer {
				lines = append(lines, fmt.Sprintf("layout(std140) uniform UB%d {", i))
				lines = append(lines, fmt.Sprintf("\t%s;", w.declVar(b.Type, fmt.Sprintf("B%d", i))))
				lines = append(lines, "};")
			}
		}
		for _, sp := range w.samplerPairs {
			glslType := "sampler2D"
			switch {
			case sp.Comparison:
				glslType = "sampler2DShadow"
			case sp.TextureType == shaderir.TextureCube:
				glslType = "samplerCube"
			case sp.TextureType == shaderir.Texture2DArray:
				glslType = "sampler2DArray"
			}
			lines = append(lines, fmt.Sprintf("uniform %s %s;", glslType, sp.Name))
		}
		lines = append(lines, "")
		if isVert {
			for i, a := range p.Attributes {
				loc := uint32(i)
				if i < len(p.AttributeMeta) {
					loc = p.AttributeMeta[i].Location
				}
				lines = append(lines, fmt.Sprintf("layout(location = %d) in %s;", loc, w.declVar(a, fmt.Sprintf("a%d", i))))
			}
			for i, v := range p.Varyings {
				qual := ""
				if i < len(p.VaryingMeta) && p.VaryingMeta[i].Interpolation == shaderir.InterpFlat {
					qual = "flat "
				}
				lines = append(lines, fmt.Sprintf("%sout %s;", qual, w.declVar(v, fmt.Sprintf("v%d", i))))
			}
		} else {
			for i, v := range p.Varyings {
				qual := ""
				if i < len(p.VaryingMeta) && p.VaryingMeta[i].Interpolation == shaderir.InterpFlat {
					qual = "flat "
				}
				lines = append(lines, fmt.Sprintf("%sin %s;", qual, w.declVar(v, fmt.Sprintf("v%d", i))))
			}
			lines = append(lines, "layout(location = 0) out vec4 fragColor;")
		}
		lines = append(lines, "")
		lines = append(lines, helperLines...)
		lines = append(lines, body...)
		return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
	}

	return PipelineSource{
		VertexSource:   buildStage(true, vsBody),
		FragmentSource: buildStage(false, fsBody),
		UniformBlocks:  w.uniformBlocks,
		SamplerPairs:   w.samplerPairs,
	}, nil
}

type pipeWriter struct {
	p             *shaderir.Program
	version       GLSLVersion
	structNames   map[string]string
	structTypes   []shaderir.Type
	uniformBlocks []UniformBlockSlot
	samplerPairs  []SamplerPair
	pairByKey     map[[2]int]string
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
	if t.Main == shaderir.Struct {
		return w.structName(t)
	}
	return basicTypeString(t.Main)
}

func (w *pipeWriter) declVar(t shaderir.Type, name string) string {
	if t.Main == shaderir.Array {
		return fmt.Sprintf("%s %s[%d]", w.typeStr(t.Sub[0]), name, t.Length)
	}
	return fmt.Sprintf("%s %s", w.typeStr(t), name)
}

func (w *pipeWriter) registerSamplerPair(texIdx, sampIdx int, comparison bool) string {
	key := [2]int{texIdx, sampIdx}
	if name, ok := w.pairByKey[key]; ok {
		return name
	}
	unit := uint32(len(w.samplerPairs))
	name := fmt.Sprintf("S_t%d_s%d", texIdx, sampIdx)
	if sampIdx < 0 {
		name = fmt.Sprintf("S_t%d_raw", texIdx)
	}
	w.pairByKey[key] = name
	var tg, tb, sg, sb uint32
	var texType shaderir.BasicType
	if texIdx >= 0 && texIdx < len(w.p.Bindings) {
		tg, tb = w.p.Bindings[texIdx].Group, w.p.Bindings[texIdx].Binding
		texType = w.p.Bindings[texIdx].Type.Main
	}
	if sampIdx >= 0 && sampIdx < len(w.p.Bindings) {
		sg, sb = w.p.Bindings[sampIdx].Group, w.p.Bindings[sampIdx].Binding
		if w.p.Bindings[sampIdx].Type.Main == shaderir.SamplerComparison {
			comparison = true
		}
	}
	w.samplerPairs = append(w.samplerPairs, SamplerPair{
		Name:           name,
		Unit:           unit,
		TextureGroup:   tg,
		TextureBinding: tb,
		SamplerGroup:   sg,
		SamplerBinding: sb,
		HasSampler:     sampIdx >= 0,
		TextureType:    texType,
		Comparison:     comparison,
	})
	return name
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

func (w *pipeWriter) emitVertex() []string {
	lines := []string{"void main() {"}
	lines = append(lines, w.emitBlock(w.p.VertexFunc.Block, w.p.VertexFunc.Block, 1, "vertex")...)
	lines = append(lines, "\tgl_Position.z = gl_Position.z * 2.0 - gl_Position.w;")
	lines = append(lines, "}", "")
	return lines
}

func (w *pipeWriter) emitFragment() []string {
	lines := []string{"void main() {"}
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
			return fmt.Sprintf("a%d", idx)
		case idx == na:
			return "gl_Position"
		case idx < na+nv+1:
			return fmt.Sprintf("v%d", idx-na-1)
		}
	case "fragment":
		nv := len(w.p.Varyings)
		switch {
		case idx == 0:
			return "gl_FragCoord"
		case idx < nv+1:
			return fmt.Sprintf("v%d", idx-1)
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
			lines = append(lines, fmt.Sprintf("%s%s;", indent, w.declVar(lt, fmt.Sprintf("l%d", b.LocalVarIndexOffset+i))))
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
				initStr = glslConstLit(s.ForInit, s.ForVarType.Main == shaderir.Uint)
				endStr = glslConstLit(s.ForEnd, s.ForVarType.Main == shaderir.Uint)
			}
			deltaStr := glslConstLit(s.ForDelta, s.ForVarType.Main == shaderir.Uint)
			lines = append(lines, fmt.Sprintf("%sfor (%s = %s; %s %s %s; %s += %s) {",
				indent, w.declVar(s.ForVarType, v), initStr, v, opString(s.ForOp), endStr, v, deltaStr))
			lines = append(lines, w.emitBlock(top, s.Blocks[0], level+1, stage)...)
			lines = append(lines, indent+"}")
		case shaderir.Continue:
			lines = append(lines, indent+"continue;")
		case shaderir.Break:
			lines = append(lines, indent+"break;")
		case shaderir.Return:
			switch stage {
			case "vertex":
				lines = append(lines, indent+"gl_Position.z = gl_Position.z * 2.0 - gl_Position.w;")
				lines = append(lines, indent+"return;")
			case "fragment":
				if len(s.Exprs) > 0 {
					lines = append(lines, fmt.Sprintf("%sfragColor = %s;", indent, w.emitExpr(s.Exprs[0], stage)))
				}
				lines = append(lines, indent+"return;")
			default:
				if len(s.Exprs) == 0 {
					lines = append(lines, indent+"return;")
				} else {
					lines = append(lines, fmt.Sprintf("%sreturn %s;", indent, w.emitExpr(s.Exprs[0], stage)))
				}
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
		return glslConstLit(e.Const, e.Unsigned)
	case shaderir.LocalVariable:
		return w.localName(stage, e.Index)
	case shaderir.BindingVariable:
		return fmt.Sprintf("B%d", e.Index)
	case shaderir.BuiltinInput:
		switch shaderir.StageBuiltin(e.Index) {
		case shaderir.BuiltinVertexIndex:
			return "uint(gl_VertexID)"
		case shaderir.BuiltinInstanceIndex:
			return "uint(gl_InstanceID)"
		case shaderir.BuiltinFrontFacing:
			return "gl_FrontFacing"
		case shaderir.BuiltinPosition:
			return "gl_FragCoord"
		}
	case shaderir.StructConstruct:
		name := w.typeStr(*e.StructType)
		args := make([]string, len(e.Exprs))
		for i, a := range e.Exprs {
			args[i] = w.emitExpr(a, stage)
		}
		return fmt.Sprintf("%s(%s)", name, strings.Join(args, ", "))
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
		if callee.Type == shaderir.FunctionExpr {
			args := make([]string, len(e.Exprs)-1)
			for i, a := range e.Exprs[1:] {
				args[i] = w.emitExpr(a, stage)
			}
			return fmt.Sprintf("F%d(%s)", callee.Index, strings.Join(args, ", "))
		}
		switch callee.BuiltinFunc {
		case shaderir.TextureSample:
			texIdx := e.Exprs[1].Index
			sampIdx := e.Exprs[2].Index
			comb := w.registerSamplerPair(texIdx, sampIdx, false)
			suffix := ""
			if texIdx >= 0 && texIdx < len(w.p.Bindings) && w.p.Bindings[texIdx].Type.Main == shaderir.TextureDepth2D {
				suffix = ".r"
			}
			if len(e.Exprs) >= 5 {
				return fmt.Sprintf("textureOffset(%s, %s, %s)%s", comb, w.emitExpr(e.Exprs[3], stage), w.emitExpr(e.Exprs[4], stage), suffix)
			}
			return fmt.Sprintf("texture(%s, %s)%s", comb, w.emitExpr(e.Exprs[3], stage), suffix)
		case shaderir.TextureSampleLevel:
			texIdx := e.Exprs[1].Index
			comb := w.registerSamplerPair(texIdx, e.Exprs[2].Index, false)
			lod := w.emitExpr(e.Exprs[4], stage)
			suffix := ""
			if texIdx >= 0 && texIdx < len(w.p.Bindings) && w.p.Bindings[texIdx].Type.Main == shaderir.TextureDepth2D {
				lod = fmt.Sprintf("float(%s)", lod)
				suffix = ".r"
			}
			if len(e.Exprs) >= 6 {
				return fmt.Sprintf("textureLodOffset(%s, %s, %s, %s)%s", comb, w.emitExpr(e.Exprs[3], stage), lod, w.emitExpr(e.Exprs[5], stage), suffix)
			}
			return fmt.Sprintf("textureLod(%s, %s, %s)%s", comb, w.emitExpr(e.Exprs[3], stage), lod, suffix)
		case shaderir.TextureSampleBias:
			comb := w.registerSamplerPair(e.Exprs[1].Index, e.Exprs[2].Index, false)
			return fmt.Sprintf("texture(%s, %s, %s)", comb, w.emitExpr(e.Exprs[3], stage), w.emitExpr(e.Exprs[4], stage))
		case shaderir.TextureSampleGrad:
			comb := w.registerSamplerPair(e.Exprs[1].Index, e.Exprs[2].Index, false)
			return fmt.Sprintf("textureGrad(%s, %s, %s, %s)", comb, w.emitExpr(e.Exprs[3], stage), w.emitExpr(e.Exprs[4], stage), w.emitExpr(e.Exprs[5], stage))
		case shaderir.TextureSampleCompare:
			comb := w.registerSamplerPair(e.Exprs[1].Index, e.Exprs[2].Index, true)
			if len(e.Exprs) >= 6 {
				return fmt.Sprintf("textureOffset(%s, vec3(%s, %s), %s)", comb, w.emitExpr(e.Exprs[3], stage), w.emitExpr(e.Exprs[4], stage), w.emitExpr(e.Exprs[5], stage))
			}
			return fmt.Sprintf("texture(%s, vec3(%s, %s))", comb, w.emitExpr(e.Exprs[3], stage), w.emitExpr(e.Exprs[4], stage))
		case shaderir.TextureSampleCompareLevel:
			comb := w.registerSamplerPair(e.Exprs[1].Index, e.Exprs[2].Index, true)
			if len(e.Exprs) >= 6 {
				return fmt.Sprintf("textureGradOffset(%s, vec3(%s, %s), vec2(0.0), vec2(0.0), %s)", comb, w.emitExpr(e.Exprs[3], stage), w.emitExpr(e.Exprs[4], stage), w.emitExpr(e.Exprs[5], stage))
			}
			return fmt.Sprintf("textureGrad(%s, vec3(%s, %s), vec2(0.0), vec2(0.0))", comb, w.emitExpr(e.Exprs[3], stage), w.emitExpr(e.Exprs[4], stage))
		case shaderir.TextureLoad:
			texIdx := e.Exprs[1].Index
			comb := w.registerSamplerPair(texIdx, -1, false)
			if texIdx >= 0 && texIdx < len(w.p.Bindings) && w.p.Bindings[texIdx].Type.Main == shaderir.TextureDepth2D {
				return fmt.Sprintf("texelFetch(%s, ivec2(%s), int(%s)).r", comb, w.emitExpr(e.Exprs[2], stage), w.emitExpr(e.Exprs[3], stage))
			}
			return fmt.Sprintf("texelFetch(%s, ivec2(%s), int(%s))", comb, w.emitExpr(e.Exprs[2], stage), w.emitExpr(e.Exprs[3], stage))
		case shaderir.TextureDimensions:
			comb := w.registerSamplerPair(e.Exprs[1].Index, -1, false)
			return fmt.Sprintf("uvec2(textureSize(%s, 0))", comb)
		default:
			args := make([]string, len(e.Exprs)-1)
			for i, a := range e.Exprs[1:] {
				args[i] = w.emitExpr(a, stage)
			}
			var cc compileContext
			return fmt.Sprintf("%s(%s)", cc.builtinFuncString(callee.BuiltinFunc), strings.Join(args, ", "))
		}
	}
	return "0"
}

func glslConstLit(c constant.Value, unsigned bool) string {
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
