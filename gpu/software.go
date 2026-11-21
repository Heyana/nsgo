package gpu

import (
	"image"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// SoftwareRenderer hosts a standalone, deterministic pure-Go software 3D GPU
// device for headless unit tests and offscreen rendering without a UI window.
type SoftwareRenderer struct {
	soft *driver.GPUSoftwareDevice
	dev  *Device
	err  error
}

// NewSoftwareRenderer creates a pure-Go software 3D GPU renderer with a swapchain
// of width x height pixels and an optional depthFormat (e.g. FormatDepth24Plus).
func NewSoftwareRenderer(width, height int, depthFormat TextureFormat) *SoftwareRenderer {
	cfg := driver.GPUViewConfig{
		ColorFormat: driver.GPUFormatRGBA8Unorm,
		DepthFormat: driver.GPUTextureFormat(depthFormat),
		SampleCount: 1,
	}
	soft := driver.NewGPUSoftwareDevice(width, height, cfg)
	r := &SoftwareRenderer{soft: soft}
	dev := newDevice(
		nil,
		nil,
		1,
		width,
		height,
		FormatRGBA8Unorm,
		limitsFromDriver(driver.DefaultGPULimits()),
		Features{Depth32Float: true},
	)
	dev.submitFn = func(frame driver.GPUFrame) {
		rbs, err := soft.Submit(frame)
		if err != nil {
			r.err = err
		}
		for _, rb := range rbs {
			dev.completeReadback(rb.ID, rb.Pixels, rb.Err)
		}
	}
	r.dev = dev
	return r
}

// Device returns the gpu.Device backed by this software rasterizer.
func (r *SoftwareRenderer) Device() *Device { return r.dev }

// Resize updates the software swapchain size in pixels.
func (r *SoftwareRenderer) Resize(width, height int) {
	r.soft.Resize(width, height)
	r.dev.width, r.dev.height = r.soft.Size()
}

// Image returns an independent RGBA snapshot of the rendered swapchain surface.
func (r *SoftwareRenderer) Image() *image.RGBA {
	return r.soft.SurfaceImage()
}

// FrameCount returns how many GPU frames have been submitted.
func (r *SoftwareRenderer) FrameCount() int {
	return len(r.soft.Frames())
}

// LastError returns the last execution error reported by the software device.
func (r *SoftwareRenderer) LastError() error {
	return r.err
}
