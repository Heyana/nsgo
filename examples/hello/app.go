// Command hello shows text, an input, a button and a list on every target.
package main

import (
	"fmt"
	"strings"

	"github.com/oneo-me/nsgo"
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

var (
	ink    = core.RGB(0x1f2328)
	muted  = core.RGB(0x6e7781)
	green  = core.RGB(0x1f883d)
	card   = core.Style{Background: core.RGB(0xffffff), CornerRadius: 10, BorderWidth: 1, BorderColor: core.RGB(0xd0d7de)}
	field  = core.Style{Height: core.Pt(40), Padding: core.Symmetric(0, 10), FontSize: 16}.Merge(card)
	button = core.Style{Background: green, CornerRadius: 10, Padding: core.Symmetric(10, 18), AlignItems: core.AlignCenter,
		Active: &core.Style{Background: core.RGB(0x1a7f37)}}
	label = core.Style{FontSize: 16, FontWeight: core.FontWeightSemibold, Color: core.RGB(0xffffff)}
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
		Style: core.Style{FlexGrow: 1, Padding: core.Edges{Top: 56, Right: 24, Bottom: 24, Left: 24}, Gap: 16,
			Background: core.RGB(0xf4f5f7), Color: ink},
		Children: []core.Node{
			core.Text{ContentFunc: greeting, Style: core.Style{FontSize: 28, FontWeight: core.FontWeightBold}},
			core.View{Style: core.Style{Direction: core.Row, Gap: 10}, Children: []core.Node{
				core.Input{
					Value:       draft.Get,
					OnChange:    draft.Set,
					OnSubmit:    func(string) { add() },
					Placeholder: "Something to do",
					Style:       field.Merge(core.Style{FlexGrow: 1}),
					TestID:      "draft",
				},
				base.Button{OnPress: add, Style: button, Label: "Add", TestID: "add", Children: []core.Node{
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
									base.Button{OnPress: func() { remove(todo().ID) }, Label: "Done",
										Style: core.Style{Padding: core.Symmetric(4, 10), CornerRadius: 6, Background: core.RGB(0xeef1f4),
											Hover: &core.Style{Background: core.RGB(0xdde3ea)}},
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
