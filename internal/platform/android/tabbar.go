//go:build android

package android

import (
	"encoding/json"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/mobile/locales"
)

// tabBar is core.TabBar's component, which the backend registers itself
// as NSH's TabBarComponent.java: the runtime's TabBar, a Material 3
// navigation bar, reporting the user's choice as "change".
type tabBar struct{ cx *driver.NativeContext }

// tabBarHeight is Material 3's navigation bar's content height. The runtime reports a taller natural size only where the bar overlaps system navigation.
const tabBarHeight driver.Points = 80

func newTabBar(cx *driver.NativeContext) driver.NativeComponent {
	cx.SetSize(driver.Size{Height: tabBarHeight})
	return &tabBar{cx: cx}
}

func (*tabBar) View() any { return "dev.oneo.nsgo.TabBar" }

func (*tabBar) SetProp(string, any) {}

// ViewProps implements driver.AndroidPropMapper: the items cross as one
// JSON string.
func (*tabBar) ViewProps(name string, value any) []driver.AndroidViewProp {
	color := func(v any) []driver.AndroidViewProp {
		c, _ := v.(float64)
		rgba := uint32(c)
		return []driver.AndroidViewProp{{Name: name, Value: int64(rgba>>8 | rgba<<24)}}
	}
	switch name {
	case "items":
		data, err := tabBarItems(value)
		if err != nil {
			return nil
		}
		return []driver.AndroidViewProp{{Name: "items", Value: string(data)}}
	case "label":
		return []driver.AndroidViewProp{{Name: name, Value: value}}
	case "badges":
		badges, _ := value.(driver.TabBadges)
		messages := locales.In(badges.Language)
		type badgeEntry struct {
			driver.TabBadge
			Announcement string `json:"announcement,omitempty"`
		}
		entries := make([]badgeEntry, len(badges.Items))
		for i, badge := range badges.Items {
			announcement := badge.Text
			if announcement == "" && badge.Dot {
				announcement = messages.TabBadgeNew()
			}
			entries[i] = badgeEntry{TabBadge: badge, Announcement: announcement}
		}
		data, _ := json.Marshal(entries)
		return []driver.AndroidViewProp{{Name: name, Value: string(data)}}
	case "selectedIndex":
		i, _ := value.(float64)
		return []driver.AndroidViewProp{{Name: "selectedIndex", Value: int(i)}}
	case "tint":
		// As ARGB, which an int would overflow on its way through JSON.
		c, _ := value.(float64)
		rgba := uint32(c)
		return []driver.AndroidViewProp{{Name: "tint", Value: int64(rgba>>8 | rgba<<24)}}
	case "background", "inactive", "indicator", "activeContent", "badgeColor":
		return color(value)
	}
	return nil
}

// ViewEvent implements driver.AndroidEventMapper.
func (t *tabBar) ViewEvent(name string, args []any) {
	if len(args) != 1 {
		return
	}
	if value, ok := args[0].(float64); ok {
		switch name {
		case "onChange":
			t.cx.Emit("change", value)
		case "onSize":
			t.cx.SetSize(driver.Size{Height: driver.Points(value)})
		}
	}
}

func (*tabBar) Command(string, any) {}

func (*tabBar) Close() {}
