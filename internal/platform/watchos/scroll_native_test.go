//go:build ios && nsgotest

package watchos_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScrollBelowTheScreensTopRestsAtZero puts a scroll view 40 points down, within the top of the screen that is not safe: SwiftUI extends it up to the screen's edge and insets its content by as much, which reached Go as an offset of -40 at rest (#814). Offsets the app sets and reads stay measured from where the content rests.
func TestScrollBelowTheScreensTopRestsAtZero(t *testing.T) {
	var (
		mu     sync.Mutex
		at     core.Point
		events int
		ref    core.Ref
	)
	offset := func() (core.Point, int) {
		mu.Lock()
		defer mu.Unlock()
		return at, events
	}
	fill := core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		var rows []core.Node
		for i := range 20 {
			rows = append(rows, core.Text{Content: fmt.Sprint("Row ", i), Style: core.Style{Height: core.Pt(30)}})
		}
		return core.View{Style: fill, Children: []core.Node{
			core.View{TestID: "top", OnPress: func(core.PressEvent) { ref.ScrollTo(core.Point{}, false) }, Style: core.Style{Height: core.Pt(40)},
				Children: []core.Node{core.Text{Content: "Header"}}},
			core.Scroll{TestID: "list", Ref: &ref, Style: fill, Children: rows, OnScroll: func(e core.ScrollEvent) {
				mu.Lock()
				at, events = e.Offset, events+1
				mu.Unlock()
			}},
		}}
	})
	app.WaitFor("id:list", 5*time.Second)
	// The host reports the scroll view's place once it lays it out.
	app.WaitUntil(5*time.Second, func() bool { _, n := offset(); return n > 0 })
	app.FastForward(time.Second)
	if got, _ := offset(); got != (core.Point{}) {
		t.Fatalf("the scroll view rests at %v, want its top", got)
	}
	waitMatch := func(desc string, match func(core.Point) bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); ; {
			app.Settle()
			got, _ := offset()
			if match(got) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("the scroll view is at %v, want %s", got, desc)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	app.Scroll("id:list", 60)
	waitMatch("Y > 20", func(p core.Point) bool { return p.Y > 20 })
	app.Press("id:top")
	waitMatch("(0, 0)", func(p core.Point) bool { return p == (core.Point{}) })
}
