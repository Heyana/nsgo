package nsgotest

import (
	"maps"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ImageConfig records the source and appearance of a mounted image.
type ImageConfig struct {
	Source       core.ImageSource
	Placeholder  core.ImageSource
	ResizeMode   core.ResizeMode
	Sampling     core.ImageSampling
	BlurRadius   core.Points
	FadeDuration time.Duration
	HitTestAlpha float32
	// Position locates the fitted image within its view.
	Position core.ObjectPosition
}

func imageConfig(c driver.ImageConfig) ImageConfig {
	c.Source.Variants = maps.Clone(c.Source.Variants)
	c.Placeholder.Variants = maps.Clone(c.Placeholder.Variants)
	return ImageConfig{
		Source:       c.Source,
		Placeholder:  c.Placeholder,
		ResizeMode:   c.ResizeMode,
		Sampling:     c.Sampling,
		BlurRadius:   c.BlurRadius,
		FadeDuration: c.FadeDuration,
		HitTestAlpha: c.HitTestAlpha,
		Position:     c.Position,
	}
}

func (c ImageConfig) imageConfig() driver.ImageConfig {
	return driver.ImageConfig{
		Source:       c.Source,
		Placeholder:  c.Placeholder,
		ResizeMode:   c.ResizeMode,
		Sampling:     c.Sampling,
		BlurRadius:   c.BlurRadius,
		FadeDuration: c.FadeDuration,
		HitTestAlpha: c.HitTestAlpha,
		Position:     c.Position,
	}
}

// Same reports whether c and other show the same image with the same appearance.
func (c ImageConfig) Same(other ImageConfig) bool {
	return c.imageConfig().Same(other.imageConfig())
}
