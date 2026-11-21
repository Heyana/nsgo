package android

import (
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestKeyframesMatchJava(t *testing.T) {
	animations := []struct {
		name string
		anim driver.KeyframeAnimation
	}{
		{"none", driver.KeyframeAnimation{}},
		{"spin", driver.KeyframeAnimation{Keyframes: driver.KeyframesSpin}},
		{"pulse", driver.KeyframeAnimation{Keyframes: driver.KeyframesPulse}},
		{"ping", driver.KeyframeAnimation{Keyframes: driver.KeyframesPing}},
		{"bounce", driver.KeyframeAnimation{Keyframes: driver.KeyframesBounce}},
		{"custom", driver.KeyframeAnimation{
			Keyframes: driver.KeyframesSpin, Duration: 840 * time.Millisecond, Delay: 75 * time.Millisecond, Iterations: 3,
			Easing: driver.Curve{X1: 0.2, Y1: 0.4, X2: 0.8, Y2: 0.9},
		}},
	}
	progress := []float32{0, 0.125, 0.25, 0.5, 0.6875, 0.75, 0.875, 1}
	const height = 120
	const opacity, scaleX, scaleY, rotation, translateY = 0.8, 1.2, 0.7, -20, 12
	var input writer
	input.u32(uint32(len(animations)))
	wantBytes := 0
	for _, tc := range animations {
		writeKeyframes(&input, tc.anim)
		input.u32(height)
		input.floats(opacity, scaleX, scaleY, rotation, translateY)
		input.u32(uint32(len(progress)))
		input.floats(progress...)
		_, frames := tc.anim.Cycle()
		wantBytes += 24 + 36*len(frames) + 20*len(progress)
	}
	out := runJavaAlgorithm(t, "keyframes", "KeyframesCheck", input.buf, "Keyframes", "Animator", "Protocol")
	if len(out) != wantBytes {
		t.Fatalf("Java wrote %d bytes, want %d", len(out), wantBytes)
	}
	r := reader{out}
	for _, tc := range animations {
		t.Run(tc.name, func(t *testing.T) {
			d, frames := tc.anim.Cycle()
			if duration, delay, iterations, count := r.u64(), r.u64(), r.u32(), r.u32(); duration != uint64(d.Milliseconds()) ||
				delay != uint64(tc.anim.Delay.Milliseconds()) || iterations != uint32(tc.anim.Iterations) || count != uint32(len(frames)) {
				t.Errorf("metadata duration %d, delay %d, iterations %d, steps %d", duration, delay, iterations, count)
			}
			for i, k := range frames {
				want := [9]float32{k.Offset, k.Opacity, k.Scale, k.Rotate, k.TranslateY, k.Easing.X1, k.Easing.Y1, k.Easing.X2, k.Easing.Y2}
				for j, value := range want {
					if got := r.float(); got != value {
						t.Errorf("step %d field %d: Java %g, Go %g", i, j, got, value)
					}
				}
			}
			// Java receives a cycle's progress from ValueAnimator. Sample
			// Go's single cycle so progress 1 means the final step.
			cycle := tc.anim
			cycle.Iterations = 1
			for _, fraction := range progress {
				k, _ := cycle.At(cycle.Delay + time.Duration(float64(d)*float64(fraction)))
				want := [5]float32{k.Opacity * opacity, k.Scale * scaleX, k.Scale * scaleY, rotation + k.Rotate, translateY + k.TranslateY*height}
				for i, value := range want {
					got := r.float()
					difference := math.Abs(float64(got - value))
					if math.IsNaN(difference) || difference > max(1e-4, 1e-5*math.Abs(float64(value))) {
						t.Errorf("progress %g field %d: Java %g, Go %g", fraction, i, got, value)
					}
				}
			}
		})
	}
}
