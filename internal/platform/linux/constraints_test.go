//go:build linux && !android

package linux

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestDecorationLayoutKeepsTheSessionsPlacement(t *testing.T) {
	for _, tc := range []struct {
		layout string
		limits driver.WindowConstraints
		want   string
	}{
		{"close,minimize:maximize", driver.WindowConstraints{NoMinimize: true}, "close:maximize"},
		{"minimize,close:maximize", driver.WindowConstraints{NoMinimize: true, NoMaximize: true}, "close:"},
		{"appmenu,close:minimize,maximize", driver.WindowConstraints{NoResize: true}, "appmenu,close:minimize"},
		{"close:maximize,minimize", driver.WindowConstraints{}, "close:maximize,minimize"},
		{":", driver.WindowConstraints{NoMinimize: true}, ":"},
	} {
		if got := decorationLayout(tc.layout, tc.limits); got != tc.want {
			t.Errorf("decorationLayout(%q,%+v)=%q, want %q", tc.layout, tc.limits, got, tc.want)
		}
	}
}
