package core

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A frame callback runs once, with the frame's time, and not again.
func TestRequestAnimationFrameRunsOnce(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, nil)
	h.clock.Advance(5 * time.Millisecond)
	var runs int
	var at time.Time
	h.root.RequestAnimationFrame(func(when time.Time) { runs++; at = when })

	h.frame()
	if runs != 1 {
		t.Fatalf("the callback ran %d times, want once", runs)
	}
	if want := h.clock.Now(); !at.Equal(want) {
		t.Fatalf("frame time = %v, want the clock's %v", at, want)
	}
	h.clock.Advance(driver.FrameInterval)
	h.frame()
	if runs != 1 {
		t.Fatalf("an uncancelled callback ran again: %d times", runs)
	}
}

// stop cancels the wait, and so does disposing the owner that registered
// it: a screen that goes takes its frame callbacks with it.
func TestRequestAnimationFrameStops(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, nil)
	runs := 0
	stop := h.root.RequestAnimationFrame(func(time.Time) { runs++ })
	stop()
	h.frame()
	if runs != 0 {
		t.Fatalf("a stopped callback ran %d times", runs)
	}

	child := h.root.Child()
	child.RequestAnimationFrame(func(time.Time) { runs++ })
	child.Dispose()
	h.frame()
	if runs != 0 {
		t.Fatalf("a disposed owner's callback ran %d times", runs)
	}
}

// The host is asked for one frame at a time, however many callbacks wait,
// and again only when one of them waits for the next.
func TestRequestAnimationFrameAsksOncePerFrame(t *testing.T) {
	requests := 0
	root := testRoot(RootOptions{
		Post:         func(func()) {},
		Clock:        NewManualClock(time.Unix(0, 0)),
		RequestFrame: func() { requests++ },
	})
	root.RequestAnimationFrame(func(time.Time) {})
	root.RequestAnimationFrame(func(time.Time) {})
	if requests != 1 {
		t.Fatalf("the host was asked %d times for one frame, want once", requests)
	}
	root.RunFrame()
	if requests != 1 {
		t.Fatalf("the host was asked again with nothing waiting: %d", requests)
	}

	var loop func(time.Time)
	loop = func(time.Time) { root.RequestAnimationFrame(loop) }
	root.RequestAnimationFrame(loop)
	if requests != 2 {
		t.Fatalf("registering a callback did not ask for a frame: %d requests", requests)
	}
	root.RunFrame()
	if requests != 3 {
		t.Fatalf("a callback waiting again did not ask for the next frame: %d requests", requests)
	}
}

// A callback that registers another during a frame waits for the next
// one, and a callback stopped during a frame does not come back in it.
func TestRequestAnimationFrameChangesDuringAFrame(t *testing.T) {
	root := testRoot(RootOptions{
		Post:         func(func()) {},
		Clock:        NewManualClock(time.Unix(0, 0)),
		RequestFrame: func() {},
	})
	var later, stopped int
	var stop func()
	root.RequestAnimationFrame(func(time.Time) {
		root.RequestAnimationFrame(func(time.Time) { later++ })
		stop()
	})
	stop = root.RequestAnimationFrame(func(time.Time) { stopped++ })

	root.RunFrame()
	if later != 0 {
		t.Fatalf("a callback registered during a frame ran in it: %d times", later)
	}
	root.RunFrame()
	if later != 1 {
		t.Fatalf("a callback registered during a frame ran %d times, want once", later)
	}
	if stopped > 1 {
		t.Fatalf("a callback stopped during a frame ran %d times: it came back", stopped)
	}
}

// A frame the display timed runs its callbacks at the display's time, which may lie ahead of the clock, a frame without one at the clock's, and no frame's time precedes the one before: an app stepping by the difference between two frames sees the display's cadence and never a step back.
func TestRunFrameAtGivesTheDisplaysTime(t *testing.T) {
	clock := NewManualClock(time.Unix(100, 0))
	root := testRoot(RootOptions{Post: func(func()) {}, Clock: clock, RequestFrame: func() {}})
	var got []time.Time
	var loop func(time.Time)
	loop = func(at time.Time) {
		got = append(got, at)
		root.RequestAnimationFrame(loop)
	}
	root.RequestAnimationFrame(loop)

	shown := clock.Now().Add(driver.FrameInterval)
	root.RunFrameAt(shown)
	root.RunFrameAt(shown.Add(driver.FrameInterval))
	// A late frame from the clock, behind the display's last.
	clock.Advance(time.Millisecond)
	root.RunFrame()
	clock.Advance(time.Second)
	root.RunFrame()
	// A display time earlier than the frame before.
	root.RunFrameAt(shown)

	want := []time.Time{shown, shown.Add(driver.FrameInterval), shown.Add(driver.FrameInterval), clock.Now(), clock.Now()}
	if len(got) != len(want) {
		t.Fatalf("the loop ran %d times, want %d", len(got), len(want))
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("frame %d's time = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestPreferredFrameRateCadence(t *testing.T) {
	for _, display := range []int{60, 120} {
		clock := NewManualClock(time.Unix(100, 0))
		root := testRoot(RootOptions{Clock: clock, RequestFrame: func() {}})
		root.SetPreferredFrameRate(FrameRateRange{Preferred: 30})
		var times []time.Time
		var next func(time.Time)
		next = func(at time.Time) {
			times = append(times, at)
			root.RequestAnimationFrame(next)
		}
		root.RequestAnimationFrame(next)
		frame := time.Second / time.Duration(display)
		for i := range display {
			// Small timestamp differences should not turn every second
			// interval into a skipped frame on a 60 Hz display.
			jitter := time.Duration(i%3-1) * 130 * time.Microsecond
			root.RunFrameAt(clock.Now().Add(time.Duration(i)*frame + jitter))
		}
		if len(times) != 30 {
			t.Fatalf("%d Hz display: delivered %d frames, want 30", display, len(times))
		}
		for i := 1; i < len(times); i++ {
			if d := times[i].Sub(times[i-1]) - time.Second/30; d < -500*time.Microsecond || d > 500*time.Microsecond {
				t.Errorf("%d Hz display: gap %s after frame %d", display, times[i].Sub(times[i-1]), i-1)
			}
		}
		root.SetPreferredFrameRate(FrameRateRange{})
		for i := display; i < display+3; i++ {
			root.RunFrameAt(clock.Now().Add(time.Duration(i) * frame))
		}
		if len(times) != 33 {
			t.Errorf("%d Hz display: the cleared preference delivered %d more frames, want 3", display, len(times)-30)
		}
	}
}

func TestChangingFrameRateWaitsForADistinctFrame(t *testing.T) {
	start := time.Unix(100, 0)
	root := testRoot(RootOptions{Clock: NewManualClock(start), RequestFrame: func() {}})
	root.SetPreferredFrameRate(FrameRateRange{Preferred: 30})
	var frames []time.Time
	var next func(time.Time)
	next = func(at time.Time) {
		frames = append(frames, at)
		root.SetPreferredFrameRate(FrameRateRange{})
		root.RequestAnimationFrame(next)
	}
	root.RequestAnimationFrame(next)
	root.RunFrameAt(start)
	root.RunFrameAt(start.Add(125 * time.Nanosecond))
	root.RunFrameAt(start.Add(driver.FrameInterval))
	if len(frames) != 2 || !frames[1].Equal(start.Add(driver.FrameInterval)) {
		t.Errorf("retuning delivered frames at %v, want distinct display times", frames)
	}
}
