package nsgotest

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ViewStyle records the resolved look of one mounted view. Effects, custom
// cursors keep their immutable app-value identities.
type ViewStyle struct {
	Background core.Color
	// CornerRadius rounds every corner, unless Corners is set: then each
	// corner has its own radius. Platforms fit radii that do not fit
	// with Corners.Fit.
	CornerRadius core.Points
	Corners      core.Corners
	// ContinuousCorners eases the corners into the edges (RoundedRect)
	// instead of drawing quarter circles.
	ContinuousCorners bool
	// BorderWidth and BorderColor are the border on every side, unless
	// Borders is set: then each side has its width there and its color
	// in BorderColors.
	BorderWidth  core.Points
	BorderColor  core.Color
	Borders      core.Edges
	BorderColors core.EdgeColors
	// BorderStyle dashes or dots the border; 0 is solid.
	BorderStyle core.BorderStyle
	Outline     core.Outline
	Opacity     float32
	// Clip clips children to the bounds and corners.
	Clip         bool
	ShadowColor  core.Color
	ShadowOffset core.Point
	ShadowRadius core.Points
	// TintColor recolors an image's opaque pixels; 0 keeps its colors.
	TintColor core.Color
	// Cursor is the pointer's look over the view; Auto inherits the
	// nearest ancestor's. Pressables keep the platform's arrow.
	Cursor core.Cursor
	// CursorImage is an optional custom cursor image over the view; it
	// travels behind a pointer so ViewStyle stays comparable, and the core
	// keeps the same pointer while Equal.
	CursorImage *core.CustomCursor
	// Transform moves, scales and rotates the view around its transform
	// origin without moving its children's frames; hit testing follows
	// it. The core has resolved its percentages.
	Transform core.Transform
	// Origin is where the transform acts, from the view's center, and
	// OriginZ how far toward the viewer.
	Origin  core.Point
	OriginZ core.Points
	// BackfaceHidden hides the view while its rotations turn its back to
	// the viewer.
	BackfaceHidden bool
	// ChildPerspective, when set, is the depth children's rotations out
	// of the plane are seen from, at the view's center (ChildPerspective
	// the function).
	ChildPerspective core.Points
	BlendMode        core.BlendMode
	// Material and Glass record the view's backdrop presentation.
	Material core.BackdropMaterial
	// Elevation is Android's: how high the view floats, which casts the
	// system's shadow under it where the style casts none.
	Elevation core.Points
	Glass     core.GlassEffect
	// GlassInteractive lets Liquid Glass answer presses on platforms that support it (macOS 27 and iOS 26).
	GlassInteractive bool
	// Effects are shadows, gradients, filters and masks, nil without any.
	Effects *core.Effects
	// Transition, when it has a duration, animates the change from the
	// previous ViewStyle to this one.
	Transition core.Transition
	// Animation runs keyframes over the view while it is set; a change
	// starts it again.
	Animation core.KeyframeAnimation
	// Hidden hides the view and its children (display none, visibility
	// hidden).
	Hidden bool
}

func viewStyle(s driver.ViewStyle) ViewStyle {
	return ViewStyle{
		Background:        s.Background,
		CornerRadius:      s.CornerRadius,
		Corners:           s.Corners,
		ContinuousCorners: s.ContinuousCorners,
		BorderWidth:       s.BorderWidth,
		BorderColor:       s.BorderColor,
		Borders:           s.Borders,
		BorderColors:      s.BorderColors,
		BorderStyle:       s.BorderStyle,
		Outline:           s.Outline,
		Opacity:           s.Opacity,
		Clip:              s.Clip,
		ShadowColor:       s.ShadowColor,
		ShadowOffset:      s.ShadowOffset,
		ShadowRadius:      s.ShadowRadius,
		TintColor:         s.TintColor,
		Cursor:            s.Cursor,
		CursorImage:       s.CursorImage,
		Transform:         s.Transform,
		Origin:            s.Origin,
		OriginZ:           s.OriginZ,
		BackfaceHidden:    s.BackfaceHidden,
		ChildPerspective:  s.ChildPerspective,
		BlendMode:         s.BlendMode,
		Material:          s.Material,
		Elevation:         s.Elevation,
		Glass:             s.Glass,
		GlassInteractive:  s.GlassInteractive,
		Effects:           s.Effects,
		Transition:        s.Transition,
		Animation:         s.Animation,
		Hidden:            s.Hidden,
	}
}

func (s ViewStyle) driverStyle() driver.ViewStyle {
	return driver.ViewStyle{
		Background:        s.Background,
		CornerRadius:      s.CornerRadius,
		Corners:           s.Corners,
		ContinuousCorners: s.ContinuousCorners,
		BorderWidth:       s.BorderWidth,
		BorderColor:       s.BorderColor,
		Borders:           s.Borders,
		BorderColors:      s.BorderColors,
		BorderStyle:       s.BorderStyle,
		Outline:           s.Outline,
		Opacity:           s.Opacity,
		Clip:              s.Clip,
		ShadowColor:       s.ShadowColor,
		ShadowOffset:      s.ShadowOffset,
		ShadowRadius:      s.ShadowRadius,
		TintColor:         s.TintColor,
		Cursor:            s.Cursor,
		CursorImage:       s.CursorImage,
		Transform:         s.Transform,
		Origin:            s.Origin,
		OriginZ:           s.OriginZ,
		BackfaceHidden:    s.BackfaceHidden,
		ChildPerspective:  s.ChildPerspective,
		BlendMode:         s.BlendMode,
		Material:          s.Material,
		Elevation:         s.Elevation,
		Glass:             s.Glass,
		GlassInteractive:  s.GlassInteractive,
		Effects:           s.Effects,
		Transition:        s.Transition,
		Animation:         s.Animation,
		Hidden:            s.Hidden,
	}
}

// Radii returns each corner's radius fitted to size.
func (s *ViewStyle) Radii(size core.Size) core.Corners {
	native := s.driverStyle()
	return native.Radii(size)
}

// Rounded reports whether any corner is rounded.
func (s *ViewStyle) Rounded() bool {
	native := s.driverStyle()
	return native.Rounded()
}

// Rim returns the rounded border color and fill inset that avoid edge bleed.
func (s *ViewStyle) Rim() (rim core.Color, inset core.Points, ok bool) {
	native := s.driverStyle()
	return native.Rim()
}

// Matrix returns the view's transform around its origin for size.
func (s *ViewStyle) Matrix(size core.Size) core.Matrix3D {
	native := s.driverStyle()
	return native.Matrix(size)
}
