//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/platform/internal/apple"
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
func materialCode(m core.WindowMaterial) (int, bool) {
	switch m {
	case core.WindowMaterialWindow:
		return 12, true // windowBackground
	case core.WindowMaterialContent:
		return 18, true // contentBackground
	case core.WindowMaterialSidebar:
		return 7, true // sidebar
	case core.WindowMaterialHeader:
		return 10, true // headerView
	case core.WindowMaterialTitlebar:
		return 3, true // titlebar
	case core.WindowMaterialUnderWindow:
		return 21, true // underWindowBackground
	case core.WindowMaterialUnderPage:
		return 22, true // underPageBackground
	case core.WindowMaterialMenu:
		return 5, true // menu
	case core.WindowMaterialPopover:
		return 6, true // popover
	case core.WindowMaterialHUD:
		return 13, true // hudWindow
	case core.WindowMaterialSheet:
		return 11, true // sheet
	case core.WindowMaterialTooltip:
		return 17, true // toolTip
	case core.WindowMaterialSelection:
		return 4, true // selection
	case core.WindowMaterialFullScreenUI:
		return 15, true // fullScreenUI
	// Windows 11's backdrops: the closest materials.
	case core.WindowMaterialMica, core.WindowMaterialTabbed:
		return 21, true // underWindowBackground
	case core.WindowMaterialAcrylic:
		return 13, true // hudWindow
	}
	return 0, false
}

// applyMaterial puts the window's material behind the app's views. It
// does nothing without a material.
func (s *surface) applyMaterial(m core.WindowMaterial) {
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
