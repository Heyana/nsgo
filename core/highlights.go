package core

import (
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// UpdateHighlights is DevTools' overlay: it outlines for a moment each view a commit of its surface changed, as React DevTools' highlight updates and Flutter's repaint rainbow do, so work an update did not need shows, and holds the layout audit, which outlines every view's boxes with a grid and a ruler. It draws above the app's overlays and modals (Overlay.Topmost) and leaves its own views out. nsgo's dev client mounts it over every surface under nsgo dev -highlight, -layout and -devtools; it needs a host, an engine or a test driver, and shows nothing without one.
type UpdateHighlights struct {
	// NoUpdates starts it without outlining commits, which DevTools turn on (devtools.Surface.Highlight): the dev client sets it without -highlight.
	NoUpdates bool
	// Layout is what the layout audit shows as it starts: nsgo dev -layout's. DevTools show and hide it, and so does Mod+Alt+Shift+L when Layout configures bounds or a grid (LayoutAudit).
	Layout LayoutAudit
	// LayoutHidden starts the layout audit hidden even when Layout sets bounds or a grid (such as a window opened after the user hid a -layout audit), keeping Layout's boxes, grid and Mod+Alt+Shift+L binding ready for when it shows again.
	LayoutHidden bool
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
	// sheets are DevTools' overlay layers mounted inside platform-presented iOS sheets (ModalPageSheet, ModalFormSheet), which live in their own view controller above the root view.
	sheets []*sheetHighlights
	// mountSheet mounts a sheetHighlights layer inside a sheet modal; nil unless mountHighlights set it, so release builds link none of the overlay.
	mountSheet func(cx *Scope, modal *node)
}

// sheetHighlights is the DevTools overlay layer mounted inside one reported iOS sheet modal.
type sheetHighlights struct {
	modal   *node
	owner   *Scope
	root    *Ref
	boxes   *Signal[[]highlightBox]
	inspect *Signal[inspection]
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
		inspect: NewSignal(cx, inspection{}), picking: NewSignal(cx, false), audit: newLayoutAudit(cx, opts.Layout, opts.LayoutHidden)}
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
	// inspectedBoxes draws the inspected view as browsers' DevTools draw it: its margin around its border box, which shows its border and padding, around its content.
	inspectedBoxes := func(sig *Signal[inspection]) Node {
		return Group{
			View{PointerEvents: PointerEventsNone, StyleFunc: func() Style { i := sig.Get(); return boxStyle(i.bounds, i.margin, 1, marginColor) }},
			View{PointerEvents: PointerEventsNone, StyleFunc: func() Style { i := sig.Get(); return boxStyle(i.bounds, [4]Points{}, 0, paddingColor) }},
			View{PointerEvents: PointerEventsNone, StyleFunc: func() Style { i := sig.Get(); return boxStyle(i.bounds, i.borderPadding, -1, contentColor) }},
		}
	}
	// catcherIn covers root while DevTools pick: what it is over and what it presses is the app's view under it.
	catcherIn := func(root *node) Node {
		return Show{When: h.picking.Get, Then: func(cx *Scope) Node {
			return View{TestID: "nsgo-devtools-pick", Style: Style{Position: Absolute, Left: Pt(0), Top: Pt(0), Width: Pct(100), Height: Pct(100)},
				OnMouseMove: func(e MouseEvent) { t.pickIn(root, e.PagePosition, false) },
				OnPressDown: func(e PressEvent) { t.pickIn(root, e.PagePosition, true) },
				OnPress:     func(PressEvent) {}}
		}}
	}
	// mountSheet mounts an overlay inside a platform-presented iOS sheet once the platform reports its size.
	h.mountSheet = func(modalCx *Scope, modal *node) {
		if modal == nil || modal.hoist == nil || modal.hoist.reported == nil {
			return
		}
		var sheetBoxes []auditBox
		t.within(modalCx, func() {
			t.mount(modalCx, modal, Show{
				When: func() bool { return modal.hoist != nil && modal.hoist.reported.Get() != nil },
				Then: func(scx *Scope) Node {
					scx = scx.Child()
					sh := &sheetHighlights{
						modal:   modal,
						owner:   scx,
						root:    new(Ref),
						boxes:   NewSignalFunc[[]highlightBox](scx, nil, nil),
						inspect: NewSignal(scx, inspection{}),
					}
					h.sheets = append(h.sheets, sh)
					scx.Cleanup(func() {
						h.sheets = slices.DeleteFunc(h.sheets, func(o *sheetHighlights) bool { return o == sh })
					})
					sizeFn := func() Size {
						if r := modal.hoist.reported.Get(); r != nil {
							return r.size
						}
						return modal.frame.Size
					}
					insetsFn := func() Edges {
						if r := modal.hoist.reported.Get(); r != nil {
							return r.insets
						}
						return Edges{}
					}
					return Overlay{Ref: sh.root, Topmost: true, PointerEvents: PointerEventsBoxNone, TestID: devtools.OverlayTestID,
						Style: Style{Width: Pct(100), Height: Pct(100)},
						Children: []Node{
							t.auditLayerIn(h.audit, modal, sizeFn, insetsFn, &sheetBoxes),
							Each(sh.boxes.Get, func(b highlightBox) uint64 { return b.id }, box),
							inspectedBoxes(sh.inspect),
							catcherIn(modal),
						}}
				},
			})
		})
	}
	// Topmost, so DevTools outline, inspect and pick inside the app's modals too.
	t.mount(cx, parent, Overlay{Ref: h.root, Topmost: true, PointerEvents: PointerEventsBoxNone, TestID: devtools.OverlayTestID,
		Style:    Style{Width: Pct(100), Height: Pct(100)},
		Children: []Node{t.auditLayer(h.audit), Each(h.boxes.Get, func(b highlightBox) uint64 { return b.id }, box), inspectedBoxes(h.inspect), catcherIn(t.view)}})
	// If UpdateHighlights mounted after a sheet modal (as in core tests), wire any existing sheets too.
	var wireExisting func(host *node)
	wireExisting = func(host *node) {
		for _, n := range host.hosted {
			if n.deleted || n.hoist == nil {
				continue
			}
			if m := n.hoist.modal; m != nil && (m.Presentation == ModalPageSheet || m.Presentation == ModalFormSheet) {
				h.mountSheet(n.owner, n)
			}
			wireExisting(n)
		}
	}
	wireExisting(t.view)
}

// pickIn finds the app's innermost view inside root at p, past DevTools' own, outlines it and reports it: as picked when pressed, which ends picking.
func (t *Tree) pickIn(root *node, p Point, pressed bool) {
	h := t.highlights
	if h == nil {
		return
	}
	t.stepDevDrawn()
	if root != t.view {
		// PagePosition is in the sheet's own space; displayed geometry is in the surface's.
		if m, ok := t.devGeometryFrame().Matrices[root.id]; ok {
			x, y := m.Apply(float64(p.X), float64(p.Y))
			p = Point{X: Points(x), Y: Points(y)}
		}
	}
	id, _ := t.hitDevWith(root, p, func(n *node) bool {
		if h.owns(n) {
			return false
		}
		if root == t.view && h.sheetFor(n) != nil {
			return false
		}
		return true
	})
	n := t.node(id)
	if n == nil {
		return
	}
	h.setInspect(t, n)
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
	var rootBoxes []highlightBox
	var sheetBoxes map[*sheetHighlights][]highlightBox
	geometry := t.devGeometryFrame()
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
		if b := geometry.Bounds[n.id]; b.Size.Width > 0 && b.Size.Height > 0 {
			if sh := h.sheetFor(n); sh != nil {
				if sheetBoxes == nil {
					sheetBoxes = map[*sheetHighlights][]highlightBox{}
				}
				sheetBoxes[sh] = append(sheetBoxes[sh], highlightBox{target: n, bounds: transformBounds(t.devSpace(geometry, sh.modal), b)})
			} else {
				rootBoxes = append(rootBoxes, highlightBox{target: n, bounds: b})
			}
		}
	}
	if len(rootBoxes) > 0 {
		h.owner.Post(func() { h.showIn(h.owner, h.boxes, rootBoxes) })
	}
	for sh, bs := range sheetBoxes {
		sh, bs := sh, bs
		sh.owner.Post(func() { h.showIn(sh.owner, sh.boxes, bs) })
	}
}

// sheetFor returns the active reported sheet overlay holding n, nil if n is not inside one.
func (h *highlights) sheetFor(n *node) *sheetHighlights {
	if len(h.sheets) == 0 {
		return nil
	}
	for p := n; p != nil; p = p.nativeParent() {
		for _, sh := range h.sheets {
			if p == sh.modal && !sh.modal.hoist.dismissed {
				return sh
			}
		}
	}
	return nil
}

// setInspect selects n and outlines it in its owning layer (root or a reported sheet, in the sheet's own space), clearing any outline in other layers; nil clears them all.
func (h *highlights) setInspect(t *Tree, n *node) {
	h.selected = n
	target := h.sheetFor(n)
	i := t.inspected(n)
	if target == nil {
		h.inspect.Set(i)
	} else {
		h.inspect.Set(inspection{})
		i.bounds = transformBounds(t.devSpace(t.devGeometryFrame(), target.modal), i.bounds)
	}
	for _, sh := range h.sheets {
		if sh == target {
			sh.inspect.Set(i)
		} else {
			sh.inspect.Set(inspection{})
		}
	}
}

// clearInspect drops the selection and its outline in root and every sheet layer, so a later geometry snapshot does not outline it again.
func (h *highlights) clearInspect() {
	h.selected = nil
	h.inspect.Set(inspection{})
	for _, sh := range h.sheets {
		sh.inspect.Set(inspection{})
	}
}

// devSpace maps displayed geometry, which is in the surface's coordinates, into root's own: the identity for the surface's root view, while a platform-presented sheet's DevTools layer draws from the sheet's corner wherever the sheet shows.
func (t *Tree) devSpace(geometry driver.GeometrySnapshot, root *node) driver.Matrix3D {
	if root != t.view {
		if m, ok := geometry.Matrices[root.id]; ok {
			if inv, ok := m.Invert(); ok {
				return inv
			}
		}
	}
	return driver.Identity3D
}

// ownsScope reports whether o is inside DevTools' own overlay scope or any of its sheet overlays' scopes.
func (h *highlights) ownsScope(o *Scope) bool {
	for ; o != nil; o = o.parent {
		if o == h.owner {
			return true
		}
		for _, sh := range h.sheets {
			if o == sh.owner {
				return true
			}
		}
	}
	return false
}

// owns reports whether n is one of the highlights' own views, in the root overlay or a sheet overlay.
func (h *highlights) owns(n *node) bool {
	root := h.root.n
	for ; n != nil; n = n.parent {
		if n == root {
			return true
		}
		for _, sh := range h.sheets {
			if n == sh.root.n {
				return true
			}
		}
	}
	return false
}

// showIn adds an outline for each of bounds to sig under owner, and takes them away after highlightFor.
func (h *highlights) showIn(owner *Scope, sig *Signal[[]highlightBox], bounds []highlightBox) {
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
	sig.Update(func(bs []highlightBox) []highlightBox { return append(slices.Clip(bs), added...) })
	owner.AfterFunc(highlightFor, func() {
		sig.Update(func(bs []highlightBox) []highlightBox {
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
