package flow

import (
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/camera"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// card is a node's face of a fixed width, as a styled card is.
func card(title string, width core.Points) core.Node {
	return core.View{Style: core.Style{Width: core.Pt(width), Padding: core.All(12), Gap: 4},
		Children: []core.Node{core.Text{Content: title}, core.Text{Content: title + " details"}}}
}

func sample() Data {
	return Data{
		Nodes: []Node{
			{ID: "plan", Label: "Plan", Content: card("Plan", 240)},
			{ID: "build", Label: "Build", Content: card("Build", 240)},
			{ID: "test", Label: "Test", Content: card("Test", 240)},
			{ID: "ship", Label: "Ship", Muted: true, Content: card("Ship", 240)},
			{ID: "fixed", Label: "Fixed", Width: 90, Height: 40},
		},
		Edges: []Edge{
			{Source: "plan", Target: "build"},
			{Source: "plan", Target: "test", Dashed: true, Label: "depends on"},
			{Source: "build", Target: "ship"},
			{Source: "test", Target: "ship"},
			{Source: "ship", Target: "ship"},
			{Source: "ship", Target: "missing"},
		},
	}
}

type harness struct {
	app           *nsgotest.App
	state         State
	cam           camera.State
	data          *core.Signal[int]
	current       Data
	selected      *core.Signal[string]
	picks, opened []string
}

// newHarness shows the sample on a camera that hands it the pointer.
func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{current: sample()}
	h.app = nsgotest.Start(t, func(cx *core.Scope) core.Node {
		h.data = core.NewSignal(cx, 0)
		h.selected = core.NewSignal(cx, "")
		return camera.View{TestID: "diagram", State: &h.cam, OnPress: h.state.Press, OnHover: h.state.Hover,
			Style: core.Style{Width: core.Pt(900), Height: core.Pt(700)},
			Content: View{Label: "Pipeline", State: &h.state, Camera: &h.cam,
				Data:     func() Data { h.data.Get(); return h.current },
				Selected: h.selected.Get,
				OnSelect: func(id string) { h.picks = append(h.picks, id); h.selected.Set(id) },
				OnOpen:   func(id string) { h.opened = append(h.opened, id) }}}
	}, nsgotest.Size(900, 700), nsgotest.Environment(driver.Environment{ReduceMotion: true, Idiom: driver.IdiomDesktop}))
	h.app.Settle()
	if !h.state.Ready() {
		t.Fatalf("diagram did not become ready\n%s", h.app.Dump())
	}
	return h
}

// change shows h.current again.
func (h *harness) change() {
	h.app.Post(func() { h.data.Update(func(n int) int { return n + 1 }) })
	h.app.Settle()
}

// at is where the middle of node id shows in the view.
func (h *harness) at(t *testing.T, id string) driver.Point {
	t.Helper()
	b, ok := h.state.Bounds(id)
	if !ok {
		t.Fatalf("%s is not laid out", id)
	}
	return h.cam.ToView(core.Point{X: b.Origin.X + b.Size.Width/2, Y: b.Origin.Y + b.Size.Height/2})
}

func TestViewMeasuresAndLaysOut(t *testing.T) {
	h := newHarness(t)
	v := h.state.v
	if len(v.nodes) != 5 || len(v.edges) != 4 {
		t.Fatalf("view kept %d nodes and %d edges", len(v.nodes), len(v.edges))
	}
	for _, n := range v.nodes {
		if !n.measured {
			t.Fatalf("node %s is unmeasured", n.id)
		}
	}
	if n := v.nodes[v.index["plan"]]; n.width != 240 || n.height <= 0 {
		t.Fatalf("card measured %vx%v", n.width, n.height)
	}
	if n := v.nodes[v.index["fixed"]]; n.width != 90 || n.height != 40 {
		t.Fatalf("fixed node measured %vx%v", n.width, n.height)
	}
	_, planY, _ := h.state.Position("plan")
	_, buildY, _ := h.state.Position("build")
	_, shipY, _ := h.state.Position("ship")
	if !(planY < buildY && buildY < shipY) {
		t.Fatalf("ranks at %v, %v, %v", planY, buildY, shipY)
	}
	if _, _, ok := h.state.Position("missing"); ok {
		t.Fatal("unknown node is laid out")
	}
	// The diagram is as large as its layout, which the camera fits.
	if got, want := h.app.Find("label:Pipeline").Frame.Size, (core.Size{Width: core.Points(v.result.width), Height: core.Points(v.result.height)}); got != want {
		t.Fatalf("the diagram is %v, its layout %v", got, want)
	}
	if z := h.cam.Zoom(); z <= 0 || z > 1 {
		t.Fatalf("fitted zoom %v", z)
	}
	if !h.app.Exists("label:Plan") {
		t.Fatalf("a node lacks its label\n%s", h.app.Dump())
	}
}

func TestViewSelectsAndOpens(t *testing.T) {
	h := newHarness(t)
	h.app.PressAt("id:diagram", h.at(t, "build"))
	if len(h.picks) != 1 || h.picks[0] != "build" {
		t.Fatalf("selected %v", h.picks)
	}
	if !h.app.Find("label:Build").Props.Selected {
		t.Fatalf("the selected node does not say so\n%s", h.app.Dump())
	}
	h.app.Advance(time.Second)
	test := h.at(t, "test")
	h.app.PressAt("id:diagram", test)
	h.app.PressAt("id:diagram", test)
	if len(h.opened) != 1 || h.opened[0] != "test" {
		t.Fatalf("opened %v", h.opened)
	}
	// Pressing empty space clears the selection without opening anything.
	h.app.Advance(time.Second)
	h.app.PressAt("id:diagram", driver.Point{X: 4, Y: 690})
	if last := h.picks[len(h.picks)-1]; last != "" || len(h.opened) != 1 {
		t.Fatalf("empty press selected %q and opened %v", last, h.opened)
	}
	// The pointer over a node emphasizes it.
	h.app.HoverAt("id:diagram-pointer", h.at(t, "ship"))
	if got := core.Untracked(h.state.v.cx, h.state.Hovered); got != "ship" {
		t.Fatalf("hovering found %q", got)
	}
	// Assistive technology and the keyboard press the node itself.
	h.app.AccessibilityAction("label:Plan", driver.ActionActivate)
	if last := h.picks[len(h.picks)-1]; last != "plan" {
		t.Fatalf("activating selected %q", last)
	}
	// Revealing a node centers it.
	h.app.Post(func() { h.state.Reveal("ship") })
	h.app.Settle()
	if p := h.at(t, "ship"); math.Abs(float64(p.X)-450) > 1 || math.Abs(float64(p.Y)-350) > 1 {
		t.Fatalf("the revealed node shows at %v", p)
	}
}

func TestViewRelaysOutWhenDataChanges(t *testing.T) {
	h := newHarness(t)
	h.current.Nodes = append(h.current.Nodes, Node{ID: "docs", Label: "Docs", Content: card("Docs", 240)})
	h.current.Edges = append(h.current.Edges, Edge{Source: "ship", Target: "docs"})
	h.change()
	_, shipY, _ := h.state.Position("ship")
	_, docsY, ok := h.state.Position("docs")
	if !ok || docsY <= shipY {
		t.Fatalf("new node at %v below %v", docsY, shipY)
	}
	if !h.state.v.nodes[h.state.v.index["docs"]].measured {
		t.Fatal("new node is unmeasured")
	}
	// A node given a new size is laid out again at it.
	h.current.Nodes[4].Width = 160
	h.change()
	if n := h.state.v.nodes[h.state.v.index["fixed"]]; n.width != 160 {
		t.Fatalf("resized node measured %v wide", n.width)
	}
	h.current = Data{}
	h.change()
	if !h.app.Exists("text:—") {
		t.Fatalf("empty state is missing\n%s", h.app.Dump())
	}
}

func TestViewShownAgainKeepsItsSizes(t *testing.T) {
	var state State
	shown := (*core.Signal[bool])(nil)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		shown = core.NewSignal(cx, true)
		return core.Show{When: shown.Get, Then: func(*core.Scope) core.Node { return View{State: &state, Data: sample} }}
	}, nsgotest.Size(900, 700), nsgotest.Environment(driver.Environment{ReduceMotion: true, Idiom: driver.IdiomDesktop}))
	app.Settle()
	sizes := len(state.sizes)
	app.Post(func() { shown.Set(false) })
	app.Settle()
	app.Post(func() { shown.Set(true) })
	app.Settle()
	if !state.Ready() || sizes == 0 || len(state.sizes) != sizes {
		t.Fatalf("diagram shown again measured %d sizes, had %d", len(state.sizes), sizes)
	}
}

// edgeInks returns the colors and alphas the edge canvas under the diagram strokes and fills with, in the order it paints, and the colors it fills with while it erases.
func edgeInks(t *testing.T, app *nsgotest.App) (strokes, fills, erases []ink) {
	t.Helper()
	var canvas *nsgotest.View
	var find func(v *nsgotest.View)
	find = func(v *nsgotest.View) {
		if v.Kind == nsgotest.KindCanvas && canvas == nil {
			canvas = v
		}
		for _, c := range v.Children {
			find(c)
		}
	}
	find(app.Find("label:Pipeline"))
	if canvas == nil || len(canvas.Ops) == 0 {
		t.Fatalf("no edge canvas drew\n%s", app.Dump())
	}
	type state struct {
		alpha     float32
		composite driver.CompositeOp
	}
	st, stack := state{alpha: 1}, []state(nil)
	for _, op := range canvas.Ops {
		switch op.Kind {
		case driver.DrawSave:
			stack = append(stack, st)
		case driver.DrawRestore:
			st, stack = stack[len(stack)-1], stack[:len(stack)-1]
		case driver.DrawAlpha:
			st.alpha = op.A
		case driver.DrawComposite:
			st.composite = op.Composite
		case driver.DrawStrokeColor:
			strokes = append(strokes, ink{op.Color, st.alpha})
		case driver.DrawFillColor:
			if st.composite == driver.CompositeDestinationOut {
				erases = append(erases, ink{op.Color, st.alpha})
			} else {
				fills = append(fills, ink{op.Color, st.alpha})
			}
		}
	}
	return strokes, fills, erases
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }

// A view with no Look draws its edges and labels in the text color it has, faded, whatever paints the views around it set: base reads no theme, and a recorded CurrentColor would draw nothing.
func TestViewDrawsInTheTextColor(t *testing.T) {
	var state State
	text := core.RGB(0x2050a0)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Color: text, Stroke: core.PaintColor(core.RGB(0xff0000)), Fill: core.PaintColor(core.RGB(0x00ff00))},
			Children: []core.Node{View{Label: "Pipeline", State: &state, Data: sample}}}
	}, nsgotest.Size(900, 700), nsgotest.Environment(driver.Environment{ReduceMotion: true, Idiom: driver.IdiomDesktop}))
	app.Settle()
	strokes, fills, erases := edgeInks(t, app)
	want := map[string]float32{"line": .35, "muted line": .35 * mutedEdgeOpacity, "label": .55, "label border": .2}
	found := map[string]bool{}
	for _, k := range append(strokes, fills...) {
		if k.color != text {
			t.Fatalf("painted %08x at %v, want the text color %08x", k.color, k.alpha, text)
		}
		for name, a := range want {
			if near(k.alpha, a) {
				found[name] = true
			}
		}
	}
	for name, a := range want {
		if !found[name] {
			t.Errorf("nothing painted as a %s, at alpha %v: strokes %v, fills %v", name, a, strokes, fills)
		}
	}
	// The label clears the edges under it, as an opaque background would hide them.
	if len(erases) != 1 || erases[0].color&0xff != 0xff || erases[0].alpha != 1 {
		t.Errorf("the label erased %v under it, want one opaque fill", erases)
	}
}

// A Look's colors draw as they are, at full alpha.
func TestViewDrawsItsLook(t *testing.T) {
	var state State
	look := Look{Line: core.RGB(0x111111), Emphasis: core.RGB(0x222222), Label: core.RGB(0x333333), LabelBackground: core.RGB(0x444444), LabelBorder: core.RGB(0x555555)}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return View{Label: "Pipeline", State: &state, Data: sample, LookFunc: func() Look { return look }}
	}, nsgotest.Size(900, 700), nsgotest.Environment(driver.Environment{ReduceMotion: true, Idiom: driver.IdiomDesktop}))
	app.Settle()
	strokes, fills, erases := edgeInks(t, app)
	if len(erases) != 0 {
		t.Errorf("a label with a background erased %v", erases)
	}
	allowed := map[core.Color]bool{look.Line: true, look.Line.Faded(mutedEdgeOpacity): true, look.Label: true, look.LabelBackground: true, look.LabelBorder: true}
	for _, k := range append(strokes, fills...) {
		if !allowed[k.color] || k.alpha != 1 {
			t.Fatalf("painted %08x at %v; want the look's colors at alpha 1: strokes %v, fills %v", k.color, k.alpha, strokes, fills)
		}
	}
}
