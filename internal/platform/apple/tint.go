//go:build darwin

package apple

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// BackdropTint tracks and paints a view's background color on its backdrop's tint layer (macOS's tint view or iOS's UIVisualEffectView contentView), following background transitions and animated background values while leaving an active tint transition alone on frames that animate other properties.
type BackdropTint struct {
	color  driver.Color
	styled bool
}

// Reset forgets the tint's styled state after the backdrop is removed or rebuilt, so the next style paints immediately without starting a transition.
func (t *BackdropTint) Reset() {
	t.color, t.styled = 0, false
}

// Style updates layer with vs.Background, with the same transition and presentation-value retargeting as a view's own layer. Only color changes touch layer, so frames animating other properties leave an active tint transition alone. When animatedBackground is true, the bound background value takes over immediately and cancels any running background transition.
func (t *BackdropTint) Style(layer objc.ID, vs *driver.ViewStyle, animatedBackground bool) {
	if layer == 0 {
		return
	}
	if animatedBackground {
		layer.Send(selRemoveAnimation, NSString("nsgo.backgroundColor"))
	}
	if t.styled && t.color == vs.Background {
		return
	}
	next := driver.ViewStyle{Background: vs.Background, Opacity: 1}
	tr := vs.Transition
	if t.styled && !animatedBackground &&
		(tr.Properties == 0 || tr.Properties&driver.TransitionBackground != 0) {
		next.Transition = tr
		next.Transition.Properties = driver.TransitionBackground
	}
	if next.Transition.Duration <= 0 {
		layer.Send(selRemoveAnimation, NSString("nsgo.backgroundColor"))
	}
	WithoutAnimation(func() { StyleLayer(layer, nil, &next, 0) })
	t.color, t.styled = vs.Background, true
}

// HandoffBackground copies from's currently displayed background color (presentation color while transitioning, else model color) onto to without animation so a background transition on to starts from what the tint showed rather than from transparent.
func HandoffBackground(from, to objc.ID) {
	if from == 0 || to == 0 {
		return
	}
	shown := from.Send(sel("presentationLayer"))
	if shown == 0 {
		shown = from
	}
	color := shown.Send(selValueForKey, NSString("backgroundColor"))
	if color != 0 {
		color = color.Send(sel("retain"))
		defer color.Send(selRelease)
	}
	WithoutAnimation(func() {
		to.Send(selRemoveAnimation, NSString("nsgo.backgroundColor"))
		to.Send(selSetBackgroundColor, uintptr(color))
	})
}

// ClearBackgroundAnimation cancels any running background color transition on layer.
func ClearBackgroundAnimation(layer objc.ID) {
	if layer != 0 {
		layer.Send(selRemoveAnimation, NSString("nsgo.backgroundColor"))
	}
}
