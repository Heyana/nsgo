package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestTouchAssignsDistinctIDsAndReportsActiveFingers(t *testing.T) {
	type touchLog struct {
		phase   core.TouchPhase
		touches []core.Touch
	}
	var events []touchLog
	app := Start(t, func(*core.Scope) core.Node {
		return core.View{
			TestID: "pad",
			Style:  core.Style{Width: core.Pt(200), Height: core.Pt(120)},
			OnTouch: func(e core.TouchEvent) {
				touches := make([]core.Touch, len(e.Touches))
				copy(touches, e.Touches)
				events = append(events, touchLog{phase: e.Phase, touches: touches})
			},
		}
	})

	check := func(wantPhase core.TouchPhase, want []core.Touch) {
		t.Helper()
		if len(events) == 0 {
			t.Fatalf("no touch event recorded; want phase %v", wantPhase)
		}
		got := events[len(events)-1]
		if got.phase != wantPhase {
			t.Fatalf("phase = %v, want %v", got.phase, wantPhase)
		}
		if len(got.touches) != len(want) {
			t.Fatalf("len(Touches) = %d (%+v), want %d (%+v)", len(got.touches), got.touches, len(want), want)
		}
		for i := range want {
			if got.touches[i].ID != want[i].ID ||
				got.touches[i].Position != want[i].Position ||
				got.touches[i].Changed != want[i].Changed {
				t.Fatalf("Touches[%d] = %+v, want %+v (all: %+v)", i, got.touches[i], want[i], got.touches)
			}
		}
	}

	p1 := core.Point{X: 20, Y: 30}
	p2 := core.Point{X: 120, Y: 40}
	f1 := app.Touch("id:pad", p1)
	if f1.ID() != 1 {
		t.Fatalf("f1.ID() = %d, want 1", f1.ID())
	}
	check(core.TouchStart, []core.Touch{
		{ID: 1, Position: p1, Changed: true},
	})

	f2 := app.Touch("id:pad", p2)
	if f2.ID() != 2 {
		t.Fatalf("f2.ID() = %d, want 2", f2.ID())
	}
	check(core.TouchStart, []core.Touch{
		{ID: 1, Position: p1, Changed: false},
		{ID: 2, Position: p2, Changed: true},
	})

	p1b := core.Point{X: 25, Y: 35}
	f1.Move(p1b)
	check(core.TouchMove, []core.Touch{
		{ID: 1, Position: p1b, Changed: true},
		{ID: 2, Position: p2, Changed: false},
	})

	p2b := core.Point{X: 130, Y: 50}
	f2.Move(p2b)
	check(core.TouchMove, []core.Touch{
		{ID: 1, Position: p1b, Changed: false},
		{ID: 2, Position: p2b, Changed: true},
	})

	// Lifting f1 reports both fingers with only f1 changed, and leaves f2 down.
	f1.Lift()
	check(core.TouchEnd, []core.Touch{
		{ID: 1, Position: p1b, Changed: true},
		{ID: 2, Position: p2b, Changed: false},
	})

	p2c := core.Point{X: 140, Y: 60}
	f2.Move(p2c)
	check(core.TouchMove, []core.Touch{
		{ID: 2, Position: p2c, Changed: true},
	})

	f2.Lift()
	check(core.TouchEnd, []core.Touch{
		{ID: 2, Position: p2c, Changed: true},
	})

	// Subsequent touches continue incrementing rather than reusing lifted IDs.
	p3 := core.Point{X: 10, Y: 10}
	f3 := app.Touch("id:pad", p3)
	if f3.ID() != 3 {
		t.Fatalf("f3.ID() = %d, want 3", f3.ID())
	}
	check(core.TouchStart, []core.Touch{
		{ID: 3, Position: p3, Changed: true},
	})
	f3.Cancel()
	check(core.TouchCancel, []core.Touch{
		{ID: 3, Position: p3, Changed: true},
	})
}

func TestTouchMultiViewAndCancelPreservesOtherFinger(t *testing.T) {
	var dpadEvents, jumpEvents []core.TouchEvent
	var jumpPresses []core.PressEvent
	app := Start(t, func(*core.Scope) core.Node {
		return core.View{
			Style: core.Style{Direction: core.Row, Width: core.Pt(300), Height: core.Pt(100)},
			Children: []core.Node{
				core.View{
					TestID: "dpad",
					Style:  core.Style{Width: core.Pt(150), Height: core.Pt(100)},
					OnTouch: func(e core.TouchEvent) {
						dpadEvents = append(dpadEvents, e)
					},
				},
				core.View{
					TestID: "jump",
					Style:  core.Style{Width: core.Pt(150), Height: core.Pt(100)},
					OnTouch: func(e core.TouchEvent) {
						jumpEvents = append(jumpEvents, e)
					},
					OnPress: func(e core.PressEvent) {
						jumpPresses = append(jumpPresses, e)
					},
				},
			},
		}
	})

	// Hold left thumb on d-pad while right thumb taps jump.
	left := app.Touch("id:dpad", core.Point{X: 30, Y: 50})
	right := app.Touch("id:jump", core.Point{X: 40, Y: 50})
	if left.ID() == right.ID() {
		t.Fatalf("expected distinct IDs, both were %d", left.ID())
	}
	if len(jumpEvents) != 1 || len(jumpEvents[0].Touches) != 2 {
		t.Fatalf("jump start events = %+v, want 1 event with 2 touches", jumpEvents)
	}
	if jumpEvents[0].Touches[0].Changed || !jumpEvents[0].Touches[1].Changed {
		t.Fatalf("jump start Changed flags = [%v, %v], want [false, true]",
			jumpEvents[0].Touches[0].Changed, jumpEvents[0].Touches[1].Changed)
	}

	right.Lift()
	if len(jumpPresses) != 1 || jumpPresses[0].Keyboard {
		t.Fatalf("jump presses = %+v, want 1 pointer press", jumpPresses)
	}

	// Moving left thumb still delivers to d-pad with only left remaining.
	left.Move(core.Point{X: 60, Y: 50})
	last := dpadEvents[len(dpadEvents)-1]
	if last.Phase != core.TouchMove || len(last.Touches) != 1 || last.Touches[0].ID != left.ID() || !last.Touches[0].Changed {
		t.Fatalf("dpad move after jump lift = %+v, want single changed touch ID %d", last, left.ID())
	}

	// Cancelling a second finger also leaves the first finger active.
	second := app.Touch("id:jump", core.Point{X: 20, Y: 20})
	second.Cancel()
	left.Move(core.Point{X: 70, Y: 50})
	last = dpadEvents[len(dpadEvents)-1]
	if last.Phase != core.TouchMove || len(last.Touches) != 1 || last.Touches[0].ID != left.ID() {
		t.Fatalf("dpad move after second cancel = %+v, want single touch ID %d", last, left.ID())
	}
	left.Lift()
}

func TestTouchReportsPointerModalityAfterKey(t *testing.T) {
	var downEvents, upEvents, pressEvents []core.PressEvent
	app := Start(t, func(*core.Scope) core.Node {
		return core.View{
			TestID:      "btn",
			Style:       core.Style{Width: core.Pt(120), Height: core.Pt(44)},
			OnPressDown: func(e core.PressEvent) { downEvents = append(downEvents, e) },
			OnPressUp:   func(e core.PressEvent) { upEvents = append(upEvents, e) },
			OnPress:     func(e core.PressEvent) { pressEvents = append(pressEvents, e) },
		}
	}, Environment(driver.Environment{OS: driver.OSMacOS, Idiom: driver.IdiomDesktop}))

	// Keyboard activation delivers Keyboard == true.
	app.Focus("id:btn")
	app.Key("Return")
	if len(pressEvents) != 1 || !pressEvents[0].Keyboard {
		t.Fatalf("keyboard press = %+v, want Keyboard == true", pressEvents)
	}

	// A subsequent key followed by a Touch tap must deliver Keyboard == false.
	app.Key("a")
	f := app.Touch("id:btn", core.Point{X: 20, Y: 20})
	f.Lift()
	if len(downEvents) < 2 || downEvents[len(downEvents)-1].Keyboard {
		t.Fatalf("touch OnPressDown Keyboard = true, want false (%+v)", downEvents)
	}
	if len(upEvents) < 2 || upEvents[len(upEvents)-1].Keyboard {
		t.Fatalf("touch OnPressUp Keyboard = true, want false (%+v)", upEvents)
	}
	if len(pressEvents) != 2 || pressEvents[1].Keyboard {
		t.Fatalf("touch OnPress = %+v, want Keyboard == false", pressEvents)
	}

	// Drag after a key also switches back to pointer modality.
	app.Key("a")
	app.Drag("id:btn")
	if len(pressEvents) != 3 || pressEvents[2].Keyboard {
		t.Fatalf("drag OnPress = %+v, want Keyboard == false", pressEvents)
	}
}
