package nsgotest

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// PointerDelta sends relative motion to the view holding the pointer
// lock. A surface with no lock ignores it.
func (a *App) PointerDelta(dx, dy float64) {
	a.step("PointerDelta(%g, %g)", dx, dy)
	defer a.done()
	if a.target != nil {
		if surface, ok := a.target.surface.(testhost.PointerLockSurface); ok {
			surface.PointerDelta(dx, dy)
		}
		return
	}
	if a.pointerLock.ID != 0 {
		a.dispatch(a.pointerLock.Motion(dx, dy))
	}
}

func (a *App) setPointerLock(it driver.MountItem) {
	if it.On {
		a.pointerLock = driver.PointerLock{ID: it.ID, Count: it.Count}
	} else if a.pointerLock.Matches(it.ID, it.Count) {
		a.pointerLock = driver.PointerLock{}
	}
}

func (a *App) unlockPointer() bool {
	if a.pointerLock.ID == 0 {
		return false
	}
	lost := a.pointerLock.Unlocked()
	a.pointerLock = driver.PointerLock{}
	a.dispatch(lost)
	return true
}
