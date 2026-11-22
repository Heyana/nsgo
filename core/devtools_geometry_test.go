package core

import (
	"maps"
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestDevToolsAndAuditFollowCommittedTransforms(t *testing.T) {
	h := newHost(t, Size{Width: 300, Height: 250}, func(*Scope) Node {
		return Group{
			View{TestID: "parent", Style: Style{Position: Absolute, Left: Pt(20), Top: Pt(30), Width: Pt(100), Height: Pt(100),
				Transform: Transform{TranslateX: 10, TranslateY: 20}}, Children: []Node{
				View{TestID: "child", Style: Style{Position: Absolute, Left: Pt(5), Top: Pt(7), Width: Pt(40), Height: Pt(20), Transform: Transform{Scale: 2}}},
			}},
			audit,
		}
	})
	h.commit()
	root := (&devSurface{t: h.tree}).Elements()
	var child *devtools.Node
	var visit func(*devtools.Node)
	visit = func(n *devtools.Node) {
		if n.TestID == "child" {
			child = n
		}
		for i := range n.Children {
			visit(&n.Children[i])
		}
	}
	visit(&root)
	want := [4]float32{15, 47, 80, 40}
	if child == nil || child.Bounds != want {
		t.Fatalf("transformed DevTools child = %+v, want bounds %v", child, want)
	}
	for _, box := range h.tree.auditBoxes(nil) {
		if box.bounds == (Bounds{Origin: Point{X: 15, Y: 47}, Size: Size{Width: 80, Height: 40}}) {
			return
		}
	}
	t.Fatal("audit did not outline the transformed child")
}

func TestDevGeometryObservationSurvivesPaintAndEndsWithTree(t *testing.T) {
	h := newHost(t, Size{Width: 300, Height: 250}, func(*Scope) Node {
		return Group{View{TestID: "moving", Style: Style{Width: Pt(40), Height: Pt(20)}}, audit}
	})
	var report func(driver.GeometrySnapshot)
	started, stopped := 0, 0
	h.tree.SetDevGeometrySource(func(next func(driver.GeometrySnapshot)) {
		if next == nil {
			stopped++
		} else {
			started++
			report = next
		}
	})
	batch := h.commit()
	if report == nil || batch.GeometryVersion == 0 {
		t.Fatal("audit did not request versioned presentation geometry")
	}
	id := h.find("moving").id
	for _, x := range []Points{70, 90} {
		bounds := maps.Clone(h.tree.devGeometry())
		bounds[id] = Bounds{Origin: Point{X: x, Y: 30}, Size: Size{Width: 40, Height: 20}}
		report(driver.GeometrySnapshot{Version: batch.GeometryVersion, Bounds: bounds})
		h.drain()
		h.commit()
		if got := h.tree.devGeometry()[id].Origin.X; got != x {
			t.Fatalf("presentation after repaint = %v, want %v", got, x)
		}
		if started != 1 || stopped != 0 {
			t.Fatalf("paint changed the native subscription: starts=%d stops=%d", started, stopped)
		}
	}
	h.root.Dispose()
	if stopped != 1 {
		t.Fatalf("tree disposal stopped observation %d times", stopped)
	}
}

func TestDevGeometryFollowsTheDisplayedFrameWithoutChangingRefBounds(t *testing.T) {
	var left *Signal[Points]
	var ref Ref
	h := newHost(t, Size{Width: 300, Height: 250}, func(cx *Scope) Node {
		left = NewSignal(cx, Points(10))
		return Group{View{TestID: "moving", Ref: &ref, StyleFunc: func() Style {
			return Style{Position: Absolute, Left: Pt(left.Get()), Width: Pt(40), Height: Pt(20)}
		}}, audit}
	})
	h.tree.SetDevGeometrySource(func(func(driver.GeometrySnapshot)) {})
	first := h.commit()
	id := h.find("moving").id
	old := maps.Clone(h.tree.devGeometry())
	old[id] = Bounds{Origin: Point{X: 90}, Size: Size{Width: 40, Height: 20}}
	left.Set(30)
	second := h.commit()
	h.tree.acceptDevGeometry(driver.GeometrySnapshot{Version: first.GeometryVersion, Bounds: old})
	if got := h.tree.devGeometry()[id].Origin.X; got != 90 {
		t.Fatalf("the displayed frame was replaced by the layout destination: x=%v", got)
	}
	fresh := maps.Clone(h.tree.devGeometry())
	fresh[id] = Bounds{Origin: Point{X: 50}, Size: Size{Width: 40, Height: 20}}
	h.tree.acceptDevGeometry(driver.GeometrySnapshot{Version: second.GeometryVersion, Bounds: fresh})
	if got := h.tree.devGeometry()[id].Origin.X; got != 50 {
		t.Fatalf("current presentation was lost: x=%v", got)
	}
	if layout, ok := ref.Bounds(); !ok || layout.Origin.X != 30 {
		t.Fatalf("native presentation changed Ref.Bounds' committed layout: %v, %v", layout, ok)
	}
}

func TestDevGeometryRejectsSnapshotsForReusedNodeIDs(t *testing.T) {
	var shown *Signal[bool]
	h := newHost(t, Size{Width: 300, Height: 250}, func(cx *Scope) Node {
		shown = NewSignal(cx, true)
		return Group{Show{When: shown.Get, Then: func(*Scope) Node {
			return View{TestID: "reused", Style: Style{Position: Absolute, Left: Pt(30), Width: Pt(40), Height: Pt(20)}}
		}}, audit}
	})
	h.tree.SetDevGeometrySource(func(func(driver.GeometrySnapshot)) {})
	first := h.commit()
	id := h.find("reused").id
	old := maps.Clone(h.tree.devGeometry())
	old[id] = Bounds{Origin: Point{X: 200}, Size: Size{Width: 40, Height: 20}}
	shown.Set(false)
	h.commit()
	shown.Set(true)
	h.commit()
	if got := h.find("reused").id; got != id {
		t.Fatalf("fixture did not reuse the node ID: old=%d new=%d", id, got)
	}
	h.tree.acceptDevGeometry(driver.GeometrySnapshot{Version: first.GeometryVersion, Bounds: old})
	if got := h.tree.devGeometry()[id].Origin.X; got != 30 {
		t.Fatalf("an earlier view's snapshot reached the reused node ID: x=%v", got)
	}
}

func TestDevGeometryIncludesNestedStickyOffsets(t *testing.T) {
	h := newHost(t, Size{Width: 300, Height: 400}, func(cx *Scope) Node {
		return Group{Scroll{TestID: "scroll", Style: Style{Height: Pt(200)}, Children: []Node{
			View{Style: Style{Height: Pt(100)}},
			View{Style: Style{Height: Pt(500)}, Children: []Node{
				View{Style: Style{Height: Pt(30)}},
				View{TestID: "header", Style: Style{Height: Pt(20), Position: Sticky, Top: Pt(10)}, Children: []Node{
					View{TestID: "child", Style: Style{Height: Pt(10), Transform: Transform{TranslateY: 3}}},
				}},
			}},
		}}, audit}
	})
	h.commit()
	header, child, scroll := h.find("header"), h.find("child"), h.find("scroll")
	for _, step := range []struct{ offset, top Points }{{0, 130}, {200, 10}, {590, -10}} {
		scroll.scroll.offset.Y = step.offset
		bounds := h.tree.devGeometry()
		if got := bounds[header.id].Origin.Y; got != step.top {
			t.Fatalf("sticky offset %v: header y=%v, want %v", step.offset, got, step.top)
		}
		if got := bounds[child.id].Origin.Y; got != step.top+3 {
			t.Fatalf("sticky offset %v: transformed child y=%v, want %v", step.offset, got, step.top+3)
		}
	}
}

func TestDevGeometryStaysIdleUntilRead(t *testing.T) {
	h := newHost(t, Size{Width: 300, Height: 250}, func(*Scope) Node { return View{} })
	observations := 0
	h.tree.SetDevGeometrySource(func(func(driver.GeometrySnapshot)) { observations++ })
	if batch := h.commit(); batch.GeometryVersion != 0 || observations != 0 || h.tree.devNative != nil {
		t.Fatalf("ordinary commit activated diagnostics: version=%d observations=%d state=%+v", batch.GeometryVersion, observations, h.tree.devNative)
	}
	(&devSurface{t: h.tree}).Elements()
	if observations != 1 {
		t.Fatalf("first diagnostic read started %d observations", observations)
	}
}

func TestDevGeometryLateObservationRejectsUnversionedReusedIDs(t *testing.T) {
	var shown *Signal[bool]
	h := newHost(t, Size{Width: 300, Height: 250}, func(cx *Scope) Node {
		shown = NewSignal(cx, true)
		return Show{When: shown.Get, Then: func(*Scope) Node {
			return View{TestID: "reused", Style: Style{Position: Absolute, Left: Pt(30), Width: Pt(40), Height: Pt(20)}}
		}}
	})
	h.tree.SetDevGeometrySource(func(func(driver.GeometrySnapshot)) {})
	first := h.commit()
	id := h.find("reused").id
	shown.Set(false)
	h.commit()
	shown.Set(true)
	h.commit()
	if first.GeometryVersion != 0 || h.find("reused").id != id {
		t.Fatal("fixture did not reuse an unobserved native ID")
	}
	(&devSurface{t: h.tree}).Elements()
	h.tree.acceptDevGeometry(driver.GeometrySnapshot{Bounds: map[NodeID]Bounds{id: {Origin: Point{X: 200}, Size: Size{Width: 40, Height: 20}}}})
	if got := h.tree.devGeometry()[id].Origin.X; got != 30 {
		t.Fatalf("unobserved earlier incarnation supplied bounds: x=%v", got)
	}
	barrier := h.commit()
	if barrier == nil || barrier.GeometryVersion == 0 || len(barrier.Items) != 0 {
		t.Fatalf("idle diagnostic read did not send a versioned barrier: %+v", barrier)
	}
	h.tree.acceptDevGeometry(driver.GeometrySnapshot{Version: barrier.GeometryVersion, Bounds: map[NodeID]Bounds{id: {Origin: Point{X: 90}, Size: Size{Width: 40, Height: 20}}}})
	if got := h.tree.devGeometry()[id].Origin.X; got != 90 {
		t.Fatalf("post-barrier native frame was rejected: x=%v", got)
	}
}

func TestNativeGeometryKeepsQuadOutlinesAndPicking(t *testing.T) {
	h := newHost(t, Size{Width: 300, Height: 250}, func(*Scope) Node {
		return Group{View{TestID: "moving", Style: Style{Width: Pt(40), Height: Pt(20)}}, audit}
	})
	h.tree.SetDevGeometrySource(func(func(driver.GeometrySnapshot)) {})
	batch := h.commit()
	n := h.find("moving")
	frame := h.tree.devCommittedGeometryFrame()
	matrix := driver.Translate3D(110, 80, 0).Mul(driver.Transform{Rotate: 35, Perspective: 300}.Matrix(Point{X: 20, Y: 10}))
	frame.Version = batch.GeometryVersion
	frame.Matrices[n.id], frame.Sizes[n.id] = matrix, Size{Width: 40, Height: 20}
	frame.Bounds[n.id] = transformBounds(matrix, Bounds{Size: frame.Sizes[n.id]})
	h.tree.acceptDevGeometry(frame)
	found := false
	for _, box := range h.tree.auditBoxes(nil) {
		if box.matrix == matrix && box.size == frame.Sizes[n.id] {
			found = true
		}
	}
	if !found {
		t.Fatal("native presentation matrix was lost from the audit's quad")
	}
	x, y := matrix.Apply(5, 5)
	id, local := h.tree.hitDevWith(h.tree.view, Point{X: Points(x), Y: Points(y)}, func(candidate *node) bool { return candidate == n })
	if id != n.id || math.Abs(float64(local.X-5)) > 1e-4 || math.Abs(float64(local.Y-5)) > 1e-4 {
		t.Fatalf("native pick = %v,%v; want %v at 5,5", id, local, n.id)
	}
	if h.tree.devDrawn != nil {
		t.Fatal("native feedback retained a duplicate animation tracker")
	}
}
