//go:build darwin

package apple

import (
	"math"
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestLayerBoundsIncludeTransforms(t *testing.T) {
	root := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer root.Send(selRelease)
	child := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer child.Send(selRelease)
	root.Send(sel("setBounds:"), Rect(0, 0, 300, 250))
	child.Send(sel("setBounds:"), Rect(0, 0, 40, 20))
	child.Send(sel("setAnchorPoint:"), CGPoint{})
	child.Send(sel("setPosition:"), CGPoint{X: 20, Y: 30})
	root.Send(sel("addSublayer:"), child)
	child.Send(sel("setTransform:"), transform3D{M11: 2, M22: 2, M33: 1, M44: 1, M41: -20, M42: -10})
	want := driver.Bounds{Origin: driver.Point{X: 0, Y: 20}, Size: driver.Size{Width: 80, Height: 40}}
	if got, ok := LayerBounds(child, root); !ok || got != want {
		t.Fatalf("native layer bounds = %v, %v; want %v", got, ok, want)
	}
}

func TestLayerGeometryAnimationIgnoresOpacity(t *testing.T) {
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	animation := func(path string) objc.ID {
		a := objc.ID(Class("CABasicAnimation")).Send(sel("animationWithKeyPath:"), NSString(path))
		a.Send(sel("setDuration:"), float64(10))
		return a
	}
	layer.Send(selAddAnimation, animation("opacity"), NSString("fade"))
	if LayerGeometryAnimating(layer) {
		t.Fatal("an opacity-only animation keeps the geometry clock running")
	}
	layer.Send(selAddAnimation, animation("transform"), NSString("move"))
	if !LayerGeometryAnimating(layer) {
		t.Fatal("a transform animation was not observed")
	}
}

func TestLayerGeometryEndingLeavesEndlessKeyframesOut(t *testing.T) {
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	animation := objc.ID(Class("CABasicAnimation")).Send(sel("animationWithKeyPath:"), NSString("transform"))
	animation.Send(sel("setDuration:"), float64(10))
	animation.Send(sel("setRepeatCount:"), float32(math.Inf(1)))
	layer.Send(selAddAnimation, animation, NSString("endless"))
	if LayerGeometryEnding(layer) || !LayerGeometryAnimating(layer) {
		t.Fatal("endless geometry must run the display clock but leave app tests settled")
	}
	animation.Send(sel("setRepeatCount:"), float32(0))
	layer.Send(selAddAnimation, animation, NSString("finite"))
	if !LayerGeometryEnding(layer) {
		t.Fatal("finite native transform animation must keep app tests settling")
	}
}
