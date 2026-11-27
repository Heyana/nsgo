//go:build ios

package ios

import (
	"cmp"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// A canvas's named parts are UIAccessibilityElements the canvas holds as
// a container, at their bounds in it, with their traits, label, hint, value,
// identifier and states. VoiceOver actions route to OnAccessibilityAction when
// the canvas handles them, or fall back to a tap at the node's center.

var canvasNodeClass objc.Class

// axNodes maps the elements to their canvas and node.
var axNodes = map[objc.ID]axNode{}

type axNode struct {
	v    *view
	node driver.CanvasNode
}

func registerCanvasNodes() {
	canvasNodeClass = register("NsgoCanvasNode", apple.Class("UIAccessibilityElement"), nil, []objc.MethodDef{
		{Cmd: sel("accessibilityActivate"), Fn: canvasNodeActivate},
		{Cmd: sel("accessibilityIncrement"), Fn: canvasNodeIncrement},
		{Cmd: sel("accessibilityDecrement"), Fn: canvasNodeDecrement},
		{Cmd: sel("accessibilityScroll:"), Fn: canvasNodeScroll},
		{Cmd: sel("accessibilityPerformEscape"), Fn: canvasNodeEscape},
	})
}

// setCanvasNodes makes v's drawing's nodes its accessibility elements and
// announces live-region changes.
func (v *view) setCanvasNodes(nodes []driver.CanvasNode) {
	if len(nodes) == 0 && len(v.axNodes) == 0 {
		return
	}
	var prev []driver.CanvasNode
	if len(v.axNodes) > 0 {
		prev = make([]driver.CanvasNode, 0, len(v.axNodes))
		for _, el := range v.axNodes {
			if an, ok := axNodes[el]; ok {
				prev = append(prev, an.node)
			}
		}
	}
	v.releaseCanvasNodes()
	if len(nodes) > 0 && canvasNodeClass == 0 {
		registerCanvasNodes()
	}
	for _, n := range nodes {
		el := objc.ID(canvasNodeClass).Send(sel("alloc")).Send(sel("initWithAccessibilityContainer:"), v.native)
		el.Send(selSetIsAccessibilityElement, true)
		el.Send(sel("setAccessibilityLabel:"), nsOrNil(n.Label))
		el.Send(selSetAccessibilityHint, nsOrNil(n.Hint))
		el.Send(selSetAccessibilityIdentifier, nsOrNil(cmp.Or(n.TestID, n.ID)))
		vp := driver.CanvasNodeViewProps(n)
		el.Send(selSetAccessibilityValue, nsOrNil(spokenValue(&vp)))
		var t uint64
		switch n.Role {
		case driver.RoleButton, driver.RoleCheckbox, driver.RoleSwitch, driver.RoleRadio, driver.RoleTab,
			driver.RoleMenuItem, driver.RoleMenuItemCheckbox, driver.RoleMenuItemRadio,
			driver.RoleOption, driver.RoleTreeItem, driver.RoleCombobox:
			t = traits["Button"]
		case driver.RoleLink:
			t = traits["Link"]
		case driver.RoleHeader, driver.RoleColumnHeader:
			t = traits["Header"]
		case driver.RoleImage:
			t = traits["Image"]
		case driver.RoleText:
			t = traits["StaticText"]
		case driver.RoleAdjustable, driver.RoleSpinButton:
			t = traits["Adjustable"]
		case driver.RoleProgressBar:
			t = traits["UpdatesFrequently"]
		case driver.RoleSearchField:
			t = traits["SearchField"]
		}
		actions := n.EffectiveActions()
		if actions.Has(driver.CanvasActionIncrement) || actions.Has(driver.CanvasActionDecrement) {
			t |= traits["Adjustable"]
		}
		if n.Selected {
			t |= traits["Selected"]
		}
		if n.Disabled {
			t |= traits["NotEnabled"]
		}
		if n.Busy {
			t |= traits["UpdatesFrequently"]
		}
		el.Send(selSetAccessibilityTraits, t)
		b := n.Bounds
		el.Send(sel("setAccessibilityFrameInContainerSpace:"), apple.Rect(float64(b.Origin.X), float64(b.Origin.Y),
			float64(b.Size.Width), float64(b.Size.Height)))
		axNodes[el] = axNode{v: v, node: n}
		v.axNodes = append(v.axNodes, el)
	}
	// A canvas with named parts is their container, not an element.
	v.native.Send(selSetIsAccessibilityElement, len(v.axNodes) == 0 && v.axElement)
	v.native.Send(sel("setAccessibilityElements:"), nsArray(v.axNodes))
	for _, a := range driver.CanvasLiveChanges(prev, nodes) {
		uiAccessibilityPost(announcement, apple.NSString(a.Text))
	}
}

// focusAccessibilityNode moves VoiceOver to the named CanvasNode, or to v when
// nodeID is empty.
func (v *view) focusAccessibilityNode(nodeID string) {
	if nodeID == "" {
		v.focusAccessibility()
		return
	}
	for _, el := range v.axNodes {
		if an, ok := axNodes[el]; ok && an.node.NodeID() == nodeID {
			uiAccessibilityPost(layoutChanged, el)
			return
		}
	}
}

// releaseCanvasNodes lets a canvas's elements go.
func (v *view) releaseCanvasNodes() {
	for _, el := range v.axNodes {
		delete(axNodes, el)
		el.Send(selRelease)
	}
	v.axNodes = v.axNodes[:0]
}

// nsArray returns an autoreleased array of items, empty for none.
func nsArray(items []objc.ID) objc.ID {
	a := objc.ID(apple.Class("NSMutableArray")).Send(sel("array"))
	for _, it := range items {
		a.Send(sel("addObject:"), it)
	}
	return a
}

func (n axNode) sendAction(name string) bool {
	if n.v == nil || n.node.Disabled || !n.v.events.Has(driver.EventAccessibilityAction) || !n.node.HasAction(name) {
		return false
	}
	n.v.report(driver.Event{
		Kind:       driver.EventAccessibilityAction,
		Text:       name,
		CanvasNode: n.node.NodeID(),
	})
	return true
}

func canvasNodeIncrement(self objc.ID, _ objc.SEL) {
	if n, ok := axNodes[self]; ok {
		n.sendAction(driver.ActionIncrement)
	}
}

func canvasNodeDecrement(self objc.ID, _ objc.SEL) {
	if n, ok := axNodes[self]; ok {
		n.sendAction(driver.ActionDecrement)
	}
}

func canvasNodeScroll(self objc.ID, _ objc.SEL, _ int64) bool {
	n, ok := axNodes[self]
	return ok && n.sendAction(driver.ActionScroll)
}

func canvasNodeEscape(self objc.ID, _ objc.SEL) bool {
	n, ok := axNodes[self]
	return ok && n.sendAction(driver.ActionDismiss)
}

func canvasNodeActivate(self objc.ID, _ objc.SEL) bool {
	n, ok := axNodes[self]
	if !ok || n.node.Disabled {
		return false
	}
	if n.sendAction(driver.ActionActivate) {
		return true
	}
	v, b := n.v, n.node.Bounds
	s := v.surface
	center := apple.CGPoint{X: float64(b.Origin.X + b.Size.Width/2), Y: float64(b.Origin.Y + b.Size.Height/2)}
	at := objc.Send[apple.CGPoint](v.native, sel("convertPoint:toView:"), center, s.native)
	hit := objc.Send[objc.ID](s.native, sel("hitTest:withEvent:"), at, objc.ID(0))
	for ; hit != 0; hit = hit.Send(selSuperview) {
		w := views[hit]
		if w == nil || w.native != hit || int(w.id) >= len(s.views) || s.views[w.id] != w {
			continue
		}
		if !w.events.Has(driver.EventPress) && !w.events.Has(driver.EventPressDown) {
			continue
		}
		p := objc.Send[apple.CGPoint](s.native, sel("convertPoint:toView:"), at, w.native)
		e := driver.Event{Point: driver.Point{X: driver.Points(p.X), Y: driver.Points(p.Y)},
			Page: driver.Point{X: driver.Points(at.X), Y: driver.Points(at.Y)}, Count: 1}
		for _, k := range []driver.EventKind{driver.EventPressDown, driver.EventPressUp, driver.EventPress} {
			e.Kind = k
			w.report(e)
		}
		return true
	}
	return false
}
