//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// The window's background material is an NSVisualEffectView behind the app's views, blending with what is behind the window. The window itself gives up its opaque background so the material shows where the app draws nothing opaque, and takes it back without one unless it is transparent (applyBackground). NSVisualEffectMaterial values follow AppKit's.

var (
	selSetMaterial            = sel("setMaterial:")
	selSetBlendingMode        = sel("setBlendingMode:")
	selSetState               = sel("setState:")
	selSetAutoresizingMask    = sel("setAutoresizingMask:")
	selSetOpaque              = sel("setOpaque:")
	selClearColor             = sel("clearColor")
	selSetEffectIsInteractive = sel("setEffectIsInteractive:")
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

// setMaterial puts m behind the app's views, or takes the material away and makes the window opaque again for WindowMaterialNone, unless the window or panel is transparent, which stays clear (applyBackground).
func (s *surface) setMaterial(m driver.WindowMaterial) {
	code, ok := materialCode(m)
	if !ok {
		if s.effect != 0 {
			s.effect.Send(sel("removeFromSuperview"))
			s.effect = 0
			s.applyBackground()
		}
		s.material = driver.WindowMaterialNone
		return
	}
	if s.effect == 0 {
		s.effect = objc.ID(classMaterial).Send(selAlloc).Send(sel("initWithFrame:"),
			objc.Send[apple.CGRect](s.native, selBounds))
		s.effect.Send(selSetBlendingMode, blendingBehindWindow)
		s.effect.Send(selSetState, effectStateActive)
		s.effect.Send(selSetAutoresizingMask, widthHeightSizable)
		s.native.Send(selAddSubviewPositioned, s.effect, viewBelow, objc.ID(0))
		s.effect.Send(selRelease)
		s.applyBackground()
	}
	s.effect.Send(selSetMaterial, code)
	s.material = m
}

// A view's backdrop blur is a CABackdropLayer under its children whose Gaussian filter follows the style's radius, as CSS's backdrop-filter does; an NSVisualEffectView of the closest material when the view names an iOS material, whose strength AppKit sets by material, not radius; or Liquid Glass on macOS 26 and later. The view's background is a tint above the blur, where CSS has it. The Gaussian blur samples everything drawn under it, the view's own layer included, so that layer leaves the background to the tint (styleLayer); the material is opaque enough to hide it.

const blendingWithinWindow = 1

// backdropMaterial is the NSVisualEffectMaterial of a backdrop material.
func backdropMaterial(m driver.BackdropMaterial) int {
	switch m {
	case driver.BackdropThin:
		return 6 // popover
	case driver.BackdropRegular:
		return 5 // menu
	case driver.BackdropThick:
		return 11 // sheet
	case driver.BackdropChrome:
		return 3 // titlebar
	}
	return 15 // fullScreenUI
}

// keepGlassActive keeps Liquid Glass as it is in a window in the back or
// an app not in front, as a blur does at effectStateActive: AppKit redraws
// it flat from the window's active appearance, and NSGlassEffectView has
// no state of its own to set. The appearance is the window's, so its title
// bar stays active too, for as long as it lives. A host's window is the
// host's to say.
func (s *surface) keepGlassActive() {
	force := sel("_setForceMainAppearance:")
	if s.glassActive || s.embedded || s.window == 0 || !objc.Send[bool](s.window, sel("respondsToSelector:"), force) {
		return
	}
	s.window.Send(force, true)
	s.glassActive = true
}

// fitBackdrop lays v's blur out for style vs.
func (v *view) fitBackdrop(vs *driver.ViewStyle) {
	b := objc.Send[apple.CGRect](v.native, selBounds)
	apple.FitBackdrop(v.backdrop, vs, b.Size.Width, b.Size.Height)
	if v.backdropLayer != 0 {
		// A layer of its own would ease to its frame on its own.
		apple.WithoutAnimation(func() { v.backdropLayer.Send(selSetFrame, objc.Send[apple.CGRect](v.backdrop, selBounds)) })
	}
	if v.tint != 0 {
		v.tint.Send(selSetFrame, objc.Send[apple.CGRect](v.backdrop, selBounds))
	}
}

// drawsGlass reports whether vs's backdrop is Liquid Glass, which macOS has from version 26.
func drawsGlass(vs *driver.ViewStyle) bool {
	return vs.Glass != driver.GlassNone && apple.Class("NSGlassEffectView") != 0
}

// blursByRadius reports whether vs's backdrop is drawn by its filters, as CSS's backdrop-filter draws it: a Gaussian blur of the style's radius and its color filters as a matrix, with no glass to draw and no material for a blur. BackdropMaterial's zero value names none, so a bare blur follows its radius and an iOS material keeps AppKit's closest one; color filters without a blur draw no material, as on iOS, and change the colors behind.
func blursByRadius(vs *driver.ViewStyle, glass bool) bool {
	if glass || vs.Effects == nil {
		return false
	}
	_, color, blur, _ := driver.SplitFilters(vs.Effects.BackdropFilters)
	if blur > 0 {
		return vs.Material == driver.BackdropUltraThin
	}
	return color
}

// borderHost is the layer v's per-side borders go in, above its backdrop and below its children: the glass's content view, as Liquid Glass draws over its own layer's sublayers, else the tint over the material or blur; zero, v's own layer, without a backdrop.
func (v *view) borderHost() objc.ID {
	switch {
	case v.backdrop == 0:
		return 0
	case v.glass:
		return objc.ID(apple.Msg(v.glassContent, selLayer))
	case v.tint != 0:
		return objc.ID(apple.Msg(v.tint, selLayer))
	}
	return 0
}

// setBackdrop gives v the backdrop its style asks for, or takes it away.
func (v *view) setBackdrop(vs *driver.ViewStyle) {
	glass := drawsGlass(vs)
	gaussian := blursByRadius(vs, glass)
	want := glass || gaussian || vs.Effects.BackdropBlur() > 0
	if !want || glass != v.glass || gaussian != v.gaussian {
		apple.MoveBorders(v.native.Send(selLayer), 0)
		if v.backdrop != 0 {
			v.backdrop.Send(sel("removeFromSuperview"))
			v.backdrop, v.backdropLayer, v.tint, v.glassContent = 0, 0, 0, 0
			v.tintState.Reset()
		}
		if !want {
			v.glass, v.gaussian, v.glassInteractive = false, false, false
			return
		}
	}
	if v.backdrop == 0 {
		class := classMaterial
		if gaussian {
			// A holder the mouse goes through, as the material is.
			class = classOverlay
		}
		if glass {
			class = apple.Class("NSGlassEffectView")
		}
		v.backdrop = objc.ID(class).Send(selAlloc).Send(sel("initWithFrame:"), objc.Send[apple.CGRect](v.native, selBounds))
		v.backdrop.Send(selSetAutoresizingMask, widthHeightSizable)
		if gaussian {
			v.backdrop.Send(selSetWantsLayer, true)
			v.backdropLayer = objc.ID(apple.Class("CABackdropLayer")).Send(selNew)
			v.backdrop.Send(selLayer).Send(sel("addSublayer:"), v.backdropLayer)
			v.backdropLayer.Send(selRelease)
		}
		v.native.Send(selAddSubviewPositioned, v.backdrop, viewBelow, objc.ID(0))
		v.backdrop.Send(selRelease)
		v.glass = glass
		v.gaussian = gaussian
	}
	if glass {
		v.surface.keepGlassActive()
		v.backdrop.Send(sel("setStyle:"), int(vs.Glass)-1)
		v.backdrop.Send(sel("setCornerRadius:"), float64(max(vs.CornerRadius, vs.Corners.TopLeft)))
		interactive := false
		if objc.Send[bool](v.backdrop, sel("respondsToSelector:"), selSetEffectIsInteractive) {
			interactive = vs.GlassInteractive
			v.backdrop.Send(selSetEffectIsInteractive, interactive)
		}
		v.glassInteractive = interactive
		if v.glassContent == 0 {
			// Glass draws over its own layer's sublayers but under its content view.
			v.glassContent = newBorderHost(v.backdrop, classGlassContent)
			v.backdrop.Send(sel("setContentView:"), v.glassContent)
			v.glassContent.Send(selRelease)
		}
		apple.MoveBorders(v.native.Send(selLayer), v.borderHost())
		return
	}
	v.glassInteractive = false
	if gaussian {
		v.backdropLayer.Send(sel("setFilters:"), apple.BackdropFilters(vs.Effects.BackdropFilters))
	} else {
		v.backdrop.Send(selSetMaterial, backdropMaterial(vs.Material))
		v.backdrop.Send(selSetBlendingMode, blendingWithinWindow)
		v.backdrop.Send(selSetState, effectStateActive)
		v.backdrop.Send(sel("setWantsLayer:"), true)
	}
	v.fitBackdrop(vs)
	if v.tint == 0 {
		v.tint = newBorderHost(v.backdrop, classOverlay)
		v.backdrop.Send(sel("addSubview:"), v.tint)
		v.tint.Send(selRelease)
	}
	apple.MoveBorders(v.native.Send(selLayer), v.borderHost())
}

// newBorderHost returns a layer-backed view of class filling backdrop, with the same coordinate orientation as NSGo's views.
func newBorderHost(backdrop objc.ID, class objc.Class) objc.ID {
	host := objc.ID(class).Send(selAlloc).Send(sel("initWithFrame:"), objc.Send[apple.CGRect](backdrop, selBounds))
	host.Send(selSetAutoresizingMask, widthHeightSizable)
	host.Send(sel("setWantsLayer:"), true)
	return host
}

// styleTint draws the background above the blur, with the same transition and presentation-value retargeting as the view's own layer. Only color changes touch it, so frames moving other properties leave a tint's transition alone. A background binding takes over immediately.
func (v *view) styleTint(vs *driver.ViewStyle) {
	if v.tint == 0 {
		return
	}
	v.tintState.Style(objc.ID(apple.Msg(v.tint, selLayer)), vs, v.animated.Has(driver.PropBackground))
}
