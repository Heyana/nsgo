//go:build js && wasm

package web

import (
	"math"
	"strings"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func hapticPatternsAvailable(navigator js.Value) bool {
	return navigator.Get("vibrate").Type() == js.TypeFunction &&
		strings.Contains(navigator.Get("userAgent").String(), "Android")
}

// vibrationTiming turns the amplitude waveform into the web's alternating
// on and off durations, with a zero on for a leading pause.
func vibrationTiming(pattern driver.HapticPattern) []any {
	times := []any{float64(0)}
	on := true
	for _, segment := range driver.HapticWaveform(pattern) {
		playing := segment.Amplitude > 0
		if playing != on {
			times = append(times, float64(0))
			on = playing
		}
		i := len(times) - 1
		times[i] = min(times[i].(float64)+float64(segment.Milliseconds), math.MaxUint32)
	}
	return times
}

// The browser has one vibration stream for every surface on the page.
// Only the surface whose request replaced it may cancel that stream.
var hapticOwner *surface

func (s *surface) playHapticPattern(pattern *driver.HapticPattern) {
	if pattern == nil || s.doc.Get("visibilityState").String() == "hidden" {
		s.stopHapticPattern()
		return
	}
	navigator := js.Global().Get("navigator")
	if hapticPatternsAvailable(navigator) {
		if navigator.Call("vibrate", vibrationTiming(*pattern)).Bool() {
			hapticOwner = s
		}
	}
}

func (s *surface) stopHapticPattern() {
	if hapticOwner == s {
		hapticOwner = nil
		navigator := js.Global().Get("navigator")
		if navigator.Get("vibrate").Type() == js.TypeFunction {
			navigator.Call("vibrate", 0)
		}
	}
}
