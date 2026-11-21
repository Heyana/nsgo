package core

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestHapticPatternReplacesAndStopsInBackground(t *testing.T) {
	var scope *Scope
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		scope = cx
		return View{}
	})
	env := Environment{OS: OSAndroid, HapticPatterns: true}
	h.tree.SetEnvironment(env)
	h.commit()
	commands := func() []*driver.HapticPattern {
		var patterns []*driver.HapticPattern
		for _, item := range h.commit().Items {
			if item.Kind == driver.ItemHapticPattern {
				patterns = append(patterns, item.HapticPattern)
			}
		}
		return patterns
	}
	first := HapticPattern{{Intensity: .25, Sharpness: -1}, {At: -1, Intensity: 1}}
	scope.PlayHapticPattern(first)
	first[0].Intensity = .75
	if got := commands(); len(got) != 1 || got[0] == nil || !slices.Equal(*got[0], HapticPattern{{Intensity: .25}}) {
		t.Fatalf("first haptic pattern = %v", got)
	}
	second := HapticPattern{{At: 100 * time.Millisecond, Duration: 40 * time.Millisecond, Intensity: 1}}
	scope.PlayHapticPattern(second)
	if got := commands(); len(got) != 1 || got[0] == nil || !slices.Equal(*got[0], second) {
		t.Fatalf("replacement haptic pattern = %v", got)
	}
	env.Lifecycle = LifecycleBackground
	h.tree.SetEnvironment(env)
	if got := commands(); len(got) != 1 || got[0] != nil {
		t.Fatalf("background requires one cancellation: %v", got)
	}
	scope.PlayHapticPattern(second)
	h.tree.SetEnvironment(env)
	if got := commands(); len(got) != 0 {
		t.Fatalf("background must stay silent: %v", got)
	}
}

func TestHapticsPlayOnPhonesAlone(t *testing.T) {
	played := func(env Environment, hs ...Haptic) []uint32 {
		var scope *Scope
		h := newPhone(t, env, func(cx *Scope) Node {
			scope = cx
			return View{}
		})
		h.commit()
		for _, x := range hs {
			scope.PlayHaptic(x)
		}
		var out []uint32
		for _, it := range h.commit().Items {
			if it.Kind == driver.ItemHaptic {
				out = append(out, it.Count)
			}
		}
		return out
	}
	if got := played(phone, HapticSelection, HapticHeavy, HapticError, 0, HapticError+1); !slices.Equal(got,
		[]uint32{driver.HapticSelection, driver.HapticHeavy, driver.HapticError}) {
		t.Errorf("an iPhone played %v", got)
	}
	if got := played(Environment{OS: OSAndroid}, HapticSuccess); !slices.Equal(got, []uint32{driver.HapticSuccess}) {
		t.Errorf("Android played %v", got)
	}
	if got := played(Environment{OS: OSMacOS}, HapticMedium); len(got) != 0 {
		t.Errorf("a Mac played %v", got)
	}
	if got := played(Environment{OS: OSIOS, Idiom: IdiomPhone, Simulated: true}, HapticSelection); len(got) != 0 {
		t.Errorf("a web build that runs as iOS played %v", got)
	}
}
