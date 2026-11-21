package core

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// FocusDirection is the direction MoveFocus moves keyboard focus in: the core's part of package gamepad's focus navigation (gamepad.EnableFocusNavigation).
type FocusDirection uint8

const (
	FocusDirNone FocusDirection = iota
	FocusDirUp
	FocusDirDown
	FocusDirLeft
	FocusDirRight
	FocusDirNext
	FocusDirPrev
)

type focusStop struct {
	n    *node
	rect Bounds
}

// MoveFocus moves keyboard focus across the active focus stops in cx's tree — the same enabled, visible text inputs, pressable views and Focusable views Tab reaches, scoped to the topmost modal when one shows — in dir.
//
// Directional moves (Up, Down, Left, Right) pick the nearest stop in that direction by layout geometry; when no stop has been laid out yet they fall back to Tab order. When wrap is true and no stop lies further in dir, focus wraps in Tab order.
//
// MoveFocus is the core's part of package gamepad's focus navigation (gamepad.EnableFocusNavigation). It reports whether focus moved.
func MoveFocus(cx *Scope, dir FocusDirection, wrap bool) bool {
	if cx == nil || dir == FocusDirNone {
		return false
	}
	t := cx.tree()
	if t == nil || t.view == nil || !t.mayMoveFocus(cx.current()) {
		return false
	}
	stops := t.focusStops()
	if len(stops) == 0 {
		return false
	}
	cur := -1
	for i, s := range stops {
		if s.n == t.focus {
			cur = i
			break
		}
	}
	var target *node
	if cur < 0 {
		switch dir {
		case FocusDirPrev, FocusDirUp, FocusDirLeft:
			target = stops[len(stops)-1].n
		default:
			target = stops[0].n
		}
	} else {
		switch dir {
		case FocusDirNext:
			if wrap || cur+1 < len(stops) {
				target = stops[(cur+1)%len(stops)].n
			}
		case FocusDirPrev:
			if wrap || cur > 0 {
				target = stops[(cur+len(stops)-1)%len(stops)].n
			}
		case FocusDirUp, FocusDirDown, FocusDirLeft, FocusDirRight:
			target = pickDirectionalStop(stops, cur, dir, wrap)
		}
	}
	if target == nil || target == t.focus {
		return false
	}
	t.focusStopNode(target)
	return true
}

// ActivateFocused activates the currently focused view in cx's tree as Return or Space does (press down, up and press on a pressable view; submit on a text input). It is the core's part of package gamepad's focus navigation (gamepad.EnableFocusNavigation) and reports whether a focused view was activated.
func ActivateFocused(cx *Scope) bool {
	if cx == nil {
		return false
	}
	t := cx.tree()
	if t == nil || t.focus == nil || t.focus.deleted || !t.mayMoveFocus(cx.current()) {
		return false
	}
	n := t.focus
	t.inputModality(driver.ModalityKeyboard)
	if n.labelProps().Disabled {
		return false
	}
	if n.kind == driver.KindInput {
		if n.h.submit != nil {
			t.Dispatch(driver.Event{Kind: driver.EventSubmitEditing, ID: n.id})
			return true
		}
		return false
	}
	if !n.takesPresses() && !n.events().Pressable() {
		return false
	}
	id := n.id
	for _, kind := range []driver.EventKind{driver.EventPressDown, driver.EventPressUp, driver.EventPress} {
		if n.deleted {
			break
		}
		t.Dispatch(driver.Event{Kind: kind, ID: id})
	}
	return true
}

// TriggerBack dispatches Back in cx's tree through the same chain Escape, Android's back button and tvOS's Menu button use (overlays, modals, Scope.OnBackPress handlers and navigation stacks). It is the core's part of package gamepad's focus navigation (gamepad.EnableFocusNavigation).
func TriggerBack(cx *Scope) {
	if cx == nil {
		return
	}
	t := cx.tree()
	if t == nil || t.view == nil || !t.mayMoveFocus(cx.current()) {
		return
	}
	id := t.view.id
	if t.focus != nil && !t.focus.deleted {
		id = t.focus.id
	}
	t.Dispatch(driver.Event{Kind: driver.EventBack, ID: id})
}

// focusStopNode moves focus to target with keyboard modality, using native ItemFocus on platforms that focus views (and always for text inputs) and engine-side focus state on platforms where ordinary views cannot hold native focus.
func (t *Tree) focusStopNode(target *node) {
	t.inputModality(driver.ModalityKeyboard)
	env := t.root.Env()
	focusesViews := (&driver.Environment{
		OS:    env.os.GetUntracked(),
		Idiom: env.idiom.GetUntracked(),
	}).FocusesViews()
	if focusesViews || target.kind == driver.KindInput {
		t.command(driver.ItemFocus, target)
		return
	}
	// A platform that focuses no ordinary views (driver.Environment.FocusesViews false) holds no native focus for them, so update the tree's focus state directly while blurring any previously focused native text field.
	old := t.focus
	if old != nil && old != target && !old.deleted {
		if old.kind == driver.KindInput {
			t.command(driver.ItemBlur, old)
		}
		t.setState(old, stateFocused, false)
		t.focusReported(old, false)
		if old.h.blur != nil {
			t.within(old.owner, old.h.blur)
		}
	}
	t.setState(target, stateFocused, true)
	t.focusReported(target, true)
	if target.h.focus != nil {
		t.within(target.owner, target.h.focus)
	}
}

// focusStops collects the active, visible, enabled focus stops in Tab order within the topmost modal (or root view) together with their surface-space bounding boxes.
func (t *Tree) focusStops() []focusStop {
	host := t.view
	for top := topModal(host); top != nil; top = topModal(host) {
		host = top
	}
	var raw []focusEntry
	var walk func(n *node, origin Point, disabled bool)
	walk = func(n *node, origin Point, disabled bool) {
		if n == nil || n.deleted || n.hidden || n.style.Display == DisplayNone || n.style.Visibility == VisibilityHidden {
			return
		}
		if n.hoist != nil && n.hoist.dismissed {
			return
		}
		if n != host && n.kind == driver.KindModal {
			return
		}
		props := n.sentProps()
		dis := disabled || props.Disabled
		rect := Bounds{
			Origin: Point{X: origin.X + n.frame.Origin.X, Y: origin.Y + n.frame.Origin.Y},
			Size:   n.frame.Size,
		}
		if n == host && n.kind == driver.KindModal {
			rect = Bounds{Origin: Point{}, Size: n.frame.Size}
		}
		raw = append(raw, focusEntry{n: n, rect: rect, tabAfter: props.TabAfter, disabled: dis})

		childOrigin := rect.Origin
		if n.kind == driver.KindScroll && n.scroll != nil {
			childOrigin.X -= n.scroll.offset.X
			childOrigin.Y -= n.scroll.offset.Y
		}
		for _, c := range n.nativeChildren() {
			nextOrigin := childOrigin
			if c.hoist != nil {
				// Hosted overlays are positioned in their host's coordinates.
				nextOrigin = rect.Origin
			}
			walk(c, nextOrigin, dis)
		}
	}
	walk(host, Point{}, false)

	// Reorder entries honoring TabAfter overlays, matching nsgotest's tabOrder.
	ordered := orderFocusEntries(raw)
	stops := make([]focusStop, 0, len(ordered))
	for _, e := range ordered {
		if !e.disabled && isNodeFocusStop(e.n) {
			stops = append(stops, focusStop{n: e.n, rect: e.rect})
		}
	}
	return stops
}

type focusEntry struct {
	n        *node
	rect     Bounds
	tabAfter NodeID
	disabled bool
}

func orderFocusEntries(in []focusEntry) []focusEntry {
	hasTabAfter := false
	for _, e := range in {
		if e.tabAfter != 0 {
			hasTabAfter = true
			break
		}
	}
	if !hasTabAfter {
		return in
	}
	// Group nodes under any ancestor with TabAfter.
	overlayOf := func(n *node) *node {
		for p := n; p != nil; p = p.parent {
			if p.hoist != nil && p.hoist.tabAfter != 0 {
				return p
			}
		}
		return nil
	}
	inside := map[*node][]focusEntry{}
	var rest []focusEntry
	var overlays []*node
	for _, e := range in {
		if o := overlayOf(e.n); o != nil {
			if _, ok := inside[o]; !ok {
				overlays = append(overlays, o)
			}
			inside[o] = append(inside[o], e)
			continue
		}
		rest = append(rest, e)
	}
	placed := map[NodeID]bool{}
	for _, e := range rest {
		placed[e.n.id] = true
	}
	after := map[NodeID][]*node{}
	for _, o := range overlays {
		if placed[o.hoist.tabAfter] {
			after[o.hoist.tabAfter] = append(after[o.hoist.tabAfter], o)
		} else {
			rest = append(rest, inside[o]...)
		}
	}
	out := make([]focusEntry, 0, len(in))
	var emit func(e focusEntry)
	emit = func(e focusEntry) {
		out = append(out, e)
		for _, o := range after[e.n.id] {
			for _, w := range inside[o] {
				emit(w)
			}
		}
	}
	for _, e := range rest {
		emit(e)
	}
	return out
}

func isNodeFocusStop(n *node) bool {
	if n == nil || n.deleted || n.isRoot {
		return false
	}
	// Check ancestor disabled state as well as node's own props.
	for p := n; p != nil; p = p.parent {
		if p.labelProps().Disabled {
			return false
		}
	}
	props := n.sentProps()
	if props.Disabled || props.NoFocus {
		return false
	}
	if n.kind == driver.KindInput {
		return n.input == nil || (!n.input.config.Readonly && !n.input.config.Disabled)
	}
	return props.FocusStop(n.events())
}

func pickDirectionalStop(stops []focusStop, cur int, dir FocusDirection, wrap bool) *node {
	hasGeometry := false
	for _, s := range stops {
		if s.rect.Size.Width > 0 || s.rect.Size.Height > 0 {
			hasGeometry = true
			break
		}
	}
	if !hasGeometry {
		switch dir {
		case FocusDirDown, FocusDirRight:
			if wrap || cur+1 < len(stops) {
				return stops[(cur+1)%len(stops)].n
			}
		case FocusDirUp, FocusDirLeft:
			if wrap || cur > 0 {
				return stops[(cur+len(stops)-1)%len(stops)].n
			}
		}
		return nil
	}

	from := stops[cur].rect
	fromCX := float64(from.Origin.X + from.Size.Width/2)
	fromCY := float64(from.Origin.Y + from.Size.Height/2)
	fromX0, fromX1 := float64(from.Origin.X), float64(from.Origin.X+from.Size.Width)
	fromY0, fromY1 := float64(from.Origin.Y), float64(from.Origin.Y+from.Size.Height)

	bestIdx := -1
	bestScore := math.MaxFloat64

	for i, cand := range stops {
		if i == cur {
			continue
		}
		r := cand.rect
		cx := float64(r.Origin.X + r.Size.Width/2)
		cy := float64(r.Origin.Y + r.Size.Height/2)
		x0, x1 := float64(r.Origin.X), float64(r.Origin.X+r.Size.Width)
		y0, y1 := float64(r.Origin.Y), float64(r.Origin.Y+r.Size.Height)

		var primaryCenter, primaryGap, crossGap, crossCenter float64
		var edgeSeparated bool

		switch dir {
		case FocusDirDown:
			primaryCenter = cy - fromCY
			edgeSeparated = y0 >= fromY1-1
			primaryGap = math.Max(0, y0-fromY1)
			crossGap = spanGap(fromX0, fromX1, x0, x1)
			crossCenter = math.Abs(cx - fromCX)
		case FocusDirUp:
			primaryCenter = fromCY - cy
			edgeSeparated = y1 <= fromY0+1
			primaryGap = math.Max(0, fromY0-y1)
			crossGap = spanGap(fromX0, fromX1, x0, x1)
			crossCenter = math.Abs(cx - fromCX)
		case FocusDirRight:
			primaryCenter = cx - fromCX
			edgeSeparated = x0 >= fromX1-1
			primaryGap = math.Max(0, x0-fromX1)
			crossGap = spanGap(fromY0, fromY1, y0, y1)
			crossCenter = math.Abs(cy - fromCY)
		case FocusDirLeft:
			primaryCenter = fromCX - cx
			edgeSeparated = x1 <= fromX0+1
			primaryGap = math.Max(0, fromX0-x1)
			crossGap = spanGap(fromY0, fromY1, y0, y1)
			crossCenter = math.Abs(cy - fromCY)
		}

		// Candidate must be strictly ahead in the chosen direction, and either clear of the current view's edge or within a generous cone so slightly misaligned horizontal siblings are not picked on Up/Down.
		if primaryCenter <= 1 {
			continue
		}
		if !edgeSeparated && primaryCenter < crossCenter*0.25 {
			continue
		}

		primaryDist := primaryGap
		if !edgeSeparated {
			primaryDist = primaryCenter
		}
		score := primaryDist + crossGap*3.0 + crossCenter*0.25 + primaryCenter*0.05
		if score < bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestIdx >= 0 {
		return stops[bestIdx].n
	}
	if wrap {
		switch dir {
		case FocusDirDown, FocusDirRight:
			return stops[(cur+1)%len(stops)].n
		case FocusDirUp, FocusDirLeft:
			return stops[(cur+len(stops)-1)%len(stops)].n
		}
	}
	return nil
}

func spanGap(a0, a1, b0, b1 float64) float64 {
	if b0 > a1 {
		return b0 - a1
	}
	if a0 > b1 {
		return a0 - b1
	}
	return 0
}
