package main

import (
	"fmt"
	"time"

	"github.com/oneo-me/nsgo/core"
)

type photo struct {
	id            int
	title, author string
	tone          tone
	views         int
}

var (
	photoAuthors = []string{"Ada", "Linus", "Grace", "Ken", "Barbara"}
	photoTones   = []tone{toneAccent, toneInfo, toneWarn, toneUp, toneDanger}
	photoPlaces  = []string{"Harbor", "Ridge", "Old town", "Dunes", "Canal", "Orchard", "Quarry", "Pier", "Meadow"}
)

// photoPage is how many photos a page loads.
const photoPage = 200

func makePhotos(from, n int) []photo {
	out := make([]photo, n)
	for i := range out {
		id := from + i
		out[i] = photo{id, fmt.Sprintf("%s %04d", photoPlaces[id%len(photoPlaces)], id), photoAuthors[id%len(photoAuthors)],
			photoTones[id%len(photoTones)], (id*7919)%9800 + 120}
	}
	return out
}

// compact is 1234 as "1.2k".
func compact(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return itoa(n)
}

// photoRow is a photo in the phone layout's list.
func photoRow(cx *core.Scope, p func() photo) core.Node {
	colors := colorsOf(cx)
	fin := finOf(cx)
	return press(pressProps{scale: 0.985, label: "Photo"},
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(0, 16), Height: core.Pt(76)}, Children: []core.Node{
			core.Image{SourceFunc: func() core.ImageSource { return core.Src(fmt.Sprintf("https://picsum.photos/seed/%d/112/112", p().id)) },
				Style: fin.raised.over(core.Style{Width: core.Pt(56), Height: core.Pt(56), CornerRadius: radiusInset, BorderCurve: core.BorderCurveContinuous,
					BorderWidth: 1, BorderColor: colors.border, Background: colors.raised})},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 4, AlignSelf: core.AlignStretch, JustifyContent: core.JustifyCenter,
				Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: colors.line}}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return p().title }, MaxLines: 1, Style: core.Style{FontSize: 15, FontWeight: semibold, Color: colors.text}},
				photoByline(cx, p, 13),
			}},
			textf(func() string { return fmt.Sprintf("#%d", p().id) }, 13, regular, colors.faint),
		}})
}

// photoByline is a photo's tone dot, author and views.
func photoByline(cx *core.Scope, p func() photo, size core.Points) core.Node {
	colors := colorsOf(cx)
	return hrow(6,
		core.View{StyleFunc: func() core.Style {
			return core.Style{Width: core.Pt(6), Height: core.Pt(6), CornerRadius: 3, Background: p().tone.color(cx)}
		}},
		core.Text{ContentFunc: func() string { return p().author }, Style: core.Style{FontSize: size, Color: colors.muted}},
		core.Text{ContentFunc: func() string { return fmt.Sprintf("· %s views", compact(p().views)) }, Style: core.Style{FontSize: size, Color: colors.faint}},
	)
}

// photoTile is a photo in the desktop layout's grid: the picture over its
// title and author, a fill under the pointer.
func photoTile(cx *core.Scope, p func() photo) core.Node {
	colors := colorsOf(cx)
	fin := finOf(cx)
	return press(pressProps{scale: 0.97, label: "Photo"},
		core.View{Style: core.Style{Padding: core.All(6), Gap: 8, CornerRadius: radiusCard, BorderCurve: core.BorderCurveContinuous, Cursor: core.CursorPointer,
			Hover: &core.Style{Background: colors.surface}}, Children: []core.Node{
			core.Image{SourceFunc: func() core.ImageSource { return core.Src(fmt.Sprintf("https://picsum.photos/seed/%d/480/360", p().id)) },
				Style: fin.raised.over(core.Style{Width: core.Pct(100), AspectRatio: 4.0 / 3, CornerRadius: radiusInset, BorderCurve: core.BorderCurveContinuous,
					BorderWidth: 1, BorderColor: colors.border, Background: colors.raised})},
			core.View{Style: core.Style{Gap: 3, Padding: core.Edges{Right: 4, Bottom: 4, Left: 4}}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8}, Children: []core.Node{
					core.Text{ContentFunc: func() string { return p().title }, MaxLines: 1, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 14, FontWeight: semibold, Color: colors.text}},
					textf(func() string { return fmt.Sprintf("#%d", p().id) }, 12, regular, colors.faint),
				}},
				photoByline(cx, p, 12),
			}},
		}})
}

// photosDemo is the stream: rows on phones, a grid of as many columns as
// fit (at least 200 pt each) in the desktop layout, a list that recycles
// its cells, pulls to refresh and loads pages as it nears its end.
func photosDemo(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	fin := finOf(cx)
	wide := shell.desktop()
	photos := core.NewSignalFunc(cx, makePhotos(1, 1000), func(a, b []photo) bool { return false })
	refreshing := core.NewSignal(cx, false)
	refreshes := core.NewSignal(cx, 0)
	// The scroll offset, fed by the platform: the header's parallax, its
	// fade and the compact bar run with no engine work per frame.
	scrollY := core.NewAnimatedValue(cx, 0)
	refresh := func() {
		if refreshing.GetUntracked() {
			return
		}
		refreshing.Set(true)
		cx.AfterFunc(800*time.Millisecond, func() {
			photos.Set(makePhotos(1, 1000))
			refreshes.Update(func(n int) int { return n + 1 })
			refreshing.Set(false)
		})
	}
	refreshKey := bindRefresh(cx, refresh)
	loadMore := func() {
		current := photos.GetUntracked()
		photos.Set(append(current[:len(current):len(current)], makePhotos(len(current)+1, photoPage)...))
	}
	desktopIdiom := cx.Env().Desktop()

	hero := func() core.Node {
		pad := core.Edges{Top: 8, Right: 16, Bottom: 16, Left: 16}
		if wide {
			pad = core.Edges{Bottom: 16}
		}
		hintText := fmt.Sprintf("Pull to refresh. Scrolling to the end loads %d more.", photoPage)
		if desktopIdiom {
			hintText = fmt.Sprintf("Scrolling to the end loads %d more.", photoPage)
			if refreshKey != "" {
				hintText = refreshKey + " refreshes. " + hintText
			}
		}
		foot := []core.Node{icon(iconArrowDown, 14, colors.faint), core.Text{Content: hintText, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 13, Color: colors.faint}}}
		// The mouse does not pull scroll views down: desktops refresh here.
		if desktopIdiom {
			foot = append(foot, core.Dynamic(func(*core.Scope) core.Node {
				label := "Refresh"
				if refreshing.Get() {
					label = "Refreshing…"
				}
				return button(buttonProps{label: label, variant: secondary, disabled: refreshing.Get}, refresh)
			}))
		}
		return core.View{Style: core.Style{Padding: pad, Animated: &core.AnimatedStyle{
			Opacity: core.Interpolate(scrollY, core.Interpolation{Input: []float64{0, 150}, Output: []float64{1, 0}, Extrapolate: core.ExtrapolateClamp}),
			// Moves at half speed: the list slides over it.
			TranslateY: core.Interpolate(scrollY, core.Interpolation{Input: []float64{-200, 0, 300}, Output: []float64{0, 0, 150}}),
		}}, Children: []core.Node{
			core.View{Style: cardStyle(cx).Merge(core.Style{Padding: core.All(16), Gap: 12}), Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
					core.View{Style: fin.raised.over(core.Style{Width: core.Pt(40), Height: core.Pt(40), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous,
						Background: colors.raised, BorderWidth: 1, BorderColor: colors.border, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}),
						Children: []core.Node{logo(cx, 24)}},
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
						text("Photo stream", 17, semibold, colors.text), text("A recycled list of remote images", 13, regular, colors.muted)}},
					badge(cx, "Remote", toneInfo),
				}},
				core.Text{Spans: []core.Span{
					{ContentFunc: func() string { return thousands(len(photos.Get())) }},
					{Content: " photos", Style: core.Style{FontSize: 15, FontWeight: medium, Color: colors.accent}},
				}, Style: core.Style{FontSize: 40, FontWeight: semibold, Color: colors.text, LetterSpacing: -1}},
				core.View{Style: core.Style{Direction: core.Row, Borders: core.Edges{Top: 1}, BorderColors: core.EdgeColors{Top: colors.line}, Padding: core.Edges{Top: 10}}, Children: []core.Node{
					stat(cx, "Pages", func() string { return itoa((len(photos.Get()) + photoPage - 1) / photoPage) }, toneNeutral, core.Pct(33)),
					stat(cx, "Per page", label(itoa(photoPage)), toneNeutral, core.Pct(33)),
					core.Dynamic(func(*core.Scope) core.Node {
						t := toneNeutral
						if refreshes.Get() > 0 {
							t = toneAccent
						}
						return stat(cx, "Refreshes", func() string { return itoa(refreshes.Get()) }, t, core.Pct(33))
					}),
				}},
			}},
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Edges{Top: 12}}, Children: foot},
		}}
	}
	footer := func() core.Node {
		return core.Text{Content: fmt.Sprintf("Loading %d more…", photoPage), Style: core.Style{TextAlign: core.TextAlignCenter, FontSize: 13, Color: colors.faint, Padding: core.All(16)}}
	}
	at := func(r core.ListRow) func() photo {
		return func() photo {
			ps := photos.Get()
			if i := r.Index(); i < len(ps) {
				return ps[i]
			}
			return photo{}
		}
	}
	list := core.List{TestID: "photos", Count: func() int { return len(photos.Get()) }, Key: func(i int) any { return photos.GetUntracked()[i].id },
		Style: core.Style{FlexGrow: 1}, OffsetValues: core.AnimatedXY{Y: scrollY}, Refreshing: refreshing.Get, OnRefresh: refresh, OnLoadMore: loadMore}
	var body core.Node
	if !wide {
		list.EstimatedSize = 76
		list.Style.Padding = core.Edges{Bottom: 24}
		list.Header, list.Footer = hero(), footer()
		list.Row = func(cx *core.Scope, r core.ListRow) core.Node { return photoRow(cx, at(r)) }
		body = list
	} else {
		columns := core.NewMemo(cx, func() int { return columnsFor(200, 8, shell.paneWidth(), 16) })
		// Built again when the number of columns changes.
		body = core.Dynamic(func(cx *core.Scope) core.Node {
			n := columns.Get()
			return core.Func(func(cx *core.Scope) core.Node {
				l := list
				l.NumColumns = n
				l.EstimatedSize = core.Untracked(cx, func() core.Points { return (shell.paneWidth()-48)/core.Points(n)*0.75 + 60 })
				l.Style.Padding, l.Gap, l.ColumnGap = core.All(24), 16, 16
				l.Header, l.Footer = hero(), footer()
				l.Row = func(cx *core.Scope, r core.ListRow) core.Node { return photoTile(cx, at(r)) }
				return l
			})
		})
	}
	return core.View{Style: fillPage.Merge(core.Style{Background: colors.background}), Children: []core.Node{
		body,
		// The compact bar fades in once the hero has scrolled away.
		core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Height: core.Pt(44),
			Padding: core.Symmetric(0, 16), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Background: colors.background,
			Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: colors.border}, Opacity: core.Alpha(0.001),
			Animated: &core.AnimatedStyle{
				Opacity:    core.Interpolate(scrollY, core.Interpolation{Input: []float64{140, 190}, Output: []float64{0, 1}, Extrapolate: core.ExtrapolateClamp}),
				TranslateY: core.Interpolate(scrollY, core.Interpolation{Input: []float64{140, 190}, Output: []float64{-8, 0}, Extrapolate: core.ExtrapolateClamp}),
			}}, Children: []core.Node{
			core.Text{Content: "Photo stream", Style: core.Style{FlexGrow: 1, FontSize: 15, FontWeight: semibold, Color: colors.text}},
			textf(func() string { return thousands(len(photos.Get())) + " photos" }, 13, regular, colors.muted),
		}},
	}}
}
