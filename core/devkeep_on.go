//go:build nsgodev

package core

import (
	"encoding"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/devtools"
)

type devKeepState struct {
	navSeq map[string]int
}

var (
	jsonMarshalerType   = reflect.TypeFor[json.Marshaler]()
	jsonUnmarshalerType = reflect.TypeFor[json.Unmarshaler]()
	textMarshalerType   = reflect.TypeFor[encoding.TextMarshaler]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// DevKeep preserves sig's value across nsgo dev restarts under key as JSON.
// When a previous run in the same dev session saved a value under key that
// decodes into T, DevKeep restores it immediately; subsequent writes to sig
// are saved while cx is live. Values that cannot encode or decode as JSON are
// ignored. Release builds compile DevKeep out.
func DevKeep[T any](cx *Scope, key string, sig *Signal[T]) {
	if sig == nil {
		return
	}
	DevKeepFunc(cx, key, sig.Get, sig.Set)
}

// DevKeepFunc preserves state across nsgo dev restarts under key as JSON.
// When a previous run in the same dev session saved a value under key that
// decodes into T, restore is called immediately with it. An effect on cx
// tracks read and saves each encoded value while cx is live. Values that
// cannot encode or decode as JSON are ignored. Release builds compile
// DevKeepFunc out.
func DevKeepFunc[T any](cx *Scope, key string, read func() T, restore func(T)) {
	if cx == nil || cx.disposed || key == "" || read == nil {
		return
	}
	if restore != nil {
		if raw, ok := devtools.ReadState(key); ok {
			var v T
			if json.Unmarshal(raw, &v) == nil {
				restore(v)
			}
		}
	}
	cx.Effect(func() {
		v := read()
		raw, err := json.Marshal(v)
		if err != nil {
			return
		}
		devtools.WriteState(key, raw)
	})
}

// DevKeepNav is base.NewNav's dev-restart stack persistence. It skips route
// types that cannot safely round-trip through JSON (such as interfaces,
// functions, channels, or structs with only unexported fields). Release builds
// compile DevKeepNav out.
func DevKeepNav[R any](cx *Scope, read func() []R, restore func([]R)) {
	if cx == nil || cx.disposed || read == nil {
		return
	}
	t := reflect.TypeFor[R]()
	if !canKeepNavType(t, map[reflect.Type]bool{}) {
		return
	}
	typeName := t.String()
	if t.PkgPath() != "" && t.Name() != "" {
		typeName = t.PkgPath() + "." + t.Name()
	}
	if cx.rt.devKeep.navSeq == nil {
		cx.rt.devKeep.navSeq = map[string]int{}
	}
	idx := cx.rt.devKeep.navSeq[typeName]
	cx.rt.devKeep.navSeq[typeName] = idx + 1
	key := "nsgo:nav:" + typeName + "#" + strconv.Itoa(idx)
	DevKeepFunc(cx, key, read, restore)
}

func canKeepNavType(t reflect.Type, seen map[reflect.Type]bool) bool {
	if t == nil {
		return false
	}
	if seen[t] {
		return true
	}
	ptr := reflect.PointerTo(t)
	if (t.Implements(jsonMarshalerType) || ptr.Implements(jsonMarshalerType)) && ptr.Implements(jsonUnmarshalerType) {
		return true
	}
	if (t.Implements(textMarshalerType) || ptr.Implements(textMarshalerType)) && ptr.Implements(textUnmarshalerType) {
		return true
	}
	switch t.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return true
	case reflect.Pointer, reflect.Slice, reflect.Array:
		seen[t] = true
		return canKeepNavType(t.Elem(), seen)
	case reflect.Map:
		seen[t] = true
		k := t.Key()
		kptr := reflect.PointerTo(k)
		keyOK := k.Kind() == reflect.String ||
			(k.Kind() >= reflect.Int && k.Kind() <= reflect.Uintptr) ||
			((k.Implements(textMarshalerType) || kptr.Implements(textMarshalerType)) && kptr.Implements(textUnmarshalerType))
		return keyOK && canKeepNavType(t.Elem(), seen)
	case reflect.Struct:
		seen[t] = true
		if t.NumField() == 0 {
			return true
		}
		exported := 0
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			if name, _, _ := strings.Cut(tag, ","); name == "-" && tag == "-" {
				continue
			}
			exported++
			if !canKeepNavType(f.Type, seen) {
				return false
			}
		}
		return exported > 0
	default:
		return false
	}
}
