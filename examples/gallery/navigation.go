package main

import (
	"fmt"
	"strings"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// insetsRow is the safe-area insets where it renders, one figure per
// edge.
func insetsRow(cx *core.Scope) core.Node {
	env := cx.Env()
	edge := func(f func(core.Edges) core.Points) func() string {
		return func() string { return fmt.Sprintf("%d pt", int(f(core.Insets(cx))+0.5)) }
	}
	_ = env
	return core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
		stat("Top", edge(func(e core.Edges) core.Points { return e.Top }), toneNeutral, core.Pct(25)),
		stat("Bottom", edge(func(e core.Edges) core.Points { return e.Bottom }), toneNeutral, core.Pct(25)),
		stat("Left", edge(func(e core.Edges) core.Points { return e.Left }), toneNeutral, core.Pct(25)),
		stat("Right", edge(func(e core.Edges) core.Points { return e.Right }), toneNeutral, core.Pct(25)),
	}}
}

// navPage is the body of a pushed screen: what it shows, then where its
// safe area is.
func navPage(cx *core.Scope, intro string, children ...core.Node) core.Node {
	return core.Scroll{Style: core.Style{FlexGrow: 1, Background: colors.background}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Edges{Top: 16, Right: 16, Bottom: 40, Left: 16}, Gap: 16}, Children: []core.Node{
			appear(0, false, core.Text{Content: intro, Style: core.Style{FontSize: 15, LineHeight: 21, Color: colors.muted, Padding: core.Symmetric(0, 4)}}),
			appear(1, false, section("Safe area", "Insets", insetsRow(cx))),
			appear(2, false, core.View{Style: core.Style{Gap: 16}, Children: children}),
		}},
	}}
}

func largeTitlePage(cx *core.Scope) core.Screen {
	var rows []core.Node
	for i := 1; i <= 20; i++ {
		s := core.Style{Height: core.Pt(48), Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.Symmetric(0, 10)}
		if i > 1 {
			s.Borders, s.BorderColors = core.Edges{Top: 1}, core.EdgeColors{Top: colors.line}
		}
		rows = append(rows, core.View{Style: s, Children: []core.Node{
			core.Text{Content: fmt.Sprintf("Row %d", i), Style: core.Style{FlexGrow: 1, FontSize: 15, Color: colors.text}},
			text(fmt.Sprintf("%d pt", i*48), 13, regular, colors.faint),
		}})
	}
	s := screen("Large title", nil, navPage(cx, "iOS: the title shrinks into the bar as the content scrolls.",
		card(cardProps{flush: true}, inset(insetProps{exact: true}, rows...))))
	s.HeaderLargeTitle = true
	return s
}

func transparentPage(cx *core.Scope) core.Screen {
	bg := core.RGB(0xf8ece3)
	if isDark {
		bg = core.RGB(0x21160f)
	}
	return core.Screen{Title: "Transparent", HeaderTransparent: true, HeaderStyle: core.Style{TintColor: colors.text, Color: colors.text},
		Header: pageHeader(pageHeaderProps{title: "Transparent", transparent: true, tint: colors.text}),
		Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, Background: bg, Overflow: core.Hidden, Padding: core.Edges{Top: 16, Right: 16, Bottom: 56, Left: 16},
			JustifyContent: core.JustifyEnd, Gap: 16}, Children: []core.Node{
			core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(-90), Right: core.Pt(-70), Width: core.Pt(320), Height: core.Pt(320), CornerRadius: 160, Background: tints.accent}},
			core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(150), Left: core.Pt(-110), Width: core.Pt(260), Height: core.Pt(260), CornerRadius: 130, Background: tints.info}},
			appear(0, false, core.View{Style: core.Style{Gap: 12}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{badge("HeaderTransparent", toneAccent)}},
				core.Text{Content: "Content under the header", Style: core.Style{FontSize: 30, FontWeight: bold, LetterSpacing: -0.5, Color: colors.text}},
				core.Text{Content: "The circles start at the top of the screen; the bar and its back button float over them.", Style: core.Style{FontSize: 15, LineHeight: 21, Color: colors.muted}},
			}}),
			appear(1, false, inset(insetProps{}, insetsRow(cx))),
		}}}}
}

func customHeaderPage(cx *core.Scope) core.Screen {
	likes := core.NewSignal(cx, 0)
	// Each call makes the elements anew: the system header (phones) and
	// the app's (desktops, the web) each mount their own.
	left := func() core.Node {
		return core.Text{Content: "Close", OnPress: func(core.PressEvent) { shell.nav.Pop(1) }, Role: core.RoleButton,
			Style: core.Style{Color: colors.onAccent, FontSize: 17, FontWeight: medium}}
	}
	center := func() core.Node {
		return hrow(7,
			core.View{Style: core.Style{Width: core.Pt(10), Height: core.Pt(10), CornerRadius: 5, Background: colors.onAccent}},
			text("Custom title", 17, bold, colors.onAccent))
	}
	right := func() core.Node {
		return core.View{OnPress: func(core.PressEvent) { likes.Update(func(n int) int { return n + 1 }) }, Role: core.RoleButton, Label: "Star", HitSlop: core.All(10),
			Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 5}, Children: []core.Node{
				icon(iconStar, 18, colors.onAccent),
				textf(func() string { return itoa(likes.Get()) }, 17, semibold, colors.onAccent),
			}}
	}
	return core.Screen{Title: "Custom", HeaderStyle: core.Style{TintColor: colors.onAccent, Background: colors.accentFill, Color: colors.onAccent},
		HeaderBackHidden: true, HeaderLeft: left(), HeaderTitle: center(), HeaderRight: right(),
		Header: pageHeader(pageHeaderProps{title: "Custom", background: colors.accentFill, left: left(), center: center(), right: right()}),
		Children: []core.Node{navPage(cx, "The header's items are views: the star on the right counts presses and updates the header.",
			section("Header", "HeaderRight", figure(func() string { return itoa(likes.Get()) }, "", func() string {
				if likes.Get() == 1 {
					return "star"
				}
				return "stars"
			}, toneNeutral)))}}
}

func lockedPage(cx *core.Scope) core.Screen {
	s := screen("No swipe back", nil, navPage(cx, hint(cx,
		"The back gesture is off on this screen: no swipe on iOS. The back buttons still work.",
		"The back gesture is off on this screen (touch screens only). The back button and Escape still work."),
		section("Leaving", "NoPopOnSwipe", core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
			stat("Swipe back", label("Off"), toneDanger, core.Pct(50)),
			stat("Back buttons", label("On"), toneUp, core.Pct(50)),
		}})))
	s.NoPopOnSwipe = true
	return s
}

// unsavedPage asks before leaving with unsaved text.
func unsavedPage(cx *core.Scope) core.Screen {
	draft := core.NewSignal(cx, "")
	confirm := func() {
		base.ShowAlertDialog(cx, base.AlertDialog{Title: "Discard your changes?", Actions: []core.AlertAction{
			{Label: "Keep editing", Role: core.AlertCancel},
			{Label: "Discard", Role: core.AlertDestructive},
		}}, func(label string) {
			if label == "Discard" {
				shell.nav.Pop(1)
			}
		})
	}
	back := func() {
		if draft.GetUntracked() != "" {
			confirm()
		} else {
			shell.nav.Pop(1)
		}
	}
	s := core.Screen{Title: "Unsaved changes", HeaderStyle: headerStyle(), NoPopOnBack: true, OnBackPress: back,
		Header: pageHeader(pageHeaderProps{title: "Unsaved changes", onBack: back}),
		Children: []core.Node{navPage(cx, hint(cx,
			"Type something, then go back (header button, swipe, Android back): the screen asks first.",
			"Type something, then go back (header button, Escape, the mouse's back button): the screen asks first."),
			card(cardProps{icon: iconForm, title: "Draft", subtitle: "NoPopOnBack + OnBackPress", right: core.Dynamic(func(*core.Scope) core.Node {
				if draft.Get() != "" {
					return badge("Guarded", toneWarn)
				}
				return badge("Empty", toneNeutral)
			})},
				darkField(core.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "A draft", TestID: "draft"}),
				notef(func() string {
					if n := len([]rune(draft.Get())); n > 0 {
						return fmt.Sprintf("%d characters would be lost: back asks first", n)
					}
					return "Nothing to lose: back pops"
				}),
			))}}
	return s
}

func headerlessPage(cx *core.Scope) core.Screen {
	return core.Screen{HeaderHidden: true, Children: []core.Node{base.SafeAreaView{Edges: base.Top | base.Bottom | base.Left | base.Right,
		Style: core.Style{FlexGrow: 1, Background: colors.background}, Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(16), Gap: 16}, Children: []core.Node{
				appear(0, false, core.View{Style: core.Style{Gap: 8, Padding: core.Edges{Top: 12, Right: 4, Left: 4}}, Children: []core.Node{
					core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{badge("HeaderHidden", toneAccent)}},
					core.Text{Content: "No header", Style: core.Style{FontSize: 30, FontWeight: bold, LetterSpacing: -0.5, Color: colors.text}},
					core.Text{Content: "The screen owns the whole surface; a SafeAreaView keeps it clear of the status bar and the home indicator.",
						Style: core.Style{FontSize: 15, LineHeight: 21, Color: colors.muted}},
				}}),
				appear(1, false, section("Safe area", "SafeAreaView", insetsRow(cx))),
				spacer(),
				button(buttonProps{label: "Back"}, func() { shell.nav.Pop(1) }),
			}},
		}}}}
}

func deepPage(depth int) func(cx *core.Scope) core.Screen {
	return func(cx *core.Scope) core.Screen {
		var levels []core.Node
		for level := 1; level <= depth; level++ {
			on := level == depth
			s := core.Style{Width: core.Pt(32), Height: core.Pt(32), CornerRadius: 8, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}
			fg := colors.muted
			if on {
				s = fin.accent.over(s)
				s.Background, fg = colors.accentFill, colors.onAccent
			} else {
				s = fin.raised.over(s)
				s.BorderWidth, s.BorderColor, s.Background = 1, colors.border, colors.raised
			}
			levels = append(levels, core.View{Style: s, Children: []core.Node{text(itoa(level), 13, semibold, fg)}})
		}
		title := fmt.Sprintf("Level %d", depth)
		return screen(title, nil, navPage(cx, "Every push is a native screen; PopToRoot unwinds them all at once.",
			card(cardProps{icon: iconNav, title: title, subtitle: fmt.Sprintf("%d deep in the stack", depth)},
				core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 6}, Children: levels},
				core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
					button(buttonProps{label: "Deeper", icon: iconPlus, flex: 1}, func() { shell.push(deepPage(depth + 1)) }),
					button(buttonProps{label: "To the index", variant: secondary, flex: 1}, func() { shell.nav.PopToRoot() }),
				}},
			)))
	}
}

type modalKind struct {
	key, title, note string
	icon             iconName
}

// modalBody is a modal's content: what it is, its safe area and a close
// button.
func modalBody(cx *core.Scope, title, noteText func() string, body func() string, onClose func()) core.Node {
	return base.SafeAreaView{Edges: base.Top | base.Bottom | base.Left | base.Right, Style: core.Style{FlexGrow: 1, Background: colors.background}, Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(16), Gap: 16}, Children: []core.Node{
			core.View{Style: core.Style{Gap: 8, Padding: core.Edges{Top: 8, Right: 4, Left: 4}}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{badgef(noteText, toneAccent, false)}},
				core.Text{ContentFunc: title, Style: core.Style{FontSize: 28, FontWeight: bold, LetterSpacing: -0.5, Color: colors.text}},
				core.Text{ContentFunc: body, Style: core.Style{FontSize: 15, LineHeight: 21, Color: colors.muted}},
			}},
			section("Safe area", "the modal's", insetsRow(cx)),
			spacer(),
			button(buttonProps{label: "Close"}, onClose),
		}},
	}}
}

// navigationDemo is base.Nav's screens and their options, core.Modal's
// presentations through base.Dialog, and the back button.
func navigationDemo(cx *core.Scope) core.Node {
	open := core.NewSignal(cx, "")
	intercept := core.NewSignal(cx, false)
	backs, cancels := core.NewSignal(cx, 0), core.NewSignal(cx, 0)
	closeModal := func() { open.Set("") }
	modals := []modalKind{
		{"fullScreen", "Full screen", "Slides up", iconMobile},
		{"pageSheet", "Page sheet", hint(cx, "iOS sheet", "A tall centered card"), iconComponents},
		{"formSheet", "Form sheet", hint(cx, "iOS, centered", "A smaller centered card"), iconForm},
		{"fade", "Fade", "Fades in", iconSpark},
		{"dialog", "Dialog", "See-through", iconBell},
		{"kept", "Kept open", hint(cx, "No swipe to close", "Escape does not close"), iconArrowDown},
	}
	// A handler hears back before the stack pops; it lives only while
	// intercepting, in a scope of its own.
	var handler *core.Scope
	cx.Effect(func() {
		on := intercept.Get()
		if handler != nil {
			handler.Dispose()
			handler = nil
		}
		if on {
			handler = cx.Child()
			handler.OnBackPress(func() bool {
				backs.Update(func(n int) int { return n + 1 })
				return true
			})
		}
	})
	// The dialog springs up from a little smaller each time it opens.
	pop := core.NewAnimatedValue(cx, 0)
	cx.Effect(func() {
		if open.Get() != "dialog" {
			return
		}
		pop.Set(0)
		spring(cx, pop, 1, settleSpring)
	})
	current := func() modalKind {
		for _, m := range modals {
			if m.key == open.Get() {
				return m
			}
		}
		return modalKind{}
	}
	platformName := hint(cx, "UIKit / View", "Native views")
	if cx.Env().OS() == core.OSLinux {
		platformName = "GTK 4"
	}

	stack := func(cx *core.Scope) core.Node {
		return card(cardProps{icon: iconNav, title: "Native stack", subtitle: "Headers, transitions, the safe area", right: badge(platformName, toneNeutral)}, insetsRow(cx))
	}
	pages := func(cx *core.Scope) core.Node {
		return card(cardProps{title: "Pages", subtitle: "Screen options, pushed with Nav.Push", flush: true},
			inset(insetProps{exact: true},
				linkRowFirst(true, iconText, "Large title", "HeaderLargeTitle", func() { shell.push(largeTitlePage) }),
				linkRowFirst(false, iconStyles, "Transparent header", "HeaderTransparent", func() { shell.push(transparentPage) }),
				linkRowFirst(false, iconStar, "Custom header items", "HeaderLeft / Title / Right", func() { shell.push(customHeaderPage) }),
				linkRowFirst(false, iconMobile, "No header", "HeaderHidden", func() { shell.push(headerlessPage) }),
				linkRowFirst(false, iconGestures, "No swipe back", "NoPopOnSwipe", func() { shell.push(lockedPage) }),
				linkRowFirst(false, iconForm, "Unsaved changes", "NoPopOnBack + OnBackPress", func() { shell.push(unsavedPage) }),
				linkRowFirst(false, iconLists, "A deep stack", "Push / PopToRoot", func() { shell.push(deepPage(1)) }),
			))
	}
	modalsCard := func(cx *core.Scope) core.Node {
		var tiles []core.Node
		for _, m := range modals {
			tiles = append(tiles, press(pressProps{onPress: func() { open.Set(m.key) }, label: m.title, style: core.Style{Width: core.Pct(48.5)}},
				core.View{Style: fin.inset.over(core.Style{MinHeight: core.Pt(100), Padding: core.All(12), Gap: 12, CornerRadius: radiusInset, BorderCurve: core.BorderCurveContinuous,
					Background: colors.inset, BorderWidth: 1, BorderColor: colors.line}), Children: []core.Node{
					iconTile(m.icon, 32),
					core.View{Style: core.Style{Gap: 2}, Children: []core.Node{text(m.title, 15, medium, colors.text), text(m.note, 13, regular, colors.faint)}},
				}}))
		}
		return card(cardProps{title: "Modals", subtitle: "Presentation styles and transitions", flush: true},
			core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, JustifyContent: core.JustifySpaceBetween, RowGap: 8}, Children: tiles})
	}
	backCard := func(cx *core.Scope) core.Node {
		return card(cardProps{icon: iconSystem, title: "Back button", subtitle: hint(cx, "OnBackPress: the back button and gesture", "OnBackPress: Escape, the mouse's back button"),
			right: toggle(intercept.Get, intercept.Set, "Intercept the back button")},
			figure(func() string { return itoa(backs.Get()) }, "", func() string {
				unit := "presses"
				if backs.Get() == 1 {
					unit = "press"
				}
				if intercept.Get() {
					return unit + " caught"
				}
				return unit + " popped"
			}, toneAccent),
			notef(func() string {
				if intercept.Get() {
					return "The handler returns true: back presses stay on this screen."
				}
				return hint(cx, "Off: the back button pops this screen.", "Off: Escape pops this screen, or the app's last screen does nothing.")
			}),
		)
	}

	isShown := func(keys ...string) func() bool {
		return func() bool {
			return strings.Contains(" "+strings.Join(keys, " ")+" ", " "+open.Get()+" ") && open.Get() != ""
		}
	}
	modalsNodes := []core.Node{
		core.Dynamic(func(cx *core.Scope) core.Node {
			k := open.Get()
			presentation, animation := core.ModalFullScreen, core.ModalSlide
			switch k {
			case "pageSheet":
				presentation = core.ModalPageSheet
			case "formSheet":
				presentation = core.ModalFormSheet
			case "fade":
				animation = core.ModalFade
			}
			if !isShown("fullScreen", "pageSheet", "formSheet", "fade")() {
				return core.Group{}
			}
			return base.Dialog{Open: func() bool { return true }, Presentation: presentation, Animation: animation, Label: current().title,
				OnOpenChange: func(o bool, _ base.Reason) {
					if !o {
						closeModal()
					}
				},
				PopupStyle: core.Style{FlexGrow: 1, AlignSelf: core.AlignStretch},
				Children: []core.Node{modalBody(cx, func() string { return current().title }, func() string { return k }, func() string {
					if k == "pageSheet" || k == "formSheet" {
						return hint(cx, "Close with the button, the back button (Android) or a swipe down (iOS sheets): it closes on the platform, then tells the app.",
							"Close with the button, Escape or a click outside the card.")
					}
					return hint(cx, "Close with the button or the back button (Android): it closes on the platform, then tells the app.", "Close with the button or Escape.")
				}, closeModal)}}
		}),
		base.Dialog{Open: isShown("kept"), Presentation: core.ModalPageSheet, NoCloseOnEscape: true, NoCloseOnBackdropPress: true, NoCloseOnSwipe: true,
			OnCancel: func() { cancels.Update(func(n int) int { return n + 1 }) }, Label: "Kept open",
			PopupStyle: core.Style{FlexGrow: 1, AlignSelf: core.AlignStretch},
			Children: []core.Node{modalBody(cx, label("Kept open"), label("NoCloseOn…"), func() string {
				return fmt.Sprintf("%s only call OnCancel (%d so far): the button closes it.", hint(cx, "Swiping it down (iOS) and back (Android)", "Escape and clicks outside"), cancels.Get())
			}, closeModal)}},
		base.Dialog{Open: isShown("dialog"), Animation: core.ModalFade, OnOpenChange: func(o bool, _ base.Reason) {
			if !o {
				closeModal()
			}
		}, Label: "A dialog", PopupStyle: core.Style{FlexGrow: 1, AlignSelf: core.AlignStretch},
			Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, Background: black(0.62), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(16)},
				Children: []core.Node{core.View{Style: core.Style{AlignSelf: core.AlignStretch, MaxWidth: core.Pt(480), Animated: &core.AnimatedStyle{Opacity: pop, Scale: lerp(pop, 0.9, 1)}},
					Children: []core.Node{card(cardProps{icon: iconBell, title: "A dialog", subtitle: "Animation: ModalFade"},
						core.Text{Content: "A dialog owns no look: what is behind it shows through, and the card springs in on the platform.",
							Style: core.Style{FontSize: 15, LineHeight: 21, Color: colors.muted}},
						core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
							button(buttonProps{label: "Cancel", variant: secondary, flex: 1}, closeModal),
							button(buttonProps{label: "OK", flex: 1}, closeModal),
						}},
					)}}}}}},
	}

	return demoPage(cx, pageProps{after: modalsNodes}, stack, pages, modalsCard, backCard)
}
