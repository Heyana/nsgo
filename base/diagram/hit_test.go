package diagram

import (
	"testing"

	"github.com/firede/limu/scene"
)

func TestItemAtHitsTopmostLabelsAndEdges(t *testing.T) {
	edge := scene.Item{Kind: scene.Edge, Link: "https://example.com/edge", Parts: []scene.Part{{
		Path: new(scene.Path).MoveTo(0, 0).LineTo(40, 0), Paint: scene.StrokeOnly,
	}}}
	node := scene.Item{Kind: scene.Node, Link: "https://example.com/node", Box: scene.Rect{X: 10, Y: -10, W: 20, H: 20}}
	sc := &scene.Scene{Items: []scene.Item{edge, node}}
	if got := itemAt(sc, scene.Point{X: 20, Y: 0}, 4); got != &sc.Items[1] {
		t.Fatalf("overlap hit = %#v, want the topmost node", got)
	}
	if got := itemAt(sc, scene.Point{X: 35, Y: 2}, 4); got != &sc.Items[0] {
		t.Fatalf("edge hit = %#v, want the edge", got)
	}
	if got := itemAt(sc, scene.Point{X: 60, Y: 20}, 4); got != nil {
		t.Fatalf("empty hit = %#v, want nil", got)
	}
}
