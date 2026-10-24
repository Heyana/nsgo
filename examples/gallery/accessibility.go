package main

import (
	"fmt"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/ui"
)

// accessibilityPage shows what screen readers read beyond roles and
// labels. Turn on VoiceOver or TalkBack to hear it.
func accessibilityPage(cx *core.Scope, t *theme) core.Node {
	count := core.NewSignal(cx, 2)
	open := core.NewSignal(cx, false)
	lastAction := core.NewSignal(cx, "none yet")
	price := core.NewSignal(cx, 120)
	hidden := core.NewSignal(cx, true)
	added := core.NewSignal(cx, 0)
	var result core.Ref

	adjust := func(by int) {
		count.Update(func(n int) int { return min(max(n+by, 0), 10) })
	}
	quantity := core.View{Style: row, Role: core.RoleAdjustable, Label: "Quantity", Hint: "Swipe up or down to adjust",
		TestID: "a11y-quantity",
		AccessibilityValue: func() core.AccessibilityValue {
			return core.AccessibilityValue{Max: 10, Now: float32(count.Get()), Text: fmt.Sprintf("%d items", count.Get())}
		},
		AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
		OnAccessibilityAction: func(name string) {
			if name == core.ActionIncrement {
				adjust(1)
			} else {
				adjust(-1)
			}
		},
		Children: []core.Node{
			// The buttons are for the pointer; the screen reader adjusts
			// the quantity itself.
			core.View{AccessibilityElementsHidden: always, Children: []core.Node{t.plain("−", func() { adjust(-1) })}},
			core.Text{ContentFunc: func() string { return fmt.Sprintf("%d items", count.Get()) },
				Style: core.Style{FontFamily: core.FontMonospace, MinWidth: core.Pt(64), TextAlign: core.TextAlignCenter}},
			core.View{AccessibilityElementsHidden: always, Children: []core.Node{t.plain("+", func() { adjust(1) })}},
		}}

	disclosure := core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
		base.Button{Style: t.secondary(), Label: "Shipping details", Expanded: open.Get, TestID: "a11y-disclosure",
			OnPress: func(core.PressEvent) { open.Update(func(o bool) bool { return !o }) },
			Children: []core.Node{core.Text{ContentFunc: func() string {
				if open.Get() {
					return "Shipping details ▾"
				}
				return "Shipping details ▸"
			}}}},
		core.Show{When: open.Get, Then: func(*core.Scope) core.Node {
			return t.note("Ships in two days from Lisbon.")
		}},
	}}

	message := core.View{Style: core.Style{Background: t.sunken, CornerRadius: radius, Padding: core.All(12), Gap: 4},
		Accessible: true, TestID: "a11y-message",
		AccessibilityActions: []core.AccessibilityAction{
			{Name: "archive", Label: "Archive"}, {Name: "flag", Label: "Flag"},
			{Name: core.ActionActivate}, {Name: core.ActionEscape}, {Name: core.ActionMagicTap},
		},
		OnAccessibilityAction: lastAction.Set,
		Children: []core.Node{
			core.Text{Content: "Ana", Style: core.Style{FontWeight: core.FontWeightSemibold}},
			core.Text{Content: "Lunch on Friday?"},
		}}

	card := core.View{Style: core.Style{Background: t.sunken, CornerRadius: radius, Padding: core.All(12), Gap: 4},
		Accessible: true, TestID: "a11y-card", Children: []core.Node{
			core.Text{Content: "Apples", Style: core.Style{FontWeight: core.FontWeightSemibold}},
			core.Text{ContentFunc: func() string { return fmt.Sprintf("$%d.%02d a kilo", price.Get()/100, price.Get()%100) }},
		}}

	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Values and hints", "An adjustable element with a range value and a hint; the screen reader's swipes or volume keys perform its increment and decrement actions.",
			quantity),
		t.demo("Expanded state", "Expanded makes an element expandable; screen readers say whether it is expanded.",
			disclosure),
		t.demo("Actions", "Custom actions show in the screen reader's actions menu; activate, escape and magic tap replace its own gestures. Accessible makes the message one element.",
			message,
			t.live(func() string { return "Last action: " + lastAction.Get() }),
		),
		t.demo("Grouped elements", "Accessible reads the card as one element, labelled with the text inside, which follows the price.",
			card,
			t.plain("Change the price", func() { price.Update(func(p int) int { return p + 15 }) }),
		),
		t.demo("Hidden elements", "AccessibilityElementsHidden hides decoration from screen readers while it shows.",
			core.View{AccessibilityElementsHidden: hidden.Get, TestID: "a11y-hidden",
				Children: []core.Node{t.note("✦ ✦ ✦ decoration ✦ ✦ ✦")}},
			ui.Switch{Checked: hidden.Get, OnChange: hidden.Set, Label: "Hide the decoration"},
		),
		t.demo("Live regions", "A live region's changes are announced as they happen, without moving focus.",
			core.Text{LiveRegion: core.LiveRegionPolite, TestID: "a11y-live",
				ContentFunc: func() string { return fmt.Sprintf("%d in the basket", added.Get()) }},
			t.plain("Add to basket", func() { added.Update(func(n int) int { return n + 1 }) }),
		),
		t.demo("Focus", "Ref.SetAccessibilityFocus moves the screen reader to an element, as after a search.",
			base.Button{Style: t.secondary(), Label: "Search apples", TestID: "a11y-search", OnPress: func(core.PressEvent) { result.SetAccessibilityFocus() },
				Children: []core.Node{core.Text{Content: "Search apples"}}},
			core.Text{Ref: &result, Content: "3 results for “apples”", TestID: "a11y-result"},
		),
	}}
}
