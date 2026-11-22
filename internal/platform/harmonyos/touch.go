package harmonyos

import "github.com/nsgo-dev/nsgo/internal/driver"

// touchNode tracks the geometry, transforms, scroll state, pointer events and
// touch listeners of one HarmonyOS view in Go so EventTouch snapshots follow
// bound and animated presentation transforms without changing Index.ets.
type touchNode struct {
	id              driver.NodeID
	kind            driver.ViewKind
	parent          *touchNode
	children        []*touchNode
	frame           driver.Bounds
	style           driver.ViewStyle
	props           driver.ViewProps
	events          driver.EventMask
	overflow        driver.Bounds
	transformedKids bool
	offset          driver.Point
	zoom            float32
	zoomMin         float32
	zoomMax         float32
	zoomSet         bool
	horizontal      bool
	sticky          []driver.StickyCell
	shift           driver.Point
	deleted         bool
}

func (n *touchNode) shownFrame() driver.Bounds {
	f := n.frame
	f.Origin.X += n.shift.X
	f.Origin.Y += n.shift.Y
	return f
}

// touchScene maintains the HarmonyOS view hierarchy for presentation-space
// touch hit testing and local coordinate snapshotting.
type touchScene struct {
	nodes   map[driver.NodeID]*touchNode
	root    driver.NodeID
	anim    *driver.Animator
	tracker driver.TouchTracker
}

func newTouchScene(anim *driver.Animator) *touchScene {
	return &touchScene{nodes: map[driver.NodeID]*touchNode{}, anim: anim}
}

func (s *touchScene) detach(n *touchNode) {
	p := n.parent
	if p == nil {
		return
	}
	for i, c := range p.children {
		if c == n {
			p.children = append(p.children[:i], p.children[i+1:]...)
			break
		}
	}
	n.parent = nil
}

// applyBatch applies a whole mount batch, including animation bindings, for
// unit tests and standalone callers.
func (s *touchScene) applyBatch(b *driver.MountBatch) {
	for i := range b.Items {
		it := &b.Items[i]
		if it.Kind == driver.ItemAnimation {
			if s.anim != nil {
				s.anim.Apply(it.Animation)
			}
			continue
		}
		s.applyItem(b.Root, it)
	}
}

// applyItem records the view-tree and presentation geometry effects of it.
func (s *touchScene) applyItem(root driver.NodeID, it *driver.MountItem) {
	switch it.Kind {
	case driver.ItemCreate:
		n := &touchNode{
			id:      it.ID,
			kind:    it.View,
			style:   driver.DefaultViewStyle,
			zoom:    1,
			zoomMin: 1,
			zoomMax: 1,
		}
		s.nodes[it.ID] = n
		if it.ID == root {
			s.root = it.ID
		}
	case driver.ItemInsert:
		n, parent := s.nodes[it.ID], s.nodes[it.Parent]
		if n == nil || parent == nil {
			return
		}
		s.detach(n)
		at := len(parent.children)
		if it.Before != 0 {
			for i, c := range parent.children {
				if c.id == it.Before {
					at = i
					break
				}
			}
		}
		parent.children = append(parent.children, nil)
		copy(parent.children[at+1:], parent.children[at:])
		parent.children[at] = n
		n.parent = parent
	case driver.ItemRemove:
		if n := s.nodes[it.ID]; n != nil {
			s.detach(n)
		}
	case driver.ItemDelete:
		if n := s.nodes[it.ID]; n != nil {
			s.detach(n)
			for _, c := range n.children {
				c.parent = nil
			}
			n.children = nil
			n.deleted = true
			delete(s.nodes, it.ID)
			if s.root == it.ID {
				s.root = 0
			}
		}
	case driver.ItemFrame:
		if n := s.nodes[it.ID]; n != nil {
			n.frame = it.Frame
		}
	case driver.ItemViewStyle:
		if n := s.nodes[it.ID]; n != nil && it.ViewStyle != nil {
			n.style = *it.ViewStyle
		}
	case driver.ItemViewProps:
		if n := s.nodes[it.ID]; n != nil && it.ViewProps != nil {
			n.props = *it.ViewProps
		}
	case driver.ItemEvents:
		if n := s.nodes[it.ID]; n != nil {
			n.events = it.Events
		}
	case driver.ItemOverflow:
		if n := s.nodes[it.ID]; n != nil {
			n.overflow, n.transformedKids = it.Frame, it.Transformed
		}
	case driver.ItemScrollConfig:
		if n := s.nodes[it.ID]; n != nil && it.Scroll != nil {
			n.horizontal = it.Scroll.Horizontal
			zmin, zmax := it.Scroll.MinZoomScale, it.Scroll.MaxZoomScale
			if zmin <= 0 {
				zmin = 1
			}
			if zmax < zmin {
				zmax = zmin
			}
			n.zoomMin, n.zoomMax = zmin, zmax
			if !n.zoomSet {
				z := it.Scroll.ZoomScale
				if z <= 0 {
					z = 1
				}
				n.zoom = min(max(z, zmin), zmax)
			} else {
				n.zoom = min(max(n.zoom, zmin), zmax)
			}
			s.stick(n)
		}
	case driver.ItemZoomTo:
		if it.Zoom != nil {
			s.setZoom(it.ID, it.Zoom.Scale, nil)
		}
	case driver.ItemStickyCells:
		if n := s.nodes[it.ID]; n != nil {
			for _, c := range n.sticky {
				if cell := s.nodes[c.Cell]; cell != nil {
					cell.shift = driver.Point{}
				}
			}
			n.sticky = it.Sticky
			s.stick(n)
		}
	case driver.ItemScrollTo:
		s.setScrollOffset(it.ID, it.Point)
	case driver.ItemScrollBy:
		if n := s.nodes[it.ID]; n != nil {
			s.setScrollOffset(it.ID, driver.Point{X: n.offset.X + it.Point.X, Y: n.offset.Y + it.Point.Y})
		}
	}
}

func (s *touchScene) setScrollOffset(id driver.NodeID, offset driver.Point) {
	if n := s.nodes[id]; n != nil {
		n.offset = offset
		s.stick(n)
	}
}

func (s *touchScene) setZoom(id driver.NodeID, scale float32, offset *driver.Point) {
	n := s.nodes[id]
	if n == nil {
		return
	}
	if scale <= 0 {
		scale = 1
	}
	n.zoom = min(max(scale, n.zoomMin), n.zoomMax)
	n.zoomSet = true
	if offset != nil {
		n.offset = *offset
		s.stick(n)
	}
}

func (s *touchScene) stick(n *touchNode) {
	at := n.offset.Y
	if n.horizontal {
		at = n.offset.X
	}
	for _, c := range n.sticky {
		cell := s.nodes[c.Cell]
		if cell == nil {
			continue
		}
		d := c.StickyShift(at)
		if n.horizontal {
			cell.shift = driver.Point{X: d}
		} else {
			cell.shift = driver.Point{Y: d}
		}
	}
}

func (s *touchScene) shownStyle(n *touchNode) driver.ViewStyle {
	st := n.style
	if s.anim != nil {
		if vals, ok := s.anim.Values(n.id); ok {
			st = vals.Apply(st)
		}
	}
	return st
}

func (s *touchScene) pointerMatrix(n *touchNode) driver.Matrix3D {
	st := s.shownStyle(n)
	f := n.shownFrame()
	m := st.Matrix(f.Size)
	if p := n.parent; p != nil {
		at := f.Origin
		if p.kind == driver.KindScroll {
			at.X -= p.offset.X
			at.Y -= p.offset.Y
		}
		m = driver.ChildPerspective(s.shownStyle(p).ChildPerspective, p.frame.Size, at).Mul(m)
	}
	return m
}

func (s *touchScene) surfacePoint(n *touchNode, p driver.Point) (driver.Point, bool) {
	if n.parent != nil {
		var ok bool
		p, ok = s.surfacePoint(n.parent, p)
		if !ok {
			return driver.Point{}, false
		}
		if n.parent.kind == driver.KindScroll {
			p.X += n.parent.offset.X
			p.Y += n.parent.offset.Y
			if n.parent.zoom > 0 && n.parent.zoom != 1 {
				p.X /= driver.Points(n.parent.zoom)
				p.Y /= driver.Points(n.parent.zoom)
			}
		}
	}
	return driver.IntoView(n.shownFrame(), s.pointerMatrix(n), p)
}

func (s *touchScene) hasTransformedDescendant(n *touchNode) bool {
	for _, c := range n.children {
		if c == nil || c.deleted {
			continue
		}
		st := s.shownStyle(c)
		if st.Hidden {
			continue
		}
		if st.Transform != (driver.Transform{}) || c.shift != (driver.Point{}) {
			return true
		}
		if !st.Clip && c.kind != driver.KindScroll && s.hasTransformedDescendant(c) {
			return true
		}
	}
	return false
}

func (s *touchScene) hit(n *touchNode, p driver.Point) *touchNode {
	if n == nil || n.deleted {
		return nil
	}
	st := s.shownStyle(n)
	if st.Hidden {
		return nil
	}
	f := n.shownFrame()
	local, ok := driver.IntoView(f, s.pointerMatrix(n), p)
	if !ok {
		return nil
	}
	area, transformed := n.overflow, n.transformedKids
	if st.Clip || n.kind == driver.KindScroll {
		area, transformed = driver.Bounds{}, false
	} else if !transformed {
		transformed = s.hasTransformedDescendant(n)
	}
	self, children := driver.ReachView(f.Size, &n.props, area, transformed, local)
	if children {
		childPoint := local
		if n.kind == driver.KindScroll {
			childPoint.X += n.offset.X
			childPoint.Y += n.offset.Y
			if n.zoom > 0 && n.zoom != 1 {
				childPoint.X /= driver.Points(n.zoom)
				childPoint.Y /= driver.Points(n.zoom)
			}
		}
		for i := len(n.children) - 1; i >= 0; i-- {
			if h := s.hit(n.children[i], childPoint); h != nil {
				return h
			}
		}
	}
	if self {
		return n
	}
	return nil
}

// captureTouches snapshots native hit targets and local touch positions under
// the current static and bound presentation transforms before dispatching e.
func (s *touchScene) captureTouches(e *driver.Event) {
	s.tracker.Capture(e, func(p driver.Point) driver.NodeID {
		if root := s.nodes[s.root]; root != nil {
			if h := s.hit(root, p); h != nil {
				return h.id
			}
		}
		return 0
	}, func(id driver.NodeID) (driver.NodeID, driver.EventMask) {
		n := s.nodes[id]
		if n == nil || n.deleted {
			return 0, 0
		}
		var up driver.NodeID
		if n.parent != nil {
			up = n.parent.id
		}
		return up, n.events
	}, func(id driver.NodeID, p driver.Point) (driver.Point, bool) {
		n := s.nodes[id]
		if n == nil || n.deleted {
			return driver.Point{}, false
		}
		return s.surfacePoint(n, p)
	})
}
