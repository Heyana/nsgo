//go:build darwin

package apple

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestLayoutHostStartsOnlyInsertedSubtreeRootsAndRetargetsCurrentFrames(t *testing.T) {
	frame := driver.Bounds{Origin: driver.Point{X: 4}, Size: driver.Size{Width: 20}}
	motions := map[int]*LayoutMotion{}
	ticks, placements := 0, 0
	now := 10 * time.Millisecond
	host := LayoutHost[int]{
		Motions: motions,
		ID:      func(v int) driver.NodeID { return driver.NodeID(v) },
		Framed:  func(int) bool { return true },
		Frame:   func(int) driver.Bounds { return frame },
		Place:   func(_ int, next driver.Bounds) { frame = next; placements++ },
		Now:     func() time.Duration { return now },
		Start:   func(*driver.LayoutTransition) time.Duration { return 5 * time.Millisecond },
		Tick:    func() { ticks++ },
	}
	create := &driver.LayoutTransition{Curve: driver.LayoutLinear, Duration: time.Second}
	update := &driver.LayoutTransition{Curve: driver.LayoutLinear, Duration: 100 * time.Millisecond}
	batch := &LayoutBatch{Animation: &driver.LayoutAnimation{Create: create, Update: update}, Created: map[driver.NodeID]bool{1: true, 2: true}}
	host.Appear(2, batch, 1)
	if len(motions) != 0 || ticks != 0 {
		t.Fatal("a child started a second appearance inside its appearing parent")
	}
	host.Appear(1, batch, 0)
	if m := motions[1]; m == nil || m.Appear != create || m.AppearStart != 10*time.Millisecond {
		t.Fatal("the inserted root did not start its appearance on the surface clock")
	}
	delete(batch.Created, 1)
	to := frame
	to.Origin.X = 104
	host.SetFrame(1, &driver.MountItem{Frame: to}, batch)
	m := motions[1]
	if m.Appear != create || m.Frame == nil || m.Frame.From() != frame || m.Frame.Target() != to || placements != 0 {
		t.Fatal("the frame motion lost its appearance or its frames")
	}
	if shown, _ := m.Frame.Step(55 * time.Millisecond); shown.Origin.X != 54 {
		t.Fatalf("halfway shows x %v; the motion did not start on the host's transition clock", shown.Origin.X)
	}
	host.SetFrame(1, &driver.MountItem{Frame: to}, batch)
	if ticks != 2 || m.Frame.Target() != to {
		t.Fatal("an unchanged target restarted its running transition")
	}
	// Retargeting continues from where the motion shows now, on the surface clock.
	now = 55 * time.Millisecond
	far := to
	far.Origin.X = 154
	host.SetFrame(1, &driver.MountItem{Frame: far}, batch)
	if m.Frame.From().Origin.X != 54 || m.Frame.Target() != far {
		t.Fatalf("a retargeted motion starts at %v toward %v, want from 54", m.Frame.From(), m.Frame.Target())
	}
	host.SetFrame(1, &driver.MountItem{Frame: frame}, nil)
	if m.Frame != nil || m.Appear != create || placements != 1 {
		t.Fatal("immediate placement did not cancel only the frame motion")
	}
}
