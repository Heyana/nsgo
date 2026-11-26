package hlsl

import (
	"fmt"
	"go/constant"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// A storageRef is a typed byte address within a read-only render binding.
// Keep constant offsets separate so, for example, b[1].field reads only the
// field and folds the element stride and member offset into one literal.
type storageRef struct {
	binding int
	typ     shaderir.Type
	dynamic string
	offset  int
}

func (r storageRef) address() string {
	if r.dynamic == "" {
		return fmt.Sprintf("%du", r.offset)
	}
	if r.offset == 0 {
		return r.dynamic
	}
	return fmt.Sprintf("(%s + %du)", r.dynamic, r.offset)
}

func (w *pipeWriter) storageRef(e shaderir.Expr, stage string) (storageRef, bool) {
	switch e.Type {
	case shaderir.BindingVariable:
		if e.Index >= 0 && e.Index < len(w.p.Bindings) {
			b := w.p.Bindings[e.Index]
			if b.Kind == shaderir.BindingStorageBufferRead {
				return storageRef{binding: e.Index, typ: b.Type}, true
			}
		}
	case shaderir.FieldSelector:
		if m := e.Exprs[1]; m.Type == shaderir.StructMember {
			if r, ok := w.storageRef(e.Exprs[0], stage); ok {
				r.offset += r.typ.StorageStructFieldOffset(m.Index)
				r.typ = r.typ.Sub[m.Index]
				return r, true
			}
		}
	case shaderir.Index:
		r, ok := w.storageRef(e.Exprs[0], stage)
		if !ok {
			break
		}
		switch {
		case r.typ.Main == shaderir.Array:
			r.typ = r.typ.Sub[0]
		case r.typ.IsMatrix():
			r.typ = storageMatrixColumn(r.typ)
		default:
			// Vector subscripts operate on the loaded HLSL vector.
			return storageRef{}, false
		}
		stride := r.typ.StorageStride()
		idx := e.Exprs[1]
		if idx.Type == shaderir.NumberExpr {
			if i, exact := constant.Int64Val(idx.Const); exact {
				r.offset += int(i) * stride
				return r, true
			}
		}
		term := fmt.Sprintf("uint(%s) * %du", w.emitExpr(idx, stage), stride)
		if r.dynamic == "" {
			r.dynamic = term
		} else {
			r.dynamic += " + " + term
		}
		return r, true
	}
	return storageRef{}, false
}

func storageMatrixColumn(t shaderir.Type) shaderir.Type {
	return shaderir.Type{Main: shaderir.VectorOf(shaderir.Float, t.MatrixRows())}
}

func (w *pipeWriter) storageLoad(r storageRef) string {
	t := r.typ
	if t.Main == shaderir.Struct || t.Main == shaderir.Array || t.IsMatrix() {
		return fmt.Sprintf("%s(%s)", w.storageHelper(r.binding, t), r.address())
	}
	suffix := [...]string{"", "", "2", "3", "4"}[max(t.VectorElementCount(), 1)]
	load := fmt.Sprintf("B%d.Load%s(%s)", r.binding, suffix, r.address())
	switch t.Scalar() {
	case shaderir.Float:
		return "asfloat(" + load + ")"
	case shaderir.Int:
		return "asint(" + load + ")"
	case shaderir.Uint:
		return load
	case shaderir.Bool:
		return "(" + load + " != 0u)"
	default:
		w.storageErr = fmt.Errorf("hlsl: cannot load %s from a read-only storage buffer", t.String())
		return ""
	}
}

type storageHelperKey struct {
	binding int
	typ     string
}

// Only whole aggregate values need helpers. The uint parameter evaluates the
// address once; nested helpers are emitted before the callers that use them.
func (w *pipeWriter) storageHelper(binding int, t shaderir.Type) string {
	if t.Main == shaderir.Array && t.Length == 0 {
		w.storageErr = fmt.Errorf("hlsl: cannot load a runtime-sized storage array as a value")
		return ""
	}
	key := storageHelperKey{binding: binding, typ: t.String()}
	if name, ok := w.storageHelpers[key]; ok {
		return name
	}
	name := fmt.Sprintf("__load_B%d_%d", binding, len(w.storageHelpers))
	w.storageHelpers[key] = name
	typ := w.typeStr(t)
	lines := []string{fmt.Sprintf("%s %s(uint at) {", typ, name)}
	switch {
	case t.Main == shaderir.Struct:
		lines = append(lines, fmt.Sprintf("\t%s value = (%s)0;", typ, typ))
		for i, field := range t.Sub {
			r := storageRef{binding: binding, typ: field, dynamic: "at", offset: t.StorageStructFieldOffset(i)}
			lines = append(lines, fmt.Sprintf("\tvalue.M%d = %s;", i, w.storageLoad(r)))
		}
		lines = append(lines, "\treturn value;")
	case t.Main == shaderir.Array:
		elem := t.Sub[0]
		r := storageRef{binding: binding, typ: elem, dynamic: fmt.Sprintf("at + i * %du", elem.StorageStride())}
		lines = append(lines, fmt.Sprintf("\t%s value;", typ),
			fmt.Sprintf("\tfor (uint i = 0u; i < %du; ++i) {", t.Length),
			"\t\tvalue[i] = "+w.storageLoad(r)+";", "\t}", "\treturn value;")
	case t.IsMatrix():
		col := storageMatrixColumn(t)
		parts := make([]string, t.MatrixCols())
		for i := range parts {
			parts[i] = w.storageLoad(storageRef{binding: binding, typ: col, dynamic: "at", offset: i * col.StorageStride()})
		}
		// HLSL constructors take rows, WGSL stores columns.
		lines = append(lines, fmt.Sprintf("\treturn transpose(float%dx%d(%s));", t.MatrixCols(), t.MatrixRows(), strings.Join(parts, ", ")))
	}
	w.storageFuncs = append(w.storageFuncs, append(lines, "}", "")...)
	return name
}
