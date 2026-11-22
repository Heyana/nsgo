package diagram

import (
	"math"

	"github.com/firede/limu/scene"
)

// itemAt is limumygo's scene hit test, kept here because the native canvas needs the item rather than only the scene's bounds. Items drawn later are on top, and edge lines remain targetable with a small pointer slop.
func itemAt(sc *scene.Scene, p scene.Point, slop float64) *scene.Item {
	for i := len(sc.Items) - 1; i >= 0; i-- {
		if hits(&sc.Items[i], p, slop) {
			return &sc.Items[i]
		}
	}
	return nil
}

func hits(it *scene.Item, p scene.Point, slop float64) bool {
	if labelHolds(it, p) {
		return true
	}
	if it.Kind == scene.Edge || linesOnly(it) {
		for _, part := range it.Parts {
			if part.Path == nil {
				if near(part.Box, p, slop) {
					return true
				}
				continue
			}
			for _, line := range part.Path.Flatten(0.5) {
				if lineNear(line, p, slop) {
					return true
				}
			}
		}
		for _, marker := range it.Ends {
			for _, part := range marker.Outline() {
				b := part.Box
				if part.Path != nil {
					b = part.Path.Bounds()
				}
				if near(b, p, slop) {
					return true
				}
			}
		}
		if len(it.Parts) == 0 {
			for i := 1; i < len(it.Route); i++ {
				if segmentNear(it.Route[i-1], it.Route[i], p, slop) {
					return true
				}
			}
		}
		return false
	}
	if it.Box.W > 0 || it.Box.H > 0 {
		return it.Box.Contains(p)
	}
	for _, part := range it.Parts {
		b := part.Box
		if part.Path != nil {
			b = part.Path.Bounds()
		}
		if b.Contains(p) {
			return true
		}
	}
	return false
}

func linesOnly(it *scene.Item) bool {
	for _, part := range it.Parts {
		if part.Paint != scene.StrokeOnly {
			return false
		}
	}
	return len(it.Parts) > 0
}

func labelHolds(it *scene.Item, p scene.Point) bool {
	label := it.Label
	if label == nil || label.Empty() {
		return false
	}
	if it.Rotate != 0 {
		sin, cos := math.Sincos(-it.Rotate * math.Pi / 180)
		dx, dy := p.X-it.At.X, p.Y-it.At.Y
		p = scene.Point{X: it.At.X + dx*cos - dy*sin, Y: it.At.Y + dx*sin + dy*cos}
	}
	x := it.At.X - label.Width/2
	switch it.Align {
	case scene.Start:
		x = it.At.X
	case scene.End:
		x = it.At.X - label.Width
	}
	return scene.Rect{X: x, Y: it.At.Y - label.Height/2, W: label.Width, H: label.Height}.Contains(p)
}

func near(b scene.Rect, p scene.Point, slop float64) bool {
	return scene.Rect{X: b.X - slop, Y: b.Y - slop, W: b.W + 2*slop, H: b.H + 2*slop}.Contains(p)
}

func lineNear(line scene.Polyline, p scene.Point, slop float64) bool {
	for i := 1; i < len(line.Points); i++ {
		if segmentNear(line.Points[i-1], line.Points[i], p, slop) {
			return true
		}
	}
	return line.Closed && len(line.Points) > 1 && segmentNear(line.Points[len(line.Points)-1], line.Points[0], p, slop)
}

func segmentNear(a, b, p scene.Point, slop float64) bool {
	dx, dy := b.X-a.X, b.Y-a.Y
	t := 0.0
	if length := dx*dx + dy*dy; length > 0 {
		t = max(0, min(1, ((p.X-a.X)*dx+(p.Y-a.Y)*dy)/length))
	}
	return math.Hypot(a.X+dx*t-p.X, a.Y+dy*t-p.Y) <= slop
}
