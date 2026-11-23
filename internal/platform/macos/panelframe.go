//go:build darwin && !ios

package macos

import (
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

const panelRadius = 12.0
const panelTitleStyle = styleTitled | maskFullSize

func panelRounded(size apple.CGSize) bool {
	return size.Width >= 2*panelRadius && size.Height >= 2*panelRadius
}

func setPanelTitlebar(w objc.ID) {
	w.Send(sel("setTitlebarAppearsTransparent:"), true)
	w.Send(sel("setTitleVisibility:"), titleHidden)
	for i := range 3 {
		if b := w.Send(selStandardButton, i); b != 0 {
			b.Send(selSetHidden, true)
		}
	}
}

// panelVisible includes covered panels, whose frame clock can still settle
// via its existing stall timer, but not panels hidden along with the app.
func (s *surface) panelVisible() bool {
	return objc.Send[bool](s.window, sel("isVisible")) &&
		!objc.Send[bool](s.window, sel("isMiniaturized")) && !objc.Send[bool](nsApp(), sel("isHidden"))
}

func (s *surface) setPanelFrame(frame apple.CGRect, animated bool, now time.Time) {
	if animated && s.panelVisible() && !s.motionReduced() {
		bounds := func(r apple.CGRect) driver.Bounds {
			return driver.Bounds{Origin: driver.Point{X: driver.Points(r.Origin.X), Y: driver.Points(r.Origin.Y)},
				Size: driver.Size{Width: driver.Points(r.Size.Width), Height: driver.Points(r.Size.Height)}}
		}
		if s.panelMotion.Start(bounds(objc.Send[apple.CGRect](s.window, sel("frame"))), bounds(frame), now) {
			s.startTicking()
			return
		}
	}
	s.panelMotion.Stop()
	s.applyPanelFrame(frame)
}

func (s *surface) stepPanelFrame(now time.Time) {
	if !s.panelMotion.Active() {
		return
	}
	if !s.panelVisible() {
		s.panelMotion.Stop()
		s.repickPanel()
		return
	}
	if frame, ok := s.panelMotion.Step(now, s.motionReduced()); ok {
		s.applyPanelFrame(apple.Rect(float64(frame.Origin.X), float64(frame.Origin.Y), float64(frame.Size.Width), float64(frame.Size.Height)))
	}
}

// applyPanelFrame applies style and geometry together. AppKit can rebuild
// the frame view and resize the content when the style changes; don't lay
// out or pick against these intermediate geometries.
func (s *surface) applyPanelFrame(frame apple.CGRect) {
	w := s.window
	current := objc.Send[apple.CGRect](w, sel("frame"))
	if frame == current {
		return
	}
	if frame.Size == current.Size {
		w.Send(selSetFrameOrigin, frame.Origin)
		s.pointer.tested = false
		s.queuePanelRepick()
		return
	}
	before := objc.Send[apple.CGRect](s.native, selBounds).Size
	s.settingPanelFrame = true
	var key bool
	var responder objc.ID
	if !s.transparent {
		mask := objc.Send[uint](w, selStyleMask)
		style := mask &^ panelTitleStyle
		if panelRounded(frame.Size) {
			style |= panelTitleStyle
		}
		if style != mask {
			key = objc.Send[bool](w, sel("isKeyWindow"))
			responder = w.Send(sel("firstResponder")).Send(sel("retain"))
			radius := 0.0
			if style&styleTitled != 0 {
				// Grow the borderless frame before asking AppKit to title it.
				w.Send(sel("setFrame:display:"), frame, false)
				w.Send(sel("setStyleMask:"), style)
				setPanelTitlebar(w)
				radius = panelRadius
			} else {
				// Remove the titled frame before it can clamp the smaller size.
				w.Send(sel("setStyleMask:"), style)
			}
			apple.WithoutAnimation(func() {
				s.native.Send(selLayer).Send(selSetCornerRadius, radius)
			})
		}
	}
	w.Send(sel("setFrame:display:"), frame, false)
	if key && !objc.Send[bool](w, sel("isKeyWindow")) {
		w.Send(sel("makeKeyWindow"))
	}
	if responder != 0 {
		if responder != w.Send(sel("firstResponder")) {
			w.Send(sel("makeFirstResponder:"), responder)
		}
		responder.Send(selRelease)
	}
	s.settingPanelFrame = false
	s.pointer.tested = false
	s.queuePanelRepick()
	if size := objc.Send[apple.CGRect](s.native, selBounds).Size; size != before && s.engine != nil {
		s.resize(driver.Size{Width: driver.Points(size.Width), Height: driver.Points(size.Height)})
	}
	if !s.closed {
		w.Send(sel("displayIfNeeded"))
		s.refresh()
	}
}

// The WindowServer's hit test can still see the old frame when AppKit
// returns from setFrame:. Pick once the display presents the change,
// including the last frame, which ends the motion.
func (s *surface) queuePanelRepick() {
	if !s.panelVisible() {
		return
	}
	s.panelRepickAfter = mediaTime()
	if !s.ticking || s.pace != 0 {
		s.startTicking()
	}
}

// Don't hold the clock open for a pointer over a transparent native corner.
const panelHitGrace = 100 * time.Millisecond

func (s *surface) pickPresentedPanel(linked bool) {
	if s.panelRepickAfter == 0 {
		return
	}
	now := mediaTime()
	var presented time.Duration
	if linked && s.link != 0 {
		// A tick queued before the resize can run just after it. The
		// link's timestamp is when the previous frame was displayed.
		presented = time.Duration(objc.Send[float64](s.link, selTimestamp) * float64(time.Second))
	} else {
		presented = now - time.Second/60
	}
	if presented >= s.panelRepickAfter &&
		s.pickPanel(s.windowMouseLocation(), now-s.panelRepickAfter < panelHitGrace) {
		s.panelRepickAfter = 0
	}
}

// repickPanel waits for a batch's layout when it also changed the frame.
func (s *surface) repickPanel() {
	if s.settingPanelFrame || s.closed {
		return
	}
	if s.applying {
		s.panelRepick = true
		return
	}
	s.pickHere()
}

// pickPanel reports whether native hit testing settled yet. The bounded wait
// is only for frame changes, never for a pointer event.
func (s *surface) pickPanel(at apple.CGPoint, settling bool) bool {
	up := s.native.Send(selSuperview)
	if up == 0 {
		return true
	}
	var hit objc.ID
	if s.panelVisible() {
		var tested bool
		if s.pointer.mode == driver.WindowPointerPainted {
			hit, tested = s.passUnpaintedAt(at)
		}
		if s.pointer.ignoring {
			hit = 0
		} else if !tested {
			hit = s.hitAt(at)
		}
	}
	if hit != 0 {
		// Use the same shape and z-order as mouse-down, including
		// transparent windows that the pointer passes through.
		point := objc.Send[apple.CGPoint](s.window, sel("convertPointToScreen:"), at)
		window := objc.ID(apple.Class("NSWindow"))
		atPoint := sel("windowNumberAtPoint:belowWindowWithWindowNumber:")
		number := objc.Send[int](s.window, sel("windowNumber"))
		front := objc.Send[int](window, atPoint, point, 0)
		if front != number {
			// If the hit is already below this panel, no window covers
			// it. The server may still be publishing the panel's region.
			if settling && objc.Send[int](window, atPoint, point, number) == front {
				return false
			}
			hit = 0
		}
	}
	s.pickHit(hit, up, at, false, false)
	return true
}
