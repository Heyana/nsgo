package watchos

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/finger"
)

// responderTouches tracks watchOS's single surface touch stream for EventTouch and the touch responder, snapshotting presentation hit targets and local coordinates on every phase before dispatching.
type responderTouches struct {
	fingers  finger.Fingers
	last     driver.Point
	lastTime float64
}

type (
	touchHitFunc    func(driver.Point) driver.NodeID
	touchParentFunc func(driver.NodeID) (driver.NodeID, driver.EventMask)
	touchLocalFunc  func(driver.NodeID, driver.Point) (driver.Point, bool)
)

func (r *responderTouches) down(listening bool, at driver.Point, t float64,
	hit touchHitFunc, parent touchParentFunc, local touchLocalFunc) (driver.Event, bool) {
	r.last, r.lastTime = at, t
	ev, ok := r.fingers.Down(listening, at)
	return r.finish(ev, ok, t, hit, parent, local)
}

func (r *responderTouches) move(at driver.Point, t float64,
	hit touchHitFunc, parent touchParentFunc, local touchLocalFunc) (driver.Event, bool) {
	r.last, r.lastTime = at, t
	ev, ok := r.fingers.Move(at)
	return r.finish(ev, ok, t, hit, parent, local)
}

func (r *responderTouches) up(at driver.Point, t float64, cancelled bool,
	hit touchHitFunc, parent touchParentFunc, local touchLocalFunc) (driver.Event, bool) {
	r.last, r.lastTime = at, t
	var (
		ev driver.Event
		ok bool
	)
	if cancelled {
		ev, ok = r.fingers.Cancel(at)
	} else {
		ev, ok = r.fingers.Up(at)
	}
	return r.finish(ev, ok, t, hit, parent, local)
}

func (r *responderTouches) cancel(hit touchHitFunc, parent touchParentFunc, local touchLocalFunc) (driver.Event, bool) {
	ev, ok := r.fingers.Cancel(r.last)
	return r.finish(ev, ok, r.lastTime, hit, parent, local)
}

func (r *responderTouches) finish(ev driver.Event, ok bool, t float64,
	hit touchHitFunc, parent touchParentFunc, local touchLocalFunc) (driver.Event, bool) {
	if !ok {
		return driver.Event{}, false
	}
	if t > 0 {
		ev.Time = time.Duration(t * float64(time.Second))
	}
	r.fingers.Capture(&ev, hit, parent, local)
	return ev, true
}

// affinePointerMatrix computes the exact affine transform watchOS sends to SwiftUI.
func affinePointerMatrix(st driver.ViewStyle, f driver.Bounds, k driver.Keyframe, scale float64) driver.Matrix3D {
	m := k.Matrix(f.Size).Mul(st.Matrix(f.Size))
	if scale != 1 {
		m = driver.Transform{Scale: float32(scale)}.Matrix(driver.Point{X: f.Size.Width / 2, Y: f.Size.Height / 2}).Mul(m)
	}
	if _, _, _, _, _, _, ok := m.Affine(); !ok {
		return driver.Identity3D
	}
	return m
}
