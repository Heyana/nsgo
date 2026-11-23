//go:build js && wasm && nsgotest

package harmonyos

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// TestTestHostPressesPositions checks that the test host's KeyCodeDown and KeyCodeUp report InputKit's KeyCode for the position, so core.KeyCodesDown reads it, and the US layout's key as the name.
func TestTestHostPressesPositions(t *testing.T) {
	s := &surface{}
	s.m = newMounter(s)
	p := &keyboardBridgePlatform{m: s.m}
	var codes, keys []string
	s.engine = engine.Start(p, func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: func() string {
			codes, keys = core.KeyCodesDown(cx), core.KeysDown(cx)
			return strings.Join(codes, " ") + "|" + strings.Join(keys, " ")
		}}
	}, engine.Options{})
	defer s.engine.Close()
	s.engine.Sync()
	ts := &testSurface{s: s}
	wait := func() {
		until := time.Now().Add(5 * time.Second)
		for s.m.keyRunning && time.Now().Before(until) {
			time.Sleep(time.Millisecond)
		}
		s.engine.Sync()
	}
	for _, step := range []struct {
		code       string
		down       bool
		codes, key []string
	}{
		{"KeyW", true, []string{"KeyW"}, []string{"w"}},
		{"ShiftRight", true, []string{"KeyW", "ShiftRight"}, []string{"shift", "w"}},
		{"KeyW", false, []string{"ShiftRight"}, []string{"shift"}},
		{"ShiftRight", false, nil, nil},
		{"Numpad1", true, []string{"Numpad1"}, []string{"1"}},
		{"Numpad1", false, nil, nil},
	} {
		name := map[string]string{"KeyW": "w", "ShiftRight": "shift", "Numpad1": "1"}[step.code]
		var err error
		if step.down {
			err = ts.KeyCodeDown(step.code, name)
		} else {
			err = ts.KeyCodeUp(step.code, name)
		}
		if err != nil {
			t.Fatal(err)
		}
		wait()
		if !slices.Equal(codes, step.codes) || !slices.Equal(keys, step.key) {
			t.Fatalf("after %s %v: KeyCodesDown %v, KeysDown %v, want %v and %v", step.code, step.down, codes, keys, step.codes, step.key)
		}
	}
}
