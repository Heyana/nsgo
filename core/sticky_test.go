package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestStickyEndInsets(t *testing.T) {
	for _, horizontal := range []bool{false, true} {
		for _, c := range []struct {
			name                   string
			before, content, inset Points
			offsets, positions     []Points
		}{
			{"long", 0, 1000, 0, []Points{0, 400, 840}, []Points{160, 160, 160}},
			{"inset", 0, 1000, 10, []Points{0, 400, 840}, []Points{150, 150, 150}},
			{"short", 0, 60, 10, []Points{0}, []Points{60}},
			{"parent boundary", 300, 1000, 10, []Points{0, 200, 1100, 1300}, []Points{300, 150, 150, 0}},
		} {
			t.Run(c.name+map[bool]string{false: "/vertical", true: "/horizontal"}[horizontal], func(t *testing.T) {
				axisStyle := func(length Points) Style {
					if horizontal {
						return Style{Width: Pt(length), Height: Pt(200)}
					}
					return Style{Height: Pt(length), Width: Pt(200)}
				}
				h := newHost(t, Size{Width: 300, Height: 300}, func(cx *Scope) Node {
					footer := axisStyle(40)
					footer.Position = Sticky
					footer.Bottom = Pt(c.inset)
					if horizontal {
						footer.Right = Pt(c.inset)
					}
					section := axisStyle(c.content + 40)
					scroll := Style{Width: Pt(200), Height: Pt(200)}
					if horizontal {
						section.Direction, scroll.Direction = Row, Row
					}
					return Scroll{TestID: "scroll", Horizontal: horizontal, Style: scroll, Children: []Node{
						View{Style: axisStyle(c.before)},
						View{Style: section, Children: []Node{
							View{Style: axisStyle(c.content)},
							View{TestID: "footer", Style: footer},
						}},
						View{Style: axisStyle(300)},
					}}
				})
				h.commit()
				scroll, cell, ok := pinRange(h.find("footer"))
				if !ok || scroll != h.find("scroll") {
					t.Fatal("footer has no sticky range in its scroll view")
				}
				for i, offset := range c.offsets {
					got := c.before + c.content - offset + cell.StickyShift(offset)
					if got != c.positions[i] {
						t.Errorf("offset %v: footer at %v, want %v", offset, got, c.positions[i])
					}
				}
			})
		}
	}
}

func TestStickyEndRangeFollowsChanges(t *testing.T) {
	var content *Signal[Points]
	var bottom *Signal[Length]
	h := newHost(t, Size{Width: 300, Height: 300}, func(cx *Scope) Node {
		content, bottom = NewSignal(cx, Points(1000)), NewSignal(cx, Pt(0))
		return Scroll{TestID: "scroll", Style: Style{Height: Pt(200)}, Children: []Node{
			View{StyleFunc: func() Style { return Style{Height: Pt(content.Get())} }},
			View{TestID: "footer", StyleFunc: func() Style { return Style{Height: Pt(40), Position: Sticky, Bottom: bottom.Get()} }},
		}}
	})
	check := func(want Points) {
		t.Helper()
		b := h.commit()
		var cells []stickyRange
		for _, it := range b.Items {
			if it.Kind == driver.ItemStickyCells {
				cells = it.Sticky
			}
		}
		if len(cells) != 1 {
			t.Fatalf("sticky ranges = %v, want one", cells)
		}
		if got := h.find("footer").frame.Origin.Y + cells[0].StickyShift(0); got != want {
			t.Errorf("footer at %v, want %v", got, want)
		}
	}
	check(160)
	bottom.Set(Pct(10))
	check(140)
	content.Set(60)
	check(60)
	bottom.Set(Length{})
	b := h.commit()
	for _, it := range b.Items {
		if it.Kind == driver.ItemStickyCells && len(it.Sticky) != 0 {
			t.Errorf("auto inset retained sticky ranges: %v", it.Sticky)
		}
	}
}
