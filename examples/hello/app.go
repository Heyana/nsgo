// Command hello shows text, an input, a button and a list on every target.
package main

import (
	"fmt"
	"strings"

	"github.com/oneo-me/nsgo"
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// The hello example uses the Onevra design language: warm neutral surfaces,
// a terracotta accent and 8-point control radii.
var (
	ink    = core.RGB(0x33332f)
	muted  = core.RGB(0x72726a)
	accent = core.RGB(0xbd481b)
	card   = core.Style{Background: core.RGB(0xfefefd), CornerRadius: 8, BorderWidth: 1, BorderColor: core.RGB(0xdeded5)}
	field  = core.Style{Height: core.Pt(36), Padding: core.Symmetric(0, 10), FontSize: 15}.Merge(card)
	button = core.Style{Background: accent, CornerRadius: 8, Padding: core.Symmetric(6, 14), AlignItems: core.AlignCenter,
		Hover:  &core.Style{Background: core.RGB(0xa63f18)},
		Active: &core.Style{Background: core.RGB(0x8e3614)}}
	label = core.Style{FontSize: 15, FontWeight: core.FontWeightMedium, Color: core.RGB(0xffffff)}
)

// Todo is one entry of the list.
type Todo struct {
	ID   int
	Text string
}

// Root shows a greeting, a field that adds todos and the list of them.
func Root(cx *core.Scope) core.Node {
	draft := core.NewSignal(cx, "")
	todos := core.NewSignalFunc(cx, []Todo{{1, "Read ARCHITECTURE.md"}, {2, "Port a subsystem"}}, nil)
	next := 3

	add := func() {
		text := strings.TrimSpace(draft.GetUntracked())
		if text == "" {
			return
		}
		todos.Update(func(ts []Todo) []Todo { return append(ts, Todo{next, text}) })
		next++
		draft.Set("")
	}
	remove := func(id int) {
		todos.Update(func(ts []Todo) []Todo {
			out := make([]Todo, 0, len(ts))
			for _, t := range ts {
				if t.ID != id {
					out = append(out, t)
				}
			}
			return out
		})
	}
	greeting := func() string {
		if d := strings.TrimSpace(draft.Get()); d != "" {
			return "Add “" + d + "”?"
		}
		return "Hello, NSGo!"
	}

	return core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.Edges{Top: 56, Right: 24, Bottom: 24, Left: 24}, Gap: 16,
			Background: core.RGB(0xfcfbf9), Color: ink},
		Children: []core.Node{
			core.Text{ContentFunc: greeting, Style: core.Style{FontSize: 24, FontWeight: core.FontWeightSemibold}},
			core.View{Style: core.Style{Direction: core.Row, Gap: 10}, Children: []core.Node{
				core.Input{
					Value:       draft.Get,
					OnChange:    draft.Set,
					OnSubmit:    func(string) { add() },
					Placeholder: "Something to do",
					Style:       field.Merge(core.Style{FlexGrow: 1}),
					TestID:      "draft",
				},
				base.Button{OnPress: func(core.PressEvent) { add() }, Style: button, Label: "Add", TestID: "add", Children: []core.Node{
					core.Text{Content: "Add", Style: label},
				}},
			}},
			core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
				core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					core.Each(todos.Get, func(t Todo) int { return t.ID },
						func(cx *core.Scope, todo func() Todo, index func() int) core.Node {
							return core.View{
								Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.All(12), Gap: 10}.Merge(card),
								Children: []core.Node{
									core.Text{ContentFunc: func() string { return fmt.Sprintf("%d.", index()+1) },
										Style: core.Style{Color: muted, FontSize: 15}},
									core.Text{ContentFunc: func() string { return todo().Text },
										Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 15}},
									base.Button{OnPress: func(core.PressEvent) { remove(todo().ID) }, Label: "Done",
										Style: core.Style{Padding: core.Symmetric(4, 10), CornerRadius: 6, Background: core.RGB(0xeceae4),
											Hover:  &core.Style{Background: core.RGB(0xe4e3e1)},
											Active: &core.Style{Background: core.RGB(0xd8d7d5)}},
										Children: []core.Node{core.Text{Content: "Done", Style: core.Style{FontSize: 13, Color: muted}}},
									},
								},
							}
						}),
				}},
			}},
			core.Show{
				When: func() bool { return len(todos.Get()) == 0 },
				Then: func(cx *core.Scope) core.Node {
					return core.Text{Content: "All done.", Style: core.Style{Color: muted, TextAlign: core.TextAlignCenter}}
				},
			},
			core.Text{ContentFunc: func() string { return fmt.Sprintf("%d to do", len(todos.Get())) },
				Style: core.Style{FontSize: 13, Color: muted}},
		},
	}
}

func main() { nsgo.Run(Root) }
