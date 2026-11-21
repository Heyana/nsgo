package driver

import (
	"slices"
	"testing"
	"time"
)

func TestHapticWaveformKeepsTimingAndPeakAmplitude(t *testing.T) {
	p := HapticPattern{
		{At: 80 * time.Millisecond, Intensity: 1},
		{At: 20 * time.Millisecond, Duration: 20 * time.Millisecond, Intensity: .25},
		{At: 30 * time.Millisecond, Duration: 20 * time.Millisecond, Intensity: .75},
	}
	want := []HapticSegment{{20, 0}, {10, 64}, {20, 191}, {30, 0}, {20, 255}}
	if got := HapticWaveform(p); !slices.Equal(got, want) {
		t.Fatalf("waveform = %v, want %v", got, want)
	}
}
