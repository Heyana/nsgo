package nsgotest

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ModalConfig records how a mounted modal appears and closes.
type ModalConfig struct {
	Animation core.ModalAnimation
	// Presentation, NoCloseOnSwipe and Orientations configure iOS presentation.
	Presentation   core.ModalPresentation
	NoCloseOnSwipe bool
	Orientations   core.Orientation
	// NoCloseOnEscape and NoCloseOnBackdropPress describe the app's close policy.
	// StatusBarTranslucent configures the Android status bar.
	NoCloseOnEscape, NoCloseOnBackdropPress bool
	StatusBarTranslucent                    bool
}

func modalConfig(c driver.ModalConfig) ModalConfig {
	return ModalConfig{
		Animation:              c.Animation,
		Presentation:           c.Presentation,
		NoCloseOnSwipe:         c.NoCloseOnSwipe,
		Orientations:           c.Orientations,
		NoCloseOnEscape:        c.NoCloseOnEscape,
		NoCloseOnBackdropPress: c.NoCloseOnBackdropPress,
		StatusBarTranslucent:   c.StatusBarTranslucent,
	}
}
