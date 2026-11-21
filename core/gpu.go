package core

import (
	"sync/atomic"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// This file is the core's part of the gpu package (docs/architecture/gpu.md):
// it mounts a KindGPU host node, allocates monotonic resource IDs, routes
// EventGPU reports back onto the engine goroutine, and submits off-commit
// GPUFrame batches directly to the platform without triggering UI view-tree commits.

var nextGPUID atomic.Uint64

// GPUSurface connects a mounted gpu.View to its owning Tree and platform
// off-commit GPU submission hook. This is core's part of gpu.Device.
type GPUSurface struct {
	tree     *Tree
	node     *node
	disposed bool
}

// SurfaceID returns the owning surface's ID.
func (s *GPUSurface) SurfaceID() uint32 {
	if s == nil || s.tree == nil {
		return 0
	}
	return s.tree.id
}

// ViewID returns the mounted KindGPU view's NodeID.
func (s *GPUSurface) ViewID() NodeID {
	if s == nil || s.node == nil {
		return 0
	}
	return s.node.id
}

// NextID allocates a process-wide monotonic GPU resource identifier.
func (s *GPUSurface) NextID() uint64 {
	return nextGPUID.Add(1)
}

// Disposed reports whether the owning GPU view has unmounted.
func (s *GPUSurface) Disposed() bool {
	return s == nil || s.disposed
}

// Submit sends an off-commit driver.GPUFrame directly to the platform backend
// without marking the shadow tree dirty or producing a MountBatch.
func (s *GPUSurface) Submit(frame any) {
	if s == nil || s.disposed || s.tree == nil || s.tree.gpuSubmit == nil {
		return
	}
	f, ok := frame.(driver.GPUFrame)
	if !ok {
		return
	}
	f.SurfaceID = s.tree.id
	f.ViewID = s.node.id
	s.tree.gpuSubmit(f)
}

// GPUView is core's host element for package gpu's View (docs/architecture/gpu.md).
// Apps construct views via package gpu rather than using GPUView directly.
type GPUView struct {
	ColorFormat uint8
	DepthFormat uint8
	SampleCount int

	// OnMount runs on the engine goroutine once the KindGPU node is allocated.
	OnMount func(surf *GPUSurface)
	// OnEvent receives platform GPU events (*driver.GPUEvent) on the engine goroutine.
	OnEvent func(ev any)

	Class     string
	Style     Style
	StyleFunc func() Style
	Children  []Node
	OnPress   func(PressEvent)
	OnLayout  func(Bounds)
	Ref       *Ref
	Label     string
	LabelFunc func() string
	TestID    string
}

// Render implements Node.
func (v GPUView) Render(*Scope) Node { return v }

func (v GPUView) mountIn(t *Tree, cx *Scope, parent *node) { t.mountGPU(cx, parent, v) }

type gpuNode struct {
	cfg     driver.GPUViewConfig
	fresh   bool
	surf    *GPUSurface
	onEvent func(any)
}

func (t *Tree) mountGPU(cx *Scope, parent *node, v GPUView) {
	n := t.newNode(driver.KindGPU, parent, cx)
	t.setClasses(n, v.Class)
	t.bindStyle(cx, n, v.Style, v.StyleFunc)
	for _, c := range v.Children {
		t.mount(cx, n, c)
	}
	t.setRef(n, v.Ref)
	n.h = handlers{press: v.OnPress, layout: v.OnLayout}
	n.props = driver.ViewProps{Label: v.Label, TestID: v.TestID}
	t.bindIllustrationLabel(cx, n, v.LabelFunc)

	samples := v.SampleCount
	if samples <= 0 {
		samples = 1
	}
	surf := &GPUSurface{tree: t, node: n}
	gn := &gpuNode{
		cfg: driver.GPUViewConfig{
			ColorFormat: driver.GPUTextureFormat(v.ColorFormat),
			DepthFormat: driver.GPUTextureFormat(v.DepthFormat),
			SampleCount: samples,
		},
		fresh:   true,
		surf:    surf,
		onEvent: v.OnEvent,
	}
	n.gpu = gn
	cx.Cleanup(func() {
		surf.disposed = true
		gn.onEvent = nil
	})
	if v.OnMount != nil {
		v.OnMount(surf)
	}
}

// SetGPUSubmitter connects the off-commit GPU frame submission hook for this
// surface. Called by internal/engine and nsgotest; never by apps.
func (t *Tree) SetGPUSubmitter(fn func(driver.GPUFrame)) {
	t.gpuSubmit = fn
}
