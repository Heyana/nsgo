package android

import (
	"bytes"
	"fmt"
	"math"
	"os/exec"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type motionSample struct {
	name string
	want float64
}

func TestMotionMatchesJava(t *testing.T) {
	javac, java := androidJDK(t)
	classes := t.TempDir()
	cmd := exec.CommandContext(t.Context(), javac, "-d", classes,
		"java/dev/oneo/nsgo/Protocol.java", "java/dev/oneo/nsgo/Animator.java",
		"java/dev/oneo/nsgo/ScrollPhysics.java", "java/dev/oneo/nsgo/LayoutTransition.java",
		"testdata/motion/android/animation/TimeInterpolator.java", "testdata/motion/dev/oneo/nsgo/MotionCheck.java")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	var input writer
	want := scrollSamples(&input)
	want = append(want, stickySamples(&input)...)
	want = append(want, layoutSamples(&input)...)
	want = append(want, retargetSamples(&input)...)
	cmd = exec.CommandContext(t.Context(), java, "-cp", classes, "dev.oneo.nsgo.MotionCheck")
	cmd.Stdin = bytes.NewReader(input.buf)
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("java: %v\n%s", err, diagnostics.Bytes())
	}
	if len(out) != 8*len(want) {
		t.Fatalf("Java wrote %d bytes, want %d", len(out), 8*len(want))
	}
	r := reader{out}
	for _, sample := range want {
		got := math.Float64frombits(r.u64())
		delta := math.Abs(got - sample.want)
		if math.IsNaN(got) != math.IsNaN(sample.want) || delta > 1e-4 {
			t.Errorf("%s: Java %g, Go %g", sample.name, got, sample.want)
		}
	}
	t.Logf("compared %d Go and Java results", len(want))
}

func scrollSamples(w *writer) []motionSample {
	modes := []struct {
		name     string
		behavior driver.ScrollBehavior
	}{
		{"free", driver.ScrollBehavior{}},
		{"paging", driver.ScrollBehavior{Paging: true}},
		{"interval", driver.ScrollBehavior{SnapToInterval: 40}},
		{"center", driver.ScrollBehavior{SnapToInterval: 40, SnapToAlignment: driver.SnapCenter}},
		{"end", driver.ScrollBehavior{SnapToInterval: 140, SnapToAlignment: driver.SnapEnd}},
		{"offsets", driver.ScrollBehavior{SnapToOffsets: []driver.Points{84, -12, 40, 41, 310}}},
		{"one step", driver.ScrollBehavior{SnapToInterval: 40, DisableIntervalMomentum: true}},
	}
	velocities := []float32{-1.2, -0.3, 0, 0.3, 1.2}
	offsets := []driver.Points{-8, 39.5, 40.25, 81, 229}
	contentLengths := []driver.Points{86, 329}
	w.u32(uint32(len(modes) * len(contentLengths) * len(offsets) * len(velocities)))
	var samples []motionSample
	const viewport = 100
	for _, mode := range modes {
		b := mode.behavior
		rate := b.Deceleration()
		for _, content := range contentLengths {
			for _, offset := range offsets {
				for _, velocity := range velocities {
					w.points(b.SnapToInterval)
					w.byte(byte(b.SnapToAlignment))
					w.flags(b.Paging)
					w.flags(b.DisableIntervalMomentum)
					w.u32(uint32(len(b.SnapToOffsets)))
					for _, point := range b.SnapToOffsets {
						w.points(point)
					}
					w.points(offset)
					w.floats(velocity, rate, viewport, float32(content))
					projected := driver.Projected(offset, velocity, rate)
					snap, ok := b.SnapTarget(offset, projected, velocity, viewport, content)
					target := math.NaN()
					if ok {
						target = float64(snap)
					}
					name := fmt.Sprintf("%s content %g offset %g velocity %g", mode.name, content, offset, velocity)
					samples = append(samples, motionSample{name + " projected", float64(projected)}, motionSample{name + " snap", target})
				}
			}
		}
	}
	return samples
}

func stickySamples(w *writer) []motionSample {
	a := driver.NewAnimator(nil)
	a.Apply([]driver.AnimationOp{{Kind: driver.AnimCreateValue, Node: 1, Value: 22.25}})
	cells := []driver.StickyCell{
		{Start: 10, End: 50},
		{Start: 50, End: 10},
		{Start: 10, End: driver.Points(math.Inf(1))},
		{Start: 10, End: 50, Node: 1, Inset: 14},
		{Start: 50, End: 10, Node: 1, Inset: 28},
		{Start: 10, End: 50, Node: 2, Inset: 14},
	}
	offsets := []driver.Points{-5, 10, 20, 45, 75}
	w.u32(uint32(len(cells) * len(offsets)))
	var samples []motionSample
	for i, cell := range cells {
		animatedInset, _, ok := a.Value(cell.Node)
		if !ok {
			animatedInset = math.NaN()
		}
		for _, offset := range offsets {
			w.floats(float32(cell.Start), float32(cell.End), float32(offset))
			w.f64(animatedInset)
			w.points(cell.Inset)
			name := fmt.Sprintf("sticky cell %d offset %g", i, offset)
			samples = append(samples, motionSample{name, float64(a.StickyShift(cell, offset))})
		}
	}
	return samples
}

func layoutSamples(w *writer) []motionSample {
	d, delay := 200*time.Millisecond, 40*time.Millisecond
	// Android uses the IME's own keyboard curve.
	transitions := []driver.LayoutTransition{
		{},
		{Curve: driver.LayoutLinear, Duration: d},
		{Curve: driver.LayoutEaseIn, Duration: d, Delay: delay},
		{Curve: driver.LayoutEaseOut, Duration: d, Delay: delay},
		{Curve: driver.LayoutEaseInEaseOut, Duration: d, Delay: delay},
		{Curve: driver.LayoutSpring, Duration: d, Delay: delay, Damping: 0.25},
		{Curve: driver.LayoutSpring, Duration: d, Damping: 1},
		{Curve: driver.LayoutSpring, Response: 300 * time.Millisecond, DampingRatio: 0.3},
		{Curve: driver.LayoutSpring, Response: 300 * time.Millisecond, DampingRatio: 1, Delay: delay},
		{Curve: driver.LayoutSpring, Response: 300 * time.Millisecond, DampingRatio: 2.5},
		{Curve: driver.LayoutSpring},
		{Curve: driver.LayoutBezier, Duration: d, Bezier: driver.Curve{X1: 0.12, Y1: 0.76, X2: 0.77, Y2: 1.23}},
	}
	// The later times reach springs that run past Duration until they settle.
	times := []time.Duration{-time.Millisecond, 0, delay, delay + d/10, d / 2, d, delay + d, delay + d + time.Millisecond,
		400 * time.Millisecond, time.Second, 3 * time.Second}
	w.u32(uint32(len(transitions) * len(times)))
	var samples []motionSample
	for i, tr := range transitions {
		for _, elapsed := range times {
			writeLayoutTransition(w, &tr)
			w.u64(uint64(elapsed))
			at, done := tr.At(elapsed)
			finished := 0.0
			if done {
				finished = 1
			}
			name := fmt.Sprintf("layout transition %d at %v", i, elapsed)
			samples = append(samples, motionSample{name, at}, motionSample{name + " done", finished})
		}
	}
	return samples
}

// retargetSamples moves frames with Android's LayoutTransition.Motion and Go's driver.LayoutMotion, redirecting each mid-flight, and compares the frames shown, the velocities and whether the motions settled.
func retargetSamples(w *writer) []motionSample {
	spring := driver.LayoutTransition{Curve: driver.LayoutSpring, Response: 300 * time.Millisecond, DampingRatio: 0.4}
	stiff := driver.LayoutTransition{Curve: driver.LayoutSpring, Response: 150 * time.Millisecond, DampingRatio: 1}
	linear := driver.LayoutTransition{Curve: driver.LayoutLinear, Duration: 200 * time.Millisecond}
	frame := func(x, y, width, height driver.Points) driver.Bounds {
		return driver.Bounds{Origin: driver.Point{X: x, Y: y}, Size: driver.Size{Width: width, Height: height}}
	}
	cases := []struct {
		from, to, next driver.Bounds
		tr, nextTr     driver.LayoutTransition
		at             time.Duration
	}{
		// Back toward where it started while moving fast, as a toggle pressed twice.
		{frame(0, 0, 100, 40), frame(200, 0, 100, 40), frame(0, 80, 100, 40), spring, spring, 48 * time.Millisecond},
		// Onto where it already is, its momentum carrying it past.
		{frame(0, 0, 100, 40), frame(200, 0, 160, 40), frame(120, 0, 120, 40), spring, stiff, 90 * time.Millisecond},
		// A timed curve drops the spring's velocity.
		{frame(0, 0, 100, 40), frame(200, 0, 100, 40), frame(50, 0, 100, 40), spring, linear, 48 * time.Millisecond},
		// After it settled, a new target starts from rest.
		{frame(0, 0, 100, 40), frame(200, 0, 100, 40), frame(0, 0, 100, 40), stiff, spring, 3 * time.Second},
	}
	steps := []time.Duration{0, 16 * time.Millisecond, 100 * time.Millisecond, 250 * time.Millisecond, 600 * time.Millisecond, 2 * time.Second}
	w.u32(uint32(len(cases)))
	w.u32(uint32(len(steps)))
	var samples []motionSample
	for i, c := range cases {
		w.bounds(c.from)
		w.bounds(c.to)
		writeLayoutTransition(w, &c.tr)
		w.u64(uint64(c.at))
		w.bounds(c.next)
		writeLayoutTransition(w, &c.nextTr)
		m := driver.StartLayoutMotion(c.from, c.to, c.tr, 0)
		m.Retarget(c.next, c.nextTr, c.at)
		for _, step := range steps {
			w.u64(uint64(c.at + step))
			shown, done := m.Step(c.at + step)
			name := fmt.Sprintf("retarget %d at %v after", i, step)
			vel := m.Velocity()
			finished := 0.0
			if done {
				finished = 1
			}
			samples = append(samples,
				motionSample{name + " x", float64(shown.Origin.X)}, motionSample{name + " y", float64(shown.Origin.Y)},
				motionSample{name + " width", float64(shown.Size.Width)}, motionSample{name + " height", float64(shown.Size.Height)},
				motionSample{name + " x velocity", vel[0]}, motionSample{name + " y velocity", vel[1]},
				motionSample{name + " width velocity", vel[2]}, motionSample{name + " height velocity", vel[3]},
				motionSample{name + " done", finished})
		}
	}
	return samples
}
