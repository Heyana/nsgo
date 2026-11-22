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

// container puts managed children, hosted native content and the host's text, canvas or shader layers in their own view once.
func (s *surface) container(host *view) objc.ID {
	if host.container != 0 {
		return host.container
	}
	c := apple.NewChildContainer(host.native, objc.ID(classView), false, func(sub objc.ID) bool {
		w := views[sub]
		return w != nil && w != host || sub == host.componentView ||
			host.input != nil && (sub == host.input.field || sub == host.input.label) ||
			host.sc != nil && sub == host.sc.zoom
	})
	host.container = c
	if host.contentLayer != 0 {
		host.attachContentLayer(host.contentLayer)
	}
	if host.shader != nil {
		host.attachContentLayer(host.shader.Layer)
	}
	if host.web != nil {
		host.web.SetHolder(c)
	}
	if host.video != nil {
		host.video.SetHolder(c)
	}
	views[c] = host
	return c
}

// attachContentLayer puts drawn content in the child holder when one exists.
func (v *view) attachContentLayer(layer objc.ID) {
	host := v.native
	if v.container != 0 {
		host = v.container
	}
	host.Send(selLayer).Send(sel("addSublayer:"), layer)
}

// clipsInHolder reports whether v clips what it holds without its own layer, which would clip its own shadow and outline too (apple.NeedsClipHolder): a plain view, text, canvas, shader, hosted native control or scroll viewport in a holder of its children and content, or an image in its own image layer.
func (v *view) clipsInHolder(vs *driver.ViewStyle) bool {
	switch {
	case !apple.NeedsClipHolder(vs):
		return false
	case v.kind == driver.KindImage:
		return true
	}
	return v.kind == driver.KindView || v.kind == driver.KindText || v.kind == driver.KindCanvas || v.kind == driver.KindShader ||
		v.kind == driver.KindInput || v.kind == driver.KindWeb || v.kind == driver.KindVideo || v.kind == driver.KindNative || v.sc != nil
}

// holdClip clips v's children as vs says, in a holder it makes the first time a view with drawn or hosted content or a scroll viewport needs one, and reports whether v's own layer leaves them to it (clipHeld). An existing holder keeps following the view's bounds and perspective after the shadow or outline goes away, a scroll viewport's as its offset moves too (apple.ClipHolder).
func (v *view) holdClip(vs *driver.ViewStyle) bool {
	held := v.clipsInHolder(vs)
	if held && v.kind != driver.KindImage && v.container == 0 {
		v.surface.container(v)
	}
	if v.container != 0 {
		clip := *vs
		clip.Clip = held
		v.fitHolder(&clip)
	}
	v.clipHeld = held
	return held
}

// fitHolder fits v's holder to v's bounds and clips it as vs says, nil for not at all.
func (v *view) fitHolder(vs *driver.ViewStyle) {
	bounds := objc.Send[apple.CGRect](v.native, selBounds)
	v.container.Send(selSetFrame, bounds)
	if v.sc != nil {
		// The viewport stays over the scroll view while its bounds keep children in their content coordinates for UIKit hit testing.
		v.container.Send(sel("setBounds:"), bounds)
	}
	apple.ClipHolder(v.container.Send(selLayer), vs, false)
}
