//go:build ios && nsgotest

package watchos_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/watchos"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestCrownAdjustsSliderAndPansView verifies that turning the Digital Crown
// steps a value-turning control (base.Slider) and pans a view listening to
// OnPan when focused or active on watchOS.
func TestCrownAdjustsSliderAndPansView(t *testing.T) {
	var (
		mu          sync.Mutex
		sliderVal   = 50.0
		panChanges  int
		panEnded    bool
		panTotalX   core.Points
		sliderThumb core.Ref
		panRef      core.Ref
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		val := core.NewSignal(cx, base.SingleValue(50))
		return core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(8), Gap: 8}, Children: []core.Node{
			base.Slider{
				TestID:      "level",
				ThumbTestID: "level-thumb",
				ThumbRef:    &sliderThumb,
				Value:       val.Get,
				Min:         func() float64 { return 0 },
				Max:         func() float64 { return 100 },
				Step:        func() float64 { return 5 },
				OnChange: func(v base.SliderValue) {
					val.Set(v)
					mu.Lock()
					sliderVal = v.End
					mu.Unlock()
				},
				Style:          core.Style{Height: core.Pt(32)},
				TrackStyle:     core.Style{Height: core.Pt(32)},
				IndicatorStyle: core.Style{Height: core.Pt(8)},
				ThumbStyle:     core.Style{Width: core.Pt(20), Height: core.Pt(20)},
			},
			core.View{
				TestID:        "dial",
				Ref:           &panRef,
				Style:         core.Style{Height: core.Pt(60)},
				PanActivation: core.PanActivation{ActiveOffsetX: 4},
				OnPan: func(e core.PanEvent) {
					mu.Lock()
					defer mu.Unlock()
					switch e.Phase {
					case core.GestureChanged:
						panChanges++
						panTotalX = e.Translation.X
					case core.GestureEnded:
						panEnded = true
						panTotalX = e.Translation.X
					}
				},
			},
		}}
	})
	app.WaitFor("id:level-thumb", 5*time.Second)

	// Slider is the primary value-turning view on screen: turning the crown steps it.
	watchos.RotateCrownForTest(1.0, 1.0)
	app.Settle()
	mu.Lock()
	gotVal := sliderVal
	mu.Unlock()
	if gotVal != 60 {
		t.Fatalf("slider after two +1 crown detents = %v, want 60", gotVal)
	}

	// Focusing the panning view routes the Digital Crown to OnPan along X.
	app.Post(func() { panRef.Focus() })
	app.Settle()
	watchos.RotateCrownForTest(1.0, 0.5)
	app.Settle()
	mu.Lock()
	changes, ended, totalX := panChanges, panEnded, panTotalX
	mu.Unlock()
	if changes == 0 || !ended || totalX <= 0 {
		t.Fatalf("pan from crown = (changes=%d, ended=%v, totalX=%v), want positive horizontal pan ended", changes, ended, totalX)
	}
}

// TestNativeTouchesReachScrollAndField verifies that testSurface Drag, Scroll
// and Press reach SwiftUI's ScrollView and TextField on watchOS.
func TestNativeTouchesReachScrollAndField(t *testing.T) {
	var (
		mu        sync.Mutex
		offset    core.Point
		submitted string
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		rows := []core.Node{
			core.Input{
				TestID:      "name",
				Placeholder: "Name",
				Style:       core.Style{Height: core.Pt(36)},
				OnSubmit: func(s string) {
					mu.Lock()
					submitted = s
					mu.Unlock()
				},
			},
		}
		for i := range 20 {
			rows = append(rows, core.Text{Content: fmt.Sprint("Item ", i), Style: core.Style{Height: core.Pt(28)}})
		}
		return core.Scroll{
			TestID:   "list",
			Style:    core.Style{FlexGrow: 1},
			Children: rows,
			OnScroll: func(e core.ScrollEvent) {
				mu.Lock()
				offset = e.Offset
				mu.Unlock()
			},
		}
	})
	app.WaitFor("id:name", 5*time.Second)

	app.Press("id:name")
	app.Type("id:name", "Watch")
	app.Submit("id:name")
	mu.Lock()
	gotSub := submitted
	mu.Unlock()
	if gotSub != "Watch" {
		t.Fatalf("submitted = %q, want Watch", gotSub)
	}

	app.Drag("id:list", core.Point{Y: -80})
	app.WaitUntil(5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return offset.Y > 10
	})
}
