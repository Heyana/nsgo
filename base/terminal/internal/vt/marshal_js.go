//go:build js && !nsgominiprogram && !nsgoharmony

package vt

import (
	"fmt"
	"unsafe"
)

type wasmAllocation struct {
	host, guest  uintptr
	typ          string
	count, bytes uintptr
	pointers     map[uintptr][]byte
}

type wasmCall struct{ allocations []*wasmAllocation }

func (s *wasmCall) marshal(host uintptr, typ string, count uintptr) uintptr {
	if host == 0 || count == 0 {
		return 0
	}
	n := abiSize(wasm.guest, typ) * count
	if n == 0 {
		panic("vt: unknown WebAssembly argument type " + typ)
	}
	guest := uintptr(uint32(wasm.alloc.Invoke(int(n)).Int()))
	if guest == 0 {
		panic("vt: WebAssembly allocation failed")
	}
	a := &wasmAllocation{host: host, guest: guest, typ: typ, count: count, bytes: n}
	s.allocations = append(s.allocations, a)
	hostSize, guestSize := abiSize(wasm.host, typ), abiSize(wasm.guest, typ)
	dst := make([]byte, n)
	for i := range count {
		src := unsafe.Slice((*byte)(hostMemory(host+i*hostSize)), hostSize)
		out := dst[i*guestSize : (i+1)*guestSize]
		convertABI(out, src, typ, wasm.guest, wasm.host)
		s.children(a, out, src, typ, i*hostSize)
	}
	writeForeignBytes(guest, dst)
	return guest
}

// children copies the host-owned buffers referenced by ABI structs. Other
// pointers (terminal handles, grid references, callbacks) remain guest values.
func (s *wasmCall) children(a *wasmAllocation, dst, src []byte, typ string, hostBase uintptr) {
	t := wasm.host.Types[typ]
	if t.Kind != "struct" {
		return
	}
	for name, f := range t.Fields {
		to := wasm.guest.Types[typ].Fields[name]
		if f.Type != "pointer" {
			if nested := wasm.host.Types[f.Type]; nested.Kind == "struct" {
				s.children(a, dst[to.Offset:to.Offset+to.Size], src[f.Offset:f.Offset+f.Size], f.Type, hostBase+f.Offset)
			}
			continue
		}
		elem, length := "", ""
		switch {
		case typ == "GhosttyString" && name == "ptr":
			elem, length = "u8", "len"
		case typ == "GhosttySelectionBuffer" && name == "ptr":
			elem, length = "GhosttySelection", "cap"
		case (typ == "GhosttyFormatterTerminalOptions" || typ == "GhosttyTerminalSelectionFormatOptions") && name == "selection":
			elem = "GhosttySelection"
		case typ == "GhosttyClipboardReadReply" && name == "contents":
			elem, length = "GhosttyClipboardContent", "contents_len"
		case typ == "GhosttyClipboardReadReply" && name == "available":
			elem, length = "GhosttyString", "available_len"
		case typ == "GhosttyPaste" && name == "mimes":
			elem, length = "GhosttyString", "mimes_len"
		}
		if elem == "" {
			continue
		}
		ptr := uintptr(unsignedABI(src[f.Offset : f.Offset+f.Size]))
		count := uintptr(1)
		if length != "" {
			field := t.Fields[length]
			count = uintptr(unsignedABI(src[field.Offset : field.Offset+field.Size]))
		}
		if ptr == 0 || count == 0 {
			continue
		}
		child := s.marshal(ptr, elem, count)
		putUnsignedABI(dst[to.Offset:to.Offset+to.Size], uint64(child))
		if a.pointers == nil {
			a.pointers = map[uintptr][]byte{}
		}
		a.pointers[hostBase+f.Offset] = append([]byte(nil), src[f.Offset:f.Offset+f.Size]...)
	}
}

func (s *wasmCall) copyBack() {
	for _, a := range s.allocations {
		hostSize, guestSize := abiSize(wasm.host, a.typ), abiSize(wasm.guest, a.typ)
		src := foreignBytes(a.guest, a.bytes)
		buf := make([]byte, hostSize*a.count)
		for i := range a.count {
			convertABI(buf[i*hostSize:(i+1)*hostSize], src[i*guestSize:(i+1)*guestSize], a.typ, wasm.host, wasm.guest)
		}
		for offset, value := range a.pointers {
			copy(buf[offset:], value)
		}
		copy(unsafe.Slice((*byte)(hostMemory(a.host)), uintptr(len(buf))), buf)
	}
}

func (s *wasmCall) free() {
	for _, a := range s.allocations {
		if a.guest != 0 {
			wasm.free.Invoke(int(a.guest), int(a.bytes))
		}
	}
}

func (s *wasmCall) retain(event uintptr) {
	if previous, ok := wasm.retained[event]; ok {
		wasm.free.Invoke(int(previous.guest), int(previous.bytes))
		delete(wasm.retained, event)
	}
	if len(s.allocations) > 0 {
		a := s.allocations[0]
		if len(s.allocations) != 1 {
			panic(fmt.Sprintf("vt: unexpected UTF-8 allocations: %d", len(s.allocations)))
		}
		wasm.retained[event] = *a
		a.guest = 0
	}
}
