//go:build darwin

package apple

import (
	"math"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// NeedsClipHolder reports whether a view that clips also draws outside its box: an outer shadow, a React Native shadow or an outline. Core Animation clips those with the children when a layer sets masksToBounds or a mask, so the backends clip such a view's children in a holder view of their own and leave the styled layer unclipped, as CSS keeps a box's own shadows and outline outside its overflow clip.
func NeedsClipHolder(vs *driver.ViewStyle) bool {
	if vs == nil || !vs.Clip {
		return false
	}
	if vs.Outline.Visible() || vs.ShadowColor&0xff != 0 {
		return true
	}
	if vs.Effects != nil {
		for _, sh := range vs.Effects.Shadows {
			if !sh.Inset && sh.Color&0xff != 0 {
				return true
			}
		}
	}
	return false
}

// ShapedCorners reports whether vs's rounded corners differ in radius, which a layer's own corner radius cannot draw and a shape mask does.
func ShapedCorners(vs *driver.ViewStyle) bool {
	// Fitting to a box scales every radius alike, so an unbounded box answers for all.
	return needsShape(vs, vs.Radii(driver.Size{Width: math.MaxFloat32, Height: math.MaxFloat32}))
}

// holderMasks are the shape masks ClipHolder gave holders whose corners differ, which they keep.
var holderMasks = map[objc.ID]objc.ID{}

// ClipHolder clips layer, the layer of the view holding a view's children, to the box and corners of vs, the view's style; nil or a style that does not clip leaves the children unclipped, or clipped to the holder's bounds for a scroll view's container (scrolls), as NSClipView clips. Corners of different radii take a shape mask over the current bounds; a scroll holder calls this again after committing a new bounds origin. A mask an effect set in its place, hiding the children, stays until the effect puts back HolderMask. A holder that is not a scroll view's container also takes the view's perspective (ChildPerspective), clipping or not, which would otherwise stop at the holder.
func ClipHolder(layer objc.ID, vs *driver.ViewStyle, scrolls bool) {
	if layer == 0 {
		return
	}
	WithoutAnimation(func() {
		m := holderMasks[layer]
		b := objc.Send[CGRect](layer, selBounds)
		w, h := b.Size.Width, b.Size.Height
		if !scrolls {
			// The view's perspective reaches only its own sublayer, the holder, which is flat: the holder gives it to the children. A scroll view's container moves its children with its sublayer transform.
			var depth driver.Points
			if vs != nil {
				depth = vs.ChildPerspective
			}
			setPerspective(layer, depth, w, h)
		}
		if vs == nil || !vs.Clip {
			dropHolderMask(layer, m)
			layer.Send(selSetCornerRadius, 0.0)
			layer.Send(selSetMasksToBounds, scrolls)
			return
		}
		radii := vs.Radii(driver.Size{Width: driver.Points(w), Height: driver.Points(h)})
		if !needsShape(vs, radii) {
			dropHolderMask(layer, m)
			r, masked := cornersOf(radii)
			setCorners(layer, r, masked, vs.ContinuousCorners, w, h)
			layer.Send(selSetMasksToBounds, true)
			return
		}
		layer.Send(selSetCornerRadius, 0.0)
		layer.Send(selSetMasksToBounds, false)
		if m == 0 {
			m = objc.ID(Class("CAShapeLayer")).Send(selNew)
			holderMasks[layer] = m
			if layer.Send(sel("mask")) == 0 {
				layer.Send(selSetMask, m)
			}
		}
		m.Send(selSetFrame, CGRect{Origin: b.Origin, Size: b.Size})
		path := boxPath(0, 0, w, h, radii, vs.ContinuousCorners)
		m.Send(selSetPath, path)
		Release(path)
	})
}

// dropHolderMask removes m, layer's shape mask, unless an effect's mask stands in for it.
func dropHolderMask(layer, m objc.ID) {
	if m == 0 {
		return
	}
	if layer.Send(sel("mask")) == m {
		layer.Send(selSetMask, objc.ID(0))
	}
	m.Send(selRelease)
	delete(holderMasks, layer)
}

// HolderMask is the mask ClipHolder keeps on layer, 0 for none: what an effect that hid the holder's children puts back.
func HolderMask(layer objc.ID) objc.ID { return holderMasks[layer] }

// ForgetHolder releases what ClipHolder made for a holder going away.
func ForgetHolder(layer objc.ID) {
	if m := holderMasks[layer]; m != 0 {
		m.Send(selRelease)
		delete(holderMasks, layer)
	}
}
