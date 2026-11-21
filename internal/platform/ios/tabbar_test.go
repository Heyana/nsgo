//go:build ios && nsgotest

package ios_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/ios"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestTabBarBadgesNative(t *testing.T) {
	if nsgotest.Target() == "tvos_simulator" {
		t.Skip("requires the iOS tab bar")
	}
	var change func(mobile.TabBadge)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		badge := core.NewSignal(cx, mobile.TabBadge{Text: "5"})
		change = badge.Set
		return mobile.TabBar{TestID: "bar", ItemsFunc: func() []mobile.TabBarItem {
			return []mobile.TabBarItem{{Title: "Inbox", Icon: "envelope", Badge: badge.Get()}}
		}}
	})
	app.WaitFor("id:bar", 5*time.Second)
	var item uintptr
	for _, step := range []struct {
		badge       mobile.TabBadge
		text, value string
	}{{mobile.TabBadge{Text: "5"}, "5", "5"}, {mobile.TabBadge{Dot: true}, " ", "New"}, {}} {
		app.Post(func() { change(step.badge) })
		app.WaitUntil(5*time.Second, func() bool {
			id, badge, value := ios.TabBadgeForTest(0)
			if id == 0 || badge != step.text || value != step.value || app.Find("id:bar").TabBadge(0) != step.badge {
				return false
			}
			if item != 0 && item != id {
				t.Fatalf("badge update recreated the UITabBarItem: %x to %x", item, id)
			}
			item = id
			return true
		})
	}
}
