//go:build darwin

package apple

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Effect draws a shader over native children without changing their input.
// The platform supplies the host and its child container, while Effect owns
// the shader layer, its mask and the pictures it samples.
type Effect struct {
	shader *driver.Shader
	layer  *ShaderLayer
	ready  func()

	pics      [4]Picture
	before    Picture
	hasBefore bool
	stale     bool
	host      objc.ID
	holder    objc.ID
	masked    bool
	frame     driver.Bounds
}

// EffectTarget is the native view an effect draws over for this frame.
// Holder makes its child container on demand.
type EffectTarget struct {
	View, Root objc.ID
	Holder     func() objc.ID
	Scale      float64
	Lanes      func([]float64)
}

func NewEffect(ready func()) *Effect { return &Effect{ready: ready} }

const effectZ = 1e6

var emptyEffectMask objc.ID

// Set updates the effect and reports whether Metal can draw it.
func (fx *Effect) Set(sh *driver.Shader, scale float64, actions objc.ID) bool {
	if fx.layer == nil {
		fx.layer = NewShaderLayer(scale)
		if fx.layer == nil {
			return false
		}
		fx.layer.Layer.Send(sel("setActions:"), actions)
		fx.layer.Layer.Send(sel("setZPosition:"), float64(effectZ))
		fx.layer.Ready = fx.ready
	}
	was := fx.shader
	fx.shader = sh
	fx.layer.Set(sh)
	if sh.Role.Transitions() {
		switch {
		case sh.Running && fx.hasBefore:
			side := 2
			if sh.Backwards {
				side = 3
			}
			fx.pics[side], fx.before = fx.before, fx.pics[side]
			fx.hasBefore = false
		case !sh.Running && was != nil && was.Running:
			fx.pics[2].Release()
			fx.pics[3].Release()
		}
	}
	fx.stale = true
	return true
}

// CaptureStart pictures what shows before the transition's batch applies.
func (fx *Effect) CaptureStart(target EffectTarget) {
	if target.View == 0 {
		return
	}
	if fx.shader != nil && fx.shader.Running && fx.host != 0 {
		// A transition interrupted by another starts from what it last drew.
		fx.Draw(target)
	}
	fx.hasBefore = fx.picture(target, &fx.before, false)
}

// Covers reports whether the effect targets the surface's root view.
func (fx *Effect) Covers() bool { return fx.shader != nil && fx.shader.Role.Covers() }

// Navigating reports whether a navigation shader is drawing the transition.
func (fx *Effect) Navigating() bool {
	return fx != nil && fx.shader != nil && fx.shader.Role == driver.ShaderNav && fx.shader.Running && fx.layer != nil
}

// Invalidate marks the pictures for capture on the next draw.
func (fx *Effect) Invalidate() { fx.stale = true }

// NewChildContainer moves the host's managed children into a native view.
// A layer-backed AppKit container asks for a layer at creation.
func NewChildContainer(host, class objc.ID, layerBacked bool, child func(objc.ID) bool) objc.ID {
	c := class.Send(sel("new"))
	if layerBacked {
		c.Send(sel("setWantsLayer:"), true)
	}
	c.Send(selSetFrame, objc.Send[CGRect](host, selBounds))
	const widthSizable, heightSizable = 2, 16
	c.Send(sel("setAutoresizingMask:"), uint(widthSizable|heightSizable))
	subs := host.Send(sel("subviews"))
	n := objc.Send[uint](subs, sel("count"))
	var children []objc.ID
	for i := range n {
		sub := subs.Send(sel("objectAtIndex:"), i)
		if child(sub) {
			children = append(children, sub)
		}
	}
	for _, sub := range children {
		sub.Send(sel("retain"))
		sub.Send(sel("removeFromSuperview"))
		c.Send(sel("addSubview:"), sub)
		sub.Send(selRelease)
	}
	host.Send(sel("addSubview:"), c)
	c.Send(selRelease)
	return c
}

func (fx *Effect) attach(target EffectTarget) {
	if fx.host == target.View {
		return
	}
	fx.mask(false)
	fx.layer.Layer.Send(sel("removeFromSuperlayer"))
	fx.holder = target.Holder()
	target.View.Send(sel("layer")).Send(sel("addSublayer:"), fx.layer.Layer)
	fx.host = target.View
}

func (fx *Effect) mask(on bool) {
	if fx.masked == on || fx.host == 0 {
		return
	}
	fx.masked = on
	layer := fx.holder.Send(sel("layer"))
	WithoutAnimation(func() {
		if on {
			if emptyEffectMask == 0 {
				emptyEffectMask = objc.ID(Class("CALayer")).Send(sel("new"))
			}
			layer.Send(sel("setMask:"), emptyEffectMask)
		} else {
			layer.Send(sel("setMask:"), HolderMask(layer))
		}
		fx.layer.Layer.Send(selSetHidden, !on)
	})
}

// picture captures the host's box and overflow. The backdrop is the root
// with the host's layer hidden.
func (fx *Effect) picture(target EffectTarget, pic *Picture, backdrop bool) bool {
	ov := 0.0
	if fx.shader != nil {
		ov = fx.shader.Overflow
	}
	b := objc.Send[CGRect](target.View, selBounds)
	w, h := b.Size.Width+2*ov, b.Size.Height+2*ov
	if backdrop {
		if target.Root == 0 {
			return false
		}
		at := objc.Send[CGPoint](target.View, sel("convertPoint:toView:"), CGPoint{X: -ov, Y: -ov}, target.Root)
		return pic.Take(target.Root.Send(sel("layer")), at.X, at.Y, w, h, target.Scale, target.View.Send(sel("layer")))
	}
	return pic.Take(target.Holder().Send(sel("layer")), -ov, -ov, w, h, target.Scale, 0)
}

// Draw attaches the effect to its host and captures the pictures it reads.
func (fx *Effect) Draw(target EffectTarget) {
	if fx.layer == nil || target.View == 0 {
		return
	}
	fx.attach(target)
	sh := fx.shader
	shows := sh != nil && (!sh.Role.Transitions() || sh.Running) && fx.layer.Drawable()
	fx.mask(shows)
	if !shows {
		return
	}
	ov := sh.Overflow
	b := objc.Send[CGRect](target.View, selBounds)
	w, h := b.Size.Width+2*ov, b.Size.Height+2*ov
	frame := driver.Bounds{Origin: driver.Point{X: driver.Points(-ov), Y: driver.Points(-ov)},
		Size: driver.Size{Width: driver.Points(w), Height: driver.Points(h)}}
	if frame != fx.frame {
		fx.frame = frame
		WithoutAnimation(func() {
			fx.layer.Layer.Send(selSetFrame, Rect(-ov, -ov, w, h))
		})
		fx.layer.Resize(w, h, target.Scale)
		fx.stale = true
	}
	reads := sh.Pictures
	if sh.Role.Transitions() {
		// The live side is pictured each frame while it runs.
		live := 3
		if sh.Backwards {
			live = 2
		}
		if reads&(1<<live) != 0 {
			fx.picture(target, &fx.pics[live], false)
		}
	} else if fx.stale {
		if reads&driver.PictureContent != 0 {
			fx.picture(target, &fx.pics[0], false)
		}
		if reads&driver.PictureBackdrop != 0 {
			fx.picture(target, &fx.pics[1], true)
		}
	}
	fx.stale = false
	fx.layer.SetPictures([4]*Picture{&fx.pics[0], &fx.pics[1], &fx.pics[2], &fx.pics[3]})
	fx.layer.Draw(target.Lanes)
}

// Close restores the host's children and releases the shader's resources.
func (fx *Effect) Close() {
	fx.mask(false)
	if fx.layer != nil {
		fx.layer.Layer.Send(sel("removeFromSuperlayer"))
		fx.layer.Release()
	}
	for i := range fx.pics {
		fx.pics[i].Release()
	}
	fx.before.Release()
}
