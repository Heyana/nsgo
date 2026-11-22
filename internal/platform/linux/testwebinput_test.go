//go:build linux && !android

package linux

import "testing"

func TestWebInputProvenance(t *testing.T) {
	previous := testWebInput
	t.Cleanup(func() { testWebInput = previous })
	testWebInput.sources = map[int32]bool{4: true, 5: true}
	testWebInput.requests = [128]uintptr{42}
	testWebInput.events = nil
	for _, tc := range []struct {
		name    string
		source  int32
		serial  uintptr
		trusted bool
	}{
		{"physical device with scripted serial", 6, 42, false},
		{"unrecorded XTest request", 4, 41, false},
		{"scripted pointer", 4, 42, true},
		{"unrecorded keyboard", 5, 43, false},
		{"scripted keyboard", 5, 42, true},
	} {
		i := len(testWebInput.events)
		rememberTestWebInput(xInputDeviceEvent{event: 4, time: 123, detail: 1, window: 100, source: tc.source, serial: tc.serial})
		if got := testWebInput.events[i]; got.trusted != tc.trusted {
			t.Errorf("%s trusted = %t, want %t", tc.name, got.trusted, tc.trusted)
		}
	}
}
