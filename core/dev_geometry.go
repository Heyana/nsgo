package core

import (
	"maps"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type devGeometryState struct {
	requested  bool
	pending    bool
	cleanup    bool
	generation uint64
	birth      map[NodeID]uint64
	snapshot   driver.GeometrySnapshot
}

// SetDevGeometrySource connects internal/devtools to the platform's displayed geometry. The engine installs it; observation starts only when bounds are read and ends when the tree goes. Callbacks may arrive on the native thread.
func (t *Tree) SetDevGeometrySource(watch func(func(driver.GeometrySnapshot))) {
	state := t.devNative
	if state != nil && state.requested && t.devSource != nil {
		t.devSource(nil)
	}
	t.devSource = watch
	if watch != nil {
		t.devDrawn = nil
	}
	if state == nil {
		return
	}
	state.generation++
	state.snapshot = driver.GeometrySnapshot{}
	if state.requested {
		t.requestDevGeometry()
		t.observeDevGeometry()
	}
}

// requestDevGeometry sends a versioned barrier even when an otherwise idle app has no mount items. It separates already queued unobserved mounts from the first snapshot whose node IDs the current tree can trust.
func (t *Tree) requestDevGeometry() {
	if t.devSource != nil {
		t.devNative.pending, t.changed = true, true
	}
}

func (t *Tree) devGeometryState() *devGeometryState {
	if t.devNative == nil {
		t.devNative = &devGeometryState{}
	}
	return t.devNative
}

func (t *Tree) observeDevGeometry() {
	state := t.devGeometryState()
	if t.devSource == nil || t.Closed() {
		return
	}
	if !state.cleanup {
		state.cleanup = true
		t.root.addCleanup(func() {
			if t.devSource != nil {
				t.devSource(nil)
				t.devSource = nil
			}
			state.snapshot, state.birth = driver.GeometrySnapshot{}, nil
		})
	}
	run := state.generation
	t.devSource(func(snapshot driver.GeometrySnapshot) {
		t.root.Post(func() {
			if !t.Closed() && state.generation == run {
				t.acceptDevGeometry(snapshot)
			}
		})
	})
}

// acceptDevGeometry never takes an older tree's IDs for a view recreated since that snapshot. Debug-only commits do not invalidate app geometry.
func (t *Tree) acceptDevGeometry(snapshot driver.GeometrySnapshot) {
	state := t.devGeometryState()
	if snapshot.Version < state.snapshot.Version {
		return
	}
	app := make(map[NodeID]Bounds, len(snapshot.Bounds))
	matrices := make(map[NodeID]driver.Matrix3D, len(snapshot.Bounds))
	sizes := make(map[NodeID]Size, len(snapshot.Bounds))
	for id, bounds := range snapshot.Bounds {
		n := t.node(id)
		if n != nil && !n.deleted && snapshot.Version >= state.birth[id] && (t.highlights == nil || !t.highlights.owns(n)) {
			app[id] = bounds
			if m, ok := snapshot.Matrices[id]; ok {
				matrices[id], sizes[id] = m, snapshot.Sizes[id]
			} else {
				matrices[id], sizes[id] = driver.Translate3D(float64(bounds.Origin.X), float64(bounds.Origin.Y), 0), bounds.Size
			}
		}
	}
	same := maps.Equal(app, state.snapshot.Bounds) && maps.Equal(matrices, state.snapshot.Matrices) && maps.Equal(sizes, state.snapshot.Sizes)
	state.snapshot = driver.GeometrySnapshot{Version: snapshot.Version, Bounds: app, Matrices: matrices, Sizes: sizes}
	if same || t.highlights == nil {
		return
	}
	h := t.highlights
	outlined := len(h.boxes.GetUntracked()) > 0
	for _, sh := range h.sheets {
		outlined = outlined || len(sh.boxes.GetUntracked()) > 0
	}
	if h.selected == nil && !outlined && !h.audit.shown.GetUntracked() {
		return
	}
	selected := h.selected
	if selected != nil && (selected.deleted || t.node(selected.id) != selected) {
		selected = nil
	}
	h.setInspect(t, selected)
	// Each layer's outlines follow their views, a presented sheet's in the sheet's own space.
	follow := func(sig *Signal[[]highlightBox], space driver.Matrix3D) {
		if len(sig.GetUntracked()) == 0 {
			return
		}
		sig.Update(func(boxes []highlightBox) []highlightBox {
			out := append([]highlightBox(nil), boxes...)
			for i := range out {
				if n := out[i].target; n != nil && !n.deleted && t.node(n.id) == n {
					out[i].bounds = transformBounds(space, app[n.id])
				}
			}
			return out
		})
	}
	follow(h.boxes, driver.Identity3D)
	for _, sh := range h.sheets {
		follow(sh.boxes, t.devSpace(state.snapshot, sh.modal))
	}
	t.framesMoved, t.changed = true, true
}

func (t *Tree) noteGeometryVersion(items []driver.MountItem) {
	state := t.devNative
	if state == nil || !state.requested {
		return
	}
	for _, item := range items {
		if item.Kind == driver.ItemCreate {
			if state.birth == nil {
				state.birth = map[NodeID]uint64{}
			}
			state.birth[item.ID] = uint64(t.commits)
		}
	}
}

// devGeometry is the committed visual geometry before a native presentation snapshot is available. Ref.Bounds keeps its layout-frame contract.

func (t *Tree) devGeometry() map[NodeID]Bounds { return t.devGeometryFrame().Bounds }

func (t *Tree) devGeometryFrame() driver.GeometrySnapshot {
	state := t.devGeometryState()
	if !state.requested {
		state.requested = true
		state.birth = make(map[NodeID]uint64, len(t.nodes))
		for _, n := range t.nodes {
			if n != nil && !n.deleted {
				state.birth[n.id] = 1
			}
		}
		t.requestDevGeometry()
		t.observeDevGeometry()
	}
	if t.devSource == nil {
		t.ensureDevDrawn()
		t.stepDevDrawn()
		frame := driver.GeometrySnapshot{Bounds: map[NodeID]Bounds{}, Matrices: map[NodeID]driver.Matrix3D{}, Sizes: map[NodeID]Size{}}
		for _, n := range t.nodes {
			if n != nil && !n.deleted {
				m, size := t.drawnMatrix(n)
				frame.Bounds[n.id], frame.Matrices[n.id], frame.Sizes[n.id] = transformBounds(m, Bounds{Size: size}), m, size
			}
		}
		return frame
	}
	if state.snapshot.Bounds == nil {
		return t.devCommittedGeometryFrame()
	}
	frame := state.snapshot
	var fallback driver.GeometrySnapshot
	copied := false
	for id, birth := range state.birth {
		n := t.node(id)
		if frame.Version >= birth || n == nil || n.deleted || t.highlights != nil && t.highlights.owns(n) {
			continue
		}
		if !copied {
			fallback = t.devCommittedGeometryFrame()
			frame.Bounds = maps.Clone(frame.Bounds)
			frame.Matrices = maps.Clone(frame.Matrices)
			frame.Sizes = maps.Clone(frame.Sizes)
			copied = true
		}
		frame.Bounds[id], frame.Matrices[id], frame.Sizes[id] = fallback.Bounds[id], fallback.Matrices[id], fallback.Sizes[id]
	}
	return frame
}

func (t *Tree) devCommittedGeometryFrame() driver.GeometrySnapshot {
	shifts := map[NodeID]Point{}
	for _, n := range t.nodes {
		if n == nil || n.deleted || n.scroll == nil {
			continue
		}
		at := n.scroll.offset.Y
		if n.scroll.config.Horizontal {
			at = n.scroll.offset.X
		}
		for _, cell := range n.scroll.sticky {
			shift := shifts[cell.Cell]
			if n.scroll.config.Horizontal {
				shift.X = cell.StickyShift(at)
			} else {
				shift.Y = cell.StickyShift(at)
			}
			shifts[cell.Cell] = shift
		}
	}
	views := make(map[NodeID]driver.ViewGeometry, len(t.nodes))
	for _, n := range t.nodes {
		if n == nil || n.deleted {
			continue
		}
		frame := n.frame
		shift := shifts[n.id]
		frame.Origin.X, frame.Origin.Y = frame.Origin.X+shift.X, frame.Origin.Y+shift.Y
		v := driver.ViewGeometry{Frame: frame, Matrix: n.pointerMatrixAt(frame)}
		if parent := n.nativeParent(); parent != nil {
			v.Parent = parent.id
		}
		if n.scroll != nil {
			v.Offset = n.scroll.offset
		}
		views[n.id] = v
	}
	return driver.ProjectGeometry(views)
}

// hitDevWith uses native viewport matrices directly in surface coordinates; the fallback retains the Go tracker for backends without feedback.
func (t *Tree) hitDevWith(n *node, p Point, ok func(*node) bool) (NodeID, Point) {
	if t.devSource == nil {
		t.stepDevDrawn()
		return t.hitDrawnWith(n, p, ok)
	}
	frame := t.devGeometryFrame()
	var hit func(*node) (NodeID, Point)
	hit = func(n *node) (NodeID, Point) {
		m, mapped := frame.Matrices[n.id]
		if !mapped {
			return 0, Point{}
		}
		local, mapped := m.Unproject(p)
		if !mapped {
			return 0, Point{}
		}
		size := frame.Sizes[n.id]
		self, inside := driver.ReachView(size, &n.props, n.sent.overflow, true, local)
		x, y := n.clips()
		if x && (local.X < 0 || local.X >= size.Width) || y && (local.Y < 0 || local.Y >= size.Height) {
			inside = false
		}
		if n.style.Display == DisplayNone || n.style.Visibility == VisibilityHidden || n.hidden || !self && !inside || n.attach != nil && n.attach.cut {
			return 0, Point{}
		}
		if inside {
			kids := n.nativeChildren()
			for i := len(kids) - 1; i >= 0; i-- {
				if id, at := hit(kids[i]); id != 0 {
					return id, at
				}
			}
		}
		if self && ok(n) {
			return n.id, local
		}
		return 0, Point{}
	}
	return hit(n)
}
