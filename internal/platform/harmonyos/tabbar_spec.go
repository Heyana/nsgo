package harmonyos

import (
	"encoding/json"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
	"github.com/nsgo-dev/nsgo/mobile/locales"
)

// tabBarHeight is HarmonyOS ArkUI's standard bottom tab bar height in vp.
// verticalTabBarWidth is the width of a vertical leading or trailing rail.
const (
	tabBarHeight        driver.Points = 56
	verticalTabBarWidth driver.Points = 72
)

// tabBarItemSpec is one tab serialized into the "tb" op for Index.ets.
type tabBarItemSpec struct {
	Title   string `json:"t"`
	Icon    string `json:"i,omitempty"`
	Drawing string `json:"d,omitempty"`
}

// tabBarBadgeSpec is one tab's badge serialized into the "tb" op for Index.ets.
type tabBarBadgeSpec struct {
	Dot          bool   `json:"d,omitempty"`
	Text         string `json:"t,omitempty"`
	Announcement string `json:"a,omitempty"`
}

// tabBarSpec is the JSON payload Index.ets reads from op "tb" to draw a native
// core.TabBar with ArkUI Tabs, TabContent and Badge.
type tabBarSpec struct {
	Items         []tabBarItemSpec  `json:"items"`
	Badges        []tabBarBadgeSpec `json:"badges,omitempty"`
	Selected      int               `json:"selected"`
	Edge          string            `json:"edge,omitempty"`
	Label         string            `json:"label,omitempty"`
	Tint          string            `json:"tint,omitempty"`
	Background    string            `json:"bg,omitempty"`
	Inactive      string            `json:"inactive,omitempty"`
	Indicator     string            `json:"indicator,omitempty"`
	ActiveContent string            `json:"active,omitempty"`
	BadgeColor    string            `json:"badgeColor,omitempty"`
	Dark          bool              `json:"dark,omitempty"`
	InsetBottom   float64           `json:"insetBottom,omitempty"`
	InsetLeft     float64           `json:"insetLeft,omitempty"`
	InsetRight    float64           `json:"insetRight,omitempty"`
}

// harmonyTabBar keeps the merged props and reported size of a mounted
// core.TabBar view on HarmonyOS.
type harmonyTabBar struct {
	spec            tabBarSpec
	reportedSize    driver.Size
	hasReportedSize bool
	lastJSON        string
}

// applyProps folds a NativeConfig delta map into the tab bar's state.
func (b *harmonyTabBar) applyProps(props map[string]any, encodeDrawing func(*driver.Drawing) string) {
	if b.spec.Edge == "" {
		b.spec.Edge = "bottom"
	}
	for name, value := range props {
		switch name {
		case "items":
			b.spec.Items = encodeHarmonyTabItems(value, encodeDrawing)
		case "selectedIndex":
			b.spec.Selected = tabBarInt(value)
		case "edge":
			if s, ok := value.(string); ok && s != "" {
				b.spec.Edge = s
			} else {
				b.spec.Edge = "bottom"
			}
		case "label":
			if s, ok := value.(string); ok {
				b.spec.Label = s
			} else {
				b.spec.Label = ""
			}
		case "tint":
			b.spec.Tint = tabBarColor(value)
		case "background":
			b.spec.Background = tabBarColor(value)
		case "inactive":
			b.spec.Inactive = tabBarColor(value)
		case "indicator":
			b.spec.Indicator = tabBarColor(value)
		case "activeContent":
			b.spec.ActiveContent = tabBarColor(value)
		case "badgeColor":
			b.spec.BadgeColor = tabBarColor(value)
		case "badges":
			b.spec.Badges = encodeHarmonyTabBadges(value)
		}
	}
}

// naturalSize returns the size the tab bar asks layout for given its overlap
// with the window safe area edges, plus the bottom, left and right padding the
// native bar applies to keep its items clear of system bars.
func (b *harmonyTabBar) naturalSize(overlap driver.Edges, rtl bool) (size driver.Size, insetBottom, insetLeft, insetRight float64) {
	switch b.spec.Edge {
	case "leading":
		if rtl {
			insetRight = float64(overlap.Right)
			return driver.Size{Width: verticalTabBarWidth + overlap.Right}, 0, 0, insetRight
		}
		insetLeft = float64(overlap.Left)
		return driver.Size{Width: verticalTabBarWidth + overlap.Left}, 0, insetLeft, 0
	case "trailing":
		if rtl {
			insetLeft = float64(overlap.Left)
			return driver.Size{Width: verticalTabBarWidth + overlap.Left}, 0, insetLeft, 0
		}
		insetRight = float64(overlap.Right)
		return driver.Size{Width: verticalTabBarWidth + overlap.Right}, 0, 0, insetRight
	default:
		insetBottom = float64(overlap.Bottom)
		return driver.Size{Height: tabBarHeight + overlap.Bottom}, insetBottom, 0, 0
	}
}

// marshal returns the JSON payload for op "tb".
func (b *harmonyTabBar) marshal() string {
	data, err := json.Marshal(b.spec)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func encodeHarmonyTabItems(value any, encodeDrawing func(*driver.Drawing) string) []tabBarItemSpec {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]tabBarItemSpec, 0, len(items))
	for _, raw := range items {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		var item tabBarItemSpec
		if t, ok := m["title"].(string); ok {
			item.Title = t
		}
		if ic, ok := m["icon"].(string); ok {
			item.Icon = ic
		}
		if d, ok := m["drawing"].(*driver.Drawing); ok && d != nil && len(d.Ops) > 0 && encodeDrawing != nil {
			item.Drawing = encodeDrawing(d)
		}
		out = append(out, item)
	}
	return out
}

func encodeHarmonyTabBadges(value any) []tabBarBadgeSpec {
	badges, ok := value.(driver.TabBadges)
	if !ok || len(badges.Items) == 0 {
		return nil
	}
	messages := locales.In(badges.Language)
	out := make([]tabBarBadgeSpec, len(badges.Items))
	anySet := false
	for i, badge := range badges.Items {
		announcement := badge.Text
		if announcement == "" && badge.Dot {
			announcement = messages.TabBadgeNew()
		}
		if badge.Dot || badge.Text != "" {
			anySet = true
		}
		out[i] = tabBarBadgeSpec{
			Dot:          badge.Dot,
			Text:         badge.Text,
			Announcement: announcement,
		}
	}
	if !anySet {
		return nil
	}
	return out
}

func tabBarColor(value any) string {
	switch c := value.(type) {
	case float64:
		if c == 0 {
			return ""
		}
		return webstyle.Color(driver.Color(uint32(c)))
	case driver.Color:
		if c == 0 {
			return ""
		}
		return webstyle.Color(c)
	case uint32:
		if c == 0 {
			return ""
		}
		return webstyle.Color(driver.Color(c))
	}
	return ""
}

func tabBarInt(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}
