package ui

import (
	"slices"
	"strings"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
)

// Command is an entry of a CommandPalette.
type Command struct {
	ID, Title, Description string
	// Group heads a run of commands: consecutive commands of one group
	// share its heading.
	Group string
	Icon  *core.SVG
	// Shortcut is shown at the row's end, such as "⌘,".
	Shortcut string
	// Keywords are words search finds the command by without showing them.
	Keywords []string
	Disabled bool
}

// FilterCommands returns the commands every whitespace-separated term of
// query occurs in, ignoring case: in the title, description, group or
// keywords. Those whose title starts with the query come first; the order
// is kept otherwise. An empty query returns every command.
func FilterCommands(commands []Command, query string) []Command {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return commands
	}
	terms := strings.Fields(q)
	var first, rest []Command
	for _, c := range commands {
		if !matchAll(strings.ToLower(strings.Join([]string{c.Title, c.Description, c.Group,
			strings.Join(c.Keywords, " ")}, " ")), terms) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(c.Title), q) {
			first = append(first, c)
		} else {
			rest = append(rest, c)
		}
	}
	return append(first, rest...)
}

// CommandPalette is a dialog for finding and running commands: a search
// field over the matching commands, grouped. Up and Down move the active
// command while the field keeps focus, Enter or a press runs one, and
// Escape or a press outside closes it. Each opening starts with an empty
// search.
//
// Open and OnOpenChange control it: running a command asks to close with
// base.ReasonSelect, then reports the command's ID to OnSelect.
type CommandPalette struct {
	Open         func() bool
	OnOpenChange func(open bool, reason base.Reason)
	// Commands is read while the palette is open, again when what it
	// reads changes.
	Commands func() []Command
	OnSelect func(id string)
	// The texts default to English.
	Placeholder, EmptyText string
	// Label names the dialog for assistive technology.
	Label         string
	BackdropStyle core.Style
	PopupStyle    core.Style
	TestID        string
}

// Render implements core.Node.
func (p CommandPalette) Render(cx *core.Scope) core.Node {
	label := p.Label
	if label == "" {
		label = "Command palette"
	}
	return base.Dialog{
		Open: p.Open, OnOpenChange: p.OnOpenChange, Animation: core.ModalFade,
		BackdropStyleFunc: look(cx, p.BackdropStyle, Tokens.backdrop),
		PopupStyleFunc:    look(cx, p.PopupStyle, Tokens.palette),
		Children:          []core.Node{palette{p}},
		Label:             label, TestID: p.TestID,
	}
}

func (t Tokens) palette() core.Style {
	return core.Style{Width: core.Pt(600), MaxWidth: core.Pct(92), MaxHeight: core.Pct(85),
		Background: t.Surface, Color: t.SurfaceForeground, FontSize: t.FontSize,
		CornerRadius: t.RadiusLarge + 4, BorderWidth: 1, BorderColor: t.Border, Overflow: core.Hidden,
		ShadowColor: t.Shadow, ShadowRadius: 24, ShadowOffset: core.Point{Y: 8}}
}

// palette is the open palette's content, built anew each time it opens.
type palette struct{ CommandPalette }

func (p palette) Render(cx *core.Scope) core.Node {
	theme := UseTheme(cx)
	query := core.NewSignal(cx, "")
	results := core.NewMemoFunc(cx, func() []Command {
		var all []Command
		if p.Commands != nil {
			all = p.Commands()
		}
		return FilterCommands(all, query.Get())
	}, nil)
	// chosen is the command Up and Down chose; active is the one Enter runs:
	// chosen while it matches, the first enabled match otherwise.
	chosen := core.NewSignal(cx, "")
	active := core.NewMemo(cx, func() string {
		rs, id := results.Get(), chosen.Get()
		if i := slices.IndexFunc(rs, func(c Command) bool { return c.ID == id }); i >= 0 && !rs[i].Disabled {
			return id
		}
		for _, c := range rs {
			if !c.Disabled {
				return c.ID
			}
		}
		return ""
	})
	run := func(id string) {
		if id == "" {
			return
		}
		if p.OnOpenChange != nil {
			p.OnOpenChange(false, base.ReasonSelect)
		}
		if p.OnSelect != nil {
			p.OnSelect(id)
		}
	}

	// Rows report where they are, so moving keeps the active one in view.
	list := &core.Ref{}
	rows := map[string]core.Bounds{}
	var offset, visible core.Points
	reveal := func(id string) {
		b, ok := rows[id]
		if !ok {
			return
		}
		switch top, bottom := b.Origin.Y, b.Origin.Y+b.Size.Height; {
		case top < offset:
			list.ScrollTo(core.Point{Y: top}, false)
		case bottom > offset+visible:
			list.ScrollTo(core.Point{Y: bottom - visible}, false)
		}
	}
	move := func(by int) {
		rs, id := results.GetUntracked(), active.GetUntracked()
		i := slices.IndexFunc(rs, func(c Command) bool { return c.ID == id })
		for j := i + by; j >= 0 && j < len(rs); j += by {
			if !rs[j].Disabled {
				chosen.Set(rs[j].ID)
				reveal(rs[j].ID)
				return
			}
		}
	}
	cx.OnKey("up", func() { move(-1) })
	cx.OnKey("down", func() { move(1) })

	muted := func(size bool) func() core.Style {
		return func() core.Style {
			t := theme()
			s := core.Style{Color: t.MutedForeground}
			if size {
				s.FontSize = t.small()
			}
			return s
		}
	}
	row := func(cx *core.Scope, item func() Command, index func() int) core.Node {
		c := core.Untracked(cx, item)
		heading := core.Show{
			When: func() bool {
				i, rs := index(), results.Get()
				return c.Group != "" && (i == 0 || i >= len(rs) || rs[i-1].Group != c.Group)
			},
			Then: func(*core.Scope) core.Node {
				return core.Text{Content: c.Group, MaxLines: 1, Role: core.RoleHeader,
					Style: core.Style{Padding: core.Edges{Top: 8, Right: 10, Bottom: 4, Left: 10}}, StyleFunc: muted(true)}
			},
		}
		var parts []core.Node
		if c.Icon != nil {
			parts = append(parts, core.Icon{SVG: c.Icon, Style: core.Style{Width: core.Pt(18), Height: core.Pt(18), FlexShrink: 0}})
		}
		text := []core.Node{core.Text{Content: c.Title, MaxLines: 1}}
		if c.Description != "" {
			text = append(text, core.Text{Content: c.Description, MaxLines: 1, StyleFunc: muted(true)})
		}
		parts = append(parts, core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 1}, Children: text})
		if c.Shortcut != "" {
			parts = append(parts, core.Text{Content: c.Shortcut, MaxLines: 1, StyleFunc: muted(true)})
		}
		return core.View{
			OnLayout: func(b core.Bounds) { rows[c.ID] = b },
			Children: []core.Node{heading, core.View{
				OnPress: func() { run(c.ID) }, Role: core.RoleButton, Label: c.Title, TestID: "command-" + c.ID,
				Disabled: func() bool { return c.Disabled }, Selected: func() bool { return active.Get() == c.ID },
				StyleFunc: func() core.Style {
					t := theme()
					s := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Symmetric(9, 10),
						CornerRadius: t.Radius + 4,
						Selected:     &core.Style{Background: t.Tint(0.1)},
						Disabled:     &core.Style{Color: t.MutedForeground}}
					// Hover merges over selected, so only rows not chosen take it.
					if active.Get() != c.ID {
						s.Hover = &core.Style{Background: t.Tint(0.04)}
					}
					return s
				},
				Children: parts,
			}},
		}
	}

	return core.Group{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Height: core.Pt(56),
			Padding: core.Symmetric(0, 18), FlexShrink: 0}, Children: []core.Node{
			core.Icon{SVG: lucide.Search(), Style: core.Style{Width: core.Pt(20), Height: core.Pt(20), FlexShrink: 0}, StyleFunc: muted(false)},
			core.Input{Value: query.Get, AutoFocus: true, Placeholder: orString(p.Placeholder, "Search commands"),
				Label: "Search commands", TestID: "command-search",
				OnChange: func(text string) {
					query.Set(text)
					chosen.Set("")
					list.ScrollTo(core.Point{}, false)
				},
				OnSubmit: func(string) { run(active.GetUntracked()) },
				StyleFunc: func() core.Style {
					t := theme()
					return core.Style{FlexGrow: 1, FlexShrink: 1, Height: core.Pct(100), FontSize: t.FontSize + 2, Color: t.Foreground}
				}},
		}},
		core.View{StyleFunc: func() core.Style { return core.Style{Height: core.Pt(1), FlexShrink: 0, Background: theme().Border} }},
		core.Scroll{Ref: list, Style: core.Style{Height: core.Pt(340), FlexShrink: 1},
			OnScroll: func(e core.ScrollEvent) { offset = e.Offset.Y },
			OnLayout: func(b core.Bounds) { visible = b.Size.Height },
			Children: []core.Node{
				core.View{Style: core.Style{Padding: core.All(8)}, Children: []core.Node{
					core.Each(results.Get, func(c Command) string { return c.ID }, row),
				}},
				core.Show{When: func() bool { return len(results.Get()) == 0 }, Then: func(*core.Scope) core.Node {
					return core.Text{Content: orString(p.EmptyText, "No matching commands"), Role: core.RoleText,
						Style: core.Style{Padding: core.All(24), TextAlign: core.TextAlignCenter}, StyleFunc: muted(false)}
				}},
			}},
	}
}

func orString(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
