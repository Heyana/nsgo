package linux

import (
	"math"
	"testing"
	"time"
)

func TestShieldScrollReleaseVelocity(t *testing.T) {
	var s shieldScroll
	t0 := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)

	if !s.delta(1000, t0, 0, 12) {
		t.Fatal("first delta did not report began")
	}
	if s.delta(1008, t0.Add(8*time.Millisecond), 0, 12) {
		t.Fatal("second delta unexpectedly reported began")
	}
	if s.delta(1016, t0.Add(16*time.Millisecond), 0, 12) {
		t.Fatal("third delta unexpectedly reported began")
	}
	if s.delta(1024, t0.Add(24*time.Millisecond), 0, 12) {
		t.Fatal("fourth delta unexpectedly reported began")
	}
	vx, vy, ok := s.stop(1024, t0.Add(24*time.Millisecond))
	if !ok {
		t.Fatal("stop returned ok=false, want true")
	}
	if vx != 0 || math.Abs(vy-1500) > 1 {
		t.Fatalf("release velocity = (%v, %v), want (0, 1500)", vx, vy)
	}
	if s.active {
		t.Fatal("tracker remained active after stop")
	}
}

func TestShieldScrollHoldThenFlickAndCancelWithoutMove(t *testing.T) {
	var s shieldScroll
	t0 := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)

	// Hold followed by lift without scrolling yields no release.
	s.hold(1000, t0)
	if !s.active || s.moved {
		t.Fatalf("after hold: active=%v moved=%v, want true/false", s.active, s.moved)
	}
	if _, _, ok := s.stop(1050, t0.Add(50*time.Millisecond)); ok {
		t.Fatal("stop after hold without deltas returned ok=true, want false")
	}
	if s.active {
		t.Fatal("tracker remained active after stop without deltas")
	}

	// Hold followed by diagonal deltas yields velocity along both axes,
	// without delta reporting began (the hold already started it).
	s.hold(2000, t0)
	if s.delta(2010, t0.Add(10*time.Millisecond), -10, 20) {
		t.Fatal("delta after hold reported began")
	}
	if s.delta(2020, t0.Add(20*time.Millisecond), -10, 20) {
		t.Fatal("second delta after hold reported began")
	}
	vx, vy, ok := s.stop(2020, t0.Add(20*time.Millisecond))
	if !ok {
		t.Fatal("stop after hold+deltas returned ok=false")
	}
	if math.Abs(vx-(-1000)) > 1 || math.Abs(vy-2000) > 1 {
		t.Fatalf("velocity = (%v, %v), want (-1000, 2000)", vx, vy)
	}
}

func TestShieldScrollPauseStopsVelocity(t *testing.T) {
	var s shieldScroll
	t0 := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)

	s.delta(1000, t0, 0, 20)
	s.delta(1010, t0.Add(10*time.Millisecond), 0, 20)
	s.delta(1020, t0.Add(20*time.Millisecond), 0, 20)
	// Fingers pause for 150 ms before lifting: release still reports ok=true
	// (so OnWheel receives GestureEnded), with zero velocity so scroll views
	// do not glide.
	vx, vy, ok := s.stop(1170, t0.Add(170*time.Millisecond))
	if !ok {
		t.Fatal("stop after pause returned ok=false, want true")
	}
	if vx != 0 || vy != 0 {
		t.Fatalf("velocity after pause = (%v, %v), want (0, 0)", vx, vy)
	}
}

func TestShieldScrollTimestampWrapAndWallFallback(t *testing.T) {
	t0 := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)

	// GDK_CURRENT_TIME (0) falls back to wall timestamps.
	var wall shieldScroll
	wall.delta(0, t0, 0, 15)
	wall.delta(0, t0.Add(10*time.Millisecond), 0, 15)
	wall.delta(0, t0.Add(20*time.Millisecond), 0, 15)
	vx, vy, ok := wall.stop(0, t0.Add(20*time.Millisecond))
	if !ok || vx != 0 || math.Abs(vy-1500) > 1 {
		t.Fatalf("wall fallback velocity = (%v, %v, %v), want (0, 1500, true)", vx, vy, ok)
	}

	// 32-bit GDK millisecond timestamp wraparound across 0xFFFFFFFF.
	var wrap shieldScroll
	wrap.delta(0xfffffff0, t0, 0, 16)
	wrap.delta(0xfffffff8, t0.Add(8*time.Millisecond), 0, 16)
	wrap.delta(0x00000008, t0.Add(24*time.Millisecond), 0, 16)
	_, vy, ok = wrap.stop(0x00000008, t0.Add(24*time.Millisecond))
	if !ok || math.Abs(vy-1333.33) > 5 {
		t.Fatalf("wrapped timestamp vy = %v (ok=%v), want ~1333", vy, ok)
	}
}

func TestShieldScrollLatchingAndStaleExpiry(t *testing.T) {
	var s shieldScroll
	t0 := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)

	if s.latched(1000, t0, false, false) {
		t.Fatal("inactive tracker reported latched")
	}

	s.delta(1000, t0, 0, 15)
	// Hosted view scrolled out from under pointer 8 ms later: stays latched.
	if !s.latched(1008, t0.Add(8*time.Millisecond), false, false) {
		t.Fatal("tracker did not stay latched within shieldScrollIdle")
	}
	// Stop event always latches while active.
	if !s.latched(1500, t0.Add(500*time.Millisecond), false, true) {
		t.Fatal("tracker did not latch stop event")
	}
	// After shieldScrollIdle without a stop event over an unshielded control,
	// latch expires so a later scroll on an unshielded control is not hijacked.
	if s.latched(1200, t0.Add(200*time.Millisecond), false, false) {
		t.Fatal("tracker stayed latched past shieldScrollIdle over unshielded point")
	}
	// If a new shielded delta arrives after shieldScrollIdle without an
	// intervening stop, delta restarts a fresh gesture and reports began=true.
	if !s.delta(1300, t0.Add(300*time.Millisecond), 0, 10) {
		t.Fatal("stale delta did not begin a new gesture")
	}
	s.reset()
	if s.active {
		t.Fatal("reset did not clear active state")
	}
}
