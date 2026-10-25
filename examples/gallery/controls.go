package main

import (
	"fmt"
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// controlsPage shows base's controls: checkboxes, radios, switches, tabs,
// progress bars and a collapsible region, each styled here.
func controlsPage(cx *core.Scope, t *theme) core.Node {
	options := []string{"Photos", "Documents", "Music"}
	picked := core.NewSignalFunc(cx, map[string]bool{"Photos": true}, nil)
	count := func() int {
		n := 0
		for _, o := range options {
			if picked.Get()[o] {
				n++
			}
		}
		return n
	}
	set := func(name string, on bool) {
		picked.Update(func(m map[string]bool) map[string]bool {
			out := map[string]bool{}
			for k, v := range m {
				out[k] = v
			}
			out[name] = on
			return out
		})
	}
	box := func(label string, checked, mixed func() bool, onChange func(bool)) core.Node {
		return base.Checkbox{Label: label, Checked: checked, Indeterminate: mixed, OnChange: onChange, Style: row,
			IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 5, BorderWidth: 1.5,
				BorderColor: t.borderStrong, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
				Checked:       &core.Style{Background: t.accent, BorderColor: t.accent},
				Indeterminate: &core.Style{Background: t.accent, BorderColor: t.accent}},
			Indicator: core.Text{ContentFunc: func() string {
				switch {
				case mixed != nil && mixed():
					return "–"
				case checked():
					return "✓"
				}
				return ""
			}, Style: core.Style{Color: t.onAccent, FontSize: 12, FontWeight: core.FontWeightBold}},
			Children: []core.Node{core.Text{Content: label}}}
	}

	plan := core.NewSignal(cx, "Monthly")
	radio := func(name string) core.Node {
		return base.Radio{Label: name, Checked: func() bool { return plan.Get() == name }, OnChange: func(bool) { plan.Set(name) },
			Style: row.Merge(core.Style{Group: "radio"}),
			Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9, BorderWidth: 1.5,
					BorderColor: t.borderStrong, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
					GroupStyle: &core.GroupStyle{Checked: &core.Style{BorderColor: t.accent}}},
					Children: []core.Node{core.View{Style: core.Style{Width: core.Pt(8), Height: core.Pt(8), CornerRadius: 4,
						GroupStyle: &core.GroupStyle{Checked: &core.Style{Background: t.accent}}}}}},
				core.Text{Content: name},
			}}
	}

	wifi := core.NewSignal(cx, true)
	tab := core.NewSignal(cx, 0)
	tabNames := []string{"Overview", "Activity", "Settings"}
	tabs := make([]core.Node, len(tabNames))
	for i, name := range tabNames {
		tabs[i] = base.Tab{Label: name, Selected: func() bool { return tab.Get() == i }, OnPress: func(core.PressEvent) { tab.Set(i) },
			Style: core.Style{Padding: core.Symmetric(6, 12), CornerRadius: radius - 2, Color: t.muted,
				Hover: &core.Style{Background: t.hover},
				Selected: &core.Style{Background: t.surface, Color: t.ink, FontWeight: core.FontWeightMedium,
					ShadowColor: core.RGBA(0, 0.08), ShadowRadius: 2, ShadowOffset: core.Point{Y: 1}}},
			Children: []core.Node{core.Text{Content: name}}}
	}

	progress := core.NewSignal[float32](cx, 35)
	more := core.NewSignal(cx, false)
	starred := core.NewSignal(cx, false)
	downs := core.NewSignal(cx, 0)
	track := core.Style{Height: core.Pt(6), CornerRadius: 3, Background: t.sunken, Overflow: core.Hidden, Width: core.Pct(100)}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Checkbox", "base.Checkbox reports the value a press asks for; \"All\" is indeterminate over a partial selection.",
			box("All", func() bool { return count() == len(options) }, func() bool { n := count(); return n > 0 && n < len(options) },
				func(on bool) {
					for _, o := range options {
						set(o, on)
					}
				}),
			core.View{Style: core.Style{Gap: 8, Padding: core.Edges{Left: 26}}, Children: func() []core.Node {
				out := make([]core.Node, len(options))
				for i, o := range options {
					out[i] = box(o, func() bool { return picked.Get()[o] }, nil, func(on bool) { set(o, on) })
				}
				return out
			}()},
		),
		t.demo("Radio group", "base.Radio asks to be checked; the app keeps which one is. The dot follows its radio as a group style.",
			base.RadioGroup{Label: "Plan", Style: core.Style{Gap: 8}, Children: []core.Node{radio("Monthly"), radio("Yearly"), radio("Lifetime")}},
			t.live(func() string { return "Plan: " + plan.Get() }),
		),
		t.demo("Switch", "base.Switch's track and thumb are parts whose checked styles follow it.",
			t.toggle("Wi-Fi", wifi.Get, wifi.Set),
		),
		t.demo("Tabs", "base.Tabs holds the tabs; the app shows the selected one's content.",
			base.Tabs{Label: "Sections", Style: core.Style{Direction: core.Row, Gap: 2, Padding: core.All(3), Background: t.quiet,
				CornerRadius: radius, AlignSelf: core.AlignStart}, Children: tabs},
			core.Text{ContentFunc: func() string { return "Showing " + tabNames[tab.Get()] + "." }},
		),
		t.demo("Progress", "base.Progress reports its value to screen readers; its indicator is the value's share of the track.",
			base.Progress{Label: "Upload", Value: progress.Get, Style: core.Style{Gap: 6},
				TrackStyle: track, IndicatorStyle: core.Style{Height: core.Pct(100), Background: t.accent, CornerRadius: 3,
					Transition: core.Transition{Duration: 200 * time.Millisecond}},
				Children: []core.Node{t.live(func() string { return fmt.Sprintf("%.0f%%", progress.Get()) })}},
			flow(
				t.plain("−10%", func() { progress.Update(func(v float32) float32 { return max(v-10, 0) }) }),
				t.plain("+10%", func() { progress.Update(func(v float32) float32 { return min(v+10, 100) }) }),
			),
			base.Progress{Label: "Connecting", Indeterminate: func() bool { return true },
				TrackStyle: track, IndicatorStyle: core.Style{Height: core.Pct(100), Width: core.Pct(30), Background: t.accent, CornerRadius: 3}},
		),
		t.demo("Collapsible", "base.Collapsible makes its content the first time it opens and keeps it while closed.",
			base.Collapsible{Open: more.Get, Style: core.Style{Gap: 8}, Children: []core.Node{
				base.Button{Label: "Details", Expanded: more.Get, OnPress: func(core.PressEvent) { more.Set(!more.Get()) },
					Style: t.secondary(), Children: []core.Node{core.Text{ContentFunc: func() string {
						if more.Get() {
							return "Hide details"
						}
						return "Show details"
					}}}},
			}, Content: core.View{Style: core.Style{Gap: 8, Padding: core.All(12), Background: t.sunken, CornerRadius: radius},
				Children: []core.Node{
					t.note("This field keeps what you type while the region is closed."),
					core.Input{Placeholder: "Type here", Style: t.field()},
				}}},
		),
		t.demo("Button states", "base.Button takes a role, a selected state and press down and up for the app's own feedback.",
			flow(
				base.Button{Label: "Star", Selected: starred.Get, OnPress: func(core.PressEvent) { starred.Set(!starred.Get()) },
					Style: t.secondary().Merge(core.Style{Selected: &core.Style{Background: t.accentSoft, BorderColor: t.accent, Color: t.accent}}),
					Children: []core.Node{core.Text{ContentFunc: func() string {
						if starred.Get() {
							return "★ Starred"
						}
						return "☆ Star"
					}}}},
				base.Button{Label: "Docs", Role: core.RoleLink, OnPress: func(core.PressEvent) {},
					OnPressDown: func(core.PressEvent) { downs.Update(func(n int) int { return n + 1 }) },
					Style:       core.Style{Padding: core.Symmetric(6, 4)},
					Children:    []core.Node{core.Text{Content: "A link", Style: core.Style{Color: t.accent, TextDecoration: core.TextDecorationUnderline}}}},
			),
			t.live(func() string { return fmt.Sprintf("%d presses went down on the link", downs.Get()) }),
		),
	}}
}
