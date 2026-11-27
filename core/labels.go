package core

import (
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// labelRelations keeps declared refs separate from their projection. In particular, removing an association must restore the label's own states.
type labelRelations struct {
	forRef            *Ref
	by                []*Ref
	target            *node
	name              string
	ids               *[]NodeID
	disabled, invalid bool
	registered        bool
}

func (t *Tree) setLabels(n *node, target *Ref, by []*Ref) {
	if n.labels == nil {
		if target == nil && len(by) == 0 {
			return
		}
		n.ownExtra().labels = new(labelRelations)
	}
	l := n.labels
	l.forRef, l.by = target, slices.Clone(by)
	if !l.registered {
		l.registered = true
		t.labels = append(t.labels, n)
	}
}

// labelRef accepts only live, visible refs in the same surface and confinement. Naming must not expose a sandbox's surrounding views either.
func (t *Tree) labelRef(source *node, r *Ref) *node {
	if r == nil || r.t != t || !t.labelMounted(r.n) || !t.labelMounted(source) || r.n == source || confinement(source.owner) != confinement(r.n.owner) {
		return nil
	}
	if !labelVisible(source) || !labelVisible(r.n) {
		return nil
	}
	return r.n
}

func (t *Tree) labelMounted(n *node) bool {
	if n == nil || n.deleted {
		return false
	}
	for p := n; p != nil; p = p.parent {
		if p == t.view {
			return true
		}
	}
	return false
}

func labelVisible(n *node) bool {
	for p := n; p != nil; p = p.parent {
		if p.deleted || p.style.Display == DisplayNone || p.style.Visibility == VisibilityHidden || p.props.ElementsHidden || p.hoist != nil && p.hoist.dismissed {
			return false
		}
	}
	return true
}

func (n *node) labelTarget() *node {
	if l := n.labels; l != nil {
		return l.target
	}
	return nil
}

// labelProps applies state projection without changing the element's props.
func (n *node) labelProps() driver.ViewProps {
	p := *n.props
	if n.input != nil && n.input.config.Disabled {
		p.Disabled = true
	}
	if nn := n.native; nn != nil && nn.component == driver.SwitchComponent {
		disabled, _ := nn.props["disabled"].(bool)
		p.Disabled = p.Disabled || disabled
	}
	if l := n.labels; l != nil {
		p.Disabled = p.Disabled || l.disabled
		p.Invalid = p.Invalid || l.invalid
	}
	return p
}

// updateLabels visits registered associations, not every view. It runs after text preparation, so names include the current visible spans and attachments. Refs may acquire or lose their node without a prop changing.
func (t *Tree) updateLabels() bool {
	if len(t.labels) == 0 && len(t.labelled) == 0 {
		return false
	}
	live := t.labels[:0]
	by := map[*node][]*node{}
	targets := map[*node]*node{}
	involved := map[*node]bool{}
	for _, n := range t.labels {
		if n.deleted {
			continue
		}
		live = append(live, n)
		involved[n] = true
		if !t.labelMounted(n) {
			continue
		}
		for _, r := range n.labels.by {
			if source := t.labelRef(n, r); source != nil && !slices.Contains(by[n], source) {
				by[n] = append(by[n], source)
			}
		}
		if target := t.labelRef(n, n.labels.forRef); target != nil {
			targets[n] = target
			involved[target] = true
		}
	}
	clear(t.labels[len(live):])
	t.labels = live
	// Implicit labels follow document order even when a region moves or a later-mounted label is inserted before an existing one.
	for label, target := range targets {
		if target.props.Label == "" && (target.labels == nil || len(target.labels.by) == 0) {
			by[target] = append(by[target], label)
		}
	}
	for n, sources := range by {
		if n.labels == nil || len(n.labels.by) == 0 {
			slices.SortFunc(sources, func(a, b *node) int {
				if a == b {
					return 0
				}
				if (&Ref{n: a}).Before(&Ref{n: b}) {
					return -1
				}
				return 1
			})
		}
	}
	// Drop every edge participating in a cycle, rather than making the result depend on map traversal order. A text's own label still works.
	var reaches func(*node, *node, map[*node]bool) bool
	reaches = func(n, goal *node, seen map[*node]bool) bool {
		if n == goal {
			return true
		}
		if seen[n] {
			return false
		}
		seen[n] = true
		for _, s := range by[n] {
			if reaches(s, goal, seen) {
				return true
			}
		}
		return false
	}
	acyclic := make(map[*node][]*node, len(by))
	for n, sources := range by {
		for _, s := range sources {
			if !reaches(s, n, map[*node]bool{}) {
				acyclic[n] = append(acyclic[n], s)
			}
		}
	}
	by = acyclic
	var name func(*node, map[*node]bool) string
	name = func(n *node, seen map[*node]bool) string {
		if seen[n] {
			return ""
		}
		seen[n] = true
		defer delete(seen, n)
		var parts []string
		for _, s := range by[n] {
			if text := name(s, seen); text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
		if n.props.Label != "" {
			return strings.TrimSpace(n.props.Label)
		}
		if tn := n.text; tn != nil {
			return strings.TrimSpace(driver.ShownText(tn.text.Text, tn.text.Spans, tn.attachmentLabels()))
		}
		for _, c := range n.children {
			if labelVisible(c) {
				if text := name(c, seen); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, " ")
	}
	// The old targets also need a turn: their last label may have gone.
	for _, n := range t.labelled {
		if !n.deleted {
			involved[n] = true
		}
	}
	clear(t.labelled)
	t.labelled = t.labelled[:0]
	changed := false
	for n := range involved {
		if n.labels == nil {
			n.ownExtra().labels = new(labelRelations)
		}
		l := n.labels
		var ids []NodeID
		var text string
		if sources := by[n]; len(sources) > 0 {
			for _, s := range sources {
				if name(s, map[*node]bool{}) != "" {
					ids = append(ids, s.id)
				}
			}
			if len(ids) > 0 {
				text = name(n, map[*node]bool{})
			}
		}
		if text == "" && targets[n] != nil {
			text = name(n, map[*node]bool{})
		}
		var oldIDs []NodeID
		if l.ids != nil {
			oldIDs = *l.ids
		}
		idsChanged := !slices.Equal(ids, oldIDs)
		if idsChanged {
			l.ids = nil
			if len(ids) > 0 {
				l.ids = &ids
			}
		}
		disabled, invalid := false, false
		seen := map[*node]bool{n: true}
		for target := targets[n]; target != nil && !seen[target]; target = targets[target] {
			seen[target] = true
			disabled = disabled || target.props.Disabled || target.input != nil && target.input.config.Disabled
			if nn := target.native; nn != nil && nn.component == driver.SwitchComponent {
				off, _ := nn.props["disabled"].(bool)
				disabled = disabled || off
			}
			invalid = invalid || target.props.Invalid
		}
		if l.name != text || l.target != targets[n] || l.disabled != disabled || l.invalid != invalid || idsChanged {
			l.name, l.target, l.disabled, l.invalid = text, targets[n], disabled, invalid
			if t.restyle(n) {
				t.markLayout(n)
			}
			t.restyleGroup(n)
			changed = true
		}
		if l.target != nil || l.ids != nil {
			t.labelled = append(t.labelled, n)
		}
	}
	return changed
}

// labelActive prevents a label in a modal from activating a control below it. The backend checks focus placement too, but the action runs here.
func (t *Tree) labelActive(n *node) bool {
	for _, host := range append([]*node{t.view}, t.frames...) {
		top := topModal(host)
		if top == nil {
			continue
		}
		for next := topModal(top); next != nil; next = topModal(top) {
			top = next
		}
		insideHost, above := host == t.view, false
		for p := n; p != nil; p = p.parent {
			insideHost = insideHost || p == host
			above = above || p == top || p.hoist != nil && p.hoist.topmost() && p.hoist.host == top
		}
		if insideHost && !above {
			return false
		}
	}
	return true
}

func (t *Tree) pressLabel(n *node, ev driver.Event) {
	if n.labels == nil || n.deleted {
		return
	}
	target := t.labelRef(n, n.labels.forRef)
	if target == nil || n.labelProps().Disabled || target.labelProps().Disabled ||
		!t.labelActive(n) || !t.labelActive(target) || !t.mayMoveFocus(n.owner) || !t.mayMoveFocus(target.owner) {
		return
	}
	t.command(driver.ItemFocus, target)
	if nn := target.native; nn != nil && nn.component == driver.SwitchComponent {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemNativeCommand, ID: target.id, Value: "activate"})
		return
	}
	if fn := target.h.press; fn != nil {
		e := pressOf(ev)
		e.Keyboard = t.modality == driver.ModalityKeyboard
		// Positions on the label are not coordinates inside the control.
		e.Position = Point{}
		t.within(target.owner, func() { fn(e) })
	} else if target.h.action != nil && target.props.HasAction(driver.ActionActivate) {
		t.within(target.owner, func() { target.h.action(driver.ActionActivate) })
	}
}
