package core

import (
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// tooltipDelay is how long the pointer rests on a view before its
// tooltip shows, as NSH's and GPUI's.
const tooltipDelay = 500 * time.Millisecond

// mountTooltip shows tip in a hint overlay above n while the pointer
// rests on it, on desktops, or the focus the keyboard moved is on it, at
// once, until it leaves or Escape is pressed.
func (t *Tree) mountTooltip(cx *Scope, n *node, tip Node) {
	if n.ref == nil {
		t.setRef(n, &Ref{})
	}
	anchor := n.ref
	// The synthetic root separates the trigger's text and accessibility from its tooltip, including elements a renderer moves into it.
	content := t.newNode(0, n, cx)
	content.ownExtra().tooltipOwner = n
	content.ownProps().ElementsHidden = true
	t.tooltips = append(t.tooltips, n)
	if h, ok := tip.(HintTooltip); ok {
		hint, change := h.Hint(cx, anchor)
		n.ownExtra().tip = &tooltipHooks{content: content, show: func() { change(HintHover) }, hide: func() { change(HintLeave) },
			focus: func(on bool) {
				if on {
					change(HintFocus)
				} else {
					change(HintBlur)
				}
			}}
		t.mount(cx, content, hint)
		return
	}
	hovered, focused := NewSignal(cx, false), NewSignal(cx, false)
	var stop func() bool
	hide := func() {
		if stop != nil {
			stop()
			stop = nil
		}
		hovered.Set(false)
	}
	n.ownExtra().tip = &tooltipHooks{content: content, show: func() {
		if cx.Env().Desktop() && stop == nil {
			stop = cx.AfterFunc(tooltipDelay, func() { stop = nil; hovered.Set(true) })
		}
	}, hide: hide, focus: focused.Set}
	t.mount(cx, content, Group{
		Overlay{Anchor: anchor, Placement: PlaceTop, Offset: 4, Margin: 4, PointerEvents: PointerEventsNone,
			StyleFunc: func() Style {
				if !hovered.Get() && !focused.Get() {
					return Style{Display: DisplayNone}
				}
				return Style{}
			}, Children: []Node{tip}},
		// Escape hides what the focus shows, while it does.
		Show{When: focused.Get, Then: func(cx *Scope) Node {
			cx.OnKey("escape", func() { hide(); focused.Set(false) })
			return nil
		}},
	})
}

// tooltipHooks are what a view's tooltip hears: show as the pointer
// comes onto the view, hide as it leaves or, where the view takes presses
// already, presses, and focus as the keyboard moves the focus onto the
// view or into it (true) and the focus leaves (false).
type tooltipHooks struct {
	content    *node
	show, hide func()
	focus      func(on bool)
}

// inTooltip follows the current shadow ancestry: a TSX renderer may move an already mounted element into or out of a tooltip.
func (n *node) inTooltip() bool {
	for p := n; p != nil; p = p.parent {
		if p.tooltipOwner != nil {
			return true
		}
	}
	return false
}

func tooltipWords(s string) string { return strings.Join(strings.Fields(s), " ") }

// tooltipText reads the content of each bubble even while the bubble itself is hidden. Visibility and accessibility choices inside it still apply.
func tooltipText(root *node) string {
	var text strings.Builder
	var read func(*node, bool)
	read = func(n *node, bubble bool) {
		if n.deleted || n != root && n.tooltipOwner != nil {
			return
		}
		if !bubble && (n.props.ElementsHidden || n.style.Display == DisplayNone || n.style.Visibility == VisibilityHidden) {
			return
		}
		if n.kind != 0 {
			var part string
			switch {
			case n.props.Label != "":
				part = n.props.Label
			case n.text != nil:
				part = driver.ShownText(n.text.text.Text, n.text.text.Spans, n.text.attachmentLabels())
			}
			if words := tooltipWords(part); words != "" {
				if text.Len() > 0 {
					text.WriteByte(' ')
				}
				text.WriteString(words)
				return
			}
		}
		for _, c := range n.children {
			read(c, bubble && n.kind == 0)
		}
	}
	read(root, true)
	return text.String()
}

// tooltipTargets unwraps a trigger's decorative containers, so a tooltip around a button describes that button, rather than an anonymous wrapper.
func tooltipTargets(anchor *node) []*node {
	var targets []*node
	var visit func(*node)
	visit = func(n *node) {
		if n.deleted || n.tooltipOwner != nil || n.hoist != nil || n.props.ElementsHidden ||
			n.style.Display == DisplayNone || n.style.Visibility == VisibilityHidden || n != anchor && n.tip != nil {
			return
		}
		if n.kind != 0 && n.props.Accessible != driver.AccessibleNo &&
			(n.props.Accessible == AccessibleYes || n.props.Role != RoleNone || n.props.Label != "" || n.props.Focusable ||
				n.text != nil || n.input != nil || n.native != nil || n.h.press != nil || n.h.longPress != nil ||
				n.h.pressDown != nil || n.h.pressUp != nil) {
			targets = append(targets, n)
			return
		}
		for _, c := range n.children {
			visit(c)
		}
	}
	visit(anchor)
	if len(targets) == 0 && labelVisible(anchor) {
		targets = append(targets, anchor)
	}
	return targets
}

// updateTooltipDescriptions runs after text and visible labels resolve. Keep the app's description separate from this projection, so changes, removal and nested tooltips restore the control's own description.
func (t *Tree) updateTooltipDescriptions() {
	if len(t.tooltips) == 0 && len(t.described) == 0 {
		return
	}
	live := t.tooltips[:0]
	next := make(map[*node]string)
	for _, anchor := range t.tooltips {
		if anchor.deleted {
			continue
		}
		live = append(live, anchor)
		if !t.labelMounted(anchor) || !labelVisible(anchor) {
			continue
		}
		if text := tooltipText(anchor.tip.content); text != "" {
			for _, target := range tooltipTargets(anchor) {
				next[target] = text
			}
		}
	}
	clear(t.tooltips[len(live):])
	t.tooltips = live
	for _, n := range t.described {
		if !n.deleted && next[n] == "" && n.tooltipDescription != "" {
			n.nodeExtra.tooltipDescription = ""
			t.touch(n)
		}
	}
	clear(t.described)
	t.described = t.described[:0]
	for n, text := range next {
		if text != n.tooltipDescription {
			n.ownExtra().tooltipDescription = text
			t.touch(n)
		}
		// The tooltip can stay unchanged while the trigger's visible name changes to or from the same words.
		if n.sentProps().Description != n.sent.props.Description {
			t.touch(n)
		}
		t.described = append(t.described, n)
	}
}
