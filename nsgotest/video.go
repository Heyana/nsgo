package nsgotest

import (
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// VideoConfig records the source and playback settings of a mounted video.
type VideoConfig struct {
	Source               string
	Headers              *VideoHeaders
	Paused               bool
	Muted                bool
	Volume               float32
	Rate                 float32
	Repeat               bool
	ResizeMode           core.ResizeMode
	Controls             bool
	PictureInPicture     bool
	AutoPictureInPicture bool
	ProgressInterval     time.Duration
	AudioSource          string
	AudioSession         core.AudioSession
	// HitTestAlpha passes the pointer through video pixels below it in painted windows; 0 disables it.
	HitTestAlpha float32
}

func videoConfig(c driver.VideoConfig) VideoConfig {
	return VideoConfig{
		Source:               c.Source,
		Headers:              c.Headers,
		Paused:               c.Paused,
		Muted:                c.Muted,
		Volume:               c.Volume,
		Rate:                 c.Rate,
		Repeat:               c.Repeat,
		ResizeMode:           c.ResizeMode,
		Controls:             c.Controls,
		PictureInPicture:     c.PictureInPicture,
		AutoPictureInPicture: c.AutoPictureInPicture,
		ProgressInterval:     c.ProgressInterval,
		AudioSource:          c.AudioSource,
		AudioSession:         c.AudioSession,
		HitTestAlpha:         c.HitTestAlpha,
	}
}
