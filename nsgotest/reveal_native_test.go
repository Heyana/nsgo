package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

type revealSurface struct {
	noSurface
	scroll func(driver.Point, driver.Point)
}

func (s revealSurface) Scroll(at, delta driver.Point) error { s.scroll(at, delta); return nil }

// A native footer can cover a target whose center is inside its scroll view. Both the model and the target must find a visible point, including for a nested horizontal scroll whose gesture would otherwise land on the footer.
func TestRevealMovesTargetAboveNativeFooter(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		top, height, content, footer       core.Points
		nested, hidden, passThrough, child bool
	}{
		{name: "footer", top: 150, height: 40, content: 600, footer: 60},
		{name: "large footer", top: 150, height: 40, content: 600, footer: 130},
		{name: "oversized row", top: 50, height: 260, content: 600, footer: 60},
		{name: "clamped scroll", top: 560, height: 40, content: 660, footer: 60},
		{name: "nested horizontal", top: 30, height: 40, content: 600, footer: 60, nested: true},
		{name: "hidden footer", top: 150, height: 40, content: 600, footer: 60, hidden: true},
		{name: "pointer passes through", top: 150, height: 40, content: 600, footer: 60, passThrough: true},
		{name: "target contains native child", top: 150, height: 40, content: 600, footer: 60, child: true, hidden: true},
	} {
		for _, native := range []bool{false, true} {
			mode := "model"
			if native {
				mode = "target"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				app := Start(t, func(*core.Scope) core.Node {
					row := core.View{TestID: "row", Style: core.Style{Position: core.Absolute, Top: core.Pt(tc.top), Width: core.Pt(200), Height: core.Pt(tc.height)}, OnPress: func(core.PressEvent) {}}
					if tc.child {
						row.Children = []core.Node{core.Native{Component: "row-child", Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}}
					}
					var content core.Node = row
					if tc.nested {
						row.Style.Left, row.Style.Width = core.Pt(320), core.Pt(40)
						content = core.Scroll{TestID: "inner", Horizontal: true, Style: core.Style{Position: core.Absolute, Top: core.Pt(120), Width: core.Pt(200), Height: core.Pt(100)}, Children: []core.Node{
							core.View{Style: core.Style{Width: core.Pt(500), Height: core.Pt(100)}, Children: []core.Node{row}},
						}}
					}
					footerStyle := core.Style{Position: core.Absolute, Bottom: core.Pt(0), Width: core.Pt(200), Height: core.Pt(tc.footer)}
					if tc.hidden {
						footerStyle.Visibility = core.VisibilityHidden
					}
					if tc.passThrough {
						footerStyle.PointerEvents = core.PointerEventsNone
					}
					return core.View{Style: core.Style{Width: core.Pt(200), Height: core.Pt(200)}, Children: []core.Node{
						core.Scroll{TestID: "scroll", Style: core.Style{Width: core.Pt(200), Height: core.Pt(200)}, Children: []core.Node{
							core.View{Style: core.Style{Height: core.Pt(tc.content)}, Children: []core.Node{content}},
						}},
						core.Native{Component: "footer", TestID: "footer", Style: footerStyle},
					}}
				}, Size(200, 200))
				row := app.Find("id:row")
				var at driver.Point
				if native {
					surface := revealSurface{scroll: func(point, delta driver.Point) {
						if !tc.hidden && !tc.passThrough && app.FindMounted("id:footer").Bounds().Contains(point) {
							t.Fatalf("scroll starts under the native footer: %v", point)
						}
						if delta.X != 0 && tc.nested {
							app.Scroll("id:inner", delta.X)
						}
						if delta.Y != 0 {
							app.Scroll("id:scroll", delta.Y)
						}
					}}
					at = (&target{a: app, surface: surface}).reveal(row)
				} else {
					at = app.reveal(row)
				}
				footer := app.FindMounted("id:footer").Bounds()
				if tc.hidden || tc.passThrough {
					if app.Find("id:scroll").Offset.Y != 0 {
						t.Fatal("an unobscured target scrolled")
					}
				} else if footer.Contains(at) || at.Y < 0 || at.Y >= 200 || app.Find("id:scroll").Offset.Y <= 0 {
					t.Fatalf("target stays under the native footer: point=%v footer=%+v offset=%+v", at, footer, app.Find("id:scroll").Offset)
				}
			})
		}
	}
}
