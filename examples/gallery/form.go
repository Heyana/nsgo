package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// fieldProps are a labelled input's: api names the property it shows,
// hint is a plain note, error turns it red, prefix goes before the text
// and next is the input return focuses.
type fieldProps struct {
	label, api, prefix string
	hint               func() string
	err                func() string
	next               *core.Ref
}

// formField is a labelled input: the border glows accent while focused
// (a ring fading in on the platform), an error turns it red, slides a
// message in (layout animation) and shakes the field.
func formField(p fieldProps, in base.Input) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		focus := core.NewAnimatedValue(cx, 0)
		shake := core.NewAnimatedValue(cx, 0)
		errText := p.err
		if errText == nil {
			errText = func() string { return "" }
		}
		shown := false
		cx.Effect(func() {
			has := errText() != ""
			if has == shown {
				return
			}
			shown = has
			cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)
			if has {
				var steps []*core.Animation
				for _, x := range []float64{9, -8, 6, -4, 2, 0} {
					steps = append(steps, core.Timing(shake, x, core.TimingConfig{Duration: 45 * time.Millisecond}))
				}
				cx.StartAnimation(core.Sequence(steps...), nil)
			}
		})
		if p.next != nil {
			in.ReturnKeyType = core.ReturnKeyNext
			in.OnSubmit = func(string) { p.next.Focus() }
		} else if in.ReturnKeyType == 0 {
			in.ReturnKeyType = core.ReturnKeyDone
		}
		onFocus, onBlur := in.OnFocus, in.OnBlur
		in.OnFocus = func() {
			timing(cx, focus, 1, core.TimingConfig{Duration: 160 * time.Millisecond})
			if onFocus != nil {
				onFocus()
			}
		}
		in.OnBlur = func() {
			timing(cx, focus, 0, core.TimingConfig{Duration: 220 * time.Millisecond})
			if onBlur != nil {
				onBlur()
			}
		}
		fg := colors.text
		if in.Readonly {
			fg = colors.muted
		}
		box := core.Style{Height: core.Pt(46)}
		if in.Multiline {
			box = core.Style{MinHeight: core.Pt(46), Padding: core.Edges{Top: 13, Bottom: 13}}
		}
		own, ownFunc := in.Style, in.StyleFunc
		in.Style = core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.Symmetric(0, 14), FontSize: 15, Color: fg,
			PlaceholderColor: colors.faint, SelectionColor: colors.accent}.Merge(box)
		if in.Multiline {
			in.Style.Padding = core.Edges{Top: 13, Right: 14, Bottom: 13, Left: 14}
		}
		in.Style = in.Style.Merge(own)
		in.StyleFunc = ownFunc

		head := []core.Node{core.Text{Content: p.label, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 13, FontWeight: medium, Color: colors.muted}}}
		if p.api != "" {
			head = append(head, core.Text{Content: p.api, Style: core.Style{FontSize: 12, Color: colors.faint, FontFamily: mono}})
		}
		if p.hint != nil {
			head = append(head, textf(p.hint, 13, regular, colors.faint))
		}
		var well []core.Node
		if p.prefix != "" {
			well = append(well, core.Text{Content: p.prefix, Style: core.Style{Padding: core.Edges{Left: 14}, FontSize: 15, Color: colors.muted}})
		}
		well = append(well, in, core.View{PointerEvents: core.PointerEventsNone, StyleFunc: func() core.Style {
			c := colors.accent
			if errText() != "" {
				c = colors.danger
			}
			// Around the field's border: its radius plus the 1 pt it
			// reaches out.
			return core.Style{Position: core.Absolute, Top: core.Pt(-1), Left: core.Pt(-1), Right: core.Pt(-1), Bottom: core.Pt(-1),
				CornerRadius: radiusControl + 1, BorderCurve: core.BorderCurveContinuous, BorderWidth: 2, BorderColor: c,
				Animated: &core.AnimatedStyle{Opacity: focus}}
		}})
		align := core.AlignCenter
		if in.Multiline {
			align = core.AlignStart
		}
		readonly := in.Readonly
		return core.View{Style: core.Style{Gap: 6, Animated: &core.AnimatedStyle{TranslateX: shake}}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8}, Children: head},
			core.View{StyleFunc: func() core.Style {
				border := colors.fieldBorder
				switch {
				case errText() != "":
					border = colors.danger
				case readonly:
					border = colors.line
				}
				s := core.Style{Direction: core.Row, AlignItems: align, CornerRadius: radiusControl, BorderCurve: core.BorderCurveContinuous,
					BorderWidth: 1, BorderColor: border, Background: colors.field}
				// Recessed while editable; a read-only field lies flat.
				if !readonly {
					s.Shadows = fin.field.shadows
				}
				return s
			}, Children: well},
			core.Show{When: func() bool { return errText() != "" }, Then: func(*core.Scope) core.Node {
				return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 7}, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(6), Height: core.Pt(6), CornerRadius: 3, Background: colors.danger}},
					core.Text{ContentFunc: errText, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 13, Color: colors.danger}},
				}}
			}},
		}}
	})
}

// pair is two nodes side by side, equal widths.
func pair(a, b core.Node) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{cell(a), cell(b)}}
}

// linkRowFirst is linkRow with a hairline above all but the first.
func linkRowFirst(first bool, name iconName, title, noteText string, onPress func()) core.Node {
	if first {
		return linkRow(name, title, noteText, onPress)
	}
	return core.View{Style: core.Style{Borders: core.Edges{Top: 1}, BorderColors: core.EdgeColors{Top: colors.line}},
		Children: []core.Node{linkRow(name, title, noteText, onPress)}}
}

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// strength is 0 to 4: length, mixed case, a digit, a symbol.
func strength(p string) int {
	if p == "" {
		return 0
	}
	var lower, upper, digit, symbol bool
	for _, r := range p {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		case !unicode.IsSpace(r) && r != '_':
			symbol = true
		}
	}
	n := 0
	for _, ok := range []bool{len([]rune(p)) >= 8, lower && upper, digit, symbol} {
		if ok {
			n++
		}
	}
	return n
}

var (
	strengthLabel = []string{"Empty", "Weak", "Fair", "Good", "Strong"}
	strengthTone  = []tone{toneNeutral, toneDanger, toneWarn, toneInfo, toneUp}
)

// formDemo is an account form in a scroll view: the focused input stays
// above the keyboard (on the platform), return moves to the next one,
// dragging the form can hide the keyboard; leaving a field or submitting
// checks it.
func formDemo(cx *core.Scope) core.Node {
	env := cx.Env()
	name, email, password := core.NewSignal(cx, ""), core.NewSignal(cx, ""), core.NewSignal(cx, "")
	code, shout, notes := core.NewSignal(cx, ""), core.NewSignal(cx, ""), core.NewSignal(cx, "")
	mode := core.NewSignal(cx, core.KeyboardDismissOnDrag)
	type status struct {
		t    tone
		text string
	}
	result := core.NewSignal(cx, status{})
	sheetOpen := core.NewSignal(cx, false)
	touched := core.NewSignalFunc(cx, map[string]bool{}, func(a, b map[string]bool) bool { return false })
	touch := func(key string) func() {
		return func() {
			touched.Update(func(t map[string]bool) map[string]bool {
				out := map[string]bool{key: true}
				for k, v := range t {
					out[k] = v
				}
				return out
			})
		}
	}
	// Inputs in return-key order.
	refs := make([]*core.Ref, 16)
	for i := range refs {
		refs[i] = &core.Ref{}
	}

	nameError := func() string {
		if touched.Get()["name"] && strings.TrimSpace(name.Get()) == "" {
			return "Enter your name"
		}
		return ""
	}
	emailError := func() string {
		if !touched.Get()["email"] || emailPattern.MatchString(email.Get()) {
			return ""
		}
		if email.Get() != "" {
			return "That address needs an @ and a domain"
		}
		return "Enter your e-mail"
	}
	passwordError := func() string {
		if touched.Get()["password"] && len([]rune(password.Get())) < 8 {
			return "Use at least 8 characters"
		}
		return ""
	}
	score := func() int { return strength(password.Get()) }

	submit := func() {
		touched.Set(map[string]bool{"name": true, "email": true, "password": true})
		issues := 0
		for _, e := range []string{nameError(), emailError(), passwordError()} {
			if e != "" {
				issues++
			}
		}
		cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)
		switch issues {
		case 0:
			result.Set(status{toneUp, fmt.Sprintf("Account created for %s <%s>", name.GetUntracked(), email.GetUntracked())})
		case 1:
			result.Set(status{toneDanger, "1 field needs attention"})
		default:
			result.Set(status{toneDanger, fmt.Sprintf("%d fields need attention", issues)})
		}
	}

	keyboard := func(cx *core.Scope) core.Node {
		modeName := map[core.KeyboardDismissMode]string{core.KeyboardDismissNone: "none", core.KeyboardDismissOnDrag: "on-drag", core.KeyboardDismissInteractive: "interactive"}
		return card(cardProps{icon: iconForm, title: "Keyboard", subtitle: "KeyboardDismissMode while dragging",
			right: core.Dynamic(func(*core.Scope) core.Node {
				h := env.KeyboardHeight()
				t := toneNeutral
				if h > 0 {
					t = toneAccent
				}
				return badge(fmt.Sprintf("%d pt", int(h+0.5)), t)
			})},
			segmented([]option[core.KeyboardDismissMode]{{value: core.KeyboardDismissNone, label: "None"}, {value: core.KeyboardDismissOnDrag, label: "On drag"},
				{value: core.KeyboardDismissInteractive, label: "Interactive"}}, mode.Get, mode.Set, false),
			caption(hint(cx, "Drag the form with the keyboard up", "Desktops have no on-screen keyboard to dismiss"), func() string { return modeName[mode.Get()] }),
		)
	}

	account := func(cx *core.Scope) core.Node {
		return section("Account", "AutoComplete",
			formField(fieldProps{label: "Name", next: refs[1], err: nameError},
				base.Input{Ref: refs[0], Value: name.Get, OnChange: name.Set, OnBlur: touch("name"), AutoCapitalize: core.AutoCapitalizeWords,
					AutoComplete: core.AutoCompleteName, Placeholder: "Ada Lovelace", TestID: "name"}),
			formField(fieldProps{label: "E-mail", api: "KeyboardEmail", next: refs[2], err: emailError},
				base.Input{Ref: refs[1], Value: email.Get, OnChange: email.Set, OnBlur: touch("email"), KeyboardType: core.KeyboardEmail,
					AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true, AutoComplete: core.AutoCompleteEmail, Placeholder: "ada@example.com", TestID: "email"}),
			core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
				formField(fieldProps{label: "Password", api: "Masked", next: refs[3], err: passwordError},
					base.Input{Ref: refs[2], Value: password.Get, OnChange: func(v string) {
						if strength(v) != core.Untracked(cx, score) {
							cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)
						}
						password.Set(v)
					}, OnBlur: touch("password"), Masked: true, AutoComplete: core.AutoCompleteNewPassword, Placeholder: "At least 8 characters", TestID: "password"}),
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
					core.View{Style: fill, Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
						s := score()
						return meter(func() float64 { return float64(s) / 4 }, strengthTone[s], -1)
					})}},
					core.Text{ContentFunc: func() string { return strengthLabel[score()] }, StyleFunc: func() core.Style {
						c := colors.faint
						if s := score(); s > 0 {
							c = strengthTone[s].color()
						}
						return core.Style{Width: core.Pt(52), TextAlign: core.TextAlignEnd, FontSize: 13, FontWeight: medium, Color: c}
					}},
				}},
			}},
			formField(fieldProps{label: "Phone", api: "KeyboardPhone", next: refs[4]},
				base.Input{Ref: refs[3], KeyboardType: core.KeyboardPhone, AutoComplete: core.AutoCompleteTel, Placeholder: "+1 555 0100"}),
			formField(fieldProps{label: "Website", api: "KeyboardURL", next: refs[5]},
				base.Input{Ref: refs[4], KeyboardType: core.KeyboardURL, AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true, Placeholder: "https://"}),
		)
	}

	numbers := func(cx *core.Scope) core.Node {
		return section("Numbers", "KeyboardType",
			pair(
				formField(fieldProps{label: "Age", hint: label("3 digits"), next: refs[6]},
					base.Input{Ref: refs[5], KeyboardType: core.KeyboardNumber, MaxLength: 3, Placeholder: "36"}),
				formField(fieldProps{label: "Amount", api: "KeyboardDecimal", prefix: "$", next: refs[7]},
					base.Input{Ref: refs[6], KeyboardType: core.KeyboardDecimal, Placeholder: "0.00"}),
			),
			formField(fieldProps{label: "Offset", hint: label("dark keyboard (iOS)")},
				base.Input{KeyboardAppearance: core.KeyboardAppearanceDark, Placeholder: "-12.5"}),
			formField(fieldProps{label: "Verification code", hint: func() string { return fmt.Sprintf("%d/6", len(code.Get())) }, next: refs[8]},
				base.Input{Ref: refs[7], TestID: "code", Value: code.Get, OnChange: code.Set, KeyboardType: core.KeyboardNumber, MaxLength: 6, Placeholder: "······",
					Style: core.Style{LetterSpacing: 10, FontSize: 22, FontWeight: medium, TextAlign: core.TextAlignCenter}}),
		)
	}

	shipping := func(cx *core.Scope) core.Node {
		return section("Shipping address", "scrolls into view",
			formField(fieldProps{label: "Street", next: refs[9]}, base.Input{Ref: refs[8], AutoComplete: core.AutoCompleteStreetAddress, Placeholder: "12 Analytical Way"}),
			pair(
				formField(fieldProps{label: "City", next: refs[10]}, base.Input{Ref: refs[9], AutoComplete: core.AutoCompleteCity, Placeholder: "London"}),
				formField(fieldProps{label: "Postal code", next: refs[11]}, base.Input{Ref: refs[10], AutoCapitalize: core.AutoCapitalizeCharacters,
					AutoComplete: core.AutoCompletePostalCode, Placeholder: "SW1A 1AA"}),
			),
			formField(fieldProps{label: "Country", next: refs[12]}, base.Input{Ref: refs[11], AutoComplete: core.AutoCompleteCountry, Placeholder: "United Kingdom"}),
		)
	}

	selection := func(cx *core.Scope) core.Node {
		return section("Selection and keys", "OnSelectionChange", selectionDemo())
	}

	textCard := func(cx *core.Scope) core.Node {
		return section("Text", "Input options",
			formField(fieldProps{label: "Controlled", hint: label("Uppercased as you type"), next: refs[13]},
				base.Input{Ref: refs[12], TestID: "shout", Value: shout.Get, OnChange: func(t string) { shout.Set(strings.ToUpper(t)) }, Placeholder: "TYPE IN LOWER CASE"}),
			formField(fieldProps{label: "Read only", api: "Readonly"}, base.Input{Value: label("Not editable"), Readonly: true}),
			formField(fieldProps{label: "Custom colors", api: "PlaceholderColor, SelectionColor", next: refs[14]},
				base.Input{Ref: refs[13], Placeholder: "A blue placeholder, an amber selection",
					Style: core.Style{PlaceholderColor: colors.info, SelectionColor: colors.warn}}),
			core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
				formField(fieldProps{label: "Notes", hint: func() string { return fmt.Sprintf("%d/280", len([]rune(notes.Get()))) }},
					base.Input{Ref: refs[14], Multiline: true, MaxLength: 280, Value: notes.Get, OnChange: notes.Set, Placeholder: "Return adds a line",
						Style: core.Style{MinHeight: core.Pt(100)}}),
				meter(func() float64 { return float64(len([]rune(notes.Get()))) / 280 }, toneAccent, -1),
			}},
			autoGrow(),
			formField(fieldProps{label: "No caret", api: "HideCaret"}, base.Input{HideCaret: true, DefaultValue: "No caret here"}),
		)
	}

	submitCard := func(cx *core.Scope) core.Node {
		return card(cardProps{icon: iconCheck, title: "Create the account", subtitle: "Submitting checks every field",
			right: core.Dynamic(func(*core.Scope) core.Node {
				r := result.Get()
				switch {
				case r.text == "":
					return core.Group{}
				case r.t == toneUp:
					return badge("Sent", toneUp)
				}
				return badge("Check", toneDanger)
			})},
			core.Show{When: func() bool { return result.Get().text != "" }, Then: func(*core.Scope) core.Node {
				return core.Dynamic(func(*core.Scope) core.Node {
					r := result.Get()
					fg, bg, name := colors.danger, tints.danger, iconBell
					if r.t == toneUp {
						fg, bg, name = colors.up, tints.up, iconCheck
					}
					return core.View{TestID: "form-status", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.All(12),
						CornerRadius: radiusControl, Background: bg}, Children: []core.Node{
						icon(name, 16, fg),
						core.Text{Content: r.text, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 14, LineHeight: 19, Color: fg}},
					}}
				})
			}},
			pair(button(buttonProps{label: "Submit"}, submit), button(buttonProps{label: "Hide keyboard", variant: secondary}, cx.DismissKeyboard)),
		)
	}

	more := func(cx *core.Scope) core.Node {
		return card(cardProps{title: "More keyboards", subtitle: "Sheets and bars that ride on the keyboard", flush: true},
			inset(insetProps{exact: true},
				linkRowFirst(true, iconComponents, "Inputs in a sheet", "Presentation: ModalPageSheet", func() { sheetOpen.Set(true) }),
				linkRowFirst(false, iconText, "A message bar at the bottom", "KeyboardAvoidingView", func() { shell.push(messageBar) }),
			))
	}

	var fields []core.Node
	for _, l := range []string{"First", "Second", "Third", "Fourth", "Fifth", "Sixth", "Seventh"} {
		fields = append(fields, formField(fieldProps{label: l}, base.Input{Placeholder: l + " field"}))
	}
	dialog := base.Dialog{Open: sheetOpen.Get, Presentation: core.ModalPageSheet, OnOpenChange: func(o bool, _ base.Reason) { sheetOpen.Set(o) },
		PopupStyle: core.Style{FlexGrow: 1, AlignSelf: core.AlignStretch}, Label: "A sheet",
		Children: []core.Node{base.SafeAreaView{Edges: base.Top | base.Bottom | base.Left | base.Right, Style: core.Style{FlexGrow: 1, Background: colors.background}, Children: []core.Node{
			core.Scroll{Style: core.Style{FlexGrow: 1}, Behavior: core.ScrollBehavior{KeyboardDismissMode: core.KeyboardDismissInteractive}, Children: []core.Node{
				core.View{Style: core.Style{Padding: core.Edges{Top: 16, Right: 16, Bottom: 40, Left: 16}, Gap: 16}, Children: []core.Node{
					core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Edges{Top: 8, Left: 4}}, Children: []core.Node{
						core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
							core.Text{Content: "A sheet", Style: core.Style{FontSize: 24, FontWeight: bold, LetterSpacing: -0.4, Color: colors.text}},
							text("Its inputs stay above the keyboard too", 13, regular, colors.muted),
						}},
						button(buttonProps{label: "Done", variant: secondary}, func() { sheetOpen.Set(false) }),
					}},
					inset(insetProps{}, fields...),
				}},
			}},
		}}}}

	return demoPage(cx, pageProps{behaviorFunc: func() core.ScrollBehavior { return core.ScrollBehavior{KeyboardDismissMode: mode.Get()} }, after: []core.Node{dialog}},
		keyboard, account, numbers, shipping, selection, textCard, submitCard, more)
}

// autoGrow is a multiline input that grows with its text
// (OnContentSizeChange), up to 5 lines.
func autoGrow() core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		height := core.NewSignal(cx, core.Points(0))
		size := core.NewSignal(cx, "–")
		return formField(fieldProps{label: "Grows with its text", hint: size.Get},
			base.Input{Multiline: true, Placeholder: "Type several lines",
				OnContentSizeChange: func(s core.Size) {
					size.Set(fmt.Sprintf("%d×%d", int(s.Width+0.5), int(s.Height+0.5)))
					height.Set(min(s.Height, 110))
				},
				StyleFunc: func() core.Style { return core.Style{Height: core.Pt(max(height.Get(), 20) + 26)} }})
	})
}

type message struct {
	mine bool
	text string
}

// messageBar is an input pinned to the bottom, outside any scroll view:
// it rides on the keyboard.
func messageBar(cx *core.Scope) base.NavPage {
	env := cx.Env()
	messages := core.NewSignalFunc(cx, []message{
		{false, "The bar below sits outside the scroll view."},
		{false, "KeyboardAvoidingView lifts it with the keyboard. Say something."},
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
	return screen("Message bar", nil, base.KeyboardAvoidingView{Style: core.Style{FlexGrow: 1, Background: colors.background}, Children: []core.Node{
		core.Scroll{Style: core.Style{FlexGrow: 1}, Behavior: core.ScrollBehavior{KeyboardDismissMode: core.KeyboardDismissInteractive}, Children: []core.Node{
			core.View{Style: core.Style{Padding: core.All(16), Gap: 8}, Children: []core.Node{
				core.Text{Content: "Today", Style: core.Style{AlignSelf: core.AlignCenter, FontSize: 12, FontWeight: medium, Color: colors.faint, Padding: core.Edges{Bottom: 8}}},
				core.Each(messages.Get, func(m message) message { return m }, func(cx *core.Scope, m func() message, _ func() int) core.Node {
					msg := core.Untracked(cx, m)
					return appear(0, false, bubble(msg))
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
				Background: colors.surface, Borders: core.Edges{Top: 1}, BorderColors: core.EdgeColors{Top: colors.border}}
		}, Children: []core.Node{
			base.Input{Value: draft.Get, OnChange: draft.Set, OnSubmit: func(string) { send() }, SubmitBehavior: core.SubmitBehaviorSubmit,
				Placeholder: "Message", ReturnKeyType: core.ReturnKeySend, TestID: "message",
				Style: fin.inset.over(core.Style{PlaceholderColor: colors.faint, SelectionColor: colors.accent, FlexGrow: 1, FlexShrink: 1, Height: core.Pt(44),
					Padding: core.Symmetric(0, 16), FontSize: 15, Color: colors.text, Background: colors.inset, BorderWidth: 1, BorderColor: colors.border, CornerRadius: 22})},
			roundButton(iconArrowUp, "Send", primary, 0, send),
		}},
	}})
}

// bubble is a chat message: orange on the right for mine, raised on the
// left for theirs.
func bubble(m message) core.Node {
	s := core.Style{AlignSelf: core.AlignStart, MaxWidth: core.Pct(80), Padding: core.Symmetric(10, 14), CornerRadii: core.Corners{TopLeft: 18, TopRight: 18, BottomRight: 18, BottomLeft: 6},
		BorderCurve: core.BorderCurveContinuous, Background: colors.raised}
	fg := colors.text
	if m.mine {
		s = fin.accent.over(s)
		s.AlignSelf, s.Background, s.CornerRadii = core.AlignEnd, colors.accentFill, core.Corners{TopLeft: 18, TopRight: 18, BottomRight: 6, BottomLeft: 18}
		fg = colors.onAccent
	} else {
		s = fin.raised.over(s)
	}
	return core.View{Style: s, Children: []core.Node{core.Text{Content: m.text, Style: core.Style{FontSize: 15, LineHeight: 20, Color: fg}}}}
}

// selectionDemo is OnSelectionChange, OnKeyPress, SelectedRange and
// Ref.SetSelection.
func selectionDemo() core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		value := core.NewSignal(cx, "Select part of this text")
		rng := core.NewSignal(cx, core.TextRange{})
		keys := core.NewSignalFunc(cx, []string(nil), slices.Equal)
		sel := core.NewSignal(cx, core.TextRange{})
		input := &core.Ref{}
		selected := func() string {
			// The range is in UTF-16 units; the demo's text is ASCII
			// until the user types otherwise.
			u := []rune(value.Get())
			r := rng.Get()
			if r.Start < 0 || r.End > len(u) || r.Start > r.End {
				return ""
			}
			return string(u[r.Start:r.End])
		}
		return core.View{Style: core.Style{Gap: 12}, Children: []core.Node{
			formField(fieldProps{label: "Text", hint: func() string { r := rng.Get(); return fmt.Sprintf("%d–%d", r.Start, r.End) }},
				base.Input{Ref: input, Value: value.Get, OnChange: value.Set, SelectedRange: sel.Get, OnSelectionChange: rng.Set, TestID: "selection",
					OnKeyPress: func(k string) {
						keys.Update(func(ks []string) []string {
							ks = append(slices.Clone(ks), k)
							return ks[max(0, len(ks)-8):]
						})
					}, AutoComplete: core.AutoCompleteOff}),
			caption("Selected", func() string { return fmt.Sprintf("%q", selected()) }),
			core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignCenter, Gap: 6, MinHeight: core.Pt(28)}, Children: []core.Node{
				core.Dynamic(func(*core.Scope) core.Node {
					ks := keys.Get()
					if len(ks) == 0 {
						return text("Keys you press show here", 13, regular, colors.faint)
					}
					var chips []core.Node
					for _, k := range ks {
						if k == " " {
							k = "␣"
						}
						chips = append(chips, core.View{Style: fin.raised.over(core.Style{MinWidth: core.Pt(28), Height: core.Pt(28), Padding: core.Symmetric(0, 8), CornerRadius: 7,
							AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: colors.raised, BorderWidth: 1, BorderColor: colors.border}),
							Children: []core.Node{text(k, 13, regular, colors.text)}})
					}
					return core.Group(chips)
				}),
			}},
			pair(
				button(buttonProps{label: "Select all"}, func() { sel.Set(core.TextRange{Start: 0, End: core.UTF16Len(value.GetUntracked())}) }),
				button(buttonProps{label: "Caret to start", variant: secondary}, func() { input.SetSelection(0, 0) }),
			),
			formField(fieldProps{label: "One-time code", api: "AutoCompleteOneTimeCode"},
				base.Input{KeyboardType: core.KeyboardNumber, AutoComplete: core.AutoCompleteOneTimeCode, MaxLength: 6, Placeholder: "123456"}),
		}}
	})
}
