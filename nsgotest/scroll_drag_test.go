package nsgotest

import (
	"fmt"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

// A finger drags the content by its distance, as on a device, even as a List scrolls by itself under it to keep its
// visible rows in place while rows coming in above them turn out shorter than estimated.
func TestScrollDragsOnFromAListsScrollBy(t *testing.T) {
	ref := &core.Ref{}
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.List{TestID: "list", Ref: ref, Style: core.Style{Height: core.Pt(400)},
			Count: func() int { return 100 }, Key: func(i int) any { return i },
			EstimatedSize: 80, Row: func(_ *core.Scope, row core.ListRow) core.Node {
				return core.View{Style: core.Style{Height: core.Pt(20)}, Children: []core.Node{
					core.Text{ContentFunc: func() string { return fmt.Sprintf("row %d", row.Index()) }},
				}}
			}}
	})
	app.Post(func() { ref.ScrollToEnd(false) })
	app.Settle()
	before := app.Find("text:row 99").Bounds().Origin.Y
	app.Scroll("id:list", -200)
	if got := app.Find("text:row 99").Bounds().Origin.Y - before; got != 200 {
		t.Errorf("dragging the list down 200 moved its last row %v", got)
	}
}
