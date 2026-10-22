package main

import (
	"fmt"
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

func controlsPage(cx *core.Scope, t *theme) core.Node {
	presses := core.NewSignal(cx, 0)
	agree := core.NewSignal(cx, false)
	news := core.NewSignal(cx, false)
	wifi := core.NewSignal(cx, true)
	size := core.NewSignal(cx, "M")
	tab := core.NewSignal(cx, 0)
	name := core.NewSignal(cx, "")
	password := core.NewSignal(cx, "")

	check := core.Style{Width: core.Pt(16), Height: core.Pt(16), CornerRadius: 4, BorderWidth: 1, BorderColor: t.borderStrong,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
		Checked:  &core.Style{Background: t.accent, BorderColor: t.accent},
		Disabled: &core.Style{Opacity: core.Alpha(0.45)}}
	mark := func(on func() bool) core.Node {
		return core.Show{When: on, Then: func(*core.Scope) core.Node {
			return core.Text{Content: "✓", Style: core.Style{Color: t.onAccent, FontSize: 11, FontWeight: core.FontWeightBold}}
		}}
	}
	radio := func(v string) core.Node {
		on := func() bool { return size.Get() == v }
		return base.Radio{Checked: on, OnChange: func(bool) { size.Set(v) }, Label: v, Style: row, Children: []core.Node{
			core.View{Checked: on, Style: core.Style{Width: core.Pt(16), Height: core.Pt(16), CornerRadius: 8, BorderWidth: 1, BorderColor: t.borderStrong,
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
				Checked: &core.Style{Background: t.accent, BorderColor: t.accent}}, Children: []core.Node{
				core.Show{When: on, Then: func(*core.Scope) core.Node {
					return core.View{Style: core.Style{Width: core.Pt(6), Height: core.Pt(6), CornerRadius: 3, Background: t.onAccent}}
				}},
			}},
			core.Text{Content: v},
		}}
	}
	pill := core.Points(18)
	tabStyle := core.Style{Padding: core.Symmetric(6, 12), CornerRadius: pill, Color: t.muted,
		Hover: &core.Style{Color: t.ink},
		Selected: &core.Style{Background: t.raised, Color: t.ink, FontWeight: core.FontWeightSemibold,
			ShadowColor: core.RGBA(0, 0.14), ShadowRadius: 2, ShadowOffset: core.Point{Y: 1}}}
	tabNames := []string{"Day", "Week", "Month"}
	var tabs []core.Node
	for i, n := range tabNames {
		tabs = append(tabs, base.Tab{Selected: func() bool { return tab.Get() == i }, OnPress: func() { tab.Set(i) },
			Style: tabStyle, Label: n, Children: []core.Node{core.Text{Content: n}}})
	}
	onOff := func(on bool) string {
		if on {
			return "on"
		}
		return "off"
	}
	labelled := func(label string, field core.Node) core.Node {
		return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{core.Text{Content: label, Style: t.caption()}, field}}
	}

	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Buttons", "base.Button is a pressable view with the button role; its look is the app's.",
			flow(
				t.primary("Primary", func() { presses.Update(func(n int) int { return n + 1 }) }),
				t.plain("Secondary", func() { presses.Update(func(n int) int { return n + 1 }) }),
				t.destructive("Destructive", func() { presses.Update(func(n int) int { return n + 1 }) }),
				base.Button{OnPress: func() {}, Disabled: func() bool { return true }, Label: "Disabled", Style: t.button(),
					Children: []core.Node{core.Text{Content: "Disabled"}}},
			),
			t.live(func() string { return fmt.Sprintf("%d presses", presses.Get()) }),
		),
		t.demo("Checkbox and switch", "The app owns the value; a press reports the next one to OnChange.",
			base.Checkbox{Checked: agree.Get, OnChange: agree.Set, Label: "Agree", Style: row, IndicatorStyle: check,
				Indicator: mark(agree.Get), Children: []core.Node{core.Text{Content: "I agree to the terms"}}},
			base.Checkbox{Checked: news.Get, OnChange: news.Set, Disabled: func() bool { return !agree.Get() }, Label: "Newsletter",
				Style: row, IndicatorStyle: check, Indicator: mark(news.Get),
				Children: []core.Node{core.Text{Content: "Send me news (needs the terms)"}}},
			base.Switch{Checked: wifi.Get, OnChange: wifi.Set, Label: "Wi-Fi", Style: row,
				TrackStyle: core.Style{Width: core.Pt(40), Height: core.Pt(20), CornerRadius: 10, Background: t.switchOff, Padding: core.All(2),
					Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyStart,
					Transition: core.Transition{Duration: 150 * time.Millisecond, Properties: core.TransitionBackground},
					Checked:    &core.Style{Background: t.accent, JustifyContent: core.JustifyEnd}},
				ThumbStyle: core.Style{Width: core.Pt(20), Height: core.Pt(16), CornerRadius: 8, Background: t.knob,
					ShadowColor: core.RGBA(0, 0.18), ShadowRadius: 2, ShadowOffset: core.Point{Y: 1}},
				Children: []core.Node{core.Text{ContentFunc: func() string { return "Wi-Fi " + onOff(wifi.Get()) }}}},
		),
		t.demo("Radio group", "Radios in a RadioGroup; the app checks one and unchecks the others.",
			base.RadioGroup{Label: "Size", Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{radio("S"), radio("M"), radio("L")}},
			t.live(func() string { return "Size " + size.Get() }),
		),
		t.demo("Tabs", "Tab's Selected state style marks the selected tab.",
			base.Tabs{Label: "Range", Style: core.Style{Direction: core.Row, Gap: 2, Padding: core.All(2), CornerRadius: pill,
				Background: t.track(), AlignSelf: core.AlignStart}, Children: tabs},
			core.Text{ContentFunc: func() string { return []string{"Today's view", "This week's view", "This month's view"}[tab.Get()] }},
		),
		t.demo("Text fields", "Input is the platform's field: controlled, masked, placeholders and keyboard types.",
			labelled("Name", core.Input{Value: name.Get, OnChange: name.Set, Placeholder: "Your name", Style: t.field(), Label: "Name", TestID: "name"}),
			labelled("Password", core.Input{Value: password.Get, OnChange: password.Set, Masked: true, Placeholder: "Masked", Style: t.field(), Label: "Password"}),
			core.View{Style: core.Style{Display: core.DisplayGrid, GridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))", Gap: 10},
				Children: []core.Node{
					labelled("Email", core.Input{KeyboardType: core.KeyboardEmail, Placeholder: "name@example.com", Style: t.field(), Label: "Email"}),
					labelled("Number", core.Input{KeyboardType: core.KeyboardNumber, Placeholder: "42", Style: t.field(), Label: "Number"}),
					labelled("Phone", core.Input{KeyboardType: core.KeyboardPhone, Placeholder: "+1 555 0100", Style: t.field(), Label: "Phone"}),
					labelled("URL", core.Input{KeyboardType: core.KeyboardURL, Placeholder: "https://", Style: t.field(), Label: "URL"}),
					labelled("Read-only", core.Input{DefaultValue: "Read-only text", Readonly: true, Style: t.field(), Label: "Read-only"}),
					labelled("Disabled", core.Input{DefaultValue: "Disabled", Disabled: func() bool { return true }, Style: t.field(), Label: "Disabled"}),
				}},
			t.live(func() string {
				if name.Get() == "" {
					return "Type a name above."
				}
				return fmt.Sprintf("Hello, %s. Your password has %d characters.", name.Get(), len([]rune(password.Get())))
			}),
		),
	}}
}
