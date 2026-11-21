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
	ColorFormat TextureFormat
	// DepthFormat enables an automatic swapchain depth/stencil buffer resized
	// with the view (e.g. FormatDepth24Plus). FormatUndefined disables it.
	DepthFormat TextureFormat
	// SampleCount is 1 (default) or 4 for swapchain MSAA.
	SampleCount int

	// OnReady runs on the engine goroutine when the GPU device and surface are
	// ready, and again after recovery from device loss.
	OnReady func(dev *Device)
	// OnFrame, when set, runs on every display frame via RequestAnimationFrame
	// once the device is ready, submitting commands off-commit with zero UI
	// view-tree commits per frame.
	OnFrame func(frame *Frame)
	// OnResize runs when the drawable size in physical pixels changes.
	OnResize func(width, height int)
	// OnDeviceLost runs if the underlying GPU context or device is lost; all
	// handles from the previous Device are invalidated.
	OnDeviceLost func(DeviceLostEvent)
	// OnError reports asynchronous GPU validation, unsupported, or OOM errors.
	OnError func(Error)

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
		stopFrame func()
		lastAt    time.Time
	)

	stopLoop := func() {
		if stopFrame != nil {
			stopFrame()
			stopFrame = nil
		}
	}

	startLoop := func(dev *Device) {
		stopLoop()
		if props.OnFrame == nil || dev == nil {
			return
		}
		lastAt = time.Time{}
		var tick func(at time.Time)
		tick = func(at time.Time) {
			if activeDev != dev || !dev.alive() || cx.Disposed() {
				return
			}
			f := dev.BeginFrame(at)
			if !lastAt.IsZero() && at.After(lastAt) {
				f.delta = at.Sub(lastAt)
			}
			lastAt = at
			props.OnFrame(f)
			if !f.submitted && (len(f.enc.passes) > 0 || len(f.enc.copies) > 0 || len(dev.pendingOps) > 0) {
				f.Submit()
			}
			if activeDev == dev && dev.alive() && !cx.Disposed() {
				stopFrame = cx.RequestAnimationFrame(tick)
			}
		}
		stopFrame = cx.RequestAnimationFrame(tick)
	}

	cx.Cleanup(func() {
		stopLoop()
		if activeDev != nil {
			activeDev.invalidate(LostDestroyed, "view unmounted")
			activeDev = nil
		}
	})

	return core.GPUView{
		ColorFormat: uint8(props.ColorFormat),
		DepthFormat: uint8(props.DepthFormat),
		SampleCount: props.SampleCount,
		Class:       props.Class,
		Style:       props.Style,
		StyleFunc:   props.StyleFunc,
		Children:    props.Children,
		OnPress:     props.OnPress,
		OnLayout:    props.OnLayout,
		Ref:         props.Ref,
		Label:       props.Label,
		LabelFunc:   props.LabelFunc,
		TestID:      props.TestID,
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
				if activeDev != nil {
					activeDev.invalidate(LostReset, "device recreated")
				}
				dev := newDevice(
					surf,
					cx,
					ev.Generation,
					ev.Width,
					ev.Height,
					TextureFormat(ev.SurfaceFormat),
					limitsFromDriver(ev.Limits),
					featuresFromDriver(ev.Features),
				)
				activeDev = dev
				if props.OnReady != nil {
					props.OnReady(dev)
				}
				// Flush any setup resource creations/uploads even if OnReady didn't submit a draw yet.
				dev.flushOpsOnly()
				startLoop(dev)

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
				if activeDev != nil && (ev.Generation == 0 || activeDev.generation == ev.Generation) {
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
			}
		},
	}
}
