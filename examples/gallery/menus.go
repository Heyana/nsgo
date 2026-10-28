package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/desktop"
)

// The look of every menu here is the gallery's: base draws none.

// menuLook is the menus' look on the device: popups on desktops, sheets
// at the bottom on touch screens.
type menuLook struct {
	desktop bool
	height  core.Points
}

func newMenuLook(cx *core.Scope) menuLook {
	l := menuLook{desktop: cx.Env().Desktop(), height: 46}
	if l.desktop {
		l.height = 32
	}
	return l
}

// surface is the popup on desktops, the sheet at the bottom on touch
// screens, for a PopupStyleFunc.
func (l menuLook) surface(cx *core.Scope) func() core.Style {
	colors := colorsOf(cx)
	return func() core.Style {
		if l.desktop {
			return finOf(cx).raised().over(core.Style{Background: colors.raised(), CornerRadius: radiusInset, BorderWidth: 1, BorderColor: colors.border(),
				Padding: core.All(6), MinWidth: core.Pt(220)})
		}
		return core.Style{Background: colors.surface(), CornerRadii: core.Corners{TopLeft: 20, TopRight: 20},
			Padding: core.Edges{Top: 10, Right: 10, Bottom: 34, Left: 10}}
	}
}

func (menuLook) backdrop() core.Style { return core.Style{Background: black(0.45)} }

// itemHovered and itemPressed are an item under the pointer and pressed:
// an orange tint with a hairline of it, read in a StyleFunc.
func itemHovered(cx *core.Scope) *core.Style {
	if cx.Dark() {
		return &core.Style{Background: core.RGBA(0xed986e, 0.08), BorderColor: core.RGBA(0xed986e, 0.22)}
	}
	return &core.Style{Background: core.RGBA(0xd4581b, 0.08), BorderColor: core.RGBA(0xd4581b, 0.24)}
}

func itemPressed(cx *core.Scope) *core.Style {
	if cx.Dark() {
		return &core.Style{Background: core.RGBA(0xed986e, 0.14), BorderColor: core.RGBA(0xed986e, 0.32)}
	}
	return &core.Style{Background: core.RGBA(0xd4581b, 0.14), BorderColor: core.RGBA(0xd4581b, 0.34)}
}

// itemProps are a menu row: a mark at the trailing edge (a check, a
// dot), the label and a note (a shortcut, a value).
type itemProps struct {
	label    func() string
	onSelect func()
	note     string
	mark     func() mark
	disabled func() bool
	danger   bool
	keepOpen bool
}

// markDot is a menu row's choice of one.
const markDot = markDash

func (l menuLook) rowStyle() core.Style {
	return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Height: core.Pt(l.height), Padding: core.Symmetric(0, 8),
		CornerRadius: 7, BorderWidth: 1, BorderColor: transparent}
}

func (l menuLook) item(cx *core.Scope, p itemProps) core.Node {
	colors := colorsOf(cx)
	fg := colors.text
	if p.danger {
		fg = colors.danger
	}
	s := l.rowStyle()
	s.Disabled = &core.Style{Opacity: core.Alpha(0.35)}
	states := func() core.Style { return core.Style{Hover: itemHovered(cx), Active: itemPressed(cx)} }
	children := []core.Node{core.Text{ContentFunc: p.label, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 14}, StyleFunc: colored(fg)}}
	if p.note != "" {
		children = append(children, core.Text{Content: p.note, Style: core.Style{FontSize: 12, FontFamily: mono}, StyleFunc: colored(colors.faint)})
	}
	if p.mark != nil {
		children = append(children, core.View{Style: core.Style{Width: core.Pt(16), AlignItems: core.AlignCenter}, Children: []core.Node{
			core.Dynamic(func(*core.Scope) core.Node {
				switch p.mark() {
				case markCheck:
					return icon(iconCheck, 14, colors.accent)
				case markDot:
					return core.View{Style: core.Style{Width: core.Pt(7), Height: core.Pt(7), CornerRadius: 4}, StyleFunc: filled(colors.accent)}
				}
				return core.Group{}
			}),
		}})
	}
	return base.MenuItem{OnSelect: p.onSelect, Disabled: p.disabled, NoCloseOnSelect: p.keepOpen, Style: s, StyleFunc: states, Children: children}
}

// label is a static item label.
func label(s string) func() string { return func() string { return s } }

// submenu is the row that opens a submenu, and the row a sheet shows to
// come back.
func (l menuLook) submenu(cx *core.Scope, name string, disabled bool, children ...core.Node) core.Node {
	colors := colorsOf(cx)
	row := l.rowStyle()
	row.Gap = 0
	if disabled {
		row.Opacity = core.Alpha(0.35)
	}
	back := l.rowStyle()
	back.Gap = 6
	return base.Submenu{Disabled: func() bool { return disabled }, Label: name,
		Trigger: core.View{Style: row, StyleFunc: func() core.Style { return core.Style{Hover: itemHovered(cx), Active: itemPressed(cx)} }, Children: []core.Node{
			core.Text{Content: name, Style: core.Style{FlexGrow: 1, FontSize: 14}, StyleFunc: colored(colors.text)},
			icon(iconChevron, 14, colors.faint),
		}},
		Back:     core.View{Style: back, StyleFunc: func() core.Style { return core.Style{Active: itemPressed(cx)} }, Children: []core.Node{icon(iconBack, 14, colors.accent), text(name, 14, semibold, colors.accent)}},
		Children: children}
}

// groupTitle is a group's title: not an item, nothing to press.
func groupTitle(cx *core.Scope, s string) core.Node {
	colors := colorsOf(cx)
	return core.Text{Content: s, Style: core.Style{FontSize: 11, FontWeight: semibold, LetterSpacing: 0.6,
		TextTransform: core.TextTransformUppercase, Padding: core.Edges{Top: 8, Right: 8, Bottom: 4, Left: 8}}, StyleFunc: colored(colors.faint)}
}

func separator(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	return base.MenuSeparator{Style: core.Style{Height: core.Pt(1), Margin: core.Symmetric(4, 4)}, StyleFunc: filled(colors.line)}
}

// chip is a pressable that opens a menu.
func chip(cx *core.Scope, label func() string, name iconName) core.Node {
	colors := colorsOf(cx)
	fin := finOf(cx)
	children := []core.Node{}
	if name != "" {
		children = append(children, icon(name, 14, colors.muted))
	}
	children = append(children, textf(label, 14, medium, colors.text))
	return core.View{StyleFunc: func() core.Style {
		return fin.raised().over(core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, AlignSelf: core.AlignStart,
			Padding: core.Symmetric(0, 12), Height: core.Pt(34), JustifyContent: core.JustifyCenter, CornerRadius: radiusControl, Background: colors.raised(),
			Active: &core.Style{Background: colors.border()}, Hover: &core.Style{Background: colors.border()}})
	}, Children: children}
}

var zones = []string{
	"UTC−10 Honolulu", "UTC−8 Los Angeles", "UTC−7 Denver", "UTC−6 Chicago", "UTC−5 New York", "UTC−3 São Paulo",
	"UTC±0 London", "UTC+1 Berlin", "UTC+2 Cairo", "UTC+3 Moscow", "UTC+4 Dubai", "UTC+5:30 Mumbai", "UTC+7 Bangkok",
	"UTC+8 Hong Kong", "UTC+8 Shanghai", "UTC+8 Singapore", "UTC+9 Tokyo", "UTC+9 Seoul", "UTC+10 Sydney", "UTC+12 Auckland",
}

type menuFile struct {
	id      int
	name    string
	starred bool
}

// menusDemo is base's menus as complete as an app's: a menu bar whose
// menus have shortcuts, submenus three levels deep, checkable items and a
// choice of one, group titles, disabled and destructive items; menus on
// the rows of a list; where a popup goes; a long menu that scrolls; a menu
// a shortcut opens. On desktops they are popups with cascading submenus,
// on touch screens sheets whose submenus take the place of their items.
// Then the platform's own menus: the application menu and a popup menu.
func menusDemo(cx *core.Scope) core.Node {
	l := newMenuLook(cx)
	events := core.NewSignalFunc(cx, []string(nil), slices.Equal)
	log := func(what string) {
		events.Update(func(es []string) []string { return append([]string{what}, es[:min(len(es), 5)]...) })
	}
	changed := func(menu string) func(bool, base.Reason) {
		return func(open bool, reason base.Reason) {
			state := "closed"
			if open {
				state = "opened"
			}
			log(fmt.Sprintf("%s %s · %s", menu, state, reason))
		}
	}
	key := func(k string) string {
		if l.desktop {
			return cx.KeyLabel(k)
		}
		return ""
	}
	menu := func(name string, trigger core.Node, children ...core.Node) base.Menu {
		return base.Menu{Offset: 4, Margin: 8, PopupStyleFunc: l.surface(cx), BackdropStyle: l.backdrop(), OnOpenChange: changed(name),
			Label: name, Trigger: trigger, Children: children}
	}

	menuBar := func(cx *core.Scope) core.Node {
		history, undone := core.NewSignalFunc(cx, []string(nil), slices.Equal), core.NewSignalFunc(cx, []string(nil), slices.Equal)
		sidebar, statusBar := core.NewSignal(cx, true), core.NewSignal(cx, false)
		zoom := core.NewSignal(cx, "Medium")
		recent := core.NewSignalFunc(cx, []string{"Budget.numbers", "Roadmap.md", "Trip photos"}, slices.Equal)
		act := func(what string) {
			history.Update(func(h []string) []string { return append(slices.Clone(h), what) })
			undone.Set(nil)
			log("chose " + what)
		}
		move := func(from, to *core.Signal[[]string], verb string) {
			f := from.GetUntracked()
			if len(f) == 0 {
				return
			}
			last := f[len(f)-1]
			from.Set(slices.Clone(f[:len(f)-1]))
			to.Update(func(t []string) []string { return append(slices.Clone(t), last) })
			log(verb + " " + last)
		}
		undo, redo := func() { move(history, undone, "undid") }, func() { move(undone, history, "redid") }
		// The shortcuts the items show work without the menu too.
		cx.OnKey("mod+alt+n", func() { act("New document") })
		cx.OnKey("mod+alt+s", func() { act("Save") })
		cx.OnKey("mod+alt+z", undo)
		cx.OnKey("mod+alt+shift+z", redo)
		lastOf := func(s *core.Signal[[]string], verb string) func() string {
			return func() string {
				if h := s.Get(); len(h) > 0 {
					return verb + " " + h[len(h)-1]
				}
				return verb
			}
		}
		empty := func(s *core.Signal[[]string]) func() bool { return func() bool { return len(s.Get()) == 0 } }
		check := func(s *core.Signal[bool]) func() mark {
			return func() mark {
				if s.Get() {
					return markCheck
				}
				return markNone
			}
		}
		var sizes []core.Node
		for _, size := range []string{"Small", "Medium", "Large"} {
			sizes = append(sizes, l.item(cx, itemProps{label: label(size), keepOpen: true, onSelect: func() { zoom.Set(size) }, mark: func() mark {
				if zoom.Get() == size {
					return markDot
				}
				return markNone
			}}))
		}
		return card(cx, cardProps{icon: iconLists, title: "Menu bar", subtitle: hint(cx, "Each menu a sheet; submenus take its place", "Hover across submenus; the shortcuts work too")},
			core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
				menu("File", chip(cx, label("File"), ""),
					l.item(cx, itemProps{label: label("New document"), note: key("mod+alt+n"), onSelect: func() { act("New document") }}),
					l.item(cx, itemProps{label: label("New window"), onSelect: func() { act("New window") }}),
					l.submenu(cx, "Open recent", false,
						core.Each(recent.Get, func(s string) string { return s }, func(cx *core.Scope, name func() string, _ func() int) core.Node {
							return l.item(cx, itemProps{label: name, onSelect: func() { act("Open " + name()) }})
						}),
						separator(cx),
						l.item(cx, itemProps{label: label("Clear recent"), disabled: empty(recent), onSelect: func() { recent.Set(nil) }}),
					),
					separator(cx),
					l.item(cx, itemProps{label: label("Save"), note: key("mod+alt+s"), onSelect: func() { act("Save") }}),
					l.submenu(cx, "Export as", false,
						l.item(cx, itemProps{label: label("PDF"), onSelect: func() { act("Export PDF") }}),
						l.submenu(cx, "Image", false,
							l.item(cx, itemProps{label: label("PNG"), onSelect: func() { act("Export PNG") }}),
							l.item(cx, itemProps{label: label("JPEG"), onSelect: func() { act("Export JPEG") }}),
							l.item(cx, itemProps{label: label("WebP"), onSelect: func() { act("Export WebP") }}),
						),
						l.submenu(cx, "Web page", true, l.item(cx, itemProps{label: label("HTML")})),
					),
					separator(cx),
					l.item(cx, itemProps{label: label("Delete document"), danger: true, onSelect: func() { act("Delete document") }}),
				),
				menu("Edit", chip(cx, label("Edit"), ""),
					l.item(cx, itemProps{label: lastOf(history, "Undo"), note: key("mod+alt+z"), disabled: empty(history), onSelect: undo}),
					l.item(cx, itemProps{label: lastOf(undone, "Redo"), note: key("mod+alt+shift+z"), disabled: empty(undone), onSelect: redo}),
					separator(cx),
					groupTitle(cx, "Clipboard"),
					l.item(cx, itemProps{label: label("Cut"), onSelect: func() { act("Cut") }}),
					l.item(cx, itemProps{label: label("Copy"), onSelect: func() { act("Copy") }}),
					l.item(cx, itemProps{label: label("Paste"), onSelect: func() { act("Paste") }}),
				),
				menu("View", chip(cx, label("View"), ""), append([]core.Node{
					groupTitle(cx, "Show"),
					l.item(cx, itemProps{label: label("Sidebar"), mark: check(sidebar), keepOpen: true, onSelect: func() { toggled(sidebar) }}),
					l.item(cx, itemProps{label: label("Status bar"), mark: check(statusBar), keepOpen: true, onSelect: func() { toggled(statusBar) }}),
					separator(cx),
					groupTitle(cx, "Zoom"),
				}, sizes...)...),
			}},
			captionf(cx, func() string {
				return fmt.Sprintf("Sidebar %s · status bar %s · zoom %s", onOff(sidebar.Get()), onOff(statusBar.Get()), zoom.Get())
			}),
			captionf(cx, func() string {
				if h := history.Get(); len(h) > 0 {
					return "Done: " + strings.Join(h, ", ")
				}
				return "Nothing done yet"
			}),
		)
	}

	rows := func(cx *core.Scope) core.Node {
		next := 5
		files := core.NewSignalFunc(cx, []menuFile{{1, "Invoice March.pdf", false}, {2, "Holiday.jpg", true}, {3, "Notes.txt", false}, {4, "Slides.key", false}}, slices.Equal)
		find := func(id int) menuFile {
			for _, f := range files.GetUntracked() {
				if f.id == id {
					return f
				}
			}
			return menuFile{}
		}
		actions := func(id int) []core.Node {
			name := func() string { return find(id).name }
			return []core.Node{
				l.item(cx, itemProps{label: label("Open"), onSelect: func() { log("open " + name()) }}),
				l.item(cx, itemProps{label: func() string {
					for _, f := range files.Get() {
						if f.id == id && f.starred {
							return "Unstar"
						}
					}
					return "Star"
				}, onSelect: func() {
					files.Update(func(fs []menuFile) []menuFile {
						out := slices.Clone(fs)
						for i := range out {
							if out[i].id == id {
								out[i].starred = !out[i].starred
							}
						}
						return out
					})
				}}),
				l.item(cx, itemProps{label: label("Duplicate"), onSelect: func() {
					f := find(id)
					ext := ""
					base := f.name
					if i := strings.LastIndex(f.name, "."); i > 0 {
						base, ext = f.name[:i], f.name[i:]
					}
					c := menuFile{id: next, name: base + " copy" + ext}
					next++
					files.Update(func(fs []menuFile) []menuFile {
						at := slices.IndexFunc(fs, func(x menuFile) bool { return x.id == id })
						return slices.Insert(slices.Clone(fs), at+1, c)
					})
				}}),
				l.submenu(cx, "Move to", false,
					l.item(cx, itemProps{label: label("Documents"), onSelect: func() { log("moved " + name() + " to Documents") }}),
					l.item(cx, itemProps{label: label("Archive"), onSelect: func() { log("moved " + name() + " to Archive") }}),
				),
				separator(cx),
				l.item(cx, itemProps{label: label("Delete"), danger: true, onSelect: func() {
					files.Update(func(fs []menuFile) []menuFile {
						return slices.DeleteFunc(slices.Clone(fs), func(x menuFile) bool { return x.id == id })
					})
				}}),
			}
		}
		return card(cx, cardProps{icon: iconPhotos, title: "On the rows of a list", subtitle: hint(cx, "Hold a finger on a row, or press its ⋯", "Right click a row, or its ⋯ button")},
			core.Show{When: func() bool { return len(files.Get()) > 0 }, Else: func(*core.Scope) core.Node { return caption(cx, "No files left", nil) },
				Then: func(*core.Scope) core.Node {
					return core.View{Children: []core.Node{core.Each(files.Get, func(f menuFile) int { return f.id },
						func(cx *core.Scope, file func() menuFile, i func() int) core.Node {
							colors := colorsOf(cx)
							id := core.Untracked(cx, file).id
							starred := func() bool { return file().starred }
							return base.ContextMenu{Margin: 8, PopupStyleFunc: l.surface(cx), BackdropStyle: l.backdrop(), OnOpenChange: changed(core.Untracked(cx, file).name),
								Trigger: core.View{StyleFunc: func() core.Style {
									s := core.Style{MinHeight: core.Pt(48), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}
									if i() != 0 {
										s.Borders, s.BorderColors = core.Edges{Top: 1}, core.EdgeColors{Top: colors.line()}
									}
									return s
								}, Children: []core.Node{
									core.Dynamic(func(*core.Scope) core.Node {
										if starred() {
											return icon(iconStar, 16, colors.accent)
										}
										return icon(iconText, 16, colors.muted)
									}),
									core.Text{ContentFunc: func() string { return file().name }, MaxLines: 1, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 14}, StyleFunc: colored(colors.text)},
									base.Menu{Align: core.AlignEnd, Offset: 4, Margin: 8, PopupStyleFunc: l.surface(cx), BackdropStyle: l.backdrop(),
										Label: "Actions for " + core.Untracked(cx, file).name,
										Trigger: core.View{Tooltip: core.Text{Content: "More actions"}, HitSlop: core.All(8),
											Style:    core.Style{Padding: core.Symmetric(0, 8), Height: core.Pt(28), JustifyContent: core.JustifyCenter, Active: &core.Style{Opacity: core.Alpha(0.5)}},
											Children: []core.Node{text("⋯", 18, regular, colors.muted)}},
										Children: actions(id)},
								}},
								Children: actions(id)}
						})}}
				}},
		)
	}

	placement := func(cx *core.Scope) core.Node {
		side := core.NewSignal(cx, core.PlaceBottom)
		align := core.NewSignal(cx, core.AlignStart)
		alignName := map[core.Align]string{core.AlignStart: "start", core.AlignCenter: "center", core.AlignEnd: "end"}
		return card(cx, cardProps{icon: iconComponents, title: "Where it goes", subtitle: hint(cx, "Phones show menus as sheets: try a desktop", "The side and alignment; it flips where there is no room")},
			segmented([]option[core.Placement]{{value: core.PlaceTop, label: "Top"}, {value: core.PlaceBottom, label: "Bottom"}, {value: core.PlaceLeft, label: "Left"}, {value: core.PlaceRight, label: "Right"}},
				side.Get, side.Set, true),
			segmented([]option[core.Align]{{value: core.AlignStart, label: "Start"}, {value: core.AlignCenter, label: "Center"}, {value: core.AlignEnd, label: "End"}},
				align.Get, align.Set, true),
			core.View{Style: core.Style{AlignItems: core.AlignCenter, Padding: core.Symmetric(24, 0)}, Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
				p, a := side.Get(), align.Get()
				m := menu("Placed", chip(cx, label(placementName(p)+" · "+alignName[a]), ""),
					l.item(cx, itemProps{label: label("One"), onSelect: func() { log("chose One") }}),
					l.item(cx, itemProps{label: label("Two"), onSelect: func() { log("chose Two") }}),
					l.item(cx, itemProps{label: label("Three"), onSelect: func() { log("chose Three") }}),
				)
				m.Placement, m.Align, m.Offset = p, a, 6
				return m
			})}},
		)
	}

	longList := func(cx *core.Scope) core.Node {
		zone := core.NewSignal(cx, "UTC+8 Hong Kong")
		var items []core.Node
		for _, z := range zones {
			items = append(items, l.item(cx, itemProps{label: label(z), onSelect: func() { zone.Set(z) }, mark: func() mark {
				if zone.Get() == z {
					return markCheck
				}
				return markNone
			}}))
		}
		maxHeight := core.Points(360)
		if l.desktop {
			maxHeight = 280
		}
		m := menu("Time zone", chip(cx, zone.Get, iconSearch),
			core.Scroll{Style: core.Style{MaxHeight: core.Pt(maxHeight)}, Children: []core.Node{core.View{Style: core.Style{Padding: core.All(6)}, Children: items}}})
		m.PopupStyle.Padding = core.Edges{}
		if !l.desktop {
			m.PopupStyle.Padding = core.Edges{Bottom: 34}
		}
		return card(cx, cardProps{icon: iconScroll, title: "A long menu", subtitle: "Twenty choices in a menu that scrolls"}, m)
	}

	commands := func(cx *core.Scope) core.Node {
		open := core.NewSignal(cx, false)
		cx.OnKey("mod+shift+p", func() { toggled(open) })
		var right core.Node
		if l.desktop {
			right = badge(cx, cx.KeyLabel("mod+shift+p"), toneNeutral)
		}
		m := menu("Commands", chip(cx, label("Commands"), iconPlus),
			groupTitle(cx, "Commands"),
			l.item(cx, itemProps{label: label("New note"), onSelect: func() { log("ran New note") }}),
			l.item(cx, itemProps{label: label("Toggle theme"), onSelect: func() { log("ran Toggle theme") }}),
			l.item(cx, itemProps{label: label("Sync now"), onSelect: func() { log("ran Sync now") }}),
		)
		m.Open = open.Get
		m.OnOpenChange = func(next bool, reason base.Reason) {
			open.Set(next)
			changed("commands")(next, reason)
		}
		return card(cx, cardProps{icon: iconSpark, title: "Opened by the app", subtitle: "A controlled menu: a button or a shortcut opens it", right: right},
			m, button(buttonProps{label: "Open the commands", variant: secondary}, func() { open.Set(true) }))
	}

	native := func(cx *core.Scope) core.Node {
		colors := colorsOf(cx)
		chosen := core.NewSignal(cx, "Nothing chosen yet")
		items := func() []desktop.MenuItem {
			choose := func(s string) func() { return func() { chosen.Set("Chose " + s); log("native " + s) } }
			return []desktop.MenuItem{
				{Label: "Cut", Shortcut: "mod+x", OnSelect: choose("Cut")},
				{Label: "Copy", Shortcut: "mod+c", OnSelect: choose("Copy")},
				{Label: "Paste", Disabled: true},
				{Separator: true},
				{Label: "Share", Items: []desktop.MenuItem{{Label: "Mail", OnSelect: choose("Mail")}, {Label: "Messages", OnSelect: choose("Messages")}}},
				{Label: "Pinned", Checkable: true, Checked: true, OnSelect: choose("Pinned")},
			}
		}
		show := func(at core.Point) { desktop.PopupMenu(cx, items(), at, nil) }
		return card(cx, cardProps{icon: iconMenu, title: "The platform's menu", subtitle: "PopupMenu shows the system's own at a point", right: badge(cx, "Native", toneInfo)},
			core.View{TestID: "popup-area", Role: core.RoleButton, Label: "Show the native menu",
				StyleFunc: func() core.Style {
					return core.Style{Height: core.Pt(72), CornerRadius: radiusInset, Background: colors.raised(), BorderWidth: 1, BorderColor: colors.border(),
						AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Active: &core.Style{Background: colors.pressed()}}
				},
				OnPress:       func(e core.PressEvent) { show(e.PagePosition) },
				OnContextMenu: func(e core.ContextMenuEvent) { show(e.PagePosition) },
				Children:      []core.Node{text(hint(cx, "Press here", "Right click or press here"), 14, regular, colors.muted)}},
			captionf(cx, chosen.Get),
		)
	}

	appMenu := func(cx *core.Scope) core.Node {
		children := []core.Node{
			note(cx, "SetApplicationMenu sets the menu bar on macOS, Windows and Linux: this gallery's has the roles the platform carries out (About, Hide, Undo … Select All, Minimize, Zoom) and a View menu that toggles the sidebar and opens each demo."),
			caption(cx, "Last chosen", shell.chosen.Get),
		}
		if env := cx.Env(); env.Desktop() && env.OS() != core.OSMacOS && env.OS() != core.OSWeb {
			children = append(children, note(cx, "SetMenuBar hides the bar Windows and Linux draw until Alt is pressed alone."),
				hrow(8,
					button(buttonProps{label: "Always shown", variant: secondary, small: true}, func() { desktop.SetMenuBar(cx, desktop.MenuBarShown) }),
					button(buttonProps{label: "Auto-hide", variant: secondary, small: true}, func() { desktop.SetMenuBar(cx, desktop.MenuBarAutoHide) }),
				))
		}
		return card(cx, cardProps{icon: iconMenu, title: "The application menu", subtitle: "The system's menu bar, set by the app", right: badge(cx, "Native", toneInfo)}, children...)
	}

	eventsCard := func(cx *core.Scope) core.Node {
		colors := colorsOf(cx)
		return card(cx, cardProps{icon: iconSystem, title: "Events", subtitle: "OnOpenChange with its reason, and what was chosen"},
			core.Show{When: func() bool { return len(events.Get()) > 0 }, Else: func(*core.Scope) core.Node { return caption(cx, "Open a menu", nil) },
				Then: func(*core.Scope) core.Node {
					return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
						var lines []core.Node
						for i, e := range events.Get() {
							c := colors.faint
							if i == 0 {
								c = colors.text
							}
							lines = append(lines, core.Text{Content: e, Style: core.Style{FontSize: 12, FontFamily: mono}, StyleFunc: colored(c)})
						}
						return core.View{Style: core.Style{Gap: 4}, Children: lines}
					})}}
				}},
		)
	}

	return demoPage(cx, pageProps{}, menuBar, rows, placement, longList, commands, native, appMenu, eventsCard)
}
