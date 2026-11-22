//go:build darwin

package apple

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Effects owns a surface's effects, including captures made before a
// transition is installed. Its zero value is ready to use.
type Effects[V comparable] struct {
	entries map[V]*Effect
	active  map[V]bool
}

// EffectHost supplies the native views and animation graph of one surface.
type EffectHost[V comparable] struct {
	Root      V
	Scale     float64
	Actions   objc.ID
	Animator  *driver.Animator
	ID        func(V) driver.NodeID
	Native    func(V) objc.ID
	Container func(V) objc.ID
	Current   func(V) bool
	Ready     func(V)
}

// Get returns the effect of v, including an uninstalled transition capture.
func (effects *Effects[V]) Get(v V) *Effect { return effects.entries[v] }

func (effects *Effects[V]) ensure(v V, host EffectHost[V]) *Effect {
	if fx := effects.Get(v); fx != nil {
		return fx
	}
	if effects.entries == nil {
		effects.entries = make(map[V]*Effect)
	}
	ready := host.Ready
	fx := NewEffect(func() { ready(v) })
	effects.entries[v] = fx
	return fx
}

// Set installs a shader and its animated lanes, or releases a removed effect.
func (effects *Effects[V]) Set(v V, sh *driver.Shader, host EffectHost[V]) {
	if sh == nil || sh.Program == nil {
		effects.End(v)
		return
	}
	if effects.ensure(v, host).Set(sh, host.Scale, host.Actions) {
		host.Animator.BindShader(host.ID(v), sh.Nodes)
		if effects.active == nil {
			effects.active = make(map[V]bool)
		}
		effects.active[v] = true
		host.Ready(v)
	}
}

func (effects *Effects[V]) target(v V, host EffectHost[V]) EffectTarget {
	target := EffectTarget{
		Scale: host.Scale,
		Lanes: func(lanes []float64) { host.Animator.ShaderLanes(host.ID(v), lanes) },
	}
	var zero V
	if host.Root != zero {
		target.Root = host.Native(host.Root)
	}
	view := v
	if effects.Get(v).Covers() {
		view = host.Root
	}
	if view != zero {
		target.View = host.Native(view)
		target.Holder = func() objc.ID { return host.Container(view) }
	}
	return target
}

// CaptureStart preserves the pixels before a transition's batch applies.
func (effects *Effects[V]) CaptureStart(v V, host EffectHost[V]) {
	fx := effects.ensure(v, host)
	fx.CaptureStart(effects.target(v, host))
}

// Draw refreshes installed effects and forgets views replaced in the surface.
func (effects *Effects[V]) Draw(host EffectHost[V]) {
	for v := range effects.active {
		if !host.Current(v) {
			effects.End(v)
			continue
		}
		effects.Get(v).Draw(effects.target(v, host))
	}
}

// Invalidate marks installed effects for capture on the next draw.
func (effects *Effects[V]) Invalidate() {
	for v := range effects.active {
		effects.Get(v).Invalidate()
	}
}

// End releases an effect and any capture it retained before installation.
func (effects *Effects[V]) End(v V) {
	if fx := effects.Get(v); fx != nil {
		delete(effects.entries, v)
		delete(effects.active, v)
		fx.Close()
	}
}
