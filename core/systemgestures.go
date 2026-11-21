package core

import (
	"math"
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// SystemGestureEdge implements mobile.SystemGestureEdge.
type SystemGestureEdge = driver.SystemGestureEdge

// The physical edges for mobile.DeferSystemGestures.
const (
	GestureEdgeTop    = driver.GestureEdgeTop
	GestureEdgeLeft   = driver.GestureEdgeLeft
	GestureEdgeBottom = driver.GestureEdgeBottom
	GestureEdgeRight  = driver.GestureEdgeRight
)

// SystemGestureOptions implements mobile.SystemGestureOptions.
type SystemGestureOptions = driver.SystemGestureOptions

// DeferSystemGestures implements mobile.DeferSystemGestures on the scope's surface.
func (cx *Scope) DeferSystemGestures(opts SystemGestureOptions) (release func()) {
	owner := cx.current()
	t := owner.tree()
	if t == nil || t.closed || owner.disposed {
		return func() {}
	}
	hold := &SystemGestureOptions{
		IOSEdges: opts.IOSEdges & (GestureEdgeTop | GestureEdgeLeft | GestureEdgeBottom | GestureEdgeRight),
	}
	for _, r := range opts.AndroidRects {
		if validGestureRect(r) {
			hold.AndroidRects = append(hold.AndroidRects, r)
		}
	}
	if hold.IOSEdges == 0 && len(hold.AndroidRects) == 0 {
		return func() {}
	}
	t.systemGestures = append(t.systemGestures, hold)
	t.updateSystemGestures()
	done := false // only read and written on the graph's goroutine
	releaseNow := func() {
		if done {
			return
		}
		done = true
		t.systemGestures = slices.DeleteFunc(t.systemGestures, func(other *SystemGestureOptions) bool { return hold == other })
		if !t.closed {
			t.updateSystemGestures()
		}
	}
	owner.addCleanup(releaseNow)
	return func() { owner.Post(releaseNow) }
}

func validGestureRect(r Bounds) bool {
	if !(r.Size.Width > 0 && r.Size.Height > 0) {
		return false
	}
	for _, p := range [...]Points{r.Origin.X, r.Origin.Y, r.Origin.X + r.Size.Width, r.Origin.Y + r.Size.Height} {
		if math.IsNaN(float64(p)) || math.IsInf(float64(p), 0) {
			return false
		}
	}
	return true
}

func (t *Tree) updateSystemGestures() {
	var combined SystemGestureOptions
	for _, hold := range t.systemGestures {
		combined.IOSEdges |= hold.IOSEdges
		combined.AndroidRects = append(combined.AndroidRects, hold.AndroidRects...)
	}
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemSystemGestures, SystemGestures: &combined})
	t.changed = true
}
