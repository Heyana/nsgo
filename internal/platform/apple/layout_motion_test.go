//go:build darwin

package apple

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestStepLayoutMotionsUsesCurrentViewsAndFinishesEachTrack(t *testing.T) {
	frame := driver.StartLayoutMotion(driver.Bounds{Origin: driver.Point{X: 10}}, driver.Bounds{Origin: driver.Point{X: 110}},
		driver.LayoutTransition{Curve: driver.LayoutLinear, Duration: 100 * time.Millisecond}, 10*time.Millisecond)
	stale := frame
	m := &LayoutMotion{Frame: &frame, Appear: &driver.LayoutTransition{Curve: driver.LayoutLinear, Duration: 75 * time.Millisecond}, AppearStart: 10 * time.Millisecond}
	motions := map[int]*LayoutMotion{1: m, 2: {Frame: &stale}}
	current := func(id int) bool { return id == 1 }
	var shown driver.Bounds
	var rests int
	place := func(id int, frame driver.Bounds) {
		if id != 1 {
			t.Fatalf("placed an old view %d", id)
		}
		shown = frame
	}
	restyle := func(id int) {
		if id != 1 {
			t.Fatalf("restyled an old view %d", id)
		}
		rests++
	}

	StepLayoutMotions(60*time.Millisecond, motions, nil, current, place, restyle)
	if len(motions) != 1 || shown.Origin.X != 60 || rests != 1 {
		t.Fatalf("midway got %d motions, frame %v and %d restyles", len(motions), shown, rests)
	}
	StepLayoutMotions(90*time.Millisecond, motions, nil, current, place, restyle)
	if m.Frame == nil || m.Appear != nil || rests != 2 {
		t.Fatalf("entrance should end while the frame still moves: %+v and %d restyles", m, rests)
	}
	StepLayoutMotions(110*time.Millisecond, motions, nil, current, place, restyle)
	if len(motions) != 0 || shown.Origin.X != 110 || rests != 2 {
		t.Fatalf("at the end got %d motions, frame %v and %d restyles", len(motions), shown, rests)
	}
}
