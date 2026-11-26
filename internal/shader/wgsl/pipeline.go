package wgsl

import (
	"fmt"
	"go/constant"
	"strconv"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// PipelineProgram is a compiled 3D GPU WGSL module containing @vertex, @fragment, and/or @compute entry points and explicit @group/@binding resource declarations.
type PipelineProgram struct {
	IR              *shaderir.Program
	VertexEntries   []string
	FragmentEntries []string
	ComputeEntries  []string
	WorkgroupSize   [3]uint32
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
			for i, mb := range d.members {
				if seen[mb.name] {
					fail(mb.pos, "struct %s declares member %s twice", d.name, mb.name)
				}
				seen[mb.name] = true
				ft := c.resolveType(mb.typ)
				if ft.k == kArray && ft.n == 0 && i != len(d.members)-1 {
					fail(mb.pos, "runtime-sized array must be the last member of struct %s", d.name)
				}
				st.fields = append(st.fields, structField{
					name:  mb.name,
					t:     ft,
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
		var mutable bool
		var storageFmt shaderir.StorageTextureFormat
		var storageAcc shaderir.StorageAccess
		switch g.space {
		case "uniform":
			if t.k == kArray && t.n == 0 {
				fail(g.pos, "uniform buffer %s cannot have a runtime-sized array type", g.name)
			}
			if member, reason, ok := uniformLayoutError(t.ir(), t.String()); ok {
				if member == "" {
					fail(g.pos, "uniform buffer %s %s", g.name, reason)
				}
				fail(g.pos, "uniform buffer %s member %s %s", g.name, member, reason)
			}
			kind = shaderir.BindingUniformBuffer
		case "storage":
			switch g.access {
			case "", "read":
				kind = shaderir.BindingStorageBufferRead
				storageAcc = shaderir.StorageAccessRead
			case "read_write":
				kind = shaderir.BindingStorageBufferReadWrite
				storageAcc = shaderir.StorageAccessReadWrite
				mutable = true
			default:
				fail(g.pos, "unsupported storage buffer access mode %q (expected read or read_write)", g.access)
			}
		case "":
			switch t.k {
			case kTexture2D, kTextureCube, kTexture2DArray, kTextureDepth2D, kTextureMultisampled2D:
				kind = shaderir.BindingTexture
			case kTextureStorage2D:
				kind = shaderir.BindingStorageTexture
				storageFmt = t.storageFmt
				storageAcc = t.storageAccess
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
			Group:         grp,
			Binding:       bnd,
			Name:          g.name,
			Kind:          kind,
			Type:          t.ir(),
			StorageFormat: storageFmt,
			StorageAccess: storageAcc,
		})
		c.declare(g.name, g.pos, &symbol{kind: symBinding, t: t, binding: idx, mutable: mutable})
	}

	// 3. Separate helper functions from @vertex / @fragment / @compute entry points.
	var vertFns, fragFns, compFns []*fnDecl
	for _, f := range m.fns {
		stage := entryStage(f.attrs)
		switch stage {
		case "compute":
			compFns = append(compFns, f)
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

	if len(vertFns) == 0 && len(fragFns) == 0 && len(compFns) == 0 {
		fail(0, "pipeline module defines no @vertex, @fragment, or @compute entry point")
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

	// 5. Compile @vertex, @fragment, and @compute entry points.
	res := &PipelineProgram{IR: p, Source: src}
	for _, vf := range vertFns {
		res.VertexEntries = append(res.VertexEntries, vf.name)
	}
	for _, ff := range fragFns {
		res.FragmentEntries = append(res.FragmentEntries, ff.name)
	}
	for _, cf := range compFns {
		res.ComputeEntries = append(res.ComputeEntries, cf.name)
	}
	if len(vertFns) > 0 {
		c.compileVertexEntry(vertFns[0])
	}
	if len(fragFns) > 0 {
		c.compileFragmentEntry(fragFns[0])
	}
	if len(compFns) > 0 {
		c.compileComputeEntry(compFns[0])
		res.WorkgroupSize = p.ComputeFunc.WorkgroupSize
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
	nv := len(p.Varyings)
	var outFields []stageFieldBinding
	isFragColorType := func(t ty) bool {
		if t.k == kF32 || t.k == kI32 || t.k == kU32 {
			return true
		}
		return t.k == kVec && (t.elem == kF32 || t.elem == kI32 || t.elem == kU32)
	}

	if retT.k == kStruct && retT.st != nil {
		seenLoc := map[uint32]bool{}
		for i, fld := range retT.st.fields {
			fb := parseStageField(fld.name, fld.t, fld.attrs, fld.pos, i)
			if fb.builtin != shaderir.BuiltinNone {
				fail(fld.pos, "@fragment %s output builtin @builtin(%s) is not supported yet", f.name, fb.builtin)
			}
			if !isFragColorType(fld.t) {
				fail(fld.pos, "@fragment %s output %s must be a scalar or vector color type, not %s", f.name, fld.name, fld.t)
			}
			if seenLoc[fb.loc] {
				fail(fld.pos, "@fragment %s output declares duplicate @location(%d)", f.name, fb.loc)
			}
			seenLoc[fb.loc] = true
			fb.irIdx = nv + 1 + len(p.FragmentOutputs)
			p.FragmentOutputs = append(p.FragmentOutputs, fld.t.ir())
			p.FragmentOutputMeta = append(p.FragmentOutputMeta, shaderir.StageIO{
				Name:     fld.name,
				Location: fb.loc,
			})
			outFields = append(outFields, fb)
		}
		if len(outFields) == 0 {
			fail(f.ret.pos, "@fragment %s output struct must have at least one @location(N) color member", f.name)
		}
	} else {
		if !isFragColorType(retT) {
			fail(f.ret.pos, "@fragment %s must return a color scalar or vector, not %s", f.name, retT)
		}
		loc, hasLoc := attrUint(f.retAttrs, "location")
		if hasLoc && (loc != 0 || !retT.equal(vec(kF32, 4))) {
			fb := stageFieldBinding{
				fieldIdx: -1,
				loc:      loc,
				hasLoc:   true,
				name:     "color",
				t:        retT,
				irIdx:    nv + 1,
			}
			p.FragmentOutputs = append(p.FragmentOutputs, retT.ir())
			p.FragmentOutputMeta = append(p.FragmentOutputMeta, shaderir.StageIO{
				Name:     "color",
				Location: loc,
			})
			outFields = append(outFields, fb)
		} else if !retT.equal(vec(kF32, 4)) {
			fb := stageFieldBinding{
				fieldIdx: -1,
				loc:      0,
				hasLoc:   true,
				name:     "color",
				t:        retT,
				irIdx:    nv + 1,
			}
			p.FragmentOutputs = append(p.FragmentOutputs, retT.ir())
			p.FragmentOutputMeta = append(p.FragmentOutputMeta, shaderir.StageIO{
				Name:     "color",
				Location: 0,
			})
			outFields = append(outFields, fb)
		}
	}

	pseudoCount := nv + 1 + len(p.FragmentOutputs)
	top := &shaderir.Block{LocalVarIndexOffset: pseudoCount}
	info := &fnInfo{decl: f, index: -1, ret: retT}
	c.cur = &fnState{
		info:   info,
		params: pseudoCount,
		top:    top,
		stage:  "fragment",
		onReturn: func(v value, out *[]shaderir.Stmt) {
			if len(outFields) == 0 {
				*out = append(*out, shaderir.Stmt{Type: shaderir.Return, Exprs: []shaderir.Expr{c.ir(v)}})
				return
			}
			if len(outFields) == 1 && outFields[0].fieldIdx == -1 {
				of := outFields[0]
				*out = append(*out, assign(local(of.irIdx, of.t), c.ir(v)))
				*out = append(*out, shaderir.Stmt{Type: shaderir.Return})
				return
			}
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

func (c *checker) compileComputeEntry(f *fnDecl) {
	p := c.prog
	p.ComputeEntry = f.name

	if f.ret != nil {
		fail(f.ret.pos, "@compute %s must not return a value", f.name)
	}
	ws := parseWorkgroupSize(f.attrs, f.pos, f.name)
	p.ComputeFunc.WorkgroupSize = ws

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
				fb := parseComputeField(fld.name, fld.t, fld.attrs, fld.pos, i)
				plan.fields = append(plan.fields, fb)
			}
		} else {
			fb := parseComputeField(prm.name, t, prm.attrs, prm.pos, -1)
			plan.fields = append(plan.fields, fb)
		}
		plans = append(plans, plan)
	}

	top := &shaderir.Block{LocalVarIndexOffset: 0}
	info := &fnInfo{decl: f, index: -1, ret: tNone}
	c.cur = &fnState{
		info:   info,
		params: 0,
		top:    top,
		stage:  "compute",
	}

	c.push()
	for _, plan := range plans {
		if len(plan.fields) == 1 && plan.fields[0].fieldIdx == -1 {
			fb := plan.fields[0]
			c.declare(plan.name, plan.pos, &symbol{
				kind: symExpr,
				t:    fb.t,
				expr: mk(shaderir.Expr{Type: shaderir.BuiltinInput, Index: int(fb.builtin)}, fb.t),
			})
			continue
		}
		idx := c.newLocal(plan.t)
		top.Stmts = append(top.Stmts, shaderir.Stmt{Type: shaderir.Init, InitIndex: idx})
		for _, fb := range plan.fields {
			srcExpr := mk(shaderir.Expr{Type: shaderir.BuiltinInput, Index: int(fb.builtin)}, fb.t)
			dstExpr := mk(shaderir.Expr{
				Type:  shaderir.FieldSelector,
				Exprs: []shaderir.Expr{local(idx, plan.t), {Type: shaderir.StructMember, Index: fb.fieldIdx}},
			}, fb.t)
			top.Stmts = append(top.Stmts, assign(dstExpr, srcExpr))
		}
		c.declare(plan.name, plan.pos, &symbol{kind: symLocal, t: plan.t, local: idx})
	}

	c.stmts(f.body.stmts, &top.Stmts)
	c.pop()
	for _, b := range c.cur.blocks {
		b.LocalVarIndexOffset = len(top.LocalVars)
	}
	c.cur = nil
	p.ComputeFunc.Block = top
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
	checkLayer := func(arg expr) shaderir.Expr {
		v := c.concrete(c.expr(arg, out), arg.at())
		if v.t.k != kI32 && v.t.k != kU32 {
			fail(arg.at(), "%s array layer and level must be i32 or u32, not %s", name, v.t)
		}
		return c.ir(v)
	}
	checkCoord := func(texT ty, arg expr) shaderir.Expr {
		var want ty
		switch texT.k {
		case kTexture2D, kTexture2DArray, kTextureDepth2D:
			want = vec(kF32, 2)
		case kTextureCube:
			want = vec(kF32, 3)
		default:
			fail(arg.at(), "%s does not support texture type %s", name, texT)
		}
		return c.ir(c.convert(c.expr(arg, out), want, arg.at()))
	}
	checkTexAndSampler := func(comp bool) (ty, shaderir.Expr, shaderir.Expr) {
		tex := c.expr(x.args[0], out)
		switch tex.t.k {
		case kTexture2D, kTextureCube, kTexture2DArray, kTextureDepth2D:
		default:
			fail(x.args[0].at(), "%s first argument must be a sampled texture, not %s", name, tex.t)
		}
		samp := c.expr(x.args[1], out)
		if comp {
			if samp.t.k != kSamplerComparison {
				fail(x.args[1].at(), "%s second argument must be sampler_comparison, not %s", name, samp.t)
			}
		} else if samp.t.k != kSampler {
			fail(x.args[1].at(), "%s second argument must be sampler, not %s", name, samp.t)
		}
		return tex.t, c.ir(tex), c.ir(samp)
	}

	offsetArg := func(arg expr) shaderir.Expr {
		return c.ir(c.convert(c.expr(arg, out), vec(kI32, 2), arg.at()))
	}
	f32Arg := func(arg expr) shaderir.Expr {
		return c.ir(c.convert(c.expr(arg, out), tF32, arg.at()))
	}
	sampledRet := func(texT ty) ty {
		if texT.k == kTextureDepth2D {
			return tF32
		}
		return vec(kF32, 4)
	}

	// IR argument order: (texture, sampler, coords[, layer for texture_2d_array], extra...). Emitters and the software VM tell the array layer apart from a 2D offset by the texture type.
	switch name {
	case "textureSample":
		if len(x.args) < 3 || len(x.args) > 4 {
			fail(x.at(), "textureSample takes 3 or 4 arguments")
		}
		texT, texIR, sampIR := checkTexAndSampler(false)
		args := []shaderir.Expr{texIR, sampIR, checkCoord(texT, x.args[2])}
		switch {
		case texT.k == kTexture2DArray:
			if len(x.args) != 4 {
				fail(x.at(), "textureSample on texture_2d_array<f32> takes 4 arguments (texture, sampler, uv, layer)")
			}
			args = append(args, checkLayer(x.args[3]))
		case len(x.args) == 4:
			if texT.k == kTextureCube {
				fail(x.args[3].at(), "textureSample on texture_cube<f32> takes no offset")
			}
			args = append(args, offsetArg(x.args[3]))
		}
		retT := sampledRet(texT)
		return value{t: retT, e: builtinCall(shaderir.TextureSample, retT, args...)}, true
	case "textureSampleLevel":
		if len(x.args) < 4 || len(x.args) > 5 {
			fail(x.at(), "textureSampleLevel takes 4 or 5 arguments")
		}
		texT, texIR, sampIR := checkTexAndSampler(false)
		args := []shaderir.Expr{texIR, sampIR, checkCoord(texT, x.args[2])}
		switch texT.k {
		case kTexture2DArray:
			if len(x.args) != 5 {
				fail(x.at(), "textureSampleLevel on texture_2d_array<f32> takes 5 arguments (texture, sampler, uv, layer, level)")
			}
			args = append(args, checkLayer(x.args[3]), f32Arg(x.args[4]))
		case kTextureDepth2D:
			// texture_depth_2d takes an integer level, as on main.
			args = append(args, c.ir(c.concrete(c.expr(x.args[3], out), x.args[3].at())))
			if len(x.args) == 5 {
				args = append(args, offsetArg(x.args[4]))
			}
		default:
			args = append(args, f32Arg(x.args[3]))
			if len(x.args) == 5 {
				if texT.k == kTextureCube {
					fail(x.args[4].at(), "textureSampleLevel on texture_cube<f32> takes no offset")
				}
				args = append(args, offsetArg(x.args[4]))
			}
		}
		retT := sampledRet(texT)
		return value{t: retT, e: builtinCall(shaderir.TextureSampleLevel, retT, args...)}, true
	case "textureSampleBias":
		if len(x.args) < 4 || len(x.args) > 5 {
			fail(x.at(), "textureSampleBias takes 4 or 5 arguments")
		}
		texT, texIR, sampIR := checkTexAndSampler(false)
		if texT.k == kTextureDepth2D {
			fail(x.args[0].at(), "textureSampleBias does not take texture_depth_2d")
		}
		args := []shaderir.Expr{texIR, sampIR, checkCoord(texT, x.args[2])}
		switch texT.k {
		case kTexture2DArray:
			if len(x.args) != 5 {
				fail(x.at(), "textureSampleBias on texture_2d_array<f32> takes 5 arguments (texture, sampler, uv, layer, bias)")
			}
			args = append(args, checkLayer(x.args[3]), f32Arg(x.args[4]))
		default:
			args = append(args, f32Arg(x.args[3]))
			if len(x.args) == 5 {
				if texT.k == kTextureCube {
					fail(x.args[4].at(), "textureSampleBias on texture_cube<f32> takes no offset")
				}
				args = append(args, offsetArg(x.args[4]))
			}
		}
		return value{t: vec(kF32, 4), e: builtinCall(shaderir.TextureSampleBias, vec(kF32, 4), args...)}, true
	case "textureSampleGrad":
		if len(x.args) < 5 || len(x.args) > 6 {
			fail(x.at(), "textureSampleGrad takes 5 or 6 arguments")
		}
		texT, texIR, sampIR := checkTexAndSampler(false)
		if texT.k == kTextureDepth2D {
			fail(x.args[0].at(), "textureSampleGrad does not take texture_depth_2d")
		}
		args := []shaderir.Expr{texIR, sampIR, checkCoord(texT, x.args[2])}
		gradT := vec(kF32, 2)
		if texT.k == kTextureCube {
			gradT = vec(kF32, 3)
		}
		grad := func(arg expr) shaderir.Expr { return c.ir(c.convert(c.expr(arg, out), gradT, arg.at())) }
		switch texT.k {
		case kTexture2DArray:
			if len(x.args) != 6 {
				fail(x.at(), "textureSampleGrad on texture_2d_array<f32> takes 6 arguments (texture, sampler, uv, layer, ddx, ddy)")
			}
			args = append(args, checkLayer(x.args[3]), grad(x.args[4]), grad(x.args[5]))
		default:
			args = append(args, grad(x.args[3]), grad(x.args[4]))
			if len(x.args) == 6 {
				if texT.k == kTextureCube {
					fail(x.args[5].at(), "textureSampleGrad on texture_cube<f32> takes no offset")
				}
				args = append(args, offsetArg(x.args[5]))
			}
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
			args = append(args, offsetArg(x.args[4]))
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
		if tex.t.k == kTextureCube {
			fail(x.args[0].at(), "textureLoad does not take texture_cube<f32>")
		}
		args := []shaderir.Expr{c.ir(tex)}
		if tex.t.k == kTexture2DArray {
			if len(x.args) != 4 {
				fail(x.at(), "textureLoad on texture_2d_array<f32> takes 4 arguments (texture, coords, layer, level)")
			}
			coord := c.concrete(c.expr(x.args[1], out), x.args[1].at())
			if coord.t.k != kVec || coord.t.n != 2 || !isInt(coord.t.elem) {
				fail(x.args[1].at(), "textureLoad coordinates must be vec2<i32> or vec2<u32>, not %s", coord.t)
			}
			args = append(args, c.ir(coord), checkLayer(x.args[2]), checkLayer(x.args[3]))
			return value{t: vec(kF32, 4), e: builtinCall(shaderir.TextureLoad, vec(kF32, 4), args...)}, true
		}
		for i := 1; i < len(x.args); i++ {
			a := x.args[i]
			args = append(args, c.ir(c.concrete(c.expr(a, out), a.at())))
		}
		retT := sampledRet(tex.t)
		return value{t: retT, e: builtinCall(shaderir.TextureLoad, retT, args...)}, true
	case "textureDimensions":
		if len(x.args) < 1 || len(x.args) > 2 {
			fail(x.at(), "textureDimensions takes 1 or 2 arguments")
		}
		tex := c.expr(x.args[0], out)
		args := []shaderir.Expr{c.ir(tex)}
		if len(x.args) == 2 {
			args = append(args, checkLayer(x.args[1]))
		}
		return value{t: vec(kU32, 2), e: builtinCall(shaderir.TextureDimensions, vec(kU32, 2), args...)}, true
	case "textureStore":
		if len(x.args) != 3 {
			fail(x.at(), "textureStore takes 3 arguments (texture, coords, value)")
		}
		tex := c.expr(x.args[0], out)
		if tex.t.k != kTextureStorage2D {
			fail(x.args[0].at(), "textureStore requires a storage texture, not %s", tex.t)
		}
		if tex.t.storageAccess == shaderir.StorageAccessRead {
			fail(x.args[0].at(), "textureStore requires a write or read_write storage texture")
		}
		coord := c.concrete(c.expr(x.args[1], out), x.args[1].at())
		if coord.t.k != kVec || coord.t.n != 2 || !isInt(coord.t.elem) {
			fail(x.args[1].at(), "textureStore coords must be vec2<i32> or vec2<u32>, not %s", coord.t)
		}
		val := c.convert(c.expr(x.args[2], out), vec(kF32, 4), x.args[2].at())
		return value{t: tNone, e: builtinCall(shaderir.TextureStore, tNone, c.ir(tex), c.ir(coord), c.ir(val))}, true
	case "arrayLength":
		if len(x.args) != 1 {
			fail(x.at(), "arrayLength takes 1 argument")
		}
		argExpr := x.args[0]
		if u, ok := argExpr.(*unaryExpr); ok && u.op == "&" {
			argExpr = u.x
		}
		v := c.expr(argExpr, out)
		if v.t.k != kArray || v.t.n != 0 {
			fail(x.args[0].at(), "arrayLength requires a runtime-sized storage array, not %s", v.t)
		}
		return value{t: tU32, e: builtinCall(shaderir.ArrayLength, tU32, c.ir(v))}, true
	case "workgroupBarrier":
		if len(x.args) != 0 {
			fail(x.at(), "workgroupBarrier takes no arguments")
		}
		if c.cur == nil || c.cur.stage != "compute" {
			fail(x.at(), "workgroupBarrier is only allowed in @compute shaders")
		}
		return value{t: tNone, e: builtinCall(shaderir.WorkgroupBarrier, tNone)}, true
	case "storageBarrier":
		if len(x.args) != 0 {
			fail(x.at(), "storageBarrier takes no arguments")
		}
		if c.cur == nil || c.cur.stage != "compute" {
			fail(x.at(), "storageBarrier is only allowed in @compute shaders")
		}
		return value{t: tNone, e: builtinCall(shaderir.StorageBarrier, tNone)}, true
	}
	return value{}, false
}

func parseStorageTextureFormat(name string) (shaderir.StorageTextureFormat, bool) {
	switch name {
	case "rgba8unorm":
		return shaderir.StorageFormatRGBA8Unorm, true
	case "rgba16float":
		return shaderir.StorageFormatRGBA16Float, true
	case "rgba32float":
		return shaderir.StorageFormatRGBA32Float, true
	case "r32float":
		return shaderir.StorageFormatR32Float, true
	case "r32uint":
		return shaderir.StorageFormatR32Uint, true
	case "r32sint":
		return shaderir.StorageFormatR32Sint, true
	default:
		return shaderir.StorageFormatUndefined, false
	}
}

func parseStorageAccess(name string) (shaderir.StorageAccess, bool) {
	switch name {
	case "read":
		return shaderir.StorageAccessRead, true
	case "write":
		return shaderir.StorageAccessWrite, true
	case "read_write":
		return shaderir.StorageAccessReadWrite, true
	default:
		return 0, false
	}
}

func parseWorkgroupSize(attrs []attr, pos int, fnName string) [3]uint32 {
	for _, a := range attrs {
		if a.name != "workgroup_size" {
			continue
		}
		if len(a.args) < 1 || len(a.args) > 3 {
			fail(a.pos, "@workgroup_size takes 1 to 3 positive integer arguments")
		}
		ws := [3]uint32{1, 1, 1}
		for i, raw := range a.args {
			v, err := strconv.ParseUint(raw, 10, 32)
			if err != nil || v == 0 {
				fail(a.pos, "@workgroup_size argument %q must be a positive integer", raw)
			}
			ws[i] = uint32(v)
		}
		return ws
	}
	fail(pos, "@compute %s requires @workgroup_size(X[, Y[, Z]])", fnName)
	return [3]uint32{1, 1, 1}
}

func parseComputeField(name string, t ty, attrs []attr, pos int, fieldIdx int) stageFieldBinding {
	fb := stageFieldBinding{fieldIdx: fieldIdx, name: name, t: t}
	b, ok := attrString(attrs, "builtin")
	if !ok {
		fail(pos, "@compute parameter %s requires a compute @builtin(...)", name)
	}
	switch b {
	case "global_invocation_id":
		if !t.equal(vec(kU32, 3)) {
			fail(pos, "@builtin(global_invocation_id) must have type vec3<u32>, not %s", t)
		}
		fb.builtin = shaderir.BuiltinGlobalInvocationID
	case "local_invocation_id":
		if !t.equal(vec(kU32, 3)) {
			fail(pos, "@builtin(local_invocation_id) must have type vec3<u32>, not %s", t)
		}
		fb.builtin = shaderir.BuiltinLocalInvocationID
	case "workgroup_id":
		if !t.equal(vec(kU32, 3)) {
			fail(pos, "@builtin(workgroup_id) must have type vec3<u32>, not %s", t)
		}
		fb.builtin = shaderir.BuiltinWorkgroupID
	case "num_workgroups":
		if !t.equal(vec(kU32, 3)) {
			fail(pos, "@builtin(num_workgroups) must have type vec3<u32>, not %s", t)
		}
		fb.builtin = shaderir.BuiltinNumWorkgroups
	case "local_invocation_index":
		if !t.equal(tU32) {
			fail(pos, "@builtin(local_invocation_index) must have type u32, not %s", t)
		}
		fb.builtin = shaderir.BuiltinLocalInvocationIndex
	default:
		fail(pos, "unsupported compute @builtin(%s)", b)
	}
	return fb
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

// uniformLayoutError returns the first WGSL uniform-address-space layout rule
// violated by the natural layout of t. The IR's uniform layout is deliberately
// stricter than storage layout, so accepted uniform buffers have the same
// offsets in every writer.
func uniformLayoutError(t shaderir.Type, prefix string) (member, reason string, ok bool) {
	switch t.Main {
	case shaderir.Array:
		if len(t.Sub) == 0 {
			return "", "", false
		}
		if stride := t.Sub[0].StorageStride(); stride%16 != 0 {
			return prefix, fmt.Sprintf("array elements are %d bytes apart; uniform arrays need a multiple of 16, such as array<vec4<f32>, N>", stride), true
		}
		return uniformLayoutError(t.Sub[0], prefix)
	case shaderir.Struct:
		for i, f := range t.Sub {
			name := fmt.Sprintf("field%d", i)
			if i < len(t.FieldNames) && t.FieldNames[i] != "" {
				name = t.FieldNames[i]
			}
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}
			offset := t.StorageStructFieldOffset(i)
			if (f.Main == shaderir.Struct || f.Main == shaderir.Array) && offset%16 != 0 {
				return path, fmt.Sprintf("starts at offset %d, which is not a multiple of 16; add padding before this member", offset), true
			}
			if i > 0 {
				previous := t.Sub[i-1]
				if previous.Main == shaderir.Struct || previous.Main == shaderir.Array {
					previousEnd := t.StorageStructFieldOffset(i-1) + roundUp16(previous.StorageSizeBytes())
					if offset < previousEnd {
						return path, fmt.Sprintf("starts at offset %d, before the required offset %d after the preceding %v member; add padding", offset, previousEnd, previous.Main), true
					}
				}
			}
			if member, reason, ok := uniformLayoutError(f, path); ok {
				return member, reason, true
			}
		}
	}
	return "", "", false
}

func roundUp16(n int) int {
	return (n + 15) / 16 * 16
}
