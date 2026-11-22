package core

// PointerHit is a reference reached by a committed-geometry pointer hit test.
// Position is in that reference's element.
type PointerHit struct {
	Ref      *Ref
	Position Point
}

// HitTest returns the supplied references reached by p in surface points,
// innermost first. Live references must be in one surface; references from a
// different surface than the first live one are ignored. One tree walk follows
// committed transforms, scrolling, clipping, pointer events and stacking;
// disabled subtrees and views below a modal receive nothing. base/dragdrop uses
// the batch form to avoid walking the tree again for every registered target.
// Native animations between commits are not sampled by this query.
func HitTest(p Point, refs ...*Ref) []PointerHit {
	var tree *Tree
	nodes := make(map[*node]*Ref, len(refs))
	for _, r := range refs {
		if !r.placed() || !r.n.alive() {
			continue
		}
		if tree == nil {
			tree = r.t
		}
		if r.t == tree && tree.labelActive(r.n) {
			nodes[r.n] = r
		}
	}
	if tree == nil || tree.view == nil || len(nodes) == 0 {
		return nil
	}
	id, _ := hitWith(tree.view, p, func(*node) bool { return true })
	leaf := tree.node(id)
	for n := leaf; n != nil; n = n.nativeParent() {
		if n.labelProps().Disabled {
			return nil
		}
	}
	var hits []PointerHit
	for n := leaf; n != nil; n = n.nativeParent() {
		if r := nodes[n]; r != nil {
			hits = append(hits, PointerHit{Ref: r, Position: n.surfacePoint(p)})
		}
	}
	return hits
}

// HitTest reports whether p, in surface points, reaches the element or one of
// its descendants and returns p in its coordinates, using HitTest's committed
// geometry and interaction boundaries.
func (r *Ref) HitTest(p Point) (Point, bool) {
	hits := HitTest(p, r)
	if len(hits) == 0 {
		return Point{}, false
	}
	return hits[0].Position, true
}
