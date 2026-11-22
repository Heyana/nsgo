package testhost

import "github.com/nsgo-dev/nsgo/internal/driver"

// WindowState is a snapshot read from the native window on its UI thread. Frame uses desktop screen points; Move is false when its position is unknown.
type WindowState struct {
	Frame            driver.WindowFrame
	Visible, Focused bool
}

// WindowStateSurface observes native state rather than the app's commands.
type WindowStateSurface interface {
	WindowState() (WindowState, error)
}
