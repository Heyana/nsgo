//go:build js && wasm && nsgotest

package web

import (
	"sync"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestTouchDragOnTouchViewDoesNotScrollOrCancel drives real Chromium touch
// drags over CDP (Input.dispatchTouchEvent) across:
//  1. an OnTouch-only view sitting on a scrollable page,
//  2. an OnTouch-only view nested inside a core.Scroll, and
//  3. a plain child inside the same core.Scroll.
//
// The first two must deliver continuous TouchMove events with zero
// pointercancel / TouchCancel and zero page/container scroll, while the third
// must still scroll the core.Scroll natively.
func TestTouchDragOnTouchViewDoesNotScrollOrCancel(t *testing.T) {
	g := js.Global()
	doc := g.Get("document")
	if !doc.Truthy() || g.Get("nsgoTest").Type() != js.TypeFunction {
		t.Skip("requires the browser test host: run with nsgo test -target web")
	}

	// Apply the global rule that page.css emits in real web builds.
	style := doc.Call("createElement", "style")
	style.Set("textContent", "* { touch-action: pan-x pan-y; }\nhtml, body { margin: 0; height: auto; min-height: 2000px; }")
	doc.Get("head").Call("appendChild", style)
	defer style.Call("remove")

	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-touch-drag-test")
	setStyle(mount, map[string]string{"width": "360px", "height": "520px", "position": "relative"})
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")

	// Reset any previous page scroll.
	g.Call("scrollTo", 0, 0)

	type touchLog struct {
		starts, moves, ends, cancels int
	}
	var (
		mu       sync.Mutex
		pageLog  touchLog
		innerLog touchLog
		pCancels int
	)
	record := func(l *touchLog, phase core.TouchPhase) {
		mu.Lock()
		defer mu.Unlock()
		switch phase {
		case core.TouchStart:
			l.starts++
		case core.TouchMove:
			l.moves++
		case core.TouchEnd:
			l.ends++
		case core.TouchCancel:
			l.cancels++
		}
	}

	s := open(func(*core.Scope) core.Node {
		return core.View{
			Style: core.Style{Width: core.Pt(360), Height: core.Pt(520), Direction: core.Column},
			Children: []core.Node{
				// Top half: OnTouch-only view (no OnPan/OnPinch/OnRotation) on the scrollable page.
				core.View{
					TestID:  "page-pad",
					Style:   core.Style{Width: core.Pt(360), Height: core.Pt(200)},
					OnTouch: func(e core.TouchEvent) { record(&pageLog, e.Phase) },
				},
				// Bottom half: a Scroll view containing an OnTouch view and tall plain content.
				core.Scroll{
					TestID: "scroll",
					Style:  core.Style{Width: core.Pt(360), Height: core.Pt(320)},
					Children: []core.Node{
						core.View{
							TestID:  "scroll-pad",
							Style:   core.Style{Width: core.Pt(360), Height: core.Pt(140)},
							OnTouch: func(e core.TouchEvent) { record(&innerLog, e.Phase) },
						},
						core.View{
							TestID: "scroll-plain",
							Style:  core.Style{Width: core.Pt(360), Height: core.Pt(900)},
						},
					},
				},
			},
		}
	}, Options{Mount: "#nsgo-touch-drag-test"}, nil)
	defer s.engine.Close()

	var scrollView, pagePad, scrollPad *view
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		for _, v := range s.views {
			if v == nil {
				continue
			}
			switch v.props.TestID {
			case "scroll":
				if v.kind == driver.KindScroll && v.scroll.Height >= 1000 {
					scrollView = v
				}
			case "page-pad":
				pagePad = v
			case "scroll-pad":
				scrollPad = v
			}
		}
		if scrollView != nil && pagePad != nil && scrollPad != nil {
			break
		}
	}
	if scrollView == nil || pagePad == nil || scrollPad == nil {
		t.Fatal("test views never mounted")
	}
	s.engine.Sync()

	// Verify computed touch-action on the OnTouch views beats `* { touch-action: pan-x pan-y; }`.
	for name, v := range map[string]*view{"page-pad": pagePad, "scroll-pad": scrollPad} {
		if got := g.Call("getComputedStyle", v.el).Get("touchAction").String(); got != "none" {
			t.Fatalf("%s computed touch-action = %q, want %q", name, got, "none")
		}
	}

	cancelListener := js.FuncOf(func(js.Value, []js.Value) any {
		mu.Lock()
		pCancels++
		mu.Unlock()
		return nil
	})
	s.host.Call("addEventListener", "pointercancel", cancelListener, true)
	defer func() {
		s.host.Call("removeEventListener", "pointercancel", cancelListener, true)
		cancelListener.Release()
	}()

	touchDrag := func(x0, y0, x1, y1 float64) {
		t.Helper()
		err := new(testSurface).do(map[string]any{
			"op": "touchDrag",
			"path": []map[string]any{
				{"x": x0, "y": y0},
				{"x": x1, "y": y1},
			},
		})
		if err != nil {
			t.Fatalf("touchDrag failed: %v", err)
		}
		s.engine.Sync()
	}

	// 1. Drag diagonally/vertically across the OnTouch view on the scrollable page.
	pr := pagePad.el.Call("getBoundingClientRect")
	px := pr.Get("left").Float() + 180
	py := pr.Get("top").Float() + 160
	touchDrag(px, py, px-40, py-110)

	mu.Lock()
	gotPage, gotPCancels := pageLog, pCancels
	mu.Unlock()
	if gotPCancels != 0 || gotPage.cancels != 0 {
		t.Fatalf("page-pad drag cancelled: pointercancel=%d TouchCancel=%d (log=%+v)", gotPCancels, gotPage.cancels, gotPage)
	}
	if gotPage.starts != 1 || gotPage.moves < 3 || gotPage.ends != 1 {
		t.Fatalf("page-pad touch phases = %+v, want 1 start, >=3 moves, 1 end", gotPage)
	}
	if sy := g.Get("scrollY").Float(); sy != 0 {
		t.Fatalf("page scrolled during OnTouch drag: window.scrollY = %v, want 0", sy)
	}

	// 2. Drag upward across the OnTouch view inside the scrollable core.Scroll.
	ir := scrollPad.el.Call("getBoundingClientRect")
	ix := ir.Get("left").Float() + 180
	iy := ir.Get("top").Float() + 120
	touchDrag(ix, iy, ix-20, iy-90)

	mu.Lock()
	gotInner, gotPCancels := innerLog, pCancels
	mu.Unlock()
	if gotPCancels != 0 || gotInner.cancels != 0 {
		t.Fatalf("scroll-pad drag cancelled: pointercancel=%d TouchCancel=%d (log=%+v)", gotPCancels, gotInner.cancels, gotInner)
	}
	if gotInner.starts != 1 || gotInner.moves < 3 || gotInner.ends != 1 {
		t.Fatalf("scroll-pad touch phases = %+v, want 1 start, >=3 moves, 1 end", gotInner)
	}
	if sy := g.Get("scrollY").Float(); sy != 0 {
		t.Fatalf("page scrolled during nested OnTouch drag: window.scrollY = %v, want 0", sy)
	}
	if st := scrollView.el.Get("scrollTop").Float(); st != 0 {
		t.Fatalf("core.Scroll scrolled during nested OnTouch drag: scrollTop = %v, want 0", st)
	}

	// 3. Drag upward on the plain area inside core.Scroll and verify it still scrolls natively.
	sr := scrollView.el.Call("getBoundingClientRect")
	sx := sr.Get("left").Float() + 180
	sy0 := sr.Get("top").Float() + 260
	touchDrag(sx, sy0, sx, sy0-100)

	var scrollTop float64
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(15 * time.Millisecond) {
		if scrollTop = scrollView.el.Get("scrollTop").Float(); scrollTop > 0 {
			break
		}
	}
	if scrollTop <= 0 {
		t.Fatalf("core.Scroll did not scroll on plain touch drag: scrollTop = %v, want > 0", scrollTop)
	}
}
