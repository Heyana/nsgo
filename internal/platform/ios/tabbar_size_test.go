package ios

import (
	"math"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestTabBarSizingDetachedInitialAndFallbacks(t *testing.T) {
	cases := []struct {
		name string
		fit  float64
		want driver.Points
	}{
		{"normal height", 49, 49},
		{"liquid glass or taller base", 62, 62},
		{"zero falls back", 0, defaultTabBarHeight},
		{"negative falls back", -12, defaultTabBarHeight},
		{"nan falls back", math.NaN(), defaultTabBarHeight},
		{"positive inf falls back", math.Inf(1), defaultTabBarHeight},
		{"negative inf falls back", math.Inf(-1), defaultTabBarHeight},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s tabBarSizing
			w, ok := s.measureWidth(false, 0, 0)
			if !ok || w != defaultTabBarWidth {
				t.Fatalf("measureWidth(detached) = (%v, %v), want (%v, true)", w, ok, defaultTabBarWidth)
			}
			h, changed := s.update(tc.fit)
			if !changed || h != tc.want {
				t.Fatalf("update(%v) = (%v, %v), want (%v, true)", tc.fit, h, changed, tc.want)
			}
			// Further detached callbacks before attachment (e.g. setItems) or on detach must not re-measure.
			if w, ok := s.measureWidth(false, 390, 49); ok || w != 0 {
				t.Fatalf("second detached measureWidth = (%v, %v), want (0, false)", w, ok)
			}
		})
	}
}

func TestTabBarSizingAttachmentPlacementAndDeduplication(t *testing.T) {
	var s tabBarSizing
	var reported []driver.Points
	measure := func(attached bool, bw, bh, fit float64) float64 {
		w, ok := s.measureWidth(attached, bw, bh)
		if !ok {
			return 0
		}
		if h, changed := s.update(fit); changed {
			reported = append(reported, h)
		}
		return w
	}

	// Creation while detached: measures once at default width, without any key-window safe area.
	if w := measure(false, 0, 0, 49); w != defaultTabBarWidth {
		t.Fatalf("detached fit width = %v, want %v", w, defaultTabBarWidth)
	}

	// Initial attachment before first relayout (height 0): skipped, preserving base height.
	if w := measure(true, 390, 0, 49); w != 0 {
		t.Fatalf("pre-layout attached fit width = %v, want 0", w)
	}

	// Placed at Top: 60 (no bottom safe-area overlap): matches initial 49, so no duplicate SetSize.
	if w := measure(true, 390, 49, 49); w != 390 {
		t.Fatalf("top placement fit width = %v, want 390", w)
	}

	// Moved to Bottom: 0 (34pt bottom safe area): reports 83.
	if w := measure(true, 390, 49, 83); w != 390 {
		t.Fatalf("bottom placement fit width = %v, want 390", w)
	}
	// Relayout to the new 83pt frame repeats sizeThatFits == 83: deduplicated.
	measure(true, 390, 83, 83)
	// Sub-epsilon float jitter is also deduplicated.
	measure(true, 390, 83, 83.004)
	measure(true, 390, 83, 82.996)

	// Moved to Bottom: 34 (above safe area): shrinks back to 49.
	measure(true, 390, 83, 49)
	measure(true, 390, 49, 49)

	// Moved to Bottom: 17 (half safe-area overlap): grows to 66.
	measure(true, 390, 49, 66)
	measure(true, 390, 66, 66)

	// Real fractional-point safe-area change (e.g. 1/3 pt on 3x display) is reported.
	measure(true, 390, 66, 66.333333)

	// Rotated to landscape (compact height 32 + 21 bottom inset = 53) with wider bounds.
	if w := measure(true, 844, 66.333333, 53); w != 844 {
		t.Fatalf("landscape fit width = %v, want 844", w)
	}
	measure(true, 844, 53, 53)

	// Hidden via DisplayNone (collapsed to 0x0): skipped, preserving last height.
	if w := measure(true, 0, 0, 49); w != 0 {
		t.Fatalf("collapsed DisplayNone fit width = %v, want 0", w)
	}
	if w := measure(true, 0, 53, 49); w != 0 {
		t.Fatalf("zero-width fit width = %v, want 0", w)
	}

	// Unhidden back at Top: 60 in portrait: updates to 49.
	if w := measure(true, 390, 53, 49); w != 390 {
		t.Fatalf("restored portrait fit width = %v, want 390", w)
	}

	// Detached from window (e.g. reparenting or closing): skipped.
	if w := measure(false, 390, 49, 99); w != 0 {
		t.Fatalf("post-detach fit width = %v, want 0", w)
	}

	// Re-attached to another window (e.g. iPad with 50 + 20 safe area): updates to 70.
	if w := measure(true, 768, 49, 70); w != 768 {
		t.Fatalf("re-attached iPad fit width = %v, want 768", w)
	}

	want := []driver.Points{49, 83, 49, 66, driver.Points(66.333333), 53, 49, 70}
	if !slices.Equal(reported, want) {
		t.Fatalf("reported heights = %v, want %v", reported, want)
	}
}
