package core

import (
	"cmp"
	"math/bits"
	"slices"
)

// watch is the states cond reads of the element it describes. A group's states are read from the group, which reports them itself.
func (c *classCond) watch() uint16 {
	w := c.states | c.notStates
	for _, s := range c.has {
		w |= s.watch
	}
	return w
}

func (t *Tree) trackClassRelations(n *node) {
	if old := t.relationSources[n]; old != nil {
		t.queueRelationRoot(old)
		delete(t.relationSources, n)
		t.relationScopes = nil
	}
	if n.cls != nil && n.cls.list.relational {
		if t.relationSources == nil {
			t.relationSources = make(map[*node]*node)
		}
		scope := relationScope(n)
		t.relationSources[n] = scope
		t.relationScopes = nil
		n.cls.related, n.cls.watch = true, n.cls.list.watch|n.cls.list.reach
		t.queueRelationRoot(scope)
	}
	t.queueRelations(n)
}

// Descendant selectors stay in their source's subtree. A sibling or ancestor selector expands only that source's matching scope.
func relationScope(n *node) *node {
	mode := uint8(0)
	if n.cls != nil {
		mode = n.cls.list.relationScope
	}
	if mode > 0 {
		if p := n.nativeParent(); p != nil {
			n = p
		}
	}
	if mode > 1 {
		for n.nativeParent() != nil {
			n = n.nativeParent()
		}
	}
	return n
}

// 0 is descendants, 1 includes siblings, and 2 also reads ancestors.
func (s *classSelector) scopeMode() uint8 {
	mode := uint8(0)
	for _, steps := range s.choices {
		depth := 0
		for i, step := range steps {
			if step.part.anchor && i > 0 {
				mode = 2
			}
			if i > 0 {
				if step.relation == ' ' || step.relation == '>' {
					depth++
				} else if depth == 0 {
					mode = max(mode, 1)
				}
			}
			for _, f := range step.part.functions {
				if i == 0 {
					mode = max(mode, f.selector.scopeMode())
				}
			}
		}
	}
	return mode
}

// relationIndex maps each matching scope to the sources whose selectors may match inside it. It is rebuilt only after a source or its scope changes.
func (t *Tree) relationIndex() map[*node][]*node {
	if t.relationScopes == nil && len(t.relationSources) > 0 {
		t.relationScopes = make(map[*node][]*node, len(t.relationSources))
		for source, scope := range t.relationSources {
			t.relationScopes[scope] = append(t.relationScopes[scope], source)
		}
	}
	return t.relationScopes
}

// sourcesFor returns the live sources whose scope holds n, oldest first. It walks n's ancestors rather than every source, so a refresh costs the depth of the tree per node.
func (t *Tree) sourcesFor(n *node) []*node {
	index := t.relationIndex()
	if len(index) == 0 {
		return nil
	}
	var out []*node
	for a := n; a != nil; a = a.nativeParent() {
		for _, source := range index[a] {
			if !source.deleted && source.cls != nil && source.cls.list.relational {
				out = append(out, source)
			}
		}
	}
	slices.SortFunc(out, func(a, b *node) int { return cmp.Compare(a.id, b.id) })
	return out
}

func inRelationScope(n, scope *node) bool {
	for ; n != nil; n = n.nativeParent() {
		if n == scope {
			return true
		}
	}
	return false
}

func (t *Tree) queueRelationRoot(root *node) {
	if root == nil || t.refreshingRelations {
		return
	}
	if t.relationRoots == nil {
		t.relationRoots = make(map[*node]struct{})
	}
	t.relationRoots[root] = struct{}{}
	t.changed = true
}

// Only sources whose matching scope changed are refreshed, once per commit, instead of walking the app for every paint or reactive write.
func (t *Tree) queueRelations(n *node) {
	if t.refreshingRelations {
		return
	}
	for source, old := range t.relationSources {
		current := relationScope(source)
		if source.deleted || current != old {
			t.queueRelationRoot(old)
			t.relationScopes = nil
			if source.deleted {
				delete(t.relationSources, source)
				continue
			}
			t.relationSources[source] = current
			t.queueRelationRoot(current)
		}
		if inRelationScope(n, current) || inRelationScope(current, n) {
			t.queueRelationRoot(current)
		}
	}
}

func (t *Tree) updateRelationStyles() {
	if len(t.relationRoots) == 0 {
		return
	}
	t.refreshingRelations = true
	defer func() { t.refreshingRelations = false }()
	roots := make([]*node, 0, len(t.relationRoots))
	for root := range t.relationRoots {
		if !root.deleted {
			roots = append(roots, root)
		}
	}
	clear(t.relationRoots)
	var visit func(*node)
	visit = func(n *node) {
		if n.deleted {
			return
		}
		sources := t.sourcesFor(n)
		related, watch := len(sources) > 0, uint16(0)
		for _, source := range sources {
			watch |= source.cls.list.reach
		}
		if related && n.cls == nil {
			var env classEnv
			Untracked(n.owner, func() bool { env = t.classEnv(n); return true })
			n.cls = &nodeClasses{list: t.rt.parseClasses(&env, " "), env: env}
		}
		if c := n.cls; c != nil && (related || c.related) {
			wasSpace := c.props.hasSpaceX || c.props.hasSpaceY
			c.related, c.watch = related, watch|c.list.watch
			Untracked(n.owner, func() bool { c.env = t.classEnv(n); return true })
			if n.created {
				c.env.rtl = n.rtl
			}
			if !c.list.parsedFor(&c.env) {
				c.list = t.rt.parseClasses(&c.env, c.list.class)
				t.rt.reportUnknown(c.list)
			}
			if related && !c.bound {
				c.bound = true
				t.followEnv(n)
			}
			t.resolveClasses(n)
			if t.restyle(n) {
				t.markLayout(n)
			}
			if wasSpace || c.list.spacing || c.props.hasSpaceX || c.props.hasSpaceY {
				// A former spacing rule must clear margins even after the source no longer has any classes or matching rules.
				for _, child := range n.flatChildren() {
					if t.restyle(child) {
						t.markLayout(child)
					}
				}
			}
		}
		for _, k := range n.flatChildren() {
			visit(k)
		}
	}
	for _, root := range roots {
		covered := false
		for _, other := range roots {
			if other != root && inRelationScope(root, other) {
				covered = true
				break
			}
		}
		if !covered {
			visit(root)
		}
	}
}

func (t *Tree) relationCondition(n *node, cond *classCond) bool {
	var env classEnv
	if n.cls != nil {
		env = n.cls.env
	} else {
		Untracked(n.owner, func() bool { env = t.classEnv(n); return true })
	}
	env.position = n.position()
	env.has = func(s *classSelector) bool { return s.has(t, n) }
	env.peer = func(p classPeer) bool { return t.matchesPeer(n, p) }
	if !cond.matches(&env) || env.position&cond.position != cond.position {
		return false
	}
	shown, states := t.selectorStates(n)
	if states&cond.states != cond.states || states&cond.notStates != 0 || cond.data != "" && !slices.Contains(shown.data, cond.data) {
		return false
	}
	if g := cond.group; g != nil {
		group := t.groupState(n, g.name)
		var flags uint16
		for i, on := range group.on {
			if on {
				flags |= classStates[i]
			}
		}
		if flags&g.states != g.states || g.data != "" && !slices.Contains(group.data, g.data) {
			return false
		}
	}
	return true
}

func (t *Tree) relationMatches(rule *classRule, source, target *node) bool {
	var match func(int, *node) bool
	match = func(i int, n *node) bool {
		if i < 0 {
			return n == source
		}
		r := &rule.relations[i]
		return r.selector.eachAnchor(t, n, nil, func(anchor *node) bool {
			return anchor != nil && t.relationCondition(anchor, &r.condition) && match(i-1, anchor)
		})
	}
	return match(len(rule.relations)-1, target) && t.relationCondition(target, &rule.cond)
}

// Specificity is considered before variant rank for selectors that compete on the same target. & carries the utility's one class; a type adds one, an attribute/class/pseudo-class adds 4096, and :where adds 0.
func (s *classSelector) specificity() int {
	best := 0
	for _, steps := range s.choices {
		score := 0
		for _, step := range steps {
			p := &step.part
			if p.tag != "" && p.tag != "*" {
				score++
			}
			score += 4096 * (len(p.classes) + len(p.attrs) + bits.OnesCount16(p.states|p.notStates) + bits.OnesCount8(p.position))
			if p.nth > 0 {
				score += 4096
			}
			for _, f := range p.functions {
				if f.name != "where" {
					score += f.selector.specificity()
				}
			}
		}
		best = max(best, score)
	}
	return best
}

func (c *classCond) specificity() int {
	score := 4096 * (bits.OnesCount16(c.states|c.notStates) + bits.OnesCount8(c.position))
	if c.data != "" {
		score += 4096
	}
	if g := c.group; g != nil {
		score += 4096 * bits.OnesCount16(g.states)
		if g.data != "" {
			score += 4096
		}
	}
	for _, p := range c.peers {
		score += 4096 * bits.OnesCount16(p.states|p.notStates)
		if p.data != "" {
			score += 4096
		}
	}
	for _, s := range c.has {
		score += s.specificity()
	}
	return score
}

// stateful reports whether cond reads states, as a state variant's style does.
func (c *classCond) stateful() bool {
	return c.states != 0 || c.notStates != 0 || c.data != "" || c.group != nil || len(c.peers) > 0 || len(c.has) > 0
}

func (r *classRule) specificity() int {
	score := 4096 + r.cond.specificity()
	for _, relation := range r.relations {
		score += relation.selector.specificity() + relation.condition.specificity()
	}
	return score
}

func (t *Tree) resolveRelationClasses(n *node) {
	c := n.cls
	type matched struct {
		rule        classRule
		specificity int
	}
	var rules []matched
	appendRule := func(r classRule, source *node) {
		if !t.relationMatches(&r, source, n) {
			return
		}
		specificity := r.specificity()
		r.resolvedState = r.cond.stateful()
		for _, relation := range r.relations {
			r.resolvedState = r.resolvedState || relation.selector.watch != 0 || relation.condition.stateful()
		}
		// Conditions have been matched to their actual elements. Keeping them as state slots would put an ancestor's hover on the child and could make a less-specific local state win the cascade.
		r.cond = classCond{placeholder: r.cond.placeholder, selection: r.cond.selection}
		r.relations = nil
		rules = append(rules, matched{r, specificity})
	}
	for _, r := range c.list.rules {
		appendRule(r, n)
	}
	for _, source := range t.sourcesFor(n) {
		if source == n {
			continue
		}
		for _, r := range source.cls.list.rules {
			if len(r.relations) > 0 {
				appendRule(r, source)
			}
		}
	}
	slices.SortStableFunc(rules, func(a, b matched) int {
		if a.rule.important != b.rule.important {
			if a.rule.important {
				return 1
			}
			return -1
		}
		if order := cmp.Compare(a.specificity, b.specificity); order != 0 {
			return order
		}
		return a.rule.rank.compare(b.rule.rank)
	})
	list := *c.list
	list.rules = make([]classRule, len(rules))
	for i, r := range rules {
		list.rules[i] = r.rule
	}
	env := c.env
	normal, important, props := list.resolve(&env)
	if groups := c.list.groups; len(groups) > 0 {
		normal.Group = groups[len(groups)-1]
		if normal.Group == "" {
			normal.Group = "group"
		}
	}
	c.normal = t.internStyle(&normal)
	c.important = t.internStyle(&important)
	c.props = props
}
