//go:build darwin && !ios

package macos

import (
	"cmp"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// A canvas's named parts are NSAccessibilityElements, its accessibility
// children at their bounds in it, with their role, label, hint, value, states,
// identifier and actions. VoiceOver actions route to OnAccessibilityAction
// when the canvas handles them, or fall back to pressing at the node's center.

var (
	canvasNodeClass       objc.Class
	selCanvasCustomAction = sel("nsgoCanvasNodeAction:")
)

// axNodes maps the elements to their canvas and node.
var axNodes = map[objc.ID]axNode{}

type axNode struct {
	v    *view
	node driver.CanvasNode
}

func registerCanvasNodes() {
	canvasNodeClass = register("NsgoCanvasNode", apple.Class("NSAccessibilityElement"), nil, []objc.MethodDef{
		{Cmd: sel("accessibilityPerformPress"), Fn: canvasNodePress},
		{Cmd: sel("accessibilityPerformIncrement"), Fn: canvasNodeIncrement},
		{Cmd: sel("accessibilityPerformDecrement"), Fn: canvasNodeDecrement},
		{Cmd: sel("accessibilityPerformCancel"), Fn: canvasNodeCancel},
		{Cmd: selCanvasCustomAction, Fn: canvasNodeCustomAction},
		{Cmd: sel("isAccessibilitySelectorAllowed:"), Fn: canvasNodeSelectorAllowed},
		{Cmd: sel("accessibilityAttributeNames"), Fn: canvasNodeAttributeNames},
		{Cmd: sel("accessibilityAttributeValue:"), Fn: canvasNodeAttributeValue},
	})
}

// setCanvasNodes makes v's drawing's nodes its accessibility children and
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
			delete(axNodes, el)
			el.Send(selRelease)
		}
	}
	v.axNodes = v.axNodes[:0]
	if len(nodes) > 0 && canvasNodeClass == 0 {
		registerCanvasNodes()
	}
	for _, n := range nodes {
		el := objc.ID(canvasNodeClass).Send(selNew)
		role := roles[n.Role]
		if role[0] == "" {
			role = [2]string{"AXGroup", ""}
		}
		el.Send(selSetAccessibilityRole, apple.NSString(role[0]))
		el.Send(selSetAccessibilitySubrole, nsOrNil(role[1]))
		el.Send(sel("setAccessibilityLabel:"), nsOrNil(n.Label))
		el.Send(selSetAccessibilityHelp, nsOrNil(n.Hint))
		el.Send(selSetAccessibilityIdentifier, nsOrNil(cmp.Or(n.TestID, n.ID)))
		el.Send(selSetAccessibilityEnabled, !n.Disabled)
		el.Send(selSetAccessibilitySelected, n.Selected)
		if n.Expanded != driver.ExpandedUnset {
			el.Send(selSetAccessibilityExpanded, n.Expanded == driver.ExpandedTrue)
		}
		switch {
		case n.HasRange() || n.Role == driver.RoleAdjustable || n.Role == driver.RoleProgressBar:
			el.Send(sel("setAccessibilityMinValue:"), nsNumber(float64(n.Min)))
			el.Send(sel("setAccessibilityMaxValue:"), nsNumber(float64(n.Max)))
			el.Send(selSetAccessibilityValue, nsNumber(float64(n.Value)))
			el.Send(sel("setAccessibilityValueDescription:"), nsOrNil(n.ValueText))
		case n.IsCheckable():
			val := 0
			switch n.Checked {
			case driver.CheckedTrue:
				val = 1
			case driver.CheckedMixed:
				val = 2
			}
			el.Send(selSetAccessibilityValue, objc.ID(apple.Class("NSNumber")).Send(sel("numberWithInt:"), val))
		case n.ValueText != "":
			el.Send(selSetAccessibilityValue, apple.NSString(n.ValueText))
		}
		if n.EffectiveActions().Has(driver.CanvasActionScroll) {
			act := objc.ID(apple.Class("NSAccessibilityCustomAction")).Send(selAlloc).Send(
				sel("initWithName:target:selector:"), apple.NSString(driver.ActionScroll), el, selCanvasCustomAction)
			el.Send(sel("setAccessibilityCustomActions:"), nsArray([]objc.ID{act}))
			act.Send(selRelease)
		}
		el.Send(sel("setAccessibilityParent:"), v.native)
		b := n.Bounds
		el.Send(sel("setAccessibilityFrameInParentSpace:"), apple.Rect(float64(b.Origin.X), float64(b.Origin.Y),
			float64(b.Size.Width), float64(b.Size.Height)))
		axNodes[el] = axNode{v: v, node: n}
		v.axNodes = append(v.axNodes, el)
	}
	v.updateAccessibility()
	for _, a := range driver.CanvasLiveChanges(prev, nodes) {
		announceWith(a.Text, a.Live)
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
			info := objc.ID(apple.Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"),
				nsArray([]objc.ID{el}), apple.Global(appKitPath, "NSAccessibilityUIElementsKey"))
			axPost()(el, apple.Global(appKitPath, "NSAccessibilityLayoutChangedNotification"), info)
			axPost()(el, apple.Global(appKitPath, "NSAccessibilityFocusedUIElementChangedNotification"), 0)
			return
		}
	}
}

// releaseCanvasNodes lets a deleted canvas's elements go.
func (v *view) releaseCanvasNodes() {
	for _, el := range v.axNodes {
		delete(axNodes, el)
		el.Send(selRelease)
	}
	v.axNodes = nil
}

func (n axNode) dispatchAction(name string) bool {
	if n.v == nil || n.v.surface == nil || n.v.surface.views == nil || n.node.Disabled {
		return false
	}
	if !n.v.events.Has(driver.EventAccessibilityAction) || !n.node.HasAction(name) {
		return false
	}
	n.v.report(driver.Event{
		Kind:       driver.EventAccessibilityAction,
		Text:       name,
		CanvasNode: n.node.NodeID(),
	})
	return true
}

func canvasNodeIncrement(self objc.ID, _ objc.SEL) bool {
	n, ok := axNodes[self]
	return ok && n.dispatchAction(driver.ActionIncrement)
}

func canvasNodeDecrement(self objc.ID, _ objc.SEL) bool {
	n, ok := axNodes[self]
	return ok && n.dispatchAction(driver.ActionDecrement)
}

func canvasNodeCancel(self objc.ID, _ objc.SEL) bool {
	n, ok := axNodes[self]
	return ok && n.dispatchAction(driver.ActionDismiss)
}

func canvasNodeCustomAction(self objc.ID, _ objc.SEL, action objc.ID) bool {
	n, ok := axNodes[self]
	if !ok {
		return false
	}
	if apple.GoString(action.Send(sel("name"))) == driver.ActionScroll {
		return n.dispatchAction(driver.ActionScroll)
	}
	return false
}

func canvasNodeSelectorAllowed(self objc.ID, cmd objc.SEL, selector objc.SEL) bool {
	n, ok := axNodes[self]
	if !ok {
		return apple.SendSuper[bool](self, cmd, selector)
	}
	actions := n.node.EffectiveActions()
	switch selector {
	case sel("accessibilityPerformIncrement"):
		return actions.Has(driver.CanvasActionIncrement)
	case sel("accessibilityPerformDecrement"):
		return actions.Has(driver.CanvasActionDecrement)
	case sel("accessibilityPerformCancel"):
		return actions.Has(driver.CanvasActionDismiss)
	case sel("accessibilityPerformPress"):
		if n.node.Disabled {
			return false
		}
		return actions.Has(driver.CanvasActionActivate) || n.v.events.Has(driver.EventPress)
	case sel("accessibilityExpanded"):
		return n.node.Expanded != driver.ExpandedUnset
	case sel("accessibilityMinValue"), sel("accessibilityMaxValue"):
		return n.node.HasRange() || n.node.Role == driver.RoleAdjustable || n.node.Role == driver.RoleProgressBar
	}
	return apple.SendSuper[bool](self, cmd, selector)
}

func canvasNodeAttributeNames(self objc.ID, cmd objc.SEL) objc.ID {
	names := apple.SendSuper[objc.ID](self, cmd)
	if n, ok := axNodes[self]; ok && n.node.Busy && names != 0 {
		return axAddAttr(names, "AXElementBusy")
	}
	return names
}

func canvasNodeAttributeValue(self objc.ID, cmd objc.SEL, name objc.ID) objc.ID {
	if n, ok := axNodes[self]; ok && n.node.Busy && apple.GoString(name) == "AXElementBusy" {
		return objc.ID(apple.Class("NSNumber")).Send(sel("numberWithBool:"), true)
	}
	return apple.SendSuper[objc.ID](self, cmd, name)
}

func canvasNodePress(self objc.ID, _ objc.SEL) bool {
	n, ok := axNodes[self]
	if !ok || n.v.surface.views == nil || n.node.Disabled {
		return false
	}
	if n.dispatchAction(driver.ActionActivate) {
		return true
	}
	v, b := n.v, n.node.Bounds
	center := apple.CGPoint{X: float64(b.Origin.X + b.Size.Width/2), Y: float64(b.Origin.Y + b.Size.Height/2)}
	loc := objc.Send[apple.CGPoint](v.native, sel("convertPoint:toView:"), center, objc.ID(0))
	s := v.surface
	up := s.native.Send(selSuperview)
	if up == 0 {
		return false
	}
	hit := s.native.Send(sel("hitTest:"), objc.Send[apple.CGPoint](up, selConvertPoint, loc, objc.ID(0)))
	for ; hit != 0 && hit != up; hit = hit.Send(selSuperview) {
		w := views[hit]
		if w == nil || w.native != hit || int(w.id) >= len(s.views) || s.views[w.id] != w {
			continue
		}
		if !w.events.Has(driver.EventPress) && !w.events.Has(driver.EventPressDown) {
			continue
		}
		p := objc.Send[apple.CGPoint](w.native, selConvertPoint, loc, objc.ID(0))
		e := driver.Event{Point: driver.Point{X: driver.Points(p.X), Y: driver.Points(p.Y)}, Page: w.page(loc), Count: 1}
		for _, k := range []driver.EventKind{driver.EventPressDown, driver.EventPressUp, driver.EventPress} {
			e.Kind = k
			w.report(e)
		}
		return true
	}
	return false
}
