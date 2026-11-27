package watchos

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Host.swift's Digital Crown phases.
const (
	crownBegan uint8 = iota
	crownChanged
	crownEnded
)

// Host.swift's opCrown modes.
const (
	// crownModeNone leaves the Digital Crown to SwiftUI's own ScrollViews.
	crownModeNone uint8 = iota
	// crownModeContinuous captures the crown for a view that pans or takes wheel input.
	crownModeContinuous
	// crownModeDetent captures the crown for a view that steps a value via increment/decrement actions.
	crownModeDetent
)

const (
	// crownDetentStep is the accumulated crown rotation required to emit one
	// increment or decrement action on a value-turning view.
	crownDetentStep = 1.0
	// crownWheelScale converts Digital Crown rotation units to points for EventWheel.
	crownWheelScale = 24.0
	// crownPanScale converts Digital Crown rotation units to points for EventPan.
	crownPanScale = 24.0
)

// crownKind classifies how a view consumes Digital Crown rotation.
type crownKind uint8

const (
	crownNone crownKind = iota
	crownWheel
	crownValue
	crownPan
)

func (k crownKind) hostMode() uint8 {
	switch k {
	case crownWheel, crownPan:
		return crownModeContinuous
	case crownValue:
		return crownModeDetent
	default:
		return crownModeNone
	}
}

// crownKindOf reports how node n takes Digital Crown rotation, or crownNone
// when n is ineligible, disabled, retired, culled or ignores pointer events.
func crownKindOf(n *watchNode, inputDisabled bool) crownKind {
	if n == nil || n.deleted || n.retired || n.culled {
		return crownNone
	}
	if n.props.Disabled || inputDisabled {
		return crownNone
	}
	if n.props.PointerEvents == driver.PointerEventsNone {
		return crownNone
	}
	if n.events.Has(driver.EventWheel) {
		return crownWheel
	}
	if n.events.Has(driver.EventAccessibilityAction) &&
		(n.props.HasAction(driver.ActionIncrement) || n.props.HasAction(driver.ActionDecrement)) {
		return crownValue
	}
	if n.events.Has(driver.EventPan) {
		return crownPan
	}
	return crownNone
}

// crownPanHorizontal reports whether an EventPan target pans horizontally from
// the Digital Crown based on its PanActivation constraints.
func crownPanHorizontal(act driver.PanActivation) bool {
	if act.ActiveOffsetX != 0 && act.ActiveOffsetY == 0 {
		return true
	}
	if act.FailOffsetY != 0 && act.FailOffsetX == 0 {
		return true
	}
	return false
}

func watchNodeVisible(n *watchNode) bool {
	if n == nil || n.deleted || n.retired || n.culled {
		return false
	}
	st := n.shownStyle()
	return !st.Hidden && st.Opacity > 0
}

// activeLayerRoot returns the topmost visible modal under root, or root itself
// when no modal is presented.
func activeLayerRoot(root *watchNode) *watchNode {
	if !watchNodeVisible(root) {
		return nil
	}
	var top *watchNode
	var walk func(n *watchNode)
	walk = func(n *watchNode) {
		if !watchNodeVisible(n) {
			return
		}
		if n.kind == driver.KindModal {
			top = n
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(root)
	if top != nil {
		return top
	}
	return root
}

func inWatchSubtree(root, target *watchNode) bool {
	for n := target; n != nil; n = n.parent {
		if n == root {
			return true
		}
	}
	return false
}

func watchNodeEligibleInLayer(layer, n *watchNode, isDisabled func(*watchNode) bool) crownKind {
	if layer == nil || n == nil || !inWatchSubtree(layer, n) {
		return crownNone
	}
	for p := n; p != nil; p = p.parent {
		if !watchNodeVisible(p) {
			return crownNone
		}
		if p == layer {
			break
		}
	}
	f := n.shownFrame()
	if f.Size.Width <= 0 || f.Size.Height <= 0 {
		return crownNone
	}
	disabled := isDisabled != nil && isDisabled(n)
	return crownKindOf(n, disabled)
}

func findNodeInSubtree(root *watchNode, id driver.NodeID) *watchNode {
	if root == nil || id == 0 {
		return nil
	}
	if root.id == id && !root.deleted && !root.retired {
		return root
	}
	for _, c := range root.children {
		if found := findNodeInSubtree(c, id); found != nil {
			return found
		}
	}
	return nil
}

// hasScrollableScroll reports whether layer holds a visible, enabled ScrollView
// whose content overflows its frame on its scroll axis.
func hasScrollableScroll(layer *watchNode) bool {
	if layer == nil {
		return false
	}
	var found bool
	var walk func(n *watchNode)
	walk = func(n *watchNode) {
		if found || !watchNodeVisible(n) {
			return
		}
		if n.kind == driver.KindScroll && !n.scroll.Behavior.NoScroll {
			f := n.shownFrame()
			if n.scroll.Horizontal {
				if n.content.Width > f.Size.Width+0.5 {
					found = true
					return
				}
			} else if n.content.Height > f.Size.Height+0.5 {
				found = true
				return
			}
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(layer)
	return found
}

// resolveCrownTarget picks the view that receives Digital Crown rotation:
//  1. A focused crown-eligible input (such as a RoleSpinButton NumberInput) in the active layer.
//  2. The explicitly focused or last-tapped crown-eligible view in the active layer.
//  3. When no scrollable ScrollView competes in the active layer, the first
//     visible enabled crown-eligible view (preferring wheel/value views over pan views).
//
// Returning (nil, crownNone) leaves the Digital Crown to SwiftUI's own ScrollViews.
func resolveCrownTarget(root, focusedInput *watchNode, crownFocus driver.NodeID, isDisabled func(*watchNode) bool) (*watchNode, crownKind) {
	layer := activeLayerRoot(root)
	if layer == nil {
		return nil, crownNone
	}
	if k := watchNodeEligibleInLayer(layer, focusedInput, isDisabled); k != crownNone {
		return focusedInput, k
	}
	if crownFocus != 0 {
		if n := findNodeInSubtree(layer, crownFocus); n != nil {
			if k := watchNodeEligibleInLayer(layer, n, isDisabled); k != crownNone {
				return n, k
			}
		}
	}
	if hasScrollableScroll(layer) {
		return nil, crownNone
	}
	var panFallback *watchNode
	var primary *watchNode
	var primaryKind crownKind
	var walk func(n *watchNode)
	walk = func(n *watchNode) {
		if primary != nil || !watchNodeVisible(n) {
			return
		}
		f := n.shownFrame()
		if f.Size.Width > 0 && f.Size.Height > 0 {
			disabled := isDisabled != nil && isDisabled(n)
			switch k := crownKindOf(n, disabled); k {
			case crownWheel, crownValue:
				primary, primaryKind = n, k
				return
			case crownPan:
				if panFallback == nil {
					panFallback = n
				}
			}
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(layer)
	if primary != nil {
		return primary, primaryKind
	}
	if panFallback != nil {
		return panFallback, crownPan
	}
	return nil, crownNone
}

// crownRouter tracks Digital Crown focus, host opCrown mode synchronization,
// and in-flight wheel, value-step and pan gesture state.
type crownRouter struct {
	focus         driver.NodeID
	sentMode      uint8
	activeID      driver.NodeID
	activeKind    crownKind
	panHorizontal bool
	panTotal      float64
	accum         float64
	lastLocal     driver.Point
	lastPage      driver.Point
}

func (r *crownRouter) setFocus(id driver.NodeID) {
	r.focus = id
}

func (r *crownRouter) clearFocus(id driver.NodeID) {
	if r.focus == id {
		r.focus = 0
	}
}

func (r *crownRouter) clearAllFocus() {
	r.focus = 0
}

func (r *crownRouter) forgetNode(id driver.NodeID) {
	if r.focus == id {
		r.focus = 0
	}
	if r.activeID == id {
		r.activeID = 0
		r.activeKind = crownNone
		r.panTotal = 0
		r.accum = 0
	}
}

func (r *crownRouter) reset() {
	*r = crownRouter{}
}

// syncMode updates Host.swift via opCrown whenever the active crown capture
// mode changes.
func (r *crownRouter) syncMode(w *writer, root, focusedInput *watchNode, isDisabled func(*watchNode) bool) uint8 {
	_, kind := resolveCrownTarget(root, focusedInput, r.focus, isDisabled)
	mode := kind.hostMode()
	if mode != r.sentMode {
		r.sentMode = mode
		if w != nil {
			w.crown(mode)
		}
	}
	return mode
}

func crownAxisPoint(v float64, horizontal bool) driver.Point {
	if horizontal {
		return driver.Point{X: driver.Points(v)}
	}
	return driver.Point{Y: driver.Points(v)}
}

func sanitizeCrownFloat(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// handle translates one Digital Crown report from Host.swift into the driver
// events to dispatch on the resolved target view.
func (r *crownRouter) handle(
	phase uint8,
	delta, velocity float64,
	root, focusedInput *watchNode,
	isDisabled func(*watchNode) bool,
	centerOf func(*watchNode) (local, page driver.Point),
) []driver.Event {
	delta = sanitizeCrownFloat(delta)
	velocity = sanitizeCrownFloat(velocity)

	var out []driver.Event
	endActive := func(vel float64) {
		if r.activeID == 0 {
			return
		}
		switch r.activeKind {
		case crownWheel:
			out = append(out, driver.Event{
				Kind:     driver.EventWheel,
				ID:       r.activeID,
				Phase:    driver.GestureEnded,
				Point:    r.lastLocal,
				Page:     r.lastPage,
				Velocity: driver.Point{Y: driver.Points(vel * crownWheelScale)},
				Precise:  true,
			})
		case crownPan:
			out = append(out, driver.Event{
				Kind:         driver.EventPan,
				ID:           r.activeID,
				Phase:        driver.GestureEnded,
				Point:        crownAxisPoint(r.panTotal, r.panHorizontal),
				Position:     r.lastLocal,
				PagePosition: r.lastPage,
				Velocity:     crownAxisPoint(vel*crownPanScale, r.panHorizontal),
			})
		}
		r.activeID = 0
		r.activeKind = crownNone
		r.panHorizontal = false
		r.panTotal = 0
		r.accum = 0
	}

	if phase == crownEnded {
		endActive(velocity)
		return out
	}

	target, kind := resolveCrownTarget(root, focusedInput, r.focus, isDisabled)
	if target == nil || kind == crownNone {
		endActive(0)
		return out
	}

	local, page := centerOf(target)
	if r.activeID != 0 && (r.activeID != target.id || r.activeKind != kind) {
		endActive(0)
	}
	r.lastLocal, r.lastPage = local, page

	switch kind {
	case crownWheel:
		if r.activeID == 0 || phase == crownBegan {
			if r.activeID == 0 {
				r.activeID = target.id
				r.activeKind = crownWheel
				out = append(out, driver.Event{
					Kind:    driver.EventWheel,
					ID:      target.id,
					Phase:   driver.GestureBegan,
					Point:   local,
					Page:    page,
					Precise: true,
				})
			}
		}
		if delta != 0 || velocity != 0 {
			out = append(out, driver.Event{
				Kind:     driver.EventWheel,
				ID:       target.id,
				Phase:    driver.GestureChanged,
				Point:    local,
				Page:     page,
				Delta:    driver.Point{Y: driver.Points(delta * crownWheelScale)},
				Velocity: driver.Point{Y: driver.Points(velocity * crownWheelScale)},
				Precise:  true,
			})
		}
	case crownValue:
		r.activeID = target.id
		r.activeKind = crownValue
		r.accum += delta
		for r.accum >= crownDetentStep {
			r.accum -= crownDetentStep
			if target.props.HasAction(driver.ActionIncrement) {
				out = append(out, driver.Event{
					Kind: driver.EventAccessibilityAction,
					ID:   target.id,
					Text: driver.ActionIncrement,
				})
			}
		}
		for r.accum <= -crownDetentStep {
			r.accum += crownDetentStep
			if target.props.HasAction(driver.ActionDecrement) {
				out = append(out, driver.Event{
					Kind: driver.EventAccessibilityAction,
					ID:   target.id,
					Text: driver.ActionDecrement,
				})
			}
		}
	case crownPan:
		if r.activeID == 0 {
			r.activeID = target.id
			r.activeKind = crownPan
			r.panHorizontal = crownPanHorizontal(target.props.PanActivation)
			r.panTotal = 0
			out = append(out, driver.Event{
				Kind:         driver.EventPan,
				ID:           target.id,
				Phase:        driver.GestureBegan,
				Point:        crownAxisPoint(0, r.panHorizontal),
				Position:     local,
				PagePosition: page,
			})
		}
		if delta != 0 || velocity != 0 {
			r.panTotal += delta * crownPanScale
			out = append(out, driver.Event{
				Kind:         driver.EventPan,
				ID:           target.id,
				Phase:        driver.GestureChanged,
				Point:        crownAxisPoint(r.panTotal, r.panHorizontal),
				Position:     local,
				PagePosition: page,
				Velocity:     crownAxisPoint(velocity*crownPanScale, r.panHorizontal),
			})
		}
	}
	return out
}
