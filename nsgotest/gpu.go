package nsgotest

import (
	"fmt"
	"image"
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// NoGPU disables GPU surface support (Env.SupportsGPU() == false) for headless
// tests verifying fallback UI and ErrUnsupported handling.
func NoGPU() Option {
	return func(a *App) { a.noGPU = true }
}

// NoGPUCompute disables compute shader, storage buffer and storage texture capabilities (Device.SupportsCompute() == false) to simulate WebGL 2 / OpenGL ES 3.0 targets in headless tests.
func NoGPUCompute() Option {
	return func(a *App) { a.noGPUCompute = true }
}

func (a *App) gpuFeatures() driver.GPUFeatures {
	f := driver.DefaultSoftwareGPUFeatures()
	if a.noGPUCompute {
		f.StorageBuffers = false
		f.StorageTextures = false
		f.Compute = false
	}
	return f
}

// GPUFrameRecord summarizes one off-commit GPUFrame submitted by a gpu.View.
type GPUFrameRecord struct {
	SurfaceID       uint32
	ViewID          driver.NodeID
	Generation      uint32
	FrameIndex      uint64
	FrameSlot       int
	OpsCount        int
	CopiesCount     int
	PassesCount     int
	DrawsCount      int
	InstancesCount  uint64
	DispatchesCount int
	Present         bool
}

type gpuViewState struct {
	id         driver.NodeID
	cfg        driver.GPUViewConfig
	generation uint32
	ready      bool
	lost       bool
	overrideW  int
	overrideH  int
	soft       *driver.GPUSoftwareDevice
	frames     []GPUFrameRecord
}

func (a *App) ensureGPUState(id driver.NodeID) *gpuViewState {
	if a.gpuViews == nil {
		a.gpuViews = map[driver.NodeID]*gpuViewState{}
	}
	st := a.gpuViews[id]
	if st == nil {
		st = &gpuViewState{id: id}
		a.gpuViews[id] = st
	}
	return st
}

func (a *App) gpuPixelSize(v *View) (int, int) {
	ratio := float32(1)
	if a.env != nil && a.env.PixelRatio > 0 {
		ratio = a.env.PixelRatio
	}
	w := int(math.Round(float64(float32(v.Frame.Size.Width) * ratio)))
	h := int(math.Round(float64(float32(v.Frame.Size.Height) * ratio)))
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	return w, h
}

func (a *App) configureGPUView(v *View, cfg driver.GPUViewConfig) {
	if a.target != nil {
		return
	}
	st := a.ensureGPUState(v.ID)
	st.cfg = cfg
}

func (a *App) syncGPUViews(configured []*View, resized []*View) {
	if a.target != nil {
		return
	}
	readyNow := map[driver.NodeID]bool{}
	for _, v := range configured {
		if v == nil || a.views[v.ID] != v {
			continue
		}
		st := a.ensureGPUState(v.ID)
		w, h := a.gpuPixelSize(v)
		if st.soft == nil {
			st.soft = driver.NewGPUSoftwareDevice(w, h, st.cfg)
		} else {
			st.soft.Resize(w, h)
		}
		if !st.ready && !st.lost {
			st.ready = true
			st.generation++
			gen := st.generation
			readyNow[v.ID] = true
			surfFmt := st.cfg.ColorFormat
			if surfFmt == driver.GPUFormatUndefined {
				surfFmt = driver.GPUFormatRGBA8Unorm
			}
			a.post(driver.Event{
				Kind: driver.EventGPU,
				ID:   v.ID,
				Data: &driver.GPUEvent{
					Kind:           driver.GPUEventDeviceReady,
					Generation:     gen,
					Width:          w,
					Height:         h,
					SurfaceFormat:  surfFmt,
					FramesInFlight: driver.NormalizeFramesInFlight(st.cfg.FramesInFlight),
					Limits:         driver.DefaultSoftwareGPULimits(),
					Features:       a.gpuFeatures(),
				},
			})
		}
	}
	for _, v := range resized {
		if v == nil || v.Kind != KindGPU || readyNow[v.ID] || a.views[v.ID] != v {
			continue
		}
		a.resizeGPUView(v)
	}
}

func (a *App) resizeGPUView(v *View) {
	if a.target != nil || v == nil || v.Kind != KindGPU {
		return
	}
	st := a.gpuViews[v.ID]
	if st == nil || st.soft == nil {
		return
	}
	w, h := a.gpuPixelSize(v)
	ow, oh := st.soft.Size()
	if ow == w && oh == h {
		return
	}
	st.soft.Resize(w, h)
	if st.ready && !st.lost {
		gen := st.generation
		a.post(driver.Event{
			Kind: driver.EventGPU,
			ID:   v.ID,
			Data: &driver.GPUEvent{
				Kind:       driver.GPUEventResize,
				Generation: gen,
				Width:      w,
				Height:     h,
			},
		})
	}
}

func (a *App) deleteGPUView(id driver.NodeID) {
	delete(a.gpuViews, id)
}

func (a *App) submitGPU(frame driver.GPUFrame) {
	owner := a
	if a.owner != nil {
		owner = a.owner
	}
	var draws, dispatches int
	var instances uint64
	for _, p := range frame.Passes {
		for _, c := range p.Commands {
			switch c.Kind {
			case driver.GPUCmdDraw, driver.GPUCmdDrawIndexed:
				draws++
				instances++
			case driver.GPUCmdDrawInstanced, driver.GPUCmdDrawIndexedInstanced:
				draws++
				instances += uint64(c.U1)
			case driver.GPUCmdDispatch, driver.GPUCmdDispatchIndirect:
				dispatches++
			}
		}
	}
	rec := GPUFrameRecord{
		SurfaceID:       frame.SurfaceID,
		ViewID:          frame.ViewID,
		Generation:      frame.Generation,
		FrameIndex:      frame.FrameIndex,
		FrameSlot:       frame.FrameSlot,
		OpsCount:        len(frame.Ops),
		CopiesCount:     len(frame.Copies),
		PassesCount:     len(frame.Passes),
		DrawsCount:      draws,
		InstancesCount:  instances,
		DispatchesCount: dispatches,
		Present:         frame.Present,
	}
	owner.gpuFrames = append(owner.gpuFrames, rec)

	// Node IDs belong to a tree, so the surface selects the device map and the tree that must receive its errors and readbacks.
	surface := owner.surfaceModels[frame.SurfaceID]
	if surface == nil || surface.closed {
		return
	}
	model := surface.model
	st := model.gpuViews[frame.ViewID]
	if st == nil || st.soft == nil || st.lost {
		return
	}
	if frame.Generation != 0 && frame.Generation != st.generation {
		return
	}
	st.frames = append(st.frames, rec)
	readbacks, err := st.soft.Submit(frame)
	if err != nil {
		model.postTo(model.tree, driver.Event{
			Kind: driver.EventGPU,
			ID:   frame.ViewID,
			Data: &driver.GPUEvent{
				Kind:       driver.GPUEventError,
				Generation: st.generation,
				ErrorCode:  driver.GPUErrorFailed,
				Message:    err.Error(),
			},
		})
	}
	for _, rb := range readbacks {
		rb := rb
		model.postTo(model.tree, driver.Event{
			Kind: driver.EventGPU,
			ID:   frame.ViewID,
			Data: &driver.GPUEvent{
				Kind:       driver.GPUEventReadback,
				Generation: st.generation,
				ReadbackID: rb.ID,
				Pixels:     rb.Pixels,
				Message:    rb.Err,
			},
		})
	}
	if timings := st.soft.LastPassTimings(); len(timings) > 0 {
		owner.post(driver.Event{
			Kind: driver.EventGPU,
			ID:   frame.ViewID,
			Data: &driver.GPUEvent{
				Kind:        driver.GPUEventPassTimings,
				Generation:  st.generation,
				PassTimings: timings,
			},
		})
	}
}

// GPUFrames returns all off-commit GPU frames submitted so far across the app,
// oldest first.
func (a *App) GPUFrames() []GPUFrameRecord {
	owner := a
	if a.owner != nil {
		owner = a.owner
	}
	out := make([]GPUFrameRecord, len(owner.gpuFrames))
	copy(out, owner.gpuFrames)
	return out
}

// GPUReadPixels returns an independent RGBA snapshot of the rendered pixels in
// the KindGPU view matching selector.
func (a *App) GPUReadPixels(selector string) (*image.RGBA, error) {
	a.t.Helper()
	v := a.Find(selector)
	if v == nil {
		return nil, fmt.Errorf("nsgotest: no view matches %q", selector)
	}
	if v.Kind != KindGPU {
		return nil, fmt.Errorf("nsgotest: view %q is %s, not gpu", selector, v.Kind)
	}
	st := a.gpuViews[v.ID]
	if st == nil || st.soft == nil {
		return nil, fmt.Errorf("nsgotest: gpu view %q has no active software surface", selector)
	}
	return st.soft.SurfaceImage(), nil
}

// TriggerGPUDeviceLost simulates a GPU device/context loss on the KindGPU view
// matching selector and settles the app.
func (a *App) TriggerGPUDeviceLost(selector string, reason GPUDeviceLostReason, message string) {
	a.t.Helper()
	a.TriggerGPUDeviceLostWithReason(selector, reason, message)
}

// TriggerGPUDeviceLostWithReason simulates a GPU device, WebGL/WebGPU context, or Android surface loss with an explicit reason on the KindGPU view matching selector and settles the app.
func (a *App) TriggerGPUDeviceLostWithReason(selector string, reason GPUDeviceLostReason, message string) {
	a.t.Helper()
	v := a.Find(selector)
	st := a.gpuViews[v.ID]
	if st == nil {
		a.t.Fatalf("nsgotest: view %q is not an active GPU view", selector)
	}
	st.lost = true
	st.ready = false
	a.post(driver.Event{
		Kind: driver.EventGPU,
		ID:   v.ID,
		Data: &driver.GPUEvent{
			Kind:       driver.GPUEventDeviceLost,
			Generation: st.generation,
			LostReason: driver.GPUDeviceLostReason(reason),
			Message:    message,
		},
	})
	a.Settle()
}

// RestoreGPUDevice simulates GPU device recreation after a device loss on the
// KindGPU view matching selector, bumping its generation and invoking OnReady and OnDeviceRestored.
func (a *App) RestoreGPUDevice(selector string) {
	a.t.Helper()
	v := a.Find(selector)
	st := a.gpuViews[v.ID]
	if st == nil {
		a.t.Fatalf("nsgotest: view %q is not an active GPU view", selector)
	}
	w, h := a.gpuPixelSize(v)
	if st.overrideW > 0 && st.overrideH > 0 {
		w, h = st.overrideW, st.overrideH
	}
	st.lost = false
	st.ready = true
	st.generation++
	st.soft = driver.NewGPUSoftwareDevice(w, h, st.cfg)
	surfFmt := st.cfg.ColorFormat
	if surfFmt == driver.GPUFormatUndefined {
		surfFmt = driver.GPUFormatRGBA8Unorm
	}
	a.post(driver.Event{
		Kind: driver.EventGPU,
		ID:   v.ID,
		Data: &driver.GPUEvent{
			Kind:           driver.GPUEventDeviceReady,
			Generation:     st.generation,
			Width:          w,
			Height:         h,
			SurfaceFormat:  surfFmt,
			FramesInFlight: driver.NormalizeFramesInFlight(st.cfg.FramesInFlight),
			Limits:         driver.DefaultSoftwareGPULimits(),
			Features:       a.gpuFeatures(),
		},
	})
	a.Settle()
}

// TriggerGPUSurfaceRotation simulates an Android surface destruction and recreation across orientation change on the KindGPU view matching selector. If width or height is <= 0, the current surface width and height are swapped.
func (a *App) TriggerGPUSurfaceRotation(selector string, width, height int) {
	a.t.Helper()
	v := a.Find(selector)
	owner := a
	if a.owner != nil {
		owner = a.owner
	}
	st := owner.gpuViews[v.ID]
	if st == nil {
		a.t.Fatalf("nsgotest: view %q is not an active GPU view", selector)
	}
	curW, curH := owner.gpuPixelSize(v)
	if st.soft != nil {
		curW, curH = st.soft.Size()
	}
	if width <= 0 || height <= 0 {
		width, height = curH, curW
	}
	a.TriggerGPUDeviceLostWithReason(selector, GPULostSurfaceDestroyed, "surface destroyed for rotation")
	st.overrideW = width
	st.overrideH = height
	a.RestoreGPUDevice(selector)
}
