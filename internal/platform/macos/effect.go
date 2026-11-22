//go:build darwin && !ios

package macos

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

// container puts managed children and the host's text, canvas or shader layers in their own view once.
func (s *surface) container(host *view) objc.ID {
	if host.content != host.native {
		return host.content
	}
	c := apple.NewChildContainer(host.native, objc.ID(classView), true, func(sub objc.ID) bool {
		w := views[sub]
		return w != nil && w.up == host
	})
	host.content = c
	if host.contentLayer != 0 {
		host.attachContentLayer(host.contentLayer)
	}
	if host.shader != nil {
		host.attachContentLayer(host.shader.Layer)
	}
	views[c] = host
	return c
}

// attachContentLayer puts drawn content in the child holder when one exists.
func (v *view) attachContentLayer(layer objc.ID) {
	host := v.native
	if v.content != 0 {
		host = v.content
	}
	host.Send(selLayer).Send(sel("addSublayer:"), layer)
}

// clipsInHolder reports whether v clips what it holds without its own layer, which would clip its own shadow and outline too (apple.NeedsClipHolder): a plain view, text, canvas or shader in a holder of its children and drawn content, a scroll view in the container that holds them already, or an image in its image layer, the two where one radius draws the corners. Other native views still clip their own layer.
func (v *view) clipsInHolder(vs *driver.ViewStyle) bool {
	switch {
	case !apple.NeedsClipHolder(vs):
		return false
	case v.sc != nil, v.kind == driver.KindImage:
		return !apple.ShapedCorners(vs)
	}
	return v.kind == driver.KindView || v.kind == driver.KindText || v.kind == driver.KindCanvas || v.kind == driver.KindShader
}

// holdClip clips v's children as vs says, in a holder it makes the first time a plain view, text, canvas or shader needs one, and reports whether v's own layer leaves them to it (clipHeld). A scroll view's container clips as before otherwise; any other holder, an effect's or one that clipped, follows every style unclipped, as it takes the view's perspective (apple.ClipHolder).
func (v *view) holdClip(vs *driver.ViewStyle) bool {
	held := v.clipsInHolder(vs)
	if held && v.kind != driver.KindImage && v.content == v.native {
		v.surface.container(v)
	}
	if (held || v.clipHeld || v.sc == nil) && v.content != v.native {
		clip := *vs
		clip.Clip = held
		v.fitHolder(&clip)
	}
	v.clipHeld = held
	return held
}

// fitHolder fits v's holder to v's bounds and clips it as vs says, nil for not at all; a scroll view sizes its container itself.
func (v *view) fitHolder(vs *driver.ViewStyle) {
	if v.sc == nil {
		v.content.Send(selSetFrame, objc.Send[apple.CGRect](v.native, selBounds))
	}
	apple.ClipHolder(v.content.Send(selLayer), vs, v.sc != nil)
}
