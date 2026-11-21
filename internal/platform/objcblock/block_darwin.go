//go:build darwin

// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Ebitengine Authors
// Callback sharing follows purego's objc block bridge; NSGo pins templates, allocates descriptors in native memory and checks block allocation.

// Package objcblock creates Objective-C blocks whose storage remains valid across Go stack moves and garbage collection.
package objcblock

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"structs"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// The Apple Block ABI: https://clang.llvm.org/docs/Block-ABI-Apple.html. Descriptors and signatures live in native memory for the process lifetime, one per callback type. The runtime owns each block and calls dispose when its last reference goes away; only then may Go forget its closure.
type descriptor struct {
	_                             structs.HostLayout
	reserved, size, copy, dispose uintptr
	signature                     *byte
}

type literal struct {
	_               structs.HostLayout
	isa             uintptr
	flags, reserved uint32
	invoke          uintptr
	descriptor      *descriptor
}

var (
	setup      sync.Once
	stackClass uintptr
	copyNative func(*literal) objc.Block
	malloc     func(uintptr) unsafe.Pointer
	dispose    uintptr
	typesMu    sync.Mutex
	types      = map[reflect.Type]literal{}
	functions  sync.Map // objc.Block -> callback
)

type callback struct {
	value   reflect.Value
	cleanup func()
}

func initialize() {
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	stackClass, err = purego.Dlsym(lib, "_NSConcreteStackBlock")
	if err != nil {
		panic(err)
	}
	purego.RegisterLibFunc(&copyNative, lib, "_Block_copy")
	purego.RegisterLibFunc(&malloc, lib, "malloc")
	dispose = purego.NewCallback(func(block objc.Block) {
		if entry, ok := functions.LoadAndDelete(block); ok {
			if cleanup := entry.(callback).cleanup; cleanup != nil {
				cleanup()
			}
		}
	})
}

// New returns a retained block for fn, whose first argument must be objc.Block. It supports the scalar, pointer and struct arguments the platform backends use. The caller releases its reference with Block.Release after the receiving API has copied the block, or once the callback is no longer needed. Invoke it through native APIs, not purego's Go-only Block.Invoke cache.
func New(fn any) objc.Block {
	setup.Do(initialize)
	return newBlock(fn, copyNative)
}

// NewWithCleanup is New with cleanup called when the block's final native
// reference is released. Cleanup may run on any thread.
func NewWithCleanup(fn any, cleanup func()) objc.Block {
	setup.Do(initialize)
	return newBlockWithCleanup(fn, cleanup, copyNative)
}

func newBlock(fn any, copyBlock func(*literal) objc.Block) objc.Block {
	return newBlockWithCleanup(fn, nil, copyBlock)
}

func newBlockWithCleanup(fn any, cleanup func(), copyBlock func(*literal) objc.Block) objc.Block {
	value := reflect.ValueOf(fn)
	if !value.IsValid() || value.Kind() != reflect.Func || value.IsNil() {
		panic("objcblock: expected a non-nil function")
	}
	layout := template(value.Type())
	// A uintptr hides a Go stack address from stack relocation. Pin the template on the heap, and pass a typed pointer through purego instead.
	var pin runtime.Pinner
	pin.Pin(&layout)
	defer pin.Unpin()
	block := copyBlock(&layout)
	runtime.KeepAlive(&layout)
	if block == 0 {
		panic(fmt.Sprintf("objcblock: _Block_copy failed for %s", value.Type()))
	}
	functions.Store(block, callback{value: value, cleanup: cleanup})
	return block
}

func template(typ reflect.Type) literal {
	typesMu.Lock()
	defer typesMu.Unlock()
	if layout, ok := types[typ]; ok {
		return layout
	}
	if typ.NumIn() == 0 || typ.In(0) != reflect.TypeFor[objc.Block]() || typ.NumOut() > 1 {
		panic("objcblock: expected func(objc.Block, ...) with at most one result")
	}
	signature := "v"
	if typ.NumOut() == 1 {
		signature = encode(typ.Out(0))
	}
	signature += "@?"
	for i := 1; i < typ.NumIn(); i++ {
		signature += encode(typ.In(i))
	}
	invoke := purego.NewCallback(reflect.MakeFunc(typ, func(args []reflect.Value) []reflect.Value {
		block := args[0].Interface().(objc.Block)
		fn, ok := functions.Load(block)
		if !ok {
			panic("objcblock: callback invoked after its block was released")
		}
		return fn.(callback).value.Call(args)
	}).Interface())
	bytes := append([]byte(signature), 0)
	storage := malloc(unsafe.Sizeof(descriptor{}) + uintptr(len(bytes)))
	if storage == nil {
		panic("objcblock: could not allocate a block descriptor")
	}
	desc := (*descriptor)(storage)
	*desc = descriptor{size: unsafe.Sizeof(literal{}), dispose: dispose,
		signature: (*byte)(unsafe.Add(storage, unsafe.Sizeof(descriptor{})))}
	copy(unsafe.Slice(desc.signature, len(bytes)), bytes)
	const hasCopyDispose, hasSignature = 1 << 25, 1 << 30
	layout := literal{isa: stackClass, flags: hasCopyDispose | hasSignature, invoke: invoke, descriptor: desc}
	types[typ] = layout
	return layout
}

func encode(typ reflect.Type) string {
	if typ == reflect.TypeFor[objc.ID]() {
		return "@"
	}
	if typ == reflect.TypeFor[objc.Block]() {
		return "@?"
	}
	switch typ.Kind() {
	case reflect.Bool:
		return "B"
	case reflect.Int8:
		return "c"
	case reflect.Uint8:
		return "C"
	case reflect.Int16:
		return "s"
	case reflect.Uint16:
		return "S"
	case reflect.Int32:
		return "i"
	case reflect.Uint32:
		return "I"
	case reflect.Int, reflect.Int64:
		return "q"
	case reflect.Uint, reflect.Uint64, reflect.Uintptr:
		return "Q"
	case reflect.Float32:
		return "f"
	case reflect.Float64:
		return "d"
	case reflect.Pointer:
		return "^" + encode(typ.Elem())
	case reflect.UnsafePointer:
		return "^v"
	case reflect.Struct:
		var b strings.Builder
		b.WriteString("{" + typ.Name() + "=")
		for field := range typ.Fields() {
			if field.Type != reflect.TypeFor[structs.HostLayout]() {
				b.WriteString(encode(field.Type))
			}
		}
		b.WriteByte('}')
		return b.String()
	default:
		panic(fmt.Sprintf("objcblock: unsupported callback type %s", typ))
	}
}
