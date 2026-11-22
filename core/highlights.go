package core

import (
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// UpdateHighlights is DevTools' overlay: it outlines for a moment each view a commit of its surface changed, as React DevTools' highlight updates and Flutter's repaint rainbow do, so work an update did not need shows, and holds the layout audit, which outlines every view's boxes with a grid and a ruler. It draws above the app's overlays and modals (Overlay.Topmost) and leaves its own views out. nsgo's dev client mounts it over the app under nsgo dev -highlight, -layout and -devtools; it needs a host, an engine or a test driver, and shows nothing without one.
type UpdateHighlights struct {
	// NoUpdates starts it without outlining commits, which DevTools turn on (devtools.Surface.Highlight): the dev client sets it without -highlight.
	NoUpdates bool
	// Layout is what the layout audit shows as it starts: nsgo dev -layout's. DevTools show and hide it, and so does Mod+Alt+Shift+L when it starts showing (LayoutAudit).
	Layout LayoutAudit
}

// Render implements Node.
func (h UpdateHighlights) Render(*Scope) Node { return h }

func (h UpdateHighlights) mountIn(t *Tree, cx *Scope, parent *node) { t.mountHighlights(cx, parent, h) }

// highlightFor is how long a highlight shows.
const highlightFor = 500 * time.Millisecond

// highlightColor is the outlines' color, a blue seen on light and dark.
var highlightColor = RGB(0x2f9bff)

// The fills of the view DevTools inspect, as browsers' DevTools draw them: its margin, its padding and border, and its content.
var (
	marginColor  = RGBA(0xf6b26b, 0.35)
	paddingColor = RGBA(0x93c47d, 0.35)
	contentColor = RGBA(0x6fa8dc, 0.4)
)

// inspection is the view DevTools inspect: its border box in the surface and its margin and its border and padding together, each top, right, bottom and left.
type inspection struct {
	bounds                Bounds
	margin, borderPadding [4]Points
}

// inspected is n's inspection, or none for nil.
func (t *Tree) inspected(n *node) inspection {
	if n == nil {
		return inspection{}
	}
	l := &n.lstate.Rounded
	return inspection{bounds: t.devGeometry()[n.id],
		margin: [4]Points{Points(l.Margin.Top), Points(l.Margin.Right), Points(l.Margin.Bottom), Points(l.Margin.Left)},
		borderPadding: [4]Points{Points(l.Border.Top + l.Padding.Top), Points(l.Border.Right + l.Padding.Right),
			Points(l.Border.Bottom + l.Padding.Bottom), Points(l.Border.Left + l.Padding.Left)}}
}

// boxStyle places a fill over r grown by out (top, right, bottom, left), none when r is empty.
func boxStyle(r Bounds, out [4]Points, sign Points, fill Color) Style {
	if r.Size.Width <= 0 || r.Size.Height <= 0 {
		return Style{Display: DisplayNone}
	}
	x, y := r.Origin.X-sign*out[3], r.Origin.Y-sign*out[0]
	w, h := r.Size.Width+sign*(out[1]+out[3]), r.Size.Height+sign*(out[0]+out[2])
	return Style{Position: Absolute, Left: Pt(x), Top: Pt(y), Width: Pt(max(w, 0)), Height: Pt(max(h, 0)), Background: fill}
}

// highlights are a tree's UpdateHighlights: the boxes it shows, and its overlay, whose views it leaves out. DevTools also draw the view they inspect there, take presses there while they pick one, and show the layout audit there.
type highlights struct {
	owner *Scope
	root  *Ref
	boxes *Signal[[]highlightBox]
	next  uint64
	// updates is whether commits are outlined: DevTools turn it off and on.
	updates  bool
	inspect  *Signal[inspection]
	selected *node
	picking  *Signal[bool]
	// hover and picked hear the views the pointer is over and the one pressed while picking.
	hover, picked func(id uint32)
	audit         *layoutAudit
}

type highlightBox struct {
	id     uint64
	target *node
	bounds Bounds
}

func (t *Tree) mountHighlights(cx *Scope, parent *node, opts UpdateHighlights) {
	if cx.rt.post == nil {
		return
	}
	// Its own scope holds what it runs, which DevTools leave out of the app's computations.
	cx = cx.Child()
	h := &highlights{owner: cx, root: new(Ref), boxes: NewSignalFunc[[]highlightBox](cx, nil, nil), updates: !opts.NoUpdates,
		inspect: NewSignal(cx, inspection{}), picking: NewSignal(cx, false), audit: newLayoutAudit(cx, opts.Layout)}
	t.highlights = h
	t.ensureDevDrawn()
	t.keepAudit(h.audit)
	cx.Cleanup(func() {
		h.audit.stopTicking()
		if t.highlights == h {
			t.highlights, t.auditRead, t.bumpAudit, t.tickAudit = nil, nil, nil, nil
		}
	})
	box := func(cx *Scope, b func() highlightBox, _ func() int) Node {
		return View{PointerEvents: PointerEventsNone, StyleFunc: func() Style {
			r := b().bounds
			return Style{Position: Absolute, Left: Pt(r.Origin.X), Top: Pt(r.Origin.Y), Width: Pt(r.Size.Width), Height: Pt(r.Size.Height),
				BorderWidth: 2, BorderColor: highlightColor, CornerRadius: 2}
		}}
	}
	// The inspected view as browsers' DevTools draw it: its margin around its border box, which shows its border and padding, around its content.
	inspected := Group{
		View{PointerEvents: PointerEventsNone, StyleFunc: func() Style { i := h.inspect.Get(); return boxStyle(i.bounds, i.margin, 1, marginColor) }},
		View{PointerEvents: PointerEventsNone, StyleFunc: func() Style { i := h.inspect.Get(); return boxStyle(i.bounds, [4]Points{}, 0, paddingColor) }},
		View{PointerEvents: PointerEventsNone, StyleFunc: func() Style { i := h.inspect.Get(); return boxStyle(i.bounds, i.borderPadding, -1, contentColor) }},
	}
	// While DevTools pick, a view over the whole surface takes the pointer: what it is over and what it presses is the app's view under it.
	catcher := Show{When: h.picking.Get, Then: func(cx *Scope) Node {
		return View{TestID: "nsgo-devtools-pick", Style: Style{Position: Absolute, Left: Pt(0), Top: Pt(0), Width: Pct(100), Height: Pct(100)},
			OnMouseMove: func(e MouseEvent) { t.pickAt(e.PagePosition, false) },
			OnPressDown: func(e PressEvent) { t.pickAt(e.PagePosition, true) },
			OnPress:     func(PressEvent) {}}
	}}
	// Topmost, so DevTools outline, inspect and pick inside the app's modals too.
	t.mount(cx, parent, Overlay{Ref: h.root, Topmost: true, PointerEvents: PointerEventsBoxNone, TestID: devtools.OverlayTestID,
		Style:    Style{Width: Pct(100), Height: Pct(100)},
		Children: []Node{t.auditLayer(h.audit), Each(h.boxes.Get, func(b highlightBox) uint64 { return b.id }, box), inspected, catcher}})
}

// pickAt finds the app's innermost view at p, past DevTools' own, outlines it and reports it: as picked when pressed, which ends picking.
func (t *Tree) pickAt(p Point, pressed bool) {
	h := t.highlights
	if h == nil {
		return
	}
	t.stepDevDrawn()
	id, _ := t.hitDevWith(t.view, p, func(n *node) bool { return !h.owns(n) })
	n := t.node(id)
	if n == nil {
		return
	}
	h.selected = n
	h.inspect.Set(t.inspected(n))
	if pressed {
		h.picking.Set(false)
		if h.picked != nil {
			h.picked(uint32(id))
		}
	} else if h.hover != nil {
		h.hover(uint32(id))
	}
}

// highlightKinds are the view-changing kinds of item: what a commit made, moved, restyled or filled anew.
var highlightKinds = map[driver.ItemKind]bool{
	driver.ItemCreate: true, driver.ItemFrame: true, driver.ItemViewStyle: true, driver.ItemText: true,
	driver.ItemInputValue: true, driver.ItemImage: true, driver.ItemCanvas: true, driver.ItemShader: true,
}

// noteChanges outlines the views items changed, other than the highlights' own, after the commit that sent them: it posts them, as a commit sets no signal.
func (t *Tree) noteChanges(items []driver.MountItem) {
	h := t.highlights
	if !h.updates {
		return
	}
	var boxes []highlightBox
	geometry := t.devGeometry()
	seen := map[NodeID]bool{}
	for i := range items {
		it := &items[i]
		if !highlightKinds[it.Kind] || seen[it.ID] || int(it.ID) >= len(t.nodes) {
			continue
		}
		seen[it.ID] = true
		n := t.nodes[it.ID]
		if n == nil || n.deleted || h.owns(n) {
			continue
		}
		if b := geometry[n.id]; b.Size.Width > 0 && b.Size.Height > 0 {
			boxes = append(boxes, highlightBox{target: n, bounds: b})
		}
	}
	if len(boxes) > 0 {
		h.owner.Post(func() { h.show(boxes) })
	}
}

// owns reports whether n is one of the highlights' own views.
func (h *highlights) owns(n *node) bool {
	root := h.root.n
	for ; n != nil; n = n.parent {
		if n == root {
			return true
		}
	}
	return false
}

// show adds an outline for each of bounds, and takes them away after highlightFor.
func (h *highlights) show(bounds []highlightBox) {
	// DevTools may have turned outlines off since the commit posted them, as the page's Highlight updates does.
	if !h.updates {
		return
	}
	first := h.next
	added := make([]highlightBox, len(bounds))
	for i, b := range bounds {
		h.next++
		b.id = h.next
		added[i] = b
	}
	h.boxes.Update(func(bs []highlightBox) []highlightBox { return append(slices.Clip(bs), added...) })
	h.owner.AfterFunc(highlightFor, func() {
		h.boxes.Update(func(bs []highlightBox) []highlightBox {
			kept := bs[:0:0]
			for _, b := range bs {
				if b.id <= first || b.id > first+uint64(len(added)) {
					kept = append(kept, b)
				}
			}
			return kept
		})
	})
}
