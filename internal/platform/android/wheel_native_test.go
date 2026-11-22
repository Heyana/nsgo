//go:build android && nsgotest

package android

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func wheelAndTouchpadMomentumNative(t *testing.T) {
	var mu sync.Mutex
	var events []core.WheelEvent
	surface, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		return core.View{
			Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
			Children: []core.Node{
				core.View{
					TestID: "wheel-box",
					Style:  core.Style{Position: core.Absolute, Left: core.Pt(40), Top: core.Pt(60), Width: core.Pt(240), Height: core.Pt(200)},
					OnWheel: func(e core.WheelEvent) {
						mu.Lock()
						events = append(events, e)
						mu.Unlock()
					},
				},
			},
		}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := surface.(*testSurface)
	s.Sync()

	// 1. Discrete mouse wheel notch at (140, 150) -> local (100, 90).
	if err := s.do(map[string]any{"op": "wheel", "x": 140, "y": 150, "v": -1, "h": 0, "touchpad": false}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(events)
		mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("mouse wheel event never arrived")
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	m := events[0]
	mu.Unlock()
	if math.Abs(float64(m.Position.X-100)) > 3 || math.Abs(float64(m.Position.Y-90)) > 3 {
		t.Errorf("mouse wheel Position = %+v, want (100, 90)", m.Position)
	}
	if math.Abs(float64(m.Delta.Y+99)) > 1 || m.Touchpad || m.Phase != core.GestureChanged {
		t.Errorf("mouse wheel = %+v, want Delta.Y=-99 Touchpad=false GestureChanged", m)
	}

	// 2. Touchpad scroll sequence -> GestureBegan, GestureChanged, then GestureEnded with momentum Velocity!
	if err := s.do(map[string]any{
		"op": "wheel", "x": 140, "y": 150, "touchpad": true,
		"steps": []map[string]any{
			{"h": 0, "v": 12, "dt": 16},
			{"h": 0, "v": 12, "dt": 16},
			{"h": 0, "v": 12, "dt": 16},
			{"h": 0, "v": 12, "dt": 16},
		},
	}); err != nil {
		t.Fatal(err)
	}
	var began, ended *core.WheelEvent
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		for i := 1; i < len(events); i++ {
			ev := events[i]
			if ev.Touchpad && ev.Phase == core.GestureBegan && began == nil {
				cp := ev
				began = &cp
			}
			if ev.Touchpad && ev.Phase == core.GestureEnded {
				cp := ev
				ended = &cp
			}
		}
		mu.Unlock()
		if began != nil && ended != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if began == nil || ended == nil {
		t.Fatalf("touchpad sequence incomplete: began=%v ended=%v", began, ended)
	}
	if ended.Velocity.Y <= 100 {
		t.Errorf("touchpad release Velocity = %+v, want positive Y momentum > 100 pt/s", ended.Velocity)
	}
}

func pinchPositionNative(t *testing.T) {
	var mu sync.Mutex
	var pinches []core.GestureEvent
	surface, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		return core.View{
			Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
			Children: []core.Node{
				core.View{
					TestID: "pinch-box",
					Style:  core.Style{Position: core.Absolute, Left: core.Pt(40), Top: core.Pt(60), Width: core.Pt(240), Height: core.Pt(200)},
					OnPinch: func(e core.GestureEvent) {
						mu.Lock()
						pinches = append(pinches, e)
						mu.Unlock()
					},
				},
			},
		}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := surface.(*testSurface)
	s.Sync()

	// Real two-finger spread centered at surface (160, 150) -> local (120, 90).
	if err := s.do(map[string]any{"op": "pinch", "x": 160, "y": 150, "from": 50, "to": 160, "steps": 6}); err != nil {
		t.Fatal(err)
	}
	var began, changed, ended *core.GestureEvent
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		for _, g := range pinches {
			cp := g
			switch g.Phase {
			case core.GestureBegan:
				if began == nil {
					began = &cp
				}
			case core.GestureChanged:
				changed = &cp
			case core.GestureEnded:
				ended = &cp
			}
		}
		mu.Unlock()
		if began != nil && changed != nil && ended != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if began == nil || changed == nil || ended == nil {
		t.Fatalf("pinch incomplete: began=%v changed=%v ended=%v", began, changed, ended)
	}
	for name, g := range map[string]*core.GestureEvent{"began": began, "changed": changed} {
		if math.Abs(float64(g.Position.X-120)) > 8 || math.Abs(float64(g.Position.Y-90)) > 8 {
			t.Errorf("pinch %s Position = %+v, want near (120, 90)", name, g.Position)
		}
	}
	if changed.Value <= 1.2 {
		t.Errorf("pinch spread Value = %v, want > 1.2", changed.Value)
	}
}
