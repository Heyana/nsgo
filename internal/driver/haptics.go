package driver

import (
	"math"
	"slices"
	"time"
)

// HapticEvent is one pulse of a pattern. At is relative to the start.
// A zero Duration makes a transient. Intensity and Sharpness range from 0 to 1.
type HapticEvent struct {
	At, Duration         time.Duration
	Intensity, Sharpness float32
}

// HapticPattern lists the timed events to play together.
type HapticPattern []HapticEvent

// HapticSegment is one constant-amplitude interval of an Android or web waveform.
type HapticSegment struct {
	Milliseconds int64
	Amplitude    uint8
}

// HapticWaveform divides the pattern into intervals. A transient is a
// 20 ms pulse on a vibrator, and overlapping events take the stronger
// amplitude. Amplitude zero is a pause.
func HapticWaveform(pattern HapticPattern) []HapticSegment {
	type pulse struct {
		start, end int64
		amplitude  uint8
	}
	pulses := make([]pulse, 0, len(pattern))
	edges := make([]int64, 1, 1+2*len(pattern))
	for _, event := range pattern {
		if event.Intensity <= 0 {
			continue
		}
		duration := max(event.Duration.Milliseconds(), 1)
		if event.Duration == 0 {
			duration = 20
		}
		start := event.At.Milliseconds()
		end := start + duration
		amp := max(1, uint8(math.Round(float64(event.Intensity*255))))
		pulses = append(pulses, pulse{start, end, amp})
		edges = append(edges, start, end)
	}
	slices.Sort(edges)
	edges = slices.Compact(edges)
	var result []HapticSegment
	for i := 1; i < len(edges); i++ {
		before, after := edges[i-1], edges[i]
		var amplitude uint8
		for _, pulse := range pulses {
			if pulse.start <= before && before < pulse.end {
				amplitude = max(amplitude, pulse.amplitude)
			}
		}
		if n := len(result); n > 0 && result[n-1].Amplitude == amplitude {
			result[n-1].Milliseconds += after - before
		} else {
			result = append(result, HapticSegment{after - before, amplitude})
		}
	}
	return result
}
