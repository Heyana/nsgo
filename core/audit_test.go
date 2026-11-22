package core

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// audit is DevTools' overlay showing every view's boxes, without outlining commits.
var audit = UpdateHighlights{NoUpdates: true, Layout: LayoutAudit{Bounds: true}}

// boxOf returns the box of the node with testID among boxes, failing when it is not there.
func (h *host) boxOf(boxes []auditBox, testID string) auditBox {
	h.t.Helper()
	b := h.tree.surfaceBounds(h.find(testID))
	for _, box := range boxes {
		if box.bounds == b {
			return box
		}
	}
	h.t.Fatalf("no box of %q at %v in %v", testID, b, boxes)
	return auditBox{}
}

func TestAuditBoxesPlaceEveryShownView(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 300}, func(cx *Scope) Node {
		return Group{
			View{TestID: "app", Style: Style{FlexGrow: 1, Padding: All(10), BorderWidth: 2}, Children: []Node{
				View{TestID: "child", Style: Style{Height: Pt(40), Margin: Edges{Top: 5, Left: 6}}},
				View{TestID: "gone", Style: Style{Height: Pt(40), Display: DisplayNone}},
				Overlay{TestID: "over", Style: Style{Top: Pt(100), Left: Pt(20), Width: Pt(50), Height: Pt(30)},
					Children: []Node{View{TestID: "inside", Style: Style{Height: Pt(10)}}}},
			}},
			audit,
		}
	})
	h.commit()
	boxes := h.tree.auditBoxes(nil)
	if len(boxes) != 5 {
		t.Fatalf("got %d boxes, want the root, app, child, overlay and what it holds, and none of DevTools' own: %v", len(boxes), boxes)
	}
	surface := Bounds{Size: Size{Width: 200, Height: 300}}
	for i, id := range []string{"app", "child", "over", "inside"} {
		if got, want := boxes[i+1].bounds, h.tree.surfaceBounds(h.find(id)); got != want {
			t.Errorf("box %d = %v, want %s's %v", i+1, got, id, want)
		}
		if boxes[i+1].clip != surface {
			t.Errorf("%s's clip = %v, want the surface", id, boxes[i+1].clip)
		}
	}
	app := boxes[1].box
	if app.Border != [4]float32{2, 2, 2, 2} || app.Padding != [4]float32{10, 10, 10, 10} {
		t.Errorf("app's edges = %v and %v, want its style's", app.Border, app.Padding)
	}
	if m := h.boxOf(boxes, "child").box.Margin; m != [4]float32{5, 0, 0, 6} {
		t.Errorf("child's margin = %v", m)
	}
}

func TestAuditBoxesLeaveOutWhatHiddenViewsHold(t *testing.T) {
	var vis *Signal[Visibility]
	h := newHost(t, Size{Width: 200, Height: 300}, func(cx *Scope) Node {
		vis = NewSignal(cx, VisibilityVisible)
		return Group{
			View{Style: Style{FlexGrow: 1}, Children: []Node{
				View{TestID: "parent", StyleFunc: func() Style { return Style{Height: Pt(40), Visibility: vis.Get()} }, Children: []Node{
					View{TestID: "kid", Style: Style{Width: Pt(30), Height: Pt(20), Visibility: VisibilityVisible}},
					Overlay{TestID: "floating", Style: Style{Width: Pt(10), Height: Pt(10)}},
				}},
			}},
			audit,
		}
	})
	h.commit()
	if got := len(h.tree.auditBoxes(nil)); got != 5 {
		t.Fatalf("got %d boxes, want the root's, its view's, the parent's, the kid's and the overlay's", got)
	}
	vis.Set(VisibilityHidden)
	h.commit()
	boxes := h.tree.auditBoxes(nil)
	if len(boxes) != 3 {
		t.Fatalf("got %v, want the boxes of a hidden view and of what it holds left out, as the native view hides them", boxes)
	}
	// The overlay is the root view's native child, which the parent does not hide.
	h.boxOf(boxes, "floating")
}

func TestAuditBoxesFollowScrollsAndClip(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 300}, func(cx *Scope) Node {
		return Group{
			View{Style: Style{FlexGrow: 1}, Children: []Node{
				Scroll{TestID: "scroll", Style: Style{Height: Pt(100)}, Children: []Node{
					View{TestID: "row", Style: Style{Height: Pt(400)}},
				}},
				View{TestID: "cut", Style: Style{Height: Pt(50), Width: Pt(80), Overflow: Hidden}, Children: []Node{
					View{TestID: "wide", Style: Style{Height: Pt(20), Width: Pt(300)}},
				}},
			}},
			audit,
		}
	})
	h.root.SetScrollbars(testBars{})
	h.commit()
	scroll := h.find("scroll")
	if scroll.scroll.barHost == nil {
		t.Fatal("no bar host")
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: scroll.id, Point: Point{Y: 100}})
	h.commit()
	boxes := h.tree.auditBoxes(nil)
	for _, b := range boxes {
		if b.bounds.Size == scroll.frame.Size && b.bounds != h.tree.surfaceBounds(scroll) {
			t.Errorf("a box of the viewport's size at %v: the bar's host", b.bounds)
		}
	}
	row := h.boxOf(boxes, "row")
	if row.bounds.Origin.Y != -100 {
		t.Errorf("row at %v, want moved up by the scroll", row.bounds)
	}
	if row.clip != h.tree.surfaceBounds(scroll) {
		t.Errorf("row's clip = %v, want the scroll view's bounds", row.clip)
	}
	cut := h.tree.surfaceBounds(h.find("cut"))
	if wide := h.boxOf(boxes, "wide"); wide.clip != cut {
		t.Errorf("wide's clip = %v, want its parent's %v", wide.clip, cut)
	}
}

type testBars struct{}

func (testBars) Scrollbar(cx *Scope, target *Ref, horizontal bool) Node {
	return View{Style: Style{Position: Absolute, Right: Pt(0), Width: Pt(4), Height: Pt(20)}}
}

// drawings counts the drawings a batch sends for the node with testID.
func (h *host) drawings(b *driver.MountBatch, testID string) int {
	id := h.find(testID).id
	n := 0
	for _, it := range b.Items {
		if it.Kind == driver.ItemCanvas && it.ID == id {
			n++
		}
	}
	return n
}

func TestAuditRedrawsWithinTheCommit(t *testing.T) {
	var width, padding *Signal[Points]
	var hidden *Signal[bool]
	h := newHost(t, Size{Width: 200, Height: 300}, func(cx *Scope) Node {
		width, padding = NewSignal[Points](cx, 50), NewSignal[Points](cx, 0)
		hidden = NewSignal(cx, false)
		return Group{
			View{Style: Style{FlexGrow: 1}, Children: []Node{
				View{TestID: "sibling", StyleFunc: func() Style { return Style{Width: Pt(width.Get()), Height: Pt(20), Padding: All(padding.Get())} }},
				View{StyleFunc: func() Style {
					if hidden.Get() {
						return Style{Height: Pt(10), Visibility: VisibilityHidden}
					}
					return Style{Height: Pt(10)}
				}},
				Scroll{TestID: "scroll", Style: Style{Height: Pt(100)}, Children: []Node{View{Style: Style{Height: Pt(400)}}}},
			}},
			audit,
		}
	})
	h.commit()
	a := h.tree.highlights.audit
	width.Set(70)
	if b := h.commit(); h.drawings(b, "nsgo-layout-canvas") != 1 {
		t.Fatalf("a resize sent %v, want one drawing in the same batch", kinds(b))
	}
	if got := h.boxOf(a.boxes, "sibling").bounds.Size.Width; got != 70 {
		t.Errorf("the drawing read a width of %v, want the new one", got)
	}
	boxes := len(a.boxes)
	if h.tree.Commit() != nil {
		t.Fatal("a commit after it sent more")
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: h.find("scroll").id, Point: Point{Y: 30}})
	if b := h.commit(); h.drawings(b, "nsgo-layout-canvas") != 1 {
		t.Fatalf("a scroll sent %v, want one drawing", kinds(b))
	}
	// Hiding a view and changing a padding move no frame, but change the boxes.
	hidden.Set(true)
	if b := h.commit(); h.drawings(b, "nsgo-layout-canvas") != 1 || len(a.boxes) != boxes-1 {
		t.Fatalf("hiding a view sent %v and read %d boxes, want one drawing of %d", kinds(b), len(a.boxes), boxes-1)
	}
	padding.Set(6)
	if b := h.commit(); h.drawings(b, "nsgo-layout-canvas") != 1 || h.boxOf(a.boxes, "sibling").box.Padding != [4]float32{6, 6, 6, 6} {
		t.Fatalf("a padding sent %v, want one drawing with it", kinds(b))
	}

	// With the boxes off, nothing reads them: a scroll commits nothing and a resize bumps nothing.
	a.bounds.Set(false)
	h.commit()
	if h.tree.auditRead() {
		t.Fatal("the boxes still have a reader")
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: h.find("scroll").id, Point: Point{Y: 60}})
	if h.tree.framesMoved {
		t.Error("a scroll asked for a round with no reader of the boxes")
	}
	if h.tree.Commit() != nil {
		t.Error("a scroll committed with no reader of the boxes")
	}
	bumps := a.frames.GetUntracked()
	width.Set(90)
	h.commit()
	if a.frames.GetUntracked() != bumps || h.tree.framesMoved {
		t.Error("a resize bumped the boxes' signal with no reader")
	}
}

func TestAuditMountsNothingUntilItShows(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 300}, func(cx *Scope) Node {
		return Group{View{Style: Style{FlexGrow: 1}}, UpdateHighlights{NoUpdates: true}}
	})
	h.commit()
	for _, n := range h.tree.nodes {
		if n != nil && !n.deleted && n.props.TestID == "nsgo-layout-audit" {
			t.Fatal("an audit that starts hidden mounts its layer")
		}
	}
	if h.tree.auditRead() {
		t.Error("a hidden audit reads the boxes")
	}
}

func TestPickRules(t *testing.T) {
	view := Size{Width: 200, Height: 100}
	var m measurement
	for _, p := range []Point{{X: Points(math.NaN()), Y: 1}, {X: Points(math.Inf(1)), Y: 1}, {X: -1, Y: 5}, {X: 5, Y: 101}} {
		if m = m.pick(p, view); m.points != 0 {
			t.Fatalf("picked %v", p)
		}
	}
	m = m.pick(Point{X: 150, Y: 80}, view)
	m = m.pick(Point{X: 50, Y: 20}, view)
	if m.points != 2 || m.horizontal() != 100 || m.vertical() != 60 {
		t.Fatalf("measured %+v: %v by %v, want 100 by 60 from reversed points", m, m.horizontal(), m.vertical())
	}
	if m = m.pick(Point{X: 10, Y: 10}, view); m.points != 1 || m.start != (Point{X: 10, Y: 10}) {
		t.Errorf("a third point gave %+v, want it to start anew", m)
	}
}

func TestAuditNumbersDropAWholeDecimal(t *testing.T) {
	for v, want := range map[Points]string{170: "170", 12.34: "12.3", 0.5: "0.5", 844: "844"} {
		if got := auditNumber(v); got != want {
			t.Errorf("auditNumber(%v) = %q, want %q", v, got, want)
		}
	}
}

func TestAuditGridStartsAtItsSmallestStep(t *testing.T) {
	cx := testRoot(RootOptions{})
	for grid, want := range map[Points]Points{0: 0, 2: minAuditGrid, 12: 12, Points(math.NaN()): 0, -8: 0} {
		if got := newLayoutAudit(cx, LayoutAudit{Grid: grid}).grid.GetUntracked(); got != want {
			t.Errorf("a grid of %v starts at %v, want %v", grid, got, want)
		}
	}
}

func TestAuditCursorStepMovementAndDisplay(t *testing.T) {
	cx := testRoot(RootOptions{})
	a := newLayoutAudit(cx, LayoutAudit{Bounds: true})
	vp := Size{Width: 100, Height: 60}
	if got := a.cursorStep(false); got != defaultAuditGrid {
		t.Fatalf("default step = %v, want %v", got, defaultAuditGrid)
	}
	if got := a.cursorStep(true); got != 1 {
		t.Fatalf("fine step = %v, want 1", got)
	}
	a.grid.Set(12)
	if got := a.cursorStep(false); got != 12 {
		t.Fatalf("grid step = %v, want 12", got)
	}
	// First move activates from viewport center (50, 30).
	a.moveCursor(1, 0, false, vp)
	if got := a.cursor.GetUntracked(); !got.active || got.at != (Point{X: 62, Y: 30}) {
		t.Fatalf("after right move: %+v, want active at (62, 30)", got)
	}
	// Fine step moves 1 point and clamping keeps it inside viewport.
	a.moveCursor(0, -1, true, vp)
	if got := a.cursor.GetUntracked().at; got != (Point{X: 62, Y: 29}) {
		t.Fatalf("after fine up: %v, want (62, 29)", got)
	}
	a.moveCursor(100, -100, false, vp)
	if got := a.cursor.GetUntracked().at; got != (Point{X: 100, Y: 0}) {
		t.Fatalf("clamped = %v, want (100, 0)", got)
	}
	// Displayed measurement previews live span while points == 1 and cursor is active.
	a.pickCursor(vp)
	a.moveCursor(-1, 1, false, vp)
	dm := displayedMeasurement(a.m.GetUntracked(), a.cursor.GetUntracked())
	if dm.points != 2 || dm.horizontal() != 12 || dm.vertical() != 12 {
		t.Fatalf("live displayedMeasurement = %+v (dx=%v dy=%v), want 12×12", dm, dm.horizontal(), dm.vertical())
	}
	// Pinning the second point locks the measurement even if the cursor moves later.
	a.pickCursor(vp)
	a.moveCursor(-1, 0, false, vp)
	dm = displayedMeasurement(a.m.GetUntracked(), a.cursor.GetUntracked())
	if dm.points != 2 || dm.horizontal() != 12 || dm.vertical() != 12 {
		t.Fatalf("locked displayedMeasurement = %+v (dx=%v dy=%v), want 12×12", dm, dm.horizontal(), dm.vertical())
	}
}

func TestAuditCursorClampsOnResize(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 300}, func(cx *Scope) Node {
		return Group{View{Style: Style{FlexGrow: 1}}, audit}
	})
	h.tree.SetEnvironment(Environment{OS: OSIOS, Idiom: IdiomTV})
	h.commit()
	a := h.tree.highlights.audit
	a.toggleRuler()
	h.commit()
	if c := a.cursor.GetUntracked(); !c.active || c.at != (Point{X: 100, Y: 150}) {
		t.Fatalf("initial remote cursor = %+v, want active at (100, 150)", c)
	}
	a.moveCursor(12, 18, false, Size{Width: 200, Height: 300})
	h.commit()
	if got := a.cursor.GetUntracked().at; got != (Point{X: 196, Y: 294}) {
		t.Fatalf("moved cursor = %v, want (196, 294)", got)
	}
	h.tree.Resize(Size{Width: 120, Height: 80}, Geometry{})
	h.commit()
	if got := a.cursor.GetUntracked(); !got.active || got.at != (Point{X: 120, Y: 80}) {
		t.Fatalf("resized cursor = %+v, want clamped to (120, 80)", got)
	}
}
