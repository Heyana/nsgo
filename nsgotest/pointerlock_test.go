package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestPointerLock(t *testing.T) {
	if onTarget() {
		t.Skip("this test controls the headless environment")
	}
	var first, second core.Ref
	var unlockFirst, unlockSecond, hide func()
	var firstX, secondX, secondY float64
	app := Start(t, func(cx *core.Scope) core.Node {
		hidden := core.NewSignal(cx, false)
		hide = func() { hidden.Set(true) }
		return core.View{Children: []core.Node{
			core.View{
				Ref: &first, TestID: "first", Style: core.Style{Height: core.Pt(50)},
				OnPress:        func(core.PressEvent) { unlockFirst = first.LockPointer() },
				OnPointerDelta: func(dx, _ float64) { firstX += dx },
			},
			core.View{
				Ref: &second, TestID: "second", Style: core.Style{Height: core.Pt(50)},
				StyleFunc: func() core.Style {
					if hidden.Get() {
						return core.Style{Visibility: core.VisibilityHidden}
					}
					return core.Style{}
				},
				OnPress:        func(core.PressEvent) { unlockSecond = second.LockPointer() },
				OnPointerDelta: func(dx, dy float64) { secondX, secondY = secondX+dx, secondY+dy },
			},
		}}
	}, Environment(core.Environment{CanLockPointer: true}))
	app.PointerDelta(5, 5)
	app.Press("id:first")
	stale := app.pointerLock
	app.PointerDelta(1.25, -3)
	app.Press("id:second")
	app.Post(unlockFirst)
	app.Settle()
	app.dispatch(stale.Motion(10, 10))
	app.PointerDelta(7, -8)
	if firstX != 1.25 || secondX != 7 || secondY != -8 {
		t.Fatalf("motion reached the wrong owner: %g, (%g, %g)", firstX, secondX, secondY)
	}
	app.Key("Escape")
	app.PointerDelta(2, 2)
	if secondX != 7 || app.pointerLock.ID != 0 {
		t.Fatal("Escape left the pointer locked")
	}
	app.Press("id:second")
	app.Post(hide)
	app.Settle()
	app.PointerDelta(2, 2)
	if secondX != 7 || app.pointerLock.ID != 0 {
		t.Fatal("hiding the view did not release the lock")
	}
	app.Post(unlockSecond)
	app.Settle()
}

func TestPointerLockUnavailable(t *testing.T) {
	if onTarget() {
		t.Skip("this test controls the headless environment")
	}
	var ref core.Ref
	app := Start(t, func(cx *core.Scope) core.Node {
		if cx.Env().CanLockPointer() {
			t.Fatal("pointer lock unexpectedly available")
		}
		return core.View{Ref: &ref, TestID: "view", Style: core.Style{Height: core.Pt(50)},
			OnPress:        func(core.PressEvent) { ref.LockPointer()() },
			OnPointerDelta: func(dx, dy float64) { t.Fatal("unexpected pointer motion") }}
	})
	app.Press("id:view")
	app.PointerDelta(5, 5)
	if app.pointerLock != (driver.PointerLock{}) {
		t.Fatal("unsupported pointer lock reached the platform")
	}
}
