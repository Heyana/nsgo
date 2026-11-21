package core

import (
	"math"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Haptic is feedback the device plays under the user's finger: the
// core's part of mobile.Haptic, which apps use.
type Haptic uint8

// The haptics, as iOS's feedback generators name them: a selection
// changing, an impact of a weight, and a task's outcome.
const (
	// HapticSelection is a selection changing: a picker's value, a
	// segment, a slider's step.
	HapticSelection Haptic = iota + 1
	// HapticLight to HapticRigid are elements colliding, by their weight
	// and give.
	HapticLight
	HapticMedium
	HapticHeavy
	HapticSoft
	HapticRigid
	// HapticSuccess, HapticWarning and HapticError are a task ending so.
	HapticSuccess
	HapticWarning
	HapticError
)

// PlayHaptic is the core's part of mobile.PlayHaptic, which apps use: it
// plays h on iOS and Android, and does nothing elsewhere.
func (cx *Scope) PlayHaptic(h Haptic) {
	if h < HapticSelection || h > HapticError {
		return
	}
	if os := cx.Env().hostUntracked(); os != OSIOS && os != OSAndroid {
		return
	}
	if t := cx.tree(); t != nil {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemHaptic, Count: uint32(h)})
		t.changed = true
	}
}

// HapticEvent is the core's part of mobile.HapticEvent.
type HapticEvent = driver.HapticEvent

// HapticPattern is the core's part of mobile.HapticPattern.
type HapticPattern = driver.HapticPattern

// PlayHapticPattern is the core's part of mobile.PlayHapticPattern. A
// new pattern replaces one already playing. An empty pattern stops it.
func (cx *Scope) PlayHapticPattern(pattern HapticPattern) {
	env := cx.Env()
	if !env.hapticPatterns.GetUntracked() || env.lifecycle.GetUntracked() == LifecycleBackground {
		return
	}
	t := cx.tree()
	if t == nil {
		return
	}
	if len(pattern) == 0 {
		t.setHapticPattern(nil)
		return
	}
	pulses := make(HapticPattern, 0, len(pattern))
	for _, pulse := range pattern {
		if pulse.At < 0 || pulse.Duration < 0 || math.IsNaN(float64(pulse.Intensity)) || math.IsNaN(float64(pulse.Sharpness)) {
			continue
		}
		pulse.Duration = min(pulse.Duration, 30*time.Second)
		pulse.Intensity = min(1, max(0, pulse.Intensity))
		pulse.Sharpness = min(1, max(0, pulse.Sharpness))
		pulses = append(pulses, pulse)
	}
	if len(pulses) > 0 {
		t.setHapticPattern(&pulses)
	}
}

func (t *Tree) setHapticPattern(pattern *HapticPattern) {
	t.hapticActive = pattern != nil
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemHapticPattern, HapticPattern: pattern})
	t.changed = true
}
