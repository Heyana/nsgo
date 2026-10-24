package main

import (
	"fmt"
	"slices"
	"time"

	"github.com/oneo-me/nsgo/core"
)

// listDemos show the props of core.List after NSH's: keyed rows that keep
// the visible ones in place, sticky section headers, separators,
// viewability, scroll targets, columns with pull to refresh, an inverted
// chat and a list laid out in the page that scrolls it.
func listDemos(cx *core.Scope, t *theme) []core.Node {
	// Sections of eight rows under a sticky header each, keyed by ID.
	ids := core.NewSignalFunc(cx, makeRange(0, 120), slices.Equal)
	newest := 0
	sections := &core.Ref{}
	viewing := core.NewSignal(cx, "")
	isHeader := func(id int) bool { return id >= 0 && id%8 == 0 }
	headers := func() []int {
		var at []int
		for i, id := range ids.Get() {
			if isHeader(id) {
				at = append(at, i)
			}
		}
		return at
	}
	insert := func() {
		newest -= 5
		ids.Update(func(old []int) []int { return append(makeRange(newest, newest+5), old...) })
	}
	section := core.List{TestID: "sections", Ref: sections, EstimatedSize: 44,
		Count: func() int { return len(ids.Get()) }, Key: func(i int) any { return ids.Get()[i] },
		StickyHeaderIndicesFunc: headers,
		Separator: func(*core.Scope) core.Node {
			return core.View{Style: core.Style{Height: core.Pt(1), Background: t.line}}
		},
		ViewabilityConfig: core.ViewabilityConfig{ItemVisiblePercentThreshold: 50},
		OnViewableItemsChange: func(v core.ViewableItems) {
			if n := len(v.Viewable); n > 0 {
				viewing.Set(fmt.Sprintf("Viewing %d rows from #%v", n, v.Viewable[0].Key))
			}
		},
		Style: core.Style{Height: core.Pt(320), BorderWidth: 1, BorderColor: t.line, CornerRadius: radius, Overflow: core.Hidden},
		Row: func(cx *core.Scope, r core.ListRow) core.Node {
			id := func() int { return ids.Get()[r.Index()] }
			return core.View{StyleFunc: func() core.Style {
				if isHeader(id()) {
					return core.Style{Height: core.Pt(32), Padding: core.Symmetric(0, 14), JustifyContent: core.JustifyCenter,
						Background: t.sunken}
				}
				return core.Style{Height: core.Pt(44), Padding: core.Symmetric(0, 14), JustifyContent: core.JustifyCenter,
					Background: t.surface}
			}, Children: []core.Node{core.Text{
				ContentFunc: func() string {
					if isHeader(id()) {
						return fmt.Sprintf("Section %d", id()/8+1)
					}
					return fmt.Sprintf("Row #%d", id())
				},
				StyleFunc: func() core.Style {
					if isHeader(id()) {
						return core.Style{FontWeight: core.FontWeightSemibold}
					}
					return core.Style{}
				}}}}
		}}

	// Tiles in three columns, refreshed by pulling them down.
	tiles := core.NewSignal(cx, 30)
	refreshing := core.NewSignal(cx, false)
	grid := core.List{TestID: "grid", NumColumns: 3, Gap: 8, ColumnGap: 8, EstimatedSize: 72,
		Count: tiles.Get, Refreshing: refreshing.Get,
		OnRefresh: func() {
			refreshing.Set(true)
			cx.AfterFunc(time.Second, func() {
				tiles.Set(tiles.Get() + 3)
				refreshing.Set(false)
			})
		},
		Style: core.Style{Height: core.Pt(260), Padding: core.All(8), BorderWidth: 1, BorderColor: t.line, CornerRadius: radius},
		Row: func(cx *core.Scope, r core.ListRow) core.Node {
			return core.View{StyleFunc: func() core.Style {
				return core.Style{Height: core.Pt(72), CornerRadius: radius, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
					Background: []core.Color{t.accent, t.success, t.warning, t.danger}[r.Index()%4]}
			}, Children: []core.Node{core.Text{ContentFunc: func() string { return fmt.Sprint(r.Index() + 1) },
				Style: core.Style{Color: core.RGB(0xffffff), FontWeight: core.FontWeightBold}}}}
		}}

	// A chat: the newest message is row 0, at the bottom.
	messages := core.NewSignalFunc(cx, []string{"Rows start at the bottom.", "Welcome to the chat."}, nil)
	send := func() {
		messages.Update(func(m []string) []string {
			return append([]string{fmt.Sprintf("Message %d", len(m)+1)}, m...)
		})
	}
	chat := core.List{TestID: "chat", Inverted: true, Gap: 6, EstimatedSize: 36,
		Count: func() int { return len(messages.Get()) },
		Style: core.Style{Height: core.Pt(220), Padding: core.All(10), BorderWidth: 1, BorderColor: t.line, CornerRadius: radius},
		Row: func(cx *core.Scope, r core.ListRow) core.Node {
			return core.View{Style: core.Style{AlignSelf: core.AlignStart, Padding: core.Symmetric(8, 12), CornerRadius: 14,
				Background: t.sunken}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return messages.Get()[r.Index()] }}}}
		}}

	return []core.Node{
		t.demo("Sections", "Keyed rows: rows inserted above keep the visible ones in place. Headers stick until the next pushes them out; separators go between rows.",
			flow(
				t.primary("Insert 5 above", insert),
				t.plain("Row 60 centered", func() { sections.ScrollToIndexAt(60, 0.5, 0, true) }),
				t.plain("End", func() { sections.ScrollToEnd(true) }),
			),
			section,
			t.live(viewing.Get),
		),
		t.demo("Columns", "NumColumns lays rows side by side; pull down to refresh.", grid),
		t.demo("Inverted", "A chat: row 0 at the bottom, the list keeps its distance from the end.",
			flow(t.primary("Send", send)), chat),
		t.demo("In the page", "Viewport: ListViewportParent lays the list out at its full length; the page scrolls it and only rows near the viewport mount.",
			core.List{TestID: "page-list", Viewport: core.ListViewportParent, Count: func() int { return 200 }, EstimatedSize: 40,
				Style: core.Style{BorderWidth: 1, BorderColor: t.line, CornerRadius: radius},
				Row: func(cx *core.Scope, r core.ListRow) core.Node {
					return core.View{Style: core.Style{Height: core.Pt(40), Padding: core.Symmetric(0, 14), JustifyContent: core.JustifyCenter},
						Children: []core.Node{core.Text{ContentFunc: func() string { return fmt.Sprintf("Page row %d", r.Index()+1) }}}}
				}}),
	}
}

// makeRange returns from, from+1, ..., to-1.
func makeRange(from, to int) []int {
	out := make([]int, 0, max(to-from, 0))
	for i := from; i < to; i++ {
		out = append(out, i)
	}
	return out
}
