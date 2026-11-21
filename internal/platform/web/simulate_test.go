//go:build js && wasm

package web

import (
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A simulated OS replaces the browser's, with its idiom and pointer; the web, or none, is not simulated.
func TestSimulate(t *testing.T) {
	for _, tc := range []struct {
		os        driver.OS
		want      driver.OS
		idiom     driver.Idiom
		pointer   driver.Pointer
		simulated bool
	}{
		{driver.OSIOS, driver.OSIOS, driver.IdiomPhone, driver.PointerCoarse, true},
		{driver.OSAndroid, driver.OSAndroid, driver.IdiomPhone, driver.PointerCoarse, true},
		{driver.OSMacOS, driver.OSMacOS, driver.IdiomDesktop, driver.PointerFine, true},
		{driver.OSWindows, driver.OSWindows, driver.IdiomDesktop, driver.PointerFine, true},
		{driver.OSLinux, driver.OSLinux, driver.IdiomDesktop, driver.PointerFine, true},
		{driver.OSWeb, driver.OSWeb, driver.IdiomTablet, driver.PointerFine, false},
		{"", driver.OSWeb, driver.IdiomTablet, driver.PointerFine, false},
	} {
		env := driver.Environment{OS: driver.OSWeb, Idiom: driver.IdiomTablet, Pointer: driver.PointerFine}
		simulate(&env, tc.os)
		if env.OS != tc.want || env.Idiom != tc.idiom || env.Pointer != tc.pointer || env.Simulated != tc.simulated {
			t.Errorf("%q: OS %q, idiom %v, pointer %v, simulated %v", tc.os, env.OS, env.Idiom, env.Pointer, env.Simulated)
		}
	}
}

// The top window's page has the history, and so does a frame's whose element opts in with data-nsgo-history; a cross-origin frame's element is null.
func TestOwnsHistory(t *testing.T) {
	object := js.Global().Get("Object")
	frame := func(attr bool) js.Value {
		el := object.New()
		has := js.FuncOf(func(_ js.Value, args []js.Value) any {
			return attr && args[0].String() == "data-nsgo-history"
		})
		t.Cleanup(has.Release)
		el.Set("hasAttribute", has)
		return el
	}
	top := object.New()
	top.Set("top", top)
	top.Set("frameElement", js.Null())
	window := func(el js.Value) js.Value {
		w := object.New()
		w.Set("top", top)
		w.Set("frameElement", el)
		return w
	}
	for _, tc := range []struct {
		name string
		win  js.Value
		want bool
	}{
		{"top", top, true},
		{"frame", window(frame(false)), false},
		{"frame that opts in", window(frame(true)), true},
		{"cross-origin frame", window(js.Null()), false},
	} {
		if got := ownsHistory(tc.win); got != tc.want {
			t.Errorf("%s: owns = %v", tc.name, got)
		}
	}
}
