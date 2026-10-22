package core

// A stack shows its last screen that has not been popped. Back pops the
// top screen: it reports OnDismiss and stays hidden until the app removes
// it, and the last screen never pops.

type screenNode struct {
	screen *Screen
	popped bool
	hidden bool
}

func (t *Tree) mountStack(cx *Scope, parent *node, s Stack) {
	n := t.newNode(KindView, parent, cx)
	t.bindStyle(cx, n, Style{Overflow: Hidden}.Merge(s.Style), nil)
	n.props = ViewProps{TestID: s.TestID}
	n.isStack = true
	t.stacks = append(t.stacks, n)
	for _, c := range s.Children {
		t.mount(cx, n, c)
	}
}

var fill = Style{Position: Absolute, Top: Pt(0), Right: Pt(0), Bottom: Pt(0), Left: Pt(0)}

func (t *Tree) mountScreen(cx *Scope, parent *node, s Screen) {
	n := t.newNode(KindView, parent, cx)
	t.bindStyle(cx, n, fill.Merge(s.Style), nil)
	n.props = ViewProps{Label: s.Title, TestID: s.TestID}
	n.screen = &screenNode{screen: &s}
	if s.Header != nil {
		t.mount(cx, n, s.Header)
	}
	// Content below a header keeps clear of the safe area's sides and
	// bottom only.
	inner := cx.Child()
	header := s.Header != nil
	insetsKey.Provide(inner, func() Edges {
		in := cx.Env().SafeArea()
		if header {
			in.Top = 0
		}
		return in
	})
	t.within(inner, func() {
		for _, c := range s.Children {
			t.mount(inner, n, c)
		}
	})
}

// showTop hides every screen of stack n but its top one.
func (t *Tree) showTop(n *node) {
	var top *node
	for _, c := range n.flatChildren() {
		if c.screen != nil && !c.screen.popped {
			top = c
		}
	}
	for _, c := range n.flatChildren() {
		if c.screen == nil {
			continue
		}
		if hidden := c != top; hidden != c.screen.hidden {
			c.screen.hidden = hidden
			c.restyle()
			t.relayout(c)
		}
	}
}

// screens returns the screens of stack n still shown or showable.
func (n *node) screens() (out []*node) {
	for _, c := range n.flatChildren() {
		if c.screen != nil && !c.screen.popped {
			out = append(out, c)
		}
	}
	return out
}

// pop handles back on stack n's top screen, reporting whether the stack
// took it.
func (t *Tree) pop(n *node) bool {
	screens := n.screens()
	if len(screens) < 2 {
		return false
	}
	top := screens[len(screens)-1]
	s := top.screen.screen
	if s.OnBackPress != nil {
		t.within(top.owner, s.OnBackPress)
	}
	if s.NoPopOnBack {
		return true
	}
	top.screen.popped = true
	t.showTop(n)
	if s.OnDismiss != nil {
		t.within(top.owner, s.OnDismiss)
	}
	return true
}

// innerStack returns the innermost stack under host, outside nested
// modals, that can pop.
func (t *Tree) innerStack(host *node) *node {
	var found *node
	var walk func(n *node)
	walk = func(n *node) {
		if n.deleted || (n != host && n.kind == KindModal) || n.style.Display == DisplayNone {
			return
		}
		if n.isStack && len(n.screens()) >= 2 {
			found = n
		}
		for _, c := range n.children {
			if c.hoist == nil {
				walk(c)
			}
		}
	}
	walk(host)
	return found
}
