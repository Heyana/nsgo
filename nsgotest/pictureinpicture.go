package nsgotest

import (
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// PictureInPictureConfig records a custom Picture in Picture session.
type PictureInPictureConfig struct {
	Size          core.Size
	Paused        bool
	Duration      time.Duration // zero is a live stream
	Position      time.Duration
	Automatically bool
}

func pictureInPictureConfig(c driver.PictureInPictureConfig) PictureInPictureConfig {
	return PictureInPictureConfig{
		Size:          c.Size,
		Paused:        c.Paused,
		Duration:      c.Duration,
		Position:      c.Position,
		Automatically: c.Automatically,
	}
}
