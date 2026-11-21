package core

import (
	"log"
	"slices"
	"strings"
)

// A node's Tailwind classes (NSH's core/classes.rs): resolved for the
// environment when they are set and again when what they depend on
// changes, then merged under the node's own style.

// nodeClasses are a node's classes and what they resolved to.
type nodeClasses struct {
	list              *classList
	env               classEnv // what they were resolved for
	normal, important Style
	props             classProps
	// position is the node's place among its parent's native children, from the parent's last child walk (classKids). Only positional classes keep it current.
	position uint8
	// bound is set once the node follows the environment.
	bound   bool
	related bool
	watch   uint16
}

// setClasses gives n the classes in class, resolved now.
func (t *Tree) setClasses(n *node, class string) {
	t.queuePeerSiblings(n)
	if n.cls != nil && n.cls.list.spacing {
		t.queueClassKids(n)
	}
	if class == "" {
		// A node that follows the environment keeps following it, with
		// no classes, so classes set again do not follow it twice.
		if n.cls == nil || !n.cls.bound {
			n.cls = nil
			t.trackClassRelations(n)
			return
		}
		class = " "
	}
	var env classEnv
	Untracked(n.owner, func() bool { env = t.classEnv(n); return true })
	l := t.rt.parseClasses(&env, class)
	if n.cls != nil && n.cls.list == l {
		return
	}
	t.rt.reportUnknown(l)
	c := &nodeClasses{list: l, env: env}
	old := n.cls
	if old != nil {
		c.bound = old.bound
		c.position = old.position
	}
	n.cls = c
	if l.positional && (old == nil || !old.list.positional) {
		// The node's place is not known yet: its parent's walk resolves it again before the tree resolves, so mounting N positional children never searches N siblings each.
		if p := n.nativeParent(); p != nil {
			t.queueClassKids(p)
		}
	}
	if len(l.peers) > 0 || l.peerDependent {
		t.peerStyled = true
		t.queuePeerParent(n.nativeParent())
	}
	t.trackClassRelations(n)
	t.resolveClasses(n)
	// A replacement theme may define a previously unknown color.
	if (l.env || len(l.unknown) > 0 || hasEnvSpans(n)) && !c.bound {
		c.bound = true
		t.followEnv(n)
	}
}

// bindClasses gives n class and fn's classes after them, again when what
// fn reads changes.
func (t *Tree) bindClasses(cx *Scope, n *node, class string, fn func() string) {
	if fn == nil {
		t.setClasses(n, class)
		return
	}
	first := true
	cx.renderEffect(func() {
		t.setClasses(n, joinClasses(class, fn()))
		if first {
			first = false
			return
		}
		if t.restyle(n) {
			t.markLayout(n)
		}
		t.restyleGroup(n)
	})
}

// joinClasses returns a's classes then b's.
func joinClasses(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + " " + b
}

// followSpanClasses has text n follow the environment for its spans'
// classes, when they depend on it.
func (t *Tree) followSpanClasses(n *node) {
	if !hasEnvSpans(n) {
		return
	}
	if n.cls == nil {
		var env classEnv
		Untracked(n.owner, func() bool { env = t.classEnv(n); return true })
		n.cls = &nodeClasses{list: t.rt.parseClasses(&env, " "), env: env}
	}
	if !n.cls.bound {
		n.cls.bound = true
		t.followEnv(n)
	}
}

// followEnv resolves n's classes again whenever the environment they
// were resolved for changes: the appearance, the window, the direction,
// the motion setting or the theme. The effect is the node's owner's, not
// that of a binding that set the classes, so it lasts as long as n.
func (t *Tree) followEnv(n *node) {
	first := true
	t.within(n.owner, func() {
		n.owner.renderEffect(func() {
			env := t.classEnv(n)
			if first {
				first = false
				return
			}
			c := n.cls
			if c == nil || n.deleted {
				return
			}
			c.env = env
			if n.created {
				c.env.rtl = n.rtl
			}
			if !c.list.parsedFor(&env) {
				c.list = t.rt.parseClasses(&env, c.list.class)
				t.rt.reportUnknown(c.list)
				t.trackClassRelations(n)
			}
			t.resolveClasses(n)
			t.resolveSpanClasses(n)
			if t.restyle(n) {
				t.markLayout(n)
			}
			t.restyleGroup(n)
		})
	})
}

// classEnv reads the environment n's classes resolve for, tracked.
func (t *Tree) classEnv(n *node) classEnv {
	o := n.owner
	e := o.Env()
	pal := o.Palette()
	return classEnv{dark: o.Dark(), window: e.Window(), rtl: e.RTL(), reduce: e.ReduceMotion(), contrast: e.IncreaseContrast(), os: e.OS(),
		pointer: e.Pointer(), palette: &pal, tw: o.Theme().tailwind(), theme: o.Theme(), hostColors: e.tokens.Get()}
}

// resolveClasses resolves n's classes for their environment and n's place among its siblings, as its parent's last child walk found it.
func (t *Tree) resolveClasses(n *node) {
	t.resolveClassesAt(n, n.cls.position)
}

// resolveClassesAt resolves n's classes at position, found by a walk of its parent's children.
func (t *Tree) resolveClassesAt(n *node, position uint8) {
	c := n.cls
	c.position = position
	env := c.env
	if c.list.peerDependent {
		env.peer = func(p classPeer) bool { return t.matchesPeer(n, p) }
	}
	if c.list.spacing {
		t.queueClassKids(n)
	}
	if c.related {
		t.resolveRelationClasses(n)
		return
	}
	if len(c.list.rules) > 0 && c.list.relational {
		env.has = func(s *classSelector) bool { return s.has(t, n) }
	}
	env.position = position
	c.normal, c.important, c.props = c.list.resolve(&env)
	if g := c.list.groups; len(g) > 0 {
		// An unnamed group takes a name, as only named groups are groups
		// to Style; GroupStyle's empty Name follows any.
		c.normal.Group = g[len(g)-1]
		if c.normal.Group == "" {
			c.normal.Group = "group"
		}
	}
}

// position returns n's place among the native children of its parent, searched now. Relational selectors read it of the views they match; a node's own positional classes take theirs from its parent's child walk instead.
func (n *node) position() uint8 {
	p := n.nativeParent()
	if p == nil {
		return 0
	}
	kids := p.flatChildren()
	return childPosition(slices.Index(kids, n), len(kids))
}

// childPosition returns the positional variants of child i among count children.
func childPosition(i, count int) uint8 {
	if i < 0 {
		return 0
	}
	var pos uint8
	if i == 0 {
		pos |= positionFirst
	}
	if i == count-1 {
		pos |= positionLast
	}
	if count == 1 {
		pos |= positionOnly
	}
	// odd: and even: count from 1, as CSS's nth-child.
	if i%2 == 0 {
		pos |= positionOdd
	} else {
		pos |= positionEven
	}
	return pos
}

// queueClassKids defers sibling work until the tree has finished changing. A parent holds no lasting dependency after its positional children move away or lose their classes.
func (t *Tree) queueClassKids(p *node) {
	if t.classParents == nil {
		t.classParents = make(map[*node]bool)
	}
	t.classParents[p] = t.classParents[p] || p.cls != nil && p.cls.list.spacing
	t.changed = true
}

func (t *Tree) updateClassKids() {
	for len(t.classParents) > 0 {
		for p, spacing := range t.classParents {
			delete(t.classParents, p)
			if !p.deleted {
				t.classKids(p, spacing)
			}
		}
	}
}

// classKids restyles p's children that follow their place among their
// siblings or the spacing p gives them, as p's children changed.
func (t *Tree) classKids(p *node, spacing bool) {
	spacing = spacing || p.cls != nil && p.cls.list.spacing
	kids := p.flatChildren()
	for i, c := range kids {
		positional := c.cls != nil && c.cls.list.positional
		if !spacing && !positional {
			continue
		}
		if positional {
			t.resolveClassesAt(c, childPosition(i, len(kids)))
		}
		if t.restyle(c) {
			t.markLayout(c)
		}
	}
}

// withClasses merges n's classes under style, and its important classes
// and the spacing its parent gives it over.
func (n *node) withClasses(style Style) Style {
	if c := n.cls; c != nil {
		style = mergeStyles(mergeStyles(c.normal, style), c.important)
	}
	if p := n.nativeParent(); p != nil && p.cls != nil && (p.cls.props.hasSpaceX || p.cls.props.hasSpaceY) {
		sp := p.cls.props
		kids := p.flatChildren()
		if len(kids) > 0 && kids[len(kids)-1] != n {
			m := &style.Margin
			if sp.hasSpaceX {
				side := &m.Right
				if sp.reverseX {
					side = &m.Left
				}
				if *side == 0 || sp.spaceImportant {
					*side = sp.spaceX
				}
			}
			if sp.hasSpaceY {
				side := &m.Bottom
				if sp.reverseY {
					side = &m.Top
				}
				if *side == 0 || sp.spaceImportant {
					*side = sp.spaceY
				}
			}
		}
	}
	return style
}

// mergeStyles is a.Merge(b), where edges and corners merge side by side
// and the styles of the states and of the group merge too, rather than
// b's replacing a's, as CSS's properties do.
func mergeStyles(a, b Style) Style {
	out := a.Merge(b)
	out.Margin, out.Padding, out.Borders = sides(a.Margin, b.Margin), sides(a.Padding, b.Padding), sides(a.Borders, b.Borders)
	for _, c := range []struct{ dst, x, y *Color }{
		{&out.BorderColors.Top, &a.BorderColors.Top, &b.BorderColors.Top},
		{&out.BorderColors.Right, &a.BorderColors.Right, &b.BorderColors.Right},
		{&out.BorderColors.Bottom, &a.BorderColors.Bottom, &b.BorderColors.Bottom},
		{&out.BorderColors.Left, &a.BorderColors.Left, &b.BorderColors.Left},
	} {
		*c.dst = cmpOr(*c.y, *c.x)
	}
	if b.CornerRadii != (Corners{}) && a.CornerRadii != (Corners{}) {
		out.CornerRadii = Corners{TopLeft: cmpOr(b.CornerRadii.TopLeft, a.CornerRadii.TopLeft),
			TopRight:    cmpOr(b.CornerRadii.TopRight, a.CornerRadii.TopRight),
			BottomRight: cmpOr(b.CornerRadii.BottomRight, a.CornerRadii.BottomRight),
			BottomLeft:  cmpOr(b.CornerRadii.BottomLeft, a.CornerRadii.BottomLeft)}
	}
	mergeStates(out.states(), a.states(), b.states())
	if a.GroupStyle != nil && b.GroupStyle != nil && a.GroupStyle.Name == b.GroupStyle.Name {
		g := *b.GroupStyle
		mergeStates(g.states(), a.GroupStyle.states(), b.GroupStyle.states())
		out.GroupStyle = &g
	}
	return out
}

// mergeStates sets out's states to b's over a's, merging those both have.
func mergeStates(out, a, b stateSet) {
	for i := range out.slots {
		x, y := *a.slots[i], *b.slots[i]
		switch {
		case y == nil:
			*out.slots[i] = x
		case x != nil:
			m := mergeStyles(*x, *y)
			*out.slots[i] = &m
		default:
			*out.slots[i] = y
		}
	}
	if len(*a.data) > 0 && len(*b.data) > 0 {
		merged := slices.Clone(*a.data)
		set := stateSet{data: &merged}
		for _, d := range *b.data {
			m := set.dataStyle(d.Name)
			*m = mergeStyles(*m, *d.Style)
		}
		*out.data = merged
	}
}

// sides is b's sides over a's, side by side.
func sides(a, b Edges) Edges {
	return Edges{Top: cmpOr(b.Top, a.Top), Right: cmpOr(b.Right, a.Right), Bottom: cmpOr(b.Bottom, a.Bottom),
		Left: cmpOr(b.Left, a.Left)}
}

// resizeMode is the fit of image or video n: its style's over its own.
func (n *node) resizeMode(own, def ResizeMode) ResizeMode {
	return resolveMode(cmpOr(n.style.ObjectFit, own), def)
}

// textLimits returns text n's line limit and how it ellipsizes: its
// style's, classes included, else its own.
func (n *node) textLimits() (int, EllipsizeMode) {
	tn := n.text
	lines := cmpOr(n.style.NumberOfLines, tn.maxLines)
	return max(lines, 0), cmpOr(n.style.TextOverflow, tn.ellipsize)
}

// spanClasses are the classes of a text's spans.
type spanClasses struct {
	list *classList
	base Style // the span's own style
}

// spanStyle returns a span's style: its classes' under its own, resolved
// for text n's environment.
func (t *Tree) spanStyle(n *node, class string, style Style) (Style, *spanClasses) {
	if class == "" {
		return style, nil
	}
	var env classEnv
	Untracked(n.owner, func() bool { env = t.classEnv(n); return true })
	l := t.rt.parseClasses(&env, class)
	t.rt.reportClassesWithoutPeers(l, "span classes")
	sc := &spanClasses{list: l, base: style}
	return sc.resolve(&env), sc
}

func (sc *spanClasses) resolve(env *classEnv) Style {
	normal, important, _ := sc.list.resolve(env)
	return mergeStyles(mergeStyles(normal, sc.base), important)
}

// hasEnvSpans reports whether text n has spans whose classes follow the
// environment.
func hasEnvSpans(n *node) bool {
	if n.text == nil {
		return false
	}
	for _, sp := range n.text.spans {
		if sp.cls != nil && (sp.cls.list.env || len(sp.cls.list.unknown) > 0) {
			return true
		}
	}
	return false
}

// resolveSpanClasses resolves the classes of text n's spans again.
func (t *Tree) resolveSpanClasses(n *node) {
	if n.text == nil || n.cls == nil {
		return
	}
	env := n.cls.env
	changed := false
	for i := range n.text.spans {
		sp := &n.text.spans[i]
		if sp.cls != nil && !sp.cls.list.parsedFor(&env) {
			sp.cls.list = t.rt.parseClasses(&env, sp.cls.list.class)
			t.rt.reportClassesWithoutPeers(sp.cls.list, "span classes")
			sp.style = sp.cls.resolve(&env)
			changed = true
		} else if sp.cls != nil && sp.cls.list.env {
			sp.style = sp.cls.resolve(&env)
			changed = true
		}
	}
	if changed {
		n.text.stale = true
		t.markResolve(n)
	}
}

// reportClassesWithoutPeers checks classes used without a view's sibling context. Keep the cached list intact: an ordinary view can use the same classes.
func (rt *runtime) reportClassesWithoutPeers(l *classList, where string) {
	rt.reportUnknown(l)
	if !l.peerDependent {
		return
	}
	for _, token := range strings.Fields(l.class) {
		if slices.Contains(l.unknown, token) {
			continue
		}
		parts := splitVariants(token)
		if !slices.ContainsFunc(parts[:len(parts)-1], func(v string) bool { return strings.HasPrefix(v, "peer-") }) {
			continue
		}
		key := where + "\x00" + token
		if _, seen := rt.reported.get(key); !seen {
			rt.reported.put(key, true)
			log.Printf("core: class %q is unsupported in %s: peer variants require sibling views", token, where)
		}
	}
}

// reportUnknown logs unknown classes once while this graph caches them.
func (rt *runtime) reportUnknown(l *classList) {
	for _, u := range l.unknown {
		if _, seen := rt.reported.get(u); !seen {
			rt.reported.put(u, true)
			log.Printf("core: unknown class %q", u)
		}
	}
}
