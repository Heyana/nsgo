//go:build ios && !tvos && nsgotest

package ios

import (
	"slices"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
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

func TestTabBarVerticalEdgeSwitchesContainerAndSelection(t *testing.T) {
	onMainSync(func() {
		var sizes []driver.Size
		var changed []int
		comp := newTabBar(driver.NewNativeContext(
			func(name string, data any) {
				if name == "change" {
					if i, ok := data.(float64); ok {
						changed = append(changed, int(i))
					}
				}
			},
			func(s driver.Size) { sizes = append(sizes, s) },
		))
		bar := comp.(*tabBar)
		defer bar.Close()
		bar.SetProp("items", []any{
			map[string]any{"title": "Home", "icon": "house"},
			map[string]any{"title": "Search", "icon": "magnifyingglass"},
		})
		bar.SetProp("edge", "trailing")
		if bar.rail == 0 || len(bar.buttons) != 2 {
			t.Fatalf("vertical rail not built: rail=%v buttons=%d", bar.rail, len(bar.buttons))
		}
		if last := sizes[len(sizes)-1]; last.Width < verticalTabBarWidth || last.Height != 0 {
			t.Fatalf("vertical measured size = %+v, want width >= %v and height 0", last, verticalTabBarWidth)
		}
		if !bar.selectIndex(1) || len(changed) != 1 || changed[0] != 1 {
			t.Fatalf("selectIndex(1) emitted %v, want [1]", changed)
		}
		bar.SetProp("edge", "bottom")
		if last := sizes[len(sizes)-1]; last.Height <= 0 || last.Width != 0 {
			t.Fatalf("bottom measured size = %+v, want height > 0 and width 0", last)
		}
	})
}
