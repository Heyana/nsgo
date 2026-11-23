package core

import (
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type orientationRequest struct{ mask Orientation }

type orientationRequests struct {
	live []*orientationRequest
	sent Orientation
}

func (r *orientationRequests) current() Orientation {
	if len(r.live) == 0 {
		return 0
	}
	return r.live[len(r.live)-1].mask
}

// RequestOrientation asks iOS, Android, HarmonyOS or the browser to show the app in one of the given interface orientations while the current scope lives. The newest live request wins. When none remains, the system applies its usual orientation policy. A zero mask makes no request. On iOS a presented modal's SupportedOrientations takes precedence. On the web, enter full screen from a press to let the browser lock orientation. Refusals go to the browser log.
func (cx *Scope) RequestOrientation(mask Orientation) {
	mask &= OrientationPortrait | OrientationPortraitUpsideDown | OrientationLandscape
	owner := cx.current()
	t := owner.rt.tree
	if t == nil || mask == 0 || owner.disposed {
		return
	}
	r := &t.orientations
	previous := r.current()
	request := &orientationRequest{mask}
	r.live = append(r.live, request)
	t.changed = t.changed || mask != previous
	owner.addCleanup(func() {
		previous := r.current()
		r.live = slices.DeleteFunc(r.live, func(live *orientationRequest) bool { return live == request })
		t.changed = t.changed || r.current() != previous
	})
}

func (t *Tree) emitOrientation() {
	r := &t.orientations
	if mask := r.current(); mask != r.sent {
		r.sent = mask
		t.items = append(t.items, driver.MountItem{Kind: driver.ItemOrientation, Count: uint32(mask)})
	}
}
