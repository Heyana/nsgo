package press_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/dragdrop"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A real pointer drag negotiates the responder, transfers the Go value and
// cancels the original click; the same source still clicks before and after it.
func TestApplicationDragTransfersValueAndPreservesClicks(t *testing.T) {
	var presses, drops int
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		dropped := core.NewSignal(cx, false)
		return dragdrop.Area{Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, Gap: 40}, Children: []core.Node{
			dragdrop.Source{View: core.View{TestID: "source", Label: "Film", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80), Background: core.RGB(0x999999)}, OnPress: func(core.PressEvent) { presses++ }, Children: []core.Node{core.Text{Content: "Poster"}}}, Data: func() any { return []int{2, 5} }, Preview: func(any) core.Node { return core.Text{Content: "Two films", TestID: "preview"} }},
			dragdrop.Target{View: core.View{TestID: "target", Style: core.Style{Width: core.Pt(100), Height: core.Pt(100), Background: core.RGB(0xdddddd)}, Children: []core.Node{core.Text{ContentFunc: func() string {
				if dropped.Get() {
					return "Received"
				}
				return "Playlist"
			}}}}, Accept: func(data any) bool { ids, ok := data.([]int); return ok && len(ids) == 2 }, OnDrop: func(e dragdrop.Event) {
				ids := e.Data.([]int)
				if ids[0] != 2 || ids[1] != 5 {
					t.Error("drag changed the selected values")
				}
				drops++
				dropped.Set(true)
			}},
		}}}}
	}, nsgotest.Size(360, 240))
	app.Press("source")
	before := app.Find("source").ID
	app.DragTo("source", "target")
	app.WaitFor("text:Received", time.Second)
	app.Press("source")
	if presses != 2 || drops != 1 || app.Find("source").ID != before || app.Exists("preview") {
		t.Fatalf("presses=%d drops=%d source retained=%t", presses, drops, app.Find("source").ID == before)
	}
}
