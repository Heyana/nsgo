//go:build darwin

package apple

import (
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// LayoutMotion is a view's part of a layout animation on the frame clock: its frame moving, its velocity kept as a spring retargets, or the view appearing.
type LayoutMotion struct {
	Frame       *driver.LayoutMotion
	Appear      *driver.LayoutTransition
	AppearStart time.Duration
}

// LayoutGhost is a removed view kept alive until its exit ends.
type LayoutGhost struct {
	native  objc.ID
	opacity float32
	tr      *driver.LayoutTransition
	start   time.Duration
}

var (
	layoutLayer      = Sel("layer")
	layoutSuperview  = Sel("superview")
	layoutOpacity    = Sel("opacity")
	layoutSetOpacity = Sel("setOpacity:")
	layoutRemove     = Sel("removeFromSuperview")
	layoutRelease    = Sel("release")
)

// NewLayoutGhost takes the caller's retain on native and records its opacity.
func NewLayoutGhost(native objc.ID, transition *driver.LayoutTransition, start time.Duration) *LayoutGhost {
	return &LayoutGhost{
		native:  native,
		opacity: objc.Send[float32](native.Send(layoutLayer), layoutOpacity),
		tr:      transition,
		start:   start,
	}
}

// Release takes g's native view away and lets go of it, at the end of its exit or as its surface closes.
func (g *LayoutGhost) Release() {
	ForgetTransform(g.native.Send(layoutLayer))
	g.native.Send(layoutRemove)
	g.native.Send(layoutRelease)
}

// InLayoutGhost reports whether native is inside a removed view still drawing its exit.
func InLayoutGhost(ghosts []*LayoutGhost, native objc.ID) bool {
	if len(ghosts) == 0 {
		return false
	}
	for parent := native.Send(layoutSuperview); parent != 0; parent = parent.Send(layoutSuperview) {
		for _, g := range ghosts {
			if g.native == parent {
				return true
			}
		}
	}
	return false
}

// StepLayoutMotions advances the mounted views and retires completed exits. The caller supplies view identity, placement and restyling for its toolkit.
func StepLayoutMotions[V comparable](now time.Duration, motions map[V]*LayoutMotion, ghosts []*LayoutGhost, current func(V) bool, place func(V, driver.Bounds), restyle func(V)) []*LayoutGhost {
	for view, m := range motions {
		if !current(view) {
			delete(motions, view)
			continue
		}
		done := true
		if m.Frame != nil {
			shown, end := m.Frame.Step(now)
			place(view, shown)
			if end {
				m.Frame = nil
			} else {
				done = false
			}
		}
		if m.Appear != nil {
			if _, end := m.Appear.At(now - m.AppearStart); end {
				m.Appear = nil
			} else {
				done = false
			}
			restyle(view)
		}
		if done {
			delete(motions, view)
		}
	}
	live := ghosts[:0]
	for _, g := range ghosts {
		p, end := g.tr.At(now - g.start)
		layer := g.native.Send(layoutLayer)
		if g.tr.Property == driver.LayoutScale {
			SetTransform(layer, driver.Transform{Scale: float32(max(1-p, 0.0001))})
		} else {
			layer.Send(layoutSetOpacity, g.opacity*float32(1-p))
		}
		if end {
			g.Release()
			continue
		}
		live = append(live, g)
	}
	clear(ghosts[len(live):])
	return live
}
