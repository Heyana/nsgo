//go:build android && nsgotest

package android

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func TestTabBarBadgesNative(t *testing.T) {
	var scope *core.Scope
	var change func(core.TabBadge)
	surface, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		scope = cx
		badge := core.NewSignal(cx, core.TabBadge{Text: "5"})
		change = badge.Set
		return core.TabBar{ItemsFunc: func() []core.TabBarItem {
			return []core.TabBarItem{{Title: "Inbox", Badge: badge.Get()}}
		}}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := surface.(*testSurface)
	s.Sync()
	var item int
	for _, step := range []struct {
		badge       core.TabBadge
		description string
	}{{core.TabBadge{Text: "5"}, "Inbox, 5"}, {core.TabBadge{Dot: true}, "Inbox, New"}, {core.TabBadge{}, "Inbox"}} {
		scope.Post(func() { change(step.badge) })
		s.Sync()
		deadline := time.Now().Add(4 * time.Second)
		for {
			data, err := s.ask(map[string]any{"op": "tab-badge", "index": 0})
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Item              int
				Shown             bool
				Text, Description string
			}
			if err := json.Unmarshal([]byte(data), &got); err != nil {
				t.Fatal(err)
			}
			if got.Item != 0 && got.Shown == (step.badge != (core.TabBadge{})) && got.Text == step.badge.Text && got.Description == step.description {
				if item != 0 && item != got.Item {
					t.Fatalf("badge update recreated the tab view: %x to %x", item, got.Item)
				}
				item = got.Item
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("badge %v: native tab %+v, want %q", step.badge, got, step.description)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
