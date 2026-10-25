package core

import (
	"testing"
	"time"
)

func TestKeyframeOffsetsSpreadAsTheWebDoes(t *testing.T) {
	stops := append(Stops(0, 1), StopAt(0.8, 2), KeyframeStop{Value: 3})
	var got []float64
	for _, s := range resolveStops(stops) {
		got = append(got, s.offset)
	}
	want := []float64{0, 0.4, 0.8, 1}
	for i := range want {
		if d := got[i] - want[i]; d > 1e-9 || d < -1e-9 {
			t.Fatalf("offsets %v, want %v", got, want)
		}
	}
	// An offset below the one before is held there.
	if r := resolveStops([]KeyframeStop{StopAt(0.5, 0), StopAt(0.2, 1), StopAt(1, 2)}); r[1].offset != 0.5 {
		t.Fatalf("held offset = %v", r[1].offset)
	}
}

func TestKeyframesBuildTimingTrees(t *testing.T) {
	v := &AnimatedValue{animNode{id: 7}}
	a := ValueKeyframes(v, KeyframeTiming{Duration: time.Second, Iterations: 3, Direction: PlayAlternate,
		Delay: 100 * time.Millisecond}, Stops(0, 10, 20)...)
	// A delay holding the first stop, then two passes looped once, then a
	// forward pass.
	if a.Kind != AnimationSequence || len(a.Steps) != 3 || a.Steps[0].Kind != AnimationTiming || a.Steps[0].To != 0 ||
		a.Steps[1].Kind != AnimationDelay {
		t.Fatalf("delayed keyframes = %+v", a)
	}
	body := a.Steps[2]
	if body.Kind != AnimationSequence || body.Steps[0].Kind != AnimationLoop || body.Steps[0].Iterations != 1 {
		t.Fatalf("body = %+v", body)
	}
	back := body.Steps[0].Steps[0].Steps[1]
	if back.Steps[0].To != 20 || back.Steps[2].To != 0 || back.Steps[1].Timing.Duration != 500*time.Millisecond {
		t.Fatalf("backward pass = %+v", back)
	}
	for _, p := range MotionPresets() {
		spec := p.Spec()
		for _, tr := range spec.Tracks {
			start, end, _ := keyframeEnds(tr.Stops, spec.Timing)
			rest := tr.Channel.identity()
			switch p.Kind() {
			case MotionEnter:
				if end != rest {
					t.Errorf("%s ends %v at %v", p.Name(), tr.Channel, end)
				}
			case MotionExit:
				if start != rest {
					t.Errorf("%s starts %v at %v", p.Name(), tr.Channel, start)
				}
			case MotionAttention:
				if start != rest || end != rest {
					t.Errorf("%s moves %v from %v to %v", p.Name(), tr.Channel, start, end)
				}
			}
		}
	}
}
