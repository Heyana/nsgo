//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/oneo-me/nsgo/core"
)

func TestMaterialCode(t *testing.T) {
	cases := map[core.WindowMaterial]int{
		core.WindowMaterialWindow:       12,
		core.WindowMaterialContent:      18,
		core.WindowMaterialSidebar:      7,
		core.WindowMaterialHeader:       10,
		core.WindowMaterialTitlebar:     3,
		core.WindowMaterialUnderWindow:  21,
		core.WindowMaterialUnderPage:    22,
		core.WindowMaterialMenu:         5,
		core.WindowMaterialPopover:      6,
		core.WindowMaterialHUD:          13,
		core.WindowMaterialSheet:        11,
		core.WindowMaterialTooltip:      17,
		core.WindowMaterialSelection:    4,
		core.WindowMaterialFullScreenUI: 15,
		core.WindowMaterialMica:         21,
		core.WindowMaterialAcrylic:      13,
		core.WindowMaterialTabbed:       21,
	}
	for m, want := range cases {
		if got, ok := materialCode(m); !ok || got != want {
			t.Errorf("materialCode(%d) = %d, %v; want %d, true", m, got, ok, want)
		}
	}
	for _, m := range []core.WindowMaterial{core.WindowMaterialNone, core.WindowMaterial(100)} {
		if code, ok := materialCode(m); ok {
			t.Errorf("materialCode(%d) = %d, true; want no material", m, code)
		}
	}
}
