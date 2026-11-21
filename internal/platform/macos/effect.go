//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func (s *surface) ensureEffect(v *view) *apple.Effect {
	if v.fx == nil {
		v.fx = apple.NewEffect(func() { s.shadeLater(v) })
	}
	return v.fx
}

func (s *surface) setEffect(v *view, sh *driver.Shader) {
	if sh == nil || sh.Program == nil {
		s.endEffect(v)
		return
	}
	if s.ensureEffect(v).Set(sh, float64(s.env.PixelRatio), contentActions) {
		s.animator.BindShader(v.id, sh.Nodes)
		s.effects[v] = true
		s.shadeLater(v)
	}
}

func (s *surface) effectTarget(v *view) apple.EffectTarget {
	var root *view
	if int(s.root) < len(s.views) {
		root = s.views[s.root]
	}
	target := apple.EffectTarget{
		Scale: float64(s.env.PixelRatio),
		Lanes: func(lanes []float64) { s.animator.ShaderLanes(v.id, lanes) },
	}
	if root != nil {
		target.Root = root.native
	}
	host := v
	if v.fx.Covers() {
		host = root
	}
	if host != nil {
		target.View = host.native
		target.Holder = func() objc.ID { return s.container(host) }
	}
	return target
}

func (s *surface) startTransition(v *view) {
	fx := s.ensureEffect(v)
	fx.CaptureStart(s.effectTarget(v))
}

func (s *surface) drawEffects() {
	for v := range s.effects {
		if s.views[v.id] != v || v.fx == nil {
			delete(s.effects, v)
			continue
		}
		v.fx.Draw(s.effectTarget(v))
	}
}

func (s *surface) staleEffects() {
	for v := range s.effects {
		if v.fx != nil {
			v.fx.Invalidate()
		}
	}
}

func (s *surface) navigating(host *view) bool {
	return host != nil && host.fx.Navigating()
}

func (s *surface) endEffect(v *view) {
	if v.fx == nil {
		return
	}
	fx := v.fx
	v.fx = nil
	delete(s.effects, v)
	fx.Close()
}

// container puts the host's managed children in their own view once.
func (s *surface) container(host *view) objc.ID {
	if host.content != host.native {
		return host.content
	}
	c := apple.NewChildContainer(host.native, objc.ID(classView), true, func(sub objc.ID) bool {
		w := views[sub]
		return w != nil && w.up == host
	})
	host.content = c
	views[c] = host
	return c
}

// clipsInHolder reports whether v clips what it holds without its own layer, which would clip its own shadow and outline too (apple.NeedsClipHolder): a plain view in a holder it takes for its children, a scroll view in the container that holds them already, or an image in its image layer, the two where one radius draws the corners. Other views draw what they show on their own layer, which clips it as before.
func (v *view) clipsInHolder(vs *driver.ViewStyle) bool {
	switch {
	case !apple.NeedsClipHolder(vs):
		return false
	case v.sc != nil, v.kind == driver.KindImage:
		return !apple.ShapedCorners(vs)
	}
	return v.kind == driver.KindView
}

// holdClip clips v's children as vs says, in a holder it makes the first time a plain view needs one, and reports whether v's own layer leaves them to it (clipHeld). A scroll view's container clips as before otherwise; any other holder, an effect's or one that clipped, follows every style unclipped, as it takes the view's perspective (apple.ClipHolder).
func (v *view) holdClip(vs *driver.ViewStyle) bool {
	held := v.clipsInHolder(vs)
	if held && v.kind == driver.KindView && v.content == v.native {
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
