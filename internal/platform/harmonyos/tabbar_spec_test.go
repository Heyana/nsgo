package harmonyos

import (
	"encoding/json"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/mobile/locales"
)

func TestHarmonyTabBarApplyPropsAndMarshal(t *testing.T) {
	var tb harmonyTabBar
	iconDrawing := &driver.Drawing{
		Ops: []driver.DrawOp{
			{Kind: driver.DrawBeginPath},
			{Kind: driver.DrawRect, X: 2, Y: 2, W: 20, H: 20},
			{Kind: driver.DrawStrokeColor, Color: driver.RGB(0xffffff)},
			{Kind: driver.DrawStroke},
		},
	}
	tb.applyProps(map[string]any{
		"items": []any{
			map[string]any{"title": "Components", "icon": "android:ic_menu_agenda", "drawing": iconDrawing},
			map[string]any{"title": "Settings", "icon": "android:ic_menu_preferences"},
		},
		"selectedIndex": float64(1),
		"edge":          "bottom",
		"label":         "Sections",
		"tint":          float64(driver.RGB(0x0a59f7)),
		"background":    float64(driver.RGB(0xf7f7f7)),
		"inactive":      float64(driver.RGB(0x666666)),
		"indicator":     float64(driver.RGBA(0x0a59f7, 0.12)),
		"activeContent": float64(driver.RGB(0x0a59f7)),
		"badgeColor":    float64(driver.RGB(0xe84026)),
		"badges": driver.TabBadges{
			Items:    []driver.TabBadge{{Dot: true}, {Text: "3"}},
			Language: "en",
		},
	}, func(d *driver.Drawing) string {
		if d == iconDrawing {
			return `[{"k":6}]`
		}
		return ""
	})

	var got tabBarSpec
	if err := json.Unmarshal([]byte(tb.marshal()), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items count = %d, want 2", len(got.Items))
	}
	if got.Items[0].Title != "Components" || got.Items[0].Drawing != `[{"k":6}]` {
		t.Fatalf("item[0] = %+v, want Components with serialized drawing", got.Items[0])
	}
	if got.Items[1].Title != "Settings" || got.Items[1].Drawing != "" {
		t.Fatalf("item[1] = %+v, want Settings without drawing", got.Items[1])
	}
	if got.Selected != 1 || got.Edge != "bottom" || got.Label != "Sections" {
		t.Fatalf("meta = selected:%d edge:%q label:%q", got.Selected, got.Edge, got.Label)
	}
	if got.Tint == "" || got.Background == "" || got.Inactive == "" || got.Indicator == "" || got.ActiveContent == "" || got.BadgeColor == "" {
		t.Fatalf("expected all palette colors to be serialized: %+v", got)
	}
	wantNew := locales.In("en").TabBadgeNew()
	if len(got.Badges) != 2 || !got.Badges[0].Dot || got.Badges[0].Announcement != wantNew {
		t.Fatalf("badge[0] = %+v, want dot with announcement %q", got.Badges[0], wantNew)
	}
	if got.Badges[1].Text != "3" || got.Badges[1].Announcement != "3" {
		t.Fatalf("badge[1] = %+v, want text 3", got.Badges[1])
	}

	// Incremental update changes only selectedIndex and clears badges and custom background.
	tb.applyProps(map[string]any{
		"selectedIndex": float64(0),
		"background":    float64(0),
		"label":         nil,
		"badges":        driver.TabBadges{Items: []driver.TabBadge{{}, {}}, Language: "en"},
	}, nil)
	got = tabBarSpec{}
	if err := json.Unmarshal([]byte(tb.marshal()), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 || got.Items[0].Title != "Components" {
		t.Fatalf("incremental update lost items: %+v", got.Items)
	}
	if got.Selected != 0 || got.Background != "" || got.Label != "" || len(got.Badges) != 0 {
		t.Fatalf("incremental update failed: %+v", got)
	}
}

func TestHarmonyTabBarNaturalSizeEdgesAndRTL(t *testing.T) {
	overlap := driver.Edges{Top: 38, Left: 12, Right: 20, Bottom: 28}
	var tb harmonyTabBar

	tb.spec.Edge = "bottom"
	sz, b, l, r := tb.naturalSize(overlap, false)
	if sz != (driver.Size{Height: tabBarHeight + 28}) || b != 28 || l != 0 || r != 0 {
		t.Fatalf("bottom = (%+v, %v, %v, %v)", sz, b, l, r)
	}

	tb.spec.Edge = "leading"
	sz, b, l, r = tb.naturalSize(overlap, false)
	if sz != (driver.Size{Width: verticalTabBarWidth + 12}) || b != 0 || l != 12 || r != 0 {
		t.Fatalf("leading LTR = (%+v, %v, %v, %v)", sz, b, l, r)
	}
	sz, b, l, r = tb.naturalSize(overlap, true)
	if sz != (driver.Size{Width: verticalTabBarWidth + 20}) || b != 0 || l != 0 || r != 20 {
		t.Fatalf("leading RTL = (%+v, %v, %v, %v)", sz, b, l, r)
	}

	tb.spec.Edge = "trailing"
	sz, b, l, r = tb.naturalSize(overlap, false)
	if sz != (driver.Size{Width: verticalTabBarWidth + 20}) || b != 0 || l != 0 || r != 20 {
		t.Fatalf("trailing LTR = (%+v, %v, %v, %v)", sz, b, l, r)
	}
	sz, b, l, r = tb.naturalSize(overlap, true)
	if sz != (driver.Size{Width: verticalTabBarWidth + 12}) || b != 0 || l != 12 || r != 0 {
		t.Fatalf("trailing RTL = (%+v, %v, %v, %v)", sz, b, l, r)
	}
}
