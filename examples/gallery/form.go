package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"

	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/ui"
)

// fieldProps are a labelled input's: api names the property it shows,
// hint is a plain note and next is the input return focuses.
type fieldProps struct {
	label, api string
	hint       func() string
	next       *core.Ref
}

// formField is a labelled ui.Input: the label, the property it shows and a
// note above it.
func formField(p fieldProps, in base.Input) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		if p.next != nil {
			in.ReturnKeyType = core.ReturnKeyNext
			in.OnSubmit = func(string) { p.next.Focus() }
		} else if in.ReturnKeyType == 0 {
			in.ReturnKeyType = core.ReturnKeyDone
		}
		head := []core.Node{core.Text{Content: p.label, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 13, FontWeight: ui.Medium}, StyleFunc: ui.Colored(colors.Muted)}}
		if p.api != "" {
			head = append(head, API{Text: p.api})
		}
		if p.hint != nil {
			head = append(head, ui.Textf(p.hint, 13, ui.Regular, colors.Faint))
		}
		return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8}, Children: head},
			ui.Input(in),
		}}
	})
}

// pair is two nodes side by side, equal widths.
func pair(a, b core.Node) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{ui.Cell{Child: a}, ui.Cell{Child: b}}}
}

// autoGrow is a multiline input that grows with its text
// (OnContentSizeChange), up to 5 lines.
func autoGrow() core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		tr := locales.For(cx)
		height := core.NewSignal(cx, core.Points(0))
		size := core.NewSignal(cx, "–")
		return formField(fieldProps{label: tr.FormGrowsWithItsText(), hint: size.Get},
			base.Input{Multiline: true, Placeholder: tr.FormTypeSeveralLines(),
				OnContentSizeChange: func(s core.Size) {
					size.Set(fmt.Sprintf("%d×%d", int(s.Width+0.5), int(s.Height+0.5)))
					height.Set(min(s.Height, 110))
				},
				StyleFunc: func() core.Style { return core.Style{Height: core.Pt(max(height.Get(), 20) + 14)} }})
	})
}

// plainTitle is a title that reads as plain text but is a ui.Field with
// its box turned off.
func plainTitle() core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		tr := locales.For(cx)
		return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
			APILine{Label: tr.FormPlainTitle(), Names: "ui.Field, core.Zero"},
			ui.Field{TestID: "plain-title", DefaultValue: tr.FormUntitledDocument(), Placeholder: tr.FormUntitledDocument(), ReturnKeyType: core.ReturnKeyDone,
				Style: core.Style{Height: core.Pt(36), FontSize: 24, FontWeight: ui.Bold, LetterSpacing: -0.4,
					Background: core.Transparent, BorderWidth: core.Zero}},
		}}
	})
}

// inputDemo is ui's text fields: ui.Input and its base form, ui.Field,
// the fields for numbers, tags and codes, ui.Input's options and its
// selection.
func inputDemo(cx *core.Scope) core.Node {
	return demoPage(cx, pageProps{}, inputPlayground, fieldCard, numberInputCard, tagInputCard, otpInputCard, keyboardTypesCard, autofillCard, textCard, inputSelectionCard)
}

// fieldCard is ui.Field, sized by its Style.
func fieldCard(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	value := core.NewSignal(cx, "")
	multiline, disabled := core.NewSignal(cx, false), core.NewSignal(cx, false)
	return Playground{Title: "Field", UIOnly: true, Fill: true, Stage: func() core.Node {
		style := core.Style{Height: core.Pt(36), Padding: core.Symmetric(0, 12)}
		if multiline.Get() {
			style = core.Style{Height: core.Pt(100), Padding: core.Symmetric(10, 12)}
		}
		return ui.Field{TestID: "field", Value: value.Get, OnChange: value.Set, Placeholder: tr.CompTypeHere(), Multiline: multiline.Get(), Disabled: disabled.Get, Style: style}
	}, Knobs: []core.Node{boolKnob("Multiline", multiline), boolKnob("Disabled", disabled)}}
}

// keyboardTypesCard is a field for each KeyboardType, which picks the
// on-screen keyboard and what a hardware one may type.
func keyboardTypesCard(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	code := core.NewSignal(cx, "")
	return Section{Title: tr.FormKeyboardTypes(), Note: "KeyboardType", Children: []core.Node{
		formField(fieldProps{label: tr.FormEMail(), api: "KeyboardEmail"},
			base.Input{KeyboardType: core.KeyboardEmail, AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true, Placeholder: "ada@example.com"}),
		formField(fieldProps{label: tr.FormPhone(), api: "KeyboardPhone"}, base.Input{KeyboardType: core.KeyboardPhone, Placeholder: "+1 555 0100"}),
		formField(fieldProps{label: tr.FormWebsite(), api: "KeyboardURL"},
			base.Input{KeyboardType: core.KeyboardURL, AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true, Placeholder: "https://"}),
		pair(
			formField(fieldProps{label: tr.FormAge(), api: "KeyboardNumber"}, base.Input{KeyboardType: core.KeyboardNumber, MaxLength: 3, Placeholder: "36"}),
			formField(fieldProps{label: tr.FormAmount(), api: "KeyboardDecimal"}, base.Input{KeyboardType: core.KeyboardDecimal, Placeholder: "$0.00"}),
		),
		formField(fieldProps{label: tr.FormOffset(), hint: label(tr.FormDarkKeyboardIos())},
			base.Input{IOS: core.IOSInputProps{KeyboardAppearance: core.KeyboardAppearanceDark}, Placeholder: "-12.5"}),
		formField(fieldProps{label: tr.FormVerificationCode(), hint: func() string { return fmt.Sprintf("%d/6", len(code.Get())) }},
			base.Input{TestID: "code", Value: code.Get, OnChange: code.Set, KeyboardType: core.KeyboardNumber, MaxLength: 6, Placeholder: "000000",
				Style: core.Style{Height: core.Pt(44), LetterSpacing: 10, FontSize: 22, FontWeight: ui.Medium, TextAlign: core.TextAlignCenter}}),
	}}
}

// autofillCard is a field for each kind of AutoComplete, which the
// platform fills from what it has saved.
func autofillCard(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	return Section{Title: tr.FormAutofill(), Note: "AutoComplete", Children: []core.Node{
		formField(fieldProps{label: tr.FormName(), api: "AutoCompleteName"},
			base.Input{AutoComplete: core.AutoCompleteName, AutoCapitalize: core.AutoCapitalizeWords, Placeholder: "Ada Lovelace"}),
		formField(fieldProps{label: tr.FormPassword(), api: "Masked"},
			base.Input{Masked: true, AutoComplete: core.AutoCompleteNewPassword, Placeholder: tr.FormAtLeast8Characters()}),
		formField(fieldProps{label: tr.FormStreet(), api: "AutoCompleteStreetAddress"},
			base.Input{AutoComplete: core.AutoCompleteStreetAddress, Placeholder: "12 Analytical Way"}),
		pair(
			formField(fieldProps{label: tr.FormCity()}, base.Input{AutoComplete: core.AutoCompleteCity, Placeholder: "London"}),
			formField(fieldProps{label: tr.FormPostalCode()}, base.Input{AutoComplete: core.AutoCompletePostalCode, AutoCapitalize: core.AutoCapitalizeCharacters, Placeholder: "SW1A 1AA"}),
		),
		formField(fieldProps{label: tr.FormCountry()}, base.Input{AutoComplete: core.AutoCompleteCountry, Placeholder: "United Kingdom"}),
		formField(fieldProps{label: tr.FormOneTimeCode(), api: "AutoCompleteOneTimeCode"},
			base.Input{KeyboardType: core.KeyboardNumber, AutoComplete: core.AutoCompleteOneTimeCode, MaxLength: 6, Placeholder: "123456"}),
	}}
}

// textCard is ui.Input's options.
func textCard(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	colors := ui.ColorsOf(cx)
	shout, notes := core.NewSignal(cx, ""), core.NewSignal(cx, "")
	refs := []*core.Ref{{}, {}, {}}
	return Section{Title: tr.FormText(), Note: tr.FormInputOptions(), Children: []core.Node{
		formField(fieldProps{label: tr.FormControlled(), hint: label(tr.FormUppercasedAsYouType()), next: refs[1]},
			base.Input{Ref: refs[0], TestID: "shout", Value: shout.Get, OnChange: func(t string) { shout.Set(strings.ToUpper(t)) }, Placeholder: tr.FormTypeInLowerCase()}),
		formField(fieldProps{label: tr.FormReadOnly(), api: "Readonly"}, base.Input{Value: label(tr.FormNotEditable()), Readonly: true}),
		formField(fieldProps{label: tr.FormCustomColors(), api: "PlaceholderColor, SelectionColor", next: refs[2]},
			base.Input{Ref: refs[1], Placeholder: tr.FormABluePlaceholderAnAmber(),
				StyleFunc: func() core.Style { return core.Style{PlaceholderColor: colors.Info(), SelectionColor: colors.Warn()} }}),
		core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
			formField(fieldProps{label: tr.FormNotes(), hint: func() string { return fmt.Sprintf("%d/280", len([]rune(notes.Get()))) }},
				base.Input{Ref: refs[2], Multiline: true, MaxLength: 280, Value: notes.Get, OnChange: notes.Set, Placeholder: tr.FormReturnAddsALine(),
					Style: core.Style{Height: core.Pt(100)}}),
			ui.Meter{Value: func() float64 { return float64(len([]rune(notes.Get()))) / 280 }, Variant: ui.MeterAccent},
		}},
		autoGrow(),
		formField(fieldProps{label: tr.FormNoCaret(), api: "HideCaret"}, base.Input{HideCaret: true, DefaultValue: tr.FormNoCaretHere()}),
		formField(fieldProps{label: tr.FormNoPadding(), api: "core.Zero"},
			base.Input{DefaultValue: tr.FormNoPaddingHere(), Style: core.Style{Padding: core.All(core.Zero)}}),
		plainTitle(),
	}}
}

// inputSelectionCard is a field's selection and keys.
func inputSelectionCard(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	return Section{Title: tr.FormSelectionAndKeys(), Note: "OnSelectionChange", Children: []core.Node{selectionDemo()}}
}

// selectionDemo is OnSelectionChange, OnKeyPress, SelectedRange and
// Ref.SetSelection.
func selectionDemo() core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		tr := locales.For(cx)
		colors := ui.ColorsOf(cx)
		fin := ui.FinOf(cx)
		value := core.NewSignal(cx, tr.FormSelectPartOfThisText())
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
			formField(fieldProps{label: tr.FormText(), hint: func() string { r := rng.Get(); return fmt.Sprintf("%d–%d", r.Start, r.End) }},
				base.Input{Ref: input, Value: value.Get, OnChange: value.Set, SelectedRange: sel.Get, OnSelectionChange: rng.Set, TestID: "selection",
					OnKeyPress: func(k string) {
						keys.Update(func(ks []string) []string {
							ks = append(slices.Clone(ks), k)
							return ks[max(0, len(ks)-8):]
						})
					}, AutoComplete: core.AutoCompleteOff}),
			Caption{Label: tr.FormSelected(), Note: func() string { return fmt.Sprintf("%q", selected()) }},
			core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignCenter, Gap: 6, MinHeight: core.Pt(28)}, Children: []core.Node{
				core.Dynamic(func(*core.Scope) core.Node {
					ks := keys.Get()
					if len(ks) == 0 {
						return ui.Text(tr.FormKeysYouPressShowHere(), 13, ui.Regular, colors.Faint)
					}
					var chips []core.Node
					for _, k := range ks {
						if k == " " {
							k = "␣"
						}
						chips = append(chips, core.View{StyleFunc: func() core.Style {
							return fin.Raised().Over(core.Style{MinWidth: core.Pt(28), Height: core.Pt(28), Padding: core.Symmetric(0, 8), CornerRadius: 7,
								AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: colors.Raised(), BorderWidth: 1, BorderColor: colors.Border()})
						},
							Children: []core.Node{ui.Text(k, 13, ui.Regular, colors.Text)}})
					}
					return core.Group(chips)
				}),
			}},
			pair(
				ui.Button{Text: tr.FormSelectAll(), OnPress: func() { sel.Set(core.TextRange{Start: 0, End: core.UTF16Len(value.GetUntracked())}) }},
				ui.Button{Text: tr.FormCaretToStart(), Variant: ui.ButtonSecondary, OnPress: func() { input.SetSelection(0, 0) }},
			),
		}}
	})
}

// inputPlayground is ui.Input under its base form, with their props to set.
func inputPlayground(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	value := core.NewSignal(cx, "")
	multiline, masked, readonly, disabled := core.NewSignal(cx, false), core.NewSignal(cx, false), core.NewSignal(cx, false), core.NewSignal(cx, false)
	return Playground{Title: "Input", Fill: true, Base: baseInputForm(cx, value, multiline, masked, readonly, disabled), Stage: func() core.Node {
		return ui.Input{Value: value.Get, OnChange: value.Set, Placeholder: tr.CompTypeHere(), Multiline: multiline.Get(), Masked: masked.Get(),
			ReadonlyFunc: readonly.Get, Disabled: disabled.Get}
	}, BaseKnobs: []core.Node{boolKnob("Multiline", multiline), boolKnob("Masked", masked), boolKnob("Readonly", readonly), boolKnob("Disabled", disabled)}}
}

// numberInputCard is ui.NumberInput under its base form, with their props to
// set.
func numberInputCard(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	value, step := core.NewSignal(cx, 12.0), core.NewSignal(cx, 1.0)
	hide, disabled := core.NewSignal(cx, false), core.NewSignal(cx, false)
	return Playground{Title: "NumberInput", Base: baseNumberInputForm(cx, value, step, disabled), Stage: func() core.Node {
		return ui.NumberInput{Label: tr.LibFontSize(), Value: value.Get, OnChange: value.Set, Min: 0, Max: 100, Step: step.Get(), HideSteppers: hide.Get(),
			LeadingText: "#", Disabled: disabled.Get}
	}, BaseKnobs: []core.Node{numberKnob("Step", step, 1, 10, 1), boolKnob("Disabled", disabled)}, Knobs: []core.Node{boolKnob("HideSteppers", hide)}}
}

// tagInputCard is ui.TagInput with its props to set.
func tagInputCard(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	tags := core.NewSignalFunc(cx, []string{tr.LibApple(), tr.LibBanana()}, slices.Equal)
	disabled := core.NewSignal(cx, false)
	return Playground{Title: "TagInput", UIOnly: true, Fill: true, Stage: func() core.Node {
		return ui.TagInput{Label: tr.LibFruit(), Tags: tags.Get, OnChange: tags.Set, Placeholder: tr.LibPickFruit(), Disabled: disabled.Get}
	}, Knobs: []core.Node{boolKnob("Disabled", disabled)}}
}
