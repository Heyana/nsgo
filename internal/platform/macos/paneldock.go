//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func (s *surface) dockOrigin(size apple.CGSize) (apple.CGPoint, bool) {
	frame, ok := driver.DockWindow(driver.Size{Width: driver.Points(size.Width), Height: driver.Points(size.Height)}, displays(), s.dock)
	screens := screenFrames("frame")
	if !ok || len(screens) == 0 {
		return apple.CGPoint{}, false
	}
	main := screens[0]
	return apple.CGPoint{X: main.Origin.X + float64(frame.X), Y: main.Origin.Y + main.Size.Height - float64(frame.Y) - size.Height}, true
}

func (s *surface) placeDockedPanel() bool {
	frame := objc.Send[apple.CGRect](s.window, sel("frame"))
	origin, ok := s.dockOrigin(frame.Size)
	if !ok {
		s.dockFailed()
		return false
	}
	if frame.Origin != origin {
		s.window.Send(sel("setFrameOrigin:"), origin)
	}
	return true
}

func (s *surface) dockFailed() {
	s.window.Send(sel("orderOut:"), objc.ID(0))
	if s.engine != nil {
		s.engine.Dispatch(driver.Event{Kind: driver.EventWindowPlacementFailed, Text: "the docked panel's display is unavailable"})
	}
}
