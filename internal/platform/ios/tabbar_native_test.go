//go:build ios && !tvos && nsgotest

package ios

import (
	"slices"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestTabBarRetitlesUIKitItemsInPlace(t *testing.T) {
	onMainSync(func() {
		bar := &tabBar{bar: objc.ID(apple.Class("UITabBar")).Send(selNew), selected: 1}
		defer bar.bar.Send(selRelease)
		items := func(title string) []any {
			return []any{
				map[string]any{"title": "Home", "icon": "house"},
				map[string]any{"title": title, "icon": "magnifyingglass"},
			}
		}
		bar.SetProp("items", items("Search"))
		before := slices.Clone(bar.items)
		bar.SetProp("label", "Rubriques")
		bar.SetProp("items", items("Rechercher"))
		if len(before) != 2 || !slices.Equal(before, bar.items) {
			t.Errorf("UIKit item identities changed with the title")
			return
		}
		if selected := bar.bar.Send(sel("selectedItem")); selected != before[1] {
			t.Errorf("UIKit selection changed after retitling")
		}
		if name := apple.GoString(bar.bar.Send(sel("accessibilityLabel"))); name != "Rubriques" {
			t.Errorf("UIKit tab bar accessibility label = %q", name)
		}
		if title := apple.GoString(before[1].Send(sel("title"))); title != "Rechercher" {
			t.Errorf("UIKit tab title = %q", title)
		}
	})
}
