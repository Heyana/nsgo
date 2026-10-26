package main

import (
	"fmt"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

func always() bool { return true }

// accessibilityDemo is what screen readers read beyond roles and labels:
// values and hints, expanded states, actions, groups, hidden elements,
// live regions and focus. Turn on VoiceOver or TalkBack to hear it. NSH's
// gallery shows accessibility within its other demos; NSGo gives it a page.
func accessibilityDemo(cx *core.Scope) core.Node {
	values := func(cx *core.Scope) core.Node {
		n := core.NewSignal(cx, 2)
		adjust := func(by int) { n.Update(func(v int) int { return min(max(v+by, 0), 10) }) }
		step := func(name iconName, by int, label string) core.Node {
			// The buttons are for the pointer; the screen reader adjusts
			// the quantity itself.
			return core.View{AccessibilityElementsHidden: always, Children: []core.Node{roundButton(name, label, secondary, 36, func() { adjust(by) })}}
		}
		return card(cardProps{icon: iconAccess, title: "Values and hints", subtitle: "An adjustable element: swipe up or down"},
			core.View{Role: core.RoleAdjustable, Label: "Quantity", Hint: "Swipe up or down to adjust", TestID: "a11y-quantity",
				AccessibilityValue: func() core.AccessibilityValue {
					return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d items", n.Get())}
				},
				AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
				OnAccessibilityAction: func(name string) {
					if name == core.ActionIncrement {
						adjust(1)
					} else {
						adjust(-1)
					}
				},
				Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 16},
				Children: []core.Node{
					step(iconMinus, -1, "Fewer"),
					figure(func() string { return itoa(n.Get()) }, "items", nil, toneNeutral),
					step(iconPlus, 1, "More"),
				}},
			meter(func() float64 { return float64(n.Get()) / 10 }, toneAccent, -1),
			apiLine("A range value and the increment and decrement actions", "RoleAdjustable"),
		)
	}

	expanded := func(cx *core.Scope) core.Node {
		open := core.NewSignal(cx, false)
		return card(cardProps{icon: iconAccess, title: "Expanded state", subtitle: "Screen readers say whether it is expanded"},
			base.Button{Label: "Shipping details", Expanded: open.Get, TestID: "a11y-disclosure", OnPress: func(core.PressEvent) { toggled(open) },
				Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, MinHeight: core.Pt(44), Active: &core.Style{Opacity: core.Alpha(0.7)}},
				Children: []core.Node{
					core.Text{Content: "Shipping details", Style: core.Style{FlexGrow: 1, FontSize: 15, Color: colors.text}},
					core.View{StyleFunc: func() core.Style {
						s := core.Style{Transition: core.Transition{Duration: ms(150), Properties: core.TransitionTransform}}
						if open.Get() {
							s.Transform.Rotate = 90
						}
						return s
					}, Children: []core.Node{icon(iconChevron, 16, colors.muted)}},
				}},
			core.Show{When: open.Get, Then: func(*core.Scope) core.Node { return note("Ships in two days from Lisbon.") }},
			apiLine("A disclosure", "Expanded"),
		)
	}

	actions := func(cx *core.Scope) core.Node {
		last := core.NewSignal(cx, "none yet")
		return card(cardProps{icon: iconAccess, title: "Actions", subtitle: "In the screen reader's actions menu"},
			core.View{Accessible: true, TestID: "a11y-message",
				AccessibilityActions: []core.AccessibilityAction{
					{Name: "archive", Label: "Archive"}, {Name: "flag", Label: "Flag"},
					{Name: core.ActionActivate}, {Name: core.ActionEscape}, {Name: core.ActionMagicTap},
				},
				OnAccessibilityAction: last.Set,
				Style:                 fin.raised.over(core.Style{Background: colors.raised, CornerRadius: radiusControl, BorderWidth: 1, BorderColor: colors.border, Padding: core.All(12), Gap: 4}),
				Children:              []core.Node{text("Ana", 15, semibold, colors.text), text("Lunch on Friday?", 15, regular, colors.muted)}},
			caption("Last action", last.Get),
			apiLine("Custom ones, activate, escape and magic tap", "AccessibilityActions"),
		)
	}

	grouped := func(cx *core.Scope) core.Node {
		price := core.NewSignal(cx, 120)
		return card(cardProps{icon: iconAccess, title: "Grouped elements", subtitle: "One element, labelled with the text inside"},
			core.View{Accessible: true, TestID: "a11y-card",
				Style: fin.raised.over(core.Style{Background: colors.raised, CornerRadius: radiusControl, BorderWidth: 1, BorderColor: colors.border, Padding: core.All(12), Gap: 4}),
				Children: []core.Node{
					text("Apples", 15, semibold, colors.text),
					textf(func() string { return fmt.Sprintf("$%d.%02d a kilo", price.Get()/100, price.Get()%100) }, 15, regular, colors.muted),
				}},
			button(buttonProps{label: "Change the price", variant: secondary}, func() { price.Update(func(p int) int { return p + 15 }) }),
		)
	}

	hidden := func(cx *core.Scope) core.Node {
		h := core.NewSignal(cx, true)
		return card(cardProps{icon: iconAccess, title: "Hidden elements", subtitle: "Decoration screen readers skip"},
			core.View{AccessibilityElementsHidden: h.Get, TestID: "a11y-hidden", Style: core.Style{AlignItems: core.AlignCenter},
				Children: []core.Node{text("✦ ✦ ✦  decoration  ✦ ✦ ✦", 15, regular, colors.faint)}},
			switchRow("Hide the decoration", "AccessibilityElementsHidden", "", h.Get, h.Set),
		)
	}

	live := func(cx *core.Scope) core.Node {
		added := core.NewSignal(cx, 0)
		return card(cardProps{icon: iconAccess, title: "Live regions", subtitle: "Changes announced without moving focus"},
			core.Text{LiveRegion: core.LiveRegionPolite, TestID: "a11y-live", ContentFunc: func() string { return fmt.Sprintf("%d in the basket", added.Get()) },
				Style: core.Style{FontSize: 17, FontWeight: medium, Color: colors.text}},
			button(buttonProps{label: "Add to basket", icon: iconPlus}, func() { added.Update(func(n int) int { return n + 1 }) }),
			apiLine("Announced as it changes", "LiveRegionPolite"),
		)
	}

	focus := func(cx *core.Scope) core.Node {
		result := &core.Ref{}
		return card(cardProps{icon: iconAccess, title: "Focus", subtitle: "Moving the screen reader, as after a search"},
			button(buttonProps{label: "Search apples", variant: secondary, testID: "a11y-search"}, result.SetAccessibilityFocus),
			core.Text{Ref: result, Content: "3 results for “apples”", TestID: "a11y-result", Style: core.Style{FontSize: 15, Color: colors.text}},
			apiLine("Moves the reader here", "Ref.SetAccessibilityFocus"),
		)
	}

	return demoPage(cx, pageProps{}, values, expanded, actions, grouped, hidden, live, focus)
}
