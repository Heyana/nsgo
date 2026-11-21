//go:build js && wasm

package web

import (
	"reflect"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestBrowserHapticPatternsReplaceAndStop(t *testing.T) {
	g := js.Global()
	navigator := g.Get("navigator")
	override := func(name string, value any) {
		obj := g.Get("Object")
		initial := obj.Call("getOwnPropertyDescriptor", navigator, name)
		obj.Call("defineProperty", navigator, name, map[string]any{"value": value, "configurable": true})
		t.Cleanup(func() {
			if initial.Type() == js.TypeUndefined {
				navigator.Delete(name)
			} else {
				obj.Call("defineProperty", navigator, name, initial)
			}
		})
	}
	var calls [][]int
	vibrate := js.FuncOf(func(_ js.Value, args []js.Value) any {
		value := args[0]
		if value.Type() == js.TypeNumber {
			calls = append(calls, []int{value.Int()})
		} else {
			var durations []int
			for i := 0; i < value.Length(); i++ {
				durations = append(durations, value.Index(i).Int())
			}
			calls = append(calls, durations)
		}
		return true
	})
	t.Cleanup(vibrate.Release)
	override("vibrate", vibrate)
	override("userAgent", "Android")
	doc := g.Get("Object").New()
	doc.Set("visibilityState", "visible")
	s := &surface{doc: doc}
	t.Cleanup(s.stopHapticPattern)
	first := driver.HapticPattern{
		{Intensity: .25}, {At: 90 * time.Millisecond, Intensity: .5}, {At: 200 * time.Millisecond, Intensity: 1},
	}
	second := driver.HapticPattern{{At: 10 * time.Millisecond, Duration: 50 * time.Millisecond, Intensity: .5}}
	if !hapticPatternsAvailable(navigator) {
		t.Fatal("the Android vibration API was unavailable")
	}
	s.playHapticPattern(&first)
	s.playHapticPattern(&second)
	// Another embedded surface replaces playback. Closing the previous
	// owner must leave the replacement running until its own owner stops.
	other := &surface{doc: doc}
	t.Cleanup(other.stopHapticPattern)
	other.playHapticPattern(&first)
	s.stopHapticPattern()
	other.stopHapticPattern()
	s.playHapticPattern(&second)
	doc.Set("visibilityState", "hidden")
	s.playHapticPattern(&first)
	s.stopHapticPattern()
	override("userAgent", "Desktop Chrome")
	doc.Set("visibilityState", "visible")
	if hapticPatternsAvailable(navigator) {
		t.Fatal("desktop Chrome reported Android vibration support")
	}
	s.playHapticPattern(&first)
	want := [][]int{{20, 70, 20, 90, 20}, {0, 10, 50}, {20, 70, 20, 90, 20}, {0}, {0, 10, 50}, {0}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("navigator.vibrate calls = %v, want %v", calls, want)
	}
}
