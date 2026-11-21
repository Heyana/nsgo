//go:build android && nsgotest

package android

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// The agent dispatches real MotionEvents through the Activity. These tests exercise the native scroll view and consuming, recycled rows, rather than nsgotest's model of a touch.
func listTouchStopsMomentum(t *testing.T, horizontal bool) {
	var downs, presses, dragStarts, dragEnds, starts, ends, rests atomic.Int32
	started := make(chan struct{}, 1)
	surface, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		return core.List{
			Count: func() int { return 2000 }, Horizontal: horizontal,
			Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
			Row: func(cx *core.Scope, row core.ListRow) core.Node {
				style := core.Style{Width: core.Pt(200)}
				if horizontal {
					style.Width, style.Height = core.Pt(100), core.Pt(200)
				}
				return core.View{
					Style: style, Role: core.RoleButton,
					OnPress:     func(core.PressEvent) { presses.Add(1) },
					OnPressDown: func(core.PressEvent) { downs.Add(1) },
					Children: []core.Node{core.Text{ContentFunc: func() string {
						return fmt.Sprintf("Row %d: %s", row.Index(), strings.Repeat("variable height text ", 2+row.Index()%5))
					}}},
				}
			},
			OnScrollDragStart: func(core.ScrollEvent) { dragStarts.Add(1) },
			OnScrollDragEnd:   func(core.ScrollEvent) { dragEnds.Add(1) },
			OnMomentumStart: func(core.ScrollEvent) {
				starts.Add(1)
				select {
				case started <- struct{}{}:
				default:
				}
			},
			OnMomentumEnd: func(core.ScrollEvent) { ends.Add(1) },
			OnScrollEnd:   func(core.ScrollEvent) { rests.Add(1) },
		}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := surface.(*testSurface)
	from, to := driver.Point{X: 100, Y: 400}, driver.Point{X: 100, Y: 150}
	if horizontal {
		from = driver.Point{X: 300, Y: 150}
	}
	// Wait for scrollable native content, rather than sleeping through layout.
	deadline := time.Now().Add(5 * time.Second)
	for {
		ready, err := s.ask(map[string]any{"op": "scroll-ready", "x": from.X, "y": from.Y})
		if err != nil {
			t.Fatal(err)
		}
		if ready == "true" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the native list did not become scrollable")
		}
		time.Sleep(20 * time.Millisecond)
	}
	blank := driver.Point{X: 300, Y: 150}
	if horizontal {
		blank = driver.Point{X: 100, Y: 300}
	}
	for trial, at := range []driver.Point{to, blank} {
		if err := s.do(map[string]any{"op": "swipe", "path": []driver.Point{from, to}}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("the native swipe did not start momentum")
		}
		if ends.Load() != int32(trial) {
			t.Fatal("momentum ended before the braking touch")
		}
		// The swipe's own DOWN landed on a row, so count from here.
		s.Sync()
		downsBefore, pressesBefore := downs.Load(), presses.Load()
		drift, err := s.ask(map[string]any{"op": "hold-scroll", "x": at.X, "y": at.Y})
		if err != nil {
			t.Fatal(err)
		}
		pixels, err := strconv.Atoi(drift)
		if err != nil {
			t.Fatal(err)
		}
		if pixels != 0 {
			t.Errorf("scroll moved %d native pixels over 20 frames while a child held the finger", pixels)
		}
		s.Sync()
		// The touch that brakes is the list's, as on platform lists: the row under it is neither pressed down nor pressed when the finger lifts.
		if d, p := downs.Load()-downsBefore, presses.Load()-pressesBefore; d != 0 || p != 0 {
			t.Errorf("a braking touch pressed a row down %d times and pressed it %d times; want neither (trial %d)", d, p, trial)
		}
		if starts.Load() != int32(trial+1) || ends.Load() != int32(trial+1) {
			t.Errorf("momentum events: start %d, end %d; want %d each", starts.Load(), ends.Load(), trial+1)
		}
		if dragStarts.Load() != int32(trial+1) || dragEnds.Load() != int32(trial+1) {
			t.Errorf("drag events: start %d, end %d; braking touch must not create a drag", dragStarts.Load(), dragEnds.Load())
		}
		if rests.Load() != 0 {
			t.Errorf("a touch interrupting momentum reported %d scroll ends", rests.Load())
		}
	}
	// At rest, the same row takes a tap, so the counts above can see a press.
	s.Sync()
	downsBefore, pressesBefore := downs.Load(), presses.Load()
	if err := s.Press(to, 0, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); presses.Load() == pressesBefore && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
		s.Sync()
	}
	if d, p := downs.Load()-downsBefore, presses.Load()-pressesBefore; d != 1 || p != 1 {
		t.Errorf("a tap at rest pressed a row down %d times and pressed it %d times; want 1 each", d, p)
	}
}

func zoomScrollTouchStopsMomentum(t *testing.T) {
	var downs, presses, dragStarts, dragEnds, starts, ends, rests atomic.Int32
	started := make(chan struct{}, 1)
	surface, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		var rows []core.Node
		for i := range 60 {
			rows = append(rows, core.View{
				Style: core.Style{Width: core.Pt(400), Height: core.Pt(60)}, Role: core.RoleButton,
				OnPress:     func(core.PressEvent) { presses.Add(1) },
				OnPressDown: func(core.PressEvent) { downs.Add(1) },
				Children:    []core.Node{core.Text{Content: fmt.Sprintf("Zoom row %d", i)}},
			})
		}
		return core.Scroll{
			Style:        core.Style{Width: core.Pct(100), Height: core.Pct(100)},
			MinZoomScale: 1, MaxZoomScale: 3,
			OnScrollDragStart: func(core.ScrollEvent) { dragStarts.Add(1) },
			OnScrollDragEnd:   func(core.ScrollEvent) { dragEnds.Add(1) },
			OnMomentumStart: func(core.ScrollEvent) {
				starts.Add(1)
				select {
				case started <- struct{}{}:
				default:
				}
			},
			OnMomentumEnd: func(core.ScrollEvent) { ends.Add(1) },
			OnScrollEnd:   func(core.ScrollEvent) { rests.Add(1) },
			Children:      rows,
		}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := surface.(*testSurface)
	from, to := driver.Point{X: 100, Y: 400}, driver.Point{X: 100, Y: 150}
	deadline := time.Now().Add(5 * time.Second)
	for {
		ready, err := s.ask(map[string]any{"op": "scroll-ready", "x": from.X, "y": from.Y})
		if err != nil {
			t.Fatal(err)
		}
		if ready == "true" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the native zoom scroll view did not become scrollable")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := s.do(map[string]any{"op": "swipe", "path": []driver.Point{from, to}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the native swipe did not start zoom scroll momentum")
	}
	if ends.Load() != 0 {
		t.Fatal("momentum ended before the braking touch")
	}
	s.Sync()
	downsBefore, pressesBefore := downs.Load(), presses.Load()
	drift, err := s.ask(map[string]any{"op": "hold-scroll", "x": to.X, "y": to.Y})
	if err != nil {
		t.Fatal(err)
	}
	pixels, err := strconv.Atoi(drift)
	if err != nil {
		t.Fatal(err)
	}
	if pixels != 0 {
		t.Errorf("zoom scroll moved %d native pixels over 20 frames while held", pixels)
	}
	s.Sync()
	if d, p := downs.Load()-downsBefore, presses.Load()-pressesBefore; d != 0 || p != 0 {
		t.Errorf("a braking touch on ZoomScroll pressed down %d times and pressed %d times; want neither", d, p)
	}
	if starts.Load() != 1 || ends.Load() != 1 {
		t.Errorf("momentum events: start %d, end %d; want 1 each", starts.Load(), ends.Load())
	}
	if dragStarts.Load() != 1 || dragEnds.Load() != 1 {
		t.Errorf("drag events: start %d, end %d; braking touch must not create a drag", dragStarts.Load(), dragEnds.Load())
	}
	if rests.Load() != 0 {
		t.Errorf("a touch interrupting ZoomScroll momentum reported %d scroll ends", rests.Load())
	}
	// At rest, a tap reaches the child normally.
	s.Sync()
	downsBefore, pressesBefore = downs.Load(), presses.Load()
	if err := s.Press(to, 0, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); presses.Load() == pressesBefore && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
		s.Sync()
	}
	if d, p := downs.Load()-downsBefore, presses.Load()-pressesBefore; d != 1 || p != 1 {
		t.Errorf("a tap at rest on ZoomScroll pressed down %d times and pressed %d times; want 1 each", d, p)
	}
}
