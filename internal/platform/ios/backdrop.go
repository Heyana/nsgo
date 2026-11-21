package ios

import "github.com/nsgo-dev/nsgo/internal/driver"

// This file builds on every system, so tests check the backdrop mode and material mapping on any host.

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
