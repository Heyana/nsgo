//go:build js && wasm

package harmonyos

import (
	"syscall/js"
	"testing"
)

func TestDeviceModelReachesEnvironment(t *testing.T) {
	read := js.FuncOf(func(js.Value, []js.Value) any {
		return `{"Width":320,"Height":480,"DeviceModel":"Mate 80 Pro"}`
	})
	defer read.Release()
	s := surface{bridge: js.ValueOf(map[string]any{"device": read})}
	if got := s.environment(); got.DeviceModel != "Mate 80 Pro" || got.DeviceName != "" {
		t.Fatalf("device identity = %q, %q", got.DeviceName, got.DeviceModel)
	}
}
