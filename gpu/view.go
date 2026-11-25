package gpu

import (
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ViewProps configures a GPU-backed 3D surface mounted in the NSGo view tree.
// Children render over the GPU surface as ordinary native NSGo views (HUD, menus).
type ViewProps struct {
	// ColorFormat defaults to the platform's preferred swapchain format.
	// When FormatUndefined is paired with an HDR ColorSpace, backends pick an
	// appropriate HDR format (such as FormatRGBA16Float for extended linear
	// sRGB or FormatRGB10A2Unorm for HDR10 PQ/HLG).
	ColorFormat TextureFormat
	// ColorSpace requests the swapchain's color space and transfer function.
	// ColorSpaceDefault selects standard SDR sRGB. Backends that cannot honor
	// an HDR color space fall back to SDR and report the negotiated color space
	// via Device.SurfaceColorSpace.
	ColorSpace ColorSpace
	// DepthFormat enables an automatic swapchain depth/stencil buffer resized
	// with the view (e.g. FormatDepth24Plus). FormatUndefined disables it.
	DepthFormat TextureFormat
	// SampleCount is 1 (default) or 4 for swapchain MSAA.
	SampleCount int
	// FramesInFlight configures the number of rotating per-frame ring-buffer
	// slots (2 or 3). Zero defaults to DefaultFramesInFlight (2); values outside
	// [MinFramesInFlight, MaxFramesInFlight] are clamped cleanly to [2, 3].
	FramesInFlight int

	// OnReady runs on the engine goroutine when the GPU device and surface are
	// ready, and again after recovery from device loss.
	OnReady func(dev *Device)
	// OnFrame, when set, runs on every display frame via RequestAnimationFrame
	// once the device is ready and not suspended, submitting commands off-commit with zero UI view-tree commits per frame.
	OnFrame func(frame *Frame)
	// OnResize runs when the drawable size in physical pixels changes.
	OnResize func(width, height int)
	// OnDeviceLost runs if the underlying GPU context or device is lost; all
	// non-retained handles from the previous Device are invalidated.
	OnDeviceLost func(DeviceLostEvent)
	// OnDeviceRestored runs on the engine goroutine after a lost device or surface is recreated, receiving the fresh ready *Device after any retained resources have been automatically restored.
	OnDeviceRestored func(dev *Device)
	// OnSuspend runs when the app or surface enters the background and GPU frame submission is automatically paused.
	OnSuspend func()
	// OnResume runs when the app or surface returns to the active state and GPU frame submission resumes.
	OnResume func()
	// OnError reports asynchronous GPU validation, unsupported, or OOM errors.
	OnError func(Error)
	// OnPassTimings runs on the engine goroutine when per-pass GPU timing measurements complete for a submitted frame.
	OnPassTimings func([]PassTiming)

	// RetainResources, when true, opts every resource created on this view's Device into automatic recreation after device or surface loss.
	RetainResources bool
	// PipelineCache, when set, attaches a persistent pipeline cache to every Device created by this view (including restored devices).
	PipelineCache *PipelineCache

	Class     string
	Style     core.Style
	StyleFunc func() core.Style
	Children  []core.Node
	OnPress   func(core.PressEvent)
	OnLayout  func(core.Bounds)
	Ref       *core.Ref
	Label     string
	LabelFunc func() string
	TestID    string
}

// Render implements core.Node so ViewProps may be returned directly as a node
// or wrapped via gpu.View(cx, props).
func (p ViewProps) Render(cx *core.Scope) core.Node {
	return View(cx, p)
}

// View returns a core.Node hosting a native GPU surface configured by props.
// On targets where cx.Env().SupportsGPU() is false, it renders props.Children
// inside a plain container without touching native GPU APIs and reports
// ErrUnsupported via OnError.
func View(cx *core.Scope, props ViewProps) core.Node {
	if !cx.Env().SupportsGPU() {
		if props.OnError != nil {
			err := Error{Code: ErrUnsupported, Message: "GPU view is not supported on this target"}
			cx.Post(func() {
				if !cx.Disposed() {
					props.OnError(err)
				}
			})
		}
		return core.View{
			Class:     props.Class,
			Style:     props.Style,
			StyleFunc: props.StyleFunc,
			Children:  props.Children,
			OnPress:   props.OnPress,
			OnLayout:  props.OnLayout,
			Ref:       props.Ref,
			Label:     props.Label,
			TestID:    props.TestID,
		}
	}

	var (
		surf      *core.GPUSurface
		activeDev *Device
		lastDev   *Device
		stopFrame func()
		lastAt    time.Time
		suspended bool
		everReady bool
		wasLost   bool
	)

	stopLoop := func() {
		if stopFrame != nil {
			stopFrame()
			stopFrame = nil
		}
	}

	startLoop := func(dev *Device) {
		stopLoop()
		if props.OnFrame == nil || dev == nil || suspended || !dev.alive() {
			return
		}
		lastAt = time.Time{}
		var tick func(at time.Time)
		tick = func(at time.Time) {
			if activeDev != dev || !dev.alive() || suspended || dev.IsSuspended() || cx.Disposed() {
				return
			}
			f := dev.BeginFrame(at)
			if !lastAt.IsZero() && at.After(lastAt) {
				f.delta = at.Sub(lastAt)
			}
			lastAt = at
			props.OnFrame(f)
			dev.EndFrame(f)
			if activeDev == dev && dev.alive() && !suspended && !dev.IsSuspended() && !cx.Disposed() {
				stopFrame = cx.RequestAnimationFrame(tick)
			}
		}
		stopFrame = cx.RequestAnimationFrame(tick)
	}

	// Gate GPU frame submission on app/window lifecycle so backgrounded apps (especially iOS Metal and Android Activity pause) never submit GPU work.
	cx.Effect(func() {
		lc := cx.Env().Lifecycle()
		if lc == core.LifecycleBackground {
			if !suspended {
				suspended = true
				stopLoop()
				if activeDev != nil {
					activeDev.setSuspended(true)
				}
				if props.OnSuspend != nil {
					props.OnSuspend()
				}
			}
			return
		}
		if lc == core.LifecycleActive && suspended {
			suspended = false
			if activeDev != nil {
				activeDev.setSuspended(false)
				activeDev.flushOpsOnly()
			}
			if props.OnResume != nil {
				props.OnResume()
			}
			if activeDev != nil && activeDev.alive() {
				startLoop(activeDev)
			}
		}
	})

	cx.Cleanup(func() {
		stopLoop()
		if activeDev != nil {
			activeDev.invalidate(LostDestroyed, "view unmounted")
			activeDev = nil
		}
		lastDev = nil
	})

	return core.GPUView{
		ColorFormat:    uint8(props.ColorFormat),
		ColorSpace:     uint8(props.ColorSpace),
		DepthFormat:    uint8(props.DepthFormat),
		SampleCount:    props.SampleCount,
		FramesInFlight: driver.NormalizeFramesInFlight(props.FramesInFlight),
		Class:          props.Class,
		Style:          props.Style,
		StyleFunc:      props.StyleFunc,
		Children:       props.Children,
		OnPress:        props.OnPress,
		OnLayout:       props.OnLayout,
		Ref:            props.Ref,
		Label:          props.Label,
		LabelFunc:      props.LabelFunc,
		TestID:         props.TestID,
		OnMount: func(s *core.GPUSurface) {
			surf = s
		},
		OnEvent: func(raw any) {
			ev, ok := raw.(*driver.GPUEvent)
			if !ok || ev == nil {
				return
			}
			switch ev.Kind {
			case driver.GPUEventDeviceReady:
				restoring := wasLost || everReady
				prevDev := lastDev
				if activeDev != nil {
					activeDev.invalidate(LostReset, "device recreated")
					prevDev = activeDev
				}
				fif := ev.FramesInFlight
				if fif == 0 {
					fif = props.FramesInFlight
				}
				dev := newDevice(
					surf,
					cx,
					ev.Generation,
					ev.Width,
					ev.Height,
					TextureFormat(ev.SurfaceFormat),
					ColorSpace(ev.SurfaceColorSpace),
					fif,
					limitsFromDriver(ev.Limits),
					featuresFromDriver(ev.Features),
					compressedFormatsFromDriver(ev.Features),
				)
				dev.retainResources = props.RetainResources
				dev.surfaceSamples = max(props.SampleCount, 1)
				dev.surfaceDepth = props.DepthFormat
				dev.cache = props.PipelineCache
				if suspended {
					dev.setSuspended(true)
				}
				activeDev = dev
				lastDev = dev
				everReady = true
				wasLost = false
				if restoring && prevDev != nil {
					dev.restoreRetainedFrom(prevDev)
				}
				if props.OnReady != nil {
					props.OnReady(dev)
				}
				if restoring && props.OnDeviceRestored != nil {
					props.OnDeviceRestored(dev)
				}
				// Flush setup/restored resource ops and start rendering only when active.
				if !suspended {
					dev.flushOpsOnly()
					startLoop(dev)
				}

			case driver.GPUEventResize:
				if activeDev != nil && activeDev.generation == ev.Generation {
					activeDev.width = ev.Width
					activeDev.height = ev.Height
				}
				if props.OnResize != nil {
					props.OnResize(ev.Width, ev.Height)
				}

			case driver.GPUEventDeviceLost:
				stopLoop()
				wasLost = true
				if activeDev != nil && (ev.Generation == 0 || activeDev.generation == ev.Generation) {
					lastDev = activeDev
					activeDev.invalidate(DeviceLostReason(ev.LostReason), ev.Message)
					activeDev = nil
				}
				if props.OnDeviceLost != nil {
					props.OnDeviceLost(DeviceLostEvent{
						Reason:  DeviceLostReason(ev.LostReason),
						Message: ev.Message,
					})
				}

			case driver.GPUEventError:
				if props.OnError != nil {
					props.OnError(Error{
						Code:    ErrorCode(ev.ErrorCode),
						Message: ev.Message,
					})
				}

			case driver.GPUEventReadback:
				if activeDev != nil {
					activeDev.completeReadback(ev.ReadbackID, ev.Pixels, ev.Message)
				}

			case driver.GPUEventPassTimings:
				if activeDev != nil && (ev.Generation == 0 || activeDev.generation == ev.Generation) {
					timings := activeDev.completePassTimings(ev.PassTimings)
					if props.OnPassTimings != nil && len(timings) > 0 {
						props.OnPassTimings(timings)
					}
				}

			case driver.GPUEventFrameComplete:
				if activeDev != nil {
					activeDev.completeFence(ev.FenceID)
				}
			}
		},
	}
}
