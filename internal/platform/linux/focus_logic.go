package linux

import "github.com/nsgo-dev/nsgo/internal/driver"

// This file builds on every system, so unit tests check Linux's focusable-stop
// policy, modal focus-scope reconciliation and GTK widget focusable transitions
// on any host.

// focusTreeNode is the minimal view interface needed to compute focus scopes
// and reconcile GTK widget focusability across a mounted tree.
type focusTreeNode[N any] interface {
	comparable
	focusKind() driver.ViewKind
	focusHidden() bool
	focusDeleted() bool
	focusChildren() []N
	focusStopSelf() bool
}

// activeFocusScope returns the newest visible modal under root, or root itself.
func activeFocusScope[N focusTreeNode[N]](root N) N {
	var zero N
	if root == zero {
		return zero
	}
	scope := root
	var walk func(N)
	walk = func(v N) {
		if v == zero || v.focusDeleted() || v.focusHidden() {
			return
		}
		if v.focusKind() == driver.KindModal {
			scope = v
		}
		for _, c := range v.focusChildren() {
			walk(c)
		}
	}
	walk(root)
	return scope
}

// viewCanFocusStop reports whether a mounted, visible view inside the active
// focus scope is a keyboard Tab stop.
func viewCanFocusStop(kind driver.ViewKind, props *driver.ViewProps, events driver.EventMask, inputDisabled bool) bool {
	if kind == driver.KindInput {
		return !props.Disabled && !inputDisabled
	}
	return props.FocusStop(events)
}

// widgetFocusableTargets splits a view's Tab-stop eligibility into the GTK
// focusable flags for its own NsgoView widget and its hosted input editor.
//
// Inputs host a native GTK editor (GtkEntry's GtkText or GtkTextView) that
// owns keyboard focus, so their outer NsgoView stays non-focusable while the
// editor follows Tab-stop eligibility. Ordinary Tab stops expose focusability
// directly on their NsgoView so AT-SPI reports every stop as focusable.
func widgetFocusableTargets(kind driver.ViewKind, stop bool) (viewWidget, editorWidget bool) {
	if kind == driver.KindInput {
		return false, stop
	}
	return stop, false
}

// syncFocusTree walks the tree rooted at root against its active focus scope
// and calls set(v, viewWidget, editorWidget) for each live node.
func syncFocusTree[N focusTreeNode[N]](root N, set func(v N, viewWidget, editorWidget bool)) {
	var zero N
	if root == zero {
		return
	}
	scope := activeFocusScope(root)
	syncFocusSubtree(root, scope, false, false, set)
}

// syncFocusSubtree walks the subtree rooted at v, given whether an ancestor
// entered scope and whether an ancestor is hidden, calling set on each live
// node.
func syncFocusSubtree[N focusTreeNode[N]](v, scope N, insideScope, parentHidden bool, set func(v N, viewWidget, editorWidget bool)) {
	var zero N
	if v == zero || v.focusDeleted() {
		return
	}
	hidden := parentHidden || v.focusHidden()
	inside := insideScope || v == scope
	stop := !hidden && inside && v.focusStopSelf()
	viewFocus, editorFocus := widgetFocusableTargets(v.focusKind(), stop)
	set(v, viewFocus, editorFocus)
	for _, c := range v.focusChildren() {
		syncFocusSubtree(c, scope, inside, hidden, set)
	}
}
