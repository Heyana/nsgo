package core

import (
	"errors"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Capture asks for a PNG of what the target shows in its surface. A nil
// target captures the surface that owns cx. The callback runs on the engine
// goroutine. Its PNG can be used in ImageSource.Data or saved for Share.
// A confined view or an owner held to Access cannot capture their host.
func (cx *Scope) Capture(target *Ref, done func(png []byte, err error)) {
	t := cx.tree()
	if t == nil || done == nil {
		return
	}
	owner := cx.current()
	if accessOf(owner) != nil || confinement(owner) != nil {
		owner.Post(func() { done(nil, AccessDenied("capture", "")) })
		return
	}
	var id driver.NodeID
	if target != nil {
		if target.t != t || !target.placed() {
			owner.Post(func() { done(nil, errors.New("core: capture target is not mounted on this surface")) })
			return
		}
		id = target.n.id
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemCapture, ID: id}, func(ev driver.Event) {
		switch ev.Text {
		case driver.NoCapture:
			done(nil, Unavailable(ev.Text))
		case "":
			data, _ := ev.Data.([]byte)
			if len(data) == 0 {
				done(nil, errors.New("core: capture returned no pixels"))
				return
			}
			done(data, nil)
		default:
			done(nil, errors.New(ev.Text))
		}
	})
}
