//go:build darwin && !ios

package macos

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// A gpu.View draws with Metal into a layer of its own below its sublayers, as a shader view does (shader.go), from the frames its device submits off the view tree's commits (apple/gpu.go).

// newGPU gives a GPU view its Metal layer, or reports that the machine has no Metal device.
func (v *view) newGPU(cfg driver.GPUViewConfig) {
	if v.gpu != nil {
		return
	}
	id, engine := v.id, v.surface.engine
	emit := func(ev *driver.GPUEvent) { engine.Dispatch(driver.Event{Kind: driver.EventGPU, ID: id, Data: ev}) }
	layer := apple.NewGPULayer(cfg, emit)
	if layer == nil {
		emit(&driver.GPUEvent{Kind: driver.GPUEventError, ErrorCode: driver.GPUErrorUnsupported, Message: "this Mac has no Metal device"})
		return
	}
	layer.Layer.Send(selActions, contentActions)
	layer.Layer.Send(selSetZPosition, apple.Z(contentZ))
	v.native.Send(selLayer).Send(sel("addSublayer:"), layer.Layer)
	v.gpu = layer
	v.surface.gpuLayers.Store(id, layer)
	if v.framed {
		v.sizeGPU()
	}
}

// sizeGPU keeps v's Metal layer over its bounds at the window's scale.
func (v *view) sizeGPU() {
	w, h := float64(v.frame.Size.Width), float64(v.frame.Size.Height)
	apple.WithoutAnimation(func() {
		v.gpu.Layer.Send(selSetFrame, apple.Rect(0, 0, w, h))
	})
	v.gpu.Resize(w, h, float64(v.surface.env.PixelRatio))
}

// releaseGPU lets v's Metal layer go with its view.
func (s *surface) releaseGPU(v *view) {
	s.gpuLayers.Delete(v.id)
	v.gpu.Release()
	v.gpu = nil
}

// SubmitGPU hands a GPU view's frame to its layer's render goroutine, from the engine goroutine.
func (s *surface) SubmitGPU(frame driver.GPUFrame) {
	if l, ok := s.gpuLayers.Load(frame.ViewID); ok {
		l.(*apple.GPULayer).Submit(frame)
	}
}
