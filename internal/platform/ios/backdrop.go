package ios

import "github.com/nsgo-dev/nsgo/internal/driver"

// This file builds on every system, so tests check the backdrop mode, material mapping, and tint target decisions on any host without UIKit.

// backdropMode is how iOS draws a view's backdrop.
type backdropMode uint8

const (
	backdropNone backdropMode = iota
	// backdropMaterial is a UIVisualEffectView of the view's system material.
	backdropMaterial
	// backdropFiltered is a CABackdropLayer with the view's blur and color matrix filters.
	backdropFiltered
	// backdropGlass is a UIVisualEffectView with UIGlassEffect (iOS 26 and later).
	backdropGlass
)

// usesTint reports whether mode draws and animates the view's background color on a tint layer above its backdrop (UIVisualEffectView's contentView for backdropMaterial, or the tint view above CABackdropLayer for backdropFiltered).
func (m backdropMode) usesTint() bool {
	return m == backdropMaterial || m == backdropFiltered
}

// blurStyle is the UIBlurEffectStyle of a backdrop material.
func blurStyle(m driver.BackdropMaterial) int {
	return 6 + int(m) // systemUltraThinMaterial and on
}

// chooseBackdrop reports how iOS draws vs's backdrop given whether UIKit has
// UIGlassEffect. Liquid Glass takes precedence when available. Otherwise color
// filters draw through a CABackdropLayer (with a Gaussian blur first when
// blurred), matching macOS: color filters alone always use it regardless of
// Material, and color filters with a blur use it unless Material names one
// other than BackdropUltraThin, the zero value. A plain blur without color
// filters, or a blur with an explicit material, uses UIVisualEffectView's
// system material.
func chooseBackdrop(vs *driver.ViewStyle, hasGlass bool) backdropMode {
	if vs == nil {
		return backdropNone
	}
	if vs.Glass != driver.GlassNone && hasGlass {
		return backdropGlass
	}
	if vs.Effects == nil {
		return backdropNone
	}
	_, color, blur, _ := driver.SplitFilters(vs.Effects.BackdropFilters)
	if color && (blur <= 0 || vs.Material == driver.BackdropUltraThin) {
		return backdropFiltered
	}
	if blur > 0 {
		return backdropMaterial
	}
	return backdropNone
}

// hidesOwnBackground reports whether the view's own layer leaves its background
// to the backdrop's tint view so a CABackdropLayer does not filter the view's
// own background and draw it twice.
func hidesOwnBackground(vs *driver.ViewStyle, hasGlass bool) bool {
	return vs != nil && vs.Background != 0 && chooseBackdrop(vs, hasGlass) == backdropFiltered
}

// backdropUpdate describes how a view's backdrop and background ownership change from prev to next.
type backdropUpdate struct {
	next backdropMode
	// recreate removes any existing backdrop view before applying next (leaving backdrop mode, or switching between backdrop kinds).
	recreate bool
	// handoffToOuter copies the tint's displayed background onto the outer layer before removing a tinted backdrop so an active background transition on the outer layer starts from the tint's color rather than clear.
	handoffToOuter bool
	// clearOuterAnim cancels any running backgroundColor transition on the outer layer when entering a tinted backdrop mode so it cannot paint behind it.
	clearOuterAnim bool
}

// planBackdropTransition computes the state transitions needed when a view's backdrop changes from prev to next under style vs.
func planBackdropTransition(prev, next backdropMode, vs *driver.ViewStyle, animatedBackground bool) backdropUpdate {
	u := backdropUpdate{next: next}
	if prev != next {
		u.recreate = prev != backdropNone
	}
	if prev.usesTint() && !next.usesTint() && !animatedBackground && transitionsBackground(vs) {
		u.handoffToOuter = true
	}
	if !prev.usesTint() && next.usesTint() {
		u.clearOuterAnim = true
	}
	return u
}

// transitionsBackground reports whether vs's transition animates Background.
func transitionsBackground(vs *driver.ViewStyle) bool {
	if vs == nil || vs.Transition.Duration <= 0 {
		return false
	}
	p := vs.Transition.Properties
	return p == 0 || p&driver.TransitionBackground != 0
}

// outerLayerStyle returns the style to apply to the outer UIView's CALayer. When a tinted backdrop is active (usesTint), the backdrop's tint layer draws and animates the background color above the backdrop, so the outer layer stays transparent and excludes background transitions to avoid obscuring or double-tinting the backdrop.
func outerLayerStyle(vs *driver.ViewStyle, usesTint bool) driver.ViewStyle {
	drawn := *vs
	if !usesTint {
		return drawn
	}
	drawn.Background = 0
	drawn.Transition = withoutBackgroundTransition(drawn.Transition)
	return drawn
}

// withoutBackgroundTransition returns tr with TransitionBackground excluded so other transitioning properties (opacity, transform, border, shadow) keep animating on the outer layer while background transitions stay on the tint.
func withoutBackgroundTransition(tr driver.Transition) driver.Transition {
	if tr.Duration <= 0 {
		return tr
	}
	p := tr.Properties
	if p == 0 {
		p = ^driver.TransitionProperty(0)
	}
	p &^= driver.TransitionBackground
	if p == 0 {
		return driver.Transition{}
	}
	tr.Properties = p
	return tr
}
