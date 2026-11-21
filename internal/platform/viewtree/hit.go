// Package viewtree traverses the self-drawn view trees on Windows and Linux.
package viewtree

import (
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Node gives the traversal access to a backend's current view geometry.
type Node[V any] interface {
	comparable
	PointerVisibility() (hidden, transparent bool)
	PointerReach(p driver.Point) (local driver.Point, self, children bool)
	PointerChildren(local driver.Point) (content driver.Point, children []V)
	ScrollbarContains(local driver.Point) bool
}

// Hit returns the topmost matching view. The point is in the parent's
// content coordinates. An unmatched view lets the search continue below it.
func Hit[V Node[V]](v V, p driver.Point, match func(V) bool) V {
	var none V
	if hidden, _ := v.PointerVisibility(); hidden {
		return none
	}
	local, self, children := v.PointerReach(p)
	if children {
		content, kids := v.PointerChildren(local)
		for _, child := range slices.Backward(kids) {
			if hit := Hit(child, content, match); hit != none {
				return hit
			}
		}
	}
	if self && match(v) {
		return v
	}
	return none
}

// ViewsAt appends matching views along the frontmost pointer path, outermost
// first. It reports whether the subtree takes the pointer, even if none of
// its reachable views match. Hidden and fully transparent views pass through.
func ViewsAt[V Node[V]](v V, p driver.Point, match func(V) bool, out []V) ([]V, bool) {
	hidden, transparent := v.PointerVisibility()
	if hidden || transparent {
		return out, false
	}
	local, self, children := v.PointerReach(p)
	if self && match(v) {
		out = append(out, v)
	}
	if children {
		content, kids := v.PointerChildren(local)
		for _, child := range slices.Backward(kids) {
			var took bool
			out, took = ViewsAt(child, content, match, out)
			if took {
				return out, true
			}
		}
	}
	return out, self
}

// BarAt returns the innermost reachable scrollbar and the point in its view's
// coordinates. If there is no scrollbar at the point, it returns p unchanged.
func BarAt[V Node[V]](v V, p driver.Point) (V, driver.Point) {
	hit, at, _ := barIn(v, p)
	return hit, at
}

func barIn[V Node[V]](v V, p driver.Point) (V, driver.Point, bool) {
	var none V
	hidden, transparent := v.PointerVisibility()
	if hidden || transparent {
		return none, p, false
	}
	local, self, children := v.PointerReach(p)
	took := self
	if children {
		content, kids := v.PointerChildren(local)
		for _, child := range slices.Backward(kids) {
			hit, at, childTook := barIn(child, content)
			if hit != none {
				return hit, at, true
			}
			if childTook {
				took = true
				break
			}
		}
	}
	if self && v.ScrollbarContains(local) {
		return v, local, true
	}
	return none, p, took
}
