package core

import "github.com/nsgo-dev/nsgo/internal/driver"

// AppClipOverlayPosition implements mobile.AppClipOverlayPosition.
type AppClipOverlayPosition uint8

// Positions for mobile.ShowAppClipOverlay.
const (
	AppClipOverlayBottom AppClipOverlayPosition = iota
	AppClipOverlayBottomRaised
)

// ShowAppClipOverlay implements mobile.ShowAppClipOverlay. Its request belongs to the current owner and is dismissed as that owner ends.
func (cx *Scope) ShowAppClipOverlay(position AppClipOverlayPosition) (dismiss func()) {
	owner := cx.current()
	t := owner.tree()
	noop := func() {}
	if t == nil || t.closed || owner.disposed || position > AppClipOverlayBottomRaised {
		return noop
	}
	if !Untracked(cx, cx.Env().AppClip) || cx.Env().hostUntracked() != OSIOS || Untracked(cx, cx.Env().Simulated) {
		return noop
	}
	if !t.confinedMay(owner, refusedPresent, askedToPresent) {
		return noop
	}
	value := "bottom"
	if position == AppClipOverlayBottomRaised {
		value = "bottom-raised"
	}
	t.alertID++
	id := t.alertID
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemAppClipOverlay, Count: id, On: true, Value: value})
	t.changed = true
	ended := false
	stop := func() {
		if ended {
			return
		}
		ended = true
		if !t.closed {
			t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemAppClipOverlay, Count: id})
			t.changed = true
		}
	}
	owner.addCleanup(stop)
	return func() { owner.Post(stop) }
}
