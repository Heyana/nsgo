package gpu

import (
	"image"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// SoftwareRenderer hosts a standalone, deterministic pure-Go software 3D GPU
// device for headless unit tests and offscreen rendering without a UI window.
type SoftwareRenderer struct {
	mu   sync.Mutex
	soft *driver.GPUSoftwareDevice
	dev  *Device
	err  error
}

// NewSoftwareRenderer creates a pure-Go software 3D GPU renderer with a swapchain of width x height pixels, an optional depthFormat (e.g. FormatDepth24Plus), and DefaultFramesInFlight (2) ring-buffer slots. Compute shaders, storage buffers, and storage textures are enabled by default.
func NewSoftwareRenderer(width, height int, depthFormat TextureFormat) *SoftwareRenderer {
	return newSoftwareRenderer(width, height, FormatRGBA8Unorm, depthFormat, 1, featuresFromDriver(driver.DefaultSoftwareGPUFeatures()), DefaultFramesInFlight)
}

// NewSoftwareRendererWithFeatures creates a pure-Go software 3D GPU renderer with explicit device capability flags (useful for testing unsupported feature paths).
func NewSoftwareRendererWithFeatures(width, height int, depthFormat TextureFormat, features Features) *SoftwareRenderer {
	return newSoftwareRenderer(width, height, FormatRGBA8Unorm, depthFormat, 1, features, DefaultFramesInFlight)
}

// NewSoftwareRendererWithFrames creates a pure-Go software 3D GPU renderer with framesInFlight ring-buffer slots, clamped to [MinFramesInFlight, MaxFramesInFlight].
func NewSoftwareRendererWithFrames(width, height int, depthFormat TextureFormat, framesInFlight int) *SoftwareRenderer {
	return newSoftwareRenderer(width, height, FormatRGBA8Unorm, depthFormat, 1, featuresFromDriver(driver.DefaultSoftwareGPUFeatures()), framesInFlight)
}

// NewSoftwareRendererWithConfig creates a pure-Go software 3D GPU renderer with an explicit swapchain color format (FormatUndefined means FormatRGBA8Unorm), depth/stencil format, and MSAA sample count (1 or 4).
func NewSoftwareRendererWithConfig(width, height int, colorFormat, depthFormat TextureFormat, sampleCount int) *SoftwareRenderer {
	return newSoftwareRenderer(width, height, colorFormat, depthFormat, sampleCount, featuresFromDriver(driver.DefaultSoftwareGPUFeatures()), DefaultFramesInFlight)
}

func newSoftwareRenderer(width, height int, colorFormat, depthFormat TextureFormat, sampleCount int, features Features, framesInFlight int) *SoftwareRenderer {
	if colorFormat == FormatUndefined {
		colorFormat = FormatRGBA8Unorm
	}
	if sampleCount <= 0 {
		sampleCount = 1
	}
	fif := driver.NormalizeFramesInFlight(framesInFlight)
	cfg := driver.GPUViewConfig{
		ColorFormat:    driver.GPUTextureFormat(colorFormat),
		DepthFormat:    driver.GPUTextureFormat(depthFormat),
		SampleCount:    sampleCount,
		FramesInFlight: fif,
	}
	soft := driver.NewGPUSoftwareDevice(width, height, cfg)
	r := &SoftwareRenderer{soft: soft}
	dev := newDevice(
		nil,
		nil,
		1,
		width,
		height,
		colorFormat,
		ColorSpaceSRGB,
		fif,
		limitsFromDriver(driver.DefaultSoftwareGPULimits()),
		features,
		compressedFormatsFromFeatures(features),
	)
	dev.directMapped = true
	soft.SetReadbackHandler(func(rbs []driver.GPUSoftReadback, err error) {
		if err != nil {
			r.mu.Lock()
			r.err = err
			r.mu.Unlock()
		}
		for _, rb := range rbs {
			dev.completeReadback(rb.ID, rb.Pixels, rb.Err)
		}
	})
	dev.submitFn = func(frame driver.GPUFrame) {
		r.mu.Lock()
		rbs, err := soft.Submit(frame)
		if err != nil {
			r.err = err
		}
		r.mu.Unlock()
		for _, rb := range rbs {
			dev.completeReadback(rb.ID, rb.Pixels, rb.Err)
		}
		if timings := soft.LastPassTimings(); len(timings) > 0 {
			dev.completePassTimings(timings)
		}
	}
	r.dev = dev
	return r
}

// Device returns the gpu.Device backed by this software rasterizer.
func (r *SoftwareRenderer) Device() *Device { return r.dev }

// SetPipelined enables or disables background GPU-worker execution on the
// software renderer so CPU frame N+1 ring-buffer writes overlap with GPU
// frame N rasterization, synchronized by FrameFence.
func (r *SoftwareRenderer) SetPipelined(enabled bool) {
	r.soft.SetPipelined(enabled)
}

// SetFrameHooks installs optional callbacks invoked on the GPU execution thread
// immediately before render passes execute and immediately after each frame
// finishes (before signaling the frame's FrameFence).
func (r *SoftwareRenderer) SetFrameHooks(beforePass, afterFrame func(frameIndex uint64, frameSlot int)) {
	var bHook, aHook func(driver.GPUFrame)
	if beforePass != nil {
		bHook = func(f driver.GPUFrame) { beforePass(f.FrameIndex, f.FrameSlot) }
	}
	if afterFrame != nil {
		aHook = func(f driver.GPUFrame) { afterFrame(f.FrameIndex, f.FrameSlot) }
	}
	r.soft.SetFrameHooks(bHook, aHook)
}

// WaitIdle blocks until all queued pipelined frames have completed.
func (r *SoftwareRenderer) WaitIdle() {
	r.soft.WaitIdle()
}

// Close flushes pending work and stops any pipelined worker goroutine.
func (r *SoftwareRenderer) Close() {
	r.soft.Close()
}

// Resize updates the software swapchain size in pixels.
func (r *SoftwareRenderer) Resize(width, height int) {
	r.mu.Lock()
	r.soft.Resize(width, height)
	w, h := r.soft.Size()
	r.mu.Unlock()
	r.dev.width, r.dev.height = w, h
}

// Image returns an independent RGBA snapshot of the rendered swapchain surface.
func (r *SoftwareRenderer) Image() *image.RGBA {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.soft.SurfaceImage()
}

// FrameCount returns how many GPU frames have been submitted.
func (r *SoftwareRenderer) FrameCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.soft.Frames())
}

// PassTimings returns the per-pass timings measured during the most recent frame.
func (r *SoftwareRenderer) PassTimings() []PassTiming {
	return r.dev.LastPassTimings()
}

// DebugEvents returns all debug labels, groups, and markers observed by the software renderer (empty in release builds where labels are stripped).
func (r *SoftwareRenderer) DebugEvents() []DebugEvent {
	raw := r.soft.DebugEvents()
	if len(raw) == 0 {
		return nil
	}
	out := make([]DebugEvent, len(raw))
	for i, ev := range raw {
		out[i] = DebugEvent{Kind: ev.Kind, Label: ev.Label, Depth: ev.Depth}
	}
	return out
}

// ResourceLabel returns the debug label received by the software renderer for resource id (always empty in release builds).
func (r *SoftwareRenderer) ResourceLabel(id uint64) string {
	return r.soft.ResourceLabel(driver.GPUID(id))
}

// LastError returns the last execution error reported by the software device.
func (r *SoftwareRenderer) LastError() error {
	r.mu.Lock()
	err := r.err
	r.mu.Unlock()
	if err != nil {
		return err
	}
	return r.soft.LastError()
}
