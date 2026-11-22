//go:build ios

package ios

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

var appClipOverlayClasses = sync.OnceValues(func() (objc.ID, objc.ID) {
	if _, err := purego.Dlopen("/System/Library/Frameworks/StoreKit.framework/StoreKit", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		return 0, 0
	}
	return objc.ID(apple.Class("SKOverlayAppClipConfiguration")), objc.ID(apple.Class("SKOverlay"))
})

type appClipOffer struct {
	id             uint32
	overlay, scene objc.ID
}

// StoreKit has one overlay per scene, including scenes containing more than one embedded engine. Main-thread ownership keeps an older engine's cleanup from dismissing the replacement another engine presented.
var appClipOfferOwners = map[objc.ID]*surface{}

func (s *surface) appClipOverlay(it driver.MountItem) {
	if !it.On {
		if it.Count == s.appClipOffer.id {
			s.dismissAppClipOverlay()
		}
		return
	}
	if s.closed || onTV() || !appClip() {
		return
	}
	scene := s.activeWindowScene()
	if scene == 0 {
		return
	}
	configClass, overlayClass := appClipOverlayClasses()
	if configClass == 0 || overlayClass == 0 {
		return
	}
	position := uintptr(0) // SKOverlayPositionBottom
	if it.Value == "bottom-raised" {
		position = 1
	}
	config := configClass.Send(selAlloc).Send(sel("initWithPosition:"), position)
	if config == 0 {
		return
	}
	overlay := overlayClass.Send(selAlloc).Send(sel("initWithConfiguration:"), config)
	config.Send(selRelease)
	if overlay == 0 {
		return
	}
	if previous := appClipOfferOwners[scene]; previous != nil {
		previous.dismissAppClipOverlay()
	}
	s.dismissAppClipOverlay()
	scene.Send(sel("retain"))
	s.appClipOffer = appClipOffer{id: it.Count, overlay: overlay, scene: scene}
	appClipOfferOwners[scene] = s
	overlay.Send(sel("presentInScene:"), scene)
}

// Only the currently owned offer may dismiss StoreKit's scene-wide UI. Scope cleanup sends its request ID, so a stale cleanup cannot hide a newer recommendation from another scope.
func (s *surface) dismissAppClipOverlay() {
	old := s.appClipOffer
	s.appClipOffer = appClipOffer{}
	if old.overlay == 0 {
		return
	}
	_, overlayClass := appClipOverlayClasses()
	if appClipOfferOwners[old.scene] == s {
		delete(appClipOfferOwners, old.scene)
		if overlayClass != 0 && old.scene != 0 {
			overlayClass.Send(sel("dismissOverlayInScene:"), old.scene)
		}
	}
	old.overlay.Send(selRelease)
	old.scene.Send(selRelease)
}
