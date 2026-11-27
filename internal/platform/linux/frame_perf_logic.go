package linux

import "github.com/nsgo-dev/nsgo/internal/driver"

// This file builds on every system, so unit tests and microbenchmarks check
// Linux's frame invalidation rules, path cache policy and incremental focus
// reconciliation on any host.

// viewInvalidation classifies what GTK work a view update needs:
//   - invalidateNone: nothing changed.
//   - invalidateDraw: only the view's own snapshot changed (queue_draw on self;
//     parent allocation stays intact).
//   - invalidateReposition: only the view's placement/transform in its parent
//     changed (queue_allocate on parent; the view's cached render node stays
//     intact unless BackfaceHidden or a canvas raster scale step requires a draw).
//   - invalidateBoth: both placement/visibility and the view's own drawing changed.
type viewInvalidation uint8

const (
	invalidateNone viewInvalidation = iota
	invalidateDraw
	invalidateReposition
	invalidateBoth
)

// classifyStyleInvalidation decides what GTK invalidation changing a view's
// style from old to next requires. It returns the invalidation for the view
// itself and whether its own children need re-allocation (ChildPerspective).
func classifyStyleInvalidation(old, next *driver.ViewStyle, hadTransition, hasTransition bool) (inv viewInvalidation, allocSelf bool) {
	allocSelf = old.ChildPerspective != next.ChildPerspective
	if !hadTransition && !hasTransition && *old == *next {
		return invalidateNone, false
	}
	// Visibility or active transitions/keyframes may alter both placement and paint.
	if old.Hidden != next.Hidden || hadTransition || hasTransition || old.Animation != next.Animation {
		return invalidateBoth, allocSelf
	}
	transformChanged := old.Transform != next.Transform || old.Origin != next.Origin || old.OriginZ != next.OriginZ
	paintChanged := old.Background != next.Background ||
		old.CornerRadius != next.CornerRadius ||
		old.Corners != next.Corners ||
		old.ContinuousCorners != next.ContinuousCorners ||
		old.BorderWidth != next.BorderWidth ||
		old.BorderColor != next.BorderColor ||
		old.Borders != next.Borders ||
		old.BorderColors != next.BorderColors ||
		old.BorderStyle != next.BorderStyle ||
		old.Outline != next.Outline ||
		old.Opacity != next.Opacity ||
		old.Clip != next.Clip ||
		old.ShadowColor != next.ShadowColor ||
		old.ShadowOffset != next.ShadowOffset ||
		old.ShadowRadius != next.ShadowRadius ||
		old.TintColor != next.TintColor ||
		old.BackfaceHidden != next.BackfaceHidden ||
		old.BlendMode != next.BlendMode ||
		old.Effects != next.Effects
	switch {
	case transformChanged && (paintChanged || next.BackfaceHidden):
		return invalidateBoth, allocSelf
	case transformChanged:
		return invalidateReposition, allocSelf
	case paintChanged:
		return invalidateDraw, allocSelf
	default:
		return invalidateNone, allocSelf
	}
}

// classifyAnimatedInvalidation decides what GTK invalidation changing a view's
// bound animated values from old to next requires, and whether the raster scale
// changed (which requires canvas redraw).
func classifyAnimatedInvalidation(old, next driver.AnimatedValues, backfaceHidden bool) (inv viewInvalidation, scaleChanged bool) {
	if old == next {
		return invalidateNone, false
	}
	changedBits := old.Set ^ next.Set
	transformChanged := changedBits&driver.AnimatedTransformMask != 0 ||
		(old.Set&next.Set)&driver.AnimatedTransformMask != 0 &&
			(old.TranslateX != next.TranslateX || old.TranslateY != next.TranslateY ||
				old.Scale != next.Scale || old.ScaleX != next.ScaleX || old.ScaleY != next.ScaleY ||
				old.Rotate != next.Rotate || old.RotateX != next.RotateX || old.RotateY != next.RotateY)
	const scaleMask = 1<<driver.PropScale | 1<<driver.PropScaleX | 1<<driver.PropScaleY
	scaleChanged = changedBits&scaleMask != 0 ||
		(old.Set&next.Set)&scaleMask != 0 && (old.Scale != next.Scale || old.ScaleX != next.ScaleX || old.ScaleY != next.ScaleY)
	paintMask := ^driver.AnimatedTransformMask
	paintChanged := changedBits&paintMask != 0 ||
		(old.Set&next.Set)&paintMask != 0 &&
			(old.Opacity != next.Opacity || old.Background != next.Background ||
				old.BorderColor != next.BorderColor || old.ShadowColor != next.ShadowColor ||
				old.ShadowRadius != next.ShadowRadius || old.ShadowOpacity != next.ShadowOpacity ||
				old.TextColor != next.TextColor)
	switch {
	case transformChanged && (paintChanged || backfaceHidden):
		return invalidateBoth, scaleChanged
	case transformChanged:
		return invalidateReposition, scaleChanged
	case paintChanged:
		return invalidateDraw, scaleChanged
	default:
		return invalidateNone, scaleChanged
	}
}

// classifyFrameInvalidation decides what GTK invalidation changing a view's
// shown frame from oldFrame to newFrame requires.
func classifyFrameInvalidation(oldFrame, newFrame driver.Bounds) viewInvalidation {
	if oldFrame == newFrame {
		return invalidateNone
	}
	if oldFrame.Size != newFrame.Size {
		return invalidateBoth
	}
	return invalidateReposition
}

// styleHasTransform reports whether style applies any non-identity transform
// beyond its frame origin.
func styleHasTransform(s *driver.ViewStyle) bool {
	return s.Transform != (driver.Transform{}) || s.Animation.Keyframes != 0
}

// pathCacheKey identifies a rounded rectangle GskPath so GSK can reuse its
// GPU path raster cache across frames and widget moves.
type pathCacheKey struct {
	x, y, w, h float32
	radii      [4][2]float32
	continuous bool
}

const maxCachedPaths = 128

// pathCacheEntry is one cached path in bounded LRU order.
type pathCacheEntry struct {
	key  pathCacheKey
	path uintptr
}

// boundedPathCache keeps up to maxCachedPaths GskPath handles, evicting the
// least recently used entry with release when full.
type boundedPathCache struct {
	entries []pathCacheEntry
}

func (c *boundedPathCache) get(key pathCacheKey) (uintptr, bool) {
	for i := range c.entries {
		if c.entries[i].key == key {
			if i > 0 {
				e := c.entries[i]
				copy(c.entries[1:i+1], c.entries[0:i])
				c.entries[0] = e
			}
			return c.entries[0].path, true
		}
	}
	return 0, false
}

func (c *boundedPathCache) put(key pathCacheKey, path uintptr, release func(uintptr)) {
	if len(c.entries) >= maxCachedPaths {
		last := c.entries[len(c.entries)-1]
		if release != nil && last.path != 0 {
			release(last.path)
		}
		copy(c.entries[1:], c.entries[:len(c.entries)-1])
		c.entries[0] = pathCacheEntry{key: key, path: path}
		return
	}
	c.entries = append(c.entries, pathCacheEntry{})
	copy(c.entries[1:], c.entries[:len(c.entries)-1])
	c.entries[0] = pathCacheEntry{key: key, path: path}
}

func (c *boundedPathCache) clear(release func(uintptr)) {
	if release != nil {
		for _, e := range c.entries {
			if e.path != 0 {
				release(e.path)
			}
		}
	}
	c.entries = c.entries[:0]
}
