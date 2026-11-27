package ios

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestTVScrollCanPan(t *testing.T) {
	cases := []struct {
		name                             string
		horizontal, zooms, noScroll      bool
		content, bounds                  driver.Size
		inTop, inLeft, inBottom, inRight driver.Points
		want                             bool
	}{
		{
			name:    "vertical overflowing content",
			content: driver.Size{Width: 400, Height: 900},
			bounds:  driver.Size{Width: 400, Height: 300},
			want:    true,
		},
		{
			name:    "vertical content fits within slack",
			content: driver.Size{Width: 400, Height: 300.4},
			bounds:  driver.Size{Width: 400, Height: 300},
			want:    false,
		},
		{
			name:     "vertical overflows due to content insets",
			content:  driver.Size{Width: 400, Height: 280},
			bounds:   driver.Size{Width: 400, Height: 300},
			inTop:    20,
			inBottom: 20,
			want:     true,
		},
		{
			name:     "vertical disabled by NoScroll",
			noScroll: true,
			content:  driver.Size{Width: 400, Height: 900},
			bounds:   driver.Size{Width: 400, Height: 300},
			want:     false,
		},
		{
			name:       "horizontal overflowing width",
			horizontal: true,
			content:    driver.Size{Width: 1200, Height: 200},
			bounds:     driver.Size{Width: 400, Height: 200},
			want:       true,
		},
		{
			name:       "horizontal ignores height-only overflow",
			horizontal: true,
			content:    driver.Size{Width: 400, Height: 900},
			bounds:     driver.Size{Width: 400, Height: 200},
			want:       false,
		},
		{
			name:    "zooming scroll allows horizontal overflow",
			zooms:   true,
			content: driver.Size{Width: 800, Height: 200},
			bounds:  driver.Size{Width: 400, Height: 200},
			want:    true,
		},
		{
			name:    "unlaid zero viewport cannot pan",
			content: driver.Size{Width: 400, Height: 900},
			bounds:  driver.Size{Width: 0, Height: 0},
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tvScrollCanPan(tc.horizontal, tc.zooms, tc.noScroll, tc.content, tc.bounds, tc.inTop, tc.inLeft, tc.inBottom, tc.inRight)
			if got != tc.want {
				t.Fatalf("tvScrollCanPan = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTVScrollAllowsIndirectPanAndFocus(t *testing.T) {
	// tvOS overflowing scroll view with no focusable descendants takes indirect touches and focus.
	if !tvScrollAllowsIndirectPan(true, true, true, false) {
		t.Error("tvOS non-focusable overflowing scroll view did not enable indirect pan")
	}
	if !tvScrollCanBecomeFocused(true, true, true, false, false) {
		t.Error("tvOS non-focusable overflowing scroll view did not become focusable")
	}

	// iOS / iPadOS never enables tvOS indirect pan or self-focus.
	if tvScrollAllowsIndirectPan(false, true, true, false) {
		t.Error("iOS unexpectedly enabled tvOS indirect pan")
	}
	if tvScrollCanBecomeFocused(false, true, true, false, false) {
		t.Error("iOS unexpectedly made scroll view focusable")
	}

	// Scroll view containing focusable buttons/inputs keeps focus-driven scrolling untouched.
	if tvScrollAllowsIndirectPan(true, true, true, true) {
		t.Error("tvOS scroll view with focusable children enabled indirect pan")
	}
	if tvScrollCanBecomeFocused(true, true, true, true, false) {
		t.Error("tvOS scroll view with focusable children became focusable")
	}

	// Outer scroll view wrapping an inner remote-scrollable scroll view (and no buttons):
	// inner takes focus while outer keeps indirect pan so perpendicular/edge swipes bubble up.
	if !tvScrollAllowsIndirectPan(true, true, true, false) {
		t.Error("outer tvOS scroll view with nested remote scroll disabled indirect pan")
	}
	if tvScrollCanBecomeFocused(true, true, true, false, true) {
		t.Error("outer tvOS scroll view stole focus from nested focusable scroll view")
	}

	// Non-overflowing or ineligible (hidden, disabled, behind modal) scroll view enables neither.
	if tvScrollAllowsIndirectPan(true, false, true, false) || tvScrollCanBecomeFocused(true, false, true, false, false) {
		t.Error("ineligible tvOS scroll view enabled remote scrolling")
	}
	if tvScrollAllowsIndirectPan(true, true, false, false) || tvScrollCanBecomeFocused(true, true, false, false, false) {
		t.Error("non-overflowing tvOS scroll view enabled remote scrolling")
	}
}

func TestTVScrollPanAllowedTouchTypes(t *testing.T) {
	if got := tvScrollPanAllowedTouchTypes(nil, false); len(got) != 0 {
		t.Fatalf("empty defaults, indirect=false = %v, want empty", got)
	}
	if got := tvScrollPanAllowedTouchTypes([]int{touchTypeDirect}, false); !slices.Equal(got, []int{touchTypeDirect}) {
		t.Fatalf("direct default, indirect=false = %v, want [%d]", got, touchTypeDirect)
	}
	if got := tvScrollPanAllowedTouchTypes(nil, true); !slices.Equal(got, []int{touchTypeIndirect}) {
		t.Fatalf("empty defaults, indirect=true = %v, want [%d]", got, touchTypeIndirect)
	}
	if got := tvScrollPanAllowedTouchTypes([]int{touchTypeDirect}, true); !slices.Equal(got, []int{touchTypeDirect, touchTypeIndirect}) {
		t.Fatalf("direct default, indirect=true = %v, want [%d %d]", got, touchTypeDirect, touchTypeIndirect)
	}
	if got := tvScrollPanAllowedTouchTypes([]int{touchTypeDirect, touchTypeIndirect}, true); !slices.Equal(got, []int{touchTypeDirect, touchTypeIndirect}) {
		t.Fatalf("already-indirect default, indirect=true = %v, want no duplicate", got)
	}
}
