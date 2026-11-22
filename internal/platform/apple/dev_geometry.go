//go:build darwin

package apple

import (
	"math"
	"strings"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// LayerBounds measures the border box through Core Animation's presentation
// hierarchy, including transforms and moving native scroll containers.
func LayerBounds(layer, root objc.ID) (driver.Bounds, bool) {
	m, size, ok := LayerGeometry(layer, root)
	if !ok {
		return driver.Bounds{}, false
	}
	var quad [4]driver.Point
	for i, p := range [4]driver.Point{{}, {X: size.Width}, {X: size.Width, Y: size.Height}, {Y: size.Height}} {
		x, y := m.Apply(float64(p.X), float64(p.Y))
		quad[i] = driver.Point{X: driver.Points(x), Y: driver.Points(y)}
	}
	return driver.PointBounds(quad)
}

// LayerGeometry retains the native viewport's presentation transform and size.
func LayerGeometry(layer, root objc.ID) (driver.Matrix3D, driver.Size, bool) {
	if layer == 0 || root == 0 {
		return driver.Matrix3D{}, driver.Size{}, false
	}
	if a, b := layer.Send(Sel("presentationLayer")), root.Send(Sel("presentationLayer")); a != 0 && b != 0 {
		layer, root = a, b
	}
	b := objc.Send[CGRect](layer, Sel("bounds"))
	r := objc.Send[CGRect](root, Sel("bounds"))
	var points [4]driver.Point
	for i, p := range [4]CGPoint{b.Origin, {X: b.Origin.X + b.Size.Width, Y: b.Origin.Y},
		{X: b.Origin.X + b.Size.Width, Y: b.Origin.Y + b.Size.Height}, {X: b.Origin.X, Y: b.Origin.Y + b.Size.Height}} {
		at := objc.Send[CGPoint](layer, Sel("convertPoint:toLayer:"), p, root)
		points[i] = driver.Point{X: driver.Points(at.X - r.Origin.X), Y: driver.Points(at.Y - r.Origin.Y)}
	}
	size := driver.Size{Width: driver.Points(b.Size.Width), Height: driver.Points(b.Size.Height)}
	m, ok := driver.MatrixFromQuad(points, size)
	return m, size, ok
}

// LayerGeometryAnimating asks only about animations that move the border
// box; an opacity-only animation needs no geometry refresh.
func LayerGeometryAnimating(layer objc.ID) bool { return layerGeometryAnimating(layer, false) }

// LayerGeometryEnding reports finite geometry animations that an app test
// waits for, leaving endless keyframes out of its settling barrier.
func LayerGeometryEnding(layer objc.ID) bool { return layerGeometryAnimating(layer, true) }

func layerGeometryAnimating(layer objc.ID, finite bool) bool {
	keys := layer.Send(Sel("animationKeys"))
	for i, count := uint(0), keys.Send(Sel("count")); i < uint(count); i++ {
		key := keys.Send(Sel("objectAtIndex:"), i)
		if geometryAnimation(layer.Send(Sel("animationForKey:"), key), finite) {
			return true
		}
	}
	return false
}

func geometryAnimation(animation objc.ID, finite bool) bool {
	if finite {
		repeat := objc.Send[float32](animation, Sel("repeatCount"))
		duration := objc.Send[float64](animation, Sel("duration"))
		repeatDuration := objc.Send[float64](animation, Sel("repeatDuration"))
		if repeat >= 1e6 || duration >= 1e6 || repeatDuration >= 1e6 || math.IsInf(float64(repeat), 0) || math.IsInf(duration, 0) || math.IsInf(repeatDuration, 0) {
			return false
		}
	}
	if animation.Send(Sel("respondsToSelector:"), Sel("keyPath")) != 0 {
		path := GoString(animation.Send(Sel("keyPath")))
		if strings.HasPrefix(path, "transform") || strings.HasPrefix(path, "position") || strings.HasPrefix(path, "bounds") || strings.HasPrefix(path, "anchorPoint") {
			return true
		}
	}
	if animation.Send(Sel("respondsToSelector:"), Sel("animations")) != 0 {
		children := animation.Send(Sel("animations"))
		for i, count := uint(0), children.Send(Sel("count")); i < uint(count); i++ {
			if geometryAnimation(children.Send(Sel("objectAtIndex:"), i), finite) {
				return true
			}
		}
	}
	return false
}

// NativeViews yields a surface's mounted views, each ID with its native view, for the geometry helpers below that macOS and iOS share.
type NativeViews = func(yield func(driver.NodeID, objc.ID) bool)

// ReportLayerGeometry sends observer the presented geometry of each view's layer in root's coordinates, when it observes, and reports whether a layer still animates its geometry, so the surface keeps its frames coming.
func ReportLayerGeometry(observer *driver.GeometryObserver, root objc.ID, views NativeViews) bool {
	if !observer.Active() {
		return false
	}
	geometry := map[driver.NodeID]driver.ViewGeometry{}
	for id, native := range views {
		if m, size, ok := LayerGeometry(native.Send(objc.RegisterName("layer")), root); ok {
			geometry[id] = driver.ViewGeometry{Frame: driver.Bounds{Size: size}, Matrix: m}
		}
	}
	observer.Notify(driver.ProjectGeometry(geometry))
	return LayersAnimating(observer, views)
}

// LayersAnimating reports whether, while observer observes, a view's layer still animates its geometry.
func LayersAnimating(observer *driver.GeometryObserver, views NativeViews) bool {
	if !observer.Active() {
		return false
	}
	for _, native := range views {
		if LayerGeometryAnimating(native.Send(objc.RegisterName("layer"))) {
			return true
		}
	}
	return false
}

// LayersEnding reports whether a view's layer runs a finite geometry animation, which a test waits for.
func LayersEnding(views NativeViews) bool {
	for _, native := range views {
		if LayerGeometryEnding(native.Send(objc.RegisterName("layer"))) {
			return true
		}
	}
	return false
}
