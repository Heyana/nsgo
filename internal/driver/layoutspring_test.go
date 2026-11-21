package driver

import (
	"math"
	"testing"
	"time"
)

func TestLayoutSpringRegimes(t *testing.T) {
	const resp = 250 * time.Millisecond

	// Underdamped: overshoots 1 and settles by End().
	under := LayoutTransition{Curve: LayoutSpring, Response: resp, DampingRatio: 0.4}
	peak := 0.0
	for ms := 0; ms <= 1000; ms += 5 {
		p, done := under.At(time.Duration(ms) * time.Millisecond)
		if p > peak {
			peak = p
		}
		if done {
			break
		}
	}
	if peak <= 1.2 {
		t.Fatalf("underdamped spring peak = %v, want overshoot > 1.2", peak)
	}
	if p, done := under.At(under.End()); p != 1 || !done {
		t.Fatalf("underdamped at End (%v) = %v, %v; want 1, true", under.End(), p, done)
	}

	// Critically damped: monotonic, never overshoots, settles around 1.47*Response.
	crit := LayoutTransition{Curve: LayoutSpring, Response: resp, DampingRatio: 1}
	prev := -1.0
	var critSettle time.Duration
	for ms := 0; ms <= 2000; ms++ {
		d := time.Duration(ms) * time.Millisecond
		p, done := crit.At(d)
		if p > 1+1e-9 {
			t.Fatalf("critically damped overshot at %v: %v", d, p)
		}
		if p < prev-1e-9 {
			t.Fatalf("critically damped non-monotonic at %v: %v < %v", d, p, prev)
		}
		prev = p
		if done {
			critSettle = d
			break
		}
	}
	if critSettle == 0 || critSettle < 300*time.Millisecond || critSettle > 450*time.Millisecond {
		t.Fatalf("critically damped settle = %v, want ~367ms", critSettle)
	}
	if p, done := crit.At(crit.End()); p != 1 || !done {
		t.Fatalf("critically damped at End (%v) = %v, %v; want 1, true", crit.End(), p, done)
	}

	// Overdamped: monotonic, never overshoots, settles slower than critically damped.
	over := LayoutTransition{Curve: LayoutSpring, Response: resp, DampingRatio: 2}
	prev = -1.0
	var overSettle time.Duration
	for ms := 0; ms <= 3000; ms++ {
		d := time.Duration(ms) * time.Millisecond
		p, done := over.At(d)
		if p > 1+1e-9 {
			t.Fatalf("overdamped overshot at %v: %v", d, p)
		}
		if p < prev-1e-9 {
			t.Fatalf("overdamped non-monotonic at %v: %v < %v", d, p, prev)
		}
		prev = p
		if done {
			overSettle = d
			break
		}
	}
	if overSettle <= critSettle {
		t.Fatalf("overdamped settle %v <= critically damped settle %v", overSettle, critSettle)
	}
	if p, done := over.At(over.End()); p != 1 || !done {
		t.Fatalf("overdamped at End (%v) = %v, %v; want 1, true", over.End(), p, done)
	}
}

func TestLayoutSpringRunsUntilSettledIgnoringDuration(t *testing.T) {
	// Even if Duration is short (50 ms), a physics spring with Response 400 ms
	// and low damping ratio must keep running past 50 ms until it actually settles.
	tr := LayoutTransition{
		Curve:        LayoutSpring,
		Response:     400 * time.Millisecond,
		DampingRatio: 0.3,
		Duration:     50 * time.Millisecond,
	}
	if _, done := tr.At(50 * time.Millisecond); done {
		t.Fatal("spring cut off at Duration (50 ms) instead of running until settled")
	}
	if _, done := tr.At(400 * time.Millisecond); done {
		t.Fatal("lightly damped spring settled far too early at 400 ms")
	}
	if end := tr.End(); end < time.Second {
		t.Fatalf("lightly damped spring End = %v, want > 1s", end)
	}
	if p, done := tr.At(tr.End()); p != 1 || !done {
		t.Fatalf("spring at End (%v) = %v, %v; want 1, true", tr.End(), p, done)
	}
}

func TestEvalSpring1DVelocityAndSemigroup(t *testing.T) {
	// Verify analytical velocity matches finite-difference derivative and
	// restarting from (y(t1), v(t1)) reproduces (y(t1+t2), v(t1+t2)) exactly
	// across all three damping regimes.
	const w0 = 2 * math.Pi / 0.3
	const dt = 1e-6
	for _, tc := range []struct {
		name string
		zeta float64
		y0   float64
		v0   float64
	}{
		{"underdamped", 0.45, -200, 350},
		{"critical", 1.0, -200, 350},
		{"overdamped", 1.8, -200, -150},
		{"near-critical-under", 0.99999, -150, 200},
		{"near-critical-over", 1.00001, -150, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const t1, t2 = 0.07, 0.09
			y1, v1, ok1 := evalSpring1D(tc.y0, tc.v0, w0, tc.zeta, t1, 1e-12)
			if ok1 {
				t.Fatal("unexpectedly settled at t1")
			}
			yPlus, _, _ := evalSpring1D(tc.y0, tc.v0, w0, tc.zeta, t1+dt, 1e-12)
			yMinus, _, _ := evalSpring1D(tc.y0, tc.v0, w0, tc.zeta, t1-dt, 1e-12)
			fdVel := (yPlus - yMinus) / (2 * dt)
			if math.Abs(v1-fdVel) > 1e-3 {
				t.Fatalf("velocity mismatch: analytical %v vs finite-diff %v", v1, fdVel)
			}

			// Semigroup: flow(t2, flow(t1, state)) == flow(t1+t2, state).
			yDirect, vDirect, _ := evalSpring1D(tc.y0, tc.v0, w0, tc.zeta, t1+t2, 1e-12)
			ySplit, vSplit, _ := evalSpring1D(y1, v1, w0, tc.zeta, t2, 1e-12)
			if math.Abs(yDirect-ySplit) > 1e-8 || math.Abs(vDirect-vSplit) > 1e-6 {
				t.Fatalf("semigroup mismatch: direct (%v, %v) vs split (%v, %v)", yDirect, vDirect, ySplit, vSplit)
			}
		})
	}
}

func TestLayoutMotionRetargetPreservesPositionAndVelocity(t *testing.T) {
	spring := LayoutTransition{Curve: LayoutSpring, Response: 300 * time.Millisecond, DampingRatio: 1}
	from := Bounds{Origin: Point{X: 0, Y: 0}, Size: Size{Width: 100, Height: 40}}
	midTarget := Bounds{Origin: Point{X: 300, Y: 0}, Size: Size{Width: 100, Height: 40}}

	m := StartLayoutMotion(from, midTarget, spring, 0)
	// Step 48 ms toward X = 300: view is moving rapidly in +X.
	posBefore, done := m.Step(48 * time.Millisecond)
	if done {
		t.Fatal("settled at 48 ms")
	}
	velBefore := m.Velocity()
	if velBefore[0] <= 500 {
		t.Fatalf("expected strong positive X velocity at 48 ms, got %v", velBefore[0])
	}

	// Retarget back to X = 0 (and move Y to 200) mid-flight at 48 ms.
	reverseTarget := Bounds{Origin: Point{X: 0, Y: 200}, Size: Size{Width: 100, Height: 40}}
	m.Retarget(reverseTarget, spring, 48*time.Millisecond)

	// Position must not snap on retarget.
	if m.Shown() != posBefore {
		t.Fatalf("retarget snapped position: got %v, want %v", m.Shown(), posBefore)
	}
	// Velocity must be preserved immediately at retarget time.
	if math.Abs(m.Velocity()[0]-velBefore[0]) > 1e-6 {
		t.Fatalf("retarget did not preserve X velocity: got %v, want %v", m.Velocity()[0], velBefore[0])
	}

	// 8 ms after reversing target, positive momentum must carry X slightly
	// further forward (> posBefore.Origin.X) before turning around toward 0!
	posAfter, done := m.Step(56 * time.Millisecond)
	if done {
		t.Fatal("settled immediately after retarget")
	}
	if posAfter.Origin.X <= posBefore.Origin.X {
		t.Fatalf("momentum lost on reversal: X went from %v to %v (expected brief continuation > %v)",
			posBefore.Origin.X, posAfter.Origin.X, posBefore.Origin.X)
	}
	// Meanwhile Y started from 0 with 0 velocity and moves positively toward 200.
	if posAfter.Origin.Y <= 0 || posAfter.Origin.Y >= 200 {
		t.Fatalf("Y did not begin moving toward 200: %v", posAfter.Origin.Y)
	}

	// Eventually settles cleanly at reverseTarget.
	var settled bool
	var final Bounds
	for ms := 64; ms <= 3000; ms += 16 {
		final, settled = m.Step(time.Duration(ms) * time.Millisecond)
		if settled {
			break
		}
	}
	if !settled || final != reverseTarget {
		t.Fatalf("did not settle at %v: settled=%v final=%v", reverseTarget, settled, final)
	}
}

func TestLayoutMotionRetargetToCurrentPositionWithMomentum(t *testing.T) {
	spring := LayoutTransition{Curve: LayoutSpring, Response: 250 * time.Millisecond, DampingRatio: 1}
	from := Bounds{Origin: Point{X: 0, Y: 0}, Size: Size{Width: 50, Height: 50}}
	to := Bounds{Origin: Point{X: 200, Y: 0}, Size: Size{Width: 50, Height: 50}}

	m := StartLayoutMotion(from, to, spring, 0)
	cur, _ := m.Step(40 * time.Millisecond)
	// Retarget onto the exact current position while carrying positive velocity.
	m.Retarget(cur, spring, 40*time.Millisecond)
	if m.Done() {
		t.Fatal("retarget to current position with nonzero velocity settled immediately")
	}
	next, done := m.Step(56 * time.Millisecond)
	if done || next.Origin.X <= cur.Origin.X {
		t.Fatalf("momentum did not carry past current position: cur=%v next=%v done=%v", cur.Origin.X, next.Origin.X, done)
	}
}

func TestLayoutSpringLegacyDurationFallbackAndDelay(t *testing.T) {
	legacy := LayoutTransition{Curve: LayoutSpring, Damping: 0.4, Duration: 700 * time.Millisecond}
	over := false
	for ms := 0; ms <= 700; ms += 10 {
		if p, _ := legacy.At(time.Duration(ms) * time.Millisecond); p > 1 {
			over = true
		}
	}
	if !over {
		t.Fatal("legacy spring did not overshoot")
	}
	if p, done := legacy.At(legacy.End()); p != 1 || !done {
		t.Fatalf("legacy spring at End (%v): %v, %v", legacy.End(), p, done)
	}

	delayed := LayoutTransition{Curve: LayoutSpring, Response: 200 * time.Millisecond, DampingRatio: 1, Delay: 100 * time.Millisecond}
	if p, done := delayed.At(50 * time.Millisecond); p != 0 || done {
		t.Fatalf("during delay: %v, %v", p, done)
	}
	if p, done := delayed.At(100 * time.Millisecond); p != 0 || done {
		t.Fatalf("at delay boundary: %v, %v", p, done)
	}
	if p, done := delayed.At(delayed.End()); p != 1 || !done {
		t.Fatalf("delayed at End (%v): %v, %v", delayed.End(), p, done)
	}
}
