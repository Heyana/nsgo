package scrollbar

import (
	"reflect"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestTouchSessionArbitratesNestedAxesAndCancelsOnlyOnce(t *testing.T) {
	vertical, horizontal := &driver.ScrollConfig{}, &driver.ScrollConfig{Horizontal: true}
	states := map[int]TouchView{
		1: {Config: vertical, Offset: driver.Point{Y: 40}, Frame: driver.Size{Width: 100, Height: 100}, Content: driver.Size{Width: 300, Height: 300}},
		2: {Config: horizontal, Frame: driver.Size{Width: 100, Height: 100}, Content: driver.Size{Width: 300, Height: 300}},
	}
	begins, shows, moves := 0, 0, 0
	host := TouchHost[int]{
		State: func(v int) TouchView { return states[v] },
		Begin: func(v int, braked bool) {
			if v != 1 || braked {
				t.Error("the wrong view claimed an unbraked vertical drag")
			}
			begins++
		},
		Show: func(int) { shows++ },
		Move: func(v int, offset driver.Point) {
			state := states[v]
			state.Offset = offset
			states[v] = state
			moves++
		},
	}
	now := time.Unix(0, 0)
	session := NewTouchSession([]int{1, 2}, driver.Point{}, now, host)
	if handled, keep := session.Move(driver.Point{Y: -5}, now.Add(time.Millisecond), host); handled || !keep {
		t.Fatal("a movement below the slop took the finger")
	}
	if handled, keep := session.Move(driver.Point{Y: -20}, now.Add(20*time.Millisecond), host); !handled || !keep || session.Taken != 1 {
		t.Fatal("the outer vertical view did not win over the inner horizontal view")
	}
	session.Move(driver.Point{Y: -30}, now.Add(30*time.Millisecond), host)
	if begins != 1 || moves != 2 || shows != 2 || states[1].Offset.Y != 50 {
		t.Fatalf("begins=%d moves=%d shows=%d offset=%+v", begins, moves, shows, states[1].Offset)
	}
	state := states[1]
	state.Deleted = true
	states[1] = state
	if handled, keep := session.Move(driver.Point{Y: -40}, now.Add(40*time.Millisecond), host); !handled || keep || moves != 2 {
		t.Fatal("a deleted winner was moved or gave its finger back to a press")
	}
}

func TestTouchSessionBrakingPrecedesPanArbitration(t *testing.T) {
	for _, braked := range []bool{false, true} {
		interrupts, begins := 0, 0
		config := &driver.ScrollConfig{}
		host := TouchHost[int]{
			State:     func(int) TouchView { return TouchView{Config: config, Moving: braked} },
			Interrupt: func(int) { interrupts++ },
			Begin: func(_ int, got bool) {
				if got != braked {
					t.Errorf("braked=%v, want %v", got, braked)
				}
				begins++
			},
			Move:    func(int, driver.Point) {},
			Blocked: true,
		}
		now := time.Unix(0, 0)
		session := NewTouchSession([]int{1}, driver.Point{}, now, host)
		handled, keep := session.Move(driver.Point{}, now, host)
		if handled != braked || keep != braked || begins != 0 {
			t.Fatalf("braked=%v: handled=%v keep=%v begins=%d", braked, handled, keep, begins)
		}
		if braked {
			if interrupts != 1 {
				t.Fatal("braking did not interrupt the old motion")
			}
			if handled, keep := session.Move(driver.Point{Y: -20}, now.Add(time.Millisecond), host); !handled || !keep || begins != 1 {
				t.Fatal("a braked finger was stolen by the pan")
			}
		}
	}
}

func TestInterruptReportsMomentumAroundNativeCleanup(t *testing.T) {
	var got []string
	host := InterruptHost{
		Momentum: true, Gliding: true,
		ClearAnimation: func() { got = append(got, "animation") },
		ClearGlide:     func() { got = append(got, "glide") },
		Event: func(kind driver.EventKind) {
			if kind != driver.EventMomentumEnd {
				t.Error("unexpected event", kind)
			}
			got = append(got, "end")
		},
		AfterGlide: func() { got = append(got, "spinner") },
	}
	host.Interrupt()
	if want := []string{"end", "animation", "glide", "end", "spinner"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	got = nil
	host.Momentum, host.Gliding = false, false
	host.Interrupt()
	if !reflect.DeepEqual(got, []string{"animation"}) {
		t.Fatal("an idle view reported momentum or stopped a glide")
	}
}
