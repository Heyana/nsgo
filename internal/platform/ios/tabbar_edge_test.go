package ios

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestParseTabBarEdge(t *testing.T) {
	cases := []struct {
		in   any
		want tabBarEdge
	}{
		{nil, tabBarEdgeBottom},
		{"", tabBarEdgeBottom},
		{"bottom", tabBarEdgeBottom},
		{"leading", tabBarEdgeLeading},
		{"trailing", tabBarEdgeTrailing},
		{"unknown", tabBarEdgeBottom},
		{42, tabBarEdgeBottom},
	}
	for _, c := range cases {
		if got := parseTabBarEdge(c.in); got != c.want {
			t.Errorf("parseTabBarEdge(%#v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestMapVerticalBarEdge(t *testing.T) {
	cases := []struct {
		raw  int
		want driver.VerticalBarEdge
	}{
		{0, driver.VerticalBarUnspecified},
		{1, driver.VerticalBarLeading},
		{2, driver.VerticalBarTrailing},
		{99, driver.VerticalBarUnspecified},
		{-1, driver.VerticalBarUnspecified},
	}
	for _, c := range cases {
		if got := mapVerticalBarEdge(c.raw); got != c.want {
			t.Errorf("mapVerticalBarEdge(%d) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestTabBarNaturalSize(t *testing.T) {
	// Bottom bar asks for height alone, falling back to 49 points when sizeThatFits reports zero.
	if got := tabBarNaturalSize(tabBarEdgeBottom, 0, false, 16, 20, 34); got != (driver.Size{Height: 83}) {
		t.Errorf("bottom fallback size = %+v, want height 83", got)
	}
	if got := tabBarNaturalSize(tabBarEdgeBottom, 52, false, 16, 20, 0); got != (driver.Size{Height: 52}) {
		t.Errorf("bottom fitted size = %+v, want height 52", got)
	}
	// Vertical bar asks for width alone, adding only the physical edge's safe area inset.
	if got := tabBarNaturalSize(tabBarEdgeLeading, 49, false, 16, 84, 34); got != (driver.Size{Width: verticalTabBarWidth + 16}) {
		t.Errorf("leading LTR size = %+v, want width %v", got, verticalTabBarWidth+16)
	}
	if got := tabBarNaturalSize(tabBarEdgeTrailing, 49, false, 16, 84, 34); got != (driver.Size{Width: verticalTabBarWidth + 84}) {
		t.Errorf("trailing LTR size = %+v, want width %v", got, verticalTabBarWidth+84)
	}
	if got := tabBarNaturalSize(tabBarEdgeLeading, 49, true, 16, 84, 34); got != (driver.Size{Width: verticalTabBarWidth + 84}) {
		t.Errorf("leading RTL size = %+v, want width %v", got, verticalTabBarWidth+84)
	}
	if got := tabBarNaturalSize(tabBarEdgeTrailing, 49, true, 16, 84, 34); got != (driver.Size{Width: verticalTabBarWidth + 16}) {
		t.Errorf("trailing RTL size = %+v, want width %v", got, verticalTabBarWidth+16)
	}
}

func TestVerticalBarSeparatorX(t *testing.T) {
	if got := verticalBarSeparatorX(tabBarEdgeLeading, false, 80); got != 79 {
		t.Errorf("leading LTR separator x = %v, want 79", got)
	}
	if got := verticalBarSeparatorX(tabBarEdgeTrailing, false, 80); got != 0 {
		t.Errorf("trailing LTR separator x = %v, want 0", got)
	}
	if got := verticalBarSeparatorX(tabBarEdgeLeading, true, 80); got != 0 {
		t.Errorf("leading RTL separator x = %v, want 0", got)
	}
	if got := verticalBarSeparatorX(tabBarEdgeTrailing, true, 80); got != 79 {
		t.Errorf("trailing RTL separator x = %v, want 79", got)
	}
}
