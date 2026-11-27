//go:build (!darwin || ios) && !windows && (!linux || android)

package nsgotest

import (
	"image"

	"github.com/nsgo-dev/nsgo/core"
)

// PlatformTextEngine returns nil where no windowless native engine is available.
func PlatformTextEngine() core.TextEngine { return nil }

// RegisterFont reports ErrNoPlayer where native font registration is unavailable.
func RegisterFont([]byte) error { return ErrNoPlayer }

// Render reports ErrNoPlayer where no windowless canvas player is available.
func Render(*core.Drawing, core.Size, float64) (*image.RGBA, error) { return nil, ErrNoPlayer }

func canRenderDrawings() bool { return false }
