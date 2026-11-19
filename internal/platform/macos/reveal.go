//go:build darwin && !ios

package macos

import (
	"os"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// revealDuration is how long a panel takes to grow into its open size.
const revealDuration = 0.18

// pendingReveal is an expansion whose window already has its open size, and
// whose mask waits for the content laid out for that size before it starts
// to move: growing the window with its content still the strip's shows an
// empty panel for a frame, which reads as the window flashing in first.
type pendingReveal struct {
	mask   objc.ID
	layer  objc.ID
	panelW float64
	startW float64
	endW   float64
	h      float64
	start  *time.Timer
}

// revealFrame runs a panel's frame change as a reveal of its content rather
// than as an AppKit window resize. A window resize re-measures every view on
// each animation frame: a panel with a list of rows pays that on all of
// them, and the first frames show nothing laid out yet. Here the window
// takes its final size at once and a mask over its content view grows from
// the edge it is anchored to, so the rows are laid out once and the whole
// movement is composited by the window server. A collapse runs the same
// mask the other way and hands the window its strip size at the end.
//
// It answers false when the change is too small to bother or the surface is
// not the app's own panel, and the caller falls back to AppKit's animation.
func (s *surface) revealFrame(w objc.ID, to apple.CGRect) bool {
	if os.Getenv("NSGO_PANEL_REVEAL") == "0" {
		return false
	}
	if s.window == 0 || s.embedded || s.native == 0 {
		return false
	}
	from := objc.Send[apple.CGRect](w, sel("frame"))
	fromW, toW := from.Size.Width, to.Size.Width
	if toW <= fromW+2 && toW >= fromW-2 {
		return false
	}
	cv := w.Send(sel("contentView"))
	if cv == 0 {
		return false
	}
	cv.Send(selSetWantsLayer, true)
	layer := cv.Send(selLayer)
	if layer == 0 {
		return false
	}

	growing := toW > fromW
	// The mask starts at the width the panel had and ends at the width it
	// has: right-aligned either way, since the panel is anchored to the edge.
	startW, endW := fromW, toW
	h := to.Size.Height
	mask := objc.ID(apple.Class("CALayer")).Send(selNew)
	mask.Send(sel("setBackgroundColor:"),
		objc.ID(apple.Class("NSColor")).Send(sel("blackColor")).Send(sel("CGColor")))
	setMaskFrame(mask, toW, startW, h)
	layer.Send(sel("setMask:"), mask)

	// growing: the window is already the full panel, so the content lays out
	// once at its final size and the mask only uncovers it from the right.
	if growing {
		// display:false: the window takes its size without painting first,
		// so the first thing drawn for it is the app's content at that size.
		apple.WithoutAnimation(func() {
			w.Send(sel("setFrame:display:animate:"), to, false, false)
			s.applyPanelChrome(w)
		})
		// The mask moves when that content has been laid out, not now: see
		// startPendingReveal. A timer is the fallback for an expansion that
		// brings no new batch.
		pr := &pendingReveal{mask: mask, layer: layer, panelW: toW, startW: startW, endW: endW, h: h}
		pr.start = time.AfterFunc(80*time.Millisecond, func() { apple.OnMain(s.startPendingReveal) })
		s.reveal = pr
		return true
	}
	// Collapsing: the window keeps its width while the mask narrows, and
	// takes the strip size once the content is out of sight.
	animateMask(mask, toW, startW, endW, h)
	time.AfterFunc(time.Duration(revealDuration*float64(time.Second)), func() {
		apple.OnMain(func() {
			layer.Send(sel("setMask:"), objc.ID(0))
			w.Send(sel("setFrame:display:animate:"), to, true, false)
			s.applyPanelChrome(w)
			mask.Send(selRelease)
		})
	})
	return true
}

// startPendingReveal moves the mask of the expansion waiting, once the
// content for the new size has been applied. Nothing to do when no
// expansion waits: a collapse starts its own mask at once.
func (s *surface) startPendingReveal() {
	pr := s.reveal
	if pr == nil {
		return
	}
	s.reveal = nil
	if pr.start != nil {
		pr.start.Stop()
	}
	animateMask(pr.mask, pr.panelW, pr.startW, pr.endW, pr.h)
	time.AfterFunc(time.Duration(revealDuration*float64(time.Second)), func() {
		apple.OnMain(func() {
			pr.layer.Send(sel("setMask:"), objc.ID(0))
			pr.mask.Send(selRelease)
		})
	})
}

// setMaskFrame places the reveal mask: h tall, width wide, flush to the
// content view's right edge, in its own coordinates.
func setMaskFrame(mask objc.ID, panelW, width, h float64) {
	if width < 1 {
		width = 1
	}
	mask.Send(selSetFrame, apple.Rect(panelW-width, 0, width, h))
}

// animateMask takes the mask from startW to endW with the panel's easing,
// leaving the model on the end value so nothing snaps when the animation
// is removed.
func animateMask(mask objc.ID, panelW, startW, endW, h float64) {
	if endW < 1 {
		endW = 1
	}
	setMaskFrame(mask, panelW, endW, h)
	basic := objc.ID(apple.Class("CABasicAnimation"))
	curve := objc.ID(apple.Class("CAMediaTimingFunction")).Send(sel("functionWithName:"), apple.NSString("easeOut"))
	dur := objc.ID(apple.Class("NSNumber")).Send(sel("numberWithDouble:"), revealDuration)
	number := objc.ID(apple.Class("NSNumber"))

	width := basic.Send(sel("animationWithKeyPath:"), apple.NSString("bounds.size.width"))
	width.Send(sel("setFromValue:"), number.Send(sel("numberWithDouble:"), startW))
	width.Send(sel("setToValue:"), number.Send(sel("numberWithDouble:"), endW))
	width.Send(sel("setDuration:"), dur)
	width.Send(sel("setTimingFunction:"), curve)
	mask.Send(sel("addAnimation:forKey:"), width, apple.NSString("nsgoRevealWidth"))

	position := basic.Send(sel("animationWithKeyPath:"), apple.NSString("position.x"))
	position.Send(sel("setFromValue:"), number.Send(sel("numberWithDouble:"), panelW-startW/2))
	position.Send(sel("setToValue:"), number.Send(sel("numberWithDouble:"), panelW-endW/2))
	position.Send(sel("setDuration:"), dur)
	position.Send(sel("setTimingFunction:"), curve)
	mask.Send(sel("addAnimation:forKey:"), position, apple.NSString("nsgoRevealX"))
}

// applyPanelChrome fits the panel's frame chrome to its current width: the
// corner radius, and the title style that clips the window into it.
func (s *surface) applyPanelChrome(w objc.ID) {
	frame := objc.Send[apple.CGRect](w, sel("frame"))
	content := objc.Send[apple.CGRect](w, sel("contentRectForFrameRect:"), frame).Size
	width := driver.Points(content.Width)
	s.native.Send(selLayer).Send(sel("setCornerRadius:"), panelCornerRadius(width))
	styleMask := uint(styleNonactivating | maskFullSize)
	if width >= 2*panelRadius {
		styleMask |= styleTitled
	}
	if cur := objc.Send[uint](w, sel("styleMask")); cur != styleMask {
		w.Send(sel("setStyleMask:"), styleMask)
		w.Send(sel("setHasShadow:"), true)
		if styleMask&styleTitled != 0 {
			w.Send(sel("setTitlebarAppearsTransparent:"), true)
			w.Send(sel("setTitleVisibility:"), titleHidden)
		}
	}
}
