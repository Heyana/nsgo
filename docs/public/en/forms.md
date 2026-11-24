---
title: Forms
description: Text fields, focus, the software keyboard, switches, pickers and validation.
group: Core concepts
order: 57
---

## Text fields

`ui.Input` is the platform's text field in `ui`'s look, with `base.Input`'s fields. With `Value` it is controlled: the app holds the value and updates it in `OnChange`; with `DefaultValue` it keeps its own.

```go
name, email := core.NewSignal(cx, ""), &core.Ref{}

ui.Input{
	Value:         name.Get,
	OnChange:      name.Set,
	Placeholder:   "Your name",
	ReturnKeyType: core.ReturnKeyNext,
	OnSubmit:      func(string) { email.Focus() },
}
```

`Masked` hides the characters, `Readonly` stops edits while the text still selects and copies (in mini programs only in a WeChat single-line field: a WeChat multiline field and Alipay's fields turn off as `Disabled` does, since those hosts cannot undo an edit), `Disabled` turns the field off, and `Multiline` wraps the text and grows with it unless its style sizes it. A multiline field's lines, and its placeholder's, are as tall as its style's `LineHeight`, the font centered in each as in a `core.Text`; a single-line field keeps its line centered in its height. `KeyboardType` picks the software keyboard (`core.KeyboardEmail`, `core.KeyboardNumber`, `core.KeyboardURL` and the rest), `AutoComplete` tells the system what the field holds for autofill, and `MaxLength` caps the UTF-16 units the user enters. `AutoCapitalize` and `NoAutoCorrect` tune the keyboard's behavior.

`OnSelectionChange` and `OnCaretMove` report the selection and where the caret is, for a menu that opens at it. `Suggest`, a `core.Suggest`, follows a query after a trigger such as `@`, and a `ui.SuggestList` beside the field offers rows for it at the caret, which Up, Down, Return and a press choose from, as `chat.Composer`'s `Suggest` does for mentions. `OnKeyPress` hears each key, `OnPaste` what the user pastes, and `OnCompositionChange` follows an input method.

On iOS and Android, `cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)` takes a software Return before the field submits, blurs or inserts a newline. `KeyAuto` and `KeyNotEditing` leave it to `SubmitBehavior`; on iOS, a Return that commits marked text stays with the input method.

## Focus and the keyboard

`Ref.Focus()` focuses a field or, on desktops, the web and Apple TV, an enabled pressable or `Focusable` view. `Ref.Blur()` clears only its own view's focus, and `cx.DismissKeyboard()` closes the software keyboard. `OnFocus` and `OnBlur` follow the focus, and `View.OnFocus`/`OnBlur` do the same for any focusable view. A focused view that leaves the tree, as its `Show` closes or its row goes, hears `OnBlur` once, after the commit that removed it or, when the platform reports the next focus first, before that view's `OnFocus`, and the views around it that stay hear `OnFocusOut`.

A `Ref`'s focus, blur, scroll and zoom requests do nothing while its element is in a `Holder`'s content that no place shows, and those still queued when the element leaves the tree are dropped. Showing the content again does not replay them: ask again once it is placed.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Software keyboards cover the bottom of a phone. `core.EnvKeyboard` as a bottom padding or margin keeps content above it, and its move is animated with the keyboard:

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

A scroll view around a form can dismiss the keyboard when dragged and decide what a tap does while the keyboard shows:

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

`cx.Env().KeyboardHeight()` and `cx.Env().KeyboardDuration()` read the keyboard's size and timing in code, and `cx.Env().SafeArea()` the insets around the screen.

## Choices

`ui.Switch` is a setting that takes effect at once, `ui.Checkbox` is checked, unchecked or indeterminate, and `ui.Radio` is one of a `ui.RadioGroup`. `ui.Select[T]` opens a list of options under its trigger, or as a sheet when its `Presentation` asks, and `ui.Tabs[T]` shows a few at once.

```go
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}

size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "Size",
}
```

`ui.NumberInput` uses `Step` for buttons, arrow keys and scrubbing. When a typed value is committed, it rounds to `Precision` from `Min`; zero `Precision` uses `Step`. Set `NoSnap: true` to clamp typed values to `Min` and `Max` without rounding. The default `Format` shows the rounding grid's decimal places, or the shortest round-trip decimal with `NoSnap`.

```go
gravity := core.NewSignal(cx, 9.81)
ui.NumberInput{Value: gravity.Get, OnChange: gravity.Set, Min: 0, Max: 100, Step: 0.1, Precision: 0.01, Label: "Gravity"}
```

`ui.NumberInput` and `base.NumberInput` accept `LabelFunc` for a changing accessible name. A custom `Format` follows the signals it reads while the field is not being edited; a language switch preserves the active draft until editing ends.

`ui.DatePicker` and `ui.TimeField` edit dates and times, `ui.ColorPicker` edits a color, and `ui.TagInput` edits a list of tags.

`ui.OtpInput` takes a one-time code in cells. It and `base.OtpInput` accept `LabelFunc` without replacing the focused field or its code. `Size` sets the default height and width, 36 points on desktops and TVs and 44 on touch devices. `CellWidth` and `CellHeight` override either dimension independently. `TextStyle` overrides the cell typography, `CornerRadius` changes its corners, and `CaretHeight` overrides the height derived from the line height.

## Grouped rows

`ui.FormSection` puts its rows on a card, with the title above and footer below. Set `CardStyle` for the card’s background, border or corners. `CardStyleFunc` merges over it and follows the signals it reads. `DividerInset` receives each child row, including rows made by `core.Each`: return an inset and `true` to override its divider, or `false` to keep automatic title alignment (12 points for other nodes). An inset of zero makes a full-width divider.

`ui.FormRow` places `Leading` or an icon tile before the title. Set `LeadingWidth` to reserve space for your node and align the divider below it with the title. The slot defaults to 24 points. Set `TileSize` to scale the icon tile, glyph and corners together. `TitleStyle` and `DetailStyle` override the title and detail typography. `TitleMaxLines` caps the title (zero is unlimited), `MinHeight` overrides the row’s minimum height in points, and `Below` places a node after the title and detail in their text column. Zero `MinHeight` keeps the default: 40 points, or 48 on touch devices.

For a single-choice set, wrap the section in `base.RadioGroup` and set each row’s `Role: core.RoleRadio`, `Checked` and `OnPress`. The row uses `base.Radio` for its checked and selected accessibility states, one Tab stop and arrow-key selection; pressing an already checked row does nothing. The app owns the chosen value.

`ui.FormSection.TitleFunc` and `FooterFunc`, and `ui.FormRow.TitleFunc`, `DetailFunc` and `LabelFunc`, update their words without rebuilding the row. An empty detail, section title or footer removes that optional text; an empty row label uses its current title. A searchable `ui.Select` binds `SearchPlaceholderFunc` and `EmptyFunc` while keeping its open search and selection.

## Validation

The app owns the rules: a signal holds the error, `OnBlur` or `OnSubmit` sets it, and a `core.Show` adds a message under the field while there is one.

```go
address := core.NewSignal(cx, "")
err := core.NewSignal(cx, "")
colors := ui.ColorsOf(cx)
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{
		Value:    address.Get,
		OnChange: func(v string) { address.Set(v); err.Set("") },
		Label:    "Email",
		OnSubmit: func(string) {
			if !strings.Contains(address.Get(), "@") {
				err.Set("Enter an email address.")
			}
		},
	},
	core.Show{
		When: func() bool { return err.Get() != "" },
		Then: func(*core.Scope) core.Node { return ui.Textf(err.Get, 12, ui.Regular, colors.Danger) },
	},
}}
```

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` animates a message that appears, and `OnSubmit` with `ReturnKeyNext` moves the keyboard to the next field.

## Next

- [Navigation](navigation.md): where a form lives.
- [Text](text.md): how a field's text is styled.
- [Accessibility](accessibility.md): labels and values screen readers need.
