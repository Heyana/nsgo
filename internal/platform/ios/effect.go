//go:build ios

package ios

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// effectHost binds the shared effect lifecycle to this backend's view type.
// nsgo:allow-duplicate The native view type belongs to its backend; lifecycle bookkeeping lives in apple.Effects.
func (s *surface) effectHost() apple.EffectHost[*view] {
	var root *view
	if int(s.root) < len(s.views) {
		root = s.views[s.root]
	}
	return apple.EffectHost[*view]{
		Root: root, Scale: float64(s.env.PixelRatio), Actions: contentActions,
		Animator:  s.animator,
		ID:        func(v *view) driver.NodeID { return v.id },
		Native:    func(v *view) objc.ID { return v.native },
		Container: s.container,
		Current:   func(v *view) bool { return int(v.id) < len(s.views) && s.views[v.id] == v },
		Ready:     s.shadeLater,
	}
}

func (s *surface) setEffect(v *view, sh *driver.Shader) {
	s.effects.Set(v, sh, s.effectHost())
}

func (s *surface) startTransition(v *view) {
	s.effects.CaptureStart(v, s.effectHost())
}

func (s *surface) drawEffects() { s.effects.Draw(s.effectHost()) }

func (s *surface) staleEffects() { s.effects.Invalidate() }

func (s *surface) navigating(host *view) bool {
	return host != nil && s.effects.Get(host).Navigating()
}

func (s *surface) endEffect(v *view) { s.effects.End(v) }

// container puts the host's managed children in their own view once.
func (s *surface) container(host *view) objc.ID {
	if host.container != 0 {
		return host.container
	}
	c := apple.NewChildContainer(host.native, objc.ID(classView), false, func(sub objc.ID) bool {
		w := views[sub]
		return w != nil && w != host
	})
	host.container = c
	views[c] = host
	return c
}

// clipsInHolder reports whether v clips what it holds without its own layer, which would clip its own shadow and outline too (apple.NeedsClipHolder): a plain view in a holder it takes for its children, or an image in its image layer where one radius draws the corners. Other views draw what they show on their own layer, which clips it as before, and a scroll view's children scroll in it, where a holder would hold them still.
func (v *view) clipsInHolder(vs *driver.ViewStyle) bool {
	switch {
	case !apple.NeedsClipHolder(vs) || v.sc != nil:
		return false
	case v.kind == driver.KindImage:
		return !apple.ShapedCorners(vs)
	}
	return v.kind == driver.KindView
}

// holdClip clips v's children as vs says, in a holder it makes the first time a plain view needs one, and reports whether v's own layer leaves them to it (clipHeld). Any other holder but a scroll view's, an effect's or one that clipped, follows every style unclipped, as it takes the view's perspective (apple.ClipHolder).
func (v *view) holdClip(vs *driver.ViewStyle) bool {
	held := v.clipsInHolder(vs)
	if held && v.kind == driver.KindView && v.container == 0 {
		v.surface.container(v)
	}
	if v.container != 0 && v.sc == nil {
		clip := *vs
		clip.Clip = held
		v.fitHolder(&clip)
	}
	v.clipHeld = held
	return held
}

// fitHolder fits v's holder to v's bounds and clips it as vs says, nil for not at all.
func (v *view) fitHolder(vs *driver.ViewStyle) {
	v.container.Send(selSetFrame, objc.Send[apple.CGRect](v.native, selBounds))
	apple.ClipHolder(v.container.Send(selLayer), vs, false)
}
