//go:build js && wasm

package miniprogram

import (
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestDeviceModelReachesEnvironment(t *testing.T) {
	for _, tc := range []struct {
		model any
		want  string
	}{{"Google Pixel 8", "Google Pixel 8"}, {nil, ""}, {42, ""}} {
		read := js.FuncOf(func(js.Value, []js.Value) any {
			d := map[string]any{
				"safeTop": 0, "safeRight": 0, "safeBottom": 0, "safeLeft": 0,
				"pixelRatio": 2, "fontScale": 1, "language": "en_US",
				"os": "android", "osVersion": "14",
			}
			if tc.model != nil {
				d["deviceModel"] = tc.model
			}
			return d
		})
		s := surface{bridge: js.ValueOf(map[string]any{"device": read}),
			env: driver.Environment{DeviceModel: "Previous phone"}}
		got := s.environment()
		read.Release()
		if got.DeviceModel != tc.want || got.DeviceName != "" {
			t.Errorf("device model %v: identity = %q, %q", tc.model, got.DeviceName, got.DeviceModel)
		}
	}
}
