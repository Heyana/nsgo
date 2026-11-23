//go:build !js

package vt

import (
	"encoding/json"
	"os"
	"testing"
)

// TestWriteWasmHostABI records the 64-bit C ABI the shared Go structs mirror. The browser uses it to marshal the separate 32-bit guest ABI. Rebuild it with the pinned native library whenever the binding's commit changes.
func TestWriteWasmHostABI(t *testing.T) {
	path := os.Getenv("NSGO_GHOSTTY_ABI_OUT")
	if path == "" {
		t.Skip("set NSGO_GHOSTTY_ABI_OUT on a native 64-bit target")
	}
	loadLib(t)
	var value any
	if err := json.Unmarshal([]byte(cStringAt(call(fnTypeJSON))), &value); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
