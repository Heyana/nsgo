//go:build android

package android

import (
	"log"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// SubmitGPU implements engine.Platform: compiles any shaderir pipelines in frame to GLSL ES 3.00 and streams compact binary extGPUFrame payloads over the pipe to GpuView.java on the shared GL thread without triggering a UI view-tree commit.
func (s *surface) SubmitGPU(frame driver.GPUFrame) {
	// The frame's data is serialized onto the pipe here, so its sender may reuse that memory once this returns (gpu.FrameFence), sent or not.
	if frame.OnComplete != nil {
		defer frame.OnComplete()
	}
	for _, chunk := range splitGPUFrames(frame) {
		var payload writer
		if err := encodeGPUFrame(&payload, chunk); err != nil {
			log.Printf("nsgo: gpu: %v", err)
			s.reportGPUError(frame.SurfaceID, frame.ViewID, frame.Generation, driver.GPUErrorShaderCompile, err.Error())
			return
		}
		if err := s.sendExtended(extGPUFrame, func(w *writer) {
			w.buf = append(w.buf, payload.buf...)
		}); err != nil {
			log.Printf("nsgo: gpu: %v", err)
			s.reportGPUError(frame.SurfaceID, frame.ViewID, frame.Generation, driver.GPUErrorFailed, err.Error())
			return
		}
	}
}

func (s *surface) reportGPUError(surfaceID uint32, viewID driver.NodeID, gen uint32, code driver.GPUErrorCode, msg string) {
	ev := driver.Event{
		Kind: driver.EventGPU,
		ID:   viewID,
		Data: &driver.GPUEvent{
			Kind:       driver.GPUEventError,
			Generation: gen,
			ErrorCode:  code,
			Message:    msg,
		},
	}
	go s.dispatchTo(surfaceID, ev)
}

// gpuEvent delivers a msgGPUEvent from GpuView.java to its owning surface.
func (s *surface) gpuEvent(r *reader) {
	surfID, ev := readGPUEvent(r)
	if target := s.target(surfID); target != nil {
		target.Dispatch(ev)
	}
}
