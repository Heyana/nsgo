package nsgotest

import (
	"slices"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ScrollConfig records the behavior and edge effects of a mounted scroll view.
type ScrollConfig struct {
	Horizontal    bool
	HideIndicator bool
	Behavior      core.ScrollBehavior
	// HeaderInset is the part of Behavior.ContentInset's top that a
	// transparent or large system header covers, which a large title
	// collapses into as the content scrolls under it.
	HeaderInset core.Points
	// Refreshable enables pull-to-refresh. Refreshing records whether
	// the app keeps the spinner visible.
	Refreshable bool
	Refreshing  bool
	// MinZoomScale and MaxZoomScale bound zooming. ZoomScale is the
	// scale the app requests.
	MinZoomScale, MaxZoomScale float32
	ZoomScale                  float32
	// NoBouncesZoom stops pinches at the limits. NoPinchZoom disables
	// pinch gestures. CenterContent centers smaller content.
	NoBouncesZoom bool
	NoPinchZoom   bool
	CenterContent bool
	// Edges records top, right, bottom and left effects in that order.
	Edges [4]ScrollEdge
}

func scrollConfig(c driver.ScrollConfig) ScrollConfig {
	c.Behavior.SnapToOffsets = slices.Clone(c.Behavior.SnapToOffsets)
	for i := range c.Edges {
		c.Edges[i].Bars = slices.Clone(c.Edges[i].Bars)
	}
	return ScrollConfig{
		Horizontal:    c.Horizontal,
		HideIndicator: c.HideIndicator,
		Behavior:      c.Behavior,
		HeaderInset:   c.HeaderInset,
		Refreshable:   c.Refreshable,
		Refreshing:    c.Refreshing,
		MinZoomScale:  c.MinZoomScale,
		MaxZoomScale:  c.MaxZoomScale,
		ZoomScale:     c.ZoomScale,
		NoBouncesZoom: c.NoBouncesZoom,
		NoPinchZoom:   c.NoPinchZoom,
		CenterContent: c.CenterContent,
		Edges:         c.Edges,
	}
}

func (c *ScrollConfig) scrollConfig() *driver.ScrollConfig {
	return &driver.ScrollConfig{
		Horizontal:    c.Horizontal,
		HideIndicator: c.HideIndicator,
		Behavior:      c.Behavior,
		HeaderInset:   c.HeaderInset,
		Refreshable:   c.Refreshable,
		Refreshing:    c.Refreshing,
		MinZoomScale:  c.MinZoomScale,
		MaxZoomScale:  c.MaxZoomScale,
		ZoomScale:     c.ZoomScale,
		NoBouncesZoom: c.NoBouncesZoom,
		NoPinchZoom:   c.NoPinchZoom,
		CenterContent: c.CenterContent,
		Edges:         c.Edges,
	}
}

// Zooms reports whether the configured zoom range allows zooming.
func (c *ScrollConfig) Zooms() bool {
	return c.scrollConfig().Zooms()
}

// ClampZoom returns scale within the configured zoom range.
func (c *ScrollConfig) ClampZoom(scale float32) float32 {
	return c.scrollConfig().ClampZoom(scale)
}

// Equal reports whether c and other configure scrolling identically.
func (c *ScrollConfig) Equal(other *ScrollConfig) bool {
	return c.scrollConfig().Equal(other.scrollConfig())
}
