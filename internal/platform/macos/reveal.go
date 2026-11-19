//go:build darwin && !ios

package macos

import (
	"fmt"
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
	window objc.ID
	panelW float64
	panelH float64
	startW float64
	startH float64
	endW   float64
	endH   float64
	start  *time.Timer
	at     time.Time
}

// kick pushes the start of the reveal back by one quiet interval: batches
// that land while the panel is still laying out for its new size keep
// arriving, and moving the mask over a half-drawn panel is exactly the
// content that keeps appearing after the window does.
func (p *pendingReveal) kick(s *surface) {
	if p.start != nil {
		p.start.Stop()
	}
	p.start = time.AfterFunc(40*time.Millisecond, func() { apple.OnMain(s.startPendingReveal) })
}

// hideChrome takes the window's own surface out of the reveal: a titled
// window's frame draws a background and a shadow across the whole frame at
// once, outside the app's content, so growing the frame flashes a full-size
// window with nothing in it. The panel draws its own card, so nothing is
// lost while these are away; they come back with the content.
func (p *pendingReveal) hideChrome() {
	w := p.window
	if w == 0 {
		return
	}
	w.Send(selSetOpaque, false)
	w.Send(sel("setBackgroundColor:"), objc.ID(apple.Class("NSColor")).Send(selClearColor))
	w.Send(sel("setHasShadow:"), false)
}

// showChrome puts the window's own surface back, at the panel's open size.
func (p *pendingReveal) showChrome() {
	w := p.window
	if w == 0 {
		return
	}
	w.Send(sel("setHasShadow:"), true)
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
	startH, endH := from.Size.Height, to.Size.Height
	mask := objc.ID(apple.Class("CALayer")).Send(selNew)
	mask.Send(sel("setBackgroundColor:"),
		objc.ID(apple.Class("NSColor")).Send(sel("blackColor")).Send(sel("CGColor")))
	setMaskFrame(mask, toW, to.Size.Height, startW, startH)
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
		pr := &pendingReveal{mask: mask, layer: layer, window: w, panelW: toW, panelH: to.Size.Height,
			startW: startW, startH: startH, endW: endW, endH: endH, at: time.Now()}
		pr.hideChrome()
		pr.kick(s)
		s.reveal = pr
		return true
	}
	// Collapsing: the window keeps its width while the mask narrows, and
	// takes the strip size once the content is out of sight.
	animateMask(mask, toW, to.Size.Height, startW, startH, endW, endH)
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
	if os.Getenv("NSGO_PANEL_DEBUG") != "" {
		fmt.Printf("nsgo-panel-reveal: content settled after %v\n", time.Since(pr.at).Round(time.Millisecond))
	}
	animateMask(pr.mask, pr.panelW, pr.panelH, pr.startW, pr.startH, pr.endW, pr.endH)
	time.AfterFunc(time.Duration(revealDuration*float64(time.Second)), func() {
		apple.OnMain(func() {
			pr.layer.Send(sel("setMask:"), objc.ID(0))
			pr.showChrome()
			pr.mask.Send(selRelease)
		})
	})
}

// setMaskFrame places the reveal mask: height tall, width wide, flush to the
// content view's right edge and centred vertically, in its own coordinates.
// The panel grows from the edge outwards and from its middle upwards and
// downwards, which is how the strip turns into the panel.
func setMaskFrame(mask objc.ID, panelW, panelH, width, height float64) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	mask.Send(selSetFrame, apple.Rect(panelW-width, (panelH-height)/2, width, height))
}

// animateMask takes the mask from the strip's box to the panel's, easing
// both axes. The model stays on the end value so nothing snaps when the
// animation is removed.
func animateMask(mask objc.ID, panelW, panelH, startW, startH, endW, endH float64) {
	if endW < 1 {
		endW = 1
	}
	if endH < 1 {
		endH = 1
	}
	setMaskFrame(mask, panelW, panelH, endW, endH)
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

	height := basic.Send(sel("animationWithKeyPath:"), apple.NSString("bounds.size.height"))
	height.Send(sel("setFromValue:"), number.Send(sel("numberWithDouble:"), startH))
	height.Send(sel("setToValue:"), number.Send(sel("numberWithDouble:"), endH))
	height.Send(sel("setDuration:"), dur)
	height.Send(sel("setTimingFunction:"), curve)
	mask.Send(sel("addAnimation:forKey:"), height, apple.NSString("nsgoRevealHeight"))

	position := basic.Send(sel("animationWithKeyPath:"), apple.NSString("position.x"))
	position.Send(sel("setFromValue:"), number.Send(sel("numberWithDouble:"), panelW-startW/2))
	position.Send(sel("setToValue:"), number.Send(sel("numberWithDouble:"), panelW-endW/2))
	position.Send(sel("setDuration:"), dur)
	position.Send(sel("setTimingFunction:"), curve)
	mask.Send(sel("addAnimation:forKey:"), position, apple.NSString("nsgoRevealX"))

	positionY := basic.Send(sel("animationWithKeyPath:"), apple.NSString("position.y"))
	positionY.Send(sel("setFromValue:"), number.Send(sel("numberWithDouble:"), panelH/2))
	positionY.Send(sel("setToValue:"), number.Send(sel("numberWithDouble:"), panelH/2))
	positionY.Send(sel("setDuration:"), dur)
	positionY.Send(sel("setTimingFunction:"), curve)
	mask.Send(sel("addAnimation:forKey:"), positionY, apple.NSString("nsgoRevealY"))
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
