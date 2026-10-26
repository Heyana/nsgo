package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

var listTones = []tone{toneAccent, toneInfo, toneWarn, toneUp, toneDanger, toneNeutral}

// toneNames are the tones as words, for badges.
var toneNames = map[tone]string{toneNeutral: "neutral", toneAccent: "accent", toneInfo: "info", toneWarn: "warn", toneDanger: "danger", toneUp: "up", toneDown: "down"}

// action is a small pill action for list headers (36 pt, 44 pt with its
// hit slop).
func action(label string, name iconName, onPress func()) core.Node {
	children := []core.Node{}
	if name != "" {
		children = append(children, icon(name, 14, colors.text))
	}
	children = append(children, text(label, 13, semibold, colors.text))
	return press(pressProps{onPress: onPress, label: label, hitSlop: 4},
		core.View{Style: fin.raised.over(core.Style{Height: core.Pt(36), Padding: core.Symmetric(0, 14), CornerRadius: 18, Direction: core.Row, AlignItems: core.AlignCenter,
			Gap: 6, Background: colors.raised, BorderWidth: 1, BorderColor: colors.border}), Children: children})
}

type gridItem struct {
	id     int
	height core.Points
	tone   tone
}

// listGrid is columns (as many as fit in the desktop layout, at least
// 130 pt each), a header and a footer, padding and gaps, viewability.
func listGrid(cx *core.Scope) core.Node {
	items := make([]gridItem, 90)
	for i := range items {
		items[i] = gridItem{i, 84 + core.Points(i%4)*22, listTones[i%len(listTones)]}
	}
	first, last := core.NewSignal(cx, -1), core.NewSignal(cx, -1)
	wide := shell.desktop()
	grid := func(columns int) core.Node {
		ref := &core.Ref{}
		pad, gap := core.Points(16), core.Points(8)
		if wide {
			pad, gap = 24, 12
		}
		return core.List{Ref: ref, TestID: "grid", Count: func() int { return len(items) }, NumColumns: columns, EstimatedSize: 110,
			Style: core.Style{FlexGrow: 1, Padding: core.All(pad)}, Gap: gap, ColumnGap: gap,
			ViewabilityConfig: core.ViewabilityConfig{ItemVisiblePercentThreshold: 50},
			OnViewableItemsChange: func(v core.ViewableItems) {
				if len(v.Viewable) == 0 {
					first.Set(-1)
					last.Set(-1)
					return
				}
				first.Set(v.Viewable[0].Index)
				last.Set(v.Viewable[len(v.Viewable)-1].Index)
			},
			Header: core.View{Style: cardStyle().Merge(core.Style{Padding: core.All(16), Gap: 12, Margin: core.Edges{Bottom: 4}}), Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignEnd, Gap: 8}, Children: []core.Node{
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
						text("Viewable items (at least half shown)", 13, regular, colors.muted),
						core.Text{Spans: []core.Span{
							{ContentFunc: func() string {
								if first.Get() < 0 {
									return "—"
								}
								return fmt.Sprintf("%d–%d", first.Get(), last.Get())
							}},
							{ContentFunc: func() string {
								if first.Get() < 0 {
									return ""
								}
								return fmt.Sprintf("  %d visible", last.Get()-first.Get()+1)
							}, Style: core.Style{FontSize: 15, FontWeight: medium, Color: colors.accent}},
						}, Style: core.Style{FontSize: 30, FontWeight: semibold, Color: colors.text, LetterSpacing: -0.5}},
					}},
					core.Text{Content: fmt.Sprintf("90 items, %d columns", columns), Style: core.Style{FontSize: 13, Color: colors.faint, Padding: core.Edges{Bottom: 6}}},
				}},
				meter(func() float64 {
					if last.Get() < 0 {
						return 0
					}
					return float64(last.Get()+1) / float64(len(items))
				}, toneAccent, -1),
				core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
					action("Jump to 60", iconArrowDown, func() { ref.ScrollToIndexAt(60, 0.5, 0, true) }),
					action("End", "", func() { ref.ScrollToEnd(true) }),
				}},
			}},
			Footer: core.Text{Content: "End of the list", Style: core.Style{TextAlign: core.TextAlignCenter, Color: colors.faint, FontSize: 13, Padding: core.All(16)}},
			Row: func(cx *core.Scope, r core.ListRow) core.Node {
				item := func() gridItem { return items[r.Index()] }
				return core.View{StyleFunc: func() core.Style {
					border := colors.border
					if item().id == 60 {
						border = colors.accent
					}
					return fin.raised.over(core.Style{Height: core.Pt(item().height), CornerRadius: radiusInset, BorderCurve: core.BorderCurveContinuous,
						BorderWidth: 1, BorderColor: border, Background: colors.raised, Padding: core.All(12), JustifyContent: core.JustifySpaceBetween})
				}, Children: []core.Node{
					core.View{StyleFunc: func() core.Style {
						return core.Style{Width: core.Pt(18), Height: core.Pt(4), CornerRadius: 2, Background: item().tone.color()}
					}},
					core.View{Children: []core.Node{
						textf(func() string { return itoa(item().id) }, 22, semibold, colors.text),
						textf(func() string { return fmt.Sprintf("%.0f pt", item().height) }, 12, regular, colors.faint),
					}},
				}}
			}}
	}
	if !wide {
		return grid(3)
	}
	// Built again when the number of columns changes.
	columns := core.NewMemo(cx, func() int { return columnsFor(130, 10, shell.paneWidth(), 12) })
	return core.View{Style: fillPage, Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
		n := columns.Get()
		return core.Func(func(*core.Scope) core.Node { return grid(n) })
	})}}
}

var people = [][3]string{
	{"Ada", "Lovelace", "Analytical Engine notes"},
	{"Alan", "Kay", "Smalltalk"},
	{"Barbara", "Liskov", "CLU, substitution"},
	{"Brian", "Kernighan", "AWK, The C book"},
	{"Claude", "Shannon", "Information theory"},
	{"Dennis", "Ritchie", "C, Unix"},
	{"Donald", "Knuth", "TeX, TAOCP"},
	{"Edsger", "Dijkstra", "Shortest paths"},
	{"Frances", "Allen", "Optimizing compilers"},
	{"Grace", "Hopper", "COBOL, the first compiler"},
	{"Guido", "van Rossum", "Python"},
	{"Ken", "Thompson", "Unix, UTF-8"},
	{"Linus", "Torvalds", "Linux, Git"},
	{"Margaret", "Hamilton", "Apollo flight software"},
	{"Niklaus", "Wirth", "Pascal, Oberon"},
}

// contactRow is a header or a person of the contacts list.
type contactRow struct {
	header            string
	count             int
	first, last, note string
	tone              tone
	end               bool
}

// listContacts is section headers that stick to the top until the next
// one pushes them out.
func listContacts(cx *core.Scope) core.Node {
	// One list of header and name rows; the headers' indices stick.
	var rows []contactRow
	var headers []int
	for i := 0; i < len(people); {
		letter := people[i][0][:1]
		j := i
		for j < len(people) && people[j][0][:1] == letter {
			j++
		}
		headers = append(headers, len(rows))
		rows = append(rows, contactRow{header: letter, count: j - i})
		for k, p := range people[i:j] {
			rows = append(rows, contactRow{first: p[0], last: p[1], note: p[2], tone: listTones[(len(rows)+k)%(len(listTones)-1)], end: i+k == j-1})
		}
		i = j
	}
	return core.List{TestID: "contacts", Count: func() int { return len(rows) }, StickyHeaderIndices: headers, EstimatedSize: 60,
		Style: core.Style{FlexGrow: 1, Padding: core.Edges{Bottom: 24}},
		Row: func(cx *core.Scope, r core.ListRow) core.Node {
			row := func() contactRow { return rows[r.Index()] }
			return core.Show{When: func() bool { return row().header != "" },
				Then: func(*core.Scope) core.Node {
					return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Height: core.Pt(34), Padding: core.Symmetric(0, 16), Gap: 10,
						// Opaque: rows scroll under it while it sticks.
						Background: colors.background, Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: colors.border}}, Children: []core.Node{
						textf(func() string { return row().header }, 13, bold, colors.accent),
						core.View{Style: core.Style{FlexGrow: 1, Height: core.Pt(1), Background: colors.line}},
						textf(func() string { return itoa(row().count) }, 13, regular, colors.faint),
					}}
				},
				Else: func(*core.Scope) core.Node {
					return core.View{OnPress: func(core.PressEvent) {}, Role: core.RoleButton,
						Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Edges{Left: 16}, Height: core.Pt(60), Active: &core.Style{Background: colors.raised}},
						Children: []core.Node{
							core.View{StyleFunc: func() core.Style {
								return core.Style{Width: core.Pt(38), Height: core.Pt(38), CornerRadius: 19, Background: row().tone.tint(), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}
							}, Children: []core.Node{core.Text{ContentFunc: func() string { return row().first[:1] + row().last[:1] }, StyleFunc: func() core.Style {
								return core.Style{FontSize: 14, FontWeight: semibold, Color: row().tone.color()}
							}}}},
							core.View{StyleFunc: func() core.Style {
								s := core.Style{FlexGrow: 1, FlexShrink: 1, AlignSelf: core.AlignStretch, JustifyContent: core.JustifyCenter, Gap: 2, Padding: core.Edges{Right: 16}}
								if !row().end {
									s.Borders, s.BorderColors = core.Edges{Bottom: 1}, core.EdgeColors{Bottom: colors.line}
								}
								return s
							}, Children: []core.Node{
								core.Text{Spans: []core.Span{{ContentFunc: func() string { return row().first + " " }}, {ContentFunc: func() string { return row().last }, Style: core.Style{FontWeight: semibold}}},
									Style: core.Style{FontSize: 15, Color: colors.text}},
								core.Text{ContentFunc: func() string { return row().note }, MaxLines: 1, Style: core.Style{FontSize: 13, Color: colors.muted}},
							}},
						}}
				}}
		}}
}

type chatLine struct {
	id         int
	text, time string
	mine       bool
}

var chatLines = []string{
	"Did the build go out?",
	"Yes, 2.4 is live on both stores",
	"Crash rate?",
	"Flat. 0.02 %",
	"Nice. The list scrolls so much smoother now",
	"Recycling plus native layout, no engine work per frame",
	"Ship it",
}

// listChat is an inverted list: it starts at the newest message and keeps
// its place as older ones load.
func listChat(cx *core.Scope) core.Node {
	env := cx.Env()
	next := 0
	stamp := func(n int) string { return fmt.Sprintf("%02d:%02d", 9+(n/60)%12, n%60) }
	message := func(text string, mine bool) chatLine {
		m := chatLine{next, text, stamp(next), mine}
		next++
		return m
	}
	var initial []chatLine
	for i := range 30 {
		initial = append(initial, message(chatLines[i%len(chatLines)], i%3 == 0))
	}
	messages := core.NewSignalFunc(cx, initial, func(a, b []chatLine) bool { return false })
	draft := core.NewSignal(cx, "")
	older := core.NewSignal(cx, 0)
	send := func() {
		d := draft.GetUntracked()
		if strings.TrimSpace(d) == "" {
			return
		}
		cx.AnimateLayout(core.LayoutSpringPreset)
		messages.Update(func(m []chatLine) []chatLine { return append([]chatLine{message(d, true)}, m...) })
		draft.Set("")
	}
	loadOlder := func() {
		// Ten pages of history are all the room has.
		if older.GetUntracked() >= 10 {
			return
		}
		n := len(messages.GetUntracked())
		older.Update(func(o int) int { return o + 1 })
		messages.Update(func(m []chatLine) []chatLine {
			m = slices.Clone(m)
			for i := range 20 {
				m = append(m, message(fmt.Sprintf("Older message %d", n+i+1), i%4 == 0))
			}
			return m
		})
	}
	return core.View{Style: fillPage.Merge(core.Style{PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard}}), Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Edges{Right: 16, Bottom: 8, Left: 16}}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(8), Height: core.Pt(8), CornerRadius: 4, Background: colors.up}},
			core.Text{Content: "Release room", Style: core.Style{FlexGrow: 1, FontSize: 13, Color: colors.muted}},
			textf(func() string { return fmt.Sprintf("%d messages, %d older pages", len(messages.Get()), older.Get()) }, 13, regular, colors.faint),
		}},
		core.List{TestID: "chat", Count: func() int { return len(messages.Get()) }, Key: func(i int) any { return messages.GetUntracked()[i].id },
			Inverted: true, EstimatedSize: 52, Style: core.Style{FlexGrow: 1, Padding: core.Symmetric(12, 16)}, Gap: 6,
			Behavior: core.ScrollBehavior{KeyboardDismissMode: core.KeyboardDismissInteractive}, OnLoadMore: loadOlder, LoadMoreThreshold: 0.5,
			Row: func(cx *core.Scope, r core.ListRow) core.Node {
				m := func() chatLine { return messages.Get()[r.Index()] }
				return core.View{StyleFunc: func() core.Style {
					a := core.AlignStart
					if m().mine {
						a = core.AlignEnd
					}
					return core.Style{AlignSelf: a, MaxWidth: core.Pct(78), Gap: 3, AlignItems: a}
				}, Children: []core.Node{
					core.View{StyleFunc: func() core.Style {
						s := core.Style{Padding: core.Symmetric(9, 14), BorderCurve: core.BorderCurveContinuous}
						if m().mine {
							s = fin.accent.over(s)
							s.Background, s.CornerRadii = colors.accentFill, core.Corners{TopLeft: 18, TopRight: 18, BottomRight: 6, BottomLeft: 18}
						} else {
							s = fin.raised.over(s)
							s.Background, s.CornerRadii, s.BorderWidth, s.BorderColor = colors.raised, core.Corners{TopLeft: 18, TopRight: 18, BottomRight: 18, BottomLeft: 6}, 1, colors.border
						}
						return s
					}, Children: []core.Node{core.Text{ContentFunc: func() string { return m().text }, StyleFunc: func() core.Style {
						c := colors.text
						if m().mine {
							c = colors.onAccent
						}
						return core.Style{Color: c, FontSize: 15}
					}}}},
					core.Text{ContentFunc: func() string { return m().time }, Style: core.Style{FontSize: 11, Color: colors.faint, Padding: core.Symmetric(0, 4)}},
				}}
			}},
		core.View{StyleFunc: func() core.Style {
			bottom := core.Points(8)
			if env.KeyboardHeight() == 0 {
				bottom += env.SafeArea().Bottom
			}
			return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Edges{Top: 8, Right: 16, Bottom: bottom, Left: 16},
				Background: colors.surface, Borders: core.Edges{Top: 1}, BorderColors: core.EdgeColors{Top: colors.border}}
		}, Children: []core.Node{
			base.Input{Value: draft.Get, OnChange: draft.Set, OnSubmit: func(string) { send() }, SubmitBehavior: core.SubmitBehaviorSubmit, Placeholder: "Message",
				ReturnKeyType: core.ReturnKeySend, TestID: "chat-draft",
				Style: fin.field.over(core.Style{PlaceholderColor: colors.faint, SelectionColor: colors.accent, FlexGrow: 1, FlexShrink: 1, Height: core.Pt(44), FontSize: 15,
					Color: colors.text, Background: colors.field, BorderWidth: 1, BorderColor: colors.fieldBorder, CornerRadius: 22, Padding: core.Symmetric(0, 16)})},
			roundButton(iconArrowUp, "Send", primary, 0, send),
		}},
	}}
}

var ranges = []string{"Today", "Yesterday", "This week", "This month", "This year", "All time", "Custom"}

type listEntry struct {
	n    int
	tone tone
}

// listMore is horizontal scrolling, pull to refresh on a plain scroll
// view, a sticky heading and an empty list.
func listMore(cx *core.Scope) core.Node {
	refreshing := core.NewSignal(cx, false)
	custom := core.NewSignal(cx, false)
	pull := core.NewAnimatedValue(cx, 0)
	refreshed := core.NewSignal(cx, 0)
	rng := core.NewSignal(cx, "This week")
	items := core.NewSignalFunc(cx, []listEntry(nil), slices.Equal)
	refresh := func() {
		refreshing.Set(true)
		cx.AfterFunc(time.Second, func() {
			refreshed.Update(func(n int) int { return n + 1 })
			refreshing.Set(false)
		})
	}
	refreshKey := bindRefresh(cx, refresh)
	edit := func(change func()) {
		cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)
		change()
	}
	codeSpan := func(s string) core.Span {
		return core.Span{Content: s, Style: core.Style{FontFamily: mono, Color: colors.text}}
	}
	var chips []core.Node
	for _, l := range ranges {
		on := func() bool { return rng.Get() == l }
		chips = append(chips, base.Tab{Selected: on, OnPress: func(core.PressEvent) { rng.Set(l) }, Label: l,
			Style: core.Style{Margin: core.All(-4), Padding: core.All(4), Transition: core.Transition{Duration: 150e6, Easing: core.EaseOut, Properties: core.TransitionTransform},
				Active: &core.Style{Transform: core.Transform{Scale: 0.97}}},
			Children: []core.Node{core.View{StyleFunc: func() core.Style {
				s := core.Style{Height: core.Pt(36), Padding: core.Symmetric(0, 14), CornerRadius: 18, JustifyContent: core.JustifyCenter}
				if on() {
					s = fin.light.over(s)
					s.Background = colors.light
				} else {
					s = fin.raised.over(s)
					s.Background, s.BorderWidth, s.BorderColor = colors.raised, 1, colors.border
				}
				return s
			}, Children: []core.Node{core.Text{Content: l, StyleFunc: func() core.Style {
				c := colors.muted
				if on() {
					c = colors.onLight
				}
				return core.Style{FontSize: 13, FontWeight: semibold, Color: c}
			}}}}}})
	}
	pullChildren := []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
			core.Dynamic(func(*core.Scope) core.Node {
				if refreshing.Get() {
					return core.View{Style: fin.raised.over(core.Style{Width: core.Pt(40), Height: core.Pt(40), CornerRadius: 12, BorderWidth: 1, BorderColor: colors.border,
						Background: colors.raised, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}), Children: []core.Node{icon(iconArrowDown, 20, colors.accent)}}
				}
				return iconTile(iconArrowDown, 40)
			}),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
				textf(func() string {
					if refreshing.Get() {
						return "Refreshing…"
					}
					if refreshKey != "" {
						return hint(cx, "Pull the page down", "Refresh or "+refreshKey)
					}
					return hint(cx, "Pull the page down", "Refresh below")
				}, 15, regular, colors.text),
				textf(func() string {
					if custom.Get() {
						return "An indicator the app draws, from PullValue"
					}
					return "The system spinner on a plain scroll view"
				}, 13, regular, colors.muted),
			}},
			core.View{Style: core.Style{AlignItems: core.AlignEnd}, Children: []core.Node{
				textf(func() string { return itoa(refreshed.Get()) }, 28, semibold, colors.text),
				text("refreshes", 12, regular, colors.faint),
			}},
		}},
	}
	pullChildren = append(pullChildren, segmented([]option[bool]{{value: false, label: "System spinner"}, {value: true, label: "Custom indicator"}},
		custom.Get, custom.Set, false))
	// The mouse does not pull scroll views down: desktops refresh here.
	if cx.Env().Desktop() {
		pullChildren = append(pullChildren, button(buttonProps{label: "Refresh", variant: secondary, disabled: refreshing.Get}, refresh))
	}
	page := core.Scroll{TestID: "more", Style: core.Style{FlexGrow: 1}, Refreshing: refreshing.Get, OnRefresh: refresh, PullValue: pull,
		BehaviorFunc: func() core.ScrollBehavior {
			if custom.Get() {
				return core.ScrollBehavior{RefreshIndicator: pullIndicator}
			}
			return core.ScrollBehavior{}
		}, Children: []core.Node{
			core.View{Style: core.Style{Padding: core.All(16), Gap: 16}, Children: []core.Node{
				// Sticky: in the flow, then at the top while the page scrolls
				// under it.
				core.View{Style: core.Style{Position: core.Sticky, Top: core.Pt(0), Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8,
					Margin: core.Edges{Left: -16, Right: -16, Top: -8}, Padding: core.Symmetric(8, 16), Background: colors.background,
					Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: colors.border}}, Children: []core.Node{
					core.Text{Content: "More list features", Style: core.Style{FlexGrow: 1, FontSize: 15, FontWeight: semibold, Color: colors.text}},
					core.Text{Content: "Position: Sticky", Style: core.Style{FontSize: 13, Color: colors.faint, FontFamily: mono}},
				}},
				section("Pull to refresh", "", pullChildren...),
				sectionf("Horizontal scroll view", rng.Get, core.View{Style: core.Style{Margin: core.Edges{Left: -14, Right: -14}}, Children: []core.Node{
					core.Scroll{Horizontal: true, HideIndicator: true, Children: []core.Node{
						base.Tabs{Label: "Range", Style: core.Style{Direction: core.Row, Gap: 8, Padding: core.Symmetric(4, 14)}, Children: chips},
					}},
				}}),
				section("Horizontal list", "1,000 recycled cards", core.View{Style: core.Style{Margin: core.Edges{Left: -14, Right: -14}}, Children: []core.Node{
					core.List{Count: func() int { return 1000 }, Horizontal: true, EstimatedSize: 130, HideIndicator: true,
						Style: core.Style{Height: core.Pt(124), Padding: core.Symmetric(0, 14)}, Gap: 10,
						Row: func(cx *core.Scope, r core.ListRow) core.Node {
							t := func() tone { return listTones[r.Index()%(len(listTones)-1)] }
							load := func() float64 { return float64((r.Index()*37)%100) / 100 }
							return core.View{Style: fin.raised.over(core.Style{Width: core.Pt(120), Height: core.Pt(124), CornerRadius: radiusInset, BorderCurve: core.BorderCurveContinuous,
								Background: colors.raised, BorderWidth: 1, BorderColor: colors.border, Padding: core.All(12), JustifyContent: core.JustifySpaceBetween}), Children: []core.Node{
								textf(func() string { return fmt.Sprintf("Card %d", r.Index()+1) }, 12, regular, colors.muted),
								core.Text{Spans: []core.Span{
									{ContentFunc: func() string { return itoa(int(load()*100 + 0.5)) }},
									{Content: " %", Style: core.Style{FontSize: 15, FontWeight: medium}},
								}, Style: core.Style{FontSize: 26, FontWeight: semibold, Color: colors.text}},
								core.Dynamic(func(*core.Scope) core.Node { return meter(load, t(), -1) }),
							}}
						}},
				}}),
				sectionf("An empty list", func() string { return count(len(items.Get()), "item") },
					core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
						cell(button(buttonProps{label: "Add", icon: iconPlus}, func() {
							edit(func() {
								items.Update(func(s []listEntry) []listEntry {
									return append(slices.Clone(s), listEntry{len(s) + 1, listTones[len(s)%(len(listTones)-1)]})
								})
							})
						})),
						cell(button(buttonProps{label: "Clear", variant: secondary, disabled: func() bool { return len(items.Get()) == 0 }}, func() { edit(func() { items.Set(nil) }) })),
					}},
					core.List{Viewport: core.ListViewportParent, TestID: "empty-list", Count: func() int { return len(items.Get()) }, EstimatedSize: 48,
						Style: core.Style{CornerRadius: radiusControl, Background: colors.surface, Shadows: fin.card.shadows, BorderWidth: 1, BorderColor: colors.line},
						Empty: core.View{Style: core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 10, Height: core.Pt(194)}, Children: []core.Node{
							iconTile(iconLists, 40),
							text(hint(cx, "Nothing here yet: tap Add", "Nothing here yet: click Add"), 13, regular, colors.muted),
						}},
						Separator: func(*core.Scope) core.Node {
							return core.View{Style: core.Style{Height: core.Pt(1), Margin: core.Edges{Left: 14}, Background: colors.line}}
						},
						Row: func(cx *core.Scope, r core.ListRow) core.Node {
							e := func() listEntry { return items.Get()[r.Index()] }
							return core.View{Style: core.Style{Height: core.Pt(48), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Symmetric(0, 14)}, Children: []core.Node{
								core.View{StyleFunc: func() core.Style {
									return core.Style{Width: core.Pt(8), Height: core.Pt(8), CornerRadius: 2.5, Background: e().tone.color()}
								}},
								core.Text{ContentFunc: func() string { return fmt.Sprintf("Item %d", e().n) }, Style: core.Style{FlexGrow: 1, FontSize: 15, Color: colors.text}},
								core.Dynamic(func(*core.Scope) core.Node { t := e().tone; return badge(toneNames[t], t) }),
							}}
						}},
					core.Text{Spans: []core.Span{
						codeSpan("Empty"), {Content: " while empty, separators between rows; changes animate with "}, codeSpan("AnimateLayout"),
						{Content: ". In the page ("}, codeSpan("ListViewportParent"), {Content: "), the list is as tall as its rows."},
					}, Style: core.Style{FontSize: 13, Color: colors.muted}},
				),
			}},
		}}
	return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
		page,
		core.Show{When: custom.Get, Then: func(cx *core.Scope) core.Node { return customIndicator(cx, pull, refreshing.Get) }},
	}}
}

// pullIndicator is how tall the custom indicator is: where a pull
// refreshes, and the room it keeps while refreshing.
const pullIndicator = 72

// customIndicator is a pull-to-refresh indicator the app draws, in the
// gallery's look, over the top of a scroll view: a round chip that travels
// in with the pull (its PullValue), grows and fades in, turns its arrow
// over once the pull reaches the height where it refreshes, and spins
// while it refreshes. The pull drives all of it on the frame clock.
func customIndicator(cx *core.Scope, pull *core.AnimatedValue, refreshing func() bool) core.Node {
	turn := core.NewAnimatedValue(cx, 0)
	var spin *core.AnimationRun
	cx.Effect(func() {
		if spin != nil {
			spin.Stop()
			spin = nil
		}
		if refreshing() && !cx.Env().ReduceMotion() {
			turn.Set(0)
			spin = cx.StartAnimation(core.Loop(core.Timing(turn, 1, core.TimingConfig{Duration: ms(900), Easing: core.BezierEasing(core.Linear)}), 0), nil)
		}
	})
	lerp := func(in core.AnimatedNode, from, to [2]float64) core.AnimatedNode {
		return core.Interpolate(in, core.Interpolation{Input: from[:], Output: to[:], Extrapolate: core.ExtrapolateClamp})
	}
	shown := func(on bool) core.Style {
		if on {
			return core.Style{Opacity: core.Alpha(1)}
		}
		return core.Style{Opacity: core.Alpha(0)}
	}
	return core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0),
		Height: core.Pt(pullIndicator), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{
		core.View{Style: fin.raised.over(core.Style{Width: core.Pt(40), Height: core.Pt(40), CornerRadius: 20, BorderWidth: 1, BorderColor: colors.border,
			Background: colors.raised, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Animated: &core.AnimatedStyle{
				Opacity:    lerp(pull, [2]float64{0, 24}, [2]float64{0, 1}),
				TranslateY: lerp(pull, [2]float64{0, pullIndicator}, [2]float64{-28, 0}),
				Scale:      lerp(pull, [2]float64{0, pullIndicator}, [2]float64{0.6, 1}),
			}}), Children: []core.Node{
			// The arrow points down while pulling and turns up once
			// releasing refreshes.
			core.View{StyleFunc: func() core.Style {
				return shown(!refreshing()).Merge(core.Style{Position: core.Absolute,
					Animated: &core.AnimatedStyle{Rotate: lerp(pull, [2]float64{pullIndicator * 0.5, pullIndicator}, [2]float64{0, 180})}})
			}, Children: []core.Node{icon(iconArrowDown, 18, colors.accent)}},
			core.View{StyleFunc: func() core.Style {
				return shown(refreshing()).Merge(core.Style{Position: core.Absolute, Width: core.Pt(20), Height: core.Pt(20), CornerRadius: 10,
					BorderWidth: 2, BorderColor: alpha(colors.accent, 0.25), BorderColors: core.EdgeColors{Top: colors.accent},
					Animated: &core.AnimatedStyle{Rotate: lerp(turn, [2]float64{0, 1}, [2]float64{0, 360})}})
			}},
		}},
	}}
}

// pageRow is an entry or a week heading of the page's first list.
type pageRow struct {
	week int
	id   int
	tone tone
}

func pageEntries(from, n int) []pageRow {
	out := make([]pageRow, n)
	for i := range out {
		out[i] = pageRow{id: from + i, tone: listTones[(from+i)%(len(listTones)-1)]}
	}
	return out
}

// entryRow is one row of the page's lists.
func entryRow(e func() pageRow, label string) core.Node {
	return core.View{Style: fin.raised.over(core.Style{Height: core.Pt(56), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(0, 14),
		CornerRadius: radiusInset, BorderCurve: core.BorderCurveContinuous, Background: colors.raised, BorderWidth: 1, BorderColor: colors.border}), Children: []core.Node{
		core.View{StyleFunc: func() core.Style {
			return core.Style{Width: core.Pt(8), Height: core.Pt(8), CornerRadius: 2.5, Background: e().tone.color()}
		}},
		core.Text{ContentFunc: func() string { return fmt.Sprintf("%s %d", label, e().id+1) }, Style: core.Style{FlexGrow: 1, FontSize: 15, Color: colors.text}},
		core.Dynamic(func(*core.Scope) core.Node { t := e().tone; return badge(toneNames[t], t) }),
	}}
}

// pageBar is the height of the page's bar, which hides as the page
// scrolls down.
const pageBar = 44

// listPage is headers and two lists on one page: the lists have no scroll
// view of their own (ListViewportParent), the page scrolls them as one,
// and only their rows near its viewport exist. A bar over the page hides
// as it scrolls down and comes back as it scrolls up (DiffClamp), and the
// first list's week headings stick just below it, wherever it is
// (StickyHeaderOffsetValue set to how much of it shows).
func listPage(cx *core.Scope) core.Node {
	scroll := core.NewAnimatedValue(cx, 0)
	// How much of the bar is hidden, following the page's scrolling.
	hidden := core.DiffClamp(scroll, 0, pageBar)
	// How much of it shows: where the week headings stick.
	shown := core.Add(core.Mul(hidden, core.Constant(-1)), core.Constant(pageBar))
	// 200 entries, a week heading before every 20.
	var recent []pageRow
	var weeks []int
	for _, e := range pageEntries(0, 200) {
		if e.id%20 == 0 {
			weeks = append(weeks, len(recent))
			recent = append(recent, pageRow{week: e.id/20 + 1})
		}
		recent = append(recent, e)
	}
	indexOf := func(id int) int {
		return slices.IndexFunc(recent, func(r pageRow) bool { return r.week == 0 && r.id == id })
	}
	archive := core.NewSignalFunc(cx, pageEntries(0, 100), func(a, b []pageRow) bool { return false })
	ref := &core.Ref{}
	loadMore := func() {
		if a := archive.GetUntracked(); len(a) < 500 {
			archive.Set(append(slices.Clone(a), pageEntries(len(a), 100)...))
		}
	}
	heading := func(title string, noteFn func() string) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8, Padding: core.Edges{Top: 8, Right: 4, Left: 4}}, Children: []core.Node{
			core.Text{Content: title, Style: core.Style{FlexGrow: 1, FontSize: 17, FontWeight: semibold, Color: colors.text}},
			textf(noteFn, 13, regular, colors.faint),
		}}
	}
	page := core.Scroll{TestID: "page", OffsetValues: core.AnimatedXY{Y: scroll}, Style: core.Style{FlexGrow: 1}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Edges{Top: 16 + pageBar, Right: 16, Bottom: 16, Left: 16}, Gap: 10, Width: core.Pct(100), MaxWidth: core.Pt(720),
			AlignSelf: core.AlignCenter}, Children: []core.Node{
			section("One page, two lists", "ListViewportParent",
				text("The lists below lie in the page at their full length, and the page scrolls them with this header: only the rows near its viewport exist, in recycled cells.",
					13, regular, colors.muted),
				core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
					action("Recent 150", iconArrowDown, func() { ref.ScrollToIndexAt(indexOf(149), 0.5, 0, true) }),
					action("End of recent", "", func() { ref.ScrollToEnd(true) }),
				}},
			),
			heading("Recent", label("200 rows")),
			core.List{Ref: ref, Viewport: core.ListViewportParent, Count: func() int { return len(recent) }, EstimatedSize: 56, StickyHeaderIndices: weeks, StickyHeaderOffsetValue: shown, Gap: 6,
				Row: func(cx *core.Scope, r core.ListRow) core.Node {
					row := func() pageRow { return recent[r.Index()] }
					return core.Show{When: func() bool { return row().week > 0 }, Else: func(*core.Scope) core.Node { return entryRow(row, "Recent") },
						// A week's heading: it sticks to the top of the page
						// while its rows scroll under it.
						Then: func(*core.Scope) core.Node {
							return core.View{Style: core.Style{Height: core.Pt(34), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Symmetric(0, 4),
								Background: colors.background}, Children: []core.Node{
								textf(func() string { return fmt.Sprintf("Week %d", row().week) }, 13, bold, colors.accent),
								core.View{Style: core.Style{FlexGrow: 1, Height: core.Pt(1), Background: colors.line}},
							}}
						}}
				}},
			heading("Archive", func() string { return fmt.Sprintf("%d rows", len(archive.Get())) }),
			core.List{Viewport: core.ListViewportParent, Count: func() int { return len(archive.Get()) }, EstimatedSize: 56, Gap: 6, OnLoadMore: loadMore,
				Footer: core.Text{ContentFunc: func() string {
					if len(archive.Get()) < 500 {
						return "Loading more…"
					}
					return "End of the archive"
				}, Style: core.Style{TextAlign: core.TextAlignCenter, Color: colors.faint, FontSize: 13, Padding: core.All(16)}},
				Row: func(cx *core.Scope, r core.ListRow) core.Node {
					return entryRow(func() pageRow { return archive.Get()[r.Index()] }, "Archived")
				}},
		}},
	}}
	bar := core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Height: core.Pt(pageBar),
		Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(0, 20), Background: colors.background,
		Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: colors.border},
		Animated: &core.AnimatedStyle{TranslateY: core.Mul(hidden, core.Constant(-1))}}, Children: []core.Node{
		core.Text{Content: "Recent and archived", Style: core.Style{FlexGrow: 1, FontSize: 15, FontWeight: semibold, Color: colors.text}},
		text("hides as you scroll down", 13, regular, colors.faint),
	}}
	return core.View{Style: core.Style{FlexGrow: 1, Overflow: core.Hidden}, Children: []core.Node{page, bar}}
}

// listsDemo is core.List: columns, sticky sections, an inverted chat, two
// lists in one page, horizontal lists and an empty one.
func listsDemo(cx *core.Scope) core.Node {
	tab := core.NewSignal(cx, "grid")
	head := core.Style{Padding: core.Symmetric(8, 16)}
	if shell.desktop() {
		head = core.Style{Padding: core.Edges{Top: 16, Right: 24, Bottom: 4, Left: 24}, Width: core.Pct(100), MaxWidth: core.Pt(600)}
	}
	views := map[string]func(*core.Scope) core.Node{"grid": listGrid, "sections": listContacts, "chat": listChat, "page": listPage, "more": listMore}
	return core.View{Style: fillPage.Merge(core.Style{Background: colors.background}), Children: []core.Node{
		core.View{Style: head, Children: []core.Node{segmented([]option[string]{
			{value: "grid", label: "Grid"}, {value: "sections", label: "Sections"}, {value: "chat", label: "Chat"}, {value: "page", label: "Page"}, {value: "more", label: "More"},
		}, tab.Get, tab.Set, false)}},
		core.Dynamic(func(cx *core.Scope) core.Node {
			v := views[tab.Get()]
			return appear(0, true, core.Func(v))
		}),
	}}
}
