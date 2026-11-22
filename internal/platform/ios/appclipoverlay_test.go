//go:build ios && !tvos && nsgotest

package ios

import (
	"slices"
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestAppClipOverlayStoreKitConfiguration(t *testing.T) {
	nsgotest.Start(t, func(*core.Scope) core.Node { return core.View{} })
	onMainSync(func() {
		configClass, overlayClass := appClipOverlayClasses()
		if configClass == 0 || overlayClass == 0 {
			t.Error("StoreKit did not expose the App Clip overlay classes")
			return
		}
		for position := range 2 {
			config := configClass.Send(selAlloc).Send(sel("initWithPosition:"), position)
			if config == 0 {
				t.Error("StoreKit refused an App Clip configuration")
				continue
			}
			if got := objc.Send[int](config, sel("position")); got != position {
				t.Errorf("StoreKit position=%d, want %d", got, position)
			}
			overlay := overlayClass.Send(selAlloc).Send(sel("initWithConfiguration:"), config)
			if overlay == 0 {
				t.Error("StoreKit refused its App Clip configuration")
			} else {
				if overlay.Send(sel("configuration")) == 0 {
					t.Error("overlay lost its configuration")
				}
				overlay.Send(selRelease)
			}
			config.Send(selRelease)
		}
	})
}

// A recording Objective-C overlay exercises the real purego message ABI and scene/engine ownership without claiming an App Store presentation.
func TestAppClipOverlaySceneOwnership(t *testing.T) {
	nsgotest.Start(t, func(*core.Scope) core.Node { return core.View{} })
	onMainSync(func() {
		configClass, _ := appClipOverlayClasses()
		if configClass == 0 || runner.activeWindowScene() == 0 {
			t.Error("a StoreKit configuration and foreground scene are required")
			return
		}
		var positions []int
		presented, dismissed := 0, 0
		fake := register("NsgoAppClipOverlay1472Test", apple.Class("NSObject"), nil, []objc.MethodDef{
			{Cmd: sel("initWithConfiguration:"), Fn: func(self objc.ID, _ objc.SEL, config objc.ID) objc.ID {
				positions = append(positions, objc.Send[int](config, sel("position")))
				return self
			}},
			{Cmd: sel("presentInScene:"), Fn: func(_ objc.ID, _ objc.SEL, scene objc.ID) {
				if scene != 0 {
					presented++
				}
			}},
		})
		meta := objc.ID(fake).Class()
		if !meta.AddMethod(sel("dismissOverlayInScene:"), objc.NewIMP(func(_ objc.ID, _ objc.SEL, scene objc.ID) {
			if scene != 0 {
				dismissed++
			}
		}), "v@:@") {
			t.Error("could not register the recording overlay's dismissal")
			return
		}
		factory, wasClip := appClipOverlayClasses, isAppClip
		appClipOverlayClasses = func() (objc.ID, objc.ID) { return configClass, objc.ID(fake) }
		isAppClip = 1 // Bridge test only; no real App Clip identity is claimed.
		defer func() { appClipOverlayClasses, isAppClip = factory, wasClip }()
		other := &surface{native: runner.native} // An independent embedded engine in the same scene.
		runner.appClipOverlay(driver.MountItem{Kind: driver.ItemAppClipOverlay, Count: 1, On: true, Value: "bottom"})
		other.appClipOverlay(driver.MountItem{Kind: driver.ItemAppClipOverlay, Count: 1, On: true, Value: "bottom-raised"})
		if runner.appClipOffer.overlay != 0 || other.appClipOffer.overlay == 0 || presented != 2 || dismissed != 1 {
			t.Errorf("replacement: first=%v second=%v presents=%d dismissals=%d", runner.appClipOffer, other.appClipOffer, presented, dismissed)
		}
		runner.appClipOverlay(driver.MountItem{Count: 1})
		if other.appClipOffer.overlay == 0 || dismissed != 1 {
			t.Error("old engine dismissed the new engine's offer")
		}
		other.appClipOverlay(driver.MountItem{Count: 2})
		if other.appClipOffer.overlay == 0 || dismissed != 1 {
			t.Error("stale scope dismissed the current offer")
		}
		other.appClipOverlay(driver.MountItem{Count: 1})
		other.appClipOverlay(driver.MountItem{Count: 1})
		if other.appClipOffer.overlay != 0 || dismissed != 2 {
			t.Error("owned dismissal was not idempotent")
		}
		if !slices.Equal(positions, []int{0, 1}) {
			t.Errorf("native positions=%v", positions)
		}
		if appClipOfferOwners[runner.activeWindowScene()] != nil {
			t.Error("scene retained its old owner")
		}
	})
}
