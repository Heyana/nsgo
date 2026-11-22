//go:build js && !nsgominiprogram && !nsgoharmony

package library

import _ "embed"

//go:embed lib/js-wasm/ghostty-vt.wasm
var wasm []byte

// Find returns the embedded browser module's identity; it needs no file system.
func Find() (string, error) { return "embedded:ghostty-vt.wasm", nil }

// Wasm is the pinned WebAssembly library, used by the browser VT binding.
func Wasm() []byte { return wasm }
