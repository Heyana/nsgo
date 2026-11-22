//go:build js && wasm

package web

import (
	"math"
	"slices"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func (s *surface) displaySnapshot() ([]driver.DisplayInfo, string) {
	screen := js.Global().Get("screen")
	if !screen.Truthy() || screen.Get("width").Type() != js.TypeNumber || screen.Get("height").Type() != js.TypeNumber {
		return nil, ""
	}
	frame := driver.Bounds{Size: driver.Size{Width: driver.Points(screen.Get("width").Float()), Height: driver.Points(screen.Get("height").Float())}}
	visible := frame
	if width, height := screen.Get("availWidth"), screen.Get("availHeight"); width.Type() == js.TypeNumber && height.Type() == js.TypeNumber {
		visible.Size = driver.Size{Width: driver.Points(width.Float()), Height: driver.Points(height.Float())}
	}
	const id = "screen"
	rate := s.watch.refreshRate
	return []driver.DisplayInfo{{ID: id, Frame: frame, VisibleFrame: visible, Main: true,
		Scale: float32(js.Global().Get("devicePixelRatio").Float()), RefreshRate: rate, RefreshRateEstimated: rate > 0}}, id
}

// measureDisplayRate samples a short run of visible browser frames. Idle
// pages need no continuous frame clock to report an estimate.
func (s *surface) measureDisplayRate() {
	if s.watch.measuringDisplay || s.doc.Get("visibilityState").String() != "visible" {
		return
	}
	s.watch.measuringDisplay = true
	const intervals = 12
	samples := make([]float64, 0, intervals)
	last, frames := 0.0, 0
	var frame js.Func
	frame = js.FuncOf(func(_ js.Value, args []js.Value) any {
		visible := s.doc.Get("visibilityState").String() == "visible"
		if visible {
			now := args[0].Float()
			if delta := now - last; frames > 0 && delta > 0 && delta <= 100 {
				samples = append(samples, delta)
			}
			last, frames = now, frames+1
			if len(samples) < intervals && frames < intervals*2 {
				js.Global().Call("requestAnimationFrame", frame)
				return nil
			}
		}
		s.watch.measuringDisplay = false
		frame.Release()
		if visible && len(samples) >= 4 {
			slices.Sort(samples)
			s.watch.refreshRate = math.Round(1000 / samples[len(samples)/2])
			s.updateEnv()
		}
		return nil
	})
	js.Global().Call("requestAnimationFrame", frame)
}
