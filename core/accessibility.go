package core

import (
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Importance is importantForAccessibility, Android's name for Accessible
// and AccessibilityElementsHidden in one prop.
type Importance uint8

const (
	// ImportantAuto leaves it to the view's other props.
	ImportantAuto Importance = iota
	// ImportantYes is Accessible.
	ImportantYes
	// ImportantNo makes the view no element; its descendants still are.
	ImportantNo
	// ImportantNoHideDescendants hides the view and its subtree.
	ImportantNoHideDescendants
)

// SetAccessibilityFocus moves the screen reader's focus to the element, as after a change the user should hear about first. The web and desktops move the keyboard focus with it, to a field at least, so under a confined owner it does only as Scope.Confine allows.
func (r *Ref) SetAccessibilityFocus() {
	if r.placed() && r.t.mayMoveFocus(r.n.owner) {
		r.t.command(driver.ItemAccessibilityFocus, r.n)
	}
}

// a11y is what an element's accessibility props are bound from.
type a11y struct {
	labelFunc       func() string
	hintFunc        func() string
	descriptionFunc func() string
	value           func() AccessibilityValue
	expanded        func() bool
	expandable      func() bool
	hidden          func() bool
	actions         []AccessibilityAction
	actionsFunc     func() []AccessibilityAction
	accessible      bool
	important       Importance
}

// bindAccessibility sets n's accessibility props from a and binds those
// that change.
func (t *Tree) bindAccessibility(cx *Scope, n *node, a a11y) {
	p := &n.props
	switch {
	case a.important == ImportantNo || a.important == ImportantNoHideDescendants:
		p.Accessible = driver.AccessibleNo
	case a.accessible || a.important == ImportantYes:
		p.Accessible = AccessibleYes
	}
	switch {
	case a.actionsFunc == nil:
		t.updateAccessibilityActions(n, a.actions)
	case cx == nil:
		t.updateAccessibilityActions(n, a.actionsFunc())
	default:
		cx.renderEffect(func() {
			t.updateAccessibilityActions(n, a.actionsFunc())
		})
	}
	t.bindAccessibilityText(cx, n, &p.Label, a.labelFunc)
	t.bindAccessibilityText(cx, n, &p.Hint, a.hintFunc)
	t.bindAccessibilityText(cx, n, &p.Description, a.descriptionFunc)
	if a.value != nil {
		cx.renderEffect(func() {
			p.Value = a.value()
			t.touch(n)
		})
	}
	if a.expanded != nil {
		p.Expandable = true
		cx.renderEffect(func() {
			if a.expandable != nil {
				p.Expandable = a.expandable()
			}
			p.Expanded = a.expanded()
			t.touch(n)
			t.restyleExpanded(n)
		})
	}
	always := a.important == ImportantNoHideDescendants
	p.ElementsHidden = always
	if a.hidden != nil && !always {
		cx.renderEffect(func() {
			p.ElementsHidden = a.hidden()
			t.touch(n)
		})
	}
}

// updateAccessibilityActions publishes a stable snapshot for the backend.
func (t *Tree) updateAccessibilityActions(n *node, actions []AccessibilityAction) {
	if slices.Equal(n.props.ActionList(), actions) {
		return
	}
	n.props.Actions = nil
	if len(actions) > 0 {
		snapshot := slices.Clone(actions)
		n.props.Actions = &snapshot
	}
	t.touch(n)
}

// bindAccessibilityText updates a spoken name, hint or description on the same view.
func (t *Tree) bindAccessibilityText(cx *Scope, n *node, dst *string, text func() string) {
	if text != nil {
		cx.renderEffect(func() {
			if value := text(); value != *dst {
				*dst = value
				t.touch(n)
			}
		})
	}
}

// bindIllustrationLabel makes a labeled icon, shader or GPU view an image and lets an unlabeled one remain decorative.
func (t *Tree) bindIllustrationLabel(cx *Scope, n *node, text func() string) {
	set := func(label string) {
		n.props.Label, n.props.Role = label, RoleNone
		if label != "" {
			n.props.Role = RoleImage
		}
	}
	if text == nil {
		set(n.props.Label)
		return
	}
	cx.renderEffect(func() {
		set(text())
		t.touch(n)
	})
}

// sentProps returns the props n sends: a group without a label of its
// own reads the text inside it.
func (n *node) sentProps() driver.ViewProps {
	p := n.labelProps()
	p.Appearance = n.appearance
	if n.hoist != nil {
		p.TabAfter = n.hoist.tabAfter
		p.Topmost = n.hoist.topmost()
	}
	if n.input != nil && n.input.config.Disabled {
		p.Disabled = true
	}
	if l := n.labels; l != nil {
		if l.name != "" {
			p.Label = l.name
		}
		p.LabelledBy = l.ids
		if target := n.labelTarget(); target != nil {
			p.LabelFor = target.id
			p.LabelPress = n.h.press == nil && n.h.longPress == nil && n.h.pressDown == nil && n.h.pressUp == nil
			if p.LabelPress && !p.Focusable {
				p.NoFocus = true
			}
		}
	}
	if tn := n.text; tn != nil && p.Label == "" && tn.text.HasHidden() {
		// Screen readers leave hidden spans out.
		p.Label = driver.ShownText(tn.text.Text, tn.text.Spans, tn.attachmentLabels())
	}
	if p.Accessible == AccessibleYes && p.Label == "" && n.text == nil {
		var b strings.Builder
		n.appendText(&b)
		p.Label = b.String()
	}
	if n.inTooltip() {
		// A tooltip is a visual description of its trigger, never a second screen-reader element or a live announcement.
		p.Accessible, p.ElementsHidden = driver.AccessibleNo, true
		p.Role, p.LiveRegion = RoleNone, LiveRegionNone
	} else if p.Description == "" && n.tooltipDescription != "" {
		name := p.Label
		if name == "" {
			var b strings.Builder
			if n.text != nil {
				b.WriteString(driver.ShownText(n.text.text.Text, n.text.text.Spans, n.text.attachmentLabels()))
			} else {
				n.appendText(&b)
			}
			name = b.String()
		}
		if tooltipWords(name) != tooltipWords(n.tooltipDescription) {
			p.Description = n.tooltipDescription
		}
	}
	return p
}

// appendText appends the text of the texts inside n that assistive
// technology would read, separated by spaces.
func (n *node) appendText(b *strings.Builder) {
	for _, c := range n.children {
		if c.deleted || c.style.Display == DisplayNone || c.props.ElementsHidden {
			continue
		}
		var s string
		switch {
		case c.props.Label != "":
			s = c.props.Label
		case c.text != nil:
			s = driver.ShownText(c.text.text.Text, c.text.text.Spans, c.text.attachmentLabels())
		}
		if s = strings.TrimSpace(s); s != "" {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(s)
			continue
		}
		c.appendText(b)
	}
}

// attachmentLabels are what assistive technology reads for each of the text's attachments, in order: the view's label, else the labels and texts inside it.
func (tn *textNode) attachmentLabels() []string {
	if tn.attachments == 0 {
		return nil
	}
	var labels []string
	for _, sp := range tn.spans {
		w := sp.attach
		if w == nil {
			continue
		}
		label := w.props.Label
		if label == "" {
			var b strings.Builder
			w.appendText(&b)
			label = b.String()
		}
		labels = append(labels, label)
	}
	return labels
}
