//go:build android

package android

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// Overlays over other apps (driver.SurfaceOverlay, mobile.OpenOverlay) show in windows of WindowManager's, each an NsgoView of its own that the activity's view, the first surface, hands its batches, images and events on to by the surface's ID. Go keeps an overlay from the ItemOpenSurface that opens it, attaches its tree as Java first reports the window's size, frame and environment (msgSurface), and ends it as Java reports it refused or removed. Text, fonts, shader programs and the host channel stay the process's, which the first view serves.

// overlay is what Go keeps of an overlay the app opened: the platform its tree attaches to.
type overlay struct {
	s *surface
	// engine is the tree's once Java's first report attached it, and ended set once Java reported its end or the tree closed; under s.omu.
	engine *engine.Engine
	ended  bool
	// view is the tree's, on the engine goroutine.
	view treeView
}

// openOverlays keeps the overlays the first surface's batch b opens, before Java hears of them.
func (s *surface) openOverlays(b *driver.MountBatch) {
	for i := range b.Items {
		it := &b.Items[i]
		if it.Kind != driver.ItemOpenSurface || it.Surface.Kind != driver.SurfaceOverlay {
			continue
		}
		id := it.Surface.ID
		s.omu.Lock()
		s.overlays[id] = &overlay{s: s, view: treeView{id: id, images: map[driver.NodeID]*imageView{}}}
		s.omu.Unlock()
	}
}

// Prepare implements engine.Platform with the process's text, which the first view keeps.
func (o *overlay) Prepare(t *driver.AttributedText) driver.PreparedText { return o.s.Prepare(t) }

// RequestFrame implements engine.Platform: the engine asks an opened surface for the app's frames, and the first view's Choreographer answers for every surface.
func (o *overlay) RequestFrame()                   { o.s.RequestFrame() }
func (o *overlay) SubmitGPU(frame driver.GPUFrame) { o.s.SubmitGPU(frame) }

// Mount implements engine.Platform: an overlay that ended takes nothing more, and the batch that destroys its window lets go of what Go kept of its tree, as a closed tree deletes none of its views.
func (o *overlay) Mount(b *driver.MountBatch) {
	s := o.s
	s.omu.Lock()
	ended := o.ended
	s.omu.Unlock()
	if !ended {
		s.mount(&o.view, b)
	}
	if len(b.Items) == 1 && b.Items[0].Kind == driver.ItemWindow && b.Items[0].Window.Action == driver.WindowDestroy {
		s.forget(o)
	}
}

// forget lets go of overlay o as its tree closes, on the engine goroutine: its image loads, shader programs and native components; Java drops its views with its window.
func (s *surface) forget(o *overlay) {
	s.omu.Lock()
	o.ended = true
	if s.overlays[o.view.id] == o {
		delete(s.overlays, o.view.id)
	}
	s.omu.Unlock()
	for id, v := range o.view.images {
		v.stop()
		delete(o.view.images, id)
	}
	for id := range o.view.shaders {
		s.drawWith(&o.view, id, nil)
	}
	s.releasePrograms()
	s.canvasRequests.closeSurface(o.view.id)
	s.closeSurfaceNatives(o.view.id)
}

// attach attaches o's tree as Java first reports its window and returns its Engine, on the reader goroutine, which started the first surface's.
func (o *overlay) attach() *engine.Engine {
	s := o.s
	s.omu.Lock()
	defer s.omu.Unlock()
	if o.engine == nil && s.engine != nil {
		o.engine = s.engine.Attach(o.view.id, o)
	}
	return o.engine
}

// surfaceReport takes a msgSurface, on the reader goroutine.
func (s *surface) surfaceReport(r *reader) {
	id, op := r.u32(), r.byte()
	s.omu.Lock()
	o := s.overlays[id]
	ended := o == nil || o.ended
	s.omu.Unlock()
	if ended {
		return
	}
	e := o.attach()
	if e == nil {
		return
	}
	switch op {
	case surfaceState:
		size := driver.Size{Width: r.points(), Height: r.points()}
		frame := driver.WindowFrame{X: r.points(), Y: r.points(), Move: true, Width: size.Width, Height: size.Height}
		env := readEnvironment(r)
		// SetWindowFrame places an overlay, kept on the screen (Overlays.java).
		env.WindowFrame, env.CanPlaceWindows = frame, true
		e.ResizeWithEnvironment(size, driver.Geometry{}, env)
	case surfaceEnded:
		reason := int(r.byte())
		s.omu.Lock()
		o.ended = true
		s.omu.Unlock()
		// The tree closes as a window does, and its platform hears the destroy it then sends.
		e.Dispatch(driver.Event{Kind: driver.EventWindowEnded, Index: reason})
	}
}
