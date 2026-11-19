package macos

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestPanelCornerRadiusSquaresOffTheStrip: the collapsed strip is StripWidth
// points wide. Rounding it by the panel's 12pt radius would put the corner
// arcs past the middle, and AppKit clips the window into a pointed lens —
// exactly the spike the strip showed. Any panel thinner than two radii must
// carry no radius at all; a full panel keeps its 12pt.
func TestPanelCornerRadiusSquaresOffTheStrip(t *testing.T) {
	cases := []struct {
		width driver.Points
		want  float64
	}{
		{6, 0},   // the collapsed strip
		{12, 0},  // exactly two radii: arcs would meet, so square
		{40, 12}, // anything wider keeps the panel radius
		{320, 12},
	}
	for _, c := range cases {
		if got := panelCornerRadius(c.width); got != c.want {
			t.Errorf("panelCornerRadius(%v) = %v, want %v", c.width, got, c.want)
		}
	}
}
