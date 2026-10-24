//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/apple"
)

// The window's background material is an NSVisualEffectView behind the
// app's views, blending with what is behind the window. The window itself
// gives up its opaque background so the material shows where the app draws
// nothing opaque. NSVisualEffectMaterial values follow AppKit's.

var (
	selSetMaterial         = sel("setMaterial:")
	selSetBlendingMode     = sel("setBlendingMode:")
	selSetState            = sel("setState:")
	selSetAutoresizingMask = sel("setAutoresizingMask:")
	selSetOpaque           = sel("setOpaque:")
	selClearColor          = sel("clearColor")
)

const (
	blendingBehindWindow = 0
	effectStateActive    = 1
	widthHeightSizable   = 2 | 16 // NSViewWidthSizable | NSViewHeightSizable
	viewBelow            = -1     // NSWindowBelow
)

// materialCode returns m's NSVisualEffectMaterial, and whether macOS has
// it.
func materialCode(m driver.WindowMaterial) (int, bool) {
	switch m {
	case driver.WindowMaterialWindow:
		return 12, true // windowBackground
	case driver.WindowMaterialContent:
		return 18, true // contentBackground
	case driver.WindowMaterialSidebar:
		return 7, true // sidebar
	case driver.WindowMaterialHeader:
		return 10, true // headerView
	case driver.WindowMaterialTitlebar:
		return 3, true // titlebar
	case driver.WindowMaterialUnderWindow:
		return 21, true // underWindowBackground
	case driver.WindowMaterialUnderPage:
		return 22, true // underPageBackground
	case driver.WindowMaterialMenu:
		return 5, true // menu
	case driver.WindowMaterialPopover:
		return 6, true // popover
	case driver.WindowMaterialHUD:
		return 13, true // hudWindow
	case driver.WindowMaterialSheet:
		return 11, true // sheet
	case driver.WindowMaterialTooltip:
		return 17, true // toolTip
	case driver.WindowMaterialSelection:
		return 4, true // selection
	case driver.WindowMaterialFullScreenUI:
		return 15, true // fullScreenUI
	// Windows 11's backdrops: the closest materials.
	case driver.WindowMaterialMica, driver.WindowMaterialTabbed:
		return 21, true // underWindowBackground
	case driver.WindowMaterialAcrylic:
		return 13, true // hudWindow
	}
	return 0, false
}

// applyMaterial puts the window's material behind the app's views. It
// does nothing without a material.
func (s *surface) applyMaterial(m driver.WindowMaterial) {
	code, ok := materialCode(m)
	if !ok {
		return
	}
	s.material = m
	effect := objc.ID(classMaterial).Send(selAlloc).Send(sel("initWithFrame:"),
		objc.Send[apple.CGRect](s.native, selBounds))
	effect.Send(selSetMaterial, code)
	effect.Send(selSetBlendingMode, blendingBehindWindow)
	effect.Send(selSetState, effectStateActive)
	effect.Send(selSetAutoresizingMask, widthHeightSizable)
	s.native.Send(selAddSubviewPositioned, effect, viewBelow, objc.ID(0))
	effect.Send(selRelease)
	s.window.Send(selSetOpaque, false)
	s.window.Send(selSetBackgroundColor, objc.ID(objc.GetClass("NSColor")).Send(selClearColor))
}
