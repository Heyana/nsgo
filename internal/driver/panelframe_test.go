package driver

import (
	"testing"
	"time"
)

func TestPanelFrameMotionRetargetsFromTheDisplayedFrame(t *testing.T) {
	start := time.Unix(0, 0)
	strip := Bounds{Origin: Point{X: 314, Y: 20}, Size: Size{Width: 6, Height: 400}}
	expanded := Bounds{Origin: Point{Y: 20}, Size: Size{Width: 320, Height: 400}}
	var motion PanelFrameMotion
	motion.Start(strip, expanded, start)
	half := start.Add(100 * time.Millisecond)
	shown, ok := motion.Step(half, false)
	if !ok || shown.Size.Width <= 163 || shown.Size.Width >= 320 || shown.Origin.X+shown.Size.Width != 320 {
		t.Fatalf("halfway through easing, the panel should be more than half open and still right-aligned: %+v", shown)
	}
	motion.Start(shown, strip, half)
	if got, ok := motion.Step(half, false); !ok || got != shown {
		t.Fatalf("reversing moved the panel from %+v to %+v", shown, got)
	}
	later := half.Add(100 * time.Millisecond)
	closing, ok := motion.Step(later, false)
	if !ok || closing.Size.Width <= 6 || closing.Size.Width >= shown.Size.Width {
		t.Fatalf("the panel should be closing: %+v", closing)
	}
	motion.Start(closing, strip, later)
	if got, ok := motion.Step(half.Add(200*time.Millisecond), false); !ok || got != strip {
		t.Fatalf("repeating the same destination delayed arrival at the strip: %+v", got)
	}
	if got, ok := motion.Step(half.Add(time.Second), false); ok {
		t.Fatalf("a finished motion still asked to place a frame: %+v", got)
	}
}

func TestPanelFrameMotionReducedMotionAndCancellation(t *testing.T) {
	now := time.Unix(0, 0)
	from := Bounds{Size: Size{Width: 300, Height: 200}}
	to := Bounds{Origin: Point{X: -20, Y: 40}, Size: Size{Width: 6, Height: 200}}
	var motion PanelFrameMotion
	motion.Start(from, to, now)
	if got, ok := motion.Step(now.Add(time.Millisecond), true); !ok || got != to || motion.Active() {
		t.Fatalf("turning on reduced motion should finish at the target immediately: %+v", got)
	}
	motion.Start(to, from, now)
	motion.Stop()
	if got, ok := motion.Step(now.Add(time.Second), false); ok {
		t.Fatalf("a canceled motion still asked to place a frame: %+v", got)
	}
}
