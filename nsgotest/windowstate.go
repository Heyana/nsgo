package nsgotest

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// WindowState is the native window containing a surface, sampled on its UI
// thread. Frame uses the desktop screen points Window.ShowAt takes; Move is
// false when the platform cannot report its position, as on Wayland.
type WindowState struct {
	Frame            core.WindowFrame
	Visible, Focused bool
}

// WindowState reads the native window after settling. It skips headless tests
// and targets without native window observation, rather than inferring a
// window's position or focus from the commands the app sent.
func (s *Surface) WindowState() WindowState {
	s.app.t.Helper()
	m := s.open()
	if m.target == nil {
		s.app.t.Skip("nsgotest: WindowState needs a native desktop window")
		return WindowState{}
	}
	observer, ok := m.target.surface.(testhost.WindowStateSurface)
	if !ok {
		s.app.t.Skip("nsgotest: this target does not observe native window state")
		return WindowState{}
	}
	state, err := observer.WindowState()
	if err != nil {
		s.app.t.Fatalf("nsgotest: surface %q: window state: %v", s.title(), err)
	}
	return WindowState{Frame: state.Frame, Visible: state.Visible, Focused: state.Focused}
}
