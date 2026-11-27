package ios

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestChoosePiPWake(t *testing.T) {
	cases := []struct {
		name      string
		closed    bool
		lifecycle driver.Lifecycle
		hasLayer  bool
		continues bool
		want      pipWakeMode
	}{
		{"closed", true, driver.LifecycleActive, true, true, pipWakeNone},
		{"no layer", false, driver.LifecycleActive, false, true, pipWakeNone},
		{"active foreground", false, driver.LifecycleActive, true, false, pipWakeDisplayLink},
		{"inactive foreground", false, driver.LifecycleInactive, true, false, pipWakeDisplayLink},
		{"background active pip", false, driver.LifecycleBackground, true, true, pipWakeBackgroundTimer},
		{"background stopped pip", false, driver.LifecycleBackground, true, false, pipWakeNone},
	}
	for _, tc := range cases {
		if got := choosePiPWake(tc.closed, tc.lifecycle, tc.hasLayer, tc.continues); got != tc.want {
			t.Fatalf("%s: choosePiPWake = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestNextPiPBackgroundDelay(t *testing.T) {
	if got := nextPiPBackgroundDelay(0); got != pipBackgroundFrameInterval {
		t.Fatalf("zero elapsed delay = %v, want %v", got, pipBackgroundFrameInterval)
	}
	if got := nextPiPBackgroundDelay(5 * time.Millisecond); got != pipBackgroundFrameInterval-5*time.Millisecond {
		t.Fatalf("partial elapsed delay = %v", got)
	}
	if got := nextPiPBackgroundDelay(50 * time.Millisecond); got != time.Millisecond {
		t.Fatalf("overrun delay = %v, want 1ms floor", got)
	}
}
