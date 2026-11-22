package core_test

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func approxRect(t *testing.T, name string, got, want [4]float32, eps float32) {
	t.Helper()
	for i := range 4 {
		if float32(math.Abs(float64(got[i]-want[i]))) > eps {
			t.Errorf("%s = %v, want %v (±%v)", name, got, want, eps)
			return
		}
	}
}

func startDevSurface(t *testing.T, view func(*core.Scope) core.Node, opts ...nsgotest.Option) (*nsgotest.App, devtools.Surface) {
	t.Helper()
	devtools.Enabled = true
	t.Cleanup(func() { devtools.Enabled = false })
	app := nsgotest.Start(t, view, opts...)
	surfaces := devtools.Surfaces()
	if len(surfaces) == 0 {
		t.Fatal("surface did not register with devtools")
	}
	s := surfaces[len(surfaces)-1]
	t.Cleanup(func() { devtools.Unregister(s) })
	return app, s
}

func elementBounds(t *testing.T, app *nsgotest.App, s devtools.Surface, testID string) [4]float32 {
	t.Helper()
	var root devtools.Node
	app.Post(func() { root = s.Elements() })
	app.Settle()
	n := find(root, func(n devtools.Node) bool { return n.TestID == testID })
	if n == nil {
		t.Fatalf("no element %q in %+v", testID, root)
	}
	return n.Bounds
}

func TestDevToolsElementsFollowStaticTransforms(t *testing.T) {
	app, s := startDevSurface(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			core.View{TestID: "tx", Style: core.Style{
				Position: core.Absolute, Left: core.Pt(10), Top: core.Pt(20), Width: core.Pt(50), Height: core.Pt(30),
				Transform: core.Transform{TranslateX: 35, TranslateY: -5},
			}},
			core.View{TestID: "scaleOrigin", Style: core.Style{
				Position: core.Absolute, Left: core.Pt(100), Top: core.Pt(100), Width: core.Pt(40), Height: core.Pt(20),
				TransformOrigin: core.ObjectPosition{X: core.PositionStart, Y: core.PositionStart},
				Transform:       core.Transform{ScaleX: 2, ScaleY: 3},
			}},
			core.View{TestID: "rot", Style: core.Style{
				Position: core.Absolute, Left: core.Pt(200), Top: core.Pt(100), Width: core.Pt(60), Height: core.Pt(20),
				Transform: core.Transform{Rotate: 90},
			}},
		}}
	}, nsgotest.Size(300, 300))

	approxRect(t, "tx", elementBounds(t, app, s, "tx"), [4]float32{45, 15, 50, 30}, 0.05)
	// Scale around top-left (100, 100): stays at (100, 100), size becomes 80x60.
	approxRect(t, "scaleOrigin", elementBounds(t, app, s, "scaleOrigin"), [4]float32{100, 100, 80, 60}, 0.05)
	// Rotate 90° around center (230, 110): size swaps to 20x60 centered at (230, 110) => (220, 80, 20, 60).
	approxRect(t, "rot", elementBounds(t, app, s, "rot"), [4]float32{220, 80, 20, 60}, 0.05)
}

func TestDevToolsElementsFollowAnimatedTransforms(t *testing.T) {
	var animX *core.AnimatedValue
	var scope *core.Scope
	app, s := startDevSurface(t, func(cx *core.Scope) core.Node {
		scope = cx
		animX = core.NewAnimatedValue(cx, 10)
		scrollY := core.NewAnimatedValue(cx, 0)
		parallax := core.Interpolate(scrollY, core.Interpolation{
			Input: []float64{0, 100}, Output: []float64{0, 50},
		})
		return core.Group{
			core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
				core.View{TestID: "anim", Style: core.Style{
					Position: core.Absolute, Left: core.Pt(20), Top: core.Pt(20), Width: core.Pt(40), Height: core.Pt(20),
					Animated: &core.AnimatedStyle{TranslateX: animX},
				}},
				core.View{TestID: "banner", Style: core.Style{
					Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(60), Width: core.Pt(200), Height: core.Pt(30),
					Animated: &core.AnimatedStyle{TranslateY: parallax},
				}},
				core.Scroll{TestID: "scroll", OffsetValues: core.AnimatedXY{Y: scrollY}, Style: core.Style{
					Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(100), Width: core.Pt(200), Height: core.Pt(100),
				}, Children: []core.Node{
					core.View{Style: core.Style{Height: core.Pt(400)}},
				}},
			}},
			core.UpdateHighlights{NoUpdates: true, Layout: core.LayoutAudit{Bounds: true}},
		}
	}, nsgotest.Size(300, 300), nsgotest.ManualFrames())

	// Initial bound value animX = 10 => X = 20 + 10 = 30.
	approxRect(t, "anim initial", elementBounds(t, app, s, "anim"), [4]float32{30, 20, 40, 20}, 0.05)

	// Start a 200ms linear animation from 10 to 110.
	app.Post(func() {
		scope.StartAnimation(core.Timing(animX, 110, core.TimingConfig{Duration: 200 * time.Millisecond}), nil)
	})
	app.Settle()

	// Advance halfway (100ms): animX = 60 => X = 20 + 60 = 80.
	app.Advance(100 * time.Millisecond)
	approxRect(t, "anim mid", elementBounds(t, app, s, "anim"), [4]float32{80, 20, 40, 20}, 1.0)

	// Advance to end (100ms more): animX = 110 => X = 20 + 110 = 130.
	app.Advance(100 * time.Millisecond)
	approxRect(t, "anim end", elementBounds(t, app, s, "anim"), [4]float32{130, 20, 40, 20}, 0.05)

	// Scroll bound value moves banner by parallax (scrollY=80 => TranslateY=40 => Y=60+40=100).
	app.ScrollTo("id:scroll", core.Point{Y: 80})
	approxRect(t, "banner parallax", elementBounds(t, app, s, "banner"), [4]float32{0, 100, 200, 30}, 0.05)
}

func TestDevToolsElementsFollowStickyListAndScrollHeaders(t *testing.T) {
	app, s := startDevSurface(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			core.List{
				TestID:              "list",
				Count:               func() int { return 20 },
				Key:                 func(i int) any { return i },
				EstimatedSize:       40,
				StickyHeaderIndices: []int{0, 5},
				StickyHeaderOffset:  10,
				Style:               core.Style{Width: core.Pt(200), Height: core.Pt(160)},
				Row: func(cx *core.Scope, row core.ListRow) core.Node {
					i := row.Index()
					return core.View{
						TestID: "row-" + strconv.Itoa(i),
						Style:  core.Style{Height: core.Pt(40)},
						Children: []core.Node{
							core.Text{Content: "Row " + strconv.Itoa(i)},
						},
					}
				},
			},
			core.Scroll{
				TestID:     "hscroll",
				Horizontal: true,
				Style:      core.Style{Width: core.Pt(200), Height: core.Pt(60)},
				Children: []core.Node{
					core.View{Style: core.Style{Direction: core.Row, Width: core.Pt(500), Height: core.Pt(60)}, Children: []core.Node{
						core.View{Style: core.Style{Width: core.Pt(50), Height: core.Pt(60)}},
						core.View{TestID: "hsticky", Style: core.Style{Position: core.Sticky, Left: core.Pt(12), Width: core.Pt(40), Height: core.Pt(60)}},
					}},
				},
			},
		}}
	}, nsgotest.Size(200, 300))

	// Scroll list by 90pt: row-0 (sticky header at offset 10) stays pinned at Y=10.
	app.ScrollTo("id:list", core.Point{Y: 90})
	approxRect(t, "sticky list header 0", elementBounds(t, app, s, "row-0"), [4]float32{0, 10, 200, 40}, 0.5)

	// Scroll horizontal scroll view by 100pt: hsticky pins at X=12, Y=160.
	app.ScrollTo("id:hscroll", core.Point{X: 100})
	approxRect(t, "horizontal sticky", elementBounds(t, app, s, "hsticky"), [4]float32{12, 160, 40, 60}, 0.5)
}

func TestDevToolsElementsFollowLayoutTransitionsAndMatch(t *testing.T) {
	var expanded *core.Signal[bool]
	var flipped *core.Signal[bool]
	var scope *core.Scope
	app, s := startDevSurface(t, func(cx *core.Scope) core.Node {
		scope = cx
		expanded = core.NewSignal(cx, false)
		flipped = core.NewSignal(cx, false)
		return core.Group{
			core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
				core.View{TestID: "grow", StyleFunc: func() core.Style {
					w := core.Points(40)
					if expanded.Get() {
						w = 140
					}
					return core.Style{
						Width: core.Pt(w), Height: core.Pt(20),
						Transition: core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionLayout},
					}
				}},
				core.Show{
					When: flipped.Get,
					Then: func(cx *core.Scope) core.Node {
						return core.View{TestID: "hero-b", MatchID: "hero", Style: core.Style{
							Position: core.Absolute, Left: core.Pt(120), Top: core.Pt(100), Width: core.Pt(60), Height: core.Pt(40),
						}}
					},
					Else: func(cx *core.Scope) core.Node {
						return core.View{TestID: "hero-a", MatchID: "hero", Style: core.Style{
							Position: core.Absolute, Left: core.Pt(20), Top: core.Pt(100), Width: core.Pt(40), Height: core.Pt(40),
						}}
					},
				},
			}},
			core.UpdateHighlights{NoUpdates: true, Layout: core.LayoutAudit{Bounds: true}},
		}
	}, nsgotest.Size(300, 300), nsgotest.ManualFrames())

	// TransitionLayout from width 40 to 140 over 200ms.
	app.Post(func() { expanded.Set(true) })
	app.Settle()
	approxRect(t, "grow at t=0", elementBounds(t, app, s, "grow"), [4]float32{0, 0, 40, 20}, 0.5)
	app.Advance(100 * time.Millisecond)
	approxRect(t, "grow at t=100ms", elementBounds(t, app, s, "grow"), [4]float32{0, 0, 90, 20}, 1.0)
	app.Advance(100 * time.Millisecond)
	approxRect(t, "grow at t=200ms", elementBounds(t, app, s, "grow"), [4]float32{0, 0, 140, 20}, 0.5)

	// Matched geometry transition from hero-a (20, 100, 40, 40) to hero-b (120, 100, 60, 40).
	app.Post(func() {
		scope.AnimateLayout(core.LayoutAnimation{
			Update: &core.LayoutTransition{Curve: core.LayoutLinear, Duration: 200 * time.Millisecond},
		})
		flipped.Set(true)
	})
	app.Settle()
	approxRect(t, "hero-b at t=0", elementBounds(t, app, s, "hero-b"), [4]float32{20, 100, 40, 40}, 0.5)
	app.Advance(100 * time.Millisecond)
	approxRect(t, "hero-b at t=100ms", elementBounds(t, app, s, "hero-b"), [4]float32{70, 100, 50, 40}, 1.0)
	app.Advance(100 * time.Millisecond)
	approxRect(t, "hero-b at t=200ms", elementBounds(t, app, s, "hero-b"), [4]float32{120, 100, 60, 40}, 0.5)
}

func TestDevToolsElementsSnapUnderReduceMotion(t *testing.T) {
	var tx *core.Signal[core.Points]
	var width *core.Signal[core.Points]
	var scope *core.Scope
	app, s := startDevSurface(t, func(cx *core.Scope) core.Node {
		scope = cx
		tx = core.NewSignal[core.Points](cx, 0)
		width = core.NewSignal[core.Points](cx, 40)
		return core.View{TestID: "box", StyleFunc: func() core.Style {
			return core.Style{
				Width: core.Pt(width.Get()), Height: core.Pt(20),
				Transform:  core.Transform{TranslateX: tx.Get()},
				Transition: core.Transition{Duration: 300 * time.Millisecond, Properties: core.TransitionTransform | core.TransitionLayout},
			}
		}}
	}, nsgotest.Size(300, 200), nsgotest.ManualFrames(), nsgotest.Environment(core.Environment{ReduceMotion: true}))

	app.Post(func() {
		scope.AnimateLayout(core.LayoutLinearPreset)
		tx.Set(80)
		width.Set(120)
	})
	app.Settle()
	// With ReduceMotion, drawn bounds snap immediately to final frame + transform.
	approxRect(t, "box under ReduceMotion", elementBounds(t, app, s, "box"), [4]float32{80, 0, 120, 20}, 0.05)
}
