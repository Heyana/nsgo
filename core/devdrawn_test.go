package core

import (
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func approxEq(a, b, eps Points) bool {
	return Points(math.Abs(float64(a-b))) <= eps
}

func approxBounds(t *testing.T, name string, got, want Bounds, eps Points) {
	t.Helper()
	if !approxEq(got.Origin.X, want.Origin.X, eps) ||
		!approxEq(got.Origin.Y, want.Origin.Y, eps) ||
		!approxEq(got.Size.Width, want.Size.Width, eps) ||
		!approxEq(got.Size.Height, want.Size.Height, eps) {
		t.Errorf("%s = %+v, want %+v (±%v)", name, got, want, eps)
	}
}

func TestAuditBoxesFollowStaticAndNestedTransforms(t *testing.T) {
	var tx *Signal[Points]
	h := newHost(t, Size{Width: 300, Height: 300}, func(cx *Scope) Node {
		tx = NewSignal[Points](cx, 25)
		return Group{
			View{Style: Style{FlexGrow: 1}, Children: []Node{
				View{TestID: "moved", StyleFunc: func() Style {
					return Style{Width: Pt(60), Height: Pt(40), Transform: Transform{TranslateX: tx.Get(), TranslateY: 10}}
				}},
				View{TestID: "scaled", Style: Style{
					Position: Absolute, Left: Pt(100), Top: Pt(100), Width: Pt(40), Height: Pt(20),
					Transform: Transform{Scale: 2},
				}},
				View{TestID: "rotated", Style: Style{
					Position: Absolute, Left: Pt(100), Top: Pt(200), Width: Pt(40), Height: Pt(20),
					Transform: Transform{Rotate: 90},
				}},
				View{TestID: "parent", Style: Style{
					Position: Absolute, Left: Pt(20), Top: Pt(150), Width: Pt(80), Height: Pt(40),
					Transform: Transform{TranslateX: 10, TranslateY: 5},
				}, Children: []Node{
					View{TestID: "nested", Style: Style{
						Width: Pt(30), Height: Pt(20), Transform: Transform{TranslateX: 7, TranslateY: 3},
					}},
				}},
			}},
			audit,
		}
	})
	h.commit()
	boxes := h.tree.auditBoxes(nil)

	findBox := func(id string) auditBox {
		t.Helper()
		want := h.tree.drawnBounds(h.find(id))
		for _, b := range boxes {
			if b.bounds == want {
				return b
			}
		}
		t.Fatalf("no auditBox for %q at %v", id, want)
		return auditBox{}
	}

	approxBounds(t, "moved", findBox("moved").bounds, Bounds{Origin: Point{X: 25, Y: 10}, Size: Size{Width: 60, Height: 40}}, 0.01)
	// Scaled 2x about center (120, 110): 80x40 centered at (120, 110) => (80, 90, 80, 40).
	approxBounds(t, "scaled", findBox("scaled").bounds, Bounds{Origin: Point{X: 80, Y: 90}, Size: Size{Width: 80, Height: 40}}, 0.01)
	// Rotated 90° about center (120, 210): 20x40 centered at (120, 210) => (110, 190, 20, 40).
	approxBounds(t, "rotated", findBox("rotated").bounds, Bounds{Origin: Point{X: 110, Y: 190}, Size: Size{Width: 20, Height: 40}}, 0.01)
	// Nested: parent at (20+10, 150+5) = (30, 155), child translated (7, 3) => (37, 158, 30, 20).
	approxBounds(t, "nested", findBox("nested").bounds, Bounds{Origin: Point{X: 37, Y: 158}, Size: Size{Width: 30, Height: 20}}, 0.01)

	// Changing only a static transform redraws the audit canvas in the same commit.
	tx.Set(60)
	if b := h.commit(); h.drawings(b, "nsgo-layout-canvas") != 1 {
		t.Fatalf("changing Transform sent %v, want one audit redraw", kinds(b))
	}
	approxBounds(t, "moved after update", h.tree.drawnBounds(h.find("moved")), Bounds{Origin: Point{X: 60, Y: 10}, Size: Size{Width: 60, Height: 40}}, 0.01)
}

func TestAuditBoxesFollowStickyOffsetsAndAnimatedInset(t *testing.T) {
	var inset *AnimatedValue
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		inset = NewAnimatedValue(cx, 10)
		return Group{
			Scroll{TestID: "scroll", Style: Style{Width: Pt(200), Height: Pt(120)}, Children: []Node{
				View{TestID: "section", Style: Style{Height: Pt(300)}, Children: []Node{
					View{Style: Style{Height: Pt(40)}},
					View{TestID: "sticky", Style: Style{Position: Sticky, Top: Pt(10), Height: Pt(30)}, Children: []Node{
						View{TestID: "inside", Style: Style{Margin: Edges{Left: 8, Top: 4}, Width: Pt(50), Height: Pt(20)}},
					}},
					View{TestID: "animSticky", Style: Style{
						Position: Sticky, Height: Pt(20),
						Animated: &AnimatedStyle{StickyInset: inset},
					}},
				}},
			}},
			audit,
		}
	})
	h.commit()
	scroll := h.find("scroll")

	// Before reaching sticky threshold (layout Y=40, top=10 => pins at scroll Y >= 30).
	h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: scroll.id, Point: Point{Y: 20}})
	h.commit()
	approxBounds(t, "sticky before pin", h.tree.drawnBounds(h.find("sticky")), Bounds{Origin: Point{Y: 20}, Size: Size{Width: 200, Height: 30}}, 0.01)

	// Scrolled past threshold: sticky stays pinned at Y=10 and its child moves with it.
	h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: scroll.id, Point: Point{Y: 100}})
	h.commit()
	approxBounds(t, "sticky pinned", h.tree.drawnBounds(h.find("sticky")), Bounds{Origin: Point{Y: 10}, Size: Size{Width: 200, Height: 30}}, 0.01)
	approxBounds(t, "child inside sticky", h.tree.drawnBounds(h.find("inside")), Bounds{Origin: Point{X: 8, Y: 14}, Size: Size{Width: 50, Height: 20}}, 0.01)
	// animSticky: laid out at Y=70, animated inset=10 => pinned at Y=10.
	approxBounds(t, "animSticky at inset 10", h.tree.drawnBounds(h.find("animSticky")), Bounds{Origin: Point{Y: 10}, Size: Size{Width: 200, Height: 20}}, 0.01)

	// Moving the animated sticky inset updates the pinned view's bounds and redraws the audit.
	inset.Set(25)
	if b := h.commit(); h.drawings(b, "nsgo-layout-canvas") != 1 {
		t.Fatalf("updating StickyInset sent %v, want one audit redraw", kinds(b))
	}
	approxBounds(t, "animSticky at inset 25", h.tree.drawnBounds(h.find("animSticky")), Bounds{Origin: Point{Y: 25}, Size: Size{Width: 200, Height: 20}}, 0.01)
}

func TestAuditBoxesFollowLayoutAnimationAndRequestFrames(t *testing.T) {
	clock := NewManualClock(time.Unix(1000, 0))
	frameWanted := 0
	var width *Signal[Points]
	var scope *Scope
	root := NewRoot(RootOptions{
		CheckCommits: true,
		Post:         func(fn func()) { fn() },
		Clock:        clock,
		RequestFrame: func() { frameWanted++ },
	})
	tree := NewTree(root, ApproxTextEngine{})
	tree.Resize(Size{Width: 200, Height: 200}, Geometry{})
	tree.Mount(func(cx *Scope) Node {
		scope = cx
		width = NewSignal[Points](cx, 40)
		return Group{
			View{TestID: "box", StyleFunc: func() Style { return Style{Width: Pt(width.Get()), Height: Pt(20)} }},
			audit,
		}
	})
	tree.Commit()
	box := findByTestID(tree.view, "box")
	if frameWanted != 0 {
		t.Fatalf("idle audit requested %d frames, want 0", frameWanted)
	}

	// Start a 200ms linear layout animation widening box from 40 to 140.
	scope.AnimateLayout(LayoutAnimation{
		Update: &LayoutTransition{Curve: LayoutLinear, Duration: 200 * time.Millisecond},
	})
	width.Set(140)
	tree.Commit()
	if frameWanted == 0 {
		t.Fatal("layout animation did not request a display frame for the audit")
	}
	// At t=0, drawn width is still 40 (start of move).
	approxBounds(t, "box at t=0", tree.drawnBounds(box), Bounds{Size: Size{Width: 40, Height: 20}}, 0.01)

	// Halfway through (t=100ms): drawn width is 90 and audit redraws.
	clock.Advance(100 * time.Millisecond)
	root.RunFrame()
	if b := tree.Commit(); b == nil {
		t.Fatal("expected audit redraw commit at t=100ms")
	}
	approxBounds(t, "box at t=100ms", tree.drawnBounds(box), Bounds{Size: Size{Width: 90, Height: 20}}, 0.01)

	// At t=200ms: animation settles at 140 and stops requesting frames.
	beforeWanted := frameWanted
	clock.Advance(100 * time.Millisecond)
	root.RunFrame()
	tree.Commit()
	approxBounds(t, "box at t=200ms", tree.drawnBounds(box), Bounds{Size: Size{Width: 140, Height: 20}}, 0.01)
	if frameWanted != beforeWanted {
		t.Fatalf("settled layout animation requested another frame (%d -> %d)", beforeWanted, frameWanted)
	}
}

func TestAuditBoxesFollowTransformTransitionWithDelayAndInterruption(t *testing.T) {
	clock := NewManualClock(time.Unix(1000, 0))
	var tx *Signal[Points]
	root := NewRoot(RootOptions{
		CheckCommits: true,
		Post:         func(fn func()) { fn() },
		Clock:        clock,
		RequestFrame: func() {},
	})
	tree := NewTree(root, ApproxTextEngine{})
	tree.Resize(Size{Width: 300, Height: 200}, Geometry{})
	tree.Mount(func(cx *Scope) Node {
		tx = NewSignal[Points](cx, 0)
		return Group{
			View{TestID: "card", StyleFunc: func() Style {
				return Style{
					Width: Pt(50), Height: Pt(30),
					Transform: Transform{TranslateX: tx.Get()},
					Transition: Transition{
						Delay:      100 * time.Millisecond,
						Duration:   200 * time.Millisecond,
						Properties: TransitionTransform,
					},
				}
			}},
			audit,
		}
	})
	tree.Commit()
	card := findByTestID(tree.view, "card")

	tx.Set(100)
	tree.Commit()
	// Still in 100ms delay at t=50ms.
	clock.Advance(50 * time.Millisecond)
	tree.stepDevDrawn()
	approxBounds(t, "card in delay", tree.drawnBounds(card), Bounds{Origin: Point{X: 0}, Size: Size{Width: 50, Height: 30}}, 0.01)

	// At t=200ms (100ms into 200ms duration): halfway at X=50.
	clock.Advance(150 * time.Millisecond)
	tree.stepDevDrawn()
	approxBounds(t, "card halfway", tree.drawnBounds(card), Bounds{Origin: Point{X: 50}, Size: Size{Width: 50, Height: 30}}, 0.01)

	// Interrupt at X=50 toward X=150 (with same 100ms delay + 200ms duration).
	tx.Set(150)
	tree.Commit()
	// Halfway through the new transition (100ms delay + 100ms duration = 200ms later): halfway from 50 to 150 => 100.
	clock.Advance(200 * time.Millisecond)
	tree.stepDevDrawn()
	approxBounds(t, "card interrupted halfway", tree.drawnBounds(card), Bounds{Origin: Point{X: 100}, Size: Size{Width: 50, Height: 30}}, 0.01)

	// Finish at +100ms => 150.
	clock.Advance(100 * time.Millisecond)
	tree.stepDevDrawn()
	approxBounds(t, "card settled", tree.drawnBounds(card), Bounds{Origin: Point{X: 150}, Size: Size{Width: 50, Height: 30}}, 0.01)
}

func TestAuditBoxesFollowLayoutScaleCreateAndKeyframes(t *testing.T) {
	clock := NewManualClock(time.Unix(1000, 0))
	var show *Signal[bool]
	var scope *Scope
	root := NewRoot(RootOptions{
		CheckCommits: true,
		Post:         func(fn func()) { fn() },
		Clock:        clock,
		RequestFrame: func() {},
	})
	tree := NewTree(root, ApproxTextEngine{})
	tree.Resize(Size{Width: 200, Height: 200}, Geometry{})
	tree.Mount(func(cx *Scope) Node {
		scope = cx
		show = NewSignal(cx, false)
		return Group{
			View{Style: Style{FlexGrow: 1}, Children: []Node{
				View{TestID: "bouncer", Style: Style{
					Position: Absolute, Left: Pt(10), Top: Pt(100), Width: Pt(40), Height: Pt(40),
					Animation: KeyframeAnimation{Keyframes: KeyframesBounce, Duration: 400 * time.Millisecond},
				}},
				Show{When: show.Get, Then: func(cx *Scope) Node {
					return View{TestID: "pop", Style: Style{
						Position: Absolute, Left: Pt(80), Top: Pt(80), Width: Pt(40), Height: Pt(40),
					}}
				}},
			}},
			audit,
		}
	})
	tree.Commit()
	bouncer := findByTestID(tree.view, "bouncer")
	// KeyframesBounce starts at TranslateY = -0.25 * height = -10 at t=0, drops to 0 at half-cycle (200ms).
	approxBounds(t, "bouncer at t=0", tree.drawnBounds(bouncer), Bounds{Origin: Point{X: 10, Y: 90}, Size: Size{Width: 40, Height: 40}}, 0.5)
	clock.Advance(200 * time.Millisecond)
	tree.stepDevDrawn()
	approxBounds(t, "bouncer at half cycle", tree.drawnBounds(bouncer), Bounds{Origin: Point{X: 10, Y: 100}, Size: Size{Width: 40, Height: 40}}, 0.5)

	// Insert "pop" with LayoutScale Create transition (200ms linear).
	scope.AnimateLayout(LayoutAnimation{
		Create: &LayoutTransition{Curve: LayoutLinear, Property: LayoutScale, Duration: 200 * time.Millisecond},
	})
	show.Set(true)
	tree.Commit()
	pop := findByTestID(tree.view, "pop")
	// At t=0, scale is 0 around center (100, 100).
	approxBounds(t, "pop at t=0", tree.drawnBounds(pop), Bounds{Origin: Point{X: 100, Y: 100}, Size: Size{}}, 0.01)
	// At t=100ms, scale is 0.5 around center (100, 100) => (90, 90, 20, 20).
	clock.Advance(100 * time.Millisecond)
	tree.stepDevDrawn()
	approxBounds(t, "pop at t=100ms", tree.drawnBounds(pop), Bounds{Origin: Point{X: 90, Y: 90}, Size: Size{Width: 20, Height: 20}}, 0.01)
	// At t=200ms, full size (80, 80, 40, 40).
	clock.Advance(100 * time.Millisecond)
	tree.stepDevDrawn()
	approxBounds(t, "pop at t=200ms", tree.drawnBounds(pop), Bounds{Origin: Point{X: 80, Y: 80}, Size: Size{Width: 40, Height: 40}}, 0.01)
}

func TestAuditIgnoresOpacityOnlyAnimationsForFrameTicking(t *testing.T) {
	clock := NewManualClock(time.Unix(1000, 0))
	frameWanted := 0
	root := NewRoot(RootOptions{
		CheckCommits: true,
		Post:         func(fn func()) { fn() },
		Clock:        clock,
		RequestFrame: func() { frameWanted++ },
	})
	tree := NewTree(root, ApproxTextEngine{})
	tree.Resize(Size{Width: 200, Height: 200}, Geometry{})
	tree.Mount(func(cx *Scope) Node {
		op := NewAnimatedValue(cx, 1)
		cx.StartAnimation(Loop(Timing(op, 0, TimingConfig{Duration: 500 * time.Millisecond}), 0), nil)
		return Group{
			View{TestID: "pulse", Style: Style{
				Width: Pt(40), Height: Pt(40),
				Animated:  &AnimatedStyle{Opacity: op},
				Animation: KeyframeAnimation{Keyframes: KeyframesPulse},
			}},
			audit,
		}
	})
	tree.Commit()
	if frameWanted != 0 {
		t.Fatalf("opacity-only animations requested %d audit frames, want 0", frameWanted)
	}
}

func TestPickHitsTransformedAndStickyViewsWhereTheyDraw(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		return Group{
			View{Style: Style{FlexGrow: 1}, Children: []Node{
				View{TestID: "shifted", Style: Style{
					Position: Absolute, Left: Pt(0), Top: Pt(0), Width: Pt(40), Height: Pt(40),
					Transform: Transform{TranslateX: 100, TranslateY: 50},
				}},
				Scroll{TestID: "scroll", Style: Style{Position: Absolute, Left: Pt(0), Top: Pt(100), Width: Pt(200), Height: Pt(100)}, Children: []Node{
					View{Style: Style{Height: Pt(400)}, Children: []Node{
						View{Style: Style{Height: Pt(60)}},
						View{TestID: "header", Style: Style{Position: Sticky, Top: Pt(0), Height: Pt(30)}},
					}},
				}},
			}},
			UpdateHighlights{NoUpdates: true},
		}
	})
	h.commit()
	shifted := h.find("shifted")
	scroll := h.find("scroll")
	header := h.find("header")

	// Pick at drawn position (120, 70) hits shifted; layout position (20, 20) does not.
	var picked uint32
	h.tree.highlights.picked = func(id uint32) { picked = id }
	h.tree.highlights.picking.Set(true)
	h.tree.pickAt(Point{X: 120, Y: 70}, true)
	if NodeID(picked) != shifted.id {
		t.Fatalf("picked at drawn point = %d, want shifted (%d)", picked, shifted.id)
	}
	picked = 0
	h.tree.highlights.picking.Set(true)
	h.tree.pickAt(Point{X: 20, Y: 20}, true)
	if NodeID(picked) == shifted.id {
		t.Fatalf("picked at untransformed layout point hit shifted (%d)", shifted.id)
	}

	// Scroll down 120pt so sticky header pins at top of scroll view (Y=100..130).
	h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: scroll.id, Point: Point{Y: 120}})
	h.commit()
	picked = 0
	h.tree.highlights.picking.Set(true)
	h.tree.pickAt(Point{X: 50, Y: 115}, true)
	if NodeID(picked) != header.id {
		t.Fatalf("picked at pinned sticky position = %d, want header (%d)", picked, header.id)
	}
}

func TestPaintAuditStrokesTransformedQuadsAndHighlightInspectUseDrawnBounds(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		return Group{
			View{Style: Style{FlexGrow: 1}, Children: []Node{
				View{TestID: "rot", Style: Style{
					Position: Absolute, Left: Pt(60), Top: Pt(60), Width: Pt(40), Height: Pt(20),
					Margin: All(4), Padding: All(2),
					Transform: Transform{Rotate: 45},
				}},
			}},
			UpdateHighlights{Layout: LayoutAudit{Bounds: true}},
		}
	})
	h.commit()
	rot := h.find("rot")

	// Inspect uses drawn bounds (AABB of 45° rotated 40x20 box at (64, 64) after Margin All(4), centered at (84, 74)).
	ins := h.tree.inspected(rot)
	wantHalf := Points((40 + 20) * math.Sqrt2 / 4)
	approxBounds(t, "inspected rotated", ins.bounds, Bounds{
		Origin: Point{X: 84 - wantHalf, Y: 74 - wantHalf},
		Size:   Size{Width: 2 * wantHalf, Height: 2 * wantHalf},
	}, 0.1)

	// paintAudit draws transformed boxes as closed paths (MoveTo/LineTo/ClosePath/Stroke).
	boxes := h.tree.auditBoxes(nil)
	var d Drawing
	paintAudit(&d, boxes, Size{Width: 200, Height: 200}, 1, 0, measurement{})
	moveTos := 0
	for _, op := range d.Ops {
		if op.Kind == driver.DrawMoveTo {
			moveTos++
		}
	}
	// Margin, content, and border of "rot" each emit 1 MoveTo (3 total).
	if moveTos != 3 {
		t.Errorf("rotated box emitted %d MoveTo ops, want 3 (margin, content, border)", moveTos)
	}
}

func TestDevDrawnFollowsPanValuesAndFollowTransition(t *testing.T) {
	clock := NewManualClock(time.Unix(1000, 0))
	var tab *Signal[int]
	var r0, r1 *Ref
	var dragX *AnimatedValue
	root := NewRoot(RootOptions{
		CheckCommits: true,
		Post:         func(fn func()) { fn() },
		Clock:        clock,
		RequestFrame: func() {},
	})
	tree := NewTree(root, ApproxTextEngine{})
	tree.Resize(Size{Width: 200, Height: 200}, Geometry{})
	tree.Mount(func(cx *Scope) Node {
		tab = NewSignal(cx, 0)
		r0, r1 = new(Ref), new(Ref)
		dragX = NewAnimatedValue(cx, 0)
		return Group{
			View{Style: Style{FlexGrow: 1}, Children: []Node{
				View{Style: Style{Direction: Row, Height: Pt(40)}, Children: []Node{
					View{Ref: r0, Style: Style{Width: Pt(60), Height: Pt(40)}},
					View{Ref: r1, Style: Style{Width: Pt(60), Height: Pt(40)}},
					View{
						TestID: "indicator",
						Follow: func() *Ref {
							if tab.Get() == 0 {
								return r0
							}
							return r1
						},
						FollowTransition: &LayoutTransition{Curve: LayoutLinear, Duration: 200 * time.Millisecond},
						Style:            Style{Position: Absolute, Bottom: Pt(0), Height: Pt(4)},
					},
				}},
				View{
					TestID:    "draggable",
					PanValues: AnimatedXY{X: dragX},
					Style: Style{
						Width: Pt(40), Height: Pt(40),
						Animated: &AnimatedStyle{TranslateX: dragX},
					},
				},
			}},
			audit,
		}
	})
	tree.Commit()
	ind := findByTestID(tree.view, "indicator")
	drag := findByTestID(tree.view, "draggable")

	// Move follower from tab 0 (X=0) to tab 1 (X=60) over 200ms.
	tab.Set(1)
	tree.Commit()
	approxBounds(t, "indicator at t=0", tree.drawnBounds(ind), Bounds{Origin: Point{X: 0, Y: 36}, Size: Size{Width: 60, Height: 4}}, 0.01)
	clock.Advance(100 * time.Millisecond)
	tree.stepDevDrawn()
	approxBounds(t, "indicator at t=100ms", tree.drawnBounds(ind), Bounds{Origin: Point{X: 30, Y: 36}, Size: Size{Width: 60, Height: 4}}, 0.01)
	clock.Advance(100 * time.Millisecond)
	tree.stepDevDrawn()
	approxBounds(t, "indicator at t=200ms", tree.drawnBounds(ind), Bounds{Origin: Point{X: 60, Y: 36}, Size: Size{Width: 60, Height: 4}}, 0.01)

	// Pan gesture updates PanValues-bound drawn bounds and redraws audit.
	tree.Dispatch(driver.Event{Kind: driver.EventPan, ID: drag.id, Phase: GestureBegan, Point: Point{X: 0}})
	tree.Dispatch(driver.Event{Kind: driver.EventPan, ID: drag.id, Phase: GestureChanged, Point: Point{X: 45}})
	if b := tree.Commit(); b == nil {
		t.Fatal("pan moving PanValues did not redraw audit")
	}
	approxBounds(t, "draggable panned", tree.drawnBounds(drag), Bounds{Origin: Point{X: 45, Y: 40}, Size: Size{Width: 40, Height: 40}}, 0.01)
}

func findByTestID(n *node, id string) *node {
	if n == nil || n.deleted {
		return nil
	}
	if n.props.TestID == id {
		return n
	}
	for _, c := range n.children {
		if found := findByTestID(c, id); found != nil {
			return found
		}
	}
	return nil
}
