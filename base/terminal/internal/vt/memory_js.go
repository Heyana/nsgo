//go:build js && !nsgominiprogram && !nsgoharmony

package vt

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"reflect"
	"syscall/js"
	"unsafe"
)

//go:embed abi64.json
var hostABIData []byte

func checkManifest(m manifest) error {
	if m.ABI.PointerSize != 4 || m.ABI.UsizeSize != 4 || m.ABI.Endian != "little" {
		return fmt.Errorf("unsupported WebAssembly ABI: %+v", m.ABI)
	}
	var host manifest
	if err := json.Unmarshal(hostABIData, &host); err != nil {
		return err
	}
	if err := host.check(); err != nil {
		return fmt.Errorf("host marshaling layout: %w", err)
	}
	for _, enum := range enums {
		if value, ok := m.Types[enum.typ].Values[enum.name]; !ok || value != enum.value {
			return fmt.Errorf("guest enum %s.%s differs", enum.typ, enum.name)
		}
	}
	for name, expected := range layouts() {
		t, ok := m.Types[name]
		if !ok || t.Size == 0 {
			return fmt.Errorf("missing guest type %s", name)
		}
		for field := range expected.fields {
			if _, ok := t.Fields[field]; !ok {
				return fmt.Errorf("missing guest field %s.%s", name, field)
			}
		}
	}
	if !reflect.DeepEqual(m.Types["GhosttyCell"].Bits, host.Types["GhosttyCell"].Bits) {
		return fmt.Errorf("guest packed cell layout differs")
	}
	wasm.guest, wasm.host = m, host
	return nil
}

// Every memory access obtains the current ArrayBuffer: any guest allocation can grow memory and detach all previously obtained JavaScript views.
func foreignBytes(p, n uintptr) []byte {
	if n == 0 {
		return nil
	}
	v := wasm.array.New(wasm.memory.Get("buffer"), int(p), int(n))
	out := make([]byte, n)
	js.CopyBytesToGo(out, v)
	return out
}

func writeForeignBytes(p uintptr, b []byte) {
	if len(b) > 0 {
		v := wasm.array.New(wasm.memory.Get("buffer"), int(p), len(b))
		js.CopyBytesToJS(v, b)
	}
}

func foreignValue[T any](p uintptr) T {
	var value T
	name := foreignType[T]()
	src := foreignBytes(p, abiSize(wasm.guest, name))
	dst := unsafe.Slice((*byte)(unsafe.Pointer(&value)), unsafe.Sizeof(value))
	convertABI(dst, src, name, wasm.host, wasm.guest)
	switch v := any(value).(type) {
	case clipboardRead:
		wasm.replies[v.reply] = "GhosttyClipboardReadReply"
	case clipboardWrite:
		wasm.replies[v.reply] = "GhosttyClipboardWriteReply"
	}
	return value
}

func foreignSlice[T any](p, n uintptr) []T {
	values := make([]T, n)
	if n == 0 {
		return values
	}
	name := foreignType[T]()
	guestSize := abiSize(wasm.guest, name)
	src := foreignBytes(p, n*guestSize)
	for i := range n {
		dst := unsafe.Slice((*byte)(unsafe.Pointer(&values[i])), unsafe.Sizeof(values[i]))
		convertABI(dst, src[i*guestSize:(i+1)*guestSize], name, wasm.host, wasm.guest)
	}
	return values
}

func foreignWrite[T any](p uintptr, value T) {
	name := foreignType[T]()
	src := unsafe.Slice((*byte)(unsafe.Pointer(&value)), unsafe.Sizeof(value))
	dst := make([]byte, abiSize(wasm.guest, name))
	convertABI(dst, src, name, wasm.guest, wasm.host)
	writeForeignBytes(p, dst)
}

func withForeignBytes(p, n uintptr, write func([]byte)) {
	bytes := foreignBytes(p, n)
	write(bytes)
	writeForeignBytes(p, bytes)
}

func foreignCString(p uintptr) string {
	if p == 0 {
		return ""
	}
	view := wasm.array.New(wasm.memory.Get("buffer"))
	n := uintptr(0)
	for view.Index(int(p+n)).Int() != 0 {
		n++
	}
	return string(foreignBytes(p, n))
}

func foreignType[T any]() string {
	var value T
	switch any(value).(type) {
	case uint64:
		return "u64"
	case int32:
		return "i32"
	case cString:
		return "GhosttyString"
	case clipboardRead:
		return "GhosttyClipboardRead"
	case clipboardWrite:
		return "GhosttyClipboardWrite"
	case clipboardContent:
		return "GhosttyClipboardContent"
	case sizeReport:
		return "GhosttySizeReportSize"
	case deviceAttributes:
		return "GhosttyDeviceAttributes"
	case desktopNotification:
		return "GhosttyTerminalDesktopNotification"
	case progressReport:
		return "GhosttyTerminalProgressReport"
	case semanticPrompt:
		return "GhosttyTerminalSemanticPrompt"
	case sysImage:
		return "GhosttySysImage"
	}
	panic("vt: no guest memory type for " + reflect.TypeFor[T]().String())
}
