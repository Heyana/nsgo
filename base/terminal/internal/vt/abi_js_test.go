//go:build js && !nsgominiprogram && !nsgoharmony

package vt

import (
	"bytes"
	"encoding/json"
	"testing"
	"unsafe"
)

func TestBrowserMemoryABI(t *testing.T) {
	before := wasm
	defer func() { wasm = before }()
	if _, err := open("embedded:ghostty-vt.wasm"); err != nil {
		t.Fatal(err)
	}
	p := uintptr(uint32(wasm.exports.Get("ghostty_type_json").Invoke().Int()))
	var m manifest
	if err := json.Unmarshal([]byte(foreignCString(p)), &m); err != nil {
		t.Fatal(err)
	}
	if err := checkManifest(m); err != nil {
		t.Fatal(err)
	}
	if wasm.guest.ABI.PointerSize != 4 || wasm.host.ABI.PointerSize != int(unsafe.Sizeof(uintptr(0))) {
		t.Fatal("guest and host pointer widths were not distinguished")
	}
	old := wasm.exports.Get("memory").Get("buffer")
	const size = 32 << 20
	allocation := uintptr(uint32(wasm.exports.Get("ghostty_wasm_alloc").Invoke(size).Int()))
	if allocation == 0 {
		t.Fatal("allocation failed")
	}
	defer wasm.exports.Get("ghostty_wasm_free").Invoke(int(allocation), size)
	if old.Equal(wasm.exports.Get("memory").Get("buffer")) {
		t.Fatal("control allocation did not grow guest memory")
	}
	want := []byte("memory after growth")
	writeForeignBytes(allocation, want)
	if got := foreignBytes(allocation, uintptr(len(want))); !bytes.Equal(got, want) {
		t.Fatalf("after guest memory growth: %q", got)
	}
}

func TestBrowserPromptBoolABI(t *testing.T) {
	loadLib(t)
	term, err := NewTerminal(20, 5, Effects{})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Free()
	for _, test := range []struct {
		input string
		want  bool
	}{{"", false}, {"\x1b]133;A\x07$ \x1b]133;B\x07", true}, {"\x1b[?1049h", false}} {
		term.Write([]byte(test.input))
		var got bool
		if err := result(call(fnTerminalGet, term.h, 39, uintptr(unsafe.Pointer(&got)))); err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("cursor at prompt after %q: %v, want %v", test.input, got, test.want)
		}
	}
}
