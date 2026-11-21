package wgsl

import (
	"fmt"
	"go/constant"
	"strconv"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// PipelineProgram is a compiled 3D GPU WGSL module containing @vertex and/or
// @fragment entry points and explicit @group/@binding resource declarations.
type PipelineProgram struct {
	IR              *shaderir.Program
	VertexEntries   []string
	FragmentEntries []string
	Source          string
}

// CompilePipeline compiles a WGSL 3D pipeline module with @vertex and/or
// @fragment entry points, @location(N) stage inputs/outputs, @builtin(...),
// user structs, and @group(G) @binding(B) uniform/storage/texture/sampler
// resources into a shaderir.Program.
func CompilePipeline(src string) (*PipelineProgram, error) {
	if len(src) > MaxSourceBytes {
		return nil, &Error{Message: fmt.Sprintf("the source takes %d bytes, more than %d", len(src), MaxSourceBytes)}
	}
	var out *PipelineProgram
	err := catch(func() {
		m, err := parsePipeline(src)
		if err != nil {
			panic(bail{err.(*Error)})
		}
		out = compilePipelineModule(m, src)
	})
	if err != nil {
		e := err.(*Error)
		if e.offset < len(src) {
			e.Line, e.Column = position(src, e.offset)
		}
		return nil, e
	}
	out.IR.SourceID = shaderir.CalcSourceID([]byte(src))
	return out, nil
}

func compilePipelineModule(m *module, src string) *PipelineProgram {
	c := &checker{
		m:        m,
		user:     len(src),
		pipeline: true,
		prog:     &shaderir.Program{},
	}
	c.global = &scope{names: map[string]*symbol{}}
	c.scope = c.global
	c.uniforms = map[string]uniformInfo{}
	p := c.prog

	// 1. Aliases and structs (in source order so structs may reference aliases and earlier structs).
	for _, decl := range m.order {
		switch d := decl.(type) {
		case *aliasDecl:
			c.declare(d.name, d.pos, &symbol{kind: symType, t: c.resolveType(d.typ)})
		case *structDecl:
			st := &structType{name: d.name}
			seen := map[string]bool{}
			for _, mb := range d.members {
				if seen[mb.name] {
					fail(mb.pos, "struct %s declares member %s twice", d.name, mb.name)
				}
				seen[mb.name] = true
				st.fields = append(st.fields, structField{
					name:  mb.name,
					t:     c.resolveType(mb.typ),
					attrs: mb.attrs,
					pos:   mb.pos,
				})
			}
			c.declare(d.name, d.pos, &symbol{kind: symType, t: ty{k: kStruct, st: st}})
		}
	}

	// 2. Module-scope consts and @group/@binding resource variables.
	for _, g := range m.globals {
		if g.kind == "const" {
			if g.init == nil {
				fail(g.pos, "the const %s has no value", g.name)
			}
			c.declare(g.name, g.pos, c.constSymbol(g))
			continue
		}
		if g.kind != "var" {
			fail(g.pos, "unexpected global declaration %s", g.name)
		}
		grp, hasGrp := attrUint(g.attrs, "group")
		bnd, hasBnd := attrUint(g.attrs, "binding")
		if !hasGrp || !hasBnd {
			if g.space == "private" {
				fail(g.pos, "var<private> is not supported yet: declare the variable in the function")
			}
			fail(g.pos, "resource variable %s requires @group(G) @binding(B)", g.name)
		}
		if g.typ == nil {
			fail(g.pos, "resource variable %s requires a type", g.name)
		}
		t := c.resolveType(g.typ)
		var kind shaderir.BindingKind
		switch g.space {
		case "uniform":
			kind = shaderir.BindingUniformBuffer
		case "storage":
			if g.access != "" && g.access != "read" {
				fail(g.pos, "Stage 1 storage buffers are read-only (var<storage, read>)")
			}
			kind = shaderir.BindingStorageBufferRead
		case "":
			switch t.k {
			case kTexture2D, kTextureCube, kTexture2DArray, kTextureDepth2D, kTextureMultisampled2D:
				kind = shaderir.BindingTexture
			case kSampler, kSamplerComparison:
				kind = shaderir.BindingSampler
			default:
				fail(g.pos, "handle variable %s must be a texture or sampler, not %s", g.name, t)
			}
		default:
			fail(g.pos, "unsupported address space %s for resource %s", g.space, g.name)
		}
		for _, existing := range p.Bindings {
			if existing.Group == grp && existing.Binding == bnd {
				fail(g.pos, "duplicate resource binding @group(%d) @binding(%d)", grp, bnd)
			}
		}
		idx := len(p.Bindings)
		p.Bindings = append(p.Bindings, shaderir.Binding{
			Group:   grp,
			Binding: bnd,
			Name:    g.name,
			Kind:    kind,
			Type:    t.ir(),
		})
		c.declare(g.name, g.pos, &symbol{kind: symBinding, t: t, binding: idx})
	}

	// 3. Separate helper functions from @vertex / @fragment entry points.
	var vertFns, fragFns []*fnDecl
	for _, f := range m.fns {
		stage := entryStage(f.attrs)
		switch stage {
		case "compute":
			fail(f.pos, "@compute is reserved for Stage 2")
		case "vertex":
			vertFns = append(vertFns, f)
		case "fragment":
			fragFns = append(fragFns, f)
		default:
			info := &fnInfo{decl: f, index: len(c.fns)}
			for _, prm := range f.params {
				t := c.resolveType(prm.typ)
				if t.k == kArray {
					fail(prm.pos, "a function takes no arrays")
				}
				info.params = append(info.params, t)
			}
			if f.ret != nil {
				info.ret = c.resolveType(f.ret)
				if info.ret.k == kArray {
					fail(f.ret.pos, "a function returns no arrays")
				}
			}
			c.fns = append(c.fns, info)
			c.declare(f.name, f.pos, &symbol{kind: symFn, fn: info})
		}
	}

	if len(vertFns) == 0 && len(fragFns) == 0 {
		fail(0, "pipeline module defines no @vertex or @fragment entry point")
	}

	for _, a := range m.asserts {
		v := c.constant(a.x)
		if v.t.k != kBool || !constant.BoolVal(v.c) {
			fail(a.pos, "the assertion fails")
		}
	}

	// 4. Compile helper functions first.
	for _, f := range c.fns {
		p.Funcs = append(p.Funcs, c.function(f))
	}
	c.recursion()

	// 5. Compile @vertex and @fragment entry points.
	res := &PipelineProgram{IR: p, Source: src}
	for _, vf := range vertFns {
		res.VertexEntries = append(res.VertexEntries, vf.name)
	}
	for _, ff := range fragFns {
		res.FragmentEntries = append(res.FragmentEntries, ff.name)
	}
	if len(vertFns) > 0 {
		c.compileVertexEntry(vertFns[0])
	}
	if len(fragFns) > 0 {
		c.compileFragmentEntry(fragFns[0])
	}
	return res
}

type stageFieldBinding struct {
	fieldIdx int // -1 if whole param/return
	isPos    bool
	builtin  shaderir.StageBuiltin
	loc      uint32
	hasLoc   bool
	interp   shaderir.Interpolation
	name     string
	t        ty
	irIdx    int // IR local variable index
}

func (c *checker) compileVertexEntry(f *fnDecl) {
	p := c.prog
	p.VertexEntry = f.name

	// Collect vertex input attributes.
	type paramPlan struct {
		name   string
		pos    int
		t      ty
		fields []stageFieldBinding
	}
	var plans []paramPlan
	for _, prm := range f.params {
		t := c.resolveType(prm.typ)
		plan := paramPlan{name: prm.name, pos: prm.pos, t: t}
		if t.k == kStruct && t.st != nil {
			for i, fld := range t.st.fields {
				fb := parseStageField(fld.name, fld.t, fld.attrs, fld.pos, i)
				if fb.builtin == shaderir.BuiltinNone {
					fb.irIdx = len(p.Attributes)
					p.Attributes = append(p.Attributes, fld.t.ir())
					p.AttributeMeta = append(p.AttributeMeta, shaderir.StageIO{
						Name:          fld.name,
						Location:      fb.loc,
						Interpolation: fb.interp,
					})
				}
				plan.fields = append(plan.fields, fb)
			}
		} else {
			fb := parseStageField(prm.name, t, prm.attrs, prm.pos, -1)
			if fb.builtin == shaderir.BuiltinNone {
				fb.irIdx = len(p.Attributes)
				p.Attributes = append(p.Attributes, t.ir())
				p.AttributeMeta = append(p.AttributeMeta, shaderir.StageIO{
					Name:          prm.name,
					Location:      fb.loc,
					Interpolation: fb.interp,
				})
			}
			plan.fields = append(plan.fields, fb)
		}
		plans = append(plans, plan)
	}

	// Collect vertex output position + varyings.
	if f.ret == nil {
		fail(f.pos, "@vertex %s must return a position or output struct", f.name)
	}
	retT := c.resolveType(f.ret)
	var outFields []stageFieldBinding
	hasPos := false
	na := len(p.Attributes)
	if retT.k == kStruct && retT.st != nil {
		for i, fld := range retT.st.fields {
			fb := parseStageField(fld.name, fld.t, fld.attrs, fld.pos, i)
			if fb.builtin == shaderir.BuiltinPosition {
				fb.isPos = true
				fb.irIdx = na
				hasPos = true
			} else {
				fb.irIdx = na + 1 + len(p.Varyings)
				p.Varyings = append(p.Varyings, fld.t.ir())
				p.VaryingMeta = append(p.VaryingMeta, shaderir.StageIO{
					Name:          fld.name,
					Location:      fb.loc,
					Interpolation: fb.interp,
				})
			}
			outFields = append(outFields, fb)
		}
	} else {
		fb := parseStageField("position", retT, f.retAttrs, f.ret.pos, -1)
		if fb.builtin != shaderir.BuiltinPosition {
			fail(f.ret.pos, "@vertex %s return value must be @builtin(position)", f.name)
		}
		fb.isPos = true
		fb.irIdx = na
		hasPos = true
		outFields = append(outFields, fb)
	}
	if !hasPos {
		fail(f.pos, "@vertex %s output must include @builtin(position)", f.name)
	}

	nv := len(p.Varyings)
	pseudoCount := na + nv + 1
	top := &shaderir.Block{LocalVarIndexOffset: pseudoCount}
	info := &fnInfo{decl: f, index: -1, ret: retT}
	c.cur = &fnState{
		info:   info,
		params: pseudoCount,
		top:    top,
		stage:  "vertex",
		onReturn: func(v value, out *[]shaderir.Stmt) {
			if len(outFields) == 1 && outFields[0].fieldIdx == -1 {
				*out = append(*out, assign(local(na, vec(kF32, 4)), c.ir(v)))
				*out = append(*out, shaderir.Stmt{Type: shaderir.Return})
				return
			}
			// Store returned struct in a temp if not already a simple local.
			sv := c.reuse(v, out)
			for _, of := range outFields {
				fieldExpr := mk(shaderir.Expr{
					Type:  shaderir.FieldSelector,
					Exprs: []shaderir.Expr{c.ir(sv), {Type: shaderir.StructMember, Index: of.fieldIdx}},
				}, of.t)
				*out = append(*out, assign(local(of.irIdx, of.t), fieldExpr))
			}
			*out = append(*out, shaderir.Stmt{Type: shaderir.Return})
		},
	}

	c.push()
	for _, plan := range plans {
		if len(plan.fields) == 1 && plan.fields[0].fieldIdx == -1 {
			fb := plan.fields[0]
			if fb.builtin != shaderir.BuiltinNone {
				c.declare(plan.name, plan.pos, &symbol{
					kind: symExpr,
					t:    fb.t,
					expr: mk(shaderir.Expr{Type: shaderir.BuiltinInput, Index: int(fb.builtin)}, fb.t),
				})
			} else {
				c.declare(plan.name, plan.pos, &symbol{kind: symLocal, t: fb.t, local: fb.irIdx})
			}
			continue
		}
		// Materialize input struct local and populate its fields from attributes/builtins.
		idx := c.newLocal(plan.t)
		top.Stmts = append(top.Stmts, shaderir.Stmt{Type: shaderir.Init, InitIndex: idx})
		for _, fb := range plan.fields {
			var srcExpr shaderir.Expr
			if fb.builtin != shaderir.BuiltinNone {
				srcExpr = mk(shaderir.Expr{Type: shaderir.BuiltinInput, Index: int(fb.builtin)}, fb.t)
			} else {
				srcExpr = local(fb.irIdx, fb.t)
			}
			dstExpr := mk(shaderir.Expr{
				Type:  shaderir.FieldSelector,
				Exprs: []shaderir.Expr{local(idx, plan.t), {Type: shaderir.StructMember, Index: fb.fieldIdx}},
			}, fb.t)
			top.Stmts = append(top.Stmts, assign(dstExpr, srcExpr))
		}
		c.declare(plan.name, plan.pos, &symbol{kind: symLocal, t: plan.t, local: idx})
	}

	c.stmts(f.body.stmts, &top.Stmts)
	if !returns(f.body.stmts) {
		fail(f.pos, "%s does not return a value on every path", f.name)
	}
	c.pop()
	for _, b := range c.cur.blocks {
		b.LocalVarIndexOffset = pseudoCount + len(top.LocalVars)
	}
	c.cur = nil
	p.VertexFunc.Block = top
}

func (c *checker) compileFragmentEntry(f *fnDecl) {
	p := c.prog
	p.FragmentEntry = f.name

	hasVertex := p.VertexFunc.Block != nil
	findOrAddVarying := func(fb stageFieldBinding, pos int) int {
		for i, meta := range p.VaryingMeta {
			if meta.Location == fb.loc {
				if !p.Varyings[i].Equal(&shaderir.Type{Main: fb.t.ir().Main, Sub: fb.t.ir().Sub, Length: fb.t.ir().Length}) {
					// Keep simple main type compatibility check.
					ft := fb.t.ir()
					if !p.Varyings[i].Equal(&ft) {
						fail(pos, "@location(%d) type %s does not match vertex output %s", fb.loc, fb.t, p.Varyings[i])
					}
				}
				return i + 1
			}
		}
		if hasVertex {
			fail(pos, "@fragment input @location(%d) has no matching @vertex output", fb.loc)
		}
		idx := len(p.Varyings)
		p.Varyings = append(p.Varyings, fb.t.ir())
		p.VaryingMeta = append(p.VaryingMeta, shaderir.StageIO{
			Name:          fb.name,
			Location:      fb.loc,
			Interpolation: fb.interp,
		})
		return idx + 1
	}

	type paramPlan struct {
		name   string
		pos    int
		t      ty
		fields []stageFieldBinding
	}
	var plans []paramPlan
	for _, prm := range f.params {
		t := c.resolveType(prm.typ)
		plan := paramPlan{name: prm.name, pos: prm.pos, t: t}
		if t.k == kStruct && t.st != nil {
			for i, fld := range t.st.fields {
				fb := parseStageField(fld.name, fld.t, fld.attrs, fld.pos, i)
				if fb.builtin == shaderir.BuiltinPosition {
					fb.irIdx = 0
				} else if fb.builtin == shaderir.BuiltinNone {
					fb.irIdx = findOrAddVarying(fb, fld.pos)
				}
				plan.fields = append(plan.fields, fb)
			}
		} else {
			fb := parseStageField(prm.name, t, prm.attrs, prm.pos, -1)
			if fb.builtin == shaderir.BuiltinPosition {
				fb.irIdx = 0
			} else if fb.builtin == shaderir.BuiltinNone {
				fb.irIdx = findOrAddVarying(fb, prm.pos)
			}
			plan.fields = append(plan.fields, fb)
		}
		plans = append(plans, plan)
	}

	if f.ret == nil {
		fail(f.pos, "@fragment %s must return a color", f.name)
	}
	retT := c.resolveType(f.ret)
	colorFieldIdx := -1
	if retT.k == kStruct && retT.st != nil {
		for i, fld := range retT.st.fields {
			if loc, ok := attrUint(fld.attrs, "location"); ok && loc == 0 {
				colorFieldIdx = i
				break
			}
		}
		if colorFieldIdx < 0 {
			fail(f.ret.pos, "@fragment %s output struct must have a @location(0) color member", f.name)
		}
	} else if !retT.equal(vec(kF32, 4)) {
		fail(f.ret.pos, "@fragment %s must return vec4<f32>, not %s", f.name, retT)
	}

	nv := len(p.Varyings)
	pseudoCount := nv + 1
	top := &shaderir.Block{LocalVarIndexOffset: pseudoCount}
	info := &fnInfo{decl: f, index: -1, ret: retT}
	c.cur = &fnState{
		info:   info,
		params: pseudoCount,
		top:    top,
		stage:  "fragment",
		onReturn: func(v value, out *[]shaderir.Stmt) {
			if colorFieldIdx >= 0 {
				colorExpr := mk(shaderir.Expr{
					Type:  shaderir.FieldSelector,
					Exprs: []shaderir.Expr{c.ir(v), {Type: shaderir.StructMember, Index: colorFieldIdx}},
				}, vec(kF32, 4))
				*out = append(*out, shaderir.Stmt{Type: shaderir.Return, Exprs: []shaderir.Expr{colorExpr}})
				return
			}
			*out = append(*out, shaderir.Stmt{Type: shaderir.Return, Exprs: []shaderir.Expr{c.ir(v)}})
		},
	}

	c.push()
	for _, plan := range plans {
		if len(plan.fields) == 1 && plan.fields[0].fieldIdx == -1 {
			fb := plan.fields[0]
			if fb.builtin != shaderir.BuiltinNone && fb.builtin != shaderir.BuiltinPosition {
				c.declare(plan.name, plan.pos, &symbol{
					kind: symExpr,
					t:    fb.t,
					expr: mk(shaderir.Expr{Type: shaderir.BuiltinInput, Index: int(fb.builtin)}, fb.t),
				})
			} else {
				c.declare(plan.name, plan.pos, &symbol{kind: symLocal, t: fb.t, local: fb.irIdx})
			}
			continue
		}
		idx := c.newLocal(plan.t)
		top.Stmts = append(top.Stmts, shaderir.Stmt{Type: shaderir.Init, InitIndex: idx})
		for _, fb := range plan.fields {
			var srcExpr shaderir.Expr
			if fb.builtin != shaderir.BuiltinNone && fb.builtin != shaderir.BuiltinPosition {
				srcExpr = mk(shaderir.Expr{Type: shaderir.BuiltinInput, Index: int(fb.builtin)}, fb.t)
			} else {
				srcExpr = local(fb.irIdx, fb.t)
			}
			dstExpr := mk(shaderir.Expr{
				Type:  shaderir.FieldSelector,
				Exprs: []shaderir.Expr{local(idx, plan.t), {Type: shaderir.StructMember, Index: fb.fieldIdx}},
			}, fb.t)
			top.Stmts = append(top.Stmts, assign(dstExpr, srcExpr))
		}
		c.declare(plan.name, plan.pos, &symbol{kind: symLocal, t: plan.t, local: idx})
	}

	c.stmts(f.body.stmts, &top.Stmts)
	if !returns(f.body.stmts) {
		fail(f.pos, "%s does not return a value on every path", f.name)
	}
	c.pop()
	for _, b := range c.cur.blocks {
		b.LocalVarIndexOffset = pseudoCount + len(top.LocalVars)
	}
	c.cur = nil
	p.FragmentFunc.Block = top
}

func (c *checker) structConstruct(t ty, x *callExpr, out *[]shaderir.Stmt) value {
	st := t.st
	if len(x.args) == 0 {
		irT := t.ir()
		return value{t: t, e: shaderir.Expr{Type: shaderir.StructConstruct, StructType: &irT}}
	}
	if len(x.args) != len(st.fields) {
		fail(x.at(), "struct %s has %d members, not %d", st.name, len(st.fields), len(x.args))
	}
	args := make([]shaderir.Expr, len(st.fields))
	for i, a := range x.args {
		v := c.convert(c.expr(a, out), st.fields[i].t, a.at())
		args[i] = c.ir(v)
	}
	irT := t.ir()
	return value{t: t, e: shaderir.Expr{Type: shaderir.StructConstruct, StructType: &irT, Exprs: args}}
}

func (c *checker) pipelineTextureBuiltin(name string, x *callExpr, out *[]shaderir.Stmt) (value, bool) {
	switch name {
	case "textureSample":
		if len(x.args) < 3 || len(x.args) > 4 {
			fail(x.at(), "textureSample takes 3 or 4 arguments")
		}
		tex := c.expr(x.args[0], out)
		samp := c.expr(x.args[1], out)
		if samp.t.k == kSamplerComparison {
			fail(x.args[1].at(), "textureSample expects sampler, not sampler_comparison")
		}
		coord := c.concrete(c.expr(x.args[2], out), x.args[2].at())
		args := []shaderir.Expr{c.ir(tex), c.ir(samp), c.ir(coord)}
		if len(x.args) == 4 {
			off := c.convert(c.expr(x.args[3], out), vec(kI32, 2), x.args[3].at())
			args = append(args, c.ir(off))
		}
		retT := vec(kF32, 4)
		if tex.t.k == kTextureDepth2D {
			retT = tF32
		}
		return value{t: retT, e: builtinCall(shaderir.TextureSample, retT, args...)}, true
	case "textureSampleLevel":
		if len(x.args) < 4 || len(x.args) > 5 {
			fail(x.at(), "textureSampleLevel takes 4 or 5 arguments")
		}
		tex := c.expr(x.args[0], out)
		samp := c.expr(x.args[1], out)
		if samp.t.k == kSamplerComparison {
			fail(x.args[1].at(), "textureSampleLevel expects sampler, not sampler_comparison")
		}
		args := make([]shaderir.Expr, len(x.args))
		args[0] = c.ir(tex)
		args[1] = c.ir(samp)
		for i := 2; i < len(x.args); i++ {
			a := x.args[i]
			args[i] = c.ir(c.concrete(c.expr(a, out), a.at()))
		}
		retT := vec(kF32, 4)
		if tex.t.k == kTextureDepth2D {
			retT = tF32
		}
		return value{t: retT, e: builtinCall(shaderir.TextureSampleLevel, retT, args...)}, true
	case "textureSampleBias":
		if len(x.args) < 4 || len(x.args) > 5 {
			fail(x.at(), "textureSampleBias takes 4 or 5 arguments")
		}
		args := make([]shaderir.Expr, len(x.args))
		for i, a := range x.args {
			args[i] = c.ir(c.concrete(c.expr(a, out), a.at()))
		}
		return value{t: vec(kF32, 4), e: builtinCall(shaderir.TextureSampleBias, vec(kF32, 4), args...)}, true
	case "textureSampleGrad":
		if len(x.args) < 5 || len(x.args) > 6 {
			fail(x.at(), "textureSampleGrad takes 5 or 6 arguments")
		}
		args := make([]shaderir.Expr, len(x.args))
		for i, a := range x.args {
			args[i] = c.ir(c.concrete(c.expr(a, out), a.at()))
		}
		return value{t: vec(kF32, 4), e: builtinCall(shaderir.TextureSampleGrad, vec(kF32, 4), args...)}, true
	case "textureSampleCompare", "textureSampleCompareLevel":
		if len(x.args) < 4 || len(x.args) > 5 {
			fail(x.at(), "%s takes 4 or 5 arguments", name)
		}
		tex := c.expr(x.args[0], out)
		if tex.t.k != kTextureDepth2D {
			fail(x.args[0].at(), "%s expects texture_depth_2d, not %s", name, tex.t)
		}
		samp := c.expr(x.args[1], out)
		if samp.t.k != kSamplerComparison {
			fail(x.args[1].at(), "%s expects sampler_comparison, not %s", name, samp.t)
		}
		coord := c.convert(c.expr(x.args[2], out), vec(kF32, 2), x.args[2].at())
		ref := c.convert(c.expr(x.args[3], out), tF32, x.args[3].at())
		args := []shaderir.Expr{c.ir(tex), c.ir(samp), c.ir(coord), c.ir(ref)}
		if len(x.args) == 5 {
			off := c.convert(c.expr(x.args[4], out), vec(kI32, 2), x.args[4].at())
			args = append(args, c.ir(off))
		}
		fn := shaderir.TextureSampleCompare
		if name == "textureSampleCompareLevel" {
			fn = shaderir.TextureSampleCompareLevel
		}
		return value{t: tF32, e: builtinCall(fn, tF32, args...)}, true
	case "textureLoad":
		if len(x.args) < 3 || len(x.args) > 4 {
			fail(x.at(), "textureLoad takes 3 or 4 arguments")
		}
		tex := c.expr(x.args[0], out)
		args := make([]shaderir.Expr, len(x.args))
		args[0] = c.ir(tex)
		for i := 1; i < len(x.args); i++ {
			a := x.args[i]
			args[i] = c.ir(c.concrete(c.expr(a, out), a.at()))
		}
		retT := vec(kF32, 4)
		if tex.t.k == kTextureDepth2D {
			retT = tF32
		}
		return value{t: retT, e: builtinCall(shaderir.TextureLoad, retT, args...)}, true
	case "textureDimensions":
		if len(x.args) < 1 || len(x.args) > 2 {
			fail(x.at(), "textureDimensions takes 1 or 2 arguments")
		}
		args := make([]shaderir.Expr, len(x.args))
		for i, a := range x.args {
			args[i] = c.ir(c.concrete(c.expr(a, out), a.at()))
		}
		return value{t: vec(kU32, 2), e: builtinCall(shaderir.TextureDimensions, vec(kU32, 2), args...)}, true
	}
	return value{}, false
}

func entryStage(attrs []attr) string {
	for _, a := range attrs {
		if a.name == "vertex" || a.name == "fragment" || a.name == "compute" {
			return a.name
		}
	}
	return ""
}

func attrUint(attrs []attr, name string) (uint32, bool) {
	for _, a := range attrs {
		if a.name == name {
			if len(a.args) != 1 {
				fail(a.pos, "@%s takes one integer argument", name)
			}
			v, err := strconv.ParseUint(a.args[0], 10, 32)
			if err != nil {
				fail(a.pos, "@%s argument %q is not a non-negative integer", name, a.args[0])
			}
			return uint32(v), true
		}
	}
	return 0, false
}

func attrString(attrs []attr, name string) (string, bool) {
	for _, a := range attrs {
		if a.name == name && len(a.args) >= 1 {
			return a.args[0], true
		}
	}
	return "", false
}

func parseStageField(name string, t ty, attrs []attr, pos int, fieldIdx int) stageFieldBinding {
	fb := stageFieldBinding{fieldIdx: fieldIdx, name: name, t: t}
	if b, ok := attrString(attrs, "builtin"); ok {
		switch b {
		case "position":
			if !t.equal(vec(kF32, 4)) {
				fail(pos, "@builtin(position) must have type vec4<f32>, not %s", t)
			}
			fb.builtin = shaderir.BuiltinPosition
		case "vertex_index":
			if t.k != kU32 {
				fail(pos, "@builtin(vertex_index) must have type u32, not %s", t)
			}
			fb.builtin = shaderir.BuiltinVertexIndex
		case "instance_index":
			if t.k != kU32 {
				fail(pos, "@builtin(instance_index) must have type u32, not %s", t)
			}
			fb.builtin = shaderir.BuiltinInstanceIndex
		case "front_facing":
			if t.k != kBool {
				fail(pos, "@builtin(front_facing) must have type bool, not %s", t)
			}
			fb.builtin = shaderir.BuiltinFrontFacing
		case "frag_depth":
			if t.k != kF32 {
				fail(pos, "@builtin(frag_depth) must have type f32, not %s", t)
			}
			fb.builtin = shaderir.BuiltinFragDepth
		default:
			fail(pos, "unsupported @builtin(%s)", b)
		}
		return fb
	}
	loc, ok := attrUint(attrs, "location")
	if !ok {
		fail(pos, "stage input/output %s requires @location(N) or @builtin(...)", name)
	}
	fb.loc = loc
	fb.hasLoc = true
	if interp, ok := attrString(attrs, "interpolate"); ok {
		switch interp {
		case "flat":
			fb.interp = shaderir.InterpFlat
		case "linear":
			fb.interp = shaderir.InterpLinear
		case "perspective":
			fb.interp = shaderir.InterpPerspective
		default:
			fail(pos, "unsupported @interpolate(%s)", interp)
		}
	}
	return fb
}
