package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"

	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/ui"
)

// keyboardDemo is what a phone's keyboard does to the page: its height,
// dragging the page hiding it, and sheets and bars that ride above it.
func keyboardDemo(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	colors := ui.ColorsOf(cx)
	env := cx.Env()
	mode := core.NewSignal(cx, core.KeyboardDismissOnDrag)
	sheetOpen := core.NewSignal(cx, false)

	keyboard := func(cx *core.Scope) core.Node {
		tr := locales.For(cx)
		modeName := map[core.KeyboardDismissMode]string{core.KeyboardDismissNone: "none", core.KeyboardDismissOnDrag: "on-drag", core.KeyboardDismissInteractive: "interactive"}
		return Card{Title: tr.FormKeyboard(), Description: "KeyboardDismissMode while dragging", Actions: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			h := env.KeyboardHeight()
			t := ToneNeutral
			if h > 0 {
				t = ToneAccent
			}
			return ui.Badge{Text: fmt.Sprintf("%d pt", int(h+0.5)), Variant: t.Badge()}
		})}, Children: []core.Node{
			ui.Tabs[core.KeyboardDismissMode]{Equal: true, Options: []ui.Option[core.KeyboardDismissMode]{{Value: core.KeyboardDismissNone, Label: tr.FormNone()}, {Value: core.KeyboardDismissOnDrag, Label: tr.FormOnDrag()},
				{Value: core.KeyboardDismissInteractive, Label: tr.FormInteractive()}}, Value: mode.Get, OnChange: mode.Set},
			ui.Input{TestID: "keyboard-input", Placeholder: tr.CompTypeHere()},
			ui.Button{Text: tr.FormHideKeyboard(), Variant: ui.ButtonSecondary, OnPress: cx.DismissKeyboard},
		}, Footer: []core.Node{
			Caption{Label: Hint(cx, tr.FormDragTheFormWithThe(), tr.FormDesktopsHaveNoOnScreen()), Note: func() string { return modeName[mode.Get()] }},
		}}
	}

	more := func(cx *core.Scope) core.Node {
		tr := locales.For(cx)
		return Card{Title: tr.FormMoreKeyboards(), Description: tr.FormSheetsAndBarsThatRide(), Flush: true, Children: []core.Node{
			LinkRow{Icon: lucide.Component, Title: tr.FormInputsInASheet(), Note: "Presentation: ModalPageSheet", OnPress: func() { sheetOpen.Set(true) }},
			LinkRow{Divided: true, Icon: lucide.Type, Title: tr.FormAMessageBarAtThe(), Note: "PaddingEnv: EnvKeyboard", OnPress: func() { shell.push(messageBar) }},
		}}
	}

	var fields []core.Node
	for _, l := range []string{tr.FormFirst(), tr.FormSecond(), tr.FormThird(), tr.FormFourth(), tr.FormFifth(), tr.FormSixth(), tr.FormSeventh()} {
		fields = append(fields, formField(fieldProps{label: l}, base.Input{Placeholder: l + tr.FormField()}))
	}
	dialog := base.Dialog{Open: sheetOpen.Get, Presentation: core.ModalPageSheet, OnOpenChange: func(o bool, _ base.Reason) { sheetOpen.Set(o) },
		PopupStyle: core.Style{FlexGrow: 1, AlignSelf: core.AlignStretch}, Label: tr.FormASheet(),
		Children: []core.Node{core.View{Style: core.Style{PaddingEnv: core.EnvAll(core.EnvSafeArea), FlexGrow: 1}, StyleFunc: ui.Filled(colors.Background), Children: []core.Node{
			core.Scroll{Style: core.Style{FlexGrow: 1}, Behavior: core.ScrollBehavior{KeyboardDismissMode: core.KeyboardDismissInteractive}, Children: []core.Node{
				core.View{Style: core.Style{Padding: core.Edges{Top: 16, Right: 16, Bottom: 40, Left: 16}, Gap: 16}, Children: []core.Node{
					core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Edges{Top: 8, Left: 4}}, Children: []core.Node{
						core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
							core.Text{Content: tr.FormASheet(), Style: core.Style{FontSize: 24, FontWeight: ui.Bold, LetterSpacing: -0.4}, StyleFunc: ui.Colored(colors.Text)},
							ui.Text(tr.FormItsInputsStayAboveThe(), 13, ui.Regular, colors.Muted),
						}},
						ui.Button{Text: tr.FormDone(), Variant: ui.ButtonSecondary, OnPress: func() { sheetOpen.Set(false) }},
					}},
					Inset{Gap: 10, Children: fields},
				}},
			}},
		}}}}

	return demoPage(cx, pageProps{behaviorFunc: func() core.ScrollBehavior { return core.ScrollBehavior{KeyboardDismissMode: mode.Get()} }, after: []core.Node{dialog}},
		keyboard, more)
}

type message struct {
	mine bool
	text string
}

// messageBar is an input pinned to the bottom, outside any scroll view:
// it rides on the keyboard.
func messageBar(cx *core.Scope) base.NavPage {
	tr := locales.For(cx)
	colors := ui.ColorsOf(cx)
	fin := ui.FinOf(cx)
	env := cx.Env()
	messages := core.NewSignalFunc(cx, []message{
		{false, tr.FormTheBarBelowSitsOutside()},
		{false, tr.FormBottomPaddingOfEnvkeyboardLifts()},
	}, slices.Equal)
	draft := core.NewSignal(cx, "")
	send := func() {
		d := draft.GetUntracked()
		if strings.TrimSpace(d) == "" {
			return
		}
		messages.Update(func(m []message) []message { return append(slices.Clone(m), message{true, d}) })
		draft.Set("")
	}
	return screen(cx, tr.FormMessageBar(), nil, core.View{Style: core.Style{FlexGrow: 1, PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard}}, StyleFunc: ui.Filled(colors.Background), Children: []core.Node{
		core.Scroll{Style: core.Style{FlexGrow: 1}, Behavior: core.ScrollBehavior{KeyboardDismissMode: core.KeyboardDismissInteractive}, Children: []core.Node{
			core.View{Style: core.Style{Padding: core.All(16), Gap: 8}, Children: []core.Node{
				core.Text{Content: tr.FormToday(), Style: core.Style{AlignSelf: core.AlignCenter, FontSize: 12, FontWeight: ui.Medium, Padding: core.Edges{Bottom: 8}}, StyleFunc: ui.Colored(colors.Faint)},
				core.Each(messages.Get, func(m message) message { return m }, func(cx *core.Scope, m func() message, _ func() int) core.Node {
					msg := core.Untracked(cx, m)
					return Appear{Children: []core.Node{bubble(cx, msg)}}
				}),
			}},
		}},
		core.View{StyleFunc: func() core.Style {
			bottom := core.Points(8)
			// Above the home indicator, unless the keyboard covers it.
			if env.KeyboardHeight() == 0 {
				bottom += env.SafeArea().Bottom
			}
			return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Edges{Top: 8, Right: 16, Bottom: bottom, Left: 16},
				Background: colors.Surface(), Borders: core.Edges{Top: 1}, BorderColors: core.EdgeColors{Top: colors.Border()}}
		}, Children: []core.Node{
			base.Input{Value: draft.Get, OnChange: draft.Set, OnSubmit: func(string) { send() }, SubmitBehavior: core.SubmitBehaviorSubmit,
				Placeholder: tr.FormMessage(), ReturnKeyType: core.ReturnKeySend, TestID: "message",
				StyleFunc: func() core.Style {
					return fin.Inset().Over(core.Style{PlaceholderColor: colors.Faint(), SelectionColor: colors.Accent(), FlexGrow: 1, FlexShrink: 1, Height: core.Pt(44),
						Padding: core.Symmetric(0, 16), FontSize: 13, Color: colors.Text(), Background: colors.Inset(), BorderWidth: 1, BorderColor: colors.Border(), CornerRadius: 22})
				}},
			ui.IconButton{Variant: ui.IconButtonPrimary, Icon: lucide.ArrowUp, Label: tr.FormSend(), OnPress: send},
		}},
	}})
}

// bubble is a chat message: the accent on the right for mine, raised on the
// left for theirs.
func bubble(cx *core.Scope, m message) core.Node {
	colors := ui.ColorsOf(cx)
	fin := ui.FinOf(cx)
	fg := colors.Text
	if m.mine {
		fg = colors.OnAccent
	}
	return core.View{StyleFunc: func() core.Style {
		s := core.Style{AlignSelf: core.AlignStart, MaxWidth: core.Pct(80), Padding: core.Symmetric(10, 14), CornerRadii: core.Corners{TopLeft: 18, TopRight: 18, BottomRight: 18, BottomLeft: 6},
			BorderCurve: core.BorderCurveContinuous, Background: colors.Raised()}
		if m.mine {
			s = fin.Accent().Over(s)
			s.AlignSelf, s.Background, s.CornerRadii = core.AlignEnd, colors.AccentFill(), core.Corners{TopLeft: 18, TopRight: 18, BottomRight: 6, BottomLeft: 18}
		} else {
			s = fin.Raised().Over(s)
		}
		return s
	}, Children: []core.Node{core.Text{Content: m.text, Style: core.Style{FontSize: 13, LineHeight: 20}, StyleFunc: ui.Colored(fg)}}}
}
