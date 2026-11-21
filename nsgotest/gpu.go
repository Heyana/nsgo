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

// GPUFrameRecord summarizes one off-commit GPUFrame submitted by a gpu.View.
type GPUFrameRecord struct {
	SurfaceID   uint32
	ViewID      driver.NodeID
	Generation  uint32
	OpsCount    int
	CopiesCount int
	PassesCount int
	DrawsCount  int
	Present     bool
}

type gpuViewState struct {
	id         driver.NodeID
	cfg        driver.GPUViewConfig
	generation uint32
	ready      bool
	lost       bool
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
			a.post(driver.Event{
				Kind: driver.EventGPU,
				ID:   v.ID,
				Data: &driver.GPUEvent{
					Kind:          driver.GPUEventDeviceReady,
					Generation:    gen,
					Width:         w,
					Height:        h,
					SurfaceFormat: driver.GPUFormatRGBA8Unorm,
					Limits:        driver.DefaultGPULimits(),
					Features:      driver.DefaultSoftwareGPUFeatures(),
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
	var draws int
	for _, p := range frame.Passes {
		for _, c := range p.Commands {
			switch c.Kind {
			case driver.GPUCmdDraw, driver.GPUCmdDrawIndexed, driver.GPUCmdDrawInstanced, driver.GPUCmdDrawIndexedInstanced:
				draws++
			}
		}
	}
	rec := GPUFrameRecord{
		SurfaceID:   frame.SurfaceID,
		ViewID:      frame.ViewID,
		Generation:  frame.Generation,
		OpsCount:    len(frame.Ops),
		CopiesCount: len(frame.Copies),
		PassesCount: len(frame.Passes),
		DrawsCount:  draws,
		Present:     frame.Present,
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
// KindGPU view matching selector, bumping its generation and invoking OnReady.
func (a *App) RestoreGPUDevice(selector string) {
	a.t.Helper()
	v := a.Find(selector)
	st := a.gpuViews[v.ID]
	if st == nil {
		a.t.Fatalf("nsgotest: view %q is not an active GPU view", selector)
	}
	w, h := a.gpuPixelSize(v)
	st.lost = false
	st.ready = true
	st.generation++
	st.soft = driver.NewGPUSoftwareDevice(w, h, st.cfg)
	a.post(driver.Event{
		Kind: driver.EventGPU,
		ID:   v.ID,
		Data: &driver.GPUEvent{
			Kind:          driver.GPUEventDeviceReady,
			Generation:    st.generation,
			Width:         w,
			Height:        h,
			SurfaceFormat: driver.GPUFormatRGBA8Unorm,
			Limits:        driver.DefaultGPULimits(),
			Features:      driver.GPUFeatures{Depth32Float: true},
		},
	})
	a.Settle()
}
