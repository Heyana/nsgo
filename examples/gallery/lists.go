package main

import (
	"fmt"
	"slices"
	"time"

	"github.com/oneo-me/nsgo/core"
)

// fruit is one row of the keyed list.
type fruit struct {
	ID   int
	Name string
}

var fruitNames = []string{"Apple", "Banana", "Cherry", "Date", "Elderberry", "Fig", "Grape", "Kiwi", "Lemon", "Mango"}

func listsPage(cx *core.Scope, t *theme) core.Node {
	const rows = 10000
	list := &core.Ref{}
	visible := core.NewSignal(cx, 0)

	fruits := core.NewSignalFunc(cx, []fruit{{1, "Apple"}, {2, "Banana"}, {3, "Cherry"}}, nil)
	next := 4
	add := func() {
		fruits.Update(func(fs []fruit) []fruit {
			fs = append(fs, fruit{next, fruitNames[(next-1)%len(fruitNames)]})
			next++
			return fs
		})
	}
	remove := func(id int) {
		fruits.Update(func(fs []fruit) []fruit {
			return slices.DeleteFunc(slices.Clone(fs), func(f fruit) bool { return f.ID == id })
		})
	}
	reverse := func() {
		fruits.Update(func(fs []fruit) []fruit {
			fs = slices.Clone(fs)
			slices.Reverse(fs)
			return fs
		})
	}

	pages := core.NewSignal(cx, 1)
	loading := core.NewSignal(cx, false)
	loadMore := func() {
		if loading.Get() || pages.Get() >= 10 {
			return
		}
		loading.Set(true)
		cx.AfterFunc(400*time.Millisecond, func() {
			pages.Update(func(n int) int { return n + 1 })
			loading.Set(false)
		})
	}
	return core.View{Style: core.Style{Gap: 16}, Children: append([]core.Node{
		t.demo("Loading more", "OnLoadMore asks for the next page as the viewport comes within a viewport of the end, once per count.",
			core.List{Count: func() int { return pages.Get() * 20 }, EstimatedSize: 40, OnLoadMore: loadMore,
				Style: core.Style{Height: core.Pt(220), BorderWidth: 1, BorderColor: t.line, CornerRadius: radius, Overflow: core.Hidden},
				Row: func(cx *core.Scope, r core.ListRow) core.Node {
					return core.View{Style: core.Style{Height: core.Pt(40), Padding: core.Symmetric(0, 14), JustifyContent: core.JustifyCenter,
						Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: t.line}},
						Children: []core.Node{core.Text{ContentFunc: func() string { return fmt.Sprintf("Item %d", r.Index()+1) }}}}
				}},
			t.live(func() string {
				if loading.Get() {
					return "Loading…"
				}
				return fmt.Sprintf("%d pages of 20", pages.Get())
			}),
		),
		t.demo("Virtualized list", fmt.Sprintf("core.List mounts only the rows near its viewport, %d rows in reused cells.", rows),
			flow(
				t.plain("Top", func() { list.ScrollToIndex(0, true) }),
				t.plain("Row 5,000", func() { list.ScrollToIndex(4999, true) }),
				t.plain("Bottom", func() { list.ScrollToIndex(rows-1, true) }),
			),
			core.List{Ref: list, TestID: "rows", Count: func() int { return rows }, EstimatedSize: 44,
				OnScroll: func(e core.ScrollEvent) { visible.Set(int(e.Offset.Y / 44)) },
				Style:    core.Style{Height: core.Pt(320), BorderWidth: 1, BorderColor: t.line, CornerRadius: radius, Overflow: core.Hidden},
				Row: func(cx *core.Scope, r core.ListRow) core.Node {
					return core.View{
						StyleFunc: func() core.Style {
							s := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Height: core.Pt(44),
								Padding: core.Symmetric(0, 14), Background: t.surface}
							if r.Index()%2 == 1 {
								s.Background = t.sunken
							}
							return s
						},
						Children: []core.Node{
							core.View{StyleFunc: func() core.Style {
								return core.Style{Width: core.Pt(10), Height: core.Pt(10), CornerRadius: 5,
									Background: []core.Color{t.accent, t.success, t.warning, t.danger}[r.Index()%4]}
							}},
							core.Text{ContentFunc: func() string { return fmt.Sprintf("Row %d", r.Index()+1) }, Style: core.Style{FlexGrow: 1}},
							core.Text{ContentFunc: func() string { return fmt.Sprintf("%d KB", 12+r.Index()*37%900) }, Style: t.caption()},
						},
					}
				}},
			t.live(func() string { return fmt.Sprintf("Near row %d", visible.Get()+1) }),
		),
		t.demo("Keyed list", "core.Each keeps a row per key: rows that stay are moved, not rebuilt.",
			flow(t.primary("Add", add), t.plain("Reverse", reverse)),
			core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
				core.Each(fruits.Get, func(f fruit) int { return f.ID },
					func(cx *core.Scope, f func() fruit, index func() int) core.Node {
						return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Symmetric(8, 12),
							Background: t.sunken, CornerRadius: radius}, TestID: "fruit",
							Children: []core.Node{
								core.Text{ContentFunc: func() string { return fmt.Sprintf("%d.", index()+1) }, Style: t.caption()},
								core.Text{ContentFunc: func() string { return fmt.Sprintf("%s #%d", f().Name, f().ID) }, Style: core.Style{FlexGrow: 1}},
								t.plain("Remove", func() { remove(f().ID) }),
							}}
					}),
				core.Show{When: func() bool { return len(fruits.Get()) == 0 }, Then: func(*core.Scope) core.Node {
					return t.note("Nothing left. Add a fruit.")
				}},
			}},
			t.live(func() string { return fmt.Sprintf("%d fruits", len(fruits.Get())) }),
		),
	}, listDemos(cx, t)...)}
}
