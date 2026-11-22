//go:build js && wasm

package web

import (
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func (s *surface) ObserveGeometry(report func(driver.GeometrySnapshot)) {
	s.devGeometry.Observe(report)
	s.reportGeometry()
}

func (s *surface) reportGeometry() {
	if !s.devGeometry.Active() {
		return
	}
	root := clientMatrix(s.host)
	views := make(map[driver.NodeID]driver.ViewGeometry, len(s.views))
	for _, v := range s.views {
		if v == nil || !v.el.Truthy() || !s.host.Call("contains", v.el).Bool() {
			continue
		}
		style := js.Global().Call("getComputedStyle", v.el)
		size := driver.Size{Width: driver.Points(cssNumber(style.Get("width").String(), 0)), Height: driver.Points(cssNumber(style.Get("height").String(), 0))}
		matrix := clientMatrix(v.el)
		var quad [4]driver.Point
		valid := true
		for i, p := range [4]driver.Point{{}, {X: size.Width}, {X: size.Width, Y: size.Height}, {Y: size.Height}} {
			x, y := matrix.Apply(float64(p.X), float64(p.Y))
			point, ok := root.Unproject(driver.Point{X: driver.Points(x), Y: driver.Points(y)})
			if !ok {
				valid = false
				break
			}
			quad[i] = point
		}
		if m, ok := driver.MatrixFromQuad(quad, size); valid && ok {
			views[v.id] = driver.ViewGeometry{Frame: driver.Bounds{Size: size}, Matrix: m}
		}
	}
	s.devGeometry.Notify(driver.ProjectGeometry(views))
	if s.geometryAnimating() {
		s.startTicking()
	}
}

func (s *surface) geometryAnimating() bool {
	if !s.devGeometry.Active() || s.host.Get("getAnimations").Type() != js.TypeFunction {
		return false
	}
	animations := s.host.Call("getAnimations", map[string]any{"subtree": true})
	for i := 0; i < animations.Length(); i++ {
		state := animations.Index(i).Get("playState").String()
		if state == "running" || state == "pending" {
			return true
		}
	}
	return false
}
