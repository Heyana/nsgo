//go:build android

package android

import (
	"log"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

// Overlays over other apps (driver.SurfaceOverlay, mobile.OpenOverlay) show in windows of WindowManager's, each an NsgoView of its own that the activity's view, the first surface, hands its batches, images and events on to by the surface's ID. Go keeps an overlay from the ItemOpenSurface that opens it, attaches its tree as Java first reports the window's size, frame and environment (msgSurface), and ends it as Java reports it refused or removed. Text, fonts, shader programs and the host channel stay the process's, which the first view serves.

// overlay is what Go keeps of an overlay the app opened: the platform its tree attaches to.
type overlay struct {
	s *surface
	// engine is the tree's once Java's first report attached it, and ended set once Java reported its end or the tree closed; under s.omu.
	engine *engine.Engine
	ended  bool
	// view is the tree's, on the engine goroutine.
	view        treeView
	frameWanted atomic.Bool
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

// RequestFrame asks the runtime to answer from the overlay's clock.
func (o *overlay) RequestFrame() {
	o.frameWanted.Store(true)
	o.s.send(msgWantFrame, func(w *writer) { w.u32(o.view.id) })
}

func (o *overlay) SubmitGPU(frame driver.GPUFrame) { o.s.SubmitGPU(frame) }

func (o *overlay) SetPreferredFrameRate(rate driver.FrameRateRange) {
	o.s.setFrameRate(o.view.id, rate)
}

func (s *surface) awaitingFrame(id uint32) bool {
	if id == 0 {
		return s.frameWanted.Load()
	}
	s.omu.Lock()
	defer s.omu.Unlock()
	o := s.overlays[id]
	return o != nil && o.frameWanted.Load()
}

func (s *surface) overlayFrame(id uint32, at time.Time) {
	s.omu.Lock()
	o := s.overlays[id]
	var target *engine.Engine
	if o != nil && !o.ended {
		o.frameWanted.Store(false)
		target = o.engine
	}
	s.omu.Unlock()
	if target != nil {
		target.FrameAt(at)
	}
}

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
	s.audioFiles.closeSurface(o.view.id)
	imageload.ReleaseOwner(o.view.id)
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
	if id == 0 && (op == surfaceDetach || op == surfaceResync) {
		generation := r.u64()
		// The reset that ends a detach is an extended message. A runtime that sends these reads them; should one not, keep mounting as before rather than hold batches for a reset it cannot read.
		if !s.extendedMessages.Load() {
			return
		}
		if op == surfaceDetach {
			s.main.detached.Store(true)
			return
		}
		if s.engine != nil {
			s.engine.Remount(func() {
				for id, image := range s.main.images {
					image.stop()
					delete(s.main.images, id)
				}
				s.main.canvasPixels, s.main.shaderPixels = nil, nil
				s.main.defaultsSent = false
				s.main.detached.Store(false)
				s.frameWanted.Store(false)
				if err := s.sendExtended(extSurfaceReset, func(w *writer) {
					w.u32(id)
					w.u64(generation)
				}); err != nil {
					log.Printf("android surface reset: %v", err)
				}
			})
		}
		return
	}
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
